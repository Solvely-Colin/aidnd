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
	"github.com/Solvely-Colin/aidnd/internal/saga"
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
	turnDeadline = g.TurnDeadline
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
	mux.HandleFunc("POST /tick", s.auth(s.handleTick))
	mux.HandleFunc("POST /campaigns/{id}/me/downtime", s.auth(s.handleDowntime))
	mux.HandleFunc("POST /campaigns/{id}/me/seen", s.auth(s.handleSeen))
	mux.HandleFunc("POST /campaigns/{id}/me/solo", s.auth(s.handleSolo))
	mux.HandleFunc("POST /campaigns/{id}/me/goals", s.auth(s.handleGoal))
	mux.HandleFunc("POST /campaigns/{id}/me/join", s.auth(s.handleJoinGroup))
	mux.HandleFunc("POST /campaigns/{id}/me/answer", s.auth(s.handleAnswer))
	mux.HandleFunc("POST /campaigns/{id}/me/begin", s.auth(s.handleBegin))
	mux.HandleFunc("POST /campaigns/{id}/roll", s.auth(s.handleRoll))
	mux.HandleFunc("POST /campaigns/{id}/roll/cancel", s.auth(s.handleCancelRoll))
	mux.HandleFunc("POST /campaigns/{id}/me/dice", s.auth(s.handleDice))
	mux.HandleFunc("POST /campaigns/{id}/characters/preview", s.auth(s.handlePreview))
	mux.HandleFunc("POST /campaigns/{id}/me/choose", s.auth(s.handleChoose))
	mux.HandleFunc("POST /campaigns/{id}/me/spells", s.auth(s.handleSpells))
	mux.HandleFunc("GET /campaigns/{id}/dm/history", s.auth(s.handleHistory))
	mux.HandleFunc("POST /campaigns/{id}/dm/rewind", s.auth(s.handleRewind))
	mux.HandleFunc("POST /campaigns/{id}/dm", s.auth(s.handleDM))
	mux.HandleFunc("GET /campaigns/{id}/map", s.auth(s.handleMap))
	mux.HandleFunc("GET /campaigns/{id}/locations/{lid}", s.auth(s.handleLocation))
	mux.HandleFunc("GET /campaigns/{id}/npcs/{nid}", s.auth(s.handleNPC))
	mux.HandleFunc("GET /campaigns/{id}/quests", s.auth(s.handleQuests))
	mux.HandleFunc("GET /campaigns/{id}/journal", s.auth(s.handleJournal))
	mux.HandleFunc("GET /campaigns/{id}/notes", s.auth(s.handleNotes))
	mux.HandleFunc("POST /campaigns/{id}/notes", s.auth(s.handleAddNote))
	mux.HandleFunc("POST /campaigns/{id}/notes/{nid}", s.auth(s.handleEditNote))
	mux.HandleFunc("DELETE /campaigns/{id}/notes/{nid}", s.auth(s.handleDeleteNote))
	mux.HandleFunc("POST /campaigns/{id}/stream-token", s.auth(s.handleStreamToken))
	mux.HandleFunc("GET /campaigns/{id}/events", s.handleEvents)
	mux.HandleFunc("GET /campaigns/{id}/log", s.auth(s.handleLog))
	mux.HandleFunc("GET /gm/campaigns", s.gmOnly(s.handleGMCampaigns))
	mux.HandleFunc("POST /gm/campaigns/{id}/restart", s.gmOnly(s.handleGMRestart))
	mux.HandleFunc("DELETE /gm/campaigns/{id}", s.gmOnly(s.handleGMDelete))
	mux.HandleFunc("DELETE /gm/campaigns/{id}/players/{pid}", s.gmOnly(s.handleGMRemovePlayer))
	mux.HandleFunc("GET /gm/backups/{bid}", s.gmOnly(s.handleGMDownloadBackup))
	mux.HandleFunc("POST /gm/backups/{bid}/restore", s.gmOnly(s.handleGMRestoreBackup))
	mux.HandleFunc("DELETE /gm/backups/{bid}", s.gmOnly(s.handleGMDeleteBackup))
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
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Player-ID, X-GM, Last-Event-ID")
			h.Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
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
	kitsFor := func(setting string) []map[string]any {
		kits := []map[string]any{}
		for _, k := range game.KitKeys() {
			kit := game.KitFor(setting, k)
			weapon, _ := game.ItemFor(setting, kit.Weapon)
			armor, _ := game.ItemFor(setting, kit.Armor)
			kits = append(kits, map[string]any{"key": kit.Key, "name": kit.Name, "desc": kit.Desc, "gold": kit.Gold, "weapon": weapon.Key, "armor": armor.Key,
				"items": game.KitItems(setting, kit), "suggested": kit.Suggested})
		}
		return kits
	}
	kits := kitsFor(game.SettingFantasy)
	settings := []map[string]any{}
	for _, s := range game.SettingList {
		settings = append(settings, map[string]any{"key": s.Key, "name": s.Name, "desc": s.Desc, "kits": kitsFor(s.Key)})
	}
	catalog := []game.ItemDef{}
	for _, k := range game.CatalogKeys() {
		catalog = append(catalog, game.Catalog[k])
	}
	writeJSON(w, 200, map[string]any{
		"abilities": game.Abilities, "standard_array": game.StandardArray, "proficiencies": 2, "kits": kits, "settings": settings,
		"xp_table": game.XPTable, "max_level": game.MaxLevel, "dcs": []int{5, 10, 15, 20, 25}, "catalog": catalog,
		"service_mode": s.ServiceMode(), "ruleset": rulesetView(firstNonEmpty(game.NormSetting(r.URL.Query().Get("setting")), game.SettingFantasy)),
		"magic": magicView(firstNonEmpty(game.NormSetting(r.URL.Query().Get("setting")), game.SettingFantasy)),
		"sagas": saga.List(),
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
	// say where it is the player's turn, so the app can show it
	out := make([]MyCampaign, 0, len(list))
	for _, cs := range list {
		mc := MyCampaign{CampaignSummary: cs}
		if cs.Status == game.StatusActive {
			if wld, err := s.store.Load(r.Context(), cs.ID); err == nil {
				v := campaignView(wld, p)
				mc.YourTurn = !v.FreeRoam && v.You.YourTurn
				mc.Deadline = v.Deadline
			}
		}
		out = append(out, mc)
	}
	writeJSON(w, 200, map[string]any{"campaigns": out})
}

func (s *Server) handleCreateCampaign(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Premise   string `json:"premise"`
		Lethality string `json:"lethality"` // standard (default) | story
		// Saga starts a hand-written campaign (see GET /meta sagas) instead
		// of authoring one from the premise.
		Saga string `json:"saga"`
	}
	if err := decode(r, &body); err != nil {
		s.fail(w, err)
		return
	}
	var wld *game.World
	var err error
	if body.Saga != "" {
		wld, err = s.gm.CreateSaga(r.Context(), body.Saga)
	} else {
		wld, err = s.gm.CreateCampaign(r.Context(), body.Premise)
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	if body.Lethality == game.LethalityStory {
		if wld, err = s.gm.SetLethality(r.Context(), wld.Campaign.ID, body.Lethality); err != nil {
			s.fail(w, err)
			return
		}
	}
	writeJSON(w, http.StatusCreated, campaignView(wld, whoFrom(r).PlayerID))
}

func (s *Server) handleGetCampaign(w http.ResponseWriter, r *http.Request) {
	wld, err := s.loadFor(r)
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
		Text     string        `json:"text"`
		TargetID string        `json:"target_id"`
		ZoneID   string        `json:"zone_id"`
		Check    *gm.CheckHint `json:"check"`
		Act      *gm.ActHint   `json:"act"`
	}
	if err := decode(r, &body); err != nil {
		s.fail(w, err)
		return
	}
	res, err := s.gm.TakeAction(r.Context(), r.PathValue("id"), p, body.Text, gm.Hints{TargetID: body.TargetID, ZoneID: body.ZoneID, Check: body.Check, Act: body.Act})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, res)
}

