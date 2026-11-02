package game

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Solvely-Colin/aidnd/internal/dice"
)

// Action kinds for spells and class features.
const (
	ActCast    = "cast_spell"
	ActFeature = "use_feature"
)

var magicKinds = []string{ActCast, ActFeature}

// rangeZones is how many zones away a spell reaches.
func rangeZones(r string) int {
	switch r {
	case RangeNear:
		return 1
	case RangeFar:
		return 2
	}
	return 0
}

// SpellByRef finds a spell by key, name or its name in any era.
func SpellByRef(ref string) *SpellDef {
	if s := Spells[ref]; s != nil {
		return s
	}
	k := strings.NewReplacer(" ", "_", "-", "_", "'", "").Replace(strings.ToLower(strings.TrimSpace(ref)))
	if s := Spells[k]; s != nil {
		return s
	}
	for _, key := range spellOrder {
		s := Spells[key]
		if NameMatch(s.Name, ref) {
			return s
		}
		if s.Era != nil {
			for _, n := range s.Era {
				if n != "" && NameMatch(n, ref) {
					return s
				}
			}
		}
	}
	return nil
}

// spellSlot is what casting costs from the turn budget.
func spellSlot(c *Character, a Action) (Slot, error) {
	s := SpellByRef(a.Spell)
	if s == nil {
		return "", fmt.Errorf("unknown spell %q", a.Spell)
	}
	if s.Peace {
		return SlotAction, nil
	}
	return s.Cast, nil
}

// castLevel decides the slot level for casting s: the asked level, else
// the lowest slot c has. It returns 0 for cantrips and free casts, and
// the free resource used, if any.
func castLevel(c *Character, s *SpellDef, asked int) (int, string, error) {
	if s.Level == 0 {
		return 0, "", nil
	}
	if s.Key == "hunters_mark" && resourceLeft(c, "favored_enemy") > 0 && asked <= 1 {
		return 1, "favored_enemy", nil
	}
	if p := c.Picks["mi_spell"]; len(p) > 0 && p[0] == s.Key && resourceLeft(c, "mi_spell") > 0 && asked <= 1 && lowestSlot(c, 1) == 0 {
		return 1, "mi_spell", nil
	}
	if asked > 0 {
		if asked < s.Level {
			asked = s.Level
		}
		if asked > 3 || SlotsLeft(c)[asked-1] == 0 {
			return 0, "", fmt.Errorf("%s has no level %d spell slot left", c.Name, asked)
		}
		return asked, "", nil
	}
	if l := lowestSlot(c, s.Level); l > 0 {
		return l, "", nil
	}
	if MaxSpellLevel(c) < s.Level {
		return 0, "", fmt.Errorf("%s can't cast %s yet", c.Name, s.Name)
	}
	return 0, "", fmt.Errorf("%s is out of spell slots for %s; a rest brings them back", c.Name, s.Name)
}

// spellDice is the spell's damage or healing dice at slot level lvl, with
// cantrips doubling at character level 5.
func spellDice(c *Character, s *SpellDef, lvl int) string {
	e, err := dice.Parse(s.Dice)
	if err != nil {
		return s.Dice
	}
	if s.Level == 0 && c.Level >= 5 && s.Key != "eldritch_blast" {
		e.Count *= 2
	}
	if up, err := dice.Parse(s.Upcast); err == nil && lvl > s.Level {
		e.Count += up.Count * (lvl - s.Level)
	}
	return e.String()
}

