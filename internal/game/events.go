package game

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Solvely-Colin/aidnd/internal/dice"
)

// World events: every so often the living world builds up to something
// big, and every character in it is called to one shared scene with a real
// deadline. Nothing here is scripted ahead. An event is born only where
// what the players have done meets what the world has been doing: a
// faction whose clock has been moving, and characters who are tangled up
// with it through the debts, grudges, choices, goals and marks the engine
// has recorded for them. Those threads decide how the event plays out:
// people who owe a character turn up to help, a debt a character owes
// comes due, a grudge sends one more enemy, and a faction nobody checked
// comes in stronger. The outcome changes the world for good.
//
// The engine owns every part of it. The narrator only describes the scene
// it is given.

// Event states.
const (
	EventGathering = "gathering" // called; characters are making their way to the site
	EventEngaged   = "engaged"   // the confrontation is being fought
	EventWon       = "won"
	EventLost      = "lost"
)

// Thread kinds: why a character is part of an event.
const (
	ThreadAlly     = "ally"     // someone who owes them turns up to help
	ThreadDebt     = "debt"     // someone they owe calls it in: they are at the site and in danger
	ThreadGrudge   = "grudge"   // someone with a grudge tips off the other side
	ThreadGoal     = "goal"     // something they want is at stake
	ThreadCrossed  = "crossed"  // choices and deeds that crossed the faction's people
	ThreadBrand    = "brand"    // a mark that ties them to the faction
	ThreadPressure = "pressure" // the faction's own clock: what nobody stopped
	ThreadAgenda   = "agenda"   // a named person's own plan that fed the faction
)

// Tuning.
const (
	// EventMuster is how long characters have to reach the site (Colin's
	// 48h muster), in seconds.
	EventMuster = 48 * 3600
	// EventFirstWeek is the first world week an event can be called.
	EventFirstWeek = 3
	// EventCooldown is the weeks between one event ending and the next
	// being called.
	EventCooldown = 4
	// EventThreshold is the thread weight the characters must have
	// gathered with a faction before it can come to a head.
	EventThreshold = 4
	// EventXP is what standing in a won event is worth.
	EventXP = 50
	// MaxPastEvents is how many settled events the world remembers.
	MaxPastEvents = 12
)

// EventThread is one reason the event is happening, or one way it will
// play out.
type EventThread struct {
	Kind   string `json:"kind"`
	CharID string `json:"char_id,omitempty"`
	NPCID  string `json:"npc_id,omitempty"`
	Text   string `json:"text"`
	Weight int    `json:"weight,omitempty"`
}

// WorldEvent is a convergence the engine found and called.
type WorldEvent struct {
	ID      string `json:"id"`
	Faction string `json:"faction"`
	Title   string `json:"title"`
	// Call is what everyone hears: why to come and where.
	Call     string `json:"call"`
	Region   string `json:"region,omitempty"`
	SiteID   string `json:"site_id"`
	Week     int    `json:"week"`
	Opened   int64  `json:"opened"`
	Deadline int64  `json:"deadline"`
	Status   string `json:"status"`
	// Threads are the reasons, all from engine state at the time it was
	// called.
	Threads []EventThread `json:"threads"`
	// Summoned are the characters called (everyone alive then).
	Summoned []string `json:"summoned"`
	// Answered are those who came to the site.
	Answered []string `json:"answered,omitempty"`
	// EncounterID is the confrontation's fight, once begun.
	EncounterID string `json:"encounter_id,omitempty"`
	// Allies are the NPCs who came to help (or to be saved), once begun.
	Allies []string `json:"allies,omitempty"`
	// Stood are the characters in the confrontation when it ended.
	Stood   []string `json:"stood,omitempty"`
	Outcome string   `json:"outcome,omitempty"`
	// Settled is the world week it ended in.
	Settled int `json:"settled,omitempty"`
}

