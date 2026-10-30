package game

import (
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Era names for the rest of the rulebook: class features, species traits,
// feats, fighting styles, invocations, spell lists and schools. Like
// classes and spells, the rules underneath stay the same; only the words
// change. Order is gaslight, pulp, modern, future; an empty name keeps the
// fantasy one.
var TermEra = map[string]*EraNames{
	// class features
	"action_surge":          {"Second Breath", "Last-Ditch Push", "Adrenaline Spike", "Combat Overclock"},
	"adrenaline_rush":       {"Fighting Blood", "Hopped Up", "Adrenaline Rush", "Stim Rush"},
	"arcane_recovery":       {"Midnight Study", "Burning the Midnight Oil", "Power Nap", "Cache Recovery"},
	"bardic_inspiration":    {"Stirring Word", "Pep Talk", "Hype", "Morale Uplink"},
	"brave":                 {"Stout Heart", "Nerves of Steel", "Unflappable", "Fear Dampers"},
	"breath_weapon":         {"Dragon's Breath", "Serpent's Breath", "Bio-Spray", "Plasma Exhale"},
	"colossus_slayer":       {"Finish the Wounded", "Go for the Kill", "Finisher", "Kill Confirm"},
	"cunning_action":        {"Light Fingers", "Slippery", "Quick Moves", "Ghost Protocol"},
	"cutting_words":         {"Withering Wit", "Heckle", "Clapback", "Psych-Out"},
	"danger_sense":          {"Sixth Sense", "Gut Feeling", "Danger Sense", "Proximity Alarm"},
	"dark_ones_blessing":    {"Infernal Reward", "The Old Ones Feed You", "Dark Bargain", "Void Siphon"},
	"darkvision":            {"Night Eyes", "Cat's Eyes", "Night Eyes", "Low-Light Optics"},
	"disciple_of_life":      {"Healing Hands", "Mercy's Gift", "Bedside Manner", "Regen Protocols"},
	"divine_smite":          {"Righteous Blow", "Wrath of Heaven", "Haymaker of Justice", "Charged Strike"},
	"divine_spark":          {"Holy Spark", "Spark of Grace", "Second Chance", "Bio-Pulse"},
	"draconic_resilience":   {"Wyrm Hide", "Scaled Skin", "Hardened Body", "Psi-Shell"},
	"dwarven_resilience":    {"Iron Gut", "Iron Stomach", "Iron Gut", "Toxin Filters"},
	"dwarven_toughness":     {"Stubborn Constitution", "Hard as Iron", "Built Solid", "Reinforced Frame"},
	"elven_lineage":         {"Fey Blood", "Strange Heritage", "Old Bloodline", "Gene-Gift"},
	"expertise":             {"Specialist", "Ace", "Specialist", "Specialist Training"},
	"extra_attack":          {"Quick Hands", "One-Two", "Double Tap", "Combat Routines"},
	"fast_hands":            {"Sleight of Hand", "Quick Draw", "Fast Hands", "Quick-Release Rig"},
	"favored_enemy":         {"Quarry", "Man Hunter", "Target Package", "Bounty Lock"},
	"fey_ancestry":          {"Fey Ancestry", "Clear Head", "Strong-Willed", "Mind Shield"},
	"fiendish_legacy":       {"Hellfire Blood", "Brimstone Blood", "Hellspawn", "Pyro Gene"},
	"fighting_style":        {"Fighting Style", "Fighting Style", "Combat Training", "Combat Specialty"},
	"focus":                 {"Inner Discipline", "Chi", "Focus", "Kinetic Charge"},
	"font_of_inspiration":   {"Endless Encore", "Showstopper", "Viral", "Morale Surge"},
	"font_of_magic":         {"Wellspring", "Wellspring", "Power Reserve", "Psi Reservoir"},
	"frenzy":                {"Berserk", "Blind Fury", "Berserk", "Overdrive"},
	"gnomish_cunning":       {"Clever Mind", "Sharp Mind", "Big Brain", "Firewall Mind"},
	"improved_critical":     {"Telling Blow", "Lucky Punch", "Weak Spot", "Precision Targeting"},
	"innate_sorcery":        {"Trance", "Trance", "Zone In", "Psi Surge"},
	"invocations":           {"Infernal Secrets", "Forbidden Secrets", "Dark Gifts", "Void Mods"},
	"jack_of_all_trades":    {"Man of Parts", "Jack of All Trades", "Generalist", "Generalist"},
	"keen_senses":           {"Keen Senses", "Sharp Eyes", "Keen Senses", "Enhanced Senses"},
	"lands_aid":             {"Gift of the Wild", "Swamp's Mercy", "Spirit Aid", "Ecosystem Pulse"},
	"lay_on_hands":          {"Healing Touch", "Laying On of Hands", "First Aid", "Med-Gel"},
	"lore_skills":           {"Well Read", "Know a Guy", "Well Connected", "Data Mining"},
	"luck":                  {"Charmed Life", "Horseshoe Luck", "Lucky", "Probability Edge"},
	"magical_cunning":       {"Whispered Pact", "Midnight Rite", "Recharge", "Void Recharge"},
	"martial_arts":          {"Baritsu", "Kung Fu", "Martial Arts", "Kinetic Arts"},
	"natural_recovery":      {"Communion", "Bayou Rest", "Ground Yourself", "Biofeedback"},
	"open_hand_technique":   {"Throw", "Leg Sweep", "Takedown", "Zero-G Throw"},
	"otherworldly_presence": {"Otherworldly Presence", "Uncanny Air", "Creepy Vibe", "Void Aura"},
	"pact_magic":            {"Pact Rites", "Pact Rites", "Dark Hexes", "Void Channel"},
	"potent_cantrip":        {"Stubborn Charms", "Lingering Hex", "Splash Damage", "Overflow"},
	"powerful_build":        {"Big-Boned", "Built Like an Ox", "Heavyweight", "Heavy Frame"},
	"preserve_life":         {"Miracle", "Revival Meeting", "Mass Casualty Care", "Mass Regen"},
	"rage":                  {"Fury", "Seeing Red", "Rage", "Berserker Chem"},
	"reckless_attack":       {"Wild Swing", "Haymaker", "All In", "Berserk Protocol"},
	"relentless_endurance":  {"Refuse to Fall", "Too Mean to Die", "Shake It Off", "Failsafe"},
	"sacred_weapon":         {"Blessed Blade", "Holy Iron", "Righteous Weapon", "Charged Weapon"},
	"scholar":               {"Scholar", "Egghead", "Expert", "Data Specialist"},
	"sculpt_spells":         {"Careful Workings", "Careful Rites", "Precision Hacks", "Focused Beams"},
	"sear_undead":           {"Purging Light", "Holy Fire", "Burn Out", "Purge"},
	"second_wind":           {"Second Wind", "Shake It Off", "Second Wind", "Med-Stim"},
	"skillful":              {"Skillful", "Handy", "Skillful", "Skillful"},
	"sneak_attack":          {"Low Blow", "Dirty Fighting", "Sneak Attack", "Backstab Protocol"},
	"spellcasting":          {"Workings", "Rituals", "Hacks", "Tech"},
	"steady_aim":            {"Steady Hand", "Draw a Bead", "Steady Aim", "Targeting Assist"},
	"stones_endurance":      {"Stone Skin", "Iron Hide", "Tough Skin", "Dermal Plating"},
	"stunning_strike":       {"Nerve Strike", "Pressure Point", "Knockout Blow", "Neural Strike"},
	"turn_undead":           {"Rebuke the Dead", "Drive Out the Dead", "Banish", "Purge Signal"},
	"unarmored_defense_con": {"Thick Hide", "Hard Head", "Tough Body", "Hardened Body"},
	"unarmored_defense_wis": {"Unseen Guard", "Inner Guard", "Flow", "Kinetic Field"},
	"uncanny_dodge":         {"Roll with It", "Roll with the Punch", "Duck", "Reflex Boost"},
	"channel_divinity":      {"Holy Office", "Grace", "Faith", "Med Protocols"},
	"wild_shape":            {"Beast Shape", "Skinwalker", "Shapeshift", "Bio-Morph"},
	"versatile":             {"Versatile", "Jack of Trades", "Versatile", "Adaptive"},

	// feats and fighting styles
	"alert":                     {"Light Sleeper", "Eyes in the Back of Your Head", "Alert", "Threat Sensors"},
	"magic_initiate":            {"Dabbler in the Occult", "Occult Dabbler", "Dabbler", "Psi Dabbler"},
	"savage_attacker":           {"Savage Blows", "Brawler's Instinct", "Savage", "Brutal"},
	"skilled":                   {"Skilled", "Know-How", "Skilled", "Skilled"},
	"tough":                     {"Tough", "Tough as Nails", "Tough", "Tough"},
	"ability_score_improvement": {"Self-Improvement", "Self-Improvement", "Training", "Augmentation"},
	"grappler":                  {"Wrestler", "Strong-Arm", "Grappler", "Grappler"},
	"archery":                   {"Marksman", "Sharpshooter", "Marksman", "Marksman"},
	"defense":                   {"Guarded", "Keep Your Guard Up", "Defensive", "Defensive"},
	"great_weapon_fighting":     {"Heavy Weapons", "Big Swings", "Heavy Hitter", "Heavy Weapons"},
	"two_weapon_fighting":       {"Two-Handed Fighting", "Two-Fisted", "Dual Wield", "Dual Wield"},

	// invocations
	"agonizing_blast":   {"Searing Bolt", "Searing Bolt", "Searing Blast", "Overcharged Bolt"},
	"armor_of_shadows":  {"Cloak of Shadow", "Shroud", "Shadow Skin", "Void Skin"},
	"repelling_blast":   {"Repelling Bolt", "Hurling Bolt", "Knockback", "Kinetic Push"},
	"pact_of_the_blade": {"Bound Blade", "Cursed Knife", "Hexed Weapon", "Void Blade"},
	"devils_sight":      {"Devil's Sight", "Eyes of the Abyss", "Black Sight", "Void Sight"},
	"eldritch_mind":     {"Iron Will", "Unshakable Mind", "Locked In", "Neural Anchor"},

	// spell schools
	"abjuration":    {"Warding", "Warding", "Shielding", "Shielding"},
	"conjuration":   {"Summoning", "Summoning", "Summoning", "Fabrication"},
	"divination":    {"Scrying", "Scrying", "Intel", "Scanning"},
	"enchantment":   {"Mesmerism", "Mesmerism", "Persuasion", "Neural"},
	"evocation":     {"Galvanism", "Hexcraft", "Force", "Energy"},
	"illusion":      {"Phantasmagoria", "Trickery", "Deception", "Holo"},
	"necromancy":    {"Death Rites", "Death Rites", "Death", "Necrotech"},
	"transmutation": {"Alchemy", "Alchemy", "Alteration", "Nanotech"},
}

var plainWords = map[string]bool{"Defense": true, "Alert": true, "Tough": true, "Brave": true, "Luck": true, "Skilled": true}

// TermName is a rules term's name in setting: key is the feature, feat,
// list or school key, fantasy its name in the SRD.
func TermName(key, fantasy, setting string) string {
	return eraName(setting, fantasy, TermEra[key])
}

var termReskins sync.Map // setting -> *reskinner

type reskinner struct {
	re *regexp.Regexp
	to map[string]string
}

// Reskin rewrites rules text for setting: feature, feat, spell and class
// names in it become the era's. Fantasy text is returned as is.
func Reskin(text, setting string) string {
	if text == "" {
		return text
	}
	if _, ok := eraIndex[setting]; !ok {
		return text
	}
	v, ok := termReskins.Load(setting)
	if !ok {
		v, _ = termReskins.LoadOrStore(setting, newReskinner(setting))
	}
	r := v.(*reskinner)
	return r.re.ReplaceAllStringFunc(text, func(m string) string { return r.to[m] })
}

func newReskinner(setting string) *reskinner {
	to := map[string]string{}
	add := func(from, name string) {
		// Plain words ("Light", "Defense") start ordinary sentences too;
		// those are only renamed by key, never in running text.
		if !strings.ContainsAny(from, " '") && plainWords[from] {
			return
		}
		if from != "" && name != "" && from != name {
			to[from] = name
		}
	}
	for _, k := range ClassKeys() {
		cd := Classes[k]
		for _, f := range cd.Features {
			add(f.Name, TermName(f.Key, f.Name, setting))
		}
		add(cd.Name, ClassName(cd, setting))
		add(strings.ToLower(cd.Name), strings.ToLower(ClassName(cd, setting)))
		add(cd.Subclass.Name, SubclassName(cd, setting))
	}
	for _, k := range SpeciesKeys() {
		sp := Species[k]
		for _, f := range sp.Traits {
			add(f.Name, TermName(f.Key, f.Name, setting))
		}
	}
	for k, f := range Feats {
		add(f.Name, TermName(k, f.Name, setting))
	}
	for k, f := range Invocations {
		add(f.Name, TermName(k, f.Name, setting))
	}
	for _, s := range Spells {
		if strings.Contains(s.Name, " ") || s.Name == "Thaumaturgy" {
			add(s.Name, SpellName(s, setting))
		}
	}
	// resource and choice words that are not feature names
	add("Focus Points", TermName("focus", "Focus", setting)+" Points")
	add("Sorcery Points", TermName("font_of_magic", "Sorcery", setting)+" Points")
	add("Channel Divinity", TermName("channel_divinity", "Channel Divinity", setting))
	add("Eldritch Blast", SpellName(Spells["eldritch_blast"], setting))
	add("Elven lineage", TermName("elven_lineage", "Elven lineage", setting))
	add("College of Lore", SubclassName(Classes["bard"], setting))
	keys := make([]string, 0, len(to))
	for k := range to {
		keys = append(keys, regexp.QuoteMeta(k))
	}
	// longest first, so "Channel Divinity: Turn Undead" wins over "Channel Divinity"
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	return &reskinner{re: regexp.MustCompile(`\b(?:` + strings.Join(keys, "|") + `)\b`), to: to}
}

// ReskinFeature is f with its name and description in setting's words.
func ReskinFeature(f Feature, setting string) Feature {
	f.Name = TermName(f.Key, f.Name, setting)
	f.Desc = Reskin(f.Desc, setting)
	return f
}
