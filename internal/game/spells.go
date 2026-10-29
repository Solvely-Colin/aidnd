package game

import "sort"

// Spell kinds: how the engine resolves a spell.
const (
	SpellAttack  = "attack"  // spell attack roll vs AC, damage on a hit
	SpellSave    = "save"    // targets save; damage (half on a save) and/or a condition on a fail
	SpellHeal    = "heal"    // restores HP to allies
	SpellBuff    = "buff"    // a helpful condition on allies
	SpellUtility = "utility" // no rolls: the narrator describes it
	SpellSpecial = "special" // resolved by its own code (magic missile, misty step...)
)

// Spell ranges, in zones like weapon reach.
const (
	RangeSelf  = "self"  // the caster (or their zone, for areas)
	RangeTouch = "touch" // same zone
	RangeNear  = "near"  // same or next zone
	RangeFar   = "far"   // up to two zones
)

// SpellDef is one spell.
type SpellDef struct {
	Key     string   `json:"key"`
	Name    string   `json:"name"`
	Level   int      `json:"level"` // 0 = cantrip
	School  string   `json:"school"`
	Classes []string `json:"classes"`
	Cast    Slot     `json:"cast"` // action or bonus
	Range   string   `json:"range"`
	Kind    string   `json:"kind"`
	// Area spells hit every foe in a zone: the caster's (range self) or the
	// target's.
	Area   bool    `json:"area,omitempty"`
	Dice   string  `json:"dice,omitempty"`   // damage or healing
	Upcast string  `json:"upcast,omitempty"` // extra dice per slot level above the spell's
	Save   Ability `json:"save,omitempty"`
	Half   bool    `json:"half,omitempty"` // half damage on a save
	Cond   string  `json:"cond,omitempty"` // condition on a failed save, or the buff given
	Conc   bool    `json:"conc,omitempty"`
	// Targets is how many creatures a buff or heal reaches (0 = one).
	Targets int `json:"targets,omitempty"`
	// Peace spells take minutes and can't be cast in a fight.
	Peace bool      `json:"peace,omitempty"`
	Desc  string    `json:"desc"`
	Era   *EraNames `json:"-"`
}

var (
	full  = []string{"bard", "cleric", "druid", "sorcerer", "warlock", "wizard"}
	arc   = []string{"sorcerer", "wizard"}
	arcW  = []string{"sorcerer", "warlock", "wizard"}
	arcB  = []string{"bard", "sorcerer", "warlock", "wizard"}
	heal  = []string{"bard", "cleric", "druid", "paladin", "ranger"}
	divin = []string{"cleric", "paladin"}
)

func cls(c ...string) []string { return c }

