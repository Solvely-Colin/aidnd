package game

import (
	"fmt"

	"github.com/Solvely-Colin/aidnd/internal/dice"
)

// NPC agendas: people in a living world follow their own goals between
// sessions. An agenda is a short chain of authored steps; on a world tick
// each running agenda rolls a d6 and takes its next step on 4 or more.
// A step can move its person within their region, shift how someone feels
// about strangers, or push a faction's clock along. Every step is engine
// state with story-safety rules, so nothing here can kill anyone, empty a
// scene players are standing in, or finish a threat's plot.

// AgendaStep is one thing a person does toward their goal.
type AgendaStep struct {
	// Text is how the step looks from outside, recorded as news.
	Text string `json:"text"`
	// If holds the step back until it is true.
	If *AgendaIf `json:"if,omitempty"`
	// Effects are what the step changes, all or nothing.
	Effects []AgendaEffect `json:"effects,omitempty"`
}

// AgendaIf is a step's condition. Unset parts always hold.
type AgendaIf struct {
	// DispositionAtMost is met when the agenda's person feels this way
	// about strangers or worse.
	DispositionAtMost *int `json:"disposition_at_most,omitempty"`
	// Clock and ClockAtLeast are met once that faction's clock has moved
	// that many steps.
	Clock        string `json:"clock,omitempty"`
	ClockAtLeast int    `json:"clock_at_least,omitempty"`
}

// AgendaEffect is one change a step makes.
type AgendaEffect struct {
	// NPCID is who the effect is on ("" is the agenda's own person).
	NPCID string `json:"npc_id,omitempty"`
	// MoveTo moves them to this location id.
	MoveTo string `json:"move_to,omitempty"`
	// Disposition shifts how they feel about strangers (kept in -3..3).
	Disposition int `json:"disposition,omitempty"`
	// Clock pushes that faction's clock one step (never its last).
	Clock string `json:"clock,omitempty"`
}

// Agenda is a person's goal and how far along it they are.
type Agenda struct {
	Key   string `json:"key"`
	NPCID string `json:"npc_id"`
	Goal  string `json:"goal"`
	// Region is the saga region the agenda belongs to; moves stay inside it.
	Region string `json:"region,omitempty"`
	// Storyline is the region's main quest; the agenda stops when it ends.
	Storyline string       `json:"storyline,omitempty"`
	FromWeek  int          `json:"from_week,omitempty"`
	Steps     []AgendaStep `json:"steps"`
	Progress  int          `json:"progress"`
	Done      bool         `json:"done,omitempty"`
}

// AgendaRoll is the d6 an agenda needs to take its next step.
const AgendaRoll = 4

// running reports whether an agenda can still move.
func (a *Agenda) running(w *World) bool {
	if a.Done || a.Progress >= len(a.Steps) || w.Campaign.Living.Week < a.FromWeek {
		return false
	}
	if q := w.Quest(a.Storyline); q != nil && (q.Status == QuestCompleted || q.Status == QuestTurnedIn || q.Status == QuestFailed) {
		return false
	}
	return true
}

// Faction finds a living-world faction by key.
func (lv *Living) Faction(key string) *Faction {
	for _, f := range lv.Factions {
		if f.Key == key {
			return f
		}
	}
	return nil
}

// meets reports whether a step's condition holds.
func (st *AgendaStep) meets(w *World, n *NPC) bool {
	c := st.If
	if c == nil {
		return true
	}
	if c.DispositionAtMost != nil && n.Disposition > *c.DispositionAtMost {
		return false
	}
	if c.Clock != "" {
		f := w.Campaign.Living.Faction(c.Clock)
		if f == nil || f.Progress < c.ClockAtLeast {
			return false
		}
	}
	return true
}

// occupied reports whether a group stands at a location: people there are
// in someone's scene and stay put until the players leave.
func occupied(w *World, locID string) bool {
	for _, gi := range w.GroupInfos() {
		if gi.LocationID == locID && len(gi.Members) > 0 {
			return true
		}
	}
	return false
}

