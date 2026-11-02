package game

import (
	"fmt"
	"strings"
)

// A character's own story: the goals they chase and a record of what
// they chose and did and how it turned out. The party's chronicle is the
// world's memory; this is each character's, so nothing personal gets lost
// in a long campaign. Only the engine writes it, from what resolves and
// from validated narrator directives.

// Record entry kinds.
const (
	RecChoice  = "choice"  // a decision that shaped things: a promise, a side taken, a mercy, an attack
	RecDeed    = "deed"    // something they did that people will talk about
	RecOutcome = "outcome" // how something turned out for them
)

// RecordKinds are the kinds the narrator may record.
var RecordKinds = []string{RecChoice, RecDeed, RecOutcome}

// MaxRecord is how many record entries a character keeps. Choices are
// kept before deeds and outcomes when it overflows.
const MaxRecord = 60

// RecordEntry is one line of a character's own story.
type RecordEntry struct {
	Day    int    `json:"day,omitempty"`
	Turn   int    `json:"turn"`
	Kind   string `json:"kind"`
	Text   string `json:"text"`
	Source string `json:"source,omitempty"`  // engine or gm
	NPCID  string `json:"npc_id,omitempty"`  // who it involved
	GoalID string `json:"goal_id,omitempty"` // the goal it settled
}

// Goal states.
const (
	GoalOpen    = "open"
	GoalDone    = "done"
	GoalFailed  = "failed"
	GoalDropped = "dropped" // the player let it go
)

// MaxOpenGoals is how many goals a character can chase at once.
const MaxOpenGoals = 4

// MaxGoals is how many goals, open and settled, a character keeps.
const MaxGoals = 24

// GoalXP is what reaching a personal goal is worth.
const GoalXP = 25

// Goal is something one character wants, set by their player or offered
// by the story, and settled by what happens.
type Goal struct {
	ID      string `json:"id"`
	Text    string `json:"text"`
	Status  string `json:"status"`
	Source  string `json:"source,omitempty"` // player or gm
	Day     int    `json:"day,omitempty"`
	Settled int    `json:"settled,omitempty"` // day it was settled
	Outcome string `json:"outcome,omitempty"`
}

// SourcePlayer marks what a player wrote themselves.
const SourcePlayer = "player"

// Note adds an entry to c's record. A repeat of an entry already there is
// skipped. It returns the entry, or nil when nothing was added.
func Note(w *World, c *Character, e RecordEntry) *RecordEntry {
	if c == nil {
		return nil
	}
	e.Text = trimTo(strings.TrimSpace(e.Text), 200)
	if e.Text == "" {
		return nil
	}
	if !oneOf(e.Kind, RecordKinds) {
		e.Kind = RecDeed
	}
	for _, x := range c.Record {
		if NameMatch(x.Text, e.Text) {
			return nil
		}
	}
	if e.Source == "" {
		e.Source = SourceGM
	}
	e.Day, e.Turn = w.Campaign.Scene.Day, w.Campaign.Turn
	c.Record = append(c.Record, e)
	trimRecord(c)
	return &c.Record[len(c.Record)-1]
}

// trimRecord drops the oldest deeds and outcomes first, then the oldest
// choices, until the record fits.
func trimRecord(c *Character) {
	for len(c.Record) > MaxRecord {
		drop := 0
		for i, e := range c.Record {
			if e.Kind != RecChoice {
				drop = i
				break
			}
		}
		c.Record = append(c.Record[:drop], c.Record[drop+1:]...)
	}
}

// OpenGoals lists the goals c still chases.
func (c *Character) OpenGoals() []*Goal {
	var out []*Goal
	for i := range c.Goals {
		if c.Goals[i].Status == GoalOpen {
			out = append(out, &c.Goals[i])
		}
	}
	return out
}

// Goal looks up one of c's goals by id.
func (c *Character) Goal(id string) *Goal {
	for i := range c.Goals {
		if c.Goals[i].ID == id {
			return &c.Goals[i]
		}
	}
	return nil
}

