package game

import (
	"strings"
	"testing"
)

// fight puts the party in Whisperwood at zone index pz with one enemy at
// zone index ez.
func (f *fixture) fight(t *testing.T, pz, ez int, spec EnemySpec) *Enemy {
	t.Helper()
	f.goTo(f.wood, pz)
	enc, _, err := StartEncounter(f.w, []EnemySpec{spec})
	must(t, err)
	e := enc.Enemies[0]
	e.ZoneID = f.wood.Zones[ez].ID
	f.w.Campaign.Turns.Budget = FullBudget(true)
	return e
}

var wolf = EnemySpec{Name: "Wolf", HP: 10, AC: 12, AttackBonus: 3, Damage: "1d6"}

func TestEnemiesStartAtRange(t *testing.T) {
	f := newFixture(t)
	f.goTo(f.wood, 0)
	enc, _, err := StartEncounter(f.w, []EnemySpec{wolf, {Name: "Archer", HP: 8, AC: 12, AttackBonus: 3, Damage: "1d6", Ranged: true}})
	must(t, err)
	l := f.wood
	if d := ZoneDistance(l, f.aria.ZoneID, enc.Enemies[0].ZoneID); d != 1 {
		t.Errorf("melee enemy at %d", d)
	}
	if d := ZoneDistance(l, f.aria.ZoneID, enc.Enemies[1].ZoneID); d != 2 {
		t.Errorf("ranged enemy at %d", d)
	}
	if f.w.Campaign.Turns.Budget.Free != CombatFreeActions {
		t.Errorf("free actions not limited in combat: %d", f.w.Campaign.Turns.Budget.Free)
	}
}

func TestMeleeOutOfReachRefused(t *testing.T) {
	f := newFixture(t)
	f.fight(t, 0, 2, wolf)
	r := Resolve(f.ctx(15, 5), f.aria, []Action{{Kind: ActAttack, Target: "Wolf"}})
	if e := stepErr(r, 0); !strings.Contains(e, "2 zones away") || !strings.Contains(e, "reposition closer") {
		t.Fatalf("got %q", e)
	}
	if r.TurnEnded || f.w.Campaign.Turns.Budget.Action != 1 {
		t.Fatal("refused attack spent the action")
	}
}

func TestRangedAttackWithCover(t *testing.T) {
	f := newFixture(t)
	e := f.fight(t, 0, 1, wolf) // wolf in Fallen Logs (cover), 1 zone away
	// Bram longbow: DEX+2 prof+2 = +4. AC 12 + cover 2 = 14. Face 9 -> 13 misses.
	r := Resolve(f.ctx(9), f.bram, []Action{{Kind: ActAttack, Target: e.ID}})
	if r.Steps[0].Rolls[0].Target != 14 || r.Steps[0].Tier != "failure" {
		t.Fatalf("cover not applied: %+v", r.Steps[0].Rolls)
	}
	f.w.Campaign.Turns.Budget = FullBudget(true)
	r = Resolve(f.ctx(10, 6), f.bram, []Action{{Kind: ActAttack, Target: "wolf"}})
	if e.HP != 4 || !hasChange(r, "hits Wolf for 6") {
		t.Fatalf("hit: hp %d changes %v", e.HP, r.Changes)
	}
}

func TestChargeAndAttackIsTwoSteps(t *testing.T) {
	f := newFixture(t)
	e := f.fight(t, 0, 1, wolf)
	r := Resolve(f.ctx(15, 8), f.aria, []Action{
		{Kind: ActReposition, Zone: "Fallen Logs"},
		{Kind: ActAttack, Target: "Wolf"},
	})
	if stepErr(r, 0) != "" || stepErr(r, 1) != "" {
		t.Fatalf("errors: %q %q", stepErr(r, 0), stepErr(r, 1))
	}
	if r.Steps[0].Slot != SlotMove || r.Steps[1].Slot != SlotAction || !r.TurnEnded {
		t.Fatalf("slots %s %s ended=%v", r.Steps[0].Slot, r.Steps[1].Slot, r.TurnEnded)
	}
	if e.HP != 2 {
		t.Fatalf("wolf hp %d", e.HP)
	}
	// same zone: no cover bonus
	if r.Steps[1].Rolls[0].Target != 12 {
		t.Fatalf("cover applied in same zone")
	}
}

