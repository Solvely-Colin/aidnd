package gm

import (
	"context"
	"fmt"
	"strings"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/llm"
)

const narratorSystem = `You are the narrator of a turn-based tabletop game. A rules engine has ALREADY resolved what happened; you describe it.
Rules:
- The engine's ROLLS and CHANGES are true. Never contradict them, never add hits, damage, deaths, loot or gold that the engine did not report. If a step FAILED, nothing happened for it. SKIPPED steps did not happen: leave them out, and do not invent an obstacle to explain them. Nobody moves to another place unless the engine says the party arrived. Only mention items that STATE lists here (GROUND, inventories, stock); things seen elsewhere earlier are not here.
- Refer to each character with the pronouns listed after their name.
- Never decide what a player character does, says, thinks or feels beyond what the player stated.
- Enemies act only through engine-resolved lines. Do not invent enemy movement, attacks or hits.
- Do not end with a question or a menu of options ("What do you do? A, B or C"). End on the situation.
- Do not hand the spotlight to the next player or say whose turn it is.
- 40-100 words, plain language, lead with what happened. Second person plural or character names; present tense.
- You may change the world only through the provided directive tools; code validates them. You cannot change player HP or gold.
- Use the DIRECTOR'S NOTE to steer: play any beat it names, keep the scene goal in view.`

const quickSystem = `You are the narrator of a tabletop game. The player took only free actions (looking, a remark, readying gear).
Answer briefly in at most 60 words using only the facts given. Do not invent new threats or events. Nobody arrives anywhere, leaves, fights or gets anything unless WHAT HAPPENED says so. Use each character's listed pronouns. No questions, no menus, do not mention turns.`

// narration is the narrator's output for one turn.
type narration struct {
	Text    string
	Changes []string // world changes from directives (and the pacing backstop)
}

func wordCount(s string) int { return len(strings.Fields(s)) }

func loreBlock(ls []*game.Lore) string {
	if len(ls) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("CANON LORE (secret; use for consistency, reveal only through play):\n")
	for _, l := range ls {
		fmt.Fprintf(&b, "- %s: %s\n", l.Title, l.Text)
	}
	return b.String()
}

