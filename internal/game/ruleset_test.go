package game

import (
	"strings"
	"testing"
)

// build adds a class character for player pid to the fixture, at the
// party's zone, replacing whoever that player had.
func (f *fixture) build(t *testing.T, pid, class string, level int) *Character {
	t.Helper()
	spec := FillBuild(CharacterSpec{Name: strings.ToUpper(class[:1]) + class[1:], ClassKey: class})
	if _, p := ValidateBuild(spec); len(p) > 0 {
		t.Fatalf("%s default build invalid: %v", class, p)
	}
	old := f.w.CharacterOf(pid)
	c, err := CreateCharacter(f.w, pid, spec)
	must(t, err)
	if old != nil {
		c.ZoneID = old.ZoneID
		for i, x := range f.w.Characters {
			if x == old {
				f.w.Characters = append(f.w.Characters[:i], f.w.Characters[i+1:]...)
				break
			}
		}
		for i, id := range f.w.Campaign.Turns.Order {
			if id == old.ID {
				f.w.Campaign.Turns.Order[i] = c.ID
			}
		}
	}
	for c.Level < level {
		c.Level++
		levelUp(c)
	}
	return c
}

func TestEveryClassBuildsInEveryEra(t *testing.T) {
	for _, setting := range SettingKeys() {
		for _, k := range ClassKeys() {
			f := newFixture(t)
			f.w.Campaign.Setting = setting
			c := f.build(t, "u1", k, 1)
			cd := Classes[k]
			if c.MaxHP != cd.HitDie+c.Mod(CON)+hpBonusPerLevel(c) {
				t.Errorf("%s: max HP %d", k, c.MaxHP)
			}
			if Weapon(f.w, c) == Unarmed && k != "monk" {
				t.Errorf("%s has no weapon in %s", k, setting)
			}
			if cd.Caster != CasterNone {
				cantrips, leveled := 0, 0
				for _, s := range c.Spells {
					if Spells[s].Level == 0 {
						cantrips++
					} else {
						leveled++
					}
				}
				if cantrips != CantripCount(c) || leveled != PreparedCount(c) {
					t.Errorf("%s spells %v: %d cantrips, %d prepared", k, c.Spells, cantrips, leveled)
				}
			}
			if ClassName(cd, setting) == "" || SubclassName(cd, setting) == "" {
				t.Errorf("%s has no name in %s", k, setting)
			}
			if len(PendingChoices(c)) > 0 {
				t.Errorf("%s left choices: %+v", k, PendingChoices(c))
			}
		}
	}
	if n := ClassName(Classes["wizard"], SettingPulp); n != "Occultist" {
		t.Errorf("pulp wizard is %q", n)
	}
	if n := ClassName(Classes["rogue"], SettingModern); n != "Operator" {
		t.Errorf("modern rogue is %q", n)
	}
	for _, s := range Spells {
		for i, n := range s.Era {
			if n == "" {
				t.Errorf("%s has no name in era %d", s.Key, i)
			}
		}
	}
}

func TestBuildValidation(t *testing.T) {
	spec := FillBuild(CharacterSpec{Name: "Vex", ClassKey: "rogue", Species: "halfling", Background: "criminal"})
	bad := spec
	bad.Boost = map[string]int{"STR": 2, "DEX": 1}
	if _, p := ValidateBuild(bad); !hasProblem(p, "can only raise") {
		t.Errorf("boost outside the background allowed: %v", p)
	}
	bad = spec
	bad.Skills = []string{"arcana"}
	if _, p := ValidateBuild(bad); len(p) == 0 {
		t.Error("a wizard skill was allowed for a rogue")
	}
	bad = spec
	bad.Picks = map[string][]string{"expertise": {"stealth"}}
	if _, p := ValidateBuild(bad); !hasProblem(p, "pick 2") {
		t.Errorf("missing expertise allowed: %v", p)
	}
	scores, p := ValidateBuild(spec)
	if len(p) > 0 {
		t.Fatal(p)
	}
	if scores[DEX] != 17 {
		t.Errorf("criminal +2 DEX: %d", scores[DEX])
	}
}

