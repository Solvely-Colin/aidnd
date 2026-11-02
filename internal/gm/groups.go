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
	// joining up is the choice to go where the group goes; the others in
	// the joining group keep their own choice
	c.TravelTogether = true
	ev := store.NewEvent("group_joined", playerID, map[string]any{
		"character_id": c.ID, "group": game.GroupOf(c), "from": from, "into": target,
		"joining": strings.Join(joining, ", "), "with": strings.Join(with, ", ")})
	if _, err := g.commit(ctx, w, ev); err != nil {
		return nil, err
	}
	return w, nil
}

// leaveAlone keeps one player's move from carrying their friends along.
// The engine calls it (Ctx.Leaving) outside a fight once the actor's move
// to another place has been checked. In a long campaign (async play) only
// the actor and the group members who chose to travel together go; the
// rest stay where they are, in their own group. A one-shot played at one
// sitting keeps moving as a party. It returns the group_split event to
// save with the turn (nil when nothing changed) and leaves the world
// focused on the actor's group.
func (g *GM) leaveAlone(w *game.World, actor *game.Character, playerID string) (*store.Event, error) {
	if !w.Campaign.LongForm() || w.InCombat() {
		return nil, nil
	}
	movers := []string{actor.ID}
	var going, staying []string
	for _, c := range w.Characters {
		if c.ID == actor.ID || c.Dead {
			continue
		}
		if c.TravelTogether && !c.Down() && game.CanMove(c.Conditions) == nil {
			movers = append(movers, c.ID)
			going = append(going, c.Name)
		} else {
			staying = append(staying, c.Name)
		}
	}
	if len(staying) == 0 {
		return nil, nil
	}
	from := game.GroupOf(actor)
	// where they set out from, read before the group swap moves it
	locID, where := w.Campaign.LocationID, ""
	if l := w.Location(locID); l != nil {
		where = l.Name
	}
	w.Settle()
	grp, err := game.SplitOff(w, movers, g.Now().Unix())
	if err != nil {
		// can't split (say, a fight elsewhere in the group): play on as before
		g.logf("leave alone: %v", err)
		return nil, w.Focus(from)
	}
	if err := w.Focus(grp.ID); err != nil {
		return nil, err
	}
	ev := store.NewEvent("group_split", playerID, map[string]any{
		"character_id": actor.ID, "character": actor.Name, "character_ids": movers, "group": grp.ID, "from": from,
		"location_id": locID, "location": where, "leaving": true,
		"with": strings.Join(going, ", "), "staying": strings.Join(staying, ", ")})
	return &ev, nil
}

// SetTravelTogether records whether the player's character goes along when
// someone in their group leaves for another place.
func (g *GM) SetTravelTogether(ctx context.Context, campaignID, playerID string, on bool) (*game.World, error) {
	defer g.lock(campaignID)()
	w, err := g.Store.Load(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	c := w.CharacterOf(playerID)
	if c == nil {
		return nil, ErrForbidden
	}
	c.TravelTogether = on
	if _, err := g.commit(ctx, w); err != nil {
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
