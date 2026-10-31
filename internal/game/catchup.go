package game

import (
	"fmt"
	"strings"
)

// Drop-in play: a friend can make a character and join a campaign that is
// already under way. They are written in where a party stands (in its fight,
// if one is on), at that party's level, and they get a "story so far" built
// from engine state only, so the catch-up can't tell them anything the
// world doesn't hold.

// CatchUp is the story so far, for a player who just joined.
type CatchUp struct {
	Title    string        `json:"title"`
	Pitch    string        `json:"pitch,omitempty"`
	SoFar    string        `json:"so_far,omitempty"`
	Lines    []CatchUpLine `json:"lines"`
	Day      int           `json:"day"`
	Time     string        `json:"time"`
	Location string        `json:"location"`
}

// Kinds of catch-up line, in the order they are listed.
const (
	CatchQuest   = "quest"   // the storyline and its next step
	CatchParty   = "party"   // who they are joining
	CatchHere    = "here"    // who else is in the room
	CatchFight   = "fight"   // what the party is fighting
	CatchPromise = "promise" // the party's word, still owed
	CatchFact    = "fact"    // things the world remembers
)

// CatchUpLine is one item of a catch-up.
type CatchUpLine struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// JoinGroupFor picks the group a newcomer joins: the one with the most
// characters on their feet, the main group on a tie. The world must be
// settled.
func JoinGroupFor(w *World) string {
	best, most := MainGroup, -1
	for _, id := range w.GroupIDs() {
		n := 0
		for _, c := range w.Members(id) {
			if !c.Dead {
				n++
			}
			if !c.Down() {
				n++ // standing counts twice, so a group that's all down loses a tie
			}
		}
		if n > most {
			best, most = id, n
		}
	}
	return best
}

