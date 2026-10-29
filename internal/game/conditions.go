package game

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Solvely-Colin/aidnd/internal/dice"
)

// Condition kinds. Effects (a simplified 5e set that fits zone combat):
//
//	poisoned    disadvantage on attacks and checks
//	prone       melee attacks against have advantage, ranged disadvantage;
//	            own attacks at disadvantage; standing up costs the next move
//	restrained  can't move; attacks against have advantage; own attacks at
//	            disadvantage; an escape check (action) ends it
//	frightened  disadvantage on attacks and checks; can't move closer to
//	            whoever caused it
//	blinded     own attacks at disadvantage; attacks against have advantage
//	stunned     loses its turn; attacks against have advantage
const (
	CondPoisoned   = "poisoned"
	CondProne      = "prone"
	CondRestrained = "restrained"
	CondFrightened = "frightened"
	CondBlinded    = "blinded"
	CondStunned    = "stunned"
)

// ConditionKinds lists every condition an enemy or hazard can inflict, in
// display order.
var ConditionKinds = []string{CondPoisoned, CondProne, CondRestrained, CondFrightened, CondBlinded, CondStunned}

// Stances are short states characters give themselves with tactical
// actions. Nothing can inflict them.
const (
	CondDodging    = "dodging"
	CondDisengaged = "disengaged"
	CondHidden     = "hidden"
	CondHelped     = "helped"
)

// StanceKinds lists the stances in display order.
var StanceKinds = []string{CondHidden, CondDodging, CondDisengaged, CondHelped}

// allKinds is every condition and stance in display order.
var allKinds = append(append([]string{}, ConditionKinds...), StanceKinds...)

// condRule is a condition's default save and duration.
type condRule struct {
	Save     Ability // ability for the save that resists or ends it
	Rounds   int     // turns it lasts; 0 = until the bearer's next turn starts
	SaveEnds bool    // a save at the end of each of the bearer's turns ends it
	Lingers  bool    // survives the end of a fight
	// EndsAtStart conditions last until the start of the bearer's next
	// turn instead of counting down at the end of turns.
	EndsAtStart bool
	Summary     string // player-facing effect
}

var condRules = map[string]condRule{
	CondPoisoned:   {Save: CON, Rounds: 3, SaveEnds: true, Lingers: true, Summary: "disadvantage on attacks and checks"},
	CondProne:      {Save: DEX, Rounds: 0, Summary: "attacks at disadvantage; enemies close by hit more easily; standing up costs the next move"},
	CondRestrained: {Save: STR, Rounds: 3, SaveEnds: true, Summary: "can't move; attacks at disadvantage and easier to hit; break free with a STR or DEX check"},
	CondFrightened: {Save: WIS, Rounds: 2, SaveEnds: true, Summary: "disadvantage on attacks and checks; can't move closer to what scared them"},
	CondBlinded:    {Save: CON, Rounds: 1, Summary: "attacks at disadvantage and easier to hit"},
	CondStunned:    {Save: CON, Rounds: 1, Summary: "loses their next turn and is easier to hit"},
	// Stances. Hidden is set during the bearer's own turn, so 2 lasts to the
	// end of their next turn; helped is set on someone else's turn.
	CondDodging:    {EndsAtStart: true, Summary: "attacks against them have disadvantage until their next turn"},
	CondDisengaged: {EndsAtStart: true, Summary: "can move away from enemies without opportunity attacks this turn"},
	CondHidden:     {Rounds: 2, Summary: "enemies can't target them; their next attack has advantage and gives them away"},
	CondHelped:     {Rounds: 1, Summary: "advantage on their next attack or check"},
}

// MinStunDanger is the lowest location danger at which enemies may stun.
const MinStunDanger = 3

