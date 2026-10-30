package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Solvely-Colin/aidnd/internal/dice"
	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/gm"
	"github.com/Solvely-Colin/aidnd/internal/llm"
	"github.com/Solvely-Colin/aidnd/internal/store"
)

type env struct {
	t    *testing.T
	srv  *httptest.Server
	api  *Server
	g    *gm.GM
	fake *llm.Fake
	key  string
}

func newEnv(t *testing.T, apiKey string) *env {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	f := llm.NewFake()
	g := gm.New(f, st, gm.Config{}, dice.New(11))
	g.Log = log.New(io.Discard, "", 0)
	n := 0
	g.NewID = func() string { n++; return fmt.Sprintf("camp%d", n) }
	s := New(g, Config{APIKey: apiKey, CORSOrigins: []string{"http://host.example"}})
	s.log = log.New(io.Discard, "", 0)
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(func() { srv.Close(); st.Close() })
	return &env{t: t, srv: srv, api: s, g: g, fake: f, key: apiKey}
}

type resp struct {
	code int
	body []byte
}

func (r resp) json(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(r.body, &m); err != nil {
		t.Fatalf("bad json %q: %v", r.body, err)
	}
	return m
}

// do sends a request. headers: "token" sets Bearer, "player" sets X-Player-ID.
func (e *env) do(method, path string, body any, token, player string) resp {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, rd)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if player != "" {
		req.Header.Set("X-Player-ID", player)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return resp{res.StatusCode, b}
}

var sheet = map[string]any{"name": "Aria", "kit": "warrior", "scores": map[string]int{"STR": 15, "DEX": 12, "CON": 14, "INT": 8, "WIS": 13, "CHA": 10}, "proficient": []string{"STR", "CON"}}
var sheet2 = map[string]any{"name": "Bram", "kit": "ranger", "scores": map[string]int{"STR": 10, "DEX": 15, "CON": 13, "INT": 12, "WIS": 14, "CHA": 8}, "proficient": []string{"DEX", "WIS"}}

