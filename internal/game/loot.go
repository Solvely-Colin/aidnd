package game

import (
	"fmt"
	"hash/fnv"
	"math/rand"
	"sort"
	"strings"

	"github.com/Solvely-Colin/aidnd/internal/dice"
)

// Magic items and loot. Magic weapons and armor are an ordinary item with
// a template on top (+1, flame tongue...); wondrous items and potions are
// their own table. Every name has a version for each era, as with spells.

// Rarities, lowest first.
const (
	RarityCommon    = "common"
	RarityUncommon  = "uncommon"
	RarityRare      = "rare"
	RarityVeryRare  = "very_rare"
	RarityLegendary = "legendary"
)

var rarityOrder = []string{RarityCommon, RarityUncommon, RarityRare, RarityVeryRare, RarityLegendary}

// rarityValue is a sale price in gold.
var rarityValue = map[string]int{RarityCommon: 50, RarityUncommon: 200, RarityRare: 600, RarityVeryRare: 1500, RarityLegendary: 4000}

// RarityName is the word players see.
func RarityName(r string) string { return strings.ReplaceAll(r, "_", " ") }

// Item properties the engine applies.
const (
	PropFlame      = "flame"      // weapon: +1d6 fire on a hit
	PropHoly       = "holy"       // weapon: +1d10 radiant on a hit
	PropProtection = "protection" // wondrous: +1 AC and +1 to saves
	PropArcane     = "arcane"     // wondrous: +Bonus to spell attacks and save DCs
	PropRegen      = "regen"      // wondrous: regain 1d6 HP at the start of your turn in a fight
)

// ItemExtra is what makes an item magic. It is stored in the item's extra
// column.
type ItemExtra struct {
	Bonus  int      `json:"bonus,omitempty"` // to hit and damage, or AC
	Rarity string   `json:"rarity,omitempty"`
	Props  []string `json:"props,omitempty"`
	// Floor sets an ability score while carried ("CON:19").
	Floor string `json:"floor,omitempty"`
}

// Magic reports whether the item is magical.
func (x ItemExtra) Magic() bool { return x.Rarity != "" }

func (x ItemExtra) has(p string) bool {
	for _, q := range x.Props {
		if q == p {
			return true
		}
	}
	return false
}

// ItemWondrous is a magic item that works while carried (attuned).
const ItemWondrous = "wondrous"

// MaxAttuned is how many wondrous items work at once.
const MaxAttuned = 3

// MagicTemplate turns a mundane weapon or armor into a magic one. Name is
// a pattern with %s for the base item's name.
type MagicTemplate struct {
	Key    string    `json:"key"`
	For    string    `json:"for"` // weapon | armor
	Rarity string    `json:"rarity"`
	Bonus  int       `json:"bonus"`
	Props  []string  `json:"props,omitempty"`
	Name   string    `json:"name"`
	Era    *EraNames `json:"-"`
}

