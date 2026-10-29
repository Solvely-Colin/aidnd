// Package api is the HTTP layer: routing (net/http 1.22 patterns), the two
// auth modes, player-safe views and the SSE stream.
package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/gm"
	"github.com/Solvely-Colin/aidnd/internal/store"
)

// Config for the HTTP layer.
type Config struct {
	APIKey      string   // service mode when set
	CORSOrigins []string // "*" or exact origins
	DevUI       http.Handler
	StreamTTL   time.Duration
}

// Server wires handlers to the GM.
type Server struct {
	gm     *gm.GM
	store  *store.Store
	hub    *Hub
	cfg    Config
	secret []byte
	log    *log.Logger
	now    func() time.Time
}

// New builds the server and hooks the GM's publish to the SSE hub.
func New(g *gm.GM, cfg Config) *Server {
	if cfg.StreamTTL == 0 {
		cfg.StreamTTL = 10 * time.Minute
	}
	s := &Server{gm: g, store: g.Store, hub: NewHub(), cfg: cfg, log: g.Log, now: time.Now}
	if cfg.APIKey != "" {
		s.secret = []byte("stream:" + cfg.APIKey)
	} else {
		s.secret = make([]byte, 32)
		_, _ = rand.Read(s.secret)
	}
	g.Publish = s.hub.Publish
	return s
}

// Hub exposes the SSE hub.
func (s *Server) Hub() *Hub { return s.hub }

// ServiceMode reports whether an API key is required.
func (s *Server) ServiceMode() bool { return s.cfg.APIKey != "" }

// Handler returns the routed handler with CORS.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]any{"ok": true}) })
	mux.HandleFunc("GET /meta", s.handleMeta)
	mux.HandleFunc("GET /me/campaigns", s.auth(s.handleMyCampaigns))
	mux.HandleFunc("GET /campaigns", s.auth(s.handleListCampaigns))
	mux.HandleFunc("POST /campaigns", s.auth(s.handleCreateCampaign))
	mux.HandleFunc("GET /campaigns/{id}", s.auth(s.handleGetCampaign))
	mux.HandleFunc("POST /campaigns/{id}/players", s.auth(s.handleJoin))
	mux.HandleFunc("POST /campaigns/{id}/characters/draft", s.auth(s.handleDraft))
	mux.HandleFunc("POST /campaigns/{id}/characters", s.auth(s.handleSaveCharacter))
	mux.HandleFunc("GET /campaigns/{id}/characters/{cid}", s.auth(s.handleGetCharacter))
	mux.HandleFunc("POST /campaigns/{id}/start", s.auth(s.handleStart))
	mux.HandleFunc("POST /campaigns/{id}/actions", s.auth(s.handleAction))
	mux.HandleFunc("POST /campaigns/{id}/skip", s.auth(s.handleSkip))
	mux.HandleFunc("GET /campaigns/{id}/map", s.auth(s.handleMap))
	mux.HandleFunc("GET /campaigns/{id}/locations/{lid}", s.auth(s.handleLocation))
	mux.HandleFunc("GET /campaigns/{id}/npcs/{nid}", s.auth(s.handleNPC))
	mux.HandleFunc("GET /campaigns/{id}/quests", s.auth(s.handleQuests))
	mux.HandleFunc("GET /campaigns/{id}/journal", s.auth(s.handleJournal))
	mux.HandleFunc("POST /campaigns/{id}/stream-token", s.auth(s.handleStreamToken))
	mux.HandleFunc("GET /campaigns/{id}/events", s.handleEvents)
	mux.HandleFunc("GET /campaigns/{id}/log", s.auth(s.handleLog))
	if s.cfg.DevUI != nil {
		mux.Handle("GET /dev/", http.StripPrefix("/dev", s.cfg.DevUI))
		mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/dev/", http.StatusFound) })
	}
	return s.cors(mux)
}

// --- plumbing

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	var p game.Problems
	switch {
	case errors.As(err, &p):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "validation failed", "problems": []string(p)})
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, gm.ErrNotYourTurn):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, gm.ErrConflict):
		writeError(w, http.StatusConflict, strings.TrimPrefix(err.Error(), "conflict: "))
	case errors.Is(err, gm.ErrForbidden):
		writeError(w, http.StatusForbidden, "join this campaign first")
	case errors.Is(err, gm.ErrBadRequest):
		writeError(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), "bad request: "))
	case errors.Is(err, context.Canceled):
		writeError(w, 499, "request cancelled")
	default:
		if s.log != nil {
			s.log.Printf("internal error: %v", err)
		}
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func decode(r *http.Request, v any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		return fmt.Errorf("%w: %v", gm.ErrBadRequest, err)
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("%w: invalid JSON: %v", gm.ErrBadRequest, err)
	}
	return nil
}

