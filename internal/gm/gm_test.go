package gm

import (
	"context"
	"errors"
	"io"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/Solvely-Colin/aidnd/internal/dice"
	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/llm"
	"github.com/Solvely-Colin/aidnd/internal/store"
)

var bg = context.Background()

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

var specs = []game.CharacterSpec{
	{Name: "Aria", Kit: "warrior", Scores: map[string]int{"STR": 15, "DEX": 12, "CON": 14, "INT": 8, "WIS": 13, "CHA": 10}, Proficient: []string{"STR", "CON"}},
	{Name: "Bram", Kit: "ranger", Scores: map[string]int{"STR": 10, "DEX": 15, "CON": 13, "INT": 12, "WIS": 14, "CHA": 8}, Proficient: []string{"DEX", "WIS"}},
	{Name: "Cora", Kit: "devout", Scores: map[string]int{"STR": 10, "DEX": 12, "CON": 13, "INT": 8, "WIS": 15, "CHA": 14}, Proficient: []string{"WIS", "CHA"}},
	{Name: "Dax", Kit: "arcanist", Scores: map[string]int{"STR": 8, "DEX": 14, "CON": 13, "INT": 15, "WIS": 12, "CHA": 10}, Proficient: []string{"INT", "WIS"}},
}

type rig struct {
	g    *GM
	f    *llm.Fake
	now  time.Time
	id   string
	pids []string
	pubs int
}

// newRig builds a GM on an in-memory store with the scripted fake. With
// no queued replies the fake answers in text, so campaign creation uses
// the fallback world (Harrowmere) and zones use generic layouts.
func newRig(t *testing.T, players int, seed int64) *rig {
	t.Helper()
	st, err := store.Open(":memory:")
	must(t, err)
	t.Cleanup(func() { st.Close() })
	f := llm.NewFake()
	r := &rig{f: f, now: time.Unix(1_800_000_000, 0), id: "camp1"}
	r.g = New(f, st, Config{}, dice.New(seed))
	r.g.Log = log.New(io.Discard, "", 0)
	r.g.Now = func() time.Time { return r.now }
	r.g.NewID = func() string { return r.id }
	r.g.Publish = func(string, []store.Event) { r.pubs++ }
	_, err = r.g.CreateCampaign(bg, "a haunted lake town")
	must(t, err)
	for i := 0; i < players; i++ {
		pid := []string{"u1", "u2", "u3", "u4"}[i]
		r.pids = append(r.pids, pid)
		_, _, err := r.g.Join(bg, r.id, pid, "P"+pid)
		must(t, err)
		_, err = r.g.SaveCharacter(bg, r.id, pid, specs[i])
		must(t, err)
	}
	return r
}

func (r *rig) start(t *testing.T) *game.World {
	t.Helper()
	w, _, err := r.g.Start(bg, r.id)
	must(t, err)
	return w
}

func (r *rig) world(t *testing.T) *game.World {
	t.Helper()
	w, err := r.g.Load(bg, r.id)
	must(t, err)
	return w
}

func (r *rig) act(t *testing.T, pid, text string, calls ...llm.ToolCall) *TurnResult {
	t.Helper()
	if len(calls) > 0 {
		r.f.On("interpret", llm.Calls(calls...))
	}
	res, err := r.g.TakeAction(bg, r.id, pid, text, Hints{})
	must(t, err)
	return res
}

func (r *rig) events(t *testing.T, typ string) []store.Event {
	t.Helper()
	evs, err := r.g.Store.Events(bg, r.id, 0, 0)
	must(t, err)
	var out []store.Event
	for _, e := range evs {
		if typ == "" || e.Type == typ {
			out = append(out, e)
		}
	}
	return out
}

func lastUser(req llm.Request) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			return req.Messages[i].Content
		}
	}
	return ""
}

func TestCreateCampaignFallsBackWhenModelWontCallTool(t *testing.T) {
	r := newRig(t, 0, 1)
	w := r.world(t)
	if w.Campaign.Title != "The Bell Beneath Harrowmere" {
		t.Fatalf("title %q", w.Campaign.Title)
	}
	if n := len(r.f.Tagged("author_campaign")); n != 2 {
		t.Fatalf("expected one re-prompt, got %d calls", n)
	}
	if !strings.Contains(lastUser(r.f.Tagged("author_campaign")[1]), "must answer by calling a tool") {
		t.Fatal("re-prompt text missing")
	}
	checkSkeleton(t, w)
	if evs := r.events(t, "campaign_created"); len(evs) != 1 {
		t.Fatalf("campaign_created events: %d", len(evs))
	}
}

func checkSkeleton(t *testing.T, w *game.World) {
	t.Helper()
	hub := w.Location(w.Campaign.HubID)
	lair := w.Location(w.Campaign.LairID)
	if hub == nil || hub.Stub || !hub.IsHub || lair == nil || !lair.IsLair || !lair.Stub {
		t.Fatal("hub/lair wrong")
	}
	gates := 0
	for _, e := range w.Edges {
		if e.LairGate {
			gates++
			if e.State != game.EdgeLocked || !e.Touches(lair.ID) || e.Touches(hub.ID) {
				t.Fatalf("gate %+v", e)
			}
		}
	}
	if gates != 1 || !w.Connected() {
		t.Fatalf("gates %d connected %v", gates, w.Connected())
	}
	ant := w.NPC(w.Campaign.AntagonistID)
	mq := w.MainQuest()
	last := mq.Objectives[len(mq.Objectives)-1]
	if ant == nil || ant.LocationID != lair.ID || last.Kind != game.ObjDefeatEnemy || last.Target != ant.ID {
		t.Fatalf("antagonist/main quest: %+v %+v", ant, last)
	}
	confront := false
	for _, b := range w.Beats {
		if b.Target == lair.ID && b.EffectTarget == ant.ID {
			confront = true
		}
	}
	if !confront || len(w.Lore) < 2 {
		t.Fatal("finale beat or lore missing")
	}
}

func TestCreateCampaignFromAuthorWithRetry(t *testing.T) {
	st, _ := store.Open(":memory:")
	defer st.Close()
	f := llm.NewFake()
	g := New(f, st, Config{}, dice.New(1))
	g.Log = log.New(io.Discard, "", 0)
	bad := fallbackSpec("")
	bad.Places = bad.Places[:1]
	good := fallbackSpec("")
	good.Title = "Authored Title"
	good.Lore = append(good.Lore, good.Lore[0])
	f.On("author_campaign", llm.Calls(llm.Call("create_campaign", bad)), llm.Calls(llm.Call("create_campaign", good)))
	w, err := g.CreateCampaign(bg, "x")
	must(t, err)
	if w.Campaign.Title != "Authored Title" {
		t.Fatalf("title %q", w.Campaign.Title)
	}
	reqs := f.Tagged("author_campaign")
	if len(reqs) != 2 {
		t.Fatalf("calls %d", len(reqs))
	}
	var sawErr bool
	for _, m := range reqs[1].Messages {
		if m.Role == "tool" && strings.Contains(m.Content, "at least 2 places") {
			sawErr = true
		}
	}
	if !sawErr {
		t.Fatal("validation error not sent back")
	}
	checkSkeleton(t, w)
}

