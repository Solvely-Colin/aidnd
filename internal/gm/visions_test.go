package gm

import (
	"encoding/json"
	"io"
	"log"
	"strings"
	"testing"

	"github.com/Solvely-Colin/aidnd/internal/dice"
	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/llm"
)

// phantomWorld is canonWorld where Bram once saw a drowned woman.
func phantomWorld() *game.World {
	w := canonWorld()
	w.Campaign.Turn = 5
	w.Characters[0].Visions = []game.Vision{
		{Turn: 4, Kind: game.VisionPhantom, Seed: "a drowned woman, hair plastered to her face, standing quite still", Phantom: "drowned woman"},
		{Turn: 2, Kind: game.VisionHostile, Real: true, Seed: "for an instant Ada Brennan looks at you with open hatred"},
	}
	return w
}

func TestCanonCutsPhantoms(t *testing.T) {
	w := phantomWorld()
	for _, life := range []bool{false, true} {
		c := newCanon(w, "", nil)
		c.lifeOnly = life // summaries, lore and NPC notes check them too
		for _, s := range []string{
			"A drowned woman stands by the door, dripping.",
			"The woman by the stairs is drowned and pale.",
		} {
			if is := c.check(s); len(is) != 1 || !strings.Contains(is[0].Why, "drowned woman") {
				t.Errorf("lifeOnly=%v %q: %+v", life, s, is)
			}
		}
		for _, s := range []string{
			"You look where Bram points: there is no drowned woman, only a wet coat on a hook.",
			"Bram swears he saw a drowned woman by the door.",
			"A woman in a shawl sells eels by the steps.",
		} {
			if is := c.check(s); len(is) != 0 {
				t.Errorf("lifeOnly=%v %q flagged: %+v", life, s, is)
			}
		}
	}
	// a phantom long faded is ordinary words again
	w.Campaign.Turn = 4 + game.VisionMemory + 1
	if is := newCanon(w, "", nil).check("A drowned woman stands by the door."); len(is) != 0 {
		t.Fatalf("faded phantom still guarded: %+v", is)
	}
}

func TestUnrealBlockListsOnlyFalseVisions(t *testing.T) {
	b := unrealBlock(phantomWorld())
	if !strings.Contains(b, "UNREAL") || !strings.Contains(b, "Bram: a drowned woman") {
		t.Fatalf("block %q", b)
	}
	if strings.Contains(b, "hatred") {
		t.Fatalf("a true vision was listed as unreal: %q", b)
	}
	if unrealBlock(canonWorld()) != "" {
		t.Fatal("block without visions")
	}
}

func TestNarratorCantSpawnAPhantom(t *testing.T) {
	w := phantomWorld()
	w.Locations[0].Danger = 2
	g := New(llm.NewFake(), nil, Config{}, dice.New(1))
	g.Log = log.New(io.Discard, "", 0)
	_, err := g.applyDirective(t.Context(), w, llm.Call(DirSpawnEncounter, map[string]any{"enemies": []map[string]any{{"name": "Drowned Woman", "hp": 8, "ac": 11, "attack_bonus": 2, "damage": "1d6"}}}))
	if err == nil || !strings.Contains(err.Error(), "imagined") {
		t.Fatalf("spawned a phantom: %v", err)
	}
	if w.InCombat() {
		t.Fatal("a fight started")
	}
}

func TestVisionEventIsPrivateAndHidesTheTruth(t *testing.T) {
	f := llm.NewFake()
	g := New(f, nil, Config{}, dice.New(5))
	g.Log = log.New(io.Discard, "", 0)
	w := phantomWorld()
	w.Campaign.Dread = true
	w.Characters[0].PlayerID = "p1"
	w.Characters[0].Strain = 6
	w.Characters[0].Visions = nil
	w.Campaign.Turns.Order = []string{"c1"}
	f.Default("vision", llm.Text("Something moves at the edge of the lamplight, then is still."))
	for i := 0; i < 200 && len(w.Characters[0].Visions) == 0; i++ {
		w.Campaign.Turn++
		evs := g.vision(t.Context(), w)
		if len(evs) == 0 {
			continue
		}
		if evs[0].Type != EventVision {
			t.Fatalf("type %q", evs[0].Type)
		}
		var d map[string]any
		_ = json.Unmarshal(evs[0].Data, &d)
		if d["for"] != "p1" || d["text"] == "" {
			t.Fatalf("data %v", d)
		}
		if _, leaked := d["real"]; leaked || strings.Contains(string(evs[0].Data), "kind") {
			t.Fatalf("the event gives away what it is: %s", evs[0].Data)
		}
	}
	if len(w.Characters[0].Visions) != 1 || w.Characters[0].Visions[0].Text == "" {
		t.Fatalf("vision not recorded: %+v", w.Characters[0].Visions)
	}
}

func TestVisionTextNeverAdmitsItIsFalse(t *testing.T) {
	f := llm.NewFake()
	g := New(f, nil, Config{}, dice.New(5))
	g.Log = log.New(io.Discard, "", 0)
	w := phantomWorld()
	v := &w.Characters[0].Visions[0]
	f.Default("vision", llm.Text("You see a drowned woman, but it is only a hallucination."))
	if got := g.visionText(t.Context(), w, w.Characters[0], v); strings.Contains(got, "hallucination") || !strings.Contains(got, "drowned woman") {
		t.Fatalf("text %q", got)
	}
	// a true vision that invents loot falls back to the seed
	real := &w.Characters[0].Visions[1]
	f.Default("vision", llm.Text("Ada glares at you and you find a heavy iron key under the bar."))
	if got := g.visionText(t.Context(), w, w.Characters[0], real); strings.Contains(got, "key") {
		t.Fatalf("text %q", got)
	}
}
