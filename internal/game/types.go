// Package game is the pure rules engine: types, rules, the world graph,
// quests, trade, turns, zones, validation and the item catalog. It does no
// I/O. Every change to game state goes through deterministic code here.
package game

import (
	"fmt"
	"strings"
)

// Ability is one of the six ability scores.
type Ability string

const (
	STR Ability = "STR"
	DEX Ability = "DEX"
	CON Ability = "CON"
	INT Ability = "INT"
	WIS Ability = "WIS"
	CHA Ability = "CHA"
)

// Abilities lists the six abilities in sheet order.
var Abilities = []Ability{STR, DEX, CON, INT, WIS, CHA}

// StandardArray must be assigned to the six abilities, each value once.
var StandardArray = []int{15, 14, 13, 12, 10, 8}

// ParseAbility accepts "str", "Strength", "DEX" and so on.
func ParseAbility(s string) (Ability, bool) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if len(s) >= 3 {
		s = s[:3]
	}
	for _, a := range Abilities {
		if string(a) == s {
			return a, true
		}
	}
	return "", false
}

// Scores maps each ability to its score.
type Scores map[Ability]int

// Mod is the 5e ability modifier, floor((score-10)/2).
func Mod(score int) int {
	d := score - 10
	if d < 0 {
		return (d - 1) / 2
	}
	return d / 2
}

// Campaign status values.
const (
	StatusLobby     = "lobby"
	StatusActive    = "active"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

// Player is a seat at the table, keyed by the host app's user id.
type Player struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	CharacterID string `json:"character_id,omitempty"`
	Token       string `json:"-"` // open dev mode bearer token
	JoinedTurn  int    `json:"joined_turn"`
	// AutoRoll lets the engine roll this player's dice straight away. Off
	// (the default), a turn that needs their d20 waits for them to roll it.
	AutoRoll bool `json:"auto_roll,omitempty"`
}

// Character is a player character sheet.
type Character struct {
	ID         string    `json:"id"`
	PlayerID   string    `json:"player_id"`
	Name       string    `json:"name"`
	Concept    string    `json:"concept"`
	Class      string    `json:"class"` // flavor only
	Kit        string    `json:"kit"`
	Backstory  string    `json:"backstory,omitempty"`
	Pronouns   string    `json:"pronouns,omitempty"`
	Scores     Scores    `json:"scores"`
	Proficient []Ability `json:"proficient"`
	Level      int       `json:"level"`
	XP         int       `json:"xp"`
	HP         int       `json:"hp"`
	MaxHP      int       `json:"max_hp"`
	Gold       int       `json:"gold"`
	Weapon     string    `json:"weapon,omitempty"` // equipped item id
	Armor      string    `json:"armor,omitempty"`  // equipped item id
	ZoneID     string    `json:"zone_id,omitempty"`
	// Conditions are status effects (poisoned, prone...). Only the engine
	// adds them: enemy attacks, validated hazards, and their own ticks.
	Conditions Conditions `json:"conditions,omitempty"`
	// CharExtra is the rest of the 5e sheet, stored as one JSON column.
	CharExtra
}

// CharExtra holds sheet parts added after the first schema: skills, death
// saves, reactions and readied actions.
type CharExtra struct {
	Skills    []string `json:"skills,omitempty"`    // proficient skill keys
	Expertise []string `json:"expertise,omitempty"` // skills with double proficiency
	// Death is set while the character lies at 0 HP and not yet dead.
	Death *DeathSaves `json:"death,omitempty"`
	Dead  bool        `json:"dead,omitempty"`
	// ReactedRound is the encounter round of their last reaction
	// (opportunity attack or readied attack); one per round.
	ReactedRound int `json:"reacted_round,omitempty"`
	// Readied is an attack held for when an enemy comes within reach.
	Readied *Readied `json:"readied,omitempty"`
}

// Down reports whether the character is at 0 HP (dying, stable or dead).
func (c *Character) Down() bool { return c.HP <= 0 || c.Dead }

// Mod returns the ability modifier for a.
func (c *Character) Mod(a Ability) int { return Mod(c.Scores[a]) }

// IsProficient reports proficiency in a.
func (c *Character) IsProficient(a Ability) bool {
	for _, p := range c.Proficient {
		if p == a {
			return true
		}
	}
	return false
}

