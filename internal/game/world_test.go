package game

import (
	"strings"
	"testing"
)

func TestTravelPathBFS(t *testing.T) {
	f := newFixture(t)
	w := f.w
	if _, err := w.TravelPath(f.hub.ID, f.wood.ID); err == nil || !strings.Contains(err.Error(), "not been visited") {
		t.Fatalf("unvisited destination allowed: %v", err)
	}
	f.wood.Visited = true
	path, err := w.TravelPath(f.hub.ID, f.wood.ID)
	if err != nil || len(path) != 1 || path[0] != f.wood.ID {
		t.Fatalf("path %v %v", path, err)
	}
	// hidden and locked edges are not paths, even to visited places
	f.hollow.Visited, f.vault.Visited = true, true
	if _, err := w.TravelPath(f.hub.ID, f.hollow.ID); err == nil {
		t.Fatal("travelled over hidden edge")
	}
	if _, err := w.TravelPath(f.hub.ID, f.vault.ID); err == nil {
		t.Fatal("travelled over locked edge")
	}
	must(t, RevealExit(w, w.EdgeBetween(f.wood.ID, f.hollow.ID).ID))
	path, err = w.TravelPath(f.hub.ID, f.hollow.ID)
	if err != nil || len(path) != 2 {
		t.Fatalf("two-hop path %v %v", path, err)
	}
}

func TestTravelAction(t *testing.T) {
	f := newFixture(t)
	f.wood.Visited = true
	time0 := f.w.Campaign.Scene.Time
	// d100 = 100: no random encounter
	r := Resolve(f.ctx(100), f.aria, []Action{{Kind: ActTravel, Target: "Whisperwood"}})
	if stepErr(r, 0) != "" || f.w.Campaign.LocationID != f.wood.ID {
		t.Fatalf("travel failed: %s", stepErr(r, 0))
	}
	if f.w.Campaign.Scene.Time == time0 {
		t.Error("time did not advance")
	}
	if f.aria.ZoneID != f.wood.Zones[0].ID { // arrived via the road
		t.Errorf("arrival zone %s", f.aria.ZoneID)
	}
	// travel back with an ambush roll: d100 = 1 at danger... hub is danger 0, so no roll
	r = Resolve(f.ctx(1), f.bram, []Action{{Kind: ActTravel, Target: "Oakvale"}})
	if f.w.Campaign.LocationID != f.hub.ID || f.w.InCombat() {
		t.Fatalf("return trip: %s combat=%v", stepErr(r, 0), f.w.InCombat())
	}
}

func TestTravelAmbush(t *testing.T) {
	f := newFixture(t)
	f.wood.Visited = true
	r := Resolve(f.ctx(1), f.aria, []Action{{Kind: ActTravel, Target: "Whisperwood"}})
	if !f.w.InCombat() || !r.CombatStarted {
		t.Fatalf("no ambush: %+v", r.Steps)
	}
}

func TestFillLocationStaysConnected(t *testing.T) {
	f := newFixture(t)
	w := f.w
	before := len(w.Locations)
	notes, err := FillLocation(w, f.hollow.ID, LocationFill{
		Description: "Roots and dripping water.",
		NPCs:        []NPCSpec{{Name: "Dunna", Role: "smith"}, {Name: "Old Pell", Role: "hermit"}},
		Items:       []ItemSpec{{Key: "torch"}, {Name: "Fox Skull", Kind: "treasure", Value: 9999}},
		Exits: []ExitSpec{
			{To: "Oakvale", Label: "a muddy tunnel up"},
			{To: "A", Hint: "x"}, {To: "B"}, {To: "C"}, {To: "D"}, {To: "E"},
			{To: "Black Spire"},
		},
	})
	must(t, err)
	if f.hollow.Stub || f.hollow.Description == "" {
		t.Fatal("not filled")
	}
	if got := len(w.Locations) - before; got != MaxNewExitsPerFill {
		t.Fatalf("grew by %d, want %d", got, MaxNewExitsPerFill)
	}
	if w.EdgeBetween(f.hollow.ID, f.hub.ID) == nil {
		t.Fatal("existing place name did not link")
	}
	if w.EdgeBetween(f.hollow.ID, f.lair.ID) != nil {
		t.Fatal("filled location linked straight into the lair")
	}
	dunnas := 0
	for _, n := range w.NPCs {
		if n.Name == "Dunna" {
			dunnas++
		}
	}
	if dunnas != 1 {
		t.Fatalf("NPC duplicated (%d), notes %v", dunnas, notes)
	}
	for _, it := range w.ItemsOf(OwnerLocation, f.hollow.ID) {
		if it.Value > 500 {
			t.Fatalf("item value not clamped: %d", it.Value)
		}
	}
	if !w.Connected() {
		t.Fatal("graph disconnected")
	}
	if _, err := FillLocation(w, f.hollow.ID, LocationFill{}); err == nil {
		t.Fatal("refilled a filled location")
	}
}

func TestHiddenExitRevealedBySearch(t *testing.T) {
	f := newFixture(t)
	f.goTo(f.wood, 2)
	if f.w.FindExit(f.wood.ID, "fox trail") != nil {
		t.Fatal("hidden exit visible")
	}
	r := Resolve(f.ctx(15), f.bram, []Action{{Kind: ActCheck, Ability: "WIS", DC: 15, Purpose: "search"}})
	if !hasChange(r, "hidden way") || f.w.FindExit(f.wood.ID, "fox trail") == nil {
		t.Fatalf("not revealed: %v", r.Changes)
	}
}

