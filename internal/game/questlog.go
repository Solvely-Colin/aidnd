package game

import (
	"fmt"
	"strings"
)

// The quest log is per character. A side quest is heard of by the
// characters who were there when it came up (offered, overheard, found
// or rumored), taken on by the ones who say yes, and only moves forward
// for them: their steps tick where they stand, their XP comes when it is
// done. Party quests can be taken on by anyone who knows of them, and
// friends can tell each other; personal quests stay with one character.
// The storyline is everyone's.

// Quest update kinds, for the feed.
const (
	QuestLead     = "lead"      // newly heard of
	QuestTaken    = "taken"     // someone took it on
	QuestStep     = "step"      // a step marked off
	QuestDone     = "done"      // every step done
	QuestReturned = "turned_in" // rewards handed over
	QuestDropped  = "dropped"   // someone gave it up
	QuestShared   = "shared"    // a friend passed it on
)

// Everyone lists every character in the world, whichever group is focused.
func (w *World) Everyone() []*Character {
	if w.focus != nil {
		return w.focus.characters
	}
	return w.Characters
}

func (w *World) anyone(id string) *Character {
	for _, c := range w.Everyone() {
		if c.ID == id {
			return c
		}
	}
	return nil
}

// KnowsQuest reports whether a character has heard of a quest. Quests
// from before the quest log are known to all once their giver's place
// has been visited, as before.
func KnowsQuest(w *World, q *Quest, c *Character) bool {
	if q.IsMain {
		return true
	}
	if !q.Logged {
		if q.Status != QuestOffered {
			return true
		}
		g := w.NPC(q.GiverID)
		if g == nil {
			return false
		}
		l := w.Location(g.LocationID)
		return l != nil && l.Visited
	}
	return c != nil && containsStr(q.Known, c.ID)
}

// OnQuest reports whether a character has taken the quest on. An active
// quest from before the quest log is everyone's.
func OnQuest(q *Quest, c *Character) bool {
	if q.IsMain {
		return true
	}
	if q.Status == QuestOffered || q.Status == QuestFailed {
		return false
	}
	if len(q.Takers) == 0 {
		return true
	}
	return c != nil && containsStr(q.Takers, c.ID)
}

// QuestHere reports whether anyone in the focused group is on the quest,
// so it can move forward from what they do.
func QuestHere(w *World, q *Quest) bool {
	if q.IsMain || len(q.Takers) == 0 {
		return true
	}
	for _, c := range w.Characters {
		if containsStr(q.Takers, c.ID) {
			return true
		}
	}
	return false
}

// questCrew are the characters a quest's rewards and XP go to: its takers
// wherever they are, or, for the storyline and old quests, the group here.
func questCrew(w *World, q *Quest) []*Character {
	if q.IsMain || len(q.Takers) == 0 {
		return w.Characters
	}
	var out []*Character
	for _, id := range q.Takers {
		if c := w.anyone(id); c != nil && !c.Dead {
			out = append(out, c)
		}
	}
	return out
}

// takersHere names the quest's takers standing in the focused group.
func takersHere(w *World, q *Quest) []string {
	var out []string
	for _, c := range w.Characters {
		if c.Dead {
			continue
		}
		if q.IsMain || len(q.Takers) == 0 || containsStr(q.Takers, c.ID) {
			out = append(out, c.Name)
		}
	}
	return out
}

// ids of the quest's takers (nil for everyone), for event filtering.
func questFor(q *Quest) []string {
	if q.IsMain || len(q.Takers) == 0 {
		return nil
	}
	return append([]string{}, q.Takers...)
}

// HearOf makes characters aware of a quest. It returns the ones who had
// not heard of it before.
func HearOf(w *World, q *Quest, cs ...*Character) []*Character {
	if q.IsMain {
		return nil
	}
	var fresh []*Character
	for _, c := range cs {
		if c == nil || c.Dead || containsStr(q.Known, c.ID) {
			continue
		}
		if !q.Logged && KnowsQuest(w, q, c) {
			continue // an old quest everyone already knows
		}
		if q.Personal && len(q.Known) > 0 {
			continue // someone else's business
		}
		q.Known = append(q.Known, c.ID)
		q.Logged = true
		fresh = append(fresh, c)
	}
	return fresh
}

