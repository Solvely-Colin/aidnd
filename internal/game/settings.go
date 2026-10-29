package game

import (
	"regexp"
	"strconv"
	"strings"
)

// Settings are the eras a campaign can be set in. The author picks one;
// code keeps the mechanics identical across eras by reskinning the
// fantasy catalog, so a revolver rolls exactly what a light crossbow does.
const (
	SettingFantasy  = "fantasy"
	SettingGaslight = "gaslight"
	SettingPulp     = "pulp"
	SettingModern   = "modern"
	SettingFuture   = "future"
)

// SettingInfo describes a setting for prompts and the UI.
type SettingInfo struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Desc string `json:"desc"`
}

// SettingList is every setting in display order.
var SettingList = []SettingInfo{
	{SettingFantasy, "Fantasy", "Swords, sorcery and the medieval-ish past."},
	{SettingGaslight, "Gaslight", "1800s to 1910: Victorian streets, the Wild West, steam and revolvers."},
	{SettingPulp, "Pulp", "1910s to 1950s: the Jazz Age, the Depression, noir, cosmic horror, the world wars."},
	{SettingModern, "Modern", "1960s to today: phones, cars, Kevlar and pistols."},
	{SettingFuture, "Future", "Science fiction: starships, blasters and cybernetics."},
}

// SettingKeys returns setting keys in display order.
func SettingKeys() []string {
	keys := make([]string, len(SettingList))
	for i, s := range SettingList {
		keys[i] = s.Key
	}
	return keys
}

// Setting looks up a setting by key, defaulting to fantasy.
func Setting(key string) SettingInfo {
	for _, s := range SettingList {
		if s.Key == key {
			return s
		}
	}
	return SettingList[0]
}

// NormSetting returns a valid setting key for ref, or "" if it names none.
func NormSetting(ref string) string {
	r := strings.ToLower(strings.TrimSpace(ref))
	for _, s := range SettingList {
		if r == s.Key || r == strings.ToLower(s.Name) {
			return s.Key
		}
	}
	return ""
}

var (
	yearRe   = regexp.MustCompile(`\b(1[89]\d\d|2[01]\d\d)s?\b`)
	eraWords = []struct {
		setting string
		words   []string
	}{
		{SettingFuture, []string{"sci-fi", "science fiction", "starship", "spaceship", "space station", "cyberpunk", "android", "blaster", "galaxy", "galactic", "interstellar", "orbital", "far future", "cybernetic", "mech "}},
		{SettingModern, []string{"modern", "present day", "present-day", "contemporary", "smartphone", "internet", "cell phone", "cellphone", "cold war"}},
		{SettingPulp, []string{"lovecraft", "cthulhu", "eldritch", "jazz age", "prohibition", "speakeasy", "great depression", "noir", "art deco", "roaring twenties", "world war", "pulp", "dust bowl", "gangster"}},
		{SettingGaslight, []string{"victorian", "wild west", "old west", "cowboy", "gunslinger", "steampunk", "gaslight", "gaslit", "edwardian"}},
	}
)

// InferSetting guesses a setting from free text: an explicit year or
// decade first, then era words. Anything else is fantasy.
func InferSetting(text string) string {
	t := strings.ToLower(text)
	if m := yearRe.FindStringSubmatch(t); m != nil {
		y, _ := strconv.Atoi(m[1])
		switch {
		case y < 1910:
			return SettingGaslight
		case y < 1960:
			return SettingPulp
		case y < 2050:
			return SettingModern
		default:
			return SettingFuture
		}
	}
	for _, e := range eraWords {
		for _, w := range e.words {
			if strings.Contains(t, w) {
				return e.setting
			}
		}
	}
	return SettingFantasy
}

// ChooseSetting settles the author's pick against the premise. Explicit
// era signals in the premise win, since the player wrote them.
func ChooseSetting(authored, premise string) string {
	if s := InferSetting(premise); s != SettingFantasy {
		return s
	}
	if s := NormSetting(authored); s != "" {
		return s
	}
	return SettingFantasy
}

