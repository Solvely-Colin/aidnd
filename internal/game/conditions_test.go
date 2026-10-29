package game

import (
	"strings"
	"testing"

	"github.com/Solvely-Colin/aidnd/internal/dice"
)

func TestEnemyHitInflictsConditionOnFailedSave(t *testing.T) {
	f := newFixture(t)
	e := f.fight(t, 0, 0, EnemySpec{Name: "Wolf", HP: 10, AC: 12, AttackBonus: 3, Damage: "1d6", Inflicts: "knocks prone"})
	if e.Inflicts != CondProne {
		t.Fatalf("inflicts not normalized: %q", e.Inflicts)
	}
	// target idx 0 (Aria), not the weakest, d20 18 hits, 3 damage, DEX save 2 fails
	r := dice.FromSource(dice.NewScripted(1, 2, 18, 3, 2))
	turns, _ := RunEnemyTurns(f.w, r)
	if len(turns) != 1 || turns[0].Inflicted != CondProne || !f.aria.Conditions.Has(CondProne) {
		t.Fatalf("not knocked prone: %+v conds %v", turns, f.aria.Conditions)
	}
	if s := turns[0].Save; s == nil || s.Kind != "save" || s.Target != InflictDC(3) {
		t.Fatalf("save roll %+v", s)
	}
	if !strings.Contains(turns[0].Text, "is prone") {
		t.Fatalf("text %q", turns[0].Text)
	}
}

func TestSavedConditionDoesNotApply(t *testing.T) {
	f := newFixture(t)
	f.fight(t, 0, 0, EnemySpec{Name: "Spider", HP: 10, AC: 12, AttackBonus: 3, Damage: "1d6", Inflicts: CondPoisoned})
	r := dice.FromSource(dice.NewScripted(1, 2, 18, 3, 19))
	turns, _ := RunEnemyTurns(f.w, r)
	if f.aria.Conditions.Has(CondPoisoned) || turns[0].Inflicted != "" || !strings.Contains(turns[0].Text, "resists") {
		t.Fatalf("poisoned despite the save: %+v", turns[0])
	}
}

func TestStunOnlyInDangerousPlaces(t *testing.T) {
	if k, note := NormalizeInflict("stun", 2); k != CondProne || note == "" {
		t.Fatalf("stun at danger 2: %q %q", k, note)
	}
	if k, _ := NormalizeInflict("stunning blow", 3); k != CondStunned {
		t.Fatalf("stun at danger 3: %q", k)
	}
	if k, note := NormalizeInflict("petrified", 5); k != "" || note == "" {
		t.Fatalf("unknown condition kept: %q", k)
	}
}

func TestAttackEdge(t *testing.T) {
	prone := Conditions{{Kind: CondProne}}
	poisoned := Conditions{{Kind: CondPoisoned, Rounds: 3}}
	if e, why := AttackEdge(nil, prone, 0); e != 1 || !strings.Contains(why, "prone") {
		t.Errorf("melee vs prone: %d %q", e, why)
	}
	if e, _ := AttackEdge(nil, prone, 2); e != -1 {
		t.Errorf("ranged vs prone: %d", e)
	}
	if e, _ := AttackEdge(poisoned, nil, 0); e != -1 {
		t.Errorf("poisoned attacker: %d", e)
	}
	if e, why := AttackEdge(poisoned, prone, 0); e != 0 || why != "" {
		t.Errorf("advantage and disadvantage should cancel: %d %q", e, why)
	}
}

func TestPlayerAttackRollsWithAdvantage(t *testing.T) {
	f := newFixture(t)
	e := f.fight(t, 0, 0, wolf)
	e.Conditions = Conditions{{Kind: CondRestrained, Rounds: 2}}
	// two d20s 3 and 17, keep 17; then damage
	r := Resolve(f.ctx(3, 17, 4), f.aria, []Action{{Kind: ActAttack, Target: e.ID}})
	roll := r.Steps[0].Rolls[0]
	if roll.Edge != dice.Advantage || roll.Natural != 17 || len(roll.Naturals) != 2 || !strings.Contains(roll.EdgeWhy, "restrained") {
		t.Fatalf("roll %+v", roll)
	}
	if !hasChange(r, "hits") {
		t.Fatalf("changes %v", r.Changes)
	}
}

func TestPoisonedChecksHaveDisadvantage(t *testing.T) {
	f := newFixture(t)
	f.aria.Conditions = Conditions{{Kind: CondPoisoned, Rounds: 3, SaveDC: 12, Save: CON}}
	r := Resolve(f.ctx(18, 3), f.aria, []Action{{Kind: ActCheck, Ability: "STR", DC: 10, Purpose: "other", Text: "lift the gate"}})
	roll := r.Steps[0].Rolls[0]
	if roll.Edge != dice.Disadvantage || roll.Natural != 3 || roll.Tier.Succeeded() {
		t.Fatalf("roll %+v", roll)
	}
}

func TestProneStandsUpWithTheMove(t *testing.T) {
	f := newFixture(t)
	f.aria.Conditions = Conditions{{Kind: CondProne}}
	b := FullBudget(true)
	tick := StartCharacterTurn(f.aria, &b)
	if f.aria.Conditions.Has(CondProne) || b.Move != 0 || b.Action != 1 || len(tick.Changes) != 1 || tick.LostTurn {
		t.Fatalf("stand up: conds %v budget %+v tick %+v", f.aria.Conditions, b, tick)
	}
}