// castSpell resolves cast_spell.
func castSpell(ctx *Ctx, c *Character, a Action, st *Step) error {
	w := ctx.W
	s := SpellByRef(a.Spell)
	if s == nil {
		return fmt.Errorf("unknown spell %q", a.Spell)
	}
	setting := w.Setting()
	name := SpellName(s, setting)
	if !c.KnowsSpell(s.Key) {
		return fmt.Errorf("%s doesn't know %s", c.Name, name)
	}
	if s.Peace && w.InCombat() {
		return fmt.Errorf("%s takes too long to cast in a fight", name)
	}
	if s.Key == "divine_smite" {
		return fmt.Errorf("smite when you hit: attack with the smite option")
	}
	lvl, free, err := castLevel(c, s, a.Level)
	if err != nil {
		return err
	}
	// work out the targets before spending anything
	plan, err := planSpell(w, c, s, a)
	if err != nil {
		return err
	}
	switch {
	case free != "":
		spend(c, free, 1)
	case lvl > 0:
		spendSlot(c, lvl)
	}
	c.Conditions.remove(CondInvisible)
	c.Conditions.remove(CondHidden)
	head := fmt.Sprintf("%s casts %s", c.Name, name)
	if lvl > s.Level && s.Level > 0 {
		head += fmt.Sprintf(" with a level %d slot", lvl)
	}
	st.Changes = append(st.Changes, head)
	if s.Conc {
		st.Changes = append(st.Changes, startConcentration(w, c, s.Key, lvl, plan.zone)...)
	}
	r := ctx.R
	switch {
	case s.Kind == SpellAttack:
		spellAttacks(w, r, c, s, lvl, plan.enemy, st)
	case s.Kind == SpellSave && s.Area:
		areaSave(w, r, c, s, lvl, plan.zone, st)
	case s.Kind == SpellSave:
		saveOne(w, r, c, s, lvl, plan.enemy, st)
	case s.Kind == SpellHeal:
		healSpell(r, c, s, lvl, plan.allies, st)
	case s.Kind == SpellBuff:
		for _, t := range plan.allies {
			cond := Condition{Kind: s.Cond, Rounds: condRules[s.Cond].Rounds, Source: name, SourceID: c.ID}
			if s.Conc {
				cond.Spell = s.Key
			}
			t.Conditions.add(cond)
			st.Changes = append(st.Changes, fmt.Sprintf("%s is %s", t.Name, condName(s.Cond)))
		}
	case s.Kind == SpellUtility:
		st.Info = append(st.Info, fmt.Sprintf("%s: %s", name, s.Desc))
	default:
		return specialSpell(ctx, c, s, lvl, plan, st)
	}
	if c.Conc != nil && c.Conc.Spell == s.Key && s.Area && plan.zone != "" {
		c.Conc.Zone = plan.zone
	}
	return nil
}

// spellPlan is who and where a spell lands.
type spellPlan struct {
	enemy  *Enemy
	allies []*Character
	zone   string
	exit   *Edge
}

