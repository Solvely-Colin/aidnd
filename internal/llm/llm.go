// Package llm is a tiny OpenAI-compatible chat-completions client with
// tool calling, plus a scripted fake for tests.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Message is one chat message.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// System, User, Assistant and ToolResult build messages.
func System(s string) Message    { return Message{Role: "system", Content: s} }
func User(s string) Message      { return Message{Role: "user", Content: s} }
func Assistant(s string) Message { return Message{Role: "assistant", Content: s} }
func ToolResult(id, s string) Message {
	return Message{Role: "tool", ToolCallID: id, Content: s}
}

// ToolCall is a function call requested by the model.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall carries the name and raw JSON arguments.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Call is a convenience constructor (mostly for fakes).
func Call(name string, args any) ToolCall {
	b, _ := json.Marshal(args)
	return ToolCall{ID: "call_" + name, Type: "function", Function: FunctionCall{Name: name, Arguments: string(b)}}
}

// Decode unmarshals the arguments into v. Models sometimes double-encode
// arguments as a JSON string; that is accepted too.
func (c ToolCall) Decode(v any) error {
	a := strings.TrimSpace(c.Function.Arguments)
	if a == "" {
		a = "{}"
	}
	if strings.HasPrefix(a, `"`) {
		var inner string
		if err := json.Unmarshal([]byte(a), &inner); err == nil {
			a = inner
		}
	}
	return json.Unmarshal([]byte(a), v)
}

// Tool is a function the model may call. Parameters is a JSON Schema.
type Tool struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// Request is one chat call. Tag names the job (interpret, narrate, ...)
// for logs and for the fake; it is not sent.
type Request struct {
	Tag         string
	Messages    []Message
	Tools       []Tool
	ForceTool   string // name of a tool the model must call ("" = auto)
	RequireTool bool   // must call some tool
	Temperature float64
	MaxTokens   int
}

// Response is the model's reply.
type Response struct {
	Content   string
	ToolCalls []ToolCall
}

// Client is implemented by the HTTP client and the fake.
type Client interface {
	Chat(ctx context.Context, req Request) (Response, error)
}

// HTTPClient talks to an OpenAI-compatible endpoint.
type HTTPClient struct {
	BaseURL string
	APIKey  string
	Model   string
	// ReasoningEffort is sent as reasoning_effort when set ("none", "low",
	// ...), for reasoning models that support it.
	ReasoningEffort string
	HTTP            *http.Client
}

// NewHTTP returns a client with a generous timeout (campaign creation is slow).
func NewHTTP(baseURL, apiKey, model string) *HTTPClient {
	return &HTTPClient{BaseURL: strings.TrimRight(baseURL, "/"), APIKey: apiKey, Model: model, HTTP: &http.Client{Timeout: 180 * time.Second}}
}

type wireTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description,omitempty"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

type wireRequest struct {
	Model       string     `json:"model"`
	Messages    []Message  `json:"messages"`
	Tools       []wireTool `json:"tools,omitempty"`
	ToolChoice  any        `json:"tool_choice,omitempty"`
	Temperature float64    `json:"temperature"`
	MaxTokens   int        `json:"max_tokens,omitempty"`
	Reasoning   string     `json:"reasoning_effort,omitempty"`
	Stream      bool       `json:"stream"`
}