var MagicTemplates = []MagicTemplate{
	{Key: "weapon_plus1", For: ItemWeapon, Rarity: RarityUncommon, Bonus: 1, Name: "+1 %s", Era: &EraNames{"Masterwork %s", "Lucky %s", "Custom %s", "Tuned %s"}},
	{Key: "weapon_flame", For: ItemWeapon, Rarity: RarityRare, Props: []string{PropFlame}, Name: "Flame Tongue %s", Era: &EraNames{"Phosphor %s", "Hellfire %s", "Incendiary %s", "Plasma %s"}},
	{Key: "weapon_plus2", For: ItemWeapon, Rarity: RarityRare, Bonus: 2, Name: "+2 %s", Era: &EraNames{"Exquisite %s", "Blessed %s", "Match-Grade %s", "Overclocked %s"}},
	{Key: "weapon_plus3", For: ItemWeapon, Rarity: RarityVeryRare, Bonus: 3, Name: "+3 %s", Era: &EraNames{"Legendary-Make %s", "Saint's %s", "Prototype %s", "Singularity %s"}},
	{Key: "weapon_holy", For: ItemWeapon, Rarity: RarityLegendary, Bonus: 3, Props: []string{PropHoly}, Name: "Holy Avenger %s", Era: &EraNames{"Sanctified %s of St. Michael", "Last Crusader's %s", "Vatican-Issue %s", "Seraph-Class %s"}},
	{Key: "armor_plus1", For: ItemArmor, Rarity: RarityRare, Bonus: 1, Name: "+1 %s", Era: &EraNames{"Reinforced %s", "Bulletproofed %s", "Kevlar-Lined %s", "Hardened %s"}},
	{Key: "armor_plus2", For: ItemArmor, Rarity: RarityVeryRare, Bonus: 2, Name: "+2 %s", Era: &EraNames{"Damascene %s", "Saint's %s", "Ceramic-Plate %s", "Nanoweave %s"}},
	{Key: "armor_plus3", For: ItemArmor, Rarity: RarityLegendary, Bonus: 3, Name: "+3 %s", Era: &EraNames{"Adamant %s", "Unbreakable %s", "Experimental %s", "Neutronium %s"}},
}

// WondrousDef is a wondrous item or a magic potion.
type WondrousDef struct {
	Key    string    `json:"key"`
	Name   string    `json:"name"`
	Kind   string    `json:"kind"` // wondrous | consumable
	Rarity string    `json:"rarity"`
	Bonus  int       `json:"bonus,omitempty"`
	Props  []string  `json:"props,omitempty"`
	Floor  string    `json:"floor,omitempty"`
	Effect string    `json:"effect,omitempty"`
	Desc   string    `json:"desc"`
	Era    *EraNames `json:"-"`
}

var Wondrous = map[string]*WondrousDef{
	"cloak_of_protection": {Key: "cloak_of_protection", Name: "Cloak of Protection", Kind: ItemWondrous, Rarity: RarityUncommon, Props: []string{PropProtection}, Desc: "+1 AC and +1 to saving throws.",
		Era: &EraNames{"Warded Greatcoat", "Saint Christopher Medal", "Lucky Charm", "Deflector Belt"}},
	"wand_of_the_war_mage": {Key: "wand_of_the_war_mage", Name: "Wand of the War Mage", Kind: ItemWondrous, Rarity: RarityUncommon, Bonus: 1, Props: []string{PropArcane}, Desc: "+1 to spell attacks and spell save DCs.",
		Era: &EraNames{"Ether Rod", "Occult Focus", "Hex-Etched Phone", "Psi Amplifier"}},
	"gauntlets_of_ogre_power": {Key: "gauntlets_of_ogre_power", Name: "Gauntlets of Ogre Power", Kind: ItemWondrous, Rarity: RarityUncommon, Floor: "STR:19", Desc: "Your Strength is 19.",
		Era: &EraNames{"Steam Gauntlets", "Iron Knuckles", "Power Gloves", "Servo Gauntlets"}},
	"headband_of_intellect": {Key: "headband_of_intellect", Name: "Headband of Intellect", Kind: ItemWondrous, Rarity: RarityUncommon, Floor: "INT:19", Desc: "Your Intelligence is 19.",
		Era: &EraNames{"Phrenological Crown", "Mesmerist's Circlet", "Neural Headset", "Cortex Booster"}},
	"ring_of_protection": {Key: "ring_of_protection", Name: "Ring of Protection", Kind: ItemWondrous, Rarity: RarityRare, Props: []string{PropProtection}, Desc: "+1 AC and +1 to saving throws.",
		Era: &EraNames{"Ward Ring", "Blessed Rosary", "Ballistic Talisman", "Shield Ring"}},
	"amulet_of_health": {Key: "amulet_of_health", Name: "Amulet of Health", Kind: ItemWondrous, Rarity: RarityRare, Floor: "CON:19", Desc: "Your Constitution is 19.",
		Era: &EraNames{"Vital Locket", "Radium Tonic Flask", "Experimental Serum", "Bio-Regulator"}},
	"staff_of_power": {Key: "staff_of_power", Name: "Staff of Power", Kind: ItemWondrous, Rarity: RarityVeryRare, Bonus: 2, Props: []string{PropArcane, PropProtection}, Desc: "+2 to spell attacks and DCs, +1 AC and saves.",
		Era: &EraNames{"Rod of Dominion", "Staff of the Old Ones", "Black Site Relic", "Psionic Staff"}},
	"ring_of_regeneration": {Key: "ring_of_regeneration", Name: "Ring of Regeneration", Kind: ItemWondrous, Rarity: RarityLegendary, Props: []string{PropRegen}, Desc: "Regain 1d6 HP at the start of each of your turns in a fight.",
		Era: &EraNames{"Philosopher's Ring", "Shaman's Bone Fetish", "Regen Implant", "Nanite Swarm"}},
	"superior_healing": {Key: "superior_healing", Name: "Potion of Superior Healing", Kind: ItemConsumable, Rarity: RarityRare, Effect: "heal:8d4+8", Desc: "Heals 8d4+8.",
		Era: &EraNames{"Superior Restorative", "Miracle Tonic", "Trauma Kit", "Medi-Gel Injector"}},
	"supreme_healing": {Key: "supreme_healing", Name: "Potion of Supreme Healing", Kind: ItemConsumable, Rarity: RarityVeryRare, Effect: "heal:10d4+20", Desc: "Heals 10d4+20.",
		Era: &EraNames{"Elixir of Life", "Lazarus Serum", "Combat Stim", "Full-Regen Shot"}},
}