// Setting returns the campaign's setting. Campaigns made before settings
// existed have none stored, so it is inferred from the premise.
func (w *World) Setting() string {
	if s := NormSetting(w.Campaign.Setting); s != "" {
		return s
	}
	return InferSetting(w.Campaign.Premise)
}

// universal items exist unchanged in every setting.
var universal = map[string]bool{"rations": true}

// reskins maps setting -> fantasy key -> {name, desc}. Each entry becomes
// a catalog item keyed "<setting>_<fantasy key>" with the fantasy stats.
var reskins = map[string]map[string][2]string{
	SettingGaslight: {
		"dagger":          {"Bowie Knife", ""},
		"shortsword":      {"Sword Cane", ""},
		"longsword":       {"Cavalry Saber", ""},
		"greataxe":        {"Woodsman's Axe", ""},
		"rapier":          {"Dueling Sabre", ""},
		"mace":            {"Cudgel", ""},
		"quarterstaff":    {"Walking Stick", ""},
		"handaxe":         {"Tomahawk", ""},
		"spear":           {"Harpoon", ""},
		"shortbow":        {"Derringer", "A pocket pistol for close work."},
		"longbow":         {"Lever-Action Rifle", ""},
		"crossbow":        {"Revolver", "Six chambers, well oiled."},
		"arcane_focus":    {"Galvanic Projector", "Hurls arcs of crackling current."},
		"holy_symbol":     {"Blessed Crucifix", "Channels searing radiance."},
		"lute":            {"Fiddle", "Its dissonant reels wound the mind."},
		"robes":           {"Travelling Clothes", ""},
		"leather":         {"Leather Duster", ""},
		"studded":         {"Reinforced Waistcoat", ""},
		"scale_mail":      {"Steel Breastplate", ""},
		"chain_mail":      {"Heavy Cuirass", ""},
		"shield":          {"Iron Buckler", ""},
		"healing_potion":  {"Patent Tonic", ""},
		"greater_healing": {"Physician's Elixir", ""},
		"healers_kit":     {"Doctor's Bag", ""},
		"torch":           {"Pitch Torch", ""},
		"rope":            {"Hempen Rope", ""},
		"thieves_tools":   {"Lockpicks", ""},
		"lantern":         {"Oil Lamp", ""},
	},
	SettingPulp: {
		"dagger":          {"Switchblade", ""},
		"shortsword":      {"Trench Knife", ""},
		"longsword":       {"Machete", ""},
		"greataxe":        {"Fire Axe", ""},
		"rapier":          {"Sword Cane", ""},
		"mace":            {"Blackjack", ""},
		"quarterstaff":    {"Walking Cane", ""},
		"handaxe":         {"Hatchet", ""},
		"spear":           {"Harpoon", ""},
		"shortbow":        {"Snub-Nose Revolver", ""},
		"longbow":         {"Hunting Rifle", ""},
		"crossbow":        {"Automatic Pistol", "A .45, heavy in the hand."},
		"arcane_focus":    {"Occult Tome", "Words from a forbidden book that tear at flesh."},
		"holy_symbol":     {"Crucifix", "Channels searing radiance."},
		"lute":            {"Saxophone", "Its wailing notes unsettle the mind."},
		"robes":           {"Tweed Suit", ""},
		"leather":         {"Leather Jacket", ""},
		"studded":         {"Padded Trench Coat", ""},
		"scale_mail":      {"Bulletproof Vest", ""},
		"chain_mail":      {"Steel Body Armor", ""},
		"shield":          {"Steel Plate Shield", ""},
		"healing_potion":  {"Morphine Syrette", ""},
		"greater_healing": {"Blood Plasma", ""},
		"healers_kit":     {"Medical Bag", ""},
		"torch":           {"Flashlight", ""},
		"rope":            {"Rope", ""},
		"thieves_tools":   {"Lockpick Set", ""},
		"lantern":         {"Electric Lantern", ""},
	},
	SettingModern: {
		"dagger":          {"Combat Knife", ""},
		"shortsword":      {"Machete", ""},
		"longsword":       {"Crowbar", ""},
		"greataxe":        {"Fire Axe", ""},
		"rapier":          {"Katana", ""},
		"mace":            {"Tactical Baton", ""},
		"quarterstaff":    {"Baseball Bat", ""},
		"handaxe":         {"Hatchet", ""},
		"spear":           {"Hunting Spear", ""},
		"shortbow":        {"Compact Pistol", ""},
		"longbow":         {"Rifle", ""},
		"crossbow":        {"9mm Pistol", ""},
		"arcane_focus":    {"Taser", "Precisely aimed, it locks up every nerve."},
		"holy_symbol":     {"Flare Gun", "Burning magnesium at close range."},
		"lute":            {"Megaphone", "A shrieking feedback blast that rattles the skull."},
		"robes":           {"Street Clothes", ""},
		"leather":         {"Motorcycle Jacket", ""},
		"studded":         {"Stab Vest", ""},
		"scale_mail":      {"Kevlar Vest", ""},
		"chain_mail":      {"Tactical Armor", ""},
		"shield":          {"Riot Shield", ""},
		"healing_potion":  {"First-Aid Kit", ""},
		"greater_healing": {"Trauma Kit", ""},
		"healers_kit":     {"Paramedic Bag", ""},
		"torch":           {"Flashlight", ""},
		"rope":            {"Climbing Rope", ""},
		"thieves_tools":   {"Lockpick Set", ""},
		"lantern":         {"Camping Lantern", ""},
	},
	SettingFuture: {
		"dagger":          {"Vibro-Knife", ""},
		"shortsword":      {"Monoblade", ""},
		"longsword":       {"Shock Blade", ""},
		"greataxe":        {"Power Hammer", ""},
		"rapier":          {"Mono-Rapier", ""},
		"mace":            {"Stun Baton", ""},
		"quarterstaff":    {"Shock Staff", ""},
		"handaxe":         {"Throwing Disc", ""},
		"spear":           {"Arc Lance", ""},
		"shortbow":        {"Holdout Blaster", ""},
		"longbow":         {"Rail Rifle", ""},
		"crossbow":        {"Blaster Pistol", ""},
		"arcane_focus":    {"Psi Amplifier", "Focuses the mind into a bolt of force."},
		"holy_symbol":     {"Pulse Emitter", "A searing beam of tuned light."},
		"lute":            {"Sonic Projector", "Resonant tones that wound the mind."},
		"robes":           {"Flight Suit", ""},
		"leather":         {"Padded Jumpsuit", ""},
		"studded":         {"Mesh Armor", ""},
		"scale_mail":      {"Ballistic Weave", ""},
		"chain_mail":      {"Composite Plate", ""},
		"shield":          {"Energy Shield", ""},
		"healing_potion":  {"Medi-Gel", ""},
		"greater_healing": {"Nanite Injector", ""},
		"healers_kit":     {"Medkit", ""},
		"torch":           {"Glowstick", ""},
		"rope":            {"Synthcord", ""},
		"thieves_tools":   {"Bypass Kit", ""},
		"lantern":         {"Floodlamp", ""},
	},
}

