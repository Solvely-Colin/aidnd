package gm

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/llm"
)

// proseCraft is how every narrator writes, whatever the setting: like a
// good novelist, not like a chatbot.
const proseCraft = `
Prose:
- Write like a good novelist, not like an AI. Concrete nouns and strong verbs, few adjectives. Every sentence clear on the first read.
- Show what is there and let the reader feel it. Do not name the mood ("eerie", "ominous", "tense", "unsettling", "a chill runs down your spine").
- No stock lines or aphorisms ("a ship doesn't dock itself", "the sea keeps its secrets", "some doors are best left closed"). Never end on a moral, a summary of the mood or a one-line zinger; end on something concrete in the scene.
- When someone speaks, give them a line of direct speech in their own voice, not a report of what they said. People sound like who they are: their trade, their age, where they grew up.
- Avoid these AI habits: em dashes (use commas or full stops), "not X but Y" and "it isn't X, it's Y", runs of three adjectives, "the air is thick with", "a testament to", "palpable", "the weight of", "seems to", "almost as if", "something about", "a heartbeat", giving weather and buildings feelings ("the patient harbor", "the house watches"), and several sentences that start the same way. At most one simile.`

// dreadVoice is the narrator's voice for horror campaigns: Lovecraft's
// manner, kept readable.
const dreadVoice = `
Voice (cosmic horror, in the manner of H. P. Lovecraft, but readable):
- The measured, slightly formal English of a 1920s or 1930s New England account. Set one longer sentence that builds against short plain ones. Old-fashioned, never purple: if a sentence needs reading twice, rewrite it.
- Dread comes from restraint. Pile up exact, ordinary details with one that is slightly wrong (a smell that should not be there, a gait, a sound under the floor, how a face is shaped) and let it sit. Never explain or name the horror, and never say it is horrible.
- Period texture: the things of the time and place (kerosene lamps, oilskins, trolleys, telegrams, Ford trucks, Prohibition liquor, dollars and cents) and the decay of old towns. No modern words or slang ("okay", "creepy", "vibe").
- The old Lovecraft words (eldritch, cyclopean, nameless, unspeakable, blasphemous, squamous, gibbous, ichor) at most once in a passage, and only when the thing on the page earns it.
- Locals talk like locals: flat, clipped Yankee speech ("Ain't nobody goes aboard her, mister"), a professor's dry precision, a cultist's too-calm courtesy. One or two lines, not speeches.`

// voice is the style block for a world's narration.
func voice(w *game.World) string {
	if w != nil && w.Campaign.Dread {
		return proseCraft + dreadVoice
	}
	return proseCraft
}

// styled appends the world's voice to a system prompt.
func styled(system string, w *game.World) string { return system + "\n" + voice(w) }

// tellPhrases are stock phrases that mark prose as machine-written.
var tellPhrases = regexp.MustCompile(`(?i)\b(a testament to|palpable|the air (?:is|was|hangs|hung) (?:thick|heavy)|thick with (?:tension|dread|anticipation)|the weight of (?:it|the|his|her|their|your|unspoken|silence)|almost as if|it'?s as if|as though the (?:very )?(?:air|world|town|house|sea)|something about (?:the|her|him|it|this)|sends? (?:a )?shivers? down|a chill (?:runs|ran|creeps|crept)|chill(?:s)? (?:to|down) (?:the|your) (?:bone|spine)|(?:eerie|ominous|unsettling|foreboding) (?:silence|feeling|sense|quiet|calm)|hangs? in the air|a heartbeat|for a beat|little did|tapestry|in the grand scheme|time seems to|you can'?t shake the feeling|sense of unease|an air of mystery)\b`)

// aphorism is a stock two-beat saying: "A ship doesn't dock itself, a
// logbook doesn't lie."
var aphorism = regexp.MustCompile(`(?i)\b(?:a|an|the)\s+\w+\s+(?:doesn't|does not|don't|never|can't|won't)\s+\w+[^.!?]{0,40}?\b(?:a|an|the)\s+\w+\s+(?:doesn't|does not|don't|never|can't|won't)\s+\w+`)

// notButCue is the "not X, but Y" / "isn't X, it's Y" turn.
var notButCue = regexp.MustCompile(`(?i)\b(?:(?:isn'?t|wasn'?t|is not|was not)\s+[\w' ]{1,30}[,;]\s*(?:it'?s|it is|it was)|not (?:just|merely|only)\s+[\w' ]{1,30}[,;]?\s*but)\b`)

