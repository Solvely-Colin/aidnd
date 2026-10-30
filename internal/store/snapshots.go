package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Solvely-Colin/aidnd/internal/game"
)

// KeepSnapshots is how many turns back the DM console can rewind.
const KeepSnapshots = 30

// Snapshot is a saved turn the console can rewind to (without the world).
type Snapshot struct {
	Seq       int64  `json:"seq"`
	EventID   int64  `json:"event_id"`
	Turn      int    `json:"turn"`
	Label     string `json:"label"`
	CreatedAt int64  `json:"created_at"`
}

// SaveSnapshot keeps w as it is now, before a turn changes it, tagged with
// the campaign's latest event so a rewind knows which events to undo.
func (s *Store) SaveSnapshot(ctx context.Context, w *game.World, label string) error {
	var data []byte
	if err := w.Settled(func() (err error) { data, err = json.Marshal(w); return err }); err != nil {
		return err
	}
	id := w.Campaign.ID
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO snapshots (campaign_id, seq, event_id, turn, label, world, created_at)
		SELECT ?, (SELECT COALESCE(MAX(seq), 0) + 1 FROM snapshots WHERE campaign_id = ?),
			(SELECT COALESCE(MAX(id), 0) FROM events WHERE campaign_id = ?), ?, ?, ?, ?`,
		id, id, id, w.Campaign.Turn, label, string(data), s.now().Unix()); err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM snapshots WHERE campaign_id = ? AND seq <= (SELECT MAX(seq) FROM snapshots WHERE campaign_id = ?) - ?`,
		id, id, KeepSnapshots); err != nil {
		return err
	}
	return tx.Commit()
}

// Snapshots lists a campaign's saved turns, newest first.
func (s *Store) Snapshots(ctx context.Context, campaignID string) ([]Snapshot, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT seq, event_id, turn, label, created_at FROM snapshots WHERE campaign_id = ? ORDER BY seq DESC`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Snapshot{}
	for rows.Next() {
		var sn Snapshot
		if err := rows.Scan(&sn.Seq, &sn.EventID, &sn.Turn, &sn.Label, &sn.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, sn)
	}
	return out, rows.Err()
}

// Rewind puts the campaign back as it was in snapshot seq: the world is
// restored, the events since are marked undone (hidden from the feed and
// from the model's context), and that snapshot and later ones are dropped.
// The returned world is the restored one; the caller appends a "rewound"
// event.
func (s *Store) Rewind(ctx context.Context, campaignID string, seq int64) (*game.World, *Snapshot, error) {
	var sn Snapshot
	var data string
	err := s.db.QueryRowContext(ctx, `SELECT seq, event_id, turn, label, created_at, world FROM snapshots WHERE campaign_id = ? AND seq = ?`, campaignID, seq).
		Scan(&sn.Seq, &sn.EventID, &sn.Turn, &sn.Label, &sn.CreatedAt, &data)
	if err != nil {
		return nil, nil, ErrNotFound
	}
	w := &game.World{}
	if err := json.Unmarshal([]byte(data), w); err != nil {
		return nil, nil, fmt.Errorf("snapshot %d: %w", seq, err)
	}
	if w.Campaign.ID != campaignID {
		return nil, nil, fmt.Errorf("snapshot %d belongs to another campaign", seq)
	}
	// Seats are not part of the story: whoever has joined since stays, and
	// tokens (hidden from JSON) come from the live table.
	cur, err := s.Load(ctx, campaignID)
	if err != nil {
		return nil, nil, err
	}
	then := map[string]*game.Player{}
	for _, p := range w.Players {
		then[p.ID] = p
	}
	for _, p := range cur.Players {
		if old := then[p.ID]; old != nil {
			p.CharacterID = old.CharacterID
		} else if w.Character(p.CharacterID) == nil {
			p.CharacterID = ""
		}
	}
	w.Players = cur.Players
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	if err := saveWorld(ctx, tx, w, s.now().Unix()); err != nil {
		return nil, nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE events SET undone = 1 WHERE campaign_id = ? AND id > ?`, campaignID, sn.EventID); err != nil {
		return nil, nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM snapshots WHERE campaign_id = ? AND seq >= ?`, campaignID, seq); err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	w.RefreshMagic()
	return w, &sn, nil
}
