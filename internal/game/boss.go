package game

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Solvely-Colin/aidnd/internal/dice"
)

// Bosses are single enemies built to carry a fight: more HP than the
// usual budget allows, several attacks a turn, legendary resistance, a
// signature move announced a turn ahead (so the party can get out of its
// zone), and phases that change how it fights as it is worn down.

// Boss is an enemy's boss state, kept in the enemy's extra column.
type Boss struct {
	Phase   int         `json:"phase"` // index into Phases
	Phases  []BossPhase `json:"phases"`
	Resist  int         `json:"resist"`  // legendary resistances left
	Attacks int         `json:"attacks"` // attacks per turn
	Move    BossMove    `json:"move"`
	// Charging is the signature move announced last turn; it lands at the
	// start of the boss's next turn on whoever is still in the zone.
	Charging *Telegraph `json:"charging,omitempty"`
	Cooldown int        `json:"cooldown"` // turns until it can announce again
}

// BossPhase starts once the boss's HP falls to At percent or below.
type BossPhase struct {
	At     int    `json:"at"`
	Name   string `json:"name"`
	Line   string `json:"line"` // what the party sees change
	Attack int    `json:"attack,omitempty"`
	Damage int    `json:"damage,omitempty"`
	Extra  int    `json:"extra,omitempty"`  // more attacks a turn
	Summon int    `json:"summon,omitempty"` // minions that join
}

// BossMove is the signature attack: area damage on one zone, a save for
// half.
type BossMove struct {
	Name     string  `json:"name"`
	Dice     string  `json:"dice"`
	Save     Ability `json:"save"`
	DC       int     `json:"dc"`
	Inflicts string  `json:"inflicts,omitempty"`
}

// Telegraph is a signature move on its way.
type Telegraph struct {
	Zone  string `json:"zone"`
	Round int    `json:"round"`
}

// BossSpec is how an author or the narrator asks for a boss.
type BossSpec struct {
	Move string `json:"move,omitempty"` // the signature move's name
}

// makeBoss turns e into a boss for this location and party: the stat caps
// at their top, HP near a whole encounter's budget, and the default three
// phases.
func makeBoss(w *World, e *Enemy, spec BossSpec) {
	l := w.Here()
	danger := 3
	if l != nil {
		danger = max(3, l.Danger)
	}
	level, size := w.PartyLevel(), max(1, len(w.Characters))
	caps := CapsFor(danger, level)
	e.MaxHP = max(e.MaxHP, EnemyHPBudget(danger, level, size)*3/2)
	e.HP = e.MaxHP
	e.AC = max(e.AC, caps.MaxAC-1)
	e.AttackBonus = max(e.AttackBonus, caps.MaxAttack)
	if e.Damage == "" {
		e.Damage = "1d8"
	}
	move := strings.TrimSpace(spec.Move)
	if move == "" {
		move = e.Name + "'s fury"
	}
	resist := 1
	if l != nil && l.IsLair {
		resist = 2
	}
	inflict := ""
	if danger >= 3 {
		inflict = CondProne
	}
	e.Boss = &Boss{
		Attacks: 1 + min(2, size/2),
		Resist:  resist,
		Move:    BossMove{Name: trimTo(move, 40), Dice: fmt.Sprintf("%dd6", 2+danger), Save: DEX, DC: InflictDC(e.AttackBonus) + 1, Inflicts: inflict},
		Phases: []BossPhase{
			{At: 100, Name: "Composed"},
			{At: 50, Name: "Enraged", Line: e.Name + " is bloodied and fights harder: +1 to hit, +2 damage", Attack: 1, Damage: 2},
			{At: 25, Name: "Desperate", Line: e.Name + " calls for help and strikes again each turn", Extra: 1, Summon: max(1, size/2)},
		},
	}
}

// Phase is the boss's current phase.
func (b *Boss) Current() BossPhase {
	if b == nil || len(b.Phases) == 0 {
		return BossPhase{}
	}
	return b.Phases[clamp(b.Phase, 0, len(b.Phases)-1)]
}

