package game

import (
	"encoding/json"
	"fmt"

	"github.com/Solvely-Colin/aidnd/internal/dice"
)

// DeathSaves track a character at 0 HP. Three successes make them stable;
// three failures kill them (or, in a story campaign, leave them stable but
// out of the fight).
type DeathSaves struct {
	Successes int  `json:"successes"`
	Failures  int  `json:"failures"`
	Stable    bool `json:"stable,omitempty"`
}

// Dying reports whether the character is at 0 HP and still rolling death
// saves. Characters downed before death saves existed count as stable.
func (c *Character) Dying() bool {
	return c.HP <= 0 && !c.Dead && c.Death != nil && !c.Death.Stable
}

// KnockOut drops c to 0 HP. In a fight they start dying; outside one they
// are simply stable and unconscious.
func KnockOut(w *World, c *Character) {
	c.HP = 0
	c.Conditions = nil // a downed character's fight is over until revived
	c.Readied = nil
	c.Death = &DeathSaves{Stable: !w.InCombat()}
}

// Heal restores n HP (at least 1). A character at 0 HP wakes up and their
// death saves reset. The dead stay dead.
func Heal(c *Character, n int) (gained int, woke bool) {
	if c.Dead {
		return 0, false
	}
	woke = c.HP <= 0
	before := max(0, c.HP)
	c.HP = min(c.MaxHP, before+max(1, n))
	c.Death = nil
	return c.HP - before, woke
}

// SetHP sets HP directly (rests, setbacks), clearing death saves when the
// character is back above 0. The dead stay dead.
func SetHP(c *Character, hp int) {
	if c.Dead {
		return
	}
	c.HP = clamp(hp, 0, c.MaxHP)
	if c.HP > 0 {
		c.Death = nil
	}
}

// Stabilize stops a dying character's death saves.
func Stabilize(c *Character) bool {
	if !c.Dying() {
		return false
	}
	c.Death.Stable = true
	return true
}

// DeathSaveResult is one death save and what it did.
type DeathSaveResult struct {
	Roll    Roll   `json:"roll"`
	Line    string `json:"line"`
	Revived bool   `json:"revived,omitempty"` // natural 20: back up with 1 HP
	Stable  bool   `json:"stable,omitempty"`
	Died    bool   `json:"died,omitempty"`
}

// DeathSave rolls c's death save: a flat d20, 10 or higher succeeds, a 1
// counts as two failures and a 20 brings them back with 1 HP.
func DeathSave(w *World, r *dice.Roller, c *Character) DeathSaveResult {
	if c.Death == nil {
		c.Death = &DeathSaves{}
	}
	nat := r.Die(20)
	tier := dice.Failure
	if nat >= 10 {
		tier = dice.Success
	}
	if nat == 20 {
		tier = dice.CritSuccess
	} else if nat == 1 {
		tier = dice.CritFailure
	}
	out := DeathSaveResult{Roll: Roll{Label: c.Name + " death save", Kind: "death_save", Natural: nat, Total: nat, Target: 10, Tier: tier, Against: "DC"}}
	d := c.Death
	switch {
	case nat == 20:
		c.HP, c.Death = 1, nil
		out.Revived = true
		out.Line = fmt.Sprintf("%s rolls a natural 20 on a death save and gets back up with 1 HP", c.Name)
		return out
	case nat == 1:
		d.Failures += 2
	case nat >= 10:
		d.Successes++
	default:
		d.Failures++
	}
	out.Line = fmt.Sprintf("%s's death save: %d (%d success%s, %d failure%s)", c.Name, nat, d.Successes, plural(d.Successes, "es"), min(3, d.Failures), plural(d.Failures, "s"))
	switch {
	case d.Failures >= 3 && w.Campaign.Deadly():
		c.Dead = true
		out.Died = true
		out.Line = fmt.Sprintf("%s fails a third death save and dies", c.Name)
		w.Record(ChronicleEntry{Kind: FactDeath, Source: SourceEngine, Text: fmt.Sprintf("%s died at %s", c.Name, placeName(w))})
	case d.Failures >= 3:
		d.Stable = true
		out.Stable = true
		out.Line = fmt.Sprintf("%s fails a third death save but clings to life, unconscious and out of the fight", c.Name)
	case d.Successes >= 3:
		d.Stable = true
		out.Stable = true
		out.Line = fmt.Sprintf("%s succeeds a third death save and is stable", c.Name)
	}
	return out
}