// WondrousKeys lists wondrous items and potions by rarity, then key.
func WondrousKeys() []string {
	var keys []string
	for k := range Wondrous {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := rarityRank(Wondrous[keys[i]].Rarity), rarityRank(Wondrous[keys[j]].Rarity)
		if a != b {
			return a < b
		}
		return keys[i] < keys[j]
	})
	return keys
}

func rarityRank(r string) int {
	for i, x := range rarityOrder {
		if x == r {
			return i
		}
	}
	return 0
}

// NewWondrous creates a wondrous item or potion for owner.
func NewWondrous(w *World, key string, owner Owner) *Item {
	d := Wondrous[key]
	if d == nil {
		return nil
	}
	it := &Item{ID: w.NewID("item"), Key: d.Key, Name: eraName(w.Setting(), d.Name, d.Era), Kind: d.Kind, Desc: d.Desc,
		Value: rarityValue[d.Rarity], Effect: d.Effect, Owner: owner,
		ItemExtra: ItemExtra{Bonus: d.Bonus, Rarity: d.Rarity, Props: append([]string(nil), d.Props...), Floor: d.Floor}}
	w.Items = append(w.Items, it)
	return it
}

// NewMagicGear creates a magic weapon or armor from a catalog item and a
// template, named for the setting.
func NewMagicGear(w *World, baseKey, template string, owner Owner) *Item {
	var t *MagicTemplate
	for i := range MagicTemplates {
		if MagicTemplates[i].Key == template {
			t = &MagicTemplates[i]
		}
	}
	if t == nil {
		return nil
	}
	it := NewItem(w, baseKey, owner)
	if it == nil {
		return nil
	}
	if it.Kind != t.For && !(t.For == ItemArmor && it.Kind == ItemShield) {
		w.Items = w.Items[:len(w.Items)-1]
		return nil
	}
	it.Name = fmt.Sprintf(eraName(w.Setting(), t.Name, t.Era), it.Name)
	it.Value += rarityValue[t.Rarity]
	it.ItemExtra = ItemExtra{Bonus: t.Bonus, Rarity: t.Rarity, Props: append([]string(nil), t.Props...)}
	return it
}

// Boons are what a character's magic items give beyond their own weapon
// and armor: from up to three carried wondrous items.
type Boons struct {
	AC, Saves, Spell int
	Floors           map[Ability]int
	Regen            bool
	Attuned          []string // item IDs
}