// handleRoll rolls the dice for the player's pending turn.
func (s *Server) handleRoll(w http.ResponseWriter, r *http.Request) {
	p, ok := s.needPlayer(w, r)
	if !ok {
		return
	}
	res, err := s.gm.Roll(r.Context(), r.PathValue("id"), p)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, res)
}

// handleCancelRoll takes back a pending turn before its roll.
func (s *Server) handleCancelRoll(w http.ResponseWriter, r *http.Request) {
	p, ok := s.needPlayer(w, r)
	if !ok {
		return
	}
	if err := s.gm.CancelPending(r.Context(), r.PathValue("id"), p); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// handleDice sets whether the player rolls their own dice.
func (s *Server) handleDice(w http.ResponseWriter, r *http.Request) {
	p, ok := s.needPlayer(w, r)
	if !ok {
		return
	}
	var body struct {
		AutoRoll bool `json:"auto_roll"`
	}
	if err := decode(r, &body); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.gm.SetAutoRoll(r.Context(), r.PathValue("id"), p, body.AutoRoll); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"auto_roll": body.AutoRoll})
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
	wld, err := s.loadFor(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, mapView(wld))
}

func (s *Server) handleLocation(w http.ResponseWriter, r *http.Request) {
	wld, err := s.loadFor(r)
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
	wld, err := s.loadFor(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	n := wld.NPC(r.PathValue("nid"))
	if n == nil || n.ID != r.PathValue("nid") {
		s.fail(w, store.ErrNotFound)
		return
	}
	v, ok := npcView(wld, n, wld.CharacterOf(whoFrom(r).PlayerID))
	if !ok {
		s.fail(w, store.ErrNotFound)
		return
	}
	writeJSON(w, 200, v)
}

func (s *Server) handleQuests(w http.ResponseWriter, r *http.Request) {
	wld, err := s.loadFor(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"quests": questsView(wld)})
}

