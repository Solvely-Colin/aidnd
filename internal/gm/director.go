package gm

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/Solvely-Colin/aidnd/internal/game"
)

// LoreActiveTurns is how long a triggered lore entry stays in context.
const LoreActiveTurns = 2

// Direction is what the director decided for this turn. It is computed by
// code and fed to the narrator; none of it is shown to players.
type Direction struct {
	Beat          *game.Beat
	BeatChanges   []string
	Lore          []*game.Lore
	MainObjective string
	SceneGoal     string
	RouteHint     string
	Pacing        string
	Tension       string
	ForceProgress bool
}

// Note renders the DIRECTOR'S NOTE placed last in the narrator prompt.
func (d Direction) Note() string {
	var b strings.Builder
	b.WriteString("DIRECTOR'S NOTE\n")
	if d.MainObjective != "" {
		fmt.Fprintf(&b, "- Current main objective: %s\n", d.MainObjective)
	}
	if d.SceneGoal != "" {
		fmt.Fprintf(&b, "- Scene goal: %s", d.SceneGoal)
		if d.RouteHint != "" {
			fmt.Fprintf(&b, " (%s)", d.RouteHint)
		}
		b.WriteString("\n")
	}
	if d.Beat != nil {
		fmt.Fprintf(&b, "- PLAY THIS BEAT NOW: %s. %s\n", d.Beat.Title, d.Beat.Text)
		for _, c := range d.BeatChanges {
			fmt.Fprintf(&b, "  (engine already applied: %s)\n", c)
		}
	}
	fmt.Fprintf(&b, "- Pacing: %s\n", d.Pacing)
	fmt.Fprintf(&b, "- Tension: %s\n", d.Tension)
	if d.ForceProgress {
		b.WriteString("- The scene has stalled. You MUST call one progress tool now (reveal_exit, add_exit, spawn_encounter or offer_quest) and narrate it.\n")
	}
	return b.String()
}

// noteSceneTurn updates the scene counters after a resolution.
func noteSceneTurn(w *game.World, res *game.Resolution) {
	if res.AllFree {
		return
	}
	s := &w.Campaign.Scene
	s.Turns++
	eventful := res.Arrived != "" || res.CombatStarted || res.CombatEnded || len(res.Quests) > 0 || w.InCombat()
	for _, st := range res.Steps {
		for _, c := range st.Changes {
			if strings.Contains(c, "hidden way") || strings.Contains(c, "is open") {
				eventful = true
			}
		}
	}
	if eventful {
		s.Quiet = 0
	} else {
		s.Quiet++
	}
	for _, q := range res.Quests {
		if q.ObjectiveID != "" {
			game.AdjustTension(w, -1)
		}
	}
}

// direct picks the beat, goal, pacing and lore for this turn. Beats fire
// at most once per turn, never mid-combat and never on quick answers.
func (g *GM) direct(ctx context.Context, w *game.World, res *game.Resolution, recent string) Direction {
	var d Direction
	sc := &w.Campaign.Scene
	mainObj, goal, route := g.sceneGoal(w)
	d.MainObjective, d.SceneGoal, d.RouteHint = mainObj, goal, route
	sc.Goal = goal
	cap := g.Cfg.SceneTurnCap
	switch {
	case w.InCombat():
		d.Pacing = "A fight is under way. Keep it tight and physical."
	case sc.Quiet >= cap:
		d.ForceProgress = true
		d.Pacing = "Stalled. Move the story forward now."
	case sc.Quiet >= cap/2:
		d.Pacing = "The scene is dragging. Let the world push: a sound, an approaching NPC, a clue pointing to an exit or to the scene goal."
	case sc.Turns <= 1:
		d.Pacing = "A new scene. Establish where they are in one or two vivid details."
	default:
		d.Pacing = "Let the players lead; answer what they did."
	}
	switch t := sc.Tension; {
	case t >= 7:
		d.Tension = fmt.Sprintf("%d/10. Danger is close. Short, urgent sentences.", t)
	case t >= 4:
		d.Tension = fmt.Sprintf("%d/10. Unease: small signs that something is wrong.", t)
	default:
		d.Tension = fmt.Sprintf("%d/10. Calm. Room for talk and exploration.", t)
	}
	if !res.AllFree && !w.InCombat() && w.Campaign.Status == game.StatusActive {
		if b := pickBeat(w, res); b != nil {
			b.Fired, b.FiredTurn = true, w.Campaign.Turn
			d.Beat = b
			d.BeatChanges = g.applyBeat(ctx, w, b)
			sc.Quiet = 0
			if len(d.BeatChanges) > 0 {
				d.ForceProgress = false // the beat itself moved the story
			}
		}
	}
	d.Lore = selectLore(w, recent, g.Cfg.LoreBudget)
	var lt []string
	for _, l := range d.Lore {
		lt = append(lt, l.Title)
	}
	beat := "none"
	if d.Beat != nil {
		beat = d.Beat.Title
	}
	g.debugf("campaign %s turn %d: scene turns=%d quiet=%d tension=%d goal=%q route=%q beat=%s force=%v lore=%v",
		w.Campaign.ID, w.Campaign.Turn, sc.Turns, sc.Quiet, sc.Tension, goal, route, beat, d.ForceProgress, lt)
	return d
}

