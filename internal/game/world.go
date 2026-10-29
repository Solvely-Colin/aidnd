package game

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// World is the whole state of one campaign. The store loads and saves it
// as a unit. Slices keep a stable order so seeded runs are reproducible.
type World struct {
	Campaign   Campaign         `json:"campaign"`
	Players    []*Player        `json:"players"`
	Characters []*Character     `json:"characters"`
	Locations  []*Location      `json:"locations"`
	Edges      []*Edge          `json:"edges"`
	NPCs       []*NPC           `json:"npcs"`
	Items      []*Item          `json:"items"`
	Quests     []*Quest         `json:"quests"`
	Encounter  *Encounter       `json:"encounter,omitempty"`
	Lore       []*Lore          `json:"lore"`
	Beats      []*Beat          `json:"beats"`
	Chronicle  []ChronicleEntry `json:"chronicle"`
	Defeats    []DefeatRecord   `json:"defeats"`
}

// NewID returns a fresh id like "loc12", unique within the world.
func (w *World) NewID(prefix string) string {
	w.Campaign.Seq++
	return fmt.Sprintf("%s%d", prefix, w.Campaign.Seq)
}

// NameMatch compares names leniently: case, punctuation and leading
// articles are ignored.
func NameMatch(a, b string) bool {
	na, nb := normName(a), normName(b)
	return na != "" && na == nb
}

// NameContains reports whether hay leniently contains needle.
func NameContains(hay, needle string) bool {
	n := normName(needle)
	return n != "" && strings.Contains(normName(hay), n)
}

func normName(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	space := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			space = false
		} else if !space && b.Len() > 0 {
			b.WriteByte(' ')
			space = true
		}
	}
	out := strings.TrimSpace(b.String())
	for _, art := range []string{"the ", "a ", "an "} {
		out = strings.TrimPrefix(out, art)
	}
	return out
}

// Player looks up a player by host user id.
func (w *World) Player(id string) *Player {
	for _, p := range w.Players {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// Character finds a character by id or name.
func (w *World) Character(ref string) *Character {
	for _, c := range w.Characters {
		if c.ID == ref {
			return c
		}
	}
	for _, c := range w.Characters {
		if NameMatch(c.Name, ref) {
			return c
		}
	}
	return nil
}

// CharacterOf returns a player's character.
func (w *World) CharacterOf(playerID string) *Character {
	for _, c := range w.Characters {
		if c.PlayerID == playerID {
			return c
		}
	}
	return nil
}

// Location finds a location by id or name.
func (w *World) Location(ref string) *Location {
	for _, l := range w.Locations {
		if l.ID == ref {
			return l
		}
	}
	for _, l := range w.Locations {
		if NameMatch(l.Name, ref) {
			return l
		}
	}
	return nil
}

// Here is the party's current location.
func (w *World) Here() *Location { return w.Location(w.Campaign.LocationID) }

// Edge finds an edge by id.
func (w *World) Edge(id string) *Edge {
	for _, e := range w.Edges {
		if e.ID == id {
			return e
		}
	}
	return nil
}

// NPC finds an NPC by id or name.
func (w *World) NPC(ref string) *NPC {
	for _, n := range w.NPCs {
		if n.ID == ref {
			return n
		}
	}
	for _, n := range w.NPCs {
		if NameMatch(n.Name, ref) {
			return n
		}
	}
	return nil
}

// NPCsAt lists living NPCs at a location.
func (w *World) NPCsAt(loc string) []*NPC {
	var out []*NPC
	for _, n := range w.NPCs {
		if n.LocationID == loc && n.Alive {
			out = append(out, n)
		}
	}
	return out
}

// Item finds an item by id.
func (w *World) Item(id string) *Item {
	for _, it := range w.Items {
		if it.ID == id {
			return it
		}
	}
	return nil
}

// ItemsOf lists items held by an owner.
func (w *World) ItemsOf(kind OwnerKind, id string) []*Item {
	var out []*Item
	for _, it := range w.Items {
		if it.Owner.Kind == kind && it.Owner.ID == id {
			out = append(out, it)
		}
	}
	return out
}

// FindItemOf resolves ref (id, catalog key or name) among an owner's items.
func (w *World) FindItemOf(kind OwnerKind, id, ref string) *Item {
	items := w.ItemsOf(kind, id)
	for _, it := range items {
		if it.ID == ref {
			return it
		}
	}
	for _, it := range items {
		if it.Key != "" && it.Key == ref {
			return it
		}
	}
	for _, it := range items {
		if NameMatch(it.Name, ref) {
			return it
		}
	}
	for _, it := range items {
		if NameContains(it.Name, ref) {
			return it
		}
	}
	return nil
}

// Quest finds a quest by id or title.
func (w *World) Quest(ref string) *Quest {
	for _, q := range w.Quests {
		if q.ID == ref {
			return q
		}
	}
	for _, q := range w.Quests {
		if NameMatch(q.Title, ref) {
			return q
		}
	}
	return nil
}

// MainQuest returns the storyline quest.
func (w *World) MainQuest() *Quest {
	for _, q := range w.Quests {
		if q.IsMain {
			return q
		}
	}
	return nil
}

// Enemy finds a living enemy in the encounter by id, display name or name.
func (w *World) Enemy(ref string) *Enemy {
	if w.Encounter == nil {
		return nil
	}
	living := w.Encounter.Living()
	for _, e := range living {
		if e.ID == ref {
			return e
		}
	}
	for _, e := range living {
		if NameMatch(e.Display(), ref) || (e.Tag != "" && NameMatch(e.Tag, ref)) {
			return e
		}
	}
	for _, e := range living {
		if NameMatch(e.Name, ref) {
			return e
		}
	}
	for _, e := range living {
		if NameContains(e.Display(), ref) {
			return e
		}
	}
	return nil
}

// InCombat reports whether a fight is under way here.
func (w *World) InCombat() bool {
	return w.Encounter != nil && w.Encounter.LocationID == w.Campaign.LocationID && len(w.Encounter.Living()) > 0
}

// Party returns characters in seat order.
func (w *World) Party() []*Character {
	out := make([]*Character, 0, len(w.Characters))
	for _, id := range w.Campaign.Turns.Order {
		if c := w.Character(id); c != nil {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		out = append(out, w.Characters...)
	}
	return out
}

// PartyLevel is the average level, rounded down, minimum 1.
func (w *World) PartyLevel() int {
	if len(w.Characters) == 0 {
		return 1
	}
	sum := 0
	for _, c := range w.Characters {
		sum += c.Level
	}
	if l := sum / len(w.Characters); l > 1 {
		return l
	}
	return 1
}

// AddChronicle records a public fact.
func (w *World) AddChronicle(text string) {
	w.Record(ChronicleEntry{Text: text})
}

// sortedStrings returns a sorted copy.
func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
