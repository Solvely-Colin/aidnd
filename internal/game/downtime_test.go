package game

import (
	"testing"

	"github.com/Solvely-Colin/aidnd/internal/dice"
)

func TestDowntimeOrders(t *testing.T) {
	w := &World{}
	w.Campaign.Living = &Living{}
	hub := &Location{ID: "l1", IsHub: true, Region: "r"}
	wild := &Location{ID: "l2", Danger: 2, Region: "r"}
	w.Locations = []*Location{hub, wild}
	w.Campaign.LocationID = "l1"
	c := &Character{ID: "c1", Name: "Ada", Level: 2, HP: 10, MaxHP: 10, Scores: Scores{STR: 10, DEX: 10, CON: 10, INT: 18, WIS: 10, CHA: 10}}
	c.Strain = 3
	w.Characters = []*Character{c}
	if CanDowntime(w, c, wild) == nil || CanDowntime(w, c, hub) != nil {
		t.Fatal("downtime only at a safe place")
	}
	if QueueDowntime(w, c, "nap") == nil {
		t.Fatal("unknown order accepted")
	}
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(QueueDowntime(w, c, DowntimeRest))
	res := ResolveDowntime(w, dice.New(1))
	if len(res) != 1 || c.Strain != 1 || c.Downtime != nil {
		t.Fatalf("rest: %+v strain %d", res, c.Strain)
	}
	// research finds open lore about the region, never a gated secret
	w.Lore = []*Lore{{ID: "s", Title: "Secret", Text: "x", EntityIDs: []string{"l1"}, LoreExtra: LoreExtra{Gate: "obj9"}}, {ID: "o", Title: "Open", Text: "y", EntityIDs: []string{"l1"}}}
	w.Quests = []*Quest{{ID: "q", Objectives: []Objective{{ID: "obj9"}}}}
	found := false
	for seed := int64(0); seed < 20 && !found; seed++ {
		must(QueueDowntime(w, c, DowntimeResearch))
		ResolveDowntime(w, dice.New(seed))
		found = w.Lore[1].Researched
	}
	if !found || w.Lore[0].Researched {
		t.Fatalf("research: open=%v secret=%v", w.Lore[1].Researched, w.Lore[0].Researched)
	}
	gold := c.Gold
	must(QueueDowntime(w, c, DowntimeWork))
	ResolveDowntime(w, dice.New(3))
	if c.Gold <= gold {
		t.Fatal("work pays")
	}
}
