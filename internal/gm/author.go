package gm

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/llm"
	"github.com/Solvely-Colin/aidnd/internal/store"
)

const authorSystem = `You are the world author for a turn-based fantasy tabletop game run by a rules engine.
You only produce structured content by calling the tool you are given. Keep names short and evocative,
descriptions concrete (2-3 sentences), and everything consistent with the campaign premise and canon.
Use plain language. Never decide what the player characters do.`

// CampaignSpec is the create_campaign tool's arguments.
type CampaignSpec struct {
	Title      string `json:"title"`
	Pitch      string `json:"pitch"`
	Antagonist struct {
		Name   string `json:"name"`
		Role   string `json:"role"`
		Desc   string `json:"desc"`
		Motive string `json:"motive"`
	} `json:"antagonist"`
	Hub struct {
		Name        string `json:"name"`
		Kind        string `json:"kind"`
		Description string `json:"description"`
	} `json:"hub"`
	HubNPCs []game.NPCSpec `json:"hub_npcs"`
	// Cast are the people, creatures and remains that objectives and lore
	// talk about, placed so the engine knows where they are.
	Cast []struct {
		game.NPCSpec
		At string `json:"at"`
	} `json:"cast"`
	Places []struct {
		Name       string `json:"name"`
		Hint       string `json:"hint"`
		Kind       string `json:"kind"`
		Danger     int    `json:"danger"`
		ConnectsTo string `json:"connects_to"`
	} `json:"places"`
	Lair struct {
		Name       string `json:"name"`
		Hint       string `json:"hint"`
		ConnectsTo string `json:"connects_to"`
	} `json:"lair"`
	MainQuest struct {
		Title      string               `json:"title"`
		Summary    string               `json:"summary"`
		Objectives []game.ObjectiveSpec `json:"objectives"`
	} `json:"main_quest"`
	SideQuest *struct {
		Title      string               `json:"title"`
		Summary    string               `json:"summary"`
		Giver      string               `json:"giver"`
		Objectives []game.ObjectiveSpec `json:"objectives"`
		RewardGold int                  `json:"reward_gold"`
		RewardXP   int                  `json:"reward_xp"`
	} `json:"side_quest"`
	Lore []struct {
		Title    string   `json:"title"`
		Text     string   `json:"text"`
		Keywords []string `json:"keywords"`
		AlwaysOn bool     `json:"always_on"`
	} `json:"lore"`
	Beats []struct {
		Title   string   `json:"title"`
		Text    string   `json:"text"`
		Trigger string   `json:"trigger"`
		Target  string   `json:"target"`
		Value   int      `json:"value"`
		Effects []string `json:"effects"`
		Figure  *struct {
			Name    string `json:"name"`
			Role    string `json:"role"`
			Desc    string `json:"desc"`
			Hostile bool   `json:"hostile"`
		} `json:"figure"`
	} `json:"beats"`
}

func createCampaignTool() llm.Tool {
	npc := obj(map[string]any{"name": str("name"), "role": str("e.g. blacksmith, elder, innkeeper"), "desc": str("one sentence"),
		"disposition": num("-3..3", -3, 3), "merchant": boolean("sells goods"), "stock": arr(str("catalog key"), "for merchants: catalog keys")}, "name", "role")
	place := obj(map[string]any{"name": str("place name"), "hint": str("one-line rumour about it"),
		"kind":   enum("kind", game.KindTown, game.KindWilderness, game.KindDungeon, game.KindCave, game.KindBuilding, game.KindRuin),
		"danger": num("1 mild .. 4 deadly", 1, 4), "connects_to": str("name of the hub or another place it connects to")}, "name", "hint", "kind", "danger", "connects_to")
	return tool("create_campaign", "Create the campaign skeleton.", obj(map[string]any{
		"title": str("campaign title"), "pitch": str("two-sentence player-facing pitch"),
		"antagonist": obj(map[string]any{"name": str("name"), "role": str("what they are"), "desc": str("public description"), "motive": str("SECRET: their plan")}, "name", "role", "motive"),
		"hub":        obj(map[string]any{"name": str("starting town or base"), "kind": enum("kind", game.KindTown, game.KindBuilding), "description": str("2-3 sentences")}, "name", "description"),
		"hub_npcs":   arr(npc, "2-4 NPCs at the hub; include one merchant and one quest giver"),
		"cast": arr(obj(map[string]any{"name": str("name, e.g. The Returned Fishermen"), "role": str("what they are"), "desc": str("one sentence"),
			"disposition": num("-3..3", -3, 3), "at": str("hub or place name where they are")}, "name", "role", "at"),
			"every other person, group, creature or body that the objectives, beats or lore say the party can find, talk to or examine (0-4). Anything not listed here or in hub_npcs does not exist in the game"),
		"places": arr(place, "3-5 places around the hub, revealed as stubs"),
		"lair":   obj(map[string]any{"name": str("antagonist's lair"), "hint": str("rumour"), "connects_to": str("name of a place (not the hub)")}, "name", "connects_to"),
		"main_quest": obj(map[string]any{"title": str("storyline title"), "summary": str("summary"),
			"objectives": arr(objectiveSchema, "3-5 ordered objectives; use reach_location/talk_to_npc/obtain_item/defeat_enemy with targets naming places/NPCs/cast above; the first should be talk_to_npc with the hub's quest giver, worded as hearing their request; the last must be defeat_enemy targeting the antagonist")}, "title", "objectives"),
		"side_quest": obj(map[string]any{"title": str("title"), "summary": str("summary"), "giver": str("hub NPC name"),
			"objectives": arr(objectiveSchema, "1-2 objectives"), "reward_gold": num("gold", 0, 80), "reward_xp": num("xp", 0, 150)}, "title", "giver", "objectives"),
		"lore": arr(obj(map[string]any{"title": str("title"), "text": str("SECRET canon truth, 1-3 sentences"),
			"keywords": arr(str("keyword"), "words that make this relevant"), "always_on": boolean("always relevant (use for 1-2 core truths)")}, "title", "text"), "4-8 secret canon entries: the antagonist's plan, NPC motives, place history"),
		"beats": arr(obj(map[string]any{"title": str("title"), "text": str("what happens, for the narrator"),
			"trigger": enum("trigger", game.TriggerEnterLocation, game.TriggerObjectiveDone, game.TriggerStalled, game.TriggerTension, game.TriggerTurn),
			"target":  str("place name (enter_location) or main objective number (objective_done)"),
			"value":   num("threshold for stalled/tension/turn", 0, 100),
			"effects": arr(enum("effect", game.EffectAmbush, game.EffectRevealExit, game.EffectBringNPC), "optional effects"),
			"figure":  obj(map[string]any{"name": str("name"), "role": str("what it is"), "desc": str("one sentence"), "hostile": boolean("attacks on arrival")}, "name", "role"),
		}, "title", "text", "trigger"), "3-6 story beats. A beat in which someone or something new shows up (e.g. drowned figures wading ashore) must name it as figure, so it becomes real when the beat plays"),
	}, "title", "antagonist", "hub", "hub_npcs", "places", "lair", "main_quest", "lore", "beats"))
}

