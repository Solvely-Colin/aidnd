package game

import (
	"fmt"
	"strings"

	"github.com/Solvely-Colin/aidnd/internal/dice"
)

// Downtime: in a long campaign, a character resting at a safe place can
// queue one thing to do before the world's next turn. It resolves on the
// world tick, with engine dice.

// Downtime orders.
const (
	DowntimeResearch = "research" // Investigation at a library or archive: a lore entry
	DowntimeWork     = "work"     // best ability: money, scaled by level
	DowntimeRest     = "rest"     // strain down, lingering conditions gone
	DowntimeContacts = "contacts" // Persuasion: a rumor of what a faction is about
	DowntimeCarouse  = "carouse"  // Charisma: rumors, and sometimes trouble
)

// DowntimeKinds lists the orders with what each does, for the app.
var DowntimeKinds = []struct{ Key, Name, Desc string }{
	{DowntimeResearch, "Research a lead", "Dig through books and records (Investigation). Turns up something the region keeps quiet."},
	{DowntimeWork, "Work a job", "Earn money with your best ability."},
	{DowntimeRest, "Rest and recover", "Sleep it off: strain eases and lingering conditions fade."},
	{DowntimeContacts, "Work your contacts", "Ask around (Persuasion). Learn what one of the factions here is up to."},
	{DowntimeCarouse, "Carouse", "A night out (Charisma). Rumors, and sometimes trouble."},
}

// DowntimeDC is the DC of downtime checks.
const DowntimeDC = 12

// Downtime is a queued order.
type Downtime struct {
	Kind string `json:"kind"`
	Week int    `json:"week"` // the world week it was queued in
}

// DowntimeResult is a resolved order.
type DowntimeResult struct {
	CharacterID string     `json:"character_id"`
	Name        string     `json:"name"`
	Kind        string     `json:"kind"`
	Roll        *CheckRoll `json:"roll,omitempty"`
	Text        string     `json:"text"`
}

// CanDowntime says whether c can queue an order where it stands.
func CanDowntime(w *World, c *Character, loc *Location) error {
	if w.Campaign.Living == nil {
		return fmt.Errorf("downtime is for long campaigns")
	}
	if c.Dead || c.Down() {
		return fmt.Errorf("%s can't do anything right now", c.Name)
	}
	if loc == nil || (!loc.IsHub && loc.Danger > 0) {
		return fmt.Errorf("downtime needs a safe place, like a hub")
	}
	return nil
}

// QueueDowntime sets c's order for the next world tick.
func QueueDowntime(w *World, c *Character, kind string) error {
	if kind == "" {
		c.Downtime = nil
		return nil
	}
	ok := false
	for _, k := range DowntimeKinds {
		ok = ok || k.Key == kind
	}
	if !ok {
		return fmt.Errorf("unknown downtime %q", kind)
	}
	c.Downtime = &Downtime{Kind: kind, Week: w.Campaign.Living.Week}
	return nil
}

func downtimeCheck(r *dice.Roller, c *Character, skill string, ab Ability) CheckRoll {
	if sk, ok := SkillByKey(skill); ok {
		return CheckRoll{Label: fmt.Sprintf("%s %s check", c.Name, sk.Name), Ability: sk.Ability, Parts: SkillParts(c, sk), Against: "DC",
			Check: r.D20(c.SkillMod(sk), DowntimeDC)}
	}
	return AbilityCheck(r, c, ab, DowntimeDC)
}

