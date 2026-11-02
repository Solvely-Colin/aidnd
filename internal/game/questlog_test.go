package game

import (
	"testing"
)

// splitBram sends Bram off alone to Whisperwood and focuses back on Aria.
func (f *fixture) splitBram(t *testing.T) *Group {
	t.Helper()
	g, err := SplitOff(f.w, []string{f.bram.ID}, 0)
	must(t, err)
	g.LocationID = f.wood.ID
	f.wood.Visited = true
	must(t, f.w.Focus(MainGroup))
	return g
}

// Only the characters present hear of a quest; someone off on their own
// does not, and it moves forward only for whoever took it on.
func TestQuestLogIsPerCharacter(t *testing.T) {
	f := newFixture(t)
	g := f.splitBram(t)
	q := f.sideQuest(t,
		ObjectiveSpec{Kind: ObjReachLocation, Text: "Go to Whisperwood", Target: "Whisperwood"},
		ObjectiveSpec{Kind: ObjFreeform, Text: "Find the den"})
	if !KnowsQuest(f.w, q, f.aria) || KnowsQuest(f.w, q, f.bram) {
		t.Fatalf("known %v: Aria was there, Bram was not", q.Known)
	}
	// Bram is standing at Whisperwood but knows nothing of it
	must(t, f.w.Focus(g.ID))
	if ups, _ := EvaluateQuests(f.w); len(ups) != 0 || q.Objectives[0].Done {
		t.Fatalf("Bram ticked a quest he never heard of: %v", ups)
	}
	must(t, f.w.Focus(MainGroup))
	_, ups, err := TakeOn(f.w, q.ID, f.aria)
	must(t, err)
	if q.Status != QuestActive || len(q.Takers) != 1 || f.aria.Tracked != q.ID || len(ups) != 1 || ups[0].Kind != QuestTaken {
		t.Fatalf("take on: %s takers %v tracked %q ups %+v", q.Status, q.Takers, f.aria.Tracked, ups)
	}
	// still not Bram's: being there does nothing for Aria's quest
	must(t, f.w.Focus(g.ID))
	if EvaluateQuests(f.w); q.Objectives[0].Done {
		t.Fatal("Bram standing there ticked Aria's step")
	}
	// Aria goes herself
	must(t, f.w.Focus(MainGroup))
	f.goTo(f.wood, 0)
	xp := f.aria.XP
	ups, _ = EvaluateQuests(f.w)
	o := q.Objectives[0]
	if !o.Done || o.DoneBy != "Aria" || o.DoneDay != f.w.Campaign.Scene.Day {
		t.Fatalf("step %+v ups %+v", o, ups)
	}
	if f.aria.XP <= xp || ups[0].Characters[0] != f.aria.ID {
		t.Fatalf("XP %d->%d, news for %v", xp, f.aria.XP, ups[0].Characters)
	}
}

// A party quest is taken on together by the group that heard it; one of
// them can drop it and the other carries on; when the last drops it the
// promise to the giver is broken and it is a lead again.
func TestTakeOnTogetherAndDrop(t *testing.T) {
	f := newFixture(t)
	q := f.sideQuest(t, ObjectiveSpec{Kind: ObjFreeform, Text: "Find the den"})
	_, _, err := TakeOn(f.w, q.ID, f.aria)
	must(t, err)
	if !OnQuest(q, f.bram) {
		t.Fatalf("Bram was with Aria and knew of it: takers %v", q.Takers)
	}
	_, err = DropQuest(f.w, q.ID, f.bram)
	must(t, err)
	if OnQuest(q, f.bram) || !OnQuest(q, f.aria) || q.Status != QuestActive {
		t.Fatalf("after Bram drops: takers %v status %s", q.Takers, q.Status)
	}
	disp := f.elder.Disposition
	_, err = DropQuest(f.w, q.ID, f.aria)
	must(t, err)
	if q.Status != QuestOffered || f.elder.Disposition >= disp {
		t.Fatalf("after both drop: status %s, elder %d -> %d", q.Status, disp, f.elder.Disposition)
	}
	if _, _, err := TakeOn(f.w, q.ID, f.aria); err != nil {
		t.Fatalf("a dropped quest can be taken on again: %v", err)
	}
	if OnQuest(q, f.bram) {
		t.Fatal("Bram set it aside by dropping it; taking it on again should not drag him back")
	}
}

// A personal quest stays with the one character.
func TestPersonalQuest(t *testing.T) {
	f := newFixture(t)
	q, err := AddQuest(f.w, QuestSpec{Title: "A Letter for Aria", Source: SourceFound, Lead: "A letter in her coat", Personal: true,
		Objectives: []ObjectiveSpec{{Kind: ObjFreeform, Text: "Read it"}}}, false)
	must(t, err)
	if _, ok := NewLead(f.w, q, f.aria); !ok {
		t.Fatal("no lead")
	}
	if KnowsQuest(f.w, q, f.bram) || !KnowsQuest(f.w, q, f.aria) {
		t.Fatalf("known %v", q.Known)
	}
	if _, err := TellOfQuest(f.w, q.ID, f.aria); err == nil {
		t.Fatal("a personal quest cannot be passed on")
	}
	_, _, err = TakeOn(f.w, q.ID, f.aria)
	must(t, err)
	if OnQuest(q, f.bram) {
		t.Fatal("personal quest dragged Bram in")
	}
}