// CreateCampaign authors and saves a new campaign. If the model fails
// twice, a deterministic fallback world is used so play can continue.
func (g *GM) CreateCampaign(ctx context.Context, premise string) (*game.World, error) {
	premise = strings.TrimSpace(premise)
	if premise == "" {
		premise = "A quiet frontier town where something old has begun to stir."
	}
	if len(premise) > 1000 {
		premise = premise[:1000]
	}
	id := g.NewID()
	req := llm.Request{Tag: "author_campaign", Temperature: 0.8, ForceTool: "create_campaign", Tools: []llm.Tool{createCampaignTool()},
		Messages: []llm.Message{llm.System(authorSystem), llm.User(fmt.Sprintf(
			"Premise: %s\n\nCreate the campaign for 1-4 level 1 characters. Catalog keys for merchant stock: %s.",
			premise, strings.Join(game.CatalogKeys(), ", ")))}}
	var built *game.World
	calls, err := g.forced(ctx, req, func(cs []llm.ToolCall) []string {
		var spec CampaignSpec
		if err := cs[0].Decode(&spec); err != nil {
			return []string{"arguments are not valid JSON for create_campaign: " + err.Error()}
		}
		w, p := g.buildCampaign(id, premise, spec)
		if len(p) == 0 {
			built = w
		}
		return p
	})
	if built == nil {
		g.logf("create_campaign failed (%v); using fallback world", err)
		if len(calls) > 0 {
			// try a lenient build of the last attempt
			var spec CampaignSpec
			if calls[0].Decode(&spec) == nil {
				if w, p := g.buildCampaign(id, premise, spec); len(p) == 0 {
					built = w
				}
			}
		}
		if built == nil {
			built, _ = g.buildCampaign(id, premise, fallbackSpec(premise))
		}
	}
	defer g.lock(id)()
	_, err = g.commit(ctx, built, store.NewEvent("campaign_created", "", map[string]any{"id": id, "title": built.Campaign.Title, "pitch": built.Campaign.Pitch}))
	return built, err
}