// JoinXP is the experience a newcomer starts with: the average of the
// living characters in the group they join, so they arrive at its level.
func JoinXP(w *World, group string) int {
	sum, n := 0, 0
	for _, c := range w.Members(group) {
		if !c.Dead {
			sum += c.XP
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / n
}

// JoinRunning writes a new character into the group the world is focused
// on: at its level, standing with the others (in their fight if one is
// on, taking their turns after everyone already in it), with an entry in
// their record and the chronicle. c must already be in w.Characters.
// It returns the change lines.
func JoinRunning(w *World, c *Character, xp int) []string {
	group := w.Focused()
	if group == "" {
		group = MainGroup
	}
	c.Group = group
	if group == MainGroup {
		c.Group = ""
	}
	var others []*Character
	zone := ""
	for _, o := range w.Characters {
		if o.ID == c.ID || o.Dead {
			continue
		}
		others = append(others, o)
		if zone == "" && !o.Down() {
			zone = o.ZoneID
		}
	}
	if zone == "" && len(others) > 0 {
		zone = others[0].ZoneID
	}
	JoinMidway(w, c, xp, zone)
	t := &w.Campaign.Turns
	in := false
	for _, id := range t.Order {
		in = in || id == c.ID
	}
	if !in {
		t.Order = append(t.Order, c.ID)
	}
	if len(t.Order) == 1 {
		t.Index, t.Budget = 0, FullBudget(w.InCombat())
	}
	here := "the road"
	if l := w.Here(); l != nil {
		here = l.Name
	}
	who := "the others"
	if len(others) > 0 {
		var ns []string
		for _, o := range others {
			ns = append(ns, o.Name)
		}
		who = joinNames(ns)
	}
	Note(w, c, RecordEntry{Kind: RecChoice, Source: SourceEngine, Text: fmt.Sprintf("Threw in with %s at %s", who, here)})
	w.Record(ChronicleEntry{Kind: FactDeed, Source: SourceEngine, Text: fmt.Sprintf("%s joined %s at %s.", c.Name, who, here)})
	line := fmt.Sprintf("%s joins %s at %s", c.Name, who, here)
	if w.InCombat() {
		line += ", in the middle of the fight"
	}
	if c.Level > 1 {
		line += fmt.Sprintf(" (level %d)", c.Level)
	}
	return []string{line}
}

// Caps on each kind of catch-up line, so it stays a short read.
var catchCaps = map[string]int{CatchQuest: 2, CatchParty: 5, CatchHere: 4, CatchFight: 6, CatchPromise: 3, CatchFact: 5}

// BuildCatchUp is the story so far for newcomer c, from the group the
// world is focused on.
func BuildCatchUp(w *World, c *Character) *CatchUp {
	cu := &CatchUp{
		Title: w.Campaign.Title,
		Pitch: w.Campaign.Pitch,
		SoFar: strings.TrimSpace(w.Campaign.Summary),
		Day:   w.Campaign.Scene.Day,
		Time:  w.Campaign.Scene.TimeName(),
	}
	if cu.Pitch == "" {
		cu.Pitch = w.Campaign.Premise
	}
	l := w.Here()
	if l != nil {
		cu.Location = l.Name
	}
	add := func(kind, text string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		n := 0
		for _, x := range cu.Lines {
			if x.Kind == kind {
				n++
			}
		}
		if n < catchCaps[kind] {
			cu.Lines = append(cu.Lines, CatchUpLine{Kind: kind, Text: text})
		}
	}
	if q := w.MainQuest(); q != nil && q.Status != QuestOffered {
		switch q.Status {
		case QuestCompleted, QuestTurnedIn:
			add(CatchQuest, fmt.Sprintf("%s: done.", q.Title))
		case QuestFailed:
			add(CatchQuest, fmt.Sprintf("%s: lost.", q.Title))
		default:
			add(CatchQuest, q.Title+".")
			for _, o := range q.Objectives {
				if !o.Done {
					add(CatchQuest, "Next: "+o.Text)
					break
				}
			}
		}
	}
	for _, q := range w.Quests {
		if !q.IsMain && q.Status == QuestActive {
			add(CatchQuest, "Also: "+q.Title+".")
		}
	}
	for _, o := range w.Characters {
		if o.ID == c.ID {
			continue
		}
		state := ""
		switch {
		case o.Dead:
			continue
		case o.Dying():
			state = ", dying"
		case o.Down():
			state = ", down"
		case o.HP*2 <= o.MaxHP:
			state = ", hurt"
		}
		role := o.Class
		if role == "" {
			role = "adventurer"
		}
		add(CatchParty, fmt.Sprintf("%s, %s, level %d%s.", o.Name, strings.ToLower(role), o.Level, state))
	}
	if l != nil {
		for _, n := range w.NPCsAt(l.ID) {
			if !n.Alive {
				continue
			}
			who := n.Name
			if n.Role != "" {
				who += ", " + n.Role
			}
			add(CatchHere, who+".")
		}
	}
	if w.InCombat() {
		for _, e := range w.Encounter.Living() {
			add(CatchFight, fmt.Sprintf("%s (%s).", e.Display(), woundWord(e.HP, e.MaxHP)))
		}
	}
	for _, f := range w.OpenPromises() {
		add(CatchPromise, f.Text)
	}
	// the newest things the world remembers, but not the newcomer's own
	// arrival
	for i := len(w.Chronicle) - 1; i >= 0 && len(cu.Lines) < 40; i-- {
		f := w.Chronicle[i]
		if f.Kind == FactPromise || f.Kind == FactRumor || strings.HasPrefix(f.Text, c.Name+" joined") {
			continue
		}
		add(CatchFact, f.Text)
	}
	return cu
}

// woundWord says how hurt an enemy looks.
func woundWord(hp, max int) string {
	switch {
	case max <= 0 || hp*4 >= max*3:
		return "fresh"
	case hp*2 >= max:
		return "hurt"
	default:
		return "badly hurt"
	}
}

// Text is the catch-up as plain lines, for the narrator's prompt.
func (cu *CatchUp) Text() string {
	var b strings.Builder
	if cu.SoFar != "" {
		fmt.Fprintf(&b, "So far: %s\n", cu.SoFar)
	}
	for _, l := range cu.Lines {
		fmt.Fprintf(&b, "- %s: %s\n", l.Kind, l.Text)
	}
	return b.String()
}
