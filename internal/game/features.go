package game

import (
	"fmt"
	"strings"

	"github.com/Solvely-Colin/aidnd/internal/dice"
)

// Feature targets.
const (
	TargetSelf = "self"
	TargetAlly = "ally"
	TargetFoe  = "foe"
	TargetAny  = "any" // ally to heal or foe to harm
	TargetZone = "zone"
	TargetItem = "item"
	TargetNone = "none"
)

// FeatureAct is a class or species feature used with use_feature.
type FeatureAct struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Slot     Slot   `json:"slot"`
	Resource string `json:"resource,omitempty"`
	Cost     int    `json:"cost,omitempty"`
	Target   string `json:"target"`
	Combat   bool   `json:"combat,omitempty"` // only in a fight
	Peace    bool   `json:"peace,omitempty"`  // only outside one
	Desc     string `json:"desc"`
	needs    string // the feature that grants it, when not Key
}

// FeatureActs are every usable feature, in sheet order.
var FeatureActs = []FeatureAct{
	{Key: "rage", Name: "Rage", Slot: SlotBonus, Resource: "rage", Cost: 1, Target: TargetSelf, Desc: "+2 Strength damage, half damage taken, advantage on Strength checks and saves until the fight ends."},
	{Key: "reckless_attack", Name: "Reckless Attack", Slot: SlotFree, Target: TargetSelf, Combat: true, Desc: "Advantage on your attacks this turn; attacks against you have advantage until your next turn."},
	{Key: "second_wind", Name: "Second Wind", Slot: SlotBonus, Resource: "second_wind", Cost: 1, Target: TargetSelf, Desc: "Heal 1d10 + your level."},
	{Key: "action_surge", Name: "Action Surge", Slot: SlotFree, Resource: "action_surge", Cost: 1, Target: TargetSelf, Combat: true, Desc: "Take one more action this turn."},
	{Key: "bardic_inspiration", Name: "Bardic Inspiration", Slot: SlotBonus, Resource: "bardic_inspiration", Cost: 1, Target: TargetAlly, Desc: "An ally adds your inspiration die to their next d20."},
	{Key: "divine_spark", Name: "Divine Spark", Slot: SlotAction, Resource: "channel_divinity", Cost: 1, Target: TargetAny, Desc: "Heal an ally 1d8 + WIS, or a foe takes 1d8 + WIS (CON save for half)."},
	{Key: "turn_undead", Name: "Turn Undead", Slot: SlotAction, Resource: "channel_divinity", Cost: 1, Target: TargetNone, Combat: true, Desc: "Undead in your zone make a WIS save or are frightened."},
	{Key: "preserve_life", Name: "Preserve Life", Slot: SlotAction, Resource: "channel_divinity", Cost: 1, Target: TargetNone, Desc: "Split 5 x your level in healing among wounded allies nearby."},
	{Key: "sacred_weapon", Name: "Sacred Weapon", Slot: SlotFree, Resource: "channel_divinity", Cost: 1, Target: TargetSelf, Combat: true, Desc: "Add CHA to your attack rolls until the fight ends."},
	{Key: "wild_shape", Name: "Wild Shape", Slot: SlotBonus, Resource: "wild_shape", Cost: 1, Target: TargetSelf, Desc: "Become a beast until the fight ends: 4 temporary HP per level and claws (1d8, WIS)."},
	{Key: "lands_aid", Name: "Land's Aid", Slot: SlotAction, Resource: "wild_shape", Cost: 1, Target: TargetZone, Combat: true, Desc: "Foes in a zone take 2d6 (CON save for half) and the most hurt ally nearby heals 2d6."},
	{Key: "martial_arts", Name: "Bonus Strike", Slot: SlotBonus, Target: TargetFoe, Combat: true, Desc: "An unarmed strike as a bonus action."},
	{Key: "flurry_of_blows", Name: "Flurry of Blows", Slot: SlotBonus, Resource: "focus", Cost: 1, Target: TargetFoe, Combat: true, Desc: "Two unarmed strikes as a bonus action.", needs: "focus"},
	{Key: "patient_defense", Name: "Patient Defense", Slot: SlotBonus, Resource: "focus", Cost: 1, Target: TargetSelf, Combat: true, Desc: "Dodge and disengage as a bonus action.", needs: "focus"},
	{Key: "step_of_the_wind", Name: "Step of the Wind", Slot: SlotBonus, Resource: "focus", Cost: 1, Target: TargetSelf, Desc: "Disengage and gain an extra move as a bonus action.", needs: "focus"},
	{Key: "lay_on_hands", Name: "Lay on Hands", Slot: SlotBonus, Resource: "lay_on_hands", Target: TargetAlly, Desc: "Heal from your pool (or spend 5 to cure poison)."},
	{Key: "cunning_dash", Name: "Cunning Action: Dash", Slot: SlotBonus, Target: TargetSelf, Desc: "An extra move as a bonus action.", needs: "cunning_action"},
	{Key: "cunning_disengage", Name: "Cunning Action: Disengage", Slot: SlotBonus, Target: TargetSelf, Combat: true, Desc: "Disengage as a bonus action.", needs: "cunning_action"},
	{Key: "cunning_hide", Name: "Cunning Action: Hide", Slot: SlotBonus, Target: TargetSelf, Desc: "Hide as a bonus action.", needs: "cunning_action"},
	{Key: "steady_aim", Name: "Steady Aim", Slot: SlotBonus, Target: TargetSelf, Combat: true, Desc: "Give up your move for advantage on your next attack."},
	{Key: "fast_hands", Name: "Fast Hands", Slot: SlotBonus, Target: TargetItem, Desc: "Use an item as a bonus action."},
	{Key: "innate_sorcery", Name: "Innate Sorcery", Slot: SlotBonus, Resource: "innate_sorcery", Cost: 1, Target: TargetSelf, Desc: "+1 spell save DC and advantage on spell attacks until the fight ends."},
	{Key: "create_slot", Name: "Font of Magic: Create Slot", Slot: SlotBonus, Resource: "sorcery_points", Target: TargetNone, Desc: "Turn 2/3/5 sorcery points into a level 1/2/3 spell slot.", needs: "font_of_magic"},
	{Key: "magical_cunning", Name: "Magical Cunning", Slot: SlotAction, Resource: "magical_cunning", Cost: 1, Target: TargetNone, Peace: true, Desc: "Recover half your pact slots."},
	{Key: "breath_weapon", Name: "Breath Weapon", Slot: SlotAction, Resource: "breath_weapon", Cost: 1, Target: TargetNone, Combat: true, Desc: "Foes in your zone take 1d10 (2d10 at level 5), DEX save for half."},
	{Key: "adrenaline_rush", Name: "Adrenaline Rush", Slot: SlotBonus, Resource: "adrenaline_rush", Cost: 1, Target: TargetSelf, Desc: "An extra move and temporary HP equal to your proficiency bonus."},
	{Key: "spirit_strike", Name: "Spiritual Weapon Strike", Slot: SlotBonus, Target: TargetFoe, Combat: true, Desc: "Your spiritual weapon strikes again."},
	{Key: "offhand_attack", Name: "Off-Hand Attack", Slot: SlotBonus, Target: TargetFoe, Combat: true, Desc: "Strike with a second light weapon."},
}

