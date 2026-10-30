// Package store persists game.World in SQLite (modernc.org/sqlite, no cgo).
// A world is loaded and saved as a unit, and new events are appended in
// the same transaction as the save.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Solvely-Colin/aidnd/internal/game"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// ErrNotFound is returned for unknown campaigns.
var ErrNotFound = errors.New("not found")

// Store is a SQLite-backed campaign store.
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// Open opens (or creates) the database at path and migrates it. Use
// ":memory:" for tests.
func Open(path string) (*Store, error) {
	dsn := path
	if path == ":memory:" {
		dsn = "file::memory:"
	} else if !strings.HasPrefix(path, "file:") {
		dsn = "file:" + path
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	dsn += sep + "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One connection serializes writers and keeps :memory: a single database.
	db.SetMaxOpenConns(1)
	s := &Store{db: db, now: time.Now}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// DB exposes the handle (for tests and small extra tables in other packages).
func (s *Store) DB() *sql.DB { return s.db }

var (
	createRe = regexp.MustCompile(`(?is)CREATE TABLE IF NOT EXISTS (\w+) \((.*?)\n\);`)
	colRe    = regexp.MustCompile(`^\s*(\w+)\s+(.+?),?\s*$`)
)

// schemaColumns parses schema.sql into table -> ordered column definitions.
func schemaColumns() map[string][][2]string {
	out := map[string][][2]string{}
	for _, m := range createRe.FindAllStringSubmatch(schemaSQL, -1) {
		for _, line := range strings.Split(m[2], "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "--") {
				continue
			}
			up := strings.ToUpper(line)
			if strings.HasPrefix(up, "PRIMARY KEY") || strings.HasPrefix(up, "UNIQUE") {
				continue
			}
			if c := colRe.FindStringSubmatch(line); c != nil {
				out[m[1]] = append(out[m[1]], [2]string{c[1], strings.TrimSuffix(c[2], ",")})
			}
		}
	}
	return out
}

// migrate creates missing tables, then adds any column that schema.sql
// declares but an older database lacks.
func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("schema: %w", err)
	}
	for table, cols := range schemaColumns() {
		have := map[string]bool{}
		// PRAGMA table_info rather than pragma_table_info(?): D1 allows only
		// the statement form. Table names come from our own schema.
		rows, err := s.db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
		if err != nil {
			return err
		}
		cols2, _ := rows.Columns()
		for rows.Next() {
			vals := make([]any, len(cols2))
			ptrs := make([]any, len(cols2))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				rows.Close()
				return err
			}
			for i, c := range cols2 {
				if c == "name" {
					have[asString(vals[i])] = true
				}
			}
		}
		rows.Close()
		for _, c := range cols {
			if have[c[0]] {
				continue
			}
			def := c[1]
			if strings.Contains(strings.ToUpper(def), "PRIMARY KEY") {
				return fmt.Errorf("cannot migrate primary key column %s.%s", table, c[0])
			}
			if _, err := s.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, c[0], def)); err != nil {
				return fmt.Errorf("migrate %s.%s: %w", table, c[0], err)
			}
		}
	}
	return nil
}

func asString(v any) string {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return fmt.Sprint(v)
}

func js(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(b)
}

func unjs(s string, v any) error {
	if s == "" {
		return nil
	}
	return json.Unmarshal([]byte(s), v)
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func insert(ctx context.Context, tx execer, table string, cols []string, vals ...any) error {
	q := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table, strings.Join(cols, ","), strings.TrimSuffix(strings.Repeat("?,", len(cols)), ","))
	_, err := tx.ExecContext(ctx, q, vals...)
	if err != nil {
		return fmt.Errorf("insert %s: %w", table, err)
	}
	return nil
}

var childTables = []string{"players", "characters", "locations", "edges", "npcs", "items", "quests", "encounters", "enemies", "lore", "beats", "chronicle", "defeats"}

