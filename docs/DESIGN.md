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
- Proficiency +2/+2/+3/+3/+4 for levels 1–5. XP thresholds 0/300/800/1600/2800
  (slowed 2026-10-29 from 0/150/400/800/1300). Awards: main objective 75, side
  objective 20, side quest 40 by default (authored rewards capped at 40 + 15 x danger,
  doubled for the main quest), won fight 10 x danger.
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

## Status conditions (2026-10-29)

Engine state on characters and enemies (`Conditions`), never set by the model directly.

| Condition | Effect | Ends |
|---|---|---|
| poisoned | disadvantage on attacks and checks | 3 turns, CON save at end of each turn; healer's kit; rest. Outlasts the fight |
| prone | own attacks at disadvantage; melee against has advantage, ranged against disadvantage | standing up at the start of your turn, which costs the move |
| restrained | can't move or flee; own attacks at disadvantage; attacks against have advantage | 3 turns, STR save each turn, or an `escape` check (action) |
| frightened | disadvantage on attacks and checks; can't move closer to the source | 2 turns, WIS save each turn |
| blinded | own attacks at disadvantage; attacks against have advantage | 1 turn |
| stunned | loses the next turn; attacks against have advantage | 1 turn |

- Advantage/disadvantage roll two d20s; any source of each cancels out (5e). Rolls carry
  `edge`, `naturals` and `edge_why`.
- Sources: an enemy's `inflicts` (authored, enum-validated; stunned only at danger 3+,
  else downgraded to prone): a hit forces a save vs DC 8 + attack bonus (10..15). The
  narrator's `hazard` directive (outside fights, once per narration, never stunned): the
  engine rolls the target's save vs DC 10 + danger and the narration is written after.
  Fallback wolves knock prone and cave spiders poison.
- Durations count the bearer's own turns and tick at the end of them; a stunned
  character's turn passes automatically (enemies still act). The end of a fight clears
  everything but poison; rests and setbacks clear all; going down clears all.

## Wave 1 table rules (2026-10-29)

- **Skills.** 18 5e skills. Characters store `skills` and `expertise` in the
  `extra` JSON column; sheets from before skills use their kit's list, and a
  sheet proficient in a whole ability still counts it for that ability's
  skills. Passive Perception is 10 + Perception.
- **Death saves** replace "down until healed" in fights. Outside a fight,
  0 HP is stable. Lethality is per campaign (`standard` or `story`).
- **Tactics** are stances on the condition list (dodging, disengaged,
  hidden, helped) that end at the start of the bearer's next turn.
  Reactions (opportunity and readied attacks) resolve inside the engine, one
  per creature per round.
- **Player-rolled dice.** The interpreter's plan is dry-run on a JSON copy
  of the world with a throwaway roller to build the preview; nothing from
  the dry run is kept. The real roll happens on `POST /roll`, so the preview
  can't be farmed for good dice.
- **Free play.** Outside fights there is no turn order; whoever writes acts.

## Wave 2: the SRD ruleset (2026-10-29)

- **Source.** SRD 5.2.1 (CC-BY-4.0): 12 classes with one subclass each,
  levels 1-5; 9 species; the SRD backgrounds plus six more in the same
  shape; origin and general feats, fighting styles, six invocations; about
  75 spells up to 3rd level. All data is Go tables in `internal/game`
  (`classes.go`, `origins.go`, `spells.go`); rules live in `combat5e.go`,
  `magic.go`, `features.go` and `sheet.go`.
- **Eras reskin, rules don't change.** Every class, subclass, species,
  background and spell has a name for gaslight, pulp, modern and future
  (`EraNames`); a test keeps names unique inside an era. The interpreter and
  `SpellByRef` accept the key, the SRD name or any era name.
- **Kept from before.** Weapon damage still adds no ability modifier (the
  enemy HP scale was tuned for it); rage, sneak attack, smite and the like add
  their own dice. Zones stand in for distance: touch is the same zone, near
  one zone, far two.