func TestDraftCharacterRepairsInvalidSheets(t *testing.T) {
	r := newRig(t, 0, 1)
	r.f.On("author_character", llm.Calls(llm.Call("draft_character", map[string]any{
		"name": "Ysolde", "class": "Hedge Wizard", "kit": "wizard",
		"scores":     map[string]int{"STR": 15, "DEX": 15, "CON": 15, "INT": 18, "WIS": 15, "CHA": 3},
		"proficient": []string{"INT", "INT", "LUCK"}})))
	spec := r.g.DraftCharacter(bg, r.world(t), "a wizard who talks to crows")
	if spec.Kit != "arcanist" {
		t.Fatalf("kit %q", spec.Kit)
	}
	if _, _, p := game.ValidateCharacter(spec); len(p) != 0 {
		t.Fatalf("still invalid: %v", p)
	}
	if spec.Scores["INT"] != 15 || spec.Scores["CHA"] != 8 {
		t.Fatalf("ranking not kept: %v", spec.Scores)
	}
	// total failure still yields a valid sheet
	spec = r.g.DraftCharacter(bg, r.world(t), "a sneaky thief")
	if _, _, p := game.ValidateCharacter(spec); len(p) != 0 || spec.Kit != "skirmisher" {
		t.Fatalf("fallback draft %+v %v", spec, p)
	}
}

func TestJoinIdempotentFullAndStarted(t *testing.T) {
	r := newRig(t, 4, 1)
	p, created, err := r.g.Join(bg, r.id, "u1", "again")
	must(t, err)
	if created || p.Name != "Pu1" {
		t.Fatal("join not idempotent")
	}
	if _, _, err := r.g.Join(bg, r.id, "u5", "late"); !errors.Is(err, ErrConflict) {
		t.Fatalf("5th player: %v", err)
	}
	if len(r.world(t).Players) != 4 {
		t.Fatal("players")
	}
}

func TestStartNeedsEveryCharacter(t *testing.T) {
	r := newRig(t, 2, 1)
	_, _, err := r.g.Join(bg, r.id, "u3", "Slow")
	must(t, err)
	if _, _, err := r.g.Start(bg, r.id); !errors.Is(err, ErrConflict) || !strings.Contains(err.Error(), "Slow") {
		t.Fatalf("start: %v", err)
	}
	_, err = r.g.SaveCharacter(bg, r.id, "u3", specs[2])
	must(t, err)
	w := r.start(t)
	hub := w.Here()
	if len(hub.Zones) < 2 || w.Campaign.Status != game.StatusActive || w.ActiveCharacter().PlayerID != "u1" {
		t.Fatalf("start state: zones %d status %s", len(hub.Zones), w.Campaign.Status)
	}
	for _, c := range w.Characters {
		if c.ZoneID != hub.Zones[0].ID {
			t.Fatal("party not placed")
		}
	}
	if len(r.events(t, "turn_started")) != 1 || len(r.events(t, "narration")) != 1 {
		t.Fatal("start events")
	}
	if _, err := r.g.SaveCharacter(bg, r.id, "u1", specs[0]); !errors.Is(err, ErrConflict) {
		t.Fatal("character changed after start")
	}
}

func TestTalkKeepsTurnWithFullNarration(t *testing.T) {
	r := newRig(t, 2, 1)
	r.start(t)
	res := r.act(t, "u1", "I ask the elder about the bell", llm.Call("talk_to", map[string]any{"npc": "Elder Corbin", "text": "the bell?"}))
	if res.Quick || res.TurnEnded || res.Budget == nil || res.Budget.Action != 1 {
		t.Fatalf("quick %v ended %v budget %+v", res.Quick, res.TurnEnded, res.Budget)
	}
	if len(r.f.Tagged("narrate")) != 1 || len(r.f.Tagged("narrate_quick")) != 0 {
		t.Fatal("a conversation got the quick narrator")
	}
	if r.world(t).ActiveCharacter().PlayerID != "u1" {
		t.Fatal("talking outside combat passed the turn")
	}
}

func TestQuickFreeActionKeepsTurn(t *testing.T) {
	r := newRig(t, 2, 1)
	r.start(t)
	r.f.On("narrate_quick", llm.Text("Rain slicks the cobbles. Tamsin's forge glows at the square's edge."))
	res := r.act(t, "u1", "what do I see?", llm.Call("look", map[string]any{}))
	if !res.Quick || res.TurnEnded || res.Budget == nil || res.Budget.Action != 1 {
		t.Fatalf("quick %v ended %v budget %+v", res.Quick, res.TurnEnded, res.Budget)
	}
	if len(r.f.Tagged("narrate")) != 0 || len(r.f.Tagged("narrate_quick")) != 1 {
		t.Fatal("quick answer used the full narrator")
	}
	if !strings.Contains(lastUser(r.f.Tagged("narrate_quick")[0]), "Harrowmere") {
		t.Fatal("quick narrator missing facts")
	}
	if r.world(t).ActiveCharacter().PlayerID != "u1" {
		t.Fatal("turn passed")
	}
	if evs := r.events(t, "turn_budget"); len(evs) != 1 {
		t.Fatal("no budget event")
	}
	// long quick answers are tightened
	r.f.On("narrate_quick", llm.Text(strings.Repeat("word ", 90)))
	r.f.On("tighten", llm.Text("Short."))
	res = r.act(t, "u1", "and now?", llm.Call("look", map[string]any{}))
	if res.Narration != "Short." {
		t.Fatalf("not tightened: %q", res.Narration)
	}
}

func TestNotYourTurn(t *testing.T) {
	r := newRig(t, 2, 1)
	r.start(t)
	if _, err := r.g.TakeAction(bg, r.id, "u2", "hi", Hints{}); !errors.Is(err, ErrNotYourTurn) {
		t.Fatalf("err %v", err)
	}
}

func TestMoveAuthorsStubAndEndTurnPasses(t *testing.T) {
	r := newRig(t, 2, 1)
	r.start(t)
	r.f.On("author_location", llm.Calls(llm.Call("fill_location", map[string]any{
		"description": "Reeds whisper over black water.",
		"npcs":        []map[string]any{{"name": "Old Wenna", "role": "eel-catcher"}, {"name": "Tamsin Hale", "role": "dup"}},
		"exits":       []map[string]any{{"to": "Heron Isle", "label": "a rotten jetty", "hint": "birds"}, {"to": "Harrowmere", "label": "dup"}},
		"lore":        []map[string]any{{"title": "Reed secret", "text": "The reeds hide a smugglers' cache."}},
	})))
	r.f.On("author_zones", llm.Calls(llm.Call("define_zones", map[string]any{"zones": []map[string]any{
		{"name": "Boardwalk", "adjacent": []string{"Reed Beds"}, "exits": []string{"Harrowmere"}},
		{"name": "Reed Beds", "cover": true, "adjacent": []string{"Boardwalk", "Jetty"}},
		{"name": "Jetty", "adjacent": []string{"Reed Beds"}, "exits": []string{"Heron Isle"}},
	}})))
	r.f.Default("narrate", llm.Text("You wade into the Reedmarsh."))
	res := r.act(t, "u1", "we head to the marsh", llm.Call("move", map[string]any{"exit": "Reedmarsh"}))
	w := r.world(t)
	marsh := w.Here()
	if marsh.Name != "Reedmarsh" || marsh.Stub || len(marsh.Zones) != 3 {
		t.Fatalf("arrival: %s stub=%v zones=%d", marsh.Name, marsh.Stub, len(marsh.Zones))
	}
	if w.Characters[0].ZoneID != marsh.Zones[0].ID {
		t.Fatal("party not in the exit's zone")
	}
	count := 0
	for _, n := range w.NPCs {
		if n.Name == "Tamsin Hale" {
			count++
		}
	}
	if count != 1 || w.NPC("Old Wenna") == nil || w.Location("Heron Isle") == nil {
		t.Fatal("fill not applied/deduped")
	}
	if res.TurnEnded || res.Budget.Move != 0 {
		t.Fatalf("move should keep the action: %+v", res.Budget)
	}
	if len(r.events(t, "location_changed")) != 2 { // start + arrival
		t.Fatal("location_changed missing")
	}
	// side quest objective "Scout the Reedmarsh" is not active (not accepted), so nothing ticks
	res = r.act(t, "u1", "I'm done", llm.Call("end_turn", map[string]any{}))
	if !res.TurnEnded || res.NextActor == nil || res.NextActor.PlayerID != "u2" {
		t.Fatalf("next actor %+v", res.NextActor)
	}
}