// bossPhase moves a hurt boss into any phase its HP has reached. It shakes
// off being prone, grappled or frightened as it changes.
func bossPhase(w *World, e *Enemy) []string {
	b := e.Boss
	if b == nil || e.Defeated() {
		return nil
	}
	var out []string
	for b.Phase+1 < len(b.Phases) && e.HP*100 <= b.Phases[b.Phase+1].At*e.MaxHP {
		b.Phase++
		p := b.Phases[b.Phase]
		e.AttackBonus += p.Attack
		e.Damage = addFlat(e.Damage, p.Damage)
		b.Attacks += p.Extra
		line := fmt.Sprintf("%s enters a new phase: %s", e.Display(), p.Name)
		if p.Line != "" {
			line += ". " + p.Line
		}
		out = append(out, line)
		for _, k := range []string{CondProne, CondFrightened, CondRestrained} {
			if e.Conditions.remove(k) {
				out = append(out, fmt.Sprintf("%s shakes off being %s", e.Display(), condName(k)))
			}
		}
		if p.Summon > 0 && w.Encounter != nil {
			out = append(out, summonMinions(w, e, p.Summon)...)
		}
	}
	return out
}

func summonMinions(w *World, boss *Enemy, n int) []string {
	var names []string
	for i := 0; i < n; i++ {
		m := &Enemy{ID: w.NewID("foe"), Name: "Servant of " + boss.Name, Tag: fmt.Sprintf("summoned %d", i+1),
			StatBlock: StatBlock{MaxHP: max(4, boss.MaxHP/10), AC: max(10, boss.AC-3), AttackBonus: max(2, boss.AttackBonus-3), Damage: "1d6"},
			ZoneID:    boss.ZoneID}
		m.HP = m.MaxHP
		// they act from next round
		m.ActedRound = w.Encounter.Round
		w.Encounter.Enemies = append(w.Encounter.Enemies, m)
		names = append(names, m.Display())
	}
	return []string{fmt.Sprintf("%s summons help: %s", boss.Display(), strings.Join(names, ", "))}
}

// addFlat adds n to a dice expression's flat bonus ("1d8+2" + 2 = "1d8+4").
func addFlat(d string, n int) string {
	if n == 0 {
		return d
	}
	ex, err := dice.Parse(d)
	if err != nil {
		return d
	}
	ex.Mod += n
	return ex.String()
}

// legendaryResist turns a boss's failed save into a success while it has
// resistances left.
func legendaryResist(e *Enemy, roll *Roll) {
	if e.Boss == nil || e.Boss.Resist <= 0 || roll.Tier.Succeeded() {
		return
	}
	e.Boss.Resist--
	roll.Tier = dice.Success
	roll.Label += fmt.Sprintf(" (legendary resistance: succeeds anyway, %d left)", e.Boss.Resist)
}

// bossTurn is a boss's whole turn: the signature move lands if it was
// announced, otherwise it attacks (several times) and may announce the
// next one.
func bossTurn(w *World, r *dice.Roller, l *Location, e *Enemy, targets []*Character, start ConditionTick, canMove bool) []EnemyTurn {
	b := e.Boss
	if start.LostTurn {
		et := EnemyTurn{EnemyID: e.ID, Enemy: e.Display(), Kind: "stunned", Text: e.Display() + " is stunned and does nothing"}
		if b.Charging != nil {
			b.Charging = nil
			et.Text += "; its " + b.Move.Name + " is broken off"
		}
		return []EnemyTurn{et}
	}
	if b.Charging != nil && b.Charging.Round < round(w) {
		return []EnemyTurn{bossMoveLands(w, r, l, e)}
	}
	var out []EnemyTurn
	move := canMove
	for i := 0; i < max(1, b.Attacks); i++ {
		tick := ConditionTick{}
		if i == 0 {
			tick = start
		}
		var live []*Character
		for _, c := range targets {
			if !c.Down() {
				live = append(live, c)
			}
		}
		if len(live) == 0 || e.Defeated() {
			break
		}
		et := enemyAct(w, r, l, e, live, tick, move)
		out = append(out, et)
		if et.Kind == "move" {
			move = false
		} else if et.Kind != "attack" {
			break
		}
	}
	if e.Defeated() {
		return out
	}
	if b.Cooldown > 0 {
		b.Cooldown--
		return out
	}
	if z := crowdedZone(w, l); z != "" {
		b.Charging = &Telegraph{Zone: z, Round: round(w)}
		out = append(out, EnemyTurn{EnemyID: e.ID, Enemy: e.Display(), Kind: "telegraph", ToZone: z,
			Text: fmt.Sprintf("%s readies %s at %s: on its next turn everyone still there takes %s (%s save DC %d for half). Get clear!",
				e.Display(), b.Move.Name, ZoneName(l, z), b.Move.Dice, b.Move.Save, b.Move.DC)})
	}
	return out
}

