package game

import (
	"strings"
	"testing"
)

// Regression tests for the 2026-10-31 pre-play-test audit.

func TestDefeatObjectiveMatchesWholeNames(t *testing.T) {
	cases := []struct {
		name, target string
		want         bool
	}{
		{"Wolf 2", "wolf", true},
		{"Wolf 2", "the wolves", true},
		{"Cultist 1", "Cultists of the Deep", true},
		{"Bandit Captain", "the bandit captain", true},
		{"Giant Spider 2", "giant spiders", true},
		{"Bandit 2", "the bandit captain", false}, // a plain bandit isn't the captain
		{"Rat", "the pirate captain", false},      // letters inside a word don't count
		{"Pirate", "rat", false},
		{"Fishman 3", "the fishmen", true},
	}
	for _, c := range cases {
		if got := defeatMatches(c.name, c.target); got != c.want {
			t.Errorf("defeatMatches(%q, %q) = %v, want %v", c.name, c.target, got, c.want)
		}
	}
}

func TestDefeatingAMinionDoesNotFinishTheBossQuest(t *testing.T) {
	f := newFixture(t)
	q := &Quest{ID: "q9", Title: "The captain", Status: QuestActive, Objectives: []Objective{
		{ID: "o1", Kind: ObjDefeatEnemy, Target: "the bandit captain", Text: "Defeat the bandit captain"}}}
	f.w.Quests = append(f.w.Quests, q)
	f.w.Defeats = append(f.w.Defeats, DefeatRecord{Name: "Bandit 2", Turn: 1})
	EvaluateQuests(f.w)
	if q.Objectives[0].Done {
		t.Fatal("a plain bandit finished the captain's objective")
	}
	f.w.Defeats = append(f.w.Defeats, DefeatRecord{Name: "Bandit Captain", Turn: 2})
	EvaluateQuests(f.w)
	if !q.Objectives[0].Done {
		t.Fatal("defeating the captain should finish it")
	}
}

func TestReplacementCharacterLevelsLikeTheParty(t *testing.T) {
	f := newFixture(t)
	f.w.Players = append(f.w.Players, &Player{ID: "u3", Name: "Cy"})
	c, err := CreateCharacter(f.w, "u3", FillBuild(CharacterSpec{Name: "Wren", ClassKey: "wizard"}))
	must(t, err)
	JoinMidway(f.w, c, XPTable[2], f.wood.Zones[0].ID) // level 3
	if c.Level != 3 {
		t.Fatalf("level %d", c.Level)
	}
	if want := MaxHPFor(c); c.MaxHP != want || c.HP != want {
		t.Fatalf("HP %d/%d, want %d from the wizard's hit die", c.HP, c.MaxHP, want)
	}
	second := false
	for _, k := range c.Spells {
		if Spells[k] != nil && Spells[k].Level == 2 {
			second = true
		}
	}
	if !second {
		t.Fatalf("a level 3 wizard should have a level 2 spell prepared: %v", c.Spells)
	}
}

func TestHealedCharacterLosesTheirWaitingDeathSave(t *testing.T) {
	f := newFixture(t)
	f.fight(t, 0, 1, wolf)
	KnockOut(f.w, f.bram)
	f.w.Campaign.SetPending(&PendingAction{Kind: PendingDeathSave, CharacterID: f.bram.ID, PlayerID: "u2"})
	DropStaleDeathSaves(f.w)
	if f.w.Campaign.Pending[f.bram.ID] == nil {
		t.Fatal("a dying character keeps their death save")
	}
	Heal(f.bram, 5)
	DropStaleDeathSaves(f.w)
	if f.w.Campaign.Pending != nil {
		t.Fatal("a healed character should not be asked for a death save")
	}
}

func TestEnemyPhaseReportsHorrorAndAFightWonByAReaction(t *testing.T) {
	// a readied attack kills the last foe as it closes in: the win, XP and
	// loot lines come back for the players to see
	f := newFixture(t)
	f.fight(t, 0, 1, EnemySpec{Name: "Wolf", HP: 1, AC: 5, AttackBonus: 3, Damage: "1d6"})
	f.bram.ZoneID = f.wood.Zones[3].ID
	Resolve(f.ctx(), f.aria, []Action{{Kind: ActReady}})
	_, extra := RunEnemyTurns(f.w, roller(15, 5))
	if f.w.InCombat() || !strings.Contains(strings.Join(extra, "\n"), "The fight is won") {
		t.Fatalf("fight-won lines missing: %v", extra)
	}
	// first sight of a horror in a dread campaign
	g := newFixture(t)
	g.w.Campaign.Dread = true
	g.fight(t, 0, 1, EnemySpec{Name: "Deep One", HP: 20, AC: 12, AttackBonus: 3, Damage: "1d6", Horror: 2})
	_, extra = RunEnemyTurns(g.w, roller(2))
	joined := strings.Join(extra, "\n")
	if !strings.Contains(joined, "Deep One") || (!strings.Contains(joined, "shaken") && !strings.Contains(joined, "nerve")) {
		t.Fatalf("horror saves missing: %v", extra)
	}
}

func TestTurnEndsWhenTheActorIsKnockedOutMidTurn(t *testing.T) {
	f := newFixture(t)
	f.fight(t, 0, 0, wolf) // the wolf stands with the party on the Road
	f.aria.HP = 1
	r := Resolve(f.ctx(20), f.aria, []Action{{Kind: ActReposition, Zone: f.wood.Zones[1].ID}})
	if !f.aria.Down() {
		t.Fatalf("the opportunity attack should drop Aria: %v", r.Changes)
	}
	if !r.TurnEnded {
		t.Fatal("a knocked-out actor's turn should end instead of waiting for a skip")
	}
}