// Open reports whether the event is still to be decided.
func (e *WorldEvent) Open() bool {
	return e != nil && (e.Status == EventGathering || e.Status == EventEngaged)
}

// CurrentEvent is the world's open event, or nil.
func CurrentEvent(w *World) *WorldEvent {
	lv := w.Campaign.Living
	if lv == nil || !lv.Event.Open() {
		return nil
	}
	return lv.Event
}

// eventHolds reports whether an open event is deciding a faction's fate:
// its clock can't fill on its own meanwhile.
func eventHolds(w *World, key string) bool {
	e := CurrentEvent(w)
	return e != nil && e.Faction == key
}

// linkedNPCs are the people tied to a faction: everyone alive in its
// region, and anyone whose own plan pushes its clock.
func linkedNPCs(w *World, f *Faction) map[string]*NPC {
	out := map[string]*NPC{}
	for _, n := range w.NPCs {
		if !n.Alive {
			continue
		}
		if l := w.Location(n.LocationID); l != nil && f.Region != "" && l.Region == f.Region {
			out[n.ID] = n
		}
	}
	for _, a := range w.Campaign.Living.Agendas {
		if pushes(a, f.Key) {
			if n := w.NPC(a.NPCID); n != nil && n.Alive {
				out[n.ID] = n
			}
		}
	}
	return out
}

// pushes reports whether an agenda ever pushes a faction's clock.
func pushes(a *Agenda, key string) bool {
	for _, st := range a.Steps {
		for _, e := range st.Effects {
			if e.Clock == key {
				return true
			}
		}
	}
	return false
}

// mentions reports whether text names the faction, its clock, or one of
// its people.
func mentions(text string, f *Faction, people map[string]*NPC) bool {
	if NameContains(text, f.Name) || (f.Clock != "" && NameContains(text, f.Clock)) {
		return true
	}
	for _, n := range people {
		if NameContains(text, n.Name) {
			return true
		}
	}
	return false
}

// CharacterThreads are the ways one character is tangled up with a
// faction, from what the engine has recorded about them.
func CharacterThreads(w *World, c *Character, f *Faction) []EventThread {
	if c == nil || c.Dead {
		return nil
	}
	people := linkedNPCs(w, f)
	var out []EventThread
	ids := make([]string, 0, len(people))
	for id := range people {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		n := people[id]
		for _, t := range n.TiesTo(c.ID) {
			why := ""
			if t.Text != "" {
				why = " (" + t.Text + ")"
			}
			switch t.Kind {
			case TieDebt:
				out = append(out, EventThread{Kind: ThreadAlly, CharID: c.ID, NPCID: n.ID, Weight: 2,
					Text: fmt.Sprintf("%s owes %s%s, and will be there to pay it back.", n.Name, c.Name, why)})
			case TieOwed:
				out = append(out, EventThread{Kind: ThreadDebt, CharID: c.ID, NPCID: n.ID, Weight: 2,
					Text: fmt.Sprintf("%s owes %s%s. The debt comes due: %s is caught up in it and needs getting out.", c.Name, n.Name, why, n.Name)})
			case TieGrudge:
				out = append(out, EventThread{Kind: ThreadGrudge, CharID: c.ID, NPCID: n.ID, Weight: 2,
					Text: fmt.Sprintf("%s holds a grudge against %s%s and has told the %s they are coming.", n.Name, c.Name, why, f.Name)})
			}
		}
	}
	crossed := 0
	for i := len(c.Record) - 1; i >= 0 && crossed < 2; i-- {
		e := c.Record[i]
		if e.Kind == RecOutcome && e.GoalID != "" {
			continue
		}
		if _, ok := people[e.NPCID]; ok || mentions(e.Text, f, nil) {
			out = append(out, EventThread{Kind: ThreadCrossed, CharID: c.ID, NPCID: e.NPCID, Weight: 1,
				Text: fmt.Sprintf("%s: %s", c.Name, e.Text)})
			crossed++
		}
	}
	for _, g := range c.OpenGoals() {
		if mentions(g.Text, f, people) {
			out = append(out, EventThread{Kind: ThreadGoal, CharID: c.ID, Weight: 2,
				Text: fmt.Sprintf("%s wants to %s. This is the chance.", c.Name, lowerFirst(strings.TrimSuffix(g.Text, ".")))})
		}
	}
	for _, m := range c.Marks {
		if m.Kind == MarkBrand && mentions(m.Text, f, nil) {
			out = append(out, EventThread{Kind: ThreadBrand, CharID: c.ID, Weight: 2,
				Text: fmt.Sprintf("%s carries a mark of the %s: %s.", c.Name, f.Name, strings.TrimSuffix(m.Text, "."))})
		}
	}
	return out
}

