package gm

import (
	"strings"
	"testing"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/llm"
)

func TestNarratorTiesAndMarksFollowOneCharacter(t *testing.T) {
	r := newRig(t, 2, 1)
	w := r.start(t)
	corbin := w.NPC("Elder Corbin")
	a, b := w.Characters[0], w.Characters[1]
	a.ZoneID = corbin.ZoneID
	// a grudge for one character only
	if _, err := r.g.applyDirective(bg, w, llm.Call(DirNPCTie, map[string]any{"npc_id": corbin.ID, "character_id": a.ID, "kind": "grudge", "reason": "mocked his dead wife"})); err != nil {
		t.Fatal(err)
	}
	if len(corbin.TiesTo(a.ID)) != 1 || len(corbin.TiesTo(b.ID)) != 0 {
		t.Fatalf("ties %+v", corbin.Ties)
	}
	if _, err := r.g.applyDirective(bg, w, llm.Call(DirNPCTie, map[string]any{"npc_id": corbin.ID, "character_id": a.ID, "kind": "adores", "reason": "x"})); err == nil {
		t.Fatal("bad tie kind accepted")
	}
	// a mark: one a day from the narrator, never a loss
	if _, err := r.g.applyDirective(bg, w, llm.Call(DirMarkCharacter, map[string]any{"character_id": b.ID, "kind": "loss", "text": "lost a friend"})); err == nil {
		t.Fatal("narrator wrote a loss")
	}
	if _, err := r.g.applyDirective(bg, w, llm.Call(DirMarkCharacter, map[string]any{"character_id": b.ID, "kind": "brand", "text": "a salt-white sigil burned into the palm"})); err != nil {
		t.Fatal(err)
	}
	if _, err := r.g.applyDirective(bg, w, llm.Call(DirMarkCharacter, map[string]any{"character_id": b.ID, "kind": "scar", "text": "a torn ear"})); err == nil {
		t.Fatal("two narrator marks in a day")
	}
	if len(b.Marks) != 1 || b.Marks[0].Kind != game.MarkBrand {
		t.Fatalf("marks %+v", b.Marks)
	}
	// the narrator is shown both, as personal
	mem := worldMemory(w, a)
	if !strings.Contains(mem, "Elder Corbin holds a grudge against "+a.Name+" (mocked his dead wife") {
		t.Fatalf("grudge missing from memory:\n%s", mem)
	}
	if !strings.Contains(mem, b.Name+" (brand: a salt-white sigil") {
		t.Fatalf("mark missing from memory:\n%s", mem)
	}
	if s := snapshot(w, a, false); !strings.Contains(s, "toward "+a.Name+" ") {
		t.Fatalf("snapshot lacks the personal attitude:\n%s", s)
	}
	// settling clears it
	if _, err := r.g.applyDirective(bg, w, llm.Call(DirNPCTie, map[string]any{"npc_id": corbin.ID, "character_id": a.ID, "kind": "settle", "reason": "apologised at the grave"})); err != nil || len(corbin.Ties) != 0 {
		t.Fatalf("settle: %v %+v", err, corbin.Ties)
	}
}

func TestNarratorTieCannotKillTheLiving(t *testing.T) {
	r := newRig(t, 1, 1)
	w := r.start(t)
	corbin := w.NPC("Elder Corbin")
	var other *game.NPC
	for _, n := range w.NPCs {
		if n.ID != corbin.ID && n.Alive {
			other = n
			break
		}
	}
	if other == nil {
		t.Skip("rig has no second NPC")
	}
	_, err := r.g.applyDirective(bg, w, llm.Call(DirNPCTie, map[string]any{"npc_id": corbin.ID, "character_id": w.Characters[0].ID, "kind": "grudge", "reason": other.Name + " was killed by them"}))
	if err == nil || len(corbin.Ties) != 0 {
		t.Fatalf("a tie that contradicts the world was accepted: %v", err)
	}
}
