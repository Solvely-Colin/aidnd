package game

import (
	"fmt"
	"strings"

	"github.com/Solvely-Colin/aidnd/internal/dice"
)

// MaxLevel is the level cap.
const MaxLevel = 5

// XPTable is the total XP needed to reach each level (index = level-1).
// Paced for a multi-session campaign: level 2 after the first two legs of
// the story (a couple of main objectives, a side quest, some fights each),
// level 3 by the end of a modest campaign, level 4 for long ones full of
// side work, and level 5 rarely. The 2026-10-29 play-test reached level 3
// by turn 18 on the old 0/150/400/800/1300 table with bigger awards.
var XPTable = []int{0, 300, 800, 1600, 2800}

// XP awards.
const (
	XPMainObjective = 75
	XPSideObjective = 20
	XPSideQuest     = 40
	XPPerDanger     = 10 // won fights: 10 x location danger
)

// ProficiencyBonus by level: +2 at 1-2, +3 at 3-4, +4 at 5.
func ProficiencyBonus(level int) int {
	if level < 1 {
		level = 1
	}
	return 2 + (level-1)/2
}

// LevelForXP returns the level a total XP grants.
func LevelForXP(xp int) int {
	lvl := 1
	for i, need := range XPTable {
		if xp >= need {
			lvl = i + 1
		}
	}
	return lvl
}

// HPPerLevel is the HP gained per level after the first.
func HPPerLevel(c *Character) int {
	if g := 6 + c.Mod(CON); g > 1 {
		return g
	}
	return 1
}

// AwardXP adds XP and applies level-ups. It returns change lines.
func AwardXP(c *Character, xp int) []string {
	if xp <= 0 {
		return nil
	}
	c.XP += xp
	changes := []string{fmt.Sprintf("%s gains %d XP", c.Name, xp)}
	for c.Level < MaxLevel && LevelForXP(c.XP) > c.Level {
		c.Level++
		if c.ClassKey != "" {
			changes = append(changes, levelUp(c))
			continue
		}
		gain := HPPerLevel(c)
		c.MaxHP += gain
		if !c.Down() {
			c.HP += gain
		}
		line := fmt.Sprintf("%s reaches level %d (+%d max HP, proficiency +%d)", c.Name, c.Level, gain, ProficiencyBonus(c.Level))
		if c.Level == 4 {
			best := BestAbility(c)
			c.Scores[best] = min(c.Scores[best]+2, 20)
			line += fmt.Sprintf(", %s +2", best)
			if best == CON {
				c.MaxHP += c.Level / 2
			}
		}
		changes = append(changes, line)
	}
	return changes
}

// RaiseToLevel awards the XP that brings a character to at least lvl, so
// a story milestone sets the pace and nobody has to grind.
func RaiseToLevel(c *Character, lvl int) []string {
	lvl = clamp(lvl, 1, MaxLevel)
	if need := XPTable[lvl-1]; c.XP < need {
		return AwardXP(c, need-c.XP)
	}
	return nil
}

// BestAbility is the highest score, ties broken by sheet order.
func BestAbility(c *Character) Ability {
	best := Abilities[0]
	for _, a := range Abilities {
		if c.Scores[a] > c.Scores[best] {
			best = a
		}
	}
	return best
}

// StartingHP is 10 + CON modifier.
func StartingHP(s Scores) int { return 10 + Mod(s[CON]) }

// ACPart is one term of a character's armor class, for the sheet.
type ACPart struct {
	Label string `json:"label"`
	Value int    `json:"value"`
}

// ArmorClass computes AC from equipped armor plus any shield held.
func ArmorClass(w *World, c *Character) int {
	ac := 0
	for _, p := range ArmorClassParts(w, c) {
		ac += p.Value
	}
	return ac
}

