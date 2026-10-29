# aidnd design notes

## Package layout

```
cmd/server        main: env config + wiring
cmd/play          terminal client for play-testing
internal/dice     injectable RNG (seeded or scripted), "2d6+3" notation, d20 checks -> 4 tiers
internal/game     pure rules engine, no I/O
  types.go        Campaign, Player, Character, Location, Zone, Edge, NPC, Enemy, Encounter,
                  Item/Owner, Quest/Objective, Lore, Beat, Scene, Budget, TurnState
  world.go        World aggregate (whole campaign state) + lenient id/name lookups
  catalog.go      fixed item catalog + six kits; reach inference; custom-item clamping
  rules.go        modifiers, proficiency, XP/levels, AC, attacks, subdue DC, enemy caps + HP budget
  validate.go     character sheet validation/creation, quest validation (reward caps, id resolution)
  graph.go        exits, BFS travel over visited/passable edges, stub fill-in with growth caps
  zones.go        zone layouts (authored or generic), distances, reach, cover, enemy tags
  quests.go       accept, automatic objective evaluation, freeform (judge-only), turn-in, lair gate
  trade.go        buy/sell (disposition pricing), pick up, drop, give, equip
  turns.go        budget, seat order (down skipped), setbacks, encounters, enemy AI turns
  actions.go      Action -> Step -> Resolution: the deterministic resolver for 1-4 steps
internal/store    SQLite (modernc.org/sqlite): schema.sql, whole-world load/save in one tx, events
internal/llm      OpenAI-compatible chat client + scripted fake
internal/gm       prompts, tool schemas, interpret -> resolve -> direct -> narrate -> judge pipeline
internal/api      net/http handlers, auth, player-safe views, SSE hub
internal/web      single-file /dev/ testing table
```

## Core ideas

- `game.World` is the unit of state. The store loads it, the pipeline mutates it only
  through `game` functions, and the store saves it plus new events in one transaction.
- Entities live in slices (not maps) so seeded runs iterate in a stable order. IDs come from
  a per-campaign counter (`loc3`, `npc7`, `item12`).
- Every lookup is lenient: by id first, then by name (case/punctuation/article-insensitive),
  then by substring. That is what the LLM's output is matched against.
- `game.Resolve(ctx, actor, actions)` is the only way a player turn changes state. It pays
  each step from the budget on a trial copy, executes it, and commits the budget only if the
  step succeeded; invalid ids come back as step errors without spending anything.
- The engine never calls the LLM. `Ctx.OnArrive` (fill a stub + lay out zones) and
  `Ctx.Ambush` (author enemies) are hooks the orchestrator supplies; when nil, deterministic
  fallbacks (hint-only fill, generic zones, generic enemies) are used.

## Rule decisions (where the brief left room)

Colin reviewed and confirmed these on 2026-10-29, including the play-test balance
changes below (enemies act once per round, softer early fights).

- The party moves as one: one `Campaign.LocationID`; characters only have a zone. Fleeing
  through an exit moves the whole party (down allies carried) and ends the encounter, no XP.
- A second move in a turn is a dash paid with the action, in and out of combat.
- Damage is the weapon's dice only (no ability modifier); a natural 20 doubles the dice.
- Proficiency +2/+2/+3/+3/+4 for levels 1–5. XP thresholds 0/150/400/800/1300.
  Level-up HP = 6 + CON mod (min 1).
- AC: armor base + DEX (capped per armor), +2 for a shield; unarmored 10 + DEX.
- "Badly hurt" for subdue = at or below half HP. Grapple/tie-up need the same zone;
  intimidate/calm/persuade reach 1 zone.
- Free: look, say, equip, drop, accept_quest, and (outside combat) reposition.
  Move: go through an exit, travel, reposition in combat. Action: everything else,
  including fleeing through an exit in combat. Talking and trading are free outside
  combat and cost the action in a fight. turn_in_quest is free (it pairs
  with handing over the item, which costs the action).
- Rest: a long rest (full HP, next dawn) only at danger-0 places; elsewhere a short rest
  (half max HP, down allies to 1, one time segment, random-encounter roll).
- Random encounters: d100 <= 8 x danger per travel hop or rest.
- Main-quest objectives tick strictly in order; side-quest objectives in any order.
  Talk/defeat objectives count only after the quest was accepted. A reach_location
  objective counts for any place already visited, so reaching it early is not wasted.
- Lair-gate edges ignore keys and checks; they open when all main objectives but the last
  are done. Defeating (or subduing) the antagonist completes the campaign.
