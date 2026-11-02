package gm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Solvely-Colin/aidnd/internal/dice"
	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/store"
)

// Players roll their own dice. When a turn needs the acting character's
// d20, TakeAction reads it and dry-runs it on a copy of the world to show
// what it will cost and roll against, then waits. Roll resolves it for
// real: the server's dice decide at the moment the player rolls. A dying
// character's death save waits the same way.

// rollsOwnDice reports whether c's player rolls their own d20s.
func (g *GM) rollsOwnDice(w *game.World, c *game.Character) bool {
	if !g.Cfg.PlayerDice || c == nil {
		return false
	}
	p := w.Player(c.PlayerID)
	return p != nil && !p.AutoRoll
}

// cloneWorld deep-copies w for a dry run.
func cloneWorld(w *game.World) *game.World {
	b, err := json.Marshal(w)
	if err != nil {
		return nil
	}
	var c game.World
	if json.Unmarshal(b, &c) != nil {
		return nil
	}
	return &c
}

// d20Kinds are the roll kinds a player makes with their own d20.
var d20Kinds = map[string]bool{"attack": true, "check": true, "subdue": true}

// plan dry-runs actions on a copy of the world. It returns what each step
// will cost and the d20s it needs, and whether any d20 is the actor's.
func (g *GM) plan(w *game.World, actor *game.Character, actions []game.Action) ([]game.PlannedStep, bool) {
	cw := cloneWorld(w)
	if cw == nil {
		return nil, false
	}
	ca := cw.Character(actor.ID)
	if ca == nil {
		return nil, false
	}
	res := game.Resolve(&game.Ctx{W: cw, R: dice.New(g.Now().UnixNano()), Now: g.Now()}, ca, actions)
	var out []game.PlannedStep
	needs := false
	add := func(s game.Step) {
		ps := game.PlannedStep{Kind: s.Action.Kind, Label: stepLabel(w, s.Action), Slot: s.Slot, Error: s.Error}
		for _, r := range s.Rolls {
			if r.Foe || !d20Kinds[r.Kind] || r.Natural == 0 {
				continue
			}
			ps.Rolls = append(ps.Rolls, game.PlannedRoll{Label: r.Label, Kind: r.Kind, Mod: r.Mod, Parts: r.Parts, Target: r.Target, Against: r.Against, Edge: r.Edge, EdgeWhy: r.EdgeWhy})
			needs = true
		}
		out = append(out, ps)
	}
	for _, s := range res.Steps {
		add(s)
	}
	for _, s := range res.Skipped {
		add(s)
	}
	return out, needs
}