// FeatureAct finds a usable feature by key or name.
func FeatureActByRef(ref string) *FeatureAct {
	r := strings.ToLower(strings.TrimSpace(ref))
	k := strings.NewReplacer(" ", "_", "-", "_", "'", "", ":", "").Replace(r)
	for i := range FeatureActs {
		f := &FeatureActs[i]
		if f.Key == k || strings.EqualFold(f.Name, ref) {
			return f
		}
	}
	for i := range FeatureActs {
		if NameMatch(FeatureActs[i].Name, ref) {
			return &FeatureActs[i]
		}
	}
	return nil
}

// hasAct reports whether c has the feature behind f at all.
func hasAct(w *World, c *Character, f *FeatureAct) bool {
	switch f.Key {
	case "spirit_strike":
		return c.Conc != nil && c.Conc.Spell == "spiritual_weapon"
	case "offhand_attack":
		return offhandWeapon(w, c) != nil
	}
	return c.HasFeature(firstOf(f.needs, f.Key))
}

// offhandWeapon is a second light melee weapon c carries while wielding a
// light one, or nil.
func offhandWeapon(w *World, c *Character) *Item {
	main := Weapon(w, c)
	if main == Unarmed || !lightKeys[baseKey(main)] {
		return nil
	}
	for _, it := range w.ItemsOf(OwnerCharacter, c.ID) {
		if it.ID != main.ID && it.Kind == ItemWeapon && lightKeys[baseKey(it)] {
			return it
		}
	}
	return nil
}