func TestDashCostsMoveAndAction(t *testing.T) {
	f := newFixture(t)
	f.fight(t, 0, 3, wolf)
	r := Resolve(f.ctx(), f.aria, []Action{{Kind: ActReposition, Zone: "Clearing"}, {Kind: ActLook}})
	if r.Steps[0].Slot != SlotDash || !r.TurnEnded {
		t.Fatalf("slot %s ended %v", r.Steps[0].Slot, r.TurnEnded)
	}
	if len(r.Skipped) != 1 || !strings.Contains(r.Skipped[0].Error, "turn ended") {
		t.Fatalf("skipped %+v", r.Skipped)
	}
	f.w.Campaign.Turns.Budget = FullBudget(true)
	f.aria.ZoneID = f.wood.Zones[0].ID
	r = Resolve(f.ctx(), f.aria, []Action{{Kind: ActReposition, Zone: "Thicket"}})
	if !strings.Contains(stepErr(r, 0), "at most 2 zones") {
		t.Fatalf("3-zone move allowed: %q", stepErr(r, 0))
	}
	// second single-zone move becomes a dash (paid with the action)
	r = Resolve(f.ctx(), f.aria, []Action{{Kind: ActReposition, Zone: "Fallen Logs"}, {Kind: ActReposition, Zone: "Clearing"}})
	if r.Steps[1].Slot != SlotDash || !r.TurnEnded {
		t.Fatalf("second move slot %s", r.Steps[1].Slot)
	}
}

func TestRepositionFreeOutsideCombat(t *testing.T) {
	f := newFixture(t)
	f.goTo(f.wood, 0)
	r := Resolve(f.ctx(), f.aria, []Action{{Kind: ActReposition, Zone: "Thicket"}})
	if r.Steps[0].Slot != SlotFree || !r.AllFree || r.TurnEnded {
		t.Fatalf("slot %s allFree %v", r.Steps[0].Slot, r.AllFree)
	}
}

func TestFleeRequiresExitZone(t *testing.T) {
	f := newFixture(t)
	f.fight(t, 1, 2, wolf) // party in Fallen Logs, exit home is from Road
	r := Resolve(f.ctx(), f.aria, []Action{{Kind: ActMove, Target: "Oakvale"}})
	if !strings.Contains(stepErr(r, 0), "must stand in Road") {
		t.Fatalf("flee from wrong zone: %q", stepErr(r, 0))
	}
	r = Resolve(f.ctx(), f.aria, []Action{{Kind: ActReposition, Zone: "Road"}, {Kind: ActMove, Target: "the road home"}})
	if stepErr(r, 1) != "" || f.w.Campaign.LocationID != f.hub.ID || f.w.Encounter != nil {
		t.Fatalf("flee failed: %q", stepErr(r, 1))
	}
	if f.aria.ZoneID != f.hub.Zones[0].ID {
		t.Fatalf("arrived in %s, want exit zone Square", f.aria.ZoneID)
	}
}

func TestKillEndsFightAndAwardsXP(t *testing.T) {
	f := newFixture(t)
	e := f.fight(t, 1, 1, EnemySpec{Name: "Rat", HP: 3, AC: 5, AttackBonus: 1, Damage: "1d2"})
	r := Resolve(f.ctx(10, 8), f.aria, []Action{{Kind: ActAttack, Target: "rat"}})
	if !e.Defeated() || !r.CombatEnded || f.w.Encounter != nil {
		t.Fatalf("fight not ended: %v", r.Changes)
	}
	if f.aria.XP != XPPerDanger*2 || f.bram.XP != XPPerDanger*2 {
		t.Fatalf("xp %d %d", f.aria.XP, f.bram.XP)
	}
	if len(f.w.Defeats) != 1 || f.w.Defeats[0].Name != "Rat" {
		t.Fatalf("defeats %+v", f.w.Defeats)
	}
	if f.w.Campaign.Turns.Budget.Free != Unlimited {
		t.Fatal("free actions not restored after combat")
	}
}

