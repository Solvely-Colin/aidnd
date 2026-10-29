package game

import (
	"strings"
	"testing"
	"time"

	"github.com/Solvely-Colin/aidnd/internal/dice"
)

var t0 = time.Unix(1_800_000_000, 0)

// fixture builds a small world:
//
//	Oakvale (hub, town) --known-- Whisperwood (danger 2) --hidden-- Hollow (stub)
//	Oakvale --locked(key)-- Vault (stub)
//	Whisperwood --lair gate-- Black Spire (lair)
//
// with a blacksmith merchant and an elder quest giver in Oakvale, and two
// characters, Aria (warrior) and Bram (ranger).
type fixture struct {
	w                        *World
	hub, wood, hollow, vault *Location
	lair                     *Location
	smith, elder, villain    *NPC
	aria, bram               *Character
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	w := &World{Campaign: Campaign{ID: "c1", Status: StatusLobby, MaxSetbacks: 3, MaxPlayers: 4}}
	f := &fixture{w: w}
	f.hub = &Location{ID: w.NewID("loc"), Name: "Oakvale", Kind: KindTown, IsHub: true, Visited: true, Description: "A muddy market town."}
	f.wood = &Location{ID: w.NewID("loc"), Name: "Whisperwood", Kind: KindWilderness, Danger: 2, Description: "Dark pines."}
	w.Locations = append(w.Locations, f.hub, f.wood)
	f.hollow = w.AddStub("Hollow", "a damp hollow", "cave", 2)
	f.vault = w.AddStub("Vault", "an iron door", "dungeon", 3)
	f.lair = w.AddStub("Black Spire", "the tower", "dungeon", 5)
	f.lair.IsLair = true
	w.Campaign.HubID, w.Campaign.LairID, w.Campaign.LocationID = f.hub.ID, f.lair.ID, f.hub.ID
	w.Connect(f.hub.ID, f.wood.ID, EdgeKnown, "the forest road", "the road home")
	w.Connect(f.wood.ID, f.hollow.ID, EdgeHidden, "a fox trail", "")
	v := w.Connect(f.hub.ID, f.vault.ID, EdgeLocked, "an iron door", "")
	v.KeyItem = "Iron Key"
	v.LockDC = 15
	g := w.Connect(f.wood.ID, f.lair.ID, EdgeLocked, "the spire path", "")
	g.LairGate = true
	must(t, DefineZones(w, f.hub.ID, []ZoneSpec{
		{Name: "Square", Adjacent: []string{"Forge"}, Exits: []string{"Whisperwood"}},
		{Name: "Forge", Adjacent: []string{"Square", "Elder Hall"}},
		{Name: "Elder Hall", Adjacent: []string{"Forge"}, Exits: []string{"Vault"}},
	}))
	must(t, DefineZones(w, f.wood.ID, []ZoneSpec{
		{Name: "Road", Adjacent: []string{"Fallen Logs"}, Exits: []string{"Oakvale"}},
		{Name: "Fallen Logs", Cover: true, Adjacent: []string{"Road", "Clearing"}},
		{Name: "Clearing", Adjacent: []string{"Fallen Logs", "Thicket"}, Exits: []string{"Hollow", "Black Spire"}},
		{Name: "Thicket", Adjacent: []string{"Clearing"}},
	}))
	f.smith = AddNPC(w, f.hub.ID, NPCSpec{Name: "Dunna", Role: "blacksmith", Merchant: true})
	f.smith.ZoneID = f.hub.Zones[1].ID
	NewItem(w, "longsword", Owner{Kind: OwnerNPC, ID: f.smith.ID})
	NewItem(w, "healing_potion", Owner{Kind: OwnerNPC, ID: f.smith.ID})
	NewItem(w, "chain_mail", Owner{Kind: OwnerNPC, ID: f.smith.ID})
	f.elder = AddNPC(w, f.hub.ID, NPCSpec{Name: "Elder Maren", Role: "elder", Disposition: 1})
	f.villain = AddNPC(w, f.lair.ID, NPCSpec{Name: "Vex", Role: "necromancer"})
	f.villain.Antagonist = true
	w.Campaign.AntagonistID = f.villain.ID

	w.Players = []*Player{{ID: "u1", Name: "Ann"}, {ID: "u2", Name: "Ben"}}
	var err error
	f.aria, err = CreateCharacter(w, "u1", CharacterSpec{Name: "Aria", Kit: "warrior",
		Scores: map[string]int{"STR": 15, "DEX": 12, "CON": 14, "INT": 8, "WIS": 13, "CHA": 10}, Proficient: []string{"STR", "CON"}})
	must(t, err)
	f.bram, err = CreateCharacter(w, "u2", CharacterSpec{Name: "Bram", Kit: "ranger",
		Scores: map[string]int{"STR": 10, "DEX": 15, "CON": 13, "INT": 12, "WIS": 14, "CHA": 8}, Proficient: []string{"DEX", "WIS"}})
	must(t, err)
	must(t, StartGame(w, t0))
	for _, c := range w.Characters {
		c.ZoneID = f.hub.Zones[0].ID
	}
	return f
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) ctx(faces ...int) *Ctx {
	return &Ctx{W: f.w, R: dice.FromSource(dice.NewScripted(faces...)), Now: t0}
}

// goTo puts the party at loc in zone index zi.
func (f *fixture) goTo(loc *Location, zi int) {
	f.w.Campaign.LocationID = loc.ID
	loc.Visited = true
	for _, c := range f.w.Characters {
		c.ZoneID = loc.Zones[zi].ID
	}
}

func hasChange(r *Resolution, sub string) bool {
	for _, c := range r.Changes {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}

func stepErr(r *Resolution, i int) string {
	if i >= len(r.Steps) {
		return "<no step>"
	}
	return r.Steps[i].Error
}
