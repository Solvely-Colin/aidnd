package gm

import (
	"context"
	"fmt"
	"strings"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/llm"
	"github.com/Solvely-Colin/aidnd/internal/saga"
	"github.com/Solvely-Colin/aidnd/internal/store"
)

// MaxCatchUpTicks caps the world ticks run at once after the server slept.
const MaxCatchUpTicks = 7

// worldTicks runs the world ticks a long campaign is owed. It returns how
// many ran.
func (g *GM) worldTicks(ctx context.Context, campaignID string) (int, error) {
	defer g.lock(campaignID)()
	w, err := g.Store.Load(ctx, campaignID)
	if err != nil {
		return 0, err
	}
	if !w.Campaign.LongForm() || w.Campaign.Status != game.StatusActive {
		return 0, nil
	}
	changed := saga.EnsureLiving(w)
	changed = saga.EnsureAgendas(w) || changed
	lv := w.Campaign.Living
	if lv == nil {
		return 0, nil
	}
	now := g.Now().Unix()
	started := lv.LastTickDay == 0
	due := game.TicksDue(w, now, MaxCatchUpTicks)
	if due == 0 {
		if changed || started {
			_, err := g.commit(ctx, w)
			return 0, err
		}
		return 0, nil
	}
	// players who have never caught up start from here, so the first tick
	// they miss reaches them as a dispatch
	for _, p := range w.Players {
		if lv.Seen[p.ID] == nil {
			game.NoteSeen(w, p.ID)
		}
	}
	var evs []store.Event
	for i := 0; i < due; i++ {
		idle := lv.LastActed <= lv.LastTick
		rep := game.WorldTick(w, g.Dice, now, idle)
		lv.LastTickDay++
		gz := g.gazette(ctx, w, rep)
		lv.Gazette = gz
		for _, d := range rep.Downtime {
			party := game.GroupOf(w.Character(d.CharacterID))
			evs = append(evs, tagParty(store.NewEvent("downtime_done", "", d), party))
		}
		evs = append(evs, store.NewEvent("world_tick", "", map[string]any{
			"week": rep.Week, "tide": rep.Tide, "tide_from": rep.TideFrom, "band": game.TideBand(rep.Tide), "idle": rep.Idle, "gazette": gz,
			"people": len(rep.People)}))
	}
	if _, err := g.commit(ctx, w, evs...); err != nil {
		return 0, err
	}
	return due, nil
}

const gazetteSystem = `You write the front page of the Arkham Advertiser, a 1930s New England newspaper, for a horror campaign.
From this week's facts, write:
HEADLINE: (3 or 4 lines, one per fact or two, each under 14 words, in period newspaper style)
CLASSIFIED: (one small ad that is really a hook for adventurers, under 30 words)
WEATHER: (one line)
Only report what the facts say, as a newspaper would see it from outside: never explain the supernatural, never name a secret, never invent deaths of named people.`

// gazette writes the week's front page from a tick's facts. Without the
// model, the facts are the headlines.
func (g *GM) gazette(ctx context.Context, w *game.World, rep *game.TickReport) *game.Gazette {
	gz := &game.Gazette{Week: rep.Week}
	facts := rep.Facts
	if len(facts) == 0 {
		facts = []string{"A quiet week on the North Shore."}
	}
	resp, err := g.LLM.Chat(ctx, llm.Request{Tag: "gazette", Temperature: 0.8, MaxTokens: 400, Messages: []llm.Message{
		llm.System(gazetteSystem),
		llm.User(fmt.Sprintf("SETTING: %s\nWEEK %d. THIS WEEK'S FACTS:\n- %s", w.Campaign.Premise, rep.Week, strings.Join(facts, "\n- ")))}})
	if err == nil {
		for _, line := range strings.Split(resp.Content, "\n") {
			line = strings.TrimSpace(strings.TrimLeft(line, "-*• "))
			k, v, ok := strings.Cut(line, ":")
			v = strings.TrimSpace(v)
			if !ok || v == "" {
				continue
			}
			switch strings.ToUpper(strings.TrimSpace(k)) {
			case "HEADLINE", "HEADLINES":
				if len(gz.Headlines) < 4 {
					gz.Headlines = append(gz.Headlines, trim(v, 140))
				}
			case "CLASSIFIED":
				gz.Classified = trim(v, 240)
			case "WEATHER":
				gz.Weather = trim(v, 120)
			}
		}
	}
	// the paper is model prose every player reads: a headline or ad that
	// kills the living or raises the dead is dropped
	c := newCanon(w, "", nil)
	c.lifeOnly = true
	var heads []string
	for _, h := range gz.Headlines {
		if is := c.check(h); len(is) == 0 {
			heads = append(heads, h)
		} else {
			g.debugf("gazette canon issue: %s", issueList(is))
		}
	}
	gz.Headlines = heads
	if gz.Classified != "" && len(c.check(gz.Classified)) > 0 {
		gz.Classified = ""
	}
	if len(gz.Headlines) == 0 {
		for _, f := range facts {
			if len(gz.Headlines) < 4 {
				gz.Headlines = append(gz.Headlines, f)
			}
		}
	}
	if gz.Weather == "" {
		gz.Weather = weatherLines[rep.Week%len(weatherLines)]
	}
	return gz
}

var weatherLines = []string{
	"Fog off the water by morning, clearing by noon.",
	"Rain and a raw east wind; small craft advisories along the coast.",
	"Fair and cool. A hard frost is possible inland.",
	"Overcast, with a heavy swell running on the outer reefs.",
	"Warm and close, thunder in the hills by evening.",
}

// MarkSeen records that the player has read what they missed.
func (g *GM) MarkSeen(ctx context.Context, campaignID, playerID string) (*game.World, error) {
	defer g.lock(campaignID)()
	w, err := g.Store.Load(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if w.Player(playerID) == nil {
		return nil, ErrForbidden
	}
	if err := w.FocusPlayer(playerID); err != nil {
		return nil, err
	}
	if w.Campaign.Living == nil {
		return w, nil
	}
	game.NoteSeen(w, playerID)
	if _, err := g.commit(ctx, w); err != nil {
		return nil, err
	}
	return w, nil
}

// SetDowntime queues the player's downtime order for the next world tick;
// kind "" cancels it.
func (g *GM) SetDowntime(ctx context.Context, campaignID, playerID, kind string) (*game.World, error) {
	defer g.lock(campaignID)()
	w, err := g.Store.Load(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if err := w.FocusPlayer(playerID); err != nil {
		return nil, err
	}
	if w.Campaign.Status != game.StatusActive {
		return nil, Conflict("the campaign is %s", w.Campaign.Status)
	}
	c := w.CharacterOf(playerID)
	if c == nil {
		return nil, ErrForbidden
	}
	if kind != "" {
		if w.InCombat() {
			return nil, Conflict("not in the middle of a fight")
		}
		if err := game.CanDowntime(w, c, w.Here()); err != nil {
			return nil, Conflict("%s", err.Error())
		}
	}
	if err := game.QueueDowntime(w, c, kind); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadRequest, err)
	}
	if _, err := g.commit(ctx, w); err != nil {
		return nil, err
	}
	return w, nil
}
