package game

import (
	"fmt"
	"sort"
	"strings"
)

// Groups let one shared world hold several parties at once: a character
// can split off and go solo, and groups that meet can join up again. The
// world itself (places, people, items, quests, lore, the chronicle) is
// shared; what a group owns is where it stands, its scene, its turn order,
// its fight and its story so far.
//
// The main group lives in the campaign's own fields, so a world with one
// group plays exactly as before. Every other group is kept in
// Campaign.Groups. To act for a group, Focus swaps its state into the
// campaign fields and hides everyone else's characters, the engine runs
// unchanged, and Settle lays the world back out for saving.

// MainGroup is the id of the group kept in the campaign's own fields.
const MainGroup = "main"

// Group is a party that has split from the main one.
type Group struct {
	ID          string     `json:"id"`
	LocationID  string     `json:"location_id"`
	Scene       Scene      `json:"scene"`
	Turns       TurnState  `json:"turns"`
	Encounter   *Encounter `json:"encounter,omitempty"`
	Summary     string     `json:"summary,omitempty"`
	SummaryTurn int        `json:"summary_turn"`
}

// focus remembers what Focus hid, so Settle can put it back.
type focus struct {
	group      string
	characters []*Character
	players    []*Player
	members    map[string]bool // the group's character ids when focused
}

// GroupOf is the group a character is in.
func GroupOf(c *Character) string {
	if c == nil || c.Group == "" {
		return MainGroup
	}
	return c.Group
}

// Focused is the group the world is focused on ("" when it is not).
func (w *World) Focused() string {
	if w.focus == nil {
		return ""
	}
	return w.focus.group
}

// Group returns a split-off group by id.
func (w *World) Group(id string) *Group {
	for _, g := range w.Campaign.Groups {
		if g.ID == id {
			return g
		}
	}
	return nil
}

// Members lists the characters in a group, in seat order where known.
func (w *World) Members(group string) []*Character {
	all := w.Characters
	if w.focus != nil {
		all = w.focus.characters
	}
	var out []*Character
	for _, c := range all {
		if GroupOf(c) == group {
			out = append(out, c)
		}
	}
	return out
}

// GroupIDs lists every group with at least one member, main first.
func (w *World) GroupIDs() []string {
	out := []string{MainGroup}
	for _, g := range w.Campaign.Groups {
		out = append(out, g.ID)
	}
	return out
}

// Split reports whether the world has more than one group.
func (w *World) Split() bool { return len(w.Campaign.Groups) > 0 }

// Focus makes the world look like it holds only the group: its state is
// in the campaign fields and only its characters and their players are
// listed. Focusing on the main group just hides the others. Call Settle
// before saving (the store does it for you).
func (w *World) Focus(group string) error {
	if group == "" {
		group = MainGroup
	}
	if w.focus != nil {
		if w.focus.group == group {
			return nil
		}
		w.Settle()
	}
	if group != MainGroup {
		g := w.Group(group)
		if g == nil {
			return fmt.Errorf("no group %q", group)
		}
		swapGroup(w, g)
	}
	f := &focus{group: group, characters: w.Characters, players: w.Players, members: map[string]bool{}}
	w.Characters, w.Players = nil, nil
	for _, c := range f.characters {
		if GroupOf(c) == group {
			w.Characters = append(w.Characters, c)
			f.members[c.ID] = true
		}
	}
	for _, p := range f.players {
		if c := charIn(f.characters, p.CharacterID); c == nil || GroupOf(c) == group {
			w.Players = append(w.Players, p)
		}
	}
	w.focus = f
	return nil
}

// FocusPlayer focuses on the group of the player's character.
func (w *World) FocusPlayer(playerID string) error {
	p := w.Player(playerID)
	if p == nil {
		return nil
	}
	return w.Focus(GroupOf(w.Character(p.CharacterID)))
}

// Settle undoes Focus, leaving the world as it is stored. Characters and
// players added while focused are kept.
func (w *World) Settle() {
	f := w.focus
	if f == nil {
		return
	}
	w.focus = nil
	w.Characters = mergeChars(f.characters, w.Characters, f.members)
	w.Players = mergePlayers(f.players, w.Players)
	if f.group != MainGroup {
		if g := w.Group(f.group); g != nil {
			swapGroup(w, g)
		}
	}
}

// Settled runs fn on the world as stored, then puts the focus back.
func (w *World) Settled(fn func() error) error {
	group := w.Focused()
	w.Settle()
	err := fn()
	if group != "" {
		if ferr := w.Focus(group); ferr != nil && err == nil {
			err = ferr
		}
	}
	return err
}

// swapGroup exchanges a group's state with the campaign fields. It is its
// own inverse.
func swapGroup(w *World, g *Group) {
	c := &w.Campaign
	c.LocationID, g.LocationID = g.LocationID, c.LocationID
	c.Scene, g.Scene = g.Scene, c.Scene
	c.Turns, g.Turns = g.Turns, c.Turns
	w.Encounter, g.Encounter = g.Encounter, w.Encounter
	c.Summary, g.Summary = g.Summary, c.Summary
	c.SummaryTurn, g.SummaryTurn = g.SummaryTurn, c.SummaryTurn
}

