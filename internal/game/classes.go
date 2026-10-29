package game

import (
	"fmt"
	"strings"
)

// The ruleset is the open 5e rules (System Reference Document 5.2.1,
// CC-BY-4.0), trimmed to what zone combat can resolve and to levels 1-5:
// twelve classes with one subclass each, species, backgrounds, feats and
// spells up to 3rd level. Every era keeps the mechanics and changes only
// the names, the way items are reskinned (see settings.go).

// Caster kinds.
const (
	CasterNone = ""
	CasterFull = "full"
	CasterHalf = "half"
	CasterPact = "pact"
)

// ClassDef is one class.
type ClassDef struct {
	Key         string     `json:"key"`
	Name        string     `json:"name"`
	Desc        string     `json:"desc"`
	HitDie      int        `json:"hit_die"`
	Primary     []Ability  `json:"primary"`
	Saves       []Ability  `json:"saves"`
	SkillCount  int        `json:"skill_count"`
	SkillChoice []string   `json:"skill_choice"` // empty = any skill
	Caster      string     `json:"caster,omitempty"`
	CastAbility Ability    `json:"cast_ability,omitempty"`
	Gear        []string   `json:"gear"`   // fantasy catalog keys
	Weapon      string     `json:"weapon"` // equipped from Gear
	Armor       string     `json:"armor,omitempty"`
	Gold        int        `json:"gold"`
	Kit         string     `json:"kit"` // the old starting kit this class replaces
	Subclass    Subclass   `json:"subclass"`
	Features    []Feature  `json:"features"`
	Scores      [6]int     `json:"-"` // suggested standard array, STR..CHA
	Era         *EraNames  `json:"-"`
	SubEra      *EraNames  `json:"-"`
	Choices     []ChoiceAt `json:"-"`
}

// Subclass is the one subclass each class gets at level 3.
type Subclass struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Desc string `json:"desc"`
}

// Feature is a class, subclass or species feature for the sheet. Key
// names the rule the engine applies, when there is one.
type Feature struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Level    int    `json:"level"`
	Desc     string `json:"desc"`
	Subclass bool   `json:"subclass,omitempty"`
}

// ChoiceAt is a pick a class makes at a level (fighting style, expertise).
type ChoiceAt struct {
	Level int
	Key   string
	Count int
}

// EraNames are a name in each era after fantasy: gaslight, pulp, modern,
// future.
type EraNames [4]string

var eraIndex = map[string]int{SettingGaslight: 0, SettingPulp: 1, SettingModern: 2, SettingFuture: 3}

// eraName picks the era's name for a thing, falling back to fantasy.
func eraName(setting, fantasy string, names *EraNames) string {
	if names == nil {
		return fantasy
	}
	if i, ok := eraIndex[setting]; ok && names[i] != "" {
		return names[i]
	}
	return fantasy
}

var allSkills []string

func sk(keys ...string) []string { return keys }

