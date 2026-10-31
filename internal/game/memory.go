package game

import (
	"fmt"
	"strings"
)

// Fact kinds kept in the chronicle.
const (
	FactNote    = "fact"    // something learned or established
	FactDeed    = "deed"    // something the party did that people will remember
	FactDeath   = "death"   // a named person died
	FactPromise = "promise" // the party gave its word (open until kept or broken)
	FactRumor   = "rumor"   // hearsay, may be false
)

// FactKinds are the kinds the narrator may record.
var FactKinds = []string{FactNote, FactDeed, FactPromise, FactRumor}

// Promise states.
const (
	PromiseOpen   = "open"
	PromiseKept   = "kept"
	PromiseBroken = "broken"
)

// Fact sources.
const (
	SourceEngine = "engine"
	SourceGM     = "gm"
)

// MaxNPCKeeps is how many permanent memories an NPC holds.
const MaxNPCKeeps = 6

// Keep stores a memory the NPC never forgets (being attacked, a gift, a
// kept or broken promise). The oldest is dropped past MaxNPCKeeps.
func (n *NPC) Keep(note string) {
	note = strings.TrimSpace(note)
	if note == "" {
		return
	}
	if len(note) > 160 {
		note = note[:160]
	}
	for _, k := range n.Keeps {
		if k == note {
			return
		}
	}
	n.Keeps = append(n.Keeps, note)
	if len(n.Keeps) > MaxNPCKeeps {
		n.Keeps = n.Keeps[len(n.Keeps)-MaxNPCKeeps:]
	}
}

// Recall returns everything the NPC remembers, permanent memories first.
func (n *NPC) Recall() []string {
	out := append([]string(nil), n.Keeps...)
	for _, m := range n.Memory {
		dup := false
		for _, k := range n.Keeps {
			dup = dup || k == m
		}
		if !dup {
			out = append(out, m)
		}
	}
	return out
}

// Record adds a fact to the chronicle, stamped with the turn, day and
// (unless given) the party's location. A near-duplicate of an existing
// fact is ignored and returns nil.
func (w *World) Record(f ChronicleEntry) *ChronicleEntry {
	f.Text = strings.TrimSpace(f.Text)
	if f.Text == "" {
		return nil
	}
	for _, c := range w.Chronicle {
		if NameMatch(c.Text, f.Text) {
			return nil
		}
	}
	if f.Kind == "" {
		f.Kind = FactNote
	}
	if f.Source == "" {
		f.Source = SourceGM
	}
	if f.LocationID == "" {
		f.LocationID = w.Campaign.LocationID
	}
	if f.Kind == FactPromise && f.Status == "" {
		f.Status = PromiseOpen
	}
	f.Turn, f.Day = w.Campaign.Turn, w.Campaign.Scene.Day
	f.ID = fmt.Sprintf("fact%d", len(w.Chronicle)+1)
	w.Chronicle = append(w.Chronicle, f)
	return &w.Chronicle[len(w.Chronicle)-1]
}

// Fact looks up a chronicle entry by id.
func (w *World) Fact(id string) *ChronicleEntry {
	for i := range w.Chronicle {
		if w.Chronicle[i].ID == id {
			return &w.Chronicle[i]
		}
	}
	return nil
}

// OpenPromises lists promises not yet kept or broken.
func (w *World) OpenPromises() []*ChronicleEntry {
	var out []*ChronicleEntry
	for i := range w.Chronicle {
		if c := &w.Chronicle[i]; c.Kind == FactPromise && c.Status == PromiseOpen {
			out = append(out, c)
		}
	}
	return out
}

