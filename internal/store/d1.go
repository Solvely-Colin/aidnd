package store

// A database/sql driver that sends SQL to Cloudflare D1 through a small
// HTTP endpoint on the Worker (worker/src/index.ts, POST /internal/d1).
// D1 has no interactive transactions, so a transaction buffers its writes
// and sends them on Commit as one atomic batch. Results of statements in a
// transaction are known only after Commit. Queries are sent at once.
//
// Wire format (JSON, header X-D1-Secret):
//
//	{"mode":"query","sql":..,"params":[..]} -> {"columns":[..],"rows":[[..]..]}
//	{"mode":"batch","stmts":[{"sql":..,"params":[..]}..]}
//	                                        -> {"results":[{"changes":n,"last_row_id":n}..]}
//	errors                                  -> non-2xx with {"error":".."}

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func init() { sql.Register("d1http", &d1Driver{}) }

// OpenD1 opens a store backed by D1 through the Worker endpoint at url.
func OpenD1(url, secret string) (*Store, error) {
	db := sql.OpenDB(&d1Connector{url: url, secret: secret, hc: &http.Client{Timeout: 30 * time.Second}})
	s := &Store{db: db, now: time.Now}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

type d1Driver struct{}

// Open takes "url secret" as its name.
func (d *d1Driver) Open(name string) (driver.Conn, error) {
	url, secret, _ := strings.Cut(name, " ")
	return (&d1Connector{url: url, secret: secret, hc: &http.Client{Timeout: 30 * time.Second}}).Connect(context.Background())
}

type d1Connector struct {
	url, secret string
	hc          *http.Client
}

func (c *d1Connector) Connect(context.Context) (driver.Conn, error) { return &d1Conn{c: c}, nil }
func (c *d1Connector) Driver() driver.Driver                        { return &d1Driver{} }

type d1Stmt struct {
	SQL    string `json:"sql"`
	Params []any  `json:"params"`
}

type d1Request struct {
	Mode   string   `json:"mode"`
	SQL    string   `json:"sql,omitempty"`
	Params []any    `json:"params,omitempty"`
	Stmts  []d1Stmt `json:"stmts,omitempty"`
}

type d1Meta struct {
	Changes   int64 `json:"changes"`
	LastRowID int64 `json:"last_row_id"`
}

type d1Response struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
	Results []d1Meta `json:"results"`
	Error   string   `json:"error"`
}

func (c *d1Connector) do(ctx context.Context, req d1Request) (*d1Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 300 * time.Millisecond):
			}
		}
		hr, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		hr.Header.Set("Content-Type", "application/json")
		hr.Header.Set("X-D1-Secret", c.secret)
		resp, err := c.hc.Do(hr)
		if err != nil {
			lastErr = err
			continue
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		var out d1Response
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber()
		if err := dec.Decode(&out); err != nil {
			lastErr = fmt.Errorf("d1: status %d: %s", resp.StatusCode, truncate(string(data), 200))
			if resp.StatusCode >= 500 {
				continue
			}
			return nil, lastErr
		}
		if resp.StatusCode/100 != 2 {
			lastErr = fmt.Errorf("d1: %s", out.Error)
			// The Worker answers 400 for SQL errors, which would only fail
			// again. A failed batch is rolled back, so retrying it is safe.
			if resp.StatusCode >= 500 {
				continue
			}
			return nil, lastErr
		}
		return &out, nil
	}
	return nil, lastErr
}