// Classes are the twelve classes. Gear uses fantasy catalog keys; each era
// translates them with ItemFor.
var Classes = map[string]*ClassDef{
	"barbarian": {Key: "barbarian", Name: "Barbarian", Desc: "A fury in battle who shrugs off blows while raging.", HitDie: 12,
		Primary: []Ability{STR}, Saves: []Ability{STR, CON}, SkillCount: 2, SkillChoice: sk("animal_handling", "athletics", "intimidation", "nature", "perception", "survival"),
		Gear: []string{"greataxe", "handaxe", "handaxe", "rations"}, Weapon: "greataxe", Gold: 15, Kit: "warrior",
		Scores:   [6]int{15, 13, 14, 8, 12, 10},
		Subclass: Subclass{"berserker", "Path of the Berserker", "Rage that hits even harder when you throw caution away."},
		Era:      &EraNames{"Brawler", "Bruiser", "Enforcer", "Juggernaut"}, SubEra: &EraNames{"Bare-Knuckle Champion", "Torpedo", "Wrecking Ball", "Stim-Berserker"},
		Features: []Feature{
			{"rage", "Rage", 1, "Bonus action: until the fight ends, +2 damage with Strength attacks, half damage from hits, advantage on Strength checks and saves. 2 uses (3 from level 3), one back on a short rest.", false},
			{"unarmored_defense_con", "Unarmored Defense", 1, "Without armor your AC is 10 + DEX + CON (a shield still counts).", false},
			{"reckless_attack", "Reckless Attack", 2, "Free: your attacks this turn have advantage, and attacks against you have advantage until your next turn.", false},
			{"danger_sense", "Danger Sense", 2, "Advantage on Dexterity saves.", false},
			{"frenzy", "Frenzy", 3, "While raging, your first hit each turn while reckless deals an extra 2d6.", true},
			{"extra_attack", "Extra Attack", 5, "Attack twice with the Attack action.", false},
		}},
	"bard": {Key: "bard", Name: "Bard", Desc: "A performer whose words and music carry real magic.", HitDie: 8,
		Primary: []Ability{CHA}, Saves: []Ability{DEX, CHA}, SkillCount: 3,
		Caster: CasterFull, CastAbility: CHA,
		Gear: []string{"rapier", "lute", "leather", "dagger"}, Weapon: "rapier", Armor: "leather", Gold: 19, Kit: "bard",
		Scores:   [6]int{8, 14, 12, 13, 10, 15},
		Subclass: Subclass{"lore", "College of Lore", "A collector of secrets with skills to spare."},
		Era:      &EraNames{"Showman", "Crooner", "Con Artist", "Diplomat"}, SubEra: &EraNames{"Society of Raconteurs", "Gossip Columnist", "Influencer", "Cultural Attaché"},
		Choices: []ChoiceAt{{2, "expertise", 2}},
		Features: []Feature{
			{"spellcasting", "Spellcasting", 1, "Casts bard spells with Charisma.", false},
			{"bardic_inspiration", "Bardic Inspiration", 1, "Bonus action: an ally adds a d6 to their next d20 roll. Uses equal to your CHA modifier; back on a short rest from level 5.", false},
			{"expertise", "Expertise", 2, "Double proficiency in two skills.", false},
			{"jack_of_all_trades", "Jack of All Trades", 2, "Half proficiency on ability checks without a skill you know.", false},
			{"lore_skills", "Bonus Proficiencies", 3, "Three more skills of your choice.", true},
			{"cutting_words", "Cutting Words", 3, "Your inspiration also distracts foes (narrative).", true},
			{"font_of_inspiration", "Font of Inspiration", 5, "Bardic Inspiration comes back on a short rest and grows to a d8.", false},
		}},
	"cleric": {Key: "cleric", Name: "Cleric", Desc: "A servant of a higher power who heals and smites.", HitDie: 8,
		Primary: []Ability{WIS}, Saves: []Ability{WIS, CHA}, SkillCount: 2, SkillChoice: sk("history", "insight", "medicine", "persuasion", "religion"),
		Caster: CasterFull, CastAbility: WIS,
		Gear: []string{"mace", "holy_symbol", "chain_mail", "shield", "healers_kit"}, Weapon: "mace", Armor: "chain_mail", Gold: 7, Kit: "devout",
		Scores:   [6]int{13, 8, 14, 10, 15, 12},
		Subclass: Subclass{"life", "Life Domain", "Healing that pours out of you."},
		Era:      &EraNames{"Preacher", "Chaplain", "Faith Healer", "Medic"}, SubEra: &EraNames{"Revivalist", "Field Chaplain", "Trauma Surgeon", "Life Support Specialist"},
		Features: []Feature{
			{"spellcasting", "Spellcasting", 1, "Casts cleric spells with Wisdom.", false},
			{"divine_spark", "Channel Divinity: Divine Spark", 2, "Action: heal an ally 1d8 + WIS, or a foe takes 1d8 + WIS (CON save for half). 2 uses, one back on a short rest.", false},
			{"turn_undead", "Channel Divinity: Turn Undead", 2, "Action: undead in your zone make a WIS save or are frightened.", false},
			{"disciple_of_life", "Disciple of Life", 3, "Healing spells restore an extra 2 + the spell's level.", true},
			{"preserve_life", "Preserve Life", 3, "Channel Divinity: split 5 x your level in healing among wounded allies (up to half their max HP).", true},
			{"sear_undead", "Sear Undead", 5, "Turned undead also take 1d8 radiant.", false},
		}},
	"druid": {Key: "druid", Name: "Druid", Desc: "A keeper of the wild who can wear a beast's shape.", HitDie: 8,
		Primary: []Ability{WIS}, Saves: []Ability{INT, WIS}, SkillCount: 2, SkillChoice: sk("arcana", "animal_handling", "insight", "medicine", "nature", "perception", "religion", "survival"),
		Caster: CasterFull, CastAbility: WIS,
		Gear: []string{"quarterstaff", "holy_symbol", "leather", "rations", "rope"}, Weapon: "quarterstaff", Armor: "leather", Gold: 9, Kit: "ranger",
		Scores:   [6]int{8, 13, 14, 12, 15, 10},
		Subclass: Subclass{"land", "Circle of the Land", "Magic drawn from the land you were born to."},
		Era:      &EraNames{"Hedge Witch", "Swamp Mystic", "Shaman", "Terraformer"}, SubEra: &EraNames{"Wild Country Circle", "Bayou Circle", "Urban Wilds", "Biosphere Keeper"},
		Features: []Feature{
			{"spellcasting", "Spellcasting", 1, "Casts druid spells with Wisdom.", false},
			{"wild_shape", "Wild Shape", 2, "Bonus action: take a beast's shape until the fight ends: 5 temporary HP per level and claws (1d8 + WIS). 2 uses, one back on a short rest.", false},
			{"lands_aid", "Land's Aid", 3, "Spend a Wild Shape use: foes in a zone take 2d6 (CON save for half) and one ally heals 2d6.", true},
			{"natural_recovery", "Natural Recovery", 3, "Short rest: recover a spell slot once per long rest.", true},
		}},
	"fighter": {Key: "fighter", Name: "Fighter", Desc: "A trained warrior who wins fights through grit and technique.", HitDie: 10,
		Primary: []Ability{STR, DEX}, Saves: []Ability{STR, CON}, SkillCount: 2, SkillChoice: sk("acrobatics", "animal_handling", "athletics", "history", "insight", "intimidation", "persuasion", "perception", "survival"),
		Gear: []string{"longsword", "chain_mail", "shield", "crossbow", "healing_potion"}, Weapon: "longsword", Armor: "chain_mail", Gold: 4, Kit: "warrior",
		Scores:   [6]int{15, 13, 14, 8, 12, 10},
		Subclass: Subclass{"champion", "Champion", "Raw athletic excellence and brutal criticals."},
		Era:      &EraNames{"Soldier", "Veteran", "Soldier", "Marine"}, SubEra: &EraNames{"Prize Fighter", "Doughboy", "Special Forces", "Shock Trooper"},
		Choices: []ChoiceAt{{1, "fighting_style", 1}},
		Features: []Feature{
			{"fighting_style", "Fighting Style", 1, "A combat specialty (Archery, Defense, Great Weapon Fighting or Two-Weapon Fighting).", false},
			{"second_wind", "Second Wind", 1, "Bonus action: heal 1d10 + your level. 2 uses (3 from level 4), one back on a short rest.", false},
			{"action_surge", "Action Surge", 2, "Free: take one more action this turn. Once per short rest.", false},
			{"improved_critical", "Improved Critical", 3, "Your attacks crit on a 19 or 20.", true},
			{"extra_attack", "Extra Attack", 5, "Attack twice with the Attack action.", false},
		}},
	"monk": {Key: "monk", Name: "Monk", Desc: "A martial artist whose body is the weapon.", HitDie: 8,
		Primary: []Ability{DEX, WIS}, Saves: []Ability{STR, DEX}, SkillCount: 2, SkillChoice: sk("acrobatics", "athletics", "history", "insight", "religion", "stealth"),
		Gear: []string{"spear", "dagger", "dagger", "rations"}, Weapon: "spear", Gold: 11, Kit: "skirmisher",
		Scores:   [6]int{10, 15, 13, 8, 14, 12},
		Subclass: Subclass{"open_hand", "Warrior of the Open Hand", "Strikes that throw foes to the ground."},
		Era:      &EraNames{"Pugilist", "Adept", "Martial Artist", "Kinetic Adept"}, SubEra: &EraNames{"Baritsu Master", "Kung Fu Adept", "Krav Maga Instructor", "Zero-G Grappler"},
		Features: []Feature{
			{"martial_arts", "Martial Arts", 1, "Unarmed strikes and simple melee weapons use DEX and a d6 (d8 at level 5); bonus action: an unarmed strike.", false},
			{"unarmored_defense_wis", "Unarmored Defense", 1, "Without armor or shield your AC is 10 + DEX + WIS.", false},
			{"focus", "Monk's Focus", 2, "Focus points equal to your level, back on a short rest: Flurry of Blows (two bonus strikes), Patient Defense (bonus dodge), Step of the Wind (bonus disengage and an extra move).", false},
			{"open_hand_technique", "Open Hand Technique", 3, "Flurry hits can knock a foe prone (DEX save).", true},
			{"stunning_strike", "Stunning Strike", 5, "Spend 1 focus when you hit: the foe makes a CON save or is stunned.", false},
			{"extra_attack", "Extra Attack", 5, "Attack twice with the Attack action.", false},
		}},
	"paladin": {Key: "paladin", Name: "Paladin", Desc: "A holy warrior bound by an oath.", HitDie: 10,
		Primary: []Ability{STR, CHA}, Saves: []Ability{WIS, CHA}, SkillCount: 2, SkillChoice: sk("athletics", "insight", "intimidation", "medicine", "persuasion", "religion"),
		Caster: CasterHalf, CastAbility: CHA,
		Gear: []string{"longsword", "chain_mail", "shield", "holy_symbol", "healing_potion"}, Weapon: "longsword", Armor: "chain_mail", Gold: 9, Kit: "warrior",
		Scores:   [6]int{15, 10, 13, 8, 12, 14},
		Subclass: Subclass{"devotion", "Oath of Devotion", "Honesty, courage and a blade that shines."},
		Era:      &EraNames{"Crusader", "Templar", "Vigilante", "Vanguard"}, SubEra: &EraNames{"Knight of the Order", "Sworn Lawman", "Masked Defender", "Peacekeeper"},
		Choices: []ChoiceAt{{2, "fighting_style", 1}},
		Features: []Feature{
			{"lay_on_hands", "Lay on Hands", 1, "Bonus action: heal from a pool of 5 x your level HP, or spend 5 to cure poison.", false},
			{"spellcasting", "Spellcasting", 1, "Casts paladin spells with Charisma.", false},
			{"fighting_style", "Fighting Style", 2, "A combat specialty.", false},
			{"divine_smite", "Divine Smite", 2, "When you hit, spend a spell slot: +2d8 radiant (+1d8 per slot level above 1st).", false},
			{"sacred_weapon", "Sacred Weapon", 3, "Channel Divinity: add your CHA to attack rolls until the fight ends.", true},
			{"extra_attack", "Extra Attack", 5, "Attack twice with the Attack action.", false},
		}},
	"ranger": {Key: "ranger", Name: "Ranger", Desc: "A hunter and tracker of the wild places.", HitDie: 10,
		Primary: []Ability{DEX, WIS}, Saves: []Ability{STR, DEX}, SkillCount: 3, SkillChoice: sk("animal_handling", "athletics", "insight", "investigation", "nature", "perception", "stealth", "survival"),
		Caster: CasterHalf, CastAbility: WIS,
		Gear: []string{"longbow", "shortsword", "shortsword", "studded", "rations", "rope"}, Weapon: "longbow", Armor: "studded", Gold: 7, Kit: "ranger",
		Scores:   [6]int{12, 15, 13, 8, 14, 10},
		Subclass: Subclass{"hunter", "Hunter", "A specialist at bringing down the wounded."},
		Era:      &EraNames{"Frontier Scout", "Big-Game Hunter", "Survivalist", "Scout"}, SubEra: &EraNames{"Bounty Hunter", "Safari Guide", "Tracker", "Xeno-Hunter"},
		Choices: []ChoiceAt{{2, "fighting_style", 1}},
		Features: []Feature{
			{"spellcasting", "Spellcasting", 1, "Casts ranger spells with Wisdom.", false},
			{"favored_enemy", "Favored Enemy", 1, "Cast Hunter's Mark twice per long rest without a slot.", false},
			{"fighting_style", "Fighting Style", 2, "A combat specialty.", false},
			{"colossus_slayer", "Hunter's Prey: Colossus Slayer", 3, "Once a turn, +1d8 damage to a foe that is already wounded.", true},
			{"extra_attack", "Extra Attack", 5, "Attack twice with the Attack action.", false},
		}},
	"rogue": {Key: "rogue", Name: "Rogue", Desc: "A sneak and a specialist who strikes where it hurts.", HitDie: 8,
		Primary: []Ability{DEX}, Saves: []Ability{DEX, INT}, SkillCount: 4, SkillChoice: sk("acrobatics", "athletics", "deception", "insight", "intimidation", "investigation", "perception", "persuasion", "sleight_of_hand", "stealth"),
		Gear: []string{"shortsword", "shortbow", "dagger", "dagger", "leather", "thieves_tools"}, Weapon: "shortsword", Armor: "leather", Gold: 8, Kit: "skirmisher",
		Scores:   [6]int{8, 15, 13, 12, 14, 10},
		Subclass: Subclass{"thief", "Thief", "Quick hands and a talent for climbing."},
		Era:      &EraNames{"Gunslinger", "Gumshoe", "Operator", "Smuggler"}, SubEra: &EraNames{"Cat Burglar", "Second-Story Man", "Infiltrator", "Ghost"},
		Choices: []ChoiceAt{{1, "expertise", 2}},
		Features: []Feature{
			{"sneak_attack", "Sneak Attack", 1, "Once a turn, +1d6 (2d6 at level 3, 3d6 at 5) with a finesse or ranged weapon when you have advantage or an ally is in the target's zone.", false},
			{"expertise", "Expertise", 1, "Double proficiency in two skills.", false},
			{"cunning_action", "Cunning Action", 2, "Bonus action: dash, disengage or hide.", false},
			{"steady_aim", "Steady Aim", 3, "Bonus action: give up your move for advantage on your next attack.", false},
			{"fast_hands", "Fast Hands", 3, "Bonus action: use an item.", true},
			{"uncanny_dodge", "Uncanny Dodge", 5, "Once a round, halve the damage of a hit against you.", false},
		}},
	"sorcerer": {Key: "sorcerer", Name: "Sorcerer", Desc: "Magic in the blood, shaped by will.", HitDie: 6,
		Primary: []Ability{CHA}, Saves: []Ability{CON, CHA}, SkillCount: 2, SkillChoice: sk("arcana", "deception", "insight", "intimidation", "persuasion", "religion"),
		Caster: CasterFull, CastAbility: CHA,
		Gear: []string{"arcane_focus", "dagger", "dagger", "robes"}, Weapon: "arcane_focus", Armor: "robes", Gold: 28, Kit: "arcanist",
		Scores:   [6]int{8, 13, 14, 10, 12, 15},
		Subclass: Subclass{"draconic", "Draconic Sorcery", "Dragon blood that hardens your skin."},
		Era:      &EraNames{"Medium", "Psychic", "Esper", "Psion"}, SubEra: &EraNames{"Wyrm-Touched", "Serpent Blood", "Gene-Sparked", "Psi-Dragon Line"},
		Features: []Feature{
			{"spellcasting", "Spellcasting", 1, "Casts sorcerer spells with Charisma.", false},
			{"innate_sorcery", "Innate Sorcery", 1, "Bonus action: until the fight ends, +1 spell save DC and advantage on spell attacks. Twice per long rest.", false},
			{"font_of_magic", "Font of Magic", 2, "Sorcery points equal to your level: bonus action to turn 2/3/5 points into a 1st/2nd/3rd-level slot.", false},
			{"draconic_resilience", "Draconic Resilience", 3, "+1 max HP per level; without armor your AC is 10 + DEX + CHA.", true},
		}},
	"warlock": {Key: "warlock", Name: "Warlock", Desc: "Power bargained from something vast.", HitDie: 8,
		Primary: []Ability{CHA}, Saves: []Ability{WIS, CHA}, SkillCount: 2, SkillChoice: sk("arcana", "deception", "history", "intimidation", "investigation", "nature", "religion"),
		Caster: CasterPact, CastAbility: CHA,
		Gear: []string{"arcane_focus", "dagger", "dagger", "leather"}, Weapon: "arcane_focus", Armor: "leather", Gold: 15, Kit: "arcanist",
		Scores:   [6]int{8, 13, 14, 12, 10, 15},
		Subclass: Subclass{"fiend", "Fiend Patron", "A pact with something infernal that feeds on your victories."},
		Era:      &EraNames{"Diabolist", "Cultist", "Hexer", "Void-Touched"}, SubEra: &EraNames{"Infernal Compact", "The Crawling Patron", "Deal with the Devil", "Void Entity"},
		Choices: []ChoiceAt{{1, "invocation", 1}, {2, "invocation", 3}, {5, "invocation", 5}},
		Features: []Feature{
			{"pact_magic", "Pact Magic", 1, "Few spell slots that all cast at your highest level, back on a short rest.", false},
			{"invocations", "Eldritch Invocations", 1, "Secrets that shape your power (1, then 3 at level 2, 5 at level 5).", false},
			{"magical_cunning", "Magical Cunning", 2, "Outside a fight, recover half your pact slots once per long rest.", false},
			{"dark_ones_blessing", "Dark One's Blessing", 3, "When you drop a foe, gain temporary HP equal to CHA + your level.", true},
		}},
	"wizard": {Key: "wizard", Name: "Wizard", Desc: "A scholar of magic with a book of spells.", HitDie: 6,
		Primary: []Ability{INT}, Saves: []Ability{INT, WIS}, SkillCount: 2, SkillChoice: sk("arcana", "history", "insight", "investigation", "medicine", "nature", "religion"),
		Caster: CasterFull, CastAbility: INT,
		Gear: []string{"arcane_focus", "quarterstaff", "robes", "lantern"}, Weapon: "arcane_focus", Armor: "robes", Gold: 5, Kit: "arcanist",
		Scores:   [6]int{8, 14, 13, 15, 12, 10},
		Subclass: Subclass{"evoker", "Evoker", "Blasts that never miss entirely."},
		Era:      &EraNames{"Natural Philosopher", "Occultist", "Hacker", "Technomancer"}, SubEra: &EraNames{"Galvanist", "Thaumaturge", "Black-Hat", "Plasma Engineer"},
		Choices: []ChoiceAt{{2, "scholar", 1}},
		Features: []Feature{
			{"spellcasting", "Spellcasting", 1, "Casts wizard spells with Intelligence.", false},
			{"arcane_recovery", "Arcane Recovery", 1, "Your first short rest each day recovers spell slots worth half your level.", false},
			{"scholar", "Scholar", 2, "Expertise in one of Arcana, History, Investigation, Medicine, Nature or Religion.", false},
			{"potent_cantrip", "Potent Cantrip", 3, "Foes who save against your cantrips still take half damage.", true},
			{"sculpt_spells", "Sculpt Spells", 3, "Your blasts never hit allies (narrative).", true},
		}},
}