- **Bonus action.** The budget has a bonus slot. Spending the action still
  ends the turn, so bonus actions come in the same message. Outside fights
  features cost nothing.
- **Concentration** is one spell per caster; its conditions carry the spell
  and caster, and a failed CON save (DC max(10, half the damage)) ends them.
- **Old sheets** (no `class_key`) play as the class their kit maps to with
  their old proficient abilities as saves and their old HP.

## Wave 3: bosses, magic items, the DM console (2026-10-29)

- **Bosses.** One per fight, when the GM asks for it at danger 3+ or the
  enemy is the antagonist (or an NPC whose stats say `as_boss`). A boss
  gets 1.5x the fight's HP budget, near-cap AC and attack, 2-3 attacks a
  turn, and legendary resistance (1, or 2 in its lair) that turns a failed
  save into a success; grapples, shoves and its ongoing condition saves never
  spend one. The other enemies share a third of the budget.
- **Phases** at 100%, 50% and 25% HP: Enraged adds an attack and +2 damage,
  Desperate adds another and summons minions. Entering a phase shakes off
  prone, frightened and restrained. `bossPhase` runs from `hurtEnemy`, so
  every damage path triggers it.
- **Telegraphed signature move.** The boss announces it on the most crowded
  zone, then lands it on its next turn: (2+danger)d6, DEX save for half,
  prone at danger 3+. The table gets a round to move out or Dodge (dodging
  gives advantage on DEX saves). Two-turn cooldown after it lands.
- **Magic items** have five rarities, common to legendary, named per era.
  Weapon and armor templates (+1 to +3, flame, radiance) wrap any base
  item; wondrous items (protection, ability-setting, spell focus,
  regeneration) need attunement, three at most. `RefreshMagic` recomputes
  each sheet's boons on load and at turn boundaries; nothing is stored twice.
- **Loot** is rolled by the engine when a fight ends, seeded by campaign,
  encounter and round so a dry run and the real turn agree. Gold is split;
  a boss always drops a magic item scaled by lair, danger and party level;
  other fights sometimes drop potions or an uncommon item. Drops go on the
  ground for players to take.
- **DM console ("Table" tab).** Any seated player can use it; every action
  is an engine change plus a `dm_action` event, so the table sees who did
  it. Pages come from `DM_PAGES` in the UI so more can be added.
- **Rewind.** A snapshot of the world is saved before each turn (30 kept).
  `GM.Rewind` is the only entry point, so it can later be scoped to one
  group's scene. It restores the world, keeps the current seats and tokens,
  marks later events `undone` (hidden from feed and model), and drops later
  snapshots.

## Sagas: hand-written campaigns (2026-10-29)

The first saga is *The Long Dark Below* (Lovecraftian New England from 1931).
Plan: https://claude.ai/artifact/LWUoM8J2WtWrpZ8LvD8MJL. There are no acts
(Colin, 2026-10-29: "we build the world as it goes"): one open world of
regions, each with its own storyline and boss, playable in any order.
Innsmouth (where play starts) and Arkham with the Dunwich hills are written;
more regions join the world as they are written.

- **Regions grow on arrival.** `Build` lays out the start region in full and
  a road to every other region's hub, left as a stub. The first time the
  party reaches a stub hub, `saga.Arrive` builds that region from its data.
  Written places stay stubs so the author can still add people, things and
  new ways onward around them, but their written description is canon and
  is kept (`fillLocation`).
- **Storylines, not one main quest.** Every region's storyline is a main
  quest. `World.MainQuest()` returns the unfinished one for the region the
  party stands in; `UpdateLairGate` opens each storyline's own lair; a
  storyline boss's fall (`StorylineOf`) ends that storyline, records it in
  the chronicle and applies its milestone. Only a one-shot campaign (no saga)
  ends when its antagonist falls.
- **`internal/saga`** holds the campaign bible as Go data and builds the
  world from it. Every name must resolve (places, people, items, objective
  targets, beat targets, gates); a typo fails the build and the tests, never
  a quietly unwinnable quest. Anchor places with a written description are
  not stubs, so the author never rewrites them; zones are laid out on arrival.
