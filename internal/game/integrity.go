package game

import "fmt"

// Story integrity: players may kill anyone, but the story must stay
// finishable. When someone a quest needs is dead, RepairStory reroutes
// the quest instead of leaving it stuck. It is idempotent and cheap, so
// it runs when an NPC dies (to show the changes that turn) and before
// every save (to fix campaigns saved before it existed).
//
//   - Whatever the dead carried falls where they died, so a quest item or
//     a reward is never lost with them.
//   - A side quest still on offer from a dead giver is withdrawn.
//   - A quest whose giver is dead pays out as soon as it is done; there is
//     nobody to hand it in to.
//   - "Talk to X" with X dead becomes "search what X left behind" (reach
//     the place they died); "deliver the item to X" becomes "recover the
//     item".

// giverDead reports whether a quest's giver is a known NPC who is dead.
func giverDead(w *World, q *Quest) bool {
	if q.GiverID == "" {
		return false
	}
	g := w.NPC(q.GiverID)
	return g != nil && !g.Alive
}

// deadNPC returns the dead NPC ref names, or nil.
func deadNPC(w *World, ref string) *NPC {
	if ref == "" {
		return nil
	}
	if n := w.NPC(ref); n != nil && !n.Alive {
		return n
	}
	return nil
}

// RepairStory keeps every open quest finishable around the dead. It
// returns what changed, for the turn's log.
func RepairStory(w *World) []string {
	var changes []string
	for _, n := range w.NPCs {
		if n.Alive {
			continue
		}
		for _, it := range w.ItemsOf(OwnerNPC, n.ID) {
			it.Owner = Owner{Kind: OwnerLocation, ID: n.LocationID}
			changes = append(changes, fmt.Sprintf("%s lies where %s fell", it.Name, n.Name))
		}
	}
	for _, q := range w.Quests {
		if q.Status == QuestTurnedIn || q.Status == QuestFailed {
			continue
		}
		if giverDead(w, q) {
			g := w.NPC(q.GiverID)
			switch q.Status {
			case QuestOffered:
				if !q.IsMain {
					q.Status = QuestFailed
					changes = append(changes, fmt.Sprintf("With %s dead, nobody offers %q any more", g.Name, q.Title))
					continue
				}
			case QuestCompleted:
				changes = append(changes, fmt.Sprintf("%s is dead; the reward for %q is yours to take", g.Name, q.Title))
				changes = append(changes, grantRewards(w, q, nil)...)
				q.Status = QuestTurnedIn
				keepQuestPromise(w, q)
				continue
			}
		}
		for i := range q.Objectives {
			o := &q.Objectives[i]
			if o.Done {
				continue
			}
			switch o.Kind {
			case ObjTalkToNPC:
				if n := deadNPC(w, o.Target); n != nil {
					o.Kind, o.Target = ObjReachLocation, n.LocationID
					o.Text = fmt.Sprintf("Search what %s left behind", n.Name)
					changes = append(changes, fmt.Sprintf("Quest %q: %s can't talk now; search what they left behind", q.Title, n.Name))
				}
			case ObjDeliverItem:
				if n := deadNPC(w, o.DeliverTo); n != nil {
					name := o.Target
					if it := w.findItemAnywhere(o.Target); it != nil {
						name = it.Name
					}
					o.Kind, o.DeliverTo = ObjObtainItem, ""
					o.Text = "Recover " + name
					changes = append(changes, fmt.Sprintf("Quest %q: %s is dead; recover %s instead", q.Title, n.Name, name))
				}
			}
		}
	}
	return changes
}

// findItemAnywhere finds an item by id or name, whoever holds it.
func (w *World) findItemAnywhere(ref string) *Item {
	if it := w.Item(ref); it != nil {
		return it
	}
	for _, it := range w.Items {
		if NameMatch(it.Name, ref) {
			return it
		}
	}
	return nil
}
