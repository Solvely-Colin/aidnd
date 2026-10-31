package gm

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/llm"
)

// The canon guard reads model-written prose against the engine's state
// before players or the world's memory see it. The narrator prompt asks
// the model not to invent deaths, loot or arrivals; this is what makes it
// so. Every check is plain Go over the world, so a contradiction is found
// the same way every time.
//
// A narration with problems gets one rewrite that lists them. Whatever
// still contradicts the state after that is cut, sentence by sentence;
// if too little is left, the engine's own lines stand in.

// canonIssue is one sentence that contradicts the state.
type canonIssue struct {
	Sentence string
	Why      string
}

// canon is what a passage may say this turn, derived from the world.
type canon struct {
	here     *game.Location
	arrived  string            // location id the party reached this turn
	allowed  string            // lower-cased engine lines, player words and beat text for this turn
	npcs     []*game.NPC       // every named NPC
	parts    map[string]string // distinctive name part -> NPC id (only parts that name one NPC)
	items    []string          // lower-cased names of things that are here or carried
	gold     map[int]bool      // party gold totals (narrating "you have 42 gold" is fine)
	places   []*game.Location  // other places the party could be told it reached
	numbered string            // allowed text, for number checks

	// lifeOnly limits the checks to who is alive and dead, for text about
	// the past (summaries, recorded facts) where travel and trades that
	// already happened are fair to mention.
	lifeOnly bool
}

// newCanon builds the canon for w as it stands after this turn's
// resolution. changes are the engine's change lines this turn: people
// they name may die or arrive in the prose, and things they name may
// change hands. numbers is everything else the engine reported (rolls,
// facts); a damage or gold figure in the prose must appear there or in
// changes.
func newCanon(w *game.World, arrived string, changes []string, numbers ...string) *canon {
	c := &canon{here: w.Here(), arrived: arrived, parts: map[string]string{}, gold: map[int]bool{}}
	c.allowed = strings.ToLower(strings.Join(changes, "\n"))
	c.numbered = c.allowed + "\n" + strings.ToLower(strings.Join(numbers, "\n"))
	count := map[string]int{}
	for _, n := range w.NPCs {
		if strings.TrimSpace(n.Name) == "" {
			continue
		}
		c.npcs = append(c.npcs, n)
		for _, p := range nameParts(n.Name) {
			count[p]++
			c.parts[p] = n.ID
		}
	}
	for p, k := range count {
		if k > 1 {
			delete(c.parts, p) // shared by two people: only the full name counts
		}
	}
	for _, ch := range w.Party() {
		c.gold[ch.Gold] = true
		for _, it := range w.ItemsOf(game.OwnerCharacter, ch.ID) {
			c.items = append(c.items, strings.ToLower(it.Name))
		}
	}
	if c.here != nil {
		for _, it := range w.ItemsOf(game.OwnerLocation, c.here.ID) {
			c.items = append(c.items, strings.ToLower(it.Name))
		}
		for _, n := range w.NPCsAt(c.here.ID) {
			for _, it := range w.ItemsOf(game.OwnerNPC, n.ID) {
				c.items = append(c.items, strings.ToLower(it.Name))
			}
		}
	}
	for _, l := range w.Locations {
		if c.here != nil && l.ID == c.here.ID {
			continue
		}
		if len([]rune(l.Name)) >= 4 {
			c.places = append(c.places, l)
		}
	}
	return c
}

// commonParts are capitalised name parts that are also everyday words; a
// sentence-initial "Old" or "Grey" is not a name.
var commonParts = map[string]bool{"pale": true, "old": true, "grey": true, "gray": true, "black": true, "white": true, "red": true,
	"long": true, "little": true, "young": true, "tall": true, "big": true, "dark": true, "silent": true, "blind": true, "mad": true,
	"mother": true, "father": true, "sister": true, "brother": true, "doc": true, "doctor": true, "constable": true, "captain": true,
	"lady": true, "lord": true, "the": true, "mister": true, "madam": true, "sir": true, "miss": true, "mrs": true, "mr": true,
	"widow": true, "father's": true, "keeper": true, "deacon": true, "reverend": true, "professor": true, "officer": true, "agent": true}