- Settings: every campaign has an era (`fantasy`, `gaslight`, `pulp`, `modern`, `future`).
  The author picks it, but a year, decade or era word in the premise wins, and campaigns
  saved before settings existed infer it from the premise. Each era reskins the whole
  fantasy catalog with identical stats (a pulp Automatic Pistol rolls what a Light
  Crossbow does) and renames the six kits (pulp skirmisher = Gumshoe). `NewItem`
  translates any catalog key into the campaign's era, so authored stock and loot can't
  bring swords into 1931. Existing characters keep the gear they already have.

## Changes from the live play-test (deepseek-v4.1-flash)

- Quests stay completable whatever the author names: an obtain/deliver item that exists
  nowhere is created as a quest item, held by the NPC the objective names (handed over when
  the party talks to them) or lying at the place the quest points to (named place, else the
  previous reach_location, else an unvisited place; never the hub or lair). A deliver_item
  whose target is the recipient becomes the item fetched earlier. A defeat_enemy foe that is
  not an NPC gets `Objective.Where` and appears when the party arrives there.
- Enemies act once per round, spread over the party's turns (`Enemy.ActedRound`): after
  each character's turn, ceil(waiting / turns left in the round) of them act.
- Lower early-level pressure: enemy max damage 4 + 2(danger + level) and HP budget
  party x (4 + 3 x danger + 4 x (level - 1)).
- Story-beat ambushes wait until the party is outside the safe hub; authored enemies may not
  reuse the antagonist's name (the real antagonist waits in the lair).
- The interpreter continues the conversation ("queued so far: ...; call the next tool or say
  done") when a message reads like several steps, because some models return one tool call
  per reply.
- The HTTP client retries a reply cut off by max_tokens (reasoning models spend the cap on
  hidden thinking) without the cap. `LLM_REASONING_EFFORT` passes `reasoning_effort`.
- Merchants remember trades; quest items are never for sale. A carried shield shows as
  equipped (it always counted toward AC).

## Keeping the scene where the party is (Drowned Bell play-test)

- Beats that name an NPC wait until the actor shares that NPC's zone (or the beat brings the NPC). A turn-triggered beat had the coroner unveil bodies while the party stood on another part of the town.
- Snapshots carry a SCENE line (who is in the actor's zone) and mark other NPCs as out of the scene; the narrator may only have people in the scene speak or act, and may not describe people or things that STATE does not list.
- Outside combat, talk_to walks the actor to the NPC's zone, so the state matches the conversation.
- When the interpreter returns only a reposition for words that are not about moving, it is asked for the missing step; if it still has none and an NPC is selected on the map, a talk_to with the player's words is added.
- Character names are unique per campaign: drafts avoid party names, and saving another player's name is a 409. The feed shows only each player's latest lobby sheet.
- The world author lists a `cast`: every person, group, creature or body the story says can be found, placed at the hub or a place. Objective wording must describe the one action that completes it, and the first main objective is hearing the quest giver.
- A beat in which someone new shows up names them as `figure`. The figure is created offstage and brought into the actor's zone when the beat plays (and attacks if hostile).
- Examining an NPC counts toward talk_to_npc objectives. Looking at something that is not here says so instead of describing the whole place.
- Narration must not use game words (zone, NPC, HP, ...). If it does anyway, one rewrite pass replaces them.

## World memory

The chronicle is the world's long-term memory, and the engine writes to it, not just the narrator.

- Every fact has an id, turn, day, kind (fact, deed, death, promise, rumor), place and the NPCs it concerns. Facts from before this change get ids on load.
- The engine records: a named NPC killed or subdued (with where), and accepting a quest as a promise to its giver. That promise is kept when the quest is turned in (or completes with no giver); only the engine settles it.
- When a player gives an NPC their word, the interpreter fills talk_to's `promise` field and the engine records an open promise to that NPC (live model: "I swear I'll bring you back the keeper's lantern" became one).
- The narrator records facts with `record_lore` (a promise must name who it was made to; at most 8 open) and settles its own promises with `resolve_promise`. A kept promise warms everyone it was made to by one step, a broken one cools them, and they remember it for good.
- NPCs keep up to 6 permanent memories (`keeps`: attacked, subdued, gifts, quests done, promises settled, `npc_note` with important) next to the 8 rolling notes. `adjust_disposition` takes a reason, which the NPC remembers.
- Locations store the day the party was last there; coming back on a later day adds "returns after N days away" to the resolution.
- Each narration prompt carries a WORLD MEMORY block: up to 4 facts set in this place, 2 about each person in the scene, open promises with ids, 3 other recent facts, and the named dead. Quick answers get it too. The story summary is rebuilt from the facts recorded since the last summary as well as the log.