// Spells is the spell list: cantrips through 3rd level.
var Spells = map[string]*SpellDef{
	// Cantrips
	"fire_bolt":        {Name: "Fire Bolt", School: "evocation", Classes: arc, Range: RangeFar, Kind: SpellAttack, Dice: "1d10", Desc: "A mote of fire. 1d10 fire (2d10 at level 5).", Era: &EraNames{"Galvanic Spark", "Hellfire Sigil", "Incendiary Hack", "Plasma Bolt"}},
	"eldritch_blast":   {Name: "Eldritch Blast", School: "evocation", Classes: cls("warlock"), Range: RangeFar, Kind: SpellAttack, Dice: "1d10", Desc: "A beam of crackling force, 1d10; two beams at level 5.", Era: &EraNames{"Infernal Bolt", "Void Lash", "Black Signal", "Void Beam"}},
	"sacred_flame":     {Name: "Sacred Flame", School: "evocation", Classes: cls("cleric"), Range: RangeFar, Kind: SpellSave, Save: DEX, Dice: "1d8", Desc: "Radiance falls on a foe: DEX save or 1d8 radiant (2d8 at level 5).", Era: &EraNames{"Holy Fire", "Pillar of Light", "Searing Light", "Solar Flare"}},
	"vicious_mockery":  {Name: "Vicious Mockery", School: "enchantment", Classes: cls("bard"), Range: RangeFar, Kind: SpellSave, Save: WIS, Dice: "1d6", Cond: CondMocked, Desc: "An insult laced with magic: WIS save or 1d6 psychic and disadvantage on its next attack.", Era: &EraNames{"Cutting Remark", "Wisecrack", "Roast", "Psychic Jab"}},
	"ray_of_frost":     {Name: "Ray of Frost", School: "evocation", Classes: arc, Range: RangeFar, Kind: SpellAttack, Dice: "1d8", Desc: "A frigid beam, 1d8 cold.", Era: &EraNames{"Ether Chill", "Grave Cold", "Liquid Nitrogen", "Cryo Ray"}},
	"shocking_grasp":   {Name: "Shocking Grasp", School: "evocation", Classes: arc, Range: RangeTouch, Kind: SpellAttack, Dice: "1d8", Desc: "Lightning from your hand, 1d8; the foe can't take reactions.", Era: &EraNames{"Voltaic Touch", "Live Wire", "Stun Touch", "Arc Palm"}},
	"toll_the_dead":    {Name: "Toll the Dead", School: "necromancy", Classes: cls("cleric", "warlock", "wizard"), Range: RangeFar, Kind: SpellSave, Save: WIS, Dice: "1d8", Desc: "A dolorous bell: WIS save or 1d8 necrotic (1d12 if it is wounded).", Era: &EraNames{"Funeral Knell", "Death Knell", "Sonic Pulse", "Neural Spike"}},
	"produce_flame":    {Name: "Produce Flame", School: "conjuration", Classes: cls("druid"), Range: RangeFar, Kind: SpellAttack, Dice: "1d8", Desc: "Hurl a flame from your palm, 1d8 fire.", Era: &EraNames{"Will-o'-Wisp", "Swamp Fire", "Molotov", "Thermal Lance"}},
	"poison_spray":     {Name: "Poison Spray", School: "necromancy", Classes: cls("druid", "sorcerer", "warlock", "wizard"), Range: RangeNear, Kind: SpellAttack, Dice: "1d12", Desc: "A puff of noxious gas, 1d12 poison.", Era: &EraNames{"Miasma", "Corpse Gas", "Pepper Spray", "Toxin Mist"}},
	"acid_splash":      {Name: "Acid Splash", School: "evocation", Classes: arc, Range: RangeFar, Kind: SpellSave, Area: true, Save: DEX, Dice: "1d6", Desc: "A bubble of acid bursts over a zone: DEX save or 1d6 acid.", Era: &EraNames{"Vitriol Flask", "Acid Phial", "Acid Grenade", "Corrosive Burst"}},
	"guidance":         {Name: "Guidance", School: "divination", Classes: cls("cleric", "druid"), Range: RangeTouch, Kind: SpellBuff, Cond: CondGuided, Desc: "An ally adds 1d4 to their next ability check.", Era: &EraNames{"Blessing", "Lucky Charm", "Pep Talk", "Tactical Overlay"}},
	"spare_the_dying":  {Name: "Spare the Dying", School: "necromancy", Classes: cls("cleric", "druid"), Range: RangeNear, Kind: SpellSpecial, Desc: "A dying ally becomes stable.", Era: &EraNames{"Last Rites", "Smelling Salts", "Stabilize", "Stasis Field"}},
	"shillelagh":       {Name: "Shillelagh", School: "transmutation", Classes: cls("druid"), Cast: SlotBonus, Range: RangeSelf, Kind: SpellBuff, Cond: CondShillelagh, Desc: "Your staff or club uses WIS and a d8 until the fight ends.", Era: &EraNames{"Ironwood Staff", "Hoodoo Cane", "Weighted Bat", "Kinetic Staff"}},
	"light":            {Name: "Light", School: "evocation", Classes: cls("bard", "cleric", "sorcerer", "wizard"), Range: RangeTouch, Kind: SpellUtility, Desc: "An object glows like a torch.", Era: &EraNames{"Phosphor Glow", "Witch Light", "Glow Stick", "Lumen Field"}},
	"mage_hand":        {Name: "Mage Hand", School: "conjuration", Classes: arcB, Range: RangeNear, Kind: SpellUtility, Desc: "A spectral hand moves small things at a distance.", Era: &EraNames{"Spirit Hand", "Ghost Hand", "Drone", "Manipulator Drone"}},
	"minor_illusion":   {Name: "Minor Illusion", School: "illusion", Classes: arcB, Range: RangeFar, Kind: SpellUtility, Desc: "A sound or a small image that isn't really there.", Era: &EraNames{"Phantasmagoria", "Phantom Image", "Hologram", "Holo-Decoy"}},
	"prestidigitation": {Name: "Prestidigitation", School: "transmutation", Classes: arcB, Range: RangeTouch, Kind: SpellUtility, Desc: "Small magical tricks: clean, warm, flavor, spark.", Era: &EraNames{"Parlor Trick", "Sleight", "Life Hack", "Nanotool"}},
	"thaumaturgy":      {Name: "Thaumaturgy", School: "transmutation", Classes: cls("cleric"), Range: RangeNear, Kind: SpellUtility, Desc: "A booming voice, flickering flames, tremors underfoot.", Era: &EraNames{"Fire and Brimstone", "Omen", "Feedback", "Atmospheric Control"}},
	"druidcraft":       {Name: "Druidcraft", School: "transmutation", Classes: cls("druid"), Range: RangeNear, Kind: SpellUtility, Desc: "Small nature magic: bloom a flower, predict the weather.", Era: &EraNames{"Hedge Craft", "Root Work", "Green Thumb", "Biotech Touch"}},

	// 1st level
	"magic_missile":        {Name: "Magic Missile", Level: 1, School: "evocation", Classes: arc, Range: RangeFar, Kind: SpellSpecial, Dice: "1d4+1", Desc: "Three darts that never miss, 1d4+1 force each (+1 dart per slot level above 1st).", Era: &EraNames{"Galvanic Darts", "Hungry Motes", "Smart Darts", "Seeker Swarm"}},
	"burning_hands":        {Name: "Burning Hands", Level: 1, School: "evocation", Classes: arc, Range: RangeSelf, Kind: SpellSave, Area: true, Save: DEX, Half: true, Dice: "3d6", Upcast: "1d6", Desc: "Flame sprays from your hands: foes in your zone take 3d6 fire, DEX save for half.", Era: &EraNames{"Gout of Flame", "Brimstone Breath", "Flamethrower", "Plasma Spray"}},
	"thunderwave":          {Name: "Thunderwave", School: "evocation", Level: 1, Classes: cls("bard", "druid", "sorcerer", "wizard"), Range: RangeSelf, Kind: SpellSave, Area: true, Save: CON, Half: true, Dice: "2d8", Upcast: "1d8", Desc: "A wave of thunder: foes in your zone take 2d8, CON save for half; those who fail are pushed a zone away.", Era: &EraNames{"Concussive Blast", "Thunderclap", "Concussion Grenade", "Shockwave"}},
	"cure_wounds":          {Name: "Cure Wounds", Level: 1, School: "abjuration", Classes: heal, Range: RangeTouch, Kind: SpellHeal, Dice: "2d8", Upcast: "2d8", Desc: "Heal an ally 2d8 + your casting modifier.", Era: &EraNames{"Laying On of Hands", "Patch Up", "Field Dressing", "Nano-Repair"}},
	"healing_word":         {Name: "Healing Word", Level: 1, School: "abjuration", Classes: cls("bard", "cleric", "druid"), Cast: SlotBonus, Range: RangeFar, Kind: SpellHeal, Dice: "2d4", Upcast: "2d4", Desc: "Bonus action: an ally you can see heals 2d4 + your casting modifier.", Era: &EraNames{"Word of Comfort", "Hang In There", "Adrenaline Shot", "Remote Stim"}},
	"bless":                {Name: "Bless", Level: 1, School: "enchantment", Classes: divin, Range: RangeNear, Kind: SpellBuff, Cond: CondBlessed, Conc: true, Targets: 3, Desc: "Up to three allies add 1d4 to attacks and saves.", Era: &EraNames{"Benediction", "Saint's Favor", "Rally", "Combat Stims"}},
	"shield_of_faith":      {Name: "Shield of Faith", Level: 1, School: "abjuration", Classes: divin, Cast: SlotBonus, Range: RangeNear, Kind: SpellBuff, Cond: CondShielded, Conc: true, Desc: "Bonus action: an ally gets +2 AC.", Era: &EraNames{"Aegis of Faith", "Guardian Angel", "Cover Me", "Deflector Field"}},
	"guiding_bolt":         {Name: "Guiding Bolt", Level: 1, School: "evocation", Classes: cls("cleric"), Range: RangeFar, Kind: SpellAttack, Dice: "4d6", Upcast: "1d6", Cond: CondLit, Desc: "A flash of light, 4d6 radiant; the next attack against the foe has advantage.", Era: &EraNames{"Signal Flare", "Searing Light", "Laser Designator", "Target Beam"}},
	"inflict_wounds":       {Name: "Inflict Wounds", Level: 1, School: "necromancy", Classes: cls("cleric"), Range: RangeTouch, Kind: SpellSave, Save: CON, Half: true, Dice: "2d10", Upcast: "1d10", Desc: "Necrotic touch: 2d10, CON save for half.", Era: &EraNames{"Withering Touch", "Rotting Grip", "Overdose", "Necro-Toxin"}},
	"sleep":                {Name: "Sleep", Level: 1, School: "enchantment", Classes: cls("bard", "sorcerer", "wizard"), Range: RangeFar, Kind: SpellSave, Area: true, Save: WIS, Cond: CondAsleep, Conc: true, Desc: "Foes in a zone make a WIS save or fall asleep; damage wakes them.", Era: &EraNames{"Mesmerism", "Dreamer's Call", "Sleeping Gas", "Sedative Cloud"}},
	"hunters_mark":         {Name: "Hunter's Mark", Level: 1, School: "divination", Classes: cls("ranger"), Cast: SlotBonus, Range: RangeFar, Kind: SpellSpecial, Cond: CondMarked, Conc: true, Desc: "Bonus action: your hits on the marked foe deal +1d6.", Era: &EraNames{"Tracker's Mark", "Marked Man", "Laser Sight", "Target Lock"}},
	"hex":                  {Name: "Hex", Level: 1, School: "enchantment", Classes: cls("warlock"), Cast: SlotBonus, Range: RangeFar, Kind: SpellSpecial, Cond: CondMarked, Conc: true, Desc: "Bonus action: your hits on the cursed foe deal +1d6 necrotic.", Era: &EraNames{"Evil Eye", "Black Curse", "Hex", "Malware"}},
	"mage_armor":           {Name: "Mage Armor", Level: 1, School: "abjuration", Classes: arc, Range: RangeTouch, Kind: SpellBuff, Cond: CondMageArmor, Desc: "Without armor, AC is 13 + DEX until your next long rest.", Era: &EraNames{"Ether Shroud", "Warding Glyph", "Force Field", "Personal Shield"}},
	"faerie_fire":          {Name: "Faerie Fire", Level: 1, School: "evocation", Classes: cls("bard", "druid"), Range: RangeFar, Kind: SpellSave, Area: true, Save: DEX, Cond: CondOutlined, Conc: true, Desc: "Foes in a zone make a DEX save or glow: attacks against them have advantage.", Era: &EraNames{"Phosphor Dust", "Corpse Light", "Paint Rounds", "Tracer Dust"}},
	"entangle":             {Name: "Entangle", Level: 1, School: "conjuration", Classes: cls("druid", "ranger"), Range: RangeFar, Kind: SpellSave, Area: true, Save: STR, Cond: CondRestrained, Conc: true, Desc: "Grasping weeds: foes in a zone make a STR save or are restrained.", Era: &EraNames{"Grasping Briars", "Swamp Roots", "Glue Trap", "Nanofiber Net"}},
	"command":              {Name: "Command", Level: 1, School: "enchantment", Classes: cls("bard", "cleric", "paladin"), Range: RangeFar, Kind: SpellSave, Save: WIS, Cond: CondProne, Desc: "One word, \"Grovel!\": WIS save or the foe falls prone.", Era: &EraNames{"Voice of Command", "Compulsion", "Drop It!", "Neural Command"}},
	"dissonant_whispers":   {Name: "Dissonant Whispers", Level: 1, School: "enchantment", Classes: cls("bard"), Range: RangeFar, Kind: SpellSave, Save: WIS, Half: true, Dice: "3d6", Upcast: "1d6", Cond: CondFrightened, Desc: "A maddening melody: WIS save or 3d6 psychic and frightened; half on a save.", Era: &EraNames{"Madness Whisper", "Unspeakable Words", "Sonic Static", "Psi Scream"}},
	"chromatic_orb":        {Name: "Chromatic Orb", Level: 1, School: "evocation", Classes: arc, Range: RangeFar, Kind: SpellAttack, Dice: "3d8", Upcast: "1d8", Desc: "An orb of raw energy, 3d8.", Era: &EraNames{"Ball Lightning", "Eldritch Orb", "Energy Round", "Plasma Orb"}},
	"armor_of_agathys":     {Name: "Armor of Agathys", Level: 1, School: "abjuration", Classes: cls("warlock"), Cast: SlotBonus, Range: RangeSelf, Kind: SpellSpecial, Cond: CondAgathys, Desc: "Bonus action: 5 temporary HP (+5 per slot level above 1st); while they last, melee attackers take that much cold.", Era: &EraNames{"Frost Mantle", "Shroud of Ice", "Cryo Armor", "Cryo Barrier"}},
	"charm_person":         {Name: "Charm Person", Level: 1, School: "enchantment", Classes: cls("bard", "druid", "sorcerer", "warlock", "wizard"), Range: RangeFar, Kind: SpellUtility, Desc: "A person regards you as a friend for an hour.", Era: &EraNames{"Animal Magnetism", "Glamour", "Charm Offensive", "Pheromone Spray"}},
	"detect_magic":         {Name: "Detect Magic", Level: 1, School: "divination", Classes: cls("bard", "cleric", "druid", "paladin", "ranger", "sorcerer", "wizard"), Range: RangeSelf, Kind: SpellUtility, Desc: "Sense magic nearby.", Era: &EraNames{"Ether Sense", "Second Sight", "EMF Reader", "Scanner Sweep"}},
	"disguise_self":        {Name: "Disguise Self", Level: 1, School: "illusion", Classes: cls("bard", "sorcerer", "wizard"), Range: RangeSelf, Kind: SpellUtility, Desc: "Look like someone else for an hour.", Era: &EraNames{"Master of Disguise", "Borrowed Face", "Deepfake", "Holo-Mask"}},
	"speak_with_animals":   {Name: "Speak with Animals", Level: 1, School: "divination", Classes: cls("bard", "druid", "ranger"), Range: RangeSelf, Kind: SpellUtility, Desc: "Talk with beasts for ten minutes.", Era: &EraNames{"Animal Tongue", "Beast Speech", "Animal Whisperer", "Xeno-Translator"}},
	"comprehend_languages": {Name: "Comprehend Languages", Level: 1, School: "divination", Classes: arcB, Range: RangeSelf, Kind: SpellUtility, Desc: "Understand any language you hear or read.", Era: &EraNames{"Polyglot", "Rosetta Sight", "Translation App", "Universal Translator"}},
	"divine_smite":         {Name: "Divine Smite", Level: 1, School: "evocation", Classes: cls("paladin"), Cast: SlotBonus, Range: RangeTouch, Kind: SpellSpecial, Dice: "2d8", Upcast: "1d8", Desc: "When you hit with a melee weapon: +2d8 radiant (+1d8 per slot level above 1st). Pick Smite when you attack.", Era: &EraNames{"Righteous Strike", "Holy Strike", "Power Strike", "Overcharge Strike"}},

	// 2nd level
	"scorching_ray":      {Name: "Scorching Ray", Level: 2, School: "evocation", Classes: arc, Range: RangeFar, Kind: SpellSpecial, Dice: "2d6", Desc: "Three rays, each a spell attack for 2d6 fire (+1 ray per slot level above 2nd).", Era: &EraNames{"Galvanic Arcs", "Hellfire Rays", "Incendiary Burst", "Laser Barrage"}},
	"shatter":            {Name: "Shatter", Level: 2, School: "evocation", Classes: arcB, Range: RangeFar, Kind: SpellSave, Area: true, Save: CON, Half: true, Dice: "3d8", Upcast: "1d8", Desc: "A ringing blast in a zone: 3d8 thunder, CON save for half.", Era: &EraNames{"Sonic Detonation", "Shattering Note", "Flashbang", "Sonic Grenade"}},
	"hold_person":        {Name: "Hold Person", Level: 2, School: "enchantment", Classes: cls("bard", "cleric", "druid", "sorcerer", "warlock", "wizard"), Range: RangeFar, Kind: SpellSave, Save: WIS, Cond: CondParalyzed, Conc: true, Desc: "WIS save or the foe is paralyzed (save again each turn).", Era: &EraNames{"Mesmeric Hold", "Paralysis Hex", "Tranquilizer", "Neural Lock"}},
	"misty_step":         {Name: "Misty Step", Level: 2, School: "conjuration", Classes: arcW, Cast: SlotBonus, Range: RangeSelf, Kind: SpellSpecial, Desc: "Bonus action: vanish and reappear in any zone here, with no opportunity attacks.", Era: &EraNames{"Vanishing Act", "Shadow Step", "Smoke Bomb", "Blink Drive"}},
	"spiritual_weapon":   {Name: "Spiritual Weapon", Level: 2, School: "evocation", Classes: cls("cleric"), Cast: SlotBonus, Range: RangeFar, Kind: SpellSpecial, Dice: "1d8", Upcast: "1d8", Conc: true, Desc: "Bonus action: a floating weapon strikes a foe (spell attack, 1d8 + casting modifier) and can strike again as a bonus action each turn.", Era: &EraNames{"Avenging Spirit", "Phantom Blade", "Attack Drone", "Combat Drone"}},
	"prayer_of_healing":  {Name: "Prayer of Healing", Level: 2, School: "abjuration", Classes: cls("cleric"), Range: RangeNear, Kind: SpellHeal, Dice: "2d8", Upcast: "1d8", Targets: 6, Peace: true, Desc: "Out of a fight: the party heals 2d8 + your casting modifier each.", Era: &EraNames{"Vespers", "Novena", "Triage", "Medbay Protocol"}},
	"lesser_restoration": {Name: "Lesser Restoration", Level: 2, School: "abjuration", Classes: heal, Range: RangeTouch, Kind: SpellSpecial, Desc: "End poison, blindness or paralysis on an ally.", Era: &EraNames{"Purging Draught", "Cleansing", "Antidote", "Detox Nanites"}},
	"aid":                {Name: "Aid", Level: 2, School: "abjuration", Classes: cls("bard", "cleric", "druid", "paladin", "ranger"), Range: RangeNear, Kind: SpellSpecial, Targets: 3, Desc: "Up to three allies gain 5 temporary HP (+5 per slot level above 2nd).", Era: &EraNames{"Fortitude", "Second Wind", "Energy Drink", "Booster Shot"}},
	"blindness_deafness": {Name: "Blindness/Deafness", Level: 2, School: "transmutation", Classes: cls("bard", "cleric", "sorcerer", "wizard"), Range: RangeFar, Kind: SpellSave, Save: CON, Cond: CondBlinded, Desc: "CON save or the foe is blinded (save again each turn).", Era: &EraNames{"Ether Blindness", "Curse of Darkness", "Strobe", "Optic Scramble"}},
	"invisibility":       {Name: "Invisibility", Level: 2, School: "illusion", Classes: arcB, Range: RangeTouch, Kind: SpellBuff, Cond: CondInvisible, Conc: true, Desc: "An ally turns invisible until they attack or cast.", Era: &EraNames{"Vanishing", "Unseen", "Optical Camo", "Cloaking Field"}},
	"moonbeam":           {Name: "Moonbeam", Level: 2, School: "evocation", Classes: cls("druid"), Range: RangeFar, Kind: SpellSave, Area: true, Save: CON, Half: true, Dice: "2d10", Upcast: "1d10", Conc: true, Desc: "A beam of silver light over a zone: 2d10 radiant, CON save for half, again at the start of each of your turns.", Era: &EraNames{"Moonlight Shaft", "Pale Beam", "Searchlight", "Orbital Beam"}},
	"web":                {Name: "Web", Level: 2, School: "conjuration", Classes: arc, Range: RangeFar, Kind: SpellSave, Area: true, Save: DEX, Cond: CondRestrained, Conc: true, Desc: "Sticky webbing fills a zone: DEX save or restrained.", Era: &EraNames{"Spider Silk", "Webbing", "Foam Trap", "Gel Net"}},
	"knock":              {Name: "Knock", Level: 2, School: "transmutation", Classes: cls("bard", "sorcerer", "wizard"), Range: RangeNear, Kind: SpellSpecial, Desc: "A locked door, gate or chest here springs open.", Era: &EraNames{"Open Sesame", "Unbarring", "Bypass", "Override"}},
	"suggestion":         {Name: "Suggestion", Level: 2, School: "enchantment", Classes: arcB, Range: RangeNear, Kind: SpellUtility, Desc: "Suggest a reasonable course of action; the target is inclined to follow it.", Era: &EraNames{"Mesmeric Suggestion", "Whispered Command", "Persuasion Hack", "Neural Nudge"}},

	// 3rd level
	"fireball":          {Name: "Fireball", Level: 3, School: "evocation", Classes: arc, Range: RangeFar, Kind: SpellSave, Area: true, Save: DEX, Half: true, Dice: "8d6", Upcast: "1d6", Desc: "A bead of fire explodes over a zone: 8d6 fire, DEX save for half.", Era: &EraNames{"Nitro Bomb", "Hellfire", "Incendiary Grenade", "Plasma Grenade"}},
	"lightning_bolt":    {Name: "Lightning Bolt", Level: 3, School: "evocation", Classes: arc, Range: RangeFar, Kind: SpellSave, Area: true, Save: DEX, Half: true, Dice: "8d6", Upcast: "1d6", Desc: "A stroke of lightning through a zone: 8d6, DEX save for half.", Era: &EraNames{"Tesla Coil", "Storm Call", "Power Line", "Arc Cannon"}},
	"mass_healing_word": {Name: "Mass Healing Word", Level: 3, School: "abjuration", Classes: cls("bard", "cleric"), Cast: SlotBonus, Range: RangeFar, Kind: SpellHeal, Dice: "2d4", Upcast: "1d4", Targets: 6, Desc: "Bonus action: up to six allies heal 2d4 + your casting modifier.", Era: &EraNames{"Rousing Hymn", "Rally Cry", "Mass Triage", "Nanite Broadcast"}},
	"revivify":          {Name: "Revivify", Level: 3, School: "necromancy", Classes: cls("cleric", "druid", "paladin", "ranger"), Range: RangeTouch, Kind: SpellSpecial, Desc: "An ally who died here returns with 1 HP. Costs 300 gold of diamonds.", Era: &EraNames{"Lazarus Rite", "Resurrectionist's Rite", "Defibrillator", "Revival Pod"}},
	"spirit_guardians":  {Name: "Spirit Guardians", Level: 3, School: "conjuration", Classes: cls("cleric"), Range: RangeSelf, Kind: SpellSpecial, Save: WIS, Dice: "3d8", Upcast: "1d8", Conc: true, Desc: "Spirits circle you: foes in your zone take 3d8 radiant (WIS save for half) now and at the start of each of your turns.", Era: &EraNames{"Guardian Angels", "Host of Saints", "Sentry Drones", "Defense Swarm"}},
	"hypnotic_pattern":  {Name: "Hypnotic Pattern", Level: 3, School: "illusion", Classes: cls("bard", "sorcerer", "warlock", "wizard"), Range: RangeFar, Kind: SpellSave, Area: true, Save: WIS, Cond: CondAsleep, Conc: true, Desc: "Swirling colors: foes in a zone make a WIS save or stand entranced; damage snaps them out.", Era: &EraNames{"Kaleidoscope", "Hypnotic Spiral", "Strobe Pattern", "Neural Pattern"}},
	"fear":              {Name: "Fear", Level: 3, School: "illusion", Classes: cls("bard", "sorcerer", "warlock", "wizard"), Range: RangeFar, Kind: SpellSave, Area: true, Save: WIS, Cond: CondFrightened, Conc: true, Desc: "A vision of terror: foes in a zone make a WIS save or are frightened.", Era: &EraNames{"Night Terrors", "The Fear", "Panic Attack", "Terror Broadcast"}},
	"haste":             {Name: "Haste", Level: 3, School: "transmutation", Classes: arc, Range: RangeNear, Kind: SpellBuff, Cond: CondHasted, Conc: true, Desc: "An ally gets +2 AC and an extra attack each turn.", Era: &EraNames{"Quicksilver", "Lightning Reflexes", "Overclock", "Reflex Booster"}},
	"dispel_magic":      {Name: "Dispel Magic", Level: 3, School: "abjuration", Classes: cls("bard", "cleric", "druid", "paladin", "sorcerer", "warlock", "wizard"), Range: RangeFar, Kind: SpellSpecial, Desc: "End magical effects on a creature or object.", Era: &EraNames{"Banishing", "Exorcism", "System Reset", "EMP"}},
	"fly":               {Name: "Fly", Level: 3, School: "transmutation", Classes: arcW, Range: RangeTouch, Kind: SpellUtility, Desc: "An ally can fly for ten minutes.", Era: &EraNames{"Levitation", "Wings of Night", "Jetpack", "Grav Boots"}},
	"speak_with_dead":   {Name: "Speak with Dead", Level: 3, School: "necromancy", Classes: cls("bard", "cleric", "wizard"), Range: RangeTouch, Kind: SpellUtility, Desc: "A corpse answers five questions.", Era: &EraNames{"Séance", "Necromancy", "Forensic Read", "Memory Extraction"}},
	"call_lightning":    {Name: "Call Lightning", Level: 3, School: "conjuration", Classes: cls("druid"), Range: RangeFar, Kind: SpellSave, Area: true, Save: DEX, Half: true, Dice: "3d10", Upcast: "1d10", Desc: "Lightning strikes a zone: 3d10, DEX save for half.", Era: &EraNames{"Thunderhead", "Storm Wrath", "Lightning Rod", "Orbital Strike"}},
}

