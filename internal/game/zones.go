package game

import (
	"fmt"
	"strings"
)

// Zone layout limits.
const (
	MinZones = 2
	MaxZones = 6
)

// ZoneSpec is an authored zone (define_zones tool).
type ZoneSpec struct {
	Name     string   `json:"name"`
	Desc     string   `json:"desc"`
	Cover    bool     `json:"cover"`
	Adjacent []string `json:"adjacent"` // zone names
	Exits    []string `json:"exits"`    // exit ids or destination names leaving from here
	NPCs     []string `json:"npcs"`     // ids or names of NPCs standing here
}

// DefineZones validates and installs a zone layout for loc. Adjacency is
// made symmetric; the layout must be connected; exits not assigned go to
// the first zone. Characters, NPCs and enemies are re-placed if their zone
// no longer exists.
func DefineZones(w *World, locID string, specs []ZoneSpec) error {
	l := w.Location(locID)
	if l == nil {
		return fmt.Errorf("unknown location %q", locID)
	}
	if len(specs) > MaxZones {
		specs = specs[:MaxZones]
	}
	if len(specs) < MinZones {
		return fmt.Errorf("need %d to %d zones, got %d", MinZones, MaxZones, len(specs))
	}
	zones := make([]Zone, 0, len(specs))
	byName := map[string]int{}
	for i, s := range specs {
		name := trimTo(s.Name, 40)
		if name == "" {
			return fmt.Errorf("zone %d has no name", i+1)
		}
		if _, dup := byName[normName(name)]; dup {
			return fmt.Errorf("duplicate zone name %q", name)
		}
		byName[normName(name)] = i
		zones = append(zones, Zone{ID: fmt.Sprintf("%s-z%d", l.ID, i+1), Name: name, Desc: trimTo(s.Desc, 200), Cover: s.Cover})
	}
	adj := make([]map[int]bool, len(zones))
	for i := range adj {
		adj[i] = map[int]bool{}
	}
	for i, s := range specs {
		for _, a := range s.Adjacent {
			j, ok := byName[normName(a)]
			if !ok {
				for k, z := range zones {
					if z.ID == a {
						j, ok = k, true
					}
				}
			}
			if !ok {
				return fmt.Errorf("zone %q lists unknown neighbour %q", zones[i].Name, a)
			}
			if j != i {
				adj[i][j], adj[j][i] = true, true
			}
		}
	}
	for i := range zones {
		for j := range zones {
			if adj[i][j] {
				zones[i].Adjacent = append(zones[i].Adjacent, zones[j].ID)
			}
		}
	}
	// connectivity
	seen := map[int]bool{0: true}
	queue := []int{0}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		for j := range adj[c] {
			if !seen[j] {
				seen[j] = true
				queue = append(queue, j)
			}
		}
	}
	if len(seen) != len(zones) {
		return fmt.Errorf("zones are not all connected")
	}
	l.Zones = zones
	// exits
	exits := w.Exits(locID, true)
	assigned := map[string]bool{}
	for i, s := range specs {
		for _, ref := range s.Exits {
			for _, x := range exits {
				if x.Edge.ID == ref || NameMatch(x.To.Name, ref) || NameMatch(x.Label, ref) {
					x.Edge.SetZoneAt(locID, zones[i].ID)
					assigned[x.Edge.ID] = true
				}
			}
		}
	}
	for _, x := range exits {
		if !assigned[x.Edge.ID] {
			x.Edge.SetZoneAt(locID, zones[0].ID)
		}
	}
	for i, s := range specs {
		for _, ref := range s.NPCs {
			for _, n := range w.NPCsAt(locID) {
				if n.ID == ref || NameMatch(n.Name, ref) {
					n.ZoneID = zones[i].ID
				}
			}
		}
	}
	repairPlacements(w, l)
	return nil
}

// repairPlacements moves anyone standing in an unknown zone to a valid one.
func repairPlacements(w *World, l *Location) {
	if len(l.Zones) == 0 {
		return
	}
	valid := func(z string) bool { return l.Zone(z) != nil && l.Zone(z).ID == z }
	if w.Campaign.LocationID == l.ID {
		for _, c := range w.Characters {
			if !valid(c.ZoneID) {
				c.ZoneID = l.Zones[0].ID
			}
		}
	}
	// unplaced NPCs spread over the zones past the arrival zone, so a
	// town square is not one crowded corner
	spread := 0
	for _, n := range w.NPCsAt(l.ID) {
		if !valid(n.ZoneID) {
			i := len(l.Zones) - 1
			if len(l.Zones) > 1 {
				i = len(l.Zones) - 1 - spread%(len(l.Zones)-1)
			}
			n.ZoneID = l.Zones[i].ID
			spread++
		}
	}
	if w.Encounter != nil && w.Encounter.LocationID == l.ID {
		for _, e := range w.Encounter.Enemies {
			if !valid(e.ZoneID) {
				e.ZoneID = l.Zones[len(l.Zones)-1].ID
			}
		}
	}
}