// nameParts are the distinctive parts of a name ("Vane" in "Constable
// Vane"), lower-cased.
func nameParts(name string) []string {
	var out []string
	for _, p := range strings.Fields(name) {
		p = strings.Trim(p, ",.'\"()")
		lp := strings.ToLower(p)
		if len([]rune(p)) >= 3 && !commonParts[lp] && unicode.IsUpper([]rune(p)[0]) {
			out = append(out, lp)
		}
	}
	return out
}

// splitSentences splits prose into sentences, keeping closing quotes
// with the sentence they end.
func splitSentences(text string) []string {
	var out []string
	rs := []rune(strings.TrimSpace(text))
	start := 0
	for i := 0; i < len(rs); i++ {
		if !strings.ContainsRune(".!?…", rs[i]) {
			continue
		}
		j := i + 1
		for j < len(rs) && strings.ContainsRune("\"'”’)", rs[j]) {
			j++
		}
		if j < len(rs) && !unicode.IsSpace(rs[j]) {
			continue
		}
		k := j
		for k < len(rs) && unicode.IsSpace(rs[k]) {
			k++
		}
		if k < len(rs) && unicode.IsLower(rs[k]) {
			continue // "Not tonight!" she says.
		}
		if s := strings.TrimSpace(string(rs[start:j])); s != "" {
			out = append(out, s)
		}
		start = j
		i = j - 1
	}
	if s := strings.TrimSpace(string(rs[start:])); s != "" {
		out = append(out, s)
	}
	return out
}

// words lower-cases a sentence into bare words, keeping a possessive mark
// as its own token so "Ada's" can be told from "Ada".
func words(s string) []string {
	var out []string
	for _, f := range strings.Fields(s) {
		f = strings.TrimFunc(f, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '\'' && r != '’' })
		f = strings.ReplaceAll(f, "’", "'")
		f = strings.Trim(f, "'")
		if f == "" {
			continue
		}
		lf := strings.ToLower(f)
		if strings.HasSuffix(lf, "'s") {
			out = append(out, strings.TrimSuffix(lf, "'s"), "'s")
			continue
		}
		out = append(out, lf)
	}
	return out
}

// mentions returns the word positions where the sentence names n: the
// whole name, or a distinctive part that names nobody else. A possessive
// ("Ada's shop") is not a mention of the person doing anything.
func (c *canon) mentions(ws []string, n *game.NPC) [][2]int {
	full := words(n.Name)
	var at [][2]int
	for i := range ws {
		end := -1
		if len(full) > 0 && i+len(full) <= len(ws) && equalWords(ws[i:i+len(full)], full) {
			end = i + len(full)
		} else if id, ok := c.parts[ws[i]]; ok && id == n.ID {
			end = i + 1
		}
		if end < 0 {
			continue
		}
		if end < len(ws) && ws[end] == "'s" {
			continue
		}
		at = append(at, [2]int{i, end})
	}
	return at
}

// dies reports whether a mention is written as dying or dead: "Ada
// slumps, lifeless", "the body of Ada".
func dies(ws []string, at [][2]int) bool {
	for _, m := range at {
		var aw []string
		for j := m[1]; j < len(ws) && j < m[1]+6; j++ {
			if actVerbs[ws[j]] || deathStops[ws[j]] {
				break // a new clause: what follows is about someone else
			}
			aw = append(aw, ws[j])
		}
		after := strings.Join(aw, " ")
		before := strings.Join(ws[max(0, m[0]-3):m[0]], " ")
		if deathRe.MatchString(after) || strings.Contains(before, "body of") || strings.Contains(before, "corpse of") || strings.HasSuffix(before, "dead") {
			return true
		}
	}
	return false
}