func init() {
	for setting, m := range reskins {
		for base, r := range m {
			d, ok := Catalog[base]
			if !ok {
				panic("reskin of unknown catalog key " + base)
			}
			d.Key, d.Name, d.Base, d.Setting = setting+"_"+base, r[0], base, setting
			if r[1] != "" {
				d.Desc = r[1]
			}
			Catalog[d.Key] = d
		}
	}
}

// availableIn reports whether def belongs in setting as it is.
func availableIn(def ItemDef, setting string) bool {
	if universal[def.Key] {
		return true
	}
	if def.Setting == "" {
		return setting == SettingFantasy
	}
	return def.Setting == setting
}

// ItemFor returns the catalog entry for key as it exists in setting: a
// longsword asked for in a pulp campaign becomes the pulp machete, and a
// revolver asked for in a fantasy campaign becomes a light crossbow.
func ItemFor(setting, key string) (ItemDef, bool) {
	def, ok := Catalog[key]
	if !ok {
		return ItemDef{}, false
	}
	if availableIn(def, setting) {
		return def, true
	}
	base := def.Key
	if def.Base != "" {
		base = def.Base
	}
	if d, ok := Catalog[setting+"_"+base]; ok {
		return d, true
	}
	return Catalog[base], true
}

// CatalogKeysFor returns sorted catalog keys that belong in setting.
func CatalogKeysFor(setting string) []string {
	var keys []string
	for _, k := range CatalogKeys() {
		if availableIn(Catalog[k], setting) {
			keys = append(keys, k)
		}
	}
	return keys
}

