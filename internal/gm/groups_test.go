package gm

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/llm"
)

func TestGoSoloTravelApartAndJoinBack(t *testing.T) {
	r := newRig(t, 2, 1)
	w := r.start(t)
	hub := w.Here().ID
	r.f.Default("narrate", llm.Text("The road bends."))

	w, err := r.g.GoSolo(bg, r.id, "u2")
	must(t, err)
	if len(w.Campaign.Groups) != 1 {
		t.Fatalf("groups: %d", len(w.Campaign.Groups))
	}
	solo := w.Campaign.Groups[0].ID
	if game.GroupOf(w.CharacterOf("u2")) != solo || game.GroupOf(w.CharacterOf("u1")) != game.MainGroup {
		t.Fatal("Bram should be alone in a new group")
	}
	if _, err := r.g.GoSolo(bg, r.id, "u2"); err == nil {
		t.Fatal("a group of one can't split again")
	}

	// Bram walks to the marsh alone
	r.act(t, "u2", "I head to the marsh", llm.Call("move", map[string]any{"exit": "Reedmarsh"}))
	w = r.world(t)
	if w.Here().ID != hub {
		t.Fatalf("the main group must stay at the hub, is at %s", w.Here().Name)
	}
	must(t, w.FocusPlayer("u2"))
	marsh := w.Here()
	if marsh.Name != "Reedmarsh" || len(w.Characters) != 1 || w.Characters[0].Name != "Bram" {
		t.Fatalf("Bram's view: at %s with %d characters", marsh.Name, len(w.Characters))
	}
	w.Settle()
	if len(w.Characters) != 2 {
		t.Fatalf("settle lost characters: %d", len(w.Characters))
	}
	// his move is tagged with his group; the start was not
	for _, e := range r.events(t, "location_changed") {
		var d map[string]any
		_ = json.Unmarshal(e.Data, &d)
		if d["name"] == "Reedmarsh" && d["party"] != solo {
			t.Fatalf("Bram's arrival not tagged with his group: %v", d)
		}
	}

	// Bram's narrator knew Aria stayed behind
	reqs := r.f.Tagged("narrate")
	if p := lastUser(reqs[len(reqs)-1]); !strings.Contains(p, "OTHER PLAYER CHARACTERS") || !strings.Contains(p, "Aria at Harrowmere") {
		t.Fatalf("narrator not told where Aria is:\n%s", p)
	}

	// can't join while apart
	if _, err := r.g.JoinGroup(bg, r.id, "u1", solo); err == nil {
		t.Fatal("joining from another place must fail")
	}
	r.act(t, "u1", "I follow Bram", llm.Call("move", map[string]any{"exit": "Reedmarsh"}))
	reqs = r.f.Tagged("narrate")
	if p := lastUser(reqs[len(reqs)-1]); !strings.Contains(p, "Bram (another group, here") {
		t.Fatalf("narrator not told Bram is here:\n%s", p)
	}
	w, err = r.g.JoinGroup(bg, r.id, "u1", solo)
	must(t, err)
	w = r.world(t)
	if len(w.Campaign.Groups) != 0 || game.GroupOf(w.CharacterOf("u2")) != game.MainGroup {
		t.Fatal("joining should fold everyone back into the main group")
	}
	if w.Here().Name != "Reedmarsh" || len(w.Campaign.Turns.Order) != 2 {
		t.Fatalf("joined group at %s with order %v", w.Here().Name, w.Campaign.Turns.Order)
	}
}

func TestSplitRefusedInAFight(t *testing.T) {
	w := &game.World{Campaign: game.Campaign{LocationID: "l1"}}
	w.Characters = []*game.Character{{ID: "c1", Name: "A"}, {ID: "c2", Name: "B"}}
	w.Encounter = &game.Encounter{LocationID: "l1", Enemies: []*game.Enemy{{Name: "Ghoul", HP: 5, StatBlock: game.StatBlock{MaxHP: 5}}}}
	if _, err := game.SplitOff(w, []string{"c2"}, 0); err == nil {
		t.Fatal("split in a fight must fail")
	}
	w.Encounter = nil
	g, err := game.SplitOff(w, []string{"c2"}, 0)
	must(t, err)
	// focus swaps state in and out without losing anything
	w.Campaign.LocationID = "l1"
	g.LocationID = "l2"
	must(t, w.Focus(g.ID))
	if w.Campaign.LocationID != "l2" || len(w.Characters) != 1 {
		t.Fatalf("focused: at %s with %d", w.Campaign.LocationID, len(w.Characters))
	}
	w.Settle()
	if w.Campaign.LocationID != "l1" || w.Group(g.ID).LocationID != "l2" || len(w.Characters) != 2 {
		t.Fatal("settle did not restore the world")
	}
}

func TestJoinSomeoneElsesFight(t *testing.T) {
	w := &game.World{Campaign: game.Campaign{LocationID: "l1"}}
	w.Characters = []*game.Character{{ID: "c1", Name: "A"}, {ID: "c2", Name: "B"}, {ID: "c3", Name: "C"}}
	w.Campaign.Turns.Order = []string{"c1", "c2", "c3"}
	g, err := game.SplitOff(w, []string{"c3"}, 0)
	must(t, err)
	// the main group gets into a fight; it is B's turn, half spent
	w.Encounter = &game.Encounter{LocationID: "l1", Round: 2, Enemies: []*game.Enemy{{Name: "Ghoul", HP: 5, StatBlock: game.StatBlock{MaxHP: 5}}}}
	w.Campaign.Turns.Index = 1
	w.Campaign.Turns.Budget = game.Budget{Action: 0, Move: 1}
	if err := game.JoinGroups(w, game.MainGroup, g.ID); err == nil {
		t.Fatal("a fighting group can't pull others in")
	}
	must(t, game.JoinGroups(w, g.ID, game.MainGroup))
	if w.Split() || w.Encounter == nil || w.Encounter.Round != 2 {
		t.Fatal("C should be in the main group's fight")
	}
	if o := w.Campaign.Turns.Order; len(o) != 3 || o[2] != "c3" || w.Campaign.Turns.Index != 1 || w.Campaign.Turns.Budget.Move != 1 {
		t.Fatalf("C goes last and B's turn carries on: %v index %d %+v", o, w.Campaign.Turns.Index, w.Campaign.Turns.Budget)
	}
}
