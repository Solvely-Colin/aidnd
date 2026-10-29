package gm

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Solvely-Colin/aidnd/internal/llm"
)

// ErrNoToolCall means the model answered in text twice when a tool call
// was required. Callers fall back to deterministic behaviour.
var ErrNoToolCall = errors.New("model did not call a tool")

// InvalidCalls is returned with the last calls when validation still fails
// after the retry. Callers may use the valid parts.
type InvalidCalls struct{ Problems []string }

func (e *InvalidCalls) Error() string { return "invalid tool calls: " + strings.Join(e.Problems, "; ") }

func toolNames(ts []llm.Tool) string {
	var n []string
	for _, t := range ts {
		n = append(n, t.Name)
	}
	return strings.Join(n, ", ")
}

func (g *GM) chatCalls(ctx context.Context, req llm.Request) ([]llm.ToolCall, string, error) {
	resp, err := g.LLM.Chat(ctx, req)
	if err != nil {
		return nil, "", err
	}
	calls := resp.ToolCalls
	if len(calls) == 0 && resp.Content != "" {
		calls = llm.SalvageToolCalls(resp.Content, req.Tools)
	}
	// drop calls to tools that were not offered
	var ok []llm.ToolCall
	for _, c := range calls {
		for _, t := range req.Tools {
			if t.Name == c.Function.Name {
				ok = append(ok, c)
				break
			}
		}
	}
	return ok, resp.Content, nil
}

// forced runs a request that must produce tool calls. Plain-text answers
// are salvaged or re-prompted once. If validate reports problems, they are
// sent back as tool results for exactly one retry.
func (g *GM) forced(ctx context.Context, req llm.Request, validate func([]llm.ToolCall) []string) ([]llm.ToolCall, error) {
	if req.ForceTool == "" {
		req.RequireTool = true
	}
	calls, text, err := g.chatCalls(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(calls) == 0 {
		g.logf("llm %s: no tool call, re-prompting", req.Tag)
		retry := req
		retry.Messages = append(append([]llm.Message{}, req.Messages...),
			llm.Assistant(text),
			llm.User(fmt.Sprintf("You must answer by calling a tool (%s), not with text. Call it now.", toolNames(req.Tools))))
		calls, _, err = g.chatCalls(ctx, retry)
		if err != nil {
			return nil, err
		}
		if len(calls) == 0 {
			return nil, ErrNoToolCall
		}
		req = retry
	}
	if validate == nil {
		return calls, nil
	}
	problems := validate(calls)
	if len(problems) == 0 {
		return calls, nil
	}
	g.logf("llm %s: validation problems, retrying: %v", req.Tag, problems)
	msgs := append([]llm.Message{}, req.Messages...)
	msgs = append(msgs, llm.Message{Role: "assistant", ToolCalls: calls})
	for i, c := range calls {
		res := "ok"
		if i == 0 {
			res = "ERROR: " + strings.Join(problems, "; ")
		}
		if c.ID == "" {
			calls[i].ID = fmt.Sprintf("call_%d", i)
			c.ID = calls[i].ID
		}
		msgs = append(msgs, llm.ToolResult(c.ID, res))
	}
	msgs = append(msgs, llm.User("Fix the problems above and call the tool(s) again with only valid ids and values."))
	retry := req
	retry.Messages = msgs
	again, _, err := g.chatCalls(ctx, retry)
	if err != nil {
		return calls, &InvalidCalls{Problems: problems}
	}
	if len(again) == 0 {
		return calls, &InvalidCalls{Problems: problems}
	}
	if p := validate(again); len(p) > 0 {
		return again, &InvalidCalls{Problems: p}
	}
	return again, nil
}