// buildCampaign turns a spec into a world, reporting problems.
func (g *GM) buildCampaign(id, premise string, spec CampaignSpec) (*game.World, []string) {
	var p []string
	if strings.TrimSpace(spec.Title) == "" {
		p = append(p, "title is required")
	}
	if strings.TrimSpace(spec.Antagonist.Name) == "" {
		p = append(p, "antagonist.name is required")
	}
	if strings.TrimSpace(spec.Hub.Name) == "" {
		p = append(p, "hub.name is required")
	}
	if len(spec.Places) < 2 {
		p = append(p, "need at least 2 places")
	}
	if strings.TrimSpace(spec.Lair.Name) == "" {
		p = append(p, "lair.name is required")
	}
	if len(spec.MainQuest.Objectives) == 0 {
		p = append(p, "main_quest needs objectives")
	}
	if len(p) > 0 {
		return nil, p
	}
	w := &game.World{Campaign: game.Campaign{ID: id, Title: trim(spec.Title, 80), Premise: premise, Pitch: trim(spec.Pitch, 400),
		Status: game.StatusLobby, MaxSetbacks: g.Cfg.MaxSetbacks, MaxPlayers: g.Cfg.MaxPlayers, CreatedAt: g.Now().Unix()}}
	w.Campaign.Scene = game.Scene{Day: 1, Time: 1, Tension: 2}
	w.Campaign.Turns.Budget = game.FullBudget(false)
	hubKind := spec.Hub.Kind
	if hubKind == "" {
		hubKind = game.KindTown
	}
	hub := w.AddStub(spec.Hub.Name, "", hubKind, 0)
	hub.Stub, hub.IsHub, hub.Visited, hub.Description = false, true, true, trim(spec.Hub.Description, 800)
	w.Campaign.HubID, w.Campaign.LocationID = hub.ID, hub.ID
	for i, ns := range spec.HubNPCs {
		if i >= 5 || strings.TrimSpace(ns.Name) == "" || w.NPC(ns.Name) != nil {
			continue
		}
		n := game.AddNPC(w, hub.ID, ns)
		if n.Merchant {
			stock := ns.Stock
			if len(stock) == 0 {
				stock = []string{"healing_potion", "healing_potion", "longsword", "shortbow", "leather", "rope"}
			}
			for j, k := range stock {
				if d, ok := game.LookupDef(k); ok && j < game.MaxMerchantStock {
					game.NewItem(w, d.Key, game.Owner{Kind: game.OwnerNPC, ID: n.ID})
				}
			}
		}
	}
	for i, ps := range spec.Places {
		if i >= 6 || strings.TrimSpace(ps.Name) == "" || w.Location(ps.Name) != nil {
			continue
		}
		w.AddStub(ps.Name, ps.Hint, ps.Kind, clampInt(ps.Danger, 1, 4))
	}
	for i, ps := range spec.Places {
		if i >= 6 {
			break
		}
		l := w.Location(ps.Name)
		if l == nil || l.IsHub || w.EdgeBetween(l.ID, hub.ID) != nil {
			continue
		}
		to := w.Location(ps.ConnectsTo)
		if to == nil || to.ID == l.ID || (!to.IsHub && !connectedToHub(w, to.ID)) {
			to = hub
		}
		w.Connect(to.ID, l.ID, game.EdgeKnown, "the way to "+l.Name, "back to "+to.Name)
	}
	// lair behind a gate, off a non-hub place
	lair := w.AddStub(spec.Lair.Name, spec.Lair.Hint, game.KindDungeon, 5)
	lair.IsLair = true
	w.Campaign.LairID = lair.ID
	from := w.Location(spec.Lair.ConnectsTo)
	if from == nil || from.IsHub || from.IsLair {
		from = w.Locations[len(w.Locations)-2]
	}
	gate := w.Connect(from.ID, lair.ID, game.EdgeLocked, "the way to "+lair.Name, "back to "+from.Name)
	gate.LairGate = true
	if !w.Connected() {
		return nil, []string{"places do not form a connected map"}
	}
	for i, cs := range spec.Cast {
		if i >= 4 || strings.TrimSpace(cs.Name) == "" || w.NPC(cs.Name) != nil {
			continue
		}
		at := w.Location(cs.At)
		if at == nil || at.IsLair || game.NameContains(cs.At, spec.Hub.Name) {
			at = hub
		}
		game.AddNPC(w, at.ID, game.NPCSpec{Name: cs.Name, Role: cs.Role, Desc: cs.Desc, Disposition: cs.Disposition})
	}
	ant := game.AddNPC(w, lair.ID, game.NPCSpec{Name: spec.Antagonist.Name, Role: firstOf(spec.Antagonist.Role, "villain"), Desc: spec.Antagonist.Desc, Disposition: -3})
	ant.Antagonist = true
	ant.Stats = &game.StatBlock{MaxHP: 45, AC: 15, AttackBonus: 6, Damage: "2d6+3"}
	w.Campaign.AntagonistID = ant.ID
	// main quest; force the finale to be defeating the antagonist
	objs := spec.MainQuest.Objectives
	if len(objs) > 5 {
		objs = objs[:5]
	}
	var cleaned []game.ObjectiveSpec
	for _, o := range objs {
		if o.Kind == game.ObjDefeatEnemy && game.NameContains(spec.Antagonist.Name, o.Target) {
			continue
		}
		switch o.Kind {
		case game.ObjReachLocation:
			if l := w.Location(o.Target); l == nil || l.IsLair {
				continue
			}
		case game.ObjTalkToNPC:
			if w.NPC(o.Target) == nil {
				continue
			}
		case game.ObjDeliverItem:
			if w.NPC(o.DeliverTo) == nil {
				continue
			}
		}
		if strings.TrimSpace(o.Text) == "" {
			continue
		}
		cleaned = append(cleaned, o)
	}
	cleaned = append(cleaned, game.ObjectiveSpec{Kind: game.ObjDefeatEnemy, Text: "Defeat " + ant.Name, Target: ant.ID})
	mq, err := game.AddQuest(w, game.QuestSpec{Title: firstOf(spec.MainQuest.Title, spec.Title), Summary: spec.MainQuest.Summary, Objectives: cleaned}, true)
	if err != nil {
		return nil, []string{"main_quest: " + err.Error()}
	}
	mq.Objectives[len(mq.Objectives)-1].Target = ant.ID
	if sq := spec.SideQuest; sq != nil && sq.Title != "" {
		if _, err := game.AddQuest(w, game.QuestSpec{Title: sq.Title, Summary: sq.Summary, GiverID: sq.Giver, Objectives: sq.Objectives,
			RewardGold: sq.RewardGold, RewardXP: sq.RewardXP, RewardItems: []game.RewardItemSpec{{Key: "healing_potion"}}}, false); err != nil {
			g.logf("side quest dropped: %v", err)
		}
	}
	w.Lore = append(w.Lore, &game.Lore{ID: w.NewID("lore"), Title: ant.Name + "'s plan", Text: trim(spec.Antagonist.Motive, 400),
		EntityIDs: []string{ant.ID, lair.ID}, Keywords: keywordsOf(ant.Name), AlwaysOn: true, LastTrigger: -100})
	for i, l := range spec.Lore {
		if i >= 10 || strings.TrimSpace(l.Text) == "" {
			continue
		}
		lo := &game.Lore{ID: w.NewID("lore"), Title: trim(l.Title, 80), Text: trim(l.Text, 500), AlwaysOn: l.AlwaysOn, LastTrigger: -100}
		for _, k := range l.Keywords {
			if k = strings.ToLower(strings.TrimSpace(k)); k != "" {
				lo.Keywords = append(lo.Keywords, k)
			}
		}
		for _, n := range w.NPCs {
			if game.NameContains(l.Text, n.Name) || game.NameContains(l.Title, n.Name) {
				lo.EntityIDs = append(lo.EntityIDs, n.ID)
			}
		}
		for _, loc := range w.Locations {
			if game.NameContains(l.Text, loc.Name) || game.NameContains(l.Title, loc.Name) {
				lo.EntityIDs = append(lo.EntityIDs, loc.ID)
			}
		}
		w.Lore = append(w.Lore, lo)
	}
	for i, b := range spec.Beats {
		if i >= 8 || strings.TrimSpace(b.Text) == "" {
			continue
		}
		beat := &game.Beat{ID: w.NewID("beat"), Title: trim(b.Title, 80), Text: trim(b.Text, 500), Trigger: b.Trigger, Value: b.Value}
		switch b.Trigger {
		case game.TriggerEnterLocation:
			l := w.Location(b.Target)
			if l == nil {
				continue
			}
			beat.Target = l.ID
		case game.TriggerObjectiveDone:
			n, err := strconv.Atoi(strings.TrimSpace(b.Target))
			if err != nil || n < 1 || n > len(mq.Objectives) {
				continue
			}
			beat.Target = mq.Objectives[n-1].ID
		case game.TriggerStalled, game.TriggerTension, game.TriggerTurn:
			if beat.Value <= 0 {
				beat.Value = map[string]int{game.TriggerStalled: 5, game.TriggerTension: 7, game.TriggerTurn: 20}[b.Trigger]
			}
		default:
			continue
		}
		for _, e := range b.Effects {
			if e == game.EffectAmbush || e == game.EffectRevealExit || (e == game.EffectBringNPC && b.Figure != nil) {
				beat.Effects = append(beat.Effects, e)
			}
		}
		if f := b.Figure; f != nil && strings.TrimSpace(f.Name) != "" {
			// the figure waits offstage until the beat brings it in
			n := w.NPC(f.Name)
			if n == nil || !game.NameMatch(n.Name, f.Name) {
				n = game.AddNPC(w, "", game.NPCSpec{Name: f.Name, Role: firstOf(f.Role, "stranger"), Desc: f.Desc})
			}
			if !n.Antagonist {
				beat.EffectTarget = n.ID
				beat.Effects = withEffect(beat.Effects, game.EffectBringNPC)
				if f.Hostile {
					n.Disposition = -3
					beat.Effects = withEffect(beat.Effects, game.EffectAmbush)
				}
			}
		}
		w.Beats = append(w.Beats, beat)
	}
	// the finale always plays when the party reaches the lair
	w.Beats = append(w.Beats, &game.Beat{ID: w.NewID("beat"), Title: "The confrontation", Trigger: game.TriggerEnterLocation, Target: lair.ID,
		Text: ant.Name + " is here and confronts the party. Make the stakes plain.", Effects: []string{game.EffectAmbush}, EffectTarget: ant.ID})
	return w, nil
}

