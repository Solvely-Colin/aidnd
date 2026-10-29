package game

import (
	"strings"
	"testing"

	"github.com/Solvely-Colin/aidnd/internal/dice"
)

func roller(faces ...int) *dice.Roller { return dice.FromSource(dice.NewScripted(faces...)) }

func TestDeathSavesToDeathAndStory(t *testing.T) {
	f := newFixture(t)
	f.fight(t, 0, 1, wolf)
	KnockOut(f.w, f.aria)
	if !f.aria.Dying() || !f.aria.Down() {
		t.Fatal("knocked out in a fight should be dying")
	}
	ds := DeathSave(f.w, roller(12), f.aria)
	if ds.Roll.Kind != "death_save" || f.aria.Death.Successes != 1 {
		t.Fatalf("success not counted: %+v %+v", ds, f.aria.Death)
	}
	DeathSave(f.w, roller(5), f.aria)
	ds = DeathSave(f.w, roller(1), f.aria) // a 1 is two failures: three in all
	if !ds.Died || !f.aria.Dead || f.aria.Dying() {
		t.Fatalf("three failures should kill: %+v", ds)
	}
	if Heal(f.aria, 10); f.aria.HP != 0 {
		t.Fatal("the dead were healed")
	}
	found := false
	for _, ce := range f.w.Chronicle {
		found = found || (ce.Kind == FactDeath && strings.Contains(ce.Text, "Aria"))
	}
	if !found {
		t.Fatal("death not in the chronicle")
	}

	// in a story campaign three failures leave them stable instead
	g := newFixture(t)
	g.w.Campaign.Lethality = LethalityStory
	g.fight(t, 0, 1, wolf)
	KnockOut(g.w, g.bram)
	DeathSave(g.w, roller(1), g.bram)
	if ds := DeathSave(g.w, roller(3), g.bram); ds.Died || !ds.Stable || g.bram.Dead || g.bram.Dying() {
		t.Fatalf("story lethality killed: %+v", ds)
	}
}

func TestNatural20AndHealingWake(t *testing.T) {
	f := newFixture(t)
	f.fight(t, 0, 1, wolf)
	KnockOut(f.w, f.aria)
	if ds := DeathSave(f.w, roller(20), f.aria); !ds.Revived || f.aria.HP != 1 || f.aria.Death != nil {
		t.Fatalf("natural 20: %+v hp %d", ds, f.aria.HP)
	}
	KnockOut(f.w, f.aria)
	if _, woke := Heal(f.aria, 3); !woke || f.aria.HP != 3 || f.aria.Death != nil {
		t.Fatal("healing should wake a dying character")
	}
	// out of a fight a knockout is simply stable
	g := newFixture(t)
	KnockOut(g.w, g.aria)
	if g.aria.Dying() || !g.aria.Down() {
		t.Fatal("outside a fight a knockout should be stable")
	}
}

func TestDyingStillGetTurnsInFights(t *testing.T) {
	f := newFixture(t)
	f.fight(t, 0, 1, wolf)
	KnockOut(f.w, f.aria) // aria acts first; her turn passes to bram
	if next := EndTurn(f.w, t0); next != f.bram {
		t.Fatalf("next %v", next)
	}
	if next := EndTurn(f.w, t0); next != f.aria {
		t.Fatalf("a dying character should still get a turn for the death save, got %v", next)
	}
	Stabilize(f.aria)
	if next := EndTurn(f.w, t0); next != f.bram {
		t.Fatal("bram")
	}
	if next := EndTurn(f.w, t0); next != f.bram {
		t.Fatal("a stable character should be skipped")
	}
}

func TestStabilizeWithMedicine(t *testing.T) {
	f := newFixture(t)
	f.fight(t, 0, 2, wolf)
	KnockOut(f.w, f.aria)
	// Bram WIS +2, no medicine skill but proficient in WIS: +4; 8 -> 12 vs 10
	r := Resolve(f.ctx(8), f.bram, []Action{{Kind: ActCheck, Purpose: "stabilize", Target: f.aria.ID}})
	if stepErr(r, 0) != "" || !hasChange(r, "stabilizes Aria") || f.aria.Dying() {
		t.Fatalf("not stabilized: %v %s", r.Changes, stepErr(r, 0))
	}
	if ro := r.Steps[0].Rolls[0]; !strings.Contains(ro.Label, "Medicine") || ro.Target != 10 {
		t.Fatalf("roll %+v", ro)
	}
}