func TestOpenModeFlow(t *testing.T) {
	e := newEnv(t, "")
	r := e.do("POST", "/campaigns", map[string]string{"premise": "a lake town"}, "", "")
	if r.code != 201 {
		t.Fatalf("create %d %s", r.code, r.body)
	}
	id := r.json(t)["id"].(string)
	j1 := e.do("POST", "/campaigns/"+id+"/players", map[string]string{"name": "Ann"}, "", "")
	j2 := e.do("POST", "/campaigns/"+id+"/players", map[string]string{"name": "Ben"}, "", "")
	if j1.code != 201 || j2.code != 201 {
		t.Fatalf("join %d %d", j1.code, j2.code)
	}
	t1, t2 := j1.json(t)["token"].(string), j2.json(t)["token"].(string)
	if t1 == "" || t1 == t2 {
		t.Fatal("tokens")
	}
	if r := e.do("GET", "/campaigns/"+id, nil, "bogus", ""); r.code != 401 {
		t.Fatalf("bogus token %d", r.code)
	}
	bad := map[string]any{"name": "X", "kit": "warrior", "scores": map[string]int{"STR": 18}, "proficient": []string{"STR"}}
	r = e.do("POST", "/campaigns/"+id+"/characters", map[string]any{"character": bad}, t1, "")
	if r.code != 422 || len(r.json(t)["problems"].([]any)) < 2 {
		t.Fatalf("bad sheet %d %s", r.code, r.body)
	}
	if r := e.do("POST", "/campaigns/"+id+"/start", nil, t1, ""); r.code != 409 {
		t.Fatalf("early start %d", r.code)
	}
	if r := e.do("POST", "/campaigns/"+id+"/characters", map[string]any{"character": sheet}, t1, ""); r.code != 201 {
		t.Fatalf("sheet %d %s", r.code, r.body)
	}
	r = e.do("POST", "/campaigns/"+id+"/characters/draft", map[string]string{"concept": "a sneaky thief"}, t2, "")
	if r.code != 200 || r.json(t)["character"].(map[string]any)["kit"] != "skirmisher" {
		t.Fatalf("draft %d %s", r.code, r.body)
	}
	if r := e.do("POST", "/campaigns/"+id+"/characters", map[string]any{"concept": "a sneaky thief"}, t2, ""); r.code != 201 {
		t.Fatalf("quick create %d %s", r.code, r.body)
	}
	r = e.do("POST", "/campaigns/"+id+"/start", nil, t1, "")
	if r.code != 200 {
		t.Fatalf("start %d %s", r.code, r.body)
	}
	v := e.do("GET", "/campaigns/"+id, nil, t1, "").json(t)
	you := v["you"].(map[string]any)
	if you["joined"] != true || you["your_turn"] != true || v["location"] == nil || v["budget"] == nil {
		t.Fatalf("view %s", e.do("GET", "/campaigns/"+id, nil, t1, "").body)
	}
	checkSheets(t, v)
	// outside a fight everyone on their feet may act
	if v2 := e.do("GET", "/campaigns/"+id, nil, t2, "").json(t); v2["free_roam"] != true || v2["you"].(map[string]any)["your_turn"] != true {
		t.Fatalf("free roam view %v", v2["you"])
	}
	e.fake.On("interpret", llm.Calls(llm.Call("look", map[string]any{})))
	r = e.do("POST", "/campaigns/"+id+"/actions", map[string]string{"text": "what do I see?"}, t1, "")
	if r.code != 200 {
		t.Fatalf("action %d %s", r.code, r.body)
	}
	tr := r.json(t)
	if tr["quick"] != true || tr["turn_ended"] != false || tr["budget"] == nil || tr["narration"] == "" {
		t.Fatalf("turn result %s", r.body)
	}
	e.fake.On("interpret", llm.Calls(llm.Call("end_turn", map[string]any{})))
	tr = e.do("POST", "/campaigns/"+id+"/actions", map[string]string{"text": "done"}, t1, "").json(t)
	if tr["next_actor"] != nil || tr["budget"] == nil {
		t.Fatalf("outside a fight nobody is next: %v", tr)
	}
	lg := e.do("GET", "/campaigns/"+id+"/log?after=0", nil, t1, "").json(t)
	types := map[string]bool{}
	for _, ev := range lg["events"].([]any) {
		types[ev.(map[string]any)["type"].(string)] = true
	}
	for _, want := range []string{"campaign_created", "player_joined", "character_created", "player_action", "action_resolved", "narration", "turn_started", "turn_budget", "location_changed"} {
		if !types[want] {
			t.Errorf("missing event %s", want)
		}
	}
	for _, p := range []string{"/map", "/quests", "/journal"} {
		if r := e.do("GET", "/campaigns/"+id+p, nil, t1, ""); r.code != 200 {
			t.Errorf("%s: %d", p, r.code)
		}
	}
	me := e.do("GET", "/me/campaigns", nil, "", j1.json(t)["player"].(map[string]any)["id"].(string)).json(t)
	if len(me["campaigns"].([]any)) != 1 {
		t.Fatal("me/campaigns")
	}
}

func TestServiceModeAuth(t *testing.T) {
	e := newEnv(t, "sekrit")
	if r := e.do("GET", "/campaigns", nil, "", ""); r.code != 401 {
		t.Fatalf("no key %d", r.code)
	}
	if r := e.do("GET", "/healthz", nil, "", ""); r.code != 200 {
		t.Fatal("healthz needs no auth")
	}
	id := e.do("POST", "/campaigns", map[string]string{"premise": "x"}, "sekrit", "").json(t)["id"].(string)
	if r := e.do("POST", "/campaigns/"+id+"/players", map[string]string{"name": "Ann"}, "sekrit", ""); r.code != 400 {
		t.Fatalf("join without player id %d", r.code)
	}
	r := e.do("POST", "/campaigns/"+id+"/players", map[string]string{"name": "Ann"}, "sekrit", "host-user-1")
	if r.code != 201 || r.json(t)["token"] != nil {
		t.Fatalf("join %d %s", r.code, r.body)
	}
	if r := e.do("POST", "/campaigns/"+id+"/players", map[string]string{"name": "Ann again"}, "sekrit", "host-user-1"); r.code != 200 || r.json(t)["created"] != false {
		t.Fatalf("rejoin %d %s", r.code, r.body)
	}
	if r := e.do("POST", "/campaigns/"+id+"/players", nil, "wrong", "host-user-1"); r.code != 401 {
		t.Fatalf("wrong key %d", r.code)
	}
	// stream tokens
	tok := e.do("POST", "/campaigns/"+id+"/stream-token", nil, "sekrit", "host-user-1").json(t)["token"].(string)
	if p, ok := e.api.checkStreamToken(tok, id); !ok || p != "host-user-1" {
		t.Fatal("token check")
	}
	if _, ok := e.api.checkStreamToken(tok, "other"); ok {
		t.Fatal("token valid for another campaign")
	}
	e.api.now = func() time.Time { return time.Now().Add(time.Hour) }
	if _, ok := e.api.checkStreamToken(tok, id); ok {
		t.Fatal("expired token accepted")
	}
	e.api.now = time.Now
	res, _ := http.Get(e.srv.URL + "/campaigns/" + id + "/events")
	if res.StatusCode != 401 {
		t.Fatalf("stream without token %d", res.StatusCode)
	}
	res.Body.Close()
	res, _ = http.Get(e.srv.URL + "/campaigns/" + id + "/events?stream_token=" + tok + "x")
	if res.StatusCode != 401 {
		t.Fatalf("tampered token %d", res.StatusCode)
	}
	res.Body.Close()
}

