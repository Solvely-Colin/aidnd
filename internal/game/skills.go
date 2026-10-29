package game

import (
	"fmt"
	"strings"
)

// Skill is one of the 5e skills and the ability it uses.
type Skill struct {
	Key     string  `json:"key"`
	Name    string  `json:"name"`
	Ability Ability `json:"ability"`
}

// Skills lists the eighteen skills in sheet order (grouped by ability).
var Skills = []Skill{
	{"athletics", "Athletics", STR},
	{"acrobatics", "Acrobatics", DEX},
	{"sleight_of_hand", "Sleight of Hand", DEX},
	{"stealth", "Stealth", DEX},
	{"arcana", "Arcana", INT},
	{"history", "History", INT},
	{"investigation", "Investigation", INT},
	{"nature", "Nature", INT},
	{"religion", "Religion", INT},
	{"animal_handling", "Animal Handling", WIS},
	{"insight", "Insight", WIS},
	{"medicine", "Medicine", WIS},
	{"perception", "Perception", WIS},
	{"survival", "Survival", WIS},
	{"deception", "Deception", CHA},
	{"intimidation", "Intimidation", CHA},
	{"performance", "Performance", CHA},
	{"persuasion", "Persuasion", CHA},
}

// SkillKeys lists the skill keys in sheet order.
func SkillKeys() []string {
	out := make([]string, len(Skills))
	for i, s := range Skills {
		out[i] = s.Key
	}
	return out
}

// SkillByKey finds a skill by key or name ("Sleight of Hand", "sleight").
func SkillByKey(s string) (Skill, bool) {
	t := strings.ToLower(strings.TrimSpace(s))
	t = strings.NewReplacer(" ", "_", "-", "_", "'", "").Replace(t)
	if t == "" {
		return Skill{}, false
	}
	for _, sk := range Skills {
		if sk.Key == t {
			return sk, true
		}
	}
	for _, sk := range Skills {
		if strings.HasPrefix(sk.Key, t) {
			return sk, true
		}
	}
	return Skill{}, false
}

// kitSkills are the skills a character built from a starting kit knows,
// until classes and backgrounds pick them.
var kitSkills = map[string][]string{
	"warrior":    {"athletics", "intimidation", "perception"},
	"skirmisher": {"stealth", "sleight_of_hand", "acrobatics", "deception"},
	"ranger":     {"survival", "perception", "stealth", "nature"},
	"arcanist":   {"arcana", "investigation", "history"},
	"devout":     {"religion", "medicine", "insight"},
	"bard":       {"persuasion", "performance", "deception", "insight"},
}

// KitSkills returns the default skills for a kit key.
func KitSkills(kit string) []string { return append([]string(nil), kitSkills[kit]...) }

// MaxChosenSkills caps the skills a builder may pick.
const MaxChosenSkills = 4

// NormalizeSkills validates a list of skill names, dropping duplicates.
func NormalizeSkills(in []string) ([]string, Problems) {
	var out []string
	var p Problems
	seen := map[string]bool{}
	for _, s := range in {
		sk, ok := SkillByKey(s)
		if !ok {
			p = append(p, fmt.Sprintf("unknown skill %q", s))
			continue
		}
		if !seen[sk.Key] {
			seen[sk.Key] = true
			out = append(out, sk.Key)
		}
	}
	return out, p
}

// HasSkill reports proficiency in a skill.
func (c *Character) HasSkill(key string) bool {
	for _, s := range c.SkillList() {
		if s == key {
			return true
		}
	}
	return false
}

func (c *Character) hasExpertise(key string) bool {
	for _, s := range c.Expertise {
		if s == key {
			return true
		}
	}
	return false
}

// SkillParts breaks a skill check's modifier into parts: the ability,
// proficiency, and expertise.
func SkillParts(c *Character, sk Skill) []RollPart {
	ps := []RollPart{{Label: string(sk.Ability), Value: c.Mod(sk.Ability)}}
	switch {
	case c.HasSkill(sk.Key):
		ps = append(ps, RollPart{Label: "Proficiency", Value: ProficiencyBonus(c.Level)})
		if c.hasExpertise(sk.Key) {
			ps = append(ps, RollPart{Label: "Expertise", Value: ProficiencyBonus(c.Level)})
		}
	case c.IsProficient(sk.Ability):
		// sheets from before skills were proficient in whole abilities
		ps = append(ps, RollPart{Label: "Proficiency", Value: ProficiencyBonus(c.Level)})
	}
	return ps
}

// SkillMod is the d20 modifier for a check with a skill.
func (c *Character) SkillMod(sk Skill) int {
	m := 0
	for _, p := range SkillParts(c, sk) {
		m += p.Value
	}
	return m
}

// SkillModByKey is SkillMod for a key; unknown keys fall back to 0.
func (c *Character) SkillModByKey(key string) int {
	sk, ok := SkillByKey(key)
	if !ok {
		return 0
	}
	return c.SkillMod(sk)
}

// PassivePerception is 10 plus the Perception modifier.
func (c *Character) PassivePerception() int {
	sk, _ := SkillByKey("perception")
	return 10 + c.SkillMod(sk)
}

// BestPassive returns the most perceptive conscious character.
func BestPassive(w *World) (*Character, int) {
	var best *Character
	pp := 0
	for _, c := range w.Party() {
		if c.Down() {
			continue
		}
		if v := c.PassivePerception(); best == nil || v > pp {
			best, pp = c, v
		}
	}
	return best, pp
}

// EnemyPassive is an enemy's passive Perception: 10 plus half its attack
// bonus, the same modifier it uses for saves.
func EnemyPassive(e *Enemy) int { return 10 + e.AttackBonus/2 }

// SkillList is the character's skills; sheets made before skills existed
// know their kit's.
func (c *Character) SkillList() []string {
	if len(c.Skills) == 0 {
		return kitSkills[c.Kit]
	}
	return c.Skills
}

// purposeSkill is the default skill for an ability_check purpose.
func purposeSkill(purpose string) string {
	switch purpose {
	case "search":
		return "perception"
	case "unlock":
		return "sleight_of_hand"
	case "force":
		return "athletics"
	case "stabilize":
		return "medicine"
	case "hide":
		return "stealth"
	}
	return ""
}

// subdueSkill is the skill a subdue method uses when its ability matches.
func subdueSkill(method string) string {
	switch method {
	case "grapple":
		return "athletics"
	case "tie_up":
		return "sleight_of_hand"
	case "intimidate":
		return "intimidation"
	case "calm":
		return "insight"
	case "persuade":
		return "persuasion"
	}
	return ""
}