func TestFightEndStabilizesTheDying(t *testing.T) {
	f := newFixture(t)
	e := f.fight(t, 0, 0, wolf)
	KnockOut(f.w, f.bram)
	e.HP = 1
	r := Resolve(f.ctx(18, 4), f.aria, []Action{{Kind: ActAttack, Target: e.ID}})
	if !r.CombatEnded || f.bram.Dying() || !f.bram.Down() || !hasChange(r, "Bram is stable") {
		t.Fatalf("fight end: %v", r.Changes)
	}
}

func TestSkillChecks(t *testing.T) {
	f := newFixture(t)
	// Bram the ranger knows Stealth: DEX +2, proficiency +2
	r := Resolve(f.ctx(10), f.bram, []Action{{Kind: ActCheck, Skill: "stealth", DC: 15, Text: "sneak past"}})
	ro := r.Steps[0].Rolls[0]
	if ro.Mod != 4 || len(ro.Parts) != 2 || ro.Parts[1].Label != "Proficiency" || !strings.Contains(ro.Label, "Stealth") {
		t.Fatalf("stealth roll %+v", ro)
	}
	// Aria knows no Arcana and isn't proficient in INT: INT -1 only
	f.w.Campaign.Turns.Budget = FullBudget(false)
	r = Resolve(f.ctx(10), f.aria, []Action{{Kind: ActCheck, Skill: "arcana", DC: 10, Text: "recall"}})
	if ro := r.Steps[0].Rolls[0]; ro.Mod != -1 || len(ro.Parts) != 1 {
		t.Fatalf("arcana roll %+v", ro)
	}
	if f.bram.PassivePerception() != 14 { // 10 + WIS 2 + perception 2
		t.Fatalf("passive %d", f.bram.PassivePerception())
	}
	if _, p := NormalizeSkills([]string{"Sleight of Hand", "stealth", "STEALTH", "juggling"}); len(p) != 1 {
		t.Fatalf("problems %v", p)
	}
}

func TestDodgeHelpAndHide(t *testing.T) {
	f := newFixture(t)
	e := f.fight(t, 1, 3, wolf) // party in Fallen Logs (cover), wolf in Thicket
	r := Resolve(f.ctx(), f.aria, []Action{{Kind: ActDodge}})
	if stepErr(r, 0) != "" || !f.aria.Conditions.Has(CondDodging) {
		t.Fatal("dodge")
	}
	if ed, _ := AttackEdge(e.Conditions, f.aria.Conditions, 0); ed != -1 {
		t.Fatal("dodging should give attackers disadvantage")
	}
	EndCharacterTurn(roller(10), f.aria)
	if !f.aria.Conditions.Has(CondDodging) {
		t.Fatal("dodge should last through enemy turns")
	}
	StartCharacterTurn(f.aria, &Budget{Move: 1, Action: 1})
	if f.aria.Conditions.Has(CondDodging) {
		t.Fatal("dodge should end at the start of the next turn")
	}

	f.w.Campaign.Turns.Budget = FullBudget(true)
	r = Resolve(f.ctx(), f.aria, []Action{{Kind: ActHelp, Target: f.bram.ID}})
	if stepErr(r, 0) != "" || !f.bram.Conditions.Has(CondHelped) {
		t.Fatalf("help: %s", stepErr(r, 0))
	}
	// Bram hides (in cover): Stealth +4, 9 -> 13 vs wolf passive 11
	f.w.Campaign.Turns.Budget = FullBudget(true)
	r = Resolve(f.ctx(9, 9), f.bram, []Action{{Kind: ActHide}})
	if stepErr(r, 0) != "" || !f.bram.Conditions.Has(CondHidden) {
		t.Fatalf("hide: %v %s", r.Changes, stepErr(r, 0))
	}
	if r.Steps[0].Rolls[0].Edge != dice.Advantage {
		t.Fatal("help should give advantage on the hide check")
	}
	// attacking from hiding has advantage and gives you away
	f.w.Campaign.Turns.Budget = FullBudget(true)
	r = Resolve(f.ctx(15, 3, 4), f.bram, []Action{{Kind: ActAttack, Target: e.ID}})
	if ro := r.Steps[0].Rolls[0]; ro.Edge != dice.Advantage || !strings.Contains(ro.EdgeWhy, "hiding") {
		t.Fatalf("hidden attack %+v", ro)
	}
	if f.bram.Conditions.Has(CondHidden) {
		t.Fatal("attacking should reveal")
	}

	// no hiding in the open with an enemy right there
	g := newFixture(t)
	g.fight(t, 0, 0, wolf)
	if r := Resolve(g.ctx(), g.bram, []Action{{Kind: ActHide}}); !strings.Contains(stepErr(r, 0), "nowhere to hide") {
		t.Fatalf("got %q", stepErr(r, 0))
	}
}