// ArmorClassParts breaks ArmorClass into its terms: the armor (or the
// best unarmored base), the DEX modifier after any cap, the first shield
// carried, and fighting style and spell bonuses.
func ArmorClassParts(w *World, c *Character) []ACPart {
	label, base := unarmoredBase(w, c)
	parts := []ACPart{{Label: label, Value: base}}
	dex := c.Mod(DEX)
	armor := wearsArmor(w, c)
	if a := w.Item(c.Armor); a != nil && a.Owner.ID == c.ID && a.Kind == ItemArmor && (armor != nil || a.AC > base) {
		switch {
		case a.DexCap == 0:
			dex = 0
		case a.DexCap > 0 && dex > a.DexCap:
			dex = a.DexCap
		}
		parts[0] = ACPart{Label: a.Name, Value: a.AC + a.Bonus}
	}
	if dex != 0 {
		parts = append(parts, ACPart{Label: string(DEX), Value: dex})
	}
	switch label {
	case "Unarmored Defense (CON)":
		if armor == nil {
			parts[0] = ACPart{Label: "Base", Value: 10}
			parts = append(parts, ACPart{Label: "CON (Unarmored Defense)", Value: c.Mod(CON)})
		}
	case "Unarmored Defense (WIS)":
		if armor == nil {
			parts[0] = ACPart{Label: "Base", Value: 10}
			parts = append(parts, ACPart{Label: "WIS (Unarmored Defense)", Value: c.Mod(WIS)})
		}
	case "Draconic Resilience (CHA)":
		if armor == nil {
			parts[0] = ACPart{Label: "Base", Value: 10}
			parts = append(parts, ACPart{Label: "CHA (Draconic Resilience)", Value: c.Mod(CHA)})
		}
	}
	for _, it := range w.ItemsOf(OwnerCharacter, c.ID) {
		if it.Kind == ItemShield {
			parts = append(parts, ACPart{Label: it.Name, Value: it.AC + it.Bonus})
			break
		}
	}
	if c.boons.AC > 0 {
		parts = append(parts, ACPart{Label: "Protection", Value: c.boons.AC})
	}
	if armor != nil && c.HasPick("defense") {
		parts = append(parts, ACPart{Label: "Defense style", Value: 1})
	}
	if c.Conditions.Has(CondShielded) {
		parts = append(parts, ACPart{Label: "Shield of Faith", Value: 2})
	}
	if c.Conditions.Has(CondHasted) {
		parts = append(parts, ACPart{Label: "Haste", Value: 2})
	}
	return parts
}

// IsEquipped reports whether it is worn or wielded by c. A carried shield
// always counts (see ArmorClass), so the first one shows as equipped.
func IsEquipped(w *World, c *Character, it *Item) bool {
	if c == nil || it == nil {
		return false
	}
	if c.Weapon == it.ID || c.Armor == it.ID {
		return true
	}
	if it.Kind == ItemShield && it.Owner == (Owner{Kind: OwnerCharacter, ID: c.ID}) {
		for _, s := range w.ItemsOf(OwnerCharacter, c.ID) {
			if s.Kind == ItemShield {
				return s.ID == it.ID
			}
		}
	}
	return false
}

// Unarmed is the fallback weapon.
var Unarmed = &Item{ID: "", Name: "fists", Kind: ItemWeapon, Damage: "1d2", Ability: STR}

// Weapon returns the equipped weapon or Unarmed.
func Weapon(w *World, c *Character) *Item {
	if it := w.Item(c.Weapon); it != nil && it.Owner.Kind == OwnerCharacter && it.Owner.ID == c.ID && it.Kind == ItemWeapon {
		return it
	}
	return Unarmed
}

// AttackMod is the to-hit bonus with a weapon: the weapon's ability (the
// better of STR/DEX for finesse, the casting ability for a focus, DEX for
// a monk) plus proficiency, archery and a sacred weapon.
func AttackMod(c *Character, wpn *Item) (int, Ability) {
	return sumParts(AttackParts(c, wpn)), weaponAbility(c, wpn)
}

// CheckRoll is a d20 check with its context, ready for the Resolution.
type CheckRoll struct {
	Label   string     `json:"label"`
	Ability Ability    `json:"ability,omitempty"`
	Parts   []RollPart `json:"parts,omitempty"`   // what the modifier is made of
	Against string     `json:"against,omitempty"` // "AC" or "DC"
	Cover   int        `json:"cover,omitempty"`   // added to the target
	EdgeWhy string     `json:"edge_why,omitempty"`
	dice.Check
}