func equalWords(a, b []string) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// actVerbs are present-tense things a person does in front of the party.
var actVerbs = wordSet("says", "asks", "replies", "answers", "whispers", "mutters", "murmurs", "shouts", "calls", "cries", "laughs", "smiles",
	"grins", "nods", "shrugs", "frowns", "sighs", "steps", "walks", "strides", "arrives", "enters", "approaches", "hands", "gives", "offers",
	"points", "leads", "beckons", "waves", "greets", "watches", "turns", "leans", "stands", "sits", "rises", "looks", "glances", "reaches",
	"grabs", "pulls", "pushes", "tells", "warns", "explains", "insists", "agrees", "refuses", "slides", "presses", "opens", "speaks",
	"emerges", "appears", "joins", "growls", "hisses", "snaps", "gestures", "lifts", "raises", "holds", "studies", "eyes", "waits",
	"pauses", "adds", "continues", "interrupts", "chuckles", "stares", "clutches", "shakes", "hurries", "rushes", "runs", "comes")

// recollection marks a sentence about the past or about someone's
// memory, picture or remains, where an absent or dead person may appear.
var recollection = wordSet("remember", "remembers", "remembered", "recall", "recalls", "once", "earlier", "yesterday", "memory",
	"memories", "told", "grave", "ghost", "spirit", "portrait", "photograph", "photo", "letter", "name", "named", "corpse", "body",
	"remains", "dead", "late", "mourn", "mourns", "buried", "funeral", "rumor", "rumour", "rumors", "rumours", "heard", "dream", "dreams",
	"vision", "visions", "voice", "echo", "echoes", "used", "had", "would", "said")

func wordSet(ws ...string) map[string]bool {
	m := make(map[string]bool, len(ws))
	for _, w := range ws {
		m[w] = true
	}
	return m
}

// deathStops end the words read after a name for how that person fares.
var deathStops = wordSet("that", "about", "how", "when", "where", "why", "who", "whose", "he", "she", "they", "it", "i", "you", "we", "him", "her", "them")

// clauseWords start a new clause or noun phrase: a verb after them has
// another subject ("Whatever happened to Tom, this place gives nothing").
var clauseWords = wordSet("this", "these", "those", "a", "an", "the", "that", "about", "how", "when", "where", "why", "who", "whose", "and", "but",
	"he", "she", "they", "it", "i", "you", "we", "him", "her", "them")

// acts reports whether one of the mentions is followed closely by
// something the person does ("Ada, the innkeeper, nods").
func acts(ws []string, at [][2]int) bool {
	for _, m := range at {
		end := m[1]
		for j := end; j < len(ws) && j < end+5; j++ {
			if actVerbs[ws[j]] {
				return true
			}
			if clauseWords[ws[j]] {
				break // "Tom Coyle," she says: the verb is hers
			}
		}
	}
	return false
}

func anyWord(ws []string, set map[string]bool, except ...string) bool {
	skip := wordSet(except...)
	for _, w := range ws {
		if set[w] && !skip[w] {
			return true
		}
	}
	return false
}

var (
	deathRe   = regexp.MustCompile(`(?i)\b(dies|died|is dead|lies dead|lay dead|lying dead|drops dead|falls dead|was killed|is killed|were killed|been killed|is slain|was slain|lifeless|breathes? (his|her|their|its) last|corpse of|body of|dead body)\b`)
	acquireRe = regexp.MustCompile(`(?i)\b(pockets?|pocketed|picks? up|picked up|scoops? up|receives?|received|hands? (you|over|it to)|gives? you|passes? you|slips? you|press(es)? [^.!?]{0,30}into (your|their|his|her) (hand|palm)s?|tucks? [^.!?]{0,30}into (your|their)|you (now )?(carry|hold) (a|an|the)|\b(finds?|found|discovers?|uncovers?|turns? up|comes? up with|hides?|hid|hidden|hiding|conceals?|concealed|reveals?|revealed|yields?|slides?|tosses?|pushes?) (you |her |him |them |over )?(only )?(a|an|some)\b[^.!?]{0,40}\b(key|keys|coin|coins|gold|dagger|knife|sword|blade|pistol|revolver|gun|rifle|shotgun|ring|amulet|locket|letter|note|map|journal|ledger|book|scroll|potion|vial|lantern|idol|bottle|pouch|purse|gem|jewel|medallion|talisman|charm|tome|diary|photograph|ammunition|bullets|cash|money|wallet|satchel|relic|statuette))`)
	moneyRe   = regexp.MustCompile(`(?i)(\$\s?(\d+)|\b(\d+)\s+(gold|gold pieces|coins?|gp|dollars|bucks|silver|crowns|credits)\b)`)
	damageRe  = regexp.MustCompile(`(?i)\b(\d+)\s+(points? of\s+)?(damage|hit points?|hp|wounds?)\b`)
	arriveRe  = regexp.MustCompile(`(?i)\b(you|they|the party|we|the group)\s+(now\s+)?(arrive|arrives|reach|reaches|enter|enters|step into|walk into|come to|stride into|find yourselves|find themselves|stand|are now|are in|are at)\b`)
)

