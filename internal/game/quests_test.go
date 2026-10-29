package game

import (
	"strings"
	"testing"
)

func (f *fixture) sideQuest(t *testing.T, objs ...ObjectiveSpec) *Quest {
	t.Helper()
	q, err := AddQuest(f.w, QuestSpec{Title: "Wolves at the Gate", Summary: "Thin the pack.", GiverID: "Elder Maren",
		Objectives: objs, RewardGold: 40, RewardXP: 50, RewardItems: []RewardItemSpec{{Key: "healing_potion"}}}, false)
	must(t, err)
	return q
}

func TestSideQuestLifecycle(t *testing.T) {
	f := newFixture(t)
	w := f.w
	q := f.sideQuest(t,
		ObjectiveSpec{Kind: ObjReachLocation, Text: "Go to Whisperwood", Target: "Whisperwood"},
		ObjectiveSpec{Kind: ObjDefeatEnemy, Text: "Kill the wolf", Target: "wolf"},
	)
	if q.GiverID != f.elder.ID || q.Objectives[0].Target != f.wood.ID {
		t.Fatalf("targets not resolved to ids: %+v", q)
	}
	reward := w.Item(q.RewardItems[0])
	if reward.Owner != (Owner{Kind: OwnerNPC, ID: f.elder.ID}) {
		t.Fatal("reward not held by giver")
	}
	// reward items are not for sale and not obtainable before turn-in
	r := Resolve(f.ctx(), f.aria, []Action{{Kind: ActAcceptQuest, Target: q.ID}})
	if q.Status != QuestActive || !r.AllFree {
		t.Fatalf("accept: %s %s", q.Status, stepErr(r, 0))
	}
	// walk to the wood: objective 1 ticks automatically
	f.w.Campaign.Turns.Budget = FullBudget(false)
	r = Resolve(f.ctx(), f.aria, []Action{{Kind: ActMove, Target: "Whisperwood"}})
	if !q.Objectives[0].Done || len(r.Quests) == 0 {
		t.Fatalf("reach objective not ticked: %v", r.Changes)
	}
	// the quest's wolf waits there; kill it
	enc := w.Encounter
	if enc == nil || len(enc.Enemies) != 1 || !NameContains(enc.Enemies[0].Name, "wolf") {
		t.Fatalf("quest foe should be waiting in Whisperwood: %+v", enc)
	}
	enc.Enemies[0].HP, enc.Enemies[0].AC = 2, 5
	enc.Enemies[0].ZoneID = f.aria.ZoneID
	w.Campaign.Turns.Budget = FullBudget(true)
	r = Resolve(f.ctx(10, 4), f.aria, []Action{{Kind: ActAttack, Target: "wolf"}})
	if !q.Objectives[1].Done || q.Status != QuestCompleted {
		t.Fatalf("defeat objective: %v status %s", r.Changes, q.Status)
	}
	if !hasChange(r, "return to Elder Maren") {
		t.Errorf("no turn-in hint: %v", r.Changes)
	}
	// turn-in away from the giver fails
	w.Campaign.Turns.Budget = FullBudget(false)
	r = Resolve(f.ctx(), f.aria, []Action{{Kind: ActTurnInQuest, Target: q.ID}})
	if !strings.Contains(stepErr(r, 0), "turn the quest in there") {
		t.Fatalf("turn-in away: %q", stepErr(r, 0))
	}
	f.goTo(f.hub, 0)
	gold, xp := f.aria.Gold, f.bram.XP
	r = Resolve(f.ctx(), f.aria, []Action{{Kind: ActTurnInQuest, Target: "Wolves at the Gate"}})
	if stepErr(r, 0) != "" || q.Status != QuestTurnedIn {
		t.Fatalf("turn-in: %q", stepErr(r, 0))
	}
	if f.aria.Gold != gold+40 || reward.Owner.ID != f.aria.ID || f.bram.XP != xp+50 {
		t.Fatalf("rewards: gold %d item owner %v xp %d", f.aria.Gold, reward.Owner, f.bram.XP)
	}
	if mem := f.elder.Recall(); len(mem) == 0 || !strings.Contains(mem[0], "Aria") {
		t.Fatalf("elder does not remember: %v", mem)
	}
	for _, c := range f.w.Chronicle {
		if c.QuestID == q.ID && (c.Kind != FactPromise || c.Status != PromiseKept) {
			t.Fatalf("quest promise not kept: %+v", c)
		}
	}
}