// withEffect adds e once, keeping bring_npc ahead of ambush so a figure
// is on the scene before it attacks.
func withEffect(es []string, e string) []string {
	for _, x := range es {
		if x == e {
			return es
		}
	}
	if e == game.EffectBringNPC {
		return append([]string{e}, es...)
	}
	return append(es, e)
}

func connectedToHub(w *game.World, loc string) bool {
	for _, e := range w.Edges {
		if e.Touches(loc) {
			return true
		}
	}
	return false
}

func keywordsOf(name string) []string {
	var out []string
	for _, f := range strings.Fields(strings.ToLower(name)) {
		if len(f) > 3 {
			out = append(out, strings.Trim(f, ".,'\""))
		}
	}
	return out
}

func trim(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n]
	}
	return s
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// fallbackSpec is a deterministic campaign used when authoring fails.
func fallbackSpec(premise string) CampaignSpec {
	var s CampaignSpec
	raw := `{
	 "title": "The Bell Beneath Harrowmere",
	 "pitch": "Something rings under the old mere at night, and the dead are listening. Find the source before the town of Harrowmere joins them.",
	 "antagonist": {"name": "Morwen the Pale", "role": "necromancer", "desc": "A gaunt woman in grey whom nobody remembers arriving.", "motive": "Morwen is ringing the sunken bell to wake the drowned army of the old kingdom and claim it."},
	 "hub": {"name": "Harrowmere", "kind": "town", "description": "A damp lakeside town of slate roofs and nervous fishermen. The bell's toll carries over the water each midnight."},
	 "hub_npcs": [
	  {"name": "Tamsin Hale", "role": "blacksmith", "desc": "Broad, soot-streaked, and blunt.", "disposition": 1, "merchant": true, "stock": ["healing_potion","healing_potion","longsword","shortbow","leather","shield","torch"]},
	  {"name": "Elder Corbin", "role": "elder", "desc": "The town's frail, sharp-eyed elder.", "disposition": 1}
	 ],
	 "places": [
	  {"name": "Reedmarsh", "hint": "Fishermen say lights move in the reeds.", "kind": "wilderness", "danger": 1, "connects_to": "Harrowmere"},
	  {"name": "Drowned Chapel", "hint": "A chapel half-sunk in the shallows.", "kind": "ruin", "danger": 2, "connects_to": "Reedmarsh"},
	  {"name": "Old Quarry", "hint": "Abandoned since the cave-in.", "kind": "cave", "danger": 2, "connects_to": "Harrowmere"}
	 ],
	 "lair": {"name": "The Bell Vault", "hint": "Where the bell hangs beneath the water.", "connects_to": "Drowned Chapel"},
	 "main_quest": {"title": "The Sunken Bell", "summary": "Learn who rings the bell and stop them.", "objectives": [
	  {"kind": "talk_to_npc", "text": "Hear Elder Corbin's account of the bell", "target": "Elder Corbin"},
	  {"kind": "reach_location", "text": "Search the Drowned Chapel", "target": "Drowned Chapel"},
	  {"kind": "defeat_enemy", "text": "Defeat Morwen the Pale", "target": "Morwen the Pale"}
	 ]},
	 "side_quest": {"title": "Lights in the Reeds", "summary": "Tamsin wants to know what is scaring off the fishermen.", "giver": "Tamsin Hale",
	  "objectives": [{"kind": "reach_location", "text": "Scout the Reedmarsh", "target": "Reedmarsh"}, {"kind": "defeat_enemy", "text": "Drive off the marsh wolves", "target": "Wolf"}], "reward_gold": 30, "reward_xp": 60},
	 "lore": [
	  {"title": "The drowned army", "text": "The old kingdom's soldiers were sunk with the bell; its toll stirs them.", "keywords": ["bell", "drowned", "army"], "always_on": true},
	  {"title": "Corbin's guilt", "text": "Elder Corbin sold Morwen the map to the chapel, not knowing who she was.", "keywords": ["corbin", "map"]},
	  {"title": "Chapel history", "text": "The Drowned Chapel was built over the bell to keep it silent.", "keywords": ["chapel"]}
	 ],
	 "beats": [
	  {"title": "Midnight toll", "text": "The bell tolls across the water; every dog in town howls.", "trigger": "turn", "value": 6},
	  {"title": "Something in the water", "text": "Pale hands break the surface of the shallows.", "trigger": "enter_location", "target": "Drowned Chapel", "effects": ["ambush"]},
	  {"title": "A hidden stair", "text": "Water drains from a crack, revealing old steps.", "trigger": "stalled", "value": 6, "effects": ["reveal_exit"]}
	 ]
	}`
	_ = json.Unmarshal([]byte(raw), &s)
	return s
}

