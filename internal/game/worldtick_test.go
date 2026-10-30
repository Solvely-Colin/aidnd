package game

import (
	"testing"

	"github.com/Solvely-Colin/aidnd/internal/dice"
)

func tickWorld(tide int, fs ...*Faction) *World {
	w := &World{}
	w.Campaign.Living = &Living{Tide: tide, Factions: fs}
	return w
}

func TestIdleTicksStopTheTideAt66(t *testing.T) {
	w := tickWorld(65)
	r := dice.New(1)
	WorldTick(w, r, 0, true)
	WorldTick(w, r, 0, true)
	if w.Campaign.Living.Tide != 66 || w.Campaign.Living.Week != 2 {
		t.Fatalf("tide %d week %d", w.Campaign.Living.Tide, w.Campaign.Living.Week)
	}
	WorldTick(w, r, 0, false)
	if w.Campaign.Living.Tide != 67 {
		t.Fatalf("a played tick still raises it: %d", w.Campaign.Living.Tide)
	}
}

func TestThreatClocksAndStorySafety(t *testing.T) {
	a := &Faction{Name: "A", Threat: true, Steps: 2, Progress: 1, Moves: []string{"a moves"}, Full: "A wins", FullDanger: map[string]int{"l1": 3}}
	b := &Faction{Name: "B", Threat: true, Steps: 2, Progress: 1, Full: "B wins"}
	w := tickWorld(10, a, b)
	w.Locations = []*Location{{ID: "l1", Danger: 1}}
	// idle: a threat can't fill
	for i := 0; i < 20; i++ {
		WorldTick(w, dice.New(int64(i)), 0, true)
	}
	if a.Filled || b.Filled {
		t.Fatal("an idle tick filled a threat clock")
	}
	// played: at most one fills per tick
	for i := 0; i < 50 && !(a.Filled && b.Filled); i++ {
		rep := WorldTick(w, dice.New(int64(100+i)), 0, false)
		if len(rep.Filled) > 1 {
			t.Fatalf("two threats filled in one tick: %v", rep.Filled)
		}
	}
	if !a.Filled || w.Location("l1").Danger != 3 {
		t.Fatalf("A should fill and raise l1: filled=%v danger=%d", a.Filled, w.Location("l1").Danger)
	}
}

func TestQuestsLowerTheTide(t *testing.T) {
	w := tickWorld(50)
	tideOnQuest(w, &Quest{ID: "q1", Title: "Side"})
	if w.Campaign.Living.Tide != 48 {
		t.Fatalf("side quest: %d", w.Campaign.Living.Tide)
	}
	tideOnQuest(w, &Quest{ID: "q2", Title: "Main", IsMain: true})
	if w.Campaign.Living.Tide != 38 || w.Campaign.Living.Bands["q2"] != TideSlack {
		t.Fatalf("storyline: %d %v", w.Campaign.Living.Tide, w.Campaign.Living.Bands)
	}
}

func TestTicksDue(t *testing.T) {
	w := tickWorld(5)
	day := int64(86400)
	now := 100*day + TickHourUTC*3600 + 60 // just after the turn of day 100
	if TicksDue(w, now, 7) != 0 || w.Campaign.Living.LastTickDay != 100 {
		t.Fatal("the first call starts the clock")
	}
	if TicksDue(w, now+day-120, 7) != 0 {
		t.Fatal("not due before the next morning")
	}
	if TicksDue(w, now+day, 7) != 1 || TicksDue(w, now+30*day, 7) != 7 {
		t.Fatal("due counts")
	}
}
