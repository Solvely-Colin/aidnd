package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/Solvely-Colin/aidnd/internal/gm"
	"github.com/Solvely-Colin/aidnd/internal/store"
)

// Hub fans committed events out to SSE subscribers per campaign.
type Hub struct {
	mu   sync.Mutex
	subs map[string]map[chan store.Event]struct{}
}

// NewHub returns an empty hub.
func NewHub() *Hub { return &Hub{subs: map[string]map[chan store.Event]struct{}{}} }

// Publish delivers events to every subscriber of the campaign. A
// subscriber that cannot keep up is dropped; it can resume with
// Last-Event-ID.
func (h *Hub) Publish(campaignID string, evs []store.Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[campaignID] {
		for _, e := range evs {
			select {
			case ch <- e:
			default:
				delete(h.subs[campaignID], ch)
				close(ch)
				goto next
			}
		}
	next:
	}
}

func (h *Hub) subscribe(campaignID string) chan store.Event {
	ch := make(chan store.Event, 256)
	h.mu.Lock()
	if h.subs[campaignID] == nil {
		h.subs[campaignID] = map[chan store.Event]struct{}{}
	}
	h.subs[campaignID][ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) unsubscribe(campaignID string, ch chan store.Event) {
	h.mu.Lock()
	if _, ok := h.subs[campaignID][ch]; ok {
		delete(h.subs[campaignID], ch)
		close(ch)
	}
	h.mu.Unlock()
}

// Subscribers counts listeners (for tests).
func (h *Hub) Subscribers(campaignID string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs[campaignID])
}

// visibleTo reports whether a player may read an event. Visions are
// private: only the player they were shown to sees them.
func visibleTo(e store.Event, player string) bool {
	if e.Type != gm.EventVision {
		return true
	}
	var d struct {
		For string `json:"for"`
	}
	return json.Unmarshal(e.Data, &d) == nil && d.For != "" && d.For == player
}

// visibleOnly drops the events a player may not read.
func visibleOnly(evs []store.Event, player string) []store.Event {
	out := evs[:0:0]
	for _, e := range evs {
		if visibleTo(e, player) {
			out = append(out, e)
		}
	}
	return out
}

func writeSSE(w http.ResponseWriter, e store.Event) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if e.ID == 0 {
		// Unsaved progress events carry no id, so they never move the
		// browser's Last-Event-ID.
		_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Type, b)
		return err
	}
	_, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", e.ID, e.Type, b)
	return err
}

// heartbeat is how often a comment line keeps proxies from closing idle
// streams.
var heartbeat = 15 * time.Second

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	player, ok := s.streamAuth(w, r, id)
	if !ok {
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	var last int64
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		last, _ = strconv.ParseInt(v, 10, 64)
	} else if v := r.URL.Query().Get("after"); v != "" {
		last, _ = strconv.ParseInt(v, 10, 64)
	}
	// subscribe before replay so nothing falls in the gap
	ch := s.hub.subscribe(id)
	defer s.hub.unsubscribe(id, ch)
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "retry: 2000\n\n")
	for {
		evs, err := s.store.Events(r.Context(), id, last, 500)
		if err != nil {
			return
		}
		for _, e := range evs {
			last = e.ID
			if !visibleTo(e, player) {
				continue
			}
			if writeSSE(w, e) != nil {
				return
			}
		}
		if len(evs) < 500 {
			break
		}
	}
	fl.Flush()
	tick := time.NewTicker(heartbeat)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case e, ok := <-ch:
			if !ok {
				return // dropped as too slow; client reconnects with Last-Event-ID
			}
			if e.ID != 0 && e.ID <= last {
				continue
			}
			if !visibleTo(e, player) {
				last = max(last, e.ID)
				continue
			}
			if writeSSE(w, e) != nil {
				return
			}
			if e.ID != 0 {
				last = e.ID
			}
			fl.Flush()
		case <-tick.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			fl.Flush()
		}
	}
}
