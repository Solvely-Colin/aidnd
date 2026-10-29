package game

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Solvely-Colin/aidnd/internal/dice"
)

// Slot is which part of the turn budget an action uses.
type Slot string

const (
	SlotFree   Slot = "free"
	SlotMove   Slot = "move"
	SlotAction Slot = "action"
	SlotDash   Slot = "dash" // a move paid with the action
	SlotNone   Slot = "none" // costs nothing at all (end_turn)
	SlotBonus  Slot = "bonus"
)

// CombatFreeActions is the free-action allowance in combat.
const CombatFreeActions = 3

// FullBudget is a fresh turn's budget.
func FullBudget(inCombat bool) Budget {
	if inCombat {
		return Budget{Free: CombatFreeActions, Move: 1, Action: 1, Bonus: 1}
	}
	return Budget{Free: Unlimited, Move: 1, Action: 1, Bonus: 1}
}

// Pay spends the slot from b, returning the slot actually used (a move
// becomes a dash when the move is gone but the action is not).
func (b *Budget) Pay(s Slot) (Slot, error) {
	switch s {
	case SlotNone:
		return s, nil
	case SlotFree:
		if b.Free == Unlimited {
			return s, nil
		}
		if b.Free > 0 {
			b.Free--
			return s, nil
		}
		return "", fmt.Errorf("no free actions left this turn")
	case SlotMove:
		if b.Move > 0 {
			b.Move--
			return s, nil
		}
		if b.Action > 0 {
			b.Action--
			return SlotDash, nil
		}
		return "", fmt.Errorf("no move left this turn")
	case SlotDash:
		if b.Move > 0 && b.Action > 0 {
			b.Move--
			b.Action--
			return s, nil
		}
		return "", fmt.Errorf("moving two zones needs both your move and your action")
	case SlotAction:
		if b.Action > 0 {
			b.Action--
			return s, nil
		}
		return "", fmt.Errorf("no action left this turn")
	case SlotBonus:
		if b.Bonus > 0 {
			b.Bonus--
			return s, nil
		}
		return "", fmt.Errorf("no bonus action left this turn")
	}
	return "", fmt.Errorf("unknown slot %q", s)
}

// Left describes the budget in words for prompts ("LEFT THIS TURN").
func (b Budget) Left() string {
	free := "unlimited"
	if b.Free != Unlimited {
		free = fmt.Sprint(b.Free)
	}
	return fmt.Sprintf("action %d, bonus action %d, move %d, free %s", b.Action, b.Bonus, b.Move, free)
}

// Spent reports whether the action is gone (which ends the turn).
func (b Budget) Spent() bool { return b.Action <= 0 }

// ActiveCharacter is whoever's turn it is, or nil before start.
func (w *World) ActiveCharacter() *Character {
	t := w.Campaign.Turns
	if len(t.Order) == 0 {
		return nil
	}
	return w.Character(t.Order[t.Index%len(t.Order)])
}

// StartGame fixes seat order and begins the first turn.
func StartGame(w *World, now time.Time) error {
	if len(w.Characters) == 0 {
		return fmt.Errorf("no characters")
	}
	for _, p := range w.Players {
		if p.CharacterID == "" {
			return fmt.Errorf("%s has no character yet", p.Name)
		}
	}
	order := make([]string, 0, len(w.Players))
	for _, p := range w.Players {
		order = append(order, p.CharacterID)
	}
	w.Campaign.Turns = TurnState{Order: order, Budget: FullBudget(w.InCombat()), Started: now.Unix()}
	w.Campaign.Status = StatusActive
	return nil
}