- **Secrets are gated.** `Lore.Gate` names an objective or quest id; the
  narrator and the author's canon line only see lore whose gate is done
  (`game.LoreOpen`). A missing gate stays shut.
- **Milestones.** `Campaign.Milestones` maps objective ids to levels;
  completing one lifts every character to at least that level
  (`game.RaiseToLevel`). Innsmouth: objective 2 → level 2, objective 4 → 3,
  the boss → 4; Arkham: objective 2 → 4, the boss → 5. The story sets the pace; nobody has to grind.
- **Protected NPCs** (`NPC.Protected`): people a saga's story needs. The
  narrator can't turn them into foes; players can still pick that fight.
- **Hand-written bosses.** `BossSpec.Phases` renames the three default phases
  and their lines; `Minion`/`MinionHorror` set what answers the boss's call.
- **Dread and strain** (`Campaign.Dread`, set by sagas; `game/strain.go`).
  Strain 0..6 per character. A failed WIS save against a horror (DC 10 +
  rank 1..3) adds 1, a natural 1 adds 2. At 2+ disadvantage on saves against
  fear; reaching 4 = frightened for a moment; reaching 6 = a lasting scar
  (d8 table, never twice) and strain back to 3. A long rest at a safe place
  takes 1 off. Sources: an enemy's horror rank on first sight of its kind
  (once per character per kind, at the start of the enemies' turns) and the
  narrator's `hazard` tool with condition `horror`. Scars are played by the
  narrator; "Hears whispers" also lowers passive Perception by 2.

## Shared world: groups (2026-10-30)

One campaign is one shared world. A character can go off alone, and groups that meet in the same place can join up. The world (places, people, items, quests, lore, the chronicle) is shared; each group owns where it stands, its scene, its turn order, its fight and its story so far.

- The main group lives in the campaign's own fields, so a world with one group plays and stores exactly as before. Split-off groups live in `Campaign.Groups`; a character's `Group` names its group ("" is main).
- `World.Focus(group)` swaps a group's state into the campaign fields and hides everyone else's characters and players, so the whole engine and GM pipeline run unchanged for that group. `Settle` undoes it; the store settles before saving or snapshotting. Every GM entry point that acts for a player (`TakeAction`, `Roll`, `Skip`, `CancelPending`, replacing a dead character) and every read view focuses on the caller's group.
- `SplitOff` refuses mid-fight and refuses to split a whole group. `JoinGroups` needs both groups in the same place and neither fighting; the joined group keeps the target's scene and is the main group if either was.
- Events saved while focused carry `party` (the group id); the app shows a player the events of the groups they were in at the time, so their story before a split stays in their feed. `group_split` and `group_joined` events are shown to everyone.
- The narrator gets an OTHER PLAYER CHARACTERS line: who is in another group and where, flagged when they stand in the same place, and is told never to narrate them acting.
- Not yet: side initiative when two groups share a fight, per-group rewind (the DM console rewinds the whole world), and changed-row saves.

## Async turns (2026-10-30)

Long campaigns (every saga; `Campaign.LongForm()`) are played over weeks, so a fight waits for its players.

- In a fight, the player whose turn it is has `TurnDeadline` (12 hours) to play it. Other players can't skip it before then.
- Past the deadline, autopilot plays it (`gm.autopilot`, `game.AutopilotActions`): a plain strike at the weakest foe in reach, or a dodge, never a spell slot, item or limited feature. A dying character's death save is rolled by the engine. The turn is logged as "(away) ... fights on by instinct" and narrated like any other.
- `GM.Sweep` runs every turn that's overdue, in every group of every long campaign. The server runs it every minute, and the Worker's hourly cron calls `POST /tick` so a sleeping container wakes up and catches up.
- Alerts stay in the app: `/me/campaigns` marks `your_turn` (and `deadline`), the home screen leads with that story ("Your turn · autopilot in 11h"), the tab title says "Your turn", and the dock shows the time left.

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