// check returns every sentence of text that contradicts the state.
func (c *canon) check(text string) []canonIssue {
	var issues []canonIssue
	for _, s := range splitSentences(text) {
		if why := c.checkSentence(s); why != "" {
			issues = append(issues, canonIssue{Sentence: s, Why: why})
		}
	}
	return issues
}

func (c *canon) checkSentence(s string) string {
	ws := words(s)
	ls := strings.ToLower(s)
	for _, n := range c.npcs {
		at := c.mentions(ws, n)
		if len(at) == 0 {
			continue
		}
		inAllowed := strings.Contains(c.allowed, strings.ToLower(n.Name))
		switch {
		case !n.Alive && !inAllowed && acts(ws, at) && !anyWord(ws, recollection):
			return fmt.Sprintf("%s is dead; the dead don't speak or act", n.Name)
		case n.Alive && !inAllowed && dies(ws, at):
			return fmt.Sprintf("%s is alive; nothing killed them", n.Name)
		case n.Alive && !inAllowed && !c.lifeOnly && c.here != nil && n.LocationID != c.here.ID && acts(ws, at) &&
			!anyWord(ws, recollection) && !strings.Contains(ls, strings.ToLower(placeName(c, n.LocationID))):
			return fmt.Sprintf("%s is not here (they are at %s); they can't speak or act in this scene", n.Name, placeName(c, n.LocationID))
		}
	}
	if c.lifeOnly {
		return ""
	}
	for _, m := range moneyRe.FindAllStringSubmatch(s, -1) {
		v := firstOf(m[2], m[3])
		if n, err := strconv.Atoi(v); err == nil && n > 0 && !c.gold[n] && !hasNumber(c.numbered, v) {
			return fmt.Sprintf("no %s gold or money changed hands this turn", v)
		}
	}
	if acquireRe.MatchString(s) && !c.namesKnownItem(ls) {
		return "nobody got that: no item changed hands this turn, and only things the party carries, things lying here and the engine's CHANGES exist"
	}
	for _, m := range damageRe.FindAllStringSubmatch(s, -1) {
		if !hasNumber(c.numbered, m[1]) {
			return fmt.Sprintf("the engine rolled no %s damage", m[1])
		}
	}
	if arriveRe.MatchString(s) {
		for _, l := range c.places {
			if l.ID != c.arrived && strings.Contains(ls, strings.ToLower(l.Name)) && !strings.Contains(c.allowed, strings.ToLower(l.Name)) {
				return fmt.Sprintf("the party is at %s, not %s", placeName(c, ""), l.Name)
			}
		}
	}
	return ""
}

func placeName(c *canon, id string) string {
	if id == "" && c.here != nil {
		return c.here.Name
	}
	for _, l := range c.places {
		if l.ID == id {
			return l.Name
		}
	}
	if c.here != nil && c.here.ID == id {
		return c.here.Name
	}
	return "elsewhere"
}

