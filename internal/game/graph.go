package game

import (
	"fmt"
	"strings"
)

// Growth caps for lazy world expansion.
const (
	MaxNewExitsPerFill = 3
	MaxNPCsPerFill     = 4
	MaxGroundItems     = 4
	MaxMerchantStock   = 8
	MaxLocations       = 60
)

// Exit is a visible exit as seen from one location.
type Exit struct {
	Edge  *Edge
	To    *Location
	Label string
	Zone  string
}

// Exits lists exits from loc. Hidden ones are included only if withHidden.
func (w *World) Exits(loc string, withHidden bool) []Exit {
	var out []Exit
	for _, e := range w.Edges {
		if !e.Touches(loc) || (!withHidden && !e.State.Visible()) {
			continue
		}
		to := w.Location(e.Other(loc))
		if to == nil {
			continue
		}
		label := e.LabelAt(loc)
		if label == "" {
			label = "to " + to.Name
		}
		out = append(out, Exit{Edge: e, To: to, Label: label, Zone: e.ZoneAt(loc)})
	}
	return out
}

// FindExit resolves ref (edge id, destination id or name, or label) to a
// visible exit from loc.
func (w *World) FindExit(loc, ref string) *Exit {
	exits := w.Exits(loc, false)
	for i := range exits {
		if exits[i].Edge.ID == ref || exits[i].To.ID == ref {
			return &exits[i]
		}
	}
	for i := range exits {
		if NameMatch(exits[i].To.Name, ref) || NameMatch(exits[i].Label, ref) {
			return &exits[i]
		}
	}
	for i := range exits {
		if NameContains(exits[i].To.Name, ref) || NameContains(exits[i].Label, ref) {
			return &exits[i]
		}
	}
	return nil
}

// EdgeBetween returns the edge joining a and b, if any.
func (w *World) EdgeBetween(a, b string) *Edge {
	for _, e := range w.Edges {
		if e.Touches(a) && e.Touches(b) && a != b {
			return e
		}
	}
	return nil
}

// Connect adds an edge between two locations unless one exists.
func (w *World) Connect(a, b string, state EdgeState, labelA, labelB string) *Edge {
	if e := w.EdgeBetween(a, b); e != nil {
		return e
	}
	e := &Edge{ID: w.NewID("exit"), A: a, B: b, State: state, LabelA: labelA, LabelB: labelB}
	w.Edges = append(w.Edges, e)
	return e
}

// AddStub creates an unexplored location.
func (w *World) AddStub(name, hint, kind string, danger int) *Location {
	l := &Location{ID: w.NewID("loc"), Name: trimTo(name, 60), Hint: trimTo(hint, 200), Kind: normKind(kind), Danger: clamp(danger, 0, 5), Stub: true}
	w.Locations = append(w.Locations, l)
	return l
}

func normKind(k string) string {
	k = strings.ToLower(strings.TrimSpace(k))
	switch k {
	case KindTown, KindWilderness, KindDungeon, KindCave, KindBuilding, KindRuin:
		return k
	case "village", "city", "hub", "settlement":
		return KindTown
	case "forest", "road", "field", "swamp", "mountain", "outdoors":
		return KindWilderness
	case "crypt", "tomb", "lair", "fortress":
		return KindDungeon
	case "cavern", "mine", "tunnel":
		return KindCave
	case "tavern", "inn", "shop", "temple", "house", "tower":
		return KindBuilding
	}
	return KindWilderness
}

// TravelPath finds the shortest route from -> to over passable edges
// through visited locations only. It returns the hops after from.
func (w *World) TravelPath(from, to string) ([]string, error) {
	dest := w.Location(to)
	if dest == nil {
		return nil, fmt.Errorf("unknown destination %q", to)
	}
	if !dest.Visited {
		return nil, fmt.Errorf("%s has not been visited yet; travel only goes to places you have been", dest.Name)
	}
	if dest.ID == from {
		return nil, fmt.Errorf("you are already at %s", dest.Name)
	}
	prev := map[string]string{from: ""}
	queue := []string{from}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == dest.ID {
			break
		}
		for _, e := range w.Edges {
			if !e.Touches(cur) || !e.State.Passable() {
				continue
			}
			nxt := e.Other(cur)
			if _, seen := prev[nxt]; seen {
				continue
			}
			if l := w.Location(nxt); l == nil || !l.Visited {
				continue
			}
			prev[nxt] = cur
			queue = append(queue, nxt)
		}
	}
	if _, ok := prev[dest.ID]; !ok {
		return nil, fmt.Errorf("no known path to %s", dest.Name)
	}
	var path []string
	for cur := dest.ID; cur != from; cur = prev[cur] {
		path = append([]string{cur}, path...)
	}
	return path, nil
}