// worldThreads are the faction's own side: how far its clock has run and
// the plans of named people that fed it.
func worldThreads(w *World, f *Faction) []EventThread {
	out := []EventThread{{Kind: ThreadPressure, Weight: f.Progress,
		Text: fmt.Sprintf("Nobody has stopped the %s: %s has run %d of %d steps, and they come in strong.", f.Name, f.Clock, f.Progress, f.Steps)}}
	for _, a := range w.Campaign.Living.Agendas {
		if !pushes(a, f.Key) || a.Progress == 0 {
			continue
		}
		n := w.NPC(a.NPCID)
		if n == nil {
			continue
		}
		text := fmt.Sprintf("%s has been working toward this: %s.", n.Name, lowerFirst(a.Goal))
		if a.Done {
			text = fmt.Sprintf("%s's plan came together: %s.", n.Name, lowerFirst(a.Goal))
		}
		out = append(out, EventThread{Kind: ThreadAgenda, NPCID: n.ID, Text: text})
	}
	return out
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	if len(s) > 1 && s[1] >= 'a' && s[1] <= 'z' && s[0] >= 'A' && s[0] <= 'Z' {
		return strings.ToLower(s[:1]) + s[1:]
	}
	return s
}

// convergence scores how close a faction is to coming to a head because
// of what the characters did. It returns 0 when it can't be called.
func convergence(w *World, f *Faction, living []*Character) (int, []EventThread) {
	if !f.active(w) || f.Foe == "" || f.Steps < 2 {
		return 0, nil
	}
	var threads []EventThread
	weight, involved := 0, 0
	for _, c := range living {
		ts := CharacterThreads(w, c, f)
		if len(ts) == 0 {
			continue
		}
		involved++
		for _, t := range ts {
			weight += t.Weight
		}
		threads = append(threads, ts...)
	}
	need := min(2, len(living))
	near := f.Progress+1 >= f.Steps // about to fill: anyone tangled up is enough
	switch {
	case involved == 0:
		return 0, nil
	case near:
	case involved < need || weight < EventThreshold || f.Progress*2 < f.Steps:
		return 0, nil
	}
	return weight + f.Progress, threads
}

// eventSite is where a faction's event happens: the first place its full
// clock would darken, else a built place in its region away from the hub.
func eventSite(w *World, f *Faction) *Location {
	// a place nobody has been to yet is fine: answering the call fills it in
	ok := func(l *Location) bool { return l != nil && !l.IsLair && !l.IsHub }
	ids := make([]string, 0, len(f.FullDanger))
	for id := range f.FullDanger {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if l := w.Location(id); ok(l) {
			return l
		}
	}
	for _, l := range w.Locations {
		if ok(l) && f.Region != "" && l.Region == f.Region {
			return l
		}
	}
	return nil
}