func charIn(cs []*Character, id string) *Character {
	for _, c := range cs {
		if c.ID == id {
			return c
		}
	}
	return nil
}

// mergeChars keeps the full list's order, drops focused-group characters
// removed while focused, and appends new ones.
func mergeChars(all, focused []*Character, was map[string]bool) []*Character {
	now := map[string]bool{}
	for _, c := range focused {
		now[c.ID] = true
	}
	seen := map[string]bool{}
	var out []*Character
	for _, c := range all {
		if was[c.ID] && !now[c.ID] {
			continue
		}
		out = append(out, c)
		seen[c.ID] = true
	}
	for _, c := range focused {
		if !seen[c.ID] {
			out = append(out, c)
		}
	}
	return out
}

func mergePlayers(all, focused []*Player) []*Player {
	seen := map[string]bool{}
	byID := map[string]*Player{}
	for _, p := range focused {
		byID[p.ID] = p
	}
	var out []*Player
	for _, p := range all {
		if q, ok := byID[p.ID]; ok {
			p = q
		}
		out = append(out, p)
		seen[p.ID] = true
	}
	for _, p := range focused {
		if !seen[p.ID] {
			out = append(out, p)
		}
	}
	return out
}

// SplitOff moves characters into a new group of their own at the same
// place. The world must be settled (not focused). Characters in a fight,
// or splitting off a whole group, are refused.
func SplitOff(w *World, ids []string, now int64) (*Group, error) {
	if w.focus != nil {
		return nil, fmt.Errorf("settle the world before splitting")
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("nobody to split off")
	}
	from := ""
	var movers []*Character
	for _, id := range ids {
		c := w.Character(id)
		if c == nil {
			return nil, fmt.Errorf("no character %q", id)
		}
		if from == "" {
			from = GroupOf(c)
		} else if GroupOf(c) != from {
			return nil, fmt.Errorf("%s is in another group", c.Name)
		}
		movers = append(movers, c)
	}
	if len(movers) == len(w.Members(from)) {
		return nil, fmt.Errorf("that is the whole group already")
	}
	src := w.groupState(from)
	if fighting(src) {
		return nil, fmt.Errorf("nobody can split off in the middle of a fight")
	}
	g := &Group{
		ID:          w.NewID("grp"),
		LocationID:  src.LocationID,
		Scene:       src.Scene,
		Summary:     src.Summary,
		SummaryTurn: src.SummaryTurn,
	}
	g.Scene.Goal, g.Scene.Turns, g.Scene.Quiet = "", 0, 0
	moving := map[string]bool{}
	for _, c := range movers {
		c.Group = g.ID
		moving[c.ID] = true
		g.Turns.Order = append(g.Turns.Order, c.ID)
	}
	g.Turns.Budget = FullBudget(false)
	g.Turns.Started = now
	src.Turns.Order = dropIDs(src.Turns.Order, moving)
	if src.Turns.Index >= len(src.Turns.Order) {
		src.Turns.Index = 0
	}
	w.setGroupState(from, src)
	w.Campaign.Groups = append(w.Campaign.Groups, g)
	return g, nil
}

// JoinGroups folds group a into group b; they must stand in the same
// place. If b is fighting, a joins that fight; a can't bring one of its
// own. The world must be settled. If either is the main group, the
// joined group is the main one.
func JoinGroups(w *World, a, b string) error {
	if w.focus != nil {
		return fmt.Errorf("settle the world before joining")
	}
	if a == b {
		return fmt.Errorf("already together")
	}
	sa, sb := w.groupState(a), w.groupState(b)
	if sa == nil || sb == nil {
		return fmt.Errorf("no such group")
	}
	if sa.LocationID != sb.LocationID {
		return fmt.Errorf("the groups are not in the same place")
	}
	// a group can walk into another's fight, but nobody is pulled into
	// one, and two fights don't merge
	if fighting(sa) {
		return fmt.Errorf("finish your own fight first; the others can join you in it")
	}
	midFight := fighting(sb)
	// the joined group carries on with b's scene, story and fight; it is
	// the main group if either one was
	keep, gone := b, a
	if a == MainGroup {
		keep, gone = a, b
	}
	st := *sb
	st.Turns.Order = nil
	in := map[string]bool{}
	for _, id := range sb.Turns.Order {
		if c := w.Character(id); c != nil && !in[id] && GroupOf(c) == b {
			st.Turns.Order, in[id] = append(st.Turns.Order, id), true
		}
	}
	for _, g := range []string{b, a} {
		for _, c := range w.Members(g) {
			if !in[c.ID] {
				st.Turns.Order, in[c.ID] = append(st.Turns.Order, c.ID), true
			}
		}
	}
	if !midFight {
		st.Turns.Index = 0
		st.Turns.Budget = FullBudget(false)
	}
	// mid-fight, the newcomers take their turns after everyone already in
	// it, and the turn in progress carries on
	for _, c := range w.Characters {
		if g := GroupOf(c); g == a || g == b {
			c.Group = keep
			if keep == MainGroup {
				c.Group = ""
			}
		}
	}
	w.setGroupState(keep, &st)
	w.dropGroup(gone)
	return nil
}

