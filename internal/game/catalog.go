package game

import (
	"sort"
	"strings"
)

// ItemDef is a catalog entry. Instances are created with NewItem.
type ItemDef struct {
	Key     string  `json:"key"`
	Name    string  `json:"name"`
	Kind    string  `json:"kind"`
	Value   int     `json:"value"`
	Damage  string  `json:"damage,omitempty"`
	Ability Ability `json:"ability,omitempty"`
	Finesse bool    `json:"finesse,omitempty"`
	Reach   Reach   `json:"reach"`
	AC      int     `json:"ac,omitempty"`
	DexCap  int     `json:"dex_cap,omitempty"`
	Effect  string  `json:"effect,omitempty"`
	Desc    string  `json:"desc,omitempty"`
	Base    string  `json:"base,omitempty"`    // fantasy key this reskins
	Setting string  `json:"setting,omitempty"` // "" is fantasy
}

// Catalog is the fixed item list: prices, damage dice, AC and effects.
var Catalog = map[string]ItemDef{
	// Melee weapons
	"dagger":       {Key: "dagger", Name: "Dagger", Kind: ItemWeapon, Value: 2, Damage: "1d4", Ability: STR, Finesse: true, Reach: ReachThrown},
	"shortsword":   {Key: "shortsword", Name: "Shortsword", Kind: ItemWeapon, Value: 10, Damage: "1d6", Ability: STR, Finesse: true},
	"longsword":    {Key: "longsword", Name: "Longsword", Kind: ItemWeapon, Value: 15, Damage: "1d8", Ability: STR},
	"greataxe":     {Key: "greataxe", Name: "Greataxe", Kind: ItemWeapon, Value: 30, Damage: "1d12", Ability: STR},
	"rapier":       {Key: "rapier", Name: "Rapier", Kind: ItemWeapon, Value: 25, Damage: "1d8", Ability: STR, Finesse: true},
	"mace":         {Key: "mace", Name: "Mace", Kind: ItemWeapon, Value: 5, Damage: "1d6", Ability: STR},
	"quarterstaff": {Key: "quarterstaff", Name: "Quarterstaff", Kind: ItemWeapon, Value: 1, Damage: "1d6", Ability: STR},
	"handaxe":      {Key: "handaxe", Name: "Handaxe", Kind: ItemWeapon, Value: 5, Damage: "1d6", Ability: STR, Reach: ReachThrown},
	"spear":        {Key: "spear", Name: "Spear", Kind: ItemWeapon, Value: 1, Damage: "1d6", Ability: STR, Reach: ReachThrown},
	// Ranged weapons and casting foci
	"shortbow":     {Key: "shortbow", Name: "Shortbow", Kind: ItemWeapon, Value: 25, Damage: "1d6", Ability: DEX, Reach: ReachRanged},
	"longbow":      {Key: "longbow", Name: "Longbow", Kind: ItemWeapon, Value: 50, Damage: "1d8", Ability: DEX, Reach: ReachRanged},
	"crossbow":     {Key: "crossbow", Name: "Light Crossbow", Kind: ItemWeapon, Value: 25, Damage: "1d8", Ability: DEX, Reach: ReachRanged},
	"arcane_focus": {Key: "arcane_focus", Name: "Arcane Focus", Kind: ItemWeapon, Value: 10, Damage: "1d10", Ability: INT, Reach: ReachRanged, Desc: "A crystal that hurls bolts of force."},
	"holy_symbol":  {Key: "holy_symbol", Name: "Holy Symbol", Kind: ItemWeapon, Value: 5, Damage: "1d8", Ability: WIS, Reach: ReachRanged, Desc: "Channels searing radiance."},
	"lute":         {Key: "lute", Name: "Lute", Kind: ItemWeapon, Value: 35, Damage: "1d6", Ability: CHA, Reach: ReachRanged, Desc: "Its dissonant chords wound the mind."},
	// Armor (AC is the base; DexCap -1 means full DEX)
	"robes":      {Key: "robes", Name: "Traveler's Robes", Kind: ItemArmor, Value: 1, AC: 10, DexCap: -1},
	"leather":    {Key: "leather", Name: "Leather Armor", Kind: ItemArmor, Value: 10, AC: 11, DexCap: -1},
	"studded":    {Key: "studded", Name: "Studded Leather", Kind: ItemArmor, Value: 45, AC: 12, DexCap: -1},
	"scale_mail": {Key: "scale_mail", Name: "Scale Mail", Kind: ItemArmor, Value: 50, AC: 14, DexCap: 2},
	"chain_mail": {Key: "chain_mail", Name: "Chain Mail", Kind: ItemArmor, Value: 75, AC: 16},
	"shield":     {Key: "shield", Name: "Shield", Kind: ItemShield, Value: 10, AC: 2},
	// Consumables and gear
	"healing_potion":  {Key: "healing_potion", Name: "Potion of Healing", Kind: ItemConsumable, Value: 50, Effect: "heal:2d4+2"},
	"greater_healing": {Key: "greater_healing", Name: "Potion of Greater Healing", Kind: ItemConsumable, Value: 150, Effect: "heal:4d4+4"},
	"healers_kit":     {Key: "healers_kit", Name: "Healer's Kit", Kind: ItemConsumable, Value: 5, Effect: "heal:1d4+1;revive"},
	"rations":         {Key: "rations", Name: "Rations", Kind: ItemGear, Value: 1},
	"torch":           {Key: "torch", Name: "Torch", Kind: ItemGear, Value: 1},
	"rope":            {Key: "rope", Name: "Hempen Rope", Kind: ItemGear, Value: 1},
	"thieves_tools":   {Key: "thieves_tools", Name: "Thieves' Tools", Kind: ItemGear, Value: 25, Effect: "lockpick"},
	"lantern":         {Key: "lantern", Name: "Lantern", Kind: ItemGear, Value: 5},
}

