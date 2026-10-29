package saga

import (
	"testing"

	"github.com/Solvely-Colin/aidnd/internal/game"
)

func buildAct(t *testing.T, key string, n int) *game.World {
	t.Helper()
	s := Get(key)
	if s == nil {
		t.Fatalf("no saga %q", key)
	}
	w, err := Build(s, n, "c1", Options{MaxSetbacks: 3, MaxPlayers: 5})
	if err != nil {
		t.Fatalf("build act %d: %v", n, err)
	}
	return w
}

func TestLongDarkActOneBuilds(t *testing.T) {
	w := buildAct(t, "long-dark", 1)
	c := w.Campaign
	if c.Saga != "long-dark" || c.Act != 1 || !c.Dread || c.Setting != game.SettingPulp {
		t.Fatalf("campaign flags: %+v", c.CampaignExtra)
	}
	if w.Here() == nil || !w.Here().IsHub || w.Here().Danger != 0 {
		t.Fatal("party must start at a safe hub")
	}
	mq := w.MainQuest()
	if mq == nil || len(mq.Objectives) != 6 {
		t.Fatalf("main quest: %+v", mq)
	}
	if last := mq.Objectives[5]; last.Kind != game.ObjDefeatEnemy || last.Target != c.AntagonistID {
		t.Fatalf("last objective must defeat the boss: %+v", last)
	}
	boss := w.NPC(c.AntagonistID)
	if boss == nil || boss.LocationID != c.LairID || boss.Stats.AsBoss == nil || boss.Stats.AsBoss.Minion != "Deep One" {
		t.Fatalf("boss: %+v", boss)
	}
	if len(w.Quests) != 5 {
		t.Fatalf("want main + 4 side quests, got %d", len(w.Quests))
	}
	solo := 0
	for _, q := range actOne.Side {
		if q.Solo {
			solo++
		}
	}
	if solo < 3 {
		t.Fatalf("act needs 3 solo side quests, has %d", solo)
	}
	if len(c.Milestones) != 3 {
		t.Fatalf("milestones: %v", c.Milestones)
	}
	for _, name := range []string{"Hannah Coyle", "Zadok Allen", "Eliza Pell", "Mr. Nye", "Agent Ruth Halloway"} {
		if n := exactNPC(w, name); n == nil || !n.Protected {
			t.Errorf("%s should be protected", name)
		}
	}
	// the lair is behind the gate until the clue chain is done
	gate := false
	for _, e := range w.Edges {
		if e.Touches(c.LairID) {
			gate = e.LairGate && e.State == game.EdgeLocked
		}
	}
	if !gate {
		t.Fatal("lair must sit behind a locked lair gate")
	}
}

func TestSecretsStayShutUntilUncovered(t *testing.T) {
	w := buildAct(t, "long-dark", 1)
	var bargain *game.Lore
	open := 0
	for _, l := range w.Lore {
		if l.Title == "The bargain of 1846" {
			bargain = l
		}
		if game.LoreOpen(w, l) {
			open++
		}
	}
	if bargain == nil || game.LoreOpen(w, bargain) {
		t.Fatal("the 1846 bargain must start hidden")
	}
	if open != 3 {
		t.Fatalf("want 3 open secrets at the start, got %d", open)
	}
	mq := w.MainQuest()
	mq.Objectives[0].Done, mq.Objectives[1].Done = true, true
	if !game.LoreOpen(w, bargain) {
		t.Fatal("talking to Zadok opens the 1846 bargain")
	}
}

func TestMilestoneRaisesLevel(t *testing.T) {
	w := buildAct(t, "long-dark", 1)
	ch := &game.Character{ID: "pc1", Name: "Cal", Level: 1, HP: 10, MaxHP: 10, Scores: game.Scores{game.STR: 10, game.DEX: 14, game.CON: 12, game.INT: 13, game.WIS: 12, game.CHA: 10}}
	w.Characters = append(w.Characters, ch)
	mq := w.MainQuest()
	// hand the party the logbook and talk to Zadok: objective 2 is the first milestone
	for _, it := range w.Items {
		if it.Name == "Logbook of the Mary Alder" {
			it.Owner = game.Owner{Kind: game.OwnerCharacter, ID: ch.ID}
		}
	}
	exactNPC(w, "Zadok Allen").LastTalked = w.Campaign.Turn + 1
	game.EvaluateQuests(w)
	if !mq.Objectives[1].Done {
		t.Fatalf("objectives: %+v", mq.Objectives)
	}
	if ch.Level < 2 {
		t.Fatalf("milestone should lift Cal to level 2, got %d (xp %d)", ch.Level, ch.XP)
	}
}

func TestListShowsSaga(t *testing.T) {
	var found bool
	for _, i := range List() {
		if i.Key == "long-dark" {
			found = i.Acts >= 1 && i.Levels == "1-4"
		}
	}
	if !found {
		t.Fatalf("List: %+v", List())
	}
}
