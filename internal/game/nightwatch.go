package game

import (
	"fmt"
	"strings"
)

// A night watch is a quest step about getting through a night somewhere:
// "Last the night in the attic room and end what visits it". Sleeping there
// always brings the visitor (no random roll), and the step is marked off
// only when that fight is won, never by the judge reading "they slept".

// nightWatchPhrases need the night itself in them: staying somewhere
// until morning, not any mention of watching or sleeping.
var nightWatchPhrases = []string{
	"last the night", "survive the night", "spend the night", "stay the night", "pass the night",
	"sleep the night", "the night in ", "the night at ", "a night in ", "a night at ",
	"night there", "until dawn", "till dawn", "until morning", "till morning",
}

// IsNightWatch reports whether a step is about getting through a night.
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

// WatchPlace is where a night-watch step is kept: its own Where, a place
// its text names, or the reach step just before it. "" when the engine
// can't place it, and then the judge keeps the step as plain freeform.
func WatchPlace(w *World, q *Quest, o *Objective) string {
	if !IsNightWatch(o) {
		return ""
	}
	if o.Where != "" {
		return o.Where
	}
	t := strings.ToLower(o.Text)
	best, where := "", ""
	for _, l := range w.Locations {
		if n := placeWords(l.Name); n != "" && strings.Contains(t, n) && len(n) > len(best) {
			best, where = n, l.ID
		}
	}
	if where != "" {
		return where
	}
	prev := ""
	for i := range q.Objectives {
		x := &q.Objectives[i]
		if x == o {
			return prev
		}
		if x.Kind == ObjReachLocation {
			prev = x.Target
		}
	}
	return ""
}

// EngineWatch reports whether the engine runs this step: a night watch it
// can place. Those come only by sleeping there and winning the fight.
func EngineWatch(w *World, q *Quest, o *Objective) bool {
	return WatchPlace(w, q, o) != ""
}

// NightWatch finds an open night-watch step kept here, on a quest someone
// here is on. A main quest is only looked at up to its first open step.
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
			if o.Done {
				continue
			}
			if o.Evidence == "" && WatchPlace(w, q, o) == l.ID {
				return q, o
			}
			if q.IsMain {
				break // main steps go in order
			}
		}
	}
	return nil, nil
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
