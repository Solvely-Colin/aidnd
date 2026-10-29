package gm

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/store"
)

// Hints are optional map selections sent with an action.
type Hints struct {
	TargetID string `json:"target_id,omitempty"`
	ZoneID   string `json:"zone_id,omitempty"`
}

func itemLine(it *game.Item, c *game.Character) string {
	s := fmt.Sprintf("%s [%s]", it.Name, it.ID)
	var tags []string
	if it.Kind != "" {
		tags = append(tags, it.Kind)
	}
	if it.Kind == game.ItemWeapon {
		tags = append(tags, it.Damage, game.ReachName(game.InferReach(it)))
	}
	if c != nil && (c.Weapon == it.ID || c.Armor == it.ID || (it.Kind == game.ItemShield && it.Owner.ID == c.ID)) {
		tags = append(tags, "equipped")
	}
	if it.Effect != "" {
		tags = append(tags, it.Effect)
	}
	return s + " (" + strings.Join(tags, ", ") + ")"
}

// sheetLine summarises a character.
func sheetLine(w *game.World, c *game.Character) string {
	st := ""
	if c.Down() {
		st = " DOWN"
	}
	return fmt.Sprintf("%s [%s] (%s) %s L%d, HP %d/%d%s, AC %d, gold %d", c.Name, c.ID, game.NormPronouns(c.Pronouns), c.Class, c.Level, c.HP, c.MaxHP, st, game.ArmorClass(w, c), c.Gold)
}

