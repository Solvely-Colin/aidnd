package api

import (
	"net/http"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/gm"
)

// The DM console's endpoints. Any seated player can use them.

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	p, ok := s.needPlayer(w, r)
	if !ok {
		return
	}
	list, err := s.gm.History(r.Context(), r.PathValue("id"), p)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"snapshots": list})
}

func (s *Server) handleRewind(w http.ResponseWriter, r *http.Request) {
	p, ok := s.needPlayer(w, r)
	if !ok {
		return
	}
	var body struct {
		Seq int64 `json:"seq"`
	}
	if err := decode(r, &body); err != nil {
		s.fail(w, err)
		return
	}
	wld, err := s.gm.Rewind(r.Context(), r.PathValue("id"), p, body.Seq)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, campaignView(wld, p))
}

func (s *Server) handleDM(w http.ResponseWriter, r *http.Request) {
	p, ok := s.needPlayer(w, r)
	if !ok {
		return
	}
	var body gm.DMAction
	if err := decode(r, &body); err != nil {
		s.fail(w, err)
		return
	}
	wld, text, err := s.gm.DM(r.Context(), r.PathValue("id"), p, body)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"text": text, "campaign": campaignView(wld, p)})
}

// magicView is the magic item list for the console, named for setting.
func magicView(setting string) map[string]any {
	type tmpl struct {
		Key    string `json:"key"`
		Name   string `json:"name"` // pattern with %s for the base item
		For    string `json:"for"`
		Rarity string `json:"rarity"`
	}
	type wond struct {
		Key    string `json:"key"`
		Name   string `json:"name"`
		Kind   string `json:"kind"`
		Rarity string `json:"rarity"`
		Desc   string `json:"desc"`
	}
	ts := []tmpl{}
	for _, t := range game.MagicTemplates {
		ts = append(ts, tmpl{Key: t.Key, Name: game.MagicTemplateName(t, setting), For: t.For, Rarity: t.Rarity})
	}
	ws := []wond{}
	for _, k := range game.WondrousKeys() {
		d := game.Wondrous[k]
		ws = append(ws, wond{Key: k, Name: game.WondrousName(d, setting), Kind: d.Kind, Rarity: d.Rarity, Desc: d.Desc})
	}
	return map[string]any{"templates": ts, "wondrous": ws, "console": gm.DMKinds}
}