// ClassKeys lists classes in display order.
func ClassKeys() []string {
	return []string{"barbarian", "bard", "cleric", "druid", "fighter", "monk", "paladin", "ranger", "rogue", "sorcerer", "warlock", "wizard"}
}

// kitClass maps the old starting kits to the class a sheet made from one
// plays as.
var kitClass = map[string]string{"warrior": "fighter", "skirmisher": "rogue", "ranger": "ranger", "arcanist": "wizard", "devout": "cleric", "bard": "bard"}

// ClassOf returns c's class. Sheets made before classes play as the class
// their kit maps to.
func ClassOf(c *Character) *ClassDef {
	if cd := Classes[c.ClassKey]; cd != nil {
		return cd
	}
	if k := kitClass[c.Kit]; k != "" {
		return Classes[k]
	}
	return Classes["fighter"]
}

// ClassName is the class's name in setting.
func ClassName(cd *ClassDef, setting string) string { return eraName(setting, cd.Name, cd.Era) }

// SubclassName is the subclass's name in setting.
func SubclassName(cd *ClassDef, setting string) string {
	return eraName(setting, cd.Subclass.Name, cd.SubEra)
}

// HasFeature reports whether c has class, subclass, species or feat
// feature key at their level.
func (c *Character) HasFeature(key string) bool {
	for _, f := range ClassOf(c).Features {
		if f.Key == key && c.Level >= f.Level {
			return true
		}
	}
	if sp := Species[c.SpeciesKey()]; sp != nil {
		for _, f := range sp.Traits {
			if f.Key == key && c.Level >= f.Level {
				return true
			}
		}
	}
	return c.HasFeat(key) || c.HasPick(key)
}

