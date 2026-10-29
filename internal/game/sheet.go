package game

import (
	"fmt"
	"sort"
	"strings"
)

// Choice is one build pick a character has to make: which feat, which
// skills to master, which fighting style.
type Choice struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Count   int      `json:"count"`
	Options []string `json:"options"`
	// Repeat allows the same option twice (ability score increases).
	Repeat bool `json:"repeat,omitempty"`
	// Kind says what the options are: skill, feat, style, invocation,
	// spell, list or ability.
	Kind string `json:"kind"`
}

// ChoicesFor lists every pick c should have made by their level, with
// the options open to them.
func ChoicesFor(c *Character) []Choice {
	var out []Choice
	known := map[string]bool{}
	for _, s := range c.SkillList() {
		known[s] = true
	}
	notKnown := func(exclude string) []string {
		var o []string
		for _, s := range allSkills {
			if !known[s] || containsStr(c.Picks[exclude], s) {
				o = append(o, s)
			}
		}
		return o
	}
	cc := ClassChoices(c)
	for _, key := range []string{"fighting_style", "expertise", "invocation", "scholar"} {
		n := cc[key]
		if n == 0 {
			continue
		}
		switch key {
		case "fighting_style":
			out = append(out, Choice{Key: key, Label: "Fighting style", Count: n, Options: FightingStyles(), Kind: "style"})
		case "expertise":
			out = append(out, Choice{Key: key, Label: "Expertise (double proficiency)", Count: n, Options: c.SkillList(), Kind: "skill"})
		case "invocation":
			out = append(out, Choice{Key: key, Label: "Eldritch invocations", Count: n, Options: InvocationKeys(), Kind: "invocation"})
		case "scholar":
			var o []string
			for _, s := range []string{"arcana", "history", "investigation", "medicine", "nature", "religion"} {
				if known[s] {
					o = append(o, s)
				}
			}
			if len(o) > 0 {
				out = append(out, Choice{Key: key, Label: "Scholar (expertise)", Count: 1, Options: o, Kind: "skill"})
			}
		}
	}
	if c.HasFeature("lore_skills") {
		out = append(out, Choice{Key: "lore_skills", Label: "College of Lore skills", Count: 3, Options: notKnown("lore_skills"), Kind: "skill"})
	}
	switch c.SpeciesKey() {
	case "human":
		out = append(out, Choice{Key: "skillful", Label: "Skillful (one skill)", Count: 1, Options: notKnown("skillful"), Kind: "skill"})
		out = append(out, Choice{Key: "versatile", Label: "Versatile (origin feat)", Count: 1, Options: OriginFeats(), Kind: "feat"})
	case "elf":
		out = append(out, Choice{Key: "elf_cantrip", Label: "Elven lineage cantrip", Count: 1, Options: cantripsOf("wizard"), Kind: "spell"})
	}
	if c.Level >= 4 {
		out = append(out, Choice{Key: "feat4", Label: "Level 4 feat", Count: 1, Options: GeneralFeats(), Kind: "feat"})
		if containsStr(c.Picks["feat4"], "ability_score_improvement") {
			out = append(out, Choice{Key: "asi", Label: "Ability score increase (+1 twice)", Count: 2, Options: abilityKeys(), Repeat: true, Kind: "ability"})
		}
	}
	if c.HasFeat("skilled") {
		out = append(out, Choice{Key: "skilled", Label: "Skilled (three skills)", Count: 3, Options: notKnown("skilled"), Kind: "skill"})
	}
	if c.HasFeat("magic_initiate") {
		list := miList(c)
		if list == "" {
			out = append(out, Choice{Key: "magic_initiate", Label: "Magic Initiate spell list", Count: 1, Options: []string{"cleric", "druid", "wizard"}, Kind: "list"})
		} else {
			out = append(out, Choice{Key: "mi_cantrips", Label: "Magic Initiate cantrips", Count: 2, Options: cantripsOf(list), Kind: "spell"})
			out = append(out, Choice{Key: "mi_spell", Label: "Magic Initiate 1st-level spell", Count: 1, Options: levelSpellsOf(list, 1), Kind: "spell"})
		}
	}
	return out
}

