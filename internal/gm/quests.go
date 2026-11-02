package gm

import (
	"context"
	"fmt"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/store"
)

// Quest log moves a player makes from their casebook, outside the story:
// take a quest on, drop it, set a lead aside, tell the friends with you,
// or pick the one to follow on the play screen.
const (
	QuestOpTake  = "take"
	QuestOpDrop  = "drop"
	QuestOpAside = "aside"
	QuestOpTell  = "tell"
	QuestOpTrack = "track"
	QuestOpShare = "share"
)

// QuestAct applies a casebook move for the player's character. None of
// these spend a turn; taking on, dropping and telling are news for the
// story feed.
func (g *GM) QuestAct(ctx context.Context, campaignID, playerID, questID, op string, to ...string) (*game.World, error) {
	defer g.lock(campaignID)()
	w, err := g.Store.Load(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if w.Campaign.Status != game.StatusActive {
		return nil, Conflict("the campaign is %s", w.Campaign.Status)
	}
	c := w.CharacterOf(playerID)
	if c == nil {
		return nil, ErrForbidden
	}
	if err := w.FocusPlayer(playerID); err != nil {
		return nil, err
	}
	var ups []game.QuestUpdate
	switch op {
	case QuestOpTake:
		_, u, err := game.TakeOn(w, questID, c)
		if err != nil {
			return nil, Conflict("%s", err.Error())
		}
		ups = u
		// standing where the quest sends you, or holding what it wants,
		// ticks the step at once
		if more, _ := game.EvaluateQuests(w); len(more) > 0 {
			ups = append(ups, more...)
		}
	case QuestOpDrop:
		u, err := game.DropQuest(w, questID, c)
		if err != nil {
			return nil, Conflict("%s", err.Error())
		}
		ups = u
	case QuestOpAside:
		if err := game.SetQuestAside(w, questID, c); err != nil {
			return nil, Conflict("%s", err.Error())
		}
	case QuestOpTell:
		heard, err := game.TellOfQuest(w, questID, c)
		if err != nil {
			return nil, Conflict("%s", err.Error())
		}
		q := w.Quest(questID)
		var ids, names []string
		for _, h := range heard {
			ids, names = append(ids, h.ID), append(names, h.Name)
		}
		ups = append(ups, game.QuestUpdate{QuestID: q.ID, Title: q.Title, Kind: game.QuestLead, Status: q.Status, Lead: q.Lead,
			Text: c.Name + " told " + joinList(names) + " about " + q.Title, Characters: append(ids, c.ID)})
	case QuestOpShare:
		q, shared, err := game.ShareQuest(w, questID, c, to)
		if err != nil {
			return nil, Conflict("%s", err.Error())
		}
		var ids, names []string
		for _, h := range shared {
			ids, names = append(ids, h.ID), append(names, h.Name)
		}
		ups = append(ups, game.QuestUpdate{QuestID: q.ID, Title: q.Title, Kind: game.QuestShared, Status: q.Status, Lead: q.Lead,
			By: c.ID, ByName: c.Name, Text: c.Name + " shared " + q.Title + " with " + joinList(names), Characters: append(ids, c.ID)})
	case QuestOpTrack:
		if err := game.Track(w, questID, c); err != nil {
			return nil, Conflict("%s", err.Error())
		}
	default:
		return nil, fmt.Errorf("%w: unknown quest move %q", ErrBadRequest, op)
	}
	var evs []store.Event
	for _, u := range ups {
		evs = append(evs, store.NewEvent("quest_updated", playerID, u))
	}
	if _, err := g.commit(ctx, w, evs...); err != nil {
		return nil, err
	}
	return w, nil
}

func joinList(ns []string) string {
	switch len(ns) {
	case 0:
		return "nobody"
	case 1:
		return ns[0]
	}
	out := ""
	for i, n := range ns {
		switch {
		case i == 0:
			out = n
		case i == len(ns)-1:
			out += " and " + n
		default:
			out += ", " + n
		}
	}
	return out
}