// leadUpdate announces a quest newly heard of by some characters.
func leadUpdate(q *Quest, cs []*Character) QuestUpdate {
	var ids []string
	for _, c := range cs {
		ids = append(ids, c.ID)
	}
	return QuestUpdate{QuestID: q.ID, Title: q.Title, Kind: QuestLead, Status: q.Status, Text: "New lead: " + q.Title, Lead: q.Lead, Characters: ids}
}

// TakeOn puts a character on a quest they know of, along with anyone in
// their group who knows of it too (a party quest is taken on together;
// anyone can drop it after). Taking on an active quest joins the friends
// already on it. The giver need not be present: they asked, and the
// answer can come later.
func TakeOn(w *World, questID string, c *Character) (*Quest, []QuestUpdate, error) {
	q := w.Quest(questID)
	if q == nil {
		return nil, nil, fmt.Errorf("unknown quest %q", questID)
	}
	if q.IsMain {
		return nil, nil, fmt.Errorf("everyone is on %s already", q.Title)
	}
	if c == nil {
		return nil, nil, fmt.Errorf("make a character first")
	}
	if !KnowsQuest(w, q, c) {
		return nil, nil, fmt.Errorf("%s has not heard of that", c.Name)
	}
	switch q.Status {
	case QuestOffered, QuestActive:
	default:
		return nil, nil, fmt.Errorf("%s is already %s", q.Title, strings.ReplaceAll(q.Status, "_", " "))
	}
	if OnQuest(q, c) {
		return nil, nil, fmt.Errorf("%s is already on %s", c.Name, q.Title)
	}
	if q.Personal && len(q.Takers) > 0 {
		return nil, nil, fmt.Errorf("that is someone else's to settle")
	}
	if q.Status == QuestActive && len(q.Takers) == 0 {
		// an old quest everyone was on; joining makes no difference
		return nil, nil, fmt.Errorf("%s is already on %s", c.Name, q.Title)
	}
	joining := []*Character{c}
	if !q.Personal {
		for _, m := range w.Characters {
			if m != c && !m.Dead && GroupOf(m) == GroupOf(c) && containsStr(q.Known, m.ID) && !containsStr(q.Takers, m.ID) && !containsStr(q.SetAside, m.ID) {
				joining = append(joining, m)
			}
		}
	}
	var names []string
	for _, m := range joining {
		q.Takers = append(q.Takers, m.ID)
		q.SetAside = removeStr(q.SetAside, m.ID)
		names = append(names, m.Name)
		if m.Tracked == "" || trackedDone(w, m) {
			m.Tracked = q.ID
		}
	}
	if q.Status == QuestOffered {
		q.Status = QuestActive
		q.AcceptedTurn = w.Campaign.Turn
		questPromise(w, q)
	}
	text := fmt.Sprintf("%s took on %s", joinNames(names), q.Title)
	return q, []QuestUpdate{{QuestID: q.ID, Title: q.Title, Kind: QuestTaken, Status: q.Status, Text: text, Characters: questFor(q)}}, nil
}

