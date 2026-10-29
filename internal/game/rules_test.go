package game

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Solvely-Colin/aidnd/internal/dice"
)

func TestModifiers(t *testing.T) {
	for score, want := range map[int]int{8: -1, 9: -1, 10: 0, 11: 0, 12: 1, 15: 2, 20: 5, 3: -4} {
		if got := Mod(score); got != want {
			t.Errorf("Mod(%d)=%d want %d", score, got, want)
		}
	}
	if ProficiencyBonus(1) != 2 || ProficiencyBonus(3) != 3 || ProficiencyBonus(5) != 4 {
		t.Error("proficiency table")
	}
}

func TestCharacterCreationFromKit(t *testing.T) {
	f := newFixture(t)
	a, b := f.aria, f.bram
	if a.MaxHP != 12 || a.HP != 12 || b.MaxHP != 11 {
		t.Fatalf("hp: aria %d bram %d", a.MaxHP, b.MaxHP)
	}
	if a.Gold != Kits["warrior"].Gold {
		t.Fatalf("gold %d", a.Gold)
	}
	if ac := ArmorClass(f.w, a); ac != 18 { // chain 16 + shield 2
		t.Fatalf("aria AC %d", ac)
	}
	if ac := ArmorClass(f.w, b); ac != 13 { // leather 11 + DEX 2
		t.Fatalf("bram AC %d", ac)
	}
	if w := Weapon(f.w, b); w.Key != "longbow" {
		t.Fatalf("bram weapon %q", w.Name)
	}
	if f.w.Player("u1").CharacterID != a.ID {
		t.Fatal("player not linked")
	}
}

func TestValidateCharacterRejects(t *testing.T) {
	good := map[string]int{"STR": 15, "DEX": 14, "CON": 13, "INT": 12, "WIS": 10, "CHA": 8}
	cases := []struct {
		name string
		spec CharacterSpec
		want string
	}{
		{"dup value", CharacterSpec{Name: "X", Kit: "bard", Scores: map[string]int{"STR": 15, "DEX": 15, "CON": 13, "INT": 12, "WIS": 10, "CHA": 8}, Proficient: []string{"STR", "DEX"}}, "standard array"},
		{"too high", CharacterSpec{Name: "X", Kit: "bard", Scores: map[string]int{"STR": 18, "DEX": 14, "CON": 13, "INT": 12, "WIS": 10, "CHA": 8}, Proficient: []string{"STR", "DEX"}}, "standard array"},
		{"missing", CharacterSpec{Name: "X", Kit: "bard", Scores: map[string]int{"STR": 15}, Proficient: []string{"STR", "DEX"}}, "missing score"},
		{"three profs", CharacterSpec{Name: "X", Kit: "bard", Scores: good, Proficient: []string{"STR", "DEX", "CON"}}, "exactly 2"},
		{"bad kit", CharacterSpec{Name: "X", Kit: "paladin", Scores: good, Proficient: []string{"STR", "DEX"}}, "kit must be"},
		{"no name", CharacterSpec{Kit: "bard", Scores: good, Proficient: []string{"STR", "DEX"}}, "name is required"},
		{"bad ability", CharacterSpec{Name: "X", Kit: "bard", Scores: good, Proficient: []string{"LUCK", "DEX"}}, "unknown proficiency"},
	}
	for _, c := range cases {
		_, _, p := ValidateCharacter(c.spec)
		if !strings.Contains(strings.Join(p, "|"), c.want) {
			t.Errorf("%s: problems %v, want %q", c.name, p, c.want)
		}
	}
	if _, _, p := ValidateCharacter(CharacterSpec{Name: "Ok", Kit: "bard", Scores: map[string]int{"strength": 15, "dex": 14, "Con": 13, "INT": 12, "WIS": 10, "CHA": 8}, Proficient: []string{"cha", "Dexterity"}}); len(p) != 0 {
		t.Errorf("lenient names rejected: %v", p)
	}
}

func TestAbilityCheckTiers(t *testing.T) {
	f := newFixture(t)
	cases := []struct {
		face int
		want dice.Tier
	}{{11, dice.Success}, {10, dice.Failure}, {20, dice.CritSuccess}, {1, dice.CritFailure}}
	for _, c := range cases {
		r := dice.FromSource(dice.NewScripted(c.face))
		got := AbilityCheck(r, f.aria, STR, 15) // +2 STR +2 prof
		if got.Tier != c.want || got.Mod != 4 {
			t.Errorf("face %d: %+v", c.face, got)
		}
	}
	// DCs snap to the allowed set
	if got := AbilityCheck(dice.FromSource(dice.NewScripted(10)), f.aria, INT, 12); got.Target != 10 {
		t.Errorf("dc not snapped: %d", got.Target)
	}
}

func TestAttackHitMissCrit(t *testing.T) {
	r := dice.FromSource(dice.NewScripted(10, 6, 9, 20, 3, 4))
	hit := Attack(r, "x", 4, 14, "1d8")
	if !hit.Hit || hit.Damage.Total != 6 {
		t.Fatalf("hit %+v", hit)
	}
	miss := Attack(r, "x", 4, 14, "1d8")
	if miss.Hit || miss.Damage != nil {
		t.Fatalf("miss %+v", miss)
	}
	crit := Attack(r, "x", 0, 30, "1d8")
	if !crit.Crit || len(crit.Damage.Dice) != 2 || crit.Damage.Total != 7 {
		t.Fatalf("crit %+v", crit.Damage)
	}
}