// miList is the magic initiate list: the background's, or the pick.
func miList(c *Character) string {
	if b := Backgrounds[c.Background]; b != nil && b.Feat == "magic_initiate" {
		return b.FeatList
	}
	if p := c.Picks["magic_initiate"]; len(p) > 0 {
		return p[0]
	}
	return ""
}

func cantripsOf(class string) []string { return levelSpellsOf(class, 0) }

func levelSpellsOf(class string, level int) []string {
	var out []string
	for _, k := range ClassSpells(class, level) {
		if Spells[k].Level == level {
			out = append(out, k)
		}
	}
	return out
}

func abilityKeys() []string {
	out := make([]string, len(Abilities))
	for i, a := range Abilities {
		out[i] = string(a)
	}
	return out
}

func containsStr(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// PendingChoices are the picks c still has to make.
func PendingChoices(c *Character) []Choice {
	var out []Choice
	for _, ch := range ChoicesFor(c) {
		if len(c.Picks[ch.Key]) < ch.Count {
			out = append(out, ch)
		}
	}
	return out
}

// checkPick validates picks for one choice.
func checkPick(ch Choice, picks []string) Problems {
	var p Problems
	if len(picks) > ch.Count {
		p = append(p, fmt.Sprintf("%s: pick at most %d", ch.Label, ch.Count))
	}
	seen := map[string]bool{}
	for _, v := range picks {
		if !containsStr(ch.Options, v) {
			p = append(p, fmt.Sprintf("%s: %q is not an option", ch.Label, v))
		}
		if seen[v] && !ch.Repeat {
			p = append(p, fmt.Sprintf("%s: %q picked twice", ch.Label, v))
		}
		seen[v] = true
	}
	return p
}

// Choose records picks for choice key on c, applying what they change
// (ability scores, max HP). Picks already made can't be changed once the
// campaign is under way, except by the builder.
func Choose(c *Character, key string, picks []string) Problems {
	var ch *Choice
	for _, x := range ChoicesFor(c) {
		if x.Key == key {
			x := x
			ch = &x
		}
	}
	if ch == nil {
		return Problems{fmt.Sprintf("%s has no %q choice", c.Name, key)}
	}
	if p := checkPick(*ch, picks); len(p) > 0 {
		return p
	}
	// earlier picks stay; a choice that grew with the level only adds
	for _, o := range c.Picks[key] {
		if !containsStr(picks, o) || len(picks) == len(c.Picks[key]) {
			return Problems{ch.Label + " is already chosen"}
		}
	}
	if c.Picks == nil {
		c.Picks = map[string][]string{}
	}
	c.Picks[key] = append([]string(nil), picks...)
	if key == "asi" {
		for _, a := range picks {
			ab := Ability(a)
			c.Scores[ab] = min(20, c.Scores[ab]+1)
		}
	}
	if key == "feat4" && containsStr(picks, "grappler") {
		if c.Mod(DEX) > c.Mod(STR) {
			c.Scores[DEX] = min(20, c.Scores[DEX]+1)
		} else {
			c.Scores[STR] = min(20, c.Scores[STR]+1)
		}
	}
	syncMaxHP(c)
	return nil
}

// syncMaxHP recomputes a class sheet's max HP and keeps the damage taken.
func syncMaxHP(c *Character) {
	if c.ClassKey == "" {
		return
	}
	nm := MaxHPFor(c)
	gain := nm - c.MaxHP
	c.MaxHP = nm
	if gain > 0 && !c.Down() {
		c.HP = min(c.MaxHP, c.HP+gain)
	}
	c.HP = min(c.HP, c.MaxHP)
}

// buildFrom makes an unsaved level 1 character from a class spec, for
// validation and creation.
func buildFrom(spec CharacterSpec) *Character {
	c := &Character{Name: strings.TrimSpace(spec.Name), Level: 1}
	c.ClassKey, c.Species, c.Background = spec.ClassKey, spec.Species, spec.Background
	c.Picks = map[string][]string{}
	for k, v := range spec.Picks {
		if len(v) > 0 {
			c.Picks[k] = append([]string(nil), v...)
		}
	}
	return c
}

// PreviewBuild is the unsaved character a spec makes, for the builder.
func PreviewBuild(spec CharacterSpec, scores Scores) *Character {
	c := buildFrom(spec)
	c.Skills = buildSkills(spec)
	if scores == nil {
		scores, _ = standardScores(spec.Scores)
	}
	if scores == nil {
		scores = Scores{}
		for _, a := range Abilities {
			scores[a] = 10
		}
	}
	c.Scores = scores
	c.Spells = spec.Spells
	return c
}

// ValidateBuild checks a class spec: class, species and background exist,
// the standard array, background ability boosts, class skills, picks and
// spells. It returns the final scores.
func ValidateBuild(spec CharacterSpec) (Scores, Problems) {
	var p Problems
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		p = append(p, "name is required")
	} else if len(name) > 40 {
		p = append(p, "name must be at most 40 characters")
	}
	cd := Classes[spec.ClassKey]
	if cd == nil {
		return nil, append(p, "class must be one of "+strings.Join(ClassKeys(), ", "))
	}
	if Species[spec.Species] == nil {
		p = append(p, "species must be one of "+strings.Join(SpeciesKeys(), ", "))
	}
	bg := Backgrounds[spec.Background]
	if bg == nil {
		return nil, append(p, "background must be one of "+strings.Join(BackgroundKeys(), ", "))
	}
	scores, sp := standardScores(spec.Scores)
	p = append(p, sp...)
	// background boosts: +2/+1 or +1/+1/+1 among its three abilities
	total, vals := 0, []int{}
	for k, v := range spec.Boost {
		a, ok := ParseAbility(k)
		if !ok || !containsAbility(bg.Abilities, a) {
			p = append(p, fmt.Sprintf("%s can only raise %s", bg.Name, abilityList(bg.Abilities)))
			continue
		}
		if v < 0 || v > 2 {
			p = append(p, "a background raises an ability by 1 or 2")
			continue
		}
		if v > 0 {
			total += v
			vals = append(vals, v)
		}
		if scores != nil {
			scores[a] += v
		}
	}
	sort.Ints(vals)
	if total != 3 || (len(vals) == 3 && vals[0] != 1) || (len(vals) == 2 && vals[1] != 2) {
		p = append(p, "background boosts must be +2 and +1, or +1 to all three")
	}
	// class skills
	fixed := append(append([]string{}, bg.Skills...), speciesSkills(spec.Species)...)
	n := 0
	seen := map[string]bool{}
	for _, s := range spec.Skills {
		k, ok := SkillByKey(s)
		if !ok {
			p = append(p, fmt.Sprintf("unknown skill %q", s))
			continue
		}
		if seen[k.Key] || containsStr(fixed, k.Key) {
			continue
		}
		seen[k.Key] = true
		if !containsStr(cd.SkillChoice, k.Key) {
			p = append(p, fmt.Sprintf("%s is not a %s skill", k.Name, cd.Name))
			continue
		}
		n++
	}
	if n != cd.SkillCount {
		p = append(p, fmt.Sprintf("pick %d %s skills (not counting your background's)", cd.SkillCount, cd.Name))
	}
	c := buildFrom(spec)
	c.Skills = buildSkills(spec)
	if scores != nil {
		c.Scores = scores
	}
	for _, ch := range ChoicesFor(c) {
		p = append(p, checkPick(ch, c.Picks[ch.Key])...)
		if len(c.Picks[ch.Key]) < ch.Count {
			p = append(p, fmt.Sprintf("%s: pick %d", ch.Label, ch.Count))
		}
	}
	for k := range c.Picks {
		found := false
		for _, ch := range ChoicesFor(c) {
			found = found || ch.Key == k
		}
		if !found {
			p = append(p, fmt.Sprintf("no %q choice at level 1", k))
		}
	}
	if len(spec.Spells) > 0 {
		p = append(p, ValidateSpells(c, spec.Spells)...)
	}
	return scores, p
}