// stepLabel names a planned step for players.
func stepLabel(w *game.World, a game.Action) string {
	name := func(ref string) string {
		if e := w.Enemy(ref); e != nil {
			return e.Display()
		}
		if n := w.NPC(ref); n != nil {
			return n.Name
		}
		if c := w.Character(ref); c != nil {
			return c.Name
		}
		if x := w.FindExit(w.Campaign.LocationID, ref); x != nil {
			return x.To.Name
		}
		if l := w.Location(ref); l != nil {
			return l.Name
		}
		return ref
	}
	zone := func(ref string) string {
		if l := w.Here(); l != nil {
			if z := l.Zone(ref); z != nil {
				return z.Name
			}
		}
		return ref
	}
	verb := map[string]string{
		game.ActLook: "Look", game.ActSay: "Say", game.ActEquip: "Equip", game.ActDrop: "Drop", game.ActEndTurn: "End the turn",
		game.ActMove: "Go to", game.ActReposition: "Move to", game.ActTravel: "Travel to", game.ActAttack: "Attack", game.ActSubdue: "Subdue",
		game.ActCheck: "Check", game.ActUseItem: "Use", game.ActTalkTo: "Talk to", game.ActBuy: "Buy", game.ActSell: "Sell",
		game.ActAcceptQuest: "Accept quest", game.ActTurnInQuest: "Turn in quest", game.ActPickUp: "Pick up", game.ActGiveItem: "Give",
		game.ActRest: "Rest", game.ActDodge: "Dodge", game.ActDisengage: "Disengage", game.ActHelp: "Help", game.ActHide: "Hide",
		game.ActReady: "Ready an attack", game.ActShove: "Shove", game.ActGrapple: "Grapple",
	}[a.Kind]
	if verb == "" {
		verb = a.Kind
	}
	var parts []string
	switch a.Kind {
	case game.ActReposition:
		parts = append(parts, zone(a.Zone))
	case game.ActCheck:
		what := a.Ability
		if sk, ok := game.SkillByKey(a.Skill); ok {
			what = sk.Name
		}
		verb = what + " check"
		if a.Text != "" {
			parts = append(parts, "to "+a.Text)
		}
	case game.ActUseItem, game.ActEquip, game.ActDrop, game.ActPickUp, game.ActBuy, game.ActSell, game.ActGiveItem:
		if it := w.Item(a.Item); it != nil {
			parts = append(parts, it.Name)
		}
		if a.Target != "" {
			parts = append(parts, "→ "+name(a.Target))
		}
	case game.ActSay:
		parts = append(parts, fmt.Sprintf("%q", trim(a.Text, 60)))
	case game.ActCast:
		if sp := game.SpellByRef(a.Spell); sp != nil {
			verb = "Cast " + game.SpellName(sp, w.Setting())
		}
		if a.Target != "" {
			parts = append(parts, "at "+name(a.Target))
		} else if a.Zone != "" {
			parts = append(parts, "on "+zone(a.Zone))
		}
	case game.ActFeature:
		if f := game.FeatureActByRef(a.Feature); f != nil {
			verb = f.Name
		}
		if a.Target != "" {
			parts = append(parts, name(a.Target))
		}
	default:
		if a.Target != "" {
			parts = append(parts, name(a.Target))
		}
		if a.Kind == game.ActMove && a.Target == "" && a.Zone != "" {
			parts = append(parts, zone(a.Zone))
		}
	}
	return strings.TrimSpace(verb + " " + strings.Join(parts, " "))
}

// holdForRoll stores the planned turn and tells the table who is rolling.
func (g *GM) holdForRoll(ctx context.Context, w *game.World, actor *game.Character, playerID, text string, hints Hints, actions []game.Action, preview []game.PlannedStep) (*TurnResult, error) {
	hj, _ := json.Marshal(hints)
	p := &game.PendingAction{Kind: game.PendingTurn, CharacterID: actor.ID, PlayerID: playerID, Text: text, Hints: hj, Actions: actions, Preview: preview, Created: g.Now().Unix()}
	w.Campaign.SetPending(p)
	b := w.Campaign.Turns.Budget
	if _, err := g.commit(ctx, w, store.NewEvent("action_pending", playerID, map[string]any{"character_id": actor.ID, "character": actor.Name, "text": text, "preview": preview})); err != nil {
		return nil, err
	}
	return &TurnResult{Pending: p, Budget: &b, Status: w.Campaign.Status}, nil
}

