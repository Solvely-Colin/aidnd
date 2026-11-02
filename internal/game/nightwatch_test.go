package game

import "testing"

func TestNightWatchSleepBringsTheVisitor(t *testing.T) {
	f := newFixture(t)
	w := f.w
	q := f.sideQuest(t,
		ObjectiveSpec{Kind: ObjReachLocation, Text: "Go to Whisperwood", Target: "Whisperwood"},
		ObjectiveSpec{Kind: ObjFreeform, Text: "Last the night in Whisperwood and end what visits it"},
	)
	Resolve(f.ctx(), f.aria, []Action{{Kind: ActAcceptQuest, Target: q.ID}})
	f.goTo(f.wood, 0)
	EvaluateQuests(w)
	if !q.Objectives[0].Done {
		t.Fatal("reach step should tick")
	}
	if got, _ := NightWatch(w, f.wood); got != q {
		t.Fatal("night watch not found for Whisperwood")
	}
	st := &Step{}
	if err := rest(f.ctx(100, 1, 1, 1, 1, 1, 1, 1, 1), f.aria, Action{Kind: ActRest, Method: RestNight}, st); err != nil {
		t.Fatal(err)
	}
	if w.Encounter == nil || w.Encounter.Quest != q.ID+"/"+q.Objectives[1].ID {
		t.Fatalf("sleeping on the night watch should start the visitor fight, got %+v (%v)", w.Encounter, st.Changes)
	}
	if q.Objectives[1].Done {
		t.Fatal("the step must not tick before the fight is won")
	}
	if _, _, err := CompleteFreeform(w, q.ID, q.Objectives[1].ID, "they slept"); err == nil {
		t.Fatal("the judge must not mark a night watch done")
	}
	for _, e := range w.Encounter.Enemies {
		e.HP = 0
	}
	endEncounter(w)
	EvaluateQuests(w)
	if !q.Objectives[1].Done || q.Status != QuestCompleted {
		t.Fatalf("winning the night fight should finish the quest: %+v %s", q.Objectives[1], q.Status)
	}
}

func TestNightWatchOnlyAtItsPlace(t *testing.T) {
	f := newFixture(t)
	q := f.sideQuest(t,
		ObjectiveSpec{Kind: ObjReachLocation, Text: "Go to Whisperwood", Target: "Whisperwood"},
		ObjectiveSpec{Kind: ObjFreeform, Text: "Last the night in Whisperwood and end what visits it"},
	)
	Resolve(f.ctx(), f.aria, []Action{{Kind: ActAcceptQuest, Target: q.ID}})
	if got, _ := NightWatch(f.w, f.hub); got != nil {
		t.Fatal("a night watch belongs to its place")
	}
	if IsNightWatch(&Objective{Kind: ObjFreeform, Text: "Find the night-gaunt's lair"}) {
		t.Fatal("not every mention of night is a watch")
	}
}