// EndTurn advances to the next conscious character and resets the budget.
// It returns nil if everyone is down.
func EndTurn(w *World, now time.Time) *Character {
	t := &w.Campaign.Turns
	w.Campaign.Turn++
	n := len(t.Order)
	if n == 0 {
		return nil
	}
	for i := 1; i <= n; i++ {
		idx := (t.Index + i) % n
		if c := w.Character(t.Order[idx]); c != nil && (!c.Down() || (c.Dying() && w.InCombat())) {
			if idx <= t.Index && w.Encounter != nil {
				w.Encounter.Round++
			}
			t.Index = idx
			t.Budget = FullBudget(w.InCombat())
			t.Started = now.Unix()
			return c
		}
	}
	return nil
}

// AllDown reports a party wipe.
func AllDown(w *World) bool {
	for _, c := range w.Characters {
		if !c.Down() {
			return false
		}
	}
	return len(w.Characters) > 0
}

// AdvanceTime moves the clock forward by segments of the day.
func AdvanceTime(w *World, segments int) {
	s := &w.Campaign.Scene
	s.Time += segments
	for s.Time >= len(TimesOfDay) {
		s.Time -= len(TimesOfDay)
		s.Day++
	}
}

// AdjustTension changes tension within 0..10.
func AdjustTension(w *World, delta int) {
	w.Campaign.Scene.Tension = clamp(w.Campaign.Scene.Tension+delta, 0, 10)
}

// Setback handles a party wipe: wake at the hub at dawn with half HP and a
// quarter less gold. The MaxSetbacks-th wipe fails the campaign.
func Setback(w *World) []string {
	c := &w.Campaign
	c.Setbacks++
	w.Encounter = nil
	if c.MaxSetbacks <= 0 {
		c.MaxSetbacks = 3
	}
	if c.Setbacks >= c.MaxSetbacks {
		c.Status = StatusFailed
		return []string{fmt.Sprintf("The party has fallen for the %s time. The campaign is lost.", ordinal(c.Setbacks))}
	}
	changes := []string{fmt.Sprintf("The party falls (setback %d of %d) and wakes at dawn in safety.", c.Setbacks, c.MaxSetbacks)}
	c.LocationID = c.HubID
	hub := w.Location(c.HubID)
	EnsureZones(w, c.HubID)
	for _, ch := range w.Characters {
		if ch.Dead {
			continue
		}
		SetHP(ch, max(1, ch.MaxHP/2))
		lost := ch.Gold / 4
		ch.Gold -= lost
		if hub != nil && len(hub.Zones) > 0 {
			ch.ZoneID = hub.Zones[0].ID
		}
		changes = append(changes, fmt.Sprintf("%s: %d/%d HP, lost %d gold", ch.Name, ch.HP, ch.MaxHP, lost))
	}
	ClearAllConditions(w)
	c.Scene.Day++
	c.Scene.Time = 0
	c.Scene.Tension = 2
	c.Scene.Turns, c.Scene.Quiet, c.Scene.StartTurn = 0, 0, c.Turn
	c.Turns.Budget = FullBudget(false)
	return changes
}

func ordinal(n int) string {
	switch n {
	case 1:
		return "1st"
	case 2:
		return "2nd"
	case 3:
		return "3rd"
	}
	return fmt.Sprintf("%dth", n)
}

// EnemySpec is an authored enemy (spawn_encounter directive).
type EnemySpec struct {
	Name        string `json:"name"`
	Tag         string `json:"tag,omitempty"`
	Count       int    `json:"count,omitempty"`
	HP          int    `json:"hp"`
	AC          int    `json:"ac"`
	AttackBonus int    `json:"attack_bonus"`
	Damage      string `json:"damage"`
	Ranged      bool   `json:"ranged,omitempty"`
	NPCID       string `json:"npc_id,omitempty"`
	// Inflicts is a condition each hit forces a save against: poisoned,
	// prone, restrained, frightened, blinded, or stunned (danger 3+).
	Inflicts string `json:"inflicts,omitempty"`
	// Boss asks for one boss (dangerous places only, one per fight).
	Boss *BossSpec `json:"boss,omitempty"`
	// Horror (1..3) tests the party's nerve on first sight (dread campaigns).
	Horror int `json:"horror,omitempty"`
}

