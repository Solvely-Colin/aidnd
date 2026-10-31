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
}

// AcceptQuest activates an offered quest. The party must be with the giver.
func AcceptQuest(w *World, questID string) (*Quest, error) {
	q := w.Quest(questID)
	if q == nil {
		return nil, fmt.Errorf("unknown quest %q", questID)
	}
	if q.Status != QuestOffered {
		return nil, fmt.Errorf("quest %q is %s, not on offer", q.Title, q.Status)
	}
	if g := w.NPC(q.GiverID); g != nil && g.Alive && g.LocationID != w.Campaign.LocationID {
		return nil, fmt.Errorf("%s offered that quest; find them at their location to accept", g.Name)
	}
	q.Status = QuestActive
	q.AcceptedTurn = w.Campaign.Turn
	questPromise(w, q)
	return q, nil
}

// objectiveMet evaluates a non-freeform objective from game state.
func objectiveMet(w *World, q *Quest, o *Objective) bool {
	switch o.Kind {
	case ObjReachLocation:
		// main objectives tick in order, so a place reached early still
		// counts once the objectives before it are done
		l := w.Location(o.Target)
		return w.Campaign.LocationID == o.Target || (l != nil && l.Visited)
	case ObjDefeatEnemy:
		for _, d := range w.Defeats {
			if d.Turn < q.AcceptedTurn {
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
		return n != nil && n.LastTalked >= q.AcceptedTurn
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
	for _, q := range w.Quests {
		if q.Status != QuestActive {
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

// CompleteFreeform ticks a freeform objective (judge only) with evidence.
func CompleteFreeform(w *World, questID, objID, evidence string) ([]QuestUpdate, []string, error) {
	q := w.Quest(questID)
	if q == nil || q.Status != QuestActive {
		return nil, nil, fmt.Errorf("quest %q is not active", questID)
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
	ups := []QuestUpdate{{QuestID: q.ID, Title: q.Title, ObjectiveID: o.ID, Text: "Objective complete: " + o.Text, Status: q.Status}}
	changes := []string{fmt.Sprintf("Quest %q: %s (done)", q.Title, o.Text)}
	xp := XPSideObjective
	if q.IsMain {
		xp = XPMainObjective
	}
	for _, c := range w.Characters {
		changes = append(changes, AwardXP(c, xp)...)
	}
	if lvl := w.Campaign.Milestones[o.ID]; lvl > 0 {
		for _, c := range w.Characters {
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
		ups = append(ups, QuestUpdate{QuestID: q.ID, Title: q.Title, Text: text, Status: q.Status})
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
	if g.Disposition < 3 {
		g.Disposition++
	}
	return []QuestUpdate{{QuestID: q.ID, Title: q.Title, Text: "Quest turned in: " + q.Title, Status: q.Status}}, changes, nil
}

func grantRewards(w *World, q *Quest, c *Character) []string {
	var changes []string
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
	for _, ch := range w.Characters {
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