// standardScores parses scores that must use the standard array.
func standardScores(in map[string]int) (Scores, Problems) {
	var p Problems
	scores := Scores{}
	for k, v := range in {
		a, ok := ParseAbility(k)
		if !ok {
			p = append(p, fmt.Sprintf("unknown ability %q", k))
			continue
		}
		scores[a] = v
	}
	var got []int
	for _, a := range Abilities {
		v, ok := scores[a]
		if !ok {
			p = append(p, fmt.Sprintf("missing score for %s", a))
			continue
		}
		got = append(got, v)
	}
	if len(got) == 6 {
		sorted := append([]int(nil), got...)
		sort.Sort(sort.Reverse(sort.IntSlice(sorted)))
		for i, v := range StandardArray {
			if sorted[i] != v {
				p = append(p, "scores must use the standard array 15, 14, 13, 12, 10, 8, each exactly once")
				break
			}
		}
	}
	if len(p) > 0 {
		return nil, p
	}
	return scores, nil
}

func containsAbility(xs []Ability, a Ability) bool {
	for _, x := range xs {
		if x == a {
			return true
		}
	}
	return false
}

func abilityList(xs []Ability) string {
	var s []string
	for _, a := range xs {
		s = append(s, string(a))
	}
	return strings.Join(s, ", ")
}