func (s *Server) cors(next http.Handler) http.Handler {
	allowed := map[string]bool{}
	any := false
	for _, o := range s.cfg.CORSOrigins {
		o = strings.TrimSpace(o)
		if o == "*" {
			any = true
		} else if o != "" {
			allowed[o] = true
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" && (any || allowed[origin]) {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Vary", "Origin")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Player-ID, Last-Event-ID")
			h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// --- auth

type who struct {
	PlayerID string
	Service  bool // authenticated with the API key
}

type whoKey struct{}

func whoFrom(r *http.Request) who { v, _ := r.Context().Value(whoKey{}).(who); return v }

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// auth resolves the caller. Service mode: Bearer API_KEY plus X-Player-ID.
// Open mode: a per-player bearer token from join, or X-Player-ID (dev).
func (s *Server) auth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var c who
		tok := bearer(r)
		if s.ServiceMode() {
			if subtle.ConstantTimeCompare([]byte(tok), []byte(s.cfg.APIKey)) != 1 {
				writeError(w, http.StatusUnauthorized, "missing or invalid API key")
				return
			}
			c = who{PlayerID: strings.TrimSpace(r.Header.Get("X-Player-ID")), Service: true}
		} else {
			c.PlayerID = strings.TrimSpace(r.Header.Get("X-Player-ID"))
			if tok != "" {
				id := r.PathValue("id")
				if id == "" {
					writeError(w, http.StatusUnauthorized, "player tokens are per campaign")
					return
				}
				wld, err := s.store.Load(r.Context(), id)
				if err != nil {
					s.fail(w, err)
					return
				}
				found := false
				for _, p := range wld.Players {
					if p.Token != "" && subtle.ConstantTimeCompare([]byte(p.Token), []byte(tok)) == 1 {
						c.PlayerID, found = p.ID, true
					}
				}
				if !found {
					writeError(w, http.StatusUnauthorized, "invalid player token")
					return
				}
			}
		}
		if len(c.PlayerID) > 128 {
			writeError(w, http.StatusBadRequest, "player id too long")
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), whoKey{}, c)))
	}
}

func (s *Server) needPlayer(w http.ResponseWriter, r *http.Request) (string, bool) {
	p := whoFrom(r).PlayerID
	if p == "" {
		if s.ServiceMode() {
			writeError(w, http.StatusBadRequest, "X-Player-ID header is required")
		} else {
			writeError(w, http.StatusUnauthorized, "send your player token (Authorization: Bearer ...) or X-Player-ID")
		}
		return "", false
	}
	return p, true
}

func (s *Server) mintStreamToken(campaignID, playerID string, exp time.Time) string {
	payload := campaignID + "|" + playerID + "|" + strconv.FormatInt(exp.Unix(), 10)
	m := hmac.New(sha256.New, s.secret)
	m.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func (s *Server) checkStreamToken(tok, campaignID string) (string, bool) {
	parts := strings.Split(tok, ".")
	if len(parts) != 2 {
		return "", false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", false
	}
	m := hmac.New(sha256.New, s.secret)
	m.Write(payload)
	if !hmac.Equal(sig, m.Sum(nil)) {
		return "", false
	}
	f := strings.Split(string(payload), "|")
	if len(f) != 3 || f[0] != campaignID {
		return "", false
	}
	exp, err := strconv.ParseInt(f[2], 10, 64)
	if err != nil || s.now().Unix() > exp {
		return "", false
	}
	return f[1], true
}

// streamAuth: browsers can't set headers on EventSource, so the stream
// accepts ?stream_token= (both modes), or the normal header auth.
func (s *Server) streamAuth(w http.ResponseWriter, r *http.Request, campaignID string) (string, bool) {
	if t := r.URL.Query().Get("stream_token"); t != "" {
		if p, ok := s.checkStreamToken(t, campaignID); ok {
			return p, true
		}
		writeError(w, http.StatusUnauthorized, "invalid or expired stream token")
		return "", false
	}
	if s.ServiceMode() {
		if subtle.ConstantTimeCompare([]byte(bearer(r)), []byte(s.cfg.APIKey)) != 1 {
			writeError(w, http.StatusUnauthorized, "stream_token required")
			return "", false
		}
	}
	if _, err := s.store.Load(r.Context(), campaignID); err != nil {
		s.fail(w, err)
		return "", false
	}
	return r.Header.Get("X-Player-ID"), true
}

// --- handlers

func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request) {
	kits := []map[string]any{}
	for _, k := range game.KitKeys() {
		kit := game.Kits[k]
		var items []game.ItemDef
		for _, key := range kit.Items {
			items = append(items, game.Catalog[key])
		}
		kits = append(kits, map[string]any{"key": kit.Key, "name": kit.Name, "desc": kit.Desc, "gold": kit.Gold, "weapon": kit.Weapon, "armor": kit.Armor, "items": items, "suggested": kit.Suggested})
	}
	catalog := []game.ItemDef{}
	for _, k := range game.CatalogKeys() {
		catalog = append(catalog, game.Catalog[k])
	}
	writeJSON(w, 200, map[string]any{
		"abilities": game.Abilities, "standard_array": game.StandardArray, "proficiencies": 2, "kits": kits,
		"xp_table": game.XPTable, "max_level": game.MaxLevel, "dcs": []int{5, 10, 15, 20, 25}, "catalog": catalog,
		"service_mode": s.ServiceMode(),
	})
}