// DraftCharacter asks the author for a sheet from a concept. The result is
// repaired into a valid sheet (never rejected) so players can edit it.
func (g *GM) DraftCharacter(ctx context.Context, w *game.World, concept string) game.CharacterSpec {
	concept = trim(concept, 300)
	t := tool("draft_character", "Draft a level 1 character sheet.", obj(map[string]any{
		"name": str("character name"), "concept": str("one-line concept"), "class": str("flavor class name, e.g. Hedge Witch"),
		"kit":       enum("starting kit", game.KitKeys()...),
		"backstory": str("two sentences tied to the campaign"),
		"pronouns":  str("he/him, she/her or they/them, as the concept implies; they/them if it does not say"),
		"scores": obj(map[string]any{"STR": num("", 8, 15), "DEX": num("", 8, 15), "CON": num("", 8, 15), "INT": num("", 8, 15), "WIS": num("", 8, 15), "CHA": num("", 8, 15)},
			"STR", "DEX", "CON", "INT", "WIS", "CHA"),
		"proficient": arr(abilityEnum, "exactly 2 abilities"),
	}, "name", "kit", "scores", "proficient"))
	var kits []string
	for _, k := range game.KitKeys() {
		kits = append(kits, fmt.Sprintf("%s (%s)", k, game.Kits[k].Desc))
	}
	title, taken := "", ""
	if w != nil {
		title = w.Campaign.Title + ": " + w.Campaign.Pitch
		var names []string
		for _, c := range w.Characters {
			names = append(names, c.Name)
		}
		if len(names) > 0 {
			taken = "\nAlready in the party (use a clearly different name): " + strings.Join(names, ", ") + "."
		}
		var people []string
		for _, n := range w.NPCs {
			people = append(people, n.Name)
		}
		if len(people) > 0 {
			// a shared first name ("Serafina") makes "I talk to Serafina" ambiguous
			taken += "\nPeople in the story (share no first or last name with them): " + strings.Join(people, ", ") + "."
		}
	}
	req := llm.Request{Tag: "author_character", Temperature: 0.8, ForceTool: "draft_character", Tools: []llm.Tool{t},
		Messages: []llm.Message{llm.System(authorSystem), llm.User(fmt.Sprintf(
			"Campaign: %s\nPlayer's concept: %s%s\n\nKits: %s.\nAssign the standard array 15, 14, 13, 12, 10, 8 to the six abilities, each value exactly once, and pick 2 proficient abilities. Classes are flavor only.",
			title, firstOf(concept, "a capable adventurer"), taken, strings.Join(kits, "; ")))}}
	var spec game.CharacterSpec
	calls, err := g.forced(ctx, req, nil)
	if err == nil && len(calls) > 0 {
		_ = calls[0].Decode(&spec)
	}
	spec = RepairCharacter(spec, concept)
	if w != nil {
		spec.Name = uniqueName(w, spec)
	}
	return spec
}

