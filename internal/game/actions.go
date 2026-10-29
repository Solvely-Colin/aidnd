package game

import (
	"fmt"
	"strings"
	"time"

	"github.com/Solvely-Colin/aidnd/internal/dice"
)

// Action kinds the interpreter may emit.
const (
	ActLook        = "look"
	ActSay         = "say"
	ActEquip       = "equip"
	ActDrop        = "drop"
	ActEndTurn     = "end_turn"
	ActMove        = "move"
	ActReposition  = "reposition"
	ActTravel      = "travel"
	ActAttack      = "attack"
	ActSubdue      = "subdue"
	ActCheck       = "ability_check"
	ActUseItem     = "use_item"
	ActTalkTo      = "talk_to"
	ActBuy         = "buy"
	ActSell        = "sell"
	ActAcceptQuest = "accept_quest"
	ActTurnInQuest = "turn_in_quest"
	ActPickUp      = "pick_up"
	ActGiveItem    = "give_item"
	ActRest        = "rest"
)

// ActionKinds lists every valid kind.
var ActionKinds = []string{ActLook, ActSay, ActEquip, ActDrop, ActEndTurn, ActMove, ActReposition, ActTravel,
	ActAttack, ActSubdue, ActCheck, ActUseItem, ActTalkTo, ActBuy, ActSell, ActAcceptQuest, ActTurnInQuest,
	ActPickUp, ActGiveItem, ActRest}

// MaxSteps is how many steps one message may resolve.
const MaxSteps = 4

// Action is one structured step from the interpreter. Target holds an id
// or name whose meaning depends on Kind.
type Action struct {
	Kind    string `json:"kind"`
	Target  string `json:"target,omitempty"`  // enemy, npc, character, exit, location, quest
	Item    string `json:"item,omitempty"`    // item id or name
	Zone    string `json:"zone,omitempty"`    // zone id or name
	Ability string `json:"ability,omitempty"` // for checks
	DC      int    `json:"dc,omitempty"`
	Purpose string `json:"purpose,omitempty"` // ability_check: search | unlock | force | other
	Method  string `json:"method,omitempty"`  // subdue method
	Text    string `json:"text,omitempty"`    // speech, or what the check is for
	Promise string `json:"promise,omitempty"` // talk_to: what the character gave their word to do
}

// Roll is a dice roll reported to players.
type Roll struct {
	Label   string    `json:"label"`
	Kind    string    `json:"kind"` // check | attack | damage | heal | subdue | encounter
	Natural int       `json:"natural,omitempty"`
	Dice    []int     `json:"dice,omitempty"`
	Expr    string    `json:"expr,omitempty"`
	Mod     int       `json:"mod"`
	Total   int       `json:"total"`
	Target  int       `json:"target,omitempty"` // DC or AC
	Tier    dice.Tier `json:"tier,omitempty"`
	// Parts break Mod down (ability, proficiency, tools); they sum to Mod.
	Parts   []RollPart `json:"parts,omitempty"`
	Against string     `json:"against,omitempty"` // "AC" or "DC" for d20 rolls
	Cover   int        `json:"cover,omitempty"`   // cover bonus included in Target
	Crit    bool       `json:"crit,omitempty"`    // damage dice were doubled
}

func checkRoll(kind string, c CheckRoll) Roll {
	return Roll{Label: c.Label, Kind: kind, Natural: c.Natural, Mod: c.Mod, Total: c.Total, Target: c.Target, Tier: c.Tier,
		Parts: c.Parts, Against: c.Against, Cover: c.Cover}
}

func dmgRoll(label, kind string, r dice.Result) Roll {
	return Roll{Label: label, Kind: kind, Dice: r.Dice, Expr: r.Expr, Mod: r.Mod, Total: r.Total}
}

// Step is the outcome of one action.
type Step struct {
	Action        Action        `json:"action"`
	Slot          Slot          `json:"slot,omitempty"`
	OK            bool          `json:"ok"`
	Error         string        `json:"error,omitempty"`
	Skipped       bool          `json:"skipped,omitempty"`
	Rolls         []Roll        `json:"rolls,omitempty"`
	Tier          dice.Tier     `json:"tier,omitempty"`
	Changes       []string      `json:"changes,omitempty"`
	Info          []string      `json:"info,omitempty"` // facts for the narrator (look results etc.)
	Quests        []QuestUpdate `json:"quests,omitempty"`
	Arrived       string        `json:"arrived,omitempty"` // new location id
	CombatStarted bool          `json:"combat_started,omitempty"`
	CombatEnded   bool          `json:"combat_ended,omitempty"`
}

// Resolution is everything the engine decided for one message.
type Resolution struct {
	ActorID       string        `json:"actor_id"`
	Steps         []Step        `json:"steps"`
	Changes       []string      `json:"changes"`
	Quests        []QuestUpdate `json:"quests,omitempty"`
	Skipped       []Step        `json:"skipped,omitempty"`
	TurnEnded     bool          `json:"turn_ended"`
	Budget        Budget        `json:"budget"`
	AllFree       bool          `json:"all_free"` // only minor free actions were taken (not talk or trade)
	Arrived       string        `json:"arrived,omitempty"`
	CombatStarted bool          `json:"combat_started,omitempty"`
	CombatEnded   bool          `json:"combat_ended,omitempty"`
	CampaignEnded bool          `json:"campaign_ended,omitempty"`
}

// Rolls returns every roll in order.
func (r *Resolution) Rolls() []Roll {
	var out []Roll
	for _, s := range r.Steps {
		out = append(out, s.Rolls...)
	}
	return out
}

