# aidnd HTTP API

This is the integration guide for a host app (web, Discord bot, mobile) that
puts players in front of aidnd. All bodies are JSON. Errors are always
`{"error": "..."}`, plus `"problems": [...]` on a 422.

## Running the server

| Variable | Default | Meaning |
|---|---|---|
| `LLM_MODEL` | (required) | Model name for the OpenAI-compatible endpoint. `offline` makes no model calls and runs every step on the engine's fallbacks. |
| `LLM_BASE_URL` | `https://ollama.com/v1` | Any OpenAI-compatible `/chat/completions` base URL. |
| `LLM_API_KEY` | | Bearer key for the model endpoint. |
| `LLM_REASONING_EFFORT` | | Sent as `reasoning_effort` (`none`, `low`, `medium`, `high`) to reasoning models. `low` or `none` makes turns much faster. |
| `DB_PATH` | `aidnd.db` | SQLite file. Migrations run at startup. |
| `ADDR` | `:8080` | Listen address. |
| `API_KEY` | | Set it to turn on **service mode** (see Auth). Empty means open dev mode. |
| `CORS_ORIGINS` | | Comma-separated allowed origins, or `*`. |
| `DEV_UI` | on in open mode | Serves the testing table at `/dev/`. |
| `MAX_PLAYERS` | 4 | Seats per campaign. |
| `MAX_SETBACKS` | 3 | Party wipes survived; the next one fails the campaign. |
| `SCENE_TURN_CAP` | 10 | Quiet turns in one place before the director forces progress. |
| `OBJECTIVE_CHECK_EVERY` | 1 | How often (in turns) the judge checks freeform objectives. |
| `LORE_BUDGET` | 1800 | Characters of lore given to the narrator per turn. |
| `SKIP_AFTER_SECONDS` | 120 | Another player may skip an idle turn after this long. |
| `PLAYER_DICE` | on when `LLM_API_KEY` is set | Players roll their own d20s (see Rolling your own dice). Off means the engine rolls everything at once. |
| `DEBUG_DIRECTOR` | false | Logs director beats and pacing decisions. |

```sh
LLM_MODEL=offline go run ./cmd/server        # click around with no model
LLM_MODEL=deepseek-v4.1-flash LLM_API_KEY=... go run ./cmd/server
```

## Auth

**Service mode** (`API_KEY` set). Your backend is the only client. Every
request carries `Authorization: Bearer <API_KEY>` and, when acting for a
player, `X-Player-ID: <your stable user id>`. aidnd trusts that id, so never
let a browser hold the API key. For browsers, mint a short-lived stream token
(below) and hand only that to the client.

**Open mode** (no `API_KEY`). Joining without `X-Player-ID` creates a player
and returns a per-campaign `token`; send it as `Authorization: Bearer
<token>`. For local testing, `X-Player-ID` alone is also accepted, which is
what the dev table does.

## Lifecycle

1. `POST /campaigns {"premise": "..."}` → 201 campaign view. The model
   authors the world (can take 30–90 s). A bad or failed authoring falls back
   to a built-in campaign, so this does not fail on model trouble.
2. Each player: `POST /campaigns/{id}/players {"name": "..."}`. Idempotent per
   player id. 409 when full or already started.
3. Each player makes a character (replaceable until the game starts):
   - `POST /campaigns/{id}/characters/draft {"concept": "..."}` returns a
     valid draft to show in an editor, then
   - `POST /campaigns/{id}/characters {"character": {...}}` saves it, or
   - `POST /campaigns/{id}/characters {"concept": "..."}` drafts and saves
     in one step.
4. `POST /campaigns/{id}/start` → 409 until every joined player has a
   character. Posts the opening narration and the first `turn_started`.
5. The active player sends `POST /campaigns/{id}/actions` until their turn
   ends; anyone watches `GET /campaigns/{id}/events`.

Character spec (validated; 422 lists every problem):

```json
{"name": "Wren", "class": "Hedge Witch", "kit": "arcanist", "concept": "...",
 "backstory": "...", "pronouns": "she/her",
 "scores": {"STR": 8, "DEX": 14, "CON": 13, "INT": 15, "WIS": 12, "CHA": 10},
 "proficient": ["INT", "WIS"]}
```

A 5e build sets `class_key` and then ignores `kit` and `proficient`:

```json
{"name": "Maren Voss", "class": "", "pronouns": "she/her", "class_key": "wizard",
 "species": "human", "background": "sage", "boost": {"INT": 2, "CON": 1},
 "scores": {"STR": 8, "DEX": 14, "CON": 13, "INT": 15, "WIS": 12, "CHA": 10},
 "skills": ["investigation", "insight"],
 "picks": {"skillful": ["athletics"], "versatile": ["tough"], "mi_cantrips": ["fire_bolt", "mage_hand"], "mi_spell": ["magic_missile"]},
 "spells": ["fire_bolt", "mage_hand", "ray_of_frost", "magic_missile", "shield", "sleep"]}
```

`scores` is the standard array before the background `boost` (+2/+1 or
+1/+1/+1 among the background's three abilities). `skills` are the class
picks only (background and species skills are added). `picks` answers every
choice in `POST characters/preview`'s `choices`; `spells` are the class
cantrips and prepared spells (species and feat spells are added). `class` is
an optional title; empty means the class's name in the campaign's era.
Drafts return a complete build.

Legacy sheets: scores must be a permutation of the standard array 15/14/13/12/10/8, exactly
two proficiencies, and a kit from `GET /meta`. They play as the class their
kit maps to. `pronouns` is free text the
narrator uses (normalised, e.g. "She / Her" → "she/her"); it defaults to
they/them, and drafts fill it from the concept.

## Endpoints

| Method & path | Who | Notes |
|---|---|---|
| `GET /healthz` | anyone | `{"ok": true}` |
| `GET /meta?setting=` | anyone | Abilities, standard array, kits with items (fantasy), `settings` (each with its own kits), item catalog, XP table, DCs, `service_mode`, and `ruleset`: every class, species, background, feat, invocation and spell named for `setting` (`rules_name` is the SRD name). |
| `GET /campaigns` | any caller | All campaigns: `{campaigns: [{id, title, pitch, status, players, max_players, updated_at}]}` |
| `GET /me/campaigns` | player | Campaigns this player has joined. |
| `POST /campaigns` | any caller | `{premise, lethality?}` → 201 campaign view. `lethality` is `standard` (three failed death saves kill) or `story` (they leave you stable). `{saga: "long-dark"}` instead starts act 1 of a hand-written saga (listed in `GET /meta` as `sagas`); no model call, and the view carries `saga`, `act` and `dread`. In dread campaigns each character view has `strain: {value, max, shaken, scars: [{key, name, text}]}`. |
| `GET /campaigns/{id}` | any caller | Campaign view, personalised by `you` when a player id is known. |
| `POST /campaigns/{id}/players` | player | `{name}` → `{player, created, token?}` (201 when new). |
| `POST /campaigns/{id}/characters/draft` | joined player | `{concept}` → `{character: spec}` |
| `POST /campaigns/{id}/characters` | joined player | `{character}` or `{concept}` → character view. |
| `POST /campaigns/{id}/characters/preview` | joined player | `{character, fill?}` → `{character, problems, choices, skills, max_hp, scores, cantrips, prepared, spell_options, default_spells}`. `fill` completes the build with defaults (a new class, species or background). |
| `POST /campaigns/{id}/me/choose` | joined player | `{key, picks}` answers a level-up choice from `build.pending_choices` → character view. Earlier picks are kept; a growing choice (invocations) adds to them. |
| `POST /campaigns/{id}/me/spells` | joined player | `{spells}` replaces prepared spells (409 in a fight) → character view. |
| `GET /campaigns/{id}/characters/{cid}` | any caller | Character view. |
| `POST /campaigns/{id}/start` | joined player / service | Starts the game. |
| `POST /campaigns/{id}/actions` | active player | `{text, target_id?, zone_id?, check?, act?}` → turn result, or a pending turn when the player rolls their own dice. |
| `POST /campaigns/{id}/roll` | acting player | Rolls the d20s for the caller's pending turn or death save and resolves it → turn result. |
| `POST /campaigns/{id}/roll/cancel` | acting player | Takes back a pending turn before the roll (not a death save). |
| `POST /campaigns/{id}/me/dice` | joined player | `{auto_roll}`: true lets the engine roll for this player. |
| `POST /campaigns/{id}/skip` | player / service | Ends the active turn. See Skipping. |
| `GET /campaigns/{id}/map` | any caller | Known places and passable/locked paths only. |
| `GET /campaigns/{id}/locations/{lid}` | any caller | A visited or known location; 404 otherwise. |
| `GET /campaigns/{id}/npcs/{nid}` | any caller | Public NPC view and merchant stock. |
| `GET /campaigns/{id}/quests` | any caller | `{quests: [...]}` |
| `GET /campaigns/{id}/journal` | any caller | `{summary, chronicle: [{id, turn, day, kind, text, location_id, npc_ids, source, quest_id, status, outcome}], quests, setbacks, people, places}`; `kind` is fact, deed, death, promise or rumor; promises have `status` open, kept or broken. See Journal. |
| `GET /campaigns/{id}/notes` | joined player | `{notes: [{id, text, turn, day, time, created_at, updated_at}]}`, the caller's own notes, oldest first. |
| `POST /campaigns/{id}/notes` | joined player | `{text}` → 201 note, stamped with the current turn, day and time of day. |
| `POST /campaigns/{id}/notes/{nid}` | note's author | `{text}` replaces the text. 404 for anyone else's note. |
| `DELETE /campaigns/{id}/notes/{nid}` | note's author | Deletes the note. |
| `POST /campaigns/{id}/stream-token` | any caller | `{token, expires_at}` for the event stream. |
| `GET /campaigns/{id}/events` | see SSE | Server-sent events. |
| `GET /campaigns/{id}/log?after=&limit=` | any caller | Same events as JSON pages: `{events, last_id}`. |
| `GET /campaigns/{id}/dm/history` | joined player | DM console: `{snapshots: [{seq, event_id, turn, label, created_at}]}`, newest first. The last 30 turns are kept. `/meta` carries `magic`: the item templates, wondrous items and console actions. |
| `POST /campaigns/{id}/dm/rewind` | joined player | `{seq}` puts the whole campaign back to just before that turn → campaign view. Events since are hidden from the log and the stream replay; a `rewound` event is sent so clients reload. 400 when the turn is no longer kept. |
| `POST /campaigns/{id}/dm` | joined player | `{kind, amount?, target?, item?, template?, value?}` → `{text, campaign}`. `kind` is `end_fight`, `heal_party`, `award_xp` (`amount` 1-3000), `give_item` (`target` character, `item` a catalog or wondrous key, optional `template` to make it magic gear) or `lethality` (`value` standard or story). Each sends a `dm_action` event. |

### Journal

The journal is built from the world record, never from model output, so it
only lists what the engine knows the party has come across.

- `people`: NPCs at places the party has visited (the antagonist only while
  the party is in the same place). Each has the NPC brief fields plus
  `location_name`, `met` (the party has spoken with them), `here`, and
  `quests` (titles of visible quests they gave). Nearby people come first,
  then the most recently spoken to.
- `places`: known locations with `name, kind, hint, danger, visited, here,
  is_hub, people`; `description` only once visited. Here first, then
  visited, then places only seen from a path.

Notes are private to the player who wrote them (up to 200 per campaign,
4000 characters each). They are stored apart from the world, so neither
the narrator nor other players see them.

"Any caller" means authenticated: the API key in service mode, anything in
open mode.

### Status codes

| Code | Meaning |
|---|---|
| 400 | Malformed JSON, missing player id, or a bad request body. |
| 401 | Missing/invalid API key, player token or stream token. |
| 403 | The player has not joined this campaign. |
| 404 | Unknown campaign, or something players have not discovered. |
| 409 | Not your turn, campaign full, already started, characters missing, campaign over. |
| 422 | Character validation failed; `problems` lists why. |
| 500 | Internal error (logged server-side). |

## Campaign view

`GET /campaigns/{id}` is the one read a client needs to draw the table. It
never contains secrets: no hidden exits, no unrevealed lore, no NPC memory,
and the main quest only shows objectives up to the first one not yet done.

```jsonc
{
  "id": "c_...", "title": "...", "pitch": "...", "premise": "...",
  "setting": "fantasy | gaslight | pulp | modern | future",  // pick kits from /meta settings
  "status": "lobby | active | completed | failed",
  "turn": 12, "setbacks": 0, "max_setbacks": 3, "max_players": 4,
  "scene": {"day": 1, "time": "evening", "tension": 2, "goal": "..."},
  "location": {
    "id", "name", "kind", "description", "danger", "visited", "explored", "is_hub",
    "zones": [{"id", "name", "desc", "cover", "adjacent": ["z2"], "exits": ["e1"]}],
    "exits": [{"id", "label", "to_id", "to_name", "state": "open | locked", "zone_id", "explored", "sealed"}],
    "npcs": [{"id", "name", "role", "zone_id", "merchant", "disposition", "alive"}],
    "items": [{"id", "name", "kind", ...}]           // on the ground
  },
  "party": [{
    "id", "player_id", "name", "class", "kit", "level", "xp", "level_xp", "next_level_xp",
    "hp", "max_hp", "down", "ac", "gold", "zone_id", "zone_name", "proficiency_bonus",
    "scores": {"STR": 15}, "modifiers": {"STR": 2}, "proficient": ["STR", "CON"],  // modifiers include proficiency
    "weapon": {"name", "damage": "1d8", "reach": "melee | ranged", "to_hit": 4},
    "armor": {"name", "ac": 16},                       // omitted when unarmored
    "attack": {"weapon", "bonus": 4, "ability": "STR", "damage": "1d8", "reach"},
    "ac_parts": [{"label": "Chain Mail", "value": 16}, {"label": "Shield", "value": 2}],  // sums to ac
    "level_up": {"level": 2, "hp": 8, "proficiency_bonus": 2},  // omitted at max level
    "conditions": [{"key", "label", "note"}],   // engine conditions (poisoned | prone | restrained |
                                                // frightened | blinded | stunned; note says the effect,
                                                // source and turns left) then derived states
                                                // (down | bloodied | cover | acting)
    "inventory": [{"id", "name", "kind", "equipped", "effect", "to_hit", ...}]
  }],                                                  // every member's full sheet, so players can read each other's
  "players": [{"id", "name", "character_id"}],
  "encounter": {"round": 2, "enemies": [{"id", "display", "hp", "max_hp", "zone_id", "ranged",
                "inflicts": "poisoned", "conditions": [{"key", "label", "note"}]}]},
  "quests": [{"id", "title", "summary", "is_main", "status", "giver_name",
              "objectives": [{"id", "kind", "text", "done"}], "reward_gold", "reward_xp"}],
  "active": {"character_id", "player_id", "name"},
  "turn_started": 1761600000,
  "budget": {"action": 1, "move": 1, "free": -1},  // -1 = unlimited
  "you": {"player_id", "character_id", "joined": true, "your_turn": false}
}
```

## Turns

In a fight only the active player may act. Outside fights there is no turn
order: any conscious player may act whenever they like, each message is its
own turn with a fresh budget, and skipping is refused (`free_roam` in the
campaign view). One message may contain several actions
("I step behind the cart and shoot the archer"); the engine pays for each
from the turn budget:

| Slot | Actions |
|---|---|
| free | look, say, equip, drop, pick up, accept or turn in a quest; outside combat also reposition, talk and trade |
| move | move to a zone, travel, reposition in combat (a second move spends the action as a dash) |
| none | end turn ("I wait", "done") |
| action | attack, subdue, ability checks (search, unlock, force, stabilize), use item, rest, dodge, disengage, help, hide, ready, shove, grapple, flee through an exit in combat; in combat also talk and trade |

In combat a turn has 1 action, 1 move and 3 free actions. Outside combat
free actions are unlimited. Spending the action ends the turn, but the rest
of the same message still runs where the move or free actions cover it ("I
stab it and step back"); `end_turn` stops the message outright. Several
buys or sells with one merchant in one message cost one action. Steps the
budget cannot pay for are returned in `skipped` with a reason.

`target_id` and `zone_id` are optional hints from a click (an enemy token,
an NPC, a zone). They resolve ambiguity; the text still decides what
happens.

`check` is a check the player called for: `{ability?, purpose, exit?}`.
With purpose `search`, `unlock` or `force` (the last two need a locked
`exit` that is not `sealed`) the engine rolls exactly that check and the
interpreter is skipped; a bad exit is a 400. With purpose `other` the
ability is fixed and the interpreter still sets the DC from the text. The
engine always rolls the dice. `check.skill` names one of the 18 skills
(`perception`, `sleight_of_hand`, ...); the bonus is the skill's ability
plus proficiency (doubled with expertise).

`act` is a tactic picked from a button: `{kind, target?, method?}` with
kind `dodge`, `disengage`, `help` (target an ally), `hide`, `ready`,
`shove` (method `prone` or `push`) or `grapple`. It skips the interpreter
like a called check.

### Tactics and reactions

- **Dodge**: attacks against you have disadvantage until your next turn.
- **Disengage**: leaving a zone draws no opportunity attacks this turn.
- **Help**: an ally's next attack or check has advantage.
- **Hide**: Stealth against the best enemy passive Perception
  (10 + half its attack bonus). Hidden, you can't be targeted and your next
  attack has advantage; attacking reveals you.
- **Ready**: you strike the first enemy that enters your zone.
- **Shove / grapple**: the enemy saves (STR, its save bonus) against
  8 + your STR + proficiency. Shove knocks prone or pushes to a neighbouring
  zone; a grapple restrains until you move or go down.
- Leaving a zone with an enemy in it draws one opportunity attack per enemy
  per round, and characters get the same against an enemy moving away.
- On arrival the best passive Perception may spot hidden exits
  (DC 15 + danger). Ambushers roll Stealth against it; if they win, they
  act first.

### Rolling your own dice

With `PLAYER_DICE` on, a turn that needs the acting character's d20 is not
resolved at once. The engine dry-runs it on a copy of the world and answers
`{pending: {kind: "action", character_id, text, preview: [{kind, label,
slot, error, rolls: [{label, kind, mod, parts, target, against, edge}]}]}}`
and the event `action_pending`. `POST /roll` then rolls and resolves the
turn for real (the dice are decided at that moment, not in the preview);
`POST /roll/cancel` takes it back (`action_cancelled`). The campaign view
lists open ones in `pending`. Players with `auto_roll` skip this.

### Dying

At 0 HP in a fight a character is dying: on each of their turns they make
a death save (d20: 10+ succeeds, a 1 counts twice, a 20 brings them back
with 1 HP). Three successes make them stable; three failures kill them in
a `standard` campaign and leave them stable in a `story` one. A death save
is a pending roll (`kind: "death_save"`) for players who roll their own
dice, and is rolled automatically otherwise (event `death_save`). Healing
or a DC 10 Medicine `stabilize` check helps. A dead character's player can
build a new one, who joins at the party's location with the same XP.

### Turn result

```jsonc
{
  "resolution": {
    "steps": [{
      "action": {"kind": "attack", "target": "en_3"}, "slot": "action", "ok": true,
      "error": "",                                 // why a step failed
      "rolls": [{"label", "kind": "check|attack|damage|heal|subdue|save|encounter",
                 "natural": 17, "dice": [5], "expr": "1d8+2", "mod": 5, "total": 22,
                 "target": 13, "tier": "success",
                 "parts": [{"label": "DEX", "value": 3}, {"label": "Proficiency", "value": 2}],
                 "against": "AC", "cover": 2, "crit": false,      // parts sum to mod; cover is inside target
                 "edge": "advantage", "naturals": [6, 17], "edge_why": "target prone"}],  // edge only when rolled twice
      "changes": ["Goblin takes 7 damage (0/7)"],
      "quests": [...], "arrived": "loc_4", "combat_started": false, "combat_ended": true
    }]
  },
  "narration": "...", "quick": false,
  "world_changes": ["A hidden stair is revealed"],
  "world_rolls": [{"kind": "save", ...}],          // saves against a narrated hazard
  "skipped": [{"action": {...}, "error": "no action left this turn"}],
  "budget": {"action": 0, "move": 1, "free": 2},
  "turn_ended": true,
  "enemy_turns": [{"enemy", "kind": "move|attack|hold|stunned", "target", "attack": {...}, "text",
                   "save": {...}, "inflicted": "prone",       // target's save against the hit's condition
                   "before": ["Wolf gets back up"], "after": [], "rolls": [...]}],  // the enemy's own condition ticks
  "conditions": ["Aria shakes off being poisoned (CON save 15 vs DC 12)"],  // turn-boundary ticks
  "condition_rolls": [{"kind": "save", ...}],
  "enemy_narration": "...",
  "setback": ["..."],                              // party wipe consequences
  "next_actor": {"character_id", "player_id", "name"},
  "campaign_ended": false, "status": "active"
}
```

`quick` marks a short answer to a question or a look, written without the
full narrator. Tier is one of `critical_success`, `success`, `failure`,
`critical_failure`.

A turn takes one to four model calls, so allow 5–30 s. The HTTP response and
the events carry the same information; a client can ignore the response and
render purely from events.

### Skipping

- The active player may skip their own turn at any time (the dev table's
  "End turn").
- Another joined player may skip it after `SKIP_AFTER_SECONDS` of idleness;
  before that it is a 409 that says how long is left.
- In service mode, `POST /skip {"force": true}` (or no `X-Player-ID`) skips
  unconditionally, for a host-side turn timer.

Skipping in combat still runs the enemies' turns.

## Events (SSE)

`GET /campaigns/{id}/events` streams every committed event, in order:

```
id: 42
event: narration
data: {"id":42,"campaign_id":"c_...","type":"narration","data":{...},"created_at":1761600000}
```

- **Resume:** browsers send `Last-Event-ID` automatically on reconnect.
  Otherwise pass `?after=<id>`. With neither, the stream replays from the
  start of the campaign, then goes live. Nothing falls between replay and
  live.
- **Heartbeat:** a `: ping` comment every 15 s. `retry: 2000` is sent first.
- **Slow consumers** are disconnected and should reconnect with
  `Last-Event-ID`.
- **Auth:** `EventSource` cannot send headers. In service mode, call
  `POST /stream-token` from your backend (with `X-Player-ID` if you have
  one) and open `/events?stream_token=...` in the browser. Tokens are bound
  to one campaign and expire after 10 minutes; reconnects after that need a
  fresh token. In open mode the stream needs no auth. A backend may instead
  connect with the Bearer API key directly.

| Type | Data |
|---|---|
| `campaign_created` | `{id, title, pitch}` |
| `player_joined` | `{player_id, name, seat}` |
| `character_created` | `{player_id, character_id, name, class, kit, concept, replaces}` (`replaces` is the id of the sheet this one replaced in the lobby, or empty) |
| `location_changed` | `{location_id, name}` |
| `turn_started` | `{character_id, player_id, name, budget}` |
| `player_action` | `{character_id, character, text, hints}` |
| `action_resolved` | `{character_id, steps, skipped, changes, rolls, world_changes}` |
| `narration` | `{text, quick?, opening?, skip?, setback?, character_id?}` |
| `turn_budget` | `{character_id, budget}`, sent when the turn continues |
| `enemy_turns` | `{turns, narration}` |
| `conditions` | `{changes, rolls, lost_turn}`: conditions ticking at a turn boundary (saves, wearing off, standing up, a stunned turn passing) |
| `quest_updated` | `{quest_id, title, objective_id?, text, status}` |
| `campaign_ended` | `{status: "completed" \| "failed"}` |
| `rewound` | `{by, seq, to_event, label, text}`: the story was put back; drop events after `to_event` and reload |
| `dm_action` | `{by, kind, changes, text}` |

A typical turn is `player_action`, `action_resolved`, `narration`, then
either `turn_budget` (same player continues) or `enemy_turns` (in combat)
followed by `turn_started` for the next player.

## Minimal host loop

```js
const es = new EventSource(`/campaigns/${id}/events?stream_token=${tok}`);
es.addEventListener("narration", e => show(JSON.parse(e.data).data.text));
es.addEventListener("turn_started", e => {
  const d = JSON.parse(e.data).data;
  if (d.player_id === me) enableInput(d.budget);
});
async function act(text, hints = {}) {
  await fetch(`/campaigns/${id}/actions`, {method: "POST",
    headers: {"Content-Type": "application/json", "X-Player-ID": me /* via your backend */},
    body: JSON.stringify({text, ...hints})});
}
```

The dev table (`internal/web/index.html`) is a complete working example of
this loop with four seats.

## The 5e ruleset

Classes, species, backgrounds, feats and spells follow the SRD 5.2.1
(CC-BY-4.0), levels 1 to 5, one subclass per class. Each era renames them
with the same rules: in `pulp` a wizard is an Occultist and fire bolt is
Hellfire Sigil. A character view's `build` has `class_name`, `subclass`
(from level 3), `species_name`, `background_name`, `saves`, `features`,
`abilities` (usable features: `slot`, `left`/`max`, `ready`, `why`),
`resources`, `feats`, `pending_choices`, `slots`, `spell_dc`,
`spell_attack`, `spells`, `prepare` (counts and options), `temp_hp`,
`concentration`, `made_before_classes`, and `spec` (the build, to edit
before the story starts).

The dock sends spells and abilities as `act` hints, resolved without the
interpreter: `{"kind": "cast_spell", "spell": "fire_bolt", "target": "<enemy
or party id>", "zone": "<zone id>", "level": 0}` and `{"kind":
"use_feature", "feature": "rage", "target": "..."}`. An attack hint can
carry `"option": "smite"` or `"stun"`.

Turns have a bonus action: `budget.bonus` is 1 at the start of a turn.
Spending the action ends the turn, so bonus actions go in the same message.
