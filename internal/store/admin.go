package store

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Solvely-Colin/aidnd/internal/game"
)

// GM tools: whole-campaign backups, deletes and restores, and the origin
// world a restart begins from.

// notCampaignData are tables that a campaign delete leaves alone.
var notCampaignData = map[string]bool{"gm_backups": true}

// campaignTables lists every table holding a campaign's rows, with the
// column that names the campaign. Table names come from schema.sql.
func campaignTables() [][2]string {
	var out [][2]string
	for table, cols := range schemaColumns() {
		if notCampaignData[table] {
			continue
		}
		key := ""
		for _, c := range cols {
			if c[0] == "campaign_id" {
				key = "campaign_id"
			}
		}
		if table == "campaigns" {
			key = "id"
		}
		if key != "" {
			out = append(out, [2]string{table, key})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		// campaigns first, so a restore puts the parent row in before the rest
		if (out[i][0] == "campaigns") != (out[j][0] == "campaigns") {
			return out[i][0] == "campaigns"
		}
		return out[i][0] < out[j][0]
	})
	return out
}

// Dump is every row a campaign has, by table.
type Dump struct {
	CampaignID string                      `json:"campaign_id"`
	Title      string                      `json:"title"`
	Tables     map[string][]map[string]any `json:"tables"`
}

// DumpCampaign reads all of a campaign's rows. The rewind snapshots are
// left out: they are many copies of the world and only matter mid-game.
func (s *Store) DumpCampaign(ctx context.Context, id string) (*Dump, error) {
	d := &Dump{CampaignID: id, Tables: map[string][]map[string]any{}}
	for _, t := range campaignTables() {
		if t[0] == "snapshots" {
			continue
		}
		rows, err := s.db.QueryContext(ctx, fmt.Sprintf("SELECT * FROM %s WHERE %s = ?", t[0], t[1]), id)
		if err != nil {
			return nil, fmt.Errorf("dump %s: %w", t[0], err)
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				rows.Close()
				return nil, err
			}
			row := map[string]any{}
			for i, c := range cols {
				if b, ok := vals[i].([]byte); ok {
					vals[i] = string(b)
				}
				row[c] = vals[i]
			}
			d.Tables[t[0]] = append(d.Tables[t[0]], row)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	camp := d.Tables["campaigns"]
	if len(camp) == 0 {
		return nil, ErrNotFound
	}
	d.Title = asString(camp[0]["title"])
	return d, nil
}

// DeleteCampaign removes every row of a campaign in one transaction.
func (s *Store) DeleteCampaign(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, t := range campaignTables() {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE %s = ?", t[0], t[1]), id); err != nil {
			return fmt.Errorf("delete %s: %w", t[0], err)
		}
	}
	return tx.Commit()
}

// ErrExists is returned when restoring over a campaign that is still there.
var ErrExists = errors.New("campaign exists")

