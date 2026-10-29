package gm

import (
	"context"
	"fmt"
	"strings"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/llm"
	"github.com/Solvely-Colin/aidnd/internal/store"
)

// Actor identifies whose turn it is.
type Actor struct {
	CharacterID string `json:"character_id"`
	PlayerID    string `json:"player_id"`
	Name        string `json:"name"`
}

func actorOf(c *game.Character) *Actor {
	if c == nil {
		return nil
	}
	return &Actor{CharacterID: c.ID, PlayerID: c.PlayerID, Name: c.Name}
}

// TurnResult is returned by POST /actions.
type TurnResult struct {
	Resolution     *game.Resolution `json:"resolution"`
	Narration      string           `json:"narration"`
	Quick          bool             `json:"quick"`
	WorldChanges   []string         `json:"world_changes,omitempty"`
	EnemyTurns     []game.EnemyTurn `json:"enemy_turns,omitempty"`
	EnemyNarration string           `json:"enemy_narration,omitempty"`
	Setback        []string         `json:"setback,omitempty"`
	TurnEnded      bool             `json:"turn_ended"`
	Budget         *game.Budget     `json:"budget,omitempty"`
	Skipped        []game.Step      `json:"skipped,omitempty"`
	NextActor      *Actor           `json:"next_actor,omitempty"`
	CampaignEnded  bool             `json:"campaign_ended"`
	Status         string           `json:"status"`
}

func (g *GM) recent(ctx context.Context, w *game.World) []string {
	evs, err := g.Store.RecentEvents(ctx, w.Campaign.ID, g.Cfg.RecentEvents*2, "player_action", "narration", "enemy_turns", "location_changed", "quest_updated")
	if err != nil {
		return nil
	}
	lines := eventLines(evs, w)
	if len(lines) > g.Cfg.RecentEvents {
		lines = lines[len(lines)-g.Cfg.RecentEvents:]
	}
	return lines
}

// Start places the party at the hub, lays out zones, narrates the opening
// and begins the first turn. 409 until everyone has a character.
func (g *GM) Start(ctx context.Context, campaignID string) (*game.World, string, error) {
	defer g.lock(campaignID)()
	w, err := g.Store.Load(ctx, campaignID)
	if err != nil {
		return nil, "", err
	}
	if w.Campaign.Status != game.StatusLobby {
		return nil, "", Conflict("the campaign has already started")
	}
	if len(w.Players) == 0 {
		return nil, "", Conflict("nobody has joined yet")
	}
	var missing []string
	for _, p := range w.Players {
		if p.CharacterID == "" || w.Character(p.CharacterID) == nil {
			missing = append(missing, p.Name)
		}
	}
	if len(missing) > 0 {
		return nil, "", Conflict("waiting for characters from: %s", strings.Join(missing, ", "))
	}
	hub := w.Location(w.Campaign.HubID)
	w.Campaign.LocationID = hub.ID
	hub.Visited = true
	if len(hub.Zones) == 0 {
		g.defineZones(ctx, w, hub)
	}
	for _, c := range w.Characters {
		c.ZoneID = hub.Zones[0].ID
	}
	for _, n := range w.NPCsAt(hub.ID) {
		if hub.Zone(n.ZoneID) == nil {
			n.ZoneID = hub.Zones[len(hub.Zones)-1].ID
		}
	}
	if err := game.StartGame(w, g.Now()); err != nil {
		return nil, "", Conflict("%v", err)
	}
	var party []string
	for _, c := range w.Party() {
		party = append(party, fmt.Sprintf("%s (%s; %s)", c.Name, c.Class, c.Concept))
	}
	dir := Direction{}
	dir.MainObjective, dir.SceneGoal, dir.RouteHint = g.sceneGoal(w)
	w.Campaign.Scene.Goal = dir.SceneGoal
	resp, err := g.LLM.Chat(ctx, llm.Request{Tag: "narrate_opening", Temperature: 0.8, MaxTokens: 500, Messages: []llm.Message{
		llm.System(narratorSystem),
		llm.User(fmt.Sprintf("%s\nSTATE:\n%s\nThe party: %s.\nWrite the opening narration (60-120 words): where they are, why they are here (the pitch), and the hook toward: %s. End on the situation.",
			loreBlock(selectLore(w, w.Campaign.Pitch, g.Cfg.LoreBudget)), snapshot(w, nil, true), strings.Join(party, ", "), dir.SceneGoal)),
	}})
	opening := strings.TrimSpace(resp.Content)
	if err != nil || opening == "" {
		opening = fmt.Sprintf("%s %s", w.Campaign.Pitch, hub.Description)
	}
	first := w.ActiveCharacter()
	_, err = g.commit(ctx, w,
		store.NewEvent("location_changed", "", map[string]any{"location_id": hub.ID, "name": hub.Name}),
		store.NewEvent("narration", "", map[string]any{"text": opening, "opening": true}),
		store.NewEvent("turn_started", first.PlayerID, map[string]any{"character_id": first.ID, "player_id": first.PlayerID, "name": first.Name, "budget": w.Campaign.Turns.Budget}))
	return w, opening, err
}