// RollPart is one named piece of a roll's modifier, so players can see
// where a +5 comes from.
type RollPart struct {
	Label string `json:"label"`
	Value int    `json:"value"`
}

// CheckParts splits CheckMod(a) into the ability modifier and proficiency.
func CheckParts(c *Character, a Ability) []RollPart {
	ps := []RollPart{{Label: string(a), Value: c.Mod(a)}}
	if c.IsProficient(a) {
		ps = append(ps, RollPart{Label: "Proficiency", Value: ProficiencyBonus(c.Level)})
	} else if c.HasFeature("jack_of_all_trades") {
		ps = append(ps, RollPart{Label: "Jack of All Trades", Value: ProficiencyBonus(c.Level) / 2})
	}
	return ps
}

// AttackParts splits AttackMod into its parts.
func AttackParts(c *Character, wpn *Item) []RollPart {
	ab := weaponAbility(c, wpn)
	ps := []RollPart{{Label: string(ab), Value: c.Mod(ab)}, {Label: "Proficiency", Value: ProficiencyBonus(c.Level)}}
	if c.HasPick("archery") && InferReach(wpn) == ReachRanged && !IsFocus(wpn) {
		ps = append(ps, RollPart{Label: "Archery", Value: 2})
	}
	if c.Conditions.Has(CondSacred) {
		ps = append(ps, RollPart{Label: "Sacred Weapon", Value: max(1, c.Mod(CHA))})
	}
	if wpn != nil && wpn.Bonus != 0 {
		ps = append(ps, RollPart{Label: fmt.Sprintf("Magic %+d", wpn.Bonus), Value: wpn.Bonus})
	}
	return ps
}

// AbilityCheck rolls d20 + modifier vs a snapped DC.
func AbilityCheck(r *dice.Roller, c *Character, a Ability, dc int) CheckRoll {
	dc = dice.SnapDC(dc)
	return CheckRoll{Label: fmt.Sprintf("%s %s check", c.Name, a), Ability: a, Parts: CheckParts(c, a), Against: "DC", Check: r.D20(c.CheckMod(a), dc)}
}

// AttackOutcome is an attack roll plus damage.
type AttackOutcome struct {
	Roll   CheckRoll    `json:"roll"`
	Hit    bool         `json:"hit"`
	Crit   bool         `json:"crit"`
	Damage *dice.Result `json:"damage,omitempty"`
}

// Attack rolls toHit vs ac and, on a hit, damage (dice doubled on a 20).
func Attack(r *dice.Roller, label string, toHit, ac int, damage string) AttackOutcome {
	return AttackWithEdge(r, label, toHit, ac, damage, 0, "")
}

// AttackWithEdge is Attack with advantage (edge > 0) or disadvantage
// (edge < 0) for the reasons in why.
func AttackWithEdge(r *dice.Roller, label string, toHit, ac int, damage string, edge int, why string) AttackOutcome {
	ch := r.D20Edge(toHit, ac, edge)
	out := AttackOutcome{Roll: CheckRoll{Label: label, Against: "AC", Check: ch}}
	if edge != 0 {
		out.Roll.EdgeWhy = why
	}
	if !ch.Tier.Succeeded() {
		return out
	}
	out.Hit = true
	out.Crit = ch.Tier == dice.CritSuccess
	e, err := dice.Parse(damage)
	if err != nil {
		e = dice.MustParse("1d4")
	}
	res := r.Roll(e, out.Crit)
	if res.Total < 1 {
		res.Total = 1
	}
	out.Damage = &res
	return out
}

// Subdue methods and the ability each uses.
var SubdueMethods = map[string]Ability{
	"grapple":    STR,
	"tie_up":     DEX,
	"intimidate": CHA,
	"calm":       WIS,
	"persuade":   CHA,
}

