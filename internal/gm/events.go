package gm

import (
	"context"
	"strings"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/store"
)

// World events (see game/events.go): the engine finds where the players'
// threads and a faction's clock meet, calls everyone to one place with a
// deadline, and settles the outcome for good. Here are the ways in: a
// player answering the call, a player at the site starting it early, and
// the muster running out.

// Phases of the world_event event.
const (
	EventPhaseCalled  = "called"
	EventPhaseAnswer  = "answered"
	EventPhaseBegun   = "begun"
	EventPhaseSettled = "settled"
)

func eventCalled(w *game.World, e *game.WorldEvent) store.Event {
	site := ""
	if l := w.Location(e.SiteID); l != nil {
		site = l.Name
	}
	return store.NewEvent("world_event", "", map[string]any{"phase": EventPhaseCalled, "event_id": e.ID, "title": e.Title,
		"call": e.Call, "site": site, "site_id": e.SiteID, "deadline": e.Deadline})
}

func eventSettled(res *game.EventResult) store.Event {
	return store.NewEvent("world_event", "", map[string]any{"phase": EventPhaseSettled, "event_id": res.EventID, "title": res.Title,
		"won": res.Won, "outcome": res.Outcome, "changes": res.Changes})
}

// eventEvents collects events for world events the engine settled since
// the last save: a confrontation won, or one fled or lost.
func eventEvents(w *game.World) []store.Event {
	var evs []store.Event
	for _, res := range w.TakeEventResults() {
		evs = append(evs, eventSettled(res))
	}
	if res := game.EventFightLost(w); res != nil {
		evs = append(evs, eventSettled(res))
	}
	return evs
}

// beginEvent starts the confrontation for a group at the site.
func (g *GM) beginEvent(w *game.World, group, playerID string) ([]store.Event, error) {
	if err := w.Focus(group); err != nil {
		return nil, err
	}
	e := game.CurrentEvent(w)
	changes, err := game.BeginEvent(w, g.Dice, g.Now().Unix())
	if err != nil {
		return nil, err
	}
	evs := []store.Event{store.NewEvent("world_event", playerID, map[string]any{"phase": EventPhaseBegun, "event_id": e.ID, "title": e.Title, "changes": changes})}
	if a := w.ActiveCharacter(); a != nil {
		evs = append(evs, store.NewEvent("turn_started", a.PlayerID, map[string]any{"character_id": a.ID, "player_id": a.PlayerID, "name": a.Name, "budget": w.Campaign.Turns.Budget}))
	}
	return evs, nil
}

// AnswerEvent brings the player's group to the event's site and joins it
// with anyone there. Once every living character has come, the
// confrontation begins.
func (g *GM) AnswerEvent(ctx context.Context, campaignID, playerID string) (*game.World, error) {
	defer g.lock(campaignID)()
	w, err := g.Store.Load(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	c, err := g.groupActor(w, playerID)
	if err != nil {
		return nil, err
	}
	e := game.CurrentEvent(w)
	if e == nil {
		return nil, Conflict("nothing is calling right now")
	}
	if site := w.Location(e.SiteID); site != nil && (site.Stub || len(site.Zones) == 0) {
		// the first time anyone goes there: build it as travel would
		onArrive, _ := g.arrivalHooks(ctx, w)
		onArrive(site)
	}
	from := game.GroupOf(c)
	grp, moved, joins, err := game.AnswerCall(w, c)
	if err != nil {
		return nil, Conflict("%s", err.Error())
	}
	var evs []store.Event
	if moved {
		site := w.Location(e.SiteID)
		evs = append(evs, tagParty(store.NewEvent("location_changed", playerID, map[string]any{"location_id": site.ID, "name": site.Name}), from))
	}
	for _, j := range joins {
		evs = append(evs, store.NewEvent("group_joined", playerID, map[string]any{
			"character_id": c.ID, "group": j.Group, "from": j.From, "into": j.Into,
			"joining": strings.Join(j.Joining, ", "), "with": strings.Join(j.With, ", ")}))
	}
	evs = append(evs, store.NewEvent("world_event", playerID, map[string]any{"phase": EventPhaseAnswer, "event_id": e.ID,
		"title": e.Title, "character_id": c.ID, "character": c.Name}))
	if e.Status == game.EventGathering && game.EveryoneThere(w, e) {
		more, err := g.beginEvent(w, grp, playerID)
		if err != nil {
			g.logf("event begin %s: %v", campaignID, err)
		}
		evs = append(evs, more...)
	}
	if _, err := g.commit(ctx, w, evs...); err != nil {
		return nil, err
	}
	return w, nil
}

// BeginEventNow lets a player at the site start the confrontation without
// waiting for the others.
func (g *GM) BeginEventNow(ctx context.Context, campaignID, playerID string) (*game.World, error) {
	defer g.lock(campaignID)()
	w, err := g.Store.Load(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	c, err := g.groupActor(w, playerID)
	if err != nil {
		return nil, err
	}
	evs, err := g.beginEvent(w, game.GroupOf(c), playerID)
	if err != nil {
		return nil, Conflict("%s", err.Error())
	}
	if _, err := g.commit(ctx, w, evs...); err != nil {
		return nil, err
	}
	return w, nil
}

// eventTimer handles an event whose muster ran out.
func (g *GM) eventTimer(ctx context.Context, campaignID string) error {
	defer g.lock(campaignID)()
	w, err := g.Store.Load(ctx, campaignID)
	if err != nil {
		return err
	}
	if w.Campaign.Status != game.StatusActive {
		return nil
	}
	begin, res := game.EventDeadline(w, g.Now().Unix())
	var evs []store.Event
	switch {
	case res != nil:
		evs = append(evs, eventSettled(res))
	case begin:
		e := game.CurrentEvent(w)
		more, err := g.beginEvent(w, game.SiteGroup(w, e), "")
		if err != nil {
			return err
		}
		evs = append(evs, more...)
	default:
		return nil
	}
	_, err = g.commit(ctx, w, evs...)
	return err
}
