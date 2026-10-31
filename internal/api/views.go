package api

import (
	"fmt"
	"github.com/Solvely-Colin/aidnd/internal/store"
	"sort"
	"strings"
	"time"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/gm"
)

// Player-safe views. Nothing here may carry lore, beats, hidden exits,
// NPC memory, antagonist secrets or player tokens.

type ZoneView struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Desc     string   `json:"desc,omitempty"`
	Cover    bool     `json:"cover"`
	Adjacent []string `json:"adjacent"`
	Exits    []string `json:"exits"`
}

type ExitView struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	ToID     string `json:"to_id"`
	ToName   string `json:"to_name"`
	State    string `json:"state"`
	ZoneID   string `json:"zone_id,omitempty"`
	Explored bool   `json:"explored"`
	Sealed   bool   `json:"sealed,omitempty"` // locked until the story opens it; no check can
}

type NPCBrief struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Role        string `json:"role,omitempty"`
	Desc        string `json:"desc,omitempty"`
	ZoneID      string `json:"zone_id,omitempty"`
	LocationID  string `json:"location_id"`
	Merchant    bool   `json:"merchant"`
	Disposition int    `json:"disposition"`
	Alive       bool   `json:"alive"`
}

type ItemView struct {
	ID       string `json:"id"`
	Key      string `json:"key,omitempty"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Desc     string `json:"desc,omitempty"`
	Value    int    `json:"value"`
	Price    int    `json:"price,omitempty"`
	Damage   string `json:"damage,omitempty"`
	Reach    string `json:"reach,omitempty"`
	AC       int    `json:"ac,omitempty"`
	Effect   string `json:"effect,omitempty"`
	Equipped bool   `json:"equipped,omitempty"`
	ToHit    *int   `json:"to_hit,omitempty"` // weapons on a character sheet
	Rarity   string `json:"rarity,omitempty"`
	Magic    string `json:"magic,omitempty"` // what the magic does
	// Attuned is set on a carried wondrous item: false when three others
	// already work.
	Attuned *bool `json:"attuned,omitempty"`
}

type LocationView struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Kind        string     `json:"kind"`
	Hint        string     `json:"hint,omitempty"`
	Description string     `json:"description,omitempty"`
	Danger      int        `json:"danger"`
	Visited     bool       `json:"visited"`
	Explored    bool       `json:"explored"`
	IsHub       bool       `json:"is_hub,omitempty"`
	Zones       []ZoneView `json:"zones,omitempty"`
	Exits       []ExitView `json:"exits,omitempty"`
	NPCs        []NPCBrief `json:"npcs,omitempty"`
	Items       []ItemView `json:"items,omitempty"`
}

type EnemyView struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Tag     string `json:"tag,omitempty"`
	Display string `json:"display"`
	HP      int    `json:"hp"`
	MaxHP   int    `json:"max_hp"`
	ZoneID  string `json:"zone_id"`
	Ranged  bool   `json:"ranged"`
	// Inflicts is the condition its hits can cause; Conditions are on it now.
	Inflicts   string          `json:"inflicts,omitempty"`
	Conditions []ConditionView `json:"conditions"`
	Boss       *BossView       `json:"boss,omitempty"`
}

// BossView is what the party can see of a boss: its phase, the resistances
// it has left, and the signature move it is about to unleash.
type BossView struct {
	Phase     int    `json:"phase"` // 1-based
	Phases    int    `json:"phases"`
	PhaseName string `json:"phase_name"`
	NextAt    int    `json:"next_at,omitempty"` // HP percent of the next phase
	Resist    int    `json:"resist"`
	Attacks   int    `json:"attacks"`
	Move      string `json:"move"`
	MoveDice  string `json:"move_dice"`
	MoveSave  string `json:"move_save"`
	MoveDC    int    `json:"move_dc"`
	Charging  string `json:"charging_zone,omitempty"`
}

func bossView(e *game.Enemy) *BossView {
	b := e.Boss
	if b == nil {
		return nil
	}
	v := &BossView{Phase: b.Phase + 1, Phases: len(b.Phases), PhaseName: b.Current().Name, Resist: b.Resist, Attacks: b.Attacks,
		Move: b.Move.Name, MoveDice: b.Move.Dice, MoveSave: string(b.Move.Save), MoveDC: b.Move.DC}
	if b.Phase+1 < len(b.Phases) {
		v.NextAt = b.Phases[b.Phase+1].At
	}
	if b.Charging != nil {
		v.Charging = b.Charging.Zone
	}
	return v
}

type EncounterView struct {
	Round   int         `json:"round"`
	Enemies []EnemyView `json:"enemies"`
}

type CharacterView struct {
	ID         string           `json:"id"`
	PlayerID   string           `json:"player_id"`
	Name       string           `json:"name"`
	Concept    string           `json:"concept,omitempty"`
	Class      string           `json:"class"`
	Kit        string           `json:"kit"`
	Backstory  string           `json:"backstory,omitempty"`
	Pronouns   string           `json:"pronouns"`
	Scores     game.Scores      `json:"scores"`
	Modifiers  map[string]int   `json:"modifiers"`
	Proficient []game.Ability   `json:"proficient"`
	ProfBonus  int              `json:"proficiency_bonus"`
	Level      int              `json:"level"`
	XP         int              `json:"xp"`
	LevelXP    int              `json:"level_xp"` // XP the current level began at
	NextLevel  int              `json:"next_level_xp,omitempty"`
	HP         int              `json:"hp"`
	MaxHP      int              `json:"max_hp"`
	Down       bool             `json:"down"`
	AC         int              `json:"ac"`
	Gold       int              `json:"gold"`
	Weapon     *ItemView        `json:"weapon,omitempty"`
	Armor      *ItemView        `json:"armor,omitempty"`
	Attack     AttackView       `json:"attack"`
	ACParts    []game.ACPart    `json:"ac_parts"`
	LevelUp    *LevelUpView     `json:"level_up,omitempty"`
	Conditions []ConditionView  `json:"conditions"`
	ZoneID     string           `json:"zone_id,omitempty"`
	ZoneName   string           `json:"zone_name,omitempty"`
	Inventory  []ItemView       `json:"inventory"`
	Skills     []SkillView      `json:"skills"`
	Passive    int              `json:"passive_perception"`
	Dying      bool             `json:"dying,omitempty"`
	Dead       bool             `json:"dead,omitempty"`
	DeathSaves *game.DeathSaves `json:"death_saves,omitempty"`
	Readied    bool             `json:"readied,omitempty"`
	Build      BuildView        `json:"build"`
	// Strain is set in dread campaigns (see game/strain.go).
	Strain *StrainView `json:"strain,omitempty"`
}

// StrainView is a character's mental strain and the scars it left.
type StrainView struct {
	Value  int        `json:"value"`
	Max    int        `json:"max"`
	Shaken bool       `json:"shaken"` // disadvantage on saves against fear
	Scars  []ScarView `json:"scars"`
}

// ScarView is one lasting scar.
type ScarView struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Text string `json:"text"`
}

func strainView(w *game.World, c *game.Character) *StrainView {
	if !w.Campaign.Dread {
		return nil
	}
	v := &StrainView{Value: c.Strain, Max: game.MaxStrain, Shaken: c.Strain >= game.StrainShaken, Scars: []ScarView{}}
	for _, k := range c.Scars {
		if s, ok := game.ScarByKey(k); ok {
			v.Scars = append(v.Scars, ScarView{Key: s.Key, Name: s.Name, Text: s.Text})
		}
	}
	return v
}

// SkillView is one skill line on the sheet.
type SkillView struct {
	Key        string       `json:"key"`
	Name       string       `json:"name"`
	Ability    game.Ability `json:"ability"`
	Mod        int          `json:"mod"`
	Proficient bool         `json:"proficient"`
	Expertise  bool         `json:"expertise,omitempty"`
}

// PendingView is a turn waiting for its player's roll.
type PendingView struct {
	Kind        string             `json:"kind"`
	CharacterID string             `json:"character_id"`
	PlayerID    string             `json:"player_id"`
	Name        string             `json:"name"`
	Text        string             `json:"text,omitempty"`
	Preview     []game.PlannedStep `json:"preview,omitempty"`
}

// AttackView is the equipped weapon's attack line.
type AttackView struct {
	Weapon  string       `json:"weapon"`
	Bonus   int          `json:"bonus"`
	Ability game.Ability `json:"ability"`
	Damage  string       `json:"damage"`
	Reach   string       `json:"reach"`
}

// LevelUpView is what the next level grants.
type LevelUpView struct {
	Level     int `json:"level"`
	HP        int `json:"hp"`
	ProfBonus int `json:"proficiency_bonus"`
}

// ConditionView is a state derived from the rules: the engine has no
// status effects, so these are facts like 0 HP or standing in cover.
type ConditionView struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Note  string `json:"note"`
}

type ObjectiveView struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Text string `json:"text"`
	Done bool   `json:"done"`
}

type QuestView struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	Summary     string          `json:"summary"`
	IsMain      bool            `json:"is_main"`
	Status      string          `json:"status"`
	GiverID     string          `json:"giver_id,omitempty"`
	GiverName   string          `json:"giver_name,omitempty"`
	Objectives  []ObjectiveView `json:"objectives"`
	RewardGold  int             `json:"reward_gold"`
	RewardXP    int             `json:"reward_xp"`
	RewardItems []string        `json:"reward_items,omitempty"`
}

type PlayerView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	CharacterID string `json:"character_id,omitempty"`
}

type YouView struct {
	PlayerID    string `json:"player_id,omitempty"`
	CharacterID string `json:"character_id,omitempty"`
	Joined      bool   `json:"joined"`
	YourTurn    bool   `json:"your_turn"`
	AutoRoll    bool   `json:"auto_roll"`
	// Downtime is the order queued for the next world tick; CanDowntime
	// is set where one can be queued.
	Downtime    string `json:"downtime,omitempty"`
	CanDowntime bool   `json:"can_downtime,omitempty"`
}

type SceneView struct {
	Day     int    `json:"day"`
	Time    string `json:"time"`
	Tension int    `json:"tension"`
	Goal    string `json:"goal,omitempty"`
}

type CampaignView struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	Pitch       string          `json:"pitch"`
	Premise     string          `json:"premise"`
	Setting     string          `json:"setting"`
	Status      string          `json:"status"`
	Turn        int             `json:"turn"`
	Setbacks    int             `json:"setbacks"`
	MaxSetbacks int             `json:"max_setbacks"`
	MaxPlayers  int             `json:"max_players"`
	Scene       SceneView       `json:"scene"`
	Location    *LocationView   `json:"location,omitempty"`
	Party       []CharacterView `json:"party"`
	Players     []PlayerView    `json:"players"`
	Encounter   *EncounterView  `json:"encounter,omitempty"`
	Quests      []QuestView     `json:"quests"`
	Active      *gm.Actor       `json:"active,omitempty"`
	TurnStarted int64           `json:"turn_started,omitempty"`
	Budget      *game.Budget    `json:"budget,omitempty"`
	You         YouView         `json:"you"`
	// FreeRoam is set outside fights: anyone on their feet can act.
	FreeRoam  bool          `json:"free_roam"`
	Pending   []PendingView `json:"pending"`
	Lethality string        `json:"lethality"`
	// Saga is set for a hand-written campaign; Dread turns on strain and
	// scars.
	Saga  string `json:"saga,omitempty"`
	Dread bool   `json:"dread,omitempty"`
	// Storyline is the main quest that matters where the party stands (a
	// saga has one per region).
	Storyline string `json:"storyline,omitempty"`
	// Group is the caller's group; Groups lists every group in the world
	// (one, the main group, until someone splits off).
	Group  string      `json:"group"`
	Groups []GroupView `json:"groups"`
	// Deadline is when autopilot plays the current turn of an async fight
	// (unix seconds; 0 when nothing is waiting on one).
	Deadline int64 `json:"deadline,omitempty"`
	// World is the long campaign's shared calendar, Tide and newspaper.
	World *WorldView `json:"world,omitempty"`
	// Dispatch is what the caller missed while they were away (world
	// ticks since they last played); POST me/seen clears it.
	Dispatch *game.Dispatch `json:"dispatch,omitempty"`
}

// WorldView is what players see of the living world.
type WorldView struct {
	Week    int           `json:"week"`
	Date    string        `json:"date"`
	Tide    int           `json:"tide"`
	Band    string        `json:"band"`
	Gazette *game.Gazette `json:"gazette,omitempty"`
	// Downtime lists the orders a character can queue at a safe place.
	Downtime []DowntimeKind `json:"downtime"`
}

// DowntimeKind is one downtime order.
type DowntimeKind struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Desc string `json:"desc"`
}

// sagaStart is the world's first day; a world week is a calendar week.
var sagaStart = time.Date(1931, time.April, 6, 0, 0, 0, 0, time.UTC)

func worldView(w *game.World) *WorldView {
	lv := w.Campaign.Living
	if lv == nil {
		return nil
	}
	d := sagaStart.AddDate(0, 0, 7*lv.Week)
	v := &WorldView{Week: lv.Week, Date: d.Format("January 2, 2006"), Tide: lv.Tide, Band: game.TideBand(lv.Tide), Gazette: lv.Gazette}
	for _, k := range game.DowntimeKinds {
		v.Downtime = append(v.Downtime, DowntimeKind{Key: k.Key, Name: k.Name, Desc: k.Desc})
	}
	return v
}

// MyCampaign is a campaign in the player's own list.
type MyCampaign struct {
	store.CampaignSummary
	YourTurn bool  `json:"your_turn,omitempty"`
	Deadline int64 `json:"deadline,omitempty"`
}

// turnDeadline is the GM's TurnDeadline, set by New.
var turnDeadline = func(*game.World) int64 { return 0 }

// GroupView is one party in the shared world.
type GroupView struct {
	ID       string       `json:"id"`
	Members  []MemberView `json:"members"`
	Location string       `json:"location_id"`
	Place    string       `json:"location"`
	Fighting bool         `json:"fighting"`
	Mine     bool         `json:"mine"`
	// Here is set when the group stands where the caller's group does.
	Here bool `json:"here"`
}

// MemberView names a character in a group.
type MemberView struct {
	CharacterID string `json:"character_id"`
	Name        string `json:"name"`
	Down        bool   `json:"down,omitempty"`
}

func groupsView(w *game.World, mine string) []GroupView {
	out := []GroupView{}
	for _, gi := range w.GroupInfos() {
		gv := GroupView{ID: gi.ID, Location: gi.LocationID, Fighting: gi.Fighting, Mine: gi.ID == mine,
			Here: gi.LocationID == w.Campaign.LocationID, Members: []MemberView{}}
		if l := w.Location(gi.LocationID); l != nil {
			gv.Place = l.Name
		}
		for _, c := range gi.Members {
			if c.Dead {
				continue
			}
			gv.Members = append(gv.Members, MemberView{CharacterID: c.ID, Name: c.Name, Down: c.Down()})
		}
		if len(gv.Members) > 0 {
			out = append(out, gv)
		}
	}
	return out
}

func itemView(w *game.World, it *game.Item, c *game.Character, n *game.NPC) ItemView {
	v := ItemView{ID: it.ID, Key: it.Key, Name: it.Name, Kind: it.Kind, Desc: it.Desc, Value: it.Value, Damage: it.Damage, AC: it.AC, Effect: it.Effect,
		Rarity: it.Rarity, Magic: game.MagicSummary(it)}
	if c != nil && it.Kind == game.ItemWondrous && it.Owner.ID == c.ID {
		on := c.Attuned(it.ID)
		v.Attuned = &on
	}
	if it.Kind == game.ItemWeapon {
		v.Reach = game.ReachName(game.InferReach(it))
	}
	if game.IsEquipped(w, c, it) {
		v.Equipped = true
	}
	if c != nil && it.Kind == game.ItemWeapon {
		hit, _ := game.AttackMod(c, it)
		v.ToHit = &hit
	}
	if n != nil {
		v.Price = game.BuyPrice(it, n)
	}
	return v
}

func characterView(w *game.World, c *game.Character) CharacterView {
	v := CharacterView{ID: c.ID, PlayerID: c.PlayerID, Name: c.Name, Concept: c.Concept, Class: c.Class, Kit: c.Kit, Backstory: c.Backstory, Pronouns: game.NormPronouns(c.Pronouns),
		Scores: c.Scores, Modifiers: map[string]int{}, Proficient: c.Proficient, ProfBonus: game.ProficiencyBonus(c.Level),
		Level: c.Level, XP: c.XP, HP: c.HP, MaxHP: c.MaxHP, Down: c.Down(), AC: game.ArmorClass(w, c), Gold: c.Gold, ZoneID: c.ZoneID,
		LevelXP: game.XPTable[max(c.Level, 1)-1], ACParts: game.ArmorClassParts(w, c), Conditions: conditions(w, c), Inventory: []ItemView{},
		Strain: strainView(w, c)}
	if c.Level < game.MaxLevel {
		v.NextLevel = game.XPTable[c.Level]
		v.LevelUp = &LevelUpView{Level: c.Level + 1, HP: game.HPPerLevel(c), ProfBonus: game.ProficiencyBonus(c.Level + 1)}
	}
	for _, a := range game.Abilities {
		v.Modifiers[string(a)] = c.CheckMod(a)
	}
	wpn := game.Weapon(w, c)
	wv := itemView(w, wpn, c, nil)
	v.Weapon = &wv
	bonus, ab := game.AttackMod(c, wpn)
	v.Attack = AttackView{Weapon: wpn.Name, Bonus: bonus, Ability: ab, Damage: wpn.Damage, Reach: wv.Reach}
	if a := w.Item(c.Armor); a != nil && a.Owner.ID == c.ID && a.Kind == game.ItemArmor {
		av := itemView(w, a, c, nil)
		v.Armor = &av
	}
	if l := w.Here(); l != nil && c.ZoneID != "" {
		v.ZoneName = game.ZoneName(l, c.ZoneID)
	}
	for _, it := range w.ItemsOf(game.OwnerCharacter, c.ID) {
		v.Inventory = append(v.Inventory, itemView(w, it, c, nil))
	}
	for _, sk := range game.Skills {
		sv := SkillView{Key: sk.Key, Name: sk.Name, Ability: sk.Ability, Mod: c.SkillMod(sk), Proficient: c.HasSkill(sk.Key)}
		sv.Expertise = c.HasExpertise(sk.Key)
		v.Skills = append(v.Skills, sv)
	}
	v.Passive = c.PassivePerception()
	v.Dying, v.Dead, v.Readied = c.Dying(), c.Dead, c.Readied != nil
	v.Build = buildView(w, c)
	v.Attack.Damage = game.WeaponDamage(c, wpn)
	if c.Death != nil && c.Down() {
		d := *c.Death
		v.DeathSaves = &d
	}
	return v
}

func conditions(w *game.World, c *game.Character) []ConditionView {
	out := []ConditionView{}
	switch {
	case c.Dead:
		out = append(out, ConditionView{"dead", "Dead", "Fell in battle. Their player can make a new character."})
	case c.Dying():
		out = append(out, ConditionView{"dying", "Dying", fmt.Sprintf("At 0 HP and making death saves (%d of 3 successes, %d of 3 failures). Any healing, a healer's kit or a DC 10 Medicine check stops it.", c.Death.Successes, c.Death.Failures)})
	case c.Down():
		out = append(out, ConditionView{"down", "Down", "At 0 HP but stable. Any healing or a rest brings them back."})
	case c.HP*2 <= c.MaxHP:
		out = append(out, ConditionView{"bloodied", "Bloodied", "At half health or less."})
	}
	out = append(out, statusViews(c.Conditions)...)
	if l := w.Here(); l != nil && w.Campaign.Status == game.StatusActive {
		if z := l.Zone(c.ZoneID); z != nil && z.Cover {
			out = append(out, ConditionView{"cover", "In cover", "+2 AC against attacks from other zones."})
		}
	}
	if w.Campaign.Status == game.StatusActive {
		if a := w.ActiveCharacter(); a != nil && a.ID == c.ID {
			out = append(out, ConditionView{"acting", "Acting", "It is their turn."})
		}
	}
	return out
}

// statusViews lists engine conditions (poisoned, prone...) with their
// effect and how long they last.
func statusViews(cs game.Conditions) []ConditionView {
	out := []ConditionView{}
	for _, k := range cs.Kinds() {
		c := cs.Get(k)
		note := capFirst(game.ConditionSummary(k))
		if c.Source != "" {
			note = "From " + c.Source + ". " + note
		}
		switch {
		case k == game.CondProne:
		case c.SaveDC > 0 && c.Rounds > 0:
			note += fmt.Sprintf(". Up to %s more, %s save DC %d each turn to end it early", turnsWord(c.Rounds), c.Save, c.SaveDC)
		case c.Rounds > 0:
			note += fmt.Sprintf(". %s left", turnsWord(c.Rounds))
		}
		out = append(out, ConditionView{Key: k, Label: capFirst(k), Note: note + "."})
	}
	return out
}

func capFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func turnsWord(n int) string {
	if n == 1 {
		return "1 turn"
	}
	return fmt.Sprintf("%d turns", n)
}

func npcBrief(n *game.NPC) NPCBrief {
	return NPCBrief{ID: n.ID, Name: n.Name, Role: n.Role, Desc: n.Desc, ZoneID: n.ZoneID, LocationID: n.LocationID, Merchant: n.Merchant, Disposition: n.Disposition, Alive: n.Alive}
}

// locationKnown: visited, or reachable through a visible exit from a visited place.
func locationKnown(w *game.World, l *game.Location) bool {
	if l.Visited {
		return true
	}
	for _, e := range w.Edges {
		if e.Touches(l.ID) && e.State.Visible() {
			if o := w.Location(e.Other(l.ID)); o != nil && o.Visited {
				return true
			}
		}
	}
	return false
}

func locationView(w *game.World, l *game.Location, detail bool) LocationView {
	v := LocationView{ID: l.ID, Name: l.Name, Kind: l.Kind, Hint: l.Hint, Danger: l.Danger, Visited: l.Visited, Explored: !l.Stub, IsHub: l.IsHub}
	if l.Stub || !l.Visited {
		return v
	}
	v.Description = l.Description
	if !detail {
		return v
	}
	exits := w.Exits(l.ID, false)
	for _, z := range l.Zones {
		zv := ZoneView{ID: z.ID, Name: z.Name, Desc: z.Desc, Cover: z.Cover, Adjacent: append([]string{}, z.Adjacent...), Exits: []string{}}
		for _, x := range exits {
			if x.Zone == z.ID {
				zv.Exits = append(zv.Exits, x.Edge.ID)
			}
		}
		v.Zones = append(v.Zones, zv)
	}
	for _, x := range exits {
		v.Exits = append(v.Exits, ExitView{ID: x.Edge.ID, Label: x.Label, ToID: x.To.ID, ToName: x.To.Name, State: string(x.Edge.State), ZoneID: x.Zone, Explored: !x.To.Stub,
			Sealed: x.Edge.State == game.EdgeLocked && x.Edge.LairGate})
	}
	for _, n := range w.NPCsAt(l.ID) {
		v.NPCs = append(v.NPCs, npcBrief(n))
	}
	for _, it := range w.ItemsOf(game.OwnerLocation, l.ID) {
		v.Items = append(v.Items, itemView(w, it, nil, nil))
	}
	return v
}

func questVisible(w *game.World, q *game.Quest) bool {
	if q.Status != game.QuestOffered {
		return true
	}
	g := w.NPC(q.GiverID)
	if g == nil {
		return false
	}
	l := w.Location(g.LocationID)
	return l != nil && l.Visited
}

func questView(w *game.World, q *game.Quest) QuestView {
	v := QuestView{ID: q.ID, Title: q.Title, Summary: q.Summary, IsMain: q.IsMain, Status: q.Status, GiverID: q.GiverID,
		RewardGold: q.RewardGold, RewardXP: q.RewardXP, Objectives: []ObjectiveView{}}
	if g := w.NPC(q.GiverID); g != nil {
		v.GiverName = g.Name
	}
	for _, o := range q.Objectives {
		v.Objectives = append(v.Objectives, ObjectiveView{ID: o.ID, Kind: o.Kind, Text: o.Text, Done: o.Done})
		if q.IsMain && !o.Done {
			break // later storyline steps stay hidden until reached
		}
	}
	for _, id := range q.RewardItems {
		if it := w.Item(id); it != nil {
			v.RewardItems = append(v.RewardItems, it.Name)
		}
	}
	return v
}

func questsView(w *game.World) []QuestView {
	out := []QuestView{}
	for _, q := range w.Quests {
		if questVisible(w, q) {
			out = append(out, questView(w, q))
		}
	}
	return out
}

func encounterView(w *game.World) *EncounterView {
	if !w.InCombat() {
		return nil
	}
	v := &EncounterView{Round: w.Encounter.Round}
	for _, e := range w.Encounter.Living() {
		v.Enemies = append(v.Enemies, EnemyView{ID: e.ID, Name: e.Name, Tag: e.Tag, Display: e.Display(), HP: e.HP, MaxHP: e.MaxHP, ZoneID: e.ZoneID, Ranged: e.Ranged,
			Inflicts: e.Inflicts, Conditions: statusViews(e.Conditions), Boss: bossView(e)})
	}
	return v
}

func campaignView(w *game.World, playerID string) CampaignView {
	_ = w.FocusPlayer(playerID)
	c := w.Campaign
	v := CampaignView{ID: c.ID, Title: c.Title, Pitch: c.Pitch, Premise: c.Premise, Setting: w.Setting(), Status: c.Status, Turn: c.Turn,
		Setbacks: c.Setbacks, MaxSetbacks: c.MaxSetbacks, MaxPlayers: c.MaxPlayers,
		Scene: SceneView{Day: c.Scene.Day, Time: c.Scene.TimeName(), Tension: c.Scene.Tension, Goal: c.Scene.Goal},
		Party: []CharacterView{}, Players: []PlayerView{}, Quests: questsView(w), Encounter: encounterView(w)}
	if c.Status != game.StatusLobby {
		if l := w.Here(); l != nil {
			lv := locationView(w, l, true)
			v.Location = &lv
		}
	}
	for _, ch := range w.Party() {
		v.Party = append(v.Party, characterView(w, ch))
	}
	for _, p := range w.Players {
		v.Players = append(v.Players, PlayerView{ID: p.ID, Name: p.Name, CharacterID: p.CharacterID})
	}
	v.Group = game.MainGroup
	if f := w.Focused(); f != "" {
		v.Group = f
	}
	v.Groups = groupsView(w, v.Group)
	v.Deadline = turnDeadline(w)
	v.World = worldView(w)
	v.Dispatch = game.BuildDispatch(w, playerID)
	v.Saga, v.Dread = c.Saga, c.Dread
	if mq := w.MainQuest(); mq != nil {
		v.Storyline = mq.ID
	}
	v.Lethality = c.Lethality
	if v.Lethality == "" {
		v.Lethality = game.LethalityStandard
	}
	v.Pending = []PendingView{}
	if c.Status == game.StatusActive {
		v.FreeRoam = !w.InCombat()
		if a := w.ActiveCharacter(); a != nil {
			v.Active = &gm.Actor{CharacterID: a.ID, PlayerID: a.PlayerID, Name: a.Name}
			b := c.Turns.Budget
			v.Budget = &b
			v.TurnStarted = c.Turns.Started
		}
		if v.FreeRoam {
			b := game.FullBudget(false)
			v.Budget = &b
		}
		for _, ch := range w.Party() {
			if p := c.Pending[ch.ID]; p != nil {
				v.Pending = append(v.Pending, PendingView{Kind: p.Kind, CharacterID: ch.ID, PlayerID: ch.PlayerID, Name: ch.Name, Text: p.Text, Preview: p.Preview})
			}
		}
	}
	if p := w.Player(playerID); p != nil {
		v.You = YouView{PlayerID: p.ID, CharacterID: p.CharacterID, Joined: true, AutoRoll: p.AutoRoll}
		if me := w.Character(p.CharacterID); me != nil && c.Status == game.StatusActive {
			if me.Downtime != nil {
				v.You.Downtime = me.Downtime.Kind
			}
			v.You.CanDowntime = !w.InCombat() && game.CanDowntime(w, me, w.Here()) == nil
		}
		if c.Status == game.StatusActive {
			if v.FreeRoam {
				me := w.Character(p.CharacterID)
				v.You.YourTurn = me != nil && !me.Down()
			} else {
				v.You.YourTurn = v.Active != nil && v.Active.PlayerID == p.ID
			}
		}
	} else if playerID != "" {
		v.You.PlayerID = playerID
	}
	return v
}

// MapView is GET /map: known locations and visible paths.
type MapView struct {
	CurrentID string         `json:"current_id"`
	HubID     string         `json:"hub_id"`
	Locations []LocationView `json:"locations"`
	Edges     []MapEdge      `json:"edges"`
	Current   *LocationView  `json:"current,omitempty"`
}

type MapEdge struct {
	ID    string `json:"id"`
	A     string `json:"a"`
	B     string `json:"b"`
	State string `json:"state"`
}

func mapView(w *game.World) MapView {
	v := MapView{CurrentID: w.Campaign.LocationID, HubID: w.Campaign.HubID, Locations: []LocationView{}, Edges: []MapEdge{}}
	known := map[string]bool{}
	for _, l := range w.Locations {
		if locationKnown(w, l) {
			known[l.ID] = true
			v.Locations = append(v.Locations, locationView(w, l, false))
		}
	}
	for _, e := range w.Edges {
		if e.State.Visible() && known[e.A] && known[e.B] {
			v.Edges = append(v.Edges, MapEdge{ID: e.ID, A: e.A, B: e.B, State: string(e.State)})
		}
	}
	if l := w.Here(); l != nil && w.Campaign.Status != game.StatusLobby {
		lv := locationView(w, l, true)
		v.Current = &lv
	}
	return v
}

// NPCView is GET /npcs/{id}: an NPC the party has met (at a visited place).
type NPCView struct {
	NPCBrief
	LocationName string     `json:"location_name"`
	Stock        []ItemView `json:"stock,omitempty"`
}

func npcView(w *game.World, n *game.NPC) (NPCView, bool) {
	l := w.Location(n.LocationID)
	if l == nil || !l.Visited || n.Antagonist && n.LocationID != w.Campaign.LocationID {
		return NPCView{}, false
	}
	v := NPCView{NPCBrief: npcBrief(n), LocationName: l.Name}
	if n.Merchant {
		for _, it := range w.ItemsOf(game.OwnerNPC, n.ID) {
			if !rewardItem(w, it.ID) {
				v.Stock = append(v.Stock, itemView(w, it, nil, n))
			}
		}
	}
	return v, true
}

func rewardItem(w *game.World, id string) bool {
	for _, q := range w.Quests {
		if q.Status == game.QuestTurnedIn {
			continue
		}
		for _, r := range q.RewardItems {
			if r == id {
				return true
			}
		}
	}
	return false
}

// JournalView is GET /journal: public facts only.
type JournalView struct {
	Summary   string                `json:"summary,omitempty"`
	Chronicle []game.ChronicleEntry `json:"chronicle"`
	Quests    []QuestView           `json:"quests"`
	Setbacks  int                   `json:"setbacks"`
	People    []JournalPerson       `json:"people"`
	Places    []JournalPlace        `json:"places"`
}

// JournalPerson is someone the party has come across. Everything comes
// from the world record, so the journal never lists a person the engine
// does not know about.
type JournalPerson struct {
	NPCBrief
	LocationName string   `json:"location_name"`
	Met          bool     `json:"met"`  // the party has spoken with them
	Here         bool     `json:"here"` // in the party's location now
	Quests       []string `json:"quests,omitempty"`
}

// JournalPlace is a location the party has been to or has seen a way to.
type JournalPlace struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Kind        string   `json:"kind"`
	Hint        string   `json:"hint,omitempty"`
	Description string   `json:"description,omitempty"`
	Danger      int      `json:"danger"`
	Visited     bool     `json:"visited"`
	Here        bool     `json:"here"`
	IsHub       bool     `json:"is_hub,omitempty"`
	People      []string `json:"people,omitempty"`
}

// journalPeople lists the NPCs a player may see: those at visited places,
// with the antagonist only while the party faces them (as in npcView).
// People nearby come first, then the most recently spoken to.
func journalPeople(w *game.World) []JournalPerson {
	gave := map[string][]string{}
	for _, q := range w.Quests {
		if q.GiverID != "" && questVisible(w, q) {
			gave[q.GiverID] = append(gave[q.GiverID], q.Title)
		}
	}
	out := []JournalPerson{}
	for _, n := range w.NPCs {
		v, ok := npcView(w, n)
		if !ok {
			continue
		}
		out = append(out, JournalPerson{NPCBrief: v.NPCBrief, LocationName: v.LocationName, Met: n.LastTalked >= 0,
			Here: n.LocationID == w.Campaign.LocationID, Quests: gave[n.ID]})
	}
	talked := map[string]int{}
	for _, n := range w.NPCs {
		talked[n.ID] = n.LastTalked
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Here != b.Here {
			return a.Here
		}
		if a.Alive != b.Alive {
			return a.Alive
		}
		return talked[a.ID] > talked[b.ID]
	})
	return out
}

// journalPlaces lists known locations: where the party is, then places
// visited, then places only seen from a path.
func journalPlaces(w *game.World) []JournalPlace {
	out := []JournalPlace{}
	for _, l := range w.Locations {
		if !locationKnown(w, l) {
			continue
		}
		lv := locationView(w, l, false)
		p := JournalPlace{ID: l.ID, Name: l.Name, Kind: l.Kind, Hint: l.Hint, Description: lv.Description, Danger: l.Danger,
			Visited: l.Visited, Here: l.ID == w.Campaign.LocationID, IsHub: l.IsHub}
		if l.Visited {
			for _, n := range w.NPCsAt(l.ID) {
				if !n.Antagonist {
					p.People = append(p.People, n.Name)
				}
			}
		}
		out = append(out, p)
	}
	rank := func(p JournalPlace) int {
		switch {
		case p.Here:
			return 0
		case p.Visited:
			return 1
		}
		return 2
	}
	sort.SliceStable(out, func(i, j int) bool { return rank(out[i]) < rank(out[j]) })
	return out
}
