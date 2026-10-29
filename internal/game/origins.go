package game

// SpeciesDef is a 5e species (an origin or heritage in other eras).
type SpeciesDef struct {
	Key    string    `json:"key"`
	Name   string    `json:"name"`
	Desc   string    `json:"desc"`
	Traits []Feature `json:"traits"`
	Skills []string  `json:"skills,omitempty"` // granted skills
	Era    *EraNames `json:"-"`
	EraDsc *EraNames `json:"-"`
}

// Species are the nine SRD species.
var Species = map[string]*SpeciesDef{
	"human": {Key: "human", Name: "Human", Desc: "Adaptable and ambitious.",
		Era: &EraNames{"Human", "Human", "Human", "Baseline Human"},
		Traits: []Feature{
			{"skillful", "Skillful", 1, "One extra skill.", false},
			{"versatile", "Versatile", 1, "An extra origin feat.", false},
		}},
	"dwarf": {Key: "dwarf", Name: "Dwarf", Desc: "Stout, stubborn and hard to kill.",
		Era:    &EraNames{"Delver", "Stoneborn", "Stout", "Heavyworlder"},
		EraDsc: &EraNames{"Deep-mine folk, broad and tireless.", "An old bloodline of cavern folk.", "Built dense and tough as nails.", "Raised under crushing gravity."},
		Traits: []Feature{
			{"darkvision", "Darkvision", 1, "Sees in the dark.", false},
			{"dwarven_resilience", "Dwarven Resilience", 1, "Advantage on saves against poison.", false},
			{"dwarven_toughness", "Dwarven Toughness", 1, "+1 max HP per level.", false},
		}},
	"elf": {Key: "elf", Name: "Elf", Desc: "Long-lived, keen-eyed and touched by the fey.",
		Era:    &EraNames{"Fey-Blooded", "Old Blood", "Fae-Touched", "Eldari"},
		EraDsc: &EraNames{"A changeling family with fairy blood.", "Something ancient runs in your family.", "You were never quite like other kids.", "A long-lived offworld people."},
		Skills: []string{"perception"},
		Traits: []Feature{
			{"darkvision", "Darkvision", 1, "Sees in the dark.", false},
			{"fey_ancestry", "Fey Ancestry", 1, "Advantage on saves against being charmed or put to sleep.", false},
			{"keen_senses", "Keen Senses", 1, "Proficient in Perception.", false},
			{"elven_lineage", "Elven Lineage", 1, "A wizard cantrip of your choice, cast with Charisma.", false},
		}},
	"gnome": {Key: "gnome", Name: "Gnome", Desc: "Small, clever and hard to fool.",
		Era:    &EraNames{"Tinker-Kin", "Hill Folk", "Gnomish", "Synth"},
		EraDsc: &EraNames{"A tinkering little people.", "Small, clever folk who keep to the margins.", "Small, sharp, and impossible to con.", "A synthetic mind in a small chassis."},
		Traits: []Feature{
			{"darkvision", "Darkvision", 1, "Sees in the dark.", false},
			{"gnomish_cunning", "Gnomish Cunning", 1, "Advantage on INT, WIS and CHA saves.", false},
		}},
	"goliath": {Key: "goliath", Name: "Goliath", Desc: "Towering, with a giant's strength in the blood.",
		Era:    &EraNames{"Giant-Kin", "Titan-Blooded", "Giant-Blooded", "Colossus"},
		EraDsc: &EraNames{"Seven feet of circus strongman.", "Too big for any doorway in town.", "Built like a linebacker's linebacker.", "Engineered for heavy work."},
		Traits: []Feature{
			{"stones_endurance", "Stone's Endurance", 1, "When hit, reduce the damage by 1d12 + CON. Uses equal to your proficiency bonus per long rest.", false},
			{"powerful_build", "Powerful Build", 1, "Advantage on checks to break free of grapples.", false},
		}},
	"halfling": {Key: "halfling", Name: "Halfling", Desc: "Small, brave and absurdly lucky.",
		Era:    &EraNames{"Smallfolk", "Half-Pint", "Pocket-Sized", "Lowgrav-Born"},
		EraDsc: &EraNames{"Small, cheerful and lucky.", "Short, scrappy and born under a lucky star.", "Short, brave and unreasonably lucky.", "Slight-framed and born under thin gravity."},
		Traits: []Feature{
			{"brave", "Brave", 1, "Advantage on saves against being frightened.", false},
			{"luck", "Luck", 1, "Reroll a 1 on a d20 attack, check or save.", false},
		}},
	"dragonborn": {Key: "dragonborn", Name: "Dragonborn", Desc: "Scaled and proud, with a dragon's breath.",
		Era:    &EraNames{"Wyrm-Blooded", "Serpent-Born", "Dragon-Touched", "Draconid"},
		EraDsc: &EraNames{"Scaled skin and a furnace in the chest.", "The serpent's mark runs in your veins.", "A body that changed and a fire inside.", "A reptilian species with a breath weapon."},
		Traits: []Feature{
			{"darkvision", "Darkvision", 1, "Sees in the dark.", false},
			{"breath_weapon", "Breath Weapon", 1, "Action: foes in your zone make a DEX save (8 + CON + proficiency) or take 1d10 (2d10 at level 5), half on a save. Uses equal to your proficiency bonus per long rest.", false},
		}},
	"orc": {Key: "orc", Name: "Orc", Desc: "Powerful, tireless and hard to put down.",
		Era:    &EraNames{"Half-Wild", "Iron-Blooded", "Enhanced", "Gene-Forged"},
		EraDsc: &EraNames{"Strong as two men and twice as stubborn.", "Too stubborn to die.", "Something in you just won't quit.", "Bred for war and for surviving it."},
		Traits: []Feature{
			{"darkvision", "Darkvision", 1, "Sees in the dark.", false},
			{"adrenaline_rush", "Adrenaline Rush", 1, "Bonus action: an extra move and temporary HP equal to your proficiency bonus. Uses equal to your proficiency bonus, back on a short rest.", false},
			{"relentless_endurance", "Relentless Endurance", 1, "Once per long rest, drop to 1 HP instead of 0.", false},
		}},
	"tiefling": {Key: "tiefling", Name: "Tiefling", Desc: "Fiendish blood, horns and hellfire.",
		Era:    &EraNames{"Hellspawn", "Deep One Blood", "Demon-Touched", "Xeno-Hybrid"},
		EraDsc: &EraNames{"A family curse with horns.", "Something from the deep sea married into your line.", "Your eyes glow when you're angry.", "Part human, part something from beyond."},
		Traits: []Feature{
			{"darkvision", "Darkvision", 1, "Sees in the dark.", false},
			{"fiendish_legacy", "Fiendish Legacy", 1, "Knows Fire Bolt, cast with Charisma.", false},
			{"otherworldly_presence", "Otherworldly Presence", 1, "Knows Thaumaturgy.", false},
		}},
}