func TestAcceptQuestRequiresGiverPresent(t *testing.T) {
	f := newFixture(t)
	q := f.sideQuest(t, ObjectiveSpec{Kind: ObjFreeform, Text: "Bring word"})
	f.goTo(f.wood, 0)
	r := Resolve(f.ctx(), f.aria, []Action{{Kind: ActAcceptQuest, Target: q.ID}})
	if !strings.Contains(stepErr(r, 0), "find them") {
		t.Fatalf("accepted remotely: %q", stepErr(r, 0))
	}
}

func TestObtainDeliverTalkObjectives(t *testing.T) {
	f := newFixture(t)
	w := f.w
	pelt := CustomItem(w, "Silver Pelt", ItemQuest, "", 5, Owner{Kind: OwnerLocation, ID: f.hub.ID})
	q := f.sideQuest(t,
		ObjectiveSpec{Kind: ObjTalkToNPC, Text: "Talk to Dunna", Target: "Dunna"},
		ObjectiveSpec{Kind: ObjObtainItem, Text: "Find the pelt", Target: "Silver Pelt"},
		ObjectiveSpec{Kind: ObjDeliverItem, Text: "Give the pelt to Dunna", Target: "Silver Pelt", DeliverTo: "Dunna"},
	)
	_, err := AcceptQuest(w, q.ID)
	must(t, err)
	// talking before acceptance would not count; now it does
	w.Campaign.Turn = 1
	Resolve(f.ctx(), f.aria, []Action{{Kind: ActTalkTo, Target: "Dunna"}})
	w.Campaign.Turns.Budget = FullBudget(false)
	Resolve(f.ctx(), f.aria, []Action{{Kind: ActPickUp, Item: pelt.ID}})
	if !q.Objectives[0].Done || !q.Objectives[1].Done || q.Objectives[2].Done {
		t.Fatalf("objectives %+v", q.Objectives)
	}
	w.Campaign.Turns.Budget = FullBudget(false)
	Resolve(f.ctx(), f.aria, []Action{{Kind: ActGiveItem, Item: "pelt", Target: "Dunna"}})
	if !q.Objectives[2].Done || q.Status != QuestCompleted {
		t.Fatalf("deliver not ticked: %+v", q.Objectives[2])
	}
}

func TestFreeformOnlyByJudgeWithEvidence(t *testing.T) {
	f := newFixture(t)
	q := f.sideQuest(t, ObjectiveSpec{Kind: ObjFreeform, Text: "Bring word to the elder"},
		ObjectiveSpec{Kind: ObjTalkToNPC, Text: "Talk to Dunna", Target: "Dunna"})
	_, err := AcceptQuest(f.w, q.ID)
	must(t, err)
	EvaluateQuests(f.w)
	if q.Objectives[0].Done {
		t.Fatal("freeform ticked automatically")
	}
	if _, _, err := CompleteFreeform(f.w, q.ID, q.Objectives[0].ID, ""); err == nil {
		t.Fatal("no evidence accepted")
	}
	if _, _, err := CompleteFreeform(f.w, q.ID, q.Objectives[1].ID, "they talked"); err == nil {
		t.Fatal("judge ticked a non-freeform objective")
	}
	_, _, err = CompleteFreeform(f.w, q.ID, q.Objectives[0].ID, `Aria: "Elder, the wolves are back."`)
	must(t, err)
	if !q.Objectives[0].Done || q.Objectives[0].Evidence == "" {
		t.Fatal("freeform not ticked")
	}
}

func (f *fixture) mainQuest(t *testing.T) *Quest {
	t.Helper()
	q, err := AddQuest(f.w, QuestSpec{Title: "The Black Spire", Summary: "Stop Vex.", Objectives: []ObjectiveSpec{
		{Kind: ObjTalkToNPC, Text: "Hear the elder's warning", Target: "Elder Maren"},
		{Kind: ObjReachLocation, Text: "Scout Whisperwood", Target: "Whisperwood"},
		{Kind: ObjDefeatEnemy, Text: "Defeat Vex", Target: "Vex"},
	}}, true)
	must(t, err)
	return q
}

