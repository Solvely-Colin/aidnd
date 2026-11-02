package gm

import (
	"strings"
	"testing"
	"time"

	"github.com/Solvely-Colin/aidnd/internal/game"
)

func TestWorldEventFromWhatThePlayerDid(t *testing.T) {
	r := newSagaRig(t)
	r.start(t)
	_, err := r.g.Sweep(bg)
	must(t, err)
	// a quiet world brings no event, however long it runs
	w := r.world(t)
	w.Campaign.Living.Week = 6
	_, err = r.g.Store.Save(bg, w)
	must(t, err)
	r.now = r.now.Add(24 * time.Hour)
	_, err = r.g.Sweep(bg)
	must(t, err)
	if w = r.world(t); game.CurrentEvent(w) != nil {
		t.Fatal("an event with no player threads")
	}
	// Cal gets tangled up in Innsmouth: Hannah owes them, they owe Zadok,
	// and they swear to silence the Order
	me := w.CharacterOf("u1")
	w.NPC("Hannah Coyle").Bind(w, me, game.TieDebt, "found her brother's boat", game.SourceEngine)
	w.NPC("Zadok Allen").Bind(w, me, game.TieOwed, "a bottle and a story", game.SourceEngine)
	_, err = game.AddGoal(w, me, "Break the Esoteric Order of Dagon", game.SourcePlayer)
	must(t, err)
	w.Campaign.Living.Faction("eod").Progress = 3
	_, err = r.g.Store.Save(bg, w)
	must(t, err)
	r.now = r.now.Add(24 * time.Hour)
	_, err = r.g.Sweep(bg)
	must(t, err)
	w = r.world(t)
	e := game.CurrentEvent(w)
	if e == nil || e.Faction != "eod" || w.Location(e.SiteID) == nil {
		t.Fatalf("no event from Cal's threads: %+v", w.Campaign.Living.Faction("eod"))
	}
	if evs := r.events(t, "world_event"); len(evs) != 1 || strings.Contains(string(evs[0].Data), `"party"`) || !strings.Contains(string(evs[0].Data), "called") {
		t.Fatalf("one untagged called event: %v", evs)
	}
	if p := snapshot(w, w.CharacterOf("u1"), false); !strings.Contains(p, "WORLD EVENT CALLED") || !strings.Contains(p, "Hannah Coyle owes") {
		t.Fatalf("narrator not told:\n%s", p)
	}
	// the only character answers, so it begins at once
	_, err = r.g.AnswerEvent(bg, r.id, "u1")
	must(t, err)
	w = r.world(t)
	e = game.CurrentEvent(w)
	if e == nil || e.Status != game.EventEngaged || !w.InCombat() || w.Campaign.LocationID != e.SiteID {
		t.Fatalf("after answering: %+v combat=%v", e, w.InCombat())
	}
	if w.NPC("Hannah Coyle").LocationID != e.SiteID || w.CharacterOf("u1").TempHP == 0 {
		t.Fatal("Hannah should come to stand with Cal")
	}
	if p := snapshot(w, w.CharacterOf("u1"), false); !strings.Contains(p, "WORLD EVENT, HAPPENING NOW") {
		t.Fatalf("narrator not told the fight is the event:\n%s", p)
	}
	if _, err := r.g.AnswerEvent(bg, r.id, "u1"); err == nil {
		t.Fatal("answering twice mid-fight")
	}
	// they run: the Order has its way, for good
	w.Encounter = nil
	_, err = r.g.commit(bg, w)
	must(t, err)
	w = r.world(t)
	if game.CurrentEvent(w) != nil || !w.Campaign.Living.Faction("eod").Filled || len(w.Campaign.Living.Past) != 1 {
		t.Fatal("fleeing the event should settle it as lost")
	}
	if evs := r.events(t, "world_event"); len(evs) != 4 || !strings.Contains(string(evs[3].Data), "settled") {
		t.Fatalf("called, answered, begun, settled: %d", len(evs))
	}
}

func TestWorldEventMusterRunsOut(t *testing.T) {
	r := newSagaRig(t)
	r.start(t)
	_, err := r.g.Sweep(bg)
	must(t, err)
	w := r.world(t)
	w.Campaign.Living.Week = 6
	me := w.CharacterOf("u1")
	w.NPC("Zadok Allen").Bind(w, me, game.TieOwed, "a bottle", game.SourceEngine)
	w.Campaign.Living.Faction("eod").Progress = 5
	_, err = r.g.Store.Save(bg, w)
	must(t, err)
	r.now = r.now.Add(24 * time.Hour)
	_, err = r.g.Sweep(bg)
	must(t, err)
	if game.CurrentEvent(r.world(t)) == nil {
		t.Fatal("a clock about to fill with Cal in it should be called")
	}
	r.now = r.now.Add(49 * time.Hour)
	_, err = r.g.Sweep(bg)
	must(t, err)
	w = r.world(t)
	if game.CurrentEvent(w) != nil || !w.Campaign.Living.Faction("eod").Filled {
		t.Fatal("nobody came: the Order should have its way")
	}
	if ties := w.NPC("Zadok Allen").TiesTo(w.CharacterOf("u1").ID); len(ties) == 0 || ties[len(ties)-1].Kind != game.TieGrudge {
		t.Fatalf("Zadok remembers: %+v", ties)
	}
}
