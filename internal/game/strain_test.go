package game

import (
	"testing"

	"github.com/Solvely-Colin/aidnd/internal/dice"
)

func TestHorrorSaveAddsStrain(t *testing.T) {
	f := newFixture(t)
	c := f.aria
	// a 2 on the save fails DC 12
	so := FaceHorror(dice.FromSource(dice.NewScripted(2)), c, 2, "the thing in the pool")
	if so == nil || so.Gained != 1 || c.Strain != 1 {
		t.Fatalf("strain after failed save: %+v, strain %d", so, c.Strain)
	}
	// a natural 1 adds two
	FaceHorror(dice.FromSource(dice.NewScripted(1)), c, 2, "it")
	if c.Strain != 3 {
		t.Fatalf("natural 1 should add 2, strain %d", c.Strain)
	}
	// a 19 keeps their nerve
	if so := FaceHorror(dice.FromSource(dice.NewScripted(19)), c, 2, "it"); so.Gained != 0 || c.Strain != 3 {
		t.Fatalf("passed save changed strain: %+v %d", so, c.Strain)
	}
}

func TestStrainEpisodeAndScar(t *testing.T) {
	f := newFixture(t)
	c := f.aria
	c.Strain = 3
	// failing at 3 reaches 4: a short episode
	so := FaceHorror(dice.FromSource(dice.NewScripted(2)), c, 1, "it")
	if !so.Episode || !c.Conditions.Has(CondFrightened) {
		t.Fatalf("no episode at 4: %+v %v", so, c.Conditions)
	}
	c.Conditions = nil
	c.Strain = 5
	// failing at 5 reaches 6: a scar, then back to 3 (the second die picks the scar)
	so = FaceHorror(dice.FromSource(dice.NewScripted(2, 3)), c, 1, "it")
	if so.Scar == "" || len(c.Scars) != 1 || c.Strain != StrainAfter {
		t.Fatalf("no scar at 6: %+v scars %v strain %d", so, c.Scars, c.Strain)
	}
	RecoverStrain(c, 1)
	if c.Strain != 2 {
		t.Fatalf("recover: %d", c.Strain)
	}
}

func TestStrainGivesDisadvantageAgainstFear(t *testing.T) {
	f := newFixture(t)
	c := f.aria
	c.Strain = StrainShaken
	r := CharacterSave(dice.FromSource(dice.NewScripted(15, 4)), c, WIS, 12, CondFrightened)
	if r.Edge != "disadvantage" || r.Natural != 4 {
		t.Fatalf("strain should give disadvantage against fear: %+v", r)
	}
}

func TestHorrorEnemyIsFacedOncePerKind(t *testing.T) {
	f := newFixture(t)
	f.w.Campaign.Dread = true
	e := f.fight(t, 0, 0, EnemySpec{Name: "Deep One", HP: 10, AC: 12, AttackBonus: 3, Damage: "1d6", Horror: 2})
	if e.Horror != 2 {
		t.Fatalf("horror rank not kept: %d", e.Horror)
	}
	lines := FaceEnemyHorrors(f.w, dice.FromSource(dice.NewScripted(19, 19, 19, 19)))
	if len(lines) != len(f.w.Party()) || !e.Faced {
		t.Fatalf("each character saves once: %v", lines)
	}
	if again := FaceEnemyHorrors(f.w, dice.New(1)); len(again) != 0 {
		t.Fatalf("faced twice: %v", again)
	}
	f.w.Campaign.Dread = false
	e.Faced = false
	if lines := FaceEnemyHorrors(f.w, dice.New(1)); len(lines) != 0 {
		t.Fatal("no horror saves outside dread campaigns")
	}
}
