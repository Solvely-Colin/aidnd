package game

import (
	"strings"
	"testing"

	"github.com/Solvely-Colin/aidnd/internal/dice"
)

// eventWorld has a region "r" with a hub, a harbor and a wharf, a road
// in another region, a cult faction four steps into six, Ada and Bram in
// the region, and Cal and Maren at the hub, week 5.
func eventWorld() (*World, *Faction) {
	cult := &Faction{Key: "cult", Name: "Order", Clock: "The Bell", Threat: true, Region: "r", Steps: 6, Progress: 3,
		Foe: "Acolyte", Champion: "Deep One", Full: "The Order holds the town.", FullDanger: map[string]int{"l1": 3},
		Call: "The bell is ringing."}
	w := tickWorld(30, cult)
	w.Campaign.Living.Week = 5
	w.Locations = []*Location{{ID: "l0", Name: "Hub", Region: "r", IsHub: true},
		{ID: "l1", Name: "Harbor", Region: "r", Danger: 1, Kind: KindTown},
		{ID: "l2", Name: "Wharf", Region: "r", Danger: 1},
		{ID: "l3", Name: "Road", Region: "far", Danger: 1}}
	for _, l := range w.Locations {
		EnsureZones(w, l.ID)
	}
	w.NPCs = []*NPC{{ID: "n1", Name: "Ada", LocationID: "l0", Alive: true, LastTalked: 1},
		{ID: "n2", Name: "Bram", LocationID: "l2", Alive: true, LastTalked: 1},
		{ID: "n3", Name: "Gus", LocationID: "l2", Alive: true, LastTalked: 1}}
	w.Campaign.LocationID = "l0"
	w.Campaign.Status = StatusActive
	w.Characters = []*Character{
		{ID: "c1", Name: "Cal", PlayerID: "u1", Level: 2, HP: 20, MaxHP: 20},
		{ID: "c2", Name: "Maren", PlayerID: "u2", Level: 2, HP: 20, MaxHP: 20}}
	w.Players = []*Player{{ID: "u1", Name: "Colin", CharacterID: "c1"}, {ID: "u2", Name: "Sam", CharacterID: "c2"}}
	w.Campaign.Turns.Order = []string{"c1", "c2"}
	return w, cult
}

// tangle gives both characters threads with the Order: Ada owes Cal, Cal
// owes Bram, Gus holds a grudge against Maren, and Maren wants the bell
// silenced.
func tangle(w *World) {
	cal, maren := w.Character("c1"), w.Character("c2")
	w.NPC("n1").Bind(w, cal, TieDebt, "saved her shop", SourceEngine)
	w.NPC("n2").Bind(w, cal, TieOwed, "a loan", SourceEngine)
	w.NPC("n3").Bind(w, maren, TieGrudge, "struck him", SourceEngine)
	_, _ = AddGoal(w, maren, "Silence The Bell for good", SourcePlayer) // already there on a second call
}

func TestNoEventWithoutWhatPlayersDid(t *testing.T) {
	w, cult := eventWorld()
	cult.Progress = 5 // the clock is about to fill, but nobody is tangled up in it
	if e := MaybeCallEvent(w, 100); e != nil {
		t.Fatalf("an event came from the clock alone: %+v", e)
	}
	// one character's single thin thread is not enough at half a clock
	cult.Progress = 3
	Note(w, w.Character("c1"), RecordEntry{Kind: RecChoice, NPCID: "n2", Text: "Lied to Bram"})
	if e := MaybeCallEvent(w, 100); e != nil {
		t.Fatalf("one thin thread called an event: %+v", e)
	}
	// but if the clock is about to fill, anyone tangled up is called
	cult.Progress = 5
	if e := MaybeCallEvent(w, 100); e == nil {
		t.Fatal("a clock about to fill with a character in it should come to a head")
	}
}

func TestEventIsBornFromThreads(t *testing.T) {
	w, cult := eventWorld()
	tangle(w)
	w.Campaign.Living.Week = 2
	if MaybeCallEvent(w, 100) != nil {
		t.Fatal("too early in the world")
	}
	w.Campaign.Living.Week = 5
	e := MaybeCallEvent(w, 100)
	if e == nil {
		t.Fatal("no event from four threads")
	}
	if e.SiteID != "l1" || e.Deadline != 100+EventMuster || len(e.Summoned) != 2 || e.Status != EventGathering {
		t.Fatalf("event %+v", e)
	}
	kinds := map[string]bool{}
	for _, th := range e.Threads {
		kinds[th.Kind] = true
	}
	for _, k := range []string{ThreadAlly, ThreadDebt, ThreadGrudge, ThreadGoal, ThreadPressure} {
		if !kinds[k] {
			t.Fatalf("missing %s thread: %+v", k, e.Threads)
		}
	}
	if !strings.Contains(e.Call, "Cal and Maren") || !strings.Contains(e.Title, "The Bell") {
		t.Fatalf("call %q title %q", e.Call, e.Title)
	}
	if MaybeCallEvent(w, 200) != nil {
		t.Fatal("two events at once")
	}
	// the faction's clock waits for the event to decide it
	cult.Progress = 5
	for i := 0; i < 30; i++ {
		WorldTick(w, dice.New(int64(i)), 0, false)
	}
	if cult.Filled {
		t.Fatal("the clock filled while the event was deciding it")
	}
}

