package game

import (
	"testing"

	"github.com/Solvely-Colin/aidnd/internal/dice"
)

func visionWorld() *World {
	w := &World{Campaign: Campaign{LocationID: "l1", Turn: 10}}
	w.Campaign.Dread = true
	w.Locations = []*Location{{ID: "l1", Name: "Gilman House", Description: "A damp hotel lobby.", Danger: 1}, {ID: "l2", Name: "The Reef", Danger: 3}, {ID: "l3", Name: "Cellar"}}
	w.Edges = []*Edge{{ID: "e1", A: "l1", B: "l2", State: EdgeKnown}, {ID: "e2", A: "l1", B: "l3", State: EdgeHidden}}
	w.NPCs = []*NPC{
		{ID: "n1", Name: "Ada Brennan", LocationID: "l1", Alive: true, Disposition: -3, LastTalked: 2},
		{ID: "n2", Name: "Old Corbin", LocationID: "l1", Alive: false, LastTalked: 1},
		{ID: "n3", Name: "Constable Vane", LocationID: "l2", Alive: true, LastTalked: 3},
	}
	w.Characters = []*Character{{ID: "c1", PlayerID: "p1", Name: "Cal", HP: 10, MaxHP: 10}}
	w.Campaign.Turns.Order = []string{"c1"}
	return w
}

func TestNoVisionsWithoutStrainOrDread(t *testing.T) {
	w := visionWorld()
	r := dice.New(1)
	for i := 0; i < 200; i++ {
		if c, v := RollVision(w, r); v != nil {
			t.Fatalf("calm character had a vision: %v %+v", c.Name, v)
		}
		w.Campaign.Turn++
	}
	w.Characters[0].Strain = 5
	w.Campaign.Dread = false
	for i := 0; i < 200; i++ {
		if _, v := RollVision(w, r); v != nil {
			t.Fatal("a vision outside a dread campaign")
		}
		w.Campaign.Turn++
	}
}

func TestVisionsAreRecordedTrueOrFalse(t *testing.T) {
	w := visionWorld()
	c := w.Characters[0]
	c.Strain = 5
	r := dice.New(7)
	seen := map[string]bool{}
	reals, falses := 0, 0
	for i := 0; i < 600; i++ {
		_, v := RollVision(w, r)
		w.Campaign.Turn++
		if v == nil {
			continue
		}
		seen[v.Kind] = true
		if v.Seed == "" {
			t.Fatalf("vision without a seed: %+v", v)
		}
		switch v.Kind {
		case VisionHiddenWay, VisionHostile, VisionDanger:
			if !v.Real {
				t.Fatalf("a true kind marked false: %+v", v)
			}
			reals++
		default:
			if v.Real {
				t.Fatalf("a hallucination marked real: %+v", v)
			}
			falses++
		}
		if v.Kind == VisionPhantom && v.Phantom == "" {
			t.Fatalf("phantom without key: %+v", v)
		}
	}
	if reals == 0 || falses == 0 || falses < reals {
		t.Fatalf("at strain 5 most visions should be false: real %d false %d", reals, falses)
	}
	for _, k := range []string{VisionHiddenWay, VisionHostile, VisionDanger, VisionPhantom, VisionWhisper} {
		if !seen[k] {
			t.Errorf("never saw a %s vision (%v)", k, seen)
		}
	}
	if len(c.Visions) != MaxVisions {
		t.Fatalf("kept %d visions", len(c.Visions))
	}
	for i := 1; i < len(c.Visions); i++ {
		if c.Visions[i].Turn-c.Visions[i-1].Turn < 3 {
			t.Fatalf("visions too close together: %+v", c.Visions)
		}
	}
}

func TestTrueVisionsNeedTheWorldToBearThemOut(t *testing.T) {
	w := visionWorld()
	w.Edges = nil
	w.NPCs[0].Disposition = 0
	if vs := trueVisions(w); len(vs) != 0 {
		t.Fatalf("nothing hidden or hostile here, got %+v", vs)
	}
	c := w.Characters[0]
	if v := visionFor(w, dice.New(3), c, true); v.Real {
		t.Fatalf("a true vision with nothing to show: %+v", v)
	}
}

func TestUnrealVisionsExpire(t *testing.T) {
	w := visionWorld()
	c := w.Characters[0]
	c.Visions = []Vision{{Turn: 1, Kind: VisionPhantom, Phantom: "drowned woman"}, {Turn: 9, Kind: VisionHostile, Real: true}, {Turn: 9, Kind: VisionWhisper}}
	if us := UnrealVisions(w); len(us) != 2 || us[0].Character != "Cal" {
		t.Fatalf("unreal %+v", us)
	}
	w.Campaign.Turn = 1 + VisionMemory + 1
	if us := UnrealVisions(w); len(us) != 1 || us[0].Vision.Kind != VisionWhisper {
		t.Fatalf("old phantom should have faded: %+v", us)
	}
}
