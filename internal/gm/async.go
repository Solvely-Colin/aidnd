package gm

import (
	"context"
	"time"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/store"
)

// Async play: in a long campaign (a saga) a fight can wait for a player
// for hours. Once a turn has waited TurnDeadline, autopilot plays it for
// them: a plain strike at the weakest foe in reach, or a dodge, and a death
// save rolled by the engine. Sweep runs on a timer and when the Worker's
// cron wakes the server.

// DefaultTurnDeadline is how long an async fight waits for a player.
const DefaultTurnDeadline = 12 * time.Hour

// TurnDeadline is the unix time autopilot takes the current turn of the
// focused group, or 0 when nothing is waiting on a deadline (one-shots,
// and groups of one).
func (g *GM) TurnDeadline(w *game.World) int64 {
	if !w.Campaign.LongForm() || w.Campaign.Status != game.StatusActive || !w.InCombat() || w.ActiveCharacter() == nil {
		return 0
	}
	// a group of one waits on nobody, so its fights have no deadline
	if len(w.Party()) < 2 {
		return 0
	}
	return w.Campaign.Turns.Started + int64(g.Cfg.TurnDeadline/time.Second)
}

// Sweep runs the timers of every long campaign: the world ticks it is owed
// (see worldTicks), then every fight turn past its deadline, in every
// group. It returns how many turns autopilot took.
func (g *GM) Sweep(ctx context.Context) (int, error) {
	list, err := g.Store.ListCampaigns(ctx, "")
	if err != nil {
		return 0, err
	}
	n := 0
	for _, cs := range list {
		if cs.Status != game.StatusActive {
			continue
		}
		w, err := g.Store.Load(ctx, cs.ID)
		if err != nil || !w.Campaign.LongForm() {
			continue
		}
		if _, err := g.worldTicks(ctx, cs.ID); err != nil {
			g.logf("world tick %s: %v", cs.ID, err)
		}
		for _, grp := range w.GroupIDs() {
			// a few in a row, in case the server slept through several
			for i := 0; i < 8; i++ {
				acted, err := g.autopilot(ctx, cs.ID, grp)
				if err != nil {
					g.logf("autopilot %s/%s: %v", cs.ID, grp, err)
				}
				if !acted {
					break
				}
				n++
			}
		}
	}
	return n, nil
}

// autopilot plays the group's current turn if it is overdue.
func (g *GM) autopilot(ctx context.Context, campaignID, group string) (bool, error) {
	defer g.lock(campaignID)()
	w, err := g.Store.Load(ctx, campaignID)
	if err != nil {
		return false, err
	}
	if w.Group(group) == nil && group != game.MainGroup {
		return false, nil
	}
	if err := w.Focus(group); err != nil {
		return false, err
	}
	dl := g.TurnDeadline(w)
	if dl == 0 || g.Now().Unix() < dl {
		return false, nil
	}
	actor := w.ActiveCharacter()
	if p := w.Campaign.TakePending(actor.ID); p != nil && p.Kind == game.PendingDeathSave || actor.Dying() {
		out := &TurnResult{TurnEnded: true}
		ds := game.DeathSave(w, g.Dice, actor)
		evs := []store.Event{deathSaveEvent(out, ds)}
		if ds.Revived {
			evs = append(evs, store.NewEvent("narration", "", map[string]any{"text": ds.Line + ".", "character_id": actor.ID}))
		}
		evs = append(evs, g.endTurn(ctx, w, out)...)
		if w.Campaign.Status == game.StatusFailed {
			evs = append(evs, store.NewEvent("campaign_ended", "", map[string]any{"status": w.Campaign.Status}))
		}
		_, err := g.commit(ctx, w, evs...)
		return err == nil, err
	}
	actions := append(game.AutopilotActions(w, actor), game.Action{Kind: game.ActEndTurn})
	text := "(away) " + actor.Name + " fights on by instinct."
	if _, err := g.resolveTurn(ctx, w, actor, actor.PlayerID, text, Hints{Autopilot: true}, actions, g.recent(ctx, w)); err != nil {
		return false, err
	}
	return true, nil
}