func TestAnswerBeginAndWin(t *testing.T) {
	w, cult := eventWorld()
	tangle(w)
	// Maren has gone off alone to the road
	g, err := SplitOff(w, []string{"c2"}, 0)
	must(t, err)
	g.LocationID = "l3"
	e := MaybeCallEvent(w, 100)
	if e == nil {
		t.Fatal("no event")
	}
	cal, maren := w.Character("c1"), w.Character("c2")
	if _, _, _, err := AnswerCall(w, cal); err != nil {
		t.Fatal(err)
	}
	if w.Campaign.LocationID != "l1" || EveryoneThere(w, e) || !e.Came("c1") {
		t.Fatalf("Cal at %s, came %v", w.Campaign.LocationID, e.Answered)
	}
	grp, moved, joins, err := AnswerCall(w, maren)
	must(t, err)
	if !moved || len(joins) != 1 || joins[0].Group != MainGroup || joins[0].Into != g.ID {
		t.Fatalf("joins %+v", joins)
	}
	if grp != MainGroup || w.Split() || !EveryoneThere(w, e) {
		t.Fatalf("they should be one group now: %s split=%v", grp, w.Split())
	}
	must(t, w.Focus(grp))
	changes, err := BeginEvent(w, dice.New(3), 500)
	must(t, err)
	text := strings.Join(changes, " | ")
	for _, want := range []string{"Ada comes to stand with Cal", "Bram is here", "Gus tipped them off"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
	if cal.TempHP == 0 || w.NPC("n1").LocationID != "l1" || w.NPC("n2").LocationID != "l1" || w.NPC("n3").LocationID != "l2" {
		t.Fatalf("allies: temp %d, Ada at %s, Bram at %s, Gus at %s", cal.TempHP, w.NPC("n1").LocationID, w.NPC("n2").LocationID, w.NPC("n3").LocationID)
	}
	if !w.InCombat() || e.Status != EventEngaged || e.EncounterID != w.Encounter.ID || w.Here().Danger != 1 {
		t.Fatalf("fight %v status %s danger %d", w.InCombat(), e.Status, w.Here().Danger)
	}
	acolytes := 0
	for _, en := range w.Encounter.Enemies {
		if strings.HasPrefix(en.Name, "Acolyte") {
			acolytes++
		}
	}
	// 2 + 3/2 = 3, one ally fewer, one grudge more
	if acolytes != 3 {
		var ns []string
		for _, en := range w.Encounter.Enemies {
			ns = append(ns, en.Name+"/"+en.Display())
		}
		t.Fatalf("acolytes %d: %v", acolytes, ns)
	}
	for _, en := range w.Encounter.Enemies {
		en.HP = 0
	}
	won := EndFight(w)
	w.Settle()
	if !strings.Contains(strings.Join(won, " "), "broke the Order") {
		t.Fatalf("won: %v", won)
	}
	if res := w.TakeEventResults(); len(res) != 1 || !res[0].Won {
		t.Fatalf("results %+v", res)
	}
	lv := w.Campaign.Living
	if CurrentEvent(w) != nil || len(lv.Past) != 1 || !cult.Broken || cult.active(w) || lv.Tide != 25 || lv.LastEventWeek != 5 {
		t.Fatalf("after: past %d broken %v tide %d", len(lv.Past), cult.Broken, lv.Tide)
	}
	if len(w.NPC("n1").TiesTo("c1")) != 0 || len(w.NPC("n2").TiesTo("c1")) != 0 {
		t.Fatal("the debts should be settled")
	}
	if !recorded(cal, "Stood against the Order") || !recorded(maren, "Stood against the Order") || !recorded(cal, "Paid back Bram") {
		t.Fatalf("records: %+v / %+v", cal.Record, maren.Record)
	}
	if EventFightLost(w) != nil {
		t.Fatal("nothing left to lose")
	}
	// cooldown
	tangle(w)
	if MaybeCallEvent(w, 1000) != nil {
		t.Fatal("no new event during the cooldown, nor from a broken faction")
	}
}

func recorded(c *Character, sub string) bool {
	for _, e := range c.Record {
		if strings.Contains(e.Text, sub) {
			return true
		}
	}
	return false
}

func TestNobodyComesAndTheWorldChanges(t *testing.T) {
	w, cult := eventWorld()
	tangle(w)
	e := MaybeCallEvent(w, 100)
	if begin, res := EventDeadline(w, 100+EventMuster-1); begin || res != nil {
		t.Fatal("too soon")
	}
	begin, res := EventDeadline(w, 100+EventMuster)
	if begin || res == nil || res.Won {
		t.Fatalf("begin %v res %+v", begin, res)
	}
	if !cult.Filled || w.Location("l1").Danger != 3 || w.Campaign.Living.Tide != 35 || e.Status != EventLost {
		t.Fatalf("filled %v danger %d tide %d", cult.Filled, w.Location("l1").Danger, w.Campaign.Living.Tide)
	}
	// Cal owed Bram and never came
	if ties := w.NPC("n2").TiesTo("c1"); len(ties) == 0 || ties[len(ties)-1].Kind != TieGrudge {
		t.Fatalf("Bram's ties: %+v", ties)
	}
	if !recorded(w.Character("c2"), "Was elsewhere") {
		t.Fatal("not coming is a choice on the record")
	}
}

func TestDeadlineStartsTheFightForWhoeverCame(t *testing.T) {
	w, _ := eventWorld()
	tangle(w)
	e := MaybeCallEvent(w, 100)
	must(t, func() error { _, _, _, err := AnswerCall(w, w.Character("c1")); return err }())
	begin, res := EventDeadline(w, 100+EventMuster)
	if !begin || res != nil || SiteGroup(w, e) != MainGroup {
		t.Fatalf("begin %v res %+v", begin, res)
	}
	must(t, w.Focus(SiteGroup(w, e)))
	_, err := BeginEvent(w, dice.New(1), 1)
	must(t, err)
	// they flee: the fight is gone without being won
	w.Encounter = nil
	if res := EventFightLost(w); res == nil || res.Won {
		t.Fatalf("fleeing should lose it: %+v", res)
	}
}
