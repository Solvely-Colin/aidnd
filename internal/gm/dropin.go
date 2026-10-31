package gm

import (
	"context"
	"fmt"
	"strings"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/llm"
	"github.com/Solvely-Colin/aidnd/internal/store"
)

// EventCatchUp is the story so far for a player who joined a campaign
// already under way. Only the player in its "for" field sees it.
const EventCatchUp = "catch_up"

// joinRunning brings a new player's character into a campaign that is
// already under way. They join the biggest party where it stands (in its
// fight, if one is on) at its level, get a starting goal, a line in their
// record and the chronicle, a private story-so-far, and the narrator
// writes them in. The caller holds the campaign lock; w is settled.
func (g *GM) joinRunning(ctx context.Context, w *game.World, playerID string, spec game.CharacterSpec) (*game.Character, error) {
	for _, other := range w.Characters {
		if !other.Dead && strings.EqualFold(strings.TrimSpace(other.Name), strings.TrimSpace(spec.Name)) {
			return nil, Conflict("%s is already in the story; pick another name", other.Name)
		}
	}
	group := game.JoinGroupFor(w)
	xp := game.JoinXP(w, group)
	if err := w.Focus(group); err != nil {
		return nil, err
	}
	c, err := game.CreateCharacter(w, playerID, spec)
	if err != nil {
		return nil, err
	}
	changes := game.JoinRunning(w, c, xp)
	catch := game.BuildCatchUp(w, c)
	text, goal := g.narrateArrival(ctx, w, c, catch)
	if goal == "" {
		goal = arrivalGoal(w, c)
	}
	if gl, err := game.AddGoal(w, c, goal, game.SourceGM); err == nil {
		changes = append(changes, fmt.Sprintf("%s takes up a goal: %s", c.Name, gl.Text))
	}
	game.NoteSeen(w, playerID)
	_, err = g.commit(ctx, w,
		store.NewEvent("character_created", playerID, map[string]any{
			"player_id": playerID, "character_id": c.ID, "name": c.Name, "class": c.Class, "kit": c.Kit, "concept": c.Concept,
			"joins_midway": true, "level": c.Level, "changes": changes}),
		store.NewEvent(EventCatchUp, playerID, map[string]any{"for": playerID, "character_id": c.ID, "catch_up": catch}),
		store.NewEvent("narration", "", map[string]any{"text": text, "arrival": true, "character_id": c.ID}))
	return c, err
}

// arrivalGoal is the engine's starting goal for a newcomer when the
// narrator offers none: win over the people they've just thrown in with.
func arrivalGoal(w *game.World, c *game.Character) string {
	var names []string
	for _, o := range w.Characters {
		if o.ID != c.ID && !o.Dead {
			names = append(names, firstName(o.Name))
		}
	}
	switch len(names) {
	case 0:
		return "Find out what happened here"
	case 1:
		return "Earn " + names[0] + "'s trust"
	}
	return "Earn the trust of " + andList(names)
}

// andList is "A, B and C".
func andList(names []string) string {
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// narrateArrival writes the newcomer into the scene, and may offer a
// personal goal for them (the line after "GOAL:"). Both are checked
// against the world; the fallback is plain engine text and no goal.
func (g *GM) narrateArrival(ctx context.Context, w *game.World, c *game.Character, catch *game.CatchUp) (string, string) {
	l := w.Here()
	var others []string
	for _, o := range w.Characters {
		if o.ID != c.ID && !o.Dead {
			others = append(others, o.Name)
		}
	}
	with := "the others"
	if len(others) > 0 {
		with = andList(others)
	}
	plain := fmt.Sprintf("%s arrives at %s and falls in with %s.", c.Name, l.Name, with)
	situation := "The scene is calm enough for introductions."
	if w.InCombat() {
		var foes []string
		for _, e := range w.Encounter.Living() {
			foes = append(foes, e.Display())
		}
		plain = fmt.Sprintf("%s arrives at %s in the middle of the fight and stands with %s.", c.Name, l.Name, with)
		situation = fmt.Sprintf("A fight is on against %s. The newcomer arrives in the middle of it and will take their turn after the others; describe them arriving and readying, not attacking.", strings.Join(foes, ", "))
	}
	who := sheetLine(w, c)
	if c.Concept != "" {
		who += ". Concept: " + c.Concept
	}
	if b := strings.TrimSpace(c.Backstory); b != "" {
		who += ". Backstory: " + trim(b, 400)
	}
	resp, err := g.LLM.Chat(ctx, llm.Request{Tag: "narrate_arrival", Temperature: 0.8, MaxTokens: 500, Messages: []llm.Message{
		llm.System(narratorSystem),
		llm.User(fmt.Sprintf(`%s
STATE:
%s
STORY SO FAR (engine):
%s
NEWCOMER: %s
%s
A new player character joins the story here. Write 50-100 words bringing %s into the scene as they arrive and meet %s: how they come in, and a reason to be here that fits their backstory and what the party is doing. Nothing else changes: nobody acts, nobody is hurt, nothing is found, and no new named people or items appear. Keep everything already established true.
Then, on its own last line, write "GOAL: " and one personal goal for %s here, under 15 words, tied to their backstory and this story.`,
			loreBlock(selectLore(w, c.Concept+" "+c.Backstory, g.Cfg.LoreBudget)), snapshot(w, nil, true), catch.Text(), who, situation, c.Name, with, firstName(c.Name))),
	}})
	if err != nil {
		return plain, ""
	}
	text, goal := splitGoal(resp.Content)
	if text == "" {
		return plain, ""
	}
	text, _ = g.guard(ctx, newCanon(w, "", nil, l.Description), "narrate_arrival", text, plain)
	if goal != "" {
		gc := newCanon(w, "", nil)
		gc.lifeOnly = true
		if is := gc.check(goal); len(is) > 0 {
			goal = ""
		}
	}
	return text, goal
}

// splitGoal takes the "GOAL:" line off the end of an arrival.
func splitGoal(s string) (string, string) {
	s = strings.TrimSpace(s)
	i := strings.LastIndex(strings.ToUpper(s), "GOAL:")
	if i < 0 {
		return s, ""
	}
	goal := strings.TrimSpace(s[i+len("GOAL:"):])
	if j := strings.IndexByte(goal, '\n'); j >= 0 {
		goal = strings.TrimSpace(goal[:j])
	}
	goal = strings.Trim(goal, `"*_ `)
	goal = strings.TrimSuffix(goal, ".")
	return strings.TrimSpace(strings.TrimRight(s[:i], "*_ \n")), trim(goal, 140)
}

// firstName is the first word of a name.
func firstName(name string) string {
	if f := strings.Fields(name); len(f) > 0 {
		return f[0]
	}
	return name
}
