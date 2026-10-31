package game

import (
	"fmt"
	"strings"

	"github.com/Solvely-Colin/aidnd/internal/dice"
)

// Class rules in combat: armor class, attack bonuses, damage riders
// (sneak attack, rage, smite...), damage taken (rage, uncanny dodge,
// temporary HP, concentration) and auras that tick on the caster's turn.

// baseKey is the fantasy catalog key an item reskins, or its own key.
func baseKey(it *Item) string {
	if it == nil {
		return ""
	}
	if d, ok := Catalog[it.Key]; ok && d.Base != "" {
		return d.Base
	}
	return it.Key
}

var (
	focusKeys   = map[string]bool{"arcane_focus": true, "holy_symbol": true, "lute": true}
	lightKeys   = map[string]bool{"dagger": true, "shortsword": true, "handaxe": true}
	heavyKeys   = map[string]bool{"greataxe": true}
	versatile   = map[string]bool{"longsword": true, "quarterstaff": true, "spear": true}
	monkKeys    = map[string]bool{"": true, "quarterstaff": true, "spear": true, "dagger": true, "handaxe": true, "mace": true, "shortsword": true}
	clubKeys    = map[string]bool{"quarterstaff": true, "mace": true}
	extraAttack = map[string]bool{"barbarian": true, "fighter": true, "monk": true, "paladin": true, "ranger": true}
)

// IsFocus reports whether a weapon is a casting focus that hurls bolts.
func IsFocus(it *Item) bool { return focusKeys[baseKey(it)] }

func hasShield(w *World, c *Character) bool {
	for _, it := range w.ItemsOf(OwnerCharacter, c.ID) {
		if it.Kind == ItemShield {
			return true
		}
	}
	return false
}

// wearsArmor reports real armor (robes and street clothes don't count).
func wearsArmor(w *World, c *Character) *Item {
	a := w.Item(c.Armor)
	if a == nil || a.Owner.ID != c.ID || a.Kind != ItemArmor || baseKey(a) == "robes" {
		return nil
	}
	return a
}

// unarmoredBase is the best base AC without armor: 10, a class's
// unarmored defense, or mage armor.
func unarmoredBase(w *World, c *Character) (string, int) {
	label, best := "Base", 10
	try := func(l string, v int) {
		if v > best {
			label, best = l, v
		}
	}
	if c.HasFeature("unarmored_defense_con") {
		try("Unarmored Defense (CON)", 10+c.Mod(CON))
	}
	if c.HasFeature("unarmored_defense_wis") && !hasShield(w, c) {
		try("Unarmored Defense (WIS)", 10+c.Mod(WIS))
	}
	if c.HasFeature("draconic_resilience") {
		try("Draconic Resilience (CHA)", 10+c.Mod(CHA))
	}
	if c.Conditions.Has(CondMageArmor) || c.HasPick("armor_of_shadows") {
		try("Mage Armor", 13)
	}
	return label, best
}

// weaponAbility is the ability a character attacks with using wpn.
func weaponAbility(c *Character, wpn *Item) Ability {
	ab := wpn.Ability
	if ab == "" {
		ab = STR
		if InferReach(wpn) == ReachRanged {
			ab = DEX
		}
	}
	better := func(a Ability) {
		if c.Mod(a) > c.Mod(ab) {
			ab = a
		}
	}
	key := baseKey(wpn)
	if wpn.Finesse {
		better(DEX)
	}
	switch {
	case IsFocus(wpn) && ClassOf(c).CastAbility != "":
		ab = ClassOf(c).CastAbility
	case c.Conditions.Has(CondWildShape) && wpn == Unarmed:
		ab = WIS
		better(STR)
	case c.Conditions.Has(CondShillelagh) && clubKeys[key]:
		ab = WIS
	case c.HasFeature("martial_arts") && monkKeys[key] && InferReach(wpn) != ReachRanged:
		better(DEX)
	}
	if c.HasPick("pact_of_the_blade") && !IsFocus(wpn) {
		better(CHA)
	}
	return ab
}

