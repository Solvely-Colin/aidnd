package gm

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/llm"
)

func canonWorld() *game.World {
	w := &game.World{Campaign: game.Campaign{LocationID: "loc1"}}
	w.Locations = []*game.Location{{ID: "loc1", Name: "Harrowmere"}, {ID: "loc2", Name: "The Drowned Chapel"}, {ID: "loc3", Name: "Reedmarsh"}}
	w.NPCs = []*game.NPC{
		{ID: "npc1", Name: "Ada Brennan", LocationID: "loc1", Alive: true},
		{ID: "npc2", Name: "Constable Vane", LocationID: "loc2", Alive: true},
		{ID: "npc3", Name: "Old Corbin", LocationID: "loc1", Alive: false},
	}
	w.Characters = []*game.Character{{ID: "c1", Name: "Bram", HP: 10, MaxHP: 10, Gold: 12}}
	w.Items = []*game.Item{
		{ID: "item1", Name: "Brass Lantern", Owner: game.Owner{Kind: game.OwnerCharacter, ID: "c1"}},
		{ID: "item2", Name: "Rusty Key", Owner: game.Owner{Kind: game.OwnerLocation, ID: "loc1"}},
	}
	return w
}

func TestCanonFlagsContradictions(t *testing.T) {
	w := canonWorld()
	c := newCanon(w, "", nil, "Bram attacks the Marsh Ghoul for 6 damage.")
	bad := map[string]string{
		"Ada slumps against the bar, lifeless.":                         "alive",
		"The body of Ada Brennan lies by the hearth.":                   "alive",
		"Old Corbin smiles and waves you closer.":                       "dead",
		"Corbin says the tide is wrong tonight.":                        "dead",
		"Constable Vane steps out of the fog and nods to you.":          "not here",
		"You pocket a silver locket from the shelf.":                    "nobody got",
		"Ada hands you a heavy iron key.":                               "nobody got",
		"You find a small brass idol under the pew.":                    "nobody got",
		"Ada presses 40 gold into your palm.":                           "40",
		"The ghoul takes 11 damage and reels.":                          "11",
		"At last you arrive at the Drowned Chapel, soaked to the bone.": "Harrowmere",
	}
	for s, want := range bad {
		is := c.check(s)
		if len(is) != 1 || !strings.Contains(is[0].Why, want) {
			t.Errorf("%q: got %+v, want an issue about %q", s, is, want)
		}
	}
}

func TestCanonLeavesGoodProseAlone(t *testing.T) {
	w := canonWorld()
	c := newCanon(w, "", []string{"Bram picks up Rusty Key"}, "Bram attacks the Marsh Ghoul for 6 damage.")
	good := []string{
		"Ada wipes down the bar and says the boats came back empty.",
		"Ada tells you Constable Vane went up to the chapel at dawn.",
		"Ada's shop is shuttered, its sign creaking in the wind.",
		"You remember how Old Corbin used to laugh at the tide.",
		"Corbin's grave sits under the willow, fresh flowers on it.",
		"Ada says the fishermen died last spring, all three of them.",
		"You raise the brass lantern and its light shakes over the water.",
		"You pick up the rusty key; it is cold and wet.",
		"The ghoul takes 6 damage and staggers back.",
		"You have 12 gold left, and it won't last.",
		"The road north leads toward the Drowned Chapel.",
		"Old fishing nets hang drying on the posts.",
		"You take a breath and hold it.",
		"You find a narrow stair leading down.",
	}
	for _, s := range good {
		if is := c.check(s); len(is) > 0 {
			t.Errorf("false positive on %q: %+v", s, is)
		}
	}
}

func TestCanonAllowsWhatTheEngineReported(t *testing.T) {
	w := canonWorld()
	w.NPCs[0].Alive = false // Ada died this very turn
	c := newCanon(w, "loc2", []string{"Ada Brennan is defeated", "The party arrived at The Drowned Chapel"})
	for _, s := range []string{"Ada staggers and falls dead at your feet.", "You arrive at the Drowned Chapel as the bell tolls."} {
		if is := c.check(s); len(is) > 0 {
			t.Errorf("%q flagged: %+v", s, is)
		}
	}
}

func TestCanonLifeOnly(t *testing.T) {
	c := newCanon(canonWorld(), "", nil)
	c.lifeOnly = true
	if is := c.check("Bram picked up a silver locket. Constable Vane nods at the chapel door."); len(is) > 0 {
		t.Fatalf("lifeOnly should only check life and death: %+v", is)
	}
	if is := c.check("Ada Brennan was killed in the fire."); len(is) != 1 {
		t.Fatalf("lifeOnly missed a false death: %+v", is)
	}
}

func TestSplitSentences(t *testing.T) {
	got := splitSentences(`Ada looks up. "Not tonight!" she says. Rain... falls`)
	want := []string{"Ada looks up.", `"Not tonight!" she says.`, "Rain... falls"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q", got)
	}
}