// StartEncounter creates a fight here from specs, capping stat blocks by
// danger and party level and budgeting total HP by party size. Enemies
// start 1 zone from the party (2 if ranged), as far as the layout allows.
func StartEncounter(w *World, specs []EnemySpec) (*Encounter, []string, error) {
	if w.InCombat() {
		return nil, nil, fmt.Errorf("a fight is already under way")
	}
	l := w.Here()
	if l == nil {
		return nil, nil, fmt.Errorf("party has no location")
	}
	EnsureZones(w, l.ID)
	danger := max(1, l.Danger)
	caps := CapsFor(danger, w.PartyLevel())
	var blocks []StatBlock
	var meta []EnemySpec
	bossAt := -1
	for _, s := range specs {
		n := clamp(s.Count, 1, 4)
		if s.Boss != nil {
			if bossAt >= 0 || (danger < 3 && (s.NPCID == "" || s.NPCID != w.Campaign.AntagonistID)) {
				s.Boss = nil
			} else {
				n, bossAt = 1, len(blocks)
			}
		}
		for i := 0; i < n; i++ {
			blocks = append(blocks, StatBlock{MaxHP: s.HP, AC: s.AC, AttackBonus: s.AttackBonus, Damage: s.Damage, Ranged: s.Ranged, Inflicts: s.Inflicts})
			meta = append(meta, s)
		}
	}
	if len(blocks) == 0 {
		return nil, nil, fmt.Errorf("no enemies given")
	}
	var notes []string
	for i := range blocks {
		notes = append(notes, ClampStatBlock(&blocks[i], caps)...)
		var note string
		if blocks[i].Inflicts, note = NormalizeInflict(blocks[i].Inflicts, danger); note != "" {
			notes = append(notes, note)
		}
	}
	budget := EnemyHPBudget(danger, w.PartyLevel(), len(w.Characters))
	if bossAt >= 0 {
		// the boss brings its own HP; its followers share a third
		rest := append(append([]StatBlock{}, blocks[:bossAt]...), blocks[bossAt+1:]...)
		rest = BudgetEnemies(rest, max(len(rest), budget/3))
		blocks = append(append(append([]StatBlock{}, rest[:bossAt]...), blocks[bossAt]), rest[bossAt:]...)
	} else {
		blocks = BudgetEnemies(blocks, budget)
	}
	enc := &Encounter{ID: w.NewID("enc"), LocationID: l.ID, Round: 1}
	partyZone := partyZone(w)
	for i, b := range blocks {
		s := meta[i]
		name := trimTo(s.Name, 40)
		if name == "" {
			name = "Foe"
		}
		e := &Enemy{ID: w.NewID("foe"), Name: name, Tag: trimTo(s.Tag, 30), StatBlock: b, HP: b.MaxHP}
		e.Horror = clamp(s.Horror, 0, 3)
		if n := w.NPC(s.NPCID); n != nil && n.LocationID == l.ID {
			e.NPCID = n.ID
		}
		want := 1
		if b.Ranged {
			want = 2
		}
		e.ZoneID = zoneAtDistance(l, partyZone, want)
		if i == bossAt {
			makeBoss(w, e, *s.Boss)
		}
		enc.Enemies = append(enc.Enemies, e)
	}
	TagEnemies(enc.Enemies)
	for _, c := range w.Characters {
		c.ReactedRound, c.Readied = 0, nil
	}
	w.Encounter = enc
	if b := &w.Campaign.Turns.Budget; b.Free == Unlimited || b.Free > CombatFreeActions {
		b.Free = CombatFreeActions
	}
	AdjustTension(w, 2)
	return enc, notes, nil
}