type wireResponse struct {
	Choices []struct {
		Message struct {
			Content   *string    `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Chat sends one request. Reasoning models spend max_tokens on hidden
// thinking, so a reply cut off by the cap (empty or mid-sentence) is
// retried once without it.
func (c *HTTPClient) Chat(ctx context.Context, req Request) (Response, error) {
	r, finish, err := c.chat(ctx, req)
	if err == nil && finish == "length" && req.MaxTokens > 0 {
		req.MaxTokens = 0
		r, _, err = c.chat(ctx, req)
	}
	return r, err
}

func (c *HTTPClient) chat(ctx context.Context, req Request) (Response, string, error) {
	wr := wireRequest{Model: c.Model, Messages: req.Messages, Temperature: req.Temperature, MaxTokens: req.MaxTokens, Reasoning: c.ReasoningEffort}
	for _, t := range req.Tools {
		var wt wireTool
		wt.Type = "function"
		wt.Function.Name, wt.Function.Description, wt.Function.Parameters = t.Name, t.Description, t.Parameters
		wr.Tools = append(wr.Tools, wt)
	}
	if len(req.Tools) > 0 {
		switch {
		case req.ForceTool != "":
			wr.ToolChoice = map[string]any{"type": "function", "function": map[string]string{"name": req.ForceTool}}
		case req.RequireTool:
			wr.ToolChoice = "required"
		default:
			wr.ToolChoice = "auto"
		}
	}
	body, err := json.Marshal(wr)
	if err != nil {
		return Response{}, "", err
	}
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Response{}, "", err
	}
	hr.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		hr.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := c.HTTP.Do(hr)
	if err != nil {
		return Response{}, "", fmt.Errorf("llm %s: %w", req.Tag, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return Response{}, "", err
	}
	if resp.StatusCode/100 != 2 {
		return Response{}, "", fmt.Errorf("llm %s: HTTP %d: %s", req.Tag, resp.StatusCode, truncate(string(raw), 300))
	}
	var out wireResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return Response{}, "", fmt.Errorf("llm %s: bad JSON: %w", req.Tag, err)
	}
	if out.Error != nil {
		return Response{}, "", fmt.Errorf("llm %s: %s", req.Tag, out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return Response{}, "", fmt.Errorf("llm %s: no choices", req.Tag)
	}
	m := out.Choices[0].Message
	r := Response{ToolCalls: m.ToolCalls}
	if m.Content != nil {
		r.Content = *m.Content
	}
	return r, out.Choices[0].FinishReason, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

var fenceRe = regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)```")

// SalvageToolCalls recovers tool calls a model wrote as JSON text instead
// of using the tool-calling channel. It accepts {"name":..,"arguments":..},
// {"tool":..,"args":..}, arrays of those, or (when only one tool is
// allowed) a bare arguments object.
func SalvageToolCalls(content string, allowed []Tool) []ToolCall {
	text := strings.TrimSpace(content)
	if m := fenceRe.FindStringSubmatch(text); m != nil {
		text = strings.TrimSpace(m[1])
	}
	if i := strings.IndexAny(text, "[{"); i > 0 {
		text = text[i:]
	}
	isAllowed := func(n string) bool {
		for _, t := range allowed {
			if t.Name == n {
				return true
			}
		}
		return false
	}
	var raw []map[string]json.RawMessage
	if strings.HasPrefix(text, "[") {
		_ = json.Unmarshal([]byte(text), &raw)
	} else if strings.HasPrefix(text, "{") {
		var one map[string]json.RawMessage
		dec := json.NewDecoder(strings.NewReader(text))
		if dec.Decode(&one) == nil {
			raw = append(raw, one)
		}
	}
	var out []ToolCall
	for i, obj := range raw {
		var name string
		for _, k := range []string{"name", "tool", "function"} {
			if v, ok := obj[k]; ok {
				if json.Unmarshal(v, &name) != nil {
					var fn struct {
						Name      string          `json:"name"`
						Arguments json.RawMessage `json:"arguments"`
					}
					if json.Unmarshal(v, &fn) == nil {
						name = fn.Name
						obj["arguments"] = fn.Arguments
					}
				}
				break
			}
		}
		var args json.RawMessage
		for _, k := range []string{"arguments", "args", "parameters", "input"} {
			if v, ok := obj[k]; ok {
				args = v
				break
			}
		}
		if name == "" && len(allowed) == 1 {
			name = allowed[0].Name
			b, _ := json.Marshal(obj)
			args = b
		}
		if !isAllowed(name) {
			continue
		}
		if len(args) == 0 {
			args = json.RawMessage("{}")
		}
		out = append(out, ToolCall{ID: fmt.Sprintf("salvaged_%d", i), Type: "function", Function: FunctionCall{Name: name, Arguments: string(args)}})
	}
	return out
}