func TestHiddenCantBeTargeted(t *testing.T) {
	f := newFixture(t)
	f.fight(t, 1, 1, wolf)
	addStance(f.aria, CondHidden, "")
	addStance(f.bram, CondHidden, "")
	turns, _ := RunEnemyTurns(f.w, roller(10))
	if len(turns) != 1 || turns[0].Kind != "hold" {
		t.Fatalf("enemy should not find hidden targets: %+v", turns)
	}
}

func TestShoveAndGrapple(t *testing.T) {
	f := newFixture(t)
	e := f.fight(t, 1, 1, wolf)
	// DC 8 + STR 2 + prof 2 = 12; wolf saves at +1: 5 -> 6 fails
	r := Resolve(f.ctx(5), f.aria, []Action{{Kind: ActShove, Target: e.ID, Method: "prone"}})
	if stepErr(r, 0) != "" || !e.Conditions.Has(CondProne) || r.Steps[0].Tier != dice.Success {
		t.Fatalf("shove: %v %s", r.Changes, stepErr(r, 0))
	}
	f.w.Campaign.Turns.Budget = FullBudget(true)
	r = Resolve(f.ctx(5), f.aria, []Action{{Kind: ActShove, Target: e.ID, Method: "push"}})
	if e.ZoneID == f.aria.ZoneID || ZoneDistance(f.wood, f.aria.ZoneID, e.ZoneID) != 1 {
		t.Fatalf("push: %v", r.Changes)
	}
	e.ZoneID = f.aria.ZoneID
	e.Conditions = nil
	f.w.Campaign.Turns.Budget = FullBudget(true)
	r = Resolve(f.ctx(5), f.aria, []Action{{Kind: ActGrapple, Target: e.ID}})
	if c := e.Conditions.Get(CondRestrained); c == nil || c.SourceID != f.aria.ID || c.SaveDC != 12 {
		t.Fatalf("grapple: %+v %v", e.Conditions, r.Changes)
	}
	// moving away lets go (the grappled wolf can't strike as she leaves:
	// it is restrained but not stunned, so it does get its opportunity attack)
	f.w.Campaign.Turns.Budget = FullBudget(true)
	r = Resolve(f.ctx(2), f.aria, []Action{{Kind: ActReposition, Zone: f.wood.Zones[0].ID}})
	if e.Conditions.Has(CondRestrained) || !hasChange(r, "lets go") {
		t.Fatalf("release: %v", r.Changes)
	}
}

