package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

// Note is a player's own journal entry, stamped with the story time it was
// written at. Notes live outside the world so the engine and the model
// never see them.
type Note struct {
	ID        string `json:"id"`
	PlayerID  string `json:"-"`
	Text      string `json:"text"`
	Turn      int    `json:"turn"`
	Day       int    `json:"day"`
	Time      string `json:"time"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// MaxNotes is how many notes one player may keep in a campaign.
const MaxNotes = 200

// Notes lists a player's notes in a campaign, oldest first.
func (s *Store) Notes(ctx context.Context, campaignID, playerID string) ([]Note, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, player_id, text, turn, day, time, created_at, updated_at FROM notes
		WHERE campaign_id = ? AND player_id = ? ORDER BY created_at, id`, campaignID, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Note{}
	for rows.Next() {
		var n Note
		if err := rows.Scan(&n.ID, &n.PlayerID, &n.Text, &n.Turn, &n.Day, &n.Time, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// AddNote stores a new note and returns it with its id and times set.
func (s *Store) AddNote(ctx context.Context, campaignID string, n Note) (Note, error) {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	n.ID = hex.EncodeToString(b)
	n.CreatedAt = s.now().Unix()
	n.UpdatedAt = n.CreatedAt
	err := insert(ctx, s.db, "notes", []string{"campaign_id", "id", "player_id", "text", "turn", "day", "time", "created_at", "updated_at"},
		campaignID, n.ID, n.PlayerID, n.Text, n.Turn, n.Day, n.Time, n.CreatedAt, n.UpdatedAt)
	return n, err
}

// UpdateNote rewrites a note's text. Only the note's author can change it.
func (s *Store) UpdateNote(ctx context.Context, campaignID, playerID, id, text string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE notes SET text = ?, updated_at = ? WHERE campaign_id = ? AND player_id = ? AND id = ?`,
		text, s.now().Unix(), campaignID, playerID, id)
	return found(res, err)
}

// DeleteNote removes one of a player's notes.
func (s *Store) DeleteNote(ctx context.Context, campaignID, playerID, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM notes WHERE campaign_id = ? AND player_id = ? AND id = ?`, campaignID, playerID, id)
	return found(res, err)
}

func found(res interface{ RowsAffected() (int64, error) }, err error) error {
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}