// splitSQL splits a script on semicolons after dropping -- comments. It
// is meant for our own schema, which has no semicolons inside strings.
func splitSQL(script string) []string {
	var b strings.Builder
	for _, line := range strings.Split(script, "\n") {
		if i := strings.Index(line, "--"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	var out []string
	for _, q := range strings.Split(b.String(), ";") {
		if q = strings.TrimSpace(q); q != "" {
			out = append(out, q)
		}
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// params turns driver values into JSON-friendly values.
func params(args []driver.NamedValue) ([]any, error) {
	out := make([]any, len(args))
	for i, a := range args {
		if a.Name != "" {
			return nil, errors.New("d1: named parameters are not supported")
		}
		switch v := a.Value.(type) {
		case bool:
			if v {
				out[i] = 1
			} else {
				out[i] = 0
			}
		case []byte:
			out[i] = string(v)
		case time.Time:
			out[i] = v.Unix()
		default:
			out[i] = v
		}
	}
	return out, nil
}

type d1Conn struct {
	c  *d1Connector
	tx *d1Tx
}

func (cn *d1Conn) Prepare(query string) (driver.Stmt, error) {
	return nil, errors.New("d1: prepared statements are not supported")
}
func (cn *d1Conn) Close() error { return nil }
func (cn *d1Conn) Begin() (driver.Tx, error) {
	return cn.BeginTx(context.Background(), driver.TxOptions{})
}

func (cn *d1Conn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if cn.tx != nil {
		return nil, errors.New("d1: transaction already open")
	}
	cn.tx = &d1Tx{cn: cn, ctx: ctx}
	return cn.tx, nil
}

func (cn *d1Conn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	ps, err := params(args)
	if err != nil {
		return nil, err
	}
	if cn.tx != nil {
		r := &d1Result{}
		cn.tx.stmts = append(cn.tx.stmts, d1Stmt{SQL: query, Params: nonNil(ps)})
		cn.tx.results = append(cn.tx.results, r)
		return r, nil
	}
	// A script of several statements (the schema) goes as one batch.
	stmts := []d1Stmt{{SQL: query, Params: nonNil(ps)}}
	if len(ps) == 0 {
		stmts = stmts[:0]
		for _, q := range splitSQL(query) {
			stmts = append(stmts, d1Stmt{SQL: q, Params: []any{}})
		}
	}
	out, err := cn.c.do(ctx, d1Request{Mode: "batch", Stmts: stmts})
	if err != nil {
		return nil, err
	}
	r := &d1Result{done: true}
	if len(out.Results) > 0 {
		r.meta = out.Results[0]
	}
	return r, nil
}

func (cn *d1Conn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	ps, err := params(args)
	if err != nil {
		return nil, err
	}
	out, err := cn.c.do(ctx, d1Request{Mode: "query", SQL: query, Params: nonNil(ps)})
	if err != nil {
		return nil, err
	}
	return &d1Rows{cols: out.Columns, rows: out.Rows}, nil
}

type d1Tx struct {
	cn      *d1Conn
	ctx     context.Context
	stmts   []d1Stmt
	results []*d1Result
}

func (t *d1Tx) Commit() error {
	defer func() { t.cn.tx = nil }()
	if len(t.stmts) == 0 {
		return nil
	}
	out, err := t.cn.c.do(t.ctx, d1Request{Mode: "batch", Stmts: t.stmts})
	if err != nil {
		return err
	}
	if len(out.Results) != len(t.results) {
		return fmt.Errorf("d1: batch returned %d results for %d statements", len(out.Results), len(t.results))
	}
	for i, r := range t.results {
		r.meta, r.done = out.Results[i], true
	}
	return nil
}

func (t *d1Tx) Rollback() error {
	t.cn.tx = nil
	return nil
}

// ErrBeforeCommit is returned by a result read before its transaction commits.
var ErrBeforeCommit = errors.New("d1: result is known only after commit")

type d1Result struct {
	done bool
	meta d1Meta
}

func (r *d1Result) LastInsertId() (int64, error) {
	if !r.done {
		return 0, ErrBeforeCommit
	}
	return r.meta.LastRowID, nil
}

func (r *d1Result) RowsAffected() (int64, error) {
	if !r.done {
		return 0, ErrBeforeCommit
	}
	return r.meta.Changes, nil
}

type d1Rows struct {
	cols []string
	rows [][]any
	i    int
}

func (r *d1Rows) Columns() []string { return r.cols }
func (r *d1Rows) Close() error      { return nil }

func (r *d1Rows) Next(dest []driver.Value) error {
	if r.i >= len(r.rows) {
		return io.EOF
	}
	row := r.rows[r.i]
	r.i++
	for j := range dest {
		if j >= len(row) {
			dest[j] = nil
			continue
		}
		switch v := row[j].(type) {
		case json.Number:
			if n, err := v.Int64(); err == nil {
				dest[j] = n
			} else if f, err := v.Float64(); err == nil {
				dest[j] = f
			} else {
				dest[j] = v.String()
			}
		case bool:
			dest[j] = v
		case string, nil:
			dest[j] = v
		default:
			// Blobs arrive as arrays of bytes; nothing in the schema uses them.
			b, _ := json.Marshal(v)
			dest[j] = string(b)
		}
	}
	return nil
}