// sseReader parses one SSE body in the background.
type sseReader struct{ ch chan map[string]any }

func newSSEReader(body io.Reader) *sseReader {
	r := &sseReader{ch: make(chan map[string]any, 100)}
	go func() {
		sc := bufio.NewScanner(body)
		for sc.Scan() {
			if line := sc.Text(); strings.HasPrefix(line, "data: ") {
				var m map[string]any
				_ = json.Unmarshal([]byte(line[6:]), &m)
				r.ch <- m
			}
		}
		close(r.ch)
	}()
	return r
}

func (r *sseReader) next(n int, timeout time.Duration) []map[string]any {
	var got []map[string]any
	deadline := time.After(timeout)
	for len(got) < n {
		select {
		case m, ok := <-r.ch:
			if !ok {
				return got
			}
			got = append(got, m)
		case <-deadline:
			return got
		}
	}
	return got
}

func TestSSEReplayAndLive(t *testing.T) {
	e := newEnv(t, "k")
	id := e.do("POST", "/campaigns", map[string]string{"premise": "x"}, "k", "").json(t)["id"].(string)
	e.do("POST", "/campaigns/"+id+"/players", map[string]string{"name": "Ann"}, "k", "u1")
	e.do("POST", "/campaigns/"+id+"/characters", map[string]any{"character": sheet}, "k", "u1")
	all, _ := e.g.Store.Events(t.Context(), id, 0, 0)
	if len(all) != 3 {
		t.Fatalf("events %d", len(all))
	}
	tok := e.do("POST", "/campaigns/"+id+"/stream-token", nil, "k", "u1").json(t)["token"].(string)
	req, _ := http.NewRequest("GET", e.srv.URL+"/campaigns/"+id+"/events?stream_token="+tok, nil)
	req.Header.Set("Last-Event-ID", fmt.Sprint(all[0].ID))
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("stream: %v %v", err, res)
	}
	defer res.Body.Close()
	sr := newSSEReader(res.Body)
	got := sr.next(2, 2*time.Second)
	if len(got) != 2 || got[0]["type"] != "player_joined" || got[1]["type"] != "character_created" {
		t.Fatalf("replay %v", got)
	}
	for i := 0; i < 50 && e.api.Hub().Subscribers(id) == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if r := e.do("POST", "/campaigns/"+id+"/start", nil, "k", ""); r.code != 200 {
		t.Fatalf("start %d %s", r.code, r.body)
	}
	live := sr.next(3, 2*time.Second)
	if len(live) != 3 || live[0]["type"] != "location_changed" || live[2]["type"] != "turn_started" {
		t.Fatalf("live %v", live)
	}
}