// Event is one entry of the append-only log.
type Event struct {
	ID         int64           `json:"id"`
	CampaignID string          `json:"campaign_id"`
	Type       string          `json:"type"`
	Actor      string          `json:"actor,omitempty"`
	Data       json.RawMessage `json:"data"`
	CreatedAt  int64           `json:"created_at"`
}

// NewEvent builds an event with JSON-encoded data.
func NewEvent(typ, actor string, data any) Event {
	return Event{Type: typ, Actor: actor, Data: json.RawMessage(js(data))}
}

// Save writes the whole world and appends events in one transaction. The
// returned events carry their assigned ids.
func (s *Store) Save(ctx context.Context, w *game.World, events ...Event) ([]Event, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := saveWorld(ctx, tx, w, s.now().Unix()); err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(events))
	// On D1 a statement's result is known only after Commit.
	results := make([]sql.Result, 0, len(events))
	for _, e := range events {
		e.CampaignID = w.Campaign.ID
		if e.CreatedAt == 0 {
			e.CreatedAt = s.now().Unix()
		}
		if len(e.Data) == 0 {
			e.Data = json.RawMessage("{}")
		}
		res, err := tx.ExecContext(ctx, "INSERT INTO events (campaign_id, type, actor, data, created_at) VALUES (?,?,?,?,?)",
			e.CampaignID, e.Type, e.Actor, string(e.Data), e.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("append event: %w", err)
		}
		results = append(results, res)
		out = append(out, e)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	for i, res := range results {
		out[i].ID, _ = res.LastInsertId()
	}
	return out, nil
}