// partyZone is where most conscious characters stand.
func partyZone(w *World) string {
	count := map[string]int{}
	best := ""
	for _, c := range w.Party() {
		if c.Down() {
			continue
		}
		count[c.ZoneID]++
		if best == "" || count[c.ZoneID] > count[best] {
			best = c.ZoneID
		}
	}
	if l := w.Here(); best == "" && l != nil && len(l.Zones) > 0 {
		best = l.Zones[0].ID
	}
	return best
}

// zoneAtDistance picks the first zone exactly d away, else the farthest.
func zoneAtDistance(l *Location, from string, d int) string {
	bestZ, bestD := from, 0
	for _, z := range l.Zones {
		zd := ZoneDistance(l, from, z.ID)
		if zd == d {
			return z.ID
		}
		if zd > bestD && zd < d {
			bestZ, bestD = z.ID, zd
		}
	}
	if bestD == 0 {
		for _, z := range l.Zones {
			if zd := ZoneDistance(l, from, z.ID); zd > bestD {
				bestZ, bestD = z.ID, zd
			}
		}
	}
	return bestZ
}

// GenericEnemies is the fallback when the author can't supply enemies.
func GenericEnemies(kind string, danger, partySize int) []EnemySpec {
	danger = clamp(danger, 1, 5)
	name, inflicts := "Bandit", ""
	switch kind {
	case KindCave:
		name, inflicts = "Cave Spider", CondPoisoned
	case KindDungeon, KindRuin:
		name = "Skeleton"
	case KindWilderness:
		name, inflicts = "Wolf", CondProne
	}
	n := clamp((partySize+1)/2+danger/3, 1, 4)
	return []EnemySpec{{Name: name, Count: n, HP: 5 + 3*danger, AC: 11 + danger/2, AttackBonus: 2 + danger/2, Damage: fmt.Sprintf("1d6+%d", danger/2), Inflicts: inflicts}}
}

// EnemyTurn is one engine-resolved enemy line.
type EnemyTurn struct {
	EnemyID  string         `json:"enemy_id"`
	Enemy    string         `json:"enemy"`
	Kind     string         `json:"kind"` // move | attack | hold | stunned
	TargetID string         `json:"target_id,omitempty"`
	Target   string         `json:"target,omitempty"`
	Attack   *AttackOutcome `json:"attack,omitempty"`
	ToZone   string         `json:"to_zone,omitempty"`
	Text     string         `json:"text"`
	// Save is the target's save against the attack's condition, if any;
	// Before and After are condition effects on the enemy itself.
	Save      *Roll    `json:"save,omitempty"`
	Inflicted string   `json:"inflicted,omitempty"`
	Before    []string `json:"before,omitempty"` // start of its turn: stunned, standing up
	After     []string `json:"after,omitempty"`  // end of its turn: saves, wearing off
	Rolls     []Roll   `json:"rolls,omitempty"`  // the enemy's own saves
	// Reactions are characters' automatic attacks during this enemy's turn
	// (opportunity attacks as it leaves, readied attacks as it comes close).
	Reactions []Reaction `json:"reactions,omitempty"`
	// Saves are characters' saves against a boss's signature move.
	Saves []Roll `json:"saves,omitempty"`
}

// enemiesActingNow spreads the enemies over the round: each acts once per
// round, a share after each conscious character's turn, and whoever has
// not acted yet goes after the last one.
func enemiesActingNow(w *World) []*Enemy {
	enc := w.Encounter
	var waiting []*Enemy
	for _, e := range enc.Living() {
		if e.ActedRound < enc.Round {
			waiting = append(waiting, e)
		}
	}
	t := w.Campaign.Turns
	later := 0
	for i := t.Index + 1; i < len(t.Order); i++ {
		if c := w.Character(t.Order[i]); c != nil && !c.Down() {
			later++
		}
	}
	n := (len(waiting) + later) / (later + 1) // ceil(waiting / turns left in the round)
	waiting = waiting[:n]
	for _, e := range waiting {
		e.ActedRound = enc.Round
	}
	return waiting
}