// notButBare is the short form: "Not fear, but hunger." or "It is not
// fear but hunger." (a noun after "not", never "not much" or "not yet").
var notButBare = regexp.MustCompile(`(?i)(?:^|[.!?"“]\s*|\b(?:is|was|are|were)\s+)not\s+(?:a\s+|an\s+|the\s+)?(\w+),?\s+but\s+(?:a\s+|an\s+|the\s+)?\w+[.!?]`)

// purple are the Lovecraft words that read as pastiche when stacked.
var purple = regexp.MustCompile(`(?i)\b(eldritch|cyclopean|nameless|unspeakable|unnameable|indescribable|blasphemous|squamous|gibbous|ichor(?:ous)?|unholy|abyssal)\b`)

// tells lists the machine-prose habits found in text, worst first. An
// empty list means it reads clean.
func tells(text string) []string {
	var out []string
	for _, m := range tellPhrases.FindAllString(text, -1) {
		out = append(out, fmt.Sprintf("stock phrase %q", strings.ToLower(m)))
	}
	if m := aphorism.FindString(text); m != "" {
		out = append(out, fmt.Sprintf("stock aphorism %q: say it the way this person would", m))
	}
	if m := notButCue.FindString(text); m != "" {
		out = append(out, fmt.Sprintf("\"not X but Y\" construction %q", m))
	}
	if m := notButBare.FindString(text); m != "" && !notButCue.MatchString(text) {
		out = append(out, fmt.Sprintf("\"not X but Y\" construction %q", m))
	}
	for i, n := 0, dashes(text); i < n; i++ {
		out = append(out, fmt.Sprintf("em dash %d of %d: use commas or full stops", i+1, n))
	}
	if ms := purple.FindAllString(text, -1); len(ms) > 1 {
		out = append(out, fmt.Sprintf("stacked pastiche words (%s): keep at most one", strings.ToLower(strings.Join(ms, ", "))))
	}
	return out
}

func dashes(s string) int { return strings.Count(s, "—") + strings.Count(s, " -- ") }

const polishSystem = `You are a fiction editor. Fix only the listed problems in the passage. Keep every fact, name, number, roll result and line of speech; keep the voice, tense, point of view and length. Do not add people, items, places or events. Output only the passage.`

// polish asks for one rewrite when prose shows machine habits. It keeps
// the original unless the rewrite has fewer tells, keeps every name, and
// is close to the same length.
func (g *GM) polish(ctx context.Context, w *game.World, tag, text string) string {
	found := tells(text)
	if len(found) == 0 {
		return text
	}
	g.debugf("polish (%s): %s", tag, strings.Join(found, "; "))
	resp, err := g.LLM.Chat(ctx, llm.Request{Tag: "polish", Temperature: 0.4, MaxTokens: 500, Messages: []llm.Message{
		llm.System(polishSystem + "\n" + voice(w)),
		llm.User(fmt.Sprintf("PASSAGE:\n%s\n\nPROBLEMS:\n- %s", text, strings.Join(found, "\n- ")))}})
	out := strings.TrimSpace(resp.Content)
	if err != nil || out == "" {
		return text
	}
	n, m := wordCount(text), wordCount(out)
	if m*10 < n*6 || m*10 > n*14 || len(tells(out)) >= len(found) || !keepsNames(w, text, out) {
		return text
	}
	return out
}

var nameRe = regexp.MustCompile(`\b[A-Z][a-z]+(?:\s+[A-Z][a-z]+)*`)

// keepsNames reports whether every name in a still appears whole in b: the
// world's characters, people and places wherever they stand, and any other
// capitalised word inside a sentence (or several in a row anywhere).
func keepsNames(w *game.World, a, b string) bool {
	has := func(s, name string) bool {
		return regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`).MatchString(s)
	}
	if w != nil {
		var names []string
		for _, c := range w.Characters {
			names = append(names, strings.Fields(c.Name)...)
		}
		for _, n := range w.NPCs {
			names = append(names, strings.Fields(n.Name)...)
		}
		for _, l := range w.Locations {
			names = append(names, l.Name)
		}
		for _, n := range names {
			if len(n) > 2 && n[0] >= 'A' && n[0] <= 'Z' && has(a, n) && !has(b, n) {
				return false
			}
		}
	}
	for _, loc := range nameRe.FindAllStringIndex(a, -1) {
		name := a[loc[0]:loc[1]]
		before := strings.TrimRight(a[:loc[0]], " \"'“‘(")
		if (before == "" || strings.ContainsAny(before[len(before)-1:], ".!?:\n")) && !strings.Contains(name, " ") {
			continue // one word at a sentence start: may be any word
		}
		if !has(b, name) {
			return false
		}
	}
	return true
}
