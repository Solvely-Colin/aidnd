package game

import (
	"fmt"
	"regexp"
	"strings"
)

// Checks that act on people and the scene. Before these, a successful
// Persuasion, Intimidation, Deception, Sleight of Hand or out-of-fight
// Stealth check was a generic check: the story said the guard was won over
// or the key was lifted, but nothing in the world changed. Each purpose
// here has a real result:
//
//   persuade / deceive  the NPC warms to the party (+1 attitude, at most +2
//                       from talk alone) and hands over what a quest needs
//   intimidate          the NPC is cowed into handing over what a quest needs
//                       now; a failed threat leaves a grudge
//   steal               an item the NPC carries moves to the character;
//                       caught, the NPC cools and holds a grudge
//   sneak               the character is hidden until their next message
//                       ends: a fight that breaks out finds them unseen, and
//                       an ambush can't take the party by surprise
//
// Each character gets one try of each social purpose per NPC per day, so a
// failed roll can't be rerolled until it lands.

const (
	PurposePersuade   = "persuade"
	PurposeDeceive    = "deceive"
	PurposeIntimidate = "intimidate"
	PurposeSteal      = "steal"
	PurposeSneak      = "sneak"
)

// MaxTalkedAttitude is the best attitude talk alone can win; devotion
// comes from deeds (kept promises, finished quests).
const MaxTalkedAttitude = 2

var (
	stealRe  = regexp.MustCompile(`(?i)\b(steal|stealing|steals|pickpocket\w*|pick (his|her|their|the) pocket|lift|lifts|lifting|pinch|filch\w*|swipe|swipes|snatch\w*|palm|palms|purloin\w*|nick)\b`)
	searchRe = regexp.MustCompile(`(?i)\b(search\w*|clues?|look(s|ing)? for|hunt(s|ing)? for|find|finds|finding|hidden|secret|examin\w*|inspect\w*|scour\w*|rummag\w*|comb(s|ing)? (through|the)|investigat\w*)\b`)
	sneakRe  = regexp.MustCompile(`(?i)\b(sneak\w*|creep\w*|slip(s|ping)? (past|by|in|through|away)|tiptoe\w*|unseen|unnoticed|quietly|stealth\w*|shadow(s|ing)? (him|her|them)|move silently)\b`)
	// titles skipped when matching an NPC's name in free text
	nameTitles = map[string]bool{"mr": true, "mrs": true, "miss": true, "ms": true, "dr": true, "doctor": true, "old": true, "the": true,
		"sir": true, "lady": true, "lord": true, "father": true, "mother": true, "sister": true, "brother": true, "captain": true, "professor": true, "reverend": true}
)

// inferPurpose gives a check sent as "other" the purpose its skill and
// words mean, so the result lands in the world. It returns "other" when
// nothing fits.
func inferPurpose(w *World, c *Character, a Action) string {
	sk := strings.ToLower(a.Skill)
	text := a.Text
	switch sk {
	case "persuasion", "deception", "intimidation":
		if socialNPC(w, a) == nil {
			return "other" // talking a crowd round, a performance: story only
		}
		return map[string]string{"persuasion": PurposePersuade, "deception": PurposeDeceive, "intimidation": PurposeIntimidate}[sk]
	case "sleight_of_hand":
		if stealRe.MatchString(text) && socialNPC(w, a) != nil {
			return PurposeSteal
		}
	case "stealth":
		return PurposeSneak
	case "perception", "investigation", "":
		if searchRe.MatchString(text) {
			return "search"
		}
	}
	if sk == "" && sneakRe.MatchString(text) {
		return PurposeSneak
	}
	return "other"
}

// socialNPC is the living NPC here a check is aimed at: its target, or the
// one whose name its text mentions.
func socialNPC(w *World, a Action) *NPC {
	here := w.Campaign.LocationID
	if a.Target != "" {
		if n := w.NPC(a.Target); n != nil && n.Alive && n.LocationID == here {
			return n
		}
	}
	text := " " + normName(a.Text) + " "
	for _, n := range w.NPCsAt(here) {
		if NameContains(a.Text, n.Name) {
			return n
		}
		for _, part := range strings.Fields(normName(n.Name)) {
			if len(part) >= 3 && !nameTitles[part] && strings.Contains(text, " "+part+" ") {
				return n
			}
		}
	}
	return nil
}