func TestSneakAttackAndExtraAttack(t *testing.T) {
	f := newFixture(t)
	e := f.fight(t, 0, 0, EnemySpec{Name: "Ogre", HP: 60, AC: 10, AttackBonus: 3, Damage: "1d6"})
	e.HP, e.MaxHP = 60, 60
	rogue := f.build(t, "u1", "rogue", 1)
	f.w.Campaign.Turns.Index = 0
	// d20 15 hits, shortsword 4, sneak attack 1d6 = 5; Bram stands beside the ogre
	res := Resolve(f.ctx(15, 4, 5), rogue, []Action{{Kind: ActAttack, Target: e.ID}})
	if e.HP != 60-9 || !hasChange(res, "Sneak") && !strings.Contains(strings.Join(res.Steps[0].Changes, ";"), "9 damage") {
		t.Fatalf("sneak attack: hp %d %v", e.HP, res.Changes)
	}
	// only once a turn
	f.w.Campaign.Turns.Budget = FullBudget(true)
	Resolve(f.ctx(15, 4, 5), rogue, []Action{{Kind: ActAttack, Target: e.ID}})
	if e.HP != 60-9-4 {
		t.Fatalf("second sneak attack in a turn: hp %d", e.HP)
	}

	g := newFixture(t)
	e = g.fight(t, 0, 0, EnemySpec{Name: "Ogre", HP: 60, AC: 10, AttackBonus: 3, Damage: "1d6"})
	e.HP, e.MaxHP = 60, 60
	ftr := g.build(t, "u1", "fighter", 5)
	res = Resolve(g.ctx(15, 3, 3, 15, 3), ftr, []Action{{Kind: ActAttack, Target: e.ID}})
	attacks := 0
	for _, r := range res.Steps[0].Rolls {
		if r.Kind == "attack" {
			attacks++
		}
	}
	if attacks != 2 || e.HP != 54 {
		t.Fatalf("extra attack: %d attacks, hp %d", attacks, e.HP)
	}
}

func TestCastingSpendsSlotsAndHits(t *testing.T) {
	f := newFixture(t)
	e := f.fight(t, 0, 1, EnemySpec{Name: "Ghoul", HP: 30, AC: 12, AttackBonus: 3, Damage: "1d6"})
	e.HP, e.MaxHP = 30, 30
	wiz := f.build(t, "u1", "wizard", 1)
	res := Resolve(f.ctx(3, 3, 3), wiz, []Action{{Kind: ActCast, Spell: "magic_missile", Target: e.ID}})
	if stepErr(res, 0) != "" {
		t.Fatal(stepErr(res, 0))
	}
	if e.HP != 30-12 || SlotsLeft(wiz)[0] != 1 {
		t.Fatalf("magic missile: hp %d, slots %v", e.HP, SlotsLeft(wiz))
	}
	// out of slots: a sage's magic initiate gives one free cast, then nothing
	spendSlot(wiz, 1)
	f.w.Campaign.Turns.Budget = FullBudget(true)
	res = Resolve(f.ctx(1), wiz, []Action{{Kind: ActCast, Spell: "Magic Missile", Target: e.ID}})
	if stepErr(res, 0) != "" || resourceLeft(wiz, "mi_spell") != 0 {
		t.Fatalf("free magic initiate cast: %q", stepErr(res, 0))
	}
	e.HP = 30 - 12
	f.w.Campaign.Turns.Budget = FullBudget(true)
	res = Resolve(f.ctx(3), wiz, []Action{{Kind: ActCast, Spell: "Magic Missile", Target: e.ID}})
	if !strings.Contains(stepErr(res, 0), "out of spell slots") {
		t.Fatalf("cast without slots: %q", stepErr(res, 0))
	}
	// cantrips are free: fire bolt 18 hits, 1d10 = 6
	f.w.Campaign.Turns.Budget = FullBudget(true)
	Resolve(f.ctx(18, 6), wiz, []Action{{Kind: ActCast, Spell: "fire_bolt", Target: e.ID}})
	if e.HP != 30-12-6 {
		t.Fatalf("fire bolt: hp %d", e.HP)
	}
	// a pulp occultist casts the same spell by its era name
	if s := SpellByRef("Hellfire Sigil"); s == nil || s.Key != "fire_bolt" {
		t.Fatal("era name not found")
	}
}

