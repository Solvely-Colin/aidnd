package gm

import (
	"strings"
	"testing"
	"time"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/llm"
)

func TestWorldTicksDailyAndWritesTheGazette(t *testing.T) {
	r := newSagaRig(t)
	r.start(t)
	r.f.Default("gazette", llm.Text("HEADLINE: Fog Hides Innsmouth Harbor\nHEADLINE: Fisherman Missing\nCLASSIFIED: Strong swimmers wanted. Ask at Gilman House.\nWEATHER: Raw and grey."))
	w := r.world(t)
	if w.Campaign.Living == nil || len(w.Campaign.Living.Factions) != 3 {
		t.Fatalf("Innsmouth's three factions should be in play: %+v", w.Campaign.Living)
	}
	// the first sweep starts the clock; nothing is due yet
	_, err := r.g.Sweep(bg)
	must(t, err)
	if w = r.world(t); w.Campaign.Living.LastTickDay == 0 || w.Campaign.Living.Week != 0 {
		t.Fatal("the clock should start without a tick")
	}
	tide := w.Campaign.Living.Tide
	// a night's downtime at the hotel
	if _, err := r.g.SetDowntime(bg, r.id, "u1", "nap"); err == nil {
		t.Fatal("unknown downtime accepted")
	}
	_, err = r.g.SetDowntime(bg, r.id, "u1", "work")
	must(t, err)
	gold := r.world(t).CharacterOf("u1").Gold
	r.now = r.now.Add(24 * time.Hour)
	_, err = r.g.Sweep(bg)
	must(t, err)
	w = r.world(t)
	lv := w.Campaign.Living
	if lv.Week != 1 || lv.Tide <= tide-1 || lv.Gazette == nil || lv.Gazette.Headlines[0] != "Fog Hides Innsmouth Harbor" || lv.Gazette.Classified == "" {
		t.Fatalf("after a day: week %d tide %d gazette %+v", lv.Week, lv.Tide, lv.Gazette)
	}
	if me := w.CharacterOf("u1"); me.Gold <= gold || me.Downtime != nil || len(r.events(t, "downtime_done")) != 1 {
		t.Fatalf("downtime work: gold %d -> %d", gold, me.Gold)
	}
	if evs := r.events(t, "world_tick"); len(evs) != 1 || strings.Contains(string(evs[0].Data), `"party"`) {
		t.Fatalf("one untagged world_tick event: %d", len(evs))
	}
	// the server slept for a month: at most a week of ticks catch up
	r.now = r.now.Add(30 * 24 * time.Hour)
	_, err = r.g.Sweep(bg)
	must(t, err)
	if w = r.world(t); w.Campaign.Living.Week != 1+MaxCatchUpTicks {
		t.Fatalf("catch-up: week %d", w.Campaign.Living.Week)
	}
	// the narrator knows the world's state
	r.act(t, "u1", "to the harbor", llm.Call("move", map[string]any{"exit": "Innsmouth Harbor"}))
	reqs := r.f.Tagged("narrate")
	if p := lastUser(reqs[len(reqs)-1]); !strings.Contains(p, "THE WORLD: week 8") {
		t.Fatalf("narrator not told the world's state:\n%s", p)
	}
}

func TestWhileYouWereAway(t *testing.T) {
	r := newSagaRig(t)
	r.start(t)
	// the paper may not kill the living
	r.f.Default("gazette", llm.Text("HEADLINE: Hannah Coyle Found Dead On The Wharf\nHEADLINE: Fog Rolls In Off The Reef\nWEATHER: Grey."))
	_, err := r.g.Sweep(bg)
	must(t, err)
	w := r.world(t)
	if n := len(w.Campaign.Living.Agendas); n != 6 {
		t.Fatalf("Innsmouth's people should have their own goals: %d", n)
	}
	if game.BuildDispatch(w, "u1") != nil {
		t.Fatal("nothing missed yet")
	}
	for i := 0; i < 4; i++ {
		r.now = r.now.Add(24 * time.Hour)
		_, err = r.g.Sweep(bg)
		must(t, err)
	}
	w = r.world(t)
	for _, h := range w.Campaign.Living.Gazette.Headlines {
		if strings.Contains(h, "Dead") {
			t.Fatalf("the canon guard should drop a false death: %q", h)
		}
	}
	d := game.BuildDispatch(w, "u1")
	if d == nil || d.Weeks != 4 || len(d.Lines) == 0 {
		t.Fatalf("a player away for four ticks gets a dispatch: %+v", d)
	}
	moved := 0
	for _, a := range w.Campaign.Living.Agendas {
		moved += a.Progress
	}
	if moved == 0 {
		t.Fatal("four weeks and nobody followed their own goal")
	}
	_, err = r.g.MarkSeen(bg, r.id, "u1")
	must(t, err)
	if game.BuildDispatch(r.world(t), "u1") != nil {
		t.Fatal("seen clears it")
	}
	// taking a turn also counts as catching up
	r.now = r.now.Add(24 * time.Hour)
	_, err = r.g.Sweep(bg)
	must(t, err)
	if game.BuildDispatch(r.world(t), "u1") == nil {
		t.Fatal("a new tick means a new dispatch")
	}
	r.act(t, "u1", "look around", llm.Call("look", map[string]any{}))
	if game.BuildDispatch(r.world(t), "u1") != nil {
		t.Fatal("acting catches the player up")
	}
}
