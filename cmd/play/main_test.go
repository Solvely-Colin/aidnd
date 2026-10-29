package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Solvely-Colin/aidnd/internal/api"
	"github.com/Solvely-Colin/aidnd/internal/dice"
	"github.com/Solvely-Colin/aidnd/internal/gm"
	"github.com/Solvely-Colin/aidnd/internal/llm"
	"github.com/Solvely-Colin/aidnd/internal/store"
)

// syncBuf is a goroutine-safe output buffer.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func server(t *testing.T) string {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	g := gm.New(llm.NewFake(), st, gm.Config{}, dice.New(3))
	g.Log = log.New(io.Discard, "", 0)
	g.NewID = func() string { return "c1" }
	srv := httptest.NewServer(api.New(g, api.Config{}).Handler())
	t.Cleanup(func() { srv.Close(); st.Close() })
	return srv.URL
}

func TestFullFlow(t *testing.T) {
	ctx := context.Background()
	base := server(t)
	a := &Client{Base: base, Player: "alice", HTTP: http.DefaultClient}
	b := &Client{Base: base, Player: "bob", HTTP: http.DefaultClient}
	var out bytes.Buffer
	must := func(c *Client, in string, args ...string) {
		t.Helper()
		if err := run(ctx, c, args, strings.NewReader(in), &out); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out.String())
		}
	}
	must(a, "", "new", "a", "drowned", "town")
	if !strings.Contains(out.String(), "campaign id: c1") {
		t.Fatalf("new: %s", out.String())
	}
	must(a, "", "join", "c1", "Alice")
	must(b, "", "join", "c1", "Bob")
	must(a, "n Wren\ns\n", "char", "c1", "a hedge witch")
	must(b, "\n", "char", "c1", "a burly sellsword")
	if !strings.Contains(out.String(), "saved Wren") {
		t.Fatalf("char: %s", out.String())
	}
	must(a, "", "start", "c1")

	// Alice plays: look around, read her sheet, act, end the turn.
	pr, pw := io.Pipe()
	var po syncBuf
	done := make(chan error, 1)
	go func() { done <- run(ctx, a, []string{"c1"}, pr, &po) }()
	waitFor := func(s string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for !strings.Contains(po.String(), s) {
			if time.Now().After(deadline) {
				t.Fatalf("waiting for %q in:\n%s", s, po.String())
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	waitFor("Wren (you)'s turn")
	waitFor("> your turn")
	io.WriteString(pw, ":sheet\n:look\n:map\n:quests\n")
	waitFor("Wren, L1")
	waitFor("exit:")
	waitFor("* Harrowmere ->")
	waitFor("[ ]")
	io.WriteString(pw, "I search the square for tracks\n")
	waitFor("Wren: I search the square for tracks")
	io.WriteString(pw, ":nope\n:end\n")
	waitFor("unknown command")
	waitFor("'s turn --") // Bob's
	io.WriteString(pw, ":quit\n")
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	var v campaignView
	if err := b.Do(ctx, "GET", "/campaigns/c1", nil, &v); err != nil || !v.You.YourTurn {
		t.Fatalf("bob should be up: %v %+v", err, v.You)
	}
}

func TestPlayNeedsJoin(t *testing.T) {
	base := server(t)
	c := &Client{Base: base, Player: "x", HTTP: http.DefaultClient}
	var out bytes.Buffer
	if err := run(context.Background(), c, []string{"new"}, nil, &out); err != nil {
		t.Fatal(err)
	}
	err := run(context.Background(), c, []string{"c1"}, strings.NewReader(""), &out)
	if err == nil || !strings.Contains(err.Error(), "has not joined") {
		t.Fatalf("got %v", err)
	}
	if err := run(context.Background(), c, nil, nil, &out); err != errUsage {
		t.Fatalf("got %v", err)
	}
}

func TestReadSSEAndFormat(t *testing.T) {
	var got []string
	_ = readSSE(strings.NewReader("retry: 2000\n\n: ping\n\nid: 1\nevent: x\ndata: {\"a\":1}\n\n"), func(d string) { got = append(got, d) })
	if len(got) != 1 || got[0] != `{"a":1}` {
		t.Fatalf("got %q", got)
	}
	e := Event{Type: "action_resolved", Data: []byte(`{"steps":[{"action":{"kind":"attack"},"rolls":[{"label":"Wren attacks","kind":"attack","natural":17,"mod":5,"total":22,"target":13,"tier":"success"},{"label":"damage","kind":"damage","expr":"1d8+2","dice":[5],"total":7}],"changes":["Goblin takes 7"]}],"skipped":[{"action":{"kind":"move"},"error":"no move left"}]}`)}
	s := formatEvent(e, "")
	for _, want := range []string{"d20 17+5 = 22 vs 13: success", "1d8+2 [5] = 7", "* Goblin takes 7", "skipped move: no move left"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in %q", want, s)
		}
	}
	if w := wrap("one two three four", 9); w != "one two\nthree\nfour" {
		t.Errorf("wrap: %q", w)
	}
}