// namesKnownItem reports whether a sentence names something the party
// carries, something here, or something the engine reported this turn.
func (c *canon) namesKnownItem(ls string) bool {
	for _, n := range c.items {
		if n != "" && strings.Contains(ls, n) {
			return true
		}
		if f := strings.Fields(n); len(f) > 1 {
			// "the key" can be the Rusty Key; "a heavy iron key" is a new one
			if last := f[len(f)-1]; len(last) >= 3 && regexp.MustCompile(`\b(the|your|their|his|her|its|that|this|my)\s+`+regexp.QuoteMeta(last)+`\b`).MatchString(ls) {
				return true
			}
		}
	}
	// the engine's lines this turn may name what changed hands
	for _, w := range words(ls) {
		if len(w) >= 4 && !stopItemWords[w] && hasWordIn(c.allowed, w) && itemish[w] {
			return true
		}
	}
	return false
}

var itemish = wordSet("key", "keys", "coin", "coins", "gold", "dagger", "knife", "sword", "blade", "pistol", "revolver", "rifle", "shotgun",
	"ring", "amulet", "locket", "letter", "note", "map", "journal", "ledger", "book", "scroll", "potion", "vial", "lantern", "idol", "bottle",
	"pouch", "purse", "gem", "jewel", "medallion", "talisman", "charm", "tome", "diary", "photograph", "ammunition", "bullets", "cash",
	"money", "wallet", "satchel", "relic", "statuette", "kit", "rope", "torch", "shield", "armor", "armour", "coat", "bow", "crossbow")

var stopItemWords = wordSet("your", "their", "with", "from", "into", "that", "this")

func hasWordIn(hay, w string) bool {
	for _, x := range words(hay) {
		if x == w {
			return true
		}
	}
	return false
}

func hasNumber(hay, n string) bool {
	re := regexp.MustCompile(`(^|[^0-9])` + regexp.QuoteMeta(n) + `([^0-9]|$)`)
	return re.MatchString(hay)
}

// cut drops the sentences that have issues.
func cut(text string, issues []canonIssue) string {
	bad := map[string]bool{}
	for _, i := range issues {
		bad[i.Sentence] = true
	}
	var keep []string
	for _, s := range splitSentences(text) {
		if !bad[s] {
			keep = append(keep, s)
		}
	}
	return strings.Join(keep, " ")
}

func issueList(issues []canonIssue) string {
	var b strings.Builder
	for _, i := range issues {
		fmt.Fprintf(&b, "- %q: %s\n", i.Sentence, i.Why)
	}
	return b.String()
}

const repairSystem = `You fix continuity errors in a tabletop game's narration. The game's rules engine holds the truth; the passage contradicts it in the listed places.
Rewrite the passage so it no longer says any listed thing. Change as little as possible: keep every other sentence, the voice, tense and length. Do not add new people, items, places, damage or events. Output only the passage.`

// guard checks prose against the canon. It returns the text to show and
// the problems it found (empty when the prose was clean). fallback is the
// engine's own account, used if fixing the prose leaves too little.
func (g *GM) guard(ctx context.Context, c *canon, tag, text, fallback string) (string, []canonIssue) {
	issues := c.check(text)
	if len(issues) == 0 {
		return text, nil
	}
	g.debugf("canon guard (%s): %d issue(s)\n%s", tag, len(issues), issueList(issues))
	resp, err := g.LLM.Chat(ctx, llm.Request{Tag: "canon_repair", Temperature: 0.3, MaxTokens: 400, Messages: []llm.Message{
		llm.System(repairSystem),
		llm.User(fmt.Sprintf("PASSAGE:\n%s\n\nCONTRADICTIONS (the engine is right):\n%s", text, issueList(issues)))}})
	fixed := strings.TrimSpace(resp.Content)
	if err == nil && fixed != "" {
		left := c.check(fixed)
		if len(left) == 0 {
			return fixed, issues
		}
		text, issues = fixed, left
	}
	out := cut(text, issues)
	if wordCount(out) < 8 {
		out = strings.TrimSpace(strings.TrimSpace(fallback) + " " + out)
	}
	if strings.TrimSpace(out) == "" {
		out = fallback
	}
	return out, issues
}

// issueTexts flattens issues for an event log.
func issueTexts(is []canonIssue) []string {
	var out []string
	for _, i := range is {
		out = append(out, i.Why+": "+i.Sentence)
	}
	return out
}