// MaybeCallEvent looks for a convergence after a world tick and calls the
// strongest one. It returns the new event, or nil. The world must be
// settled.
func MaybeCallEvent(w *World, now int64) *WorldEvent {
	lv := w.Campaign.Living
	if lv == nil || CurrentEvent(w) != nil || lv.Week < EventFirstWeek {
		return nil
	}
	if lv.LastEventWeek > 0 && lv.Week-lv.LastEventWeek < EventCooldown {
		return nil
	}
	var living []*Character
	for _, c := range w.Characters {
		if !c.Dead {
			living = append(living, c)
		}
	}
	if len(living) == 0 {
		return nil
	}
	var best *Faction
	var bestThreads []EventThread
	bestScore := 0
	for _, f := range lv.Factions {
		if score, ts := convergence(w, f, living); score > bestScore {
			best, bestThreads, bestScore = f, ts, score
		}
	}
	if best == nil {
		return nil
	}
	site := eventSite(w, best)
	if site == nil {
		return nil
	}
	e := &WorldEvent{ID: w.NewID("wev"), Faction: best.Key, Region: best.Region, SiteID: site.ID, Week: lv.Week,
		Opened: now, Deadline: now + EventMuster, Status: EventGathering}
	e.Title = best.Clock
	if e.Title == "" {
		e.Title = "The " + best.Name + " moves"
	}
	e.Title += " at " + site.Name
	e.Threads = append(bestThreads, worldThreads(w, best)...)
	for _, c := range living {
		e.Summoned = append(e.Summoned, c.ID)
	}
	var who []string
	seen := map[string]bool{}
	for _, t := range bestThreads {
		if c := w.Character(t.CharID); c != nil && !seen[c.ID] {
			seen[c.ID] = true
			who = append(who, c.Name)
		}
	}
	call := best.Call
	if call == "" {
		call = fmt.Sprintf("The %s is making its move at %s.", best.Name, site.Name)
	}
	e.Call = fmt.Sprintf("%s What %s set in motion has come to a head. Everyone is needed at %s within two days.", strings.TrimSpace(call), joinNames(who), site.Name)
	lv.Event = e
	text := fmt.Sprintf("%s: the %s comes to a head at %s.", e.Title, best.Name, site.Name)
	w.Record(ChronicleEntry{Kind: FactNews, Source: SourceEngine, LocationID: site.ID, Text: fmt.Sprintf("Week %d: %s", lv.Week, text)})
	logBeat(w, WorldBeat{Week: lv.Week, Kind: BeatEvent, Text: text, LocationID: site.ID})
	return e
}

