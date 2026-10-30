package gm

import (
	"context"
	"strings"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/store"
)

// GoSolo splits the player's character off from their group: from now on
// they act, travel and fight on their own in the same world.
func (g *GM) GoSolo(ctx context.Context, campaignID, playerID string) (*game.World, error) {
	defer g.lock(campaignID)()
	w, err := g.Store.Load(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	c, err := g.groupActor(w, playerID)
	if err != nil {
		return nil, err
	}
	from := game.GroupOf(c)
	grp, err := game.SplitOff(w, []string{c.ID}, g.Now().Unix())
	if err != nil {
		return nil, Conflict("%s", err.Error())
	}
	where := ""
	if l := w.Location(grp.LocationID); l != nil {
		where = l.Name
	}
	ev := store.NewEvent("group_split", playerID, map[string]any{
		"character_id": c.ID, "character": c.Name, "group": grp.ID, "from": from, "location_id": grp.LocationID, "location": where})
	if _, err := g.commit(ctx, w, ev); err != nil {
		return nil, err
	}
	return w, nil
}

// JoinGroup brings the player's whole group into another group standing
// in the same place.
func (g *GM) JoinGroup(ctx context.Context, campaignID, playerID, target string) (*game.World, error) {
	defer g.lock(campaignID)()
	w, err := g.Store.Load(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	c, err := g.groupActor(w, playerID)
	if err != nil {
		return nil, err
	}
	from := game.GroupOf(c)
	var joining, with []string
	for _, m := range w.Members(from) {
		joining = append(joining, m.Name)
	}
	for _, m := range w.Members(target) {
		with = append(with, m.Name)
	}
	if len(with) == 0 {
		return nil, Conflict("there is nobody in that group")
	}
	if err := game.JoinGroups(w, from, target); err != nil {
		return nil, Conflict("%s", err.Error())
	}
	ev := store.NewEvent("group_joined", playerID, map[string]any{
		"character_id": c.ID, "group": game.GroupOf(c), "from": from, "into": target,
		"joining": strings.Join(joining, ", "), "with": strings.Join(with, ", ")})
	if _, err := g.commit(ctx, w, ev); err != nil {
		return nil, err
	}
	return w, nil
}

// groupActor checks the player can move their character between groups.
func (g *GM) groupActor(w *game.World, playerID string) (*game.Character, error) {
	if w.Campaign.Status != game.StatusActive {
		return nil, Conflict("the campaign is %s", w.Campaign.Status)
	}
	c := w.CharacterOf(playerID)
	if c == nil {
		return nil, ErrForbidden
	}
	if c.Dead {
		return nil, Conflict("%s is dead", c.Name)
	}
	if p := w.Campaign.Pending[c.ID]; p != nil {
		return nil, Conflict("roll or take back your planned turn first")
	}
	return c, nil
}