// Ctx carries what resolution needs besides the world. The hooks let the
// orchestrator author content (stub fill-in, zones, ambush enemies)
// without the engine doing any I/O. Both hooks may be nil.
type Ctx struct {
	W   *World
	R   *dice.Roller
	Now time.Time
	// OnArrive is called when the party enters a stub or a location
	// without zones, before placement. It should fill and lay out loc.
	OnArrive func(loc *Location)
	// Ambush supplies enemies for a random encounter at loc.
	Ambush func(loc *Location) []EnemySpec
}

// SlotFor says what an action costs given the current situation. For
// reposition it depends on distance and is computed in resolve.
func SlotFor(w *World, a Action) Slot {
	switch a.Kind {
	case ActLook, ActSay, ActEquip, ActDrop, ActPickUp, ActAcceptQuest, ActTurnInQuest:
		return SlotFree
	case ActEndTurn:
		return SlotNone
	case ActMove, ActTravel:
		if a.Kind == ActMove && w.InCombat() && a.Zone == "" {
			return SlotAction // fleeing through an exit
		}
		return SlotMove
	case ActReposition:
		if !w.InCombat() {
			return SlotFree
		}
		return SlotMove
	case ActTalkTo, ActBuy, ActSell:
		if !w.InCombat() {
			return SlotFree // talking and trading only cost the action in a fight
		}
	}
	return SlotAction
}

// Resolve executes up to MaxSteps actions for the active character, in
// order, against the turn budget. Unaffordable steps are skipped and
// reported. Spending the action ends the turn, but the rest of the message
// still runs where the budget covers it ("I stab it and step back"); only
// end_turn stops the message outright. Several trades with one merchant in
// one message cost a single action.
func Resolve(ctx *Ctx, actor *Character, actions []Action) *Resolution {
	w := ctx.W
	res := &Resolution{ActorID: actor.ID, AllFree: true}
	if len(actions) > MaxSteps {
		for _, a := range actions[MaxSteps:] {
			res.Skipped = append(res.Skipped, Step{Action: a, Skipped: true, Error: fmt.Sprintf("only %d steps per message", MaxSteps)})
		}
		actions = actions[:MaxSteps]
	}
	budget := &w.Campaign.Turns.Budget
	tradedWith := map[string]bool{}
	stopped := false
	for i, a := range actions {
		if stopped || w.Campaign.Status != StatusActive {
			for _, rest := range actions[i:] {
				res.Skipped = append(res.Skipped, Step{Action: rest, Skipped: true, Error: "your turn ended before this step"})
			}
			break
		}
		if actor.Down() {
			res.Skipped = append(res.Skipped, Step{Action: a, Skipped: true, Error: actor.Name + " is down"})
			continue
		}
		if a.Kind == ActReposition && a.Zone != "" && w.Here().Zone(a.Zone) != nil && w.Here().Zone(a.Zone).ID == actor.ZoneID {
			continue // already there: nothing to do or report
		}
		slot, err := costOf(w, actor, a)
		if err != nil {
			st := Step{Action: a, Error: err.Error()}
			res.Steps = append(res.Steps, st)
			continue
		}
		trade := a.Kind == ActBuy || a.Kind == ActSell
		if trade && tradedWith[strings.ToLower(a.Target)] {
			slot = SlotFree // same bargain, already paid for
		}
		trial := *budget
		used, err := trial.Pay(slot)
		if err != nil {
			res.Skipped = append(res.Skipped, Step{Action: a, Skipped: true, Error: err.Error()})
			continue
		}
		wasCombat := w.InCombat()
		st := Step{Action: a, Slot: used}
		if err := execute(ctx, actor, a, &st); err != nil {
			st.Error = err.Error()
			st.Slot = ""
			res.Steps = append(res.Steps, st)
			continue
		}
		st.OK = true
		*budget = trial
		if trade {
			tradedWith[strings.ToLower(a.Target)] = true
		}
		if !wasCombat && w.InCombat() && (budget.Free == Unlimited || budget.Free > CombatFreeActions) {
			budget.Free = CombatFreeActions // a fight broke out during this step
		}
		if (used != SlotFree && used != SlotNone) || a.Kind == ActTalkTo || a.Kind == ActBuy || a.Kind == ActSell {
			res.AllFree = false // talk and trade are free outside fights but still get full narration
		}
		// automatic quest ticks after every successful step
		ups, ch := EvaluateQuests(w)
		st.Quests = append(st.Quests, ups...)
		st.Changes = append(st.Changes, ch...)
		if w.Encounter != nil && len(w.Encounter.Living()) == 0 {
			st.Changes = append(st.Changes, endEncounter(w)...)
			st.CombatEnded = true
		}
		if !wasCombat && w.InCombat() {
			st.CombatStarted = true
		}
		res.Steps = append(res.Steps, st)
		res.Changes = append(res.Changes, st.Changes...)
		res.Quests = append(res.Quests, st.Quests...)
		if st.Arrived != "" {
			res.Arrived = st.Arrived
		}
		res.CombatStarted = res.CombatStarted || st.CombatStarted
		res.CombatEnded = res.CombatEnded || st.CombatEnded
		if a.Kind == ActEndTurn {
			stopped = true
		}
		if a.Kind == ActEndTurn || budget.Spent() {
			res.TurnEnded = true
		}
	}
	if w.Campaign.Status == StatusCompleted {
		res.CampaignEnded = true
		res.TurnEnded = true
	}
	if res.AllFree {
		for _, s := range res.Steps {
			if s.OK && s.Action.Kind == ActEndTurn {
				res.AllFree = false
			}
		}
	}
	res.Budget = *budget
	return res
}