// eraKits override the six kits per setting. Items, Weapon and Armor are
// fantasy catalog keys, translated with ItemFor; Suggested comes from the
// base kit so drafts and validation are the same in every era.
var eraKits = map[string]map[string]Kit{
	SettingGaslight: {
		"warrior":    {Name: "Soldier", Desc: "A cavalry saber, a heavy cuirass and a buckler.", Gold: 10, Items: []string{"longsword", "chain_mail", "shield", "healing_potion"}, Weapon: "longsword", Armor: "chain_mail"},
		"skirmisher": {Name: "Gunslinger", Desc: "A revolver, a Bowie knife and a duster.", Gold: 15, Items: []string{"crossbow", "dagger", "leather", "thieves_tools"}, Weapon: "crossbow", Armor: "leather"},
		"ranger":     {Name: "Frontier Scout", Desc: "A rifle, a tomahawk and the open range.", Gold: 12, Items: []string{"longbow", "handaxe", "leather", "rations", "rope"}, Weapon: "longbow", Armor: "leather"},
		"arcanist":   {Name: "Natural Philosopher", Desc: "A galvanic projector and dangerous theories.", Gold: 8, Items: []string{"arcane_focus", "dagger", "robes", "lantern"}, Weapon: "arcane_focus", Armor: "robes"},
		"devout":     {Name: "Preacher", Desc: "A crucifix, a cudgel and a doctor's bag.", Gold: 10, Items: []string{"holy_symbol", "mace", "scale_mail", "healers_kit", "healers_kit"}, Weapon: "holy_symbol", Armor: "scale_mail"},
		"bard":       {Name: "Showman", Desc: "A sword cane, a fiddle and a silver tongue.", Gold: 20, Items: []string{"rapier", "lute", "leather", "shortbow"}, Weapon: "rapier", Armor: "leather"},
	},
	SettingPulp: {
		"warrior":    {Name: "Veteran", Desc: "Body armor, a machete and a steel shield from the war.", Gold: 10, Items: []string{"longsword", "chain_mail", "shield", "healing_potion"}, Weapon: "longsword", Armor: "chain_mail"},
		"skirmisher": {Name: "Gumshoe", Desc: "A .45, a switchblade and a set of lockpicks.", Gold: 15, Items: []string{"crossbow", "dagger", "leather", "thieves_tools", "torch"}, Weapon: "crossbow", Armor: "leather"},
		"ranger":     {Name: "Big-Game Hunter", Desc: "A hunting rifle, a hatchet and a steady eye.", Gold: 12, Items: []string{"longbow", "handaxe", "leather", "rations", "rope"}, Weapon: "longbow", Armor: "leather"},
		"arcanist":   {Name: "Occultist", Desc: "A forbidden tome and an electric lantern.", Gold: 8, Items: []string{"arcane_focus", "dagger", "robes", "lantern"}, Weapon: "arcane_focus", Armor: "robes"},
		"devout":     {Name: "Chaplain", Desc: "A crucifix, a blackjack and a medical bag.", Gold: 10, Items: []string{"holy_symbol", "mace", "scale_mail", "healers_kit", "healers_kit"}, Weapon: "holy_symbol", Armor: "scale_mail"},
		"bard":       {Name: "Reporter", Desc: "A snub-nose, a press pass smile and a silver tongue.", Gold: 20, Items: []string{"shortbow", "lute", "leather", "dagger"}, Weapon: "shortbow", Armor: "leather"},
	},
	SettingModern: {
		"warrior":    {Name: "Soldier", Desc: "Tactical armor, a riot shield and a crowbar.", Gold: 10, Items: []string{"longsword", "chain_mail", "shield", "healing_potion"}, Weapon: "longsword", Armor: "chain_mail"},
		"skirmisher": {Name: "Detective", Desc: "A 9mm, a combat knife and lockpicks.", Gold: 15, Items: []string{"crossbow", "dagger", "leather", "thieves_tools", "torch"}, Weapon: "crossbow", Armor: "leather"},
		"ranger":     {Name: "Survivalist", Desc: "A rifle, a hatchet and the backcountry.", Gold: 12, Items: []string{"longbow", "handaxe", "leather", "rations", "rope"}, Weapon: "longbow", Armor: "leather"},
		"arcanist":   {Name: "Hacker", Desc: "A taser, street clothes and too much knowledge.", Gold: 8, Items: []string{"arcane_focus", "dagger", "robes", "lantern"}, Weapon: "arcane_focus", Armor: "robes"},
		"devout":     {Name: "Paramedic", Desc: "A flare gun, a baton and a paramedic bag.", Gold: 10, Items: []string{"holy_symbol", "mace", "scale_mail", "healers_kit", "healers_kit"}, Weapon: "holy_symbol", Armor: "scale_mail"},
		"bard":       {Name: "Con Artist", Desc: "A compact pistol, a megaphone and a silver tongue.", Gold: 20, Items: []string{"shortbow", "lute", "leather", "dagger"}, Weapon: "shortbow", Armor: "leather"},
	},
	SettingFuture: {
		"warrior":    {Name: "Marine", Desc: "Composite plate, an energy shield and a shock blade.", Gold: 10, Items: []string{"longsword", "chain_mail", "shield", "healing_potion"}, Weapon: "longsword", Armor: "chain_mail"},
		"skirmisher": {Name: "Smuggler", Desc: "A blaster pistol, a vibro-knife and a bypass kit.", Gold: 15, Items: []string{"crossbow", "dagger", "leather", "thieves_tools"}, Weapon: "crossbow", Armor: "leather"},
		"ranger":     {Name: "Scout", Desc: "A rail rifle, a throwing disc and frontier worlds.", Gold: 12, Items: []string{"longbow", "handaxe", "leather", "rations", "rope"}, Weapon: "longbow", Armor: "leather"},
		"arcanist":   {Name: "Psion", Desc: "A psi amplifier and a mind that bends matter.", Gold: 8, Items: []string{"arcane_focus", "dagger", "robes", "lantern"}, Weapon: "arcane_focus", Armor: "robes"},
		"devout":     {Name: "Medic", Desc: "A pulse emitter, a stun baton and a medkit.", Gold: 10, Items: []string{"holy_symbol", "mace", "scale_mail", "healers_kit", "healers_kit"}, Weapon: "holy_symbol", Armor: "scale_mail"},
		"bard":       {Name: "Diplomat", Desc: "A mono-rapier, a sonic projector and a silver tongue.", Gold: 20, Items: []string{"rapier", "lute", "leather", "dagger"}, Weapon: "rapier", Armor: "leather"},
	},
}

// KitFor returns kit key as it exists in setting. Unknown keys return
// the zero Kit.
func KitFor(setting, key string) Kit {
	base, ok := Kits[key]
	if !ok {
		return Kit{}
	}
	k, ok := eraKits[setting][key]
	if !ok {
		return base
	}
	k.Key, k.Suggested = base.Key, base.Suggested
	return k
}

// KitItems returns the catalog entries a kit starts with in setting.
func KitItems(setting string, k Kit) []ItemDef {
	var items []ItemDef
	for _, key := range k.Items {
		if d, ok := ItemFor(setting, key); ok {
			items = append(items, d)
		}
	}
	return items
}
