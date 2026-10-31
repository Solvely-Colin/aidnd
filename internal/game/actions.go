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
var ActionKinds = append([]string{ActLook, ActSay, ActEquip, ActDrop, ActEndTurn, ActMove, ActReposition, ActTravel,
	ActAttack, ActSubdue, ActCheck, ActUseItem, ActTalkTo, ActBuy, ActSell, ActAcceptQuest, ActTurnInQuest,
	ActPickUp, ActGiveItem, ActRest}, append(tacticKinds, magicKinds...)...)

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
	Skill   string `json:"skill,omitempty"`   // for checks; sets the ability
	DC      int    `json:"dc,omitempty"`
	Purpose string `json:"purpose,omitempty"` // ability_check: search | unlock | force | escape | stabilize | other
	Method  string `json:"method,omitempty"`  // subdue method; shove: prone | push
	Text    string `json:"text,omitempty"`    // speech, or what the check is for
	Promise string `json:"promise,omitempty"` // talk_to: what the character gave their word to do
	Spell   string `json:"spell,omitempty"`   // cast_spell
	Level   int    `json:"level,omitempty"`   // cast_spell slot level; use_feature amount or slot
	Feature string `json:"feature,omitempty"` // use_feature
	Option  string `json:"option,omitempty"`  // attack: smite | stun
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
	// Edge is advantage or disadvantage, Naturals both d20s, EdgeWhy the
	// conditions behind it.
	Edge     string `json:"edge,omitempty"`
	Naturals []int  `json:"naturals,omitempty"`
	EdgeWhy  string `json:"edge_why,omitempty"`
	// Foe marks an enemy's roll made during a character's step (an
	// opportunity attack) or an enemy's save against the character.
	Foe bool   `json:"foe,omitempty"`
	By  string `json:"by,omitempty"` // who rolled a foe roll
}

func checkRoll(kind string, c CheckRoll) Roll {
	return Roll{Label: c.Label, Kind: kind, Natural: c.Natural, Mod: c.Mod, Total: c.Total, Target: c.Target, Tier: c.Tier,
		Parts: c.Parts, Against: c.Against, Cover: c.Cover, Edge: c.Edge, Naturals: c.Naturals, EdgeWhy: c.EdgeWhy}
}

func dmgRoll(label, kind string, r dice.Result) Roll {
	return Roll{Label: label, Kind: kind, Dice: r.Dice, Expr: r.Expr, Mod: r.Mod, Total: r.Total}
}

