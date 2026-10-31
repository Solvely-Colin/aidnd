package game

import (
	"strings"
	"testing"

	"github.com/Solvely-Colin/aidnd/internal/dice"
)

func TestAttackingAnNPCLeavesAPersonalGrudge(t *testing.T) {
	f := newFixture(t)
	w := f.w
	w.Campaign.Turns.Budget = FullBudget(false)
	f.smith.ZoneID = f.aria.ZoneID
	r := Resolve(f.ctx(), f.aria, []Action{{Kind: ActAttack, Target: "Dunna"}})
	if stepErr(r, 0) != "" {
		t.Fatal(stepErr(r, 0))
	}
	ties := f.smith.TiesTo(f.aria.ID)
	if len(ties) != 1 || ties[0].Kind != TieGrudge || ties[0].Source != SourceEngine {
		t.Fatalf("aria's ties %+v", ties)
	}
	if len(f.smith.TiesTo(f.bram.ID)) != 0 {
		t.Fatal("bram didn't attack anyone")
	}
}

func TestAttitudeIsPerCharacter(t *testing.T) {
	f := newFixture(t)
	w := f.w
	n := f.smith
	n.Disposition = 1
	n.Bind(w, f.aria, TieGrudge, "cheated them at dice", SourceGM)
	n.Bind(w, f.bram, TieDebt, "pulled them from the river", SourceGM)
	if n.AttitudeTo(f.aria) != 0 || n.AttitudeTo(f.bram) != 2 || n.AttitudeTo(nil) != 1 {
		t.Fatalf("attitudes aria %d bram %d party %d", n.AttitudeTo(f.aria), n.AttitudeTo(f.bram), n.AttitudeTo(nil))
	}
	it := &Item{Value: 50}
	if BuyPrice(it, n, f.aria) != 50 || BuyPrice(it, n, f.bram) != 40 {
		t.Fatalf("prices aria %d bram %d", BuyPrice(it, n, f.aria), BuyPrice(it, n, f.bram))
	}
	// a newer reason replaces the old one; settling clears it
	n.Bind(w, f.aria, TieGrudge, "broke their nose", SourceGM)
	if ts := n.TiesTo(f.aria.ID); len(ts) != 1 || ts[0].Text != "broke their nose" {
		t.Fatalf("rebind %+v", ts)
	}
	if !n.Settle(f.aria.ID, "") || len(n.TiesTo(f.aria.ID)) != 0 || n.Settle(f.aria.ID, "") {
		t.Fatal("settle")
	}
	if !strings.Contains(TieLine(w, n, n.TiesTo(f.bram.ID)[0]), "Dunna owes Bram (pulled them from the river") {
		t.Fatalf("tie line %q", TieLine(w, n, n.TiesTo(f.bram.ID)[0]))
	}
}

func TestAMerchantWithAGrudgeRefusesOnlyThatCharacter(t *testing.T) {
	f := newFixture(t)
	w := f.w
	f.smith.Disposition = -2
	f.smith.Bind(w, f.aria, TieGrudge, "robbed the forge", SourceGM)
	if _, err := Buy(w, f.aria, f.smith.ID, "longsword"); err == nil || !strings.Contains(err.Error(), "refuses") {
		t.Fatalf("aria bought: %v", err)
	}
	f.bram.Gold = 100
	if _, err := Buy(w, f.bram, f.smith.ID, "longsword"); err != nil {
		t.Fatalf("bram should still be served: %v", err)
	}
}

func TestTurningInAQuestPutsTheGiverInYourDebt(t *testing.T) {
	f := newFixture(t)
	w := f.w
	q := &Quest{ID: "q1", Title: "Iron for the forge", GiverID: f.elder.ID, Status: QuestCompleted}
	w.Quests = append(w.Quests, q)
	f.elder.Bind(w, f.bram, TieOwed, "lent them a horse", SourceGM)
	if _, _, err := TurnInQuest(w, q.ID, f.bram); err != nil {
		t.Fatal(err)
	}
	ts := f.elder.TiesTo(f.bram.ID)
	if len(ts) != 1 || ts[0].Kind != TieDebt || !strings.Contains(ts[0].Text, "Iron for the forge") {
		t.Fatalf("ties %+v", ts)
	}
}