func pickBeat(w *game.World, res *game.Resolution) *game.Beat {
	here := w.Campaign.LocationID
	for _, b := range w.Beats {
		if b.Fired {
			continue
		}
		fire := false
		switch b.Trigger {
		case game.TriggerEnterLocation:
			fire = b.Target == here && res.Arrived == here
		case game.TriggerObjectiveDone:
			for _, q := range w.Quests {
				for _, o := range q.Objectives {
					if o.ID == b.Target && o.Done {
						fire = true
					}
				}
			}
		case game.TriggerStalled:
			fire = w.Campaign.Scene.Quiet >= max(1, b.Value)
		case game.TriggerTension:
			fire = w.Campaign.Scene.Tension >= max(1, b.Value)
		case game.TriggerTurn:
			fire = w.Campaign.Turn >= b.Value
		}
		if fire && ambushes(b) && safeHere(w) {
			fire = false // no fights in the safe hub; the beat waits for the road
		}
		if fire && !castPresent(w, b, w.Character(res.ActorID)) {
			fire = false // the beat waits until the actor is with the people in it
		}
		if fire {
			return b
		}
	}
	return nil
}

// castPresent reports whether every NPC a beat names is in the actor's
// scene: same location and, where zones exist, same zone. An NPC the beat
// brings in itself does not need to be there yet.
func castPresent(w *game.World, b *game.Beat, actor *game.Character) bool {
	l := w.Here()
	if l == nil {
		return true
	}
	for _, n := range w.NPCs {
		if !n.Alive || !beatNames(b.Text, n.Name) {
			continue
		}
		if n.ID == b.EffectTarget && hasEffect(b, game.EffectBringNPC) {
			continue
		}
		if n.LocationID != l.ID {
			return false
		}
		if actor != nil && !withActor(l, actor, n) {
			return false
		}
	}
	return true
}

// honorifics are skipped when matching an NPC's name in beat text.
var honorifics = map[string]bool{"sister": true, "brother": true, "mother": true, "father": true, "doc": true, "doctor": true,
	"constable": true, "captain": true, "lady": true, "lord": true, "old": true, "the": true, "mister": true, "madam": true, "sir": true}

// beatNames reports whether text names the NPC: the full name, or a
// capitalised part of it that is not a title ("Halloway", "Ione"). A part
// at the start of a sentence does not count, so "Pale hands rise" does
// not name "Morwen the Pale".
func beatNames(text, name string) bool {
	if name == "" {
		return false
	}
	if strings.Contains(strings.ToLower(text), strings.ToLower(name)) {
		return true
	}
	parts := map[string]bool{}
	for _, p := range strings.Fields(name) {
		p = strings.Trim(p, ",.'\"")
		if len(p) >= 3 && !honorifics[strings.ToLower(p)] && unicode.IsUpper([]rune(p)[0]) {
			parts[p] = true
		}
	}
	start := true
	for _, f := range strings.Fields(text) {
		wd := strings.TrimSuffix(strings.TrimSuffix(strings.TrimFunc(f, func(r rune) bool { return !unicode.IsLetter(r) && r != '\'' }), "'s"), "'")
		if !start && parts[wd] {
			return true
		}
		start = strings.ContainsAny(f[len(f)-1:], ".!?:\"")
	}
	return false
}

func hasEffect(b *game.Beat, e string) bool {
	for _, x := range b.Effects {
		if x == e {
			return true
		}
	}
	return false
}

func ambushes(b *game.Beat) bool {
	for _, e := range b.Effects {
		if e == game.EffectAmbush {
			return true
		}
	}
	return false
}

func safeHere(w *game.World) bool {
	l := w.Here()
	return l != nil && l.IsHub && l.Danger == 0
}

func (g *GM) applyBeat(ctx context.Context, w *game.World, b *game.Beat) []string {
	var changes []string
	l := w.Here()
	for _, e := range b.Effects {
		switch e {
		case game.EffectAmbush:
			if n := w.NPC(b.EffectTarget); n != nil && n.LocationID == l.ID && n.Alive {
				if en := game.NPCJoinsFight(w, n); en != nil {
					changes = append(changes, fmt.Sprintf("%s attacks!", n.Name))
				}
				continue
			}
			specs := g.authorEnemies(ctx, w, l, "story beat: "+b.Title)
			if len(specs) == 0 {
				specs = game.GenericEnemies(l.Kind, max(1, l.Danger), len(w.Characters))
			}
			if enc, _, err := game.StartEncounter(w, specs); err == nil {
				var names []string
				for _, en := range enc.Enemies {
					names = append(names, en.Display())
				}
				changes = append(changes, "Ambush: "+strings.Join(names, ", "))
			}
		case game.EffectRevealExit:
			id := b.EffectTarget
			if e := w.Edge(id); e == nil || e.State != game.EdgeHidden || !e.Touches(l.ID) {
				id = ""
				for _, x := range w.Exits(l.ID, true) {
					if x.Edge.State == game.EdgeHidden {
						id = x.Edge.ID
						break
					}
				}
			}
			if id != "" && game.RevealExit(w, id) == nil {
				e := w.Edge(id)
				changes = append(changes, fmt.Sprintf("A hidden way is revealed: %s", firstOf(e.LabelAt(l.ID), "to "+w.Location(e.Other(l.ID)).Name)))
			}
		case game.EffectBringNPC:
			n := w.NPC(b.EffectTarget)
			if n == nil || !n.Alive || n.Antagonist {
				continue
			}
			n.LocationID = l.ID
			if len(l.Zones) > 0 {
				n.ZoneID = l.Zones[0].ID
			}
			changes = append(changes, fmt.Sprintf("%s arrives", n.Name))
		}
	}
	return changes
}

