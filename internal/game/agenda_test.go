package game

import (
	"strings"
	"testing"

	"github.com/Solvely-Colin/aidnd/internal/dice"
)

// agendaWorld has two places in one region, a third elsewhere, a lair,
// Ada at l1 and Bram at l2, and the party at l3.
func agendaWorld() *World {
	w := tickWorld(10)
	w.Locations = []*Location{{ID: "l1", Name: "Inn", Region: "r"}, {ID: "l2", Name: "Wharf", Region: "r"},
		{ID: "l3", Name: "Road", Region: "far"}, {ID: "l4", Name: "Crypt", Region: "r", IsLair: true}}
	w.NPCs = []*NPC{{ID: "n1", Name: "Ada", LocationID: "l1", Alive: true, LastTalked: 3},
		{ID: "n2", Name: "Bram", LocationID: "l2", Alive: true, LastTalked: -1}}
	w.Campaign.LocationID = "l3"
	w.Characters = []*Character{{ID: "c1", Name: "Cal", PlayerID: "u1", HP: 10, MaxHP: 10}}
	w.Players = []*Player{{ID: "u1", Name: "Colin", CharacterID: "c1"}}
	return w
}

// runAll ticks agendas until nothing can move.
func runAll(w *World) []AgendaMove {
	var out []AgendaMove
	for i := 0; i < 60; i++ {
		out = append(out, RunAgendas(w, dice.New(int64(i)))...)
	}
	return out
}

func TestAgendasMoveTheirPeople(t *testing.T) {
	w := agendaWorld()
	cult := &Faction{Key: "cult", Name: "Cult", Threat: true, Steps: 3, Moves: []string{"cult 1", "cult 2"}}
	w.Campaign.Living.Factions = []*Faction{cult}
	w.Campaign.Living.Agendas = []*Agenda{{Key: "ada", NPCID: "n1", Region: "r", Steps: []AgendaStep{
		{Text: "Ada goes to the wharf.", Effects: []AgendaEffect{{MoveTo: "l2"}}},
		{Text: "Ada sours on Bram.", Effects: []AgendaEffect{{NPCID: "n2", Disposition: -5}}},
		{Text: "Ada tells the cult.", Effects: []AgendaEffect{{Clock: "cult"}}},
		{Text: "Ada tells the cult again.", Effects: []AgendaEffect{{Clock: "cult"}}},
		{Text: "Ada tells the cult a third time.", Effects: []AgendaEffect{{Clock: "cult"}}},
	}}}
	moves := runAll(w)
	ada, bram := w.NPC("n1"), w.NPC("n2")
	if ada.LocationID != "l2" || bram.Disposition != -3 {
		t.Fatalf("ada at %s, bram disposition %d", ada.LocationID, bram.Disposition)
	}
	// an agenda can push a threat's clock but never fill it
	if cult.Progress != 2 || cult.Filled || len(moves) != 4 || moves[3].Clock != "cult 2" {
		t.Fatalf("cult %d filled=%v moves %+v", cult.Progress, cult.Filled, moves)
	}
	if a := w.Campaign.Living.AgendaFor("n1"); a.Done || a.Progress != 4 {
		t.Fatalf("the last step waits on the clock: %+v", a)
	}
}