func TestSubdue(t *testing.T) {
	f := newFixture(t)
	e := f.fight(t, 1, 1, EnemySpec{Name: "Thug", HP: 10, AC: 12, AttackBonus: 3, Damage: "1d6"})
	if SubdueDC(e) != 13 {
		t.Fatalf("dc %d", SubdueDC(e))
	}
	e.HP = 5
	if SubdueDC(e) != 10 {
		t.Fatalf("hurt dc %d", SubdueDC(e))
	}
	// Aria grapple: STR +2 +2 prof; face 6 -> 10 vs 10
	r := Resolve(f.ctx(6), f.aria, []Action{{Kind: ActSubdue, Target: "Thug", Method: "grapple"}})
	if !e.Subdued || !r.CombatEnded {
		t.Fatalf("not subdued: %v %s", r.Changes, stepErr(r, 0))
	}
}

func TestSubduedNPCSurvivesAndAttackedNPCTurnsHostile(t *testing.T) {
	f := newFixture(t)
	f.goTo(f.hub, 1) // Forge, with Dunna
	f.smith.ZoneID = f.hub.Zones[1].ID
	// "I attack the blacksmith": face 2 misses, but the NPC becomes an enemy
	r := Resolve(f.ctx(2), f.aria, []Action{{Kind: ActAttack, Target: "Dunna"}})
	if !f.w.InCombat() || f.smith.Disposition != -3 || !r.CombatStarted {
		t.Fatalf("npc not hostile: %v %s", r.Changes, stepErr(r, 0))
	}
	f.w.Campaign.Turns.Budget = FullBudget(true)
	r = Resolve(f.ctx(19), f.aria, []Action{{Kind: ActSubdue, Target: "Dunna", Method: "intimidate"}})
	if !f.smith.Alive || f.w.InCombat() {
		t.Fatalf("subdue: alive=%v combat=%v %s", f.smith.Alive, f.w.InCombat(), stepErr(r, 0))
	}
}

func TestEnemyTurnsCloseInThenAttack(t *testing.T) {
	f := newFixture(t)
	e := f.fight(t, 0, 2, wolf)
	ctx := f.ctx()
	turns, _ := RunEnemyTurns(f.w, ctx.R)
	if len(turns) != 1 || turns[0].Kind != "move" || e.ZoneID != f.wood.Zones[1].ID {
		t.Fatalf("turn 1: %+v", turns)
	}
	ctx.R = f.ctx(1, 1, 18, 4).R // pick idx 0, weakest-roll(0=weakest), d20 18, dmg 4
	if turns, _ = RunEnemyTurns(f.w, ctx.R); len(turns) != 0 {
		t.Fatalf("an enemy acts once per round: %+v", turns)
	}
	f.w.Encounter.Round++
	turns, _ = RunEnemyTurns(f.w, ctx.R)
	if turns[0].Kind != "move" {
		t.Fatalf("turn 2 should still close in: %+v", turns[0])
	}
	f.w.Encounter.Round++
	turns, _ = RunEnemyTurns(f.w, ctx.R)
	if turns[0].Kind != "attack" {
		t.Fatalf("turn 3: %+v", turns[0])
	}
	// weakest picked: Bram has fewer HP
	if turns[0].TargetID != f.bram.ID {
		t.Fatalf("target %s", turns[0].Target)
	}
}

func TestRangedEnemyAttacksFromRange(t *testing.T) {
	f := newFixture(t)
	f.fight(t, 0, 2, EnemySpec{Name: "Archer", HP: 6, AC: 11, AttackBonus: 3, Damage: "1d6", Ranged: true})
	turns, _ := RunEnemyTurns(f.w, f.ctx(2, 2, 15, 3).R)
	if turns[0].Kind != "attack" {
		t.Fatalf("archer: %+v", turns[0])
	}
}