// spellOrder is every spell key sorted by level then name.
var spellOrder []string

func init() {
	for k, s := range Spells {
		s.Key = k
		if s.Cast == "" {
			s.Cast = SlotAction
		}
		spellOrder = append(spellOrder, k)
	}
	sort.Slice(spellOrder, func(i, j int) bool {
		a, b := Spells[spellOrder[i]], Spells[spellOrder[j]]
		if a.Level != b.Level {
			return a.Level < b.Level
		}
		return a.Name < b.Name
	})
}

// SpellKeys returns every spell key, by level then name.
func SpellKeys() []string { return append([]string(nil), spellOrder...) }

// SpellName is the spell in setting.
func SpellName(s *SpellDef, setting string) string { return eraName(setting, s.Name, s.Era) }

// ClassSpells lists the spell keys on a class's list up to maxLevel.
func ClassSpells(class string, maxLevel int) []string {
	var out []string
	for _, k := range spellOrder {
		s := Spells[k]
		if s.Level > maxLevel {
			continue
		}
		for _, c := range s.Classes {
			if c == class {
				out = append(out, k)
				break
			}
		}
	}
	return out
}

// slot tables to level 5: index level-1, then slots of 1st, 2nd, 3rd.
var (
	fullSlots = [5][3]int{{2, 0, 0}, {3, 0, 0}, {4, 2, 0}, {4, 3, 0}, {4, 3, 2}}
	halfSlots = [5][3]int{{2, 0, 0}, {2, 0, 0}, {3, 0, 0}, {3, 0, 0}, {4, 2, 0}}
	// pact: number of slots and their level
	pactSlots = [5][2]int{{1, 1}, {2, 1}, {2, 2}, {2, 2}, {2, 3}}
)