func TestInterpreterRetriesBadIDThenFallsBack(t *testing.T) {
	r := newRig(t, 1, 1)
	r.start(t)
	r.f.On("interpret",
		llm.Calls(llm.Call("move", map[string]any{"exit": "exit999"})),
		llm.Calls(llm.Call("look", map[string]any{"target": "Tamsin Hale"})))
	r.f.On("narrate_quick", llm.Text("Tamsin wipes her hands."))
	res, err := r.g.TakeAction(bg, r.id, "u1", "I go to the moon gate", Hints{})
	must(t, err)
	reqs := r.f.Tagged("interpret")
	if len(reqs) != 2 {
		t.Fatalf("interpret calls %d", len(reqs))
	}
	var fed bool
	for _, m := range reqs[1].Messages {
		if m.Role == "tool" && strings.Contains(m.Content, `no exit "exit999"`) {
			fed = true
		}
	}
	if !fed || res.Resolution.Steps[0].Action.Kind != game.ActLook {
		t.Fatalf("retry not used: %+v", res.Resolution.Steps)
	}
	// text twice -> safe look
	r.f.On("interpret", llm.Text("You want to dance."), llm.Text("Still dancing."))
	res, err = r.g.TakeAction(bg, r.id, "u1", "I dance", Hints{})
	must(t, err)
	if len(res.Resolution.Steps) != 1 || res.Resolution.Steps[0].Action.Kind != game.ActLook {
		t.Fatalf("fallback %+v", res.Resolution.Steps)
	}
}

func TestSnapshotShowsBudgetAndOnlyValidIDs(t *testing.T) {
	r := newRig(t, 2, 1)
	r.start(t)
	r.act(t, "u1", "look", llm.Call("look", map[string]any{}))
	req := r.f.Tagged("interpret")[0]
	u := lastUser(req)
	for _, want := range []string{"LEFT THIS TURN: action 1, move 1, free unlimited", "EXITS:", "ZONES:", "Tamsin Hale [npc", "PLAYER (Aria) SAYS: look"} {
		if !strings.Contains(u, want) {
			t.Errorf("interpreter prompt missing %q", want)
		}
	}
	w := r.world(t)
	for _, l := range w.Lore {
		if strings.Contains(u, l.Text) {
			t.Fatal("lore leaked to interpreter")
		}
	}
	if strings.Contains(u, "SECRETS") {
		t.Fatal("secrets in interpreter prompt")
	}
	if req.Temperature > 0.3 {
		t.Fatal("interpreter temperature too high")
	}
}

// fightRig starts a 2-player game with the party in Reedmarsh and an
// encounter whose enemy is in the party's zone.
func fightRig(t *testing.T, seed int64, enemy game.EnemySpec) *rig {
	r := newRig(t, 2, seed)
	r.start(t)
	r.f.Default("narrate", llm.Text("Steel rings in the reeds."))
	r.act(t, "u1", "to the marsh", llm.Call("move", map[string]any{"exit": "Reedmarsh"}))
	w := r.world(t)
	enc, _, err := game.StartEncounter(w, []game.EnemySpec{enemy})
	must(t, err)
	enc.Enemies[0].ZoneID = w.Characters[0].ZoneID
	w.Campaign.Turns.Budget = game.FullBudget(true)
	_, err = r.g.Store.Save(bg, w)
	must(t, err)
	return r
}

func TestMultiStepWithSkippedStepsAndEnemyTurns(t *testing.T) {
	r := fightRig(t, 3, game.EnemySpec{Name: "Marsh Ghoul", HP: 30, AC: 5, AttackBonus: 2, Damage: "1d4"})
	r.f.On("narrate_enemies", llm.Text("The ghoul claws back."))
	res := r.act(t, "u1", "I look around, draw my sword and attack the ghoul twice",
		llm.Call("look", map[string]any{}),
		llm.Call("equip", map[string]any{"item": "Longsword"}),
		llm.Call("attack", map[string]any{"target": "Marsh Ghoul"}),
		llm.Call("attack", map[string]any{"target": "Marsh Ghoul"}))
	if !res.TurnEnded || res.Quick {
		t.Fatal("attack should end the turn")
	}
	if len(res.Skipped) != 1 || !strings.Contains(res.Skipped[0].Error, "no action left") {
		t.Fatalf("skipped %+v", res.Skipped)
	}
	if len(res.EnemyTurns) != 1 || res.EnemyNarration != "The ghoul claws back." {
		t.Fatalf("enemy turns %+v", res.EnemyTurns)
	}
	// the enemy narrator continues the scene the narrator just set
	if u := lastUser(r.f.Tagged("narrate_enemies")[0]); !strings.Contains(u, "Last narration: Steel rings in the reeds.") {
		t.Fatalf("enemy prompt %q", u)
	}
	if res.NextActor.PlayerID != "u2" {
		t.Fatal("next actor")
	}
	if len(r.events(t, "enemy_turns")) != 1 {
		t.Fatal("enemy_turns event")
	}
	// the narrator saw the skipped step and engine rolls
	u := lastUser(r.f.Tagged("narrate")[len(r.f.Tagged("narrate"))-1])
	if !strings.Contains(u, "SKIPPED, did not happen, do not narrate it (attack") || !strings.Contains(u, "ROLL Aria attacks") {
		t.Fatalf("narrator input:\n%s", u)
	}
	// directives offered mid-combat exclude spawn_encounter
	for _, tl := range r.f.Tagged("narrate")[len(r.f.Tagged("narrate"))-1].Tools {
		if tl.Name == DirSpawnEncounter {
			t.Fatal("spawn offered mid-combat")
		}
	}
}

func TestPartyWipeIsASetback(t *testing.T) {
	r := fightRig(t, 3, game.EnemySpec{Name: "Bog Horror", HP: 30, AC: 10, AttackBonus: 5, Damage: "1d4"})
	w := r.world(t)
	for _, c := range w.Characters {
		c.HP = 1
		c.Gold = 40
	}
	w.Characters[1].HP = 0
	_, err := r.g.Store.Save(bg, w)
	must(t, err)
	r.g.Dice = dice.FromSource(dice.NewScripted(20))
	res := r.act(t, "u1", "I wait", llm.Call("end_turn", map[string]any{}))
	if len(res.Setback) == 0 {
		t.Fatalf("no setback: %+v", res.EnemyTurns)
	}
	w = r.world(t)
	if w.Campaign.LocationID != w.Campaign.HubID || w.Campaign.Setbacks != 1 || w.Characters[0].Gold != 30 || w.InCombat() {
		t.Fatalf("after wipe: loc %s setbacks %d gold %d", w.Campaign.LocationID, w.Campaign.Setbacks, w.Characters[0].Gold)
	}
}