func TestMainQuestOrderedAndLairGate(t *testing.T) {
	f := newFixture(t)
	w := f.w
	q := f.mainQuest(t)
	if q.Status != QuestActive {
		t.Fatal("main quest not active from the start")
	}
	gate := w.EdgeBetween(f.wood.ID, f.lair.ID)
	// reaching the wood first does not tick objective 2 (ordered)
	f.goTo(f.wood, 0)
	EvaluateQuests(w)
	if q.Objectives[1].Done {
		t.Fatal("main objective ticked out of order")
	}
	f.goTo(f.hub, 0)
	f.wood.Visited = false // pretend the wood is still unseen
	Resolve(f.ctx(), f.aria, []Action{{Kind: ActTalkTo, Target: "Elder Maren"}})
	if !q.Objectives[0].Done || gate.State != EdgeLocked {
		t.Fatalf("obj1 %v gate %s", q.Objectives[0].Done, gate.State)
	}
	if f.aria.XP != XPMainObjective {
		t.Fatalf("main xp %d", f.aria.XP)
	}
	f.goTo(f.wood, 2)
	_, ch := EvaluateQuests(w)
	if !q.Objectives[1].Done || gate.State != EdgeKnown {
		t.Fatalf("gate not opened: %v", ch)
	}
	if !strings.Contains(strings.Join(ch, "|"), "Black Spire is now open") {
		t.Errorf("no gate change line: %v", ch)
	}
}

func TestPlaceReachedEarlyCountsWhenItsTurnComes(t *testing.T) {
	f := newFixture(t)
	q := f.mainQuest(t)
	f.goTo(f.wood, 0) // out of order: objective 1 is still open
	f.goTo(f.hub, 0)
	Resolve(f.ctx(), f.aria, []Action{{Kind: ActTalkTo, Target: "Elder Maren"}})
	if !q.Objectives[0].Done || !q.Objectives[1].Done {
		t.Fatalf("visited place should tick once its turn comes: %+v", q.Objectives)
	}
}

func TestAntagonistDefeatCompletesCampaign(t *testing.T) {
	f := newFixture(t)
	q := f.mainQuest(t)
	q.Objectives[0].Done, q.Objectives[1].Done = true, true
	UpdateLairGate(f.w)
	f.w.Campaign.Turns.Budget = FullBudget(false)
	Resolve(f.ctx(100), f.aria, []Action{{Kind: ActMove, Target: "Whisperwood"}})
	Resolve(f.ctx(), f.aria, []Action{{Kind: ActReposition, Zone: "Clearing"}})
	f.w.Campaign.Turns.Budget = FullBudget(false)
	r := Resolve(f.ctx(), f.aria, []Action{{Kind: ActMove, Target: "Black Spire"}})
	if stepErr(r, 0) != "" || f.w.Campaign.LocationID != f.lair.ID {
		t.Fatalf("into lair: %q", stepErr(r, 0))
	}
	f.villain.Stats = &StatBlock{MaxHP: 4, AC: 5, AttackBonus: 2, Damage: "1d6"}
	f.villain.ZoneID = f.aria.ZoneID
	// the antagonist fights as a boss; wear it down first
	boss := NPCJoinsFight(f.w, f.villain)
	if boss == nil || boss.Boss == nil {
		t.Fatal("antagonist should be a boss")
	}
	boss.HP = 1
	f.w.Campaign.Turns.Budget = FullBudget(true)
	r = Resolve(f.ctx(10, 6), f.aria, []Action{{Kind: ActAttack, Target: "Vex"}, {Kind: ActLook}})
	if f.w.Campaign.Status != StatusCompleted || !r.CampaignEnded || f.villain.Alive {
		t.Fatalf("campaign %s ended=%v %v %s", f.w.Campaign.Status, r.CampaignEnded, r.Changes, stepErr(r, 0))
	}
}

func TestOversizedRewardsRejected(t *testing.T) {
	f := newFixture(t)
	_, err := AddQuest(f.w, QuestSpec{Title: "Riches", GiverID: f.elder.ID,
		Objectives: []ObjectiveSpec{{Kind: ObjFreeform, Text: "x"}},
		RewardGold: 100000, RewardXP: 5000,
		RewardItems: []RewardItemSpec{{Key: "chain_mail"}, {Key: "longbow"}, {Key: "rapier"}}}, false)
	p, ok := err.(Problems)
	if !ok || len(p) < 3 {
		t.Fatalf("problems %v", err)
	}
	joined := strings.Join(p, "|")
	for _, want := range []string{"reward_gold", "reward_xp", "at most 2"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %v", want, p)
		}
	}
}

func TestQuestValidationRejectsUnknownIDs(t *testing.T) {
	f := newFixture(t)
	_, err := AddQuest(f.w, QuestSpec{Title: "Bad", GiverID: "Nobody",
		Objectives: []ObjectiveSpec{{Kind: ObjReachLocation, Text: "x", Target: "Atlantis"}, {Kind: "dance", Text: "y"}}}, false)
	joined := err.Error()
	for _, want := range []string{"unknown giver", "unknown location", "unknown kind"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %v", want, joined)
		}
	}
}

