package store

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestBackupDeleteRestore(t *testing.T) {
	s, err := openMem(t)
	must(t, err)
	defer s.Close()
	w := richWorld(t)
	_, err = s.Save(ctx, w, NewEvent("narration", "", map[string]any{"text": "It begins.", "n": 3}))
	must(t, err)
	must(t, s.SaveSnapshot(ctx, w, "turn 1"))
	must(t, s.SaveOrigin(ctx, w))
	_, err = s.AddNote(ctx, w.Campaign.ID, Note{PlayerID: "p1", Text: "remember the bell"})
	must(t, err)
	before, err := s.Load(ctx, w.Campaign.ID)
	must(t, err)

	d, err := s.DumpCampaign(ctx, w.Campaign.ID)
	must(t, err)
	if d.Title != w.Campaign.Title || len(d.Tables["events"]) != 1 || len(d.Tables["snapshots"]) != 0 || len(d.Tables["notes"]) != 1 {
		t.Fatalf("dump title=%q events=%d snapshots=%d notes=%d", d.Title, len(d.Tables["events"]), len(d.Tables["snapshots"]), len(d.Tables["notes"]))
	}
	bid, err := s.SaveBackup(ctx, d, "delete")
	must(t, err)
	must(t, s.DeleteCampaign(ctx, w.Campaign.ID))
	for _, tb := range campaignTables() {
		var n int
		must(t, s.db.QueryRow("SELECT COUNT(*) FROM "+tb[0]+" WHERE "+tb[1]+" = ?", w.Campaign.ID).Scan(&n))
		if n != 0 {
			t.Errorf("%s still has %d rows", tb[0], n)
		}
	}
	if _, err := s.Load(ctx, w.Campaign.ID); err != ErrNotFound {
		t.Fatalf("load after delete: %v", err)
	}
	list, err := s.Backups(ctx)
	must(t, err)
	if len(list) != 1 || list[0].ID != bid || list[0].SourceID != w.Campaign.ID || list[0].Size == 0 {
		t.Fatalf("backups %+v", list)
	}

	back, err := s.LoadBackup(ctx, bid)
	must(t, err)
	must(t, s.RestoreDump(ctx, back))
	if err := s.RestoreDump(ctx, back); err != ErrExists {
		t.Fatalf("second restore: %v", err)
	}
	after, err := s.Load(ctx, w.Campaign.ID)
	must(t, err)
	before.Campaign.CreatedAt, after.Campaign.CreatedAt = 0, 0
	if !reflect.DeepEqual(canon(t, before), canon(t, after)) {
		a, _ := json.Marshal(canon(t, before))
		b, _ := json.Marshal(canon(t, after))
		t.Fatalf("restore differs\nwant %s\ngot  %s", a, b)
	}
	if after.Players[0].Token != before.Players[0].Token {
		t.Fatal("token lost")
	}
	evs, err := s.Events(ctx, w.Campaign.ID, 0, 10)
	must(t, err)
	if len(evs) != 1 || evs[0].Type != "narration" || string(evs[0].Data) != `{"n":3,"text":"It begins."}` {
		t.Fatalf("events %+v", evs)
	}
	if !s.HasOrigin(ctx, w.Campaign.ID) {
		t.Fatal("origin not restored")
	}
	o, err := s.Origin(ctx, w.Campaign.ID)
	must(t, err)
	if o.Campaign.Title != w.Campaign.Title || len(o.Locations) != len(w.Locations) {
		t.Fatalf("origin %q %d", o.Campaign.Title, len(o.Locations))
	}
	must(t, s.DeleteBackup(ctx, bid))
	if _, err := s.LoadBackup(ctx, bid); err != ErrNotFound {
		t.Fatalf("forgotten backup: %v", err)
	}
}

func TestBackupOnD1(t *testing.T) {
	saved := openMem
	defer func() { openMem = saved }()
	openMem = func(t *testing.T) (*Store, error) { s, _ := openD1(t); return s, nil }
	TestBackupDeleteRestore(t)
}