func (g *GM) narrate(ctx context.Context, w *game.World, actor *game.Character, text string, res *game.Resolution, dir Direction, recent []string, quick bool) narration {
	var out narration
	happened := describeResolution(res, w)
	if quick {
		req := llm.Request{Tag: "narrate_quick", Temperature: 0.6, MaxTokens: 200, Messages: []llm.Message{
			llm.System(quickSystem),
			llm.User(fmt.Sprintf("%s\nPLAYER (%s): %s\n\nWHAT HAPPENED:\n%s", snapshot(w, actor, false), actor.Name, text, happened)),
		}}
		resp, err := g.LLM.Chat(ctx, req)
		if err == nil {
			out.Text = strings.TrimSpace(resp.Content)
		}
		if wordCount(out.Text) > 60 {
			out.Text = g.tighten(ctx, out.Text, 60)
		}
		if out.Text == "" {
			out.Text = fallbackNarration(res)
		}
		return out
	}
	var b strings.Builder
	b.WriteString(loreBlock(dir.Lore))
	if w.Campaign.Summary != "" {
		fmt.Fprintf(&b, "STORY SO FAR: %s\n", w.Campaign.Summary)
	}
	b.WriteString("\nSTATE:\n")
	b.WriteString(snapshot(w, actor, true))
	if len(recent) > 0 {
		fmt.Fprintf(&b, "\nRECENT LOG:\n%s\n", strings.Join(recent, "\n"))
	}
	fmt.Fprintf(&b, "\nPLAYER (%s) SAID: %s\nWHAT HAPPENED (engine):\n%s\n", actor.Name, text, happened)
	b.WriteString(dir.Note())
	tools := directiveTools()
	if w.InCombat() || res.CombatStarted {
		tools = filterTools(tools, map[string]bool{DirEndEncounter: true, DirNPCNote: true, DirRecordLore: true, DirAdjustTension: true, DirAdjustDisp: true})
	}
	req := llm.Request{Tag: "narrate", Temperature: 0.8, MaxTokens: 600, Tools: tools,
		Messages: []llm.Message{llm.System(narratorSystem), llm.User(b.String())}}
	if dir.ForceProgress {
		req.Tools = filterTools(directiveTools(), progressDirectives)
		req.RequireTool = true
	}
	resp, err := g.LLM.Chat(ctx, req)
	if err != nil {
		g.logf("narrate: %v", err)
	}
	calls := resp.ToolCalls
	if len(calls) == 0 && resp.Content != "" && strings.HasPrefix(strings.TrimSpace(resp.Content), "{") {
		calls = llm.SalvageToolCalls(resp.Content, req.Tools)
		if len(calls) > 0 {
			resp.Content = ""
		}
	}
	allowed := map[string]bool{}
	for _, t := range req.Tools {
		allowed[t.Name] = true
	}
	progressed := false
	var results []llm.Message
	for i, c := range calls {
		if c.ID == "" {
			calls[i].ID = fmt.Sprintf("dir_%d", i)
		}
		msg := "not allowed now"
		if allowed[c.Function.Name] {
			change, err := g.applyDirective(ctx, w, c)
			if err != nil {
				msg = "rejected: " + err.Error()
				g.debugf("directive %s rejected: %v", c.Function.Name, err)
			} else {
				msg = "applied: " + change
				if change != "" {
					out.Changes = append(out.Changes, change)
				}
				if progressDirectives[c.Function.Name] {
					progressed = true
				}
			}
		}
		results = append(results, llm.ToolResult(calls[i].ID, msg))
	}
	if dir.ForceProgress && !progressed {
		if ch := forceProgress(w); ch != "" {
			out.Changes = append(out.Changes, ch)
			g.debugf("pacing backstop: %s", ch)
		}
	}
	if progressed || (dir.ForceProgress && len(out.Changes) > 0) {
		w.Campaign.Scene.Quiet = 0
	}
	textOut := strings.TrimSpace(resp.Content)
	backstopped := dir.ForceProgress && !progressed && len(out.Changes) > 0
	if textOut == "" || backstopped {
		msgs := append([]llm.Message{}, req.Messages...)
		if len(calls) > 0 {
			msgs = append(msgs, llm.Message{Role: "assistant", Content: resp.Content, ToolCalls: calls})
			msgs = append(msgs, results...)
		}
		extra := ""
		if len(out.Changes) > 0 {
			extra = "\nWorld changes now in effect (mention them): " + strings.Join(out.Changes, "; ")
		}
		msgs = append(msgs, llm.User("Now write the narration (40-100 words) for everything above."+extra))
		r2, err := g.LLM.Chat(ctx, llm.Request{Tag: "narrate", Temperature: 0.8, MaxTokens: 600, Messages: msgs})
		if err == nil && strings.TrimSpace(r2.Content) != "" {
			textOut = strings.TrimSpace(r2.Content)
		}
	}
	if wordCount(textOut) > 160 {
		textOut = g.tighten(ctx, textOut, 100)
	}
	if textOut == "" {
		textOut = fallbackNarration(res)
		if len(out.Changes) > 0 {
			textOut += " " + strings.Join(out.Changes, ". ") + "."
		}
	}
	out.Text = textOut
	return out
}

func (g *GM) tighten(ctx context.Context, text string, words int) string {
	resp, err := g.LLM.Chat(ctx, llm.Request{Tag: "tighten", Temperature: 0.3, MaxTokens: 300, Messages: []llm.Message{
		llm.System(fmt.Sprintf("Condense the passage to at most %d words. Keep every fact, roll result and name. Plain language. No questions.", words)),
		llm.User(text)}})
	if err != nil || strings.TrimSpace(resp.Content) == "" {
		f := strings.Fields(text)
		if len(f) > words+20 {
			return strings.Join(f[:words+20], " ") + "…"
		}
		return text
	}
	return strings.TrimSpace(resp.Content)
}