// RunEnemyTurns lets the enemies whose turn it is act: attack a character
// in reach (sometimes the weakest), otherwise step one zone toward the
// nearest. Conditions apply at the start and end of each enemy's turn, and
// a hit from an enemy that inflicts a condition forces a save.
func RunEnemyTurns(w *World, r *dice.Roller) ([]EnemyTurn, []string) {
	if !w.InCombat() {
		return nil, nil
	}
	l := w.Here()
	var out []EnemyTurn
	changes := FaceEnemyHorrors(w, r)
	w.RefreshMagic()
	for _, e := range enemiesActingNow(w) {
		var targets, hidden []*Character
		for _, c := range w.Party() {
			switch {
			case c.Down():
			case c.Conditions.Has(CondHidden) || c.Conditions.Has(CondInvisible):
				hidden = append(hidden, c)
			default:
				targets = append(targets, c)
			}
		}
		if len(targets)+len(hidden) == 0 {
			break
		}
		start, canMove := startEnemyTurn(e)
		var ets []EnemyTurn
		switch {
		case len(targets) == 0:
			ets = []EnemyTurn{{EnemyID: e.ID, Enemy: e.Display(), Kind: "hold", Text: e.Display() + " searches for its hidden foes"}}
		case e.Boss != nil:
			ets = bossTurn(w, r, l, e, targets, start, canMove)
		default:
			ets = []EnemyTurn{enemyAct(w, r, l, e, targets, start, canMove)}
		}
		if len(ets) == 0 {
			ets = []EnemyTurn{{EnemyID: e.ID, Enemy: e.Display(), Kind: "hold", Text: e.Display() + " holds position"}}
		}
		end := endEnemyTurn(r, e)
		ets[0].Before = start.Changes
		ets[len(ets)-1].After, ets[len(ets)-1].Rolls = end.Changes, end.Rolls
		changes = append(changes, start.Changes...)
		for _, et := range ets {
			for _, re := range et.Reactions {
				changes = append(changes, re.Text)
			}
			switch et.Kind {
			case "attack", "move", "telegraph", "signature":
				changes = append(changes, et.Text)
			}
		}
		changes = append(changes, end.Changes...)
		out = append(out, ets...)
	}
	if w.Encounter != nil && len(w.Encounter.Living()) == 0 {
		changes = append(changes, endEncounter(w)...) // reactions finished them
	}
	return out, changes
}