func TestPacingCapForcesProgressDirective(t *testing.T) {
	r := newRig(t, 1, 1)
	r.start(t)
	w := r.world(t)
	w.Campaign.Scene.Quiet = r.g.Cfg.SceneTurnCap - 1
	exitsBefore := len(w.Exits(w.Here().ID, false))
	_, err := r.g.Store.Save(bg, w)
	must(t, err)
	// narrator ignores the requirement twice: code adds a path itself
	r.f.On("narrate", llm.Text("The rain keeps falling."), llm.Text("Rain, and a path you hadn't noticed."))
	res := r.act(t, "u1", "I check my boots", llm.Call("ability_check", map[string]any{"ability": "WIS", "dc": 5, "purpose": "other", "text": "check boots"}))
	req := r.f.Tagged("narrate")[0]
	if !req.RequireTool {
		t.Fatal("progress tool not required")
	}
	for _, tl := range req.Tools {
		if !progressDirectives[tl.Name] {
			t.Fatalf("non-progress tool offered: %s", tl.Name)
		}
	}
	if !strings.Contains(lastUser(req), "You MUST call one progress tool") {
		t.Fatal("director note missing force line")
	}
	if len(res.WorldChanges) == 0 || !strings.Contains(strings.Join(res.WorldChanges, "|"), "new path") {
		t.Fatalf("backstop changes %v", res.WorldChanges)
	}
	w = r.world(t)
	if len(w.Exits(w.Here().ID, false)) != exitsBefore+1 || w.Campaign.Scene.Quiet != 0 {
		t.Fatalf("exits %d quiet %d", len(w.Exits(w.Here().ID, false)), w.Campaign.Scene.Quiet)
	}
	if res.Narration != "Rain, and a path you hadn't noticed." {
		t.Fatalf("narration %q", res.Narration)
	}
}

func TestPacingCapNarratorRevealsHiddenExit(t *testing.T) {
	r := newRig(t, 1, 1)
	r.start(t)
	w := r.world(t)
	hub := w.Here()
	hidden := w.AddStub("Smugglers' Tunnel", "under the inn", "cave", 1)
	e := w.Connect(hub.ID, hidden.ID, game.EdgeHidden, "a trapdoor", "")
	w.Campaign.Scene.Quiet = 50
	for _, b := range w.Beats {
		b.Fired = true // keep the stalled beat from revealing it first
	}
	_, err := r.g.Store.Save(bg, w)
	must(t, err)
	r.f.On("narrate", func(req llm.Request) (llm.Response, error) {
		if !strings.Contains(lastUser(req), e.ID) {
			t.Error("hidden exit id not in narrator secrets")
		}
		return llm.Response{Content: "A draft rises from a trapdoor.", ToolCalls: []llm.ToolCall{llm.Call(DirRevealExit, map[string]string{"exit_id": e.ID})}}, nil
	})
	res := r.act(t, "u1", "I wait by the fire", llm.Call("ability_check", map[string]any{"ability": "WIS", "dc": 10, "purpose": "other", "text": "listen"}))
	if r.world(t).Edge(e.ID).State != game.EdgeKnown || len(res.WorldChanges) != 1 {
		t.Fatalf("not revealed: %v", res.WorldChanges)
	}
}

func TestDirectorNoteIsLastAndLoreFirst(t *testing.T) {
	r := newRig(t, 1, 1)
	r.start(t)
	r.act(t, "u1", "I ask the elder about the bell", llm.Call("talk_to", map[string]any{"npc": "Elder Corbin", "text": "the bell?"}))
	u := lastUser(r.f.Tagged("narrate")[0])
	iLore, iState, iWhat, iNote := strings.Index(u, "CANON LORE"), strings.Index(u, "STATE:"), strings.Index(u, "WHAT HAPPENED"), strings.Index(u, "DIRECTOR'S NOTE")
	if !(iLore >= 0 && iLore < iState && iState < iWhat && iWhat < iNote) {
		t.Fatalf("order lore=%d state=%d what=%d note=%d", iLore, iState, iWhat, iNote)
	}
	// keyword "corbin" + entity (Corbin is present) pull in Corbin's secret
	if !strings.Contains(u, "sold Morwen the map") {
		t.Fatal("relevant lore not injected")
	}
	// the talk completed objective 1 this turn, so the note moves on
	if !strings.Contains(u, "Current main objective: Search the Drowned Chapel") {
		t.Fatalf("main objective missing:\n%s", u[iNote:])
	}
	// talking to Corbin completes main objective 1 automatically
	w := r.world(t)
	if !w.MainQuest().Objectives[0].Done {
		t.Fatal("talk objective not ticked")
	}
}

func TestSelectLore(t *testing.T) {
	w := &game.World{Campaign: game.Campaign{LocationID: "loc1", Turn: 10}}
	w.Locations = []*game.Location{{ID: "loc1", Name: "Town"}}
	w.NPCs = []*game.NPC{{ID: "npc1", Name: "Ada", LocationID: "loc1", Alive: true}}
	w.Lore = []*game.Lore{
		{ID: "a", Title: "core", Text: "always", AlwaysOn: true, LastTrigger: -100},
		{ID: "b", Title: "ada", Text: "about Ada", EntityIDs: []string{"npc1"}, LastTrigger: -100},
		{ID: "c", Title: "kw", Text: "about the bell", Keywords: []string{"bell"}, LastTrigger: -100},
		{ID: "d", Title: "recent", Text: "triggered 2 turns ago", LastTrigger: 8},
		{ID: "e", Title: "stale", Text: "triggered long ago", LastTrigger: 3},
		{ID: "f", Title: "elsewhere", Text: "far away", EntityIDs: []string{"npc9"}, LastTrigger: -100},
	}
	got := selectLore(w, "I ring the BELL", 10000)
	ids := ""
	for _, l := range got {
		ids += l.ID
	}
	if ids != "abcd" {
		t.Fatalf("selected %q", ids)
	}
	if w.Lore[2].LastTrigger != 10 {
		t.Fatal("trigger turn not recorded")
	}
	// stays active for 2 turns, then drops
	w.Campaign.Turn = 12
	if got := selectLore(w, "", 10000); len(got) != 3 { // core, Ada, bell (turn 10); "recent" (turn 8) expired
		t.Fatalf("active window: %d", len(got))
	}
	w.Campaign.Turn = 13
	if got := selectLore(w, "", 10000); len(got) != 2 { // core + Ada (entity)
		t.Fatalf("after window: %d", len(got))
	}
	// budget
	if got := selectLore(w, "", 15); len(got) != 1 {
		t.Fatalf("budget: %d", len(got))
	}
}

func TestBeatFiresOnEnterWithAmbush(t *testing.T) {
	r := newRig(t, 1, 1)
	r.start(t)
	w := r.world(t)
	marsh, chapel := w.Location("Reedmarsh"), w.Location("Drowned Chapel")
	w.Campaign.LocationID = marsh.ID
	marsh.Visited = true
	game.EnsureZones(w, marsh.ID)
	_, err := game.FillLocation(w, marsh.ID, game.LocationFill{Description: "reeds"})
	must(t, err)
	for _, c := range w.Characters {
		c.ZoneID = marsh.Zones[0].ID
	}
	_, err = r.g.Store.Save(bg, w)
	must(t, err)
	r.f.Default("narrate", llm.Text("Pale hands rise."))
	res := r.act(t, "u1", "into the chapel", llm.Call("move", map[string]any{"exit": "Drowned Chapel"}))
	w = r.world(t)
	if w.Campaign.LocationID != chapel.ID || !w.InCombat() {
		t.Fatalf("beat ambush did not start a fight: %v", res.WorldChanges)
	}
	fired := 0
	for _, b := range w.Beats {
		if b.Fired {
			fired++
		}
	}
	if fired != 1 {
		t.Fatalf("fired %d beats", fired)
	}
	if !strings.Contains(lastUser(r.f.Tagged("narrate")[0]), "PLAY THIS BEAT NOW: Something in the water") {
		t.Fatal("beat not in director note")
	}
}