func TestTurnOrderSkipsDown(t *testing.T) {
	f := newFixture(t)
	w := f.w
	w.Players = append(w.Players, &Player{ID: "u3", Name: "Cy"})
	cora, err := CreateCharacter(w, "u3", CharacterSpec{Name: "Cora", Kit: "devout",
		Scores: map[string]int{"STR": 10, "DEX": 12, "CON": 13, "INT": 8, "WIS": 15, "CHA": 14}, Proficient: []string{"WIS", "CHA"}})
	must(t, err)
	must(t, StartGame(w, t0))
	if w.ActiveCharacter() != f.aria {
		t.Fatal("first actor")
	}
	f.bram.HP = 0
	if next := EndTurn(w, t0); next != cora {
		t.Fatalf("down character not skipped: %v", next.Name)
	}
	if next := EndTurn(w, t0); next != f.aria {
		t.Fatalf("wrap: %v", next.Name)
	}
	if w.Campaign.Turn != 2 {
		t.Fatalf("turn counter %d", w.Campaign.Turn)
	}
	f.aria.HP, cora.HP = 0, 0
	if EndTurn(w, t0) != nil || !AllDown(w) {
		t.Fatal("wipe not detected")
	}
}

func TestSetbacksThenFailure(t *testing.T) {
	f := newFixture(t)
	f.goTo(f.wood, 0)
	f.aria.Gold = 100
	for _, c := range f.w.Characters {
		c.HP = 0
	}
	day := f.w.Campaign.Scene.Day
	ch := Setback(f.w)
	if f.w.Campaign.LocationID != f.hub.ID || f.aria.HP != f.aria.MaxHP/2 || f.aria.Gold != 75 {
		t.Fatalf("setback: loc %s hp %d gold %d (%v)", f.w.Campaign.LocationID, f.aria.HP, f.aria.Gold, ch)
	}
	if f.w.Campaign.Scene.Day != day+1 || f.w.Campaign.Scene.TimeName() != "dawn" {
		t.Fatal("not dawn of the next day")
	}
	Setback(f.w)
	if f.w.Campaign.Status != StatusActive {
		t.Fatal("failed too early")
	}
	Setback(f.w)
	if f.w.Campaign.Status != StatusFailed {
		t.Fatal("third wipe did not fail the campaign")
	}
}

func TestHealingPotionRevivesAlly(t *testing.T) {
	f := newFixture(t)
	f.goTo(f.hub, 0)
	f.bram.HP = 0
	r := Resolve(f.ctx(3, 3), f.aria, []Action{{Kind: ActUseItem, Item: "potion", Target: "Bram"}})
	if f.bram.HP != 8 || !hasChange(r, "back on their feet") {
		t.Fatalf("bram hp %d: %v %s", f.bram.HP, r.Changes, stepErr(r, 0))
	}
	if f.w.FindItemOf(OwnerCharacter, f.aria.ID, "potion") != nil {
		t.Fatal("potion not consumed")
	}
}

func TestRest(t *testing.T) {
	f := newFixture(t)
	f.aria.HP = 1
	Resolve(f.ctx(), f.aria, []Action{{Kind: ActRest}})
	if f.aria.HP != f.aria.MaxHP || f.w.Campaign.Scene.Day != 1 {
		t.Fatalf("long rest at hub: hp %d day %d", f.aria.HP, f.w.Campaign.Scene.Day)
	}
	f.fight(t, 0, 2, wolf)
	f.w.Campaign.Turns.Budget = FullBudget(true)
	r := Resolve(f.ctx(), f.aria, []Action{{Kind: ActRest}})
	if !strings.Contains(stepErr(r, 0), "during a fight") {
		t.Fatalf("rested in combat: %q", stepErr(r, 0))
	}
}

func TestEnemiesSpreadOverTheRound(t *testing.T) {
	f := newFixture(t)
	f.fight(t, 0, 0, wolf)
	ctx := f.ctx()
	for i := 0; i < 2; i++ {
		e := *f.w.Encounter.Enemies[0]
		e.ID = f.w.NewID("foe")
		f.w.Encounter.Enemies = append(f.w.Encounter.Enemies, &e)
	}
	// two characters: after the first, ceil(3/2) = 2 act; after the second, the last one
	f.w.Campaign.Turns.Index = 0
	first, _ := RunEnemyTurns(f.w, ctx.R)
	f.w.Campaign.Turns.Index = 1
	second, _ := RunEnemyTurns(f.w, ctx.R)
	if len(first) != 2 || len(second) != 1 {
		t.Fatalf("spread %d + %d, want 2 + 1", len(first), len(second))
	}
}
