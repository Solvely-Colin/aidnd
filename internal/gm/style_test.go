package gm

import (
	"context"
	"strings"
	"testing"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/llm"
)

func TestTellsFindsMachineHabits(t *testing.T) {
	for _, s := range []string{
		"A ship doesn't dock itself, she says, and a logbook doesn't lie.",
		"The air is thick with salt and something older.",
		"Her eyes slide away — she is afraid.",
		"It isn't fear, it's hunger.",
		"The eldritch, cyclopean stones lean over the water.",
		"A chill runs down your spine as the door opens.",
	} {
		if len(tells(s)) == 0 {
			t.Errorf("no tell found in %q", s)
		}
	}
	for _, s := range []string{
		`Martha keeps her hands on the register. "Ain't nobody gone aboard her, mister. Not since Tuesday."`,
		"Rain beads on the window. Down the hill, a lamp burns in the harbor office, though the office closed at six.",
		"It's not much, but the room is dry.",
	} {
		if got := tells(s); len(got) > 0 {
			t.Errorf("false tell in %q: %v", s, got)
		}
	}
}

func TestPolishKeepsOriginalUnlessBetter(t *testing.T) {
	w := &game.World{}
	text := "Martha Sargent leans close — the air is thick with fish. A logbook doesn't lie, a ship doesn't dock itself."
	fake := llm.NewFake()
	g := &GM{LLM: fake}
	// The fake's fallback is a three-word line: too short, so it is refused.
	if got := g.polish(context.Background(), w, "t", text); got != text {
		t.Fatalf("short rewrite accepted: %q", got)
	}
	// A rewrite that drops a name is refused.
	fake.On("polish", llm.Text("She leans close over the desk, and the lobby smells of fish. Nobody here talks about the ship, she says, low."))
	if got := g.polish(context.Background(), w, "t", text); got != text {
		t.Fatalf("rewrite that lost a name accepted: %q", got)
	}
	good := `Martha Sargent leans close over the desk, smelling of fish. "Nobody brung her in, mister. She come in on her own."`
	fake.On("polish", llm.Text(good))
	if got := g.polish(context.Background(), w, "t", text); got != good {
		t.Fatalf("clean rewrite refused: %q", got)
	}
	// Clean prose costs no call.
	n := len(fake.Requests)
	g.polish(context.Background(), w, "t", good)
	if len(fake.Requests) != n {
		t.Fatal("polish called the model on clean prose")
	}
}

func TestVoiceFollowsCampaign(t *testing.T) {
	w := &game.World{}
	if strings.Contains(voice(w), "Lovecraft") {
		t.Fatal("fantasy campaign got the horror voice")
	}
	w.Campaign.Dread = true
	if !strings.Contains(voice(w), "Lovecraft") || !strings.Contains(voice(w), "em dashes") {
		t.Fatal("dread campaign is missing its voice or the prose rules")
	}
}