// fallbackNarration states the engine's changes when the model fails.
func fallbackNarration(res *game.Resolution) string {
	var parts []string
	for _, s := range res.Steps {
		if s.Error != "" {
			parts = append(parts, s.Error)
		}
		parts = append(parts, s.Changes...)
		if len(s.Changes) == 0 && len(s.Info) > 0 {
			parts = append(parts, s.Info...)
		}
	}
	if len(parts) == 0 {
		return "A moment passes."
	}
	if len(parts) > 6 {
		parts = parts[:6]
	}
	for i, p := range parts {
		parts[i] = strings.TrimRight(strings.TrimSpace(p), ".")
	}
	return strings.Join(parts, ". ") + "."
}

// forceProgress is the pacing backstop: reveal a hidden exit here, or add
// a new path.
func forceProgress(w *game.World) string {
	l := w.Here()
	for _, x := range w.Exits(l.ID, true) {
		if x.Edge.State == game.EdgeHidden && game.RevealExit(w, x.Edge.ID) == nil {
			return fmt.Sprintf("A hidden way is revealed: %s", firstOf(x.Label, "to "+x.To.Name))
		}
	}
	if len(w.Locations) >= game.MaxLocations {
		return ""
	}
	name := fmt.Sprintf("Trail beyond %s", l.Name)
	if w.Location(name) != nil {
		name = fmt.Sprintf("Path %d beyond %s", w.Campaign.Seq, l.Name)
	}
	stub := w.AddStub(name, "A way onward nobody had noticed before.", game.KindWilderness, max(1, min(4, l.Danger)))
	e := w.Connect(l.ID, stub.ID, game.EdgeKnown, "a newly noticed path", "back to "+l.Name)
	if len(l.Zones) > 0 {
		e.SetZoneAt(l.ID, l.Zones[0].ID)
	}
	return "A new path is noticed: " + stub.Name
}