// planSpell resolves a spell's targets and checks its range.
func planSpell(w *World, c *Character, s *SpellDef, a Action) (spellPlan, error) {
	var p spellPlan
	l := w.Here()
	reach := rangeZones(s.Range)
	switch {
	case s.Key == "knock":
		x := w.FindExit(w.Campaign.LocationID, firstNonEmpty(a.Target, a.Text))
		if x == nil {
			return p, fmt.Errorf("open what? name a locked way out")
		}
		p.exit = x.Edge
		return p, nil
	case s.Key == "misty_step":
		if l == nil || l.Zone(a.Zone) == nil {
			return p, fmt.Errorf("misty step to which zone?")
		}
		p.zone = l.Zone(a.Zone).ID
		return p, nil
	case s.Kind == SpellUtility:
		return p, nil
	case s.Area && s.Range == RangeSelf || s.Key == "spirit_guardians":
		p.zone = c.ZoneID
		return p, nil
	case s.Area:
		if l != nil && a.Zone != "" && l.Zone(a.Zone) != nil {
			p.zone = l.Zone(a.Zone).ID
		} else if e, err := findFoe(w, a.Target, nil); err == nil {
			p.zone = e.ZoneID
		} else if e := nearestFoe(w, c); e != nil {
			p.zone = e.ZoneID
		}
		if p.zone == "" {
			return p, fmt.Errorf("there is no one to catch in %s", s.Name)
		}
		if d := ZoneDistance(l, c.ZoneID, p.zone); d > reach {
			return p, fmt.Errorf("%s is out of range (%s away)", ZoneName(l, p.zone), zonesWord(d))
		}
		return p, nil
	case s.Kind == SpellAttack, s.Kind == SpellSave, s.Key == "hunters_mark", s.Key == "hex", s.Key == "magic_missile", s.Key == "scorching_ray", s.Key == "spiritual_weapon":
		e, err := findFoe(w, a.Target, c)
		if err != nil && a.Target == "" {
			e = nearestFoe(w, c)
			if e == nil {
				return p, fmt.Errorf("no foe to target")
			}
		} else if err != nil {
			return p, err
		}
		if e.Conditions.Has(CondInvisible) {
			return p, fmt.Errorf("%s can't be seen", e.Display())
		}
		if d := ZoneDistance(l, c.ZoneID, e.ZoneID); d > reach {
			return p, fmt.Errorf("%s is out of range for %s (%s away)", e.Display(), s.Name, zonesWord(d))
		}
		p.enemy = e
		return p, nil
	case s.Key == "dispel_magic" && a.Target != "" && w.Character(a.Target) == nil:
		e, err := findFoe(w, a.Target, nil)
		if err != nil {
			return p, err
		}
		p.enemy = e
		return p, nil
	}
	// spells on allies
	n := max(1, s.Targets)
	if s.Range == RangeSelf {
		p.allies = []*Character{c}
		return p, nil
	}
	inRange := func(t *Character) bool { return ZoneDistance(l, c.ZoneID, t.ZoneID) <= reach }
	if t := w.Character(a.Target); t != nil {
		if !inRange(t) {
			return p, fmt.Errorf("%s is out of range for %s", t.Name, s.Name)
		}
		p.allies = append(p.allies, t)
	}
	if s.Targets > 1 {
		cands := w.Party()
		sort.SliceStable(cands, func(i, j int) bool { return cands[i].MaxHP-cands[i].HP > cands[j].MaxHP-cands[j].HP })
		for _, t := range cands {
			if len(p.allies) >= n {
				break
			}
			if t.Dead || containsChar(p.allies, t) || !inRange(t) {
				continue
			}
			if s.Kind == SpellHeal && t.HP >= t.MaxHP {
				continue
			}
			p.allies = append(p.allies, t)
		}
	}
	if len(p.allies) == 0 {
		if a.Target != "" {
			return p, fmt.Errorf("no ally %q here", a.Target)
		}
		p.allies = []*Character{c}
	}
	for _, t := range p.allies {
		if t.Dead && s.Key != "revivify" {
			return p, fmt.Errorf("%s is beyond %s", t.Name, s.Name)
		}
	}
	return p, nil
}

func containsChar(xs []*Character, c *Character) bool {
	for _, x := range xs {
		if x == c {
			return true
		}
	}
	return false
}

// nearestFoe is the closest living, visible enemy.
func nearestFoe(w *World, c *Character) *Enemy {
	if w.Encounter == nil {
		return nil
	}
	l := w.Here()
	var best *Enemy
	for _, e := range w.Encounter.Living() {
		if e.Conditions.Has(CondInvisible) {
			continue
		}
		if best == nil || ZoneDistance(l, c.ZoneID, e.ZoneID) < ZoneDistance(l, c.ZoneID, best.ZoneID) {
			best = e
		}
	}
	return best
}

// spellAttackRoll is one spell attack against e; it returns the hit and
// whether it crit.
func spellAttackRoll(w *World, r *dice.Roller, c *Character, e *Enemy, label string, st *Step) (bool, bool) {
	l := w.Here()
	d := ZoneDistance(l, c.ZoneID, e.ZoneID)
	parts := append(SpellAttackParts(c), rollBonusDice(r, c, "attack")...)
	cover := CoverBonus(l, c.ZoneID, e.ZoneID)
	edgeN, why := AttackEdge(c.Conditions, e.Conditions, d)
	if c.Conditions.Has(CondSorcery) && edgeN <= 0 {
		edgeN++
		why = strings.TrimLeft(why+", innate sorcery", ", ")
	}
	ch := charD20(r, c, sumParts(parts), e.AC+cover, edgeN)
	roll := Roll{Label: label, Kind: "attack", Natural: ch.Natural, Mod: ch.Mod, Total: ch.Total, Target: ch.Target, Tier: ch.Tier,
		Parts: parts, Against: "AC", Cover: cover, Edge: ch.Edge, Naturals: ch.Naturals}
	if ch.Edge != "" {
		roll.EdgeWhy = why
	}
	c.Conditions.remove(CondHelped)
	e.Conditions.remove(CondLit)
	st.Rolls = append(st.Rolls, roll)
	if st.Tier == "" || ch.Tier.Succeeded() {
		st.Tier = ch.Tier
	}
	return ch.Tier.Succeeded(), ch.Tier == dice.CritSuccess
}