func speciesSkills(key string) []string {
	if sp := Species[key]; sp != nil {
		return sp.Skills
	}
	return nil
}

// buildSkills is the class picks plus background and species skills.
func buildSkills(spec CharacterSpec) []string {
	var out []string
	add := func(ks ...string) {
		for _, s := range ks {
			if k, ok := SkillByKey(s); ok && !containsStr(out, k.Key) {
				out = append(out, k.Key)
			}
		}
	}
	if bg := Backgrounds[spec.Background]; bg != nil {
		add(bg.Skills...)
	}
	add(speciesSkills(spec.Species)...)
	add(spec.Skills...)
	return out
}

// createBuild makes a class character from a validated spec.
func createBuild(w *World, playerID string, spec CharacterSpec, scores Scores) *Character {
	cd := Classes[spec.ClassKey]
	setting := w.Setting()
	c := buildFrom(spec)
	c.ID, c.PlayerID = w.NewID("char"), playerID
	c.Concept, c.Backstory, c.Pronouns = trimTo(spec.Concept, 200), trimTo(spec.Backstory, 600), NormPronouns(spec.Pronouns)
	c.Scores, c.Skills, c.Kit, c.Gold = scores, buildSkills(spec), cd.Kit, cd.Gold
	class := strings.TrimSpace(spec.Class)
	if class == "" || strings.EqualFold(class, cd.Name) {
		class = ClassName(cd, setting)
	}
	c.Class = trimTo(class, 40)
	c.Spells = spec.Spells
	if len(c.Spells) == 0 {
		c.Spells = DefaultSpells(c)
	}
	c.MaxHP = MaxHPFor(c)
	c.HP = c.MaxHP
	owner := Owner{Kind: OwnerCharacter, ID: c.ID}
	for _, key := range cd.Gear {
		it := NewItem(w, key, owner)
		if it == nil {
			continue
		}
		if key == cd.Weapon && c.Weapon == "" {
			c.Weapon = it.ID
		}
		if key == cd.Armor && c.Armor == "" {
			c.Armor = it.ID
		}
	}
	return c
}