// Condition is one status effect on a character or enemy.
type Condition struct {
	Kind     string  `json:"kind"`
	Rounds   int     `json:"rounds"`              // bearer's turns left; 0 = ends when their next turn starts
	SaveDC   int     `json:"save_dc,omitempty"`   // end-of-turn save (or escape check) DC; 0 none
	Save     Ability `json:"save,omitempty"`      // ability for that save
	Source   string  `json:"source,omitempty"`    // who or what caused it, for display
	SourceID string  `json:"source_id,omitempty"` // enemy or character id, for frightened
}

// Conditions is the set on one creature, at most one of each kind.
type Conditions []Condition

// Has reports whether kind is present.
func (cs Conditions) Has(kind string) bool { return cs.Get(kind) != nil }

// Get returns the condition of kind, or nil.
func (cs Conditions) Get(kind string) *Condition {
	for i := range cs {
		if cs[i].Kind == kind {
			return &cs[i]
		}
	}
	return nil
}

// Kinds lists the condition names in display order.
func (cs Conditions) Kinds() []string {
	var out []string
	for _, k := range allKinds {
		if cs.Has(k) {
			out = append(out, k)
		}
	}
	return out
}

// String is "poisoned, prone" or "".
func (cs Conditions) String() string { return strings.Join(cs.Kinds(), ", ") }

// add puts c in the set, keeping the longer of two of the same kind.
func (cs *Conditions) add(c Condition) {
	if old := cs.Get(c.Kind); old != nil {
		if c.Rounds > old.Rounds {
			old.Rounds = c.Rounds
		}
		old.SaveDC = max(old.SaveDC, c.SaveDC)
		return
	}
	*cs = append(*cs, c)
	sort.SliceStable(*cs, func(i, j int) bool { return condOrder((*cs)[i].Kind) < condOrder((*cs)[j].Kind) })
}

// remove drops kind and reports whether it was there.
func (cs *Conditions) remove(kind string) bool {
	for i, c := range *cs {
		if c.Kind == kind {
			*cs = append((*cs)[:i], (*cs)[i+1:]...)
			return true
		}
	}
	return false
}

func condOrder(kind string) int {
	for i, k := range allKinds {
		if k == kind {
			return i
		}
	}
	return len(allKinds)
}

// IsStance reports whether kind is a self-given stance.
func IsStance(kind string) bool {
	for _, k := range StanceKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// ParseCondition normalizes a condition name ("knocked prone", "Poison").
func ParseCondition(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case s == "":
		return "", false
	case strings.Contains(s, "poison"), strings.Contains(s, "venom"), strings.Contains(s, "sicken"):
		return CondPoisoned, true
	case strings.Contains(s, "prone"), strings.Contains(s, "knock"), strings.Contains(s, "trip"):
		return CondProne, true
	case strings.Contains(s, "restrain"), strings.Contains(s, "grappl"), strings.Contains(s, "web"), strings.Contains(s, "entangl"), strings.Contains(s, "bound"):
		return CondRestrained, true
	case strings.Contains(s, "fright"), strings.Contains(s, "fear"), strings.Contains(s, "terrif"), strings.Contains(s, "scare"):
		return CondFrightened, true
	case strings.Contains(s, "blind"):
		return CondBlinded, true
	case strings.Contains(s, "stun"), strings.Contains(s, "daze"):
		return CondStunned, true
	}
	return "", false
}

// ConditionSummary is the player-facing effect of kind.
func ConditionSummary(kind string) string { return condRules[kind].Summary }

// ConditionSave is the ability that resists kind.
func ConditionSave(kind string) Ability { return condRules[kind].Save }

// InflictDC is the save DC for an enemy's on-hit condition.
func InflictDC(attackBonus int) int { return clamp(8+attackBonus, 10, 15) }

// HazardDC is the save DC for a narrated hazard at a location's danger.
func HazardDC(danger int) int { return 10 + clamp(danger, 0, 5) }

// SaveOutcome is a resisted condition: the save, and whether it landed.
type SaveOutcome struct {
	Roll    Roll
	Applied bool
	Line    string
}