func TestObjectiveJudge(t *testing.T) {
	r := newRig(t, 1, 1)
	r.start(t)
	w := r.world(t)
	q, err := game.AddQuest(w, game.QuestSpec{Title: "Word to the Elder", GiverID: "Elder Corbin",
		Objectives: []game.ObjectiveSpec{{Kind: game.ObjFreeform, Text: "Warn the town about the bell"}}}, false)
	must(t, err)
	_, err = game.AcceptQuest(w, q.ID)
	must(t, err)
	_, err = r.g.Store.Save(bg, w)
	must(t, err)
	// fabricated evidence is rejected
	r.f.On("judge", llm.Calls(llm.Call("complete_objective", map[string]string{"quest_id": q.ID, "objective_id": q.Objectives[0].ID, "evidence": "Aria rallied the whole town"})))
	r.act(t, "u1", "I shout the news", llm.Call("say", map[string]any{"text": "The bell wakes the dead!"}), llm.Call("end_turn", map[string]any{}))
	if r.world(t).Quest(q.ID).Objectives[0].Done {
		t.Fatal("judge accepted fabricated evidence")
	}
	// quoted evidence from the log is accepted
	r.f.Default("narrate", llm.Text("The square falls silent as Aria warns everyone that the bell wakes the dead."))
	r.f.On("judge", llm.Calls(llm.Call("complete_objective", map[string]string{"quest_id": q.ID, "objective_id": q.Objectives[0].ID, "evidence": "Aria warns everyone that the bell wakes the dead"})))
	res := r.act(t, "u1", "I climb the well and warn everyone", llm.Call("ability_check", map[string]any{"ability": "CHA", "dc": 10, "purpose": "other", "text": "rouse the town"}))
	if !r.world(t).Quest(q.ID).Objectives[0].Done {
		t.Fatalf("judge did not tick: %v", res.WorldChanges)
	}
	if req := r.f.Tagged("judge")[1]; req.Temperature > 0.2 {
		t.Fatal("judge temperature")
	}
	if len(r.events(t, "quest_updated")) == 0 {
		t.Fatal("quest_updated event")
	}
}

func TestDirectivesAreValidatedAndClamped(t *testing.T) {
	r := newRig(t, 1, 1)
	r.start(t)
	w := r.world(t)
	marsh := w.Location("Reedmarsh")
	w.Campaign.LocationID = marsh.ID
	_, _ = game.FillLocation(w, marsh.ID, game.LocationFill{Description: "reeds"})
	game.EnsureZones(w, marsh.ID)
	for _, c := range w.Characters {
		c.ZoneID = marsh.Zones[0].ID
	}
	ch := w.Characters[0]
	hp, gold := ch.HP, ch.Gold
	g := r.g
	if _, err := g.applyDirective(bg, w, llm.Call("set_hp", map[string]any{"hp": 0})); err == nil {
		t.Fatal("unknown directive accepted")
	}
	if _, err := g.applyDirective(bg, w, llm.Call(DirSpawnEncounter, map[string]any{"enemies": []map[string]any{{"name": "Dragon", "count": 9, "hp": 500, "ac": 30, "attack_bonus": 20, "damage": "20d12"}}})); err != nil {
		t.Fatal(err)
	}
	caps := game.CapsFor(marsh.Danger, 1)
	for _, e := range w.Encounter.Enemies {
		if e.MaxHP > caps.MaxHP || e.AC > caps.MaxAC || e.AttackBonus > caps.MaxAttack {
			t.Fatalf("not clamped: %+v", e.StatBlock)
		}
	}
	if len(w.Encounter.Enemies) > 4 {
		t.Fatal("count not clamped")
	}
	if _, err := g.applyDirective(bg, w, llm.Call(DirSpawnEncounter, map[string]any{"enemies": []map[string]any{{"name": "x", "hp": 1, "ac": 1, "attack_bonus": 1, "damage": "1d4"}}})); err == nil {
		t.Fatal("second fight spawned mid-combat")
	}
	if _, err := g.applyDirective(bg, w, llm.Call(DirEndEncounter, map[string]any{"reason": "they flee"})); err != nil || w.InCombat() {
		t.Fatal("end_encounter")
	}
	game.AdjustTension(w, -10)
	_, _ = g.applyDirective(bg, w, llm.Call(DirAdjustTension, map[string]any{"delta": 9}))
	if w.Campaign.Scene.Tension != 2 {
		t.Fatalf("tension %d", w.Campaign.Scene.Tension)
	}
	if _, err := g.applyDirective(bg, w, llm.Call(DirPlaceItem, map[string]any{"key": "greater_healing"})); err == nil {
		t.Fatal("valuable item placed")
	}
	if _, err := g.applyDirective(bg, w, llm.Call(DirAddExit, map[string]any{"name": w.Location(w.Campaign.LairID).Name, "label": "shortcut"})); err == nil {
		t.Fatal("shortcut to the lair allowed")
	}
	if _, err := g.applyDirective(bg, w, llm.Call(DirOfferQuest, map[string]any{"title": "x", "giver_id": "Tamsin Hale", "objectives": []map[string]any{{"kind": "freeform", "text": "y"}}})); err == nil {
		t.Fatal("quest from absent giver")
	}
	if _, err := g.applyDirective(bg, w, llm.Call(DirRecordLore, map[string]any{"text": "The reeds sing at dusk."})); err != nil || len(w.Chronicle) != 1 {
		t.Fatal("record_lore")
	}
	if ch.HP != hp || ch.Gold != gold {
		t.Fatal("directive changed HP or gold")
	}
	// reward items can't be gifted
	w.Campaign.LocationID = w.Campaign.HubID
	tamsin := w.NPC("Tamsin Hale")
	var reward string
	for _, q := range w.Quests {
		if q.GiverID == tamsin.ID && len(q.RewardItems) > 0 {
			reward = q.RewardItems[0]
		}
	}
	if _, err := g.applyDirective(bg, w, llm.Call(DirNPCGivesItem, map[string]any{"npc_id": tamsin.ID, "item_id": reward, "to": ch.ID})); err == nil {
		t.Fatal("reward item gifted early")
	}
}

func TestMapHintsAreHintsWordsWin(t *testing.T) {
	r := newRig(t, 2, 1)
	w := r.start(t)
	tamsin := w.NPC("Tamsin Hale")
	a := applyHints(w, game.Action{Kind: game.ActAttack, Target: tamsin.ID}, Hints{TargetID: tamsin.ID}, "hello there")
	if a.Kind != game.ActTalkTo {
		t.Fatalf("selected NPC attacked without violent words: %+v", a)
	}
	a = applyHints(w, game.Action{Kind: game.ActAttack, Target: tamsin.ID}, Hints{TargetID: tamsin.ID}, "I attack her!")
	if a.Kind != game.ActAttack {
		t.Fatal("explicit attack overridden")
	}
	a = applyHints(w, game.Action{Kind: game.ActTalkTo}, Hints{TargetID: tamsin.ID}, "what's for sale?")
	if a.Target != tamsin.ID {
		t.Fatal("NPC hint not used for talk")
	}
	bram := w.Character("Bram")
	a = applyHints(w, game.Action{Kind: game.ActUseItem, Item: "potion"}, Hints{TargetID: bram.ID}, "heal him")
	if a.Target != bram.ID {
		t.Fatal("party hint not used for heal")
	}
	a = applyHints(w, game.Action{Kind: game.ActMove}, Hints{ZoneID: w.Here().Zones[1].ID}, "go there")
	if a.Kind != game.ActReposition {
		t.Fatal("zone hint not turned into reposition")
	}
}