// cantripsKnown and preparedCount per class by level.
var cantripsKnown = map[string][5]int{
	"bard": {2, 2, 2, 3, 3}, "cleric": {3, 3, 3, 4, 4}, "druid": {2, 2, 2, 3, 3},
	"sorcerer": {4, 4, 4, 5, 5}, "warlock": {2, 2, 2, 3, 3}, "wizard": {3, 3, 3, 4, 4},
}
var preparedCount = map[string][5]int{
	"bard": {4, 5, 6, 7, 9}, "cleric": {4, 5, 6, 7, 9}, "druid": {4, 5, 6, 7, 9}, "wizard": {4, 5, 6, 7, 9},
	"sorcerer": {2, 4, 6, 7, 9}, "warlock": {2, 3, 4, 5, 6}, "paladin": {2, 3, 4, 5, 6}, "ranger": {2, 3, 4, 5, 6},
}

func lvlIdx(level int) int { return clamp(level, 1, MaxLevel) - 1 }

// SlotsMax returns c's spell slots of each level 1..3 (index 0 = 1st).
func SlotsMax(c *Character) [3]int {
	switch ClassOf(c).Caster {
	case CasterFull:
		return fullSlots[lvlIdx(c.Level)]
	case CasterHalf:
		return halfSlots[lvlIdx(c.Level)]
	case CasterPact:
		var s [3]int
		p := pactSlots[lvlIdx(c.Level)]
		s[p[1]-1] = p[0]
		return s
	}
	return [3]int{}
}

