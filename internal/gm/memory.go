package gm

import (
	"fmt"
	"strings"

	"github.com/Solvely-Colin/aidnd/internal/game"
)

// Limits for the WORLD MEMORY block.
const (
	memHere     = 4 // facts set in this place
	memPerNPC   = 2 // facts about each person in the scene
	memPromises = 5
	memRecent   = 3 // other recent facts
	memDead     = 6
)

// worldMemory renders what the world remembers that bears on this scene:
// facts set here, facts about the people present, open promises, a few
// recent facts and the named dead. Everything comes from the chronicle and
// NPC records, so long campaigns stay consistent past the recent log.
func worldMemory(w *game.World, actor *game.Character) string {
	l := w.Here()
	shown := map[string]bool{}
	var b strings.Builder
	line := func(label string, facts []string) {
		if len(facts) > 0 {
			fmt.Fprintf(&b, "%s: %s\n", label, strings.Join(facts, "; "))
		}
	}
	var promises []string
	for _, f := range lastN(w.OpenPromises(), memPromises) {
		shown[f.ID] = true
		promises = append(promises, fmt.Sprintf("[%s] %s", f.ID, factText(w, f)))
	}
	var here []string
	for _, f := range lastN(w.FactsAt(l.ID), memHere+len(shown)) {
		if !shown[f.ID] && len(here) < memHere {
			shown[f.ID] = true
			here = append(here, factText(w, f))
		}
	}
	var about []string
	for _, n := range w.NPCsAt(l.ID) {
		if actor != nil && !withActor(l, actor, n) {
			continue
		}
		k := 0
		for _, f := range lastN(w.FactsAbout(n.ID), memPerNPC+len(shown)) {
			if !shown[f.ID] && k < memPerNPC {
				shown[f.ID], k = true, k+1
				about = append(about, factText(w, f))
			}
		}
	}
	var recent []string
	for i := len(w.Chronicle) - 1; i >= 0 && len(recent) < memRecent; i-- {
		if f := &w.Chronicle[i]; !shown[f.ID] {
			shown[f.ID] = true
			recent = append([]string{factText(w, f)}, recent...)
		}
	}
	var dead []string
	for _, n := range w.NPCs {
		if !n.Alive && len(dead) < memDead {
			dead = append(dead, n.Name)
		}
	}
	line("HAPPENED HERE BEFORE", here)
	line("ABOUT THE PEOPLE HERE", about)
	line("PARTY'S OPEN PROMISES (call resolve_promise when one is clearly kept or broken)", promises)
	line("EARLIER", recent)
	line("DEAD (never bring them back or have them appear)", dead)
	if b.Len() == 0 {
		return ""
	}
	return "WORLD MEMORY (established facts; stay consistent with them):\n" + b.String()
}

// factText renders one fact with its day and, when not here, its place.
func factText(w *game.World, f *game.ChronicleEntry) string {
	s := f.Text
	if f.Kind == game.FactRumor {
		s = "rumor: " + s
	}
	if f.Day > 0 {
		s = fmt.Sprintf("day %d, %s", f.Day, s)
	}
	if f.LocationID != "" && f.LocationID != w.Campaign.LocationID {
		if l := w.Location(f.LocationID); l != nil && !strings.Contains(f.Text, l.Name) {
			s += " (at " + l.Name + ")"
		}
	}
	if f.Status == game.PromiseKept || f.Status == game.PromiseBroken {
		s += " [" + f.Status + "]"
	}
	return s
}

func lastN[T any](s []T, n int) []T {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

// npcIDs keeps the ids that name a known NPC.
func npcIDs(w *game.World, refs []string) []string {
	var out []string
	for _, r := range refs {
		if n := w.NPC(r); n != nil {
			out = append(out, n.ID)
		}
	}
	return out
}

// factsSince lists facts recorded after a turn, for the story summary.
func factsSince(w *game.World, turn int) []string {
	var out []string
	for i := range w.Chronicle {
		if f := &w.Chronicle[i]; f.Turn > turn {
			out = append(out, factText(w, f))
		}
	}
	return lastN(out, 12)
}