// FillBuild completes a drafted class spec with sensible defaults so it
// validates: a known class, species and background, the class's
// suggested array, boosts, skills, picks and spells.
func FillBuild(spec CharacterSpec) CharacterSpec {
	if Classes[spec.ClassKey] == nil {
		spec.ClassKey = "fighter"
		if k := kitClass[spec.Kit]; k != "" {
			spec.ClassKey = k
		}
	}
	cd := Classes[spec.ClassKey]
	if Species[spec.Species] == nil {
		spec.Species = "human"
	}
	if Backgrounds[spec.Background] == nil {
		spec.Background = classBackground[cd.Key]
	}
	bg := Backgrounds[spec.Background]
	if _, p := standardScores(spec.Scores); len(p) > 0 {
		spec.Scores = map[string]int{}
		for i, a := range Abilities {
			spec.Scores[string(a)] = cd.Scores[i]
		}
	}
	// boosts: +2 to the background ability that helps the class most
	if _, p := ValidateBuild(CharacterSpec{Name: "x", ClassKey: spec.ClassKey, Species: spec.Species, Background: spec.Background, Scores: spec.Scores, Boost: spec.Boost, Skills: spec.Skills}); hasProblem(p, "boost") {
		spec.Boost = defaultBoost(cd, bg, spec.Scores)
	}
	// skills
	fixed := append(append([]string{}, bg.Skills...), speciesSkills(spec.Species)...)
	var skills []string
	for _, s := range spec.Skills {
		if k, ok := SkillByKey(s); ok && containsStr(cd.SkillChoice, k.Key) && !containsStr(fixed, k.Key) && !containsStr(skills, k.Key) && len(skills) < cd.SkillCount {
			skills = append(skills, k.Key)
		}
	}
	for _, s := range classSkillPrefs(cd) {
		if len(skills) >= cd.SkillCount {
			break
		}
		if !containsStr(fixed, s) && !containsStr(skills, s) {
			skills = append(skills, s)
		}
	}
	spec.Skills = skills
	// picks: keep valid ones, fill the rest
	c := buildFrom(spec)
	c.Skills = buildSkills(spec)
	for i := 0; i < 3; i++ { // feats can open more choices
		for _, ch := range ChoicesFor(c) {
			if len(checkPick(ch, c.Picks[ch.Key])) > 0 {
				delete(c.Picks, ch.Key)
			}
			if len(c.Picks[ch.Key]) < ch.Count {
				c.Picks[ch.Key] = defaultPicks(c, ch)
			}
		}
	}
	spec.Picks = c.Picks
	if len(spec.Spells) > 0 && len(ValidateSpells(c, spec.Spells)) > 0 {
		spec.Spells = nil
	}
	if len(spec.Spells) == 0 {
		spec.Spells = DefaultSpells(c)
	}
	spec.Proficient = nil
	return spec
}

func hasProblem(p Problems, word string) bool {
	for _, s := range p {
		if strings.Contains(s, word) {
			return true
		}
	}
	return false
}

// classBackground is a fitting default background for each class.
var classBackground = map[string]string{
	"barbarian": "farmer", "bard": "entertainer", "cleric": "acolyte", "druid": "hermit", "fighter": "soldier", "monk": "hermit",
	"paladin": "noble", "ranger": "guard", "rogue": "criminal", "sorcerer": "noble", "warlock": "sage", "wizard": "sage",
}

func defaultBoost(cd *ClassDef, bg *BackgroundDef, scores map[string]int) map[string]int {
	best := bg.Abilities[0]
	for _, a := range bg.Abilities {
		if containsAbility(cd.Primary, a) {
			best = a
			break
		}
	}
	second := bg.Abilities[0]
	for _, a := range bg.Abilities {
		if a != best && (second == best || scores[string(a)] > scores[string(second)] || a == CON) {
			second = a
		}
	}
	if second == best {
		second = bg.Abilities[1]
	}
	return map[string]int{string(best): 2, string(second): 1}
}

// classSkillPrefs orders a class's skill list with the classic picks first.
func classSkillPrefs(cd *ClassDef) []string {
	prefs := map[string][]string{
		"barbarian": {"athletics", "perception", "survival", "intimidation"},
		"bard":      {"persuasion", "deception", "performance", "insight", "perception"},
		"cleric":    {"insight", "medicine", "religion", "persuasion"},
		"druid":     {"nature", "perception", "survival", "medicine", "animal_handling"},
		"fighter":   {"athletics", "perception", "intimidation", "survival"},
		"monk":      {"acrobatics", "stealth", "insight", "athletics"},
		"paladin":   {"athletics", "persuasion", "insight", "intimidation"},
		"ranger":    {"perception", "stealth", "survival", "nature", "athletics"},
		"rogue":     {"stealth", "perception", "sleight_of_hand", "deception", "acrobatics", "investigation"},
		"sorcerer":  {"arcana", "persuasion", "deception", "insight"},
		"warlock":   {"arcana", "deception", "intimidation", "investigation"},
		"wizard":    {"arcana", "investigation", "history", "insight"},
	}
	return append(prefs[cd.Key], cd.SkillChoice...)
}

