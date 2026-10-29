package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Solvely-Colin/aidnd/internal/dice"
	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/store/d1stub"
)

var ctx = context.Background()

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// richWorld exercises every table: an active fight, a quest with reward
// items held by the giver, lore, beats, chronicle and defeats.
func richWorld(t *testing.T) *game.World {
	t.Helper()
	w := &game.World{Campaign: game.Campaign{ID: "camp1", Title: "Ashes", Premise: "a cursed valley", Status: game.StatusLobby, MaxSetbacks: 3, MaxPlayers: 4}}
	hub := &game.Location{ID: w.NewID("loc"), Name: "Oakvale", Kind: game.KindTown, IsHub: true, Visited: true, Description: "Town."}
	wood := w.AddStub("Whisperwood", "pines", "forest", 2)
	lair := w.AddStub("Spire", "tower", "dungeon", 5)
	lair.IsLair = true
	w.Locations = append([]*game.Location{hub}, w.Locations...)
	w.Campaign.HubID, w.Campaign.LairID, w.Campaign.LocationID = hub.ID, lair.ID, hub.ID
	w.Connect(hub.ID, wood.ID, game.EdgeKnown, "the road", "back")
	g := w.Connect(wood.ID, lair.ID, game.EdgeLocked, "spire path", "")
	g.LairGate = true
	game.EnsureZones(w, hub.ID)
	smith := game.AddNPC(w, hub.ID, game.NPCSpec{Name: "Dunna", Role: "smith", Merchant: true, Disposition: 1})
	smith.Remember("met the party")
	smith.Stats = &game.StatBlock{MaxHP: 9, AC: 12, AttackBonus: 2, Damage: "1d6"}
	game.NewItem(w, "longsword", game.Owner{Kind: game.OwnerNPC, ID: smith.ID})
	game.NewMagicGear(w, "longsword", "weapon_flame", game.Owner{Kind: game.OwnerNPC, ID: smith.ID})
	game.NewWondrous(w, "amulet_of_health", game.Owner{Kind: game.OwnerNPC, ID: smith.ID})
	w.Players = []*game.Player{{ID: "u1", Name: "Ann", Token: "secret-token"}}
	_, err := game.CreateCharacter(w, "u1", game.CharacterSpec{Name: "Aria", Kit: "skirmisher",
		Scores: map[string]int{"STR": 8, "DEX": 15, "CON": 14, "INT": 13, "WIS": 12, "CHA": 10}, Proficient: []string{"DEX", "CHA"}})
	must(t, err)
	_, err = game.AddQuest(w, game.QuestSpec{Title: "Pelts", GiverID: smith.ID, RewardGold: 20,
		Objectives:  []game.ObjectiveSpec{{Kind: game.ObjReachLocation, Text: "go", Target: wood.ID}, {Kind: game.ObjFreeform, Text: "tell"}},
		RewardItems: []game.RewardItemSpec{{Key: "healing_potion"}}}, false)
	must(t, err)
	_, err = game.AddQuest(w, game.QuestSpec{Title: "Main", Objectives: []game.ObjectiveSpec{{Kind: game.ObjDefeatEnemy, Text: "kill", Target: "Vex"}}}, true)
	must(t, err)
	must(t, game.StartGame(w, time.Unix(100, 0)))
	_, _, err = game.StartEncounter(w, []game.EnemySpec{{Name: "Bandit", Count: 2, HP: 7, AC: 12, AttackBonus: 3, Damage: "1d6", Ranged: true, Inflicts: game.CondPoisoned}})
	must(t, err)
	w.Encounter.Enemies[0].HP = 3
	w.Encounter.Enemies[0].Boss = &game.Boss{Phase: 1, Resist: 2, Attacks: 2, Phases: []game.BossPhase{{At: 100, Name: "Composed"}, {At: 50, Name: "Enraged", Attack: 1}},
		Move: game.BossMove{Name: "Bell Toll", Dice: "4d6", Save: game.DEX, DC: 13}, Charging: &game.Telegraph{Zone: "z1", Round: 1}}
	w.Encounter.Enemies[1].Conditions = game.Conditions{{Kind: game.CondProne}}
	w.Characters[0].Conditions = game.Conditions{{Kind: game.CondPoisoned, Rounds: 2, SaveDC: 11, Save: game.CON, Source: "Bandit", SourceID: w.Encounter.Enemies[0].ID}}
	w.Lore = []*game.Lore{{ID: "lore1", Title: "Vex's plan", Text: "He wants the bell.", Keywords: []string{"bell"}, AlwaysOn: true, LastTrigger: -100}}
	w.Beats = []*game.Beat{{ID: "beat1", Title: "Crows", Text: "Crows gather.", Trigger: game.TriggerEnterLocation, Target: wood.ID, Effects: []string{game.EffectAmbush}}}
	w.AddChronicle("The bell tolled at midnight.")
	smith.Keep("day 1: the party paid in full")
	w.Record(game.ChronicleEntry{Kind: game.FactPromise, Text: "Aria swore to find the smith's lost hammer", NPCIDs: []string{smith.ID}, Source: game.SourceGM})
	w.Here().LastDay = 2
	w.Defeats = []game.DefeatRecord{{Name: "Rat", LocationID: hub.ID, Turn: 0}}
	w.Campaign.Scene = game.Scene{Day: 2, Time: 3, Tension: 6, Goal: "find the trail", Turns: 4, Quiet: 1}
	w.Campaign.Summary = "So far..."
	return w
}