func TestAgendasRespectTheStory(t *testing.T) {
	w := agendaWorld()
	lv := w.Campaign.Living
	lv.Agendas = []*Agenda{
		{Key: "lair", NPCID: "n2", Region: "r", Steps: []AgendaStep{{Text: "Bram goes into the crypt.", Effects: []AgendaEffect{{MoveTo: "l4"}}}}},
		{Key: "out", NPCID: "n2", Region: "r", Steps: []AgendaStep{{Text: "Bram leaves the region.", Effects: []AgendaEffect{{MoveTo: "l3"}}}}},
		{Key: "cond", NPCID: "n1", Steps: []AgendaStep{{Text: "Ada sells them out.", If: &AgendaIf{DispositionAtMost: ptr(-1)}}}},
		{Key: "later", NPCID: "n1", FromWeek: 5, Steps: []AgendaStep{{Text: "Ada's later plan."}}},
	}
	if moves := runAll(w); len(moves) != 0 {
		t.Fatalf("nothing should move: %+v", moves)
	}
	// the party standing with someone keeps them in the scene
	lv.Agendas = []*Agenda{{Key: "walk", NPCID: "n1", Region: "r", Steps: []AgendaStep{{Text: "Ada walks off.", Effects: []AgendaEffect{{MoveTo: "l2"}}}}}}
	w.Campaign.LocationID = "l1"
	if moves := runAll(w); len(moves) != 0 || w.NPC("n1").LocationID != "l1" {
		t.Fatal("Ada left a scene the party is in")
	}
	// conditions and weeks open up
	w.Campaign.LocationID = "l3"
	w.NPC("n1").Disposition = -1
	lv.Week = 5
	lv.Agendas = append(lv.Agendas,
		&Agenda{Key: "cond", NPCID: "n1", Steps: []AgendaStep{{Text: "Ada sells them out.", If: &AgendaIf{DispositionAtMost: ptr(-1)}}}},
		&Agenda{Key: "later", NPCID: "n1", FromWeek: 5, Steps: []AgendaStep{{Text: "Ada's later plan."}}})
	if moves := runAll(w); len(moves) != 3 {
		t.Fatalf("three steps once allowed: %+v", moves)
	}
	// the dead make no plans
	w.NPC("n2").Alive = false
	lv.Agendas = []*Agenda{{Key: "dead", NPCID: "n2", Steps: []AgendaStep{{Text: "Bram schemes."}}}}
	if moves := runAll(w); len(moves) != 0 || !lv.Agendas[0].Done {
		t.Fatal("a dead person's agenda moved")
	}
}

func ptr(n int) *int { return &n }

func TestDispatchTellsAPlayerWhatTheyMissed(t *testing.T) {
	w := agendaWorld()
	lv := w.Campaign.Living
	if BuildDispatch(w, "u1") != nil {
		t.Fatal("a player who never played has nothing to catch up on")
	}
	NoteSeen(w, "u1")
	if BuildDispatch(w, "u1") != nil {
		t.Fatal("no tick yet, no dispatch")
	}
	lv.Factions = []*Faction{{Key: "f", Name: "F", Region: "far", Steps: 9, Moves: []string{"F moves on the road"}},
		{Key: "g", Name: "G", Region: "r", Steps: 9, Moves: []string{"G moves by the inn"}}}
	lv.Agendas = []*Agenda{
		{Key: "ada", NPCID: "n1", Region: "r", Steps: []AgendaStep{{Text: "Ada went to the wharf.", Effects: []AgendaEffect{{MoveTo: "l2"}}}}},
		{Key: "bram", NPCID: "n2", Region: "r", Steps: []AgendaStep{{Text: "Bram mended his nets."}}},
	}
	w.Record(ChronicleEntry{Kind: FactDeed, Source: SourceEngine, Text: "Maren burned the Order's ledger."})
	w.Record(ChronicleEntry{Kind: FactNote, Source: SourceGM, Text: "The narrator mused about fog."})
	for i := 0; i < 12; i++ {
		WorldTick(w, dice.New(int64(i)), 0, false)
	}
	d := BuildDispatch(w, "u1")
	if d == nil || d.Weeks != 12 || d.FromWeek != 1 || d.TideFrom != 10 || d.Tide <= 10 {
		t.Fatalf("dispatch header: %+v", d)
	}
	kinds := map[string][]DispatchLine{}
	for _, l := range d.Lines {
		kinds[l.Kind] = append(kinds[l.Kind], l)
	}
	if p := kinds[DispatchPeople]; len(p) != 1 || !strings.Contains(p[0].Text, "Ada") || p[0].Where != "Wharf" {
		t.Fatalf("people you know: %+v", p)
	}
	if de := kinds[DispatchDeed]; len(de) != 1 || !strings.Contains(de[0].Text, "Maren") {
		t.Fatalf("deeds (engine facts only): %+v", de)
	}
	for _, l := range kinds[DispatchNear] {
		if !strings.Contains(l.Text, "F moves") {
			t.Fatalf("near the party (on the road) is F's region: %+v", l)
		}
	}
	for _, l := range kinds[DispatchFar] {
		if strings.Contains(l.Text, "F moves") {
			t.Fatalf("far lines: %+v", l)
		}
	}
	if len(kinds[DispatchNear]) > dispatchCaps[DispatchNear] {
		t.Fatalf("near lines are capped: %d", len(kinds[DispatchNear]))
	}
	NoteSeen(w, "u1")
	if BuildDispatch(w, "u1") != nil {
		t.Fatal("seen clears the dispatch")
	}
}