// CharacterSave rolls c's save of ability a against dc. Poisoned and
// frightened give disadvantage, as on any check.
func CharacterSave(r *dice.Roller, c *Character, a Ability, dc int, why string) Roll {
	edge, reason := checkEdge(c.Conditions)
	ch := r.D20Edge(c.CheckMod(a), dc, edge)
	label := fmt.Sprintf("%s %s save", c.Name, a)
	if why != "" {
		label += " against " + why
	}
	return Roll{Label: label, Kind: "save", Natural: ch.Natural, Mod: ch.Mod, Total: ch.Total, Target: dc, Tier: ch.Tier,
		Edge: ch.Edge, Naturals: ch.Naturals, EdgeWhy: reason}
}

// EnemySave rolls an enemy's save: d20 + half its attack bonus.
func EnemySave(r *dice.Roller, e *Enemy, a Ability, dc int, why string) Roll {
	ch := r.D20(e.AttackBonus/2, dc)
	label := fmt.Sprintf("%s %s save", e.Display(), a)
	if why != "" {
		label += " against " + why
	}
	return Roll{Label: label, Kind: "save", Natural: ch.Natural, Mod: ch.Mod, Total: ch.Total, Target: dc, Tier: ch.Tier}
}

// newCondition builds a condition of kind with its default duration.
func newCondition(kind string, dc int, source, sourceID string) Condition {
	rule := condRules[kind]
	c := Condition{Kind: kind, Rounds: rule.Rounds, Source: trimTo(source, 40), SourceID: sourceID}
	if rule.SaveEnds || kind == CondRestrained {
		c.SaveDC, c.Save = dc, rule.Save
	}
	return c
}

// InflictOnCharacter makes c save against kind; on a failure it applies.
// A creature already down is unaffected.
func InflictOnCharacter(r *dice.Roller, c *Character, kind string, dc int, source, sourceID string) *SaveOutcome {
	if c.Down() {
		return nil
	}
	rule := condRules[kind]
	roll := CharacterSave(r, c, rule.Save, dc, kind)
	out := &SaveOutcome{Roll: roll}
	if roll.Tier.Succeeded() {
		out.Line = fmt.Sprintf("%s resists being %s (%s save %d vs DC %d)", c.Name, kind, rule.Save, roll.Total, dc)
		return out
	}
	c.Conditions.add(newCondition(kind, dc, source, sourceID))
	out.Applied = true
	out.Line = fmt.Sprintf("%s is %s (%s save %d vs DC %d)", c.Name, kind, rule.Save, roll.Total, dc)
	return out
}

// InflictOnEnemy is InflictOnCharacter for an enemy.
func InflictOnEnemy(r *dice.Roller, e *Enemy, kind string, dc int, source, sourceID string) *SaveOutcome {
	if e.Defeated() {
		return nil
	}
	rule := condRules[kind]
	roll := EnemySave(r, e, rule.Save, dc, kind)
	out := &SaveOutcome{Roll: roll}
	if roll.Tier.Succeeded() {
		out.Line = fmt.Sprintf("%s resists being %s (%s save %d vs DC %d)", e.Display(), kind, rule.Save, roll.Total, dc)
		return out
	}
	e.Conditions.add(newCondition(kind, dc, source, sourceID))
	out.Applied = true
	out.Line = fmt.Sprintf("%s is %s (%s save %d vs DC %d)", e.Display(), kind, rule.Save, roll.Total, dc)
	return out
}

// edge sums advantage (+1) and disadvantage (-1) sources; any of each
// cancel out, as in 5e. It returns -1, 0 or 1 and the reasons.
func edge(adv, dis []string) (int, string) {
	switch {
	case len(adv) > 0 && len(dis) > 0:
		return 0, ""
	case len(adv) > 0:
		return 1, strings.Join(adv, ", ")
	case len(dis) > 0:
		return -1, strings.Join(dis, ", ")
	}
	return 0, ""
}