// SubdueDC is 10 + the enemy's attack bonus, minus 3 if badly hurt
// (at or below half HP). It is not snapped: it is a contest, not a DC tier.
func SubdueDC(e *Enemy) int {
	dc := 10 + e.AttackBonus
	if e.HP*2 <= e.MaxHP {
		dc -= 3
	}
	return dc
}

// SubdueMethod normalizes a method name, defaulting to grapple.
func SubdueMethod(m string) (string, Ability) {
	m = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(m)), " ", "_")
	switch {
	case strings.Contains(m, "tie"), strings.Contains(m, "bind"):
		m = "tie_up"
	case strings.Contains(m, "intim"), strings.Contains(m, "threat"):
		m = "intimidate"
	case strings.Contains(m, "calm"), strings.Contains(m, "sooth"):
		m = "calm"
	case strings.Contains(m, "persua"), strings.Contains(m, "talk"):
		m = "persuade"
	case m == "":
		m = "grapple"
	}
	if a, ok := SubdueMethods[m]; ok {
		return m, a
	}
	return "grapple", STR
}

// EnemyCaps are the limits on an authored stat block.
type EnemyCaps struct {
	MaxHP, MaxAC, MaxAttack int
	MaxDamage               int // max possible damage roll
}

// CapsFor computes caps from location danger and party level.
func CapsFor(danger, partyLevel int) EnemyCaps {
	danger = clamp(danger, 0, 5)
	partyLevel = clamp(partyLevel, 1, MaxLevel)
	t := danger + partyLevel
	return EnemyCaps{
		MaxHP:     6 + 5*t,
		MaxAC:     11 + (t+1)/2,
		MaxAttack: 2 + t/2,
		MaxDamage: 4 + 2*t,
	}
}

// ClampStatBlock forces an authored stat block within caps and returns
// what was changed.
func ClampStatBlock(s *StatBlock, caps EnemyCaps) []string {
	var notes []string
	if s.MaxHP < 1 {
		s.MaxHP = 1
	}
	if s.MaxHP > caps.MaxHP {
		notes = append(notes, fmt.Sprintf("hp %d->%d", s.MaxHP, caps.MaxHP))
		s.MaxHP = caps.MaxHP
	}
	if s.AC < 8 {
		s.AC = 8
	}
	if s.AC > caps.MaxAC {
		notes = append(notes, fmt.Sprintf("ac %d->%d", s.AC, caps.MaxAC))
		s.AC = caps.MaxAC
	}
	s.AttackBonus = clamp(s.AttackBonus, 0, caps.MaxAttack)
	e, err := dice.Parse(s.Damage)
	if err != nil || e.Count == 0 {
		notes = append(notes, fmt.Sprintf("damage %q->1d6", s.Damage))
		e = dice.MustParse("1d6")
	}
	for e.Max() > caps.MaxDamage && (e.Count > 1 || e.Mod > 0 || e.Sides > 4) {
		switch {
		case e.Mod > 0:
			e.Mod--
		case e.Count > 1:
			e.Count--
		default:
			e.Sides -= 2
		}
	}
	if e.String() != s.Damage {
		s.Damage = e.String()
	}
	return notes
}

// EnemyHPBudget is the total enemy HP allowed in one encounter.
func EnemyHPBudget(danger, partyLevel, partySize int) int {
	partySize = clamp(partySize, 1, 4)
	danger = clamp(danger, 1, 5)
	return partySize * (4 + 3*danger + 4*(clamp(partyLevel, 1, MaxLevel)-1))
}

// BudgetEnemies trims HP so the total fits the budget, dropping extras
// beyond 6 enemies. Each keeps at least 1 HP.
func BudgetEnemies(blocks []StatBlock, budget int) []StatBlock {
	if len(blocks) > 6 {
		blocks = blocks[:6]
	}
	total := 0
	for _, b := range blocks {
		total += b.MaxHP
	}
	if total <= budget || total == 0 {
		return blocks
	}
	out := make([]StatBlock, len(blocks))
	for i, b := range blocks {
		b.MaxHP = max(1, b.MaxHP*budget/total)
		out[i] = b
	}
	return out
}
