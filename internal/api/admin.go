package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/Solvely-Colin/aidnd/internal/store"
)

// The GM area: routes only the person running the game may call. In
// service mode the Worker sends X-GM: 1 for a GM's session (and strips the
// header from everyone else's requests); the API key proves it came from
// the Worker. In open dev mode the header alone is enough.

func (s *Server) gmOnly(h http.HandlerFunc) http.HandlerFunc {
	return s.auth(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-GM") != "1" {
			writeError(w, http.StatusForbidden, "only the GM can do that")
			return
		}
		h(w, r)
	})
}

// GMCampaign is one campaign as the GM sees it.
type GMCampaign struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Status     string     `json:"status"`
	Saga       string     `json:"saga,omitempty"`
	Turn       int        `json:"turn"`
	CreatedAt  int64      `json:"created_at"`
	UpdatedAt  int64      `json:"updated_at"`
	CanRestart bool       `json:"can_restart"`
	Players    []GMPlayer `json:"players"`
	Error      string     `json:"error,omitempty"`
}

// GMPlayer is a seat and the character in it.
type GMPlayer struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Character string `json:"character,omitempty"`
	Class     string `json:"class,omitempty"`
	Level     int    `json:"level,omitempty"`
	HP        int    `json:"hp,omitempty"`
	MaxHP     int    `json:"max_hp,omitempty"`
	Dead      bool   `json:"dead,omitempty"`
}

func (s *Server) handleGMCampaigns(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListCampaigns(r.Context(), "")
	if err != nil {
		s.fail(w, err)
		return
	}
	out := make([]GMCampaign, 0, len(list))
	for _, cs := range list {
		gc := GMCampaign{ID: cs.ID, Title: cs.Title, Status: cs.Status, UpdatedAt: cs.UpdatedAt, Players: []GMPlayer{}}
		wld, err := s.store.Load(r.Context(), cs.ID)
		if err != nil {
			// still listed, so a broken campaign can be deleted
			gc.Error = err.Error()
			out = append(out, gc)
			continue
		}
		c := wld.Campaign
		gc.Saga, gc.Turn, gc.CreatedAt = c.Saga, c.Turn, c.CreatedAt
		gc.CanRestart = s.gm.CanRestart(r.Context(), wld)
		for _, p := range wld.Players {
			gp := GMPlayer{ID: p.ID, Name: p.Name}
			if ch := wld.CharacterOf(p.ID); ch != nil {
				gp.Character, gp.Class, gp.Level, gp.HP, gp.MaxHP, gp.Dead = ch.Name, ch.Class, ch.Level, ch.HP, ch.MaxHP, ch.Dead
			}
			gc.Players = append(gc.Players, gp)
		}
		out = append(out, gc)
	}
	backups, err := s.store.Backups(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"campaigns": out, "backups": backups})
}

func (s *Server) handleGMRestart(w http.ResponseWriter, r *http.Request) {
	wld, backup, err := s.gm.Restart(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"id": wld.Campaign.ID, "title": wld.Campaign.Title, "backup_id": backup})
}

func (s *Server) handleGMDelete(w http.ResponseWriter, r *http.Request) {
	backup, err := s.gm.Delete(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "backup_id": backup})
}

func (s *Server) handleGMRemovePlayer(w http.ResponseWriter, r *http.Request) {
	if err := s.gm.RemovePlayer(r.Context(), r.PathValue("id"), r.PathValue("pid")); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func backupID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("bid"), 10, 64)
	if err != nil {
		return 0, store.ErrNotFound
	}
	return id, nil
}

func (s *Server) handleGMDownloadBackup(w http.ResponseWriter, r *http.Request) {
	id, err := backupID(r)
	if err == nil {
		var d *store.Dump
		if d, err = s.store.LoadBackup(r.Context(), id); err == nil {
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="aidnd-backup-%d-%s.json"`, id, d.CampaignID))
			writeJSON(w, 200, d)
			return
		}
	}
	s.fail(w, err)
}

func (s *Server) handleGMRestoreBackup(w http.ResponseWriter, r *http.Request) {
	id, err := backupID(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	cid, err := s.gm.RestoreBackup(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"id": cid})
}

func (s *Server) handleGMDeleteBackup(w http.ResponseWriter, r *http.Request) {
	id, err := backupID(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	if err := s.store.DeleteBackup(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
