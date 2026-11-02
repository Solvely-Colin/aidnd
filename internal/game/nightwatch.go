package game

import (
	"fmt"
	"strings"
)

// A night watch is a quest step about getting through a night somewhere:
// "Last the night in the attic room and end what visits it". Sleeping there
// always brings the visitor (no random roll), and the step is marked off
// only when that fight is won, never by the judge reading "they slept".

var nightWatchPhrases = []string{
	"the night in", "the night at", "the night there", "night in the", "a night in",
	"last the night", "survive the night", "spend the night", "stay the night", "pass the night",
	"through the night", "until dawn", "till dawn", "sleep in", "sleep at", "sleep there",
	"what visits", "what comes in the night", "keep watch", "keep vigil",
}

// IsNightWatch reports whether a step is a night watch.
func IsNightWatch(o *Objective) bool {
	if o == nil || o.Kind != ObjFreeform {
		return false
	}
	t := strings.ToLower(o.Text)
	for _, p := range nightWatchPhrases {
		if strings.Contains(t, p) {
			return true
		}
	}
	return false
}

// NightWatch finds an open night-watch step for a quest someone here is on
// and that belongs to this place: the step names it, or the quest sends
// the party here (not merely where the quest was heard of).
func NightWatch(w *World, l *Location) (*Quest, *Objective) {
	if l == nil {
		return nil, nil
	}
	for _, q := range w.Quests {
		if q.Status != QuestActive || !QuestHere(w, q) {
			continue
		}
		for i := range q.Objectives {
			o := &q.Objectives[i]
			if o.Done || !IsNightWatch(o) {
				if q.IsMain && !o.Done {
					break // main steps go in order
				}
				continue
			}
			if nightWatchPlace(q, o, l) {
				return q, o
			}
		}
	}
	return nil, nil
}

func nightWatchPlace(q *Quest, o *Objective, l *Location) bool {
	if o.Where == l.ID {
		return true
	}
	if name := placeWords(l.Name); name != "" && strings.Contains(strings.ToLower(o.Text), name) {
		return true
	}
	for _, x := range q.Objectives {
		if x.Kind == ObjReachLocation && x.Target == l.ID {
			return true
		}
	}
	return false
}

// placeWords is a place's name without a leading article, lowercased.
func placeWords(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	for _, a := range []string{"the ", "a ", "an "} {
		n = strings.TrimPrefix(n, a)
	}
	if len(n) < 4 {
		return ""
	}
	return n
}

// nightVisit is a night's sleep on a night-watch step: the party beds down
// and what the quest promised comes for them.
func nightVisit(ctx *Ctx, l *Location, q *Quest, o *Objective, st *Step) {
	w := ctx.W
	w.Campaign.Scene.Time = len(TimesOfDay) - 1
	st.Changes = append(st.Changes, fmt.Sprintf("The party beds down at %s for %q, and in the dead of night something comes", l.Name, q.Title))
	var specs []EnemySpec
	if ctx.Visitor != nil {
		specs = ctx.Visitor(l, o.Text)
	}
	if len(specs) == 0 && ctx.Ambush != nil {
		specs = ctx.Ambush(l)
	}
	if len(specs) == 0 {
		specs = GenericEnemies(l.Kind, max(1, l.Danger), len(w.Characters))
	}
	if len(specs) == 0 {
		specs = []EnemySpec{{Name: "The night visitor", HP: 12, AC: 12, AttackBonus: 3, Damage: "1d8"}}
	}
	enc, _, err := StartEncounter(w, specs)
	if err != nil {
		return
	}
	enc.Quest = q.ID + "/" + o.ID
	var names []string
	for _, e := range enc.Enemies {
		names = append(names, e.Display())
	}
	st.Changes = append(st.Changes, fmt.Sprintf("Woken at %s: %s", l.Name, strings.Join(names, ", ")))
	st.CombatStarted = true
	surprise(ctx, enc, st)
}

// nightWatchWon marks a won night-watch fight on its step; EvaluateQuests
// then ticks it off and pays out.
func nightWatchWon(w *World, enc *Encounter) []string {
	qid, oid, ok := strings.Cut(enc.Quest, "/")
	if !ok {
		return nil
	}
	q := w.Quest(qid)
	if q == nil || q.Status != QuestActive {
		return nil
	}
	for i := range q.Objectives {
		o := &q.Objectives[i]
		if o.ID == oid && !o.Done {
			l := w.Here()
			where := "the night"
			if l != nil {
				where = l.Name
			}
			o.Evidence = fmt.Sprintf("Lasted the night at %s and put down what came", where)
			return nil
		}
	}
	return nil
}