func saveWorld(ctx context.Context, tx *sql.Tx, w *game.World, now int64) error {
	c := &w.Campaign
	if c.ID == "" {
		return fmt.Errorf("campaign has no id")
	}
	if c.CreatedAt == 0 {
		c.CreatedAt = now
	}
	sc, tu := c.Scene, c.Turns
	if _, err := tx.ExecContext(ctx, "DELETE FROM campaigns WHERE id = ?", c.ID); err != nil {
		return err
	}
	if err := insert(ctx, tx, "campaigns", []string{"id", "title", "premise", "pitch", "status", "hub_id", "lair_id", "antagonist_id", "location_id",
		"turn", "seq", "setbacks", "max_setbacks", "max_players", "day", "time_of_day", "tension", "scene_goal", "scene_turns", "scene_quiet", "scene_start",
		"turn_order", "turn_index", "turn_started", "budget_free", "budget_move", "budget_action", "budget_bonus", "summary", "summary_turn", "created_at", "setting", "updated_at", "extra"},
		c.ID, c.Title, c.Premise, c.Pitch, c.Status, c.HubID, c.LairID, c.AntagonistID, c.LocationID,
		c.Turn, c.Seq, c.Setbacks, c.MaxSetbacks, c.MaxPlayers, sc.Day, sc.Time, sc.Tension, sc.Goal, sc.Turns, sc.Quiet, sc.StartTurn,
		js(nonNil(tu.Order)), tu.Index, tu.Started, tu.Budget.Free, tu.Budget.Move, tu.Budget.Action, tu.Budget.Bonus, c.Summary, c.SummaryTurn, c.CreatedAt, c.Setting, now, js(c.CampaignExtra)); err != nil {
		return err
	}
	for _, t := range childTables {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+t+" WHERE campaign_id = ?", c.ID); err != nil {
			return err
		}
	}
	id := c.ID
	for i, p := range w.Players {
		if err := insert(ctx, tx, "players", []string{"campaign_id", "id", "ord", "name", "character_id", "token", "joined_turn", "auto_roll"},
			id, p.ID, i, p.Name, p.CharacterID, p.Token, p.JoinedTurn, p.AutoRoll); err != nil {
			return err
		}
	}
	for i, ch := range w.Characters {
		if err := insert(ctx, tx, "characters", []string{"campaign_id", "id", "ord", "player_id", "name", "concept", "class", "kit", "backstory", "pronouns",
			"scores", "proficient", "level", "xp", "hp", "max_hp", "gold", "weapon", "armor", "zone_id", "conditions", "extra"},
			id, ch.ID, i, ch.PlayerID, ch.Name, ch.Concept, ch.Class, ch.Kit, ch.Backstory, ch.Pronouns,
			js(ch.Scores), js(nonNil(ch.Proficient)), ch.Level, ch.XP, ch.HP, ch.MaxHP, ch.Gold, ch.Weapon, ch.Armor, ch.ZoneID, js(nonNil(ch.Conditions)), js(ch.CharExtra)); err != nil {
			return err
		}
	}
	for i, l := range w.Locations {
		if err := insert(ctx, tx, "locations", []string{"campaign_id", "id", "ord", "name", "kind", "hint", "description", "danger", "stub", "visited", "is_hub", "is_lair", "zones", "last_day", "region"},
			id, l.ID, i, l.Name, l.Kind, l.Hint, l.Description, l.Danger, l.Stub, l.Visited, l.IsHub, l.IsLair, js(nonNil(l.Zones)), l.LastDay, l.Region); err != nil {
			return err
		}
	}
	for i, e := range w.Edges {
		if err := insert(ctx, tx, "edges", []string{"campaign_id", "id", "ord", "a", "b", "label_a", "label_b", "zone_a", "zone_b", "state", "key_item", "lock_dc", "lair_gate"},
			id, e.ID, i, e.A, e.B, e.LabelA, e.LabelB, e.ZoneA, e.ZoneB, string(e.State), e.KeyItem, e.LockDC, e.LairGate); err != nil {
			return err
		}
	}
	for i, n := range w.NPCs {
		if err := insert(ctx, tx, "npcs", []string{"campaign_id", "id", "ord", "name", "role", "description", "location_id", "zone_id", "disposition", "merchant", "alive", "antagonist", "memory", "last_talked", "stats", "keeps", "extra"},
			id, n.ID, i, n.Name, n.Role, n.Desc, n.LocationID, n.ZoneID, n.Disposition, n.Merchant, n.Alive, n.Antagonist, js(nonNil(n.Memory)), n.LastTalked, js(n.Stats), js(nonNil(n.Keeps)), js(n.NPCExtra)); err != nil {
			return err
		}
	}
	for i, it := range w.Items {
		if err := insert(ctx, tx, "items", []string{"campaign_id", "id", "ord", "key", "name", "kind", "description", "value", "damage", "ability", "finesse", "reach", "ac", "dex_cap", "effect", "owner_kind", "owner_id", "extra"},
			id, it.ID, i, it.Key, it.Name, it.Kind, it.Desc, it.Value, it.Damage, string(it.Ability), it.Finesse, int(it.Reach), it.AC, it.DexCap, it.Effect, string(it.Owner.Kind), it.Owner.ID, js(it.ItemExtra)); err != nil {
			return err
		}
	}
	for i, q := range w.Quests {
		if err := insert(ctx, tx, "quests", []string{"campaign_id", "id", "ord", "title", "summary", "giver_id", "is_main", "status", "objectives", "reward_gold", "reward_xp", "reward_items", "accepted_turn"},
			id, q.ID, i, q.Title, q.Summary, q.GiverID, q.IsMain, q.Status, js(nonNil(q.Objectives)), q.RewardGold, q.RewardXP, js(nonNil(q.RewardItems)), q.AcceptedTurn); err != nil {
			return err
		}
	}
	if enc := w.Encounter; enc != nil {
		if err := insert(ctx, tx, "encounters", []string{"campaign_id", "id", "location_id", "round"}, id, enc.ID, enc.LocationID, enc.Round); err != nil {
			return err
		}
		for i, e := range enc.Enemies {
			if err := insert(ctx, tx, "enemies", []string{"campaign_id", "id", "ord", "name", "tag", "npc_id", "max_hp", "ac", "attack_bonus", "damage", "ranged", "hp", "zone_id", "subdued", "acted_round", "inflicts", "conditions", "extra"},
				id, e.ID, i, e.Name, e.Tag, e.NPCID, e.MaxHP, e.AC, e.AttackBonus, e.Damage, e.Ranged, e.HP, e.ZoneID, e.Subdued, e.ActedRound, e.Inflicts, js(nonNil(e.Conditions)), js(e.EnemyExtra)); err != nil {
				return err
			}
		}
	}
	for i, l := range w.Lore {
		if err := insert(ctx, tx, "lore", []string{"campaign_id", "id", "ord", "title", "text", "keywords", "entity_ids", "always_on", "last_trigger", "extra"},
			id, l.ID, i, l.Title, l.Text, js(nonNil(l.Keywords)), js(nonNil(l.EntityIDs)), l.AlwaysOn, l.LastTrigger, js(l.LoreExtra)); err != nil {
			return err
		}
	}
	for i, b := range w.Beats {
		if err := insert(ctx, tx, "beats", []string{"campaign_id", "id", "ord", "title", "text", "trigger_kind", "target", "value", "effects", "effect_target", "fired", "fired_turn"},
			id, b.ID, i, b.Title, b.Text, b.Trigger, b.Target, b.Value, js(nonNil(b.Effects)), b.EffectTarget, b.Fired, b.FiredTurn); err != nil {
			return err
		}
	}
	for i, ce := range w.Chronicle {
		if err := insert(ctx, tx, "chronicle", []string{"campaign_id", "ord", "turn", "text", "id", "day", "kind", "location_id", "npc_ids", "source", "quest_id", "status", "outcome"},
			id, i, ce.Turn, ce.Text, ce.ID, ce.Day, ce.Kind, ce.LocationID, js(nonNil(ce.NPCIDs)), ce.Source, ce.QuestID, ce.Status, ce.Outcome); err != nil {
			return err
		}
	}
	for i, d := range w.Defeats {
		if err := insert(ctx, tx, "defeats", []string{"campaign_id", "ord", "name", "npc_id", "location_id", "turn"}, id, i, d.Name, d.NPCID, d.LocationID, d.Turn); err != nil {
			return err
		}
	}
	return nil
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// Load reads a whole world.
func (s *Store) Load(ctx context.Context, id string) (*game.World, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	w := &game.World{}
	c := &w.Campaign
	var order, extra string
	err = tx.QueryRowContext(ctx, `SELECT id, title, premise, pitch, status, hub_id, lair_id, antagonist_id, location_id,
		turn, seq, setbacks, max_setbacks, max_players, day, time_of_day, tension, scene_goal, scene_turns, scene_quiet, scene_start,
		turn_order, turn_index, turn_started, budget_free, budget_move, budget_action, budget_bonus, summary, summary_turn, created_at, setting, extra
		FROM campaigns WHERE id = ?`, id).Scan(
		&c.ID, &c.Title, &c.Premise, &c.Pitch, &c.Status, &c.HubID, &c.LairID, &c.AntagonistID, &c.LocationID,
		&c.Turn, &c.Seq, &c.Setbacks, &c.MaxSetbacks, &c.MaxPlayers, &c.Scene.Day, &c.Scene.Time, &c.Scene.Tension, &c.Scene.Goal,
		&c.Scene.Turns, &c.Scene.Quiet, &c.Scene.StartTurn,
		&order, &c.Turns.Index, &c.Turns.Started, &c.Turns.Budget.Free, &c.Turns.Budget.Move, &c.Turns.Budget.Action, &c.Turns.Budget.Bonus,
		&c.Summary, &c.SummaryTurn, &c.CreatedAt, &c.Setting, &extra)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := errors.Join(unjs(order, &c.Turns.Order), unjs(extra, &c.CampaignExtra)); err != nil {
		return nil, err
	}
	q := func(sqlText string, scan func(*sql.Rows) error) error {
		rows, err := tx.QueryContext(ctx, sqlText, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			if err := scan(rows); err != nil {
				return err
			}
		}
		return rows.Err()
	}
	steps := []error{
		q("SELECT id, name, character_id, token, joined_turn, auto_roll FROM players WHERE campaign_id=? ORDER BY ord", func(r *sql.Rows) error {
			p := &game.Player{}
			w.Players = append(w.Players, p)
			return r.Scan(&p.ID, &p.Name, &p.CharacterID, &p.Token, &p.JoinedTurn, &p.AutoRoll)
		}),
		q(`SELECT id, player_id, name, concept, class, kit, backstory, pronouns, scores, proficient, level, xp, hp, max_hp, gold, weapon, armor, zone_id, conditions, extra
			FROM characters WHERE campaign_id=? ORDER BY ord`, func(r *sql.Rows) error {
			ch := &game.Character{}
			var scores, prof, conds, extra string
			if err := r.Scan(&ch.ID, &ch.PlayerID, &ch.Name, &ch.Concept, &ch.Class, &ch.Kit, &ch.Backstory, &ch.Pronouns, &scores, &prof,
				&ch.Level, &ch.XP, &ch.HP, &ch.MaxHP, &ch.Gold, &ch.Weapon, &ch.Armor, &ch.ZoneID, &conds, &extra); err != nil {
				return err
			}
			w.Characters = append(w.Characters, ch)
			return errors.Join(unjs(scores, &ch.Scores), unjs(prof, &ch.Proficient), unjs(conds, &ch.Conditions), unjs(extra, &ch.CharExtra))
		}),
		q(`SELECT id, name, kind, hint, description, danger, stub, visited, is_hub, is_lair, zones, last_day, region FROM locations WHERE campaign_id=? ORDER BY ord`, func(r *sql.Rows) error {
			l := &game.Location{}
			var zones string
			if err := r.Scan(&l.ID, &l.Name, &l.Kind, &l.Hint, &l.Description, &l.Danger, &l.Stub, &l.Visited, &l.IsHub, &l.IsLair, &zones, &l.LastDay, &l.Region); err != nil {
				return err
			}
			w.Locations = append(w.Locations, l)
			return unjs(zones, &l.Zones)
		}),
		q(`SELECT id, a, b, label_a, label_b, zone_a, zone_b, state, key_item, lock_dc, lair_gate FROM edges WHERE campaign_id=? ORDER BY ord`, func(r *sql.Rows) error {
			e := &game.Edge{}
			var state string
			if err := r.Scan(&e.ID, &e.A, &e.B, &e.LabelA, &e.LabelB, &e.ZoneA, &e.ZoneB, &state, &e.KeyItem, &e.LockDC, &e.LairGate); err != nil {
				return err
			}
			e.State = game.EdgeState(state)
			w.Edges = append(w.Edges, e)
			return nil
		}),
		q(`SELECT id, name, role, description, location_id, zone_id, disposition, merchant, alive, antagonist, memory, last_talked, stats, keeps, extra
			FROM npcs WHERE campaign_id=? ORDER BY ord`, func(r *sql.Rows) error {
			n := &game.NPC{}
			var mem, stats, keeps, extra string
			if err := r.Scan(&n.ID, &n.Name, &n.Role, &n.Desc, &n.LocationID, &n.ZoneID, &n.Disposition, &n.Merchant, &n.Alive, &n.Antagonist, &mem, &n.LastTalked, &stats, &keeps, &extra); err != nil {
				return err
			}
			w.NPCs = append(w.NPCs, n)
			return errors.Join(unjs(mem, &n.Memory), unjs(stats, &n.Stats), unjs(keeps, &n.Keeps), unjs(extra, &n.NPCExtra))
		}),
		q(`SELECT id, key, name, kind, description, value, damage, ability, finesse, reach, ac, dex_cap, effect, owner_kind, owner_id, extra
			FROM items WHERE campaign_id=? ORDER BY ord`, func(r *sql.Rows) error {
			it := &game.Item{}
			var ab, ok, extra string
			var reach int
			if err := r.Scan(&it.ID, &it.Key, &it.Name, &it.Kind, &it.Desc, &it.Value, &it.Damage, &ab, &it.Finesse, &reach, &it.AC, &it.DexCap, &it.Effect, &ok, &it.Owner.ID, &extra); err != nil {
				return err
			}
			it.Ability, it.Reach, it.Owner.Kind = game.Ability(ab), game.Reach(reach), game.OwnerKind(ok)
			w.Items = append(w.Items, it)
			return unjs(extra, &it.ItemExtra)
		}),
		q(`SELECT id, title, summary, giver_id, is_main, status, objectives, reward_gold, reward_xp, reward_items, accepted_turn
			FROM quests WHERE campaign_id=? ORDER BY ord`, func(r *sql.Rows) error {
			qu := &game.Quest{}
			var objs, items string
			if err := r.Scan(&qu.ID, &qu.Title, &qu.Summary, &qu.GiverID, &qu.IsMain, &qu.Status, &objs, &qu.RewardGold, &qu.RewardXP, &items, &qu.AcceptedTurn); err != nil {
				return err
			}
			w.Quests = append(w.Quests, qu)
			return errors.Join(unjs(objs, &qu.Objectives), unjs(items, &qu.RewardItems))
		}),
		q(`SELECT id, location_id, round FROM encounters WHERE campaign_id=?`, func(r *sql.Rows) error {
			w.Encounter = &game.Encounter{}
			return r.Scan(&w.Encounter.ID, &w.Encounter.LocationID, &w.Encounter.Round)
		}),
		q(`SELECT id, name, tag, npc_id, max_hp, ac, attack_bonus, damage, ranged, hp, zone_id, subdued, acted_round, inflicts, conditions, extra FROM enemies WHERE campaign_id=? ORDER BY ord`, func(r *sql.Rows) error {
			e := &game.Enemy{}
			var conds, extra string
			if err := r.Scan(&e.ID, &e.Name, &e.Tag, &e.NPCID, &e.MaxHP, &e.AC, &e.AttackBonus, &e.Damage, &e.Ranged, &e.HP, &e.ZoneID, &e.Subdued, &e.ActedRound, &e.Inflicts, &conds, &extra); err != nil {
				return err
			}
			if w.Encounter != nil {
				w.Encounter.Enemies = append(w.Encounter.Enemies, e)
			}
			return errors.Join(unjs(conds, &e.Conditions), unjs(extra, &e.EnemyExtra))
		}),
		q(`SELECT id, title, text, keywords, entity_ids, always_on, last_trigger, extra FROM lore WHERE campaign_id=? ORDER BY ord`, func(r *sql.Rows) error {
			l := &game.Lore{}
			var kw, ents, extra string
			if err := r.Scan(&l.ID, &l.Title, &l.Text, &kw, &ents, &l.AlwaysOn, &l.LastTrigger, &extra); err != nil {
				return err
			}
			w.Lore = append(w.Lore, l)
			return errors.Join(unjs(kw, &l.Keywords), unjs(ents, &l.EntityIDs), unjs(extra, &l.LoreExtra))
		}),
		q(`SELECT id, title, text, trigger_kind, target, value, effects, effect_target, fired, fired_turn FROM beats WHERE campaign_id=? ORDER BY ord`, func(r *sql.Rows) error {
			b := &game.Beat{}
			var eff string
			if err := r.Scan(&b.ID, &b.Title, &b.Text, &b.Trigger, &b.Target, &b.Value, &eff, &b.EffectTarget, &b.Fired, &b.FiredTurn); err != nil {
				return err
			}
			w.Beats = append(w.Beats, b)
			return unjs(eff, &b.Effects)
		}),
		q(`SELECT turn, text, id, day, kind, location_id, npc_ids, source, quest_id, status, outcome FROM chronicle WHERE campaign_id=? ORDER BY ord`, func(r *sql.Rows) error {
			var ce game.ChronicleEntry
			var npcs string
			if err := r.Scan(&ce.Turn, &ce.Text, &ce.ID, &ce.Day, &ce.Kind, &ce.LocationID, &npcs, &ce.Source, &ce.QuestID, &ce.Status, &ce.Outcome); err != nil {
				return err
			}
			if ce.ID == "" { // written before facts had ids
				ce.ID = fmt.Sprintf("fact%d", len(w.Chronicle)+1)
			}
			err := unjs(npcs, &ce.NPCIDs)
			w.Chronicle = append(w.Chronicle, ce)
			return err
		}),
		q(`SELECT name, npc_id, location_id, turn FROM defeats WHERE campaign_id=? ORDER BY ord`, func(r *sql.Rows) error {
			var d game.DefeatRecord
			if err := r.Scan(&d.Name, &d.NPCID, &d.LocationID, &d.Turn); err != nil {
				return err
			}
			w.Defeats = append(w.Defeats, d)
			return nil
		}),
	}
	if err := errors.Join(steps...); err != nil {
		return nil, fmt.Errorf("load %s: %w", id, err)
	}
	w.RefreshMagic()
	return w, nil
}