func TestAreaSaveAndConcentration(t *testing.T) {
	f := newFixture(t)
	e := f.fight(t, 0, 1, EnemySpec{Name: "Wolf", HP: 30, AC: 12, AttackBonus: 3, Damage: "1d6"})
	e.HP, e.MaxHP = 30, 30
	wiz := f.build(t, "u1", "wizard", 5)
	// fireball 8d6 of 3s = 24, the wolf fails (d20 2)
	res := Resolve(f.ctx(3, 3, 3, 3, 3, 3, 3, 3, 2), wiz, []Action{{Kind: ActCast, Spell: "fireball", Zone: e.ZoneID}})
	if stepErr(res, 0) != "" {
		t.Fatal(stepErr(res, 0))
	}
	if e.HP != 6 || SlotsLeft(wiz)[2] != 1 {
		t.Fatalf("fireball: hp %d slots %v", e.HP, SlotsLeft(wiz))
	}

	g := newFixture(t)
	g.fight(t, 0, 1, wolf)
	cl := g.build(t, "u1", "cleric", 1)
	Resolve(g.ctx(1), cl, []Action{{Kind: ActCast, Spell: "bless"}})
	if cl.Conc == nil || !cl.Conditions.Has(CondBlessed) || !g.bram.Conditions.Has(CondBlessed) {
		t.Fatalf("bless: conc %+v aria %v bram %v", cl.Conc, cl.Conditions, g.bram.Conditions)
	}
	// a hit for 6 forces a DC 10 CON save; a 1 breaks concentration
	_, lines := HurtCharacter(g.w, roller(1, 1), cl, 6, nil, true)
	if cl.Conc != nil || g.bram.Conditions.Has(CondBlessed) {
		t.Fatalf("concentration held: %v", lines)
	}
}

func TestBarbarianAndFighterFeatures(t *testing.T) {
	f := newFixture(t)
	f.fight(t, 0, 1, wolf)
	bar := f.build(t, "u1", "barbarian", 1)
	res := Resolve(f.ctx(), bar, []Action{{Kind: ActFeature, Feature: "rage"}})
	if stepErr(res, 0) != "" || !bar.Conditions.Has(CondRaging) || f.w.Campaign.Turns.Budget.Bonus != 0 || f.w.Campaign.Turns.Budget.Action != 1 {
		t.Fatalf("rage: %q %+v", stepErr(res, 0), f.w.Campaign.Turns.Budget)
	}
	hp := bar.HP
	if got, _ := HurtCharacter(f.w, roller(10), bar, 9, nil, true); got != 5 || bar.HP != hp-5 {
		t.Fatalf("rage halves damage: took %d", got)
	}
	if ArmorClass(f.w, bar) != 10+bar.Mod(DEX)+bar.Mod(CON) {
		t.Fatalf("unarmored defense: AC %d", ArmorClass(f.w, bar))
	}

	g := newFixture(t)
	g.fight(t, 0, 1, wolf)
	ftr := g.build(t, "u1", "fighter", 2)
	ftr.HP = 3
	res = Resolve(g.ctx(5), ftr, []Action{{Kind: ActFeature, Feature: "Second Wind"}, {Kind: ActFeature, Feature: "action_surge"}})
	b := g.w.Campaign.Turns.Budget
	if ftr.HP != 3+5+2 || b.Action != 2 || b.Bonus != 0 || resourceLeft(ftr, "action_surge") != 0 {
		t.Fatalf("second wind and surge: hp %d budget %+v errs %q %q", ftr.HP, b, stepErr(res, 0), stepErr(res, 1))
	}
}