func TestOpportunityAttacksAndDisengage(t *testing.T) {
	f := newFixture(t)
	e := f.fight(t, 1, 1, wolf)
	r := Resolve(f.ctx(2), f.aria, []Action{{Kind: ActReposition, Zone: f.wood.Zones[0].ID}})
	if len(r.Steps[0].Rolls) != 1 || !r.Steps[0].Rolls[0].Foe || !hasChange(r, "opportunity attack") {
		t.Fatalf("no opportunity attack: %v %+v", r.Changes, r.Steps[0].Rolls)
	}
	if f.aria.ZoneID != f.wood.Zones[0].ID {
		t.Fatal("a miss should not stop the move")
	}
	// one reaction per round
	f.bram.ZoneID = e.ZoneID
	f.w.Campaign.Turns.Budget = FullBudget(true)
	if r := Resolve(f.ctx(2), f.bram, []Action{{Kind: ActReposition, Zone: f.wood.Zones[2].ID}}); len(r.Steps[0].Rolls) != 0 {
		t.Fatal("the wolf already used its reaction this round")
	}
	f.w.Encounter.Round++
	f.bram.ZoneID = e.ZoneID
	f.w.Campaign.Turns.Budget = FullBudget(true)
	r = Resolve(f.ctx(2), f.bram, []Action{{Kind: ActDisengage}, {Kind: ActReposition, Zone: f.wood.Zones[2].ID}})
	if len(r.Steps) != 2 || len(r.Steps[1].Rolls) != 0 || f.bram.ZoneID != f.wood.Zones[2].ID {
		t.Fatalf("disengage: %v", r.Changes)
	}
}

func TestReadiedAndOpportunityReactions(t *testing.T) {
	f := newFixture(t)
	e := f.fight(t, 0, 1, wolf) // party on the Road, wolf one zone off
	f.bram.ZoneID = f.wood.Zones[3].ID
	r := Resolve(f.ctx(), f.aria, []Action{{Kind: ActReady}})
	if stepErr(r, 0) != "" || f.aria.Readied == nil {
		t.Fatal("ready")
	}
	f.w.Campaign.Turns.Budget = FullBudget(true)
	// the wolf closes in (aria's readied d20 15 hits, damage 5)
	turns, _ := RunEnemyTurns(f.w, roller(15, 5))
	if len(turns) != 1 || len(turns[0].Reactions) != 1 || turns[0].Reactions[0].Kind != "readied" {
		t.Fatalf("readied attack did not fire: %+v", turns)
	}
	if e.HP != e.MaxHP-5 || f.aria.Readied != nil {
		t.Fatalf("hp %d", e.HP)
	}
	// an enemy leaving a fighter's zone draws an opportunity attack
	g := newFixture(t)
	w := g.fight(t, 0, 0, wolf)
	g.aria.ZoneID = g.wood.Zones[0].ID
	g.bram.ZoneID = g.wood.Zones[2].ID // the wolf heads for Bram
	addStance(g.aria, CondHidden, "")  // so the wolf leaves her alone
	turns, _ = RunEnemyTurns(g.w, roller(15, 5))
	if len(turns) != 1 || turns[0].Kind != "move" || len(turns[0].Reactions) != 1 || turns[0].Reactions[0].Kind != "opportunity" {
		t.Fatalf("no opportunity attack on the wolf: %+v", turns)
	}
	if w.HP != w.MaxHP-5 {
		t.Fatalf("wolf hp %d", w.HP)
	}
}

func TestSurpriseAndSpotting(t *testing.T) {
	f := newFixture(t)
	f.goTo(f.wood, 0)
	enc, _, err := StartEncounter(f.w, []EnemySpec{wolf})
	must(t, err)
	st := &Step{}
	surprise(f.ctx(2), enc, st) // stealth 2+1 = 3 vs Bram's passive 14: spotted
	if !strings.Contains(strings.Join(st.Changes, " "), "spots the ambush") {
		t.Fatalf("not spotted: %v", st.Changes)
	}
	st = &Step{}
	surprise(f.ctx(19, 1, 15, 3), enc, st) // 20 beats 14: the wolf acts first
	if !strings.Contains(strings.Join(st.Changes, " "), "caught by surprise") || len(st.Changes) < 2 {
		t.Fatalf("no surprise: %v", st.Changes)
	}
}

func TestPassivePerceptionFindsHiddenWays(t *testing.T) {
	f := newFixture(t)
	f.bram.Scores[WIS] = 20 // passive 10 + 5 + 2 = 17 = 15 + danger 2
	st := &Step{}
	arrive(f.ctx(), f.wood, nil, st)
	if !strings.Contains(strings.Join(st.Changes, " "), "Bram notices a hidden way") {
		t.Fatalf("no notice: %v", st.Changes)
	}
}