// canon drops null/empty values so nil and empty slices compare equal.
func canon(t *testing.T, v any) any {
	b, err := json.Marshal(v)
	must(t, err)
	var x any
	must(t, json.Unmarshal(b, &x))
	return prune(x)
}

func prune(x any) any {
	switch v := x.(type) {
	case map[string]any:
		for k, e := range v {
			e = prune(e)
			if e == nil {
				delete(v, k)
				continue
			}
			if a, ok := e.([]any); ok && len(a) == 0 {
				delete(v, k)
				continue
			}
			if m, ok := e.(map[string]any); ok && len(m) == 0 {
				delete(v, k)
				continue
			}
			v[k] = e
		}
		return v
	case []any:
		for i := range v {
			v[i] = prune(v[i])
		}
	}
	return x
}

func TestWorldRoundTrip(t *testing.T) {
	s, err := openMem(t)
	must(t, err)
	defer s.Close()
	w := richWorld(t)
	_, err = s.Save(ctx, w)
	must(t, err)
	got, err := s.Load(ctx, w.Campaign.ID)
	must(t, err)
	w.Campaign.CreatedAt = got.Campaign.CreatedAt
	want, have := canon(t, w), canon(t, got)
	if !reflect.DeepEqual(want, have) {
		a, _ := json.MarshalIndent(want, "", " ")
		b, _ := json.MarshalIndent(have, "", " ")
		t.Fatalf("round trip differs\nwant %s\n\ngot %s", a, b)
	}
	// fields hidden from JSON survive too
	if got.Players[0].Token != "secret-token" {
		t.Fatal("token lost")
	}
	if got.Campaign.Turns.Budget != w.Campaign.Turns.Budget || got.Campaign.Turns.Budget.Free != game.CombatFreeActions {
		t.Fatalf("budget %+v", got.Campaign.Turns.Budget)
	}
	// saving again replaces rather than duplicates
	got.Characters[0].Gold = 999
	got.Encounter = nil
	_, err = s.Save(ctx, got)
	must(t, err)
	again, err := s.Load(ctx, w.Campaign.ID)
	must(t, err)
	if len(again.Characters) != 1 || again.Characters[0].Gold != 999 || again.Encounter != nil || len(again.Items) != len(w.Items) {
		t.Fatalf("resave: chars %d gold %d enc %v items %d", len(again.Characters), again.Characters[0].Gold, again.Encounter, len(again.Items))
	}
}

func TestLoadedWorldStillPlays(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.db")
	s, err := Open(path)
	must(t, err)
	w := richWorld(t)
	_, err = s.Save(ctx, w)
	must(t, err)
	s.Close()
	s, err = Open(path)
	must(t, err)
	defer s.Close()
	got, err := s.Load(ctx, "camp1")
	must(t, err)
	actor := got.ActiveCharacter()
	foe := got.Encounter.Living()[0]
	foe.ZoneID = actor.ZoneID
	if !actor.Conditions.Has(game.CondPoisoned) {
		t.Fatal("condition lost on reload")
	}
	r := game.Resolve(&game.Ctx{W: got, R: dice.FromSource(dice.NewScripted(19, 19, 4)), Now: time.Unix(200, 0)}, actor,
		[]game.Action{{Kind: game.ActAttack, Target: foe.Display()}})
	if r.Steps[0].Error != "" || !foe.Defeated() {
		t.Fatalf("attack after reload: %+v", r.Steps[0])
	}
}

func TestNotFound(t *testing.T) {
	s, err := openMem(t)
	must(t, err)
	defer s.Close()
	if _, err := s.Load(ctx, "nope"); err != ErrNotFound {
		t.Fatalf("err %v", err)
	}
}