// applyDirective validates and applies one narrator directive.
func (g *GM) applyDirective(ctx context.Context, w *game.World, c llm.ToolCall) (string, error) {
	l := w.Here()
	switch c.Function.Name {
	case DirSpawnEncounter:
		var a struct {
			Enemies []game.EnemySpec `json:"enemies"`
		}
		if err := c.Decode(&a); err != nil {
			return "", err
		}
		if w.InCombat() {
			return "", fmt.Errorf("a fight is already under way")
		}
		if l.Danger == 0 && l.IsHub {
			return "", fmt.Errorf("no fights break out in the safe hub")
		}
		if len(a.Enemies) > 3 {
			a.Enemies = a.Enemies[:3]
		}
		for i := range a.Enemies {
			if n := w.NPC(a.Enemies[i].NPCID); n == nil || n.LocationID != l.ID || n.Antagonist {
				a.Enemies[i].NPCID = ""
			}
		}
		enc, notes, err := game.StartEncounter(w, a.Enemies)
		if err != nil {
			return "", err
		}
		if len(notes) > 0 {
			g.debugf("spawn clamped: %v", notes)
		}
		var names []string
		for _, e := range enc.Enemies {
			names = append(names, e.Display())
		}
		return "Enemies appear: " + strings.Join(names, ", "), nil
	case DirOfferQuest:
		var a game.QuestSpec
		if err := c.Decode(&a); err != nil {
			return "", err
		}
		if n := w.NPC(a.GiverID); n == nil || n.LocationID != l.ID {
			return "", fmt.Errorf("giver must be an NPC present")
		}
		open := 0
		for _, q := range w.Quests {
			if !q.IsMain && (q.Status == game.QuestOffered || q.Status == game.QuestActive) {
				open++
			}
		}
		if open >= 4 {
			return "", fmt.Errorf("too many open side quests")
		}
		q, err := game.AddQuest(w, a, false)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s offers a quest: %s", w.NPC(q.GiverID).Name, q.Title), nil
	case DirRevealExit:
		var a struct {
			ExitID string `json:"exit_id"`
		}
		if err := c.Decode(&a); err != nil {
			return "", err
		}
		e := w.Edge(a.ExitID)
		if e == nil || !e.Touches(l.ID) {
			return "", fmt.Errorf("no hidden exit %q here", a.ExitID)
		}
		if err := game.RevealExit(w, e.ID); err != nil {
			return "", err
		}
		return fmt.Sprintf("A hidden way is revealed: %s", firstOf(e.LabelAt(l.ID), "to "+w.Location(e.Other(l.ID)).Name)), nil
	case DirAddExit:
		var a game.ExitSpec
		var raw struct {
			Name string `json:"name"`
		}
		if err := c.Decode(&a); err != nil {
			return "", err
		}
		_ = c.Decode(&raw)
		a.To = firstOf(a.To, raw.Name)
		if strings.TrimSpace(a.To) == "" {
			return "", fmt.Errorf("name is required")
		}
		if dest := w.Location(a.To); dest != nil && game.NameMatch(dest.Name, a.To) {
			if dest.IsLair || dest.ID == l.ID || w.EdgeBetween(l.ID, dest.ID) != nil {
				return "", fmt.Errorf("cannot add a path to %s", dest.Name)
			}
			e := w.Connect(l.ID, dest.ID, game.EdgeKnown, trim(a.Label, 80), "")
			if len(l.Zones) > 0 {
				e.SetZoneAt(l.ID, l.Zones[0].ID)
			}
			return "A path to " + dest.Name + " is found", nil
		}
		if len(w.Locations) >= game.MaxLocations {
			return "", fmt.Errorf("the world is large enough")
		}
		stub := w.AddStub(a.To, a.Hint, a.Kind, clampInt(a.Danger, max(0, l.Danger-1), min(4, l.Danger+1)))
		e := w.Connect(l.ID, stub.ID, game.EdgeKnown, firstOf(trim(a.Label, 80), "the way to "+stub.Name), "back to "+l.Name)
		if len(l.Zones) > 0 {
			e.SetZoneAt(l.ID, l.Zones[0].ID)
		}
		return "A new way opens: " + stub.Name, nil
	case DirPlaceItem:
		var a game.ItemSpec
		if err := c.Decode(&a); err != nil {
			return "", err
		}
		if len(w.ItemsOf(game.OwnerLocation, l.ID)) >= game.MaxGroundItems+2 {
			return "", fmt.Errorf("enough items lie here")
		}
		a.Value = clampInt(a.Value, 0, 25+25*l.Danger)
		if d, ok := game.LookupDef(a.Key); ok && d.Value > 25+25*l.Danger {
			return "", fmt.Errorf("%s is too valuable to leave lying here", d.Name)
		}
		it := game.PlaceItem(w, a, game.Owner{Kind: game.OwnerLocation, ID: l.ID})
		if it == nil {
			return "", fmt.Errorf("unknown item")
		}
		if it.Value > 25+25*l.Danger {
			it.Value = 25 + 25*l.Danger
		}
		return "On the ground: " + it.Name, nil
	case DirStockMerchant:
		var a struct {
			NPCID string   `json:"npc_id"`
			Keys  []string `json:"keys"`
		}
		if err := c.Decode(&a); err != nil {
			return "", err
		}
		n := w.NPC(a.NPCID)
		if n == nil || !n.Merchant || n.LocationID != l.ID {
			return "", fmt.Errorf("no merchant %q here", a.NPCID)
		}
		have := len(w.ItemsOf(game.OwnerNPC, n.ID))
		var added []string
		for _, k := range a.Keys {
			if have >= game.MaxMerchantStock+4 || len(added) >= 4 {
				break
			}
			if d, ok := game.LookupDef(k); ok {
				game.NewItem(w, d.Key, game.Owner{Kind: game.OwnerNPC, ID: n.ID})
				added = append(added, d.Name)
				have++
			}
		}
		if len(added) == 0 {
			return "", fmt.Errorf("no valid catalog keys")
		}
		return fmt.Sprintf("%s now stocks %s", n.Name, strings.Join(added, ", ")), nil
	case DirNPCNote:
		var a struct {
			NPCID string `json:"npc_id"`
			Note  string `json:"note"`
		}
		if err := c.Decode(&a); err != nil {
			return "", err
		}
		n := w.NPC(a.NPCID)
		if n == nil {
			return "", fmt.Errorf("unknown npc %q", a.NPCID)
		}
		n.Remember(trim(a.Note, 140))
		return "", nil
	case DirAdjustDisp:
		var a struct {
			NPCID string `json:"npc_id"`
			Delta int    `json:"delta"`
		}
		if err := c.Decode(&a); err != nil {
			return "", err
		}
		n := w.NPC(a.NPCID)
		if n == nil || n.LocationID != l.ID {
			return "", fmt.Errorf("no npc %q here", a.NPCID)
		}
		n.Disposition = clampInt(n.Disposition+clampInt(a.Delta, -1, 1), -3, 3)
		return "", nil
	case DirNPCGivesItem:
		var a struct {
			NPCID  string `json:"npc_id"`
			ItemID string `json:"item_id"`
			To     string `json:"to"`
		}
		if err := c.Decode(&a); err != nil {
			return "", err
		}
		n := w.NPC(a.NPCID)
		if n == nil || !n.Alive || n.LocationID != l.ID {
			return "", fmt.Errorf("no npc %q here", a.NPCID)
		}
		if n.Disposition < 0 {
			return "", fmt.Errorf("%s is not friendly enough to give gifts", n.Name)
		}
		it := w.FindItemOf(game.OwnerNPC, n.ID, a.ItemID)
		if it == nil || isRewardItem(w, it.ID) {
			return "", fmt.Errorf("%s has no giftable %q", n.Name, a.ItemID)
		}
		if it.Value > 60 {
			return "", fmt.Errorf("%s is too valuable to give away", it.Name)
		}
		to := w.Character(a.To)
		if to == nil {
			return "", fmt.Errorf("unknown character %q", a.To)
		}
		it.Owner = game.Owner{Kind: game.OwnerCharacter, ID: to.ID}
		return fmt.Sprintf("%s gives %s to %s", n.Name, it.Name, to.Name), nil
	case DirRecordLore:
		var a struct {
			Text string `json:"text"`
		}
		if err := c.Decode(&a); err != nil {
			return "", err
		}
		if strings.TrimSpace(a.Text) == "" {
			return "", fmt.Errorf("text is required")
		}
		w.AddChronicle(trim(a.Text, 300))
		return "", nil
	case DirAdjustTension:
		var a struct {
			Delta int `json:"delta"`
		}
		if err := c.Decode(&a); err != nil {
			return "", err
		}
		game.AdjustTension(w, clampInt(a.Delta, -2, 2))
		return "", nil
	case DirEndEncounter:
		if !w.InCombat() {
			return "", fmt.Errorf("no fight to end")
		}
		for _, e := range w.Encounter.Living() {
			if e.NPCID != "" && e.NPCID == w.Campaign.AntagonistID {
				return "", fmt.Errorf("the antagonist does not flee")
			}
		}
		w.Encounter = nil
		w.Campaign.Turns.Budget.Free = game.Unlimited
		game.AdjustTension(w, -1)
		return "The remaining enemies flee", nil
	}
	return "", fmt.Errorf("unknown directive %q", c.Function.Name)
}

