package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
)

// doGM is do with the Worker's GM header.
func (e *env) doGM(method, path string, body any) resp {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, rd)
	req.Header.Set("Authorization", "Bearer "+e.key)
	req.Header.Set("X-Player-ID", "colin")
	req.Header.Set("X-GM", "1")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return resp{res.StatusCode, b}
}

func gmList(t *testing.T, e *env) (map[string]map[string]any, []any) {
	t.Helper()
	r := e.doGM("GET", "/gm/campaigns", nil)
	if r.code != 200 {
		t.Fatalf("gm list %d %s", r.code, r.body)
	}
	m := r.json(t)
	out := map[string]map[string]any{}
	for _, c := range m["campaigns"].([]any) {
		c := c.(map[string]any)
		out[c["id"].(string)] = c
	}
	return out, m["backups"].([]any)
}

func TestNoSeatLimit(t *testing.T) {
	e := newEnv(t, "k")
	id := e.do("POST", "/campaigns", map[string]string{"premise": "a lake town"}, "k", "").json(t)["id"].(string)
	for i := 1; i <= 9; i++ {
		if r := e.do("POST", "/campaigns/"+id+"/players", map[string]string{"name": fmt.Sprint("P", i)}, "k", fmt.Sprint("p", i)); r.code != 201 {
			t.Fatalf("join %d: %d %s", i, r.code, r.body)
		}
	}
	v := e.do("GET", "/campaigns/"+id, nil, "k", "p1").json(t)
	if n := len(v["players"].([]any)); n != 9 {
		t.Fatalf("players %d", n)
	}
}