// SpeciesKeys lists species in display order.
func SpeciesKeys() []string {
	return []string{"human", "dwarf", "elf", "gnome", "goliath", "halfling", "dragonborn", "orc", "tiefling"}
}

// SpeciesName and SpeciesDesc are the species in setting.
func SpeciesName(sp *SpeciesDef, setting string) string { return eraName(setting, sp.Name, sp.Era) }
func SpeciesDesc(sp *SpeciesDef, setting string) string { return eraName(setting, sp.Desc, sp.EraDsc) }

// SpeciesKey is c's species; old sheets are human.
func (c *Character) SpeciesKey() string {
	if Species[c.Species] != nil {
		return c.Species
	}
	return "human"
}

// BackgroundDef is a background: three abilities to raise, two skills and
// an origin feat.
type BackgroundDef struct {
	Key       string    `json:"key"`
	Name      string    `json:"name"`
	Desc      string    `json:"desc"`
	Abilities []Ability `json:"abilities"`
	Skills    []string  `json:"skills"`
	Feat      string    `json:"feat"`
	FeatList  string    `json:"feat_list,omitempty"` // magic initiate's spell list
	Era       *EraNames `json:"-"`
}

// Backgrounds are the four SRD backgrounds and six more in the same shape.
var Backgrounds = map[string]*BackgroundDef{
	"acolyte":     {Key: "acolyte", Name: "Acolyte", Desc: "Raised in service to a temple.", Abilities: []Ability{INT, WIS, CHA}, Skills: []string{"insight", "religion"}, Feat: "magic_initiate", FeatList: "cleric", Era: &EraNames{"Seminarian", "Mission Worker", "Church Volunteer", "Temple Initiate"}},
	"criminal":    {Key: "criminal", Name: "Criminal", Desc: "You made a living outside the law.", Abilities: []Ability{DEX, CON, INT}, Skills: []string{"sleight_of_hand", "stealth"}, Feat: "alert", Era: &EraNames{"Pickpocket", "Bootlegger", "Ex-Con", "Black Marketeer"}},
	"sage":        {Key: "sage", Name: "Sage", Desc: "Years spent among books and scholars.", Abilities: []Ability{CON, INT, WIS}, Skills: []string{"arcana", "history"}, Feat: "magic_initiate", FeatList: "wizard", Era: &EraNames{"Natural Historian", "Professor", "Grad Student", "Archivist"}},
	"soldier":     {Key: "soldier", Name: "Soldier", Desc: "Drilled, deployed and battle-tested.", Abilities: []Ability{STR, DEX, CON}, Skills: []string{"athletics", "intimidation"}, Feat: "savage_attacker", Era: &EraNames{"Cavalryman", "Doughboy", "Veteran", "Ex-Military"}},
	"entertainer": {Key: "entertainer", Name: "Entertainer", Desc: "You lived for an audience.", Abilities: []Ability{STR, DEX, CHA}, Skills: []string{"acrobatics", "performance"}, Feat: "skilled", Era: &EraNames{"Music Hall Act", "Vaudevillian", "Streamer", "Holo-Star"}},
	"noble":       {Key: "noble", Name: "Noble", Desc: "Born to wealth, title and expectation.", Abilities: []Ability{STR, INT, CHA}, Skills: []string{"history", "persuasion"}, Feat: "skilled", Era: &EraNames{"Aristocrat", "Old Money", "Heir", "Corporate Scion"}},
	"sailor":      {Key: "sailor", Name: "Sailor", Desc: "Years before the mast.", Abilities: []Ability{STR, DEX, WIS}, Skills: []string{"acrobatics", "perception"}, Feat: "tough", Era: &EraNames{"Deckhand", "Merchant Mariner", "Coast Guard", "Spacer"}},
	"hermit":      {Key: "hermit", Name: "Hermit", Desc: "You lived apart and found something there.", Abilities: []Ability{CON, WIS, CHA}, Skills: []string{"medicine", "religion"}, Feat: "magic_initiate", FeatList: "druid", Era: &EraNames{"Recluse", "Hermit", "Off-Gridder", "Asteroid Prospector"}},
	"guard":       {Key: "guard", Name: "Guard", Desc: "You kept watch while others slept.", Abilities: []Ability{STR, INT, WIS}, Skills: []string{"athletics", "perception"}, Feat: "alert", Era: &EraNames{"Constable", "Beat Cop", "Security Guard", "Station Security"}},
	"farmer":      {Key: "farmer", Name: "Farmer", Desc: "Hard work and long seasons.", Abilities: []Ability{STR, CON, WIS}, Skills: []string{"animal_handling", "nature"}, Feat: "tough", Era: &EraNames{"Homesteader", "Dust Bowl Farmer", "Rancher", "Hydroponics Tech"}},
}

