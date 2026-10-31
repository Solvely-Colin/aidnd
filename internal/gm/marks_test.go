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

func TestNarratorKeepsEachCharactersOwnStory(t *testing.T) {
	r := newRig(t, 2, 1)
	w := r.start(t)
	a, b := w.Characters[0], w.Characters[1]
	if _, err := r.g.applyDirective(bg, w, llm.Call(DirPersonalGoal, map[string]any{"character_id": a.ID, "action": "add", "text": "Learn who rang the bell the night her father drowned"})); err != nil {
		t.Fatal(err)
	}
	if _, err := r.g.applyDirective(bg, w, llm.Call(DirRecordChoice, map[string]any{"character_id": a.ID, "kind": "choice", "text": "Lied to the elder about the drowned man's ring"})); err != nil {
		t.Fatal(err)
	}
	if len(a.Goals) != 1 || len(a.Record) != 1 || len(b.Goals) != 0 || len(b.Record) != 0 {
		t.Fatalf("a %+v %+v b %+v %+v", a.Goals, a.Record, b.Goals, b.Record)
	}
	mem := worldMemory(w, a)
	if !strings.Contains(mem, "YOUR STORY ("+a.Name) || !strings.Contains(mem, "[goal1] Learn who rang") || !strings.Contains(mem, "CHOSE: day") {
		t.Fatalf("own story missing:\n%s", mem)
	}
	if strings.Contains(worldMemory(w, b), "YOUR STORY") {
		t.Fatal("another character's story leaked into b's narration")
	}
	xp := a.XP
	if _, err := r.g.applyDirective(bg, w, llm.Call(DirPersonalGoal, map[string]any{"character_id": a.ID, "action": "done", "goal_id": "goal1", "text": "the bell-founder confessed"})); err != nil {
		t.Fatal(err)
	}
	if a.Goals[0].Status != game.GoalDone || a.XP != xp+game.GoalXP {
		t.Fatalf("goal %+v xp %d", a.Goals[0], a.XP)
	}
	game.AddGoal(w, a, "Another thing", game.SourcePlayer)
	if _, err := r.g.applyDirective(bg, w, llm.Call(DirPersonalGoal, map[string]any{"character_id": a.ID, "action": "done", "goal_id": "goal2", "text": "too easy"})); err == nil {
		t.Fatal("two goals settled in a day")
	}
}
