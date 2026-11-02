package game

import (
	"fmt"
	"strings"
)

// QuestUpdate describes a quest change for the Resolution and events.
type QuestUpdate struct {
	QuestID     string `json:"quest_id"`
	Title       string `json:"title"`
	ObjectiveID string `json:"objective_id,omitempty"`
	Text        string `json:"text"`
	Status      string `json:"status"`
	// Kind is lead, taken, step, done, turned_in or dropped.
	Kind string `json:"kind,omitempty"`
	Lead string `json:"lead,omitempty"`
	// Characters are the ones the news is for; empty means everyone.
	Characters []string `json:"characters,omitempty"`
	// Players are the players of Characters, so the server can keep the
	// news from everyone else (filled when the event is written).
	Players []string `json:"players,omitempty"`
	// By is the character who shared it (kind shared).
	By     string `json:"by,omitempty"`
	ByName string `json:"by_name,omitempty"`
}

// AcceptQuest puts the actor (and the friends with them who know of it)
// on a quest. A quest still on offer must come from a giver standing here
// unless the actor has already heard of it.
func AcceptQuest(w *World, questID string, c *Character) (*Quest, []QuestUpdate, error) {
	q := w.Quest(questID)
	if q == nil {
		return nil, nil, fmt.Errorf("unknown quest %q", questID)
	}
	if c != nil && q.Status == QuestOffered && !KnowsQuest(w, q, c) {
		if g := w.NPC(q.GiverID); g != nil && g.Alive && g.LocationID == w.Campaign.LocationID {
			HearOf(w, q, w.Characters...)
		} else if g != nil && g.Alive {
			return nil, nil, fmt.Errorf("%s offered that quest; find them at their location to accept", g.Name)
		}
	}
	if c == nil {
		return nil, nil, fmt.Errorf("nobody to take it on")
	}
	return TakeOn(w, questID, c)
}

// objectiveMet evaluates a non-freeform objective from game state.
func objectiveMet(w *World, q *Quest, o *Objective) bool {
	switch o.Kind {
	case ObjReachLocation:
		// main objectives tick in order, so a place reached early still
		// counts once the objectives before it are done. A side quest with
		// takers needs one of them to stand there.
		if w.Campaign.LocationID == o.Target {
			return true
		}
		l := w.Location(o.Target)
		return (q.IsMain || len(q.Takers) == 0) && l != nil && l.Visited
	case ObjDefeatEnemy:
		for _, d := range w.Defeats {
			if d.Turn < q.AcceptedTurn || ownSteps(q) && !takerDid(q, d.By) {
				continue
			}
			if d.NPCID != "" && d.NPCID == o.Target {
				return true
			}
			if defeatMatches(d.Name, o.Target) {
				return true
			}
		}
	case ObjObtainItem:
		for _, c := range w.Characters {
			if w.FindItemOf(OwnerCharacter, c.ID, o.Target) != nil {
				return true
			}
		}
	case ObjDeliverItem:
		return w.FindItemOf(OwnerNPC, o.DeliverTo, o.Target) != nil
	case ObjTalkToNPC:
		n := w.NPC(o.Target)
		if n == nil {
			return false
		}
		if ownSteps(q) {
			for _, id := range q.Takers {
				if t, ok := n.MetBy[id]; ok && t >= q.AcceptedTurn {
					return true
				}
			}
			return false
		}
		return n.LastTalked >= q.AcceptedTurn
	}
	return false
}

// defeatMatches reports whether defeating an enemy called name meets a
// defeat objective aimed at target. Whole words only, so a "Rat" never
// counts for "the pirate captain" and a plain "Bandit 2" never counts for
// "the bandit captain"; a plural target ("the wolves") counts any one of
// that kind.
func defeatMatches(name, target string) bool {
	t := normName(target)
	if t == "" {
		return false
	}
	if hasWords(normName(name), t) {
		return true
	}
	base := normName(baseName(name))
	if base == "" {
		return false
	}
	if t == base {
		return true
	}
	for _, pl := range plurals(base) {
		if hasWords(t, pl) {
			return true
		}
	}
	return false
}