// ResolveDowntime runs every queued order. The world must be settled.
func ResolveDowntime(w *World, r *dice.Roller) []DowntimeResult {
	var out []DowntimeResult
	for _, c := range w.Characters {
		d := c.Downtime
		if d == nil || c.Dead {
			continue
		}
		c.Downtime = nil
		res := DowntimeResult{CharacterID: c.ID, Name: c.Name, Kind: d.Kind}
		switch d.Kind {
		case DowntimeRest:
			c.Strain = max(0, c.Strain-2)
			for _, k := range []string{CondPoisoned, CondFrightened} {
				c.Conditions.remove(k)
			}
			res.Text = fmt.Sprintf("%s rested; the worst of it fades.", c.Name)
		case DowntimeWork:
			best := STR
			for _, a := range []Ability{DEX, CON, INT, WIS, CHA} {
				if c.CheckMod(a) > c.CheckMod(best) {
					best = a
				}
			}
			roll := AbilityCheck(r, c, best, DowntimeDC)
			res.Roll = &roll
			pay := 2 * c.Level
			if roll.Tier.Succeeded() {
				pay = 6 * c.Level
			}
			c.Gold += pay
			res.Text = fmt.Sprintf("%s worked a job and earned %d gold.", c.Name, pay)
		case DowntimeResearch:
			roll := downtimeCheck(r, c, "investigation", INT)
			res.Roll = &roll
			if roll.Tier.Succeeded() {
				if l := researchable(w, c); l != nil {
					l.Researched = true
					w.Record(ChronicleEntry{Kind: FactNote, Source: SourceEngine, Text: fmt.Sprintf("%s's research: %s. %s", c.Name, l.Title, l.Text)})
					res.Text = fmt.Sprintf("%s's research turned up %s.", c.Name, l.Title)
					break
				}
				res.Text = fmt.Sprintf("%s read everything the shelves here had to give; nothing new.", c.Name)
				break
			}
			res.Text = fmt.Sprintf("%s's research went nowhere this week.", c.Name)
		case DowntimeContacts, DowntimeCarouse:
			skill, ab := "persuasion", CHA
			if d.Kind == DowntimeCarouse {
				skill = ""
			}
			roll := downtimeCheck(r, c, skill, ab)
			res.Roll = &roll
			if !roll.Tier.Succeeded() {
				res.Text = fmt.Sprintf("%s heard nothing worth repeating.", c.Name)
				if d.Kind == DowntimeCarouse && roll.Tier == dice.CritFailure {
					lost := min(c.Gold, r.Die(6)*c.Level)
					c.Gold -= lost
					res.Text = fmt.Sprintf("%s's night out went wrong: %d gold gone and a sore head.", c.Name, lost)
				}
				break
			}
			if rumor := factionRumor(w, r); rumor != "" {
				w.Record(ChronicleEntry{Kind: FactRumor, Source: SourceEngine, Text: rumor})
				res.Text = fmt.Sprintf("%s heard something: %s", c.Name, rumor)
				break
			}
			res.Text = fmt.Sprintf("%s made friends, but heard nothing new.", c.Name)
		}
		out = append(out, res)
	}
	return out
}

// researchable is a lore entry about the character's region that is
// open to the narrator but not yet researched, never a gated secret.
func researchable(w *World, c *Character) *Lore {
	region := ""
	if l := w.Location(w.characterLocation(c)); l != nil {
		region = l.Region
	}
	for _, lo := range w.Lore {
		if lo.Researched || !LoreOpen(w, lo) || lo.AlwaysOn {
			continue
		}
		if region == "" || loreInRegion(w, lo, region) {
			return lo
		}
	}
	return nil
}

func loreInRegion(w *World, lo *Lore, region string) bool {
	for _, id := range lo.EntityIDs {
		if l := w.Location(id); l != nil && l.Region == region {
			return true
		}
		if n := w.NPC(id); n != nil {
			if l := w.Location(n.LocationID); l != nil && l.Region == region {
				return true
			}
		}
	}
	return false
}

// characterLocation is where c's group stands.
func (w *World) characterLocation(c *Character) string {
	return w.groupLocation(GroupOf(c))
}

// factionRumor hints at a running faction's next move.
func factionRumor(w *World, r *dice.Roller) string {
	var live []*Faction
	for _, f := range w.Campaign.Living.Factions {
		if f.active(w) && len(f.Moves) > 0 {
			live = append(live, f)
		}
	}
	if len(live) == 0 {
		return ""
	}
	f := live[r.Intn(len(live))]
	next := strings.TrimSuffix(f.Moves[f.Progress%len(f.Moves)], ".")
	return fmt.Sprintf("people say the %s will move soon: %s", f.Name, strings.ToLower(next[:1])+next[1:])
}
