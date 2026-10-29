package gm

import (
	"context"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/store"
)

// Choose records a level-up pick (a feat, invocations, an ability score
// increase) for the player's character.
func (g *GM) Choose(ctx context.Context, campaignID, playerID, key string, picks []string) (*game.World, *game.Character, error) {
	defer g.lock(campaignID)()
	w, err := g.Store.Load(ctx, campaignID)
	if err != nil {
		return nil, nil, err
	}
	c := w.CharacterOf(playerID)
	if c == nil {
		return nil, nil, ErrForbidden
	}
	if c.ClassKey == "" {
		return nil, nil, Conflict("this character was made before classes; rebuild them to pick")
	}
	if p := game.Choose(c, key, picks); len(p) > 0 {
		return nil, nil, p
	}
	_, err = g.commit(ctx, w, store.NewEvent("character_updated", playerID, map[string]any{"character_id": c.ID, "choice": key, "picks": picks}))
	return w, c, err
}

// SetSpells changes the player's prepared spells. Casters prepare outside
// fights.
func (g *GM) SetSpells(ctx context.Context, campaignID, playerID string, spells []string) (*game.World, *game.Character, error) {
	defer g.lock(campaignID)()
	w, err := g.Store.Load(ctx, campaignID)
	if err != nil {
		return nil, nil, err
	}
	c := w.CharacterOf(playerID)
	if c == nil {
		return nil, nil, ErrForbidden
	}
	if w.InCombat() {
		return nil, nil, Conflict("you can't change your spells in the middle of a fight")
	}
	if p := game.ValidateSpells(c, spells); len(p) > 0 {
		return nil, nil, p
	}
	c.Spells = append([]string(nil), spells...)
	_, err = g.commit(ctx, w, store.NewEvent("character_updated", playerID, map[string]any{"character_id": c.ID, "spells": spells}))
	return w, c, err
}