// hasWords reports whether needle appears in hay as whole words.
func hasWords(hay, needle string) bool {
	return needle != "" && strings.Contains(" "+hay+" ", " "+needle+" ")
}

// plurals are the likely plural forms of a (normalized) name.
func plurals(n string) []string {
	out := []string{n + "s", n + "es"}
	switch {
	case strings.HasSuffix(n, "y"):
		out = append(out, n[:len(n)-1]+"ies")
	case strings.HasSuffix(n, "f"):
		out = append(out, n[:len(n)-1]+"ves")
	case strings.HasSuffix(n, "fe"):
		out = append(out, n[:len(n)-2]+"ves")
	case strings.HasSuffix(n, "man"):
		out = append(out, n[:len(n)-3]+"men")
	}
	return out
}

// baseName strips a trailing enemy number: "Bandit 2" -> "Bandit".
func baseName(n string) string {
	f := strings.Fields(n)
	if len(f) > 1 {
		last := f[len(f)-1]
		if strings.Trim(last, "0123456789") == "" {
			return strings.Join(f[:len(f)-1], " ")
		}
	}
	return n
}

// EvaluateQuests ticks every non-freeform objective whose condition holds,
// awards XP, completes quests and opens the lair gate. Main quest
// objectives are ordered: only the first open one can tick.
func EvaluateQuests(w *World) ([]QuestUpdate, []string) {
	var ups []QuestUpdate
	var changes []string
	ups, changes = TakeUpQuests(w)
	for _, q := range w.Quests {
		if q.Status != QuestActive || !QuestHere(w, q) {
			continue
		}
		for i := range q.Objectives {
			o := &q.Objectives[i]
			if o.Done {
				continue
			}
			if o.Kind != ObjFreeform && objectiveMet(w, q, o) {
				u, ch := completeObjective(w, q, o)
				ups = append(ups, u...)
				changes = append(changes, ch...)
			}
			if q.IsMain && !o.Done {
				break
			}
		}
	}
	changes = append(changes, UpdateLairGate(w)...)
	return ups, changes
}

// TakeUpQuests turns up and takes up side quests through play: an unheard
// quest whose wanted thing someone here now holds becomes a lead, and an
// offered quest someone here knows of is taken up once they stand where
// it sends them or hold what it asks for (see takenUp). Arrival calls it
// before looking for quest foes, so a named foe waiting at the place is
// there on the first visit.
func TakeUpQuests(w *World) ([]QuestUpdate, []string) {
	var ups []QuestUpdate
	var changes []string
	for _, q := range w.Quests {
		if found := discoverByFinding(w, q); len(found) > 0 {
			ups = append(ups, found...)
			changes = append(changes, fmt.Sprintf("New lead: %q", q.Title))
		}
		if q.Status != QuestOffered {
			continue
		}
		if c := takenUp(w, q); c != nil {
			if _, u, err := TakeOn(w, q.ID, c); err == nil {
				for i := range u {
					u[i].Text = "Quest taken up: " + q.Title
				}
				ups = append(ups, u...)
				changes = append(changes, fmt.Sprintf("Quest %q taken up", q.Title))
			}
		}
	}
	return ups, changes
}

// takenUp finds who has started on a side quest they heard about but
// never formally took on: someone here who knows of it is standing at a
// place the quest sends them to, or carries what it asks for. Old quests
// with no list of who knows count as heard once the giver was met
// (talked to or looked over). Someone who set it aside is never put on
// it this way.
func takenUp(w *World, q *Quest) *Character {
	if q.IsMain {
		return nil
	}
	var who *Character
	for _, c := range w.Characters {
		if c.Dead || containsStr(q.SetAside, c.ID) {
			continue
		}
		if !q.Logged {
			if g := w.NPC(q.GiverID); g != nil && g.LastTalked < 0 {
				return nil
			}
		} else if !containsStr(q.Known, c.ID) {
			continue
		}
		who = c
		break
	}
	if who == nil {
		return nil
	}
	for _, o := range q.Objectives {
		switch o.Kind {
		case ObjReachLocation:
			if o.Target != "" && w.Campaign.LocationID == o.Target {
				return who
			}
		case ObjObtainItem:
			for _, c := range w.Characters {
				if w.FindItemOf(OwnerCharacter, c.ID, o.Target) != nil {
					return who
				}
			}
		}
	}
	return nil
}