// ResolvePromise marks a promise kept or broken. Everyone it was made to
// remembers it for good and warms or cools to the party by one step.
// Promises tied to a quest are settled by the quest, not by this.
func ResolvePromise(w *World, id string, kept bool, outcome string) (string, error) {
	f := w.Fact(id)
	if f == nil || f.Kind != FactPromise {
		return "", fmt.Errorf("no promise %q", id)
	}
	if f.Status != PromiseOpen {
		return "", fmt.Errorf("that promise is already %s", f.Status)
	}
	if f.QuestID != "" {
		return "", fmt.Errorf("that promise is a quest; it is kept by turning the quest in")
	}
	settlePromise(w, f, kept, outcome)
	verb := "kept"
	if !kept {
		verb = "broke"
	}
	return fmt.Sprintf("The party %s its word: %s", verb, f.Text), nil
}

func settlePromise(w *World, f *ChronicleEntry, kept bool, outcome string) {
	f.Status, f.Outcome = PromiseKept, strings.TrimSpace(outcome)
	delta, verb := 1, "kept their word"
	if !kept {
		f.Status, delta, verb = PromiseBroken, -1, "broke their word"
		MoveTide(w, 3) // a broken promise feeds the dark
	}
	for _, id := range f.NPCIDs {
		n := w.NPC(id)
		if n == nil || !n.Alive {
			continue
		}
		n.Keep(fmt.Sprintf("day %d: the party %s (%s)", w.Campaign.Scene.Day, verb, f.Text))
		n.Disposition = clamp(n.Disposition+delta, -3, 3)
		for _, c := range w.Party() {
			if c.Dead {
				continue
			}
			if kept {
				Note(w, c, RecordEntry{Kind: RecOutcome, Source: SourceEngine, NPCID: n.ID, Text: "Kept the party's word: " + f.Text})
			} else {
				n.Bind(w, c, TieGrudge, "broke their word: "+f.Text, SourceEngine)
				Note(w, c, RecordEntry{Kind: RecOutcome, Source: SourceEngine, NPCID: n.ID, Text: "Broke the party's word: " + f.Text})
			}
		}
	}
}

// keepQuestPromise settles the promise made when a quest was accepted.
func keepQuestPromise(w *World, q *Quest) {
	for i := range w.Chronicle {
		if f := &w.Chronicle[i]; f.QuestID == q.ID && f.Kind == FactPromise && f.Status == PromiseOpen {
			f.Status, f.Outcome = PromiseKept, "quest done"
		}
	}
}

// questPromise records accepting a quest as the party's word to the giver.
func questPromise(w *World, q *Quest) {
	f := ChronicleEntry{Kind: FactPromise, Source: SourceEngine, QuestID: q.ID, Text: "The party took on " + q.Title}
	if g := w.NPC(q.GiverID); g != nil {
		f.Text = fmt.Sprintf("The party promised %s: %s", g.Name, q.Title)
		f.NPCIDs = []string{g.ID}
		f.LocationID = g.LocationID
	}
	w.Record(f)
}

// FactsAt returns facts set at a location, oldest first.
func (w *World) FactsAt(locID string) []*ChronicleEntry {
	var out []*ChronicleEntry
	for i := range w.Chronicle {
		if w.Chronicle[i].LocationID == locID {
			out = append(out, &w.Chronicle[i])
		}
	}
	return out
}

// FactsAbout returns facts naming an NPC, oldest first.
func (w *World) FactsAbout(npcID string) []*ChronicleEntry {
	var out []*ChronicleEntry
	for i := range w.Chronicle {
		for _, id := range w.Chronicle[i].NPCIDs {
			if id == npcID {
				out = append(out, &w.Chronicle[i])
				break
			}
		}
	}
	return out
}

// LoreOpen reports whether a lore entry may be shown to the narrator: its
// gate (an objective or quest id) must be done. A gate that no longer
// exists stays shut, so a secret is never spilled by a missing id.
func LoreOpen(w *World, l *Lore) bool {
	if l.Gate == "" {
		return true
	}
	for _, q := range w.Quests {
		if q.ID == l.Gate {
			return q.Status == QuestCompleted || q.Status == QuestTurnedIn
		}
		for _, o := range q.Objectives {
			if o.ID == l.Gate {
				return o.Done
			}
		}
	}
	return false
}
