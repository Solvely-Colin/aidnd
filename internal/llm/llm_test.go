package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPClientSendsToolsAndParsesCalls(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("path %s auth %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		io.WriteString(w, `{"choices":[{"message":{"content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"look","arguments":"{\"target\":\"door\"}"}}]}}]}`)
	}))
	defer srv.Close()
	c := NewHTTP(srv.URL+"/v1/", "k", "deepseek-v4.1-flash")
	resp, err := c.Chat(context.Background(), Request{Tag: "interpret", Messages: []Message{System("s"), User("u")},
		Tools: []Tool{{Name: "look", Parameters: map[string]any{"type": "object"}}}, ForceTool: "look", Temperature: 0.2})
	if err != nil {
		t.Fatal(err)
	}
	if got["model"] != "deepseek-v4.1-flash" || got["stream"] != false {
		t.Errorf("body %v", got)
	}
	tc := got["tool_choice"].(map[string]any)
	if tc["function"].(map[string]any)["name"] != "look" {
		t.Errorf("tool_choice %v", tc)
	}
	var args struct{ Target string }
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Decode(&args) != nil || args.Target != "door" {
		t.Fatalf("resp %+v", resp)
	}
}

func TestHTTPClientErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		io.WriteString(w, "boom")
	}))
	defer srv.Close()
	_, err := NewHTTP(srv.URL, "", "m").Chat(context.Background(), Request{Tag: "x"})
	if err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("err %v", err)
	}
}

func TestDecodeDoubleEncoded(t *testing.T) {
	tc := ToolCall{Function: FunctionCall{Name: "say", Arguments: `"{\"text\":\"hi\"}"`}}
	var a struct{ Text string }
	if err := tc.Decode(&a); err != nil || a.Text != "hi" {
		t.Fatalf("%v %q", err, a.Text)
	}
}

func TestSalvageToolCalls(t *testing.T) {
	tools := []Tool{{Name: "attack"}, {Name: "move"}}
	cases := map[string]int{
		`{"name":"attack","arguments":{"target":"orc"}}`:                                                               1,
		"Sure!\n```json\n[{\"tool\":\"move\",\"args\":{\"exit\":\"e1\"}},{\"name\":\"attack\",\"arguments\":{}}]\n```": 2,
		`{"function":{"name":"attack","arguments":{"target":"x"}}}`:                                                    1,
		`{"name":"fly","arguments":{}}`:                                                                                0,
		`I attack the orc.`:                                                                                            0,
	}
	for in, want := range cases {
		if got := SalvageToolCalls(in, tools); len(got) != want {
			t.Errorf("%q: %d calls, want %d", in, len(got), want)
		}
	}
	one := SalvageToolCalls(`{"title":"X","zones":[]}`, []Tool{{Name: "define_zones"}})
	if len(one) != 1 || one[0].Function.Name != "define_zones" {
		t.Fatalf("bare object: %+v", one)
	}
}

func TestFakeQueuesByTag(t *testing.T) {
	f := NewFake()
	f.On("narrate", Text("one"), Text("two")).Default("narrate", Text("later"))
	ctx := context.Background()
	for _, want := range []string{"one", "two", "later"} {
		r, _ := f.Chat(ctx, Request{Tag: "narrate"})
		if r.Content != want {
			t.Fatalf("got %q want %q", r.Content, want)
		}
	}
	if r, _ := f.Chat(ctx, Request{Tag: "other"}); r.Content != "The moment passes." {
		t.Fatal("fallback")
	}
	if len(f.Tagged("narrate")) != 3 {
		t.Fatal("recording")
	}
}

func TestHTTPClientRetriesWhenReasoningEatsTheTokenCap(t *testing.T) {
	var caps []any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		caps = append(caps, body["max_tokens"])
		if body["reasoning_effort"] != "low" {
			t.Errorf("reasoning_effort = %v", body["reasoning_effort"])
		}
		if body["max_tokens"] != nil {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":""},"finish_reason":"length"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Fog."},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()
	c := NewHTTP(srv.URL, "", "m")
	c.ReasoningEffort = "low"
	r, err := c.Chat(context.Background(), Request{Tag: "t", Messages: []Message{User("hi")}, MaxTokens: 50})
	if err != nil || r.Content != "Fog." || len(caps) != 2 || caps[1] != nil {
		t.Fatalf("got %q %v caps=%v", r.Content, err, caps)
	}
}