// uniqueName keeps a drafted name from clashing with someone already in
// the party (models like to reuse a name they were just shown).
func uniqueName(w *game.World, s game.CharacterSpec) string {
	taken := func(n string) bool {
		for _, c := range w.Characters {
			if strings.EqualFold(strings.TrimSpace(c.Name), strings.TrimSpace(n)) {
				return true
			}
		}
		return false
	}
	if !taken(s.Name) {
		return s.Name
	}
	for _, n := range []string{trim(s.Name+" the "+firstOf(s.Class, game.Kits[s.Kit].Name), 40), fallbackName(s.Concept + " " + s.Name)} {
		if n != "" && !taken(n) {
			return n
		}
	}
	for i := 2; ; i++ {
		if n := trim(fmt.Sprintf("%s %d", s.Name, i), 40); !taken(n) {
			return n
		}
	}
}

// RepairCharacter makes any draft valid: kit by keyword, standard array by
// the draft's ranking, two proficiencies.
func RepairCharacter(s game.CharacterSpec, concept string) game.CharacterSpec {
	if s.Concept == "" {
		s.Concept = concept
	}
	if strings.TrimSpace(s.Name) == "" {
		s.Name = fallbackName(concept)
	}
	s.Name = trim(s.Name, 40)
	if _, ok := game.Kits[s.Kit]; !ok {
		s.Kit = kitFor(s.Kit + " " + s.Class + " " + concept)
	}
	if _, _, p := game.ValidateCharacter(s); len(p) == 0 {
		return s
	}
	kit := game.Kits[s.Kit]
	// rank abilities by drafted score, kit suggestions first on ties
	order := append([]game.Ability{}, kit.Suggested...)
	for _, a := range game.Abilities {
		if a != kit.Suggested[0] && a != kit.Suggested[1] {
			order = append(order, a)
		}
	}
	drafted := map[game.Ability]int{}
	for k, v := range s.Scores {
		if a, ok := game.ParseAbility(k); ok {
			drafted[a] = v
		}
	}
	ranked := append([]game.Ability{}, order...)
	for i := 1; i < len(ranked); i++ {
		for j := i; j > 0 && drafted[ranked[j]] > drafted[ranked[j-1]]; j-- {
			ranked[j], ranked[j-1] = ranked[j-1], ranked[j]
		}
	}
	s.Scores = map[string]int{}
	for i, a := range ranked {
		s.Scores[string(a)] = game.StandardArray[i]
	}
	var prof []string
	seen := map[game.Ability]bool{}
	for _, p := range s.Proficient {
		if a, ok := game.ParseAbility(p); ok && !seen[a] && len(prof) < 2 {
			seen[a] = true
			prof = append(prof, string(a))
		}
	}
	for _, a := range kit.Suggested {
		if len(prof) < 2 && !seen[a] {
			seen[a] = true
			prof = append(prof, string(a))
		}
	}
	s.Proficient = prof
	return s
}

func kitFor(text string) string {
	t := strings.ToLower(text)
	pick := []struct {
		kit   string
		words []string
	}{
		{"arcanist", []string{"wizard", "mage", "sorcer", "witch", "warlock", "arcan", "scholar", "spell"}},
		{"devout", []string{"cleric", "priest", "paladin", "monk", "holy", "faith", "devout", "healer", "nun"}},
		{"bard", []string{"bard", "singer", "musician", "poet", "minstrel", "actor"}},
		{"ranger", []string{"ranger", "hunter", "archer", "scout", "druid", "bow", "tracker"}},
		{"skirmisher", []string{"rogue", "thief", "assassin", "skirmish", "knife", "sneak", "burglar", "spy"}},
		{"warrior", []string{"fighter", "warrior", "knight", "soldier", "barbarian", "guard", "mercenary"}},
	}
	for _, p := range pick {
		for _, w := range p.words {
			if strings.Contains(t, w) {
				return p.kit
			}
		}
	}
	return "warrior"
}

