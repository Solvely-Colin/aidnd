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
		tool(game.ActAttack, "ACTION. Attack an enemy with the equipped weapon (or a named carried weapon). Must be in reach.", obj(map[string]any{
			"target": str("enemy id from ENEMIES (or an NPC id only if the player clearly attacks them)"),
			"weapon": str("optional item id of a carried weapon to use")}, "target")),
		tool(game.ActSubdue, "ACTION. Non-lethal takedown: grapple, tie up, intimidate, calm or persuade an enemy to stop fighting.", obj(map[string]any{
			"target": str("enemy id"),
			"method": enum("how", "grapple", "tie_up", "intimidate", "calm", "persuade")}, "target", "method")),
		tool(game.ActCheck, "ACTION. An ability check for anything risky or uncertain: searching (finds hidden exits), picking or forcing a lock, climbing, sneaking, persuading, recalling lore under pressure.", obj(map[string]any{
			"ability": abilityEnum,
			"dc":      intEnum("difficulty: 5 trivial, 10 easy, 15 medium, 20 hard, 25 very hard", 5, 10, 15, 20, 25),
			"purpose": enum("search reveals hidden exits; unlock/force targets a locked exit", "search", "unlock", "force", "other"),
			"target":  str("optional exit id for unlock/force"),
			"text":    str("what the check is for, in a few words")}, "ability", "dc", "purpose", "text")),
		tool(game.ActUseItem, "ACTION. Use an item: drink a potion, apply a healer's kit (can revive a downed ally), use a key or tools on a locked exit.", obj(map[string]any{
			"item":   str("item id from INVENTORY"),
			"target": str("optional party member id or exit id")}, "item")),
		tool(game.ActTalkTo, "ACTION. Have a conversation with an NPC (ask, persuade, haggle, ask for work).", obj(map[string]any{
			"npc":  str("NPC id from NPCS"),
			"text": str("what the character says or asks")}, "npc")),
		tool(game.ActBuy, "ACTION. Buy an item from a merchant here.", obj(map[string]any{"npc": str("merchant NPC id"), "item": str("item id from the merchant's STOCK")}, "npc", "item")),
		tool(game.ActSell, "ACTION. Sell a carried item to a merchant here.", obj(map[string]any{"npc": str("merchant NPC id"), "item": str("item id from INVENTORY")}, "npc", "item")),
		tool(game.ActAcceptQuest, "FREE. Accept a quest an NPC here has offered.", obj(map[string]any{"quest": str("quest id from QUESTS ON OFFER")}, "quest")),
		tool(game.ActTurnInQuest, "ACTION. Turn in a completed quest to its giver (must be at the giver's location).", obj(map[string]any{"quest": str("quest id")}, "quest")),
		tool(game.ActPickUp, "ACTION. Pick up an item from the ground.", obj(map[string]any{"item": str("item id from GROUND")}, "item")),
		tool(game.ActGiveItem, "ACTION. Hand a carried item to a party member or an NPC here.", obj(map[string]any{
			"item": str("item id from INVENTORY"), "to": str("party member id or NPC id")}, "item", "to")),
		tool(game.ActRest, "ACTION. Rest the party (not in combat). Full rest only in safe places.", obj(map[string]any{})),
	}
}

// toAction converts an interpreter tool call to an engine action.
func toAction(tc llm.ToolCall) (game.Action, error) {
	var a struct {
		Target, Weapon, Method, Ability, Purpose, Text, Item, Exit, Zone, Destination, NPC, Quest, To string
		DC                                                                                            int
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
		act.Target, act.Item = a.Target, a.Weapon
	case game.ActSubdue:
		act.Target, act.Method = a.Target, a.Method
	case game.ActCheck:
		act.Ability, act.DC, act.Purpose, act.Target = a.Ability, a.DC, a.Purpose, a.Target
	case game.ActUseItem:
		act.Item, act.Target = a.Item, a.Target
	case game.ActTalkTo:
		act.Target = firstOf(a.NPC, a.Target)
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
	DirAdjustTension  = "adjust_tension"
	DirEndEncounter   = "end_encounter"
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
	"npc_id":       str("optional: id of an NPC present who turns hostile"),
}, "name", "hp", "ac", "attack_bonus", "damage")

var objectiveSchema = obj(map[string]any{
	"kind":       enum("objective type", game.ObjectiveKinds...),
	"text":       str("short player-facing description"),
	"target":     str("location id/name, enemy name, item name, or npc id/name depending on kind"),
	"deliver_to": str("npc id/name for deliver_item"),
}, "kind", "text")

var questSchema = map[string]any{
	"title":        str("quest title"),
	"summary":      str("one or two sentences"),
	"giver_id":     str("id of the NPC offering it (must be present)"),
	"objectives":   arr(objectiveSchema, "1-4 objectives"),
	"reward_gold":  num("gold paid at turn-in", 0, 500),
	"reward_xp":    num("xp at turn-in", 0, 300),
	"reward_items": arr(obj(map[string]any{"key": str("catalog key, or empty"), "name": str("name for a custom item"), "kind": str("item kind"), "value": num("value in gold", 0, 500)}), "0-2 items held by the giver until turn-in"),
}

func directiveTools() []llm.Tool {
	return []llm.Tool{
		tool(DirSpawnEncounter, "Start a fight here (not if one is under way). Enemies are capped by the engine.", obj(map[string]any{
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
		tool(DirNPCNote, "Record a short memory for an NPC about the party (what they did/said).", obj(map[string]any{
			"npc_id": str("npc id"), "note": str("under 20 words")}, "npc_id", "note")),
		tool(DirAdjustDisp, "Shift an NPC's attitude by one step after how the party treated them.", obj(map[string]any{
			"npc_id": str("npc id"), "delta": num("-1 or +1", -1, 1)}, "npc_id", "delta")),
		tool(DirNPCGivesItem, "An NPC here hands one of their own items to a character as a gift.", obj(map[string]any{
			"npc_id": str("npc id"), "item_id": str("item id owned by that npc"), "to": str("character id")}, "npc_id", "item_id", "to")),
		tool(DirRecordLore, "Record a fact established in play for the party's journal.", obj(map[string]any{"text": str("one sentence")}, "text")),
		tool(DirAdjustTension, "Raise or lower the scene's tension.", obj(map[string]any{"delta": num("-2..2", -2, 2)}, "delta")),
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