// CatalogKeys returns sorted catalog keys.
func CatalogKeys() []string {
	keys := make([]string, 0, len(Catalog))
	for k := range Catalog {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Kit is a starting package. Weapon and Armor name the item keys equipped.
type Kit struct {
	Key       string    `json:"key"`
	Name      string    `json:"name"`
	Desc      string    `json:"desc"`
	Gold      int       `json:"gold"`
	Items     []string  `json:"items"`
	Weapon    string    `json:"weapon"`
	Armor     string    `json:"armor"`
	Suggested []Ability `json:"suggested"` // suggested proficiencies
}

// Kits are the six starting packages.
var Kits = map[string]Kit{
	"warrior":    {Key: "warrior", Name: "Warrior", Desc: "Heavy armor and a sword.", Gold: 10, Items: []string{"longsword", "chain_mail", "shield", "healing_potion"}, Weapon: "longsword", Armor: "chain_mail", Suggested: []Ability{STR, CON}},
	"skirmisher": {Key: "skirmisher", Name: "Skirmisher", Desc: "Light, quick, and full of knives.", Gold: 15, Items: []string{"shortsword", "dagger", "dagger", "leather", "thieves_tools"}, Weapon: "shortsword", Armor: "leather", Suggested: []Ability{DEX, CHA}},
	"ranger":     {Key: "ranger", Name: "Ranger", Desc: "A bow, an axe and the wilds.", Gold: 12, Items: []string{"longbow", "handaxe", "leather", "rations", "rope"}, Weapon: "longbow", Armor: "leather", Suggested: []Ability{DEX, WIS}},
	"arcanist":   {Key: "arcanist", Name: "Arcanist", Desc: "Robes, a focus and forbidden reading.", Gold: 8, Items: []string{"arcane_focus", "dagger", "robes", "lantern"}, Weapon: "arcane_focus", Armor: "robes", Suggested: []Ability{INT, WIS}},
	"devout":     {Key: "devout", Name: "Devout", Desc: "Faith, a mace and a healer's kit.", Gold: 10, Items: []string{"holy_symbol", "mace", "scale_mail", "healers_kit", "healers_kit"}, Weapon: "holy_symbol", Armor: "scale_mail", Suggested: []Ability{WIS, CHA}},
	"bard":       {Key: "bard", Name: "Bard", Desc: "A rapier, a lute and a silver tongue.", Gold: 20, Items: []string{"rapier", "lute", "leather", "dagger"}, Weapon: "rapier", Armor: "leather", Suggested: []Ability{CHA, DEX}},
}

// KitKeys returns kit keys in a stable order.
func KitKeys() []string {
	return []string{"warrior", "skirmisher", "ranger", "arcanist", "devout", "bard"}
}

// NewItem instantiates a catalog item for owner, translated into the
// campaign's setting. Unknown keys return nil.
func NewItem(w *World, key string, owner Owner) *Item {
	def, ok := ItemFor(w.Setting(), key)
	if !ok {
		return NewWondrous(w, key, owner) // nil unless a wondrous item or potion
	}
	it := &Item{
		ID: w.NewID("item"), Key: def.Key, Name: def.Name, Kind: def.Kind, Desc: def.Desc,
		Value: def.Value, Damage: def.Damage, Ability: def.Ability, Finesse: def.Finesse,
		Reach: def.Reach, AC: def.AC, DexCap: def.DexCap, Effect: def.Effect, Owner: owner,
	}
	w.Items = append(w.Items, it)
	return it
}

// LookupDef finds a catalog entry by key or lenient name.
func LookupDef(ref string) (ItemDef, bool) {
	if d, ok := Catalog[ref]; ok {
		return d, true
	}
	k := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(ref)), " ", "_")
	if d, ok := Catalog[k]; ok {
		return d, true
	}
	for _, key := range CatalogKeys() {
		if NameMatch(Catalog[key].Name, ref) {
			return Catalog[key], true
		}
	}
	return ItemDef{}, false
}