func TestSkipRules(t *testing.T) {
	r := newRig(t, 2, 1)
	r.start(t)
	if _, err := r.g.Skip(bg, r.id, "u2", SkipAsPlayer); !errors.Is(err, ErrNotYourTurn) {
		t.Fatalf("early skip of another player: %v", err)
	}
	r.now = r.now.Add(time.Duration(r.g.Cfg.SkipAfterSeconds+1) * time.Second)
	res, err := r.g.Skip(bg, r.id, "u2", SkipAsPlayer)
	must(t, err)
	if res.NextActor.PlayerID != "u2" {
		t.Fatal("idle skip")
	}
	res, err = r.g.Skip(bg, r.id, "anyone", SkipAsService)
	must(t, err)
	if res.NextActor.PlayerID != "u1" {
		t.Fatal("service skip")
	}
	if r.pubs == 0 {
		t.Fatal("nothing published")
	}
}

func TestQuestLoopThroughPipeline(t *testing.T) {
	r := newRig(t, 1, 5)
	r.start(t)
	r.f.Default("narrate", llm.Text("Done."))
	w := r.world(t)
	q := w.Quest("Lights in the Reeds")
	r.act(t, "u1", "I'll help, Tamsin", llm.Call("accept_quest", map[string]any{"quest": q.ID}))
	if r.world(t).Quest(q.ID).Status != game.QuestActive {
		t.Fatal("accept")
	}
	r.act(t, "u1", "to the marsh", llm.Call("move", map[string]any{"exit": "Reedmarsh"}))
	if !r.world(t).Quest(q.ID).Objectives[0].Done {
		t.Fatal("reach objective")
	}
	// fight a wolf there
	w = r.world(t)
	enc := w.Encounter // the quest's wolf was waiting there
	if enc == nil {
		t.Fatal("quest foe should be waiting in the marsh")
	}
	enc.Enemies[0].HP, enc.Enemies[0].AC = 1, 1
	enc.Enemies[0].ZoneID = w.Characters[0].ZoneID
	_, _ = r.g.Store.Save(bg, w)
	r.g.Dice = dice.FromSource(dice.NewScripted(15, 5))
	r.act(t, "u1", "I cut it down", llm.Call("attack", map[string]any{"target": "Wolf"}))
	w = r.world(t)
	if w.Quest(q.ID).Status != game.QuestCompleted {
		t.Fatalf("quest %s", w.Quest(q.ID).Status)
	}
	r.act(t, "u1", "back to town", llm.Call("travel", map[string]any{"destination": "Harrowmere"}))
	gold := r.world(t).Characters[0].Gold
	r.act(t, "u1", "Here's your proof, Tamsin", llm.Call("turn_in_quest", map[string]any{"quest": q.ID}))
	w = r.world(t)
	if w.Quest(q.ID).Status != game.QuestTurnedIn || w.Characters[0].Gold != gold+30 {
		t.Fatalf("turn in: %s gold %d", w.Quest(q.ID).Status, w.Characters[0].Gold)
	}
	if len(w.NPC("Tamsin Hale").Recall()) == 0 {
		t.Fatal("Tamsin doesn't remember")
	}
	// the promise made on accepting survives a reload and is now kept
	kept := false
	for _, c := range w.Chronicle {
		kept = kept || (c.QuestID == q.ID && c.Status == game.PromiseKept && len(c.NPCIDs) == 1)
	}
	if !kept {
		t.Fatalf("quest promise not kept in the chronicle: %+v", w.Chronicle)
	}
}

func TestInterpretAsksForMoreStepsOneAtATime(t *testing.T) {
	r := newRig(t, 1, 1)
	w := r.start(t)
	tamsin := w.NPC("Tamsin Hale")
	zone := w.Here().Zones[1].ID
	r.f.On("interpret", llm.Calls(llm.Call("reposition", map[string]any{"zone": zone})))
	r.f.On("interpret_more", llm.Calls(llm.Call("talk_to", map[string]any{"npc": tamsin.ID})), llm.Calls(llm.Call("talk_to", map[string]any{"npc": tamsin.ID})))
	res, err := r.g.TakeAction(bg, r.id, "u1", "I walk over to Tamsin and ask about the marsh", Hints{})
	must(t, err)
	if len(res.Resolution.Steps) != 2 || res.Resolution.Steps[1].Action.Kind != game.ActTalkTo {
		t.Fatalf("steps %+v", res.Resolution.Steps)
	}
}

func TestAmbushBeatWaitsOutsideTheSafeHub(t *testing.T) {
	r := newRig(t, 1, 1)
	w := r.start(t)
	if !safeHere(w) {
		t.Fatal("rig should start in the safe hub")
	}
	w.Beats = []*game.Beat{{ID: "b1", Title: "Ambush", Text: "x", Trigger: game.TriggerTurn, Value: 0, Effects: []string{game.EffectAmbush}}}
	if b := pickBeat(w, &game.Resolution{}); b != nil {
		t.Fatal("ambush beat fired in the hub")
	}
	w.Here().IsHub = false
	if b := pickBeat(w, &game.Resolution{}); b == nil {
		t.Fatal("ambush beat should fire away from the hub")
	}
}

// putApart moves the actor and an NPC into different zones of the hub.
func putApart(t *testing.T, w *game.World, n *game.NPC) (actor *game.Character, near, far string) {
	t.Helper()
	l := w.Here()
	if len(l.Zones) < 2 {
		t.Fatal("hub needs zones")
	}
	actor = w.ActiveCharacter()
	near, far = l.Zones[0].ID, l.Zones[len(l.Zones)-1].ID
	actor.ZoneID, n.ZoneID = near, far
	return actor, near, far
}

func TestBeatWaitsUntilActorIsWithItsNPC(t *testing.T) {
	r := newRig(t, 1, 1)
	w := r.start(t)
	corbin := w.NPC("Elder Corbin")
	actor, _, far := putApart(t, w, corbin)
	w.Here().IsHub = false
	w.Beats = []*game.Beat{{ID: "b1", Title: "The sheet", Text: "At the well, Corbin pulls back the sheet over the drowned man.", Trigger: game.TriggerTurn, Value: 0}}
	res := &game.Resolution{ActorID: actor.ID}
	if b := pickBeat(w, res); b != nil {
		t.Fatal("beat with Corbin fired while the actor was in another zone")
	}
	actor.ZoneID = far
	if b := pickBeat(w, res); b == nil {
		t.Fatal("beat should fire once the actor is with Corbin")
	}
	// an NPC the beat brings in does not need to be here yet
	corbin.LocationID = "elsewhere"
	w.Beats[0].Effects, w.Beats[0].EffectTarget = []string{game.EffectBringNPC}, corbin.ID
	if b := pickBeat(w, res); b == nil {
		t.Fatal("bring_npc beat should fire")
	}
}

func TestBeatNames(t *testing.T) {
	cases := []struct {
		text, name string
		want       bool
	}{
		{"Doc Halloway pulls back the sheet.", "Doc Halloway", true},
		{"The bell rings and Sister Ione is found unconscious.", "Sister Ione Marsh", true},
		{"Pale hands break the surface of the shallows.", "Morwen the Pale", false},
		{"The fog rolls in over the marsh.", "Sister Ione Marsh", false},
		{"The doc says nothing.", "Doc Halloway", false},
	}
	for _, c := range cases {
		if got := beatNames(c.text, c.name); got != c.want {
			t.Errorf("beatNames(%q, %q) = %v", c.text, c.name, got)
		}
	}
}