// GenericZones is the fallback layout by location kind.
func GenericZones(kind string) []ZoneSpec {
	var names [3]string
	var cover [3]bool
	switch kind {
	case KindTown:
		names = [3]string{"Square", "Market Stalls", "Side Street"}
		cover = [3]bool{false, true, false}
	case KindCave:
		names = [3]string{"Cave Mouth", "Rock Pillars", "Deep Chamber"}
		cover = [3]bool{false, true, false}
	case KindDungeon, KindRuin:
		names = [3]string{"Entrance", "Broken Columns", "Inner Hall"}
		cover = [3]bool{false, true, false}
	case KindBuilding:
		names = [3]string{"Doorway", "Common Room", "Back Room"}
		cover = [3]bool{false, true, false}
	default:
		names = [3]string{"Trail", "Thicket", "Clearing"}
		cover = [3]bool{false, true, false}
	}
	return []ZoneSpec{
		{Name: names[0], Cover: cover[0], Adjacent: []string{names[1]}},
		{Name: names[1], Cover: cover[1], Adjacent: []string{names[0], names[2]}},
		{Name: names[2], Cover: cover[2], Adjacent: []string{names[1]}},
	}
}

// EnsureZones lays out generic zones if loc has none.
func EnsureZones(w *World, locID string) {
	l := w.Location(locID)
	if l == nil || len(l.Zones) > 0 {
		return
	}
	_ = DefineZones(w, locID, GenericZones(l.Kind))
}

// ZoneDistance is the BFS distance between two zones of loc, or -1.
func ZoneDistance(l *Location, a, b string) int {
	if a == b {
		return 0
	}
	if l == nil || l.Zone(a) == nil || l.Zone(b) == nil {
		return -1
	}
	a, b = l.Zone(a).ID, l.Zone(b).ID
	dist := map[string]int{a: 0}
	queue := []string{a}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == b {
			return dist[cur]
		}
		for _, n := range l.Zone(cur).Adjacent {
			if _, ok := dist[n]; !ok {
				dist[n] = dist[cur] + 1
				queue = append(queue, n)
			}
		}
	}
	return -1
}

// NextStep returns the neighbour of from that is one step closer to to.
func NextStep(l *Location, from, to string) string {
	d := ZoneDistance(l, from, to)
	if d <= 0 {
		return from
	}
	for _, n := range l.Zone(from).Adjacent {
		if ZoneDistance(l, n, to) == d-1 {
			return n
		}
	}
	return from
}

// InReach reports whether a weapon with reach r can hit across distance d.
func InReach(r Reach, d int) bool { return d >= 0 && d <= int(r) }

// CoverBonus is +2 AC when the target is in a cover zone and the attacker
// is in a different zone.
func CoverBonus(l *Location, attackerZone, targetZone string) int {
	if l == nil || attackerZone == targetZone {
		return 0
	}
	if z := l.Zone(targetZone); z != nil && z.Cover {
		return 2
	}
	return 0
}

// ZoneName returns a zone's display name, or the id.
func ZoneName(l *Location, id string) string {
	if l != nil {
		if z := l.Zone(id); z != nil {
			return z.Name
		}
	}
	return id
}

// ReachName describes reach in words.
func ReachName(r Reach) string {
	switch r {
	case ReachThrown:
		return "thrown (1 zone)"
	case ReachRanged:
		return "ranged (2 zones)"
	}
	return "melee (same zone)"
}

// zonesWord pluralizes a zone count.
func zonesWord(n int) string {
	if n == 1 {
		return "1 zone"
	}
	return fmt.Sprintf("%d zones", n)
}

// TagEnemies gives same-named enemies distinguishing tags such as
// "Bandit 2 (scarred one)". Authored tags are kept.
func TagEnemies(enemies []*Enemy) {
	count := map[string]int{}
	for _, e := range enemies {
		count[normName(e.Name)]++
	}
	seen := map[string]int{}
	flavor := []string{"scarred one", "tall one", "limping one", "loud one", "one-eyed", "quiet one"}
	for _, e := range enemies {
		k := normName(e.Name)
		if count[k] < 2 {
			continue
		}
		seen[k]++
		n := seen[k]
		base := strings.TrimSpace(e.Name)
		if !strings.HasSuffix(base, fmt.Sprintf(" %d", n)) {
			e.Name = fmt.Sprintf("%s %d", base, n)
		}
		if e.Tag == "" {
			e.Tag = flavor[(n-1)%len(flavor)]
		}
	}
}