// costOf determines the slot, validating reposition distance.
func costOf(w *World, c *Character, a Action) (Slot, error) {
	if a.Kind == ActReposition || (a.Kind == ActMove && a.Zone != "" && a.Target == "") {
		if !w.InCombat() {
			return SlotFree, nil
		}
		l := w.Here()
		z := l.Zone(a.Zone)
		if z == nil {
			return "", fmt.Errorf("unknown zone %q", a.Zone)
		}
		switch d := ZoneDistance(l, c.ZoneID, z.ID); {
		case d == 0:
			return "", fmt.Errorf("%s is already in %s", c.Name, z.Name)
		case d == 1:
			return SlotMove, nil
		case d == 2:
			return SlotDash, nil
		default:
			return "", fmt.Errorf("%s is %s away; you can move at most 2 zones in a turn", z.Name, zonesWord(d))
		}
	}
	for _, k := range ActionKinds {
		if k == a.Kind {
			return SlotFor(w, a), nil
		}
	}
	return "", fmt.Errorf("unknown action %q", a.Kind)
}

func execute(ctx *Ctx, c *Character, a Action, st *Step) error {
	w := ctx.W
	switch a.Kind {
	case ActLook:
		facts := Look(w, c, a.Target)
		if len(facts) == 0 && a.Target != "" {
			// answer about the thing asked for, not the whole place
			st.Info = append(st.Info, fmt.Sprintf("%s looks for %s but finds nothing like it here", c.Name, trimTo(a.Target, 60)))
			return nil
		}
		st.Info = append(st.Info, facts...)
		// examining someone counts as meeting them (talk_to_npc objectives)
		if n := w.NPC(a.Target); n != nil && n.Alive && n.LocationID == w.Campaign.LocationID {
			n.LastTalked = w.Campaign.Turn
		}
		return nil
	case ActSay:
		if strings.TrimSpace(a.Text) == "" {
			return fmt.Errorf("say what?")
		}
		st.Info = append(st.Info, fmt.Sprintf("%s says: %q", c.Name, trimTo(a.Text, 300)))
		return nil
	case ActEndTurn:
		st.Changes = append(st.Changes, c.Name+" ends their turn")
		return nil
	case ActEquip:
		return one(st, func() (string, error) { return Equip(w, c, firstNonEmpty(a.Item, a.Target)) })
	case ActDrop:
		return one(st, func() (string, error) { return Drop(w, c, firstNonEmpty(a.Item, a.Target)) })
	case ActPickUp:
		return one(st, func() (string, error) { return PickUp(w, c, firstNonEmpty(a.Item, a.Target)) })
	case ActGiveItem:
		return one(st, func() (string, error) { return Give(w, c, a.Item, a.Target) })
	case ActBuy:
		return one(st, func() (string, error) { return Buy(w, c, a.Target, a.Item) })
	case ActSell:
		return one(st, func() (string, error) { return Sell(w, c, a.Target, a.Item) })
	case ActAcceptQuest:
		q, err := AcceptQuest(w, a.Target)
		if err != nil {
			return err
		}
		st.Changes = append(st.Changes, fmt.Sprintf("Quest accepted: %s", q.Title))
		st.Quests = append(st.Quests, QuestUpdate{QuestID: q.ID, Title: q.Title, Text: "Quest accepted: " + q.Title, Status: q.Status})
		return nil
	case ActTurnInQuest:
		ups, ch, err := TurnInQuest(w, a.Target, c)
		if err != nil {
			return err
		}
		st.Changes = append(st.Changes, ch...)
		st.Quests = append(st.Quests, ups...)
		return nil
	case ActTalkTo:
		return talkTo(w, c, a, st)
	case ActReposition:
		return reposition(w, c, a.Zone, st)
	case ActMove:
		if a.Target == "" && a.Zone != "" {
			return reposition(w, c, a.Zone, st)
		}
		return moveThrough(ctx, c, a.Target, st)
	case ActTravel:
		return travel(ctx, c, a.Target, st)
	case ActAttack:
		return attack(ctx, c, a, st)
	case ActSubdue:
		return subdue(ctx, c, a, st)
	case ActCheck:
		return abilityCheck(ctx, c, a, st)
	case ActUseItem:
		return useItem(ctx, c, a, st)
	case ActRest:
		return rest(ctx, c, st)
	}
	return fmt.Errorf("unknown action %q", a.Kind)
}

