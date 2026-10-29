package game

import (
	"fmt"

	"github.com/Solvely-Colin/aidnd/internal/dice"
)

// Tactical actions from 5e: Dodge, Disengage, Help, Hide, Ready, Shove and
// Grapple, plus the reactions they interact with (opportunity attacks and
// readied attacks). Reactions resolve on their own: nobody waits for a
// player to answer mid-round.

// Tactical action kinds.
const (
	ActDodge     = "dodge"
	ActDisengage = "disengage"
	ActHelp      = "help"
	ActHide      = "hide"
	ActReady     = "ready"
	ActShove     = "shove"
	ActGrapple   = "grapple"
)

var tacticKinds = []string{ActDodge, ActDisengage, ActHelp, ActHide, ActReady, ActShove, ActGrapple}

func round(w *World) int {
	if w.Encounter == nil {
		return 0
	}
	return w.Encounter.Round
}

func needFight(w *World, what string) error {
	if !w.InCombat() {
		return fmt.Errorf("%s only matters in a fight", what)
	}
	return nil
}

func addStance(c *Character, kind string, source string) {
	c.Conditions.add(Condition{Kind: kind, Rounds: condRules[kind].Rounds, Source: trimTo(source, 40)})
}

func dodge(w *World, c *Character, st *Step) error {
	if err := needFight(w, "dodging"); err != nil {
		return err
	}
	addStance(c, CondDodging, "")
	st.Changes = append(st.Changes, fmt.Sprintf("%s takes the Dodge action: attacks against them have disadvantage until their next turn", c.Name))
	return nil
}

func disengage(w *World, c *Character, st *Step) error {
	if err := needFight(w, "disengaging"); err != nil {
		return err
	}
	addStance(c, CondDisengaged, "")
	st.Changes = append(st.Changes, fmt.Sprintf("%s disengages: moving away this turn draws no opportunity attacks", c.Name))
	return nil
}

func help(w *World, c *Character, a Action, st *Step) error {
	t := w.Character(a.Target)
	if t == nil || t.ID == c.ID {
		return fmt.Errorf("help whom? name another party member")
	}
	if t.Down() {
		return fmt.Errorf("%s is down; use a healer's kit or a Medicine check to stabilize them instead", t.Name)
	}
	if w.InCombat() && t.ZoneID != c.ZoneID {
		return fmt.Errorf("%s is in %s; move to them first", t.Name, ZoneName(w.Here(), t.ZoneID))
	}
	addStance(t, CondHelped, c.Name)
	st.Changes = append(st.Changes, fmt.Sprintf("%s helps %s: advantage on their next attack or check", c.Name, t.Name))
	return nil
}

func hide(ctx *Ctx, c *Character, st *Step) error {
	w := ctx.W
	if !w.InCombat() {
		return fmt.Errorf("outside a fight, sneaking is a Stealth check (ability_check with skill stealth)")
	}
	l := w.Here()
	z := l.Zone(c.ZoneID)
	dc := 0
	for _, e := range w.Encounter.Living() {
		if z != nil && !z.Cover && ZoneDistance(l, c.ZoneID, e.ZoneID) <= 0 {
			return fmt.Errorf("there's nowhere to hide with %s right here; move to a zone with cover", e.Display())
		}
		dc = max(dc, EnemyPassive(e))
	}
	sk, _ := SkillByKey("stealth")
	ed, why := checkEdge(c.Conditions)
	ch := ctx.R.D20Edge(c.SkillMod(sk), dc, ed)
	st.Rolls = append(st.Rolls, Roll{Label: c.Name + " Stealth check to hide", Kind: "check", Natural: ch.Natural, Mod: ch.Mod, Total: ch.Total, Target: dc,
		Tier: ch.Tier, Parts: SkillParts(c, sk), Against: "DC", Edge: ch.Edge, Naturals: ch.Naturals, EdgeWhy: why})
	st.Tier = ch.Tier
	c.Conditions.remove(CondHelped)
	if !ch.Tier.Succeeded() {
		st.Changes = append(st.Changes, fmt.Sprintf("%s tries to hide but is spotted (%d vs %d)", c.Name, ch.Total, dc))
		return nil
	}
	addStance(c, CondHidden, "")
	st.Changes = append(st.Changes, fmt.Sprintf("%s slips out of sight (%d vs %d): enemies can't target them, and their next attack has advantage", c.Name, ch.Total, dc))
	return nil
}

func ready(w *World, c *Character, a Action, st *Step) error {
	if err := needFight(w, "readying an attack"); err != nil {
		return err
	}
	r := &Readied{}
	who := "the first enemy that comes within reach"
	if a.Target != "" {
		e := w.Enemy(a.Target)
		if e == nil || e.Defeated() {
			return fmt.Errorf("no enemy %q here", a.Target)
		}
		r.Target, who = e.ID, e.Display()
	}
	c.Readied = r
	st.Changes = append(st.Changes, fmt.Sprintf("%s readies an attack against %s", c.Name, who))
	return nil
}