// Step is the outcome of one action.
type Step struct {
	Action  Action    `json:"action"`
	Slot    Slot      `json:"slot,omitempty"`
	OK      bool      `json:"ok"`
	Error   string    `json:"error,omitempty"`
	Skipped bool      `json:"skipped,omitempty"`
	Rolls   []Roll    `json:"rolls,omitempty"`
	Tier    dice.Tier `json:"tier,omitempty"`
	Changes []string  `json:"changes,omitempty"`
	Info    []string  `json:"info,omitempty"` // facts for the narrator (look results etc.)
	// Grant adds to the turn budget after the step (action surge, dash).
	Grant         *Budget       `json:"-"`
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
	w.RefreshMagic()
	defer w.RefreshMagic()
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
		if st.Grant != nil {
			trial.Action += st.Grant.Action
			trial.Move = max(0, trial.Move+st.Grant.Move)
			trial.Bonus += st.Grant.Bonus
		}
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
	if w.InCombat() && actor.Down() {
		res.TurnEnded = true // knocked out mid-turn (an opportunity attack, an ambush): nothing left to do
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
	switch a.Kind {
	case ActCast:
		return spellSlot(c, a)
	case ActFeature:
		return featureSlot(w, c, a)
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
		return reposition(ctx, c, a.Zone, st)
	case ActMove:
		if a.Target == "" && a.Zone != "" {
			return reposition(ctx, c, a.Zone, st)
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
	case ActDodge:
		return dodge(w, c, st)
	case ActDisengage:
		return disengage(w, c, st)
	case ActHelp:
		return help(w, c, a, st)
	case ActHide:
		return hide(ctx, c, st)
	case ActReady:
		return ready(w, c, a, st)
	case ActShove:
		return shove(ctx, c, a, st)
	case ActGrapple:
		return grapple(ctx, c, a, st)
	case ActCast:
		return castSpell(ctx, c, a, st)
	case ActFeature:
		return useFeature(ctx, c, a, st)
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
	st.Info = append(st.Info, fmt.Sprintf("%s speaks with %s (%s, disposition %+d)", c.Name, n.Name, n.Role, n.AttitudeTo(c)))
	for _, t := range n.TiesTo(c.ID) {
		st.Info = append(st.Info, TieLine(w, n, t))
	}
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
		if q.Status != QuestActive || n.AttitudeTo(c) < 0 {
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
				st.Info = append(st.Info, fmt.Sprintf("%s sells %s for %d gold", n.Name, it.Name, BuyPrice(it, n, c)))
			}
		}
	}
	if p := strings.TrimSpace(a.Promise); p != "" {
		if f := w.Record(ChronicleEntry{Kind: FactPromise, Source: SourceEngine, NPCIDs: []string{n.ID},
			Text: fmt.Sprintf("%s promised %s: %s", c.Name, n.Name, trimTo(p, 200))}); f != nil {
			n.Remember(fmt.Sprintf("%s promised me: %s", c.Name, trimTo(p, 120)))
			st.Info = append(st.Info, fmt.Sprintf("%s gives %s their word (%s)", c.Name, n.Name, trimTo(p, 200)))
			Note(w, c, RecordEntry{Kind: RecChoice, Source: SourceEngine, NPCID: n.ID, Text: fmt.Sprintf("Gave %s their word: %s", n.Name, trimTo(p, 140))})
		}
	}
	st.Changes = append(st.Changes, fmt.Sprintf("%s talks with %s", c.Name, n.Name))
	return nil
}

func reposition(ctx *Ctx, c *Character, zoneRef string, st *Step) error {
	w := ctx.W
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
	if err := CanMove(c.Conditions); err != nil {
		return fmt.Errorf("%s is %v", c.Name, err)
	}
	if fz, ok := frightOf(w, c.Conditions); ok && ZoneDistance(l, z.ID, fz) < ZoneDistance(l, c.ZoneID, fz) {
		return fmt.Errorf("%s is too frightened to move closer to %s", c.Name, c.Conditions.Get(CondFrightened).Source)
	}
	if ctx != nil && opportunityAttacks(ctx, c, st) {
		return nil // cut down before they could get away
	}
	st.Changes = append(st.Changes, releaseGrapples(w, c)...)
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
	if err := CanMove(c.Conditions); err != nil {
		return fmt.Errorf("%s is %v", c.Name, err)
	}
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
		if opportunityAttacks(ctx, c, st) {
			return nil // cut down before they could get away
		}
		st.Changes = append(st.Changes, fmt.Sprintf("%s leads the party in a retreat to %s", c.Name, x.To.Name))
		st.Changes = append(st.Changes, StabilizeDying(w)...)
		w.Encounter = nil
		ClearFightConditions(w)
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
	if from != loc.ID {
		noticeHidden(w, loc, st)
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
	for _, ch := range w.Party() {
		if err := CanMove(ch.Conditions); err != nil {
			return fmt.Errorf("%s is %v", ch.Name, err)
		}
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
	surprise(ctx, enc, st)
	return true
}

// surprise pits the ambushers' Stealth against the party's best passive
// Perception. Spotted, they are seen in time; unseen, each gets a free
// action before anyone can react.
func surprise(ctx *Ctx, enc *Encounter, st *Step) {
	w := ctx.W
	watcher, pp := BestPassive(w)
	if watcher == nil || len(enc.Enemies) == 0 {
		return
	}
	sneak := 0
	for _, e := range enc.Enemies {
		sneak = max(sneak, e.AttackBonus/2)
	}
	ch := ctx.R.D20(sneak, pp)
	st.Rolls = append(st.Rolls, Roll{Label: fmt.Sprintf("Ambushers' Stealth against %s's passive Perception", watcher.Name), Kind: "check", Natural: ch.Natural,
		Mod: ch.Mod, Total: ch.Total, Target: pp, Tier: flip(ch.Tier), Against: "DC", Foe: true, By: "The ambushers"})
	if !ch.Tier.Succeeded() {
		st.Changes = append(st.Changes, fmt.Sprintf("%s spots the ambush in time (passive Perception %d)", watcher.Name, pp))
		return
	}
	st.Changes = append(st.Changes, "The party is caught by surprise: the ambushers strike first")
	l := w.Here()
	var targets []*Character
	for _, c := range w.Party() {
		if !c.Down() {
			targets = append(targets, c)
		}
	}
	for _, e := range enc.Enemies {
		live := targets[:0:0]
		for _, c := range targets {
			if !c.Down() {
				live = append(live, c)
			}
		}
		if len(live) == 0 {
			break
		}
		et := enemyAct(w, ctx.R, l, e, live, ConditionTick{}, true)
		st.Changes = append(st.Changes, et.Text)
		if et.Attack != nil {
			r := checkRoll("attack", et.Attack.Roll)
			r.Foe, r.By = true, e.Display()
			st.Rolls = append(st.Rolls, r)
		}
	}
}

// noticeHidden lets the party's best passive Perception spot a hidden way
// on arriving somewhere new.
func noticeHidden(w *World, loc *Location, st *Step) {
	watcher, pp := BestPassive(w)
	if watcher == nil || pp < NoticeDC(loc) {
		return
	}
	for _, x := range w.Exits(loc.ID, true) {
		if x.Edge.State == EdgeHidden && !x.Edge.LairGate {
			_ = RevealExit(w, x.Edge.ID)
			st.Changes = append(st.Changes, fmt.Sprintf("%s notices a hidden way: %s (passive Perception %d)", watcher.Name, firstNonEmpty(x.Label, "to "+x.To.Name), pp))
			return
		}
	}
}

// NoticeDC is the passive Perception that spots a hidden way on arrival.
func NoticeDC(loc *Location) int { return 15 + clamp(loc.Danger, 0, 5) }

// npcAsEnemy turns an NPC present into a combatant.
func npcAsEnemy(w *World, n *NPC) *Enemy {
	sb := StatBlock{MaxHP: 8, AC: 11, AttackBonus: 2, Damage: "1d6"}
	if n.Stats != nil {
		sb = *n.Stats
	}
	ClampStatBlock(&sb, CapsFor(max(1, w.Here().Danger), w.PartyLevel()))
	sb.Inflicts, _ = NormalizeInflict(sb.Inflicts, w.Here().Danger)
	// the antagonist (and any NPC authored as one) fights as a boss
	boss := sb.AsBoss
	if boss == nil && (n.Antagonist || n.ID == w.Campaign.AntagonistID) {
		boss = &BossSpec{}
	}
	sb.AsBoss = nil
	if w.InCombat() {
		e := &Enemy{ID: w.NewID("foe"), Name: n.Name, NPCID: n.ID, StatBlock: sb, HP: sb.MaxHP, ZoneID: n.ZoneID}
		e.Horror = sb.HorrorRank
		if boss != nil && !hasBoss(w) {
			makeBoss(w, e, *boss)
		}
		w.Encounter.Enemies = append(w.Encounter.Enemies, e)
		return e
	}
	enc, _, err := StartEncounter(w, []EnemySpec{{Name: n.Name, NPCID: n.ID, HP: sb.MaxHP, AC: sb.AC, AttackBonus: sb.AttackBonus, Damage: sb.Damage, Ranged: sb.Ranged, Inflicts: sb.Inflicts, Boss: boss, Horror: sb.HorrorRank}})
	if err != nil {
		return nil
	}
	e := enc.Enemies[0]
	if n.ZoneID != "" {
		e.ZoneID = n.ZoneID
	}
	return e
}

// findFoe finds an enemy by ref. With by set, an NPC present may be
// attacked too: they turn hostile and hold a grudge against by.
func findFoe(w *World, ref string, by *Character) (*Enemy, error) {
	if e := w.Enemy(ref); e != nil {
		return e, nil
	}
	if by != nil {
		if n := w.NPC(ref); n != nil && n.Alive && n.LocationID == w.Campaign.LocationID {
			for _, e := range w.encounterEnemies() {
				if e.NPCID == n.ID && !e.Defeated() {
					return e, nil
				}
			}
			n.Disposition = -3
			n.Keep(fmt.Sprintf("day %d: was attacked by the party", w.Campaign.Scene.Day))
			n.Bind(w, by, TieGrudge, "attacked them", SourceEngine)
			Note(w, by, RecordEntry{Kind: RecChoice, Source: SourceEngine, NPCID: n.ID, Text: fmt.Sprintf("Turned on %s and attacked them at %s", n.Name, placeName(w))})
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
	e, err := findFoe(w, a.Target, c)
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
	if c.Conditions.Has(CondWildShape) {
		wpn = &Item{Name: "claws", Kind: ItemWeapon, Damage: "1d8", Ability: WIS}
	}
	reach := InferReach(wpn)
	d := ZoneDistance(l, c.ZoneID, e.ZoneID)
	if !InReach(reach, d) {
		return fmt.Errorf("%s is %s away and %s has %s reach; reposition closer first", e.Display(), zonesWord(d), wpn.Name, ReachName(reach))
	}
	if e.Conditions.Has(CondInvisible) {
		return fmt.Errorf("%s can't be seen", e.Display())
	}
	opts := strikeOpts{Smite: a.Option == "smite" && c.HasFeature("divine_smite"), Stun: a.Option == "stun"}
	if opts.Smite && lowestSlot(c, 1) == 0 {
		return fmt.Errorf("%s has no spell slot left to smite with", c.Name)
	}
	n := attacksPerAction(c)
	if IsFocus(wpn) {
		n = 1 // a focus bolt is a cantrip, not a weapon attack
	}
	for i := 0; i < n; i++ {
		if e.Defeated() {
			if e = nextFoeInReach(w, c, reach); e == nil {
				break
			}
		}
		if strike(w, ctx.R, c, e, wpn, opts, st) && opts.Smite {
			opts.Smite = false // one smite per attack action is plenty
		}
	}
	return nil
}

// nextFoeInReach is another living foe c can reach, for a second attack.
func nextFoeInReach(w *World, c *Character, reach Reach) *Enemy {
	if w.Encounter == nil {
		return nil
	}
	l := w.Here()
	for _, e := range w.Encounter.Living() {
		if InReach(reach, ZoneDistance(l, c.ZoneID, e.ZoneID)) && !e.Conditions.Has(CondInvisible) {
			return e
		}
	}
	return nil
}

func subdue(ctx *Ctx, c *Character, a Action, st *Step) error {
	w := ctx.W
	l := w.Here()
	e, err := findFoe(w, a.Target, nil)
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
	if e.Conditions.Has(CondStunned) || e.Conditions.Has(CondRestrained) {
		dc -= 2 // easier to pin or talk down a foe that can't fight back
	}
	parts, what := CheckParts(c, ab), string(ab)
	if sk, ok := SkillByKey(subdueSkill(method)); ok && sk.Ability == ab {
		parts, what = SkillParts(c, sk), sk.Name
	}
	ch, parts, why := charCheck(ctx.R, c, ab, parts, dc)
	c.Conditions.remove(CondHelped)
	st.Rolls = append(st.Rolls, Roll{Label: fmt.Sprintf("%s tries to %s %s (%s)", c.Name, strings.ReplaceAll(method, "_", " "), e.Display(), what),
		Kind: "subdue", Natural: ch.Natural, Mod: ch.Mod, Total: ch.Total, Target: dc, Tier: ch.Tier, Parts: parts, Against: "DC",
		Edge: ch.Edge, Naturals: ch.Naturals, EdgeWhy: why})
	st.Tier = ch.Tier
	if !ch.Tier.Succeeded() {
		st.Changes = append(st.Changes, fmt.Sprintf("%s fails to subdue %s", c.Name, e.Display()))
		return nil
	}
	e.Subdued = true
	st.Changes = append(st.Changes, defeat(w, e, true)...)
	if e.NPCID != "" || e.Boss != nil {
		Note(w, c, RecordEntry{Kind: RecChoice, Source: SourceEngine, NPCID: e.NPCID, Text: fmt.Sprintf("Spared %s and subdued them at %s", e.Name, placeName(w))})
	}
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
			changes = append(changes, RepairStory(w)...)
		} else {
			n.Keep(fmt.Sprintf("day %d: was subdued by the party", w.Campaign.Scene.Day))
			w.Record(ChronicleEntry{Kind: FactDeed, Source: SourceEngine, NPCIDs: []string{n.ID}, Text: fmt.Sprintf("The party subdued %s at %s", n.Name, where)})
		}
		if q := w.StorylineOf(n.ID); q != nil {
			changes = append(changes, completeMain(w, q)...)
		} else if w.IsBoss(n.ID) {
			changes = append(changes, completeMain(w, w.MainQuest())...)
		}
	}
	return changes
}

// completeMain finishes a storyline when its antagonist falls. A one-shot
// campaign ends there; a saga's open world goes on, and the fall is
// recorded so the world remembers it.
func completeMain(w *World, mq *Quest) []string {
	changes := []string{"The antagonist has fallen!"}
	if mq != nil {
		for i := range mq.Objectives {
			o := &mq.Objectives[i]
			if !o.Done {
				o.Done = true
				if lvl := w.Campaign.Milestones[o.ID]; lvl > 0 {
					for _, c := range w.Characters {
						changes = append(changes, RaiseToLevel(c, lvl)...)
					}
				}
			}
		}
		done := mq.Status == QuestCompleted || mq.Status == QuestTurnedIn
		mq.Status = QuestTurnedIn
		keepQuestPromise(w, mq)
		if !done {
			tideOnQuest(w, mq)
		}
	}
	if w.Campaign.Saga == "" {
		w.Campaign.Status = StatusCompleted
		return changes
	}
	if mq != nil {
		changes[0] = "The storyline \"" + mq.Title + "\" is over"
		w.Record(ChronicleEntry{Kind: FactDeed, Source: SourceEngine, QuestID: mq.ID, Text: "The party ended the storyline \"" + mq.Title + "\""})
	}
	return changes
}

// EndFight ends the encounter as won, with its rewards.
func EndFight(w *World) []string { return endEncounter(w) }

func endEncounter(w *World) []string {
	l := w.Here()
	danger := 1
	if l != nil {
		danger = max(1, l.Danger)
	}
	changes := append([]string{"The fight is won"}, StabilizeDying(w)...)
	changes = append(changes, rollLoot(w)...)
	w.Encounter = nil
	ClearFightConditions(w)
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
	var patient *Character
	escapeDC := 0
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
	case "escape":
		r := c.Conditions.Get(CondRestrained)
		if r == nil {
			return fmt.Errorf("%s isn't restrained", c.Name)
		}
		if !ok || (ab != STR && ab != DEX) {
			ab = STR
			if c.Mod(DEX) > c.Mod(STR) {
				ab = DEX
			}
		}
		escapeDC = max(10, r.SaveDC)
	case "stabilize":
		t := w.Character(a.Target)
		if t == nil || !t.Dying() {
			for _, ch := range w.Party() {
				if ch.Dying() && ch.ID != c.ID {
					t = ch
					break
				}
			}
		}
		if t == nil || !t.Dying() {
			return fmt.Errorf("nobody here is dying")
		}
		if w.InCombat() && t.ZoneID != c.ZoneID {
			return fmt.Errorf("%s is in %s; move to them first", t.Name, ZoneName(l, t.ZoneID))
		}
		patient = t
	case "search":
		if !ok {
			ab = WIS
		}
	default:
		if !ok {
			ab = WIS
		}
	}
	// a skill sets the ability; a purpose's usual skill applies when the
	// ability asked for is the skill's own
	sk, hasSkill := SkillByKey(a.Skill)
	if !hasSkill {
		if def, found := SkillByKey(purposeSkill(purpose)); found && (!ok || def.Ability == ab) {
			sk, hasSkill = def, true
		}
	}
	if hasSkill && escapeDC > 0 && sk.Key != "athletics" && sk.Key != "acrobatics" {
		hasSkill = false
	}
	if hasSkill {
		ab = sk.Ability
	}
	dc := dice.SnapDC(a.DC)
	if a.DC == 0 {
		dc = 10
	}
	if patient != nil {
		dc = 10
	}
	parts := CheckParts(c, ab)
	label := fmt.Sprintf("%s %s check", c.Name, ab)
	if hasSkill {
		parts = SkillParts(c, sk)
		label = fmt.Sprintf("%s %s check", c.Name, sk.Name)
	}
	if lockEdge != nil {
		dc = lockEdge.LockDC
		if dc == 0 {
			dc = 15
		}
		if ab == DEX && w.FindItemOf(OwnerCharacter, c.ID, "thieves_tools") != nil {
			parts = append(parts, RollPart{Label: "Thieves' tools", Value: 2})
			label += " with thieves' tools"
		}
	}
	if escapeDC > 0 {
		dc = escapeDC
		if a.Text == "" {
			a.Text = "break free"
		}
	}
	if a.Text != "" {
		label += " to " + trimTo(a.Text, 60)
	}
	ch, parts, why := charCheck(ctx.R, c, ab, parts, dc)
	c.Conditions.remove(CondHelped)
	st.Rolls = append(st.Rolls, Roll{Label: label, Kind: "check", Natural: ch.Natural, Mod: ch.Mod, Total: ch.Total, Target: dc, Tier: ch.Tier, Parts: parts, Against: "DC",
		Edge: ch.Edge, Naturals: ch.Naturals, EdgeWhy: why})
	st.Tier = ch.Tier
	switch ch.Tier {
	case dice.CritFailure:
		AdjustTension(w, 1)
	}
	what := string(ab)
	if hasSkill {
		what = sk.Name
	}
	if !ch.Tier.Succeeded() {
		st.Changes = append(st.Changes, fmt.Sprintf("%s's %s check fails (%d vs DC %d)", c.Name, what, ch.Total, dc))
		return nil
	}
	st.Changes = append(st.Changes, fmt.Sprintf("%s's %s check succeeds (%d vs DC %d)", c.Name, what, ch.Total, dc))
	if patient != nil {
		Stabilize(patient)
		st.Changes = append(st.Changes, fmt.Sprintf("%s stabilizes %s, who stops dying but stays unconscious", c.Name, patient.Name))
		return nil
	}
	if escapeDC > 0 {
		c.Conditions.remove(CondRestrained)
		st.Changes = append(st.Changes, fmt.Sprintf("%s breaks free", c.Name))
		return nil
	}
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
	if target.Dead {
		return fmt.Errorf("%s is dead; no potion can help now", target.Name)
	}
	_, woke := Heal(target, res.Total)
	line := fmt.Sprintf("%s uses %s on %s: +%d HP (%d/%d)", c.Name, it.Name, target.Name, res.Total, target.HP, target.MaxHP)
	if woke {
		line += fmt.Sprintf("; %s is back on their feet", target.Name)
	}
	if strings.Contains(it.Effect, "revive") && target.Conditions.remove(CondPoisoned) {
		line += fmt.Sprintf("; %s is no longer poisoned", target.Name)
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
		ClearAllConditions(w)
		for _, ch := range w.Characters {
			LongRest(ch)
			SetHP(ch, ch.MaxHP)
			RecoverStrain(ch, 1)
		}
		s := &w.Campaign.Scene
		s.Day++
		s.Time = 0
		st.Changes = append(st.Changes, fmt.Sprintf("The party takes a long rest at %s and wakes at dawn, fully healed", l.Name))
		AdjustTension(w, -1)
		return nil
	}
	clearShortRest(w)
	for _, ch := range w.Characters {
		if ch.Dead {
			continue
		}
		if ch.Down() {
			SetHP(ch, 1)
		} else {
			SetHP(ch, ch.HP+(ch.MaxHP+1)/2)
		}
	}
	AdvanceTime(w, 1)
	st.Changes = append(st.Changes, fmt.Sprintf("The party takes a short rest (%s)", w.Campaign.Scene.TimeName()))
	for _, ch := range w.Characters {
		if !ch.Dead {
			st.Changes = append(st.Changes, ShortRest(ch)...)
		}
	}
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