// FeaturesOf lists c's features at their level: class, subclass, species
// and feats.
func FeaturesOf(c *Character) []Feature {
	var out []Feature
	for _, f := range ClassOf(c).Features {
		if c.Level >= f.Level {
			out = append(out, f)
		}
	}
	if sp := Species[c.SpeciesKey()]; sp != nil {
		for _, f := range sp.Traits {
			if c.Level >= f.Level {
				out = append(out, f)
			}
		}
	}
	for _, k := range c.FeatList() {
		if ft := Feats[k]; ft != nil {
			out = append(out, Feature{Key: k, Name: ft.Name, Level: 1, Desc: ft.Desc})
		}
	}
	return out
}

// IsCaster reports whether c casts spells (from class or a feat).
func (c *Character) IsCaster() bool {
	return ClassOf(c).Caster != CasterNone || len(c.SpellList()) > 0
}

// CastAbility is c's spellcasting ability; a non-caster with a feat uses
// INT for wizard spells, WIS for cleric and druid ones.
func (c *Character) CastAbility() Ability {
	if a := ClassOf(c).CastAbility; a != "" {
		return a
	}
	if p := c.Picks["magic_initiate"]; len(p) > 0 {
		switch p[0] {
		case "cleric", "druid":
			return WIS
		}
	}
	if c.SpeciesKey() == "tiefling" || c.SpeciesKey() == "elf" {
		return CHA
	}
	return INT
}