// enemyAct is one enemy's action.
func enemyAct(w *World, r *dice.Roller, l *Location, e *Enemy, targets []*Character, start ConditionTick, canMove bool) EnemyTurn {
	if start.LostTurn {
		return EnemyTurn{EnemyID: e.ID, Enemy: e.Display(), Kind: "stunned", Text: e.Display() + " is stunned and does nothing"}
	}
	// a readied attack triggers before it can act if it is already close
	if re := readiedAgainst(w, r, e); len(re) > 0 {
		if e.Defeated() {
			return EnemyTurn{EnemyID: e.ID, Enemy: e.Display(), Kind: "down", Text: e.Display() + " falls before it can act", Reactions: re}
		}
		et := enemyAct(w, r, l, e, targets, start, canMove)
		et.Reactions = append(re, et.Reactions...)
		return et
	}
	reach := ReachMelee
	if e.Ranged {
		reach = ReachRanged
	}
	var inReach []*Character
	for _, c := range targets {
		if InReach(reach, ZoneDistance(l, e.ZoneID, c.ZoneID)) {
			inReach = append(inReach, c)
		}
	}
	if len(inReach) > 0 {
		t := inReach[r.Intn(len(inReach))]
		if r.Intn(4) == 0 {
			sort.SliceStable(inReach, func(i, j int) bool { return inReach[i].HP < inReach[j].HP })
			t = inReach[0]
		}
		cover := CoverBonus(l, e.ZoneID, t.ZoneID)
		edge, why := AttackEdge(e.Conditions, t.Conditions, ZoneDistance(l, e.ZoneID, t.ZoneID))
		out1 := AttackWithEdge(r, fmt.Sprintf("%s attacks %s", e.Display(), t.Name), e.AttackBonus, ArmorClass(w, t)+cover, e.Damage, edge, why)
		out1.Roll.Parts, out1.Roll.Cover = []RollPart{{Label: "Attack bonus", Value: e.AttackBonus}}, cover
		et := EnemyTurn{EnemyID: e.ID, Enemy: e.Display(), Kind: "attack", TargetID: t.ID, Target: t.Name, Attack: &out1}
		if !out1.Hit {
			et.Text = fmt.Sprintf("%s attacks %s and misses", e.Display(), t.Name)
			return et
		}
		if t.Conditions.Has(CondParalyzed) && ZoneDistance(l, e.ZoneID, t.ZoneID) == 0 && !out1.Crit {
			out1.Crit = true // paralyzed: hits up close are critical
			if ex, err := dice.Parse(e.Damage); err == nil {
				more := r.Roll(dice.Expr{Count: ex.Count, Sides: ex.Sides}, false)
				out1.Damage.Total += more.Total
				out1.Damage.Dice = append(out1.Damage.Dice, more.Dice...)
			}
		}
		dmg, lines := HurtCharacter(w, r, t, out1.Damage.Total, e, ZoneDistance(l, e.ZoneID, t.ZoneID) == 0)
		et.Text = fmt.Sprintf("%s hits %s for %d damage (%d/%d HP)", e.Display(), t.Name, dmg, t.HP, t.MaxHP)
		if out1.Crit {
			et.Text = "Critical! " + et.Text
		}
		if len(lines) > 0 {
			et.Text += "; " + strings.Join(lines, "; ")
		}
		if t.Down() {
			et.Text += fmt.Sprintf("; %s goes down", t.Name)
			if lines := releaseGrapples(w, t); len(lines) > 0 {
				et.Text += "; " + strings.Join(lines, "; ")
			}
			return et
		}
		if e.Inflicts != "" {
			if so := InflictOnCharacter(r, t, e.Inflicts, InflictDC(e.AttackBonus), e.Display(), e.ID); so != nil {
				et.Save = &so.Roll
				if so.Applied {
					et.Inflicted = e.Inflicts
				}
				et.Text += "; " + so.Line
			}
		}
		return et
	}
	// close in on the nearest
	sort.SliceStable(targets, func(i, j int) bool {
		return ZoneDistance(l, e.ZoneID, targets[i].ZoneID) < ZoneDistance(l, e.ZoneID, targets[j].ZoneID)
	})
	t := targets[0]
	step := NextStep(l, e.ZoneID, t.ZoneID)
	if fz, ok := frightOf(w, e.Conditions); ok && ZoneDistance(l, step, fz) < ZoneDistance(l, e.ZoneID, fz) {
		step = e.ZoneID // too scared to close in
	}
	if step == e.ZoneID || !canMove {
		return EnemyTurn{EnemyID: e.ID, Enemy: e.Display(), Kind: "hold", Text: e.Display() + " holds position"}
	}
	reacts := enemyLeaves(w, r, e)
	if e.Defeated() {
		return EnemyTurn{EnemyID: e.ID, Enemy: e.Display(), Kind: "down", Text: e.Display() + " is cut down as it tries to move", Reactions: reacts}
	}
	e.ZoneID = step
	txt := fmt.Sprintf("%s moves to %s, toward %s", e.Display(), ZoneName(l, step), t.Name)
	reacts = append(reacts, readiedAgainst(w, r, e)...)
	if e.Defeated() {
		txt += " and is struck down"
	}
	return EnemyTurn{EnemyID: e.ID, Enemy: e.Display(), Kind: "move", TargetID: t.ID, Target: t.Name, ToZone: step, Text: txt, Reactions: reacts}
}