// arrivalHooks returns engine hooks that author stubs, zones and ambushes.
func (g *GM) arrivalHooks(ctx context.Context, w *game.World) (func(*game.Location), func(*game.Location) []game.EnemySpec) {
	onArrive := func(l *game.Location) {
		if l.Stub {
			g.fillLocation(ctx, w, l)
		}
		if len(l.Zones) == 0 {
			g.defineZones(ctx, w, l)
		}
	}
	ambush := func(l *game.Location) []game.EnemySpec { return g.authorEnemies(ctx, w, l, "an ambush on the road") }
	return onArrive, ambush
}

func (g *GM) canonContext(w *game.World) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Campaign: %s. %s\nPremise: %s\n", w.Campaign.Title, w.Campaign.Pitch, w.Campaign.Premise)
	if ant := w.NPC(w.Campaign.AntagonistID); ant != nil {
		fmt.Fprintf(&b, "Antagonist: %s (%s).\n", ant.Name, ant.Role)
	}
	for _, l := range w.Lore {
		if l.AlwaysOn {
			fmt.Fprintf(&b, "Canon: %s\n", l.Text)
		}
	}
	return b.String()
}

func (g *GM) fillLocation(ctx context.Context, w *game.World, l *game.Location) {
	var neighbours, npcs []string
	for _, x := range w.Exits(l.ID, true) {
		neighbours = append(neighbours, x.To.Name)
	}
	for _, n := range w.NPCs {
		npcs = append(npcs, n.Name)
	}
	var here []string
	for _, it := range w.ItemsOf(game.OwnerLocation, l.ID) {
		here = append(here, it.Name)
	}
	already := ""
	if len(here) > 0 {
		already = "\nAlready lying here (mention it in the description; do not add it again): " + strings.Join(here, ", ") + "."
	}
	npc := obj(map[string]any{"name": str("name (reuse an existing NPC's exact name only if it is the same person)"), "role": str("role"), "desc": str("one sentence"),
		"disposition": num("-3..3", -3, 3), "merchant": boolean("sells goods"), "stock": arr(str("catalog key"), "merchant stock")}, "name", "role")
	t := tool("fill_location", "Describe this place on the party's first arrival.", obj(map[string]any{
		"description": str("2-3 sentences of what the party sees, hears and smells"),
		"npcs":        arr(npc, "0-2 NPCs here (none in dangerous wilds unless fitting)"),
		"items":       arr(obj(map[string]any{"key": str("catalog key or empty"), "name": str("name"), "kind": str("treasure, gear, key, consumable, quest"), "desc": str("short"), "value": num("gold value", 0, 100)}), "0-2 items on the ground"),
		"exits": arr(obj(map[string]any{"to": str("destination name (an existing place's exact name links to it)"), "label": str("how the way looks"), "hint": str("rumour"),
			"kind":   enum("kind", game.KindTown, game.KindWilderness, game.KindDungeon, game.KindCave, game.KindBuilding, game.KindRuin),
			"danger": num("0..4", 0, 4), "hidden": boolean("needs a search to find"), "locked": boolean("locked"), "key_item": str("name of the key that opens it")}, "to", "label", "hint"), "1-3 new ways onward"),
		"lore": arr(obj(map[string]any{"title": str("title"), "text": str("SECRET history or truth of this place")}, "title", "text"), "0-2 secret lore entries"),
	}, "description", "exits"))
	req := llm.Request{Tag: "author_location", Temperature: 0.8, ForceTool: "fill_location", Tools: []llm.Tool{t},
		Messages: []llm.Message{llm.System(authorSystem), llm.User(fmt.Sprintf(
			"%s\nThe party arrives at %q (%s, danger %d/5). Rumour: %s\nIt connects to: %s.\nExisting NPCs: %s.%s\nCatalog keys: %s.",
			g.canonContext(w), l.Name, l.Kind, l.Danger, l.Hint, strings.Join(neighbours, ", "), strings.Join(npcs, ", "), already, strings.Join(game.CatalogKeys(), ", ")))}}
	var fill struct {
		game.LocationFill
		Lore []struct{ Title, Text string } `json:"lore"`
	}
	calls, err := g.forced(ctx, req, func(cs []llm.ToolCall) []string {
		if err := cs[0].Decode(&fill); err != nil {
			return []string{"bad arguments: " + err.Error()}
		}
		if strings.TrimSpace(fill.Description) == "" {
			return []string{"description is required"}
		}
		return nil
	})
	if err != nil && len(calls) == 0 {
		g.logf("fill_location %s failed: %v", l.Name, err)
		fill.LocationFill = game.LocationFill{Description: l.Hint}
	}
	if fill.Description == "" {
		fill.Description = l.Hint
	}
	if l.IsLair {
		fill.Exits = nil // the lair is a dead end
		fill.NPCs = nil
	}
	notes, ferr := game.FillLocation(w, l.ID, fill.LocationFill)
	if ferr != nil {
		g.logf("fill_location %s: %v", l.Name, ferr)
	}
	if len(notes) > 0 {
		g.debugf("fill %s: %v", l.Name, notes)
	}
	for i, lo := range fill.Lore {
		if i < 2 && strings.TrimSpace(lo.Text) != "" {
			w.Lore = append(w.Lore, &game.Lore{ID: w.NewID("lore"), Title: trim(lo.Title, 80), Text: trim(lo.Text, 400), EntityIDs: []string{l.ID}, Keywords: keywordsOf(l.Name), LastTrigger: -100})
		}
	}
}