// blocked says why an effect can't happen now ("" when it can).
func (a *Agenda) blocked(w *World, e AgendaEffect) string {
	who := a.NPCID
	if e.NPCID != "" {
		who = e.NPCID
	}
	n := w.NPC(who)
	if n == nil || !n.Alive {
		return "gone"
	}
	if e.MoveTo != "" && e.MoveTo != n.LocationID {
		to := w.Location(e.MoveTo)
		switch {
		case to == nil || to.Stub || to.IsLair:
			return "no such place"
		case a.Region != "" && to.Region != a.Region:
			return "outside the region"
		case n.LocationID != "" && occupied(w, n.LocationID):
			return "in a scene"
		case occupied(w, to.ID) && w.InCombatAt(to.ID):
			return "a fight there"
		}
	}
	if e.Clock != "" {
		f := w.Campaign.Living.Faction(e.Clock)
		if f == nil || !f.active(w) || f.Progress+1 >= f.Steps {
			return "clock"
		}
	}
	return ""
}

// apply makes an effect happen and returns the faction move it caused, if
// any.
func (a *Agenda) apply(w *World, e AgendaEffect) string {
	who := a.NPCID
	if e.NPCID != "" {
		who = e.NPCID
	}
	n := w.NPC(who)
	if e.MoveTo != "" {
		n.LocationID, n.ZoneID = e.MoveTo, ""
	}
	if e.Disposition != 0 {
		n.Disposition = min(3, max(-3, n.Disposition+e.Disposition))
	}
	if e.Clock != "" {
		f := w.Campaign.Living.Faction(e.Clock)
		f.Progress++
		if len(f.Moves) > 0 {
			return f.Moves[(f.Progress-1)%len(f.Moves)]
		}
	}
	return ""
}

// AgendaMove is one step a person took on a world tick.
type AgendaMove struct {
	NPCID string `json:"npc_id"`
	Text  string `json:"text"`
	// LocationID is where it happened (where they ended up).
	LocationID string `json:"location_id,omitempty"`
	// Clock is the faction move the step caused ("" for none).
	Clock string `json:"clock,omitempty"`
}

// RunAgendas gives every running agenda its roll. A step whose condition
// fails or whose effects are blocked waits for a later tick.
func RunAgendas(w *World, r *dice.Roller) []AgendaMove {
	lv := w.Campaign.Living
	if lv == nil {
		return nil
	}
	var out []AgendaMove
	for _, a := range lv.Agendas {
		if !a.running(w) {
			continue
		}
		n := w.NPC(a.NPCID)
		if n == nil || !n.Alive {
			a.Done = true // their plans died with them
			continue
		}
		if r.Die(6) < AgendaRoll {
			continue
		}
		st := &a.Steps[a.Progress]
		if !st.meets(w, n) {
			continue
		}
		ok := true
		for _, e := range st.Effects {
			if a.blocked(w, e) != "" {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		mv := AgendaMove{NPCID: n.ID, Text: st.Text}
		for _, e := range st.Effects {
			if c := a.apply(w, e); c != "" {
				mv.Clock = c
			}
		}
		mv.LocationID = n.LocationID
		a.Progress++
		if a.Progress >= len(a.Steps) {
			a.Done = true
		}
		out = append(out, mv)
	}
	return out
}

// InCombatAt reports whether a group is fighting at a location.
func (w *World) InCombatAt(locID string) bool {
	for _, gi := range w.GroupInfos() {
		if gi.LocationID == locID && gi.Fighting {
			return true
		}
	}
	return false
}

// AgendaFor finds a person's agenda.
func (lv *Living) AgendaFor(npcID string) *Agenda {
	if lv == nil {
		return nil
	}
	for _, a := range lv.Agendas {
		if a.NPCID == npcID {
			return a
		}
	}
	return nil
}

// agendaFact records an agenda move in the chronicle, tied to its person
// and place so the narrator's world memory finds it there.
func agendaFact(w *World, week int, mv AgendaMove) {
	w.Record(ChronicleEntry{Kind: FactNews, Source: SourceEngine, LocationID: mv.LocationID, NPCIDs: []string{mv.NPCID},
		Text: fmt.Sprintf("Week %d: %s", week, mv.Text)})
}