func plural(n int, suffix string) string {
	if n == 1 {
		return ""
	}
	return suffix
}

func placeName(w *World) string {
	if l := w.Here(); l != nil {
		return l.Name
	}
	return "an unknown place"
}

// StabilizeDying makes every dying character stable (a fight ended or the
// party got away).
func StabilizeDying(w *World) []string {
	var out []string
	for _, c := range w.Characters {
		if Stabilize(c) {
			out = append(out, fmt.Sprintf("%s is stable, unconscious at 0 HP", c.Name))
		}
	}
	return out
}

// Readied is an attack held until an enemy comes within reach.
type Readied struct {
	Target string `json:"target,omitempty"` // enemy id; "" means the first to come
}

// Pending kinds.
const (
	PendingTurn      = "action"
	PendingDeathSave = "death_save"
)

// PendingAction is a turn waiting for its player to roll. The engine has
// already read what they want to do; rolling resolves it for real.
type PendingAction struct {
	Kind        string          `json:"kind"` // action | death_save
	CharacterID string          `json:"character_id"`
	PlayerID    string          `json:"player_id"`
	Text        string          `json:"text,omitempty"`
	Hints       json.RawMessage `json:"hints,omitempty"`
	Actions     []Action        `json:"actions,omitempty"`
	Preview     []PlannedStep   `json:"preview,omitempty"`
	Created     int64           `json:"created"`
}

// PlannedStep is one step of a pending turn as a dry run saw it: what it
// costs and the d20 it needs, with no result.
type PlannedStep struct {
	Kind  string        `json:"kind"`
	Label string        `json:"label"`
	Slot  Slot          `json:"slot,omitempty"`
	Error string        `json:"error,omitempty"`
	Rolls []PlannedRoll `json:"rolls,omitempty"`
}

// PlannedRoll is a d20 a pending step will roll: the modifier and the
// number to beat, but not the result.
type PlannedRoll struct {
	Label   string     `json:"label"`
	Kind    string     `json:"kind"`
	Mod     int        `json:"mod"`
	Parts   []RollPart `json:"parts,omitempty"`
	Target  int        `json:"target,omitempty"`
	Against string     `json:"against,omitempty"`
	Edge    string     `json:"edge,omitempty"`
	EdgeWhy string     `json:"edge_why,omitempty"`
}

// SetPending stores p as its character's pending turn.
func (c *Campaign) SetPending(p *PendingAction) {
	if c.Pending == nil {
		c.Pending = map[string]*PendingAction{}
	}
	c.Pending[p.CharacterID] = p
}

// DropStaleDeathSaves forgets waiting death saves for characters who are
// no longer dying (healed, stabilized, or the fight was called), so a
// healed player is not stuck being asked for a death save.
func DropStaleDeathSaves(w *World) {
	for id, p := range w.Campaign.Pending {
		if p == nil || p.Kind != PendingDeathSave {
			continue
		}
		if c := w.Character(id); c != nil && c.ID == id && !c.Dying() {
			w.Campaign.TakePending(id)
		}
	}
}

// TakePending removes and returns a character's pending turn.
func (c *Campaign) TakePending(characterID string) *PendingAction {
	p := c.Pending[characterID]
	delete(c.Pending, characterID)
	if len(c.Pending) == 0 {
		c.Pending = nil
	}
	return p
}

// JoinMidway sets up a character joining a campaign in progress: they
// arrive with xp in the given zone, levelled up the same way XP levels
// anyone (class hit dice and new spells for class sheets), at full HP.
func JoinMidway(w *World, c *Character, xp int, zone string) {
	if xp > c.XP {
		AwardXP(c, xp-c.XP)
	}
	c.HP = c.MaxHP
	if l := w.Here(); l != nil && l.Zone(zone) == nil && len(l.Zones) > 0 {
		zone = l.Zones[0].ID
	}
	c.ZoneID = zone
}