// FeatureUse is a feature on the sheet with whether it can be used now.
type FeatureUse struct {
	FeatureAct
	Left  int    `json:"left"`
	Max   int    `json:"max"`
	Ready bool   `json:"ready"`
	Why   string `json:"why,omitempty"`
}

// FeatureUses lists the features c can use, and which are ready now.
func FeatureUses(w *World, c *Character) []FeatureUse {
	var out []FeatureUse
	res := map[string]Resource{}
	for _, r := range Resources(c) {
		res[r.Key] = r
	}
	for i := range FeatureActs {
		f := FeatureActs[i]
		if !hasAct(w, c, &f) {
			continue
		}
		u := FeatureUse{FeatureAct: f, Ready: true}
		if f.Resource != "" {
			r := res[f.Resource]
			u.Left, u.Max = r.Max-r.Used, r.Max
		}
		if err := canUse(w, c, &f); err != nil {
			u.Ready, u.Why = false, err.Error()
		}
		out = append(out, u)
	}
	return out
}

// canUse checks a feature's timing and uses.
func canUse(w *World, c *Character, f *FeatureAct) error {
	if f.Combat && !w.InCombat() {
		return fmt.Errorf("only in a fight")
	}
	if f.Peace && w.InCombat() {
		return fmt.Errorf("not in a fight")
	}
	cost := max(1, f.Cost)
	if f.Resource != "" && resourceLeft(c, f.Resource) < cost {
		return fmt.Errorf("no uses left; a rest brings them back")
	}
	switch f.Key {
	case "rage":
		if c.Conditions.Has(CondRaging) {
			return fmt.Errorf("already raging")
		}
		if wearsArmor(w, c) != nil && strings.Contains(strings.ToLower(wearsArmor(w, c).Name), "plate") {
			return fmt.Errorf("can't rage in heavy armor")
		}
	case "wild_shape":
		if c.Conditions.Has(CondWildShape) {
			return fmt.Errorf("already in beast form")
		}
	case "create_slot":
		if resourceLeft(c, "sorcery_points") < 2 {
			return fmt.Errorf("needs 2 sorcery points")
		}
	case "magical_cunning":
		if len(c.SlotsUsed) == 0 {
			return fmt.Errorf("no pact slots spent")
		}
	case "second_wind":
		if c.HP >= c.MaxHP {
			return fmt.Errorf("not hurt")
		}
	}
	return nil
}

// featureSlot is what using a feature costs.
func featureSlot(w *World, c *Character, a Action) (Slot, error) {
	f := FeatureActByRef(a.Feature)
	if f == nil {
		return "", fmt.Errorf("unknown feature %q", a.Feature)
	}
	if !w.InCombat() && f.Slot != SlotAction {
		return SlotFree, nil // outside a fight bonus actions are just quick
	}
	return f.Slot, nil
}