func (s *Server) handleListCampaigns(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListCampaigns(r.Context(), "")
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"campaigns": list})
}

func (s *Server) handleMyCampaigns(w http.ResponseWriter, r *http.Request) {
	p, ok := s.needPlayer(w, r)
	if !ok {
		return
	}
	list, err := s.store.ListCampaigns(r.Context(), p)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"campaigns": list})
}

func (s *Server) handleCreateCampaign(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Premise string `json:"premise"`
	}
	if err := decode(r, &body); err != nil {
		s.fail(w, err)
		return
	}
	wld, err := s.gm.CreateCampaign(r.Context(), body.Premise)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, campaignView(wld, whoFrom(r).PlayerID))
}

func (s *Server) handleGetCampaign(w http.ResponseWriter, r *http.Request) {
	wld, err := s.store.Load(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, campaignView(wld, whoFrom(r).PlayerID))
}

func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decode(r, &body); err != nil {
		s.fail(w, err)
		return
	}
	pid := whoFrom(r).PlayerID
	if pid == "" {
		if s.ServiceMode() {
			writeError(w, http.StatusBadRequest, "X-Player-ID header is required")
			return
		}
		b := make([]byte, 6)
		_, _ = rand.Read(b)
		pid = "p" + base64.RawURLEncoding.EncodeToString(b)
	}
	p, created, err := s.gm.Join(r.Context(), r.PathValue("id"), pid, body.Name)
	if err != nil {
		s.fail(w, err)
		return
	}
	resp := map[string]any{"player": PlayerView{ID: p.ID, Name: p.Name, CharacterID: p.CharacterID}, "created": created}
	if !s.ServiceMode() {
		resp["token"] = p.Token
	}
	code := http.StatusOK
	if created {
		code = http.StatusCreated
	}
	writeJSON(w, code, resp)
}