func TestLevelUpChoicesAndRests(t *testing.T) {
	f := newFixture(t)
	wl := f.build(t, "u1", "warlock", 3)
	if wl.HasFeature("dark_ones_blessing") == false {
		t.Fatal("subclass feature at 3")
	}
	AwardXP(wl, XPTable[3]-wl.XP)
	if wl.Level != 4 {
		t.Fatalf("level %d", wl.Level)
	}
	pend := PendingChoices(wl)
	if len(pend) != 2 || pend[0].Key != "invocation" || pend[1].Key != "feat4" {
		t.Fatalf("pending: %+v", pend)
	}
	old := wl.Picks["invocation"][0]
	if p := Choose(wl, "invocation", []string{"repelling_blast", "eldritch_mind", "devils_sight"}); len(p) == 0 && old != "repelling_blast" {
		t.Fatal("an earlier invocation was swapped out")
	}
	more := []string{old}
	for _, k := range InvocationKeys() {
		if len(more) < 3 && k != old {
			more = append(more, k)
		}
	}
	must(t, Choose(wl, "invocation", more).Err())
	cha := wl.Scores[CHA]
	must(t, Choose(wl, "feat4", []string{"ability_score_improvement"}).Err())
	must(t, Choose(wl, "asi", []string{"CHA", "CHA"}).Err())
	if wl.Scores[CHA] != min(20, cha+2) || len(PendingChoices(wl)) != 0 {
		t.Fatalf("asi: %d -> %d, pending %+v", cha, wl.Scores[CHA], PendingChoices(wl))
	}
	if p := Choose(wl, "asi", []string{"STR", "STR"}); len(p) == 0 {
		t.Fatal("asi chosen twice")
	}
	spendSlot(wl, 2)
	spendSlot(wl, 2)
	if SlotsLeft(wl)[1] != 0 {
		t.Fatalf("pact slots %v", SlotsLeft(wl))
	}
	ShortRest(wl)
	if SlotsLeft(wl)[1] != 2 {
		t.Fatalf("short rest: pact slots %v", SlotsLeft(wl))
	}
}

func TestOldSheetsPlayAsTheirClass(t *testing.T) {
	f := newFixture(t)
	if ClassOf(f.aria).Key != "fighter" || ClassOf(f.bram).Key != "ranger" {
		t.Fatal("kits map to classes")
	}
	if !f.aria.HasFeature("second_wind") {
		t.Fatal("an old warrior has second wind")
	}
	c, err := CreateCharacter(f.w, "u3", CharacterSpec{Name: "Ione", Kit: "arcanist",
		Scores: map[string]int{"STR": 8, "DEX": 12, "CON": 13, "INT": 15, "WIS": 14, "CHA": 10}, Proficient: []string{"INT", "WIS"}})
	must(t, err)
	if !c.KnowsSpell("fire_bolt") || !c.KnowsSpell("magic_missile") {
		t.Fatalf("old arcanist spells: %v", c.SpellList())
	}
	if c.SaveMod(INT) != c.Mod(INT)+2 {
		t.Fatal("old sheets keep their proficient saves")
	}
}

func TestSpecOfRoundTrips(t *testing.T) {
	f := newFixture(t)
	for _, k := range ClassKeys() {
		c := f.build(t, "u1", k, 1)
		spec := SpecOf(c)
		if spec == nil {
			t.Fatalf("%s: no spec", k)
		}
		scores, p := ValidateBuild(*spec)
		if len(p) > 0 {
			t.Fatalf("%s: spec from sheet is invalid: %v", k, p)
		}
		for _, a := range Abilities {
			if scores[a] != c.Scores[a] {
				t.Fatalf("%s: %s %d, sheet has %d", k, a, scores[a], c.Scores[a])
			}
		}
	}
}

// Within one era no two classes, species, backgrounds or spells may share a
// name, or the builder and the DM can't tell them apart.
func TestEraNamesAreDistinct(t *testing.T) {
	for _, set := range []string{SettingFantasy, SettingGaslight, SettingPulp, SettingModern, SettingFuture} {
		seen := map[string]string{}
		check := func(kind, key, name string) {
			k := kind + ":" + strings.ToLower(name)
			if o, ok := seen[k]; ok {
				t.Errorf("%s: %s %q is used by %s and %s", set, kind, name, o, key)
			}
			seen[k] = key
		}
		for _, k := range ClassKeys() {
			check("class", k, ClassName(Classes[k], set))
		}
		for _, k := range SpeciesKeys() {
			check("species", k, SpeciesName(Species[k], set))
		}
		for _, k := range BackgroundKeys() {
			check("background", k, BackgroundName(Backgrounds[k], set))
		}
		for _, k := range SpellKeys() {
			check("spell", k, SpellName(Spells[k], set))
		}
	}
}