// socialTarget resolves the NPC for an explicit social purpose. With no
// name given and one person here, it is them.
func socialTarget(w *World, a Action) (*NPC, error) {
	if n := socialNPC(w, a); n != nil {
		return n, nil
	}
	if a.Target == "" {
		if ns := w.NPCsAt(w.Campaign.LocationID); len(ns) == 1 {
			return ns[0], nil
		}
		return nil, fmt.Errorf("who is the check aimed at? name someone here")
	}
	return nil, fmt.Errorf("nobody called %q is here", a.Target)
}

// npcFoe is the living enemy an NPC has become in the current fight.
func npcFoe(w *World, n *NPC) *Enemy {
	if w.Encounter == nil {
		return nil
	}
	for _, e := range w.Encounter.Living() {
		if e.NPCID == n.ID {
			return e
		}
	}
	return nil
}

// swayKey is the per-day guard against rerolling a social check.
func swayKey(c *Character, purpose string) string { return c.ID + ":" + purpose }

// triedToday reports whether c already tried this purpose on n today, and
// marks the try when not.
func triedToday(w *World, n *NPC, c *Character, purpose string) bool {
	day := w.Campaign.Scene.Day
	k := swayKey(c, purpose)
	if d, ok := n.Swayed[k]; ok && d == day {
		return true
	}
	if n.Swayed == nil {
		n.Swayed = map[string]int{}
	}
	n.Swayed[k] = day
	return false
}

// socialDC is the DC to sway n: friends are easy, the wary harder, the
// hostile hard. A threat is as hard for everyone except someone who already
// hates the party.
func socialDC(n *NPC, c *Character, purpose string) int {
	att := n.AttitudeTo(c)
	if purpose == PurposeIntimidate {
		if n.Antagonist {
			return 20
		}
		return 15
	}
	switch {
	case n.Antagonist || att <= -2:
		return 20
	case att >= 1:
		return 10
	}
	return 15
}

// swayResult applies a social check's outcome to n.
func swayResult(w *World, n *NPC, c *Character, purpose string, ok, crit, critFail bool, st *Step) {
	switch purpose {
	case PurposePersuade, PurposeDeceive:
		verb := "won over"
		if purpose == PurposeDeceive {
			verb = "taken in"
		}
		if ok {
			if n.Antagonist {
				st.Changes = append(st.Changes, fmt.Sprintf("%s listens, but serves something that words will not move", n.Name))
				return
			}
			if n.Disposition < MaxTalkedAttitude {
				n.Disposition++
				st.Changes = append(st.Changes, fmt.Sprintf("%s is %s by %s (attitude now %+d)", n.Name, verb, c.Name, n.AttitudeTo(c)))
			} else {
				st.Changes = append(st.Changes, fmt.Sprintf("%s is %s by %s", n.Name, verb, c.Name))
			}
			n.Remember(fmt.Sprintf("%s %s me", c.Name, map[string]string{PurposePersuade: "talked me round", PurposeDeceive: "told me a story I believed"}[purpose]))
			handOver(w, n, c, false, st)
			return
		}
		if purpose == PurposeDeceive || critFail {
			n.Disposition = clamp(n.Disposition-1, -3, 3)
			why := "pushed too hard"
			if purpose == PurposeDeceive {
				why = "caught lying"
				n.Bind(w, c, TieGrudge, "lied to them", SourceEngine)
			}
			st.Changes = append(st.Changes, fmt.Sprintf("%s is not fooled: %s %s (attitude now %+d)", n.Name, c.Name, why, n.AttitudeTo(c)))
			n.Remember(fmt.Sprintf("%s %s", c.Name, why))
			return
		}
		st.Changes = append(st.Changes, fmt.Sprintf("%s is not swayed", n.Name))
	case PurposeIntimidate:
		if ok {
			st.Changes = append(st.Changes, fmt.Sprintf("%s is cowed by %s and does as told", n.Name, c.Name))
			st.Info = append(st.Info, fmt.Sprintf("%s is frightened of %s and answers truthfully for now", n.Name, c.Name))
			n.Remember(fmt.Sprintf("%s frightened me", c.Name))
			handOver(w, n, c, true, st)
			return
		}
		n.Bind(w, c, TieGrudge, "threatened them", SourceEngine)
		st.Changes = append(st.Changes, fmt.Sprintf("%s won't be bullied and holds it against %s (attitude now %+d)", n.Name, c.Name, n.AttitudeTo(c)))
	}
}

