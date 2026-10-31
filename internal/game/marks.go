package game

import (
	"fmt"
	"strings"
)

// Consequences that follow a character: lasting marks on the character
// (a scar, a cult brand, a friend lost, a name people call them) and ties
// NPCs hold toward one character rather than the whole party (a grudge,
// a debt owed to them, a debt they are owed). Both are engine state: the
// engine adds them from what happens, and the narrator may only propose
// them through validated directives.

// Mark kinds.
const (
	MarkScar  = "scar"  // a wound that healed but shows
	MarkBrand = "brand" // a cult sign, a curse, a sign others can see
	MarkLoss  = "loss"  // someone they lost
	MarkName  = "name"  // what people have started calling them
)

// MarkKinds lists every mark kind.
var MarkKinds = []string{MarkScar, MarkBrand, MarkLoss, MarkName}

// NarratorMarkKinds are the marks the narrator may give. Losses need a
// real death, so only the engine writes them.
var NarratorMarkKinds = []string{MarkScar, MarkBrand, MarkName}

// MaxMarks is how many marks one character can carry.
const MaxMarks = 12

// Mark is a lasting sign of something that happened to a character.
type Mark struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Text   string `json:"text"`
	Day    int    `json:"day,omitempty"`
	Source string `json:"source,omitempty"` // engine or gm
	// Who a loss was: an NPC or a fellow character.
	NPCID  string `json:"npc_id,omitempty"`
	CharID string `json:"char_id,omitempty"`
}

// AddMark gives c a lasting mark. A mark of the same kind with the same
// text is not added twice (it returns nil, nil).
func AddMark(w *World, c *Character, m Mark) (*Mark, error) {
	if c == nil {
		return nil, fmt.Errorf("no character")
	}
	m.Text = strings.TrimSpace(m.Text)
	if m.Text == "" {
		return nil, fmt.Errorf("a mark needs text")
	}
	if !oneOf(m.Kind, MarkKinds) {
		return nil, fmt.Errorf("unknown mark kind %q", m.Kind)
	}
	m.Text = trimTo(m.Text, 120)
	for _, x := range c.Marks {
		if x.Kind == m.Kind && NameMatch(x.Text, m.Text) {
			return nil, nil
		}
	}
	if len(c.Marks) >= MaxMarks {
		return nil, fmt.Errorf("%s already carries %d marks", c.Name, MaxMarks)
	}
	if m.Source == "" {
		m.Source = SourceGM
	}
	m.Day = w.Campaign.Scene.Day
	m.ID = fmt.Sprintf("mark%d", len(c.Marks)+1)
	c.Marks = append(c.Marks, m)
	return &c.Marks[len(c.Marks)-1], nil
}

// MarksToday counts the marks of a source c got today.
func (c *Character) MarksToday(w *World, source string) int {
	n := 0
	for _, m := range c.Marks {
		if m.Day == w.Campaign.Scene.Day && m.Source == source {
			n++
		}
	}
	return n
}

// scarFromFall marks a character struck down in a fight. They get at most
// one such scar a day, so a bad fight doesn't cover them in them.
func scarFromFall(w *World, c *Character, by *Enemy) {
	if c.Dead {
		return
	}
	for _, m := range c.Marks {
		if m.Kind == MarkScar && m.Source == SourceEngine && m.Day == w.Campaign.Scene.Day {
			return
		}
	}
	text := "Struck down at " + placeName(w)
	if by != nil {
		text = fmt.Sprintf("Struck down by %s at %s", by.Name, placeName(w))
	}
	AddMark(w, c, Mark{Kind: MarkScar, Source: SourceEngine, Text: text})
}

// mournCharacter gives every living companion a loss when a character dies.
func mournCharacter(w *World, dead *Character) {
	for _, c := range w.Characters {
		if c.ID == dead.ID || c.Dead {
			continue
		}
		AddMark(w, c, Mark{Kind: MarkLoss, Source: SourceEngine, CharID: dead.ID,
			Text: fmt.Sprintf("Lost %s at %s", dead.Name, placeName(w))})
	}
}

