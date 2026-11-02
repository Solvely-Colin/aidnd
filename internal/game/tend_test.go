package game

import (
	"strings"
	"testing"
)

// The live bug: "Survival check to tend to his own wounds" came through as
// purpose other, succeeded, and healed nothing.
func TestTendWoundsFromOtherCheckHeals(t *testing.T) {
	f := newFixture(t)
	f.aria.HP = 4
	// d20 15 succeeds vs DC 10, hit die roll 5
	r := tendRun(f, f.ctx(15, 5), f.aria, []Action{{Kind: ActCheck, Purpose: "other", Skill: "survival", Ability: "WIS", DC: 15, Text: "tend to his own wounds"}})
	if !r.Steps[0].OK {
		t.Fatalf("step failed: %s", r.Steps[0].Error)
	}
	want := min(f.aria.MaxHP, 4+max(1, 5+f.aria.Mod(CON)))
	if f.aria.HP != want {
		t.Fatalf("hp = %d, want %d; changes %v", f.aria.HP, want, r.Steps[0].Changes)
	}
	if r.Steps[0].Rolls[0].Target != 10 {
		t.Fatalf("tend DC should be 10, got %d", r.Steps[0].Rolls[0].Target)
	}
	if !strings.Contains(strings.Join(r.Steps[0].Changes, ";"), "regains") {
		t.Fatalf("no heal change: %v", r.Steps[0].Changes)
	}
	// once per rest
	hp := f.aria.HP
	f.aria.HP = 2
	r = tendRun(f, f.ctx(15, 5), f.aria, []Action{{Kind: ActCheck, Purpose: "tend"}})
	if r.Steps[0].OK || f.aria.HP != 2 {
		t.Fatalf("second tend before a rest should be refused (ok %v, hp %d, was %d)", r.Steps[0].OK, f.aria.HP, hp)
	}
	ShortRest(f.aria)
	r = tendRun(f, f.ctx(15, 5), f.aria, []Action{{Kind: ActCheck, Purpose: "tend"}})
	if !r.Steps[0].OK || f.aria.HP <= 2 {
		t.Fatalf("tend after a short rest should work: %v hp %d", r.Steps[0].Error, f.aria.HP)
	}
}

func TestTendAllyAndFailure(t *testing.T) {
	f := newFixture(t)
	f.bram.HP = 3
	// a failed check heals nothing and can be retried
	r := tendRun(f, f.ctx(2), f.aria, []Action{{Kind: ActCheck, Purpose: "tend", Target: f.bram.ID}})
	if f.bram.HP != 3 || f.bram.Used[tendedKey] != 0 {
		t.Fatalf("failure healed or marked: hp %d, %v", f.bram.HP, r.Steps[0].Changes)
	}
	r = tendRun(f, f.ctx(18, 4), f.aria, []Action{{Kind: ActCheck, Purpose: "tend", Target: "Bram"}})
	if f.bram.HP <= 3 {
		t.Fatalf("ally not healed: %v %s", r.Steps[0].Changes, r.Steps[0].Error)
	}
	if f.aria.Used[tendedKey] != 0 {
		t.Fatal("the healer should not be marked")
	}
}

func TestTendRefusals(t *testing.T) {
	f := newFixture(t)
	r := tendRun(f, f.ctx(15, 5), f.aria, []Action{{Kind: ActCheck, Purpose: "tend"}})
	if r.Steps[0].OK {
		t.Fatal("nobody hurt: should be refused")
	}
	f.aria.HP = 3
	f.fight(t, 0, 2, wolf)
	r = tendRun(f, f.ctx(15, 5), f.aria, []Action{{Kind: ActCheck, Purpose: "tend"}})
	if r.Steps[0].OK || f.aria.HP != 3 {
		t.Fatal("no tending in a fight")
	}
}

func TestTendWords(t *testing.T) {
	for _, s := range []string{"tend to his own wounds", "bandage my arm", "patch myself up", "treat Bram's injuries", "first aid"} {
		if !tendWords(s) {
			t.Errorf("%q should be tending", s)
		}
	}
	for _, s := range []string{"search the room", "stabilize the dying man", "climb the wall", "bind the cultist", "recall lore about the healing cult"} {
		if tendWords(s) {
			t.Errorf("%q should not be tending", s)
		}
	}
}

func tendRun(f *fixture, ctx *Ctx, c *Character, as []Action) *Resolution {
	f.w.Campaign.Turns.Budget = FullBudget(f.w.InCombat())
	r := Resolve(ctx, c, as)
	if len(r.Steps) == 0 {
		r.Steps = r.Skipped
	}
	return r
}