// AttackEdge is the advantage state of an attack from a creature with
// attacker conditions on one with target conditions, dist zones apart.
func AttackEdge(attacker, target Conditions, dist int) (int, string) {
	var adv, dis []string
	for _, k := range []string{CondPoisoned, CondFrightened, CondBlinded, CondRestrained, CondProne} {
		if attacker.Has(k) {
			dis = append(dis, "attacker "+k)
		}
	}
	for _, k := range []string{CondRestrained, CondBlinded, CondStunned} {
		if target.Has(k) {
			adv = append(adv, "target "+k)
		}
	}
	if attacker.Has(CondHidden) {
		adv = append(adv, "attacking from hiding")
	}
	if attacker.Has(CondHelped) {
		adv = append(adv, "helped by an ally")
	}
	if target.Has(CondDodging) {
		dis = append(dis, "target dodging")
	}
	if target.Has(CondProne) {
		if dist == 0 {
			adv = append(adv, "target prone")
		} else {
			dis = append(dis, "target prone at range")
		}
	}
	return edge(adv, dis)
}

// checkEdge is the advantage state of a creature's own ability checks.
func checkEdge(cs Conditions) (int, string) {
	var adv, dis []string
	for _, k := range []string{CondPoisoned, CondFrightened} {
		if cs.Has(k) {
			dis = append(dis, k)
		}
	}
	if cs.Has(CondHelped) {
		adv = append(adv, "helped by an ally")
	}
	return edge(adv, dis)
}

// CanMove reports whether conditions stop the bearer leaving its zone.
func CanMove(cs Conditions) error {
	if cs.Has(CondRestrained) {
		return fmt.Errorf("restrained and can't move; break free first")
	}
	if cs.Has(CondStunned) {
		return fmt.Errorf("stunned")
	}
	return nil
}

// frightOf is the zone of whatever frightened the bearer, if still fighting.
func frightOf(w *World, cs Conditions) (string, bool) {
	f := cs.Get(CondFrightened)
	if f == nil || f.SourceID == "" {
		return "", false
	}
	if e := w.Enemy(f.SourceID); e != nil && !e.Defeated() {
		return e.ZoneID, true
	}
	if c := w.Character(f.SourceID); c != nil && !c.Down() {
		return c.ZoneID, true
	}
	return "", false
}

// ConditionTick is what happened to conditions at a turn boundary.
type ConditionTick struct {
	Changes []string `json:"changes,omitempty"`
	Rolls   []Roll   `json:"rolls,omitempty"`
	// LostTurn is set when the bearer is stunned and loses the turn.
	LostTurn bool `json:"lost_turn,omitempty"`
}

func (t *ConditionTick) add(o ConditionTick) {
	t.Changes = append(t.Changes, o.Changes...)
	t.Rolls = append(t.Rolls, o.Rolls...)
	t.LostTurn = t.LostTurn || o.LostTurn
}

// StartCharacterTurn applies start-of-turn effects to c, whose fresh budget
// is b: standing up from prone spends the move, and a stun takes the whole
// turn (the caller ends it).
func StartCharacterTurn(c *Character, b *Budget) ConditionTick {
	var t ConditionTick
	c.Readied = nil
	for _, k := range StanceKinds {
		if condRules[k].EndsAtStart {
			c.Conditions.remove(k) // lapses quietly
		}
	}
	if c.Conditions.remove(CondProne) {
		if c.Conditions.Has(CondRestrained) || c.Conditions.Has(CondStunned) {
			c.Conditions.add(Condition{Kind: CondProne}) // can't get up yet
		} else {
			b.Move = 0
			t.Changes = append(t.Changes, fmt.Sprintf("%s gets back up (that takes the move)", c.Name))
		}
	}
	if s := c.Conditions.Get(CondStunned); s != nil {
		t.LostTurn = true
		b.Move, b.Action = 0, 0
		t.Changes = append(t.Changes, fmt.Sprintf("%s is stunned and loses the turn", c.Name))
	}
	return t
}