// Roll rolls the dice for the player's pending turn and resolves it.
func (g *GM) Roll(ctx context.Context, campaignID, playerID string) (*TurnResult, error) {
	defer g.lock(campaignID)()
	w, err := g.Store.Load(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if err := w.FocusPlayer(playerID); err != nil {
		return nil, err
	}
	if w.Campaign.Status != game.StatusActive {
		return nil, Conflict("the campaign is %s", w.Campaign.Status)
	}
	pl := w.Player(playerID)
	if pl == nil {
		return nil, ErrForbidden
	}
	actor := w.Character(pl.CharacterID)
	if actor != nil {
		if p := w.Campaign.Pending[actor.ID]; p != nil && p.Kind == game.PendingDeathSave && !actor.Dying() {
			w.Campaign.TakePending(actor.ID) // healed or stabilized since: nothing to roll
			_, _ = g.commit(ctx, w)
		}
	}
	if actor == nil || w.Campaign.Pending[actor.ID] == nil {
		return nil, Conflict("there is nothing waiting for your roll")
	}
	if w.InCombat() {
		if a := w.ActiveCharacter(); a == nil || a.ID != actor.ID {
			w.Campaign.TakePending(actor.ID) // the moment passed
			_, _ = g.commit(ctx, w)
			return nil, ErrNotYourTurn
		}
	}
	p := w.Campaign.TakePending(actor.ID)
	if p.Kind == game.PendingDeathSave {
		return g.rollDeathSave(ctx, w, actor)
	}
	if !w.InCombat() {
		if _, err := g.actorFor(w, playerID); err != nil {
			return nil, err
		}
	}
	var hints Hints
	_ = json.Unmarshal(p.Hints, &hints)
	recent := g.recent(ctx, w)
	g.progress(campaignID, "reading", map[string]any{"character_id": actor.ID, "character": actor.Name, "text": p.Text})
	return g.resolveTurn(ctx, w, actor, playerID, p.Text, hints, p.Actions, recent)
}

// rollDeathSave rolls a dying character's death save at the start of their
// turn. A natural 20 gives them their turn back; anything else passes it.
func (g *GM) rollDeathSave(ctx context.Context, w *game.World, actor *game.Character) (*TurnResult, error) {
	out := &TurnResult{}
	ds := game.DeathSave(w, g.Dice, actor)
	evs := []store.Event{deathSaveEvent(out, ds)}
	if ds.Revived {
		tick := game.StartCharacterTurn(actor, &w.Campaign.Turns.Budget)
		evs = append(evs, conditionEvents(out, tick)...)
		evs = append(evs, store.NewEvent("turn_started", actor.PlayerID, map[string]any{"character_id": actor.ID, "player_id": actor.PlayerID, "name": actor.Name, "budget": w.Campaign.Turns.Budget}))
		out.NextActor = actorOf(actor)
		b := w.Campaign.Turns.Budget
		out.Budget = &b
	} else {
		out.TurnEnded = true
		evs = append(evs, g.endTurn(ctx, w, out)...)
	}
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

// CancelPending drops the player's planned turn so they can say something
// else. A death save can't be taken back.
func (g *GM) CancelPending(ctx context.Context, campaignID, playerID string) error {
	defer g.lock(campaignID)()
	w, err := g.Store.Load(ctx, campaignID)
	if err != nil {
		return err
	}
	if err := w.FocusPlayer(playerID); err != nil {
		return err
	}
	pl := w.Player(playerID)
	if pl == nil {
		return ErrForbidden
	}
	p := w.Campaign.Pending[pl.CharacterID]
	if p == nil {
		return Conflict("there is nothing to take back")
	}
	if p.Kind == game.PendingDeathSave {
		return Conflict("a death save can't be taken back")
	}
	w.Campaign.TakePending(pl.CharacterID)
	_, err = g.commit(ctx, w, store.NewEvent("action_cancelled", playerID, map[string]any{"character_id": pl.CharacterID}))
	return err
}

// SetAutoRoll sets whether the engine rolls this player's dice for them.
func (g *GM) SetAutoRoll(ctx context.Context, campaignID, playerID string, auto bool) error {
	defer g.lock(campaignID)()
	w, err := g.Store.Load(ctx, campaignID)
	if err != nil {
		return err
	}
	pl := w.Player(playerID)
	if pl == nil {
		return ErrForbidden
	}
	pl.AutoRoll = auto
	_, err = g.commit(ctx, w)
	return err
}

// deathSaveEvent records a death save on the result and as an event.
func deathSaveEvent(out *TurnResult, ds game.DeathSaveResult) store.Event {
	out.DeathSaves = append(out.DeathSaves, ds)
	return store.NewEvent("death_save", "", ds)
}

// SetLethality sets what three failed death saves mean in this campaign.
func (g *GM) SetLethality(ctx context.Context, campaignID, lethality string) (*game.World, error) {
	defer g.lock(campaignID)()
	w, err := g.Store.Load(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	switch lethality {
	case game.LethalityStory, game.LethalityStandard:
		w.Campaign.Lethality = lethality
	default:
		return nil, fmt.Errorf("%w: lethality must be standard or story", ErrBadRequest)
	}
	_, err = g.commit(ctx, w)
	return w, err
}
