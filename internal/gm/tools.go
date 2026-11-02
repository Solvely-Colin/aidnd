package gm

import (
	"strings"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/llm"
)

// JSON Schema helpers.

func obj(props map[string]any, required ...string) map[string]any {
	o := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		o["required"] = required
	}
	return o
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

func num(desc string, min, max int) map[string]any {
	return map[string]any{"type": "integer", "description": desc, "minimum": min, "maximum": max}
}

func boolean(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}

func enum(desc string, vals ...string) map[string]any {
	return map[string]any{"type": "string", "description": desc, "enum": vals}
}

func intEnum(desc string, vals ...int) map[string]any {
	return map[string]any{"type": "integer", "description": desc, "enum": vals}
}

func arr(items map[string]any, desc string) map[string]any {
	return map[string]any{"type": "array", "items": items, "description": desc}
}

func tool(name, desc string, params map[string]any) llm.Tool {
	return llm.Tool{Name: name, Description: desc, Parameters: params}
}

var abilityEnum = enum("ability", "STR", "DEX", "CON", "INT", "WIS", "CHA")

// interpreterTools are the player-action tools.
func interpreterTools() []llm.Tool {
	return []llm.Tool{
		tool(game.ActLook, "FREE. Look around, examine something, or recall what you know.", obj(map[string]any{
			"target": str("optional id or name of an enemy, NPC, item, zone or exit to examine")})),
		tool(game.ActSay, "FREE. A quick remark or line of dialogue not aimed at a conversation with an NPC.", obj(map[string]any{
			"text": str("what the character says")}, "text")),
		tool(game.ActEquip, "FREE. Draw, wield, don or ready an item the character carries.", obj(map[string]any{
			"item": str("item id from INVENTORY")}, "item")),
		tool(game.ActDrop, "FREE. Drop a carried item on the ground.", obj(map[string]any{"item": str("item id from INVENTORY")}, "item")),
		tool(game.ActEndTurn, "Ends the turn now (skip / wait / end turn / pass).", obj(map[string]any{})),
		tool(game.ActMove, "MOVE. Go through an exit to another location. In combat this is fleeing, costs the ACTION and requires standing in the exit's zone.", obj(map[string]any{
			"exit": str("exit id from EXITS")}, "exit")),
		tool(game.ActReposition, "MOVE. Move to another zone in this location (charge, retreat, take cover, approach). In combat 1 zone costs the move, 2 zones costs move+action.", obj(map[string]any{
			"zone": str("zone id from ZONES")}, "zone")),
		tool(game.ActTravel, "MOVE. Fast-travel to a previously visited location (not in combat).", obj(map[string]any{
			"destination": str("location id from KNOWN PLACES")}, "destination")),
		tool(game.ActAttack, "ACTION. Attack an enemy with the equipped weapon (or a named carried weapon). Must be in reach. With Extra Attack the engine makes both attacks.", obj(map[string]any{
			"target": str("enemy id from ENEMIES (or an NPC id only if the player clearly attacks them)"),
			"weapon": str("optional item id of a carried weapon to use"),
			"option": enum("optional: smite spends a spell slot on Divine Smite if it hits (paladins); stun spends focus on Stunning Strike (monks level 5). Only when the player asks for it", "smite", "stun")}, "target")),
		tool(game.ActCast, "Cast one of the actor's spells listed in SPELLS (cost shown there: ACTION or BONUS). Use it whenever the player casts, prays, channels or uses a power that matches a known spell, by its name in this setting. Damage spells need a target enemy or zone; healing and buffs take a party member (empty = self).", obj(map[string]any{
			"spell":  str("spell key from SPELLS"),
			"target": str("enemy id or party member id"),
			"zone":   str("zone id for spells that fill a zone (fireball, web) or misty step's destination"),
			"level":  num("optional slot level to cast with (higher levels are stronger); 0 = lowest available", 0, 3)}, "spell")),
		tool(game.ActFeature, "Use a class or species ability from ABILITIES (rage, second wind, cunning action, flurry of blows, lay on hands, wild shape...). Cost is shown there; bonus actions can come before or after the main action in the same message.", obj(map[string]any{
			"feature": str("ability key from ABILITIES"),
			"target":  str("optional enemy or party member id"),
			"zone":    str("optional zone id"),
			"item":    str("item id for fast hands"),
			"amount":  num("lay on hands: HP to heal; create slot: slot level", 0, 50)}, "feature")),
		tool(game.ActSubdue, "ACTION. Non-lethal takedown: grapple, tie up, intimidate, calm or persuade an enemy to stop fighting.", obj(map[string]any{
			"target": str("enemy id"),
			"method": enum("how", "grapple", "tie_up", "intimidate", "calm", "persuade")}, "target", "method")),
		tool(game.ActCheck, "ACTION. A skill or ability check for anything risky or uncertain: searching (finds hidden exits), picking or forcing a lock, climbing, sneaking, persuading, recalling lore under pressure, stabilizing a dying ally.", obj(map[string]any{
			"skill":   skillEnum,
			"ability": abilityEnum,
			"dc":      intEnum("difficulty: 5 trivial, 10 easy, 15 medium, 20 hard, 25 very hard", 5, 10, 15, 20, 25),
			"purpose": enum("search reveals hidden exits; unlock/force targets a locked exit; escape breaks free when the character is restrained or grappled; stabilize stops a dying ally's death saves (Medicine, DC 10). The engine sets those DCs", "search", "unlock", "force", "escape", "stabilize", "other"),
			"target":  str("optional exit id for unlock/force, or the dying party member's id for stabilize"),
			"text":    str("what the check is for, in a few words")}, "dc", "purpose", "text")),
		tool(game.ActUseItem, "ACTION. Use an item: drink a potion, apply a healer's kit (can revive a downed ally), use a key or tools on a locked exit.", obj(map[string]any{
			"item":   str("item id from INVENTORY"),
			"target": str("optional party member id or exit id")}, "item")),
		tool(game.ActTalkTo, "ACTION in combat, FREE otherwise. Have a conversation with an NPC (ask, persuade, haggle, ask for work).", obj(map[string]any{
			"npc":     str("NPC id from NPCS"),
			"text":    str("what the character says or asks"),
			"promise": str("only if the character gives their word (a promise, oath, deal or debt): what they promised, in a few words")}, "npc")),
		tool(game.ActBuy, "ACTION in combat, FREE otherwise. Buy an item from a merchant here.", obj(map[string]any{"npc": str("merchant NPC id"), "item": str("item id from the merchant's STOCK")}, "npc", "item")),
		tool(game.ActSell, "ACTION in combat, FREE otherwise. Sell a carried item to a merchant here.", obj(map[string]any{"npc": str("merchant NPC id"), "item": str("item id from INVENTORY")}, "npc", "item")),
		tool(game.ActAcceptQuest, "FREE. Accept a quest an NPC here has offered.", obj(map[string]any{"quest": str("quest id from QUESTS ON OFFER")}, "quest")),
		tool(game.ActTurnInQuest, "FREE. Turn in a completed quest to its giver (must be at the giver's location).", obj(map[string]any{"quest": str("quest id")}, "quest")),
		tool(game.ActPickUp, "ACTION. Pick up an item from the ground.", obj(map[string]any{"item": str("item id from GROUND")}, "item")),
		tool(game.ActGiveItem, "ACTION. Hand a carried item to a party member or an NPC here.", obj(map[string]any{
			"item": str("item id from INVENTORY"), "to": str("party member id or NPC id")}, "item", "to")),
		tool(game.ActRest, "ACTION. Rest the party (not in combat). Full rest only in safe places.", obj(map[string]any{})),
		tool(game.ActDodge, "ACTION, combat only. Dodge: focus on defense; attacks against the character have disadvantage until their next turn.", obj(map[string]any{})),
		tool(game.ActDisengage, "ACTION, combat only. Disengage: back off carefully so leaving an enemy's zone this turn draws no opportunity attacks. Usually followed by a reposition.", obj(map[string]any{})),
		tool(game.ActHelp, "ACTION. Help a party member: they get advantage on their next attack or check (in combat they must share the zone).", obj(map[string]any{
			"target": str("party member id")}, "target")),
		tool(game.ActHide, "ACTION, combat only. Hide (Stealth against the enemies' passive Perception): needs cover or no enemy in the zone. Hidden characters can't be targeted and their next attack has advantage.", obj(map[string]any{})),
		tool(game.ActReady, "ACTION, combat only. Ready an attack: it strikes the first enemy (or the named one) that comes within reach or acts within reach before the character's next turn.", obj(map[string]any{
			"target": str("optional enemy id to wait for")})),
		tool(game.ActShove, "ACTION. Shove an enemy in the same zone: knock it prone, or push it into the next zone (it makes a STR save).", obj(map[string]any{
			"target": str("enemy id"),
			"method": enum("prone knocks it down; push drives it into a neighbouring zone", "prone", "push")}, "target", "method")),
		tool(game.ActGrapple, "ACTION. Grab an enemy in the same zone and hold it (it makes a STR save; held, it can't move and is easier to hit until it breaks free).", obj(map[string]any{
			"target": str("enemy id")}, "target")),
	}
}