// CampaignSummary is a row of the campaign list.
type CampaignSummary struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Pitch      string `json:"pitch,omitempty"`
	Status     string `json:"status"`
	Players    int    `json:"players"`
	MaxPlayers int    `json:"max_players"`
	UpdatedAt  int64  `json:"updated_at"`
}

// ListCampaigns lists campaigns, newest first. If playerID is set, only
// campaigns that player has joined.
func (s *Store) ListCampaigns(ctx context.Context, playerID string) ([]CampaignSummary, error) {
	query := `SELECT c.id, c.title, c.pitch, c.status, c.max_players, c.updated_at,
		(SELECT COUNT(*) FROM players p WHERE p.campaign_id = c.id) FROM campaigns c`
	var args []any
	if playerID != "" {
		query += ` WHERE c.id IN (SELECT campaign_id FROM players WHERE id = ?)`
		args = append(args, playerID)
	}
	query += ` ORDER BY c.updated_at DESC, c.id`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CampaignSummary{}
	for rows.Next() {
		var cs CampaignSummary
		if err := rows.Scan(&cs.ID, &cs.Title, &cs.Pitch, &cs.Status, &cs.MaxPlayers, &cs.UpdatedAt, &cs.Players); err != nil {
			return nil, err
		}
		out = append(out, cs)
	}
	return out, rows.Err()
}