// useFeature resolves use_feature.
func useFeature(ctx *Ctx, c *Character, a Action, st *Step) error {
	w, r := ctx.W, ctx.R
	f := FeatureActByRef(a.Feature)
	if f == nil {
		return fmt.Errorf("unknown feature %q", a.Feature)
	}
	if !hasAct(w, c, f) {
		return fmt.Errorf("%s can't use %s", c.Name, f.Name)
	}
	if err := canUse(w, c, f); err != nil {
		return fmt.Errorf("%s: %v", f.Name, err)
	}
	foe := func() (*Enemy, error) {
		e, err := findFoe(w, a.Target, true)
		if err != nil && a.Target == "" {
			if e = nearestFoe(w, c); e == nil {
				return nil, fmt.Errorf("no foe to target")
			}
			return e, nil
		}
		return e, err
	}
	ally := func(reach int) (*Character, error) {
		t := c
		if a.Target != "" {
			if t = w.Character(a.Target); t == nil {
				return nil, fmt.Errorf("no ally %q here", a.Target)
			}
		}
		if d := ZoneDistance(w.Here(), c.ZoneID, t.ZoneID); d > reach {
			return nil, fmt.Errorf("%s is too far away", t.Name)
		}
		if t.Dead {
			return nil, fmt.Errorf("%s is beyond help", t.Name)
		}
		return t, nil
	}
	useRes := func() {
		if f.Resource != "" && f.Cost > 0 {
			spend(c, f.Resource, f.Cost)
		}
	}
	pb := ProficiencyBonus(c.Level)
	switch f.Key {
	case "rage":
		useRes()
		c.Conditions.add(Condition{Kind: CondRaging, Rounds: 10, Source: "Rage", SourceID: c.ID})
		st.Changes = append(st.Changes, c.Name+" flies into a rage")
	case "reckless_attack":
		c.Conditions.add(Condition{Kind: CondReckless, Source: "Reckless Attack", SourceID: c.ID})
		st.Changes = append(st.Changes, c.Name+" throws caution away (reckless)")
	case "second_wind":
		useRes()
		ex := dice.MustParse("1d10")
		ex.Mod = c.Level
		res := r.Roll(ex, false)
		st.Rolls = append(st.Rolls, dmgRoll("Second Wind", "heal", res))
		got, _ := Heal(c, res.Total)
		st.Changes = append(st.Changes, fmt.Sprintf("%s catches a second wind (+%d HP, %d/%d)", c.Name, got, c.HP, c.MaxHP))
	case "action_surge":
		useRes()
		st.Grant = &Budget{Action: 1}
		st.Changes = append(st.Changes, c.Name+" surges into another action")
	case "bardic_inspiration":
		t, err := ally(2)
		if err != nil {
			return err
		}
		if t == c {
			return fmt.Errorf("inspire someone else")
		}
		useRes()
		die := map[bool]int{true: 8, false: 6}[c.Level >= 5]
		t.Conditions.remove(CondInspired)
		t.Conditions.add(Condition{Kind: CondInspired, Die: die, Source: c.Name, SourceID: c.ID})
		st.Changes = append(st.Changes, fmt.Sprintf("%s inspires %s (d%d)", c.Name, t.Name, die))
	case "divine_spark":
		ex := dice.MustParse("1d8")
		ex.Mod = c.Mod(WIS)
		if t := w.Character(a.Target); t != nil {
			t, err := ally(1)
			if err != nil {
				return err
			}
			useRes()
			res := r.Roll(ex, false)
			st.Rolls = append(st.Rolls, dmgRoll("Divine Spark", "heal", res))
			got, woke := Heal(t, max(1, res.Total))
			line := fmt.Sprintf("Divine light mends %s (+%d HP, %d/%d)", t.Name, got, t.HP, t.MaxHP)
			if woke {
				line += " and they rise"
			}
			st.Changes = append(st.Changes, line)
			return nil
		}
		e, err := foe()
		if err != nil {
			return err
		}
		if d := ZoneDistance(w.Here(), c.ZoneID, e.ZoneID); d > 1 {
			return fmt.Errorf("%s is too far for Divine Spark", e.Display())
		}
		useRes()
		dmg := rollDamage(r, "Divine Spark", "1d8", c.Mod(WIS), false, st)
		roll := EnemySave(r, e, CON, SpellDC(c), "Divine Spark")
		roll.Foe, roll.By = true, e.Display()
		st.Rolls = append(st.Rolls, roll)
		if roll.Tier.Succeeded() {
			dmg /= 2
		}
		lines := hurtEnemy(w, c, e, dmg)
		st.Changes = append(st.Changes, fmt.Sprintf("Divine fire sears %s for %d", e.Display(), dmg)+lines[0])
		st.Changes = append(st.Changes, lines[1:]...)
	case "turn_undead":
		useRes()
		n := 0
		for _, e := range w.Encounter.Living() {
			if e.ZoneID != c.ZoneID && ZoneDistance(w.Here(), c.ZoneID, e.ZoneID) > 1 || !undead(e) {
				continue
			}
			n++
			if so := InflictOnEnemy(r, e, CondFrightened, SpellDC(c), "Turn Undead", c.ID); so != nil {
				so.Roll.Foe, so.Roll.By = true, e.Display()
				st.Rolls = append(st.Rolls, so.Roll)
				st.Changes = append(st.Changes, so.Line)
				if so.Applied && c.HasFeature("sear_undead") {
					dmg := rollDamage(r, "Sear Undead", "1d8", 0, false, st)
					lines := hurtEnemy(w, c, e, dmg)
					st.Changes = append(st.Changes, fmt.Sprintf("Radiance sears %s for %d", e.Display(), dmg)+lines[0])
					st.Changes = append(st.Changes, lines[1:]...)
				}
			}
		}
		if n == 0 {
			st.Changes = append(st.Changes, c.Name+" presents their faith, but nothing undead is near")
		}
	case "preserve_life":
		useRes()
		pool := 5 * c.Level
		for _, t := range w.Party() {
			if pool == 0 || t.Dead || ZoneDistance(w.Here(), c.ZoneID, t.ZoneID) > 1 || t.HP >= t.MaxHP/2 {
				continue
			}
			amt := min(pool, t.MaxHP/2-t.HP)
			got, woke := Heal(t, amt)
			pool -= got
			line := fmt.Sprintf("%s is restored (+%d HP)", t.Name, got)
			if woke {
				line += " and rises"
			}
			st.Changes = append(st.Changes, line)
		}
	case "sacred_weapon":
		useRes()
		c.Conditions.add(Condition{Kind: CondSacred, Rounds: 10, Source: "Sacred Weapon", SourceID: c.ID})
		st.Changes = append(st.Changes, c.Name+"'s weapon blazes with holy light")
	case "wild_shape":
		useRes()
		c.TempHP = max(c.TempHP, 4*c.Level)
		c.Conditions.add(Condition{Kind: CondWildShape, Rounds: 10, Source: "Wild Shape", SourceID: c.ID})
		st.Changes = append(st.Changes, fmt.Sprintf("%s takes a beast's shape (%d temporary HP)", c.Name, c.TempHP))
	case "lands_aid":
		l := w.Here()
		zone := ""
		if z := l.Zone(a.Zone); z != nil {
			zone = z.ID
		} else if e, err := foe(); err == nil {
			zone = e.ZoneID
		}
		if zone == "" || ZoneDistance(l, c.ZoneID, zone) > 2 {
			return fmt.Errorf("land's aid on which zone?")
		}
		useRes()
		s := &SpellDef{Key: "lands_aid", Name: "Land's Aid", Save: CON, Half: true, Dice: "2d6"}
		areaSave(w, r, c, s, 0, zone, st)
		var hurt *Character
		for _, t := range w.Party() {
			if !t.Dead && t.HP < t.MaxHP && (hurt == nil || t.HP < hurt.HP) {
				hurt = t
			}
		}
		if hurt != nil {
			res := r.Roll(dice.MustParse("2d6"), false)
			st.Rolls = append(st.Rolls, dmgRoll("Land's Aid", "heal", res))
			got, _ := Heal(hurt, res.Total)
			st.Changes = append(st.Changes, fmt.Sprintf("%s is mended by green light (+%d HP)", hurt.Name, got))
		}
	case "martial_arts", "flurry_of_blows":
		e, err := foe()
		if err != nil {
			return err
		}
		if ZoneDistance(w.Here(), c.ZoneID, e.ZoneID) > 0 {
			return fmt.Errorf("%s is out of reach; get closer first", e.Display())
		}
		useRes()
		n := 1
		if f.Key == "flurry_of_blows" {
			n = 2
		}
		for i := 0; i < n && !e.Defeated(); i++ {
			strike(w, r, c, e, Unarmed, strikeOpts{Verb: "strikes", Prone: f.Key == "flurry_of_blows" && c.HasFeature("open_hand_technique")}, st)
		}
	case "patient_defense":
		useRes()
		c.Conditions.add(Condition{Kind: CondDodging})
		c.Conditions.add(Condition{Kind: CondDisengaged})
		st.Changes = append(st.Changes, c.Name+" settles into a patient guard (dodging, disengaged)")
	case "step_of_the_wind":
		useRes()
		c.Conditions.add(Condition{Kind: CondDisengaged})
		st.Grant = &Budget{Move: 1}
		st.Changes = append(st.Changes, c.Name+" moves like the wind (disengaged, extra move)")
	case "lay_on_hands":
		t, err := ally(0)
		if err != nil {
			return err
		}
		left := resourceLeft(c, "lay_on_hands")
		if t.Conditions.Has(CondPoisoned) && left >= 5 && (a.Text == "cure" || t.HP >= t.MaxHP) {
			spend(c, "lay_on_hands", 5)
			t.Conditions.remove(CondPoisoned)
			st.Changes = append(st.Changes, fmt.Sprintf("%s's touch purges the poison from %s", c.Name, t.Name))
			return nil
		}
		amt := min(left, t.MaxHP-t.HP)
		if a.Level > 0 {
			amt = min(amt, a.Level)
		}
		if amt <= 0 {
			return fmt.Errorf("%s isn't hurt", t.Name)
		}
		spend(c, "lay_on_hands", amt)
		got, woke := Heal(t, amt)
		line := fmt.Sprintf("%s lays hands on %s (+%d HP, %d/%d)", c.Name, t.Name, got, t.HP, t.MaxHP)
		if woke {
			line += " and they rise"
		}
		st.Changes = append(st.Changes, line)
	case "cunning_dash":
		st.Grant = &Budget{Move: 1}
		st.Changes = append(st.Changes, c.Name+" darts ahead (extra move)")
	case "cunning_disengage":
		return disengage(w, c, st)
	case "cunning_hide":
		return hide(ctx, c, st)
	case "steady_aim":
		c.Conditions.add(Condition{Kind: CondHelped, Rounds: 1, Source: "Steady Aim"})
		st.Grant = &Budget{Move: -1}
		st.Changes = append(st.Changes, c.Name+" takes careful aim (advantage on the next attack)")
	case "fast_hands":
		return useItem(ctx, c, Action{Kind: ActUseItem, Item: firstNonEmpty(a.Item, a.Text), Target: a.Target}, st)
	case "innate_sorcery":
		useRes()
		c.Conditions.add(Condition{Kind: CondSorcery, Rounds: 10, Source: "Innate Sorcery", SourceID: c.ID})
		st.Changes = append(st.Changes, c.Name+"'s magic surges to the surface")
	case "create_slot":
		lvl := clamp(a.Level, 1, 3)
		if a.Level == 0 {
			for lvl = 1; lvl <= 3 && (lvl-1 >= len(c.SlotsUsed) || c.SlotsUsed[lvl-1] == 0); lvl++ {
			}
			if lvl > 3 {
				return fmt.Errorf("no spent slot to restore")
			}
		}
		cost := map[int]int{1: 2, 2: 3, 3: 5}[lvl]
		if lvl > MaxSpellLevel(c) || lvl-1 >= len(c.SlotsUsed) || c.SlotsUsed[lvl-1] == 0 {
			return fmt.Errorf("no spent level %d slot to restore", lvl)
		}
		if !spend(c, "sorcery_points", cost) {
			return fmt.Errorf("a level %d slot costs %d sorcery points", lvl, cost)
		}
		c.SlotsUsed[lvl-1]--
		st.Changes = append(st.Changes, fmt.Sprintf("%s shapes %d sorcery points into a level %d slot", c.Name, cost, lvl))
	case "magical_cunning":
		useRes()
		n := (SlotsMax(c)[MaxSpellLevel(c)-1] + 1) / 2
		for i := range c.SlotsUsed {
			got := min(n, c.SlotsUsed[i])
			c.SlotsUsed[i] -= got
			n -= got
		}
		st.Changes = append(st.Changes, c.Name+" whispers to their patron and recovers pact magic")
	case "breath_weapon":
		useRes()
		dmg := map[bool]string{true: "2d10", false: "1d10"}[c.Level >= 5]
		s := &SpellDef{Key: "breath_weapon", Name: "Breath Weapon", Save: DEX, Half: true, Dice: dmg}
		// the DC uses CON, not a casting ability
		dc := 8 + c.Mod(CON) + pb
		var hit []*Enemy
		for _, e := range w.Encounter.Living() {
			if e.ZoneID == c.ZoneID {
				hit = append(hit, e)
			}
		}
		if len(hit) == 0 {
			st.Changes = append(st.Changes, c.Name+"'s breath scorches empty air")
			return nil
		}
		total := rollDamage(r, "Breath Weapon", s.Dice, 0, false, st)
		for _, e := range hit {
			roll := EnemySave(r, e, DEX, dc, "Breath Weapon")
			roll.Foe, roll.By = true, e.Display()
			st.Rolls = append(st.Rolls, roll)
			take := total
			if roll.Tier.Succeeded() {
				take /= 2
			}
			lines := hurtEnemy(w, c, e, take)
			st.Changes = append(st.Changes, fmt.Sprintf("Breath engulfs %s for %d", e.Display(), take)+lines[0])
			st.Changes = append(st.Changes, lines[1:]...)
		}
	case "adrenaline_rush":
		useRes()
		c.TempHP = max(c.TempHP, pb)
		st.Grant = &Budget{Move: 1}
		st.Changes = append(st.Changes, fmt.Sprintf("%s surges with adrenaline (extra move, %d temporary HP)", c.Name, pb))
	case "spirit_strike":
		e, err := foe()
		if err != nil {
			return err
		}
		spiritStrike(w, r, c, e, c.Conc.Level, st)
	case "offhand_attack":
		e, err := foe()
		if err != nil {
			return err
		}
		if ZoneDistance(w.Here(), c.ZoneID, e.ZoneID) > 0 {
			return fmt.Errorf("%s is out of reach", e.Display())
		}
		strike(w, r, c, e, offhandWeapon(w, c), strikeOpts{Verb: "follows up on", NoMod: !c.HasPick("two_weapon_fighting")}, st)
	default:
		return fmt.Errorf("%s has nothing to do", f.Name)
	}
	return nil
}

// undead guesses from a foe's name whether Turn Undead affects it.
func undead(e *Enemy) bool {
	n := strings.ToLower(e.Name + " " + e.Tag)
	for _, w := range []string{"skeleton", "zombie", "ghoul", "ghost", "wraith", "drowned", "dead", "revenant", "lich", "vampire", "mummy", "specter", "spectre", "wight", "shade", "corpse", "bone"} {
		if strings.Contains(n, w) {
			return true
		}
	}
	return false
}