// TakeAction runs the full pipeline for the active player's message.
func (g *GM) TakeAction(ctx context.Context, campaignID, playerID, text string, hints Hints) (*TurnResult, error) {
	defer g.lock(campaignID)()
	w, err := g.Store.Load(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if w.Campaign.Status != game.StatusActive {
		return nil, Conflict("the campaign is %s", w.Campaign.Status)
	}
	actor := w.ActiveCharacter()
	if actor == nil || actor.PlayerID != playerID {
		return nil, ErrNotYourTurn
	}
	text = trim(text, 600)
	if text == "" {
		return nil, fmt.Errorf("%w: text is required", ErrBadRequest)
	}
	recent := g.recent(ctx, w)

	// 1. interpret
	actions := g.interpret(ctx, w, actor, text, hints, recent)
	// 2. resolve
	onArrive, ambush := g.arrivalHooks(ctx, w)
	prevLoc := w.Campaign.LocationID
	res := game.Resolve(&game.Ctx{W: w, R: g.Dice, Now: g.Now(), OnArrive: onArrive, Ambush: ambush}, actor, actions)
	noteSceneTurn(w, res)
	quick := res.AllFree && !res.TurnEnded
	out := &TurnResult{Resolution: res, Quick: quick, Skipped: res.Skipped}
	// 3. direct
	recentText := strings.Join(append(append([]string{}, recent...), actor.Name+": "+text), "\n") + "\n" + strings.Join(res.Changes, "\n")
	var dir Direction
	if !quick {
		dir = g.direct(ctx, w, res, recentText)
	}
	// 4. narrate
	nar := g.narrate(ctx, w, actor, text, res, dir, recent, quick)
	out.Narration = nar.Text
	out.WorldChanges = append(append(out.WorldChanges, dir.BeatChanges...), nar.Changes...)

	evs := []store.Event{
		store.NewEvent("player_action", playerID, map[string]any{"character_id": actor.ID, "character": actor.Name, "text": text, "hints": hints}),
		store.NewEvent("action_resolved", playerID, map[string]any{"character_id": actor.ID, "steps": res.Steps, "skipped": res.Skipped, "changes": res.Changes, "rolls": res.Rolls(), "world_changes": out.WorldChanges}),
		store.NewEvent("narration", "", map[string]any{"text": nar.Text, "quick": quick, "skipped": res.Skipped, "character_id": actor.ID}),
	}
	quests := append([]game.QuestUpdate{}, res.Quests...)
	// 5. judge freeform objectives
	if !quick && w.Campaign.Status == game.StatusActive && (w.Campaign.Turn+1)%g.Cfg.ObjectiveCheckEvery == 0 {
		ups, ch := g.judge(ctx, w, recentText+"\n"+nar.Text)
		quests = append(quests, ups...)
		out.WorldChanges = append(out.WorldChanges, ch...)
	}
	// quests may also tick from directives (e.g. a gift completing obtain_item)
	if ups, ch := game.EvaluateQuests(w); len(ups) > 0 {
		quests = append(quests, ups...)
		out.WorldChanges = append(out.WorldChanges, ch...)
	}
	for _, q := range quests {
		evs = append(evs, store.NewEvent("quest_updated", "", q))
	}
	if w.Campaign.LocationID != prevLoc {
		l := w.Here()
		evs = append(evs, store.NewEvent("location_changed", "", map[string]any{"location_id": l.ID, "name": l.Name}))
	}
	// 6. enemy turns, setbacks, next actor
	if res.TurnEnded && w.Campaign.Status == game.StatusActive {
		evs = append(evs, g.endTurn(ctx, w, out)...)
	} else if !res.TurnEnded {
		b := w.Campaign.Turns.Budget
		out.Budget = &b
		evs = append(evs, store.NewEvent("turn_budget", playerID, map[string]any{"character_id": actor.ID, "budget": b}))
	}
	out.TurnEnded = res.TurnEnded
	if w.Campaign.Status == game.StatusCompleted || w.Campaign.Status == game.StatusFailed {
		out.CampaignEnded = true
		evs = append(evs, store.NewEvent("campaign_ended", "", map[string]any{"status": w.Campaign.Status}))
	}
	out.Status = w.Campaign.Status
	// story-so-far every N turns
	if out.TurnEnded && w.Campaign.Turn-w.Campaign.SummaryTurn >= g.Cfg.SummaryEvery {
		g.summarize(ctx, w, append(recent, actor.Name+": "+text, "GM: "+nar.Text))
	}
	// 7. save + broadcast
	if _, err := g.commit(ctx, w, evs...); err != nil {
		return nil, err
	}
	return out, nil
}

// ErrBadRequest marks invalid input.
var ErrBadRequest = fmt.Errorf("bad request")

// endTurn runs enemy turns if in combat, handles a wipe, and advances to
// the next actor.
func (g *GM) endTurn(ctx context.Context, w *game.World, out *TurnResult) []store.Event {
	var evs []store.Event
	if w.InCombat() {
		turns, _ := game.RunEnemyTurns(w, g.Dice)
		if len(turns) > 0 {
			out.EnemyTurns = turns
			out.EnemyNarration = g.narrateEnemies(ctx, w, turns, out.Narration)
			evs = append(evs, store.NewEvent("enemy_turns", "", map[string]any{"turns": turns, "narration": out.EnemyNarration}))
		}
	}
	if game.AllDown(w) {
		out.Setback = game.Setback(w)
		evs = append(evs, store.NewEvent("narration", "", map[string]any{"text": strings.Join(out.Setback, " "), "setback": true}))
		if w.Campaign.Status == game.StatusFailed {
			return evs
		}
		hub := w.Here()
		evs = append(evs, store.NewEvent("location_changed", "", map[string]any{"location_id": hub.ID, "name": hub.Name}))
	}
	next := game.EndTurn(w, g.Now())
	if next == nil {
		out.Setback = append(out.Setback, game.Setback(w)...)
		next = game.EndTurn(w, g.Now())
	}
	if next != nil {
		out.NextActor = actorOf(next)
		evs = append(evs, store.NewEvent("turn_started", next.PlayerID, map[string]any{"character_id": next.ID, "player_id": next.PlayerID, "name": next.Name, "budget": w.Campaign.Turns.Budget}))
	}
	return evs
}

// SkipMode says who is asking to skip.
type SkipMode int

const (
	SkipAsPlayer  SkipMode = iota // the player whose turn it is, or anyone once idle
	SkipAsService                 // the host backend may skip anyone at any time
)

// Skip ends the current turn. In combat, enemies still act.
func (g *GM) Skip(ctx context.Context, campaignID, playerID string, mode SkipMode) (*TurnResult, error) {
	defer g.lock(campaignID)()
	w, err := g.Store.Load(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if w.Campaign.Status != game.StatusActive {
		return nil, Conflict("the campaign is %s", w.Campaign.Status)
	}
	actor := w.ActiveCharacter()
	if actor == nil {
		return nil, Conflict("no active turn")
	}
	if mode != SkipAsService && actor.PlayerID != playerID {
		if w.Player(playerID) == nil {
			return nil, ErrForbidden
		}
		idle := g.Now().Unix() - w.Campaign.Turns.Started
		if idle < int64(g.Cfg.SkipAfterSeconds) {
			return nil, fmt.Errorf("%w: %s's turn can be skipped after %d idle seconds (%d so far)", ErrNotYourTurn, actor.Name, g.Cfg.SkipAfterSeconds, idle)
		}
	}
	out := &TurnResult{TurnEnded: true}
	evs := []store.Event{store.NewEvent("narration", "", map[string]any{"text": actor.Name + " holds and lets the moment pass.", "skip": true, "character_id": actor.ID})}
	noteSceneTurn(w, &game.Resolution{})
	evs = append(evs, g.endTurn(ctx, w, out)...)
	if w.Campaign.Status == game.StatusFailed {
		out.CampaignEnded = true
		evs = append(evs, store.NewEvent("campaign_ended", "", map[string]any{"status": w.Campaign.Status}))
	}
	out.Status = w.Campaign.Status
	if _, err := g.commit(ctx, w, evs...); err != nil {
		return nil, err
	}
	return out, nil
}