func TestQuestItemsArePlacedAndDeliverTargetFixed(t *testing.T) {
	f := newFixture(t)
	w := f.w
	q, err := AddQuest(w, QuestSpec{Title: "Lost Bell", Summary: "The elder lost a bell somewhere wild.", GiverID: f.elder.ID,
		Objectives: []ObjectiveSpec{
			{Kind: ObjObtainItem, Text: "Find the Silver Bell in the Hollow", Target: "Silver Bell"},
			{Kind: ObjDeliverItem, Text: "Return it to the elder", Target: f.elder.Name, DeliverTo: f.elder.ID},
		}}, false)
	must(t, err)
	if got := q.Objectives[1].Target; got != "Silver Bell" {
		t.Fatalf("deliver target = %q, want the fetched item", got)
	}
	it := w.FindItemOf(OwnerLocation, f.hollow.ID, "Silver Bell")
	if it == nil || it.Kind != ItemQuest {
		t.Fatalf("bell not placed in the Hollow (named in the objective): %+v", it)
	}
	// an item that already exists somewhere is not duplicated
	n := len(w.Items)
	_, err = AddQuest(w, QuestSpec{Title: "Again", Summary: "x", GiverID: f.elder.ID, Objectives: []ObjectiveSpec{{Kind: ObjObtainItem, Text: "Get the bell", Target: "Silver Bell"}}}, false)
	must(t, err)
	if len(w.Items) != n {
		t.Fatalf("existing quest item duplicated")
	}
}

func TestQuestItemHeldByNamedNPCHandedOverOnTalk(t *testing.T) {
	f := newFixture(t)
	w := f.w
	q, err := AddQuest(w, QuestSpec{Title: "Salt", Summary: "Wards.", GiverID: f.elder.ID,
		Objectives: []ObjectiveSpec{{Kind: ObjObtainItem, Text: "Get warding salt from Dunna", Target: "Warding Salt"}}}, false)
	must(t, err)
	if w.FindItemOf(OwnerNPC, f.smith.ID, "Warding Salt") == nil {
		t.Fatal("salt should be held by the NPC the objective names")
	}
	_, err = AcceptQuest(w, q.ID)
	must(t, err)
	w.Campaign.Turns.Budget = FullBudget(false)
	Resolve(f.ctx(), f.aria, []Action{{Kind: ActTalkTo, Target: "Dunna"}})
	if w.FindItemOf(OwnerCharacter, f.aria.ID, "Warding Salt") == nil || !q.Objectives[0].Done {
		t.Fatalf("salt not handed over on talk: %+v", q.Objectives)
	}
}

func TestQuestFoeWaitsAtItsPlace(t *testing.T) {
	f := newFixture(t)
	w := f.w
	q, err := AddQuest(w, QuestSpec{Title: "Wolves", Summary: "Something stalks Whisperwood.", GiverID: f.elder.ID,
		Objectives: []ObjectiveSpec{{Kind: ObjDefeatEnemy, Text: "Kill the Grey Alpha in Whisperwood", Target: "Grey Alpha"}}}, false)
	must(t, err)
	if q.Objectives[0].Where != f.wood.ID {
		t.Fatalf("foe placed at %q, want Whisperwood", q.Objectives[0].Where)
	}
	_, err = AcceptQuest(w, q.ID)
	must(t, err)
	w.Campaign.Turns.Budget = FullBudget(false)
	res := Resolve(f.ctx(), f.aria, []Action{{Kind: ActMove, Target: "Whisperwood"}})
	if !w.InCombat() || w.Encounter.Enemies[0].Name != "Grey Alpha" || !res.Steps[0].CombatStarted {
		t.Fatalf("quest foe did not appear: %+v", w.Encounter)
	}
	if b := w.Campaign.Turns.Budget; b.Free != CombatFreeActions {
		t.Fatalf("free actions not capped when the fight broke out: %+v", b)
	}
	for _, e := range w.Encounter.Enemies {
		e.HP = 0
	}
	w.Defeats = append(w.Defeats, DefeatRecord{Name: "Grey Alpha", Turn: w.Campaign.Turn})
	EvaluateQuests(w)
	if !q.Objectives[0].Done {
		t.Fatal("defeating the quest foe should tick the objective")
	}
}