// CheckMod is the d20 modifier for a check with ability a.
func (c *Character) CheckMod(a Ability) int {
	m := c.Mod(a)
	if c.IsProficient(a) {
		m += ProficiencyBonus(c.Level)
	}
	return m
}

// EdgeState is the visibility/passability of an exit.
type EdgeState string

const (
	EdgeHidden EdgeState = "hidden" // not shown to players
	EdgeKnown  EdgeState = "known"  // visible, passable, not yet used
	EdgeLocked EdgeState = "locked" // visible, needs a key or a check
	EdgeOpen   EdgeState = "open"   // visible, passable, used before
)

// Passable reports whether the party can walk through.
func (s EdgeState) Passable() bool { return s == EdgeKnown || s == EdgeOpen }

// Visible reports whether players can see the exit.
func (s EdgeState) Visible() bool { return s != EdgeHidden }

// Edge is an undirected connection between two locations. Labels and zones
// are recorded per side.
type Edge struct {
	ID       string    `json:"id"`
	A        string    `json:"a"`
	B        string    `json:"b"`
	LabelA   string    `json:"label_a,omitempty"` // how it looks from A ("a narrow tunnel")
	LabelB   string    `json:"label_b,omitempty"`
	ZoneA    string    `json:"zone_a,omitempty"` // zone in A the exit leaves from
	ZoneB    string    `json:"zone_b,omitempty"`
	State    EdgeState `json:"state"`
	KeyItem  string    `json:"key_item,omitempty"` // catalog key or item id that opens it
	LockDC   int       `json:"lock_dc,omitempty"`
	LairGate bool      `json:"lair_gate,omitempty"` // opened by main quest progress only
}

// Other returns the location on the far side from loc.
func (e *Edge) Other(loc string) string {
	if e.A == loc {
		return e.B
	}
	return e.A
}

// Touches reports whether loc is an endpoint.
func (e *Edge) Touches(loc string) bool { return e.A == loc || e.B == loc }

// ZoneAt returns the zone the exit occupies in loc.
func (e *Edge) ZoneAt(loc string) string {
	if e.A == loc {
		return e.ZoneA
	}
	return e.ZoneB
}

// LabelAt returns the label seen from loc.
func (e *Edge) LabelAt(loc string) string {
	if e.A == loc {
		return e.LabelA
	}
	return e.LabelB
}

// SetZoneAt sets the zone for loc's side.
func (e *Edge) SetZoneAt(loc, zone string) {
	if e.A == loc {
		e.ZoneA = zone
	} else if e.B == loc {
		e.ZoneB = zone
	}
}

// Zone is a named area within a location used for tactical positioning.
type Zone struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Desc     string   `json:"desc,omitempty"`
	Cover    bool     `json:"cover,omitempty"`
	Adjacent []string `json:"adjacent"`
}

// Location kinds used for generic layouts and encounter tables.
const (
	KindTown       = "town"
	KindWilderness = "wilderness"
	KindDungeon    = "dungeon"
	KindCave       = "cave"
	KindBuilding   = "building"
	KindRuin       = "ruin"
)

// Location is a node of the world graph. Stubs have only a name and hint
// until the author fills them in on first arrival.
type Location struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Hint        string `json:"hint,omitempty"`
	Description string `json:"description,omitempty"`
	Danger      int    `json:"danger"` // 0 safe .. 5 deadly
	Stub        bool   `json:"stub"`
	Visited     bool   `json:"visited"`
	IsHub       bool   `json:"is_hub,omitempty"`
	IsLair      bool   `json:"is_lair,omitempty"`
	Zones       []Zone `json:"zones,omitempty"`
	LastDay     int    `json:"last_day,omitempty"` // day the party was last here
}

// Zone looks up a zone by id or name.
func (l *Location) Zone(ref string) *Zone {
	for i := range l.Zones {
		if l.Zones[i].ID == ref {
			return &l.Zones[i]
		}
	}
	for i := range l.Zones {
		if NameMatch(l.Zones[i].Name, ref) {
			return &l.Zones[i]
		}
	}
	return nil
}