// CompleteFreeform ticks a freeform objective (judge only) with evidence.
func CompleteFreeform(w *World, questID, objID, evidence string) ([]QuestUpdate, []string, error) {
	q := w.Quest(questID)
	if q == nil || q.Status != QuestActive {
		return nil, nil, fmt.Errorf("quest %q is not active", questID)
	}
	if !QuestHere(w, q) {
		return nil, nil, fmt.Errorf("nobody here is on %q", q.Title)
	}
	for i := range q.Objectives {
		o := &q.Objectives[i]
		if o.ID != objID {
			if q.IsMain && !o.Done {
				return nil, nil, fmt.Errorf("main objectives complete in order")
			}
			continue
		}
		if o.Kind != ObjFreeform {
			return nil, nil, fmt.Errorf("objective %s is not freeform", objID)
		}
		if o.Done {
			return nil, nil, fmt.Errorf("objective %s already done", objID)
		}
		if strings.TrimSpace(evidence) == "" {
			return nil, nil, fmt.Errorf("evidence quote required")
		}
		o.Evidence = trimTo(evidence, 300)
		ups, ch := completeObjective(w, q, o)
		ch = append(ch, UpdateLairGate(w)...)
		return ups, ch, nil
	}
	return nil, nil, fmt.Errorf("unknown objective %q", objID)
}

func completeObjective(w *World, q *Quest, o *Objective) ([]QuestUpdate, []string) {
	o.Done = true
	o.DoneDay = w.Campaign.Scene.Day
	o.DoneBy = ""
	if by := takersHere(w, q); len(by) > 0 {
		o.DoneBy = trimTo(joinNames(by), 120)
	}
	who := questFor(q)
	ups := []QuestUpdate{{QuestID: q.ID, Title: q.Title, ObjectiveID: o.ID, Kind: QuestStep, Text: "Step done: " + o.Text, Status: q.Status, Characters: who}}
	changes := []string{fmt.Sprintf("Quest %q: %s (done)", q.Title, o.Text)}
	xp := XPSideObjective
	if q.IsMain {
		xp = XPMainObjective
	}
	crew := questCrew(w, q)
	for _, c := range crew {
		changes = append(changes, AwardXP(c, xp)...)
	}
	if lvl := w.Campaign.Milestones[o.ID]; lvl > 0 {
		for _, c := range crew {
			changes = append(changes, RaiseToLevel(c, lvl)...)
		}
	}
	all := true
	for _, ob := range q.Objectives {
		all = all && ob.Done
	}
	if all {
		q.Status = QuestCompleted
		tideOnQuest(w, q)
		text := "Quest complete: " + q.Title
		if q.GiverID != "" {
			if g := w.NPC(q.GiverID); g != nil {
				text += fmt.Sprintf(" (return to %s for the reward)", g.Name)
			}
		}
		ups = append(ups, QuestUpdate{QuestID: q.ID, Title: q.Title, Kind: QuestDone, Text: text, Status: q.Status, Characters: who})
		changes = append(changes, text)
		if q.GiverID == "" || giverDead(w, q) {
			changes = append(changes, grantRewards(w, q, nil)...)
			q.Status = QuestTurnedIn
			keepQuestPromise(w, q)
		}
	}
	return ups, changes
}