// snapshot is the compact state block shared by interpreter and narrator.
// secrets adds GM-only facts (hidden exits, NPC memory) for the narrator.
func snapshot(w *game.World, actor *game.Character, secrets bool) string {
	var b strings.Builder
	l := w.Here()
	line := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	sc := w.Campaign.Scene
	line("SETTING: %s", settingLine(w.Setting()))
	line("TIME: day %d, %s. Tension %d/10.", sc.Day, sc.TimeName(), sc.Tension)
	line("LOCATION: %s [%s] (%s, danger %d). %s", l.Name, l.ID, l.Kind, l.Danger, l.Description)
	if actor != nil {
		line("ACTOR: %s. Stands in %s [%s].", sheetLine(w, actor), game.ZoneName(l, actor.ZoneID), actor.ZoneID)
		var abil []string
		for _, a := range game.Abilities {
			m := actor.CheckMod(a)
			abil = append(abil, fmt.Sprintf("%s %+d", a, m))
		}
		line("CHECK MODIFIERS: %s (proficient: %v)", strings.Join(abil, ", "), actor.Proficient)
		wpn := game.Weapon(w, actor)
		line("WEAPON: %s, %s, %s", wpn.Name, wpn.Damage, game.ReachName(game.InferReach(wpn)))
		var inv []string
		for _, it := range w.ItemsOf(game.OwnerCharacter, actor.ID) {
			inv = append(inv, itemLine(it, actor))
		}
		line("INVENTORY: %s", orNone(inv))
	}
	if len(l.Zones) > 0 {
		var zs []string
		for _, z := range l.Zones {
			who := []string{}
			for _, c := range w.Party() {
				if c.ZoneID == z.ID {
					who = append(who, c.Name)
				}
			}
			for _, n := range w.NPCsAt(l.ID) {
				if n.ZoneID == z.ID {
					who = append(who, n.Name+" (npc)")
				}
			}
			if w.InCombat() {
				for _, e := range w.Encounter.Living() {
					if e.ZoneID == z.ID {
						who = append(who, e.Display()+" (enemy)")
					}
				}
			}
			dist := ""
			if actor != nil {
				dist = fmt.Sprintf(", %d from %s", game.ZoneDistance(l, actor.ZoneID, z.ID), actor.Name)
			}
			cover := ""
			if z.Cover {
				cover = ", cover"
			}
			zs = append(zs, fmt.Sprintf("%s [%s]%s%s: %s", z.Name, z.ID, dist, cover, orNone(who)))
		}
		line("ZONES:\n  %s", strings.Join(zs, "\n  "))
		if actor != nil {
			line("SCENE: %s", sceneLine(w, l, actor))
		}
	}
	var exits []string
	for _, x := range w.Exits(l.ID, false) {
		s := fmt.Sprintf("[%s] %s -> %s (%s", x.Edge.ID, x.Label, x.To.Name, x.Edge.State)
		if x.Zone != "" {
			s += ", leaves from " + game.ZoneName(l, x.Zone)
		}
		if x.To.Stub {
			s += ", unexplored"
		}
		exits = append(exits, s+")")
	}
	line("EXITS: %s", orNone(exits))
	var npcs []string
	for _, n := range w.NPCsAt(l.ID) {
		s := fmt.Sprintf("%s [%s] %s, attitude %+d, in %s", n.Name, n.ID, n.Role, n.Disposition, game.ZoneName(l, n.ZoneID))
		if actor != nil && !withActor(l, actor, n) {
			s += " (elsewhere, not in the scene)"
		}
		if n.Merchant {
			var stock []string
			for _, it := range w.ItemsOf(game.OwnerNPC, n.ID) {
				if !isRewardItem(w, it.ID) && it.Kind != game.ItemQuest {
					stock = append(stock, fmt.Sprintf("%s [%s] %dg", it.Name, it.ID, game.BuyPrice(it, n)))
				}
			}
			s += "; STOCK: " + orNone(stock)
		}
		var holds []string
		for _, it := range w.ItemsOf(game.OwnerNPC, n.ID) {
			if it.Kind == game.ItemQuest && !isRewardItem(w, it.ID) {
				holds = append(holds, it.Name)
			}
		}
		if len(holds) > 0 {
			s += "; HOLDS (gives it when talked to): " + strings.Join(holds, ", ")
		}
		if secrets && n.Desc != "" && (actor == nil || withActor(l, actor, n)) {
			s += "; looks: " + trim(n.Desc, 200)
		}
		if secrets && len(n.Memory) > 0 {
			s += "; remembers: " + strings.Join(n.Memory, " | ")
		}
		npcs = append(npcs, s)
	}
	line("NPCS: %s", orNone(npcs))
	var ground []string
	for _, it := range w.ItemsOf(game.OwnerLocation, l.ID) {
		ground = append(ground, itemLine(it, nil))
	}
	line("GROUND: %s", orNone(ground))
	if w.InCombat() {
		var es []string
		for _, e := range w.Encounter.Living() {
			d := ""
			if actor != nil {
				d = fmt.Sprintf(", %d zones from %s", game.ZoneDistance(l, actor.ZoneID, e.ZoneID), actor.Name)
			}
			kind := "melee"
			if e.Ranged {
				kind = "ranged"
			}
			es = append(es, fmt.Sprintf("%s [%s] HP %d/%d, AC %d, %s, in %s%s", e.Display(), e.ID, e.HP, e.MaxHP, e.AC, kind, game.ZoneName(l, e.ZoneID), d))
		}
		line("IN COMBAT. ENEMIES: %s", strings.Join(es, "; "))
	} else {
		line("NOT IN COMBAT.")
	}
	var party []string
	for _, c := range w.Party() {
		if actor == nil || c.ID != actor.ID {
			party = append(party, fmt.Sprintf("%s, in %s", sheetLine(w, c), game.ZoneName(l, c.ZoneID)))
		}
	}
	line("PARTY: %s", orNone(party))
	var offered, active []string
	for _, q := range w.Quests {
		switch q.Status {
		case game.QuestOffered:
			if g := w.NPC(q.GiverID); g != nil && g.LocationID == l.ID {
				offered = append(offered, fmt.Sprintf("[%s] %s (from %s): %s", q.ID, q.Title, g.Name, q.Summary))
			}
		case game.QuestActive, game.QuestCompleted:
			var objs []string
			for _, o := range q.Objectives {
				mark := " "
				if o.Done {
					mark = "x"
				}
				objs = append(objs, fmt.Sprintf("[%s] %s", mark, o.Text))
			}
			st := ""
			if q.Status == game.QuestCompleted {
				st = " READY TO TURN IN"
				if g := w.NPC(q.GiverID); g != nil {
					st += " to " + g.Name
				}
			}
			active = append(active, fmt.Sprintf("[%s] %s%s: %s", q.ID, q.Title, st, strings.Join(objs, "; ")))
		}
	}
	line("QUESTS ON OFFER HERE: %s", orNone(offered))
	line("ACTIVE QUESTS: %s", orNone(active))
	var known []string
	for _, loc := range w.Locations {
		if loc.Visited && loc.ID != l.ID {
			known = append(known, fmt.Sprintf("%s [%s]", loc.Name, loc.ID))
		}
	}
	line("KNOWN PLACES (travel): %s", orNone(known))
	if secrets {
		var hidden []string
		for _, x := range w.Exits(l.ID, true) {
			if x.Edge.State == game.EdgeHidden {
				hidden = append(hidden, fmt.Sprintf("[%s] %s -> %s", x.Edge.ID, x.Label, x.To.Name))
			}
		}
		line("SECRETS (GM only): hidden exits: %s", orNone(hidden))
	}
	return b.String()
}

