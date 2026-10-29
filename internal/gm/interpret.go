package gm

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/llm"
)

const interpreterSystem = `You translate a player's message in a turn-based tabletop game into 1-4 tool calls, in the order the player wants them.
Rules:
- Call tools only; never write prose.
- Use only ids that appear in the state below. Never invent ids.
- Compound intents need multiple calls: "I charge and finish it" = reposition + attack. "I run to the door and leave" = reposition + move. "I look around and then attack the archer" = look + attack.
- Going somewhere else ("on to the wreck", "head back to town", "lead us to X") always needs move with that exit's id, or travel for a far place already visited. Outside combat an exit can be taken from any zone, so no reposition is needed first.
- FREE actions: look, say, equip, drop, pick_up, accept_quest, turn_in_quest (and outside combat: reposition, talk_to, buy, sell). MOVE: move through an exit, travel, reposition in combat. ACTION: everything else. Spending the ACTION ends the turn, but the moves and free actions after it in the same message still happen, so always emit every step the player asked for. Buying several things is one buy call per item; in combat several buys from one merchant cost one action in total.
- Respect LEFT THIS TURN; if the player asks for more than is left, still emit their steps in order (the engine reports what could not be done).
- Attacks need the target in reach: melee = same zone, thrown = 1 zone, bows/foci/holy symbols = 2 zones. If the target is out of reach, reposition first.
- Questions like "what do I see?" or "what's in the room?" are look. Talking to an NPC in a conversation is talk_to; a passing remark is say. Outside combat talk_to walks the actor over to the NPC, so no reposition is needed first. "I talk to X: <question>" is one talk_to with that question as text. Words addressed to a named NPC ("I tell X ...", "I promise X ...") are talk_to, not say.
- When the character gives an NPC their word (promises, swears, makes a deal or owes a debt), put what they promised in talk_to's promise field.
- Anything risky or uncertain (climbing, sneaking, searching, persuading, picking a lock) is ability_check with a sensible DC (5/10/15/20/25).
- Laying hands on someone who is not an enemy in this fight (grab, pin, shove, trip, restrain, pickpocket) is ability_check (athletics or sleight_of_hand) naming that person in its text, never a bare reposition. Shoving or tripping an enemy is shove; grabbing and holding one is grapple; pinning or tying up a badly hurt enemy to end its fight is subdue.
- Teamwork in a fight: "I cover / defend myself / brace" = dodge; "I back away carefully" = disengage (then reposition); "I distract it so X can hit" / "I give X a hand" = help; "I duck out of sight" = hide; "I wait for it to come to me" / "I shoot whatever comes through the door" = ready.
- Give ability_check the skill that fits (stealth to sneak, perception to notice, insight to read someone, persuasion, deception, intimidation, athletics to climb, medicine to treat or stabilize a dying ally...).
- "skip", "pass", "wait", "end my turn" = end_turn.
- A MAP SELECTION is only a hint. The player's words win. A selected NPC means the person spoken to, traded with or examined; attack them only if the words clearly say so.`

var multiStepRe = regexp.MustCompile(`(?i)\b(and|then|after|before|while|so)\b|[,;]`)

