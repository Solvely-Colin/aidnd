package gm

import (
	"strings"
	"testing"
	"time"

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
