package game

import (
	"fmt"
	"strings"

	"github.com/Solvely-Colin/aidnd/internal/dice"
)

// Visions are what a character under strain perceives that the rest of the
// party does not: a draft behind a wall, a face at the window, a dead man
// standing at the bar. Only that character's player is told. Some are true
// hints drawn from the world's state; the rest are hallucinations. The
// engine decides which, records it here, and never tells the player. The
// group has to compare notes and test what they saw against the world.
//
// A vision is never canon. Its text goes to one player only, never into
// the narration, the log the model reads, the summary or the chronicle.
// The false ones are listed for the narrator as UNREAL, and the canon
// guard cuts any prose that treats a phantom as present.

// Vision kinds. True ones point at something the engine holds; false ones
// are made up.
const (
	VisionHiddenWay = "hidden_way" // true: a hidden exit here
	VisionHostile   = "hostile"    // true: someone here wishes the party ill
	VisionDanger    = "danger"     // true: a dangerous place lies next door
	VisionDead      = "dead"       // false: a dead person seen or heard
	VisionAbsent    = "absent"     // false: someone who is elsewhere, seen here
	VisionWrong     = "wrong"      // false: an uncanny detail on someone here
	VisionPhantom   = "phantom"    // false: a figure that isn't there
	VisionWhisper   = "whisper"    // false: a voice nobody else hears
)

// MaxVisions is how many visions a character keeps on record.
const MaxVisions = 8

// VisionMemory is how many turns a false vision stays on the narrator's
// UNREAL list and in the canon guard.
const VisionMemory = 40

// Vision is one private perception.
type Vision struct {
	Turn int    `json:"turn"`
	Kind string `json:"kind"`
	// Real is whether the world bears it out. Never sent to players.
	Real bool `json:"real"`
	// Seed is the engine's plain account of what is perceived, which the
	// narrator turns into prose ("a drowned woman stands in the doorway").
	Seed string `json:"seed"`
	// Phantom is the words that name an invented figure ("drowned woman");
	// prose that puts it in the scene is cut.
	Phantom string `json:"phantom,omitempty"`
	// Text is what the player was shown.
	Text string `json:"text,omitempty"`
}

// phantoms are figures that are never there. Key holds the words that
// name one in prose; all of them must appear.
var phantoms = []struct{ Seed, Key string }{
	{"a drowned woman, hair plastered to her face, standing quite still", "drowned woman"},
	{"a child in a white nightgown at the edge of the light", "nightgown"},
	{"a faceless man watching from a doorway", "faceless"},
	{"a tall figure in a sodden coat that drips onto the floor", "sodden coat"},
	{"a sailor dripping seawater, staring with milky eyes", "dripping sailor"},
	{"a black dog with too many eyes, sitting and waiting", "black dog"},
	{"a hooded figure that turns its head toward you", "hooded figure"},
	{"an old woman knitting something long and grey", "knitting"},
}

var whispers = []string{
	"a voice whispering your full name, close behind your ear",
	"a voice saying, quite clearly, \"it remembers you\"",
	"someone humming a hymn you last heard as a child",
	"a wet voice from below the floor saying \"come down\"",
	"knocking from inside the wall, three slow knocks, then three more",
	"your own voice, a moment behind you, repeating what you just said",
}

var uncanny = []string{
	"%s's eyes are black from edge to edge for one blink",
	"%s's shadow moves a moment after they do",
	"%s's hands are wet and webbed between the fingers",
	"%s's smile is far too wide, then it isn't",
	"water pools around %s's feet though the floor is dry",
	"%s says your name though nobody told them it",
}

// VisionChance is the percent chance per turn that a character at a given
// strain has a vision. None below StrainShaken.
func VisionChance(c *Character) int {
	if c.Strain < StrainShaken {
		if c.HasScar("whispers") {
			return 5
		}
		return 0
	}
	p := 8 * c.Strain // 16% at 2, 40% at 5
	if c.HasScar("whispers") {
		p += 10
	}
	return p
}

// FalseChance is the percent chance a vision is a hallucination.
func FalseChance(c *Character) int {
	p := 40 + 10*max(0, c.Strain-StrainShaken)
	if c.HasScar("whispers") || c.HasScar("paranoid") {
		p += 10
	}
	return min(p, 85)
}

// lastVision is the turn of a character's latest vision, or -100.
func (c *Character) lastVision() int {
	if n := len(c.Visions); n > 0 {
		return c.Visions[n-1].Turn
	}
	return -100
}