// defaultPicks fills a choice for drafts and old sheets.
func defaultPicks(c *Character, ch Choice) []string {
	pref := map[string][]string{
		"fighting_style": {map[bool]string{true: "archery", false: "defense"}[ClassOf(c).Key == "ranger"], "great_weapon_fighting"},
		"invocation":     {"agonizing_blast", "armor_of_shadows", "repelling_blast", "pact_of_the_blade", "eldritch_mind"},
		"expertise":      {"stealth", "perception", "persuasion", "deception", "sleight_of_hand"},
		"versatile":      {"tough", "alert", "skilled"},
		"feat4":          {"ability_score_improvement"},
		"magic_initiate": {"wizard"},
		"mi_cantrips":    {"fire_bolt", "guidance", "sacred_flame", "produce_flame", "mage_hand"},
		"mi_spell":       {"cure_wounds", "magic_missile", "healing_word", "bless"},
		"elf_cantrip":    {"fire_bolt", "mage_hand"},
	}[ch.Key]
	if ch.Key == "asi" {
		a := string(BestAbility(c))
		return []string{a, a}
	}
	out := append([]string(nil), c.Picks[ch.Key]...)
	for _, o := range append(pref, ch.Options...) {
		if len(out) >= ch.Count {
			break
		}
		if containsStr(ch.Options, o) && !containsStr(out, o) {
			out = append(out, o)
		}
	}
	return out
}

// Resource is a limited-use feature on the sheet.
type Resource struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Max      int    `json:"max"`
	Used     int    `json:"used"`
	Recharge string `json:"recharge"` // short | long
}

// Resources lists c's limited features with their uses.
func Resources(c *Character) []Resource {
	var out []Resource
	add := func(key, name string, mx int, recharge string) {
		if mx > 0 {
			out = append(out, Resource{Key: key, Name: name, Max: mx, Used: min(mx, c.Used[key]), Recharge: recharge})
		}
	}
	pb := ProficiencyBonus(c.Level)
	if c.HasFeature("rage") {
		add("rage", "Rage", map[bool]int{true: 3, false: 2}[c.Level >= 3], "long")
	}
	if c.HasFeature("bardic_inspiration") {
		add("bardic_inspiration", "Bardic Inspiration", max(1, c.Mod(CHA)), map[bool]string{true: "short", false: "long"}[c.Level >= 5])
	}
	if c.HasFeature("divine_spark") || c.HasFeature("sacred_weapon") {
		add("channel_divinity", "Channel Divinity", 2, "short")
	}
	if c.HasFeature("wild_shape") {
		add("wild_shape", "Wild Shape", 2, "short")
	}
	if c.HasFeature("second_wind") {
		add("second_wind", "Second Wind", map[bool]int{true: 3, false: 2}[c.Level >= 4], "short")
	}
	if c.HasFeature("action_surge") {
		add("action_surge", "Action Surge", 1, "short")
	}
	if c.HasFeature("focus") {
		add("focus", "Focus Points", c.Level, "short")
	}
	if c.HasFeature("lay_on_hands") {
		add("lay_on_hands", "Lay on Hands", 5*c.Level, "long")
	}
	if c.HasFeature("favored_enemy") {
		add("favored_enemy", "Free Hunter's Mark", 2, "long")
	}
	if c.HasFeature("innate_sorcery") {
		add("innate_sorcery", "Innate Sorcery", 2, "long")
	}
	if c.HasFeature("font_of_magic") {
		add("sorcery_points", "Sorcery Points", c.Level, "long")
	}
	if c.HasFeature("magical_cunning") {
		add("magical_cunning", "Magical Cunning", 1, "long")
	}
	if c.HasFeature("breath_weapon") {
		add("breath_weapon", "Breath Weapon", pb, "long")
	}
	if c.HasFeature("stones_endurance") {
		add("stones_endurance", "Stone's Endurance", pb, "long")
	}
	if c.HasFeature("adrenaline_rush") {
		add("adrenaline_rush", "Adrenaline Rush", pb, "short")
	}
	if c.HasFeature("relentless_endurance") {
		add("relentless_endurance", "Relentless Endurance", 1, "long")
	}
	if len(c.Picks["mi_spell"]) > 0 {
		add("mi_spell", "Free "+Spells[c.Picks["mi_spell"][0]].Name, 1, "long")
	}
	return out
}

