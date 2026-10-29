// Package d1stub is a local stand-in for the Worker's D1 endpoint
// (POST /internal/d1), backed by SQLite. It is used by tests and by
// cmd/d1stub for running the server against "D1" without Cloudflare.
package d1stub

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"sync"

	_ "modernc.org/sqlite"
)

type stmt struct {
	SQL    string `json:"sql"`
	Params []any  `json:"params"`
}

type request struct {
	Mode   string `json:"mode"`
	SQL    string `json:"sql"`
	Params []any  `json:"params"`
	Stmts  []stmt `json:"stmts"`
}

type meta struct {
	Changes   int64 `json:"changes"`
	LastRowID int64 `json:"last_row_id"`
}

// Handler serves the D1 wire format against db. Requests without the
// secret are refused.
type Handler struct {
	DB     *sql.DB
	Secret string
	mu     sync.Mutex
	// Calls counts requests by mode, for tests.
	Calls map[string]int
}

// Open returns a handler over a SQLite database at dsn (e.g. "file::memory:").
func Open(dsn, secret string) (*Handler, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return &Handler{DB: db, Secret: secret, Calls: map[string]int{}}, nil
}

func fail(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.Header.Get("X-D1-Secret") != h.Secret {
		fail(w, http.StatusForbidden, errString("forbidden"))
		return
	}
	var req request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Calls[req.Mode]++
	var out any
	var err error
	switch req.Mode {
	case "query":
		out, err = h.query(req.SQL, req.Params)
	case "batch":
		out, err = h.batch(req.Stmts)
	default:
		err = errString("unknown mode")
	}
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func (h *Handler) query(q string, params []any) (any, error) {
	rows, err := h.DB.Query(q, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	all := [][]any{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				vals[i] = string(b)
			}
		}
		all = append(all, vals)
	}
	return map[string]any{"columns": cols, "rows": all}, rows.Err()
}

// batch runs statements in one transaction, as D1's batch() does.
func (h *Handler) batch(stmts []stmt) (any, error) {
	tx, err := h.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	res := make([]meta, 0, len(stmts))
	for _, s := range stmts {
		r, err := tx.Exec(s.SQL, s.Params...)
		if err != nil {
			return nil, err
		}
		var m meta
		m.Changes, _ = r.RowsAffected()
		m.LastRowID, _ = r.LastInsertId()
		res = append(res, m)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return map[string]any{"results": res}, nil
}

type errString string

func (e errString) Error() string { return string(e) }