// handOver gives c the items n holds that one of c's active quests needs.
// Unforced, a hostile NPC keeps them.
func handOver(w *World, n *NPC, c *Character, force bool, st *Step) {
	if !force && n.AttitudeTo(c) < 0 {
		return
	}
	for _, q := range w.Quests {
		if q.Status != QuestActive || !OnQuest(q, c) {
			continue
		}
		for _, o := range q.Objectives {
			if o.Done || o.Kind != ObjObtainItem {
				continue
			}
			if it := w.FindItemOf(OwnerNPC, n.ID, o.Target); it != nil && !isReward(w, it.ID) {
				it.Owner = Owner{Kind: OwnerCharacter, ID: c.ID}
				st.Changes = append(st.Changes, fmt.Sprintf("%s hands %s to %s", n.Name, it.Name, c.Name))
			}
		}
	}
}

// stealTarget finds what c means to lift from n: the named item, or the
// one thing n carries that isn't for sale.
func stealTarget(w *World, n *NPC, a Action) (*Item, error) {
	if w.InCombat() {
		return nil, fmt.Errorf("there is no picking pockets in the middle of a fight")
	}
	ref := a.Item
	if ref != "" {
		it := w.FindItemOf(OwnerNPC, n.ID, ref)
		if it == nil {
			return nil, fmt.Errorf("%s isn't carrying %q", n.Name, ref)
		}
		if isReward(w, it.ID) {
			return nil, fmt.Errorf("%s keeps %s locked away, not on them", n.Name, it.Name)
		}
		return it, nil
	}
	var pick *Item
	for _, it := range w.ItemsOf(OwnerNPC, n.ID) {
		if isReward(w, it.ID) || (n.Merchant && it.Kind != ItemQuest) {
			continue
		}
		if it.Kind == ItemQuest || pick == nil {
			pick = it
		}
	}
	if pick == nil {
		for _, it := range w.ItemsOf(OwnerNPC, n.ID) {
			if NameContains(a.Text, it.Name) && !isReward(w, it.ID) {
				return it, nil
			}
		}
		return nil, fmt.Errorf("%s carries nothing worth lifting; name the item", n.Name)
	}
	// a name in the text beats the first pick
	for _, it := range w.ItemsOf(OwnerNPC, n.ID) {
		if NameContains(a.Text, it.Name) && !isReward(w, it.ID) {
			return it, nil
		}
	}
	return pick, nil
}

// stealDC: a wary mark watches their things.
func stealDC(n *NPC, c *Character) int {
	if n.AttitudeTo(c) <= -2 || n.Antagonist {
		return 20
	}
	return 15
}

func stealResult(w *World, n *NPC, it *Item, c *Character, ok bool, st *Step) {
	if ok {
		it.Owner = Owner{Kind: OwnerCharacter, ID: c.ID}
		st.Changes = append(st.Changes, fmt.Sprintf("%s lifts %s from %s unnoticed", c.Name, it.Name, n.Name))
		return
	}
	n.Disposition = clamp(n.Disposition-1, -3, 3)
	n.Bind(w, c, TieGrudge, "tried to rob them", SourceEngine)
	n.Remember(fmt.Sprintf("%s tried to rob me", c.Name))
	st.Changes = append(st.Changes, fmt.Sprintf("%s catches %s reaching for %s (attitude now %+d)", n.Name, c.Name, it.Name, n.AttitudeTo(c)))
}

// sneakDC: the more dangerous the place, the more is watching.
func sneakDC(w *World) int {
	if l := w.Here(); l != nil {
		return 10 + clamp(l.Danger, 0, 5)
	}
	return 10
}

func sneakResult(c *Character, ok bool, st *Step) {
	if !ok {
		st.Changes = append(st.Changes, fmt.Sprintf("%s is noticed", c.Name))
		return
	}
	c.Conditions.remove(CondHidden)
	addStance(c, CondHidden, "sneaking")
	st.Changes = append(st.Changes, fmt.Sprintf("%s moves unseen: until their next message ends, a fight that starts finds them hidden, and an ambush can't catch the party off guard", c.Name))
}

// partySneaking is the hidden party member, if any, when a fight breaks out.
func partySneaking(w *World) *Character {
	for _, c := range w.Party() {
		if !c.Down() && c.Conditions.Has(CondHidden) {
			return c
		}
	}
	return nil
}

// dropSneaking ends out-of-fight hiding for the whole party (they went
// somewhere new).
func dropSneaking(w *World) {
	if w.InCombat() {
		return
	}
	for _, c := range w.Party() {
		c.Conditions.remove(CondHidden)
	}
}

// --- mind spells on NPCs -------------------------------------------------