func (s *Server) handleJournal(w http.ResponseWriter, r *http.Request) {
	wld, err := s.loadFor(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	j := JournalView{Summary: wld.Campaign.Summary, Chronicle: wld.Chronicle, Quests: questsView(wld), Setbacks: wld.Campaign.Setbacks,
		People: journalPeople(wld), Places: journalPlaces(wld)}
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
	writeJSON(w, 200, map[string]any{"events": visibleOnly(evs, whoFrom(r).PlayerID), "last_id": last})
}

// --- player notes: private to their author, never shown to the model.

// MaxNoteLen caps one note, in bytes.
const MaxNoteLen = 4000

// notePlayer loads the campaign and checks the caller has a seat in it.
func (s *Server) notePlayer(r *http.Request) (*game.World, string, error) {
	wld, err := s.store.Load(r.Context(), r.PathValue("id"))
	if err != nil {
		return nil, "", err
	}
	p := wld.Player(whoFrom(r).PlayerID)
	if p == nil {
		return nil, "", gm.ErrForbidden
	}
	return wld, p.ID, nil
}

func noteText(r *http.Request) (string, error) {
	var body struct {
		Text string `json:"text"`
	}
	if err := decode(r, &body); err != nil {
		return "", err
	}
	t := strings.TrimSpace(body.Text)
	if t == "" {
		return "", fmt.Errorf("%w: note text is empty", gm.ErrBadRequest)
	}
	if len(t) > MaxNoteLen {
		return "", fmt.Errorf("%w: notes are limited to %d characters", gm.ErrBadRequest, MaxNoteLen)
	}
	return t, nil
}

func (s *Server) handleNotes(w http.ResponseWriter, r *http.Request) {
	wld, pid, err := s.notePlayer(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	notes, err := s.store.Notes(r.Context(), wld.Campaign.ID, pid)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"notes": notes})
}

