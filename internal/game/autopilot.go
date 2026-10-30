package game

// AutopilotActions picks a plain, safe turn for a character whose player
// is away: strike the weakest foe in reach, or else dodge. It never spends
// spell slots, items or limited features; those choices are the player's.
func AutopilotActions(w *World, c *Character) []Action {
	if w.Encounter == nil {
		return []Action{{Kind: ActEndTurn}}
	}
	l := w.Here()
	reach := InferReach(Weapon(w, c))
	var best *Enemy
	for _, e := range w.Encounter.Living() {
		if e.Conditions.Has(CondInvisible) || !InReach(reach, ZoneDistance(l, c.ZoneID, e.ZoneID)) {
			continue
		}
		if best == nil || e.HP < best.HP {
			best = e
		}
	}
	if best != nil {
		return []Action{{Kind: ActAttack, Target: best.ID}}
	}
	return []Action{{Kind: ActDodge}}
}