func TestViewsNeverLeakSecrets(t *testing.T) {
	e := newEnv(t, "k")
	id := e.do("POST", "/campaigns", map[string]string{"premise": "x"}, "k", "").json(t)["id"].(string)
	e.do("POST", "/campaigns/"+id+"/players", map[string]string{"name": "Ann"}, "k", "u1")
	e.do("POST", "/campaigns/"+id+"/players", map[string]string{"name": "Ben"}, "k", "u2")
	e.do("POST", "/campaigns/"+id+"/characters", map[string]any{"character": sheet}, "k", "u1")
	e.do("POST", "/campaigns/"+id+"/characters", map[string]any{"character": sheet2}, "k", "u2")
	// a hidden exit at the hub and a note in an NPC's memory
	w, _ := e.g.Store.Load(t.Context(), id)
	hidden := w.AddStub("Secret Cellar", "under the inn", "cave", 1)
	he := w.Connect(w.Campaign.HubID, hidden.ID, game.EdgeHidden, "a trapdoor", "")
	w.NPC("Tamsin Hale").Remember("SECRETMEMORY the party owes me")
	_, _ = e.g.Store.Save(t.Context(), w)
	e.do("POST", "/campaigns/"+id+"/start", nil, "k", "")
	e.fake.On("interpret", llm.Calls(llm.Call("talk_to", map[string]any{"npc": "Tamsin Hale", "text": "hello"})))
	if r := e.do("POST", "/campaigns/"+id+"/actions", map[string]string{"text": "hello Tamsin"}, "k", "u1"); r.code != 200 {
		t.Fatalf("action %d %s", r.code, r.body)
	}
	w, _ = e.g.Store.Load(t.Context(), id)
	var secrets []string
	for _, l := range w.Lore {
		secrets = append(secrets, l.Text)
	}
	for _, b := range w.Beats {
		secrets = append(secrets, b.Text)
	}
	for _, p := range w.Players {
		secrets = append(secrets, p.Token)
	}
	secrets = append(secrets, he.ID, "Secret Cellar", "SECRETMEMORY", w.Location(w.Campaign.LairID).Hint)
	tamsin := w.NPC("Tamsin Hale").ID
	paths := []string{"", "/map", "/quests", "/journal", "/log?after=0", "/npcs/" + tamsin, "/locations/" + w.Campaign.HubID, "/characters/" + w.Characters[0].ID}
	for _, p := range paths {
		r := e.do("GET", "/campaigns/"+id+p, nil, "k", "u1")
		if r.code != 200 {
			t.Fatalf("%s: %d", p, r.code)
		}
		for _, sec := range secrets {
			if sec != "" && strings.Contains(string(r.body), sec) {
				t.Errorf("%s leaks %q", p, sec)
			}
		}
	}
	// stubs and hidden places are not served
	if r := e.do("GET", "/campaigns/"+id+"/locations/"+hidden.ID, nil, "k", "u1"); r.code != 404 {
		t.Fatalf("hidden location %d", r.code)
	}
	if r := e.do("GET", "/campaigns/"+id+"/npcs/"+w.Campaign.AntagonistID, nil, "k", "u1"); r.code != 404 {
		t.Fatalf("antagonist npc %d", r.code)
	}
	// main quest shows only objectives reached so far
	q := e.do("GET", "/campaigns/"+id+"/quests", nil, "k", "u1").json(t)
	for _, qq := range q["quests"].([]any) {
		m := qq.(map[string]any)
		if m["is_main"] == true && len(m["objectives"].([]any)) != 1 { // only the current step
			t.Fatalf("main objectives %v", m["objectives"])
		}
	}
}

