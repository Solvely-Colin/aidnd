package game

import (
	"fmt"
	"regexp"
	"strings"
)

// Tending wounds: a check (Medicine by default, DC 10) outside a fight that
// cleans and binds a party member's injuries. Success heals as if they
// spent a hit die: one roll of their class hit die plus their CON modifier
// (a critical success rolls two). It works once per patient until
// they next rest, so it can't be repeated for free HP; a failed attempt can
// be tried again.

// tendedKey marks a character as already tended since their last rest. It
// lives in Character.Used, which a long rest clears; ShortRest clears it too.
const tendedKey = "tended"

var tendRe = regexp.MustCompile(`(?i)(wound|injur|bandag|first aid|splint|sutur|stitch (up|my|his|her|their)|bleeding|\b(tend|treat|heal|patch|fix|bind|doctor)( up)? (myself|himself|herself|themselves|me|him|her|them|ally|friend|companion)\b)`)

// tendWords reports whether a check's text is about treating wounds.
func tendWords(text string) bool {
	t := strings.ToLower(text)
	if strings.Contains(t, "stabiliz") || strings.Contains(t, "dying") {
		return false
	}
	return tendRe.MatchString(t)
}

// tendPatient finds who c is tending: the named party member, else c if
// hurt, else the most hurt companion nearby.
func tendPatient(w *World, c *Character, target string) (*Character, error) {
	if w.InCombat() {
		return nil, fmt.Errorf("there is no time to bind wounds in the middle of a fight; stabilize a dying ally or use a potion or healer's kit instead")
	}
	var t *Character
	if target != "" {
		t = w.Character(target)
		if t == nil {
			for _, ch := range w.Party() {
				if NameMatch(ch.Name, target) {
					t = ch
					break
				}
			}
		}
	}
	if t == nil && c.HP < c.MaxHP {
		t = c
	}
	if t == nil {
		for _, ch := range w.Party() {
			if ch.Dead || ch.HP >= ch.MaxHP {
				continue
			}
			if t == nil || ch.MaxHP-ch.HP > t.MaxHP-t.HP {
				t = ch
			}
		}
	}
	if t == nil {
		t = c
	}
	if t.Dead {
		return nil, fmt.Errorf("%s is beyond help", t.Name)
	}
	if t.Dying() {
		return t, nil
	}
	if t.HP >= t.MaxHP {
		return nil, fmt.Errorf("%s has no wounds that need tending", t.Name)
	}
	if t.Used[tendedKey] > 0 {
		return nil, fmt.Errorf("%s's wounds are already bound; more care won't help until after a rest", t.Name)
	}
	return t, nil
}

// tendWounds heals t after c's successful check.
func tendWounds(ctx *Ctx, c, t *Character, crit bool, st *Step) {
	die := max(4, ClassOf(t).HitDie)
	dice := 1
	if crit {
		dice = 2 // a critical success: the care of a real physician
	}
	expr := fmt.Sprintf("%dd%d", dice, die)
	if m := t.Mod(CON); m != 0 {
		expr += fmt.Sprintf("%+d", m)
	}
	amt := 1
	if res, err := ctx.R.RollString(expr); err == nil {
		amt = max(1, res.Total)
		st.Rolls = append(st.Rolls, dmgRoll(fmt.Sprintf("Tending %s's wounds", t.Name), "heal", res))
	}
	got, _ := Heal(t, amt)
	if t.Used == nil {
		t.Used = map[string]int{}
	}
	t.Used[tendedKey] = 1
	who := t.Name + "'s"
	if t == c {
		who = "their own"
	}
	st.Changes = append(st.Changes, fmt.Sprintf("%s tends %s wounds: %s regains %d HP (%d/%d)", c.Name, who, t.Name, got, t.HP, t.MaxHP))
}