// Connected reports whether every location is reachable from the hub,
// ignoring edge state.
func (w *World) Connected() bool {
	if len(w.Locations) == 0 {
		return true
	}
	start := w.Campaign.HubID
	if w.Location(start) == nil {
		start = w.Locations[0].ID
	}
	seen := map[string]bool{start: true}
	queue := []string{start}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, e := range w.Edges {
			if e.Touches(cur) && !seen[e.Other(cur)] {
				seen[e.Other(cur)] = true
				queue = append(queue, e.Other(cur))
			}
		}
	}
	return len(seen) == len(w.Locations)
}

// NPCSpec is an authored NPC.
type NPCSpec struct {
	Name        string   `json:"name"`
	Role        string   `json:"role"`
	Desc        string   `json:"desc"`
	Disposition int      `json:"disposition"`
	Merchant    bool     `json:"merchant"`
	Stock       []string `json:"stock,omitempty"` // catalog keys
}

// ItemSpec is an authored item on the ground.
type ItemSpec struct {
	Key   string `json:"key,omitempty"`
	Name  string `json:"name,omitempty"`
	Kind  string `json:"kind,omitempty"`
	Desc  string `json:"desc,omitempty"`
	Value int    `json:"value,omitempty"`
}

// ExitSpec is an authored exit to a new or existing place.
type ExitSpec struct {
	To      string `json:"to"`    // destination name
	Label   string `json:"label"` // how the exit looks
	Hint    string `json:"hint"`
	Kind    string `json:"kind"`
	Danger  int    `json:"danger"`
	Hidden  bool   `json:"hidden,omitempty"`
	Locked  bool   `json:"locked,omitempty"`
	KeyItem string `json:"key_item,omitempty"`
	LockDC  int    `json:"lock_dc,omitempty"`
}

// LocationFill is the author's content for a stub's first visit.
type LocationFill struct {
	Description string     `json:"description"`
	Danger      *int       `json:"danger,omitempty"`
	NPCs        []NPCSpec  `json:"npcs"`
	Items       []ItemSpec `json:"items"`
	Exits       []ExitSpec `json:"exits"`
}