// WeaponDamage is weaponDamage for the sheet.
func WeaponDamage(c *Character, wpn *Item) string { return weaponDamage(c, wpn) }

// weaponDamage is the damage dice for c with wpn: martial arts, beast
// claws and shillelagh raise the die.
func weaponDamage(c *Character, wpn *Item) string {
	d := wpn.Damage
	if d == "" {
		d = "1d4"
	}
	key := baseKey(wpn)
	pick := func(alt string) {
		if dieSize(alt) > dieSize(d) {
			d = alt
		}
	}
	if c.Conditions.Has(CondWildShape) && wpn == Unarmed {
		pick("1d8")
	}
	if c.Conditions.Has(CondShillelagh) && clubKeys[key] {
		pick("1d8")
	}
	if c.HasFeature("martial_arts") && monkKeys[key] && InferReach(wpn) != ReachRanged {
		pick(map[bool]string{true: "1d8", false: "1d6"}[c.Level >= 5])
	}
	if IsFocus(wpn) && c.Level >= 5 && ClassOf(c).Caster != CasterNone {
		if e, err := dice.Parse(d); err == nil && e.Count == 1 {
			d = fmt.Sprintf("2d%d", e.Sides) // a cantrip's second die at level 5
		}
	}
	return d
}

func dieSize(expr string) int {
	e, err := dice.Parse(expr)
	if err != nil {
		return 0
	}
	return e.Count * e.Sides
}

// rollBonusDice rolls the extra dice a creature adds to a d20 of kind
// attack, save or check: bless, guidance, bardic inspiration. One-shot
// bonuses are used up.
func rollBonusDice(r *dice.Roller, c *Character, kind string) []RollPart {
	var out []RollPart
	if c.Conditions.Has(CondBlessed) && (kind == "attack" || kind == "save") {
		out = append(out, RollPart{Label: "Bless d4", Value: r.Die(4)})
	}
	if kind == "check" && c.Conditions.Has(CondGuided) {
		out = append(out, RollPart{Label: "Guidance d4", Value: r.Die(4)})
		c.Conditions.remove(CondGuided)
	}
	if in := c.Conditions.Get(CondInspired); in != nil {
		die := max(6, in.Die)
		out = append(out, RollPart{Label: fmt.Sprintf("Inspiration d%d", die), Value: r.Die(die)})
		c.Conditions.remove(CondInspired)
	}
	return out
}

func sumParts(ps []RollPart) int {
	n := 0
	for _, p := range ps {
		n += p.Value
	}
	return n
}

// charD20 rolls a character's d20, with a halfling's luck rerolling a 1.
func charD20(r *dice.Roller, c *Character, mod, target, edge int) dice.Check {
	ch := r.D20Edge(mod, target, edge)
	if ch.Natural == 1 && c.HasFeature("luck") {
		re := r.Die(20)
		ch.Natural, ch.Total = re, re+mod
		ch.Tier = dice.TierFor(re, ch.Total, target)
	}
	return ch
}

// saveEdge is advantage on a save from class and species features.
func saveEdge(c *Character, a Ability, why string) []string {
	var adv []string
	w := strings.ToLower(why)
	if a == STR && c.Conditions.Has(CondRaging) {
		adv = append(adv, "raging")
	}
	if a == DEX && c.Conditions.Has(CondDodging) {
		adv = append(adv, "dodging")
	}
	if a == DEX && c.HasFeature("danger_sense") {
		adv = append(adv, "danger sense")
	}
	if (a == INT || a == WIS || a == CHA) && c.HasFeature("gnomish_cunning") {
		adv = append(adv, "gnomish cunning")
	}
	if strings.Contains(w, "poison") && c.HasFeature("dwarven_resilience") {
		adv = append(adv, "dwarven resilience")
	}
	if strings.Contains(w, "fright") && c.HasFeature("brave") {
		adv = append(adv, "brave")
	}
	if (strings.Contains(w, "asleep") || strings.Contains(w, "charm")) && c.HasFeature("fey_ancestry") {
		adv = append(adv, "fey ancestry")
	}
	if strings.Contains(w, "concentration") && c.HasPick("eldritch_mind") {
		adv = append(adv, "eldritch mind")
	}
	return adv
}