// End to end: the narrator kills an NPC who is alive. The guard asks for a
// rewrite and the turn shows the fixed prose; the event log keeps what was
// fixed.
func TestNarrationGuardRepairs(t *testing.T) {
	r := newRig(t, 1, 1)
	w := r.start(t)
	var n *game.NPC
	for _, x := range w.NPCsAt(w.Campaign.LocationID) {
		if x.Alive {
			n = x
			break
		}
	}
	if n == nil {
		t.Skip("no NPC at the hub")
	}
	r.f.On("narrate", llm.Text("You look around the square. "+n.Name+" is dead, slumped in the doorway."))
	r.f.On("canon_repair", llm.Text("You look around the square. "+n.Name+" watches you from the doorway."))
	res := r.act(t, r.pids[0], "I look around and then rest a moment", llm.Call("look", map[string]string{}), llm.Call("rest", map[string]string{}))
	if strings.Contains(res.Narration, "is dead") || !strings.Contains(res.Narration, "watches you") {
		t.Fatalf("narration not repaired: %q", res.Narration)
	}
	evs := r.events(t, "action_resolved")
	var d map[string]any
	_ = json.Unmarshal(evs[len(evs)-1].Data, &d)
	if fx, _ := d["canon_fixes"].([]any); len(fx) != 1 {
		t.Fatalf("canon_fixes not logged: %v", d["canon_fixes"])
	}
}

// When the rewrite still contradicts the state, the bad sentence is cut.
func TestNarrationGuardCuts(t *testing.T) {
	r := newRig(t, 1, 1)
	w := r.start(t)
	var n *game.NPC
	for _, x := range w.NPCsAt(w.Campaign.LocationID) {
		if x.Alive {
			n = x
			break
		}
	}
	if n == nil {
		t.Skip("no NPC at the hub")
	}
	bad := "Rain hammers the square and the gutters overflow. " + n.Name + " is dead, slumped in the doorway. The wind howls off the lake all night."
	r.f.On("narrate", llm.Text(bad))
	r.f.On("canon_repair", llm.Text(bad))
	res := r.act(t, r.pids[0], "I look around and then rest a moment", llm.Call("look", map[string]string{}), llm.Call("rest", map[string]string{}))
	if strings.Contains(res.Narration, "is dead") || !strings.Contains(res.Narration, "gutters overflow") {
		t.Fatalf("bad sentence not cut: %q", res.Narration)
	}
}

func TestCanonPlayTestCases(t *testing.T) {
	w := canonWorld()
	w.NPCs = append(w.NPCs, &game.NPC{ID: "npc4", Name: "Tom Coyle", LocationID: "loc3", Alive: true})
	c := newCanon(w, "", nil)
	if is := c.check(`"Tom Coyle," she says, slow and careful, "went out on the boat and the boat came back without him."`); len(is) > 0 {
		t.Errorf("speech about an absent man flagged: %+v", is)
	}
	if is := c.check("Her fingers hunt along the shelf. She finds only a heavy brass key and a tin of pen nibs."); len(is) != 1 {
		t.Errorf("invented find not flagged: %+v", is)
	}
}

func TestCanonDeathAcrossAnd(t *testing.T) {
	c := newCanon(canonWorld(), "", nil)
	if is := c.check("Ada staggers and falls dead at your feet."); len(is) != 1 {
		t.Fatalf("death after 'and' missed: %+v", is)
	}
}

func TestCanonPlayTestCases2(t *testing.T) {
	c := newCanon(canonWorld(), "", nil)
	if is := c.check("Behind the peg wall a loose plank hides a tin of ship's money and a small oilcloth bundle."); len(is) != 1 {
		t.Errorf("invented cache not flagged: %+v", is)
	}
	w := canonWorld()
	w.NPCs = append(w.NPCs, &game.NPC{ID: "npc4", Name: "Tom Coyle", LocationID: "loc3", Alive: true})
	c = newCanon(w, "", nil)
	if is := c.check("Whatever happened to Tom Coyle, this quiet place gives nothing back."); len(is) > 0 {
		t.Errorf("absent man flagged: %+v", is)
	}
}

func TestCanonSlidAcross(t *testing.T) {
	c := newCanon(canonWorld(), "", nil)
	if is := c.check("She slides a key across the desk, not quite meeting your eye."); len(is) != 1 {
		t.Errorf("handed key not flagged: %+v", is)
	}
	if is := c.check("She slides the rusty key across the desk."); len(is) != 0 {
		t.Errorf("known key flagged: %+v", is)
	}
}

func TestCanonOtherShapesOfActing(t *testing.T) {
	w := canonWorld()
	w.NPCs[2].Role = "harbor innkeeper" // Old Corbin, dead, here
	c := newCanon(w, "", nil)
	for _, s := range []string{
		"Corbin, the old harbor man, nods slowly.",
		"Corbin — grey and stooped — beckons you in.",
		`"Get out," says Old Corbin.`,
		`"Not tonight," mutters Corbin, turning away.`,
		"Corbin's hand closes on your sleeve.",
		"The innkeeper nods and pours you a drink.",
		`"Rooms are paid in advance," says the innkeeper.`,
	} {
		if is := c.check(s); len(is) != 1 || !strings.Contains(is[0].Why, "dead") {
			t.Errorf("%q: got %+v, want the dead acting", s, is)
		}
	}
	for _, s := range []string{
		"Constable Vane, they say, went up to the chapel.",
		`"Find Corbin's killer," Ada says.`,
		"Ada asks Corbin's ghost nothing; the room is silent.",
		"You remember Corbin's voice, rough as rope.",
		"Ada, the barmaid, wipes the counter.",
		`"Where is Vane?" asks Ada.`,
	} {
		if is := c.check(s); len(is) != 0 {
			t.Errorf("false positive on %q: %+v", s, is)
		}
	}
	// someone elsewhere, in the same shapes
	if is := c.check("Vane, rain dripping from his hat, steps out of the fog."); len(is) != 1 || !strings.Contains(is[0].Why, "not here") {
		t.Errorf("absent Vane in an aside: %+v", is)
	}
}