// RestoreDump writes a dump back. The campaign must not exist. Columns the
// schema no longer has are dropped; new ones take their defaults.
func (s *Store) RestoreDump(ctx context.Context, d *Dump) error {
	if d == nil || d.CampaignID == "" || len(d.Tables["campaigns"]) == 0 {
		return fmt.Errorf("empty backup")
	}
	var n int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM campaigns WHERE id = ?", d.CampaignID).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrExists
	}
	schema := schemaColumns()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, t := range campaignTables() {
		known := map[string]bool{}
		for _, c := range schema[t[0]] {
			known[c[0]] = true
		}
		for _, row := range d.Tables[t[0]] {
			if fmt.Sprint(row[t[1]]) != d.CampaignID {
				return fmt.Errorf("backup row in %s belongs to another campaign", t[0])
			}
			var cols []string
			var vals []any
			for c, v := range row {
				if known[c] {
					cols = append(cols, c)
					vals = append(vals, jsonValue(v))
				}
			}
			if err := insert(ctx, tx, t[0], cols, vals...); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// jsonValue turns a value decoded from JSON into one SQL accepts: whole
// numbers back to integers.
func jsonValue(v any) any {
	switch x := v.(type) {
	case float64:
		if x == float64(int64(x)) {
			return int64(x)
		}
	case json.Number:
		if n, err := x.Int64(); err == nil {
			return n
		}
		f, _ := x.Float64()
		return f
	case map[string]any, []any:
		b, _ := json.Marshal(x)
		return string(b)
	}
	return v
}

// Backup is a saved copy of a campaign, without its data.
type Backup struct {
	ID        int64  `json:"id"`
	SourceID  string `json:"campaign_id"`
	Title     string `json:"title"`
	Reason    string `json:"reason"`
	Size      int    `json:"size"`
	CreatedAt int64  `json:"created_at"`
}

// SaveBackup keeps a dump in gm_backups and returns its id.
func (s *Store) SaveBackup(ctx context.Context, d *Dump, reason string) (int64, error) {
	data, err := packDump(d)
	if err != nil {
		return 0, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, "INSERT INTO gm_backups (source_id, title, reason, data, created_at) VALUES (?,?,?,?,?)",
		d.CampaignID, d.Title, reason, data, s.now().Unix())
	if err != nil {
		return 0, fmt.Errorf("backup: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// Backups lists saved copies, newest first.
func (s *Store) Backups(ctx context.Context) ([]Backup, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, source_id, title, reason, length(data), created_at FROM gm_backups ORDER BY id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Backup{}
	for rows.Next() {
		var b Backup
		if err := rows.Scan(&b.ID, &b.SourceID, &b.Title, &b.Reason, &b.Size, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// LoadBackup returns a saved copy's dump.
func (s *Store) LoadBackup(ctx context.Context, id int64) (*Dump, error) {
	var data string
	err := s.db.QueryRowContext(ctx, "SELECT data FROM gm_backups WHERE id = ?", id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	d, err := unpackDump(data)
	if err != nil {
		return nil, fmt.Errorf("backup %d: %w", id, err)
	}
	return d, nil
}

// packDump stores a dump as gzipped JSON in base64: a long campaign's event
// log would otherwise be close to D1's 2 MB row limit.
func packDump(d *Dump) (string, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if err := json.NewEncoder(zw).Encode(d); err != nil {
		return "", err
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	return "gz:" + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func unpackDump(data string) (*Dump, error) {
	var r io.Reader = strings.NewReader(data)
	if rest, ok := strings.CutPrefix(data, "gz:"); ok {
		raw, err := base64.StdEncoding.DecodeString(rest)
		if err != nil {
			return nil, err
		}
		zr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		r = zr
	}
	dec := json.NewDecoder(r)
	dec.UseNumber()
	d := &Dump{}
	if err := dec.Decode(d); err != nil {
		return nil, err
	}
	return d, nil
}

// DeleteBackup removes a saved copy for good.
func (s *Store) DeleteBackup(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM gm_backups WHERE id = ?", id); err != nil {
		return err
	}
	return tx.Commit()
}

// SaveOrigin keeps w (a freshly written campaign) as the start a restart
// goes back to.
func (s *Store) SaveOrigin(ctx context.Context, w *game.World) error {
	var data []byte
	if err := w.Settled(func() (err error) { data, err = json.Marshal(w); return err }); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM origins WHERE campaign_id = ?", w.Campaign.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO origins (campaign_id, world, created_at) VALUES (?,?,?)",
		w.Campaign.ID, string(data), s.now().Unix()); err != nil {
		return fmt.Errorf("origin: %w", err)
	}
	return tx.Commit()
}

// Origin returns the campaign as it was written, or ErrNotFound for a
// campaign made before origins were kept.
func (s *Store) Origin(ctx context.Context, id string) (*game.World, error) {
	var data string
	err := s.db.QueryRowContext(ctx, "SELECT world FROM origins WHERE campaign_id = ?", id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	w := &game.World{}
	if err := json.Unmarshal([]byte(data), w); err != nil {
		return nil, fmt.Errorf("origin %s: %w", id, err)
	}
	return w, nil
}

// HasOrigin reports whether a campaign can be restarted from its origin.
func (s *Store) HasOrigin(ctx context.Context, id string) bool {
	var n int
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM origins WHERE campaign_id = ?", id).Scan(&n)
	return n > 0
}
