package gm

import (
	"encoding/json"
	"testing"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/llm"
)

// Casebook moves: take a lead on, follow it, give it up; each is news in
// the feed for the characters it concerns.
func TestQuestActTakeTrackDrop(t *testing.T) {
	r := newRig(t, 2, 1)
	r.start(t)
	w := r.world(t)
	q, err := game.AddQuest(w, game.QuestSpec{Title: "Lights on the Reef", Source: game.SourceOverheard, Lead: "Fishers at the bar",
		Objectives: []game.ObjectiveSpec{{Kind: game.ObjFreeform, Text: "Watch the reef"}}}, false)
	must(t, err)
	_, err = r.g.Store.Save(bg, w)
	must(t, err)
	w, err = r.g.QuestAct(bg, r.id, "u1", q.ID, QuestOpTake)
	must(t, err)
	got := w.Quest(q.ID)
	if got.Status != game.QuestActive || len(got.Takers) != 2 {
		t.Fatalf("take: %s takers %v", got.Status, got.Takers)
	}
	if _, err := r.g.QuestAct(bg, r.id, "u2", q.ID, QuestOpDrop); err != nil {
		t.Fatal(err)
	}
	w = r.world(t)
	if game.OnQuest(w.Quest(q.ID), w.CharacterOf("u2")) || !game.OnQuest(w.Quest(q.ID), w.CharacterOf("u1")) {
		t.Fatalf("after u2 drops: %v", w.Quest(q.ID).Takers)
	}
	evs := r.events(t, "quest_updated")
	if len(evs) < 2 {
		t.Fatalf("events %d", len(evs))
	}
	var last game.QuestUpdate
	must(t, json.Unmarshal(evs[len(evs)-1].Data, &last))
	if last.Kind != game.QuestDropped || len(last.Characters) == 0 {
		t.Fatalf("drop news %+v", last)
	}
	if _, err := r.g.QuestAct(bg, r.id, "u1", q.ID, "explode"); err == nil {
		t.Fatal("unknown move accepted")
	}
}

// The narrator can turn up a quest nobody offered; it becomes a lead for
// the group, announced in the feed.
func TestDiscoverQuestDirective(t *testing.T) {
	r := newRig(t, 1, 1)
	r.start(t)
	r.f.Default("narrate", llm.Calls(llm.Call(DirDiscoverQuest, map[string]any{
		"title": "The Drowned Bell", "source": "overheard", "lead": "Two fishers whispering at the pier",
		"objectives": []map[string]any{{"kind": "freeform", "text": "Find who rings it"}}})))
	r.act(t, "u1", "I listen to the fishers", llm.Call("look", map[string]any{}), llm.Call("end_turn", map[string]any{}))
	w := r.world(t)
	q := w.Quest("The Drowned Bell")
	if q == nil {
		t.Fatal("no quest discovered")
	}
	if q.Status != game.QuestOffered || q.GiverID != "" || q.Source != game.SourceOverheard || !game.KnowsQuest(w, q, w.CharacterOf("u1")) {
		t.Fatalf("quest %+v %+v", q, q.QuestExtra)
	}
	found := false
	for _, e := range r.events(t, "quest_updated") {
		var u game.QuestUpdate
		_ = json.Unmarshal(e.Data, &u)
		found = found || u.Kind == game.QuestLead && u.QuestID == q.ID
	}
	if !found {
		t.Fatal("no lead news")
	}
}
