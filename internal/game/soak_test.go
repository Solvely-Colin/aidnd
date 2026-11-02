package game

import (
	"fmt"
	"math/rand"
	"runtime/debug"
	"testing"

	"github.com/Solvely-Colin/aidnd/internal/dice"
)

// TestSoakRandomFights plays many random fights with class characters of
// every class, picking random actions (attacks, spells, features, tactics,
// moves), and checks the engine never panics and keeps its numbers sane.
func TestSoakRandomFights(t *testing.T) {
	seeds := 60
	if testing.Short() {
		seeds = 10
	}
	for seed := int64(1); seed <= int64(seeds); seed++ {
		soakOne(t, seed)
	}
}

func soakOne(t *testing.T, seed int64) {
	t.Helper()
	rng := rand.New(rand.NewSource(seed))
	f := newFixture(t)
	w := f.w
	// replace the party with four class characters
	w.Characters, w.Items = nil, nil
	keys := ClassKeys()
	w.Players = nil
	for i := 0; i < 4; i++ {
		pid := fmt.Sprintf("p%d", i)
		w.Players = append(w.Players, &Player{ID: pid, Name: pid})
		key := keys[rng.Intn(len(keys))]
		spec := FillBuild(CharacterSpec{Name: fmt.Sprintf("Hero%d", i), ClassKey: key})
		c, err := CreateCharacter(w, pid, spec)
		if err != nil {
			t.Fatalf("seed %d: create %s: %v", seed, key, err)
		}
		AwardXP(c, XPTable[rng.Intn(len(XPTable))])
		for _, ch := range PendingChoices(c) {
			Choose(c, ch.Key, defaultPicks(c, ch))
		}
	}
	must(t, StartGame(w, t0))
	f.goTo(f.wood, 0)
	r := dice.New(seed)
	ctx := &Ctx{W: w, R: r, Now: t0}
	defer func() {
		if p := recover(); p != nil {
			t.Fatalf("seed %d: panic: %v\n%s", seed, p, debug.Stack())
		}
	}()
	fights := 0
	for turn := 0; turn < 400; turn++ {
		if w.Campaign.Status != StatusActive {
			return
		}
		if !w.InCombat() {
			if fights >= 4 {
				return
			}
			fights++
			_, _, err := StartEncounter(w, GenericEnemies(KindWilderness, 2+rng.Intn(3), 4))
			if err != nil {
				t.Fatalf("seed %d: start: %v", seed, err)
			}
			w.Campaign.Turns.Budget = FullBudget(true)
		}
		actor := w.ActiveCharacter()
		if actor == nil {
			t.Fatalf("seed %d: no active character", seed)
		}
		if !actor.Down() {
			acts := randomActions(w, actor, rng)
			Resolve(ctx, actor, acts)
		}
		checkInvariants(t, seed, w)
		if !w.InCombat() {
			continue
		}
		EndCharacterTurn(r, actor)
		RunEnemyTurns(w, r)
		checkInvariants(t, seed, w)
		if AllDown(w) {
			Setback(w)
			continue
		}
		next := EndTurn(w, t0)
		if next == nil {
			Setback(w)
			continue
		}
		if next.Dying() && w.InCombat() {
			DeathSave(w, r, next)
		}
		if !next.Down() {
			StartCharacterTurn(next, &w.Campaign.Turns.Budget)
			StartTurnEffects(w, r, next)
		}
		if w.Encounter != nil && len(w.Encounter.Living()) == 0 {
			EndFight(w)
		}
		checkInvariants(t, seed, w)
	}
}

func randomActions(w *World, c *Character, rng *rand.Rand) []Action {
	var foes []*Enemy
	if w.Encounter != nil {
		foes = w.Encounter.Living()
	}
	pickFoe := func() string {
		if len(foes) == 0 {
			return ""
		}
		return foes[rng.Intn(len(foes))].ID
	}
	pickAlly := func() string {
		p := w.Party()
		return p[rng.Intn(len(p))].ID
	}
	l := w.Here()
	pickZone := func() string { return l.Zones[rng.Intn(len(l.Zones))].ID }
	var out []Action
	for i := 0; i < 1+rng.Intn(3); i++ {
		var a Action
		switch rng.Intn(9) {
		case 0, 1:
			a = Action{Kind: ActAttack, Target: pickFoe()}
			if rng.Intn(3) == 0 {
				a.Option = []string{"smite", "stun"}[rng.Intn(2)]
			}
		case 2, 3:
			sp := c.SpellList()
			if len(sp) == 0 {
				continue
			}
			a = Action{Kind: ActCast, Spell: sp[rng.Intn(len(sp))], Level: rng.Intn(4)}
			switch rng.Intn(3) {
			case 0:
				a.Target = pickFoe()
			case 1:
				a.Target = pickAlly()
			default:
				a.Zone = pickZone()
			}
		case 4:
			uses := FeatureUses(w, c)
			if len(uses) == 0 {
				continue
			}
			fa := uses[rng.Intn(len(uses))].FeatureAct
			a = Action{Kind: ActFeature, Feature: fa.Key, Level: rng.Intn(4)}
			switch fa.Target {
			case TargetFoe:
				a.Target = pickFoe()
			case TargetAlly, TargetAny:
				a.Target = pickAlly()
			case TargetZone:
				a.Zone = pickZone()
			}
		case 5:
			kinds := append([]string{}, tacticKinds...)
			a = Action{Kind: kinds[rng.Intn(len(kinds))], Target: pickFoe()}
			if a.Kind == ActHelp {
				a.Target = pickAlly()
			}
		case 6:
			a = Action{Kind: ActReposition, Zone: pickZone()}
		case 7:
			a = Action{Kind: ActCheck, Purpose: []string{"stabilize", "escape", "other"}[rng.Intn(3)], Ability: "STR"}
		default:
			a = Action{Kind: ActEndTurn}
		}
		out = append(out, a)
	}
	return out
}

func checkInvariants(t *testing.T, seed int64, w *World) {
	t.Helper()
	for _, c := range w.Characters {
		if c.HP < 0 || c.HP > c.MaxHP {
			t.Fatalf("seed %d: %s HP %d/%d", seed, c.Name, c.HP, c.MaxHP)
		}
		if c.TempHP < 0 {
			t.Fatalf("seed %d: %s temp HP %d", seed, c.Name, c.TempHP)
		}
		if c.HP > 0 && c.Death != nil {
			t.Fatalf("seed %d: %s up with death saves %+v", seed, c.Name, *c.Death)
		}
		for _, r := range Resources(c) {
			if c.Used[r.Key] > r.Max {
				t.Fatalf("seed %d: %s used %d of %d %s", seed, c.Name, c.Used[r.Key], r.Max, r.Key)
			}
		}
		mx := SlotsMax(c)
		for i, u := range c.SlotsUsed {
			if i < len(mx) && u > mx[i] {
				t.Fatalf("seed %d: %s slot level %d used %d of %d", seed, c.Name, i+1, u, mx[i])
			}
		}
	}
	if w.Encounter != nil {
		for _, e := range w.Encounter.Enemies {
			if e.HP < 0 || e.HP > e.MaxHP {
				t.Fatalf("seed %d: %s HP %d/%d", seed, e.Name, e.HP, e.MaxHP)
			}
		}
	}
}