func one(st *Step, f func() (string, error)) error {
	s, err := f()
	if err != nil {
		return err
	}
	st.Changes = append(st.Changes, s)
	return nil
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// Look returns facts about the location or a specific thing.
func Look(w *World, c *Character, target string) []string {
	l := w.Here()
	if l == nil {
		return nil
	}
	if target != "" {
		if e := w.Enemy(target); e != nil {
			return []string{fmt.Sprintf("%s: %d/%d HP, AC %d, in %s (%s from %s)", e.Display(), e.HP, e.MaxHP, e.AC,
				ZoneName(l, e.ZoneID), zonesWord(max(0, ZoneDistance(l, c.ZoneID, e.ZoneID))), c.Name)}
		}
		if n := w.NPC(target); n != nil && n.LocationID == l.ID && n.Alive {
			return []string{fmt.Sprintf("%s, %s: %s (in %s)", n.Name, n.Role, n.Desc, ZoneName(l, n.ZoneID))}
		}
		if it := w.FindItemOf(OwnerLocation, l.ID, target); it != nil {
			return []string{fmt.Sprintf("On the ground: %s. %s", it.Name, it.Desc)}
		}
		if it := w.FindItemOf(OwnerCharacter, c.ID, target); it != nil {
			return []string{fmt.Sprintf("%s's %s. %s", c.Name, it.Name, it.Desc)}
		}
		if z := l.Zone(target); z != nil {
			s := fmt.Sprintf("%s: %s", z.Name, z.Desc)
			if z.Cover {
				s += " (offers cover)"
			}
			return []string{s}
		}
		if x := w.FindExit(l.ID, target); x != nil {
			return []string{fmt.Sprintf("Exit %s leads to %s (%s). %s", x.Label, x.To.Name, x.Edge.State, x.To.Hint)}
		}
		if !lookAround(target) && !NameMatch(l.Name, target) {
			return nil // nothing by that name is here
		}
	}
	facts := []string{fmt.Sprintf("%s: %s", l.Name, l.Description)}
	if z := l.Zone(c.ZoneID); z != nil {
		facts = append(facts, fmt.Sprintf("%s stands in %s", c.Name, z.Name))
	}
	for _, n := range w.NPCsAt(l.ID) {
		facts = append(facts, fmt.Sprintf("%s (%s) is in %s", n.Name, n.Role, ZoneName(l, n.ZoneID)))
	}
	for _, it := range w.ItemsOf(OwnerLocation, l.ID) {
		facts = append(facts, "On the ground: "+it.Name)
	}
	for _, x := range w.Exits(l.ID, false) {
		facts = append(facts, fmt.Sprintf("Exit: %s to %s (%s)", x.Label, x.To.Name, x.Edge.State))
	}
	if w.InCombat() {
		for _, e := range w.Encounter.Living() {
			facts = append(facts, fmt.Sprintf("Enemy %s in %s, %d/%d HP", e.Display(), ZoneName(l, e.ZoneID), e.HP, e.MaxHP))
		}
	}
	return facts
}

// lookAround reports whether a look target just means the surroundings.
func lookAround(target string) bool {
	t := strings.ToLower(strings.TrimSpace(target))
	t = strings.TrimPrefix(strings.TrimPrefix(t, "the "), "my ")
	switch t {
	case "", "around", "here", "room", "area", "place", "surroundings", "everything", "everyone", "scene", "location", "town", "street", "it all":
		return true
	}
	return false
}

func talkTo(w *World, c *Character, a Action, st *Step) error {
	n := w.NPC(a.Target)
	if n == nil || !n.Alive || n.LocationID != w.Campaign.LocationID {
		return fmt.Errorf("nobody called %q is here", a.Target)
	}
	// outside a fight, talking to someone across the location means
	// walking over to them, so the actor ends up in the NPC's zone
	if l := w.Here(); !w.InCombat() && l.Zone(n.ZoneID) != nil && n.ZoneID != c.ZoneID {
		c.ZoneID = n.ZoneID
		st.Changes = append(st.Changes, fmt.Sprintf("%s moves to %s", c.Name, ZoneName(l, n.ZoneID)))
	}
	n.LastTalked = w.Campaign.Turn
	st.Info = append(st.Info, fmt.Sprintf("%s speaks with %s (%s, disposition %+d)", c.Name, n.Name, n.Role, n.Disposition))
	if a.Text != "" {
		st.Info = append(st.Info, fmt.Sprintf("%s says: %q", c.Name, trimTo(a.Text, 300)))
	}
	for _, q := range w.Quests {
		if q.GiverID == n.ID && q.Status == QuestOffered {
			st.Info = append(st.Info, fmt.Sprintf("%s offers quest %s [%s]", n.Name, q.Title, q.ID))
		}
		if q.GiverID == n.ID && q.Status == QuestCompleted {
			st.Info = append(st.Info, fmt.Sprintf("%s awaits the turn-in of %s [%s]", n.Name, q.Title, q.ID))
		}
	}
	// an NPC who holds what an active quest needs hands it over when asked,
	// unless they are hostile
	for _, q := range w.Quests {
		if q.Status != QuestActive || n.Disposition < 0 {
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
	if n.Merchant {
		for _, it := range w.ItemsOf(OwnerNPC, n.ID) {
			if !isReward(w, it.ID) && it.Kind != ItemQuest {
				st.Info = append(st.Info, fmt.Sprintf("%s sells %s for %d gold", n.Name, it.Name, BuyPrice(it, n)))
			}
		}
	}
	if p := strings.TrimSpace(a.Promise); p != "" {
		if f := w.Record(ChronicleEntry{Kind: FactPromise, Source: SourceEngine, NPCIDs: []string{n.ID},
			Text: fmt.Sprintf("%s promised %s: %s", c.Name, n.Name, trimTo(p, 200))}); f != nil {
			n.Remember(fmt.Sprintf("%s promised me: %s", c.Name, trimTo(p, 120)))
			st.Info = append(st.Info, fmt.Sprintf("%s gives %s their word (%s)", c.Name, n.Name, trimTo(p, 200)))
		}
	}
	st.Changes = append(st.Changes, fmt.Sprintf("%s talks with %s", c.Name, n.Name))
	return nil
}

func reposition(w *World, c *Character, zoneRef string, st *Step) error {
	l := w.Here()
	z := l.Zone(zoneRef)
	if z == nil {
		return fmt.Errorf("unknown zone %q", zoneRef)
	}
	if z.ID == c.ZoneID {
		return fmt.Errorf("%s is already in %s", c.Name, z.Name)
	}
	if ZoneDistance(l, c.ZoneID, z.ID) < 0 && c.ZoneID != "" {
		return fmt.Errorf("can't reach %s from here", z.Name)
	}
	c.ZoneID = z.ID
	line := fmt.Sprintf("%s moves to %s", c.Name, z.Name)
	if st.Slot == SlotDash {
		line += " (dash)"
	}
	st.Changes = append(st.Changes, line)
	return nil
}

func moveThrough(ctx *Ctx, c *Character, ref string, st *Step) error {
	w := ctx.W
	here := w.Here()
	x := w.FindExit(here.ID, ref)
	if x == nil {
		return fmt.Errorf("no exit %q here", ref)
	}
	e := x.Edge
	if e.State == EdgeLocked {
		if KeyFits(w, c, e) {
			e.State = EdgeKnown
			st.Changes = append(st.Changes, fmt.Sprintf("%s unlocks the way with %s", c.Name, e.KeyItem))
		} else if e.LairGate {
			return fmt.Errorf("the way to %s is sealed; the story must progress first", x.To.Name)
		} else {
			return fmt.Errorf("the way to %s is locked; use a key or try an ability_check to open it", x.To.Name)
		}
	}
	if w.InCombat() {
		if c.ZoneID != x.Zone && x.Zone != "" {
			return fmt.Errorf("to flee through %s you must stand in %s; reposition first", x.Label, ZoneName(here, x.Zone))
		}
		st.Changes = append(st.Changes, fmt.Sprintf("%s leads the party in a retreat to %s", c.Name, x.To.Name))
		w.Encounter = nil
		AdjustTension(w, -1)
	}
	arrive(ctx, x.To, e, st)
	return nil
}

// arrive moves the party into loc, authoring it if needed, and places
// everyone in the zone the exit enters through.
func arrive(ctx *Ctx, loc *Location, via *Edge, st *Step) {
	w := ctx.W
	from := w.Campaign.LocationID
	day := w.Campaign.Scene.Day
	if prev := w.Location(from); prev != nil && from != loc.ID {
		prev.LastDay = day
	}
	if loc.Visited && from != loc.ID && loc.LastDay > 0 && day > loc.LastDay {
		st.Info = append(st.Info, returnNote(loc, day))
	}
	w.Campaign.LocationID = loc.ID
	loc.Visited = true
	loc.LastDay = day
	if via != nil && via.State == EdgeKnown {
		via.State = EdgeOpen
	}
	if (loc.Stub || len(loc.Zones) == 0) && ctx.OnArrive != nil {
		ctx.OnArrive(loc)
	}
	if loc.Stub {
		_, _ = FillLocation(w, loc.ID, LocationFill{Description: loc.Hint})
	}
	EnsureZones(w, loc.ID)
	zone := loc.Zones[0].ID
	if via != nil {
		if z := via.ZoneAt(loc.ID); loc.Zone(z) != nil {
			zone = loc.Zone(z).ID
		}
	}
	for _, ch := range w.Characters {
		ch.ZoneID = zone
	}
	repairPlacements(w, loc)
	if w.Encounter != nil && w.Encounter.LocationID != loc.ID {
		w.Encounter = nil
	}
	s := &w.Campaign.Scene
	s.Turns, s.Quiet, s.StartTurn, s.Goal = 0, 0, w.Campaign.Turn, ""
	st.Arrived = loc.ID
	if from != loc.ID {
		st.Changes = append(st.Changes, fmt.Sprintf("The party arrives at %s (%s)", loc.Name, ZoneName(loc, zone)))
	}
	questFoes(ctx, loc, st)
}

// questFoes starts the fight with a quest's named foe when the party
// reaches the place it waits. It reports whether a fight began.
func questFoes(ctx *Ctx, loc *Location, st *Step) bool {
	w := ctx.W
	if w.InCombat() || loc.IsHub {
		return false
	}
	for _, q := range w.Quests {
		if q.Status != QuestActive {
			continue
		}
		for _, o := range q.Objectives {
			if o.Done || o.Kind != ObjDefeatEnemy || o.Where != loc.ID || w.NPC(o.Target) != nil {
				continue
			}
			specs := GenericEnemies(loc.Kind, max(1, loc.Danger), len(w.Characters))
			if len(specs) == 0 {
				specs = []EnemySpec{{HP: 12, AC: 12, AttackBonus: 3, Damage: "1d8"}}
			}
			specs[0].Name, specs[0].Count, specs[0].Tag = o.Target, 1, ""
			specs[0].HP *= 2 // a named foe outlasts the rank and file; caps still apply
			enc, _, err := StartEncounter(w, specs[:1])
			if err != nil {
				return false
			}
			st.CombatStarted = true
			st.Changes = append(st.Changes, fmt.Sprintf("%s is here: %s", o.Target, enc.Enemies[0].Display()))
			return true
		}
	}
	return false
}

func travel(ctx *Ctx, c *Character, ref string, st *Step) error {
	w := ctx.W
	if w.InCombat() {
		return fmt.Errorf("you can't travel during a fight; flee through an exit instead")
	}
	dest := w.Location(ref)
	if dest == nil {
		for _, l := range w.Locations {
			if NameContains(l.Name, ref) {
				dest = l
				break
			}
		}
	}
	if dest == nil {
		return fmt.Errorf("unknown destination %q", ref)
	}
	path, err := w.TravelPath(w.Campaign.LocationID, dest.ID)
	if err != nil {
		return err
	}
	prev := w.Campaign.LocationID
	for _, hop := range path {
		loc := w.Location(hop)
		edge := w.EdgeBetween(prev, hop)
		arrive(ctx, loc, edge, st)
		prev = hop
		if w.InCombat() {
			break
		}
		if loc.Danger > 0 {
			roll := ctx.R.Die(100)
			if roll <= loc.Danger*8 {
				st.Rolls = append(st.Rolls, Roll{Label: "Random encounter at " + loc.Name, Kind: "encounter", Total: roll, Target: loc.Danger * 8})
				if ambush(ctx, loc, st) {
					break
				}
			}
		}
	}
	AdvanceTime(w, max(1, (len(path)+1)/2))
	if w.Campaign.LocationID == dest.ID {
		st.Changes = append(st.Changes, fmt.Sprintf("%s leads the party to %s", c.Name, dest.Name))
	} else {
		st.Changes = append(st.Changes, fmt.Sprintf("The journey to %s is interrupted at %s", dest.Name, w.Here().Name))
	}
	return nil
}

func ambush(ctx *Ctx, loc *Location, st *Step) bool {
	var specs []EnemySpec
	if ctx.Ambush != nil {
		specs = ctx.Ambush(loc)
	}
	if len(specs) == 0 {
		specs = GenericEnemies(loc.Kind, loc.Danger, len(ctx.W.Characters))
	}
	enc, _, err := StartEncounter(ctx.W, specs)
	if err != nil {
		return false
	}
	var names []string
	for _, e := range enc.Enemies {
		names = append(names, e.Display())
	}
	st.Changes = append(st.Changes, fmt.Sprintf("Ambush at %s: %s", loc.Name, strings.Join(names, ", ")))
	return true
}

// npcAsEnemy turns an NPC present into a combatant.
func npcAsEnemy(w *World, n *NPC) *Enemy {
	sb := StatBlock{MaxHP: 8, AC: 11, AttackBonus: 2, Damage: "1d6"}
	if n.Stats != nil {
		sb = *n.Stats
	}
	ClampStatBlock(&sb, CapsFor(max(1, w.Here().Danger), w.PartyLevel()))
	if w.InCombat() {
		e := &Enemy{ID: w.NewID("foe"), Name: n.Name, NPCID: n.ID, StatBlock: sb, HP: sb.MaxHP, ZoneID: n.ZoneID}
		w.Encounter.Enemies = append(w.Encounter.Enemies, e)
		return e
	}
	enc, _, err := StartEncounter(w, []EnemySpec{{Name: n.Name, NPCID: n.ID, HP: sb.MaxHP, AC: sb.AC, AttackBonus: sb.AttackBonus, Damage: sb.Damage, Ranged: sb.Ranged}})
	if err != nil {
		return nil
	}
	e := enc.Enemies[0]
	if n.ZoneID != "" {
		e.ZoneID = n.ZoneID
	}
	return e
}

func findFoe(w *World, ref string, allowNPC bool) (*Enemy, error) {
	if e := w.Enemy(ref); e != nil {
		return e, nil
	}
	if allowNPC {
		if n := w.NPC(ref); n != nil && n.Alive && n.LocationID == w.Campaign.LocationID {
			for _, e := range w.encounterEnemies() {
				if e.NPCID == n.ID && !e.Defeated() {
					return e, nil
				}
			}
			n.Disposition = -3
			n.Keep(fmt.Sprintf("day %d: was attacked by the party", w.Campaign.Scene.Day))
			if e := npcAsEnemy(w, n); e != nil {
				return e, nil
			}
		}
	}
	if ref == "" {
		return nil, fmt.Errorf("attack whom?")
	}
	return nil, fmt.Errorf("no enemy %q here", ref)
}

func (w *World) encounterEnemies() []*Enemy {
	if w.Encounter == nil {
		return nil
	}
	return w.Encounter.Enemies
}

func attack(ctx *Ctx, c *Character, a Action, st *Step) error {
	w := ctx.W
	l := w.Here()
	e, err := findFoe(w, a.Target, true)
	if err != nil {
		return err
	}
	wpn := Weapon(w, c)
	if a.Item != "" {
		if it := w.FindItemOf(OwnerCharacter, c.ID, a.Item); it != nil && it.Kind == ItemWeapon {
			wpn = it
			c.Weapon = it.ID
		}
	}
	reach := InferReach(wpn)
	d := ZoneDistance(l, c.ZoneID, e.ZoneID)
	if !InReach(reach, d) {
		return fmt.Errorf("%s is %s away and %s has %s reach; reposition closer first", e.Display(), zonesWord(d), wpn.Name, ReachName(reach))
	}
	toHit, _ := AttackMod(c, wpn)
	cover := CoverBonus(l, c.ZoneID, e.ZoneID)
	out := Attack(ctx.R, fmt.Sprintf("%s attacks %s with %s", c.Name, e.Display(), wpn.Name), toHit, e.AC+cover, wpn.Damage)
	out.Roll.Parts, out.Roll.Cover = AttackParts(c, wpn), cover
	st.Rolls = append(st.Rolls, checkRoll("attack", out.Roll))
	st.Tier = out.Roll.Tier
	if !out.Hit {
		st.Changes = append(st.Changes, fmt.Sprintf("%s misses %s", c.Name, e.Display()))
		if out.Roll.Tier == dice.CritFailure {
			AdjustTension(w, 1)
		}
		return nil
	}
	dr := dmgRoll(fmt.Sprintf("%s damage", wpn.Name), "damage", *out.Damage)
	dr.Crit = out.Crit
	st.Rolls = append(st.Rolls, dr)
	dmg := out.Damage.Total
	e.HP = max(0, e.HP-dmg)
	line := fmt.Sprintf("%s hits %s for %d damage (%d/%d HP)", c.Name, e.Display(), dmg, e.HP, e.MaxHP)
	if out.Crit {
		line = "Critical hit! " + line
	}
	st.Changes = append(st.Changes, line)
	if e.HP == 0 {
		st.Changes = append(st.Changes, defeat(w, e, false)...)
	}
	return nil
}

func subdue(ctx *Ctx, c *Character, a Action, st *Step) error {
	w := ctx.W
	l := w.Here()
	e, err := findFoe(w, a.Target, false)
	if err != nil {
		return err
	}
	method, ab := SubdueMethod(a.Method)
	if ab2, ok := ParseAbility(a.Ability); ok {
		ab = ab2
	}
	d := ZoneDistance(l, c.ZoneID, e.ZoneID)
	maxD := 0
	if method == "intimidate" || method == "calm" || method == "persuade" {
		maxD = 1
	}
	if d > maxD {
		return fmt.Errorf("%s is %s away; get closer to %s it", e.Display(), zonesWord(d), strings.ReplaceAll(method, "_", " "))
	}
	dc := SubdueDC(e)
	ch := ctx.R.D20(c.CheckMod(ab), dc)
	st.Rolls = append(st.Rolls, Roll{Label: fmt.Sprintf("%s tries to %s %s (%s)", c.Name, strings.ReplaceAll(method, "_", " "), e.Display(), ab),
		Kind: "subdue", Natural: ch.Natural, Mod: ch.Mod, Total: ch.Total, Target: dc, Tier: ch.Tier, Parts: CheckParts(c, ab), Against: "DC"})
	st.Tier = ch.Tier
	if !ch.Tier.Succeeded() {
		st.Changes = append(st.Changes, fmt.Sprintf("%s fails to subdue %s", c.Name, e.Display()))
		return nil
	}
	e.Subdued = true
	st.Changes = append(st.Changes, defeat(w, e, true)...)
	return nil
}

// defeat records a downed or subdued enemy and handles NPC fate and the
// antagonist's fall.
func defeat(w *World, e *Enemy, subdued bool) []string {
	verb := "is defeated"
	if subdued {
		verb = "is subdued"
	}
	changes := []string{fmt.Sprintf("%s %s", e.Display(), verb)}
	w.Defeats = append(w.Defeats, DefeatRecord{Name: e.Name, NPCID: e.NPCID, LocationID: w.Campaign.LocationID, Turn: w.Campaign.Turn})
	if n := w.NPC(e.NPCID); n != nil && e.NPCID != "" {
		here := w.Location(w.Campaign.LocationID)
		where := w.Campaign.LocationID
		if here != nil {
			where = here.Name
		}
		if !subdued {
			n.Alive = false
			w.Record(ChronicleEntry{Kind: FactDeath, Source: SourceEngine, NPCIDs: []string{n.ID}, Text: fmt.Sprintf("%s was killed by the party at %s", n.Name, where)})
		} else {
			n.Keep(fmt.Sprintf("day %d: was subdued by the party", w.Campaign.Scene.Day))
			w.Record(ChronicleEntry{Kind: FactDeed, Source: SourceEngine, NPCIDs: []string{n.ID}, Text: fmt.Sprintf("The party subdued %s at %s", n.Name, where)})
		}
		if n.Antagonist || n.ID == w.Campaign.AntagonistID {
			changes = append(changes, completeMain(w)...)
		}
	}
	return changes
}

// completeMain finishes the storyline when the antagonist falls.
func completeMain(w *World) []string {
	changes := []string{"The antagonist has fallen!"}
	if mq := w.MainQuest(); mq != nil {
		for i := range mq.Objectives {
			mq.Objectives[i].Done = true
		}
		mq.Status = QuestTurnedIn
		keepQuestPromise(w, mq)
	}
	w.Campaign.Status = StatusCompleted
	return changes
}

func endEncounter(w *World) []string {
	l := w.Here()
	danger := 1
	if l != nil {
		danger = max(1, l.Danger)
	}
	w.Encounter = nil
	changes := []string{"The fight is won"}
	for _, c := range w.Characters {
		changes = append(changes, AwardXP(c, XPPerDanger*danger)...)
	}
	AdjustTension(w, -2)
	w.Campaign.Turns.Budget.Free = Unlimited
	return changes
}

func abilityCheck(ctx *Ctx, c *Character, a Action, st *Step) error {
	w := ctx.W
	l := w.Here()
	purpose := strings.ToLower(a.Purpose)
	ab, ok := ParseAbility(a.Ability)
	var lockEdge *Edge
	switch purpose {
	case "unlock", "force", "open":
		x := w.FindExit(l.ID, a.Target)
		if x == nil || x.Edge.State != EdgeLocked {
			// pick the only locked exit if the target was vague
			for _, ex := range w.Exits(l.ID, false) {
				if ex.Edge.State == EdgeLocked && !ex.Edge.LairGate {
					xx := ex
					x = &xx
					break
				}
			}
		}
		if x == nil || x.Edge.State != EdgeLocked {
			return fmt.Errorf("there is no locked exit %q here", a.Target)
		}
		if x.Edge.LairGate {
			return fmt.Errorf("the way to %s cannot be forced; the story must progress first", x.To.Name)
		}
		lockEdge = x.Edge
		if !ok {
			ab = DEX
			if purpose == "force" {
				ab = STR
			}
		}
	case "search":
		if !ok {
			ab = WIS
		}
	default:
		if !ok {
			ab = WIS
		}
	}
	dc := dice.SnapDC(a.DC)
	if a.DC == 0 {
		dc = 10
	}
	mod := c.CheckMod(ab)
	parts := CheckParts(c, ab)
	label := fmt.Sprintf("%s %s check", c.Name, ab)
	if lockEdge != nil {
		dc = lockEdge.LockDC
		if dc == 0 {
			dc = 15
		}
		if ab == DEX && w.FindItemOf(OwnerCharacter, c.ID, "thieves_tools") != nil {
			mod += 2
			parts = append(parts, RollPart{Label: "Thieves' tools", Value: 2})
			label += " with thieves' tools"
		}
	}
	if a.Text != "" {
		label += " to " + trimTo(a.Text, 60)
	}
	ch := ctx.R.D20(mod, dc)
	st.Rolls = append(st.Rolls, Roll{Label: label, Kind: "check", Natural: ch.Natural, Mod: ch.Mod, Total: ch.Total, Target: dc, Tier: ch.Tier, Parts: parts, Against: "DC"})
	st.Tier = ch.Tier
	switch ch.Tier {
	case dice.CritFailure:
		AdjustTension(w, 1)
	}
	if !ch.Tier.Succeeded() {
		st.Changes = append(st.Changes, fmt.Sprintf("%s's %s check fails (%d vs DC %d)", c.Name, ab, ch.Total, dc))
		return nil
	}
	st.Changes = append(st.Changes, fmt.Sprintf("%s's %s check succeeds (%d vs DC %d)", c.Name, ab, ch.Total, dc))
	if lockEdge != nil {
		lockEdge.State = EdgeKnown
		st.Changes = append(st.Changes, fmt.Sprintf("The way to %s is open", w.Location(lockEdge.Other(l.ID)).Name))
	}
	if purpose == "search" || purpose == "" || purpose == "other" {
		for _, x := range w.Exits(l.ID, true) {
			if x.Edge.State == EdgeHidden && (purpose == "search" || ch.Tier == dice.CritSuccess) {
				_ = RevealExit(w, x.Edge.ID)
				st.Changes = append(st.Changes, fmt.Sprintf("%s discovers a hidden way: %s", c.Name, firstNonEmpty(x.Label, "to "+x.To.Name)))
				break
			}
		}
	}
	return nil
}

func useItem(ctx *Ctx, c *Character, a Action, st *Step) error {
	w := ctx.W
	it := w.FindItemOf(OwnerCharacter, c.ID, a.Item)
	if it == nil {
		return fmt.Errorf("%s has no %q", c.Name, a.Item)
	}
	// keys and tools on exits
	if x := w.FindExit(w.Campaign.LocationID, a.Target); x != nil && x.Edge.State == EdgeLocked {
		if x.Edge.KeyItem != "" && !x.Edge.LairGate && (it.Key == x.Edge.KeyItem || it.ID == x.Edge.KeyItem || NameMatch(it.Name, x.Edge.KeyItem)) {
			x.Edge.State = EdgeKnown
			st.Changes = append(st.Changes, fmt.Sprintf("%s opens the way to %s with %s", c.Name, x.To.Name, it.Name))
			return nil
		}
		if strings.Contains(it.Effect, "lockpick") {
			return abilityCheck(ctx, c, Action{Kind: ActCheck, Purpose: "unlock", Target: a.Target, Ability: "DEX"}, st)
		}
		return fmt.Errorf("%s doesn't open that", it.Name)
	}
	heal := ""
	for _, part := range strings.Split(it.Effect, ";") {
		if strings.HasPrefix(part, "heal:") {
			heal = strings.TrimPrefix(part, "heal:")
		}
	}
	if heal == "" {
		st.Info = append(st.Info, fmt.Sprintf("%s uses %s; nothing mechanical happens", c.Name, it.Name))
		return nil
	}
	target := c
	if a.Target != "" {
		if t := w.Character(a.Target); t != nil {
			target = t
		}
	}
	if target.ID != c.ID && w.InCombat() && target.ZoneID != c.ZoneID {
		return fmt.Errorf("%s is in %s; move to them first", target.Name, ZoneName(w.Here(), target.ZoneID))
	}
	res, err := ctx.R.RollString(heal)
	if err != nil {
		return err
	}
	st.Rolls = append(st.Rolls, dmgRoll(it.Name, "heal", res))
	wasDown := target.Down()
	target.HP = min(target.MaxHP, target.HP+max(1, res.Total))
	line := fmt.Sprintf("%s uses %s on %s: +%d HP (%d/%d)", c.Name, it.Name, target.Name, res.Total, target.HP, target.MaxHP)
	if wasDown {
		line += fmt.Sprintf("; %s is back on their feet", target.Name)
	}
	st.Changes = append(st.Changes, line)
	if it.Kind == ItemConsumable {
		removeItem(w, it.ID)
	}
	return nil
}

func removeItem(w *World, id string) {
	for i, it := range w.Items {
		if it.ID == id {
			w.Items = append(w.Items[:i], w.Items[i+1:]...)
			break
		}
	}
	for _, c := range w.Characters {
		unequip(c, id)
	}
}

func rest(ctx *Ctx, c *Character, st *Step) error {
	w := ctx.W
	if w.InCombat() {
		return fmt.Errorf("you can't rest during a fight")
	}
	l := w.Here()
	if l.Danger == 0 {
		for _, ch := range w.Characters {
			ch.HP = ch.MaxHP
		}
		s := &w.Campaign.Scene
		s.Day++
		s.Time = 0
		st.Changes = append(st.Changes, fmt.Sprintf("The party takes a long rest at %s and wakes at dawn, fully healed", l.Name))
		AdjustTension(w, -1)
		return nil
	}
	for _, ch := range w.Characters {
		if ch.Down() {
			ch.HP = 1
		} else {
			ch.HP = min(ch.MaxHP, ch.HP+(ch.MaxHP+1)/2)
		}
	}
	AdvanceTime(w, 1)
	st.Changes = append(st.Changes, fmt.Sprintf("The party takes a short rest (%s)", w.Campaign.Scene.TimeName()))
	roll := ctx.R.Die(100)
	if roll <= l.Danger*8 {
		st.Rolls = append(st.Rolls, Roll{Label: "Random encounter while resting", Kind: "encounter", Total: roll, Target: l.Danger * 8})
		ambush(ctx, l, st)
	}
	return nil
}

// NPCJoinsFight turns an NPC present into an enemy (starting a fight if
// needed) and returns it.
func NPCJoinsFight(w *World, n *NPC) *Enemy {
	if n == nil || !n.Alive || n.LocationID != w.Campaign.LocationID {
		return nil
	}
	for _, e := range w.encounterEnemies() {
		if e.NPCID == n.ID && !e.Defeated() {
			return e
		}
	}
	return npcAsEnemy(w, n)
}

// returnNote tells the narrator how long the party has been away.
func returnNote(loc *Location, day int) string {
	if gap := day - loc.LastDay; gap > 1 {
		return fmt.Sprintf("The party returns to %s after %d days away", loc.Name, gap)
	}
	return fmt.Sprintf("The party returns to %s after a day away", loc.Name)
}
