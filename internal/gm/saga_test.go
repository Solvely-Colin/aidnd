package gm

import (
	"io"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/Solvely-Colin/aidnd/internal/dice"
	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/llm"
	"github.com/Solvely-Colin/aidnd/internal/store"
)

func newSagaRig(t *testing.T) *rig {
	t.Helper()
	st, err := store.Open(":memory:")
	must(t, err)
	t.Cleanup(func() { st.Close() })
	f := llm.NewFake()
	r := &rig{f: f, now: time.Unix(1_800_000_000, 0), id: "saga1"}
	r.g = New(f, st, Config{}, dice.New(3))
	r.g.Log = log.New(io.Discard, "", 0)
	r.g.Now = func() time.Time { return r.now }
	r.g.NewID = func() string { return r.id }
	r.g.Publish = func(string, []store.Event) {}
	_, err = r.g.CreateSaga(bg, "long-dark")
	must(t, err)
	r.pids = []string{"u1"}
	_, _, err = r.g.Join(bg, r.id, "u1", "Pu1")
	must(t, err)
	_, err = r.g.SaveCharacter(bg, r.id, "u1", specs[3])
	must(t, err)
	return r
}

func TestSagaActOnePlays(t *testing.T) {
	r := newSagaRig(t)
	w := r.start(t)
	if !w.Campaign.Dread || w.Here().Name != "Gilman House" {
		t.Fatalf("start: dread=%v at %s", w.Campaign.Dread, w.Here().Name)
	}
	if z := w.NPC("Zadok Allen"); z == nil || !z.Protected {
		t.Fatal("protected flag must survive a save and load")
	}
	if n := len(r.f.Tagged("author_campaign")); n != 0 {
		t.Fatalf("a saga must not author a campaign, got %d calls", n)
	}
	r.act(t, "u1", "to the harbor", llm.Call("move", map[string]any{"exit": "Innsmouth Harbor"}))
	w = r.world(t)
	if w.Here().Name != "Innsmouth Harbor" {
		t.Fatalf("at %s", w.Here().Name)
	}
	// the harbor's hand-written description is canon: no fill_location call
	if n := len(r.f.Tagged("author_location")); n != 0 {
		t.Fatalf("anchor places keep their canon description, got %d author calls", n)
	}
	var beat bool
	for _, b := range w.Beats {
		beat = beat || (b.Title == "The Mary Alder" && b.Fired)
	}
	if !beat {
		t.Fatal("the Mary Alder beat should play on arrival")
	}
	var logID string
	for _, it := range w.ItemsOf(game.OwnerLocation, w.Here().ID) {
		if strings.Contains(it.Name, "Logbook") {
			logID = it.ID
		}
	}
	r.act(t, "u1", "take the logbook", llm.Call("pick_up", map[string]any{"item": logID}))
	w = r.world(t)
	if !w.MainQuest().Objectives[0].Done {
		t.Fatalf("logbook should complete objective 1: %+v", w.MainQuest().Objectives[0])
	}
	// the narrator still doesn't know the 1846 bargain
	for _, l := range w.Lore {
		if l.Title == "The bargain of 1846" && game.LoreOpen(w, l) {
			t.Fatal("bargain opened too early")
		}
	}
	if len(r.f.Tagged("narrate")) == 0 {
		t.Fatal("expected narration")
	}
	for _, req := range r.f.Tagged("narrate") {
		for _, m := range req.Messages {
			if strings.Contains(m.Content, "Y'ha-nthlei") || strings.Contains(m.Content, "bargain with the Deep Ones") {
				t.Fatal("a gated secret reached the narrator")
			}
		}
	}
}