// MaxSpellLevel is the highest slot level c has.
func MaxSpellLevel(c *Character) int {
	m := SlotsMax(c)
	for i := 2; i >= 0; i-- {
		if m[i] > 0 {
			return i + 1
		}
	}
	return 0
}

// CantripCount and PreparedCount are how many cantrips and leveled spells
// c's class knows at their level.
func CantripCount(c *Character) int {
	if v, ok := cantripsKnown[ClassOf(c).Key]; ok {
		return v[lvlIdx(c.Level)]
	}
	return 0
}
func PreparedCount(c *Character) int {
	if v, ok := preparedCount[ClassOf(c).Key]; ok {
		return v[lvlIdx(c.Level)]
	}
	return 0
}

// SlotsLeft returns unspent slots of each level.
func SlotsLeft(c *Character) [3]int {
	m := SlotsMax(c)
	for i := range m {
		if i < len(c.SlotsUsed) {
			m[i] = max(0, m[i]-c.SlotsUsed[i])
		}
	}
	return m
}

// spendSlot uses one slot of level lvl.
func spendSlot(c *Character, lvl int) bool {
	if lvl < 1 || lvl > 3 || SlotsLeft(c)[lvl-1] == 0 {
		return false
	}
	for len(c.SlotsUsed) < 3 {
		c.SlotsUsed = append(c.SlotsUsed, 0)
	}
	c.SlotsUsed[lvl-1]++
	return true
}

