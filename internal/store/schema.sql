-- aidnd schema. Every column is NOT NULL with a default so that columns
-- added here later are migrated onto older databases with ALTER TABLE.
-- Nested lists (zones, objectives, memory notes) are JSON text columns.

CREATE TABLE IF NOT EXISTS campaigns (
    id            TEXT PRIMARY KEY,
    title         TEXT NOT NULL DEFAULT '',
    premise       TEXT NOT NULL DEFAULT '',
    pitch         TEXT NOT NULL DEFAULT '',
    setting       TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'lobby',
    hub_id        TEXT NOT NULL DEFAULT '',
    lair_id       TEXT NOT NULL DEFAULT '',
    antagonist_id TEXT NOT NULL DEFAULT '',
    location_id   TEXT NOT NULL DEFAULT '',
    turn          INTEGER NOT NULL DEFAULT 0,
    seq           INTEGER NOT NULL DEFAULT 0,
    setbacks      INTEGER NOT NULL DEFAULT 0,
    max_setbacks  INTEGER NOT NULL DEFAULT 3,
    max_players   INTEGER NOT NULL DEFAULT 4,
    day           INTEGER NOT NULL DEFAULT 0,
    time_of_day   INTEGER NOT NULL DEFAULT 0,
    tension       INTEGER NOT NULL DEFAULT 0,
    scene_goal    TEXT NOT NULL DEFAULT '',
    scene_turns   INTEGER NOT NULL DEFAULT 0,
    scene_quiet   INTEGER NOT NULL DEFAULT 0,
    scene_start   INTEGER NOT NULL DEFAULT 0,
    turn_order    TEXT NOT NULL DEFAULT '[]',
    turn_index    INTEGER NOT NULL DEFAULT 0,
    turn_started  INTEGER NOT NULL DEFAULT 0,
    budget_free   INTEGER NOT NULL DEFAULT -1,
    budget_move   INTEGER NOT NULL DEFAULT 1,
    budget_action INTEGER NOT NULL DEFAULT 1,
    summary       TEXT NOT NULL DEFAULT '',
    summary_turn  INTEGER NOT NULL DEFAULT 0,
    created_at    INTEGER NOT NULL DEFAULT 0,
    updated_at    INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS players (
    campaign_id  TEXT NOT NULL,
    id           TEXT NOT NULL,
    ord          INTEGER NOT NULL DEFAULT 0,
    name         TEXT NOT NULL DEFAULT '',
    character_id TEXT NOT NULL DEFAULT '',
    token        TEXT NOT NULL DEFAULT '',
    joined_turn  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (campaign_id, id)
);
CREATE INDEX IF NOT EXISTS players_by_id ON players(id);

CREATE TABLE IF NOT EXISTS characters (
    campaign_id TEXT NOT NULL,
    id          TEXT NOT NULL,
    ord         INTEGER NOT NULL DEFAULT 0,
    player_id   TEXT NOT NULL DEFAULT '',
    name        TEXT NOT NULL DEFAULT '',
    concept     TEXT NOT NULL DEFAULT '',
    class       TEXT NOT NULL DEFAULT '',
    kit         TEXT NOT NULL DEFAULT '',
    backstory   TEXT NOT NULL DEFAULT '',
    pronouns    TEXT NOT NULL DEFAULT '',
    scores      TEXT NOT NULL DEFAULT '{}',
    proficient  TEXT NOT NULL DEFAULT '[]',
    level       INTEGER NOT NULL DEFAULT 1,
    xp          INTEGER NOT NULL DEFAULT 0,
    hp          INTEGER NOT NULL DEFAULT 0,
    max_hp      INTEGER NOT NULL DEFAULT 0,
    gold        INTEGER NOT NULL DEFAULT 0,
    weapon      TEXT NOT NULL DEFAULT '',
    armor       TEXT NOT NULL DEFAULT '',
    zone_id     TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (campaign_id, id)
);

CREATE TABLE IF NOT EXISTS locations (
    campaign_id TEXT NOT NULL,
    id          TEXT NOT NULL,
    ord         INTEGER NOT NULL DEFAULT 0,
    name        TEXT NOT NULL DEFAULT '',
    kind        TEXT NOT NULL DEFAULT '',
    hint        TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    danger      INTEGER NOT NULL DEFAULT 0,
    stub        INTEGER NOT NULL DEFAULT 1,
    visited     INTEGER NOT NULL DEFAULT 0,
    is_hub      INTEGER NOT NULL DEFAULT 0,
    is_lair     INTEGER NOT NULL DEFAULT 0,
    zones       TEXT NOT NULL DEFAULT '[]',
    last_day    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (campaign_id, id)
);

CREATE TABLE IF NOT EXISTS edges (
    campaign_id TEXT NOT NULL,
    id          TEXT NOT NULL,
    ord         INTEGER NOT NULL DEFAULT 0,
    a           TEXT NOT NULL DEFAULT '',
    b           TEXT NOT NULL DEFAULT '',
    label_a     TEXT NOT NULL DEFAULT '',
    label_b     TEXT NOT NULL DEFAULT '',
    zone_a      TEXT NOT NULL DEFAULT '',
    zone_b      TEXT NOT NULL DEFAULT '',
    state       TEXT NOT NULL DEFAULT 'known',
    key_item    TEXT NOT NULL DEFAULT '',
    lock_dc     INTEGER NOT NULL DEFAULT 0,
    lair_gate   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (campaign_id, id)
);

CREATE TABLE IF NOT EXISTS npcs (
    campaign_id TEXT NOT NULL,
    id          TEXT NOT NULL,
    ord         INTEGER NOT NULL DEFAULT 0,
    name        TEXT NOT NULL DEFAULT '',
    role        TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    location_id TEXT NOT NULL DEFAULT '',
    zone_id     TEXT NOT NULL DEFAULT '',
    disposition INTEGER NOT NULL DEFAULT 0,
    merchant    INTEGER NOT NULL DEFAULT 0,
    alive       INTEGER NOT NULL DEFAULT 1,
    antagonist  INTEGER NOT NULL DEFAULT 0,
    memory      TEXT NOT NULL DEFAULT '[]',
    keeps       TEXT NOT NULL DEFAULT '[]',
    last_talked INTEGER NOT NULL DEFAULT -1,
    stats       TEXT NOT NULL DEFAULT 'null',
    PRIMARY KEY (campaign_id, id)
);

CREATE TABLE IF NOT EXISTS items (
    campaign_id TEXT NOT NULL,
    id          TEXT NOT NULL,
    ord         INTEGER NOT NULL DEFAULT 0,
    key         TEXT NOT NULL DEFAULT '',
    name        TEXT NOT NULL DEFAULT '',
    kind        TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    value       INTEGER NOT NULL DEFAULT 0,
    damage      TEXT NOT NULL DEFAULT '',
    ability     TEXT NOT NULL DEFAULT '',
    finesse     INTEGER NOT NULL DEFAULT 0,
    reach       INTEGER NOT NULL DEFAULT 0,
    ac          INTEGER NOT NULL DEFAULT 0,
    dex_cap     INTEGER NOT NULL DEFAULT 0,
    effect      TEXT NOT NULL DEFAULT '',
    owner_kind  TEXT NOT NULL DEFAULT '',
    owner_id    TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (campaign_id, id)
);
CREATE INDEX IF NOT EXISTS items_by_owner ON items(campaign_id, owner_kind, owner_id);

CREATE TABLE IF NOT EXISTS quests (
    campaign_id   TEXT NOT NULL,
    id            TEXT NOT NULL,
    ord           INTEGER NOT NULL DEFAULT 0,
    title         TEXT NOT NULL DEFAULT '',
    summary       TEXT NOT NULL DEFAULT '',
    giver_id      TEXT NOT NULL DEFAULT '',
    is_main       INTEGER NOT NULL DEFAULT 0,
    status        TEXT NOT NULL DEFAULT 'offered',
    objectives    TEXT NOT NULL DEFAULT '[]',
    reward_gold   INTEGER NOT NULL DEFAULT 0,
    reward_xp     INTEGER NOT NULL DEFAULT 0,
    reward_items  TEXT NOT NULL DEFAULT '[]',
    accepted_turn INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (campaign_id, id)
);

CREATE TABLE IF NOT EXISTS encounters (
    campaign_id TEXT PRIMARY KEY,
    id          TEXT NOT NULL DEFAULT '',
    location_id TEXT NOT NULL DEFAULT '',
    round       INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS enemies (
    campaign_id  TEXT NOT NULL,
    id           TEXT NOT NULL,
    ord          INTEGER NOT NULL DEFAULT 0,
    name         TEXT NOT NULL DEFAULT '',
    tag          TEXT NOT NULL DEFAULT '',
    npc_id       TEXT NOT NULL DEFAULT '',
    max_hp       INTEGER NOT NULL DEFAULT 1,
    ac           INTEGER NOT NULL DEFAULT 10,
    attack_bonus INTEGER NOT NULL DEFAULT 0,
    damage       TEXT NOT NULL DEFAULT '1d4',
    ranged       INTEGER NOT NULL DEFAULT 0,
    hp           INTEGER NOT NULL DEFAULT 1,
    zone_id      TEXT NOT NULL DEFAULT '',
    subdued      INTEGER NOT NULL DEFAULT 0,
    acted_round  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (campaign_id, id)
);

CREATE TABLE IF NOT EXISTS lore (
    campaign_id  TEXT NOT NULL,
    id           TEXT NOT NULL,
    ord          INTEGER NOT NULL DEFAULT 0,
    title        TEXT NOT NULL DEFAULT '',
    text         TEXT NOT NULL DEFAULT '',
    keywords     TEXT NOT NULL DEFAULT '[]',
    entity_ids   TEXT NOT NULL DEFAULT '[]',
    always_on    INTEGER NOT NULL DEFAULT 0,
    last_trigger INTEGER NOT NULL DEFAULT -100,
    PRIMARY KEY (campaign_id, id)
);

CREATE TABLE IF NOT EXISTS beats (
    campaign_id   TEXT NOT NULL,
    id            TEXT NOT NULL,
    ord           INTEGER NOT NULL DEFAULT 0,
    title         TEXT NOT NULL DEFAULT '',
    text          TEXT NOT NULL DEFAULT '',
    trigger_kind  TEXT NOT NULL DEFAULT '',
    target        TEXT NOT NULL DEFAULT '',
    value         INTEGER NOT NULL DEFAULT 0,
    effects       TEXT NOT NULL DEFAULT '[]',
    effect_target TEXT NOT NULL DEFAULT '',
    fired         INTEGER NOT NULL DEFAULT 0,
    fired_turn    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (campaign_id, id)
);

CREATE TABLE IF NOT EXISTS chronicle (
    campaign_id TEXT NOT NULL,
    ord         INTEGER NOT NULL,
    turn        INTEGER NOT NULL DEFAULT 0,
    text        TEXT NOT NULL DEFAULT '',
    id          TEXT NOT NULL DEFAULT '',
    day         INTEGER NOT NULL DEFAULT 0,
    kind        TEXT NOT NULL DEFAULT '',
    location_id TEXT NOT NULL DEFAULT '',
    npc_ids     TEXT NOT NULL DEFAULT '[]',
    source      TEXT NOT NULL DEFAULT '',
    quest_id    TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT '',
    outcome     TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (campaign_id, ord)
);

CREATE TABLE IF NOT EXISTS defeats (
    campaign_id TEXT NOT NULL,
    ord         INTEGER NOT NULL,
    name        TEXT NOT NULL DEFAULT '',
    npc_id      TEXT NOT NULL DEFAULT '',
    location_id TEXT NOT NULL DEFAULT '',
    turn        INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (campaign_id, ord)
);

-- Append-only event log. id is global and monotonic, used as the SSE
-- event id so clients can resume with Last-Event-ID.
CREATE TABLE IF NOT EXISTS events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    campaign_id TEXT NOT NULL,
    type        TEXT NOT NULL DEFAULT '',
    actor       TEXT NOT NULL DEFAULT '',
    data        TEXT NOT NULL DEFAULT '{}',
    created_at  INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS events_by_campaign ON events(campaign_id, id);
