// Package saga holds hand-written campaigns (the campaign bible as data)
// and builds a world from one. A saga is the top canon layer: the engine,
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

// Saga is a multi-act campaign.
type Saga struct {
	Key     string
	Title   string
	Pitch   string // player-facing
	Premise string
	Setting string // era key
	Acts    []Act
}

// Act is one chapter: a region, a mystery and a boss.
type Act struct {
	N        int
	Title    string
	Levels   [2]int // the level range it is built for
	Hub      Place
	Places   []Place
	People   []Person
	Things   []Thing
	Main     Quest // the last objective (defeat the boss) is added by Build
	Side     []Quest
	Secrets  []Secret
	Beats    []Beat
	Boss     Boss
	Openings []string // player-facing lines for the act card
}

// Place is a location. From names the place it connects to (the hub if
// empty). A lair connects through a gate that opens with the main quest.
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

// Quest is a main or side quest. Milestones maps an objective number
// (1-based) to the level every character reaches when it is done.
type Quest struct {
	Title, Summary, Giver string
	Solo                  bool // sized for one character
	Objectives            []game.ObjectiveSpec
	Gold, XP              int
	Rewards               []game.RewardItemSpec
	Milestones            map[int]int
}

// Secret is canon the narrator sees. Gate is "" (always open), "main:N"
// (after main objective N) or "side:<title>" (after that side quest).
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

// Boss is the act's antagonist, who waits in the lair.
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

// Info is a saga's player-facing card.
type Info struct {
	Key     string `json:"key"`
	Title   string `json:"title"`
	Pitch   string `json:"pitch"`
	Setting string `json:"setting"`
	Acts    int    `json:"acts"`
	Levels  string `json:"levels"`
}