func joinNames(names []string) string {
	switch len(names) {
	case 0:
		return "you"
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// Called reports whether a character was called to the event.
func (e *WorldEvent) Called(charID string) bool { return oneOf(charID, e.Summoned) }

// Came reports whether a character answered the call.
func (e *WorldEvent) Came(charID string) bool { return oneOf(charID, e.Answered) }

// atSite lists the groups standing at the event's site.
func atSite(w *World, e *WorldEvent) []GroupInfo {
	var out []GroupInfo
	for _, gi := range w.GroupInfos() {
		if gi.LocationID == e.SiteID && len(livingIn(gi.Members)) > 0 {
			out = append(out, gi)
		}
	}
	return out
}

func livingIn(cs []*Character) []*Character {
	var out []*Character
	for _, c := range cs {
		if !c.Dead {
			out = append(out, c)
		}
	}
	return out
}

// noteAnswered adds the living members of groups at the site to Answered.
func noteAnswered(w *World, e *WorldEvent) {
	for _, gi := range atSite(w, e) {
		for _, c := range livingIn(gi.Members) {
			if !e.Came(c.ID) {
				e.Answered = append(e.Answered, c.ID)
			}
		}
	}
}

// GroupJoin is one joining-up AnswerCall made: group From folded into
// Into, leaving everyone in Group.
type GroupJoin struct {
	From, Into, Group string
	Joining, With     []string // names
}

// AnswerCall brings c's whole group to the event's site and joins it up
// with anyone already there, so everyone plays one shared scene. It
// returns the group the character is in afterwards, whether it moved, and
// the joins it made. The world must be settled.
func AnswerCall(w *World, c *Character) (string, bool, []GroupJoin, error) {
	if w.focus != nil {
		return "", false, nil, fmt.Errorf("settle the world before answering")
	}
	e := CurrentEvent(w)
	if e == nil {
		return "", false, nil, fmt.Errorf("nothing is calling right now")
	}
	if c == nil || c.Dead {
		return "", false, nil, fmt.Errorf("the dead don't answer")
	}
	site := w.Location(e.SiteID)
	if site == nil {
		return "", false, nil, fmt.Errorf("the place is gone")
	}
	gid := GroupOf(c)
	st := w.groupState(gid)
	if st == nil {
		return "", false, nil, fmt.Errorf("no group")
	}
	moved := st.LocationID != site.ID
	if moved {
		if fighting(st) {
			return "", false, nil, fmt.Errorf("finish the fight you are in first")
		}
		if prev := w.Location(st.LocationID); prev != nil {
			prev.LastDay = st.Scene.Day
		}
		if site.Stub {
			_, _ = FillLocation(w, site.ID, LocationFill{Description: firstNonEmpty(site.Description, site.Hint)})
		}
		EnsureZones(w, site.ID)
		st.LocationID = site.ID
		st.Encounter = nil
		st.Scene.Goal, st.Scene.Quiet = "", 0
		site.Visited = true
		site.LastDay = st.Scene.Day
		for _, m := range w.Members(gid) {
			m.ZoneID = site.Zones[0].ID
		}
		w.setGroupState(gid, st)
	}
	// everyone there becomes one group; a fight already going is joined
	joined := false
	var joins []GroupJoin
	for range 8 {
		var gi *GroupInfo
		for _, x := range atSite(w, e) {
			if x.ID != gid {
				gi = &x
				break
			}
		}
		if gi == nil {
			break
		}
		j := GroupJoin{From: gid, Into: gi.ID, Joining: names(w.Members(gid)), With: names(gi.Members)}
		if !gi.Fighting {
			j = GroupJoin{From: gi.ID, Into: gid, Joining: names(gi.Members), With: names(w.Members(gid))}
		}
		if gi.Fighting {
			if err := JoinGroups(w, gid, gi.ID); err != nil {
				return "", false, nil, err
			}
		} else if err := JoinGroups(w, gi.ID, gid); err != nil {
			return "", false, nil, err
		}
		gid, joined = GroupOf(c), true
		j.Group = gid
		joins = append(joins, j)
	}
	if !moved && !joined {
		return "", false, nil, fmt.Errorf("you are already at %s", site.Name)
	}
	noteAnswered(w, e)
	return GroupOf(c), moved, joins, nil
}

func names(cs []*Character) []string {
	var out []string
	for _, c := range cs {
		if !c.Dead {
			out = append(out, c.Name)
		}
	}
	return out
}

// EveryoneThere reports whether every living summoned character stands at
// the site.
func EveryoneThere(w *World, e *WorldEvent) bool {
	here := map[string]bool{}
	for _, gi := range atSite(w, e) {
		for _, c := range gi.Members {
			here[c.ID] = true
		}
	}
	for _, id := range e.Summoned {
		if c := w.Character(id); c != nil && !c.Dead && !here[id] {
			return false
		}
	}
	return true
}

// SiteGroup is the group standing at the event's site ("" when none). If
// several stand there, the first fighting one, else the first.
func SiteGroup(w *World, e *WorldEvent) string {
	gs := atSite(w, e)
	for _, gi := range gs {
		if gi.Fighting {
			return gi.ID
		}
	}
	if len(gs) > 0 {
		return gs[0].ID
	}
	return ""
}

// BeginEvent starts the confrontation for the focused group, which must
// stand at the site: allies arrive, debts are called in, and the
// faction's people attack, as many as the threads say. It returns what
// changed.
func BeginEvent(w *World, r *dice.Roller, now int64) ([]string, error) {
	e := CurrentEvent(w)
	if e == nil || e.Status != EventGathering {
		return nil, fmt.Errorf("there is no event waiting to begin")
	}
	l := w.Here()
	if l == nil || l.ID != e.SiteID {
		return nil, fmt.Errorf("you must be at %s", siteName(w, e))
	}
	if w.InCombat() {
		return nil, fmt.Errorf("finish the fight you are in first")
	}
	f := w.Campaign.Living.Faction(e.Faction)
	if f == nil {
		return nil, fmt.Errorf("no such faction")
	}
	noteAnswered(w, e)
	present := map[string]bool{}
	for _, c := range w.Characters {
		if !c.Dead {
			present[c.ID] = true
		}
	}
	var changes []string
	changes = append(changes, fmt.Sprintf("%s begins.", e.Title))
	count := 2 + f.Progress/2
	for _, t := range e.Threads {
		n := w.NPC(t.NPCID)
		switch t.Kind {
		case ThreadAlly, ThreadDebt:
			if n == nil || !n.Alive || n.Antagonist {
				continue
			}
			if n.LocationID != l.ID && occupied(w, n.LocationID) {
				continue // they are in someone else's scene and can't come
			}
			n.LocationID, n.ZoneID = l.ID, ""
			if !oneOf(n.ID, e.Allies) {
				e.Allies = append(e.Allies, n.ID)
			}
			if t.Kind == ThreadDebt {
				changes = append(changes, fmt.Sprintf("%s is here, caught in the middle.", n.Name))
				continue
			}
			count--
			if c := w.Character(t.CharID); c != nil && present[c.ID] && !c.Down() {
				temp := 4 + 2*c.Level
				if temp > c.TempHP {
					c.TempHP = temp
				}
				changes = append(changes, fmt.Sprintf("%s comes to stand with %s (%d temporary HP), and one fewer of the enemy reaches them.", n.Name, c.Name, temp))
			} else {
				changes = append(changes, fmt.Sprintf("%s comes to help, and one fewer of the enemy gets through.", n.Name))
			}
		case ThreadGrudge:
			if n != nil && n.Alive {
				count++
				changes = append(changes, fmt.Sprintf("%s tipped them off: one more of them is waiting.", n.Name))
			}
		}
	}
	count = clamp(count, 2, 6)
	// the fight is harder than the place alone would make it
	saved := l.Danger
	l.Danger = clamp(max(1, l.Danger)+1, 2, 5)
	defer func() { l.Danger = saved }()
	base := GenericEnemies(l.Kind, l.Danger, len(w.Characters))[0]
	foe := base
	foe.Name, foe.Inflicts, foe.Horror = f.Foe, "", f.FoeHorror
	var specs []EnemySpec
	champion := f.Champion != "" && f.Progress+2 >= f.Steps
	if champion {
		count--
	}
	for count > 0 {
		n := min(4, count)
		s := foe
		s.Count = n
		specs = append(specs, s)
		count -= n
	}
	if champion {
		ch := base
		ch.Name, ch.Count, ch.Tag, ch.Horror = f.Champion, 1, "", f.ChampionHorror
		ch.HP *= 2
		ch.AttackBonus++
		specs = append(specs, ch)
	}
	enc, _, err := StartEncounter(w, specs)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, en := range enc.Enemies {
		names = append(names, en.Display())
	}
	changes = append(changes, fmt.Sprintf("The %s attack: %s.", f.Name, strings.Join(names, ", ")))
	e.Status = EventEngaged
	e.EncounterID = enc.ID
	w.Campaign.Turns.Started = now
	return changes, nil
}

func siteName(w *World, e *WorldEvent) string {
	if l := w.Location(e.SiteID); l != nil {
		return l.Name
	}
	return "the site"
}

// EventResult is how an event ended, for events and the feed.
type EventResult struct {
	EventID string   `json:"event_id"`
	Title   string   `json:"title"`
	Won     bool     `json:"won"`
	Outcome string   `json:"outcome"`
	Changes []string `json:"changes"`
}

// eventFightWon settles the event when its confrontation is won. The
// world is focused on the group that won it.
func eventFightWon(w *World, encID string) *EventResult {
	e := CurrentEvent(w)
	if e == nil || e.Status != EventEngaged || e.EncounterID != encID {
		return nil
	}
	for _, c := range w.Characters {
		if !c.Dead && !oneOf(c.ID, e.Stood) {
			e.Stood = append(e.Stood, c.ID)
		}
	}
	return settleEvent(w, e, true)
}

// EventFightLost settles an engaged event whose fight ended without being
// won (the party fled or fell). It returns nil when there is nothing to
// settle.
func EventFightLost(w *World) *EventResult {
	e := CurrentEvent(w)
	if e == nil || e.Status != EventEngaged {
		return nil
	}
	for _, id := range w.GroupIDs() {
		if st := w.peekGroup(id); st != nil && st.Encounter != nil && st.Encounter.ID == e.EncounterID && len(st.Encounter.Living()) > 0 {
			return nil // still being fought
		}
	}
	return settleEvent(w, e, false)
}

// EventDeadline handles an event whose muster ran out: the fight begins
// for whoever is at the site (the caller focuses that group and calls
// BeginEvent), or, with nobody there, the faction gets its way. It
// reports whether the caller should begin the fight for SiteGroup.
func EventDeadline(w *World, now int64) (begin bool, res *EventResult) {
	e := CurrentEvent(w)
	if e == nil || e.Status != EventGathering || now < e.Deadline {
		return false, nil
	}
	if g := SiteGroup(w, e); g != "" {
		for _, gi := range atSite(w, e) {
			if gi.ID == g && gi.Fighting {
				return false, nil // another fight first; the event waits for it
			}
		}
		return true, nil
	}
	e.Outcome = "Nobody came."
	return false, settleEvent(w, e, false)
}

// settleEvent makes the outcome permanent.
func settleEvent(w *World, e *WorldEvent, won bool) *EventResult {
	lv := w.Campaign.Living
	f := lv.Faction(e.Faction)
	site := w.Location(e.SiteID)
	siteN := siteName(w, e)
	res := &EventResult{EventID: e.ID, Title: e.Title, Won: won}
	var names []string
	for _, id := range e.Stood {
		if c := w.Character(id); c != nil {
			names = append(names, c.Name)
		}
	}
	fname := e.Faction
	if f != nil {
		fname = f.Name
	}
	if won {
		e.Status = EventWon
		if f != nil {
			f.Broken = true
		}
		down := -2
		if f != nil && f.Threat {
			down = -5
		}
		by := MoveTide(w, down)
		if site != nil && site.Danger > 1 {
			site.Danger--
		}
		e.Outcome = fmt.Sprintf("%s broke the %s at %s. Their plan is finished for good.", joinNames(names), fname, siteN)
		res.Changes = append(res.Changes, e.Outcome)
		if by < 0 {
			res.Changes = append(res.Changes, fmt.Sprintf("The Tide falls %d.", -by))
		}
		w.Record(ChronicleEntry{Kind: FactDeed, Source: SourceEngine, LocationID: e.SiteID, Text: e.Outcome})
		for _, id := range e.Stood {
			c := w.Character(id)
			if c == nil || c.Dead {
				continue
			}
			res.Changes = append(res.Changes, AwardXP(c, EventXP)...)
			Note(w, c, RecordEntry{Kind: RecDeed, Source: SourceEngine, Text: fmt.Sprintf("Stood against the %s at %s when %s came to a head, and won", fname, siteN, e.Title)})
		}
	} else {
		e.Status = EventLost
		if e.Outcome == "" {
			e.Outcome = fmt.Sprintf("The %s had its way at %s.", fname, siteN)
		} else {
			e.Outcome += fmt.Sprintf(" The %s had its way at %s.", fname, siteN)
		}
		res.Changes = append(res.Changes, e.Outcome)
		if f != nil {
			f.Progress, f.Filled = f.Steps, true
			if f.Full != "" {
				res.Changes = append(res.Changes, f.Full)
				w.Record(ChronicleEntry{Kind: FactNews, Source: SourceEngine, LocationID: e.SiteID, Text: fmt.Sprintf("Week %d: %s", lv.Week, f.Full)})
			}
			for id, d := range f.FullDanger {
				if l := w.Location(id); l != nil && l.Danger < d && !l.IsHub {
					l.Danger = d
				}
			}
			up := 2
			if f.Threat {
				up = 5
			}
			if by := MoveTide(w, up); by > 0 {
				res.Changes = append(res.Changes, fmt.Sprintf("The Tide rises %d.", by))
			}
		}
		w.Record(ChronicleEntry{Kind: FactNews, Source: SourceEngine, LocationID: e.SiteID, Text: fmt.Sprintf("Week %d: %s", lv.Week, e.Outcome)})
		for _, id := range e.Stood {
			if c := w.Character(id); c != nil && !c.Dead {
				Note(w, c, RecordEntry{Kind: RecOutcome, Source: SourceEngine, Text: fmt.Sprintf("Stood against the %s at %s when %s came to a head, and lost", fname, siteN, e.Title)})
			}
		}
	}
	// the threads come due
	for _, t := range e.Threads {
		n, c := w.NPC(t.NPCID), w.Character(t.CharID)
		if n == nil || c == nil {
			continue
		}
		came := e.Came(c.ID)
		switch t.Kind {
		case ThreadAlly:
			if won && n.Alive && n.Settle(c.ID, TieDebt) {
				res.Changes = append(res.Changes, fmt.Sprintf("%s has paid back what they owed %s.", n.Name, c.Name))
				Note(w, c, RecordEntry{Kind: RecOutcome, Source: SourceEngine, NPCID: n.ID, Text: fmt.Sprintf("%s paid back their debt by standing with them at %s", n.Name, siteN)})
			}
		case ThreadDebt:
			switch {
			case won && came && n.Alive && n.Settle(c.ID, TieOwed):
				res.Changes = append(res.Changes, fmt.Sprintf("%s's debt to %s is paid: they got them out.", c.Name, n.Name))
				Note(w, c, RecordEntry{Kind: RecOutcome, Source: SourceEngine, NPCID: n.ID, Text: fmt.Sprintf("Paid back %s by getting them out at %s", n.Name, siteN)})
			case !came && n.Alive:
				n.Bind(w, c, TieGrudge, "left them to it at "+siteN, SourceEngine)
				n.Keep(fmt.Sprintf("%s did not come when it mattered, at %s.", c.Name, siteN))
				res.Changes = append(res.Changes, fmt.Sprintf("%s won't forget that %s didn't come.", n.Name, c.Name))
			}
		}
	}
	for _, id := range e.Summoned {
		c := w.Character(id)
		if c == nil || c.Dead || e.Came(id) {
			continue
		}
		Note(w, c, RecordEntry{Kind: RecChoice, Source: SourceEngine, Text: fmt.Sprintf("Was elsewhere when %s came to a head", e.Title)})
	}
	e.Settled = lv.Week
	res.Outcome = e.Outcome
	logBeat(w, WorldBeat{Week: lv.Week, Kind: BeatEvent, Text: e.Outcome, LocationID: e.SiteID})
	lv.LastEventWeek = lv.Week
	lv.Past = append(lv.Past, *e)
	if len(lv.Past) > MaxPastEvents {
		lv.Past = lv.Past[len(lv.Past)-MaxPastEvents:]
	}
	lv.Event = nil
	return res
}