func TestSnapshotKeepsTheSceneInTheActorsZone(t *testing.T) {
	r := newRig(t, 1, 1)
	w := r.start(t)
	corbin, tamsin := w.NPC("Elder Corbin"), w.NPC("Tamsin Hale")
	actor, near, _ := putApart(t, w, corbin)
	tamsin.ZoneID = near
	s := snapshot(w, actor, true)
	if !strings.Contains(s, "SCENE: "+actor.Name+" stands in "+game.ZoneName(w.Here(), near)+" with: Tamsin Hale.") {
		t.Fatalf("scene line missing or wrong:\n%s", s)
	}
	if !strings.Contains(s, "Elder Corbin ["+corbin.ID+"] elder, attitude +1, in "+game.ZoneName(w.Here(), corbin.ZoneID)+" (elsewhere, not in the scene)") {
		t.Fatalf("absent NPC not marked:\n%s", s)
	}
	if strings.Contains(s, "Tamsin Hale ["+tamsin.ID+"] blacksmith, attitude +1, in "+game.ZoneName(w.Here(), near)+" (elsewhere") {
		t.Fatal("present NPC marked absent")
	}
	// the narrator sees how people in the scene look, and only them
	tamsin.Desc, corbin.Desc = "Soot to the elbows.", "Heard but not seen."
	s = snapshot(w, actor, true)
	if !strings.Contains(s, "looks: Soot to the elbows.") || strings.Contains(s, "Heard but not seen.") {
		t.Fatalf("scene descriptions wrong:\n%s", s)
	}
}

func TestTalkToWalksOverToTheNPC(t *testing.T) {
	r := newRig(t, 1, 1)
	w := r.start(t)
	corbin := w.NPC("Elder Corbin")
	_, _, far := putApart(t, w, corbin)
	_, err := r.g.Store.Save(bg, w)
	must(t, err)
	res := r.act(t, "u1", "I ask Corbin about the bell", llm.Call("talk_to", map[string]any{"npc": corbin.ID}))
	if res.Resolution.Steps[0].Error != "" || r.world(t).Character(res.Resolution.ActorID).ZoneID != far {
		t.Fatalf("talk_to did not walk the actor to %s: %+v", far, res.Resolution.Steps)
	}
}

func TestInterpretKeepsTheTalkAfterAStrayReposition(t *testing.T) {
	r := newRig(t, 1, 1)
	w := r.start(t)
	corbin := w.NPC("Elder Corbin")
	_, _, far := putApart(t, w, corbin)
	_, err := r.g.Store.Save(bg, w)
	must(t, err)
	// the model only walks over, and adds nothing when asked for more
	r.f.On("interpret", llm.Calls(llm.Call("reposition", map[string]any{"zone": far})))
	r.f.On("interpret_more", llm.Text("done"))
	res, err := r.g.TakeAction(bg, r.id, "u1", "I talk to Elder Corbin: do you have any jobs?", Hints{TargetID: corbin.ID})
	must(t, err)
	steps := res.Resolution.Steps
	if len(steps) != 2 || steps[1].Action.Kind != game.ActTalkTo || steps[1].Action.Target != corbin.ID {
		t.Fatalf("talk step lost: %+v", steps)
	}
	if len(r.f.Tagged("interpret_more")) == 0 {
		t.Fatal("interpreter was not asked for the missing step")
	}
	// plain movement stays a plain move
	if strayReposition([]game.Action{{Kind: game.ActReposition}}, "I move to the green") {
		t.Fatal("explicit move treated as stray")
	}
}

func TestCharacterNamesStayUnique(t *testing.T) {
	r := newRig(t, 1, 1)
	_, _, err := r.g.Join(bg, r.id, "u2", "Pu2")
	must(t, err)
	if _, err := r.g.SaveCharacter(bg, r.id, "u2", specs[0]); err == nil || !strings.Contains(err.Error(), "already in the party") {
		t.Fatalf("duplicate name accepted: %v", err)
	}
	// a player can re-save their own sheet under the same name
	_, err = r.g.SaveCharacter(bg, r.id, "u1", specs[0])
	must(t, err)
	r.f.On("author_character", llm.Calls(llm.Call("draft_character", map[string]any{"name": "Aria", "class": "Caravan Guard", "kit": "warrior"})))
	d := r.g.DraftCharacter(bg, r.world(t), "caravan guard")
	if strings.EqualFold(d.Name, "Aria") {
		t.Fatalf("draft reused a party name: %q", d.Name)
	}
	// the model is told the story's people too, so a hero doesn't share a
	// first name with a watch captain
	u := lastUser(r.f.Tagged("author_character")[0])
	if !strings.Contains(u, "People in the story") || !strings.Contains(u, "Elder Corbin") {
		t.Fatalf("draft prompt lacks the story's people:\n%s", u)
	}
}

func TestCastAndBeatFiguresBecomeRealNPCs(t *testing.T) {
	r := newRig(t, 0, 1)
	spec := fallbackSpec("")
	spec.Cast = append(spec.Cast, struct {
		game.NPCSpec
		At string `json:"at"`
	}{game.NPCSpec{Name: "The Returned Fishermen", Role: "three gilled fishermen"}, "Harrowmere"},
		struct {
			game.NPCSpec
			At string `json:"at"`
		}{game.NPCSpec{Name: "Old Hermit", Role: "marsh hermit"}, "Reedmarsh"})
	spec.MainQuest.Objectives = append([]game.ObjectiveSpec{{Kind: game.ObjTalkToNPC, Text: "Examine the returned fishermen", Target: "The Returned Fishermen"}}, spec.MainQuest.Objectives...)
	spec.Beats[0].Figure = &struct {
		Name    string `json:"name"`
		Role    string `json:"role"`
		Desc    string `json:"desc"`
		Hostile bool   `json:"hostile"`
	}{Name: "Drowned Choir", Role: "singing dead", Hostile: false}
	w, p := r.g.buildCampaign("c9", "premise", spec)
	if len(p) > 0 {
		t.Fatal(p)
	}
	fish, hermit, choir := w.NPC("The Returned Fishermen"), w.NPC("Old Hermit"), w.NPC("Drowned Choir")
	if fish == nil || fish.LocationID != w.Campaign.HubID {
		t.Fatalf("fishermen not at the hub: %+v", fish)
	}
	if hermit == nil || hermit.LocationID != w.Location("Reedmarsh").ID {
		t.Fatalf("hermit not in the marsh: %+v", hermit)
	}
	if o := w.MainQuest().Objectives[0]; o.Target != fish.ID {
		t.Fatalf("objective not tied to the fishermen: %+v", o)
	}
	b := w.Beats[0]
	if choir == nil || choir.LocationID != "" || b.EffectTarget != choir.ID || b.Effects[0] != game.EffectBringNPC {
		t.Fatalf("figure not waiting offstage for its beat: %+v %+v", choir, b)
	}
}

func TestBeatFigureArrivesInTheActorsZone(t *testing.T) {
	r := newRig(t, 1, 1)
	w := r.start(t)
	l := w.Here()
	actor := w.ActiveCharacter()
	actor.ZoneID = l.Zones[len(l.Zones)-1].ID
	choir := game.AddNPC(w, "", game.NPCSpec{Name: "Drowned Choir", Role: "singing dead"})
	b := &game.Beat{ID: "b9", Title: "Ashore", Text: "The Drowned Choir wades out of the fog.", Effects: []string{game.EffectBringNPC}, EffectTarget: choir.ID}
	ch := r.g.applyBeat(bg, w, b, actor)
	if choir.LocationID != l.ID || choir.ZoneID != actor.ZoneID || len(ch) == 0 {
		t.Fatalf("choir at %s/%s, changes %v", choir.LocationID, choir.ZoneID, ch)
	}
}