const enemySystem = `You narrate enemy actions in a tabletop game. The engine resolved them; narrate ONLY the lines given, in order.
Do not add attacks, moves, damage or deaths. 30-80 words, plain present tense, no questions, do not mention whose turn is next.`

func (g *GM) narrateEnemies(ctx context.Context, w *game.World, turns []game.EnemyTurn) string {
	var lines []string
	for _, t := range turns {
		line := "- " + t.Text
		if e := w.Enemy(t.EnemyID); e != nil && e.Ranged && t.Kind == "attack" {
			line += " (a ranged attack, from a distance)"
		}
		lines = append(lines, line)
	}
	resp, err := g.LLM.Chat(ctx, llm.Request{Tag: "narrate_enemies", Temperature: 0.7, MaxTokens: 300, Messages: []llm.Message{
		llm.System(enemySystem),
		llm.User(fmt.Sprintf("Location: %s.\nParty (use these pronouns): %s.\nEngine-resolved enemy actions:\n%s", w.Here().Name, partyPronouns(w), strings.Join(lines, "\n")))}})
	text := strings.TrimSpace(resp.Content)
	if err != nil || text == "" {
		var parts []string
		for _, t := range turns {
			parts = append(parts, t.Text)
		}
		return strings.Join(parts, ". ") + "."
	}
	if wordCount(text) > 160 {
		text = g.tighten(ctx, text, 80)
	}
	return text
}

