package gm

import (
	"strings"
	"testing"
	"time"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/llm"
)

func TestAutopilotPlaysAnOverdueTurn(t *testing.T) {
	r := fightRig(t, 3, game.EnemySpec{Name: "Marsh Ghoul", HP: 40, AC: 5, AttackBonus: 2, Damage: "1d4"})
	r.f.Default("narrate_enemies", llm.Text("The ghoul claws back."))
	w := r.world(t)
	if r.g.TurnDeadline(w) != 0 {
		t.Fatal("a one-shot has no deadline")
	}
	w.Campaign.Async = true
	_, err := r.g.Store.Save(bg, w)
	must(t, err)
	w = r.world(t)
	actor := w.ActiveCharacter()
	dl := r.g.TurnDeadline(w)
	if dl != w.Campaign.Turns.Started+int64(12*time.Hour/time.Second) {
		t.Fatalf("deadline %d, started %d", dl, w.Campaign.Turns.Started)
	}
	// another player can't skip it early in a long campaign
	other := "u2"
	if actor.PlayerID == "u2" {
		other = "u1"
	}
	r.now = r.now.Add(time.Hour)
	if _, err := r.g.Skip(bg, r.id, other, SkipAsPlayer); err == nil {
		t.Fatal("skipping an async turn before its deadline must fail")
	}
	// nothing is due yet
	if n, err := r.g.Sweep(bg); err != nil || n != 0 {
		t.Fatalf("early sweep: %d %v", n, err)
	}
	r.now = r.now.Add(12 * time.Hour)
	n, err := r.g.Sweep(bg)
	must(t, err)
	if n != 1 {
		t.Fatalf("sweep played %d turns", n)
	}
	w = r.world(t)
	if w.ActiveCharacter().ID == actor.ID {
		t.Fatal("the turn should have passed on")
	}
	var played bool
	for _, e := range r.events(t, "player_action") {
		played = played || strings.Contains(string(e.Data), actor.Name) && strings.Contains(string(e.Data), "(away)")
	}
	if !played {
		t.Fatal("no autopilot action logged")
	}
}

func TestAutopilotActions(t *testing.T) {
	w := &game.World{}
	c := &game.Character{ID: "c1", Name: "A", ZoneID: "z1"}
	if a := game.AutopilotActions(w, c); a[0].Kind != game.ActEndTurn {
		t.Fatalf("no fight: %+v", a)
	}
}
