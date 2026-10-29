package game

import (
	"fmt"
	"strings"

	"github.com/Solvely-Colin/aidnd/internal/dice"
)

// Dread is the horror layer of saga campaigns. The SRD has no sanity
// rules, so this is our own, built like conditions: engine state the model
// can never set directly.
//
// Strain runs 0..6. A failed Wisdom save against a horror (DC 10 + its
// rank) adds 1, or 2 on a natural 1. At 2 and above the character has
// disadvantage on saves against being frightened. Reaching 4 brings a
// short episode (frightened for two turns). Reaching 6 leaves a lasting
// scar and drops strain back to 3. A long rest at a safe place takes 1 off.

// Strain limits.
const (
	MaxStrain     = 6
	StrainShaken  = 2 // disadvantage on saves against fear
	StrainEpisode = 4 // a short episode
	StrainAfter   = 3 // strain left after a scar
)

// Scar is a lasting quirk left by reaching full strain.
type Scar struct {
	Key  string
	Name string
	Text string
}

// Scars are dealt from a d8; a character never gets one twice. "whispers"
// has a rule effect (passive Perception -2); the others are played by the
// narrator, who sees every character's scars.
var Scars = []Scar{
	{"dark", "Afraid of the dark", "Won't sleep without a light; darkness sets them on edge."},
	{"water", "Dread of deep water", "Deep or black water makes them freeze."},
	{"whispers", "Hears whispers", "The whispers never stop: passive Perception is 2 lower."},
	{"insomnia", "Sleepless", "Wakes every night at the same hour, sure something called them."},
	{"tremor", "Shaking hands", "Their hands shake whenever they remember."},
	{"paranoid", "Trusts no one", "Every friendly face might be a mask."},
	{"compulsion", "Counts everything", "Counts doors, steps and windows, and can't stop."},
	{"marked", "Dreams of the sea", "The sea knows their name. Something follows their dreams."},
}

// ScarByKey looks up a scar.
func ScarByKey(key string) (Scar, bool) {
	for _, s := range Scars {
		if s.Key == key {
			return s, true
		}
	}
	return Scar{}, false
}

// StrainOutcome is the result of a horror save.
type StrainOutcome struct {
	Roll    Roll
	Gained  int
	Lines   []string
	Episode bool   // went to frightened from reaching StrainEpisode
	Scar    string // scar key dealt, if any
}

// HorrorDC is the save DC against a horror of a rank (1..3).
func HorrorDC(rank int) int { return 10 + clamp(rank, 1, 3) }

// FaceHorror rolls a character's Wisdom save against a horror of rank
// 1..3 and applies the strain. Only dread campaigns call it.
func FaceHorror(r *dice.Roller, c *Character, rank int, cause string) *StrainOutcome {
	if c.Down() {
		return nil
	}
	rank = clamp(rank, 1, 3)
	dc := HorrorDC(rank)
	roll := CharacterSave(r, c, WIS, dc, "horror")
	out := &StrainOutcome{Roll: roll}
	if cause = strings.TrimSpace(cause); cause == "" {
		cause = "a horror"
	}
	if roll.Tier.Succeeded() {
		out.Lines = []string{fmt.Sprintf("%s keeps their nerve before %s (WIS save %d vs DC %d)", c.Name, cause, roll.Total, dc)}
		return out
	}
	gain := 1
	if roll.Natural == 1 {
		gain = 2
	}
	out.Gained = gain
	out.Lines = []string{fmt.Sprintf("%s is shaken by %s (WIS save %d vs DC %d): strain %d of %d", c.Name, cause, roll.Total, dc, min(c.Strain+gain, MaxStrain), MaxStrain)}
	out.Lines = append(out.Lines, addStrain(r, c, gain, cause, out)...)
	return out
}

// addStrain raises strain, dealing the episode and the scar when their
// thresholds are crossed.
func addStrain(r *dice.Roller, c *Character, n int, cause string, out *StrainOutcome) []string {
	before := c.Strain
	c.Strain = clamp(c.Strain+n, 0, MaxStrain)
	var lines []string
	if before < StrainEpisode && c.Strain >= StrainEpisode && c.Strain < MaxStrain && !c.Conditions.Has(CondFrightened) {
		c.Conditions.add(newCondition(CondFrightened, HorrorDC(1), cause, ""))
		out.Episode = true
		lines = append(lines, c.Name+" breaks down for a moment and is frightened")
	}
	if c.Strain >= MaxStrain {
		if s, ok := dealScar(r, c); ok {
			out.Scar = s.Key
			lines = append(lines, fmt.Sprintf("%s is scarred: %s. %s", c.Name, s.Name, s.Text))
		}
		c.Strain = StrainAfter
	}
	return lines
}

// dealScar gives the character a scar they don't have yet.
func dealScar(r *dice.Roller, c *Character) (Scar, bool) {
	start := r.Intn(len(Scars))
	for i := range Scars {
		s := Scars[(start+i)%len(Scars)]
		if !c.HasScar(s.Key) {
			c.Scars = append(c.Scars, s.Key)
			return s, true
		}
	}
	return Scar{}, false
}

// HasScar reports whether the character carries a scar.
func (c *Character) HasScar(key string) bool {
	for _, s := range c.Scars {
		if s == key {
			return true
		}
	}
	return false
}

// RecoverStrain lowers strain by n (a long rest at a safe place is 1).
func RecoverStrain(c *Character, n int) {
	c.Strain = max(0, c.Strain-n)
}

// FaceEnemyHorrors makes each conscious character who has not yet faced a
// horror enemy's kind save against it, once per kind per character. It
// runs at the start of the enemies' turns, so the party sees the thing
// before it acts.
func FaceEnemyHorrors(w *World, r *dice.Roller) []string {
	if !w.Campaign.Dread || !w.InCombat() {
		return nil
	}
	var lines []string
	for _, e := range w.Encounter.Enemies {
		if e.Horror <= 0 || e.Faced || e.Defeated() {
			continue
		}
		e.Faced = true
		kind := strings.ToLower(baseName(e.Name))
		for _, c := range w.Party() {
			if c.Down() || containsStr(c.Faced, kind) {
				continue
			}
			c.Faced = append(c.Faced, kind)
			if so := FaceHorror(r, c, e.Horror, "the sight of "+e.Name); so != nil {
				lines = append(lines, so.Lines...)
			}
		}
	}
	return lines
}