// rollDamage rolls expr (doubled dice on a crit) as a damage roll on st.
func rollDamage(r *dice.Roller, label, expr string, mod int, crit bool, st *Step) int {
	e, err := dice.Parse(expr)
	if err != nil {
		e = dice.MustParse("1d4")
	}
	res := r.Roll(e, crit)
	res.Mod += mod
	res.Total = max(0, res.Total+mod)
	dr := dmgRoll(label, "damage", res)
	dr.Crit = crit
	st.Rolls = append(st.Rolls, dr)
	return res.Total
}

// markBonus is the extra 1d6 when c hits a foe they marked or hexed.
func markBonus(r *dice.Roller, c *Character, e *Enemy, crit bool, st *Step) int {
	if m := e.Conditions.Get(CondMarked); m != nil && m.SourceID == c.ID && m.Spell == "hex" {
		return rollDamage(r, firstOf(m.Source, "Hex"), "1d6", 0, crit, st)
	}
	return 0
}

// spellAttacks resolves an attack-roll spell: one attack, or eldritch
// blast's beams at level 5.
func spellAttacks(w *World, r *dice.Roller, c *Character, s *SpellDef, lvl int, e *Enemy, st *Step) {
	name := SpellName(s, w.Setting())
	beams := 1
	if s.Key == "eldritch_blast" && c.Level >= 5 {
		beams = 2
	}
	for i := 0; i < beams; i++ {
		if e == nil || e.Defeated() {
			if e = nearestFoe(w, c); e == nil {
				return
			}
		}
		hit, crit := spellAttackRoll(w, r, c, e, fmt.Sprintf("%s hurls %s at %s", c.Name, name, e.Display()), st)
		if !hit {
			st.Changes = append(st.Changes, fmt.Sprintf("%s misses %s", name, e.Display()))
			continue
		}
		mod := 0
		if s.Key == "eldritch_blast" && c.HasPick("agonizing_blast") {
			mod = c.Mod(CHA)
		}
		dmg := rollDamage(r, name+" damage", spellDice(c, s, lvl), mod, crit, st)
		dmg += markBonus(r, c, e, crit, st)
		lines := hurtEnemy(w, c, e, dmg)
		line := fmt.Sprintf("%s hits %s for %d damage", name, e.Display(), dmg)
		if crit {
			line = "Critical hit! " + line
		}
		st.Changes = append(st.Changes, line+lines[0])
		st.Changes = append(st.Changes, lines[1:]...)
		if e.Defeated() {
			continue
		}
		switch {
		case s.Cond != "":
			e.Conditions.add(Condition{Kind: s.Cond, Rounds: condRules[s.Cond].Rounds, Source: name, SourceID: c.ID})
			st.Changes = append(st.Changes, fmt.Sprintf("%s is %s", e.Display(), condName(s.Cond)))
		case s.Key == "shocking_grasp":
			e.ReactedRound = round(w)
		case s.Key == "eldritch_blast" && c.HasPick("repelling_blast"):
			if z := awayFrom(w.Here(), c.ZoneID, e.ZoneID); z != "" {
				e.ZoneID = z
				st.Changes = append(st.Changes, fmt.Sprintf("%s is hurled back to %s", e.Display(), ZoneName(w.Here(), z)))
			}
		}
	}
}

// awayFrom is a zone next to from that is farther from origin, or "".
func awayFrom(l *Location, origin, from string) string {
	if l == nil {
		return ""
	}
	for _, z := range l.Zones {
		if ZoneDistance(l, from, z.ID) == 1 && ZoneDistance(l, origin, z.ID) > ZoneDistance(l, origin, from) {
			return z.ID
		}
	}
	return ""
}