// RollVision decides whether anyone in the party has a vision this turn
// and, if so, what it is. At most one per turn, and no character more
// than once in three turns. It records the vision on the character (the
// caller fills in Text) and returns it, or nil. Dread campaigns only.
func RollVision(w *World, r *dice.Roller) (*Character, *Vision) {
	if !w.Campaign.Dread || w.Here() == nil {
		return nil, nil
	}
	turn := w.Campaign.Turn
	var pool []*Character
	for _, c := range w.Party() {
		if c.Down() || turn-c.lastVision() < 3 || VisionChance(c) == 0 {
			continue
		}
		pool = append(pool, c)
	}
	if len(pool) == 0 {
		return nil, nil
	}
	c := pool[r.Intn(len(pool))]
	if r.Die(100) > VisionChance(c) {
		return nil, nil
	}
	v := visionFor(w, r, c, r.Die(100) > FalseChance(c))
	v.Turn = turn
	c.Visions = append(c.Visions, *v)
	if len(c.Visions) > MaxVisions {
		c.Visions = c.Visions[len(c.Visions)-MaxVisions:]
	}
	return c, &c.Visions[len(c.Visions)-1]
}

// visionFor builds a vision for c: a true one when wantReal and the world
// has something to show, else a false one.
func visionFor(w *World, r *dice.Roller, c *Character, wantReal bool) *Vision {
	if wantReal {
		if opts := trueVisions(w); len(opts) > 0 {
			return opts[r.Intn(len(opts))]
		}
	}
	opts := falseVisions(w, c)
	return opts[r.Intn(len(opts))]
}

// trueVisions are hints the world bears out.
func trueVisions(w *World) []*Vision {
	l := w.Here()
	var out []*Vision
	for _, x := range w.Exits(l.ID, true) {
		if x.Edge.State == EdgeHidden && x.To != nil {
			out = append(out, &Vision{Kind: VisionHiddenWay, Real: true,
				Seed: "a cold draft and a faint sound from somewhere you can't see a way through, as if this place has another way out (do not name where it leads)"})
			break
		}
	}
	for _, n := range w.NPCsAt(l.ID) {
		if n.Disposition <= -2 || n.Antagonist {
			out = append(out, &Vision{Kind: VisionHostile, Real: true,
				Seed: fmt.Sprintf("for an instant %s looks at you with open hatred, then the face is ordinary again", n.Name)})
		}
	}
	for _, x := range w.Exits(l.ID, false) {
		if x.To != nil && x.To.Danger >= 2 && x.To.Danger > l.Danger {
			out = append(out, &Vision{Kind: VisionDanger, Real: true,
				Seed: fmt.Sprintf("a certainty, cold in the gut, that something waits %s", firstNonEmpty(x.Label, "toward "+x.To.Name))})
			break
		}
	}
	return out
}

// falseVisions are hallucinations. There is always at least one.
func falseVisions(w *World, c *Character) []*Vision {
	l := w.Here()
	var out []*Vision
	for _, n := range w.NPCs {
		if !n.Alive && strings.TrimSpace(n.Name) != "" && n.LastTalked >= 0 {
			out = append(out, &Vision{Kind: VisionDead, Seed: fmt.Sprintf("%s, who is dead, standing among the people here and looking straight at you", n.Name)})
			break
		}
	}
	for _, n := range w.NPCs {
		if n.Alive && n.LocationID != l.ID && n.LastTalked >= 0 && strings.TrimSpace(n.Name) != "" {
			out = append(out, &Vision{Kind: VisionAbsent, Seed: fmt.Sprintf("%s, who is not here, passing by and glancing at you", n.Name)})
			break
		}
	}
	if here := w.NPCsAt(l.ID); len(here) > 0 {
		n := here[(w.Campaign.Turn+len(c.Visions))%len(here)]
		out = append(out, &Vision{Kind: VisionWrong, Seed: fmt.Sprintf(uncanny[(w.Campaign.Turn+len(c.Name))%len(uncanny)], n.Name)})
	}
	scene := strings.ToLower(l.Name + " " + l.Description)
	for _, n := range w.NPCsAt(l.ID) {
		scene += " " + strings.ToLower(n.Name+" "+n.Desc)
	}
	for i := range phantoms {
		p := phantoms[(i+w.Campaign.Turn)%len(phantoms)]
		if !allWordsIn(scene, p.Key) {
			out = append(out, &Vision{Kind: VisionPhantom, Seed: p.Seed, Phantom: p.Key})
			break
		}
	}
	out = append(out, &Vision{Kind: VisionWhisper, Seed: whispers[(w.Campaign.Turn+len(c.Visions))%len(whispers)]})
	return out
}

// allWordsIn reports whether every word of key appears in s.
func allWordsIn(s, key string) bool {
	for _, k := range strings.Fields(strings.ToLower(key)) {
		if !strings.Contains(s, k) {
			return false
		}
	}
	return true
}

// Unreal is a false vision still in memory, with who had it.
type Unreal struct {
	Character string
	Vision    Vision
}

// UnrealVisions lists the party's false visions from the last
// VisionMemory turns, oldest first per character.
func UnrealVisions(w *World) []Unreal {
	var out []Unreal
	for _, c := range w.Characters {
		for _, v := range c.Visions {
			if !v.Real && w.Campaign.Turn-v.Turn <= VisionMemory {
				out = append(out, Unreal{Character: c.Name, Vision: v})
			}
		}
	}
	return out
}