// TurnInQuest hands over a completed quest's rewards. The party must be at
// the giver's location. Gold goes to the character turning it in; reward
// items move from the giver to that character; XP goes to everyone.
func TurnInQuest(w *World, questID string, c *Character) ([]QuestUpdate, []string, error) {
	q := w.Quest(questID)
	if q == nil {
		return nil, nil, fmt.Errorf("unknown quest %q", questID)
	}
	if q.Status != QuestCompleted {
		return nil, nil, fmt.Errorf("quest %q is %s; finish its objectives first", q.Title, q.Status)
	}
	if c != nil && !OnQuest(q, c) {
		return nil, nil, fmt.Errorf("%s is not on %q; whoever took it on turns it in", c.Name, q.Title)
	}
	g := w.NPC(q.GiverID)
	if g == nil || !g.Alive {
		return nil, nil, fmt.Errorf("nobody can pay out %q", q.Title)
	}
	if g.LocationID != w.Campaign.LocationID {
		loc := w.Location(g.LocationID)
		where := g.LocationID
		if loc != nil {
			where = loc.Name
		}
		return nil, nil, fmt.Errorf("%s is at %s; turn the quest in there", g.Name, where)
	}
	changes := grantRewards(w, q, c)
	q.Status = QuestTurnedIn
	keepQuestPromise(w, q)
	g.Keep(fmt.Sprintf("day %d: %s completed %q for me", w.Campaign.Scene.Day, c.Name, q.Title))
	if c != nil {
		g.Settle(c.ID, TieOwed)
		g.Bind(w, c, TieDebt, "finished "+q.Title+" for them", SourceEngine)
		Note(w, c, RecordEntry{Kind: RecOutcome, Source: SourceEngine, NPCID: g.ID, Text: fmt.Sprintf("Finished %s for %s", q.Title, g.Name)})
	}
	if g.Disposition < 3 {
		g.Disposition++
	}
	return []QuestUpdate{{QuestID: q.ID, Title: q.Title, Kind: QuestReturned, Text: "Quest turned in: " + q.Title, Status: q.Status, Characters: questFor(q)}}, changes, nil
}

func grantRewards(w *World, q *Quest, c *Character) []string {
	var changes []string
	crew := questCrew(w, q)
	if c == nil && len(crew) > 0 {
		c = crew[0]
	}
	if c == nil && len(w.Characters) > 0 {
		c = w.Party()[0]
	}
	if c != nil && q.RewardGold > 0 {
		c.Gold += q.RewardGold
		changes = append(changes, fmt.Sprintf("%s receives %d gold", c.Name, q.RewardGold))
	}
	for _, id := range q.RewardItems {
		it := w.Item(id)
		if it == nil || c == nil {
			continue
		}
		held := it.Owner.Kind == OwnerNPC && it.Owner.ID == q.GiverID
		fell := giverDead(w, q) && it.Owner.Kind == OwnerLocation // dropped where the giver died
		if q.GiverID != "" && !held && !fell {
			continue // the giver no longer has it
		}
		it.Owner = Owner{Kind: OwnerCharacter, ID: c.ID}
		changes = append(changes, fmt.Sprintf("%s receives %s", c.Name, it.Name))
	}
	xp := q.RewardXP
	if xp == 0 && !q.IsMain {
		xp = XPSideQuest
	}
	for _, ch := range crew {
		changes = append(changes, AwardXP(ch, xp)...)
	}
	return changes
}

// UpdateLairGate unlocks lair-gate edges once every main objective except
// the last is done.
//
// Each storyline opens its own lair (where its boss waits); a world with
// one storyline opens every lair gate, as before.
func UpdateLairGate(w *World) []string {
	var changes []string
	mains := 0
	for _, q := range w.Quests {
		if q.IsMain {
			mains++
		}
	}
	for _, mq := range w.Quests {
		if !mq.IsMain || len(mq.Objectives) == 0 {
			continue
		}
		ready := true
		for _, o := range mq.Objectives[:len(mq.Objectives)-1] {
			ready = ready && o.Done
		}
		if !ready {
			continue
		}
		lair := w.Campaign.LairID
		if b := w.StorylineBoss(mq); b != nil && mains > 1 {
			lair = b.LocationID
		}
		for _, e := range w.Edges {
			if !e.LairGate || e.State == EdgeOpen || e.State == EdgeKnown || (mains > 1 && !e.Touches(lair)) {
				continue
			}
			e.State = EdgeKnown
			if to := w.Location(lair); to != nil {
				changes = append(changes, fmt.Sprintf("The way to %s is now open", to.Name))
			}
		}
	}
	return changes
}