// spellSave makes e save against c's spell; it applies damage (half on a
// save when allowed) and the condition on a failure.
func spellSave(w *World, r *dice.Roller, c *Character, s *SpellDef, e *Enemy, dmg int, st *Step) {
	name := SpellName(s, w.Setting())
	dc := SpellDC(c)
	roll := EnemySave(r, e, s.Save, dc, name)
	roll.Foe, roll.By = true, e.Display()
	st.Rolls = append(st.Rolls, roll)
	saved := roll.Tier.Succeeded()
	if s.Dice != "" {
		take := dmg
		if saved {
			take = 0
			if s.Half || (s.Level == 0 && c.HasFeature("potent_cantrip")) {
				take = dmg / 2
			}
		}
		if s.Key == "toll_the_dead" && !saved && e.HP < e.MaxHP {
			take += r.Die(4) // the wounded hear a louder bell (d12 instead of d8, roughly)
		}
		if take > 0 {
			lines := hurtEnemy(w, c, e, take)
			verb := "fails the save"
			if saved {
				verb = "saves"
			}
			st.Changes = append(st.Changes, fmt.Sprintf("%s %s and takes %d damage", e.Display(), verb, take)+lines[0])
			st.Changes = append(st.Changes, lines[1:]...)
		} else {
			st.Changes = append(st.Changes, fmt.Sprintf("%s shrugs off %s (%s save %d vs DC %d)", e.Display(), name, s.Save, roll.Total, dc))
		}
	}
	if saved || e.Defeated() {
		if s.Dice == "" {
			st.Changes = append(st.Changes, fmt.Sprintf("%s resists %s (%s save %d vs DC %d)", e.Display(), name, s.Save, roll.Total, dc))
		}
		return
	}
	if s.Key == "thunderwave" {
		if z := awayFrom(w.Here(), c.ZoneID, e.ZoneID); z != "" {
			e.ZoneID = z
			st.Changes = append(st.Changes, fmt.Sprintf("%s is blasted back to %s", e.Display(), ZoneName(w.Here(), z)))
		}
	}
	if s.Cond == "" {
		return
	}
	cond := Condition{Kind: s.Cond, Rounds: condRules[s.Cond].Rounds, Source: name, SourceID: c.ID}
	if s.Conc {
		cond.Spell = s.Key
	}
	if condRules[s.Cond].SaveEnds {
		cond.SaveDC, cond.Save = dc, s.Save
	}
	if s.Cond == CondFrightened || s.Cond == CondRestrained {
		cond.Rounds = 10
	}
	e.Conditions.add(cond)
	st.Changes = append(st.Changes, fmt.Sprintf("%s is %s", e.Display(), condName(s.Cond)))
}

// saveOne is a single-target save spell.
func saveOne(w *World, r *dice.Roller, c *Character, s *SpellDef, lvl int, e *Enemy, st *Step) {
	dmg := 0
	if s.Dice != "" {
		dmg = rollDamage(r, SpellName(s, w.Setting())+" damage", spellDice(c, s, lvl), 0, false, st)
		dmg += markBonus(r, c, e, false, st)
	}
	spellSave(w, r, c, s, e, dmg, st)
	if s.Level == 0 {
		st.Tier = dice.Success
	}
}

// areaSave is a save spell on every foe in zone; the damage is rolled
// once, as in 5e.
func areaSave(w *World, r *dice.Roller, c *Character, s *SpellDef, lvl int, zone string, st *Step) {
	var hit []*Enemy
	if w.Encounter != nil {
		for _, e := range w.Encounter.Living() {
			if e.ZoneID == zone {
				hit = append(hit, e)
			}
		}
	}
	name := SpellName(s, w.Setting())
	if len(hit) == 0 {
		st.Changes = append(st.Changes, fmt.Sprintf("%s catches no one in %s", name, ZoneName(w.Here(), zone)))
		return
	}
	dmg := 0
	if s.Dice != "" {
		dmg = rollDamage(r, name+" damage", spellDice(c, s, lvl), 0, false, st)
	}
	for _, e := range hit {
		spellSave(w, r, c, s, e, dmg, st)
	}
	st.Tier = dice.Success
}