// Tie kinds: how one NPC stands with one character.
const (
	TieGrudge = "grudge" // the NPC holds something against the character
	TieDebt   = "debt"   // the NPC owes the character
	TieOwed   = "owed"   // the character owes the NPC
)

// TieKinds lists every tie kind.
var TieKinds = []string{TieGrudge, TieDebt, TieOwed}

// MaxTies is how many ties one NPC keeps; the oldest goes first.
const MaxTies = 12

// Tie is what an NPC holds toward one character.
type Tie struct {
	CharID string `json:"char_id"`
	Kind   string `json:"kind"`
	Text   string `json:"text"` // why, e.g. "attacked them in the chapel"
	Day    int    `json:"day,omitempty"`
	Source string `json:"source,omitempty"`
}

// Bind records a tie from n to c. An existing tie of the same kind to the
// same character is replaced by the newer reason.
func (n *NPC) Bind(w *World, c *Character, kind, text, source string) {
	if n == nil || c == nil || !oneOf(kind, TieKinds) {
		return
	}
	t := Tie{CharID: c.ID, Kind: kind, Text: trimTo(strings.TrimSpace(text), 120), Day: w.Campaign.Scene.Day, Source: source}
	if t.Source == "" {
		t.Source = SourceGM
	}
	n.Settle(c.ID, kind)
	n.Ties = append(n.Ties, t)
	if len(n.Ties) > MaxTies {
		n.Ties = n.Ties[len(n.Ties)-MaxTies:]
	}
}

// Settle drops n's ties of a kind to a character ("" drops them all). It
// reports whether any were dropped.
func (n *NPC) Settle(charID, kind string) bool {
	if n == nil {
		return false
	}
	kept, dropped := n.Ties[:0], false
	for _, t := range n.Ties {
		if t.CharID == charID && (kind == "" || t.Kind == kind) {
			dropped = true
			continue
		}
		kept = append(kept, t)
	}
	n.Ties = kept
	if len(n.Ties) == 0 {
		n.Ties = nil
	}
	return dropped
}

// TiesTo returns n's ties to one character.
func (n *NPC) TiesTo(charID string) []Tie {
	var out []Tie
	for _, t := range n.Ties {
		if t.CharID == charID {
			out = append(out, t)
		}
	}
	return out
}

// AttitudeTo is how n feels about c in particular: the party-wide
// disposition, one worse for a grudge and one better for a debt they owe c.
func (n *NPC) AttitudeTo(c *Character) int {
	d := n.Disposition
	if c == nil {
		return d
	}
	for _, t := range n.TiesTo(c.ID) {
		switch t.Kind {
		case TieGrudge:
			d--
		case TieDebt:
			d++
		}
	}
	return clamp(d, -3, 3)
}

// TieLine renders a tie for the narrator and the info log, e.g. "Ned Pike
// holds a grudge against Cal (attacked them, day 3)".
func TieLine(w *World, n *NPC, t Tie) string {
	who := t.CharID
	if c := w.Character(t.CharID); c != nil {
		who = c.Name
	}
	var s string
	switch t.Kind {
	case TieGrudge:
		s = fmt.Sprintf("%s holds a grudge against %s", n.Name, who)
	case TieDebt:
		s = fmt.Sprintf("%s owes %s", n.Name, who)
	case TieOwed:
		s = fmt.Sprintf("%s owes %s", who, n.Name)
	}
	var why []string
	if t.Text != "" {
		why = append(why, t.Text)
	}
	if t.Day > 0 {
		why = append(why, fmt.Sprintf("day %d", t.Day))
	}
	if len(why) > 0 {
		s += " (" + strings.Join(why, ", ") + ")"
	}
	return s
}

func oneOf(s string, list []string) bool {
	for _, x := range list {
		if s == x {
			return true
		}
	}
	return false
}