// attacksPerAction is how many weapon attacks the Attack action makes.
func attacksPerAction(c *Character) int {
	if c.Level >= 5 && extraAttack[ClassOf(c).Key] && c.HasFeature("extra_attack") {
		return 2
	}
	return 1
}

// critRange is the lowest natural d20 that crits for c.
func critRange(c *Character) int {
	if c.HasFeature("improved_critical") {
		return 19
	}
	return 20
}

// strikeOpts tune one weapon strike.
type strikeOpts struct {
	Verb   string // "attacks", "lashes out at"
	Smite  bool   // spend a slot on Divine Smite if it hits
	Stun   bool   // spend focus on Stunning Strike if it hits
	Prone  bool   // Open Hand Technique: knock prone on a hit
	NoMod  bool   // off-hand attack without two-weapon fighting
	Offset int    // extra to-hit
}

// strike is one weapon attack by c on e with every class rider applied.
// It records rolls and change lines on st and returns whether it hit.
func strike(w *World, r *dice.Roller, c *Character, e *Enemy, wpn *Item, o strikeOpts, st *Step) bool {
	l := w.Here()
	d := ZoneDistance(l, c.ZoneID, e.ZoneID)
	parts := AttackParts(c, wpn)
	parts = append(parts, rollBonusDice(r, c, "attack")...)
	toHit := sumParts(parts) + o.Offset
	cover := CoverBonus(l, c.ZoneID, e.ZoneID)
	edgeN, why := AttackEdge(c.Conditions, e.Conditions, d)
	if c.HasFeat("grappler") {
		if g := e.Conditions.Get(CondRestrained); g != nil && g.SourceID == c.ID && edgeN <= 0 {
			edgeN, why = edgeN+1, strings.TrimLeft(why+", grappling it", ", ")
		}
	}
	ch := charD20(r, c, toHit, e.AC+cover, edgeN)
	verb := firstOf(o.Verb, "attacks")
	label := fmt.Sprintf("%s %s %s with %s", c.Name, verb, e.Display(), wpn.Name)
	roll := Roll{Label: label, Kind: "attack", Natural: ch.Natural, Mod: ch.Mod, Total: ch.Total, Target: ch.Target, Tier: ch.Tier,
		Parts: parts, Against: "AC", Cover: cover, Edge: ch.Edge, Naturals: ch.Naturals}
	if ch.Edge != "" {
		roll.EdgeWhy = why
	}
	hit := ch.Tier.Succeeded()
	crit := ch.Tier == dice.CritSuccess
	if ch.Natural >= critRange(c) && ch.Natural < 20 {
		hit, crit = true, true
		roll.Tier = dice.CritSuccess
	}
	if hit && d == 0 && e.Conditions.Has(CondParalyzed) {
		crit = true // paralyzed foes take critical hits up close
	}
	c.Conditions.remove(CondHelped)
	c.Conditions.remove(CondInvisible)
	if c.Conditions.remove(CondHidden) {
		st.Changes = append(st.Changes, c.Name+" gives away their hiding place")
	}
	e.Conditions.remove(CondLit)
	st.Rolls = append(st.Rolls, roll)
	if st.Tier == "" || hit {
		st.Tier = roll.Tier
	}
	if !hit {
		st.Changes = append(st.Changes, fmt.Sprintf("%s misses %s", c.Name, e.Display()))
		if ch.Tier == dice.CritFailure {
			AdjustTension(w, 1)
		}
		return false
	}
	before := e.HP
	total := 0
	addDmg := func(lbl, expr string, mod int) {
		ex, err := dice.Parse(expr)
		if err != nil {
			return
		}
		res := r.Roll(ex, crit)
		res.Mod += mod
		res.Total += mod
		dr := dmgRoll(lbl, "damage", res)
		dr.Crit = crit
		st.Rolls = append(st.Rolls, dr)
		total += res.Total
	}
	// the weapon's own dice, with great weapon fighting and savage attacker
	expr := weaponDamage(c, wpn)
	ex, _ := dice.Parse(expr)
	res := r.Roll(ex, crit)
	key := baseKey(wpn)
	if c.HasPick("great_weapon_fighting") && d == 0 && (heavyKeys[key] || (versatile[key] && !hasShield(w, c))) {
		res = gwf(res)
	}
	if c.HasFeat("savage_attacker") && onceThisTurn(w, c, "savage_attacker") {
		if alt := r.Roll(ex, crit); alt.Total > res.Total {
			res = alt
		}
	}
	if o.NoMod {
		res.Total -= res.Mod
		res.Mod = 0
	}
	res.Mod += wpn.Bonus
	res.Total += wpn.Bonus
	ab := weaponAbility(c, wpn)
	if c.Conditions.Has(CondRaging) && ab == STR && d == 0 {
		res.Mod += 2
		res.Total += 2
	}
	res.Total = max(1, res.Total)
	dr := dmgRoll(fmt.Sprintf("%s damage", wpn.Name), "damage", res)
	dr.Crit = crit
	st.Rolls = append(st.Rolls, dr)
	total += res.Total
	// riders
	if wpn.has(PropFlame) {
		addDmg("Flame", "1d6", 0)
	}
	if wpn.has(PropHoly) {
		addDmg("Radiance", "1d10", 0)
	}
	if c.HasFeature("sneak_attack") && (wpn.Finesse || InferReach(wpn) == ReachRanged && !IsFocus(wpn)) && (ch.Edge == dice.Advantage || allyNear(w, c, e)) && ch.Edge != dice.Disadvantage && onceThisTurn(w, c, "sneak_attack") {
		addDmg("Sneak Attack", fmt.Sprintf("%dd6", (c.Level+1)/2), 0)
	}
	if m := e.Conditions.Get(CondMarked); m != nil && m.SourceID == c.ID {
		addDmg(firstOf(m.Source, "Mark"), "1d6", 0)
	}
	if c.HasFeature("colossus_slayer") && before < e.MaxHP && onceThisTurn(w, c, "colossus_slayer") {
		addDmg("Colossus Slayer", "1d8", 0)
	}
	if c.HasFeature("frenzy") && c.Conditions.Has(CondRaging) && c.Conditions.Has(CondReckless) && onceThisTurn(w, c, "frenzy") {
		addDmg("Frenzy", "2d6", 0)
	}
	if o.Smite && d == 0 {
		if lvl := lowestSlot(c, 1); lvl > 0 && spendSlot(c, lvl) {
			addDmg(fmt.Sprintf("Divine Smite (level %d slot)", lvl), fmt.Sprintf("%dd8", 1+lvl), 0)
		} else {
			st.Changes = append(st.Changes, c.Name+" has no spell slot left to smite with")
		}
	}
	line := fmt.Sprintf("%s hits %s for %d damage", c.Name, e.Display(), total)
	if crit {
		line = "Critical hit! " + line
	}
	lines := hurtEnemy(w, c, e, total)
	st.Changes = append(st.Changes, line+lines[0])
	st.Changes = append(st.Changes, lines[1:]...)
	if e.Defeated() {
		return true
	}
	if o.Stun && c.HasFeature("stunning_strike") && onceThisTurn(w, c, "stunning_strike") && spend(c, "focus", 1) {
		if so := InflictOnEnemy(r, e, CondStunned, 8+ProficiencyBonus(c.Level)+c.Mod(WIS), "Stunning Strike", c.ID); so != nil {
			so.Roll.Foe, so.Roll.By = true, e.Display()
			st.Rolls = append(st.Rolls, so.Roll)
			st.Changes = append(st.Changes, so.Line)
		}
	}
	if o.Prone {
		if so := InflictOnEnemy(r, e, CondProne, 8+ProficiencyBonus(c.Level)+c.Mod(WIS), "Open Hand", c.ID); so != nil {
			so.Roll.Foe, so.Roll.By = true, e.Display()
			st.Rolls = append(st.Rolls, so.Roll)
			st.Changes = append(st.Changes, so.Line)
		}
	}
	return true
}

