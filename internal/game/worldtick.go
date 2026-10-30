package game

import (
	"fmt"

	"github.com/Solvely-Colin/aidnd/internal/dice"
)

// The living world. In a long campaign the world takes a turn once a real
// day (a world tick), even when nobody plays: the shared calendar moves on a
// week, factions push their agendas, and the Tide rises. Everything here is
// engine rules with seeded dice; the model only writes the Gazette from the
// facts a tick produces.

// The Tide runs 0..100: how bad things are getting.
const (
	TideMax = 100
	// TideIdleCap is as high as ticks with nobody playing can push the
	// Tide, so a holiday can't flood the world.
	TideIdleCap = 66
	// Tide bands, recorded when a storyline ends.
	TideEbb   = "Ebb"
	TideSlack = "Slack"
	TideFlood = "Flood"
)

// FactNews is a chronicle fact from the Gazette or a world tick.
const FactNews = "news"

// Living is the shared world's clock and the agendas that move on it.
type Living struct {
	Week int `json:"week"` // world weeks since the campaign began
	Tide int `json:"tide"`
	// LastTickDay is the day index (see TickDay) of the last tick.
	LastTickDay int64 `json:"last_tick_day"`
	// LastActed is when a player last took a turn (unix seconds); a tick
	// with nobody playing since the one before is idle.
	LastActed int64      `json:"last_acted"`
	LastTick  int64      `json:"last_tick"`
	Factions  []*Faction `json:"factions,omitempty"`
	// Bands records the Tide band each storyline ended in, by quest id.
	Bands map[string]string `json:"bands,omitempty"`
	// Gazette is the latest front page.
	Gazette *Gazette `json:"gazette,omitempty"`
}

// Faction is a group with an agenda clock that fills on world ticks.
// Threats are not human and don't bargain; when one's clock fills it raises
// the Tide.
type Faction struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Threat bool   `json:"threat,omitempty"`
	Region string `json:"region,omitempty"`
	// Storyline is the main quest of the faction's region; its clock stops
	// once that storyline is over.
	Storyline string `json:"storyline,omitempty"`
	Clock     string `json:"clock"`
	Steps     int    `json:"steps"`
	Progress  int    `json:"progress"`
	// FromWeek is the first world week the faction acts in.
	FromWeek int `json:"from_week,omitempty"`
	// Moves are what each step looks like from outside, for the Gazette.
	Moves []string `json:"moves,omitempty"`
	// Full is the fact recorded when the clock fills; FullDanger raises
	// places' danger (location id -> at least this).
	Full       string         `json:"full,omitempty"`
	FullDanger map[string]int `json:"full_danger,omitempty"`
	Filled     bool           `json:"filled,omitempty"`
}

// Gazette is a world tick's front page.
type Gazette struct {
	Week       int      `json:"week"`
	Headlines  []string `json:"headlines"`
	Classified string   `json:"classified,omitempty"`
	Weather    string   `json:"weather,omitempty"`
}

// TickReport is what a world tick did, in plain facts.
type TickReport struct {
	Week     int      `json:"week"`
	Idle     bool     `json:"idle"`
	TideFrom int      `json:"tide_from"`
	Tide     int      `json:"tide"`
	Facts    []string `json:"facts"`
	Filled   []string `json:"filled,omitempty"` // faction names
}

// TickHourUTC is when the world's day turns over (about 5am in New England).
const TickHourUTC = 10

// TickDay is the world-tick day index of a unix time.
func TickDay(unix int64) int64 { return (unix - TickHourUTC*3600) / 86400 }

// TideBand names the band a Tide value is in.
func TideBand(t int) string {
	switch {
	case t <= 33:
		return TideEbb
	case t <= 66:
		return TideSlack
	}
	return TideFlood
}

// MoveTide changes the Tide within 0..100 and returns the change made.
func MoveTide(w *World, by int) int {
	lv := w.Campaign.Living
	if lv == nil {
		return 0
	}
	from := lv.Tide
	lv.Tide = min(TideMax, max(0, lv.Tide+by))
	return lv.Tide - from
}

// NoteActed marks that a player took a turn now.
func NoteActed(w *World, now int64) {
	if lv := w.Campaign.Living; lv != nil {
		lv.LastActed = now
	}
}

// active reports whether a faction's clock still runs.
func (f *Faction) active(w *World) bool {
	if f.Filled || w.Campaign.Living.Week < f.FromWeek {
		return false
	}
	if q := w.Quest(f.Storyline); q != nil && (q.Status == QuestCompleted || q.Status == QuestTurnedIn || q.Status == QuestFailed) {
		return false
	}
	return true
}

// WorldTick advances the world a week. Each faction whose clock runs rolls
// a d6 and moves a step on 5 or 6. At most one threat clock fills per tick,
// and a tick with nobody playing can't fill a threat clock at all or push
// the Tide past TideIdleCap. The world must be settled.
func WorldTick(w *World, r *dice.Roller, now int64, idle bool) *TickReport {
	lv := w.Campaign.Living
	if lv == nil {
		return nil
	}
	lv.Week++
	lv.LastTick = now
	rep := &TickReport{Week: lv.Week, Idle: idle, TideFrom: lv.Tide}
	fact := func(text string) {
		rep.Facts = append(rep.Facts, text)
		w.Record(ChronicleEntry{Kind: FactNews, Source: SourceEngine, Text: fmt.Sprintf("Week %d: %s", lv.Week, text)})
	}
	threatFilled := false
	for _, f := range lv.Factions {
		if !f.active(w) || r.Die(6) < 5 {
			continue
		}
		last := f.Progress+1 >= f.Steps
		if last && f.Threat && (threatFilled || idle) {
			continue // story safety: the clock waits a tick
		}
		f.Progress++
		if len(f.Moves) > 0 {
			fact(f.Moves[(f.Progress-1)%len(f.Moves)])
		}
		if !last {
			continue
		}
		f.Filled = true
		rep.Filled = append(rep.Filled, f.Name)
		if f.Full != "" {
			fact(f.Full)
		}
		for id, d := range f.FullDanger {
			if l := w.Location(id); l != nil && l.Danger < d && !l.IsHub {
				l.Danger = d
			}
		}
		if f.Threat {
			threatFilled = true
			MoveTide(w, 1+r.Die(4)) // 2..5
		}
	}
	if !idle || lv.Tide < TideIdleCap {
		MoveTide(w, 1)
	}
	rep.Tide = lv.Tide
	return rep
}

// TicksDue is how many world ticks are owed at now (at most max), and
// starts the clock on a world that has never ticked.
func TicksDue(w *World, now int64, max int) int {
	lv := w.Campaign.Living
	if lv == nil {
		return 0
	}
	day := TickDay(now)
	if lv.LastTickDay == 0 {
		lv.LastTickDay = day
		return 0
	}
	return int(min(int64(max), day-lv.LastTickDay))
}

// tideOnQuest lowers the Tide when a quest that works against the dark is
// done: a storyline 10, any other quest 2.
func tideOnQuest(w *World, q *Quest) {
	if w.Campaign.Living == nil || q == nil {
		return
	}
	if q.IsMain {
		MoveTide(w, -10)
		if w.Campaign.Living.Bands == nil {
			w.Campaign.Living.Bands = map[string]string{}
		}
		band := TideBand(w.Campaign.Living.Tide)
		w.Campaign.Living.Bands[q.ID] = band
		w.Record(ChronicleEntry{Kind: FactNews, Source: SourceEngine, QuestID: q.ID,
			Text: fmt.Sprintf("The storyline %q ended with the Tide at %s.", q.Title, band)})
		return
	}
	MoveTide(w, -2)
}