// InferReach decides weapon reach: catalog first, then keywords in the name.
func InferReach(it *Item) Reach {
	if it == nil {
		return ReachMelee
	}
	if d, ok := Catalog[it.Key]; ok {
		return d.Reach
	}
	if it.Reach != ReachMelee {
		return it.Reach
	}
	n := strings.ToLower(it.Name)
	for _, kw := range []string{"bow", "focus", "symbol", "wand", "staff of", "orb", "sling", "crossbow", "rod"} {
		if strings.Contains(n, kw) {
			return ReachRanged
		}
	}
	for _, kw := range []string{"javelin", "throwing", "dagger", "knife", "handaxe", "spear", "dart"} {
		if strings.Contains(n, kw) {
			return ReachThrown
		}
	}
	return ReachMelee
}

// CustomItem builds a validated non-catalog item (quest items, treasure,
// keys). Value and damage are clamped so authored items stay sane.
func CustomItem(w *World, name, kind, desc string, value int, owner Owner) *Item {
	if d, ok := LookupDef(name); ok {
		it := NewItem(w, d.Key, owner)
		return it
	}
	switch kind {
	case ItemWeapon, ItemArmor, ItemShield, ItemConsumable, ItemKey, ItemGear, ItemTreasure, ItemQuest:
	default:
		kind = ItemGear
	}
	value = clamp(value, 0, 500)
	it := &Item{ID: w.NewID("item"), Name: strings.TrimSpace(name), Kind: kind, Desc: desc, Value: value, Owner: owner}
	if kind == ItemWeapon {
		it.Damage = "1d6"
		it.Ability = STR
		it.Reach = InferReach(it)
		if it.Reach == ReachRanged {
			it.Ability = DEX
		}
	}
	if kind == ItemConsumable && strings.Contains(strings.ToLower(name), "potion") {
		it.Effect = "heal:2d4+2"
	}
	w.Items = append(w.Items, it)
	return it
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
