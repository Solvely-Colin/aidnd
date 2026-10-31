// Package saga holds hand-written campaigns (the campaign bible as data)
// and builds an open world from one, region by region as play reaches it. A saga is the top canon layer: the engine,
// the chronicle and the narrator build on it and never contradict it.
//
// Secrets in a saga are gated: each lore entry names the objective or quest
// that must be done before the narrator is told it, so it can't spoil the
// story by accident.
package saga

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Solvely-Colin/aidnd/internal/game"
)

// Saga is a long campaign in one open world. There are no acts: the
// world is made of regions, each with its own storyline, and a region is
// built out the first time anyone travels there.
type Saga struct {
	Key     string
	Title   string
	Pitch   string // player-facing
	Premise string
	Setting string // era key
	Start   string // key of the region play begins in
	Regions []Region
}

// StartTide is where the Tide stands when a saga begins.
const StartTide = 5

// Region is a part of the world: a safe hub, the places around it, its
// people, and a storyline that ends with a boss. Regions can be played in
// any order; Levels is only the range it is pitched at.
type Region struct {
	Key    string
	Name   string
	Brief  string // player-safe canon the narrator sees anywhere in the region
	From   string // region key whose hub the road leaves from ("" for the start)
	Road   string // how the road looks from there
	Levels [2]int
	Hub    Place
	Places []Place
	People []Person
	Things []Thing
	Main   Quest // the last objective (defeat the boss) is added by the build
	Side   []Quest
	// Secrets are gated: see Secret.
	Secrets []Secret
	Beats   []Beat
	Boss    Boss
	// Factions move on world ticks once the region is built.
	Factions []Faction
	// Agendas are its people's own goals, also moving on world ticks.
	Agendas []Agenda
}

// Agenda is a person's goal (see game.Agenda), by names: Who is a Person
// of the region.
type Agenda struct {
	Key, Who, Goal string
	FromWeek       int
	Steps          []AgendaStep
}

// AgendaStep is one step of an agenda. MoveTo is a place of the region;
// Disposition shifts On (a person's name, "" for Who); Clock pushes a
// faction (by key) one step. The If fields hold the step back until Who
// feels that way or worse, or that faction's clock has moved that far.
type AgendaStep struct {
	Text                string
	MoveTo              string
	Disposition         int
	On                  string
	Clock               string
	IfDispositionAtMost *int
	IfClock             string
	IfClockAtLeast      int
}

// atMost is a pointer for AgendaStep.IfDispositionAtMost.
func atMost(d int) *int { return &d }

// Faction is a group with an agenda clock (see game.Faction). FullDanger
// names places by name; FromWeek delays its first move.
type Faction struct {
	Key, Name, Clock string
	Threat           bool
	Steps            int
	FromWeek         int
	Moves            []string
	Full             string
	FullDanger       map[string]int
}

// Place is a location. From names the place it connects to (the region's
// hub if empty). A lair connects through a gate its storyline opens.
type Place struct {
	Name, Kind, Hint, Desc string
	Danger                 int
	From                   string
	Lair                   bool
}

// Person is a named NPC. At names their place; "" keeps them offstage until
// a beat brings them in.
type Person struct {
	Name, Role, Desc, At string
	Disposition          int
	Merchant             bool
	Stock                []string // catalog keys
	Protected            bool
	Keeps                []string // memories never forgotten
	Stats                *game.StatBlock
}

// Thing is an item placed at a place or held by a person.
type Thing struct {
	Name, Kind, Desc string
	Value            int
	At               string // place name, or person name if Held
	Held             bool
}

// Quest is a storyline or a side quest. Milestones maps an objective
// number (1-based) to the level every character reaches when it is done.
type Quest struct {
	Title, Summary, Giver string
	Solo                  bool // sized for one character
	Objectives            []game.ObjectiveSpec
	Gold, XP              int
	Rewards               []game.RewardItemSpec
	Milestones            map[int]int
}

// Secret is canon the narrator sees. Gate is "" (always open), "main:N"
// (after the region storyline's objective N) or "side:<title>" (after that
// side quest).
type Secret struct {
	Title, Text string
	Keywords    []string
	AlwaysOn    bool
	Gate        string
}

// Beat is a scripted story event. Target is a place name (enter_location)
// or "main:N" (objective_done). Figure names a Person the beat brings in.
type Beat struct {
	Title, Text, Trigger, Target string
	Value                        int
	Effects                      []string
	Figure                       string
}

