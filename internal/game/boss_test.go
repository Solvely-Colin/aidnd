package game

import (
	"strings"
	"testing"

	"github.com/Solvely-Colin/aidnd/internal/dice"
)

// bossFight starts a fight in a danger-3 Whisperwood with a boss and a
// minion, the party in the Road.
func (f *fixture) bossFight(t *testing.T) (*Enemy, *Enemy) {
	t.Helper()
	f.wood.Danger = 3
	f.goTo(f.wood, 0)
	enc, _, err := StartEncounter(f.w, []EnemySpec{
		{Name: "Bog Queen", HP: 20, AC: 12, AttackBonus: 3, Damage: "1d8", Boss: &BossSpec{Move: "Drowning Wail"}},
		{Name: "Bog Rat", HP: 30, AC: 10, AttackBonus: 2, Damage: "1d4"},
	})
	must(t, err)
	f.w.Campaign.Turns.Budget = FullBudget(true)
	return enc.Enemies[0], enc.Enemies[1]
}

func TestBossStatsAndBudget(t *testing.T) {
	f := newFixture(t)
	boss, rat := f.bossFight(t)
	if boss.Boss == nil || boss.Boss.Move.Name != "Drowning Wail" {
		t.Fatalf("boss state %+v", boss.Boss)
	}
	normal := CapsFor(3, f.w.PartyLevel()).MaxHP
	if boss.MaxHP <= 20 || boss.HP != boss.MaxHP {
		t.Fatalf("boss HP %d/%d, normal cap %d", boss.HP, boss.MaxHP, normal)
	}
	if rat.Boss != nil || rat.MaxHP > EnemyHPBudget(3, f.w.PartyLevel(), 2)/3 {
		t.Fatalf("followers share a third of the budget: rat %d", rat.MaxHP)
	}
	// only one boss, and none in a mild place
	f2 := newFixture(t)
	f2.goTo(f2.wood, 0)
	enc, _, err := StartEncounter(f2.w, []EnemySpec{{Name: "Wolf Lord", HP: 20, AC: 12, AttackBonus: 3, Damage: "1d6", Boss: &BossSpec{}}})
	must(t, err)
	if enc.Enemies[0].Boss != nil {
		t.Fatal("a danger-2 place got a boss")
	}
}

func TestBossPhasesAndSummons(t *testing.T) {
	f := newFixture(t)
	boss, _ := f.bossFight(t)
	atk, n := boss.AttackBonus, len(f.w.Encounter.Enemies)
	lines := hurtEnemy(f.w, f.aria, boss, boss.MaxHP/2+1)
	if boss.Boss.Phase != 1 || boss.AttackBonus != atk+1 || !strings.Contains(strings.Join(lines, "|"), "Enraged") {
		t.Fatalf("phase 2: phase %d atk %d lines %v", boss.Boss.Phase, boss.AttackBonus, lines)
	}
	boss.Conditions.add(newCondition(CondProne, 12, "shove", f.aria.ID))
	lines = hurtEnemy(f.w, f.aria, boss, boss.HP-boss.MaxHP/4)
	if boss.Boss.Phase != 2 || len(f.w.Encounter.Enemies) <= n || boss.Conditions.Has(CondProne) {
		t.Fatalf("phase 3: phase %d enemies %d prone %v lines %v", boss.Boss.Phase, len(f.w.Encounter.Enemies), boss.Conditions.Has(CondProne), lines)
	}
	if !strings.Contains(strings.Join(lines, "|"), "summons") {
		t.Fatalf("no summon line: %v", lines)
	}
}

func TestBossTelegraphsThenLands(t *testing.T) {
	f := newFixture(t)
	boss, rat := f.bossFight(t)
	rat.HP = 0
	boss.ZoneID = f.aria.ZoneID
	boss.AttackBonus = -20 // its attacks miss; only the move matters here
	f.w.Campaign.Turns.Index = len(f.w.Campaign.Turns.Order) - 1
	r := dice.New(7)
	turns, _ := RunEnemyTurns(f.w, r)
	var told bool
	for _, et := range turns {
		if et.Kind == "telegraph" {
			told = true
		}
	}
	if !told || boss.Boss.Charging == nil || boss.Boss.Charging.Zone != f.aria.ZoneID {
		t.Fatalf("no telegraph: %+v / %+v", turns, boss.Boss.Charging)
	}
	// Bram gets clear; Aria stays
	f.bram.ZoneID = f.wood.Zones[1].ID
	f.w.Encounter.Round++
	hp := f.aria.HP
	turns, _ = RunEnemyTurns(f.w, r)
	if len(turns) != 1 || turns[0].Kind != "signature" || len(turns[0].Saves) != 1 {
		t.Fatalf("move should land on Aria alone: %+v", turns)
	}
	if f.aria.HP >= hp || f.bram.HP != f.bram.MaxHP {
		t.Fatalf("aria %d->%d, bram %d", hp, f.aria.HP, f.bram.HP)
	}
	if boss.Boss.Charging != nil || boss.Boss.Cooldown != 2 {
		t.Fatalf("after landing: %+v", boss.Boss)
	}
}

func TestLegendaryResistance(t *testing.T) {
	f := newFixture(t)
	boss, _ := f.bossFight(t)
	left := boss.Boss.Resist
	roll := EnemySave(dice.FromSource(dice.NewScripted(2)), boss, WIS, 30, "Hold Person")
	if !roll.Tier.Succeeded() || boss.Boss.Resist != left-1 || !strings.Contains(roll.Label, "legendary") {
		t.Fatalf("resist: %+v left %d", roll, boss.Boss.Resist)
	}
}

func TestAntagonistFightsAsBoss(t *testing.T) {
	f := newFixture(t)
	f.lair.Zones = nil
	EnsureZones(f.w, f.lair.ID)
	f.goTo(f.lair, 0)
	f.villain.ZoneID = f.lair.Zones[0].ID
	e := NPCJoinsFight(f.w, f.villain)
	if e == nil || e.Boss == nil || e.Boss.Resist != 2 {
		t.Fatalf("antagonist enemy %+v", e)
	}
}