// fighting reports whether a group's fight is still on.
func fighting(s *Group) bool {
	return s.Encounter != nil && s.Encounter.LocationID == s.LocationID && len(s.Encounter.Living()) > 0
}

// groupState copies a group's state (main's from the campaign fields).
func (w *World) groupState(id string) *Group {
	if id == MainGroup {
		c := &w.Campaign
		return &Group{ID: MainGroup, LocationID: c.LocationID, Scene: c.Scene, Turns: c.Turns, Encounter: w.Encounter, Summary: c.Summary, SummaryTurn: c.SummaryTurn}
	}
	g := w.Group(id)
	if g == nil {
		return nil
	}
	cp := *g
	return &cp
}

func (w *World) setGroupState(id string, s *Group) {
	if id == MainGroup {
		c := &w.Campaign
		c.LocationID, c.Scene, c.Turns, w.Encounter, c.Summary, c.SummaryTurn = s.LocationID, s.Scene, s.Turns, s.Encounter, s.Summary, s.SummaryTurn
		return
	}
	if g := w.Group(id); g != nil {
		s.ID = id
		*g = *s
	}
}

func (w *World) dropGroup(id string) {
	out := w.Campaign.Groups[:0]
	for _, g := range w.Campaign.Groups {
		if g.ID != id {
			out = append(out, g)
		}
	}
	w.Campaign.Groups = out
}

// PruneGroups drops split-off groups nobody is in any more (a character
// died or was replaced).
func PruneGroups(w *World) {
	if w.focus != nil {
		return
	}
	for _, g := range append([]*Group(nil), w.Campaign.Groups...) {
		if len(w.Members(g.ID)) == 0 {
			w.dropGroup(g.ID)
		}
	}
}

func dropIDs(ids []string, drop map[string]bool) []string {
	var out []string
	for _, id := range ids {
		if !drop[id] {
			out = append(out, id)
		}
	}
	return out
}

// Elsewhere describes the other groups for a prompt: who they are and
// where, those in the same place first. The world must be focused.
func (w *World) Elsewhere() []string {
	if w.focus == nil || !w.Split() {
		return nil
	}
	var here, away []string
	for _, id := range w.GroupIDs() {
		if id == w.focus.group {
			continue
		}
		var names []string
		for _, c := range w.Members(id) {
			if !c.Dead {
				names = append(names, c.Name)
			}
		}
		if len(names) == 0 {
			continue
		}
		loc := w.groupLocation(id)
		who := strings.Join(names, ", ")
		if loc == w.Campaign.LocationID {
			what := "here but not part of this scene's action"
			if st := w.peekGroup(id); st != nil && fighting(st) {
				what = "here and locked in a fight; this group may join it, but never narrate that fight's blows or outcome"
			}
			here = append(here, who+" (another group, "+what+")")
		} else if l := w.Location(loc); l != nil {
			away = append(away, who+" at "+l.Name)
		}
	}
	sort.Strings(away)
	return append(here, away...)
}

// groupLocation is where a group stands, whether or not it is focused.
func (w *World) groupLocation(id string) string {
	if st := w.peekGroup(id); st != nil {
		return st.LocationID
	}
	return ""
}

// peekGroup reads a group's state whether or not the world is focused.
// Don't modify what it returns.
func (w *World) peekGroup(id string) *Group {
	f := w.Focused()
	switch {
	case id == f || (f == "" && id == MainGroup):
		c := &w.Campaign
		return &Group{ID: id, LocationID: c.LocationID, Scene: c.Scene, Turns: c.Turns, Encounter: w.Encounter, Summary: c.Summary, SummaryTurn: c.SummaryTurn}
	case id == MainGroup:
		// main's state is parked in the focused group's record
		if g := w.Group(f); g != nil {
			cp := *g
			cp.ID = MainGroup
			return &cp
		}
		return nil
	}
	return w.Group(id)
}

// GroupInfo is what anyone can see of a group.
type GroupInfo struct {
	ID         string
	Members    []*Character
	LocationID string
	Fighting   bool
}

// GroupInfos lists every group, main first.
func (w *World) GroupInfos() []GroupInfo {
	var out []GroupInfo
	for _, id := range w.GroupIDs() {
		st := w.peekGroup(id)
		if st == nil {
			continue
		}
		gi := GroupInfo{ID: id, Members: w.Members(id), LocationID: st.LocationID}
		gi.Fighting = fighting(st)
		out = append(out, gi)
	}
	return out
}
