// Package gm orchestrates the LLM around the rules engine: prompts, tool
// schemas, campaign/character/location authoring, and the
// interpret -> resolve -> direct -> narrate -> judge -> enemies -> save
// pipeline. The LLM never mutates state; every tool call it makes is
// validated and applied by code.
package gm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Solvely-Colin/aidnd/internal/dice"
	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/llm"
	"github.com/Solvely-Colin/aidnd/internal/store"
)

// Errors the API maps to status codes.
var (
	ErrNotFound    = store.ErrNotFound
	ErrConflict    = errors.New("conflict")
	ErrNotYourTurn = errors.New("not your turn")
	ErrForbidden   = errors.New("forbidden")
)

// Conflict wraps a message as an ErrConflict.
func Conflict(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrConflict, fmt.Sprintf(format, a...))
}

// Config holds tunables (from environment variables in cmd/server).
type Config struct {
	SceneTurnCap        int  // quiet turns before progress is forced (default 10)
	MaxPlayers          int  // default 4
	MaxSetbacks         int  // default 3
	ObjectiveCheckEvery int  // run the freeform judge every N turns (default 1)
	LoreBudget          int  // characters of lore per narrator prompt (default 1800)
	DebugDirector       bool // log director decisions
	SkipAfterSeconds    int  // players may skip an idle turn after this (default 120)
	SummaryEvery        int  // regenerate story-so-far every N turns (default 10)
	RecentEvents        int  // log lines given to the LLM (default 12)
	// PlayerDice makes turns that need a player's d20 wait for that player
	// to roll it (unless they chose auto-roll). Off, the engine rolls at once.
	PlayerDice bool
	// TurnDeadline is how long an async fight waits for a player before
	// autopilot plays their turn (default 12h).
	TurnDeadline time.Duration
}

func (c *Config) defaults() {
	if c.TurnDeadline <= 0 {
		c.TurnDeadline = DefaultTurnDeadline
	}
	if c.SceneTurnCap <= 0 {
		c.SceneTurnCap = 10
	}
	if c.MaxPlayers <= 0 || c.MaxPlayers > 4 {
		c.MaxPlayers = 4
	}
	if c.MaxSetbacks <= 0 {
		c.MaxSetbacks = 3
	}
	if c.ObjectiveCheckEvery <= 0 {
		c.ObjectiveCheckEvery = 1
	}
	if c.LoreBudget <= 0 {
		c.LoreBudget = 1800
	}
	if c.SkipAfterSeconds <= 0 {
		c.SkipAfterSeconds = 120
	}
	if c.SummaryEvery <= 0 {
		c.SummaryEvery = 10
	}
	if c.RecentEvents <= 0 {
		c.RecentEvents = 12
	}
}

// GM is the orchestrator. It is safe for concurrent use; each campaign's
// pipeline is serialized by a per-campaign mutex.
type GM struct {
	LLM   llm.Client
	Store *store.Store
	Cfg   Config
	Dice  *dice.Roller
	Now   func() time.Time
	Log   *log.Logger
	NewID func() string
	// Publish is called after every committed save with the new events.
	Publish func(campaignID string, events []store.Event)

	locks sync.Map // campaign id -> *sync.Mutex
}

// New builds a GM with defaults filled in.
func New(client llm.Client, st *store.Store, cfg Config, roller *dice.Roller) *GM {
	cfg.defaults()
	if roller == nil {
		roller = dice.New(time.Now().UnixNano())
	}
	return &GM{LLM: client, Store: st, Cfg: cfg, Dice: roller, Now: time.Now, Log: log.Default(), NewID: randomID}
}

func randomID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "c" + hex.EncodeToString(b)
}