// RefreshMagic recomputes every character's boons from what they carry.
// The store calls it on load and Resolve after every step.
func (w *World) RefreshMagic() {
	for _, c := range w.Characters {
		c.boons = boonsOf(w, c)
	}
}

func boonsOf(w *World, c *Character) Boons {
	var b Boons
	for _, it := range w.ItemsOf(OwnerCharacter, c.ID) {
		if it.Kind != ItemWondrous || len(b.Attuned) >= MaxAttuned {
			continue
		}
		b.Attuned = append(b.Attuned, it.ID)
		if it.has(PropProtection) {
			b.AC++
			b.Saves++
		}
		if it.has(PropArcane) {
			b.Spell += max(1, it.Bonus)
		}
		if it.has(PropRegen) {
			b.Regen = true
		}
		if ab, v, ok := strings.Cut(it.Floor, ":"); ok {
			if a, ok := ParseAbility(ab); ok {
				if b.Floors == nil {
					b.Floors = map[Ability]int{}
				}
				var n int
				fmt.Sscan(v, &n)
				b.Floors[a] = max(b.Floors[a], n)
			}
		}
	}
	return b
}

// Score is c's ability score with magic items applied.
func (c *Character) Score(a Ability) int { return max(c.Scores[a], c.boons.Floors[a]) }

// Attuned reports whether a wondrous item is working for c.
func (c *Character) Attuned(itemID string) bool {
	for _, id := range c.boons.Attuned {
		if id == itemID {
			return true
		}
	}
	return false
}

// regenTick heals a character wearing regeneration at the start of their
// turn in a fight.
func regenTick(r *dice.Roller, c *Character) []string {
	if !c.boons.Regen || c.HP <= 0 || c.HP >= c.MaxHP {
		return nil
	}
	n := r.Die(6)
	c.HP = min(c.MaxHP, c.HP+n)
	return []string{fmt.Sprintf("%s regenerates %d HP (%d/%d)", c.Name, n, c.HP, c.MaxHP)}
}

// ---- loot

// rollLoot shares out what the defeated enemies of the fight leave: gold
// split across the party, and potions and magic items on the ground. It is
// seeded by the encounter so a dry run and the real turn agree.
func rollLoot(w *World) []string {
	enc, l := w.Encounter, w.Here()
	if enc == nil || l == nil {
		return nil
	}
	h := fnv.New64a()
	fmt.Fprintf(h, "%s/%s/%d", w.Campaign.ID, enc.ID, enc.Round)
	rng := rand.New(rand.NewSource(int64(h.Sum64())))
	danger := clamp(l.Danger, 1, 5)
	level := w.PartyLevel()
	gold, beaten := 0, 0
	var boss *Enemy
	for _, e := range enc.Enemies {
		if !e.Defeated() {
			continue
		}
		beaten++
		gold += danger + rng.Intn(4*danger)
		if e.Boss != nil {
			boss = e
			gold += 20 * danger
		}
	}
	if beaten == 0 {
		return nil
	}
	ground := Owner{Kind: OwnerLocation, ID: l.ID}
	var found []*Item
	if boss != nil {
		rarity := RarityUncommon
		switch {
		case l.IsLair && level >= 5 && rng.Intn(4) == 0:
			rarity = RarityLegendary
		case l.IsLair && level >= 4 && rng.Intn(3) == 0:
			rarity = RarityVeryRare
		case l.IsLair || danger >= 4 || level >= 3:
			rarity = RarityRare
		}
		found = append(found, randomMagic(w, rng, rarity, ground))
		found = append(found, NewItem(w, "greater_healing", ground))
	} else {
		if rng.Intn(10) < 3 {
			found = append(found, NewItem(w, "healing_potion", ground))
		}
		if danger >= 3 && rng.Intn(100) < 10+10*(danger-2) {
			found = append(found, randomMagic(w, rng, RarityUncommon, ground))
		}
	}
	var out []string
	if gold > 0 {
		party := w.Party()
		share, rest := gold/max(1, len(party)), gold%max(1, len(party))
		for i, c := range party {
			c.Gold += share
			if i == 0 {
				c.Gold += rest
			}
		}
		out = append(out, fmt.Sprintf("Loot: %d gold, shared (%d each)", gold, share))
	}
	var names []string
	for _, it := range found {
		if it == nil {
			continue
		}
		n := it.Name
		if it.Magic() {
			n += " (" + RarityName(it.Rarity) + ")"
		}
		names = append(names, n)
	}
	if len(names) > 0 {
		out = append(out, "Left on the ground: "+strings.Join(names, ", "))
	}
	return out
}