func TestEventsAppendAndReplay(t *testing.T) {
	s, err := openMem(t)
	must(t, err)
	defer s.Close()
	w := richWorld(t)
	evs, err := s.Save(ctx, w, NewEvent("campaign_created", "", map[string]string{"title": "Ashes"}), NewEvent("player_joined", "u1", nil))
	must(t, err)
	if len(evs) != 2 || evs[0].ID == 0 || evs[1].ID <= evs[0].ID {
		t.Fatalf("ids %+v", evs)
	}
	other := richWorld(t)
	other.Campaign.ID = "camp2"
	_, err = s.Save(ctx, other, NewEvent("campaign_created", "", nil))
	must(t, err)
	_, err = s.Save(ctx, w, NewEvent("narration", "", map[string]string{"text": "hi"}))
	must(t, err)
	all, err := s.Events(ctx, "camp1", 0, 0)
	must(t, err)
	if len(all) != 3 {
		t.Fatalf("camp1 events %d", len(all))
	}
	after, err := s.Events(ctx, "camp1", evs[1].ID, 0)
	must(t, err)
	if len(after) != 1 || after[0].Type != "narration" || string(after[0].Data) != `{"text":"hi"}` {
		t.Fatalf("replay after %d: %+v", evs[1].ID, after)
	}
	recent, err := s.RecentEvents(ctx, "camp1", 2, "player_joined", "narration")
	must(t, err)
	if len(recent) != 2 || recent[0].Type != "player_joined" {
		t.Fatalf("recent %+v", recent)
	}
}

func TestSaveIsAtomic(t *testing.T) {
	s, err := openMem(t)
	must(t, err)
	defer s.Close()
	w := richWorld(t)
	_, err = s.Save(ctx, w)
	must(t, err)
	// a campaign with no id fails before anything is written
	bad := richWorld(t)
	bad.Campaign.ID = ""
	if _, err := s.Save(ctx, bad, NewEvent("x", "", nil)); err == nil {
		t.Fatal("saved campaign without id")
	}
	// a cancelled context rolls the whole save back
	w.Characters[0].Gold = 12345
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.Save(cctx, w, NewEvent("y", "", nil)); err == nil {
		t.Fatal("cancelled save succeeded")
	}
	got, err := s.Load(ctx, "camp1")
	must(t, err)
	evs, _ := s.Events(ctx, "camp1", 0, 0)
	if got.Characters[0].Gold == 12345 || len(evs) != 0 {
		t.Fatal("partial save visible")
	}
}

func TestListCampaigns(t *testing.T) {
	s, err := openMem(t)
	must(t, err)
	defer s.Close()
	a := richWorld(t)
	b := richWorld(t)
	b.Campaign.ID = "camp2"
	b.Players = []*game.Player{{ID: "u9", Name: "Zed"}}
	_, err = s.Save(ctx, a)
	must(t, err)
	_, err = s.Save(ctx, b)
	must(t, err)
	all, err := s.ListCampaigns(ctx, "")
	must(t, err)
	mine, err := s.ListCampaigns(ctx, "u9")
	must(t, err)
	if len(all) != 2 || len(mine) != 1 || mine[0].ID != "camp2" || mine[0].Players != 1 {
		t.Fatalf("all %+v mine %+v", all, mine)
	}
}

// oldSchema is a v0 database: fewer columns on campaigns and characters,
// no lore/beats tables.
const oldSchema = `
CREATE TABLE campaigns (id TEXT PRIMARY KEY, title TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'lobby',
  hub_id TEXT NOT NULL DEFAULT '', location_id TEXT NOT NULL DEFAULT '', turn INTEGER NOT NULL DEFAULT 0, seq INTEGER NOT NULL DEFAULT 0);
CREATE TABLE characters (campaign_id TEXT NOT NULL, id TEXT NOT NULL, ord INTEGER NOT NULL DEFAULT 0, player_id TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL DEFAULT '', kit TEXT NOT NULL DEFAULT '', scores TEXT NOT NULL DEFAULT '{}', proficient TEXT NOT NULL DEFAULT '[]',
  level INTEGER NOT NULL DEFAULT 1, hp INTEGER NOT NULL DEFAULT 0, max_hp INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (campaign_id, id));
INSERT INTO campaigns (id, title, status, hub_id, location_id, turn, seq) VALUES ('old1', 'Old Tale', 'active', 'loc1', 'loc1', 7, 3);
INSERT INTO characters (campaign_id, id, player_id, name, kit, scores, proficient, level, hp, max_hp)
  VALUES ('old1', 'char2', 'u1', 'Vet', 'warrior', '{"STR":15,"DEX":14,"CON":13,"INT":12,"WIS":10,"CHA":8}', '["STR","CON"]', 2, 9, 17);
`

func TestMigratesOlderSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	raw, err := sql.Open("sqlite", "file:"+path)
	must(t, err)
	_, err = raw.Exec(oldSchema)
	must(t, err)
	raw.Close()

	s, err := Open(path)
	must(t, err)
	defer s.Close()
	w, err := s.Load(ctx, "old1")
	must(t, err)
	c := w.Campaign
	if c.Title != "Old Tale" || c.Turn != 7 || c.MaxSetbacks != 3 || c.MaxPlayers != 4 || c.Turns.Budget.Free != game.Unlimited || c.Turns.Budget.Action != 1 {
		t.Fatalf("migrated campaign %+v", c)
	}
	ch := w.Characters[0]
	if ch.Name != "Vet" || ch.Gold != 0 || ch.XP != 0 || ch.Scores[game.STR] != 15 || ch.HP != 9 {
		t.Fatalf("migrated character %+v", ch)
	}
	// the migrated database accepts a full save
	ch.Gold = 5
	w.Lore = []*game.Lore{{ID: "l1", Title: "t", Text: "x"}}
	_, err = s.Save(ctx, w, NewEvent("turn_started", "", nil))
	must(t, err)
	again, err := s.Load(ctx, "old1")
	must(t, err)
	if again.Characters[0].Gold != 5 || len(again.Lore) != 1 {
		t.Fatal("save after migration lost data")
	}
	// reopening is idempotent
	s2, err := Open(path)
	must(t, err)
	s2.Close()
}

func TestSchemaParser(t *testing.T) {
	cols := schemaColumns()
	if len(cols) < 15 {
		t.Fatalf("parsed %d tables", len(cols))
	}
	found := false
	for _, c := range cols["characters"] {
		if c[0] == "gold" && c[1] == "INTEGER NOT NULL DEFAULT 0" {
			found = true
		}
	}
	if !found {
		t.Fatalf("characters columns %v", cols["characters"])
	}
}

// openMem opens an empty store; TestOnD1 points it at the D1 stand-in.
var openMem = func(t *testing.T) (*Store, error) { return Open(":memory:") }

func openD1(t *testing.T) (*Store, *d1stub.Handler) {
	h, err := d1stub.Open("file::memory:", "s3cret")
	must(t, err)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	s, err := OpenD1(srv.URL, "s3cret")
	must(t, err)
	return s, h
}

func TestOnD1(t *testing.T) {
	saved := openMem
	defer func() { openMem = saved }()
	openMem = func(t *testing.T) (*Store, error) { s, _ := openD1(t); return s, nil }
	for name, f := range map[string]func(*testing.T){
		"RoundTrip": TestWorldRoundTrip, "NotFound": TestNotFound, "Events": TestEventsAppendAndReplay,
		"Atomic": TestSaveIsAtomic, "List": TestListCampaigns, "Notes": TestNotes,
	} {
		t.Run(name, f)
	}
}

func TestD1SaveIsOneBatch(t *testing.T) {
	s, h := openD1(t)
	defer s.Close()
	before := h.Calls["batch"]
	_, err := s.Save(ctx, richWorld(t), NewEvent("a", "", nil))
	must(t, err)
	if h.Calls["batch"]-before != 1 {
		t.Fatalf("save took %d batches", h.Calls["batch"]-before)
	}
}

func TestD1RefusesWrongSecret(t *testing.T) {
	h, err := d1stub.Open("file::memory:", "right")
	must(t, err)
	srv := httptest.NewServer(h)
	defer srv.Close()
	if _, err := OpenD1(srv.URL, "wrong"); err == nil {
		t.Fatal("opened with the wrong secret")
	}
}

func TestNotes(t *testing.T) {
	s, err := openMem(t)
	must(t, err)
	defer s.Close()
	w := richWorld(t)
	_, err = s.Save(ctx, w)
	must(t, err)
	id := w.Campaign.ID
	a, err := s.AddNote(ctx, id, Note{PlayerID: "u1", Text: "the ferryman lied", Turn: 3, Day: 2, Time: "night"})
	must(t, err)
	_, err = s.AddNote(ctx, id, Note{PlayerID: "u2", Text: "not yours"})
	must(t, err)
	// saving the world again must not wipe notes
	_, err = s.Save(ctx, w)
	must(t, err)
	got, err := s.Notes(ctx, id, "u1")
	must(t, err)
	if len(got) != 1 || got[0].ID != a.ID || got[0].Text != "the ferryman lied" || got[0].Day != 2 || got[0].Time != "night" {
		t.Fatalf("notes %+v", got)
	}
	if err := s.UpdateNote(ctx, id, "u2", a.ID, "stolen"); err != ErrNotFound {
		t.Fatalf("another player edited a note: %v", err)
	}
	must(t, s.UpdateNote(ctx, id, "u1", a.ID, "the ferryman lied twice"))
	got, _ = s.Notes(ctx, id, "u1")
	if got[0].Text != "the ferryman lied twice" {
		t.Fatalf("edit %+v", got)
	}
	if err := s.DeleteNote(ctx, id, "u2", a.ID); err != ErrNotFound {
		t.Fatalf("another player deleted a note: %v", err)
	}
	must(t, s.DeleteNote(ctx, id, "u1", a.ID))
	if got, _ = s.Notes(ctx, id, "u1"); len(got) != 0 {
		t.Fatalf("after delete %+v", got)
	}
}
