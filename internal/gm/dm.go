package gm

import (
	"context"
	"fmt"
	"strings"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/store"
)

// The DM console: table-side controls any seated player can use. Every
// change goes through the engine and leaves a dm_action event in the feed,
// so the table sees who did what. None of it asks the model.

// DMAction is one console request.
type DMAction struct {
	Kind     string `json:"kind"` // end_fight | heal_party | award_xp | give_item | lethality
	Amount   int    `json:"amount,omitempty"`
	Target   string `json:"target,omitempty"` // character id for give_item
	Item     string `json:"item,omitempty"`   // catalog or wondrous key
	Template string `json:"template,omitempty"`
	Value    string `json:"value,omitempty"`
}

// DMKinds are the console's actions.
var DMKinds = []string{"end_fight", "heal_party", "award_xp", "give_item", "lethality"}

func (g *GM) dmPlayer(w *game.World, playerID string) (*game.Player, error) {
	p := w.Player(playerID)
	if p == nil {
		return nil, ErrForbidden
	}
	return p, nil
}

// History lists the turns the table can rewind to, newest first.
func (g *GM) History(ctx context.Context, campaignID, playerID string) ([]store.Snapshot, error) {
	w, err := g.Store.Load(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if _, err := g.dmPlayer(w, playerID); err != nil {
		return nil, err
	}
	return g.Store.Snapshots(ctx, campaignID)
}

// Rewind puts the whole campaign back to before turn seq. It is the one
// place a rewind happens, so it can later be narrowed (to one group's
// scene, say) without touching its callers.
func (g *GM) Rewind(ctx context.Context, campaignID, playerID string, seq int64) (*game.World, error) {
	defer g.lock(campaignID)()
	cur, err := g.Store.Load(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	p, err := g.dmPlayer(cur, playerID)
	if err != nil {
		return nil, err
	}
	w, sn, err := g.Store.Rewind(ctx, campaignID, seq)
	if err != nil {
		if err == store.ErrNotFound {
			return nil, fmt.Errorf("%w: that turn is no longer kept", ErrBadRequest)
		}
		return nil, err
	}
	w.Campaign.Turns.Started = g.Now().Unix()
	_, err = g.commit(ctx, w, store.NewEvent("rewound", playerID, map[string]any{"by": p.Name, "seq": sn.Seq, "to_event": sn.EventID, "label": sn.Label,
		"text": fmt.Sprintf("%s rewound the story to just before %q.", p.Name, sn.Label)}))
	return w, err
}

// DM applies one console action.
func (g *GM) DM(ctx context.Context, campaignID, playerID string, a DMAction) (*game.World, string, error) {
	defer g.lock(campaignID)()
	w, err := g.Store.Load(ctx, campaignID)
	if err != nil {
		return nil, "", err
	}
	p, err := g.dmPlayer(w, playerID)
	if err != nil {
		return nil, "", err
	}
	var lines []string
	switch a.Kind {
	case "end_fight":
		if !w.InCombat() {
			return nil, "", Conflict("there is no fight to end")
		}
		for _, e := range w.Encounter.Living() {
			e.Subdued = true
		}
		lines = append(lines, "The DM calls the fight")
		lines = append(lines, game.EndFight(w)...)
		w.Campaign.Turns.Budget = game.FullBudget(false)
	case "heal_party":
		for _, c := range w.Characters {
			if !c.Dead {
				c.HP, c.Death = c.MaxHP, nil
			}
		}
		if !w.InCombat() {
			game.ClearAllConditions(w)
		}
		lines = append(lines, "The party is restored to full health")
	case "award_xp":
		if a.Amount < 1 || a.Amount > 3000 {
			return nil, "", fmt.Errorf("%w: XP must be 1 to 3000", ErrBadRequest)
		}
		for _, c := range w.Party() {
			lines = append(lines, game.AwardXP(c, a.Amount)...)
		}
	case "give_item":
		c := w.Character(a.Target)
		if c == nil {
			return nil, "", fmt.Errorf("%w: give it to whom?", ErrBadRequest)
		}
		owner := game.Owner{Kind: game.OwnerCharacter, ID: c.ID}
		var it *game.Item
		if a.Template != "" {
			it = game.NewMagicGear(w, a.Item, a.Template, owner)
		} else {
			it = game.NewItem(w, a.Item, owner)
		}
		if it == nil {
			return nil, "", fmt.Errorf("%w: unknown item %q", ErrBadRequest, a.Item)
		}
		lines = append(lines, fmt.Sprintf("%s receives %s", c.Name, it.Name))
	case "lethality":
		switch a.Value {
		case game.LethalityStory, game.LethalityStandard:
			w.Campaign.Lethality = a.Value
			lines = append(lines, "Death is now "+map[string]string{game.LethalityStory: "off: the fallen stay stable", game.LethalityStandard: "on: three failed death saves kill"}[a.Value])
		default:
			return nil, "", fmt.Errorf("%w: lethality must be standard or story", ErrBadRequest)
		}
	default:
		return nil, "", fmt.Errorf("%w: unknown console action %q", ErrBadRequest, a.Kind)
	}
	w.RefreshMagic()
	text := p.Name + " (DM console): " + strings.Join(lines, "; ")
	_, err = g.commit(ctx, w, store.NewEvent("dm_action", playerID, map[string]any{"by": p.Name, "kind": a.Kind, "changes": lines, "text": text}))
	return w, text, err
}