// lowestSlot is the lowest slot level of at least min that c has left, or 0.
func lowestSlot(c *Character, minLevel int) int {
	left := SlotsLeft(c)
	for l := max(1, minLevel); l <= 3; l++ {
		if left[l-1] > 0 {
			return l
		}
	}
	return 0
}

// SpellList is the spells c can cast: chosen class spells plus those from
// species and feats. Sheets from before spells know their class's
// defaults.
func (c *Character) SpellList() []string {
	var out []string
	seen := map[string]bool{}
	add := func(ks ...string) {
		for _, k := range ks {
			if Spells[k] != nil && !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	if len(c.Spells) == 0 && c.ClassKey == "" {
		add(DefaultSpells(c)...)
	} else {
		add(c.Spells...)
	}
	switch c.SpeciesKey() {
	case "tiefling":
		add("fire_bolt", "thaumaturgy")
	case "elf":
		add(c.Picks["elf_cantrip"]...)
	}
	add(c.Picks["mi_cantrips"]...)
	add(c.Picks["mi_spell"]...)
	if c.HasFeature("favored_enemy") {
		add("hunters_mark")
	}
	if c.HasFeature("divine_smite") {
		add("divine_smite")
	}
	return out
}

// KnowsSpell reports whether c can cast key.
func (c *Character) KnowsSpell(key string) bool {
	for _, k := range c.SpellList() {
		if k == key {
			return true
		}
	}
	return false
}

// spellFavorites order the defaults a class picks first.
var spellFavorites = map[string][]string{
	// leveled picks are in priority order across levels, so a level 5
	// caster gets their best 3rd-level spells before more 1st-level ones
	"bard":     {"vicious_mockery", "minor_illusion", "mage_hand", "healing_word", "dissonant_whispers", "hypnotic_pattern", "shatter", "faerie_fire", "hold_person", "mass_healing_word", "charm_person", "invisibility", "suggestion", "fear"},
	"cleric":   {"sacred_flame", "guidance", "toll_the_dead", "spare_the_dying", "cure_wounds", "healing_word", "bless", "guiding_bolt", "spirit_guardians", "spiritual_weapon", "revivify", "shield_of_faith", "aid", "lesser_restoration", "hold_person", "mass_healing_word"},
	"druid":    {"produce_flame", "shillelagh", "guidance", "cure_wounds", "healing_word", "entangle", "faerie_fire", "call_lightning", "moonbeam", "thunderwave", "revivify", "lesser_restoration", "hold_person", "dispel_magic"},
	"sorcerer": {"fire_bolt", "ray_of_frost", "shocking_grasp", "mage_hand", "magic_missile", "burning_hands", "scorching_ray", "fireball", "misty_step", "haste", "hold_person", "lightning_bolt"},
	"warlock":  {"eldritch_blast", "minor_illusion", "toll_the_dead", "hex", "armor_of_agathys", "hold_person", "hypnotic_pattern", "misty_step", "charm_person", "shatter", "invisibility", "fear"},
	"wizard":   {"fire_bolt", "mage_hand", "ray_of_frost", "shocking_grasp", "magic_missile", "mage_armor", "sleep", "thunderwave", "fireball", "scorching_ray", "misty_step", "haste", "web", "burning_hands", "detect_magic", "hold_person", "knock", "lightning_bolt", "hypnotic_pattern"},
	"paladin":  {"bless", "cure_wounds", "shield_of_faith", "command", "aid", "lesser_restoration", "revivify"},
	"ranger":   {"cure_wounds", "entangle", "speak_with_animals", "aid", "lesser_restoration"},
}

// DefaultSpells picks c's cantrips and prepared spells from the class's
// favorites, for drafts and for sheets made before spells.
func DefaultSpells(c *Character) []string {
	cd := ClassOf(c)
	favs := spellFavorites[cd.Key]
	maxL := MaxSpellLevel(c)
	var out []string
	cantrips, leveled := CantripCount(c), PreparedCount(c)
	for _, k := range favs {
		s := Spells[k]
		if s == nil || s.Level > maxL || autoGranted(c, k) {
			continue
		}
		if s.Level == 0 && cantrips > 0 {
			out = append(out, k)
			cantrips--
		} else if s.Level > 0 && leveled > 0 {
			out = append(out, k)
			leveled--
		}
	}
	return out
}

// autoGranted spells come with a feature and don't use a choice.
func autoGranted(c *Character, key string) bool {
	return (key == "hunters_mark" && c.HasFeature("favored_enemy")) || (key == "divine_smite" && c.HasFeature("divine_smite"))
}

// ValidateSpells checks a class spell choice for c.
func ValidateSpells(c *Character, keys []string) Problems {
	var p Problems
	cd := ClassOf(c)
	maxL := MaxSpellLevel(c)
	cantrips, leveled := 0, 0
	seen := map[string]bool{}
	for _, k := range keys {
		s := Spells[k]
		if s == nil {
			p = append(p, "unknown spell "+k)
			continue
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		onList := false
		for _, cl := range s.Classes {
			onList = onList || cl == cd.Key
		}
		switch {
		case !onList:
			p = append(p, s.Name+" is not on the "+cd.Name+" list")
		case s.Level > maxL:
			p = append(p, s.Name+" is too high a level")
		case s.Level == 0:
			cantrips++
		default:
			leveled++
		}
	}
	if cantrips > CantripCount(c) {
		p = append(p, "too many cantrips")
	}
	if leveled > PreparedCount(c) {
		p = append(p, "too many prepared spells")
	}
	return p
}