// AddGoal gives c a new goal. source is player or gm.
func AddGoal(w *World, c *Character, text, source string) (*Goal, error) {
	text = trimTo(strings.TrimSpace(text), 140)
	if text == "" {
		return nil, fmt.Errorf("a goal needs words")
	}
	if c.Dead {
		return nil, fmt.Errorf("%s is dead", c.Name)
	}
	open := c.OpenGoals()
	if len(open) >= MaxOpenGoals {
		return nil, fmt.Errorf("%s already chases %d goals; settle or drop one first", c.Name, MaxOpenGoals)
	}
	for _, g := range open {
		if NameMatch(g.Text, text) {
			return nil, fmt.Errorf("%s already wants that", c.Name)
		}
	}
	if source == "" {
		source = SourceGM
	}
	next := 1
	for _, g := range c.Goals {
		var n int
		if _, err := fmt.Sscanf(g.ID, "goal%d", &n); err == nil && n >= next {
			next = n + 1
		}
	}
	// keep the newest settled goals; open ones are never dropped
	for len(c.Goals) >= MaxGoals {
		for i, g := range c.Goals {
			if g.Status != GoalOpen {
				c.Goals = append(c.Goals[:i], c.Goals[i+1:]...)
				break
			}
		}
	}
	c.Goals = append(c.Goals, Goal{ID: fmt.Sprintf("goal%d", next), Text: text, Status: GoalOpen, Source: source, Day: w.Campaign.Scene.Day})
	return &c.Goals[len(c.Goals)-1], nil
}

// SettleGoal closes one of c's goals: done, failed or dropped. Reaching a
// goal is worth GoalXP, and every settled goal goes on their record.
// It returns the change lines.
func SettleGoal(w *World, c *Character, id, status, outcome string) ([]string, error) {
	g := c.Goal(id)
	if g == nil {
		return nil, fmt.Errorf("%s has no goal %q", c.Name, id)
	}
	if g.Status != GoalOpen {
		return nil, fmt.Errorf("that goal is already %s", g.Status)
	}
	if status != GoalDone && status != GoalFailed && status != GoalDropped {
		return nil, fmt.Errorf("a goal ends done, failed or dropped")
	}
	g.Status, g.Outcome, g.Settled = status, trimTo(strings.TrimSpace(outcome), 160), w.Campaign.Scene.Day
	var changes []string
	switch status {
	case GoalDone:
		changes = append(changes, fmt.Sprintf("%s reaches a goal: %s", c.Name, g.Text))
		changes = append(changes, AwardXP(c, GoalXP)...)
		Note(w, c, RecordEntry{Kind: RecOutcome, Source: SourceEngine, GoalID: g.ID, Text: "Reached a goal: " + g.Text + outcomeTail(g.Outcome)})
	case GoalFailed:
		changes = append(changes, fmt.Sprintf("%s's goal is lost: %s", c.Name, g.Text))
		Note(w, c, RecordEntry{Kind: RecOutcome, Source: SourceEngine, GoalID: g.ID, Text: "Lost a goal: " + g.Text + outcomeTail(g.Outcome)})
	case GoalDropped:
		Note(w, c, RecordEntry{Kind: RecChoice, Source: SourcePlayer, GoalID: g.ID, Text: "Let go of a goal: " + g.Text})
	}
	return changes, nil
}

// GoalsSettledToday counts c's goals settled today by the story (done or
// failed), so the narrator can't hand out goal XP every turn.
func (c *Character) GoalsSettledToday(w *World) int {
	n := 0
	for _, g := range c.Goals {
		if g.Settled == w.Campaign.Scene.Day && (g.Status == GoalDone || g.Status == GoalFailed) {
			n++
		}
	}
	return n
}

func outcomeTail(s string) string {
	if s == "" {
		return ""
	}
	return " (" + s + ")"
}
