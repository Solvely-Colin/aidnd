package api

import (
	"sort"

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
}

type EncounterView struct {
	Round   int         `json:"round"`
	Enemies []EnemyView `json:"enemies"`
}

type CharacterView struct {
	ID         string         `json:"id"`
	PlayerID   string         `json:"player_id"`
	Name       string         `json:"name"`
	Concept    string         `json:"concept,omitempty"`
	Class      string         `json:"class"`
	Kit        string         `json:"kit"`
	Backstory  string         `json:"backstory,omitempty"`
	Pronouns   string         `json:"pronouns"`
	Scores     game.Scores    `json:"scores"`
	Modifiers  map[string]int `json:"modifiers"`
	Proficient []game.Ability `json:"proficient"`
	ProfBonus  int            `json:"proficiency_bonus"`
	Level      int            `json:"level"`
	XP         int            `json:"xp"`
	NextLevel  int            `json:"next_level_xp,omitempty"`
	HP         int            `json:"hp"`
	MaxHP      int            `json:"max_hp"`
	Down       bool           `json:"down"`
	AC         int            `json:"ac"`
	Gold       int            `json:"gold"`
	Weapon     *ItemView      `json:"weapon,omitempty"`
	ZoneID     string         `json:"zone_id,omitempty"`
	Inventory  []ItemView     `json:"inventory"`
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
}

func itemView(w *game.World, it *game.Item, c *game.Character, n *game.NPC) ItemView {
	v := ItemView{ID: it.ID, Key: it.Key, Name: it.Name, Kind: it.Kind, Desc: it.Desc, Value: it.Value, Damage: it.Damage, AC: it.AC, Effect: it.Effect}
	if it.Kind == game.ItemWeapon {
		v.Reach = game.ReachName(game.InferReach(it))
	}
	if game.IsEquipped(w, c, it) {
		v.Equipped = true
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
		Inventory: []ItemView{}}
	if c.Level < game.MaxLevel {
		v.NextLevel = game.XPTable[c.Level]
	}
	for _, a := range game.Abilities {
		v.Modifiers[string(a)] = c.CheckMod(a)
	}
	wpn := game.Weapon(w, c)
	wv := itemView(w, wpn, c, nil)
	v.Weapon = &wv
	for _, it := range w.ItemsOf(game.OwnerCharacter, c.ID) {
		v.Inventory = append(v.Inventory, itemView(w, it, c, nil))
	}
	return v
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
		v.Exits = append(v.Exits, ExitView{ID: x.Edge.ID, Label: x.Label, ToID: x.To.ID, ToName: x.To.Name, State: string(x.Edge.State), ZoneID: x.Zone, Explored: !x.To.Stub})
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
		v.Enemies = append(v.Enemies, EnemyView{ID: e.ID, Name: e.Name, Tag: e.Tag, Display: e.Display(), HP: e.HP, MaxHP: e.MaxHP, ZoneID: e.ZoneID, Ranged: e.Ranged})
	}
	return v
}

func campaignView(w *game.World, playerID string) CampaignView {
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
	if c.Status == game.StatusActive {
		if a := w.ActiveCharacter(); a != nil {
			v.Active = &gm.Actor{CharacterID: a.ID, PlayerID: a.PlayerID, Name: a.Name}
			b := c.Turns.Budget
			v.Budget = &b
			v.TurnStarted = c.Turns.Started
		}
	}
	if p := w.Player(playerID); p != nil {
		v.You = YouView{PlayerID: p.ID, CharacterID: p.CharacterID, Joined: true, YourTurn: v.Active != nil && v.Active.PlayerID == p.ID}
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