// DropQuest takes a character off a quest. When the last one drops it,
// the quest goes back to being a lead, and a promise made to its giver is
// broken.
func DropQuest(w *World, questID string, c *Character) ([]QuestUpdate, error) {
	q := w.Quest(questID)
	if q == nil {
		return nil, fmt.Errorf("unknown quest %q", questID)
	}
	if q.IsMain {
		return nil, fmt.Errorf("the storyline cannot be dropped")
	}
	if c == nil || !OnQuest(q, c) {
		return nil, fmt.Errorf("you are not on %s", q.Title)
	}
	if q.Status != QuestActive {
		return nil, fmt.Errorf("%s is %s", q.Title, strings.ReplaceAll(q.Status, "_", " "))
	}
	if len(q.Takers) == 0 {
		// an old quest everyone was on: everyone else stays on it
		for _, m := range w.Everyone() {
			if !m.Dead {
				q.Takers = append(q.Takers, m.ID)
			}
		}
	}
	logEveryone(w, q)
	q.Takers = removeStr(q.Takers, c.ID)
	q.SetAside = append(removeStr(q.SetAside, c.ID), c.ID)
	if c.Tracked == q.ID {
		c.Tracked = ""
	}
	text := fmt.Sprintf("%s gave up on %s", c.Name, q.Title)
	if len(q.Takers) == 0 {
		q.Status = QuestOffered
		for i := range w.Chronicle {
			if f := &w.Chronicle[i]; f.QuestID == q.ID && f.Kind == FactPromise && f.Status == PromiseOpen {
				settlePromise(w, f, false, c.Name+" gave it up")
			}
		}
	}
	for_ := append(questFor(q), c.ID)
	return []QuestUpdate{{QuestID: q.ID, Title: q.Title, Kind: QuestDropped, Status: q.Status, Text: text, Characters: for_}}, nil
}

// SetQuestAside files a lead under "not now" for a character.
func SetQuestAside(w *World, questID string, c *Character) error {
	q := w.Quest(questID)
	if q == nil || c == nil || !KnowsQuest(w, q, c) {
		return fmt.Errorf("unknown quest %q", questID)
	}
	if OnQuest(q, c) {
		return fmt.Errorf("drop it instead")
	}
	logEveryone(w, q)
	if !containsStr(q.SetAside, c.ID) {
		q.SetAside = append(q.SetAside, c.ID)
	}
	return nil
}

// TellOfQuest lets a character pass on a party quest they know to the
// friends standing with them. It returns who heard of it.
func TellOfQuest(w *World, questID string, c *Character) ([]*Character, error) {
	q := w.Quest(questID)
	if q == nil || c == nil || !KnowsQuest(w, q, c) {
		return nil, fmt.Errorf("unknown quest %q", questID)
	}
	if q.IsMain {
		return nil, fmt.Errorf("everyone knows the storyline")
	}
	if q.Personal {
		return nil, fmt.Errorf("that one is yours alone")
	}
	if !q.Logged {
		return nil, fmt.Errorf("everyone knows of it already")
	}
	var with []*Character
	for _, m := range w.Everyone() {
		if m != c && w.groupLocation(GroupOf(m)) == w.groupLocation(GroupOf(c)) {
			with = append(with, m)
		}
	}
	heard := HearOf(w, q, with...)
	if len(heard) == 0 {
		return nil, fmt.Errorf("nobody here who hasn't heard of it")
	}
	return heard, nil
}

// ShareQuest passes a party quest on to friends anywhere in the world (to
// lists character ids; empty means every friend not on it yet). It reaches
// them as a lead from the sharer: news for one who never heard of it, an
// invitation for one who did (even one who set it aside).
func ShareQuest(w *World, questID string, from *Character, to []string) (*Quest, []*Character, error) {
	q := w.Quest(questID)
	if q == nil || from == nil || !KnowsQuest(w, q, from) {
		return nil, nil, fmt.Errorf("unknown quest %q", questID)
	}
	if q.IsMain {
		return nil, nil, fmt.Errorf("everyone knows the storyline")
	}
	if q.Personal {
		return nil, nil, fmt.Errorf("that one is yours alone")
	}
	if q.Status != QuestOffered && q.Status != QuestActive {
		return nil, nil, fmt.Errorf("%s is already settled", q.Title)
	}
	logEveryone(w, q)
	var shared []*Character
	for _, m := range w.Everyone() {
		if m == from || m.Dead || OnQuest(q, m) || (len(to) > 0 && !containsStr(to, m.ID)) {
			continue
		}
		if !containsStr(q.Known, m.ID) {
			q.Known = append(q.Known, m.ID)
		}
		q.SetAside = removeStr(q.SetAside, m.ID)
		if q.SharedBy == nil {
			q.SharedBy = map[string]string{}
		}
		q.SharedBy[m.ID] = from.Name
		shared = append(shared, m)
	}
	if len(shared) == 0 {
		return nil, nil, fmt.Errorf("everyone you could share it with is already on it")
	}
	return q, shared, nil
}