func TestSkipAndCORS(t *testing.T) {
	e := newEnv(t, "k")
	id := e.do("POST", "/campaigns", map[string]string{"premise": "x"}, "k", "").json(t)["id"].(string)
	for i, s := range []map[string]any{sheet, sheet2} {
		pid := fmt.Sprintf("u%d", i+1)
		e.do("POST", "/campaigns/"+id+"/players", map[string]string{"name": pid}, "k", pid)
		e.do("POST", "/campaigns/"+id+"/characters", map[string]any{"character": s}, "k", pid)
	}
	e.do("POST", "/campaigns/"+id+"/start", nil, "k", "")
	// outside a fight there is no turn to skip
	if r := e.do("POST", "/campaigns/"+id+"/skip", map[string]bool{"force": true}, "k", ""); r.code != 409 {
		t.Fatalf("skip outside a fight: %d %s", r.code, r.body)
	}
	w, err := e.g.Store.Load(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := game.StartEncounter(w, []game.EnemySpec{{Name: "Rat", HP: 20, AC: 10, Damage: "1d2"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.g.Store.Save(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	if r := e.do("POST", "/campaigns/"+id+"/skip", nil, "k", "u2"); r.code != 409 {
		t.Fatalf("u2 skipping u1 early: %d %s", r.code, r.body)
	}
	r := e.do("POST", "/campaigns/"+id+"/skip", map[string]bool{"force": true}, "k", "")
	if r.code != 200 || r.json(t)["next_actor"].(map[string]any)["player_id"] != "u2" {
		t.Fatalf("backend skip %d %s", r.code, r.body)
	}
	req, _ := http.NewRequest("OPTIONS", e.srv.URL+"/campaigns", nil)
	req.Header.Set("Origin", "http://host.example")
	res, _ := http.DefaultClient.Do(req)
	if res.StatusCode != 204 || res.Header.Get("Access-Control-Allow-Origin") != "http://host.example" {
		t.Fatalf("cors %d %v", res.StatusCode, res.Header)
	}
	req.Header.Set("Origin", "http://evil.example")
	res, _ = http.DefaultClient.Do(req)
	if res.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("cors allowed unknown origin")
	}
}

func TestMeta(t *testing.T) {
	e := newEnv(t, "")
	m := e.do("GET", "/meta", nil, "", "").json(t)
	if len(m["kits"].([]any)) != 6 || len(m["standard_array"].([]any)) != 6 || len(m["xp_table"].([]any)) != 5 {
		t.Fatalf("meta %v", m)
	}
	settings := m["settings"].([]any)
	if len(settings) != 5 {
		t.Fatalf("settings %v", settings)
	}
	for _, sv := range settings {
		s := sv.(map[string]any)
		if s["key"] != "pulp" {
			continue
		}
		gum := s["kits"].([]any)[1].(map[string]any)
		if gum["name"] != "Gumshoe" || gum["weapon"] != "pulp_crossbow" {
			t.Fatalf("pulp skirmisher %v", gum)
		}
	}
}

// checkSheets: every party member's full sheet is in the view, so players
// can read each other's sheets.
func checkSheets(t *testing.T, v map[string]any) {
	t.Helper()
	party := v["party"].([]any)
	if len(party) != 2 {
		t.Fatalf("party %d", len(party))
	}
	active := v["active"].(map[string]any)["character_id"]
	for _, p := range party {
		c := p.(map[string]any)
		sum := 0.0
		for _, part := range c["ac_parts"].([]any) {
			sum += part.(map[string]any)["value"].(float64)
		}
		if sum != c["ac"].(float64) {
			t.Errorf("%s: ac parts %v != ac %v", c["name"], sum, c["ac"])
		}
		atk := c["attack"].(map[string]any)
		if atk["weapon"] == "" || atk["damage"] == "" || atk["bonus"].(float64) < 2 {
			t.Errorf("%s: attack %v", c["name"], atk)
		}
		if lu := c["level_up"].(map[string]any); lu["level"].(float64) != 2 || lu["hp"].(float64) < 1 {
			t.Errorf("%s: level_up %v", c["name"], lu)
		}
		if c["armor"] == nil || c["zone_name"] == "" {
			t.Errorf("%s: armor %v zone %v", c["name"], c["armor"], c["zone_name"])
		}
		acting := false
		for _, cd := range c["conditions"].([]any) {
			if cd.(map[string]any)["key"] == "acting" {
				acting = true
			}
		}
		if acting != (c["id"] == active) {
			t.Errorf("%s: acting condition %v, active %v", c["name"], acting, active)
		}
		weapons := 0
		for _, it := range c["inventory"].([]any) {
			it := it.(map[string]any)
			if it["kind"] == "weapon" {
				weapons++
				if _, ok := it["to_hit"]; !ok {
					t.Errorf("%s: %s has no to_hit", c["name"], it["name"])
				}
			}
		}
		if weapons == 0 {
			t.Errorf("%s: no weapons carried", c["name"])
		}
	}
}

func TestJournalPeopleAndPlaces(t *testing.T) {
	e := newEnv(t, "k")
	id := e.do("POST", "/campaigns", map[string]string{"premise": "x"}, "k", "").json(t)["id"].(string)
	e.do("POST", "/campaigns/"+id+"/players", map[string]string{"name": "Ann"}, "k", "u1")
	e.do("POST", "/campaigns/"+id+"/characters", map[string]any{"character": sheet}, "k", "u1")
	e.do("POST", "/campaigns/"+id+"/start", nil, "k", "")
	e.fake.On("interpret", llm.Calls(llm.Call("talk_to", map[string]any{"npc": "Tamsin Hale", "text": "hello"})))
	e.do("POST", "/campaigns/"+id+"/actions", map[string]string{"text": "hello Tamsin"}, "k", "u1")
	w, _ := e.g.Store.Load(t.Context(), id)
	antagonist := w.NPC(w.Campaign.AntagonistID).Name

	var j JournalView
	if err := json.Unmarshal(e.do("GET", "/campaigns/"+id+"/journal", nil, "k", "u1").body, &j); err != nil {
		t.Fatal(err)
	}
	var tamsin *JournalPerson
	for i, p := range j.People {
		if p.Name == antagonist {
			t.Errorf("journal lists the antagonist %q away from the lair", antagonist)
		}
		if p.Name == "Tamsin Hale" {
			tamsin = &j.People[i]
		}
	}
	if tamsin == nil || !tamsin.Met || !tamsin.Here || tamsin.LocationName == "" {
		t.Fatalf("tamsin %+v in %+v", tamsin, j.People)
	}
	if len(j.Places) < 2 || !j.Places[0].Here || j.Places[0].ID != w.Campaign.HubID || j.Places[0].Description == "" {
		t.Fatalf("places %+v", j.Places)
	}
	for _, p := range j.Places {
		if !p.Visited && p.Description != "" {
			t.Errorf("unvisited %s shows its description", p.Name)
		}
	}
}

func TestNotesArePrivate(t *testing.T) {
	e := newEnv(t, "k")
	id := e.do("POST", "/campaigns", map[string]string{"premise": "x"}, "k", "").json(t)["id"].(string)
	e.do("POST", "/campaigns/"+id+"/players", map[string]string{"name": "Ann"}, "k", "u1")
	e.do("POST", "/campaigns/"+id+"/players", map[string]string{"name": "Ben"}, "k", "u2")
	base := "/campaigns/" + id + "/notes"
	if r := e.do("POST", base, map[string]string{"text": "hi"}, "k", "u3"); r.code != 403 {
		t.Fatalf("outsider wrote a note: %d", r.code)
	}
	if r := e.do("POST", base, map[string]string{"text": "  "}, "k", "u1"); r.code != 400 {
		t.Fatalf("empty note %d", r.code)
	}
	r := e.do("POST", base, map[string]string{"text": "Tamsin flinched at the bell"}, "k", "u1")
	if r.code != 201 {
		t.Fatalf("add %d %s", r.code, r.body)
	}
	nid := r.json(t)["id"].(string)
	if n := e.do("GET", base, nil, "k", "u2").json(t)["notes"].([]any); len(n) != 0 {
		t.Fatalf("u2 sees %v", n)
	}
	if r := e.do("POST", base+"/"+nid, map[string]string{"text": "mine now"}, "k", "u2"); r.code != 404 {
		t.Fatalf("u2 edited u1's note: %d", r.code)
	}
	if r := e.do("POST", base+"/"+nid, map[string]string{"text": "Tamsin flinched twice"}, "k", "u1"); r.code != 200 {
		t.Fatalf("edit %d", r.code)
	}
	n := e.do("GET", base, nil, "k", "u1").json(t)["notes"].([]any)
	if len(n) != 1 || n[0].(map[string]any)["text"] != "Tamsin flinched twice" {
		t.Fatalf("notes %v", n)
	}
	if r := e.do("DELETE", base+"/"+nid, nil, "k", "u1"); r.code != 200 {
		t.Fatalf("delete %d", r.code)
	}
	if r := e.do("DELETE", base+"/"+nid, nil, "k", "u1"); r.code != 404 {
		t.Fatalf("second delete %d", r.code)
	}
	// the narrator never sees notes: nothing about them reaches the world
	e.do("POST", base, map[string]string{"text": "PRIVATENOTE"}, "k", "u1")
	w, _ := e.g.Store.Load(t.Context(), id)
	if b, _ := json.Marshal(w); strings.Contains(string(b), "PRIVATENOTE") {
		t.Fatal("a note leaked into the world")
	}
}

func TestRulesetBuilderAndLevelUp(t *testing.T) {
	e := newEnv(t, "")
	m := e.do("GET", "/meta?setting=pulp", nil, "", "").json(t)
	rs := m["ruleset"].(map[string]any)
	found := false
	for _, c := range rs["classes"].([]any) {
		cv := c.(map[string]any)
		found = found || (cv["key"] == "wizard" && cv["name"] == "Occultist")
	}
	if !found || len(rs["spells"].([]any)) < 60 || rs["setting"] != "pulp" {
		t.Fatalf("pulp ruleset: wizard named %v, %d spells", found, len(rs["spells"].([]any)))
	}
	id := e.do("POST", "/campaigns", map[string]string{"premise": "a lake town"}, "", "").json(t)["id"].(string)
	tok := e.do("POST", "/campaigns/"+id+"/players", map[string]string{"name": "Ann"}, "", "").json(t)["token"].(string)
	// a half-made sheet: the preview fills it and lists the choices
	r := e.do("POST", "/campaigns/"+id+"/characters/preview", map[string]any{"character": map[string]any{"name": "Vex", "class_key": "rogue", "species": "human"}, "fill": true}, tok, "")
	if r.code != 200 {
		t.Fatalf("preview %d %s", r.code, r.body)
	}
	pv := r.json(t)
	if len(pv["problems"].([]any)) != 0 || len(pv["choices"].([]any)) < 3 {
		t.Fatalf("preview: %s", r.body)
	}
	spec := pv["character"]
	r = e.do("POST", "/campaigns/"+id+"/characters", map[string]any{"character": spec}, tok, "")
	if r.code != 201 {
		t.Fatalf("save %d %s", r.code, r.body)
	}
	cv := r.json(t)
	b := cv["build"].(map[string]any)
	if b["class_key"] != "rogue" || len(b["pending_choices"].([]any)) != 0 || cv["hp"].(float64) < 8+cv["modifiers"].(map[string]any)["CON"].(float64) {
		t.Fatalf("saved build: %s", r.body)
	}
	// nothing to choose at level 1
	if r := e.do("POST", "/campaigns/"+id+"/me/choose", map[string]any{"key": "feat4", "picks": []string{"tough"}}, tok, ""); r.code != 422 {
		t.Fatalf("early feat %d %s", r.code, r.body)
	}
}

func TestDMConsoleRewindAndActions(t *testing.T) {
	e := newEnv(t, "")
	id := e.do("POST", "/campaigns", map[string]string{"premise": "a lake town"}, "", "").json(t)["id"].(string)
	tok := e.do("POST", "/campaigns/"+id+"/players", map[string]string{"name": "Ann"}, "", "").json(t)["token"].(string)
	if r := e.do("POST", "/campaigns/"+id+"/characters", map[string]any{"character": sheet}, tok, ""); r.code != 201 {
		t.Fatalf("sheet %d %s", r.code, r.body)
	}
	e.do("POST", "/campaigns/"+id+"/start", nil, tok, "")
	gold := func() float64 {
		return e.do("GET", "/campaigns/"+id, nil, tok, "").json(t)["party"].([]any)[0].(map[string]any)["gold"].(float64)
	}
	e.fake.On("interpret", llm.Calls(llm.Call("look", map[string]any{})))
	e.do("POST", "/campaigns/"+id+"/actions", map[string]string{"text": "I look around"}, tok, "")
	before := gold()
	lg := e.do("GET", "/campaigns/"+id+"/log?after=0", nil, tok, "").json(t)
	nEvents := len(lg["events"].([]any))
	// a console action: give gold-worth XP and an item
	r := e.do("POST", "/campaigns/"+id+"/dm", map[string]any{"kind": "award_xp", "amount": 50}, tok, "")
	if r.code != 200 {
		t.Fatalf("award %d %s", r.code, r.body)
	}
	pc := r.json(t)["campaign"].(map[string]any)["party"].([]any)[0].(map[string]any)
	r = e.do("POST", "/campaigns/"+id+"/dm", map[string]any{"kind": "give_item", "target": pc["id"], "item": "ring_of_protection"}, tok, "")
	if r.code != 200 || !strings.Contains(string(r.body), "receives") {
		t.Fatalf("give %d %s", r.code, r.body)
	}
	if r := e.do("POST", "/campaigns/"+id+"/dm", map[string]any{"kind": "end_fight"}, tok, ""); r.code != 409 {
		t.Fatalf("no fight to end: %d", r.code)
	}
	// a second turn, then rewind to before it
	e.fake.On("interpret", llm.Calls(llm.Call("look", map[string]any{})))
	e.do("POST", "/campaigns/"+id+"/actions", map[string]string{"text": "I look again"}, tok, "")
	h := e.do("GET", "/campaigns/"+id+"/dm/history", nil, tok, "").json(t)["snapshots"].([]any)
	if len(h) != 2 || !strings.Contains(h[0].(map[string]any)["label"].(string), "look again") {
		t.Fatalf("history %v", h)
	}
	seq := h[1].(map[string]any)["seq"]
	r = e.do("POST", "/campaigns/"+id+"/dm/rewind", map[string]any{"seq": seq}, tok, "")
	if r.code != 200 {
		t.Fatalf("rewind %d %s", r.code, r.body)
	}
	v := r.json(t)
	pc = v["party"].([]any)[0].(map[string]any)
	if pc["xp"].(float64) != 0 || len(pc["inventory"].([]any)) == 0 || gold() != before {
		t.Fatalf("rewound sheet %v", pc)
	}
	for _, it := range pc["inventory"].([]any) {
		if it.(map[string]any)["rarity"] != nil {
			t.Fatal("the ring given after the snapshot should be gone")
		}
	}
	lg = e.do("GET", "/campaigns/"+id+"/log?after=0", nil, tok, "").json(t)
	evs := lg["events"].([]any)
	last := evs[len(evs)-1].(map[string]any)
	if last["type"] != "rewound" || len(evs) > nEvents {
		t.Fatalf("log after rewind: %d events (had %d before the first turn), last %v", len(evs), nEvents, last["type"])
	}
	// the player can still act: the token survived
	e.fake.On("interpret", llm.Calls(llm.Call("look", map[string]any{})))
	if r := e.do("POST", "/campaigns/"+id+"/actions", map[string]string{"text": "I look once more"}, tok, ""); r.code != 200 {
		t.Fatalf("act after rewind %d %s", r.code, r.body)
	}
	if r := e.do("POST", "/campaigns/"+id+"/dm/rewind", map[string]any{"seq": 99}, tok, ""); r.code != 400 && r.code != 422 {
		t.Fatalf("unknown snapshot %d", r.code)
	}
}

func TestStartSagaCampaign(t *testing.T) {
	e := newEnv(t, "")
	meta := e.do("GET", "/meta", nil, "", "").json(t)
	sagas, _ := meta["sagas"].([]any)
	if len(sagas) == 0 || sagas[0].(map[string]any)["key"] != "long-dark" {
		t.Fatalf("meta sagas: %v", meta["sagas"])
	}
	r := e.do("POST", "/campaigns", map[string]string{"saga": "long-dark"}, "", "")
	if r.code != 201 {
		t.Fatalf("create saga %d %s", r.code, r.body)
	}
	v := r.json(t)
	if v["saga"] != "long-dark" || v["dread"] != true || v["setting"] != "pulp" {
		t.Fatalf("saga view: saga=%v dread=%v setting=%v", v["saga"], v["dread"], v["setting"])
	}
	id := v["id"].(string)
	j := e.do("POST", "/campaigns/"+id+"/players", map[string]string{"name": "Ann"}, "", "")
	tok := j.json(t)["token"].(string)
	if r := e.do("POST", "/campaigns/"+id+"/characters", map[string]any{"character": sheet}, tok, ""); r.code != 201 {
		t.Fatalf("sheet %d %s", r.code, r.body)
	}
	got := e.do("GET", "/campaigns/"+id, nil, tok, "").json(t)
	party := got["party"].([]any)
	st, ok := party[0].(map[string]any)["strain"].(map[string]any)
	if !ok || st["max"] != float64(6) {
		t.Fatalf("strain on the sheet: %v", party[0].(map[string]any)["strain"])
	}
	if r := e.do("POST", "/campaigns", map[string]string{"saga": "nope"}, "", ""); r.code < 400 {
		t.Fatalf("unknown saga should fail, got %d", r.code)
	}
}