// Events returns a campaign's events with id > after, oldest first.
func (s *Store) Events(ctx context.Context, campaignID string, after int64, limit int) ([]Event, error) {
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, campaign_id, type, actor, data, created_at FROM events
		WHERE campaign_id = ? AND id > ? AND undone = 0 ORDER BY id LIMIT ?`, campaignID, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		var data string
		if err := rows.Scan(&e.ID, &e.CampaignID, &e.Type, &e.Actor, &data, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.Data = json.RawMessage(data)
		out = append(out, e)
	}
	return out, rows.Err()
}

// RecentEvents returns the last n events of the given types (all if none),
// oldest first. Used to build compact LLM context.
func (s *Store) RecentEvents(ctx context.Context, campaignID string, n int, types ...string) ([]Event, error) {
	query := `SELECT id, campaign_id, type, actor, data, created_at FROM events WHERE campaign_id = ? AND undone = 0`
	args := []any{campaignID}
	if len(types) > 0 {
		query += ` AND type IN (` + strings.TrimSuffix(strings.Repeat("?,", len(types)), ",") + `)`
		for _, t := range types {
			args = append(args, t)
		}
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, n)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var data string
		if err := rows.Scan(&e.ID, &e.CampaignID, &e.Type, &e.Actor, &data, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.Data = json.RawMessage(data)
		out = append([]Event{e}, out...)
	}
	return out, rows.Err()
}
