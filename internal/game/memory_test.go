package game

import (
	"strings"
	"testing"
)

func TestRecordStampsAndDedupes(t *testing.T) {
	f := newFixture(t)
	w := f.w
	w.Campaign.Turn, w.Campaign.Scene.Day = 7, 3
	got := w.Record(ChronicleEntry{Text: "The mill burned down.", NPCIDs: []string{f.smith.ID}})
	if got == nil || got.ID != "fact1" || got.Day != 3 || got.Turn != 7 || got.LocationID != f.hub.ID || got.Kind != FactNote || got.Source != SourceGM {
		t.Fatalf("recorded %+v", got)
	}
	if w.Record(ChronicleEntry{Text: "the mill burned down"}) != nil || len(w.Chronicle) != 1 {
		t.Fatal("duplicate fact recorded")
	}
	if len(w.FactsAt(f.hub.ID)) != 1 || len(w.FactsAbout(f.smith.ID)) != 1 || len(w.FactsAbout(f.elder.ID)) != 0 {
		t.Fatal("fact lookups")
	}
}

func TestPromisesKeptAndBroken(t *testing.T) {
	f := newFixture(t)
	w := f.w
	smith, elder := f.smith.Disposition, f.elder.Disposition
	a := w.Record(ChronicleEntry{Kind: FactPromise, Text: "Aria swore to bring Dunna iron", NPCIDs: []string{f.smith.ID}})
	b := w.Record(ChronicleEntry{Kind: FactPromise, Text: "Bram promised Maren silence", NPCIDs: []string{f.elder.ID}})
	if a.Status != PromiseOpen || len(w.OpenPromises()) != 2 {
		t.Fatalf("open promises %+v", w.OpenPromises())
	}
	if _, err := ResolvePromise(w, a.ID, true, "brought the iron"); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolvePromise(w, b.ID, false, "told the whole tavern"); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolvePromise(w, a.ID, false, ""); err == nil {
		t.Fatal("resolved a promise twice")
	}
	if f.smith.Disposition != smith+1 || f.elder.Disposition != elder-1 {
		t.Fatalf("dispositions %d %d", f.smith.Disposition, f.elder.Disposition)
	}
	if !strings.Contains(strings.Join(f.elder.Recall(), "|"), "broke their word") || len(w.OpenPromises()) != 0 {
		t.Fatalf("elder recalls %v", f.elder.Recall())
	}
}

func TestQuestPromiseIsKeptByTheQuestOnly(t *testing.T) {
	f := newFixture(t)
	w := f.w
	q, err := AddQuest(w, QuestSpec{Title: "Find the cat", GiverID: f.elder.ID, Objectives: []ObjectiveSpec{{Kind: ObjFreeform, Text: "Find it"}}}, false)
	must(t, err)
	if _, err := AcceptQuest(w, q.ID); err != nil {
		t.Fatal(err)
	}
	p := w.OpenPromises()
	if len(p) != 1 || p[0].QuestID != q.ID || p[0].NPCIDs[0] != f.elder.ID || !strings.Contains(p[0].Text, "Elder Maren") {
		t.Fatalf("quest promise %+v", p)
	}
	if _, err := ResolvePromise(w, p[0].ID, false, ""); err == nil {
		t.Fatal("narrator broke a quest promise")
	}
}

func TestNPCKeepsSurviveChatter(t *testing.T) {
	n := &NPC{}
	n.Keep("the party saved my son")
	for i := 0; i < MaxNPCMemory+3; i++ {
		n.Remember(strings.Repeat("y", i+1))
	}
	if got := n.Recall(); got[0] != "the party saved my son" || len(got) != MaxNPCMemory+1 {
		t.Fatalf("recall %v", got)
	}
	for i := 0; i < MaxNPCKeeps+2; i++ {
		n.Keep(strings.Repeat("k", i+1))
	}
	if len(n.Keeps) != MaxNPCKeeps {
		t.Fatalf("keeps %d", len(n.Keeps))
	}
}

func TestKillingANamedNPCIsRemembered(t *testing.T) {
	f := newFixture(t)
	w := f.w
	w.Campaign.Turns.Budget = FullBudget(false)
	f.smith.ZoneID = f.aria.ZoneID
	r := Resolve(f.ctx(), f.aria, []Action{{Kind: ActAttack, Target: "Dunna"}})
	if stepErr(r, 0) != "" {
		t.Fatal(stepErr(r, 0))
	}
	if !strings.Contains(strings.Join(f.smith.Recall(), "|"), "attacked") {
		t.Fatalf("smith forgot the attack: %v", f.smith.Recall())
	}
	e := w.Encounter.Living()[0]
	defeat(w, e, false)
	facts := w.FactsAbout(f.smith.ID)
	if len(facts) != 1 || facts[0].Kind != FactDeath || facts[0].Source != SourceEngine || !strings.Contains(facts[0].Text, "Oakvale") {
		t.Fatalf("death facts %+v", facts)
	}
}

func TestReturningNotesTimeAway(t *testing.T) {
	f := newFixture(t)
	w := f.w
	w.Campaign.Scene.Day = 2
	st := &Step{}
	arrive(f.ctx(), f.wood, nil, st)
	if f.hub.LastDay != 2 || f.wood.LastDay != 2 {
		t.Fatalf("last day hub %d wood %d", f.hub.LastDay, f.wood.LastDay)
	}
	w.Campaign.Scene.Day = 5
	st = &Step{}
	arrive(f.ctx(), f.hub, nil, st)
	if len(st.Info) != 1 || !strings.Contains(st.Info[0], "3 days away") {
		t.Fatalf("info %v", st.Info)
	}
	// the party left the wood today, so going straight back is no news
	st = &Step{}
	arrive(f.ctx(), f.wood, nil, st)
	if len(st.Info) != 0 {
		t.Fatalf("same-day return %v", st.Info)
	}
}

func TestTalkWithAPromiseRecordsIt(t *testing.T) {
	f := newFixture(t)
	f.w.Campaign.Turns.Budget = FullBudget(false)
	Resolve(f.ctx(), f.aria, []Action{{Kind: ActTalkTo, Target: "Elder Maren", Text: "I'll find him, I swear", Promise: "find the elder's son"}})
	p := f.w.OpenPromises()
	if len(p) != 1 || p[0].NPCIDs[0] != f.elder.ID || !strings.Contains(p[0].Text, "Aria promised Elder Maren: find the elder's son") {
		t.Fatalf("promises %+v", p)
	}
	if !strings.Contains(strings.Join(f.elder.Recall(), "|"), "promised me") {
		t.Fatalf("elder recall %v", f.elder.Recall())
	}
}