// crowdedZone is where most of the standing party is.
func crowdedZone(w *World, l *Location) string {
	count := map[string]int{}
	var zones []string
	for _, c := range w.Party() {
		if c.Down() {
			continue
		}
		if count[c.ZoneID] == 0 {
			zones = append(zones, c.ZoneID)
		}
		count[c.ZoneID]++
	}
	sort.SliceStable(zones, func(i, j int) bool { return count[zones[i]] > count[zones[j]] })
	if len(zones) == 0 {
		return ""
	}
	return zones[0]
}

// bossMoveLands resolves the announced signature move: one damage roll,
// a save each for half.
func bossMoveLands(w *World, r *dice.Roller, l *Location, e *Enemy) EnemyTurn {
	b := e.Boss
	z := b.Charging.Zone
	b.Charging, b.Cooldown = nil, 2
	et := EnemyTurn{EnemyID: e.ID, Enemy: e.Display(), Kind: "signature", ToZone: z}
	var caught []*Character
	for _, c := range w.Party() {
		if c.ZoneID == z && !c.Down() {
			caught = append(caught, c)
		}
	}
	if len(caught) == 0 {
		et.Text = fmt.Sprintf("%s unleashes %s on %s, but everyone got clear", e.Display(), b.Move.Name, ZoneName(l, z))
		return et
	}
	dmg := 0
	if ex, err := dice.Parse(b.Move.Dice); err == nil {
		dmg = r.Roll(ex, false).Total
	}
	parts := []string{fmt.Sprintf("%s unleashes %s on %s (%d damage)", e.Display(), b.Move.Name, ZoneName(l, z), dmg)}
	for _, c := range caught {
		roll := CharacterSave(r, c, b.Move.Save, b.Move.DC, b.Move.Name)
		roll.By = c.Name
		et.Saves = append(et.Saves, roll)
		take := dmg
		if roll.Tier.Succeeded() {
			take = dmg / 2
		}
		got, lines := HurtCharacter(w, r, c, take, e, false)
		line := fmt.Sprintf("%s %s and takes %d (%d/%d HP)", c.Name, map[bool]string{true: "saves", false: "fails"}[roll.Tier.Succeeded()], got, c.HP, c.MaxHP)
		if len(lines) > 0 {
			line += "; " + strings.Join(lines, "; ")
		}
		if c.Down() {
			line += "; " + c.Name + " goes down"
		} else if !roll.Tier.Succeeded() && b.Move.Inflicts != "" {
			c.Conditions.add(newCondition(b.Move.Inflicts, b.Move.DC, b.Move.Name, e.ID))
			line += "; " + c.Name + " is " + condName(b.Move.Inflicts)
		}
		parts = append(parts, line)
	}
	et.Text = strings.Join(parts, "; ")
	return et
}

// hasBoss reports whether the fight already has a living boss.
func hasBoss(w *World) bool {
	for _, e := range w.encounterEnemies() {
		if e.Boss != nil && !e.Defeated() {
			return true
		}
	}
	return false
}