// BackgroundKeys lists backgrounds in display order.
func BackgroundKeys() []string {
	return []string{"acolyte", "criminal", "sage", "soldier", "entertainer", "noble", "sailor", "hermit", "guard", "farmer"}
}

// BackgroundName is the background in setting.
func BackgroundName(b *BackgroundDef, setting string) string { return eraName(setting, b.Name, b.Era) }

// FeatDef is a feat. Origin feats come from backgrounds (and humans);
// general feats are taken at level 4.
type FeatDef struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Desc   string `json:"desc"`
	Kind   string `json:"kind"` // origin | general | style
	Repeat bool   `json:"-"`
}

// Feats lists the SRD origin and general feats the engine applies, plus
// the fighting styles.
var Feats = map[string]*FeatDef{
	"alert":           {Key: "alert", Name: "Alert", Kind: "origin", Desc: "You can't be caught by surprise while conscious."},
	"magic_initiate":  {Key: "magic_initiate", Name: "Magic Initiate", Kind: "origin", Desc: "Two cantrips and a 1st-level spell (once per long rest without a slot) from the cleric, druid or wizard list."},
	"savage_attacker": {Key: "savage_attacker", Name: "Savage Attacker", Kind: "origin", Desc: "Once a turn, roll weapon damage twice and keep the higher."},
	"skilled":         {Key: "skilled", Name: "Skilled", Kind: "origin", Desc: "Three extra skills."},
	"tough":           {Key: "tough", Name: "Tough", Kind: "origin", Desc: "+2 max HP per level."},
	"ability_score_improvement": {Key: "ability_score_improvement", Name: "Ability Score Improvement", Kind: "general", Repeat: true,
		Desc: "+2 to one ability or +1 to two (max 20)."},
	"grappler":              {Key: "grappler", Name: "Grappler", Kind: "general", Desc: "Advantage on attacks against a foe you are grappling, and +1 STR."},
	"archery":               {Key: "archery", Name: "Archery", Kind: "style", Desc: "+2 to hit with ranged weapons."},
	"defense":               {Key: "defense", Name: "Defense", Kind: "style", Desc: "+1 AC while wearing armor."},
	"great_weapon_fighting": {Key: "great_weapon_fighting", Name: "Great Weapon Fighting", Kind: "style", Desc: "Treat 1s and 2s on two-handed weapon damage dice as 3s."},
	"two_weapon_fighting":   {Key: "two_weapon_fighting", Name: "Two-Weapon Fighting", Kind: "style", Desc: "Add your ability modifier to your off-hand attack's damage."},
}