func (g *GM) lock(id string) func() {
	m, _ := g.locks.LoadOrStore(id, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// commit saves the world with events and publishes them.
func (g *GM) commit(ctx context.Context, w *game.World, evs ...store.Event) ([]store.Event, error) {
	evs = append(evs, eventEvents(w)...)
	if party := w.Focused(); party != "" {
		for i := range evs {
			// the shared story is everyone's news
			if t := evs[i].Type; t != "quest_updated" && t != "campaign_ended" && t != "world_event" {
				evs[i] = tagParty(evs[i], party)
			}
		}
	}
	game.DropStaleDeathSaves(w)
	if ch := game.RepairStory(w); len(ch) > 0 {
		g.logf("story repaired: %v", ch) // a death outside a fight, or a campaign saved before the repair existed
	}
	saved, err := g.Store.Save(ctx, w, evs...)
	if err != nil {
		return nil, err
	}
	if g.Publish != nil && len(saved) > 0 {
		g.Publish(w.Campaign.ID, saved)
	}
	return saved, nil
}

// tagParty marks an event with the group it happened to, so players in
// other groups can leave it out of their feed.
func tagParty(e store.Event, party string) store.Event {
	var m map[string]any
	if len(e.Data) == 0 {
		m = map[string]any{}
	} else if err := json.Unmarshal(e.Data, &m); err != nil || m == nil {
		return e
	}
	if _, ok := m["party"]; ok {
		return e
	}
	m["party"] = party
	if b, err := json.Marshal(m); err == nil {
		e.Data = b
	}
	return e
}

// Load reads a campaign (no lock; for read-only views).
func (g *GM) Load(ctx context.Context, id string) (*game.World, error) {
	return g.Store.Load(ctx, id)
}

func (g *GM) debugf(format string, a ...any) {
	if g.Cfg.DebugDirector && g.Log != nil {
		g.Log.Printf("[director] "+format, a...)
	}
}

func (g *GM) logf(format string, a ...any) {
	if g.Log != nil {
		g.Log.Printf(format, a...)
	}
}

// Join seats a player. It is idempotent per player id. It fails with
// ErrConflict if the table is full or the story has ended. A player who
// joins a campaign under way makes a character and is written in where
// the party stands (see joinRunning).
func (g *GM) Join(ctx context.Context, campaignID, playerID, name string) (*game.Player, bool, error) {
	defer g.lock(campaignID)()
	w, err := g.Store.Load(ctx, campaignID)
	if err != nil {
		return nil, false, err
	}
	if p := w.Player(playerID); p != nil {
		return p, false, nil
	}
	if w.Campaign.Status != game.StatusLobby && w.Campaign.Status != game.StatusActive {
		return nil, false, Conflict("the story has ended")
	}
	max := w.Campaign.MaxPlayers
	if max <= 0 {
		max = g.Cfg.MaxPlayers
	}
	if len(w.Players) >= max {
		return nil, false, Conflict("the table is full (%d players)", max)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = fmt.Sprintf("Player %d", len(w.Players)+1)
	}
	if len(name) > 40 {
		name = name[:40]
	}
	p := &game.Player{ID: playerID, Name: name, Token: randomID() + randomID(), JoinedTurn: w.Campaign.Turn}
	w.Players = append(w.Players, p)
	_, err = g.commit(ctx, w, store.NewEvent("player_joined", playerID, map[string]any{"player_id": playerID, "name": name, "seat": len(w.Players)}))
	return p, true, err
}

// SaveCharacter validates and stores a sheet for the player. Players can
// replace their character until the game starts.
func (g *GM) SaveCharacter(ctx context.Context, campaignID, playerID string, spec game.CharacterSpec) (*game.Character, error) {
	defer g.lock(campaignID)()
	w, err := g.Store.Load(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	p := w.Player(playerID)
	if p == nil {
		return nil, ErrForbidden
	}
	if w.Campaign.Status == game.StatusActive {
		old := w.CharacterOf(playerID)
		if old != nil && old.Dead {
			return g.replaceDead(ctx, w, playerID, old, spec)
		}
		if old == nil {
			return g.joinRunning(ctx, w, playerID, spec)
		}
	}
	if w.Campaign.Status != game.StatusLobby {
		return nil, Conflict("characters are locked once the campaign starts")
	}
	for _, other := range w.Characters {
		if other.PlayerID != playerID && strings.EqualFold(strings.TrimSpace(other.Name), strings.TrimSpace(spec.Name)) {
			return nil, Conflict("%s is already in the party; pick another name", other.Name)
		}
	}
	old := w.CharacterOf(playerID)
	if old != nil {
		removeCharacter(w, old.ID)
	}
	c, err := game.CreateCharacter(w, playerID, spec)
	if err != nil {
		return nil, err
	}
	_, err = g.commit(ctx, w, store.NewEvent("character_created", playerID, map[string]any{
		"player_id": playerID, "character_id": c.ID, "name": c.Name, "class": c.Class, "kit": c.Kit, "concept": c.Concept, "replaces": replaced(old)}))
	return c, err
}

func replaced(c *game.Character) string {
	if c == nil {
		return ""
	}
	return c.ID
}

func removeCharacter(w *game.World, id string) {
	for i, c := range w.Characters {
		if c.ID == id {
			w.Characters = append(w.Characters[:i], w.Characters[i+1:]...)
			break
		}
	}
	items := w.Items[:0]
	for _, it := range w.Items {
		if !(it.Owner.Kind == game.OwnerCharacter && it.Owner.ID == id) {
			items = append(items, it)
		}
	}
	w.Items = items
}

// CreateCharacterFromConcept drafts with the LLM and saves in one go.
func (g *GM) CreateCharacterFromConcept(ctx context.Context, campaignID, playerID, concept string) (*game.Character, error) {
	w, err := g.Store.Load(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if w.Player(playerID) == nil {
		return nil, ErrForbidden
	}
	spec := g.DraftCharacter(ctx, w, concept)
	return g.SaveCharacter(ctx, campaignID, playerID, spec)
}

// replaceDead brings in a new character for a player whose character died.
// They join where the party stands, with the dead character's XP, and take
// the same seat; the fallen character's belongings are left where they lie.
func (g *GM) replaceDead(ctx context.Context, w *game.World, playerID string, old *game.Character, spec game.CharacterSpec) (*game.Character, error) {
	for _, other := range w.Characters {
		if other.ID != old.ID && strings.EqualFold(strings.TrimSpace(other.Name), strings.TrimSpace(spec.Name)) {
			return nil, Conflict("%s is already in the party; pick another name", other.Name)
		}
	}
	// they join the dead character's group, where it stands
	if err := w.FocusPlayer(playerID); err != nil {
		return nil, err
	}
	for _, it := range w.ItemsOf(game.OwnerCharacter, old.ID) {
		it.Owner = game.Owner{Kind: game.OwnerLocation, ID: w.Campaign.LocationID}
	}
	zone, seat := old.ZoneID, -1
	for i, id := range w.Campaign.Turns.Order {
		if id == old.ID {
			seat = i
		}
	}
	removeCharacter(w, old.ID)
	c, err := game.CreateCharacter(w, playerID, spec)
	if err != nil {
		return nil, err
	}
	c.Group = old.Group
	game.JoinMidway(w, c, old.XP, zone)
	if seat >= 0 {
		w.Campaign.Turns.Order[seat] = c.ID
	}
	_, err = g.commit(ctx, w, store.NewEvent("character_created", playerID, map[string]any{
		"player_id": playerID, "character_id": c.ID, "name": c.Name, "class": c.Class, "kit": c.Kit, "concept": c.Concept, "replaces": old.ID, "joins_midway": true}))
	return c, err
}