// ContestDC is the DC to resist a character's shove or grapple:
// 8 + STR modifier + proficiency (2024 rules).
func ContestDC(c *Character) int { return 8 + c.Mod(STR) + ProficiencyBonus(c.Level) }

func shove(ctx *Ctx, c *Character, a Action, st *Step) error {
	w := ctx.W
	l := w.Here()
	e, err := findFoe(w, a.Target, false)
	if err != nil {
		return err
	}
	if ZoneDistance(l, c.ZoneID, e.ZoneID) != 0 {
		return fmt.Errorf("%s is out of arm's reach; move into its zone first", e.Display())
	}
	push := a.Method == "push" || a.Method == "away"
	var to string
	if push {
		best := -1
		if z := l.Zone(e.ZoneID); z != nil {
			for _, adj := range z.Adjacent {
				if d := ZoneDistance(l, c.ZoneID, adj); d > best {
					best, to = d, adj
				}
			}
		}
		if to == "" {
			return fmt.Errorf("there's nowhere to shove %s to", e.Display())
		}
	}
	dc := ContestDC(c)
	roll := EnemySave(ctx.R, e, STR, dc, "being shoved by "+c.Name)
	roll.Foe, roll.By = true, e.Display()
	st.Rolls = append(st.Rolls, roll)
	st.Tier = flip(roll.Tier)
	c.Conditions.remove(CondHelped)
	if roll.Tier.Succeeded() {
		st.Changes = append(st.Changes, fmt.Sprintf("%s tries to shove %s but it holds its ground (save %d vs DC %d)", c.Name, e.Display(), roll.Total, dc))
		return nil
	}
	if push {
		e.ZoneID = to
		st.Changes = append(st.Changes, fmt.Sprintf("%s shoves %s back into %s (save %d vs DC %d)", c.Name, e.Display(), ZoneName(l, to), roll.Total, dc))
		return nil
	}
	e.Conditions.add(Condition{Kind: CondProne, Source: c.Name, SourceID: c.ID})
	st.Changes = append(st.Changes, fmt.Sprintf("%s knocks %s prone (save %d vs DC %d)", c.Name, e.Display(), roll.Total, dc))
	return nil
}

func grapple(ctx *Ctx, c *Character, a Action, st *Step) error {
	w := ctx.W
	l := w.Here()
	e, err := findFoe(w, a.Target, false)
	if err != nil {
		return err
	}
	if ZoneDistance(l, c.ZoneID, e.ZoneID) != 0 {
		return fmt.Errorf("%s is out of arm's reach; move into its zone first", e.Display())
	}
	dc := ContestDC(c)
	roll := EnemySave(ctx.R, e, STR, dc, "a grapple by "+c.Name)
	roll.Foe, roll.By = true, e.Display()
	st.Rolls = append(st.Rolls, roll)
	st.Tier = flip(roll.Tier)
	c.Conditions.remove(CondHelped)
	if roll.Tier.Succeeded() {
		st.Changes = append(st.Changes, fmt.Sprintf("%s grabs for %s, who twists free (save %d vs DC %d)", c.Name, e.Display(), roll.Total, dc))
		return nil
	}
	cond := newCondition(CondRestrained, dc, "grappled by "+c.Name, c.ID)
	e.Conditions.add(cond)
	st.Changes = append(st.Changes, fmt.Sprintf("%s grapples %s and holds it fast (save %d vs DC %d); it can't move and is easier to hit", c.Name, e.Display(), roll.Total, dc))
	return nil
}

// flip turns a defender's save tier into the attacker's result.
func flip(t dice.Tier) dice.Tier {
	switch t {
	case dice.CritSuccess:
		return dice.CritFailure
	case dice.Success:
		return dice.Failure
	case dice.Failure:
		return dice.Success
	case dice.CritFailure:
		return dice.CritSuccess
	}
	return t
}

// releaseGrapples frees enemies c is holding (c moved away or went down).
func releaseGrapples(w *World, c *Character) []string {
	var out []string
	for _, e := range w.encounterEnemies() {
		if r := e.Conditions.Get(CondRestrained); r != nil && r.SourceID == c.ID && !e.Defeated() {
			e.Conditions.remove(CondRestrained)
			out = append(out, fmt.Sprintf("%s lets go of %s", c.Name, e.Display()))
		}
	}
	return out
}

// canReact reports whether a character can make a reaction now.
func canReact(w *World, c *Character) bool {
	return !c.Down() && c.ReactedRound != round(w) && !c.Conditions.Has(CondStunned)
}

func enemyCanReact(w *World, e *Enemy) bool {
	return !e.Defeated() && e.ReactedRound != round(w) && !e.Conditions.Has(CondStunned)
}