// randomMagic makes one magic item of rarity: a weapon or armor for
// someone in the party, or a wondrous item.
func randomMagic(w *World, rng *rand.Rand, rarity string, owner Owner) *Item {
	type pick struct{ base, tmpl, wondrous string }
	var picks []pick
	for _, k := range WondrousKeys() {
		if d := Wondrous[k]; d.Rarity == rarity && d.Kind == ItemWondrous {
			picks = append(picks, pick{wondrous: k})
		}
	}
	for _, c := range w.Party() {
		for _, t := range MagicTemplates {
			if t.Rarity != rarity {
				continue
			}
			if it := w.Item(c.Weapon); t.For == ItemWeapon && it != nil && it.Key != "" && !it.Magic() && !IsFocus(it) {
				picks = append(picks, pick{base: it.Key, tmpl: t.Key})
			}
			if it := w.Item(c.Armor); t.For == ItemArmor && it != nil && it.Key != "" && !it.Magic() && it.Key != "robes" {
				picks = append(picks, pick{base: it.Key, tmpl: t.Key})
			}
		}
	}
	if len(picks) == 0 {
		return NewItem(w, "greater_healing", owner)
	}
	p := picks[rng.Intn(len(picks))]
	if p.wondrous != "" {
		return NewWondrous(w, p.wondrous, owner)
	}
	return NewMagicGear(w, baseOf(p.base), p.tmpl, owner)
}

// baseOf is the fantasy catalog key an era's item reskins.
func baseOf(key string) string {
	if d, ok := Catalog[key]; ok && d.Base != "" {
		return d.Base
	}
	return key
}

// MagicSummary says in a few words what an item's magic does.
func MagicSummary(it *Item) string {
	if !it.Magic() {
		return ""
	}
	var parts []string
	switch {
	case it.Bonus != 0 && it.Kind == ItemWeapon:
		parts = append(parts, fmt.Sprintf("%+d to hit and damage", it.Bonus))
	case it.Bonus != 0 && (it.Kind == ItemArmor || it.Kind == ItemShield):
		parts = append(parts, fmt.Sprintf("%+d AC", it.Bonus))
	}
	for _, p := range it.Props {
		switch p {
		case PropFlame:
			parts = append(parts, "+1d6 fire on a hit")
		case PropHoly:
			parts = append(parts, "+1d10 radiant on a hit")
		case PropProtection:
			parts = append(parts, "+1 AC and saves")
		case PropArcane:
			parts = append(parts, fmt.Sprintf("%+d spell attacks and DCs", max(1, it.Bonus)))
		case PropRegen:
			parts = append(parts, "regain 1d6 HP each turn in a fight")
		}
	}
	if ab, v, ok := strings.Cut(it.Floor, ":"); ok {
		parts = append(parts, ab+" becomes "+v)
	}
	return strings.Join(parts, ", ")
}

// MagicTemplateName is a template's name pattern in setting.
func MagicTemplateName(t MagicTemplate, setting string) string {
	return eraName(setting, t.Name, t.Era)
}

// WondrousName is a wondrous item's name in setting.
func WondrousName(d *WondrousDef, setting string) string { return eraName(setting, d.Name, d.Era) }
