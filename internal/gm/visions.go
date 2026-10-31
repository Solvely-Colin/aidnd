package gm

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/llm"
	"github.com/Solvely-Colin/aidnd/internal/store"
)

// EventVision is a private perception: only the player in its "for" field
// may read it (the API filters it out for everyone else).
const EventVision = "vision"

const visionSystem = `You write one private moment of perception in a horror game. Only this one player will read it; nobody else in the scene notices.
Rules:
- 1 or 2 sentences, at most 40 words, second person ("you"), present tense.
- Describe exactly what the SEED says as a sensory impression: what they see, hear, smell or feel. Name anyone the SEED names. Add nothing else: no new people, items, places or events, no damage.
- Never say or hint whether it is real, imagined, a hallucination, a dream or a trick of the light. Never say that others don't notice. Do not explain it.
- No game words, no questions, no advice.`

// vision may give one party member a private perception this turn. It
// returns the event to commit (nil when nobody has one).
func (g *GM) vision(ctx context.Context, w *game.World) []store.Event {
	c, v := game.RollVision(w, g.Dice)
	if v == nil {
		return nil
	}
	text := g.visionText(ctx, w, c, v)
	v.Text = text
	return []store.Event{store.NewEvent(EventVision, "", map[string]any{"for": c.PlayerID, "character_id": c.ID, "text": text})}
}

// visionText has the model put a vision's seed into words. A true vision
// is checked against the world like any narration; if the prose strays,
// the seed itself is shown.
func (g *GM) visionText(ctx context.Context, w *game.World, c *game.Character, v *game.Vision) string {
	fallback := visionFallback(v)
	l := w.Here()
	resp, err := g.LLM.Chat(ctx, llm.Request{Tag: "vision", Temperature: 0.9, MaxTokens: 160, Messages: []llm.Message{
		llm.System(visionSystem),
		llm.User(fmt.Sprintf("CHARACTER: %s (%s)\nPLACE: %s. %s\nSEED: %s", c.Name, game.NormPronouns(c.Pronouns), l.Name, trim(l.Description, 300), v.Seed)),
	}})
	text := strings.TrimSpace(resp.Content)
	if err != nil || text == "" || wordCount(text) > 70 || unrealWords.MatchString(text) {
		return fallback
	}
	if v.Real {
		if is := newCanon(w, "", []string{v.Seed}, v.Seed).check(text); len(is) > 0 {
			g.debugf("vision canon issues:\n%s", issueList(is))
			return fallback
		}
	}
	return text
}

// visionFallback is the seed as a plain sentence.
func visionFallback(v *game.Vision) string {
	seed := v.Seed
	if i := strings.Index(seed, " (do not"); i >= 0 {
		seed = seed[:i]
	}
	lead := "You see "
	switch v.Kind {
	case game.VisionWhisper:
		lead = "You hear "
	case game.VisionHiddenWay:
		lead = "You feel "
	case game.VisionDanger:
		lead = "You feel "
	case game.VisionWrong:
		lead = "For a moment, "
	}
	return lead + strings.TrimRight(seed, ".") + "."
}

// unrealBlock tells the narrator which of the party's recent visions were
// false, so a player who acts on one finds nothing. It is not shown to
// players and never written into memory.
func unrealBlock(w *game.World) string {
	us := game.UnrealVisions(w)
	if len(us) == 0 {
		return ""
	}
	us = lastN(us, 6)
	var b strings.Builder
	b.WriteString("\nUNREAL (things a character under strain alone saw or heard; they are NOT real. Never describe them as present or true, and never bring them up yourself. If a player acts on one, they find nothing there):\n")
	for _, u := range us {
		fmt.Fprintf(&b, "- %s: %s\n", u.Character, strings.TrimSuffix(u.Vision.Seed, "."))
	}
	return b.String()
}

// phantomIn returns the phantom a sentence puts in the scene, if any.
func (c *canon) phantomIn(ws []string, ls string) string {
	for _, p := range c.phantoms {
		if allWords(ls, p) && !anyWord(ws, doubt) {
			return p
		}
	}
	return ""
}

// phantomOf returns the false vision's phantom that text names, if any.
func phantomOf(w *game.World, text string) string {
	lt := strings.ToLower(text)
	for _, u := range game.UnrealVisions(w) {
		if p := u.Vision.Phantom; p != "" && allWords(lt, p) {
			return p
		}
	}
	return ""
}

// allWords reports whether every word of key is a word of s.
func allWords(s, key string) bool {
	ws := wordSet(words(s)...)
	for _, k := range strings.Fields(strings.ToLower(key)) {
		if !ws[k] {
			return false
		}
	}
	return true
}

// doubt marks a sentence that denies or only reports a phantom ("there is
// no drowned woman", "Cal swears he saw a drowned woman").
var doubt = wordSet("no", "not", "nobody", "nothing", "none", "never", "empty", "gone", "isn't", "wasn't", "aren't", "weren't",
	"imagined", "imagine", "imagines", "trick", "vanished", "vanishes", "vanish", "only", "claims", "claimed", "swears", "swore",
	"thinks", "thought", "believes", "believed", "insists", "insisted", "dream", "dreamed", "dreamt", "seems", "seemed")

// unrealWords give away whether a vision is real; such prose is replaced.
var unrealWords = regexp.MustCompile(`(?i)\b(hallucinat\w*|imagin\w*|tricks? of|not real|isn't real|unreal|no one else|nobody else|none of the others|the others don't|figment|mind plays)\b`)