func actionList(as []game.Action) string {
	var parts []string
	for _, a := range as {
		p := a.Kind
		for _, v := range []string{a.Target, a.Item, a.Zone} {
			if v != "" {
				p += " " + v
			}
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, ", ")
}

func containsAction(as []game.Action, b game.Action) bool {
	for _, a := range as {
		if a.Kind == b.Kind && a.Target == b.Target && a.Item == b.Item && a.Zone == b.Zone {
			return true
		}
	}
	return false
}

var violentRe = regexp.MustCompile(`(?i)\b(attack|kill|stab|strike|hit|shoot|slash|punch|murder|fight|smite|cut down|run through|fire at|throw .* at)\b`)

func (g *GM) interpret(ctx context.Context, w *game.World, actor *game.Character, text string, hints Hints, recent []string) []game.Action {
	var b strings.Builder
	b.WriteString(snapshot(w, actor, false))
	fmt.Fprintf(&b, "LEFT THIS TURN: %s\n", w.Campaign.Turns.Budget.Left())
	if h := describeHints(w, hints); h != "" {
		fmt.Fprintf(&b, "MAP SELECTION (hint only): %s\n", h)
	}
	if len(recent) > 0 {
		n := len(recent)
		if n > 4 {
			recent = recent[n-4:]
		}
		fmt.Fprintf(&b, "RECENT:\n%s\n", strings.Join(recent, "\n"))
	}
	fmt.Fprintf(&b, "\nPLAYER (%s) SAYS: %s", actor.Name, text)
	req := llm.Request{Tag: "interpret", Temperature: 0.1, Tools: interpreterTools(), RequireTool: true,
		Messages: []llm.Message{llm.System(interpreterSystem), llm.User(b.String())}}
	var parsed []game.Action
	validate := func(cs []llm.ToolCall) []string {
		parsed = parsed[:0]
		var problems []string
		for i, c := range cs {
			a, err := toAction(c)
			if err != nil {
				problems = append(problems, fmt.Sprintf("call %d (%s): bad arguments: %v", i+1, c.Function.Name, err))
				continue
			}
			a = applyHints(w, a, hints, text)
			if a.Kind == game.ActSay && a.Text == "" {
				a.Text = text
			}
			if msg := validateAction(w, actor, &a); msg != "" {
				problems = append(problems, fmt.Sprintf("call %d (%s): %s", i+1, c.Function.Name, msg))
				continue
			}
			parsed = append(parsed, a)
		}
		return problems
	}
	calls, err := g.forced(ctx, req, validate)
	// Some models answer with one tool call at a time. When the message
	// reads like several steps, or the only step is walking somewhere the
	// player did not ask to go, show what is queued and let them add more.
	if err == nil && len(parsed) > 0 && len(parsed) < game.MaxSteps && (multiStepRe.MatchString(text) || strayReposition(parsed, text)) {
		msgs := append([]llm.Message{}, req.Messages...)
		for round := 0; round < 2 && len(parsed) < game.MaxSteps; round++ {
			msgs = append(msgs, llm.Message{Role: "assistant", ToolCalls: calls})
			for i := range calls {
				if calls[i].ID == "" {
					calls[i].ID = fmt.Sprintf("q%d_%d", round, i)
				}
				msgs = append(msgs, llm.ToolResult(calls[i].ID, "queued"))
			}
			msgs = append(msgs, llm.User("Queued so far: "+actionList(parsed)+". If the player's message asks for more steps after these, call the next tool(s) now, in order. If everything is covered, reply with the word done."))
			more, _, err2 := g.chatCalls(ctx, llm.Request{Tag: "interpret_more", Temperature: 0.1, Tools: req.Tools, Messages: msgs})
			if err2 != nil || len(more) == 0 {
				break
			}
			prev := append([]game.Action(nil), parsed...)
			_ = validate(more) // invalid extras are dropped
			added := 0
			for _, a := range parsed {
				if !containsAction(prev, a) {
					prev = append(prev, a)
					added++
				}
			}
			parsed = prev
			if added == 0 {
				break
			}
			g.logf("interpret: %d more step(s) for %q", added, text)
			calls = more
		}
		if len(parsed) > game.MaxSteps {
			parsed = parsed[:game.MaxSteps]
		}
	}
	for _, c := range calls {
		g.debugf("interpret %q -> %s %s", text, c.Function.Name, c.Function.Arguments)
	}
	if err != nil && len(calls) == 0 {
		g.logf("interpret failed (%v); falling back to look", err)
		return []game.Action{{Kind: game.ActLook}}
	}
	if len(parsed) == 0 {
		return []game.Action{{Kind: game.ActLook}}
	}
	// still only a walk: if the player picked an NPC on the map, they
	// meant to talk to them
	if strayReposition(parsed, text) && len(parsed) < game.MaxSteps {
		if n := w.NPC(hints.TargetID); n != nil && n.ID == hints.TargetID {
			a := game.Action{Kind: game.ActTalkTo, Target: n.ID, Text: text}
			if validateAction(w, actor, &a) == "" {
				parsed = append(parsed, a)
			}
		}
	}
	return append([]game.Action(nil), parsed...)
}

var walkRe = regexp.MustCompile(`(?i)\b(go|goes|going|head|heads|move|moves|walk|walks|run|runs|step|steps|approach|approaches|cross|crosses|wander|return|returns|back to|over to|join|joins|follow|follows|reposition|stand|hide|take cover|duck|climb)\b`)

// strayReposition is true when every step is a reposition but the player's
// words do not talk about moving: the model dropped the step that mattered.
func strayReposition(as []game.Action, text string) bool {
	for _, a := range as {
		if a.Kind != game.ActReposition {
			return false
		}
	}
	return len(as) > 0 && !walkRe.MatchString(text)
}

func describeHints(w *game.World, h Hints) string {
	var parts []string
	if h.TargetID != "" {
		switch {
		case w.Enemy(h.TargetID) != nil:
			e := w.Enemy(h.TargetID)
			parts = append(parts, fmt.Sprintf("enemy %s [%s]", e.Display(), e.ID))
		case w.NPC(h.TargetID) != nil:
			n := w.NPC(h.TargetID)
			parts = append(parts, fmt.Sprintf("NPC %s [%s]", n.Name, n.ID))
		case w.Character(h.TargetID) != nil:
			c := w.Character(h.TargetID)
			parts = append(parts, fmt.Sprintf("party member %s [%s]", c.Name, c.ID))
		}
	}
	if h.ZoneID != "" {
		if z := w.Here().Zone(h.ZoneID); z != nil {
			parts = append(parts, fmt.Sprintf("zone %s [%s]", z.Name, z.ID))
		}
	}
	if h.Check != nil {
		if sk, ok := game.SkillByKey(h.Check.Skill); ok {
			parts = append(parts, fmt.Sprintf("the player asks to make a %s (%s) ability_check", sk.Name, sk.Ability))
		} else if ab, ok := game.ParseAbility(h.Check.Ability); ok {
			parts = append(parts, fmt.Sprintf("the player asks to make a %s ability_check", ab))
		}
	}
	return strings.Join(parts, ", ")
}

// directCheck turns a dock check (search, pick or force a lock) into the
// action itself, so the roll is exactly the one the player chose. It
// returns nil when the hint is absent or is left to the interpreter.
func directCheck(w *game.World, actor *game.Character, h Hints) ([]game.Action, error) {
	if h.Act != nil {
		return directAct(w, actor, *h.Act)
	}
	ch := h.Check
	if ch == nil {
		return nil, nil
	}
	a := game.Action{Kind: game.ActCheck, Purpose: ch.Purpose, Ability: ch.Ability, Skill: ch.Skill}
	switch ch.Purpose {
	case "search":
		a.Text = "search the area"
	case "unlock", "force":
		x := w.FindExit(w.Campaign.LocationID, ch.Exit)
		if x == nil || x.Edge.State != game.EdgeLocked {
			return nil, fmt.Errorf("%w: there is no locked way %q here", ErrBadRequest, ch.Exit)
		}
		if x.Edge.LairGate {
			return nil, fmt.Errorf("%w: the way to %s cannot be forced; the story must progress first", ErrBadRequest, x.To.Name)
		}
		a.Target = x.Edge.ID
		a.Text = map[string]string{"unlock": "pick the lock to ", "force": "force the way to "}[ch.Purpose] + x.To.Name
	default:
		return nil, nil
	}
	if msg := validateAction(w, actor, &a); msg != "" {
		return nil, fmt.Errorf("%w: %s", ErrBadRequest, msg)
	}
	return []game.Action{a}, nil
}

// tacticKinds are the actions the dock may send directly.
var dockActs = map[string]bool{game.ActDodge: true, game.ActDisengage: true, game.ActHelp: true, game.ActHide: true,
	game.ActReady: true, game.ActShove: true, game.ActGrapple: true}

// directAct turns a dock tactic into the action itself.
func directAct(w *game.World, actor *game.Character, h ActHint) ([]game.Action, error) {
	if !dockActs[h.Kind] {
		return nil, fmt.Errorf("%w: unknown dock action %q", ErrBadRequest, h.Kind)
	}
	a := game.Action{Kind: h.Kind, Target: h.Target, Method: h.Method}
	if msg := validateAction(w, actor, &a); msg != "" {
		return nil, fmt.Errorf("%w: %s", ErrBadRequest, msg)
	}
	return []game.Action{a}, nil
}

// applyHints fills gaps from map selections without overriding words.
func applyHints(w *game.World, a game.Action, h Hints, text string) game.Action {
	enemy := w.Enemy(h.TargetID)
	var npc *game.NPC
	if n := w.NPC(h.TargetID); n != nil && n.ID == h.TargetID {
		npc = n
	}
	var mate *game.Character
	if c := w.Character(h.TargetID); c != nil && c.ID == h.TargetID {
		mate = c
	}
	switch a.Kind {
	case game.ActAttack, game.ActSubdue:
		if a.Target == "" && enemy != nil {
			a.Target = enemy.ID
		}
		if a.Kind == game.ActAttack && npc != nil && (a.Target == npc.ID || game.NameMatch(a.Target, npc.Name)) && !violentRe.MatchString(text) {
			a = game.Action{Kind: game.ActTalkTo, Target: npc.ID, Text: text}
		}
	case game.ActTalkTo, game.ActBuy, game.ActSell:
		if a.Target == "" && npc != nil {
			a.Target = npc.ID
		}
	case game.ActGiveItem:
		if a.Target == "" {
			if mate != nil {
				a.Target = mate.ID
			} else if npc != nil {
				a.Target = npc.ID
			}
		}
	case game.ActUseItem:
		if a.Target == "" && mate != nil {
			a.Target = mate.ID
		}
	case game.ActLook:
		if a.Target == "" && h.TargetID != "" && (enemy != nil || npc != nil) {
			a.Target = h.TargetID
		}
	case game.ActCheck:
		if h.Check != nil {
			if ab, ok := game.ParseAbility(h.Check.Ability); ok {
				a.Ability = string(ab)
			}
			if sk, ok := game.SkillByKey(h.Check.Skill); ok {
				a.Skill, a.Ability = sk.Key, string(sk.Ability)
			}
		}
	case game.ActReposition:
		if a.Zone == "" {
			a.Zone = h.ZoneID
		}
	case game.ActMove:
		if a.Target == "" && h.ZoneID != "" {
			a = game.Action{Kind: game.ActReposition, Zone: h.ZoneID}
		}
	}
	return a
}

// validateAction checks every reference against the world, normalizing
// names to ids. It returns "" when the action is usable.
func validateAction(w *game.World, c *game.Character, a *game.Action) string {
	l := w.Here()
	item := func(kind game.OwnerKind, owner, ref, what string) string {
		if ref == "" {
			return what + " is required"
		}
		it := w.FindItemOf(kind, owner, ref)
		if it == nil {
			return fmt.Sprintf("no %s %q", what, ref)
		}
		a.Item = it.ID
		return ""
	}
	switch a.Kind {
	case game.ActLook:
		// an unknown target stays, so the engine can say it is not here
		// instead of describing the whole place
	case game.ActSay:
		if strings.TrimSpace(a.Text) == "" {
			return "text is required"
		}
	case game.ActEndTurn, game.ActRest:
	case game.ActEquip, game.ActDrop, game.ActSell:
		if msg := item(game.OwnerCharacter, c.ID, a.Item, "carried item"); msg != "" {
			return msg
		}
		if a.Kind == game.ActSell {
			return npcHere(w, a, true)
		}
	case game.ActUseItem:
		if msg := item(game.OwnerCharacter, c.ID, a.Item, "carried item"); msg != "" {
			return msg
		}
		if a.Target != "" {
			if t := w.Character(a.Target); t != nil {
				a.Target = t.ID
			} else if x := w.FindExit(l.ID, a.Target); x != nil {
				a.Target = x.Edge.ID
			} else {
				return fmt.Sprintf("no party member or exit %q", a.Target)
			}
		}
	case game.ActPickUp:
		return item(game.OwnerLocation, l.ID, a.Item, "item on the ground")
	case game.ActGiveItem:
		if msg := item(game.OwnerCharacter, c.ID, a.Item, "carried item"); msg != "" {
			return msg
		}
		if t := w.Character(a.Target); t != nil {
			a.Target = t.ID
			return ""
		}
		return npcHere(w, a, false)
	case game.ActMove:
		x := w.FindExit(l.ID, a.Target)
		if x == nil {
			return fmt.Sprintf("no exit %q here", a.Target)
		}
		a.Target = x.Edge.ID
	case game.ActReposition:
		z := l.Zone(a.Zone)
		if z == nil {
			return fmt.Sprintf("no zone %q here", a.Zone)
		}
		a.Zone = z.ID
	case game.ActTravel:
		d := w.Location(a.Target)
		if d == nil || !d.Visited {
			return fmt.Sprintf("%q is not a visited place", a.Target)
		}
		a.Target = d.ID
	case game.ActAttack, game.ActSubdue:
		if e := w.Enemy(a.Target); e != nil {
			a.Target = e.ID
			if a.Item != "" {
				if it := w.FindItemOf(game.OwnerCharacter, c.ID, a.Item); it != nil {
					a.Item = it.ID
				} else {
					a.Item = ""
				}
			}
			return ""
		}
		if a.Kind == game.ActAttack {
			if n := w.NPC(a.Target); n != nil && n.Alive && n.LocationID == l.ID {
				a.Target = n.ID
				return ""
			}
		}
		return fmt.Sprintf("no enemy %q here", a.Target)
	case game.ActTalkTo:
		return npcHere(w, a, false)
	case game.ActBuy:
		if msg := npcHere(w, a, true); msg != "" {
			return msg
		}
		return item(game.OwnerNPC, a.Target, a.Item, "item in stock")
	case game.ActAcceptQuest, game.ActTurnInQuest:
		q := w.Quest(a.Target)
		if q == nil {
			return fmt.Sprintf("no quest %q", a.Target)
		}
		a.Target = q.ID
	case game.ActCheck:
		if sk, ok := game.SkillByKey(a.Skill); ok {
			a.Skill, a.Ability = sk.Key, string(sk.Ability)
		} else {
			a.Skill = ""
		}
		if _, ok := game.ParseAbility(a.Ability); !ok {
			a.Ability = "WIS"
		}
		if a.Target != "" {
			if x := w.FindExit(l.ID, a.Target); x != nil {
				a.Target = x.Edge.ID
			} else if t := w.Character(a.Target); t != nil {
				a.Target = t.ID
			}
		}
	case game.ActDodge, game.ActDisengage, game.ActHide:
	case game.ActHelp:
		t := w.Character(a.Target)
		if t == nil || t.ID == c.ID {
			return fmt.Sprintf("no other party member %q", a.Target)
		}
		a.Target = t.ID
	case game.ActReady:
		if a.Target != "" {
			if e := w.Enemy(a.Target); e != nil {
				a.Target = e.ID
			} else {
				a.Target = ""
			}
		}
	case game.ActShove, game.ActGrapple:
		e := w.Enemy(a.Target)
		if e == nil {
			return fmt.Sprintf("no enemy %q here", a.Target)
		}
		a.Target = e.ID
	default:
		return fmt.Sprintf("unknown action %q", a.Kind)
	}
	return ""
}

func npcHere(w *game.World, a *game.Action, merchant bool) string {
	n := w.NPC(a.Target)
	if n == nil || !n.Alive || n.LocationID != w.Campaign.LocationID {
		return fmt.Sprintf("no NPC %q here", a.Target)
	}
	if merchant && !n.Merchant {
		return fmt.Sprintf("%s is not a merchant", n.Name)
	}
	a.Target = n.ID
	return ""
}