func TestGMRestartDeleteRestore(t *testing.T) {
	e := newEnv(t, "k")
	id := e.do("POST", "/campaigns", map[string]string{"premise": "a lake town"}, "k", "").json(t)["id"].(string)
	for _, p := range []string{"ann", "ben", "cat"} {
		e.do("POST", "/campaigns/"+id+"/players", map[string]string{"name": p}, "k", p)
	}
	if r := e.do("POST", "/campaigns/"+id+"/characters", map[string]any{"character": sheet}, "k", "ann"); r.code != 201 {
		t.Fatalf("sheet %d %s", r.code, r.body)
	}

	// players can't reach the GM area, even with the key
	if r := e.do("GET", "/gm/campaigns", nil, "k", "ann"); r.code != 403 {
		t.Fatalf("player reached gm area: %d", r.code)
	}
	// nor without the key
	req, _ := http.NewRequest("GET", e.srv.URL+"/gm/campaigns", nil)
	req.Header.Set("X-GM", "1")
	if res, _ := http.DefaultClient.Do(req); res.StatusCode != 401 {
		t.Fatalf("gm header without key: %d", res.StatusCode)
	}

	// a seat can be freed in the lobby
	if r := e.doGM("DELETE", "/gm/campaigns/"+id+"/players/cat", nil); r.code != 200 {
		t.Fatalf("kick %d %s", r.code, r.body)
	}
	list, backups := gmList(t, e)
	c := list[id]
	if c == nil || len(c["players"].([]any)) != 2 || c["can_restart"] != true || len(backups) != 0 {
		t.Fatalf("gm view %v backups %v", c, backups)
	}
	if p := c["players"].([]any)[0].(map[string]any); p["character"] != "Aria" || p["level"] != float64(1) {
		t.Fatalf("player row %v", p)
	}

	// play a little, then restart
	e.do("POST", "/campaigns/"+id+"/characters", map[string]any{"character": sheet2}, "k", "ben")
	if r := e.do("POST", "/campaigns/"+id+"/start", nil, "k", "ann"); r.code != 200 {
		t.Fatalf("start %d %s", r.code, r.body)
	}
	if r := e.doGM("DELETE", "/gm/campaigns/"+id+"/players/ben", nil); r.code != 409 {
		t.Fatalf("kick mid-story should be refused: %d", r.code)
	}
	r := e.doGM("POST", "/gm/campaigns/"+id+"/restart", nil)
	if r.code != 200 {
		t.Fatalf("restart %d %s", r.code, r.body)
	}
	nid := r.json(t)["id"].(string)
	if nid == id {
		t.Fatal("restart kept the old id")
	}
	if r := e.do("GET", "/campaigns/"+id, nil, "k", "ann"); r.code != 404 {
		t.Fatalf("old campaign still there: %d", r.code)
	}
	v := e.do("GET", "/campaigns/"+nid, nil, "k", "ann").json(t)
	if v["status"] != "lobby" || len(v["players"].([]any)) != 2 || len(v["party"].([]any)) != 0 || v["you"].(map[string]any)["joined"] != true {
		t.Fatalf("restarted view status=%v players=%v party=%v", v["status"], v["players"], v["party"])
	}
	// the restarted story can be restarted again
	if r := e.doGM("POST", "/gm/campaigns/"+nid+"/restart", nil); r.code != 200 {
		t.Fatalf("second restart %d %s", r.code, r.body)
	}
	nid = e.doGM("GET", "/gm/campaigns", nil).json(t)["campaigns"].([]any)[0].(map[string]any)["id"].(string)

	// delete, then bring it back
	r = e.doGM("DELETE", "/gm/campaigns/"+nid, nil)
	if r.code != 200 {
		t.Fatalf("delete %d %s", r.code, r.body)
	}
	bid := int(r.json(t)["backup_id"].(float64))
	list, backups = gmList(t, e)
	if len(list) != 0 || len(backups) != 3 {
		t.Fatalf("after delete: %d campaigns, %d backups", len(list), len(backups))
	}
	if r := e.doGM("GET", fmt.Sprintf("/gm/backups/%d", bid), nil); r.code != 200 || r.json(t)["campaign_id"] != nid {
		t.Fatalf("download %d %s", r.code, r.body)
	}
	if r := e.doGM("POST", fmt.Sprintf("/gm/backups/%d/restore", bid), nil); r.code != 200 {
		t.Fatalf("restore %d %s", r.code, r.body)
	}
	v = e.do("GET", "/campaigns/"+nid, nil, "k", "ben").json(t)
	if v["status"] != "lobby" || len(v["players"].([]any)) != 2 {
		t.Fatalf("restored view %v", v)
	}
	if r := e.doGM("POST", fmt.Sprintf("/gm/backups/%d/restore", bid), nil); r.code != 409 {
		t.Fatalf("restore over a live campaign: %d", r.code)
	}
	// the restored story keeps its start
	if list, _ := gmList(t, e); list[nid]["can_restart"] != true {
		t.Fatal("restored story lost its restart")
	}
	if r := e.doGM("DELETE", fmt.Sprintf("/gm/backups/%d", bid), nil); r.code != 200 {
		t.Fatalf("forget %d", r.code)
	}
	if _, backups := gmList(t, e); len(backups) != 2 {
		t.Fatalf("backups after forget %d", len(backups))
	}
}

func TestGMRestartOlderCampaigns(t *testing.T) {
	e := newEnv(t, "k")
	authored := e.do("POST", "/campaigns", map[string]string{"premise": "x"}, "k", "").json(t)["id"].(string)
	sg := e.do("POST", "/campaigns", map[string]string{"saga": "long-dark"}, "k", "").json(t)["id"].(string)
	// campaigns made before origins were kept
	if _, err := e.api.store.DB().Exec("DELETE FROM origins"); err != nil {
		t.Fatal(err)
	}
	list, _ := gmList(t, e)
	if list[authored]["can_restart"] != false || list[sg]["can_restart"] != true {
		t.Fatalf("can_restart authored=%v saga=%v", list[authored]["can_restart"], list[sg]["can_restart"])
	}
	if r := e.doGM("POST", "/gm/campaigns/"+authored+"/restart", nil); r.code != 409 {
		t.Fatalf("restart without origin: %d %s", r.code, r.body)
	}
	r := e.doGM("POST", "/gm/campaigns/"+sg+"/restart", nil)
	if r.code != 200 {
		t.Fatalf("saga restart %d %s", r.code, r.body)
	}
	v := e.do("GET", "/campaigns/"+r.json(t)["id"].(string), nil, "k", "").json(t)
	if v["saga"] != "long-dark" || v["status"] != "lobby" {
		t.Fatalf("saga restart view saga=%v status=%v", v["saga"], v["status"])
	}
}