// withActor reports whether an NPC shares the actor's zone. NPCs with no
// zone, or locations without zones, count as present.
func withActor(l *game.Location, actor *game.Character, n *game.NPC) bool {
	return len(l.Zones) == 0 || n.ZoneID == "" || l.Zone(n.ZoneID) == nil || n.ZoneID == actor.ZoneID
}

// sceneLine names who is with the actor, so the narrator keeps the scene
// in the actor's zone instead of wherever the story wants to go.
func sceneLine(w *game.World, l *game.Location, actor *game.Character) string {
	var with []string
	for _, c := range w.Party() {
		if c.ID != actor.ID && c.ZoneID == actor.ZoneID {
			with = append(with, c.Name)
		}
	}
	for _, n := range w.NPCsAt(l.ID) {
		if withActor(l, actor, n) {
			with = append(with, n.Name)
		}
	}
	return fmt.Sprintf("%s stands in %s with: %s. Anyone in another zone is out of the scene: they can be seen from a distance or mentioned, but they do not speak, act or show anything to %s.",
		actor.Name, game.ZoneName(l, actor.ZoneID), orNone(with), actor.Name)
}

func orNone(s []string) string {
	if len(s) == 0 {
		return "none"
	}
	return strings.Join(s, "; ")
}

func isRewardItem(w *game.World, id string) bool {
	for _, q := range w.Quests {
		if q.Status == game.QuestTurnedIn {
			continue
		}
		for _, r := range q.RewardItems {
			if r == id {
				return true
			}
		}
	}
	return false
}

// describeResolution renders what the engine decided, for the narrator.
func describeResolution(res *game.Resolution, w *game.World) string {
	var b strings.Builder
	actor := w.Character(res.ActorID)
	for i, s := range res.Steps {
		fmt.Fprintf(&b, "Step %d: %s", i+1, s.Action.Kind)
		if s.Action.Text != "" {
			fmt.Fprintf(&b, " (%q)", s.Action.Text)
		}
		b.WriteString("\n")
		if s.Error != "" {
			fmt.Fprintf(&b, "  FAILED, nothing happened: %s\n", s.Error)
			continue
		}
		for _, r := range s.Rolls {
			fmt.Fprintf(&b, "  ROLL %s: %s\n", r.Label, rollText(r))
		}
		for _, c := range s.Changes {
			fmt.Fprintf(&b, "  CHANGE: %s\n", c)
		}
		for _, in := range s.Info {
			fmt.Fprintf(&b, "  FACT: %s\n", in)
		}
	}
	for _, s := range res.Skipped {
		fmt.Fprintf(&b, "SKIPPED, did not happen, do not narrate it (%s %s): %s\n", s.Action.Kind, s.Action.Target, s.Error)
	}
	if actor != nil && !res.TurnEnded {
		fmt.Fprintf(&b, "%s's turn continues (%s left).\n", actor.Name, res.Budget.Left())
	}
	return b.String()
}

func rollText(r game.Roll) string {
	switch r.Kind {
	case "damage", "heal":
		return fmt.Sprintf("%s = %d", r.Expr, r.Total)
	case "encounter":
		return fmt.Sprintf("d100 %d (encounter on %d or less)", r.Total, r.Target)
	}
	return fmt.Sprintf("d20 %d %+d = %d vs %d, %s", r.Natural, r.Mod, r.Total, r.Target, strings.ReplaceAll(string(r.Tier), "_", " "))
}

// eventLines turns recent log events into short text for LLM context.
func eventLines(evs []store.Event, w *game.World) []string {
	var out []string
	for _, e := range evs {
		var d map[string]any
		_ = json.Unmarshal(e.Data, &d)
		switch e.Type {
		case "player_action":
			out = append(out, fmt.Sprintf("%v: %v", d["character"], d["text"]))
		case "narration":
			out = append(out, fmt.Sprintf("GM: %v", d["text"]))
		case "enemy_turns":
			out = append(out, fmt.Sprintf("GM: %v", d["narration"]))
		case "location_changed":
			out = append(out, fmt.Sprintf("(The party arrived at %v.)", d["name"]))
		case "quest_updated":
			out = append(out, fmt.Sprintf("(%v)", d["text"]))
		}
	}
	return out
}