// ShareCandidates are the friends a character could pass a quest on to.
func ShareCandidates(w *World, q *Quest, from *Character) []*Character {
	if q.IsMain || q.Personal || from == nil || (q.Status != QuestOffered && q.Status != QuestActive) || !KnowsQuest(w, q, from) {
		return nil
	}
	var out []*Character
	for _, m := range w.Everyone() {
		if m != from && !m.Dead && !OnQuest(q, m) {
			out = append(out, m)
		}
	}
	return out
}

// Track sets the quest a character follows on the play screen ("" for
// the storyline).
func Track(w *World, questID string, c *Character) error {
	if c == nil {
		return fmt.Errorf("make a character first")
	}
	if questID == "" {
		c.Tracked = ""
		return nil
	}
	q := w.Quest(questID)
	if q == nil || !KnowsQuest(w, q, c) {
		return fmt.Errorf("unknown quest %q", questID)
	}
	c.Tracked = q.ID
	return nil
}

func trackedDone(w *World, c *Character) bool {
	q := w.Quest(c.Tracked)
	return q == nil || q.Status == QuestTurnedIn || q.Status == QuestFailed || !OnQuest(q, c)
}

// OpenStep is the first open step of a quest, or nil.
func OpenStep(q *Quest) *Objective {
	for i := range q.Objectives {
		if !q.Objectives[i].Done {
			return &q.Objectives[i]
		}
	}
	return nil
}

// discoverByFinding lets a quest be found through play: holding the thing
// an unknown quest is after, or standing where its first step points,
// makes the group here aware of it.
func discoverByFinding(w *World, q *Quest) []QuestUpdate {
	if q.IsMain || q.Status != QuestOffered || !q.Logged || q.Personal {
		return nil
	}
	var finders []*Character
	for _, o := range q.Objectives {
		if o.Kind != ObjObtainItem {
			continue
		}
		for _, c := range w.Characters {
			if !containsStr(q.Known, c.ID) && w.FindItemOf(OwnerCharacter, c.ID, o.Target) != nil {
				finders = append(finders, c)
			}
		}
	}
	if len(finders) == 0 {
		return nil
	}
	// the find tells the whole group standing there
	heard := HearOf(w, q, w.Characters...)
	if len(heard) == 0 {
		return nil
	}
	if q.Lead == "" || q.Source == SourceOffered {
		q.Lead = fmt.Sprintf("%s found something someone wants", finders[0].Name)
	}
	return []QuestUpdate{leadUpdate(q, heard)}
}

func removeStr(xs []string, s string) []string {
	out := xs[:0:0]
	for _, x := range xs {
		if x != s {
			out = append(out, x)
		}
	}
	return out
}

// logEveryone turns an old quest that everyone knew into a kept list.
func logEveryone(w *World, q *Quest) {
	if q.Logged {
		return
	}
	for _, m := range w.Everyone() {
		if KnowsQuest(w, q, m) && !containsStr(q.Known, m.ID) {
			q.Known = append(q.Known, m.ID)
		}
	}
	q.Logged = true
}

// NewLead announces a side quest that just came up. A personal one is
// known only to the character it came to.
func NewLead(w *World, q *Quest, actor *Character) (QuestUpdate, bool) {
	if q.IsMain || q.Status != QuestOffered {
		return QuestUpdate{}, false
	}
	if q.Personal && actor != nil {
		q.Known, q.Logged = []string{actor.ID}, true
	}
	var cs []*Character
	for _, id := range q.Known {
		if c := w.anyone(id); c != nil {
			cs = append(cs, c)
		}
	}
	return leadUpdate(q, cs), true
}
