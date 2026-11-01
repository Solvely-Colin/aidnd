// Command server runs the aidnd HTTP API.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Solvely-Colin/aidnd/internal/api"
	"github.com/Solvely-Colin/aidnd/internal/dice"
	"github.com/Solvely-Colin/aidnd/internal/gm"
	"github.com/Solvely-Colin/aidnd/internal/llm"
	"github.com/Solvely-Colin/aidnd/internal/store"
	"github.com/Solvely-Colin/aidnd/internal/web"
)

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(k)); err == nil {
		return v
	}
	return def
}

func envBool(k string, def bool) bool {
	switch strings.ToLower(os.Getenv(k)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

type offline struct{}

func (offline) Chat(context.Context, llm.Request) (llm.Response, error) {
	return llm.Response{}, errors.New("offline mode: no model")
}

func main() {
	logger := log.New(os.Stderr, "aidnd ", log.LstdFlags|log.Lmsgprefix)
	model := env("LLM_MODEL", "")
	if model == "" {
		logger.Fatal("LLM_MODEL is required (e.g. deepseek-v4.1-flash)")
	}
	// LLM_MODEL=offline makes no model calls; every step uses the engine's
	// fallbacks, which is enough to click through the testing table.
	hc := llm.NewHTTP(env("LLM_BASE_URL", "https://ollama.com/v1"), os.Getenv("LLM_API_KEY"), model)
	hc.ReasoningEffort = env("LLM_REASONING_EFFORT", "")
	var client llm.Client = hc
	if model == "offline" {
		client = offline{}
	}
	// On Cloudflare the database is D1, reached through the Worker.
	var st *store.Store
	var err error
	if u := os.Getenv("D1_URL"); u != "" {
		st, err = store.OpenD1(u, os.Getenv("D1_SECRET"))
	} else {
		st, err = store.Open(env("DB_PATH", "aidnd.db"))
	}
	if err != nil {
		logger.Fatalf("open db: %v", err)
	}
	defer st.Close()
	apiKey := os.Getenv("API_KEY")
	// Players roll their own d20s in the real app (service mode); the dev
	// table has no roll button, so open mode rolls at once.
	g := gm.New(client, st, gm.Config{
		SceneTurnCap:        envInt("SCENE_TURN_CAP", 10),
		MaxSetbacks:         envInt("MAX_SETBACKS", 3),
		ObjectiveCheckEvery: envInt("OBJECTIVE_CHECK_EVERY", 1),
		LoreBudget:          envInt("LORE_BUDGET", 1800),
		DebugDirector:       envBool("DEBUG_DIRECTOR", false),
		SkipAfterSeconds:    envInt("SKIP_AFTER_SECONDS", 120),
		PlayerDice:          envBool("PLAYER_DICE", apiKey != ""),
	}, dice.New(time.Now().UnixNano()))
	g.Log = logger
	cfg := api.Config{APIKey: apiKey}
	if o := os.Getenv("CORS_ORIGINS"); o != "" {
		cfg.CORSOrigins = strings.Split(o, ",")
	}
	// The dev table is on by default only in open mode.
	if envBool("DEV_UI", apiKey == "") {
		cfg.DevUI = web.Handler()
	}
	srv := api.New(g, cfg)
	addr := env("ADDR", ":8080")
	hs := &http.Server{Addr: addr, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second}
	mode := "open dev mode (per-player tokens)"
	if apiKey != "" {
		mode = "service mode (API_KEY)"
	}
	logger.Printf("listening on %s, %s, model %s", addr, mode, model)
	if cfg.DevUI != nil {
		host := addr
		if strings.HasPrefix(host, ":") || strings.HasPrefix(host, "0.0.0.0:") {
			host = "localhost:" + host[strings.LastIndex(host, ":")+1:]
		}
		logger.Printf("testing table at http://%s/dev/", host)
	}
	// Timers: autopilot for turns past their deadline in async fights.
	go func() {
		for range time.Tick(time.Minute) {
			if n, err := g.Sweep(context.Background()); err != nil {
				logger.Printf("sweep: %v", err)
			} else if n > 0 {
				logger.Printf("sweep: autopilot played %d turns", n)
			}
		}
	}()
	go func() {
		if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal(err)
		}
	}()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = hs.Shutdown(ctx)
}