// OriginFeats and GeneralFeats list feat keys a builder may offer.
func OriginFeats() []string {
	return []string{"alert", "magic_initiate", "savage_attacker", "skilled", "tough"}
}
func GeneralFeats() []string {
	return []string{"ability_score_improvement", "grappler", "alert", "magic_initiate", "savage_attacker", "skilled", "tough"}
}
func FightingStyles() []string {
	return []string{"archery", "defense", "great_weapon_fighting", "two_weapon_fighting"}
}

// Invocations are the warlock's eldritch invocations the engine applies.
var Invocations = map[string]Feature{
	"agonizing_blast":   {Key: "agonizing_blast", Name: "Agonizing Blast", Desc: "Add CHA to Eldritch Blast damage."},
	"armor_of_shadows":  {Key: "armor_of_shadows", Name: "Armor of Shadows", Desc: "Without armor your AC is 13 + DEX."},
	"repelling_blast":   {Key: "repelling_blast", Name: "Repelling Blast", Desc: "Eldritch Blast hits push the foe a zone away."},
	"pact_of_the_blade": {Key: "pact_of_the_blade", Name: "Pact of the Blade", Desc: "Your weapon attacks use CHA."},
	"devils_sight":      {Key: "devils_sight", Name: "Devil's Sight", Desc: "You see in magical and normal darkness."},
	"eldritch_mind":     {Key: "eldritch_mind", Name: "Eldritch Mind", Desc: "Advantage on saves to keep concentration."},
}

// InvocationKeys lists invocations in display order.
func InvocationKeys() []string {
	return []string{"agonizing_blast", "armor_of_shadows", "repelling_blast", "pact_of_the_blade", "devils_sight", "eldritch_mind"}
}

// FeatList is c's feats: the background's, a human's extra one, and any
// taken at level 4.
func (c *Character) FeatList() []string {
	var out []string
	if b := Backgrounds[c.Background]; b != nil {
		out = append(out, b.Feat)
	}
	out = append(out, c.Picks["versatile"]...)
	out = append(out, c.Picks["feat4"]...)
	return out
}

// HasFeat reports whether c has feat key.
func (c *Character) HasFeat(key string) bool {
	for _, f := range c.FeatList() {
		if f == key {
			return true
		}
	}
	return false
}

// HasPick reports whether any of c's picks is key (fighting styles,
// invocations).
func (c *Character) HasPick(key string) bool {
	for k, v := range c.Picks {
		if k == "asi" {
			continue
		}
		for _, p := range v {
			if p == key {
				return true
			}
		}
	}
	return false
}