func TestStunLosesTheTurnThenWearsOff(t *testing.T) {
	f := newFixture(t)
	f.aria.Conditions = Conditions{{Kind: CondStunned, Rounds: 1}}
	b := FullBudget(true)
	if tick := StartCharacterTurn(f.aria, &b); !tick.LostTurn || !b.Spent() {
		t.Fatalf("stunned turn not lost: %+v %+v", tick, b)
	}
	tick := EndCharacterTurn(f.ctx().R, f.aria)
	if f.aria.Conditions.Has(CondStunned) || len(tick.Changes) != 1 {
		t.Fatalf("stun did not wear off: %v %+v", f.aria.Conditions, tick)
	}
}

func TestSaveEndsPoison(t *testing.T) {
	f := newFixture(t)
	f.aria.Conditions = Conditions{{Kind: CondPoisoned, Rounds: 3, SaveDC: 12, Save: CON}}
	// poisoned saves at disadvantage: 15 and 14 keep 14, +2 CON +2 prof = 18
	tick := EndCharacterTurn(f.ctx(15, 14).R, f.aria)
	if f.aria.Conditions.Has(CondPoisoned) || len(tick.Rolls) != 1 || !strings.Contains(tick.Changes[0], "shakes off") {
		t.Fatalf("poison stayed: %v %+v", f.aria.Conditions, tick)
	}
	f.aria.Conditions = Conditions{{Kind: CondPoisoned, Rounds: 3, SaveDC: 12, Save: CON}}
	EndCharacterTurn(f.ctx(2, 2).R, f.aria)
	if c := f.aria.Conditions.Get(CondPoisoned); c == nil || c.Rounds != 2 {
		t.Fatalf("failed save should tick down: %+v", f.aria.Conditions)
	}
}

func TestRestrainedCantMoveUntilFree(t *testing.T) {
	f := newFixture(t)
	f.fight(t, 0, 3, wolf)
	f.aria.Conditions = Conditions{{Kind: CondRestrained, Rounds: 3, SaveDC: 13, Save: STR}}
	r := Resolve(f.ctx(), f.aria, []Action{{Kind: ActReposition, Zone: "Fallen Logs"}})
	if !strings.Contains(stepErr(r, 0), "restrained") || f.w.Campaign.Turns.Budget.Move != 1 {
		t.Fatalf("restrained moved: %q", stepErr(r, 0))
	}
	// STR +2, prof +2: 10 + 4 = 14 vs DC 13
	r = Resolve(f.ctx(10), f.aria, []Action{{Kind: ActCheck, Ability: "STR", DC: 5, Purpose: "escape"}})
	if f.aria.Conditions.Has(CondRestrained) || r.Steps[0].Rolls[0].Target != 13 || !hasChange(r, "breaks free") {
		t.Fatalf("escape: %v %+v", r.Changes, r.Steps[0].Rolls)
	}
}

func TestFrightenedWontApproach(t *testing.T) {
	f := newFixture(t)
	e := f.fight(t, 0, 2, wolf)
	f.aria.Conditions = Conditions{{Kind: CondFrightened, Rounds: 2, SaveDC: 12, Save: WIS, Source: "Wolf", SourceID: e.ID}}
	r := Resolve(f.ctx(), f.aria, []Action{{Kind: ActReposition, Zone: "Fallen Logs"}})
	if !strings.Contains(stepErr(r, 0), "frightened") {
		t.Fatalf("frightened approached: %q", stepErr(r, 0))
	}
}

func TestStunnedEnemyLosesItsTurn(t *testing.T) {
	f := newFixture(t)
	e := f.fight(t, 0, 0, wolf)
	e.Conditions = Conditions{{Kind: CondStunned, Rounds: 1}}
	turns, _ := RunEnemyTurns(f.w, f.ctx().R)
	if len(turns) != 1 || turns[0].Kind != "stunned" || f.aria.HP != f.aria.MaxHP || e.Conditions.Has(CondStunned) {
		t.Fatalf("stunned enemy: %+v conds %v", turns, e.Conditions)
	}
}

func TestFightEndClearsAllButPoison(t *testing.T) {
	f := newFixture(t)
	e := f.fight(t, 0, 0, EnemySpec{Name: "Rat", HP: 1, AC: 5, AttackBonus: 0, Damage: "1d2"})
	f.aria.Conditions = Conditions{{Kind: CondPoisoned, Rounds: 3}, {Kind: CondProne}}
	f.bram.Conditions = Conditions{{Kind: CondFrightened, Rounds: 2}}
	r := Resolve(f.ctx(15, 15, 2), f.aria, []Action{{Kind: ActAttack, Target: e.ID}}) // disadvantage: two d20s
	if !r.CombatEnded {
		t.Fatalf("fight not over: %v", r.Changes)
	}
	if f.aria.Conditions.String() != CondPoisoned || len(f.bram.Conditions) != 0 {
		t.Fatalf("after fight: aria %v bram %v", f.aria.Conditions, f.bram.Conditions)
	}
	f.w.Campaign.Turns.Budget = FullBudget(false)
	f.goTo(f.hub, 0)
	Resolve(f.ctx(), f.aria, []Action{{Kind: ActRest}})
	if len(f.aria.Conditions) != 0 {
		t.Fatalf("rest kept %v", f.aria.Conditions)
	}
}