// List returns every saga's card, by title.
func List() []Info {
	var out []Info
	for _, s := range registry {
		lo, hi := 20, 1
		for _, a := range s.Acts {
			lo, hi = min(lo, a.Levels[0]), max(hi, a.Levels[1])
		}
		out = append(out, Info{Key: s.Key, Title: s.Title, Pitch: s.Pitch, Setting: s.Setting, Acts: len(s.Acts), Levels: fmt.Sprintf("%d-%d", lo, hi)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out
}

// Options are the campaign settings Build doesn't take from the saga.
type Options struct {
	MaxSetbacks, MaxPlayers int
	Now                     int64
}

// Build makes a new world for act n of the saga. Every name in the act
// must resolve; a problem is an error, never a silently dropped piece.
func Build(s *Saga, n int, id string, opt Options) (*game.World, error) {
	if n < 1 || n > len(s.Acts) {
		return nil, fmt.Errorf("saga %s has no act %d", s.Key, n)
	}
	a := &s.Acts[n-1]
	w := &game.World{Campaign: game.Campaign{ID: id, Title: s.Title + ": " + a.Title, Premise: s.Premise, Pitch: s.Pitch,
		Setting: s.Setting, Status: game.StatusLobby, MaxSetbacks: opt.MaxSetbacks, MaxPlayers: opt.MaxPlayers, CreatedAt: opt.Now}}
	w.Campaign.Saga, w.Campaign.Act, w.Campaign.Dread = s.Key, n, true
	w.Campaign.Scene = game.Scene{Day: 1, Time: 1, Tension: 2}
	w.Campaign.Turns.Budget = game.FullBudget(false)

	hub := w.AddStub(a.Hub.Name, a.Hub.Hint, firstOf(a.Hub.Kind, game.KindTown), 0)
	hub.Stub, hub.IsHub, hub.Visited, hub.Description = false, true, true, a.Hub.Desc
	w.Campaign.HubID, w.Campaign.LocationID = hub.ID, hub.ID

	// places first, then the roads between them, so From can name any place
	for _, p := range a.Places {
		if w.Location(p.Name) != nil {
			return nil, fmt.Errorf("place %q twice", p.Name)
		}
		l := w.AddStub(p.Name, p.Hint, p.Kind, p.Danger)
		l.Description, l.IsLair = p.Desc, p.Lair
		// a hand-written description is canon; zones are laid out on arrival
		l.Stub = p.Desc == ""
	}
	var lair *game.Location
	for _, p := range a.Places {
		l := exact(w, p.Name)
		from := hub
		if p.From != "" {
			if from = exact(w, p.From); from == nil {
				return nil, fmt.Errorf("place %q connects to unknown %q", p.Name, p.From)
			}
		}
		if p.Lair {
			if lair != nil {
				return nil, fmt.Errorf("two lairs")
			}
			lair = l
			gate := w.Connect(from.ID, l.ID, game.EdgeLocked, "the way down to "+l.Name, "back to "+from.Name)
			gate.LairGate = true
			continue
		}
		w.Connect(from.ID, l.ID, game.EdgeKnown, "the way to "+l.Name, "back to "+from.Name)
	}
	if lair == nil {
		return nil, fmt.Errorf("act %d has no lair", n)
	}
	w.Campaign.LairID = lair.ID
	if !w.Connected() {
		return nil, fmt.Errorf("act %d's places do not form one map", n)
	}

	for _, p := range a.People {
		if _, err := addPerson(w, p); err != nil {
			return nil, err
		}
	}
	boss, err := addPerson(w, a.Boss.Person)
	if err != nil {
		return nil, err
	}
	if boss.LocationID != lair.ID {
		return nil, fmt.Errorf("the boss must wait in the lair")
	}
	boss.Antagonist, boss.Disposition = true, -3
	if boss.Stats == nil {
		boss.Stats = &game.StatBlock{MaxHP: 45, AC: 14, AttackBonus: 5, Damage: "1d8+2"}
	}
	boss.Stats.AsBoss = &game.BossSpec{Move: a.Boss.Move, Phases: a.Boss.Phases, Minion: a.Boss.Minion, MinionHorror: a.Boss.MinionHorror}
	w.Campaign.AntagonistID = boss.ID

	for _, t := range a.Things {
		owner := game.Owner{Kind: game.OwnerLocation}
		if t.Held {
			pn := exactNPC(w, t.At)
			if pn == nil {
				return nil, fmt.Errorf("thing %q held by unknown %q", t.Name, t.At)
			}
			owner = game.Owner{Kind: game.OwnerNPC, ID: pn.ID}
		} else {
			l := exact(w, t.At)
			if l == nil {
				return nil, fmt.Errorf("thing %q at unknown %q", t.Name, t.At)
			}
			owner.ID = l.ID
		}
		if game.PlaceItem(w, game.ItemSpec{Name: t.Name, Kind: t.Kind, Desc: t.Desc, Value: t.Value}, owner) == nil {
			return nil, fmt.Errorf("thing %q could not be placed", t.Name)
		}
	}

	main := a.Main
	main.Objectives = append(append([]game.ObjectiveSpec{}, main.Objectives...),
		game.ObjectiveSpec{Kind: game.ObjDefeatEnemy, Text: "Defeat " + boss.Name, Target: boss.ID})
	mq, err := addQuest(w, main, true)
	if err != nil {
		return nil, fmt.Errorf("main quest: %w", err)
	}
	quests := map[string]*game.Quest{}
	for _, q := range a.Side {
		sq, err := addQuest(w, q, false)
		if err != nil {
			return nil, fmt.Errorf("side quest %q: %w", q.Title, err)
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

	for _, sec := range a.Secrets {
		g, err := gate(sec.Gate)
		if err != nil {
			return nil, fmt.Errorf("secret %q: %w", sec.Title, err)
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

	for _, b := range a.Beats {
		beat := &game.Beat{ID: w.NewID("beat"), Title: b.Title, Text: b.Text, Trigger: b.Trigger, Value: b.Value, Effects: b.Effects}
		switch b.Trigger {
		case game.TriggerEnterLocation:
			l := exact(w, b.Target)
			if l == nil {
				return nil, fmt.Errorf("beat %q: unknown place %q", b.Title, b.Target)
			}
			beat.Target = l.ID
		case game.TriggerObjectiveDone:
			g, err := gate(b.Target)
			if err != nil || !strings.HasPrefix(b.Target, "main:") {
				return nil, fmt.Errorf("beat %q: target must be main:N", b.Title)
			}
			beat.Target = g
		case game.TriggerStalled, game.TriggerTension, game.TriggerTurn:
			if b.Value <= 0 {
				return nil, fmt.Errorf("beat %q needs a value", b.Title)
			}
		default:
			return nil, fmt.Errorf("beat %q: unknown trigger %q", b.Title, b.Trigger)
		}
		if b.Figure != "" {
			f := exactNPC(w, b.Figure)
			if f == nil {
				return nil, fmt.Errorf("beat %q: unknown figure %q", b.Title, b.Figure)
			}
			beat.EffectTarget = f.ID
		}
		w.Beats = append(w.Beats, beat)
	}
	// the finale always plays when the party reaches the lair
	w.Beats = append(w.Beats, &game.Beat{ID: w.NewID("beat"), Title: "The confrontation", Trigger: game.TriggerEnterLocation, Target: lair.ID,
		Text: boss.Name + " is here and confronts the party. Make the stakes plain.", Effects: []string{game.EffectAmbush}, EffectTarget: boss.ID})

	// milestones: main quest and side quests
	w.Campaign.Milestones = map[string]int{}
	for k, lvl := range main.Milestones {
		if k < 1 || k > len(mq.Objectives) {
			return nil, fmt.Errorf("milestone for missing objective %d", k)
		}
		w.Campaign.Milestones[mq.Objectives[k-1].ID] = lvl
	}
	return w, nil
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