func (s *Server) handleAddNote(w http.ResponseWriter, r *http.Request) {
	wld, pid, err := s.notePlayer(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	text, err := noteText(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	have, err := s.store.Notes(r.Context(), wld.Campaign.ID, pid)
	if err != nil {
		s.fail(w, err)
		return
	}
	if len(have) >= store.MaxNotes {
		s.fail(w, gm.Conflict("the journal is full (%d notes); delete some first", store.MaxNotes))
		return
	}
	c := wld.Campaign
	n, err := s.store.AddNote(r.Context(), c.ID, store.Note{PlayerID: pid, Text: text, Turn: c.Turn, Day: c.Scene.Day, Time: c.Scene.TimeName()})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 201, n)
}

func (s *Server) handleEditNote(w http.ResponseWriter, r *http.Request) {
	wld, pid, err := s.notePlayer(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	text, err := noteText(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	if err := s.store.UpdateNote(r.Context(), wld.Campaign.ID, pid, r.PathValue("nid"), text); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleDeleteNote(w http.ResponseWriter, r *http.Request) {
	wld, pid, err := s.notePlayer(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	if err := s.store.DeleteNote(r.Context(), wld.Campaign.ID, pid, r.PathValue("nid")); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

// loadFor reads the campaign as the caller's group sees it: their group's
// place, scene, turns and fight.
func (s *Server) loadFor(r *http.Request) (*game.World, error) {
	wld, err := s.store.Load(r.Context(), r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	return wld, wld.FocusPlayer(whoFrom(r).PlayerID)
}

// handleSolo splits the player's character off to go on alone.
func (s *Server) handleSolo(w http.ResponseWriter, r *http.Request) {
	p, ok := s.needPlayer(w, r)
	if !ok {
		return
	}
	wld, err := s.gm.GoSolo(r.Context(), r.PathValue("id"), p)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"campaign": campaignView(wld, p)})
}

// handleAnswer answers a world event's call: the player's group goes to
// its site and joins whoever is there.
func (s *Server) handleAnswer(w http.ResponseWriter, r *http.Request) {
	p, ok := s.needPlayer(w, r)
	if !ok {
		return
	}
	wld, err := s.gm.AnswerEvent(r.Context(), r.PathValue("id"), p)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"campaign": campaignView(wld, p)})
}

// handleBegin starts a world event's confrontation now, for a player at
// its site.
func (s *Server) handleBegin(w http.ResponseWriter, r *http.Request) {
	p, ok := s.needPlayer(w, r)
	if !ok {
		return
	}
	wld, err := s.gm.BeginEventNow(r.Context(), r.PathValue("id"), p)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"campaign": campaignView(wld, p)})
}

// handleJoinGroup brings the player's group into another one in the same
// place.
func (s *Server) handleJoinGroup(w http.ResponseWriter, r *http.Request) {
	p, ok := s.needPlayer(w, r)
	if !ok {
		return
	}
	var body struct {
		Group string `json:"group"`
	}
	if err := decode(r, &body); err != nil {
		s.fail(w, err)
		return
	}
	wld, err := s.gm.JoinGroup(r.Context(), r.PathValue("id"), p, body.Group)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"campaign": campaignView(wld, p)})
}

// handleTick runs the timers now: overdue turns in async fights. The
// Worker's cron calls it so a sleeping server wakes to do this.
func (s *Server) handleTick(w http.ResponseWriter, r *http.Request) {
	n, err := s.gm.Sweep(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"autopilot_turns": n})
}

// handleSeen marks the player as caught up with the world, clearing their
// "while you were away" dispatch.
func (s *Server) handleSeen(w http.ResponseWriter, r *http.Request) {
	p, ok := s.needPlayer(w, r)
	if !ok {
		return
	}
	wld, err := s.gm.MarkSeen(r.Context(), r.PathValue("id"), p)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"campaign": campaignView(wld, p)})
}

// handleDowntime queues (or, with kind "", cancels) the player's downtime
// order for the next world tick.
func (s *Server) handleDowntime(w http.ResponseWriter, r *http.Request) {
	p, ok := s.needPlayer(w, r)
	if !ok {
		return
	}
	var body struct {
		Kind string `json:"kind"`
	}
	if err := decode(r, &body); err != nil {
		s.fail(w, err)
		return
	}
	wld, err := s.gm.SetDowntime(r.Context(), r.PathValue("id"), p, body.Kind)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"campaign": campaignView(wld, p)})
}

// handleGoal adds a personal goal ({text}) or drops one ({drop: id}).
func (s *Server) handleGoal(w http.ResponseWriter, r *http.Request) {
	p, ok := s.needPlayer(w, r)
	if !ok {
		return
	}
	var body struct {
		Text string `json:"text"`
		Drop string `json:"drop"`
	}
	if err := decode(r, &body); err != nil {
		s.fail(w, err)
		return
	}
	wld, err := s.gm.SetGoal(r.Context(), r.PathValue("id"), p, body.Text, body.Drop)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"campaign": campaignView(wld, p)})
}