// opportunityAttacks lets melee enemies in c's zone strike as c leaves it.
// It reports whether c went down.
func opportunityAttacks(ctx *Ctx, c *Character, st *Step) bool {
	w := ctx.W
	if !w.InCombat() || c.Conditions.Has(CondDisengaged) || c.Conditions.Has(CondHidden) {
		return false
	}
	for _, e := range w.Encounter.Living() {
		if e.ZoneID != c.ZoneID || e.Ranged || !enemyCanReact(w, e) {
			continue
		}
		e.ReactedRound = round(w)
		edge, why := AttackEdge(e.Conditions, c.Conditions, 0)
		out := AttackWithEdge(ctx.R, fmt.Sprintf("%s strikes at %s as they pull away", e.Display(), c.Name), e.AttackBonus, ArmorClass(w, c), e.Damage, edge, why)
		out.Roll.Parts = []RollPart{{Label: "Attack bonus", Value: e.AttackBonus}}
		r := checkRoll("attack", out.Roll)
		r.Foe, r.By = true, e.Display()
		st.Rolls = append(st.Rolls, r)
		if !out.Hit {
			st.Changes = append(st.Changes, fmt.Sprintf("%s takes a swing at %s as they move away and misses (opportunity attack)", e.Display(), c.Name))
			continue
		}
		dmg := out.Damage.Total
		c.HP = max(0, c.HP-dmg)
		line := fmt.Sprintf("%s catches %s with an opportunity attack for %d damage (%d/%d HP)", e.Display(), c.Name, dmg, c.HP, c.MaxHP)
		if c.HP == 0 {
			KnockOut(w, c)
			line += fmt.Sprintf("; %s goes down", c.Name)
			st.Changes = append(st.Changes, line)
			st.Changes = append(st.Changes, releaseGrapples(w, c)...)
			return true
		}
		st.Changes = append(st.Changes, line)
	}
	return false
}

// Reaction is a character's automatic attack on an enemy's turn: an
// opportunity attack as it leaves their zone, or a readied attack.
type Reaction struct {
	CharacterID string        `json:"character_id"`
	Character   string        `json:"character"`
	Kind        string        `json:"kind"` // opportunity | readied
	Attack      AttackOutcome `json:"attack"`
	Text        string        `json:"text"`
}

// reactionStrike is c's attack on e outside their own turn.
func reactionStrike(w *World, r *dice.Roller, c *Character, e *Enemy, kind string) Reaction {
	l := w.Here()
	c.ReactedRound = round(w)
	wpn := Weapon(w, c)
	d := ZoneDistance(l, c.ZoneID, e.ZoneID)
	toHit, _ := AttackMod(c, wpn)
	cover := CoverBonus(l, c.ZoneID, e.ZoneID)
	edge, why := AttackEdge(c.Conditions, e.Conditions, d)
	verb := "lashes out at"
	if kind == "readied" {
		verb = "unleashes a readied attack on"
	}
	out := AttackWithEdge(r, fmt.Sprintf("%s %s %s", c.Name, verb, e.Display()), toHit, e.AC+cover, wpn.Damage, edge, why)
	out.Roll.Parts, out.Roll.Cover = AttackParts(c, wpn), cover
	re := Reaction{CharacterID: c.ID, Character: c.Name, Kind: kind, Attack: out}
	c.Conditions.remove(CondHidden)
	c.Conditions.remove(CondHelped)
	if !out.Hit {
		re.Text = fmt.Sprintf("%s %s %s and misses", c.Name, verb, e.Display())
		return re
	}
	e.HP = max(0, e.HP-out.Damage.Total)
	re.Text = fmt.Sprintf("%s %s %s and hits for %d damage (%d/%d HP)", c.Name, verb, e.Display(), out.Damage.Total, e.HP, e.MaxHP)
	if e.HP == 0 {
		for _, ch := range defeat(w, e, false) {
			re.Text += "; " + ch
		}
	}
	return re
}

// enemyLeaves gives characters in e's zone an opportunity attack as it
// moves to another zone.
func enemyLeaves(w *World, r *dice.Roller, e *Enemy) []Reaction {
	var out []Reaction
	for _, c := range w.Party() {
		if e.Defeated() {
			break
		}
		if c.ZoneID != e.ZoneID || !canReact(w, c) || InferReach(Weapon(w, c)) != ReachMelee || c.Readied != nil {
			continue
		}
		out = append(out, reactionStrike(w, r, c, e, "opportunity"))
	}
	return out
}

// readiedAgainst fires readied attacks at e once it is within reach.
func readiedAgainst(w *World, r *dice.Roller, e *Enemy) []Reaction {
	var out []Reaction
	l := w.Here()
	for _, c := range w.Party() {
		if e.Defeated() {
			break
		}
		rd := c.Readied
		if rd == nil || !canReact(w, c) || (rd.Target != "" && rd.Target != e.ID) {
			continue
		}
		if !InReach(InferReach(Weapon(w, c)), ZoneDistance(l, c.ZoneID, e.ZoneID)) {
			continue
		}
		c.Readied = nil
		out = append(out, reactionStrike(w, r, c, e, "readied"))
	}
	return out
}
