package saga

import (
	"testing"

	"github.com/Solvely-Colin/aidnd/internal/game"
)

func buildSaga(t *testing.T, key string) *game.World {
	t.Helper()
	s := Get(key)
	if s == nil {
		t.Fatalf("no saga %q", key)
	}
	w, err := Build(s, "c1", Options{MaxSetbacks: 3, MaxPlayers: 5})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return w
}

func TestLongDarkStartsInInnsmouth(t *testing.T) {
	w := buildSaga(t, "long-dark")
	c := w.Campaign
	if c.Saga != "long-dark" || !c.Dread || c.Setting != game.SettingPulp || c.Title != "The Long Dark Below" {
		t.Fatalf("campaign: %q %+v", c.Title, c.CampaignExtra)
	}
	if w.Here() == nil || w.Here().Name != "Gilman House" || !w.Here().IsHub || w.Here().Danger != 0 {
		t.Fatal("party must start at the Gilman House")
	}
	mq := w.MainQuest()
	if mq == nil || mq.Title != "Salt and Silence" || len(mq.Objectives) != 6 {
		t.Fatalf("storyline: %+v", mq)
	}
	if last := mq.Objectives[5]; last.Kind != game.ObjDefeatEnemy || last.Target != c.AntagonistID {
		t.Fatalf("last objective must defeat the boss: %+v", last)
	}
	if boss := w.NPC(c.AntagonistID); boss == nil || boss.LocationID != c.LairID || boss.Stats.AsBoss.Minion != "Deep One" {
		t.Fatalf("boss: %+v", boss)
	}
	solo := 0
	for _, q := range innsmouth.Side {
		if q.Solo {
			solo++
		}
	}
	if solo < 3 {
		t.Fatalf("a region needs 3 solo side quests, has %d", solo)
	}
	for _, name := range []string{"Hannah Coyle", "Zadok Allen", "Eliza Pell", "Mr. Nye", "Agent Ruth Halloway"} {
		if n := exactNPC(w, name); n == nil || !n.Protected {
			t.Errorf("%s should be protected", name)
		}
	}
}

func TestOtherRegionsWaitOnTheRoad(t *testing.T) {
	w := buildSaga(t, "long-dark")
	lib := exact(w, "Orne Library")
	if lib == nil || !lib.Stub || !lib.IsHub || lib.Region != "arkham" {
		t.Fatalf("Arkham should be a stub hub on the road: %+v", lib)
	}
	if w.EdgeBetween(w.Campaign.HubID, lib.ID) == nil {
		t.Fatal("no road from Innsmouth to Arkham")
	}
	if exact(w, "Dunwich Village") != nil || exactNPC(w, "Curtis Whateley") != nil {
		t.Fatal("Arkham must not be built before anyone goes there")
	}
	built, err := Arrive(w, lib)
	if err != nil || !built {
		t.Fatalf("arrive: %v %v", built, err)
	}
	if lib.Stub || lib.Description == "" || exact(w, "Sentinel Hill") == nil || exactNPC(w, "Curtis Whateley") == nil {
		t.Fatal("Arkham should be built on arrival")
	}
	if again, _ := Arrive(w, lib); again {
		t.Fatal("a region is built once")
	}
	if !w.Connected() {
		t.Fatal("map split after building Arkham")
	}
	mains := 0
	for _, q := range w.Quests {
		if q.IsMain {
			mains++
		}
	}
	if mains != 2 {
		t.Fatalf("each region has its own storyline, got %d", mains)
	}
	// standing in Arkham, Arkham's storyline is the one that matters
	w.Campaign.LocationID = lib.ID
	if mq := w.MainQuest(); mq == nil || mq.Title != "The Sunken Codex" {
		t.Fatalf("main quest in Arkham: %+v", mq)
	}
}

func TestEveryRegionBuilds(t *testing.T) {
	s := Get("long-dark")
	w := buildSaga(t, "long-dark")
	for _, r := range s.Regions {
		for _, l := range w.Locations {
			if l.IsHub && l.Region == r.Key && l.Stub {
				if _, err := Arrive(w, l); err != nil {
					t.Fatalf("region %s: %v", r.Key, err)
				}
			}
		}
	}
	if !w.Connected() {
		t.Fatal("the whole world must be one map")
	}
}

func TestSecretsStayShutUntilUncovered(t *testing.T) {
	w := buildSaga(t, "long-dark")
	var bargain *game.Lore
	for _, l := range w.Lore {
		if l.Title == "The bargain of 1846" {
			bargain = l
		}
	}
	if bargain == nil || game.LoreOpen(w, bargain) {
		t.Fatal("the 1846 bargain must start hidden")
	}
	mq := w.MainQuest()
	mq.Objectives[0].Done, mq.Objectives[1].Done = true, true
	if !game.LoreOpen(w, bargain) {
		t.Fatal("talking to Zadok opens the 1846 bargain")
	}
}

func TestMilestoneRaisesLevel(t *testing.T) {
	w := buildSaga(t, "long-dark")
	ch := &game.Character{ID: "pc1", Name: "Cal", Level: 1, HP: 10, MaxHP: 10, Scores: game.Scores{game.STR: 10, game.DEX: 14, game.CON: 12, game.INT: 13, game.WIS: 12, game.CHA: 10}}
	w.Characters = append(w.Characters, ch)
	mq := w.MainQuest()
	for _, it := range w.Items {
		if it.Name == "Logbook of the Mary Alder" {
			it.Owner = game.Owner{Kind: game.OwnerCharacter, ID: ch.ID}
		}
	}
	exactNPC(w, "Zadok Allen").LastTalked = w.Campaign.Turn + 1
	game.EvaluateQuests(w)
	if !mq.Objectives[1].Done || ch.Level < 2 {
		t.Fatalf("milestone should lift Cal to level 2: level %d, objectives %+v", ch.Level, mq.Objectives)
	}
}

func TestListShowsSaga(t *testing.T) {
	var found bool
	for _, i := range List() {
		if i.Key == "long-dark" {
			found = i.Start == "Innsmouth" && i.Regions >= 2
		}
	}
	if !found {
		t.Fatalf("List: %+v", List())
	}
}

func TestEveryRegionsAgendasBuild(t *testing.T) {
	s := Get("long-dark")
	w := buildSaga(t, "long-dark")
	for _, l := range w.Locations {
		if l.IsHub && l.Stub {
			if _, err := Arrive(w, l); err != nil {
				t.Fatalf("arrive %s: %v", l.Name, err)
			}
		}
	}
	want := 0
	for _, r := range s.Regions {
		want += len(r.Agendas)
	}
	lv := w.Campaign.Living
	if want == 0 || len(lv.Agendas) != want {
		t.Fatalf("agendas: %d, want %d", len(lv.Agendas), want)
	}
	// a world made before agendas gets them back for the regions it built
	lv.Agendas = nil
	if !EnsureAgendas(w) || len(lv.Agendas) != want || EnsureAgendas(w) {
		t.Fatalf("EnsureAgendas: %d, want %d", len(lv.Agendas), want)
	}
	for _, a := range lv.Agendas {
		for _, st := range a.Steps {
			for _, e := range st.Effects {
				if e.Clock != "" && lv.Faction(e.Clock) == nil {
					t.Fatalf("agenda %s pushes unknown clock %q", a.Key, e.Clock)
				}
			}
			if st.If != nil && st.If.Clock != "" && lv.Faction(st.If.Clock) == nil {
				t.Fatalf("agenda %s waits on unknown clock %q", a.Key, st.If.Clock)
			}
		}
	}
}