func (g *GM) defineZones(ctx context.Context, w *game.World, l *game.Location) {
	var exits []string
	for _, x := range w.Exits(l.ID, true) {
		exits = append(exits, fmt.Sprintf("%s (%s to %s)", x.Edge.ID, x.Label, x.To.Name))
	}
	var npcs []string
	for _, n := range w.NPCsAt(l.ID) {
		npcs = append(npcs, fmt.Sprintf("%s (%s, %s)", n.ID, n.Name, n.Role))
	}
	if len(npcs) == 0 {
		npcs = []string{"none"}
	}
	t := tool("define_zones", "Split the location into 2-6 named zones for tactical positioning.", obj(map[string]any{
		"zones": arr(obj(map[string]any{
			"name": str("short zone name, e.g. Cave Mouth"), "desc": str("one short sentence"), "cover": boolean("offers cover (crates, pillars, trees)"),
			"adjacent": arr(str("names of neighbouring zones"), "neighbours"), "exits": arr(str("exit ids that leave from this zone"), "exits here"),
			"npcs": arr(str("npc ids standing in this zone"), "people here"),
		}, "name", "adjacent"), "2-6 zones, all connected"),
	}, "zones"))
	req := llm.Request{Tag: "author_zones", Temperature: 0.5, ForceTool: "define_zones", Tools: []llm.Tool{t},
		Messages: []llm.Message{llm.System(authorSystem), llm.User(fmt.Sprintf(
			"Location: %s (%s). %s\nExits (assign each to one zone): %s.\nPeople (place each where their work or role puts them): %s.\nMake 2-6 zones; at least one should offer cover.",
			l.Name, l.Kind, l.Description, strings.Join(exits, "; "), strings.Join(npcs, "; ")))}}
	_, err := g.forced(ctx, req, func(cs []llm.ToolCall) []string {
		var z struct {
			Zones []game.ZoneSpec `json:"zones"`
		}
		if err := cs[0].Decode(&z); err != nil {
			return []string{"bad arguments: " + err.Error()}
		}
		if err := game.DefineZones(w, l.ID, z.Zones); err != nil {
			return []string{err.Error()}
		}
		return nil
	})
	if err != nil || len(l.Zones) == 0 {
		g.logf("define_zones %s failed (%v); using generic layout", l.Name, err)
		game.EnsureZones(w, l.ID)
	}
}

// authorEnemies asks for an encounter suited to the place; nil means the
// caller should use generic enemies.
func (g *GM) authorEnemies(ctx context.Context, w *game.World, l *game.Location, why string) []game.EnemySpec {
	t := tool("spawn_enemies", "Choose the enemies for this fight.", obj(map[string]any{"enemies": arr(enemySchema, "1-3 enemy types")}, "enemies"))
	caps := game.CapsFor(l.Danger, w.PartyLevel())
	req := llm.Request{Tag: "author_enemies", Temperature: 0.7, ForceTool: "spawn_enemies", Tools: []llm.Tool{t},
		Messages: []llm.Message{llm.System(authorSystem), llm.User(fmt.Sprintf(
			"%s\nLocation: %s (%s, danger %d). Reason: %s. Party: %d characters of level %d.\nKeep each enemy at most %d HP, AC %d, +%d to hit.",
			g.canonContext(w), l.Name, l.Kind, l.Danger, why, len(w.Characters), w.PartyLevel(), caps.MaxHP, caps.MaxAC, caps.MaxAttack))}}
	calls, err := g.forced(ctx, req, nil)
	if err != nil || len(calls) == 0 {
		return nil
	}
	var a struct {
		Enemies []game.EnemySpec `json:"enemies"`
	}
	if calls[0].Decode(&a) != nil {
		return nil
	}
	// the antagonist is a real NPC waiting in the lair; a stand-in with
	// the same name would be a fake villain
	var out []game.EnemySpec
	for _, e := range a.Enemies {
		if ant := w.NPC(w.Campaign.AntagonistID); ant != nil && (game.NameContains(e.Name, ant.Name) || game.NameContains(ant.Name, e.Name)) {
			continue
		}
		out = append(out, e)
	}
	return out
}

var fallbackNames = []string{"Wren", "Bram", "Ilse", "Corvin", "Maud", "Tobiah", "Sera", "Aldric", "Nell", "Fenwick", "Odile", "Rook"}

// fallbackName picks a stable name for a concept when the model gave none.
func fallbackName(concept string) string {
	h := 0
	for _, r := range concept {
		h = h*31 + int(r)
	}
	if h < 0 {
		h = -h
	}
	return fallbackNames[h%len(fallbackNames)]
}