func TestLockedExitNeedsKeyOrCheck(t *testing.T) {
	f := newFixture(t)
	f.goTo(f.hub, 2)
	r := Resolve(f.ctx(), f.aria, []Action{{Kind: ActMove, Target: "iron door"}})
	if !strings.Contains(stepErr(r, 0), "locked") {
		t.Fatalf("locked door passed: %q", stepErr(r, 0))
	}
	CustomItem(f.w, "Iron Key", ItemKey, "", 0, Owner{Kind: OwnerCharacter, ID: f.aria.ID})
	r = Resolve(f.ctx(), f.aria, []Action{{Kind: ActMove, Target: "Vault"}})
	if stepErr(r, 0) != "" || f.w.Campaign.LocationID != f.vault.ID {
		t.Fatalf("key did not open: %q", stepErr(r, 0))
	}
	if f.vault.Stub || len(f.vault.Zones) < MinZones {
		t.Fatal("arrival did not fill/zone the stub")
	}
}

func TestLockPickedByCheck(t *testing.T) {
	f := newFixture(t)
	f.goTo(f.hub, 2)
	r := Resolve(f.ctx(14), f.bram, []Action{{Kind: ActCheck, Purpose: "unlock", Target: "Vault"}})
	// DEX +2, prof +2 -> 18 vs 15
	if !hasChange(r, "is open") || f.w.EdgeBetween(f.hub.ID, f.vault.ID).State != EdgeKnown {
		t.Fatalf("lock not picked: %v %s", r.Changes, stepErr(r, 0))
	}
}

func TestLairGateCannotBeForced(t *testing.T) {
	f := newFixture(t)
	f.goTo(f.wood, 2)
	r := Resolve(f.ctx(20), f.aria, []Action{{Kind: ActCheck, Purpose: "force", Target: "Black Spire"}})
	if !strings.Contains(stepErr(r, 0), "story must progress") {
		t.Fatalf("gate forced: %q %v", stepErr(r, 0), r.Changes)
	}
	r = Resolve(f.ctx(), f.aria, []Action{{Kind: ActMove, Target: "Black Spire"}})
	if !strings.Contains(stepErr(r, 0), "sealed") {
		t.Fatalf("gate walked: %q", stepErr(r, 0))
	}
}

func TestOnArriveHookAuthorsStub(t *testing.T) {
	f := newFixture(t)
	f.goTo(f.wood, 2)
	must(t, RevealExit(f.w, f.w.EdgeBetween(f.wood.ID, f.hollow.ID).ID))
	called := 0
	ctx := f.ctx()
	ctx.OnArrive = func(l *Location) {
		called++
		_, err := FillLocation(f.w, l.ID, LocationFill{Description: "A damp hollow full of roots."})
		must(t, err)
		must(t, DefineZones(f.w, l.ID, []ZoneSpec{
			{Name: "Burrow Mouth", Adjacent: []string{"Root Hall"}, Exits: []string{"Whisperwood"}},
			{Name: "Root Hall", Cover: true},
		}))
	}
	r := Resolve(ctx, f.aria, []Action{{Kind: ActMove, Target: "fox trail"}, {Kind: ActLook}})
	if called != 1 || r.Arrived != f.hollow.ID {
		t.Fatalf("hook called %d, arrived %q, err %q", called, r.Arrived, stepErr(r, 0))
	}
	if f.aria.ZoneID != f.hollow.Zones[0].ID || f.bram.ZoneID != f.aria.ZoneID {
		t.Fatalf("party not placed in exit zone: %s", f.aria.ZoneID)
	}
	if !strings.Contains(strings.Join(r.Steps[1].Info, "|"), "damp hollow") {
		t.Fatalf("look after arrival: %v", r.Steps[1].Info)
	}
}

func TestZoneLayoutValidation(t *testing.T) {
	f := newFixture(t)
	if err := DefineZones(f.w, f.vault.ID, []ZoneSpec{{Name: "A"}, {Name: "B"}}); err == nil {
		t.Fatal("disconnected zones accepted")
	}
	if err := DefineZones(f.w, f.vault.ID, []ZoneSpec{{Name: "A"}}); err == nil {
		t.Fatal("single zone accepted")
	}
	if err := DefineZones(f.w, f.vault.ID, []ZoneSpec{{Name: "A", Adjacent: []string{"Nowhere"}}, {Name: "B"}}); err == nil {
		t.Fatal("unknown neighbour accepted")
	}
	must(t, DefineZones(f.w, f.vault.ID, []ZoneSpec{{Name: "A", Adjacent: []string{"B"}}, {Name: "B"}}))
	if l := f.vault; len(l.Zones[1].Adjacent) != 1 {
		t.Fatal("adjacency not symmetric")
	}
	// exits not mentioned land in the first zone
	if z := f.w.EdgeBetween(f.hub.ID, f.vault.ID).ZoneAt(f.vault.ID); z != f.vault.Zones[0].ID {
		t.Fatalf("unassigned exit zone %q", z)
	}
	for _, kind := range []string{KindTown, KindCave, KindDungeon, KindBuilding, KindWilderness} {
		w := &World{}
		l := &Location{ID: "x", Kind: kind}
		w.Locations = []*Location{l}
		EnsureZones(w, "x")
		if len(l.Zones) != 3 || ZoneDistance(l, l.Zones[0].ID, l.Zones[2].ID) != 2 {
			t.Errorf("generic %s layout bad", kind)
		}
	}
}