func (s *Server) handleDraft(w http.ResponseWriter, r *http.Request) {
	p, ok := s.needPlayer(w, r)
	if !ok {
		return
	}
	var body struct {
		Concept string `json:"concept"`
	}
	if err := decode(r, &body); err != nil {
		s.fail(w, err)
		return
	}
	wld, err := s.store.Load(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	if wld.Player(p) == nil {
		s.fail(w, gm.ErrForbidden)
		return
	}
	writeJSON(w, 200, map[string]any{"character": s.gm.DraftCharacter(r.Context(), wld, body.Concept)})
}

func (s *Server) handleSaveCharacter(w http.ResponseWriter, r *http.Request) {
	p, ok := s.needPlayer(w, r)
	if !ok {
		return
	}
	var body struct {
		Character *game.CharacterSpec `json:"character"`
		Concept   string              `json:"concept"`
	}
	if err := decode(r, &body); err != nil {
		s.fail(w, err)
		return
	}
	id := r.PathValue("id")
	var c *game.Character
	var err error
	switch {
	case body.Character != nil:
		c, err = s.gm.SaveCharacter(r.Context(), id, p, *body.Character)
	case strings.TrimSpace(body.Concept) != "":
		c, err = s.gm.CreateCharacterFromConcept(r.Context(), id, p, body.Concept)
	default:
		err = fmt.Errorf("%w: send {character: {...}} or {concept: \"...\"}", gm.ErrBadRequest)
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	wld, err := s.store.Load(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, characterView(wld, wld.Character(c.ID)))
}

func (s *Server) handleGetCharacter(w http.ResponseWriter, r *http.Request) {
	wld, err := s.store.Load(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	c := wld.Character(r.PathValue("cid"))
	if c == nil || c.ID != r.PathValue("cid") {
		s.fail(w, store.ErrNotFound)
		return
	}
	writeJSON(w, 200, characterView(wld, c))
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	c := whoFrom(r)
	id := r.PathValue("id")
	if !c.Service {
		wld, err := s.store.Load(r.Context(), id)
		if err != nil {
			s.fail(w, err)
			return
		}
		if wld.Player(c.PlayerID) == nil {
			s.fail(w, gm.ErrForbidden)
			return
		}
	}
	wld, opening, err := s.gm.Start(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"campaign": campaignView(wld, c.PlayerID), "narration": opening})
}

func (s *Server) handleAction(w http.ResponseWriter, r *http.Request) {
	p, ok := s.needPlayer(w, r)
	if !ok {
		return
	}
	var body struct {
		Text     string `json:"text"`
		TargetID string `json:"target_id"`
		ZoneID   string `json:"zone_id"`
	}
	if err := decode(r, &body); err != nil {
		s.fail(w, err)
		return
	}
	res, err := s.gm.TakeAction(r.Context(), r.PathValue("id"), p, body.Text, gm.Hints{TargetID: body.TargetID, ZoneID: body.ZoneID})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) handleSkip(w http.ResponseWriter, r *http.Request) {
	c := whoFrom(r)
	var body struct {
		Force bool `json:"force"`
	}
	if err := decode(r, &body); err != nil {
		s.fail(w, err)
		return
	}
	mode := gm.SkipAsPlayer
	if c.Service && (body.Force || c.PlayerID == "") {
		mode = gm.SkipAsService
	} else if c.PlayerID == "" {
		s.needPlayer(w, r)
		return
	}
	res, err := s.gm.Skip(r.Context(), r.PathValue("id"), c.PlayerID, mode)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) handleMap(w http.ResponseWriter, r *http.Request) {
	wld, err := s.store.Load(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, mapView(wld))
}

func (s *Server) handleLocation(w http.ResponseWriter, r *http.Request) {
	wld, err := s.store.Load(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	l := wld.Location(r.PathValue("lid"))
	if l == nil || l.ID != r.PathValue("lid") || !locationKnown(wld, l) {
		s.fail(w, store.ErrNotFound)
		return
	}
	writeJSON(w, 200, locationView(wld, l, true))
}

func (s *Server) handleNPC(w http.ResponseWriter, r *http.Request) {
	wld, err := s.store.Load(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	n := wld.NPC(r.PathValue("nid"))
	if n == nil || n.ID != r.PathValue("nid") {
		s.fail(w, store.ErrNotFound)
		return
	}
	v, ok := npcView(wld, n)
	if !ok {
		s.fail(w, store.ErrNotFound)
		return
	}
	writeJSON(w, 200, v)
}

func (s *Server) handleQuests(w http.ResponseWriter, r *http.Request) {
	wld, err := s.store.Load(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"quests": questsView(wld)})
}

func (s *Server) handleJournal(w http.ResponseWriter, r *http.Request) {
	wld, err := s.store.Load(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	j := JournalView{Summary: wld.Campaign.Summary, Chronicle: wld.Chronicle, Quests: questsView(wld), Setbacks: wld.Campaign.Setbacks}
	if j.Chronicle == nil {
		j.Chronicle = []game.ChronicleEntry{}
	}
	writeJSON(w, 200, j)
}

func (s *Server) handleStreamToken(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.store.Load(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	exp := s.now().Add(s.cfg.StreamTTL)
	writeJSON(w, 200, map[string]any{"token": s.mintStreamToken(id, whoFrom(r).PlayerID, exp), "expires_at": exp.Unix()})
}

func (s *Server) handleLog(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.store.Load(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	evs, err := s.store.Events(r.Context(), id, after, limit)
	if err != nil {
		s.fail(w, err)
		return
	}
	var last int64 = after
	if len(evs) > 0 {
		last = evs[len(evs)-1].ID
	}
	writeJSON(w, 200, map[string]any{"events": evs, "last_id": last})
}