// SpellDC is 8 + proficiency + casting modifier (+1 with Innate Sorcery).
func SpellDC(c *Character) int {
	dc := 8 + ProficiencyBonus(c.Level) + c.Mod(c.CastAbility()) + c.boons.Spell
	if c.Conditions.Has(CondSorcery) {
		dc++
	}
	return dc
}

// SpellAttackParts is the spell attack modifier's parts.
func SpellAttackParts(c *Character) []RollPart {
	a := c.CastAbility()
	ps := []RollPart{{Label: string(a), Value: c.Mod(a)}, {Label: "Proficiency", Value: ProficiencyBonus(c.Level)}}
	if c.boons.Spell > 0 {
		ps = append(ps, RollPart{Label: "Magic focus", Value: c.boons.Spell})
	}
	return ps
}

// SaveMod is c's saving throw modifier for a: the ability plus
// proficiency in the class's two saves (any proficient ability on old
// sheets).
func (c *Character) SaveMod(a Ability) int {
	m := c.Mod(a) + c.boons.Saves
	if c.SaveProficient(a) {
		m += ProficiencyBonus(c.Level)
	}
	return m
}

// SaveProficient reports proficiency in a's saves.
func (c *Character) SaveProficient(a Ability) bool {
	if c.ClassKey == "" {
		return c.IsProficient(a)
	}
	for _, s := range ClassOf(c).Saves {
		if s == a {
			return true
		}
	}
	return false
}