var skillEnum = enum("the skill the check uses (it sets the ability): athletics climb/jump/swim/force, acrobatics balance/tumble, sleight_of_hand pick pockets/locks, stealth sneak, arcana magic, history, investigation deduce/search a thing, nature, religion, animal_handling, insight read someone, medicine, perception notice, survival track/forage, deception lie, intimidation threaten, performance, persuasion convince", game.SkillKeys()...)

// toAction converts an interpreter tool call to an engine action.
func toAction(tc llm.ToolCall) (game.Action, error) {
	var a struct {
		Target, Weapon, Method, Ability, Skill, Purpose, Text, Item, Exit, Zone, Destination, NPC, Quest, To, Promise string
		Spell, Feature, Option                                                                                        string
		DC, Level, Amount                                                                                             int
	}
	if err := tc.Decode(&a); err != nil {
		return game.Action{}, err
	}
	act := game.Action{Kind: tc.Function.Name, Text: a.Text}
	switch act.Kind {
	case game.ActLook:
		act.Target = a.Target
	case game.ActEquip, game.ActDrop, game.ActPickUp:
		act.Item = a.Item
	case game.ActMove:
		act.Target = firstOf(a.Exit, a.Target)
		act.Zone = a.Zone
	case game.ActReposition:
		act.Zone = firstOf(a.Zone, a.Target)
	case game.ActTravel:
		act.Target = firstOf(a.Destination, a.Target)
	case game.ActAttack:
		act.Target, act.Item, act.Option = a.Target, a.Weapon, a.Option
	case game.ActCast:
		act.Spell, act.Target, act.Zone, act.Level = a.Spell, a.Target, a.Zone, a.Level
	case game.ActFeature:
		act.Feature, act.Target, act.Zone, act.Item, act.Level = a.Feature, a.Target, a.Zone, a.Item, a.Amount
	case game.ActSubdue:
		act.Target, act.Method = a.Target, a.Method
	case game.ActCheck:
		act.Ability, act.Skill, act.DC, act.Purpose, act.Target = a.Ability, a.Skill, a.DC, a.Purpose, a.Target
	case game.ActHelp, game.ActReady, game.ActGrapple:
		act.Target = a.Target
	case game.ActShove:
		act.Target, act.Method = a.Target, a.Method
	case game.ActUseItem:
		act.Item, act.Target = a.Item, a.Target
	case game.ActTalkTo:
		act.Target, act.Promise = firstOf(a.NPC, a.Target), a.Promise
	case game.ActBuy, game.ActSell:
		act.Target, act.Item = firstOf(a.NPC, a.Target), a.Item
	case game.ActAcceptQuest, game.ActTurnInQuest:
		act.Target = firstOf(a.Quest, a.Target)
	case game.ActGiveItem:
		act.Item, act.Target = a.Item, firstOf(a.To, a.Target)
	}
	return act, nil
}

