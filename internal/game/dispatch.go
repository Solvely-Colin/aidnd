package game

import "strings"

// "While you were away": each player gets a short dispatch of what the
// world did since they last played. It is built from engine state only
// (the world log, the chronicle's engine facts and the Tide), so it can't
// tell a player anything the world doesn't hold.

// Kinds of world-log beat.
const (
	BeatFaction  = "faction"  // a faction's clock moved
	BeatFilled   = "filled"   // a faction's clock filled
	BeatPerson   = "person"   // someone took a step toward their goal
	BeatDowntime = "downtime" // a character's downtime order came to something
	BeatEvent    = "event"    // a world event was called or settled
)

// MaxWorldLog is how many beats the world log keeps.
const MaxWorldLog = 150

// WorldBeat is one thing the world did on a tick.
type WorldBeat struct {
	Week        int    `json:"week"`
	Kind        string `json:"kind"`
	Text        string `json:"text"`
	Region      string `json:"region,omitempty"`
	LocationID  string `json:"location_id,omitempty"`
	NPCID       string `json:"npc_id,omitempty"`
	CharacterID string `json:"character_id,omitempty"`
}

// Seen is where a player last caught up with the world.
type Seen struct {
	Week int `json:"week"`
	Tide int `json:"tide"`
	// Fact is the chronicle's length then: later entries are new to them.
	Fact int `json:"fact"`
}

// logBeat adds a beat to the world log, keeping the newest MaxWorldLog.
func logBeat(w *World, b WorldBeat) {
	lv := w.Campaign.Living
	if b.Region == "" && b.LocationID != "" {
		if l := w.Location(b.LocationID); l != nil {
			b.Region = l.Region
		}
	}
	lv.Log = append(lv.Log, b)
	if len(lv.Log) > MaxWorldLog {
		lv.Log = append([]WorldBeat(nil), lv.Log[len(lv.Log)-MaxWorldLog:]...)
	}
}

// NoteSeen marks the player as caught up with the world as it stands.
func NoteSeen(w *World, playerID string) {
	lv := w.Campaign.Living
	if lv == nil || playerID == "" {
		return
	}
	if lv.Seen == nil {
		lv.Seen = map[string]*Seen{}
	}
	lv.Seen[playerID] = &Seen{Week: lv.Week, Tide: lv.Tide, Fact: len(w.Chronicle)}
}

// Kinds of dispatch line, in the order a dispatch lists them.
const (
	DispatchYou    = "you"    // the player's own character
	DispatchDeed   = "deed"   // what other players did meanwhile
	DispatchPeople = "people" // people the party has met
	DispatchNear   = "near"   // the region the player is in
	DispatchFar    = "far"    // everywhere else
)

// DispatchLine is one item of a dispatch.
type DispatchLine struct {
	Kind string `json:"kind"`
	Week int    `json:"week,omitempty"`
	Text string `json:"text"`
	// Where is where the person in a people line is now.
	Where string `json:"where,omitempty"`
}

// Dispatch is what a player missed.
type Dispatch struct {
	Weeks    int            `json:"weeks"`
	FromWeek int            `json:"from_week"`
	Week     int            `json:"week"`
	TideFrom int            `json:"tide_from"`
	Tide     int            `json:"tide"`
	Band     string         `json:"band"`
	Lines    []DispatchLine `json:"lines"`
}

// Caps on each kind of dispatch line, so the dispatch stays a glance.
var dispatchCaps = map[string]int{DispatchYou: 3, DispatchDeed: 4, DispatchPeople: 4, DispatchNear: 4, DispatchFar: 2}

// BuildDispatch is what playerID missed, or nil when they missed nothing:
// no world tick since they last played, or they have never played.
func BuildDispatch(w *World, playerID string) *Dispatch {
	lv := w.Campaign.Living
	if lv == nil {
		return nil
	}
	seen := lv.Seen[playerID]
	if seen == nil || lv.Week <= seen.Week {
		return nil
	}
	d := &Dispatch{Weeks: lv.Week - seen.Week, FromWeek: seen.Week + 1, Week: lv.Week, TideFrom: seen.Tide, Tide: lv.Tide, Band: TideBand(lv.Tide)}
	var me *Character
	if p := w.Player(playerID); p != nil {
		me = w.Character(p.CharacterID)
	}
	region := ""
	if me != nil {
		for _, gi := range w.GroupInfos() {
			if gi.ID == GroupOf(me) {
				if l := w.Location(gi.LocationID); l != nil {
					region = l.Region
				}
			}
		}
	}
	byKind := map[string][]DispatchLine{}
	add := func(l DispatchLine) {
		byKind[l.Kind] = append(byKind[l.Kind], l)
	}
	for _, b := range lv.Log {
		if b.Week <= seen.Week {
			continue
		}
		switch {
		case b.Kind == BeatDowntime:
			if me != nil && b.CharacterID == me.ID {
				add(DispatchLine{Kind: DispatchYou, Week: b.Week, Text: b.Text})
			}
		case b.Kind == BeatPerson && known(w, b.NPCID):
			l := DispatchLine{Kind: DispatchPeople, Week: b.Week, Text: b.Text}
			if n := w.NPC(b.NPCID); n != nil && n.Alive {
				if loc := w.Location(n.LocationID); loc != nil {
					l.Where = loc.Name
				}
			}
			add(l)
		case region != "" && b.Region == region || region == "" && b.Region == "":
			add(DispatchLine{Kind: DispatchNear, Week: b.Week, Text: b.Text})
		default:
			add(DispatchLine{Kind: DispatchFar, Week: b.Week, Text: b.Text})
		}
	}
	// what other players did meanwhile: the engine's own record of deeds,
	// deaths and storylines ending
	for i := seen.Fact; i < len(w.Chronicle); i++ {
		c := w.Chronicle[i]
		if c.Source != SourceEngine {
			continue
		}
		if c.Kind == FactDeed || c.Kind == FactDeath || c.Kind == FactNews && c.QuestID != "" {
			add(DispatchLine{Kind: DispatchDeed, Text: strings.TrimSpace(c.Text)})
		}
	}
	for _, k := range []string{DispatchYou, DispatchDeed, DispatchPeople, DispatchNear, DispatchFar} {
		ls := byKind[k]
		if c := dispatchCaps[k]; len(ls) > c {
			ls = ls[len(ls)-c:] // the newest
		}
		d.Lines = append(d.Lines, ls...)
	}
	if len(d.Lines) == 0 {
		d.Lines = []DispatchLine{{Kind: DispatchNear, Text: "The weeks passed quietly. Nobody you know made a move, and nothing reached the papers."}}
	}
	return d
}

// known reports whether the party has met someone.
func known(w *World, npcID string) bool {
	n := w.NPC(npcID)
	return n != nil && npcID != "" && n.LastTalked >= 0
}