// CharmTurns is how many messages Charm Person lasts (about an hour).
const CharmTurns = 10

// Charm is a Charm Person on an NPC: the attitude it had before, restored
// (one worse, as they know they were charmed) when it ends.
type Charm struct {
	By     string `json:"by"`
	Before int    `json:"before"`
	Until  int    `json:"until"` // campaign turn
}

// spellNPC finds the NPC a mind spell targets; they must be here and not
// fighting the party.
func spellNPC(w *World, a Action, name string) (*NPC, error) {
	n := socialNPC(w, a)
	if n == nil {
		if a.Target == "" {
			if ns := w.NPCsAt(w.Campaign.LocationID); len(ns) == 1 {
				n = ns[0]
			}
		}
		if n == nil {
			return nil, fmt.Errorf("%s needs a person here to work on", name)
		}
	}
	if npcFoe(w, n) != nil {
		return nil, fmt.Errorf("%s is fighting you; %s won't take hold in the fray", n.Name, name)
	}
	return n, nil
}

// npcSave rolls an NPC's save against c's spell DC.
func npcSave(ctx *Ctx, n *NPC, c *Character, label string, st *Step) bool {
	mod := 0
	if n.Stats != nil {
		mod = n.Stats.AttackBonus / 2
	}
	if n.Antagonist {
		mod += 4
	}
	dc := SpellDC(c)
	ch := ctx.R.D20(mod, dc)
	st.Rolls = append(st.Rolls, Roll{Label: label, Kind: "check", Natural: ch.Natural, Mod: ch.Mod, Total: ch.Total, Target: dc,
		Tier: flip(ch.Tier), Against: "DC", Foe: true, By: n.Name})
	return ch.Tier.Succeeded()
}

// mindSpell resolves Charm Person and Suggestion on an NPC.
func mindSpell(ctx *Ctx, c *Character, s *SpellDef, n *NPC, a Action, st *Step) {
	w := ctx.W
	name := SpellName(s, w.Setting())
	if npcSave(ctx, n, c, fmt.Sprintf("%s's WIS save against %s", n.Name, name), st) {
		st.Changes = append(st.Changes, fmt.Sprintf("%s shrugs off %s", n.Name, name))
		if s.Key == "charm_person" {
			n.Remember(fmt.Sprintf("%s tried to bewitch me", c.Name))
		}
		return
	}
	switch s.Key {
	case "charm_person":
		before := n.Disposition
		if n.Charm != nil {
			before = n.Charm.Before
		}
		n.Charm = &Charm{By: c.ID, Before: before, Until: w.Campaign.Turn + CharmTurns}
		n.Disposition = max(n.Disposition, 2)
		st.Changes = append(st.Changes, fmt.Sprintf("%s is charmed by %s and treats the party as friends (attitude %+d for about an hour)", n.Name, c.Name, n.AttitudeTo(c)))
		handOver(w, n, c, false, st)
	case "suggestion":
		what := strings.TrimSpace(a.Text)
		st.Changes = append(st.Changes, fmt.Sprintf("%s falls under %s's suggestion", n.Name, c.Name))
		if what != "" {
			st.Info = append(st.Info, fmt.Sprintf("%s will follow the suggestion if it is reasonable: %s", n.Name, trimTo(what, 160)))
		}
		handOver(w, n, c, true, st)
	}
}

// ExpireCharms ends Charm Person effects whose hour is up.
func ExpireCharms(w *World) []string {
	var out []string
	for _, n := range w.NPCs {
		if n.Charm == nil || w.Campaign.Turn < n.Charm.Until {
			continue
		}
		by := n.Charm.By
		n.Disposition = clamp(n.Charm.Before-1, -3, 3)
		n.Charm = nil
		who := by
		if ch := w.Character(by); ch != nil {
			who = ch.Name
		}
		n.Remember(fmt.Sprintf("%s bewitched me", who))
		out = append(out, fmt.Sprintf("%s's charm on %s fades, and they know it", who, n.Name))
	}
	return out
}

// searchFinds tells the narrator what a search that revealed no hidden way
// turned up, so it doesn't invent secret passages or loot.
func searchFinds(w *World, l *Location) string {
	var names []string
	for _, it := range w.ItemsOf(OwnerLocation, l.ID) {
		names = append(names, it.Name)
	}
	if len(names) == 0 {
		return "The search finds no hidden way and nothing to take here (clues and details may still be described)"
	}
	return "The search finds no hidden way; things lying here that can be picked up: " + strings.Join(names, ", ")
}