// NPC is a persistent non-player character.
type NPC struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Role        string   `json:"role,omitempty"` // "blacksmith", "elder"
	Desc        string   `json:"desc,omitempty"`
	LocationID  string   `json:"location_id"`
	ZoneID      string   `json:"zone_id,omitempty"`
	Disposition int      `json:"disposition"` // -3 hostile .. +3 devoted
	Merchant    bool     `json:"merchant,omitempty"`
	Alive       bool     `json:"alive"`
	Antagonist  bool     `json:"antagonist,omitempty"`
	Memory      []string `json:"memory,omitempty"` // short notes, newest last
	Keeps       []string `json:"keeps,omitempty"`  // memories never forgotten
	LastTalked  int      `json:"last_talked"`      // campaign turn, -1 never
	// Combat stats used if the NPC becomes an enemy.
	Stats *StatBlock `json:"stats,omitempty"`
}

// MaxNPCMemory is how many memory notes an NPC keeps.
const MaxNPCMemory = 8

// Remember appends a memory note, keeping the newest MaxNPCMemory.
func (n *NPC) Remember(note string) {
	note = strings.TrimSpace(note)
	if note == "" {
		return
	}
	if len(note) > 160 {
		note = note[:160]
	}
	n.Memory = append(n.Memory, note)
	if len(n.Memory) > MaxNPCMemory {
		n.Memory = n.Memory[len(n.Memory)-MaxNPCMemory:]
	}
}

// StatBlock is the combat profile of an enemy.
type StatBlock struct {
	MaxHP       int    `json:"max_hp"`
	AC          int    `json:"ac"`
	AttackBonus int    `json:"attack_bonus"`
	Damage      string `json:"damage"` // dice notation
	Ranged      bool   `json:"ranged,omitempty"`
	// Inflicts is a condition a hit forces a save against (see
	// NormalizeInflict); "" for none.
	Inflicts string `json:"inflicts,omitempty"`
}

// Enemy is a combatant in the current encounter.
type Enemy struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Tag   string `json:"tag,omitempty"` // distinguishing tag, "scarred one"
	NPCID string `json:"npc_id,omitempty"`
	StatBlock
	HP      int    `json:"hp"`
	ZoneID  string `json:"zone_id"`
	Subdued bool   `json:"subdued,omitempty"`
	// ActedRound is the last round this enemy took its turn in.
	ActedRound int        `json:"acted_round,omitempty"`
	Conditions Conditions `json:"conditions,omitempty"`
	EnemyExtra
}

// EnemyExtra holds enemy state added after the first schema, stored as one
// JSON column.
type EnemyExtra struct {
	// ReactedRound is the round of its last opportunity attack.
	ReactedRound int `json:"reacted_round,omitempty"`
}

// Defeated reports whether the enemy is out of the fight.
func (e *Enemy) Defeated() bool { return e.HP <= 0 || e.Subdued }

// Display is the name players use to refer to it.
func (e *Enemy) Display() string {
	if e.Tag != "" {
		return fmt.Sprintf("%s (%s)", e.Name, e.Tag)
	}
	return e.Name
}

// Encounter is an active fight.
type Encounter struct {
	ID         string   `json:"id"`
	LocationID string   `json:"location_id"`
	Enemies    []*Enemy `json:"enemies"`
	Round      int      `json:"round"`
}

// Living returns enemies still in the fight.
func (e *Encounter) Living() []*Enemy {
	var out []*Enemy
	for _, en := range e.Enemies {
		if !en.Defeated() {
			out = append(out, en)
		}
	}
	return out
}

// OwnerKind says who holds an item.
type OwnerKind string

const (
	OwnerCharacter OwnerKind = "character"
	OwnerNPC       OwnerKind = "npc"
	OwnerLocation  OwnerKind = "location"
)

// Owner identifies the single holder of an item.
type Owner struct {
	Kind OwnerKind `json:"kind"`
	ID   string    `json:"id"`
}

// Item kinds.
const (
	ItemWeapon     = "weapon"
	ItemArmor      = "armor"
	ItemShield     = "shield"
	ItemConsumable = "consumable"
	ItemKey        = "key"
	ItemGear       = "gear"
	ItemTreasure   = "treasure"
	ItemQuest      = "quest"
)

// Reach is how many zones away a weapon can strike.
type Reach int