// resourceLeft is the remaining uses of key.
func resourceLeft(c *Character, key string) int {
	for _, r := range Resources(c) {
		if r.Key == key {
			return r.Max - r.Used
		}
	}
	return 0
}

// spend uses n of resource key, if c has them.
func spend(c *Character, key string, n int) bool {
	if resourceLeft(c, key) < n {
		return false
	}
	if c.Used == nil {
		c.Used = map[string]int{}
	}
	c.Used[key] += n
	return true
}

// onceThisTurn reports whether c can still use a once-a-turn feature, and
// marks it used.
func onceThisTurn(w *World, c *Character, key string) bool {
	t := w.Campaign.Turn + 1 // 0 means never
	if c.Once[key] == t {
		return false
	}
	if c.Once == nil {
		c.Once = map[string]int{}
	}
	c.Once[key] = t
	return true
}

// LongRest restores every use, slot and hit point.
func LongRest(c *Character) {
	c.Used, c.SlotsUsed, c.TempHP, c.Conc = nil, nil, 0, nil
}

// ShortRest restores short-rest features: pact slots, focus, action
// surge, and one use of rage, second wind, channel divinity and wild
// shape. Wizards (and land druids) recover slots once a day.
func ShortRest(c *Character) []string {
	var lines []string
	for _, r := range Resources(c) {
		if r.Used == 0 {
			continue
		}
		switch r.Key {
		case "focus", "action_surge", "bardic_inspiration", "adrenaline_rush":
			if r.Recharge == "short" {
				c.Used[r.Key] = 0
			}
		case "rage", "second_wind", "channel_divinity", "wild_shape":
			c.Used[r.Key]--
		}
	}
	if ClassOf(c).Caster == CasterPact && len(c.SlotsUsed) > 0 {
		c.SlotsUsed = nil
		lines = append(lines, c.Name+" recovers their pact slots")
	}
	if (c.HasFeature("arcane_recovery") || c.HasFeature("natural_recovery")) && c.Used["slot_recovery"] == 0 && len(c.SlotsUsed) > 0 {
		budget := (c.Level + 1) / 2
		got := 0
		for l := 3; l >= 1; l-- {
			for l <= budget && l-1 < len(c.SlotsUsed) && c.SlotsUsed[l-1] > 0 {
				c.SlotsUsed[l-1]--
				budget -= l
				got++
			}
		}
		if got > 0 {
			if c.Used == nil {
				c.Used = map[string]int{}
			}
			c.Used["slot_recovery"] = 1
			lines = append(lines, fmt.Sprintf("%s recovers %d spell slot(s)", c.Name, got))
		}
	}
	return lines
}

// levelUp applies a class sheet's level gain: max HP from the hit die,
// new spells prepared. Feats and other picks wait for the player.
func levelUp(c *Character) string {
	old := c.MaxHP
	syncMaxHP(c)
	if c.IsCaster() && ClassOf(c).Caster != CasterNone {
		for _, k := range DefaultSpells(c) {
			if !containsStr(c.Spells, k) && len(ValidateSpells(c, append(append([]string{}, c.Spells...), k))) == 0 {
				c.Spells = append(c.Spells, k)
			}
		}
	}
	line := fmt.Sprintf("%s reaches level %d (+%d max HP, proficiency +%d)", c.Name, c.Level, c.MaxHP-old, ProficiencyBonus(c.Level))
	if c.Level == 3 {
		line += ", " + ClassOf(c).Subclass.Name
	}
	if n := len(PendingChoices(c)); n > 0 {
		line += fmt.Sprintf(" with %d choice(s) to make", n)
	}
	return line
}