func TestLookAtSomethingAbsentOrSomeone(t *testing.T) {
	r := newRig(t, 1, 1)
	w := r.start(t)
	r.f.Default("narrate_quick", llm.Text("Nothing like that here."))
	res := r.act(t, "u1", "I examine the gills", llm.Call("look", map[string]any{"target": "gills"}))
	info := strings.Join(res.Resolution.Steps[0].Info, " | ")
	if !strings.Contains(info, "finds nothing like it here") || strings.Contains(info, w.Here().Description) {
		t.Fatalf("absent look described the place: %s", info)
	}
	corbin := w.NPC("Elder Corbin")
	q, err := game.AddQuest(w, game.QuestSpec{Title: "See him", GiverID: "Tamsin Hale", Objectives: []game.ObjectiveSpec{{Kind: game.ObjTalkToNPC, Text: "Examine Corbin", Target: corbin.ID}}}, false)
	must(t, err)
	_, err = game.AcceptQuest(w, q.ID)
	must(t, err)
	_, err = r.g.Store.Save(bg, w)
	must(t, err)
	r.act(t, "u1", "I study the elder", llm.Call("look", map[string]any{"target": corbin.ID}))
	if !r.world(t).Quest(q.ID).Objectives[0].Done {
		t.Fatal("examining the NPC did not complete the talk_to_npc objective")
	}
}

func TestNarrationLosesEngineWords(t *testing.T) {
	r := newRig(t, 1, 1)
	r.start(t)
	r.f.Default("narrate_quick", llm.Text("Brim stands with the elder. Tamsin is one zone off."))
	r.f.On("dejargon", llm.Text("Brim stands with the elder. Tamsin works across the square."))
	res := r.act(t, "u1", "where is everyone?", llm.Call("look", map[string]any{}))
	if strings.Contains(res.Narration, "zone") || !strings.Contains(res.Narration, "across the square") {
		t.Fatalf("narration kept engine words: %q", res.Narration)
	}
}

func TestCreateCampaignKeepsGearInTheSetting(t *testing.T) {
	st, _ := store.Open(":memory:")
	defer st.Close()
	f := llm.NewFake()
	g := New(f, st, Config{}, dice.New(1))
	g.Log = log.New(io.Discard, "", 0)
	spec := fallbackSpec("")
	spec.Setting = "fantasy" // the premise's year should win
	f.On("author_campaign", llm.Calls(llm.Call("create_campaign", spec)))
	w, err := g.CreateCampaign(bg, "Innsmouth, 1932: the fishermen have stopped talking.")
	must(t, err)
	if w.Campaign.Setting != game.SettingPulp {
		t.Fatalf("setting %q", w.Campaign.Setting)
	}
	if p := lastUser(f.Tagged("author_campaign")[0]); !strings.Contains(p, "pulp_crossbow") || strings.Contains(p, " longsword,") {
		t.Fatalf("prompt should list pulp catalog keys: %s", p)
	}
	stocked := 0
	for _, it := range w.Items {
		if it.Owner.Kind == game.OwnerNPC {
			stocked++
			if d := game.Catalog[it.Key]; d.Setting != game.SettingPulp && d.Key != "rations" {
				t.Errorf("merchant sells %s (%s) in a pulp game", it.Name, it.Key)
			}
		}
	}
	if stocked == 0 {
		t.Fatal("no merchant stock")
	}
	got, err := st.Load(bg, w.Campaign.ID)
	must(t, err)
	if got.Campaign.Setting != game.SettingPulp {
		t.Fatalf("setting not saved: %q", got.Campaign.Setting)
	}
	r := newRig(t, 0, 1)
	r.f.On("author_character", llm.Calls(llm.Call("draft_character", map[string]any{"name": "Cal", "kit": "detective"})))
	draft := r.g.DraftCharacter(bg, w, "a hard-drinking private detective")
	if draft.Kit != "skirmisher" {
		t.Fatalf("kit %q", draft.Kit)
	}
	if p := lastUser(r.f.Tagged("author_character")[0]); !strings.Contains(p, "Gumshoe") || !strings.Contains(p, "Setting: Pulp") {
		t.Fatalf("draft prompt lacks the setting's kits: %s", p)
	}
}

func TestWorldRemembersPromisesAcrossTurns(t *testing.T) {
	r := newRig(t, 1, 1)
	r.start(t)
	corbin := r.world(t).NPC("Elder Corbin")
	// the narrator records the promise the player makes
	r.f.On("narrate", func(llm.Request) (llm.Response, error) {
		return llm.Response{Content: "Corbin nods slowly.", ToolCalls: []llm.ToolCall{
			llm.Call(DirRecordLore, map[string]any{"text": "Ewan promised Corbin to ring the bell again", "kind": "promise", "npc_ids": []string{corbin.ID}}),
			llm.Call(DirAdjustDisp, map[string]any{"npc_id": corbin.ID, "delta": 1, "reason": "offered to help with the bell"}),
		}}, nil
	})
	r.act(t, "u1", "I'll ring your bell again, I swear it", llm.Call("talk_to", map[string]any{"npc": "Elder Corbin", "text": "I swear it"}))
	w := r.world(t)
	p := w.OpenPromises()
	if len(p) != 1 || p[0].NPCIDs[0] != corbin.ID || p[0].Day == 0 {
		t.Fatalf("promise not stored: %+v", w.Chronicle)
	}
	if !strings.Contains(strings.Join(w.NPC(corbin.ID).Recall(), "|"), "warmed to the party: offered to help") {
		t.Fatalf("Corbin forgot why: %v", w.NPC(corbin.ID).Recall())
	}
	// on the next turn the narrator is shown the open promise and settles it
	r.f.On("narrate", llm.Calls(llm.Call(DirResolvePromise, map[string]any{"promise_id": p[0].ID, "outcome": "kept", "note": "the bell rang"})))
	before := w.NPC(corbin.ID).Disposition
	seen := len(r.f.Tagged("narrate"))
	r.act(t, "u1", "I tell Corbin the bell is ringing", llm.Call("talk_to", map[string]any{"npc": "Elder Corbin", "text": "done"}))
	u := lastUser(r.f.Tagged("narrate")[seen])
	if !strings.Contains(u, "WORLD MEMORY") || !strings.Contains(u, "["+p[0].ID+"] day") || !strings.Contains(u, "ring the bell again") {
		t.Fatalf("narrator not shown the promise:\n%s", u)
	}
	w = r.world(t)
	if len(w.OpenPromises()) != 0 || w.Fact(p[0].ID).Status != game.PromiseKept {
		t.Fatalf("promise not kept: %+v", w.Fact(p[0].ID))
	}
	n := w.NPC(corbin.ID)
	if n.Disposition != clampInt(before+1, -3, 3) || len(n.Keeps) == 0 {
		t.Fatalf("Corbin disposition %d keeps %v", n.Disposition, n.Keeps)
	}
}

func TestRecordLoreRejectsPromiseToNobody(t *testing.T) {
	r := newRig(t, 1, 1)
	w := r.start(t)
	if _, err := r.g.applyDirective(bg, w, llm.Call(DirRecordLore, map[string]any{"text": "Ewan swore an oath", "kind": "promise"})); err == nil {
		t.Fatal("promise without anyone to keep it to")
	}
	if _, err := r.g.applyDirective(bg, w, llm.Call(DirRecordLore, map[string]any{"text": "They say the lake has no bottom", "kind": "rumor"})); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(worldMemory(w, w.Characters[0]), "rumor: They say") {
		t.Fatalf("memory %q", worldMemory(w, w.Characters[0]))
	}
}