// healSpell heals allies: dice + casting modifier (+2 + level for a life
// cleric).
func healSpell(r *dice.Roller, c *Character, s *SpellDef, lvl int, allies []*Character, st *Step) {
	bonus := c.Mod(c.CastAbility())
	if c.HasFeature("disciple_of_life") && s.Level > 0 {
		bonus += 2 + lvl
	}
	ex, err := dice.Parse(spellDice(c, s, lvl))
	if err != nil {
		return
	}
	for _, t := range allies {
		res := r.Roll(ex, false)
		res.Mod += bonus
		res.Total = max(1, res.Total+bonus)
		st.Rolls = append(st.Rolls, dmgRoll(fmt.Sprintf("%s heals %s", s.Name, t.Name), "heal", res))
		got, woke := Heal(t, res.Total)
		line := fmt.Sprintf("%s regains %d HP (%d/%d)", t.Name, got, t.HP, t.MaxHP)
		if woke {
			line += " and is back on their feet"
		}
		st.Changes = append(st.Changes, line)
	}
}

// specialSpell resolves the spells with their own rules.
func specialSpell(ctx *Ctx, c *Character, s *SpellDef, lvl int, p spellPlan, st *Step) error {
	w, r := ctx.W, ctx.R
	name := SpellName(s, w.Setting())
	switch s.Key {
	case "spare_the_dying":
		t := firstAlly(p.allies)
		if t == nil || !Stabilize(t) {
			return fmt.Errorf("spare the dying needs a dying ally")
		}
		st.Changes = append(st.Changes, t.Name+" is stable")
	case "magic_missile":
		e := p.enemy
		total := 0
		for i := 0; i < 3+lvl-1; i++ {
			total += rollDamage(r, fmt.Sprintf("%s dart %d", name, i+1), "1d4+1", 0, false, st)
		}
		lines := hurtEnemy(w, c, e, total)
		st.Changes = append(st.Changes, fmt.Sprintf("%d darts strike %s for %d damage", 3+lvl-1, e.Display(), total)+lines[0])
		st.Changes = append(st.Changes, lines[1:]...)
		st.Tier = dice.Success
	case "scorching_ray":
		e := p.enemy
		for i := 0; i < 3+lvl-2; i++ {
			if e == nil || e.Defeated() {
				if e = nearestFoe(w, c); e == nil {
					break
				}
			}
			hit, crit := spellAttackRoll(w, r, c, e, fmt.Sprintf("%s fires ray %d at %s", c.Name, i+1, e.Display()), st)
			if !hit {
				st.Changes = append(st.Changes, fmt.Sprintf("Ray %d misses %s", i+1, e.Display()))
				continue
			}
			dmg := rollDamage(r, fmt.Sprintf("Ray %d damage", i+1), "2d6", 0, crit, st)
			lines := hurtEnemy(w, c, e, dmg)
			st.Changes = append(st.Changes, fmt.Sprintf("Ray %d burns %s for %d damage", i+1, e.Display(), dmg)+lines[0])
			st.Changes = append(st.Changes, lines[1:]...)
		}
	case "hunters_mark", "hex":
		e := p.enemy
		e.Conditions.remove(CondMarked)
		e.Conditions.add(Condition{Kind: CondMarked, Rounds: 10, Source: name, SourceID: c.ID, Spell: s.Key})
		st.Changes = append(st.Changes, fmt.Sprintf("%s is marked by %s", e.Display(), c.Name))
	case "armor_of_agathys":
		hp := 5 * lvl
		c.TempHP = max(c.TempHP, hp)
		c.Conditions.remove(CondAgathys)
		c.Conditions.add(Condition{Kind: CondAgathys, Die: hp, Source: name, SourceID: c.ID})
		st.Changes = append(st.Changes, fmt.Sprintf("%s is sheathed in frost (%d temporary HP)", c.Name, hp))
	case "misty_step":
		l := w.Here()
		st.Changes = append(st.Changes, releaseGrapples(w, c)...)
		c.ZoneID = p.zone
		st.Changes = append(st.Changes, fmt.Sprintf("%s vanishes and reappears in %s", c.Name, ZoneName(l, p.zone)))
	case "spiritual_weapon":
		spiritStrike(w, r, c, p.enemy, lvl, st)
	case "lesser_restoration":
		t := firstAlly(p.allies)
		fixed := false
		for _, k := range []string{CondPoisoned, CondBlinded, CondParalyzed} {
			if t.Conditions.remove(k) {
				fixed = true
				st.Changes = append(st.Changes, fmt.Sprintf("%s is no longer %s", t.Name, k))
			}
		}
		if !fixed {
			st.Changes = append(st.Changes, t.Name+" feels refreshed, though nothing ailed them")
		}
	case "aid":
		hp := 5 * (lvl - 1)
		for _, t := range p.allies {
			if t.Dead {
				continue
			}
			t.TempHP = max(t.TempHP, hp)
			st.Changes = append(st.Changes, fmt.Sprintf("%s gains %d temporary HP", t.Name, hp))
		}
	case "knock":
		if p.exit == nil || p.exit.State != EdgeLocked {
			return fmt.Errorf("that way isn't locked")
		}
		if p.exit.LairGate {
			return fmt.Errorf("that way is sealed by more than a lock")
		}
		p.exit.State = EdgeKnown
		st.Changes = append(st.Changes, fmt.Sprintf("With a loud knock the way to %s springs open", w.Location(p.exit.Other(w.Campaign.LocationID)).Name))
	case "revivify":
		t := firstAlly(p.allies)
		if t == nil || !t.Dead {
			return fmt.Errorf("revivify needs an ally who has died")
		}
		if c.Gold < 300 {
			return fmt.Errorf("revivify needs 300 gold of diamonds and %s has %d gold", c.Name, c.Gold)
		}
		c.Gold -= 300
		t.Dead, t.Death, t.HP = false, nil, 1
		st.Changes = append(st.Changes, fmt.Sprintf("%s gasps back to life with 1 HP (300 gold of diamonds turn to dust)", t.Name))
		w.Record(ChronicleEntry{Kind: FactDeed, Source: SourceEngine, Text: fmt.Sprintf("%s brought %s back from death", c.Name, t.Name)})
	case "spirit_guardians":
		areaSave(w, r, c, s, lvl, c.ZoneID, st)
	case "dispel_magic":
		var cs *Conditions
		who := ""
		if p.enemy != nil {
			cs, who = &p.enemy.Conditions, p.enemy.Display()
		} else if t := firstAlly(p.allies); t != nil {
			cs, who = &t.Conditions, t.Name
		}
		if cs == nil {
			st.Info = append(st.Info, name+": "+s.Desc)
			return nil
		}
		n := 0
		keep := (*cs)[:0]
		for _, x := range *cs {
			if containsStr(EffectKinds, x.Kind) || x.Spell != "" {
				n++
				continue
			}
			keep = append(keep, x)
		}
		*cs = keep
		st.Changes = append(st.Changes, fmt.Sprintf("%d magical effect(s) on %s unravel", n, who))
	default:
		st.Info = append(st.Info, name+": "+s.Desc)
	}
	return nil
}

func firstAlly(xs []*Character) *Character {
	if len(xs) == 0 {
		return nil
	}
	return xs[0]
}

// spiritStrike is a spiritual weapon's attack.
func spiritStrike(w *World, r *dice.Roller, c *Character, e *Enemy, lvl int, st *Step) {
	if e == nil || e.Defeated() {
		if e = nearestFoe(w, c); e == nil {
			return
		}
	}
	s := Spells["spiritual_weapon"]
	name := SpellName(s, w.Setting())
	hit, crit := spellAttackRoll(w, r, c, e, fmt.Sprintf("%s's %s strikes at %s", c.Name, name, e.Display()), st)
	if !hit {
		st.Changes = append(st.Changes, fmt.Sprintf("%s misses %s", name, e.Display()))
		return
	}
	dmg := rollDamage(r, name+" damage", spellDice(c, s, max(2, lvl)), c.Mod(c.CastAbility()), crit, st)
	lines := hurtEnemy(w, c, e, dmg)
	st.Changes = append(st.Changes, fmt.Sprintf("%s hits %s for %d damage", name, e.Display(), dmg)+lines[0])
	st.Changes = append(st.Changes, lines[1:]...)
}