// Boss is a storyline's antagonist, who waits in the region's lair.
type Boss struct {
	Person
	Move         string
	Phases       []game.BossPhase
	Minion       string
	MinionHorror int
}

var registry = map[string]*Saga{}

func register(s *Saga) { registry[s.Key] = s }

// Get returns a saga by key.
func Get(key string) *Saga { return registry[key] }

// Region returns one of the saga's regions.
func (s *Saga) Region(key string) *Region {
	for i := range s.Regions {
		if s.Regions[i].Key == key {
			return &s.Regions[i]
		}
	}
	return nil
}

// Info is a saga's player-facing card.
type Info struct {
	Key     string `json:"key"`
	Title   string `json:"title"`
	Pitch   string `json:"pitch"`
	Setting string `json:"setting"`
	Start   string `json:"start"`   // the region play begins in
	Regions int    `json:"regions"` // regions written so far
}

// List returns every saga's card, by title.
func List() []Info {
	var out []Info
	for _, s := range registry {
		start := ""
		if r := s.Region(s.Start); r != nil {
			start = r.Name
		}
		out = append(out, Info{Key: s.Key, Title: s.Title, Pitch: s.Pitch, Setting: s.Setting, Start: start, Regions: len(s.Regions)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out
}

// Options are the campaign settings Build doesn't take from the saga.
type Options struct {
	MaxSetbacks, MaxPlayers int
	Now                     int64
}

// Build makes a new world for the saga: the start region in full, and a
// road to every other region's hub, left as a stub until someone goes
// there (see Arrive). Every name must resolve; a problem is an error,
// never a silently dropped piece.
func Build(s *Saga, id string, opt Options) (*game.World, error) {
	start := s.Region(s.Start)
	if start == nil {
		return nil, fmt.Errorf("saga %s has no start region %q", s.Key, s.Start)
	}
	w := &game.World{Campaign: game.Campaign{ID: id, Title: s.Title, Premise: s.Premise, Pitch: s.Pitch,
		Setting: s.Setting, Status: game.StatusLobby, MaxSetbacks: opt.MaxSetbacks, MaxPlayers: opt.MaxPlayers, CreatedAt: opt.Now}}
	w.Campaign.Saga, w.Campaign.Dread, w.Campaign.Async = s.Key, true, true
	w.Campaign.Living = &game.Living{Tide: StartTide}
	w.Campaign.Scene = game.Scene{Day: 1, Time: 1, Tension: 2}
	w.Campaign.Turns.Budget = game.FullBudget(false)
	w.Campaign.Milestones = map[string]int{}

	// every region's hub first, so roads can name any of them
	hubs := map[string]*game.Location{}
	for i := range s.Regions {
		r := &s.Regions[i]
		if hubs[r.Key] != nil {
			return nil, fmt.Errorf("region %q twice", r.Key)
		}
		h := w.AddStub(r.Hub.Name, r.Hub.Hint, firstOf(r.Hub.Kind, game.KindTown), 0)
		h.IsHub, h.Region = true, r.Key
		hubs[r.Key] = h
	}
	for i := range s.Regions {
		r := &s.Regions[i]
		if r.Key == s.Start {
			continue
		}
		from := hubs[r.From]
		if from == nil {
			return nil, fmt.Errorf("region %q is reached from unknown %q", r.Key, r.From)
		}
		w.Connect(from.ID, hubs[r.Key].ID, game.EdgeKnown, firstOf(r.Road, "the road to "+r.Name), "the road back to "+hubs[r.From].Name)
	}
	hub := hubs[start.Key]
	w.Campaign.HubID, w.Campaign.LocationID = hub.ID, hub.ID
	hub.Visited = true
	boss, lair, err := buildRegion(w, start, hub)
	if err != nil {
		return nil, fmt.Errorf("region %s: %w", start.Key, err)
	}
	w.Campaign.LairID, w.Campaign.AntagonistID = lair.ID, boss.ID
	if !w.Connected() {
		return nil, fmt.Errorf("the saga's places do not form one map")
	}
	return w, nil
}

// Arrive builds out a region the first time the party reaches its hub.
// It reports whether it built anything.
func Arrive(w *game.World, l *game.Location) (bool, error) {
	s := Get(w.Campaign.Saga)
	if s == nil || l == nil || !l.IsHub || !l.Stub || l.Region == "" {
		return false, nil
	}
	r := s.Region(l.Region)
	if r == nil {
		return false, nil
	}
	if _, _, err := buildRegion(w, r, l); err != nil {
		return false, fmt.Errorf("region %s: %w", r.Key, err)
	}
	return true, nil
}

// buildRegion lays a region out around its hub and returns its boss and
// lair.
func buildRegion(w *game.World, r *Region, hub *game.Location) (*game.NPC, *game.Location, error) {
	hub.Stub, hub.Description = false, r.Hub.Desc
	// places first, then the roads between them, so From can name any place
	for _, p := range r.Places {
		if exact(w, p.Name) != nil {
			return nil, nil, fmt.Errorf("place %q twice", p.Name)
		}
		l := w.AddStub(p.Name, p.Hint, p.Kind, p.Danger)
		// a written description is canon: the author keeps it and only
		// adds what is around it (people, things, new ways onward)
		l.Description, l.IsLair, l.Region = p.Desc, p.Lair, r.Key
	}
	var lair *game.Location
	for _, p := range r.Places {
		l := exact(w, p.Name)
		from := hub
		if p.From != "" {
			if from = exact(w, p.From); from == nil {
				return nil, nil, fmt.Errorf("place %q connects to unknown %q", p.Name, p.From)
			}
		}
		if p.Lair {
			if lair != nil {
				return nil, nil, fmt.Errorf("two lairs")
			}
			lair = l
			gate := w.Connect(from.ID, l.ID, game.EdgeLocked, "the way down to "+l.Name, "back to "+from.Name)
			gate.LairGate = true
			continue
		}
		w.Connect(from.ID, l.ID, game.EdgeKnown, "the way to "+l.Name, "back to "+from.Name)
	}
	if lair == nil {
		return nil, nil, fmt.Errorf("no lair")
	}

	for _, p := range r.People {
		if _, err := addPerson(w, p); err != nil {
			return nil, nil, err
		}
	}
	boss, err := addPerson(w, r.Boss.Person)
	if err != nil {
		return nil, nil, err
	}
	if boss.LocationID != lair.ID {
		return nil, nil, fmt.Errorf("the boss must wait in the lair")
	}
	boss.Antagonist, boss.Disposition = true, -3
	if boss.Stats == nil {
		boss.Stats = &game.StatBlock{MaxHP: 45, AC: 14, AttackBonus: 5, Damage: "1d8+2"}
	}
	boss.Stats.AsBoss = &game.BossSpec{Move: r.Boss.Move, Phases: r.Boss.Phases, Minion: r.Boss.Minion, MinionHorror: r.Boss.MinionHorror}

	for _, t := range r.Things {
		owner := game.Owner{Kind: game.OwnerLocation}
		if t.Held {
			pn := exactNPC(w, t.At)
			if pn == nil {
				return nil, nil, fmt.Errorf("thing %q held by unknown %q", t.Name, t.At)
			}
			owner = game.Owner{Kind: game.OwnerNPC, ID: pn.ID}
		} else {
			l := exact(w, t.At)
			if l == nil {
				return nil, nil, fmt.Errorf("thing %q at unknown %q", t.Name, t.At)
			}
			owner.ID = l.ID
		}
		if game.PlaceItem(w, game.ItemSpec{Name: t.Name, Kind: t.Kind, Desc: t.Desc, Value: t.Value}, owner) == nil {
			return nil, nil, fmt.Errorf("thing %q could not be placed", t.Name)
		}
	}

	main := r.Main
	main.Objectives = append(append([]game.ObjectiveSpec{}, main.Objectives...),
		game.ObjectiveSpec{Kind: game.ObjDefeatEnemy, Text: "Defeat " + boss.Name, Target: boss.ID})
	mq, err := addQuest(w, main, true)
	if err != nil {
		return nil, nil, fmt.Errorf("storyline: %w", err)
	}
	quests := map[string]*game.Quest{}
	for _, q := range r.Side {
		sq, err := addQuest(w, q, false)
		if err != nil {
			return nil, nil, fmt.Errorf("side quest %q: %w", q.Title, err)
		}
		quests[q.Title] = sq
	}

	gate := func(ref string) (string, error) {
		switch {
		case ref == "":
			return "", nil
		case strings.HasPrefix(ref, "main:"):
			k, err := strconv.Atoi(strings.TrimPrefix(ref, "main:"))
			if err != nil || k < 1 || k > len(mq.Objectives) {
				return "", fmt.Errorf("bad gate %q", ref)
			}
			return mq.Objectives[k-1].ID, nil
		case strings.HasPrefix(ref, "side:"):
			q := quests[strings.TrimPrefix(ref, "side:")]
			if q == nil {
				return "", fmt.Errorf("bad gate %q", ref)
			}
			return q.ID, nil
		}
		return "", fmt.Errorf("bad gate %q", ref)
	}

	var regionIDs []string
	for _, l := range w.Locations {
		if l.Region == r.Key {
			regionIDs = append(regionIDs, l.ID)
		}
	}
	if r.Brief != "" {
		w.Lore = append(w.Lore, &game.Lore{ID: w.NewID("lore"), Title: r.Name, Text: r.Brief, EntityIDs: regionIDs, Keywords: []string{strings.ToLower(r.Name)}, LastTrigger: -100})
	}
	for _, sec := range r.Secrets {
		g, err := gate(sec.Gate)
		if err != nil {
			return nil, nil, fmt.Errorf("secret %q: %w", sec.Title, err)
		}
		lo := &game.Lore{ID: w.NewID("lore"), Title: sec.Title, Text: sec.Text, AlwaysOn: sec.AlwaysOn, LastTrigger: -100}
		lo.Gate = g
		for _, k := range sec.Keywords {
			lo.Keywords = append(lo.Keywords, strings.ToLower(k))
		}
		for _, np := range w.NPCs {
			if game.NameContains(sec.Text, np.Name) || game.NameContains(sec.Title, np.Name) {
				lo.EntityIDs = append(lo.EntityIDs, np.ID)
			}
		}
		for _, l := range w.Locations {
			if game.NameContains(sec.Text, l.Name) || game.NameContains(sec.Title, l.Name) {
				lo.EntityIDs = append(lo.EntityIDs, l.ID)
			}
		}
		w.Lore = append(w.Lore, lo)
	}

	if w.Campaign.Living != nil {
		for _, f := range r.Factions {
			gf, err := makeFaction(w, r, f, mq)
			if err != nil {
				return nil, nil, err
			}
			w.Campaign.Living.Factions = append(w.Campaign.Living.Factions, gf)
		}
		for _, a := range r.Agendas {
			ga, err := makeAgenda(w, r, a, mq)
			if err != nil {
				return nil, nil, err
			}
			w.Campaign.Living.Agendas = append(w.Campaign.Living.Agendas, ga)
		}
	}

	for _, b := range r.Beats {
		beat := &game.Beat{ID: w.NewID("beat"), Title: b.Title, Text: b.Text, Trigger: b.Trigger, Value: b.Value, Effects: b.Effects}
		switch b.Trigger {
		case game.TriggerEnterLocation:
			l := exact(w, b.Target)
			if l == nil {
				return nil, nil, fmt.Errorf("beat %q: unknown place %q", b.Title, b.Target)
			}
			beat.Target = l.ID
		case game.TriggerObjectiveDone:
			g, err := gate(b.Target)
			if err != nil || !strings.HasPrefix(b.Target, "main:") {
				return nil, nil, fmt.Errorf("beat %q: target must be main:N", b.Title)
			}
			beat.Target = g
		case game.TriggerStalled, game.TriggerTension, game.TriggerTurn:
			if b.Value <= 0 {
				return nil, nil, fmt.Errorf("beat %q needs a value", b.Title)
			}
		default:
			return nil, nil, fmt.Errorf("beat %q: unknown trigger %q", b.Title, b.Trigger)
		}
		if b.Figure != "" {
			f := exactNPC(w, b.Figure)
			if f == nil {
				return nil, nil, fmt.Errorf("beat %q: unknown figure %q", b.Title, b.Figure)
			}
			beat.EffectTarget = f.ID
		}
		w.Beats = append(w.Beats, beat)
	}
	// the confrontation always plays when the party reaches the lair
	w.Beats = append(w.Beats, &game.Beat{ID: w.NewID("beat"), Title: "The confrontation", Trigger: game.TriggerEnterLocation, Target: lair.ID,
		Text: boss.Name + " is here and confronts the party. Make the stakes plain.", Effects: []string{game.EffectAmbush}, EffectTarget: boss.ID})

	if w.Campaign.Milestones == nil {
		w.Campaign.Milestones = map[string]int{}
	}
	for k, lvl := range main.Milestones {
		if k < 1 || k > len(mq.Objectives) {
			return nil, nil, fmt.Errorf("milestone for missing objective %d", k)
		}
		w.Campaign.Milestones[mq.Objectives[k-1].ID] = lvl
	}
	return boss, lair, nil
}

func addPerson(w *game.World, p Person) (*game.NPC, error) {
	if exactNPC(w, p.Name) != nil {
		return nil, fmt.Errorf("person %q twice", p.Name)
	}
	loc := ""
	if p.At != "" {
		l := exact(w, p.At)
		if l == nil {
			return nil, fmt.Errorf("person %q at unknown %q", p.Name, p.At)
		}
		loc = l.ID
	}
	n := game.AddNPC(w, loc, game.NPCSpec{Name: p.Name, Role: p.Role, Desc: p.Desc, Disposition: p.Disposition, Merchant: p.Merchant})
	n.Protected = p.Protected
	n.Keeps = append(n.Keeps, p.Keeps...)
	if p.Stats != nil {
		sb := *p.Stats
		n.Stats = &sb
	}
	for _, k := range p.Stock {
		if _, ok := game.LookupDef(k); !ok {
			return nil, fmt.Errorf("person %q stocks unknown %q", p.Name, k)
		}
		game.NewItem(w, k, game.Owner{Kind: game.OwnerNPC, ID: n.ID})
	}
	return n, nil
}

func addQuest(w *game.World, q Quest, main bool) (*game.Quest, error) {
	// copies: validation resolves names to ids in place, and the saga's
	// data is shared by every world built from it
	spec := game.QuestSpec{Title: q.Title, Summary: q.Summary, RewardGold: q.Gold, RewardXP: q.XP,
		Objectives:  append([]game.ObjectiveSpec{}, q.Objectives...),
		RewardItems: append([]game.RewardItemSpec{}, q.Rewards...)}
	if q.Giver != "" {
		g := exactNPC(w, q.Giver)
		if g == nil {
			return nil, fmt.Errorf("unknown giver %q", q.Giver)
		}
		spec.GiverID = g.ID
	}
	for _, o := range spec.Objectives {
		if err := checkTarget(w, o); err != nil {
			return nil, err
		}
	}
	return game.AddQuest(w, spec, main)
}

// checkTarget makes sure an objective names something that exists, so a
// typo in the bible fails a test instead of making a quest unwinnable.
func checkTarget(w *game.World, o game.ObjectiveSpec) error {
	switch o.Kind {
	case game.ObjReachLocation:
		if exact(w, o.Target) == nil {
			return fmt.Errorf("objective %q: unknown place %q", o.Text, o.Target)
		}
	case game.ObjTalkToNPC:
		if exactNPC(w, o.Target) == nil {
			return fmt.Errorf("objective %q: unknown person %q", o.Text, o.Target)
		}
	case game.ObjObtainItem, game.ObjDeliverItem:
		found := false
		for _, it := range w.Items {
			found = found || game.NameMatch(it.Name, o.Target)
		}
		if !found {
			return fmt.Errorf("objective %q: unknown item %q", o.Text, o.Target)
		}
		if o.Kind == game.ObjDeliverItem && exactNPC(w, o.DeliverTo) == nil {
			return fmt.Errorf("objective %q: unknown person %q", o.Text, o.DeliverTo)
		}
	case game.ObjDefeatEnemy, game.ObjFreeform:
	default:
		return fmt.Errorf("objective %q: unknown kind %q", o.Text, o.Kind)
	}
	return nil
}

// exact finds a location by its exact name (case-insensitive), so the
// lenient lookups can't quietly pick the wrong place.
func exact(w *game.World, name string) *game.Location {
	for _, l := range w.Locations {
		if strings.EqualFold(l.Name, name) {
			return l
		}
	}
	return nil
}

func exactNPC(w *game.World, name string) *game.NPC {
	for _, n := range w.NPCs {
		if strings.EqualFold(n.Name, name) {
			return n
		}
	}
	return nil
}

func firstOf(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}

// EnsureLiving gives a saga world made before the living world its clock
// and the factions of the regions already built. It reports whether it
// changed anything.
func EnsureLiving(w *game.World) bool {
	s := Get(w.Campaign.Saga)
	if s == nil || w.Campaign.Living != nil {
		return false
	}
	w.Campaign.Living = &game.Living{Tide: StartTide}
	for i := range s.Regions {
		r := &s.Regions[i]
		hub := exact(w, r.Hub.Name)
		if hub == nil || hub.Stub {
			continue
		}
		var mq *game.Quest
		for _, q := range w.Quests {
			if q.IsMain && q.Title == r.Main.Title {
				mq = q
			}
		}
		for _, f := range r.Factions {
			if gf, err := makeFaction(w, r, f, mq); err == nil {
				w.Campaign.Living.Factions = append(w.Campaign.Living.Factions, gf)
			}
		}
	}
	EnsureAgendas(w)
	return true
}

// EnsureAgendas gives a living world the agendas of the regions already
// built that it doesn't have yet (worlds made before agendas existed). It
// reports whether it changed anything.
func EnsureAgendas(w *game.World) bool {
	s := Get(w.Campaign.Saga)
	lv := w.Campaign.Living
	if s == nil || lv == nil {
		return false
	}
	have := map[string]bool{}
	for _, a := range lv.Agendas {
		have[a.Key] = true
	}
	changed := false
	for i := range s.Regions {
		r := &s.Regions[i]
		hub := exact(w, r.Hub.Name)
		if hub == nil || hub.Stub {
			continue
		}
		var mq *game.Quest
		for _, q := range w.Quests {
			if q.IsMain && q.Title == r.Main.Title {
				mq = q
			}
		}
		for _, a := range r.Agendas {
			if have[a.Key] {
				continue
			}
			if ga, err := makeAgenda(w, r, a, mq); err == nil {
				lv.Agendas = append(lv.Agendas, ga)
				changed = true
			}
		}
	}
	return changed
}

// makeAgenda places a region's agenda in the world.
func makeAgenda(w *game.World, r *Region, a Agenda, storyline *game.Quest) (*game.Agenda, error) {
	who := exactNPC(w, a.Who)
	if who == nil {
		return nil, fmt.Errorf("agenda %q: unknown person %q", a.Key, a.Who)
	}
	if len(a.Steps) == 0 {
		return nil, fmt.Errorf("agenda %q has no steps", a.Key)
	}
	ga := &game.Agenda{Key: a.Key, NPCID: who.ID, Goal: a.Goal, Region: r.Key, FromWeek: a.FromWeek}
	if storyline != nil {
		ga.Storyline = storyline.ID
	}
	for _, st := range a.Steps {
		gs := game.AgendaStep{Text: st.Text}
		if st.IfDispositionAtMost != nil || st.IfClock != "" {
			gs.If = &game.AgendaIf{DispositionAtMost: st.IfDispositionAtMost, Clock: st.IfClock, ClockAtLeast: st.IfClockAtLeast}
		}
		if st.MoveTo != "" {
			l := exact(w, st.MoveTo)
			if l == nil {
				return nil, fmt.Errorf("agenda %q: unknown place %q", a.Key, st.MoveTo)
			}
			gs.Effects = append(gs.Effects, game.AgendaEffect{MoveTo: l.ID})
		}
		if st.Disposition != 0 {
			e := game.AgendaEffect{Disposition: st.Disposition}
			if st.On != "" {
				on := exactNPC(w, st.On)
				if on == nil {
					return nil, fmt.Errorf("agenda %q: unknown person %q", a.Key, st.On)
				}
				e.NPCID = on.ID
			}
			gs.Effects = append(gs.Effects, e)
		}
		if st.Clock != "" {
			gs.Effects = append(gs.Effects, game.AgendaEffect{Clock: st.Clock})
		}
		ga.Steps = append(ga.Steps, gs)
	}
	return ga, nil
}

// makeFaction places a region's faction in the world.
func makeFaction(w *game.World, r *Region, f Faction, storyline *game.Quest) (*game.Faction, error) {
	gf := &game.Faction{Key: f.Key, Name: f.Name, Threat: f.Threat, Region: r.Key, Clock: f.Clock,
		Steps: f.Steps, FromWeek: f.FromWeek, Moves: f.Moves, Full: f.Full}
	if gf.Steps < 2 {
		return nil, fmt.Errorf("faction %q needs at least 2 steps", f.Name)
	}
	if storyline != nil {
		gf.Storyline = storyline.ID
	}
	for name, d := range f.FullDanger {
		l := exact(w, name)
		if l == nil {
			return nil, fmt.Errorf("faction %q: unknown place %q", f.Name, name)
		}
		if gf.FullDanger == nil {
			gf.FullDanger = map[string]int{}
		}
		gf.FullDanger[l.ID] = d
	}
	return gf, nil
}
