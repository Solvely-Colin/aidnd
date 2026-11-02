package game

import (
	"fmt"
	"strings"
	"testing"
)

func TestGoalsAddSettleAndCap(t *testing.T) {
	f := newFixture(t)
	w := f.w
	g, err := AddGoal(w, f.aria, "Find my brother in the Whisperwood", SourcePlayer)
	if err != nil || g.ID != "goal1" || g.Status != GoalOpen {
		t.Fatalf("goal %+v %v", g, err)
	}
	if _, err := AddGoal(w, f.aria, "find my brother in the whisperwood", SourceGM); err == nil {
		t.Fatal("duplicate goal")
	}
	for i := 0; i < MaxOpenGoals-1; i++ {
		_, err := AddGoal(w, f.aria, fmt.Sprintf("goal number %d", i), SourceGM)
		must(t, err)
	}
	if _, err := AddGoal(w, f.aria, "one too many", SourceGM); err == nil {
		t.Fatal("open goal cap")
	}
	xp := f.aria.XP
	ch, err := SettleGoal(w, f.aria, g.ID, GoalDone, "he was alive in the hollow")
	if err != nil || !strings.Contains(strings.Join(ch, "|"), "reaches a goal") || f.aria.XP != xp+GoalXP {
		t.Fatalf("settle %v %v xp %d", ch, err, f.aria.XP)
	}
	if _, err := SettleGoal(w, f.aria, g.ID, GoalFailed, ""); err == nil {
		t.Fatal("settled twice")
	}
	if f.aria.GoalsSettledToday(w) != 1 {
		t.Fatal("settled today")
	}
	last := f.aria.Record[len(f.aria.Record)-1]
	if last.Kind != RecOutcome || last.GoalID != g.ID || !strings.Contains(last.Text, "he was alive") {
		t.Fatalf("record %+v", last)
	}
	if _, err := AddGoal(w, f.aria, "a fresh goal", SourcePlayer); err != nil {
		t.Fatal(err)
	}
	if f.aria.Goals[len(f.aria.Goals)-1].ID != fmt.Sprintf("goal%d", MaxOpenGoals+1) {
		t.Fatalf("ids %+v", f.aria.Goals)
	}
}

func TestRecordKeepsChoicesWhenFull(t *testing.T) {
	f := newFixture(t)
	w := f.w
	Note(w, f.bram, RecordEntry{Kind: RecChoice, Text: "Let the thief go"})
	for i := 0; i < MaxRecord+5; i++ {
		Note(w, f.bram, RecordEntry{Kind: RecDeed, Text: fmt.Sprintf("deed %d", i)})
	}
	if len(f.bram.Record) != MaxRecord || f.bram.Record[0].Text != "Let the thief go" {
		t.Fatalf("record len %d first %+v", len(f.bram.Record), f.bram.Record[0])
	}
	if Note(w, f.bram, RecordEntry{Kind: RecChoice, Text: "let the thief go."}) != nil {
		t.Fatal("duplicate entry")
	}
}

func TestEngineWritesThePersonalRecord(t *testing.T) {
	f := newFixture(t)
	w := f.w
	w.Campaign.Turns.Budget = FullBudget(false)
	// a promise made in conversation is the speaker's choice
	r := Resolve(f.ctx(), f.bram, []Action{{Kind: ActTalkTo, Target: f.elder.ID, Text: "I will", Promise: "find the missing shepherd"}})
	if stepErr(r, 0) != "" {
		t.Fatal(stepErr(r, 0))
	}
	if len(f.bram.Record) != 1 || f.bram.Record[0].Kind != RecChoice || !strings.Contains(f.bram.Record[0].Text, "shepherd") || len(f.aria.Record) != 0 {
		t.Fatalf("bram %+v aria %+v", f.bram.Record, f.aria.Record)
	}
	// attacking someone is the attacker's choice; killing them is their deed
	f.smith.ZoneID = f.aria.ZoneID
	r = Resolve(f.ctx(), f.aria, []Action{{Kind: ActAttack, Target: "Dunna"}})
	if stepErr(r, 0) != "" {
		t.Fatal(stepErr(r, 0))
	}
	e := w.Encounter.Living()[0]
	hurtEnemy(w, f.aria, e, 999)
	var texts []string
	for _, x := range f.aria.Record {
		texts = append(texts, x.Kind+":"+x.Text)
	}
	got := strings.Join(texts, "|")
	if !strings.Contains(got, "choice:Turned on Dunna") || !strings.Contains(got, "deed:Killed Dunna") {
		t.Fatalf("aria record %s", got)
	}
}