// FillLocation turns a stub into a full location, clamping growth.
// Exits naming an existing place connect to it; NPC names already in the
// world are reused rather than duplicated. Returns notes about clamping.
func FillLocation(w *World, locID string, f LocationFill) ([]string, error) {
	l := w.Location(locID)
	if l == nil {
		return nil, fmt.Errorf("unknown location %q", locID)
	}
	if !l.Stub {
		return nil, fmt.Errorf("%s is already filled in", l.Name)
	}
	var notes []string
	l.Description = trimTo(f.Description, 800)
	if l.Description == "" {
		l.Description = l.Hint
	}
	if f.Danger != nil && !l.IsHub {
		l.Danger = clamp(*f.Danger, max(0, l.Danger-1), min(5, l.Danger+1))
	}
	if l.IsHub {
		l.Danger = 0
	}
	l.Stub = false

	for i, ns := range f.NPCs {
		if i >= MaxNPCsPerFill {
			notes = append(notes, "extra npcs dropped")
			break
		}
		if strings.TrimSpace(ns.Name) == "" {
			continue
		}
		if existing := w.NPC(ns.Name); existing != nil && NameMatch(existing.Name, ns.Name) {
			notes = append(notes, "reused npc "+existing.Name)
			continue
		}
		n := AddNPC(w, l.ID, ns)
		for j, key := range ns.Stock {
			if j >= MaxMerchantStock {
				break
			}
			if d, ok := LookupDef(key); ok && n.Merchant {
				NewItem(w, d.Key, Owner{Kind: OwnerNPC, ID: n.ID})
			}
		}
	}
	for i, is := range f.Items {
		if i >= MaxGroundItems {
			notes = append(notes, "extra items dropped")
			break
		}
		PlaceItem(w, is, Owner{Kind: OwnerLocation, ID: l.ID})
	}
	added := 0
	for _, xs := range f.Exits {
		if strings.TrimSpace(xs.To) == "" {
			continue
		}
		state := EdgeKnown
		switch {
		case xs.Hidden:
			state = EdgeHidden
		case xs.Locked:
			state = EdgeLocked
		}
		if dest := w.Location(xs.To); dest != nil && NameMatch(dest.Name, xs.To) {
			if dest.ID == l.ID || w.EdgeBetween(l.ID, dest.ID) != nil {
				continue
			}
			if dest.IsLair {
				continue // the lair is only reached by its gated edge
			}
			e := w.Connect(l.ID, dest.ID, state, trimTo(xs.Label, 80), "")
			setLock(e, xs)
			notes = append(notes, "linked existing "+dest.Name)
			continue
		}
		if added >= MaxNewExitsPerFill || len(w.Locations) >= MaxLocations {
			notes = append(notes, "extra exits dropped")
			continue
		}
		danger := xs.Danger
		if danger == 0 {
			danger = l.Danger
		}
		danger = clamp(danger, max(0, l.Danger-1), min(4, l.Danger+1)) // only the lair is 5
		stub := w.AddStub(xs.To, xs.Hint, xs.Kind, danger)
		e := w.Connect(l.ID, stub.ID, state, trimTo(xs.Label, 80), "back to "+l.Name)
		setLock(e, xs)
		added++
	}
	if !w.Connected() {
		return notes, fmt.Errorf("world graph became disconnected")
	}
	return notes, nil
}

func setLock(e *Edge, xs ExitSpec) {
	if e.State != EdgeLocked {
		return
	}
	e.KeyItem = strings.TrimSpace(xs.KeyItem)
	e.LockDC = 15
	if xs.LockDC > 0 {
		e.LockDC = clamp(xs.LockDC, 10, 20)
	}
}

// AddNPC creates an NPC at loc from a spec.
func AddNPC(w *World, loc string, ns NPCSpec) *NPC {
	n := &NPC{ID: w.NewID("npc"), Name: trimTo(ns.Name, 50), Role: trimTo(ns.Role, 40), Desc: trimTo(ns.Desc, 300),
		LocationID: loc, Disposition: clamp(ns.Disposition, -3, 3), Merchant: ns.Merchant, Alive: true, LastTalked: -1}
	if l := w.Location(loc); l != nil && len(l.Zones) > 0 {
		n.ZoneID = l.Zones[len(l.Zones)-1].ID
	}
	w.NPCs = append(w.NPCs, n)
	return n
}

// PlaceItem creates an authored item for owner (catalog key preferred).
func PlaceItem(w *World, is ItemSpec, owner Owner) *Item {
	if _, ok := Catalog[is.Key]; ok {
		return NewItem(w, is.Key, owner)
	}
	name := is.Name
	if name == "" {
		name = is.Key
	}
	if strings.TrimSpace(name) == "" {
		return nil
	}
	return CustomItem(w, name, is.Kind, trimTo(is.Desc, 200), is.Value, owner)
}

// RevealExit makes a hidden edge visible.
func RevealExit(w *World, edgeID string) error {
	e := w.Edge(edgeID)
	if e == nil {
		return fmt.Errorf("unknown exit %q", edgeID)
	}
	if e.State != EdgeHidden {
		return fmt.Errorf("exit %s is not hidden", edgeID)
	}
	e.State = EdgeKnown
	if e.KeyItem != "" || e.LairGate {
		e.State = EdgeLocked
	}
	return nil
}

// KeyFits reports whether the character holds the edge's key item.
func KeyFits(w *World, c *Character, e *Edge) bool {
	if e.KeyItem == "" || e.LairGate {
		return false
	}
	return w.FindItemOf(OwnerCharacter, c.ID, e.KeyItem) != nil
}