// EndCharacterTurn ticks c's conditions down at the end of their turn:
// save-ends conditions get a save, and durations run out.
func EndCharacterTurn(r *dice.Roller, c *Character) ConditionTick {
	var t ConditionTick
	keep := c.Conditions[:0]
	for _, cond := range c.Conditions {
		if cond.Kind == CondProne || condRules[cond.Kind].EndsAtStart {
			keep = append(keep, cond) // ends at their next turn (standing up, or the stance lapsing)
			continue
		}
		cond.Rounds--
		if cond.Rounds <= 0 {
			t.Changes = append(t.Changes, fmt.Sprintf("%s is no longer %s", c.Name, cond.Kind))
			continue
		}
		if condRules[cond.Kind].SaveEnds && cond.SaveDC > 0 {
			roll := CharacterSave(r, c, cond.Save, cond.SaveDC, cond.Kind)
			t.Rolls = append(t.Rolls, roll)
			if roll.Tier.Succeeded() {
				t.Changes = append(t.Changes, fmt.Sprintf("%s shakes off being %s (%s save %d vs DC %d)", c.Name, cond.Kind, cond.Save, roll.Total, cond.SaveDC))
				continue
			}
		}
		keep = append(keep, cond)
	}
	c.Conditions = keep
	return t
}

// startEnemyTurn applies start-of-turn effects to an enemy: a stun takes
// the turn and standing from prone takes the move. It reports whether the
// enemy may still move this turn.
func startEnemyTurn(e *Enemy) (ConditionTick, bool) {
	var t ConditionTick
	canMove := true
	if e.Conditions.Has(CondStunned) {
		t.LostTurn = true
		t.Changes = append(t.Changes, fmt.Sprintf("%s is stunned and loses its turn", e.Display()))
		return t, false
	}
	if e.Conditions.Has(CondProne) && !e.Conditions.Has(CondRestrained) {
		e.Conditions.remove(CondProne)
		canMove = false
		t.Changes = append(t.Changes, fmt.Sprintf("%s gets back up", e.Display()))
	}
	if CanMove(e.Conditions) != nil {
		canMove = false
	}
	return t, canMove
}

func endEnemyTurn(r *dice.Roller, e *Enemy) ConditionTick {
	var t ConditionTick
	keep := e.Conditions[:0]
	for _, cond := range e.Conditions {
		if cond.Kind == CondProne {
			keep = append(keep, cond)
			continue
		}
		cond.Rounds--
		if cond.Rounds <= 0 {
			t.Changes = append(t.Changes, fmt.Sprintf("%s is no longer %s", e.Display(), cond.Kind))
			continue
		}
		if condRules[cond.Kind].SaveEnds && cond.SaveDC > 0 {
			roll := EnemySave(r, e, cond.Save, cond.SaveDC, cond.Kind)
			t.Rolls = append(t.Rolls, roll)
			if roll.Tier.Succeeded() {
				t.Changes = append(t.Changes, fmt.Sprintf("%s shakes off being %s", e.Display(), cond.Kind))
				continue
			}
		}
		keep = append(keep, cond)
	}
	e.Conditions = keep
	return t
}

// ClearFightConditions drops everything except lingering conditions
// (poison) when a fight ends.
func ClearFightConditions(w *World) {
	for _, c := range w.Characters {
		keep := c.Conditions[:0]
		for _, cond := range c.Conditions {
			if condRules[cond.Kind].Lingers {
				keep = append(keep, cond)
			}
		}
		c.Conditions = keep
	}
}

// ClearAllConditions is what rest and a setback do.
func ClearAllConditions(w *World) {
	for _, c := range w.Characters {
		c.Conditions = nil
	}
}

// NormalizeInflict validates an authored on-hit condition for a location
// of the given danger, returning "" (with a note) if it isn't allowed.
func NormalizeInflict(s string, danger int) (string, string) {
	if strings.TrimSpace(s) == "" {
		return "", ""
	}
	k, ok := ParseCondition(s)
	if !ok {
		return "", fmt.Sprintf("inflicts %q dropped", s)
	}
	if k == CondStunned && danger < MinStunDanger {
		return CondProne, fmt.Sprintf("inflicts stunned->prone below danger %d", MinStunDanger)
	}
	return k, ""
}
