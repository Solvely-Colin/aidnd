package gm

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/llm"
)

func TestJoinARunningStory(t *testing.T) {
	r := newRig(t, 2, 1)
	r.start(t)
	// the party has been at it a while
	w := r.world(t)
	game.AwardXP(w.Characters[0], 900)
	game.AwardXP(w.Characters[1], 700)
	w.Characters[1].ZoneID = w.Here().Zones[1].ID
	w.Characters[0].ZoneID = w.Here().Zones[1].ID
	_, err := r.g.Store.Save(bg, w)
	must(t, err)

	_, created, err := r.g.Join(bg, r.id, "u3", "Late")
	must(t, err)
	if !created {
		t.Fatal("a friend can take a seat in a story under way")
	}
	r.f.On("narrate_arrival", llm.Text("Cora shoulders through the door, rain on her coat, and nods to Aria and Bram.\nGOAL: Find the sister who wrote to her from this town"))
	c, err := r.g.SaveCharacter(bg, r.id, "u3", specs[2])
	must(t, err)

	w = r.world(t)
	c = w.Character(c.ID)
	if c.XP != 800 || c.Level < 2 || c.HP != c.MaxHP {
		t.Fatalf("newcomer arrives at the party's level: xp %d level %d hp %d/%d", c.XP, c.Level, c.HP, c.MaxHP)
	}
	if c.ZoneID != w.Characters[0].ZoneID || game.GroupOf(c) != game.MainGroup {
		t.Fatalf("newcomer stands with the party: zone %s vs %s, group %s", c.ZoneID, w.Characters[0].ZoneID, game.GroupOf(c))
	}
	if o := w.Campaign.Turns.Order; len(o) != 3 || o[2] != c.ID {
		t.Fatalf("newcomer takes turns after the others: %v", o)
	}
	if g := c.OpenGoals(); len(g) != 1 || !strings.Contains(g[0].Text, "sister") {
		t.Fatalf("newcomer gets the narrator's goal: %+v", c.Goals)
	}
	if len(c.Record) != 1 || !strings.Contains(c.Record[0].Text, "Aria") {
		t.Fatalf("newcomer's record starts with joining: %+v", c.Record)
	}
	found := false
	for _, f := range w.Chronicle {
		found = found || strings.HasPrefix(f.Text, "Cora joined")
	}
	if !found {
		t.Fatal("the chronicle remembers the newcomer joining")
	}
	if w.Campaign.Living != nil && w.Campaign.Living.Seen["u3"] == nil {
		t.Fatal("newcomer starts caught up with the world")
	}

	// a private story so far, and the narrator writes them in
	cu := r.events(t, EventCatchUp)
	if len(cu) != 1 {
		t.Fatalf("catch-up events: %d", len(cu))
	}
	var d struct {
		For     string        `json:"for"`
		CatchUp *game.CatchUp `json:"catch_up"`
	}
	must(t, json.Unmarshal(cu[0].Data, &d))
	if d.For != "u3" || d.CatchUp == nil || d.CatchUp.Location == "" {
		t.Fatalf("catch-up: %+v", d)
	}
	party := 0
	for _, l := range d.CatchUp.Lines {
		if l.Kind == game.CatchParty {
			party++
		}
	}
	if party != 2 {
		t.Fatalf("catch-up names the two companions: %+v", d.CatchUp.Lines)
	}
	narr := r.events(t, "narration")
	last := narr[len(narr)-1]
	if !strings.Contains(string(last.Data), "arrival") || strings.Contains(string(last.Data), "GOAL") {
		t.Fatalf("arrival narration: %s", last.Data)
	}
	u := lastUser(r.f.Tagged("narrate_arrival")[0])
	if !strings.Contains(u, "NEWCOMER: Cora") || !strings.Contains(u, "Aria") {
		t.Fatalf("arrival prompt:\n%s", u)
	}

	// they can play straight away (outside a fight anyone may act)
	r.act(t, "u3", "I look around", llm.Call("look", map[string]any{}))
}

func TestJoinMidFightAndFallbacks(t *testing.T) {
	r := newRig(t, 2, 1)
	r.start(t)
	w := r.world(t)
	w.Encounter = &game.Encounter{ID: "e1", LocationID: w.Campaign.LocationID, Round: 2, Enemies: []*game.Enemy{{ID: "en1", Name: "Ghoul", HP: 5, StatBlock: game.StatBlock{MaxHP: 9}}}}
	w.Campaign.Turns.Index = 1
	_, err := r.g.Store.Save(bg, w)
	must(t, err)
	_, _, err = r.g.Join(bg, r.id, "u3", "Late")
	must(t, err)
	// a name already taken is refused
	if _, err := r.g.SaveCharacter(bg, r.id, "u3", specs[0]); err == nil {
		t.Fatal("duplicate name accepted mid-story")
	}
	// the model fails: plain engine text and the engine's goal
	r.f.On("narrate_arrival", func(llm.Request) (llm.Response, error) { return llm.Response{}, errors.New("model down") })
	c, err := r.g.SaveCharacter(bg, r.id, "u3", specs[3])
	must(t, err)
	w = r.world(t)
	c = w.Character(c.ID)
	if o := w.Campaign.Turns.Order; len(o) != 3 || o[2] != c.ID || w.Campaign.Turns.Index != 1 || w.Encounter == nil {
		t.Fatalf("newcomer goes last in the fight and the turn carries on: %v index %d", o, w.Campaign.Turns.Index)
	}
	if g := c.OpenGoals(); len(g) != 1 || g[0].Text != "Earn the trust of Aria and Bram" {
		t.Fatalf("engine goal: %+v", c.Goals)
	}
	narr := r.events(t, "narration")
	if !strings.Contains(string(narr[len(narr)-1].Data), "in the middle of the fight") {
		t.Fatalf("fallback arrival: %s", narr[len(narr)-1].Data)
	}
	var d struct {
		CatchUp *game.CatchUp `json:"catch_up"`
	}
	cu := r.events(t, EventCatchUp)
	must(t, json.Unmarshal(cu[0].Data, &d))
	fight := false
	for _, l := range d.CatchUp.Lines {
		fight = fight || (l.Kind == game.CatchFight && strings.Contains(l.Text, "Ghoul (hurt)"))
	}
	if !fight {
		t.Fatalf("catch-up shows the fight: %+v", d.CatchUp.Lines)
	}
	// a seated player with a character can't swap it mid-story
	if _, err := r.g.SaveCharacter(bg, r.id, "u3", specs[3]); err == nil {
		t.Fatal("living character replaced mid-story")
	}
}

func TestJoinRefusedOnceTheStoryEnds(t *testing.T) {
	r := newRig(t, 1, 1)
	r.start(t)
	w := r.world(t)
	w.Campaign.Status = game.StatusCompleted
	_, err := r.g.Store.Save(bg, w)
	must(t, err)
	if _, _, err := r.g.Join(bg, r.id, "u2", "Late"); err == nil {
		t.Fatal("joined a finished story")
	}
}

func TestSplitGoal(t *testing.T) {
	text, goal := splitGoal("She arrives.\n\n**GOAL:** \"Pay back the debt.\"")
	if text != "She arrives." || goal != "Pay back the debt" {
		t.Fatalf("%q %q", text, goal)
	}
	if text, goal := splitGoal("No goal here."); text != "No goal here." || goal != "" {
		t.Fatalf("%q %q", text, goal)
	}
}
