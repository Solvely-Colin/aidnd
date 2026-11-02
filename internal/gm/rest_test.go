package gm

import (
	"testing"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/llm"
)

// "sleep the night" reaches the engine as a night's rest.
func TestRestLengthReachesEngine(t *testing.T) {
	a, err := toAction(llm.ToolCall{Function: llm.FunctionCall{Name: game.ActRest, Arguments: `{"length":"night"}`}})
	if err != nil || a.Method != game.RestNight {
		t.Fatalf("rest night: %+v %v", a, err)
	}
	a, err = toAction(llm.ToolCall{Function: llm.FunctionCall{Name: game.ActRest, Arguments: `{}`}})
	if err != nil || a.Method != "" {
		t.Fatalf("plain rest: %+v %v", a, err)
	}
}