func TestLevelUps(t *testing.T) {
	f := newFixture(t)
	a := f.aria
	ch := AwardXP(a, 800)
	if a.Level != 3 || len(ch) != 3 {
		t.Fatalf("level %d changes %v", a.Level, ch)
	}
	hp := a.MaxHP
	AwardXP(a, 800) // 1600 -> level 4, +2 STR
	if a.Level != 4 || a.Scores[STR] != 17 || a.MaxHP <= hp {
		t.Fatalf("level 4: lvl %d STR %d", a.Level, a.Scores[STR])
	}
	AwardXP(a, 10000)
	if a.Level != MaxLevel {
		t.Fatalf("cap: %d", a.Level)
	}
}

func TestClampStatBlockAndBudget(t *testing.T) {
	caps := CapsFor(1, 1)
	s := StatBlock{MaxHP: 500, AC: 30, AttackBonus: 15, Damage: "10d12+10"}
	ClampStatBlock(&s, caps)
	if s.MaxHP != caps.MaxHP || s.AC != caps.MaxAC || s.AttackBonus != caps.MaxAttack {
		t.Fatalf("not clamped: %+v caps %+v", s, caps)
	}
	if e := dice.MustParse(s.Damage); e.Max() > caps.MaxDamage {
		t.Fatalf("damage %s max %d > %d", s.Damage, e.Max(), caps.MaxDamage)
	}
	bad := StatBlock{MaxHP: 5, AC: 12, Damage: "lots"}
	ClampStatBlock(&bad, caps)
	if bad.Damage != "1d6" {
		t.Fatalf("bad damage -> %q", bad.Damage)
	}
	blocks := BudgetEnemies([]StatBlock{{MaxHP: 40}, {MaxHP: 40}, {MaxHP: 40}}, 30)
	total := 0
	for _, b := range blocks {
		total += b.MaxHP
	}
	if total > 30 {
		t.Fatalf("budget exceeded: %d", total)
	}
}

func TestInferReach(t *testing.T) {
	cases := map[string]Reach{"Elven Longbow": ReachRanged, "Wand of Sparks": ReachRanged, "Throwing Knife": ReachThrown, "Rusty Cleaver": ReachMelee}
	for name, want := range cases {
		if got := InferReach(&Item{Name: name}); got != want {
			t.Errorf("%s: %v want %v", name, got, want)
		}
	}
	if InferReach(&Item{Key: "holy_symbol", Name: "Anything"}) != ReachRanged {
		t.Error("catalog reach ignored")
	}
}

func TestTagEnemies(t *testing.T) {
	es := []*Enemy{{Name: "Bandit"}, {Name: "Bandit"}, {Name: "Wolf"}}
	TagEnemies(es)
	if es[0].Display() != "Bandit 1 (scarred one)" || es[1].Display() != "Bandit 2 (tall one)" || es[2].Display() != "Wolf" {
		t.Fatalf("%s / %s / %s", es[0].Display(), es[1].Display(), es[2].Display())
	}
}

func TestNPCMemoryCapped(t *testing.T) {
	n := &NPC{}
	for i := 0; i < 12; i++ {
		n.Remember(strings.Repeat("x", i+1))
	}
	if len(n.Memory) != MaxNPCMemory || n.Memory[0] != strings.Repeat("x", 5) {
		t.Fatalf("memory %v", n.Memory)
	}
}

func TestNormPronouns(t *testing.T) {
	for in, want := range map[string]string{"She / Her": "she/her", "": "they/them", "he,him": "he/him", "xe/xem": "xe/xem"} {
		if got := NormPronouns(in); got != want {
			t.Errorf("NormPronouns(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestArmorClassParts(t *testing.T) {
	f := newFixture(t)
	for _, c := range []*Character{f.aria, f.bram} {
		sum := 0
		for _, p := range ArmorClassParts(f.w, c) {
			sum += p.Value
		}
		if sum != ArmorClass(f.w, c) {
			t.Fatalf("%s parts sum %d, AC %d", c.Name, sum, ArmorClass(f.w, c))
		}
	}
	got := fmt.Sprint(ArmorClassParts(f.w, f.aria)) // chain mail ignores DEX
	if !strings.Contains(got, "16") || strings.Contains(got, "DEX") || !strings.Contains(got, "2}") {
		t.Fatalf("aria parts %s", got)
	}
	f.aria.Armor = ""
	if p := ArmorClassParts(f.w, f.aria); p[0].Label != "Base" || p[0].Value != 10 {
		t.Fatalf("unarmored %v", p)
	}
}

// TestLevelPacing walks a typical opening leg of a campaign (the first
// two main objectives, a side quest with its objectives, a few small
// fights) and checks the party is still level 1 at the end of it, as the
// 2026-10-29 play-test was already level 3 by then. One more leg of the
// same size earns level 2; level 3 needs most of the main quest.
func TestLevelPacing(t *testing.T) {
	leg := 2*XPMainObjective + 2*XPSideObjective + XPSideQuest + 3*XPPerDanger*1
	if LevelForXP(leg) != 1 {
		t.Fatalf("one leg (%d XP) reaches level %d", leg, LevelForXP(leg))
	}
	if LevelForXP(2*leg) != 2 {
		t.Fatalf("two legs (%d XP) reach level %d", 2*leg, LevelForXP(2*leg))
	}
	// a modest full campaign: the main quest (5 objectives), three side
	// quests at the cap and
	// a dozen danger-2 fights. Longer campaigns reach 4.
	campaign := 5*XPMainObjective + 3*RewardCapsFor(2, 2, false).XP + 6*XPSideObjective + 12*XPPerDanger*2
	if l := LevelForXP(campaign); l != 3 {
		t.Fatalf("a full campaign (%d XP) reaches level %d, want 3", campaign, l)
	}
}