const (
	ReachMelee  Reach = 0
	ReachThrown Reach = 1
	ReachRanged Reach = 2
)

// Item is a concrete object in the world with exactly one owner.
type Item struct {
	ID      string  `json:"id"`
	Key     string  `json:"key,omitempty"` // catalog key, empty for custom items
	Name    string  `json:"name"`
	Kind    string  `json:"kind"`
	Desc    string  `json:"desc,omitempty"`
	Value   int     `json:"value"` // gold price
	Damage  string  `json:"damage,omitempty"`
	Ability Ability `json:"ability,omitempty"` // attack ability for weapons
	Finesse bool    `json:"finesse,omitempty"`
	Reach   Reach   `json:"reach"`
	AC      int     `json:"ac,omitempty"`      // armor base AC or shield bonus
	DexCap  int     `json:"dex_cap,omitempty"` // -1 no cap, 0 no dex
	Effect  string  `json:"effect,omitempty"`  // "heal:2d4+2"
	Owner   Owner   `json:"owner"`
}

// Quest statuses.
const (
	QuestOffered   = "offered"
	QuestActive    = "active"
	QuestCompleted = "completed" // objectives done, rewards not yet claimed
	QuestTurnedIn  = "turned_in"
	QuestFailed    = "failed"
)

// Objective kinds.
const (
	ObjReachLocation = "reach_location"
	ObjDefeatEnemy   = "defeat_enemy"
	ObjObtainItem    = "obtain_item"
	ObjDeliverItem   = "deliver_item"
	ObjTalkToNPC     = "talk_to_npc"
	ObjFreeform      = "freeform"
)

// ObjectiveKinds lists all valid kinds.
var ObjectiveKinds = []string{ObjReachLocation, ObjDefeatEnemy, ObjObtainItem, ObjDeliverItem, ObjTalkToNPC, ObjFreeform}

// Objective is one step of a quest. Target meaning depends on Kind:
// location id, enemy name, item id/key/name, or npc id. DeliverTo is an npc id.
type Objective struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Text      string `json:"text"`
	Target    string `json:"target,omitempty"`
	DeliverTo string `json:"deliver_to,omitempty"`
	Where     string `json:"where,omitempty"` // location of a defeat_enemy foe that is not an NPC
	Done      bool   `json:"done"`
	Evidence  string `json:"evidence,omitempty"` // judge's quote for freeform
}

// Quest is the main storyline or a side quest.
type Quest struct {
	ID           string      `json:"id"`
	Title        string      `json:"title"`
	Summary      string      `json:"summary"`
	GiverID      string      `json:"giver_id,omitempty"` // npc holding rewards
	IsMain       bool        `json:"is_main,omitempty"`
	Status       string      `json:"status"`
	Objectives   []Objective `json:"objectives"`
	RewardGold   int         `json:"reward_gold"`
	RewardXP     int         `json:"reward_xp"`
	RewardItems  []string    `json:"reward_items,omitempty"` // item ids owned by giver
	AcceptedTurn int         `json:"accepted_turn"`
}

// Lore is a canon secret truth. Never exposed through the API.
type Lore struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Text        string   `json:"text"`
	Keywords    []string `json:"keywords,omitempty"`
	EntityIDs   []string `json:"entity_ids,omitempty"`
	AlwaysOn    bool     `json:"always_on,omitempty"`
	LastTrigger int      `json:"last_trigger"` // campaign turn, -100 never
}

// Beat trigger kinds.
const (
	TriggerEnterLocation = "enter_location"
	TriggerObjectiveDone = "objective_done"
	TriggerStalled       = "stalled"
	TriggerTension       = "tension"
	TriggerTurn          = "turn"
)

// Beat effect kinds.
const (
	EffectAmbush     = "ambush"
	EffectRevealExit = "reveal_exit"
	EffectBringNPC   = "bring_npc"
)

// Beat is a pre-written story event. Never exposed through the API.
type Beat struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Text         string   `json:"text"`
	Trigger      string   `json:"trigger"`
	Target       string   `json:"target,omitempty"` // location id / objective id
	Value        int      `json:"value,omitempty"`  // tension or turn threshold
	Effects      []string `json:"effects,omitempty"`
	EffectTarget string   `json:"effect_target,omitempty"` // edge/npc id
	Fired        bool     `json:"fired"`
	FiredTurn    int      `json:"fired_turn,omitempty"`
}