func TestBreakingYourWordEarnsEachCharacterAGrudge(t *testing.T) {
	f := newFixture(t)
	w := f.w
	p := w.Record(ChronicleEntry{Kind: FactPromise, Text: "The party promised Maren silence", NPCIDs: []string{f.elder.ID}})
	if _, err := ResolvePromise(w, p.ID, false, "told the tavern"); err != nil {
		t.Fatal(err)
	}
	for _, c := range []*Character{f.aria, f.bram} {
		if ts := f.elder.TiesTo(c.ID); len(ts) != 1 || ts[0].Kind != TieGrudge {
			t.Fatalf("%s ties %+v", c.Name, ts)
		}
	}
}

func TestFallingInAFightLeavesOneScarADay(t *testing.T) {
	f := newFixture(t)
	w := f.w
	f.goTo(f.wood, 0)
	enc, _, err := StartEncounter(w, []EnemySpec{{Name: "Ghoul", HP: 10, AC: 10, AttackBonus: 3, Damage: "1d6"}})
	must(t, err)
	r := dice.FromSource(dice.NewScripted(10, 10, 10))
	HurtCharacter(w, r, f.aria, 999, enc.Enemies[0], true)
	if len(f.aria.Marks) != 1 || f.aria.Marks[0].Kind != MarkScar || !strings.Contains(f.aria.Marks[0].Text, "Ghoul") {
		t.Fatalf("marks %+v", f.aria.Marks)
	}
	Heal(f.aria, 5)
	HurtCharacter(w, r, f.aria, 999, enc.Enemies[0], true)
	if len(f.aria.Marks) != 1 {
		t.Fatalf("two scars in a day: %+v", f.aria.Marks)
	}
	w.Campaign.Scene.Day++
	Heal(f.aria, 5)
	enc.Enemies[0].Name = "Wight"
	HurtCharacter(w, r, f.aria, 999, enc.Enemies[0], true)
	if len(f.aria.Marks) != 2 {
		t.Fatalf("no scar on a new day: %+v", f.aria.Marks)
	}
}

func TestADeadCompanionIsALossForTheLiving(t *testing.T) {
	f := newFixture(t)
	w := f.w
	f.goTo(f.wood, 0)
	f.aria.HP, f.aria.Death = 0, &DeathSaves{Failures: 2}
	res := DeathSave(w, dice.FromSource(dice.NewScripted(3)), f.aria)
	if !res.Died {
		t.Fatalf("aria should die: %+v", res)
	}
	if len(f.bram.Marks) != 1 || f.bram.Marks[0].Kind != MarkLoss || f.bram.Marks[0].CharID != f.aria.ID || !strings.Contains(f.bram.Marks[0].Text, "Aria") {
		t.Fatalf("bram marks %+v", f.bram.Marks)
	}
	if len(f.aria.Marks) != 0 {
		t.Fatal("the dead don't mourn themselves")
	}
}

func TestAddMarkValidatesAndCaps(t *testing.T) {
	f := newFixture(t)
	w := f.w
	if _, err := AddMark(w, f.aria, Mark{Kind: "tattoo", Text: "x"}); err == nil {
		t.Fatal("bad kind accepted")
	}
	m, err := AddMark(w, f.aria, Mark{Kind: MarkBrand, Text: "The Order's sigil on her palm"})
	if err != nil || m == nil || m.ID != "mark1" || m.Source != SourceGM {
		t.Fatalf("mark %+v %v", m, err)
	}
	if m, _ := AddMark(w, f.aria, Mark{Kind: MarkBrand, Text: "the order's sigil on her palm."}); m != nil {
		t.Fatal("duplicate mark")
	}
	for i := len(f.aria.Marks); i < MaxMarks; i++ {
		must(t, func() error {
			_, err := AddMark(w, f.aria, Mark{Kind: MarkName, Text: strings.Repeat("x", i+1)})
			return err
		}())
	}
	if _, err := AddMark(w, f.aria, Mark{Kind: MarkName, Text: "one too many"}); err == nil {
		t.Fatal("mark cap not enforced")
	}
}
