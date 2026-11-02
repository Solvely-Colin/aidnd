package gm

import (
	"testing"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/llm"
)

func TestPlayerRollsTheirOwnD20(t *testing.T) {
	r := fightRig(t, 3, game.EnemySpec{Name: "Marsh Ghoul", HP: 30, AC: 5, AttackBonus: 2, Damage: "1d4"})
	r.g.Cfg.PlayerDice = true
	r.f.On("narrate_enemies", llm.Text("The ghoul claws back."))
	res := r.act(t, "u1", "I attack the ghoul", llm.Call("attack", map[string]any{"target": "Marsh Ghoul"}))
	if res.Pending == nil || len(res.Pending.Preview) != 1 || len(res.Pending.Preview[0].Rolls) != 1 || res.Narration != "" {
		t.Fatalf("attack should wait for the roll: %+v", res)
	}
	pr := res.Pending.Preview[0].Rolls[0]
	if pr.Kind != "attack" || pr.Target == 0 || pr.Against != "AC" {
		t.Fatalf("preview roll %+v", pr)
	}
	w := r.world(t)
	if w.Campaign.Pending[w.Characters[0].ID] == nil || w.Encounter.Enemies[0].HP != w.Encounter.Enemies[0].MaxHP {
		t.Fatal("nothing should be resolved before the roll")
	}
	if _, err := r.g.TakeAction(bg, r.id, "u1", "something else", Hints{}); err == nil {
		t.Fatal("a second message while a roll waits should be refused")
	}
	// taking it back clears it
	must(t, r.g.CancelPending(bg, r.id, "u1"))
	if w := r.world(t); len(w.Campaign.Pending) != 0 {
		t.Fatal("cancel")
	}
	res = r.act(t, "u1", "I attack the ghoul", llm.Call("attack", map[string]any{"target": "Marsh Ghoul"}))
	if res.Pending == nil {
		t.Fatal("pending again")
	}
	res, err := r.g.Roll(bg, r.id, "u1")
	must(t, err)
	if res.Resolution == nil || len(res.Resolution.Rolls()) == 0 || !res.TurnEnded || res.Narration == "" {
		t.Fatalf("roll should resolve the turn: %+v", res)
	}
	// a look needs no roll and resolves at once
	r.g.Cfg.PlayerDice = true
	w = r.world(t)
	if w.ActiveCharacter().PlayerID == "u2" {
		res = r.act(t, "u2", "what's here?", llm.Call("look", map[string]any{}))
		if res.Pending != nil {
			t.Fatal("a look should not wait for a roll")
		}
	}
	// auto-roll players never wait
	must(t, r.g.SetAutoRoll(bg, r.id, "u2", true))
	if w := r.world(t); !w.Player("u2").AutoRoll {
		t.Fatal("auto roll not saved")
	}
}

func TestDeathSaveWaitsForThePlayer(t *testing.T) {
	r := fightRig(t, 5, game.EnemySpec{Name: "Marsh Ghoul", HP: 30, AC: 5, AttackBonus: 2, Damage: "1d4"})
	r.g.Cfg.PlayerDice = true
	r.f.Default("narrate_enemies", llm.Text("The ghoul looms."))
	w := r.world(t)
	game.KnockOut(w, w.Characters[1]) // u2 lies dying
	_, err := r.g.Store.Save(bg, w)
	must(t, err)
	must(t, r.g.SetAutoRoll(bg, r.id, "u1", true))
	res := r.act(t, "u1", "I wait", llm.Call("end_turn", map[string]any{}))
	if res.NextActor == nil || res.NextActor.PlayerID != "u2" {
		t.Fatalf("u2 should be up for the death save: %+v", res.NextActor)
	}
	w = r.world(t)
	if p := w.Campaign.Pending[w.Characters[1].ID]; p == nil || p.Kind != game.PendingDeathSave {
		t.Fatal("death save should wait for its player")
	}
	if _, err := r.g.TakeAction(bg, r.id, "u2", "I crawl", Hints{}); err == nil {
		t.Fatal("a dying character can only roll")
	}
	if err := r.g.CancelPending(bg, r.id, "u2"); err == nil {
		t.Fatal("a death save can't be taken back")
	}
	res, err = r.g.Roll(bg, r.id, "u2")
	must(t, err)
	if len(res.DeathSaves) != 1 {
		t.Fatalf("death save not rolled: %+v", res)
	}
	if res.DeathSaves[0].Revived {
		if res.NextActor == nil || res.NextActor.PlayerID != "u2" {
			t.Fatal("a natural 20 should give the turn back")
		}
	} else if res.NextActor == nil || res.NextActor.PlayerID != "u1" {
		t.Fatalf("the turn should pass: %+v", res.NextActor)
	}
}

// A character healed from the DM console while their death save waits
// must not stay stuck being asked to roll it.
func TestHealedFromTheConsoleSkipsTheWaitingDeathSave(t *testing.T) {
	r := fightRig(t, 5, game.EnemySpec{Name: "Marsh Ghoul", HP: 30, AC: 5, AttackBonus: 2, Damage: "1d4"})
	r.g.Cfg.PlayerDice = true
	r.f.Default("narrate_enemies", llm.Text("The ghoul looms."))
	w := r.world(t)
	game.KnockOut(w, w.Characters[1])
	_, err := r.g.Store.Save(bg, w)
	must(t, err)
	must(t, r.g.SetAutoRoll(bg, r.id, "u1", true))
	r.act(t, "u1", "I wait", llm.Call("end_turn", map[string]any{}))
	w = r.world(t)
	if p := w.Campaign.Pending[w.Characters[1].ID]; p == nil || p.Kind != game.PendingDeathSave {
		t.Fatal("death save should wait for its player")
	}
	_, _, err = r.g.DM(bg, r.id, "u1", DMAction{Kind: "heal_party"})
	must(t, err)
	w = r.world(t)
	if w.Campaign.Pending[w.Characters[1].ID] != nil {
		t.Fatal("the healed character still owes a death save")
	}
	r.f.On("interpret", llm.Calls(llm.Call("end_turn", map[string]any{})))
	if _, err := r.g.TakeAction(bg, r.id, "u2", "I get up", Hints{}); err != nil {
		t.Fatalf("the healed character should act on their turn: %v", err)
	}
}