// sceneGoal derives the current main objective and a route hint.
func (g *GM) sceneGoal(w *game.World) (mainObj, goal, route string) {
	for _, q := range w.Quests {
		if q.Status == game.QuestCompleted && q.GiverID != "" {
			if n := w.NPC(q.GiverID); n != nil {
				return currentMain(w), fmt.Sprintf("Turn in %q to %s", q.Title, n.Name), routeTo(w, n.LocationID)
			}
		}
	}
	mq := w.MainQuest()
	if mq == nil {
		return "", "", ""
	}
	for _, o := range mq.Objectives {
		if o.Done {
			continue
		}
		mainObj = o.Text
		switch o.Kind {
		case game.ObjReachLocation:
			return mainObj, o.Text, routeTo(w, o.Target)
		case game.ObjTalkToNPC:
			if n := w.NPC(o.Target); n != nil {
				return mainObj, o.Text, routeTo(w, n.LocationID)
			}
		case game.ObjDefeatEnemy:
			if n := w.NPC(o.Target); n != nil {
				return mainObj, o.Text, routeTo(w, n.LocationID)
			}
		}
		return mainObj, o.Text, ""
	}
	return "", "", ""
}

func currentMain(w *game.World) string {
	if mq := w.MainQuest(); mq != nil {
		for _, o := range mq.Objectives {
			if !o.Done {
				return o.Text
			}
		}
	}
	return ""
}

// routeTo describes the first hop toward loc over visible edges.
func routeTo(w *game.World, loc string) string {
	here := w.Campaign.LocationID
	dest := w.Location(loc)
	if dest == nil {
		return ""
	}
	if loc == here {
		return "they are here"
	}
	prev := map[string]string{here: ""}
	queue := []string{here}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, e := range w.Edges {
			if !e.Touches(cur) || !e.State.Visible() {
				continue
			}
			n := e.Other(cur)
			if _, ok := prev[n]; !ok {
				prev[n] = cur
				queue = append(queue, n)
			}
		}
	}
	if _, ok := prev[loc]; !ok {
		return fmt.Sprintf("no known way to %s yet; a search, an NPC or a new path is needed", dest.Name)
	}
	hop := loc
	for prev[hop] != here {
		hop = prev[hop]
	}
	if hop == loc {
		return fmt.Sprintf("%s is one exit away", dest.Name)
	}
	return fmt.Sprintf("head toward %s, via %s", dest.Name, w.Location(hop).Name)
}

// selectLore chooses relevant canon within a character budget: always-on
// entries, entries about entities in the scene, entries whose keywords
// appear in recent text, and entries triggered in the last two turns.
func selectLore(w *game.World, recent string, budget int) []*game.Lore {
	turn := w.Campaign.Turn
	inScene := map[string]bool{w.Campaign.LocationID: true}
	for _, n := range w.NPCsAt(w.Campaign.LocationID) {
		inScene[n.ID] = true
	}
	if w.Encounter != nil {
		for _, e := range w.Encounter.Enemies {
			if e.NPCID != "" {
				inScene[e.NPCID] = true
			}
		}
	}
	text := strings.ToLower(recent)
	type cand struct {
		l    *game.Lore
		prio int
	}
	var cs []cand
	for i, l := range w.Lore {
		prio := -1
		switch {
		case l.AlwaysOn:
			prio = 0
		case hasAny(inScene, l.EntityIDs):
			prio = 1
			l.LastTrigger = turn
		case keywordHit(text, l.Keywords):
			prio = 2
			l.LastTrigger = turn
		case l.LastTrigger >= turn-LoreActiveTurns:
			prio = 3
		}
		if prio >= 0 {
			cs = append(cs, cand{l, prio*1000 + i})
		}
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].prio < cs[j].prio })
	var out []*game.Lore
	used := 0
	for _, c := range cs {
		n := len(c.l.Title) + len(c.l.Text) + 4
		if used+n > budget && len(out) > 0 {
			continue
		}
		used += n
		out = append(out, c.l)
	}
	return out
}

func hasAny(set map[string]bool, ids []string) bool {
	for _, id := range ids {
		if set[id] {
			return true
		}
	}
	return false
}

func keywordHit(text string, kws []string) bool {
	for _, k := range kws {
		k = strings.ToLower(strings.TrimSpace(k))
		if len(k) >= 3 && strings.Contains(text, k) {
			return true
		}
	}
	return false
}