func firstOf(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// Directive tool names.
const (
	DirSpawnEncounter = "spawn_encounter"
	DirOfferQuest     = "offer_quest"
	DirRevealExit     = "reveal_exit"
	DirAddExit        = "add_exit"
	DirPlaceItem      = "place_item"
	DirStockMerchant  = "stock_merchant"
	DirNPCNote        = "npc_note"
	DirAdjustDisp     = "adjust_disposition"
	DirNPCGivesItem   = "npc_gives_item"
	DirRecordLore     = "record_lore"
	DirResolvePromise = "resolve_promise"
	DirAdjustTension  = "adjust_tension"
	DirEndEncounter   = "end_encounter"
	DirHazard         = "hazard"
	DirMarkCharacter  = "mark_character"
	DirNPCTie         = "npc_tie"
	DirPersonalGoal   = "personal_goal"
	DirRecordChoice   = "record_choice"
)

// progressDirectives are the only tools offered when pacing forces progress.
var progressDirectives = map[string]bool{DirRevealExit: true, DirAddExit: true, DirSpawnEncounter: true, DirOfferQuest: true}

var enemySchema = obj(map[string]any{
	"name":         str("enemy name, e.g. Ghoul"),
	"count":        num("how many of this enemy", 1, 4),
	"hp":           num("hit points each", 1, 60),
	"ac":           num("armor class", 8, 18),
	"attack_bonus": num("to-hit bonus", 0, 8),
	"damage":       str("damage dice, e.g. 1d6+1"),
	"ranged":       boolean("attacks from range"),
	"inflicts":     enum("optional: a condition its hits force a save against (venom poisons, a charge knocks prone, webs or nets restrain, a dread wail frightens, a flash or spray blinds, a crushing blow stuns; stunned only where danger is 3+). Leave empty for ordinary foes", game.ConditionKinds...),
	"npc_id":       str("optional: id of an NPC present who turns hostile"),
	"boss":         obj(map[string]any{"move": str("signature attack, 2-4 words")}, "move"),
}, "name", "hp", "ac", "attack_bonus", "damage")

var objectiveSchema = obj(map[string]any{
	"kind":       enum("objective type", game.ObjectiveKinds...),
	"text":       str("short player-facing description of exactly the one action that completes it, e.g. \"Ask Constable Vane about the drownings\" for talk_to_npc; never two actions in one objective"),
	"target":     str("location id/name, enemy name, item name, or npc id/name depending on kind; talk_to_npc is done by talking to or examining that NPC"),
	"deliver_to": str("npc id/name for deliver_item"),
}, "kind", "text")

var questSchema = map[string]any{
	"title":        str("quest title"),
	"summary":      str("one or two sentences"),
	"giver_id":     str("id of the NPC offering it (must be present)"),
	"objectives":   arr(objectiveSchema, "1-4 objectives"),
	"reward_gold":  num("gold paid at turn-in", 0, 500),
	"reward_xp":    num("xp at turn-in (a level takes several hundred; 30-80 is typical)", 0, 150),
	"reward_items": arr(obj(map[string]any{"key": str("catalog key, or empty"), "name": str("name for a custom item"), "kind": str("item kind"), "value": num("value in gold", 0, 500)}), "0-2 items held by the giver until turn-in"),
}

func directiveTools() []llm.Tool {
	return []llm.Tool{
		tool(DirSpawnEncounter, "Start a fight here (not if one is under way). Enemies are capped by the engine. Give at most one enemy a boss block, only for a named, dangerous leader where danger is 3+: it gets phases and a signature move announced a turn ahead.", obj(map[string]any{
			"enemies": arr(enemySchema, "1-3 enemy types")}, "enemies")),
		tool(DirOfferQuest, "An NPC present offers a side quest.", obj(questSchema, "title", "giver_id", "objectives")),
		tool(DirRevealExit, "Reveal a hidden exit listed in SECRETS.", obj(map[string]any{"exit_id": str("hidden exit id")}, "exit_id")),
		tool(DirAddExit, "Add a new path from here to a new place (or an existing place by exact name).", obj(map[string]any{
			"name": str("destination name"), "label": str("how the way looks from here"), "hint": str("one-line hint about the destination"),
			"kind":   enum("kind", game.KindTown, game.KindWilderness, game.KindDungeon, game.KindCave, game.KindBuilding, game.KindRuin),
			"danger": num("0 safe .. 4 dangerous", 0, 4)}, "name", "label", "hint")),
		tool(DirPlaceItem, "Put a modest item on the ground here.", obj(map[string]any{
			"key": str("catalog key, or empty"), "name": str("name if not a catalog item"), "kind": str("item kind"), "desc": str("short description"), "value": num("value", 0, 100)})),
		tool(DirStockMerchant, "Add catalog items to a merchant's stock here.", obj(map[string]any{
			"npc_id": str("merchant id"), "keys": arr(str("catalog key"), "1-4 catalog keys")}, "npc_id", "keys")),
		tool(DirNPCNote, "Record a short memory for an NPC about the party (what they did/said, what the NPC told them).", obj(map[string]any{
			"npc_id": str("npc id"), "note": str("under 20 words"),
			"important": boolean("true for something they will never forget: a threat, a kindness, a secret shared")}, "npc_id", "note")),
		tool(DirAdjustDisp, "Shift an NPC's attitude by one step after how the party treated them.", obj(map[string]any{
			"npc_id": str("npc id"), "delta": num("-1 or +1", -1, 1), "reason": str("what the party did, under 15 words")}, "npc_id", "delta", "reason")),
		tool(DirNPCTie, "One NPC here now holds something toward ONE character, not the whole party: a grudge (they wronged, threatened, shamed or cheated them), a debt (the character saved or helped them and they owe it), owed (the character took a favor, loan or help and now owes them), or settle (an old grudge forgiven or a debt repaid). Use it when what one character did will follow them personally.", obj(map[string]any{
			"npc_id":       str("npc id (present)"),
			"character_id": str("character id"),
			"kind":         enum("tie", game.TieGrudge, game.TieDebt, game.TieOwed, "settle"),
			"reason":       str("what happened, under 12 words, e.g. \"pulled her from the harbor\"")}, "npc_id", "character_id", "kind", "reason")),
		tool(DirMarkCharacter, "Give one character a lasting mark from something that just happened in this scene and will show for the rest of the campaign: scar (a wound that heals but shows), brand (a cult sign, a curse, a mark others can see), name (what people have started calling them after a deed). Rare: only for a turning point, never for ordinary hits.", obj(map[string]any{
			"character_id": str("character id"),
			"kind":         enum("mark", game.NarratorMarkKinds...),
			"text":         str("how it shows, under 12 words, e.g. \"a ring of sucker scars around the left wrist\" or \"the Bell-ringer of Innsmouth\"")}, "character_id", "kind", "text")),
		tool(DirRecordChoice, "Write one line in a character's own story when they make a choice that shapes things (take a side, show mercy, refuse a bargain, lie to someone who trusts them), do something people will talk about, or something turns out for them. The engine already records promises, attacks, kills and quests.", obj(map[string]any{
			"character_id": str("character id"),
			"kind":         enum("kind", game.RecordKinds...),
			"text":         str("one sentence naming who and what, e.g. \"Let the Deacon's son go free on the harbor steps\""),
			"npc_id":       str("optional: npc id it involved")}, "character_id", "kind", "text")),
		tool(DirPersonalGoal, "Manage one character's personal goals (YOUR STORY in WORLD MEMORY). add: the story gives them something to want (at most 4 open). done: they clearly reached one (worth XP; at most one goal settled per character per day). failed: it can no longer happen.", obj(map[string]any{
			"character_id": str("character id"),
			"action":       enum("what to do", "add", game.GoalDone, game.GoalFailed),
			"goal_id":      str("goal id for done or failed, e.g. goal2"),
			"text":         str("add: the goal, under 15 words; done/failed: what happened, one short sentence")}, "character_id", "action")),
		tool(DirNPCGivesItem, "An NPC here hands one of their own items to a character as a gift.", obj(map[string]any{
			"npc_id": str("npc id"), "item_id": str("item id owned by that npc"), "to": str("character id")}, "npc_id", "item_id", "to")),
		tool(DirRecordLore, "Record a fact established in play, for the party's journal and the world's memory. Use kind promise when a character gives their word to someone (a promise, a deal, a debt), deed for something the party did that people will talk about, rumor for hearsay.", obj(map[string]any{
			"text":    str("one sentence, naming who and what"),
			"kind":    enum("kind of fact", game.FactKinds...),
			"npc_ids": arr(str("npc id"), "the NPCs this is about or was promised to")}, "text")),
		tool(DirResolvePromise, "Settle one of the party's open promises (WORLD MEMORY) once the party clearly kept or broke it.", obj(map[string]any{
			"promise_id": str("id from PARTY'S OPEN PROMISES, e.g. fact3"),
			"outcome":    enum("result", game.PromiseKept, game.PromiseBroken),
			"note":       str("what happened, one short sentence")}, "promise_id", "outcome")),
		tool(DirAdjustTension, "Raise or lower the scene's tension.", obj(map[string]any{"delta": num("-2..2", -2, 2)}, "delta")),
		tool(DirHazard, "A danger in the scene threatens one character with a condition (a trap's poison needle, a collapsing floor knocking them prone, a snare, a horror that frightens). Use the condition horror for something unnatural seen or understood: the engine rolls a Wisdom save against mental strain. The engine rolls their save and decides; never narrate the outcome before the result. At most once per narration, never stunned.", obj(map[string]any{
			"target":    str("character id"),
			"condition": enum("condition threatened", game.CondPoisoned, game.CondProne, game.CondRestrained, game.CondFrightened, game.CondBlinded, "horror"),
			"cause":     str("what causes it, a few words")}, "target", "condition", "cause")),
		tool(DirEndEncounter, "Remaining enemies flee or surrender, ending the fight (no rewards).", obj(map[string]any{"reason": str("why")}, "reason")),
	}
}

func filterTools(tools []llm.Tool, keep map[string]bool) []llm.Tool {
	var out []llm.Tool
	for _, t := range tools {
		if keep[t.Name] {
			out = append(out, t)
		}
	}
	return out
}
