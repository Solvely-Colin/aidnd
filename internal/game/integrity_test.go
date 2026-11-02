package game

import (
	"strings"
	"testing"
)

func integrityWorld() *World {
	w := &World{Campaign: Campaign{LocationID: "loc1", Turn: 5}}
	w.Locations = []*Location{{ID: "loc1", Name: "Harbor", Visited: true}, {ID: "loc2", Name: "Chapel"}}
	w.NPCs = []*NPC{
		{ID: "npc1", Name: "Ada", LocationID: "loc1", Alive: true},
		{ID: "npc2", Name: "Vane", LocationID: "loc1", Alive: true},
	}
	w.Characters = []*Character{{ID: "c1", Name: "Bram", HP: 10, MaxHP: 10}}
	w.Items = []*Item{
		{ID: "item1", Name: "Ledger", Owner: Owner{Kind: OwnerNPC, ID: "npc1"}},
		{ID: "item2", Name: "Silver Ring", Owner: Owner{Kind: OwnerNPC, ID: "npc1"}},
	}
	return w
}

func TestRepairStoryAroundTheDead(t *testing.T) {
	w := integrityWorld()
	side := &Quest{ID: "q1", Title: "The Ledger", GiverID: "npc1", Status: QuestActive, RewardGold: 30, RewardItems: []string{"item2"},
		Objectives: []Objective{{ID: "o1", Kind: ObjTalkToNPC, Target: "npc2", Text: "Ask Vane"}, {ID: "o2", Kind: ObjDeliverItem, Target: "item1", DeliverTo: "npc2", Text: "Bring Vane the ledger"}}}
	offered := &Quest{ID: "q2", Title: "Errand", GiverID: "npc1", Status: QuestOffered}
	main := &Quest{ID: "q3", Title: "Storyline", IsMain: true, Status: QuestActive, Objectives: []Objective{{ID: "m1", Kind: ObjTalkToNPC, Target: "Ada", Text: "Hear Ada out"}}}
	w.Quests = []*Quest{side, offered, main}

	// the party kills Ada (giver, holder of the ledger and the reward ring)
	w.NPCs[0].Alive = false
	RepairStory(w)
	for _, it := range w.Items {
		if it.Owner.Kind != OwnerLocation || it.Owner.ID != "loc1" {
			t.Fatalf("%s did not fall where Ada died: %+v", it.Name, it.Owner)
		}
	}
	if offered.Status != QuestFailed {
		t.Fatalf("offer from the dead still open: %s", offered.Status)
	}
	if o := main.Objectives[0]; o.Kind != ObjReachLocation || o.Target != "loc1" || !strings.Contains(o.Text, "Ada") {
		t.Fatalf("talk to the dead not rerouted: %+v", o)
	}
	// the storyline can now finish: Ada died at a visited place
	EvaluateQuests(w)
	if !main.Objectives[0].Done {
		t.Fatal("rerouted objective still unfinishable")
	}

	// later Vane dies too: the side quest's talk and delivery reroute
	w.NPCs[1].Alive = false
	RepairStory(w)
	if o := side.Objectives[1]; o.Kind != ObjObtainItem || o.DeliverTo != "" {
		t.Fatalf("delivery to the dead not rerouted: %+v", o)
	}
	// finishing it pays out at once: the giver is dead
	w.Items[0].Owner = Owner{Kind: OwnerCharacter, ID: "c1"}
	w.Campaign.LocationID = "loc1"
	EvaluateQuests(w)
	if side.Status != QuestTurnedIn || w.Characters[0].Gold != 30 {
		t.Fatalf("dead giver's quest did not pay out: %s, gold %d", side.Status, w.Characters[0].Gold)
	}
	if w.Items[1].Owner.Kind != OwnerCharacter {
		t.Fatal("reward ring left on the ground")
	}
	// idempotent
	if ch := RepairStory(w); len(ch) != 0 {
		t.Fatalf("second pass changed things: %v", ch)
	}
}

func TestRepairStoryPaysACompletedQuest(t *testing.T) {
	w := integrityWorld()
	q := &Quest{ID: "q1", Title: "Done", GiverID: "npc1", Status: QuestCompleted, RewardGold: 10, Objectives: []Objective{{ID: "o1", Done: true}}}
	w.Quests = []*Quest{q}
	w.NPCs[0].Alive = false
	RepairStory(w)
	if q.Status != QuestTurnedIn || w.Characters[0].Gold != 10 {
		t.Fatalf("stuck at %s with gold %d", q.Status, w.Characters[0].Gold)
	}
}