// ChronicleEntry is a fact established in play (public via /journal).
// It is the world's long-term memory: the narrator is shown the entries
// about the place and people in the scene, and open promises.
type ChronicleEntry struct {
	ID         string   `json:"id,omitempty"`
	Turn       int      `json:"turn"`
	Day        int      `json:"day,omitempty"`
	Kind       string   `json:"kind,omitempty"` // see Fact* kinds; "" is FactNote
	Text       string   `json:"text"`
	LocationID string   `json:"location_id,omitempty"`
	NPCIDs     []string `json:"npc_ids,omitempty"`
	Source     string   `json:"source,omitempty"` // "engine" or "gm"
	QuestID    string   `json:"quest_id,omitempty"`
	Status     string   `json:"status,omitempty"` // promises: open, kept, broken
	Outcome    string   `json:"outcome,omitempty"`
}

// Times of day, in order.
var TimesOfDay = []string{"dawn", "morning", "afternoon", "evening", "night"}

// Scene is the engine-owned dramatic state.
type Scene struct {
	Day       int    `json:"day"`
	Time      int    `json:"time"` // index into TimesOfDay
	Tension   int    `json:"tension"`
	Goal      string `json:"goal,omitempty"`
	Turns     int    `json:"turns"` // non-free turns in this scene
	Quiet     int    `json:"quiet"` // consecutive turns with nothing happening
	StartTurn int    `json:"start_turn"`
}

// TimeName returns the current time of day.
func (s Scene) TimeName() string { return TimesOfDay[((s.Time%5)+5)%5] }

// Unlimited marks an unbounded free-action budget.
const Unlimited = -1

// Budget is what the active character has left this turn.
type Budget struct {
	Free   int `json:"free"` // Unlimited outside combat
	Move   int `json:"move"`
	Action int `json:"action"`
}

// TurnState tracks whose turn it is.
type TurnState struct {
	Order   []string `json:"order"` // character ids in seat order
	Index   int      `json:"index"`
	Budget  Budget   `json:"budget"`
	Started int64    `json:"started"` // unix seconds the turn began
}

// DefeatRecord logs a defeated enemy for quest evaluation.
type DefeatRecord struct {
	Name       string `json:"name"`
	NPCID      string `json:"npc_id,omitempty"`
	LocationID string `json:"location_id"`
	Turn       int    `json:"turn"`
}

// Campaign is the top-level record.
type Campaign struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Premise      string    `json:"premise"`
	Pitch        string    `json:"pitch,omitempty"`
	Setting      string    `json:"setting,omitempty"` // see SettingList; "" means infer from premise
	Status       string    `json:"status"`
	HubID        string    `json:"hub_id"`
	LairID       string    `json:"lair_id"`
	AntagonistID string    `json:"antagonist_id,omitempty"`
	LocationID   string    `json:"location_id"` // where the party is
	Turn         int       `json:"turn"`        // completed player turns
	Seq          int       `json:"seq"`         // id counter
	Setbacks     int       `json:"setbacks"`
	MaxSetbacks  int       `json:"max_setbacks"`
	MaxPlayers   int       `json:"max_players"`
	Scene        Scene     `json:"scene"`
	Turns        TurnState `json:"turns"`
	Summary      string    `json:"summary,omitempty"` // running story so far
	SummaryTurn  int       `json:"summary_turn"`
	CreatedAt    int64     `json:"created_at"`
	CampaignExtra
}

// Lethality settings: what three failed death saves mean.
const (
	LethalityStandard = "standard" // the character dies
	LethalityStory    = "story"    // the character is out of the fight but lives
)

// CampaignExtra holds campaign state added after the first schema, stored
// as one JSON column.
type CampaignExtra struct {
	Lethality string `json:"lethality,omitempty"` // "" is standard
	// Pending holds turns waiting for their player to roll, by character id.
	Pending map[string]*PendingAction `json:"pending,omitempty"`
}

// Deadly reports whether characters can die in this campaign.
func (c *Campaign) Deadly() bool { return c.Lethality != LethalityStory }
