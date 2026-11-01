package gm

import (
	"context"
	"errors"
	"fmt"

	"github.com/Solvely-Colin/aidnd/internal/game"
	"github.com/Solvely-Colin/aidnd/internal/saga"
	"github.com/Solvely-Colin/aidnd/internal/store"
)

// GM tools for the person running the table: restart a story from the top,
// delete one, bring a deleted one back, and free a seat in a lobby. Every
// restart and delete saves a backup first.

// Lock holds a campaign's lock for writes made outside the GM, such as
// player notes, so they cannot slip between a backup and a delete.
func (g *GM) Lock(id string) func() { return g.lock(id) }

// saveOrigin keeps a newly written campaign so it can be restarted later.
// A failure only costs the restart option, so it is logged, not returned.
func (g *GM) saveOrigin(ctx context.Context, w *game.World) {
	if err := g.Store.SaveOrigin(ctx, w); err != nil {
		g.logf("save origin %s: %v", w.Campaign.ID, err)
	}
}

// CanRestart reports whether a campaign has a start to go back to.
func (g *GM) CanRestart(ctx context.Context, w *game.World) bool {
	return w.Campaign.Saga != "" && saga.Get(w.Campaign.Saga) != nil || g.Store.HasOrigin(ctx, w.Campaign.ID)
}

// Restart begins a campaign again from how it was written. The same
// players keep their seats in a fresh lobby and make new characters; the
// old campaign is backed up and removed. The restarted story gets a new id,
// so nothing cached for the old one (art, open event streams) carries over.
func (g *GM) Restart(ctx context.Context, id string) (*game.World, int64, error) {
	defer g.lock(id)()
	old, err := g.Store.Load(ctx, id)
	if err != nil {
		return nil, 0, err
	}
	newID := g.NewID()
	fresh, err := g.Store.Origin(ctx, id)
	switch {
	case err == nil:
	case errors.Is(err, store.ErrNotFound) && old.Campaign.Saga != "":
		s := saga.Get(old.Campaign.Saga)
		if s == nil {
			return nil, 0, Conflict("the saga %q is no longer in the game", old.Campaign.Saga)
		}
		if fresh, err = saga.Build(s, newID, saga.Options{MaxSetbacks: g.Cfg.MaxSetbacks, Now: g.Now().Unix()}); err != nil {
			return nil, 0, err
		}
	case errors.Is(err, store.ErrNotFound):
		return nil, 0, Conflict("this story was written before restarts were kept, so it can only be deleted")
	default:
		return nil, 0, err
	}
	fresh.Campaign.ID = newID
	fresh.Campaign.Status = game.StatusLobby
	fresh.Campaign.CreatedAt = g.Now().Unix()
	fresh.Campaign.MaxPlayers = 0
	// choices made at the table carry over
	fresh.Campaign.Lethality = old.Campaign.Lethality
	fresh.Campaign.Async = old.Campaign.Async
	fresh.Players = nil
	fresh.Characters = nil
	for _, p := range old.Players {
		fresh.Players = append(fresh.Players, &game.Player{ID: p.ID, Name: p.Name, Token: randomID() + randomID(), AutoRoll: p.AutoRoll})
	}

	backup, err := g.backup(ctx, id, "restart")
	if err != nil {
		return nil, 0, err
	}
	defer g.lock(newID)()
	if _, err := g.commit(ctx, fresh, store.NewEvent("campaign_created", "", map[string]any{
		"id": newID, "title": fresh.Campaign.Title, "pitch": fresh.Campaign.Pitch, "saga": fresh.Campaign.Saga, "restarted_from": id})); err != nil {
		return nil, 0, err
	}
	// the new campaign's start is the same written world, without seats
	seated := fresh.Players
	fresh.Players = nil
	g.saveOrigin(ctx, fresh)
	fresh.Players = seated
	if err := g.Store.DeleteCampaign(ctx, id); err != nil {
		return nil, 0, fmt.Errorf("restarted as %s, but the old copy could not be removed: %w", newID, err)
	}
	g.publishGone(id, "restarted", newID)
	w, err := g.Store.Load(ctx, newID)
	return w, backup, err
}

// Delete backs a campaign up and removes it.
func (g *GM) Delete(ctx context.Context, id string) (int64, error) {
	defer g.lock(id)()
	backup, err := g.backup(ctx, id, "delete")
	if err != nil {
		return 0, err
	}
	if err := g.Store.DeleteCampaign(ctx, id); err != nil {
		return 0, err
	}
	g.publishGone(id, "deleted", "")
	return backup, nil
}

// RestoreBackup puts a backed-up campaign back as it was.
func (g *GM) RestoreBackup(ctx context.Context, backupID int64) (string, error) {
	d, err := g.Store.LoadBackup(ctx, backupID)
	if err != nil {
		return "", err
	}
	defer g.lock(d.CampaignID)()
	if err := g.Store.RestoreDump(ctx, d); err != nil {
		if errors.Is(err, store.ErrExists) {
			return "", Conflict("that campaign is still here; delete it first to bring the backup back")
		}
		return "", err
	}
	return d.CampaignID, nil
}

// RemovePlayer frees a seat in a lobby, with the character made for it.
// Once a story is under way a character is part of it, so it stays.
func (g *GM) RemovePlayer(ctx context.Context, campaignID, playerID string) error {
	defer g.lock(campaignID)()
	w, err := g.Store.Load(ctx, campaignID)
	if err != nil {
		return err
	}
	if w.Player(playerID) == nil {
		return store.ErrNotFound
	}
	if w.Campaign.Status != game.StatusLobby {
		return Conflict("players can only be removed before the story starts")
	}
	if c := w.CharacterOf(playerID); c != nil {
		removeCharacter(w, c.ID)
	}
	for i, p := range w.Players {
		if p.ID == playerID {
			w.Players = append(w.Players[:i], w.Players[i+1:]...)
			break
		}
	}
	_, err = g.commit(ctx, w, store.NewEvent("player_left", playerID, map[string]any{"player_id": playerID, "by": "gm"}))
	return err
}

func (g *GM) backup(ctx context.Context, id, reason string) (int64, error) {
	d, err := g.Store.DumpCampaign(ctx, id)
	if err != nil {
		return 0, err
	}
	return g.Store.SaveBackup(ctx, d, reason)
}

// publishGone tells open tabs the campaign is gone. The event is not
// saved: there is no campaign left to save it in.
func (g *GM) publishGone(id, how, next string) {
	if g.Publish == nil {
		return
	}
	data := map[string]any{"how": how}
	if next != "" {
		data["next"] = next
	}
	e := store.NewEvent("campaign_gone", "", data)
	e.CampaignID = id
	g.Publish(id, []store.Event{e})
}