// A quest nobody offered needs no giver and pays out when it is done.
func TestDiscoveredQuestNeedsNoGiver(t *testing.T) {
	f := newFixture(t)
	q, err := AddQuest(f.w, QuestSpec{Title: "The Drowned Bell", Source: SourceOverheard, Lead: "Two fishers whispering at the pier",
		RewardXP: 30, Objectives: []ObjectiveSpec{{Kind: ObjReachLocation, Text: "Go to Whisperwood", Target: "Whisperwood"}}}, false)
	must(t, err)
	if q.GiverID != "" || q.Source != SourceOverheard || q.FoundAt != f.hub.ID {
		t.Fatalf("quest %+v", q.QuestExtra)
	}
	if _, err := AddQuest(f.w, QuestSpec{Title: "Nobody asked", Objectives: []ObjectiveSpec{{Kind: ObjFreeform, Text: "x"}}}, false); err == nil {
		t.Fatal("an offered quest still needs a giver")
	}
	_, _, err = TakeOn(f.w, q.ID, f.aria)
	must(t, err)
	f.goTo(f.wood, 0)
	xp := f.aria.XP
	EvaluateQuests(f.w)
	if q.Status != QuestTurnedIn || f.aria.XP < xp+30 {
		t.Fatalf("status %s xp %d -> %d", q.Status, xp, f.aria.XP)
	}
}

// Picking up the thing an unheard quest is after turns it into a lead.
func TestFindingTheItemRevealsTheQuest(t *testing.T) {
	f := newFixture(t)
	q := f.sideQuest(t, ObjectiveSpec{Kind: ObjObtainItem, Text: "Find the silver pelt", Target: "Silver Pelt"})
	q.Known = nil // offered somewhere nobody heard
	it := f.w.FindItemOf(OwnerLocation, q.Objectives[0].Where, "Silver Pelt")
	if it == nil {
		for _, x := range f.w.Items {
			if x.Name == "Silver Pelt" {
				it = x
			}
		}
	}
	if it == nil {
		t.Fatal("no pelt")
	}
	it.Owner = Owner{Kind: OwnerCharacter, ID: f.aria.ID}
	ups, _ := EvaluateQuests(f.w)
	if !KnowsQuest(f.w, q, f.aria) || !KnowsQuest(f.w, q, f.bram) || len(ups) == 0 || ups[0].Kind != QuestLead {
		t.Fatalf("known %v ups %+v", q.Known, ups)
	}
}

// Telling friends standing with you passes a party quest on.
func TestTellOfQuest(t *testing.T) {
	f := newFixture(t)
	q := f.sideQuest(t, ObjectiveSpec{Kind: ObjFreeform, Text: "x"})
	q.Known = []string{f.aria.ID}
	heard, err := TellOfQuest(f.w, q.ID, f.aria)
	must(t, err)
	if len(heard) != 1 || heard[0] != f.bram || !KnowsQuest(f.w, q, f.bram) {
		t.Fatalf("heard %v", heard)
	}
	if _, err := TellOfQuest(f.w, q.ID, f.aria); err == nil {
		t.Fatal("telling again should say nobody new is here")
	}
}

// Quests saved before the quest log keep working: everyone knows them
// and an active one is everyone's.
func TestOldQuestsStayShared(t *testing.T) {
	f := newFixture(t)
	q := f.sideQuest(t, ObjectiveSpec{Kind: ObjReachLocation, Text: "Go to Whisperwood", Target: "Whisperwood"})
	q.QuestExtra = QuestExtra{}
	q.Status = QuestActive
	g := f.splitBram(t)
	if !OnQuest(q, f.aria) || !OnQuest(q, f.bram) {
		t.Fatal("an old active quest is everyone's")
	}
	must(t, f.w.Focus(g.ID))
	if EvaluateQuests(f.w); !q.Objectives[0].Done {
		t.Fatal("old quests tick for anyone, as before")
	}
}

// Sharing reaches a friend anywhere in the world, even one off on their
// own, as a lead from the sharer; a lead they set aside comes back.
func TestShareQuestAnywhere(t *testing.T) {
	f := newFixture(t)
	f.splitBram(t)
	q := f.sideQuest(t, ObjectiveSpec{Kind: ObjFreeform, Text: "Find the den"})
	if KnowsQuest(f.w, q, f.bram) {
		t.Fatal("Bram was away")
	}
	if c := ShareCandidates(f.w, q, f.aria); len(c) != 1 || c[0] != f.bram {
		t.Fatalf("candidates %v", c)
	}
	_, shared, err := ShareQuest(f.w, q.ID, f.aria, []string{f.bram.ID})
	must(t, err)
	if len(shared) != 1 || !KnowsQuest(f.w, q, f.bram) || q.SharedBy[f.bram.ID] != "Aria" {
		t.Fatalf("shared %v known %v by %v", shared, q.Known, q.SharedBy)
	}
	_, _, err = TakeOn(f.w, q.ID, f.bram)
	must(t, err)
	if _, _, err := ShareQuest(f.w, q.ID, f.aria, nil); err == nil {
		t.Fatal("everyone is on it: nobody to share with")
	}
	_, err = DropQuest(f.w, q.ID, f.bram)
	must(t, err)
	must(t, SetQuestAside(f.w, q.ID, f.bram))
	if _, _, err := ShareQuest(f.w, q.ID, f.aria, []string{f.bram.ID}); err != nil || containsStr(q.SetAside, f.bram.ID) {
		t.Fatalf("re-share should bring it back: %v aside %v", err, q.SetAside)
	}
	q.Personal = true
	if _, _, err := ShareQuest(f.w, q.ID, f.aria, nil); err == nil {
		t.Fatal("personal quests cannot be shared")
	}
}