const judgeSystem = `You are a strict objective judge for a tabletop game. You may only mark FREEFORM objectives complete, and only when the
recent log clearly shows the party did it. Quote the exact evidence from the log (a short verbatim excerpt). If nothing is clearly complete, call no tool.`

// judge ticks freeform objectives backed by quoted evidence from the log.
func (g *GM) judge(ctx context.Context, w *game.World, logText string) ([]game.QuestUpdate, []string) {
	var open []string
	for _, q := range w.Quests {
		if q.Status != game.QuestActive {
			continue
		}
		for _, o := range q.Objectives {
			if o.Kind == game.ObjFreeform && !o.Done {
				open = append(open, fmt.Sprintf("quest_id=%s objective_id=%s: %s (%s)", q.ID, o.ID, o.Text, q.Title))
			}
			if q.IsMain && !o.Done {
				break
			}
		}
	}
	if len(open) == 0 {
		return nil, nil
	}
	t := tool("complete_objective", "Mark a freeform objective complete with quoted evidence.", obj(map[string]any{
		"quest_id": str("quest id"), "objective_id": str("objective id"), "evidence": str("verbatim quote from the log")}, "quest_id", "objective_id", "evidence"))
	resp, err := g.LLM.Chat(ctx, llm.Request{Tag: "judge", Temperature: 0.1, MaxTokens: 300, Tools: []llm.Tool{t}, Messages: []llm.Message{
		llm.System(judgeSystem),
		llm.User(fmt.Sprintf("OPEN FREEFORM OBJECTIVES:\n%s\n\nRECENT LOG:\n%s", strings.Join(open, "\n"), logText))}})
	if err != nil {
		return nil, nil
	}
	calls := resp.ToolCalls
	if len(calls) == 0 {
		calls = llm.SalvageToolCalls(resp.Content, []llm.Tool{t})
	}
	var ups []game.QuestUpdate
	var changes []string
	norm := normalizeQuote(logText)
	for _, c := range calls {
		var a struct {
			QuestID     string `json:"quest_id"`
			ObjectiveID string `json:"objective_id"`
			Evidence    string `json:"evidence"`
		}
		if c.Decode(&a) != nil {
			continue
		}
		ev := normalizeQuote(a.Evidence)
		if len(ev) < 8 || !strings.Contains(norm, ev) {
			g.debugf("judge evidence rejected: %q", a.Evidence)
			continue
		}
		u, ch, err := game.CompleteFreeform(w, a.QuestID, a.ObjectiveID, a.Evidence)
		if err != nil {
			g.debugf("judge: %v", err)
			continue
		}
		ups = append(ups, u...)
		changes = append(changes, ch...)
	}
	return ups, changes
}

func normalizeQuote(s string) string {
	s = strings.ToLower(s)
	r := strings.NewReplacer("\"", "", "'", "", "“", "", "”", "", "‘", "", "’", "", "\n", " ")
	return strings.Join(strings.Fields(r.Replace(s)), " ")
}

func (g *GM) summarize(ctx context.Context, w *game.World, recent []string) {
	resp, err := g.LLM.Chat(ctx, llm.Request{Tag: "summarize", Temperature: 0.3, MaxTokens: 400, Messages: []llm.Message{
		llm.System("Summarize the story so far in at most 120 words: key events, promises, enemies and open threads. Facts only."),
		llm.User(fmt.Sprintf("PREVIOUS SUMMARY: %s\n\nRECENT LOG:\n%s", w.Campaign.Summary, strings.Join(recent, "\n")))}})
	if err == nil && strings.TrimSpace(resp.Content) != "" {
		w.Campaign.Summary = trim(resp.Content, 1200)
		w.Campaign.SummaryTurn = w.Campaign.Turn
	}
}

func partyPronouns(w *game.World) string {
	var ps []string
	for _, c := range w.Characters {
		ps = append(ps, fmt.Sprintf("%s (%s)", c.Name, game.NormPronouns(c.Pronouns)))
	}
	return strings.Join(ps, ", ")
}