// HitDieAvg is the HP a level after the first adds before CON: half the
// die plus one, as in 5e. Old sheets keep their 6.
func hitDieAvg(c *Character) int {
	if c.ClassKey == "" {
		return 6
	}
	return ClassOf(c).HitDie/2 + 1
}

// ClassStartingHP is the hit die's maximum plus CON, plus species and feat
// bonuses.
func ClassStartingHP(c *Character) int {
	return ClassOf(c).HitDie + c.Mod(CON) + hpBonusPerLevel(c)
}

// hpBonusPerLevel is extra max HP each level from Dwarven Toughness,
// Draconic Resilience (from level 3) and Tough.
func hpBonusPerLevel(c *Character) int {
	n := 0
	if c.SpeciesKey() == "dwarf" {
		n++
	}
	if c.HasFeat("tough") {
		n += 2
	}
	return n
}

// MaxHPFor computes the full max HP for c at their level from scratch.
func MaxHPFor(c *Character) int {
	if c.ClassKey == "" {
		return 0 // old sheets keep what they have
	}
	hp := ClassStartingHP(c)
	for l := 2; l <= c.Level; l++ {
		hp += max(1, hitDieAvg(c)+c.Mod(CON)) + hpBonusPerLevel(c)
	}
	if c.HasFeature("draconic_resilience") {
		hp += c.Level
	}
	return hp
}

// ClassChoices returns the class picks c should have made by now: key and
// total count.
func ClassChoices(c *Character) map[string]int {
	out := map[string]int{}
	for _, ch := range ClassOf(c).Choices {
		if c.Level >= ch.Level && ch.Count > out[ch.Key] {
			out[ch.Key] = ch.Count
		}
	}
	return out
}

// classLabel is "Level 3 Rogue (Thief)" in the setting.
func classLabel(c *Character, setting string) string {
	cd := ClassOf(c)
	s := fmt.Sprintf("Level %d %s", c.Level, ClassName(cd, setting))
	if c.Level >= 3 {
		s += " (" + SubclassName(cd, setting) + ")"
	}
	return s
}

// ClassLine is the class line for prompts and the sheet.
func ClassLine(c *Character, setting string) string { return classLabel(c, setting) }

func init() {
	for _, s := range Skills {
		allSkills = append(allSkills, s.Key)
	}
	for _, cd := range Classes {
		if len(cd.SkillChoice) == 0 {
			cd.SkillChoice = allSkills
		}
		for _, f := range cd.Features {
			if strings.TrimSpace(f.Name) == "" {
				panic("feature without a name in " + cd.Key)
			}
		}
	}
}