// gwf treats 1s and 2s on the dice as 3s.
func gwf(res dice.Result) dice.Result {
	sum := 0
	for i, v := range res.Dice {
		if v < 3 {
			res.Dice[i] = 3
		}
		sum += res.Dice[i]
	}
	res.Total = max(0, sum+res.Mod)
	return res
}

// allyNear reports another conscious party member in e's zone.
func allyNear(w *World, c *Character, e *Enemy) bool {
	for _, o := range w.Party() {
		if o.ID != c.ID && !o.Down() && o.ZoneID == e.ZoneID {
			return true
		}
	}
	return false
}

func firstOf(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// hurtEnemy deals dmg to e from c (nil for no one). The first line is a
// suffix for the hit's own line (" (3/7 HP)"); the rest are what followed:
// waking, defeat, a warlock's blessing.
func hurtEnemy(w *World, c *Character, e *Enemy, dmg int) []string {
	e.HP = max(0, e.HP-dmg)
	out := []string{fmt.Sprintf(" (%d/%d HP)", e.HP, e.MaxHP)}
	if dmg > 0 && e.Conditions.remove(CondAsleep) && e.HP > 0 {
		out = append(out, e.Display()+" snaps awake")
	}
	out = append(out, bossPhase(w, e)...)
	if e.HP == 0 {
		out = append(out, defeat(w, e, false)...)
		if c != nil && c.HasFeature("dark_ones_blessing") {
			t := max(1, c.Mod(CHA)+c.Level)
			if t > c.TempHP {
				c.TempHP = t
				out = append(out, fmt.Sprintf("%s drinks in the kill (%d temporary HP)", c.Name, t))
			}
		}
	}
	return out
}

// HurtCharacter deals dmg to c, applying everything that softens a blow
// in order: uncanny dodge, rage, stone's endurance, temporary HP. Melee
// attackers (attacker non-nil) take frost from armor of agathys. It may
// knock c out (or leave them at 1 HP with relentless endurance) and break
// concentration. It returns the damage taken and change lines.
func HurtCharacter(w *World, r *dice.Roller, c *Character, dmg int, attacker *Enemy, melee bool) (int, []string) {
	var lines []string
	if dmg <= 0 || c.Dead {
		return 0, nil
	}
	if c.HasFeature("uncanny_dodge") && attacker != nil && canReact(w, c) {
		c.ReactedRound = round(w)
		dmg = (dmg + 1) / 2
		lines = append(lines, c.Name+" twists away (Uncanny Dodge halves it)")
	}
	if c.Conditions.Has(CondRaging) {
		dmg = (dmg + 1) / 2
	}
	if c.HasFeature("stones_endurance") && attacker != nil && canReact(w, c) && dmg >= 6 && spend(c, "stones_endurance", 1) {
		c.ReactedRound = round(w)
		cut := r.Die(12) + c.Mod(CON)
		dmg = max(0, dmg-cut)
		lines = append(lines, fmt.Sprintf("%s braces like stone (%d less damage)", c.Name, cut))
	}
	if c.TempHP > 0 {
		soak := min(c.TempHP, dmg)
		c.TempHP -= soak
		dmg -= soak
		if ag := c.Conditions.Get(CondAgathys); ag != nil && melee && attacker != nil && !attacker.Defeated() {
			cold := max(5, ag.Die)
			attacker.HP = max(0, attacker.HP-cold)
			lines = append(lines, fmt.Sprintf("Frost bites %s for %d (%d/%d HP)", attacker.Display(), cold, attacker.HP, attacker.MaxHP))
			if attacker.HP == 0 {
				lines = append(lines, defeat(w, attacker, false)...)
			}
		}
		if c.TempHP == 0 {
			c.Conditions.remove(CondAgathys)
			if c.Conditions.remove(CondWildShape) {
				lines = append(lines, c.Name+" is forced back into their own shape")
			}
		}
	}
	if dmg == 0 {
		return 0, lines
	}
	c.Conditions.remove(CondAsleep)
	c.HP = max(0, c.HP-dmg)
	if c.HP == 0 && c.HasFeature("relentless_endurance") && spend(c, "relentless_endurance", 1) {
		c.HP = 1
		lines = append(lines, c.Name+" refuses to fall (Relentless Endurance)")
	}
	if c.HP == 0 {
		KnockOut(w, c)
		scarFromFall(w, c, attacker)
		endConcentration(w, c)
		return dmg, lines
	}
	if c.Conc != nil {
		dc := max(10, dmg/2)
		roll := CharacterSave(r, c, CON, dc, "concentration")
		if !roll.Tier.Succeeded() {
			lines = append(lines, fmt.Sprintf("%s loses concentration on %s (CON save %d vs DC %d)", c.Name, Spells[c.Conc.Spell].Name, roll.Total, dc))
			endConcentration(w, c)
		}
	}
	return dmg, lines
}

// startConcentration begins holding spell, dropping any earlier one.
func startConcentration(w *World, c *Character, spell string, level int, zone string) []string {
	var lines []string
	if c.Conc != nil {
		lines = append(lines, fmt.Sprintf("%s lets %s go", c.Name, Spells[c.Conc.Spell].Name))
		endConcentration(w, c)
	}
	c.Conc = &Concentration{Spell: spell, Level: level, Zone: zone}
	return lines
}

// endConcentration ends c's held spell and every effect it keeps up.
func endConcentration(w *World, c *Character) {
	if c.Conc == nil {
		return
	}
	spell := c.Conc.Spell
	c.Conc = nil
	drop := func(cs *Conditions) {
		keep := (*cs)[:0]
		for _, x := range *cs {
			if !(x.Spell == spell && x.SourceID == c.ID) {
				keep = append(keep, x)
			}
		}
		*cs = keep
	}
	for _, ch := range w.Characters {
		drop(&ch.Conditions)
	}
	for _, e := range w.encounterEnemies() {
		drop(&e.Conditions)
	}
}

// StartTurnEffects runs a character's start-of-turn spell effects: spirit
// guardians and moonbeam burn foes, and the round's reactions reset.
func StartTurnEffects(w *World, r *dice.Roller, c *Character) ConditionTick {
	var t ConditionTick
	w.RefreshMagic()
	if w.InCombat() {
		t.Changes = append(t.Changes, regenTick(r, c)...)
	}
	if c.Conc == nil || !w.InCombat() || c.Down() {
		return t
	}
	s := Spells[c.Conc.Spell]
	switch c.Conc.Spell {
	case "spirit_guardians":
		st := &Step{}
		areaSave(w, r, c, s, c.Conc.Level, c.ZoneID, st)
		t.Changes, t.Rolls = append(t.Changes, st.Changes...), st.Rolls
	case "moonbeam":
		st := &Step{}
		areaSave(w, r, c, s, c.Conc.Level, c.Conc.Zone, st)
		t.Changes, t.Rolls = append(t.Changes, st.Changes...), st.Rolls
	}
	return t
}

// charCheck rolls c's ability check with every modifier: conditions,
// rage on Strength, guidance and inspiration dice, and halfling luck. It
// returns the roll, the final parts and why it had an edge.
func charCheck(r *dice.Roller, c *Character, ab Ability, parts []RollPart, dc int) (dice.Check, []RollPart, string) {
	adv, dis := []string{}, checkDis(c.Conditions)
	if c.Conditions.Has(CondHelped) {
		adv = append(adv, "helped by an ally")
	}
	if ab == STR && c.Conditions.Has(CondRaging) {
		adv = append(adv, "raging")
	}
	ed, why := edge(adv, dis)
	parts = append(append([]RollPart{}, parts...), rollBonusDice(r, c, "check")...)
	return charD20(r, c, sumParts(parts), dc, ed), parts, why
}
