// Package launch runs launcher mode (the Steam shortcut): self-update, sync,
// start the game. It never shows a window unless there is work to do, and it
// never uses joke mode.
package launch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/Kiirb/CleanInstaller/internal/apperr"
	"github.com/Kiirb/CleanInstaller/internal/download"
	"github.com/Kiirb/CleanInstaller/internal/install"
	"github.com/Kiirb/CleanInstaller/internal/progress"
	"github.com/Kiirb/CleanInstaller/internal/selfupdate"
	"github.com/Kiirb/CleanInstaller/internal/steam"
	"github.com/Kiirb/CleanInstaller/internal/syncer"
	"github.com/Kiirb/CleanInstaller/internal/ui"
)

// Deps is shared with the installer.
type Deps = install.Deps

var errNotInstalled = errors.New("not installed")

func notInstalled(err error) error {
	return apperr.New("Among Us Modded ist nicht installiert. Bitte führe den CleanInstaller erneut aus.", err)
}

// Run executes launcher mode.
func Run(ctx context.Context, d Deps, currentVersion string) error {
	if err := selfupdate.CleanupOld(d.SelfPath); err != nil {
		d.Log.Warn("cleanup old exe", "err", err)
	}
	dataDir, err := d.P.DataDir()
	if err != nil {
		return apperr.New("Der Programmordner konnte nicht gelesen werden.", err)
	}
	st, err := syncer.LoadState(dataDir)
	if err != nil || st == nil {
		return notInstalled(errors.Join(errNotInstalled, err))
	}

	// Started again while playing: don't touch files in use, don't start twice.
	if d.P.ProcessRunning(filepath.Join(st.ModdedDir, steam.GameExe)) {
		d.Log.Info("modded game already running")
		return nil
	}
	// Play pressed twice (or the installer is open): the other instance
	// handles it, so exit quietly.
	rel, ok := d.P.Lock()
	if !ok {
		d.Log.Info("another CleanInstaller instance is running")
		return nil
	}
	var once sync.Once
	release := func() { once.Do(rel) }
	defer release()

	st, err = update(ctx, d, dataDir, st, currentVersion, release)
	if errors.Is(err, errRestarted) {
		return nil
	}
	if err != nil {
		return err
	}

	// The folder may have moved during the sync (Steam library change).
	gameExe := filepath.Join(st.ModdedDir, steam.GameExe)
	if _, err := os.Stat(gameExe); err != nil {
		return notInstalled(err)
	}
	refreshArtwork(d, dataDir)
	d.UI.Close()
	proc, err := d.P.StartProcess(gameExe, st.ModdedDir)
	if err != nil {
		return apperr.New("Among Us Modded konnte nicht gestartet werden.", err)
	}
	if proc != nil {
		// Keep running while the game runs so Steam shows the user as playing.
		if _, err := proc.Wait(); err != nil {
			d.Log.Warn("wait for game", "err", err)
		}
	}
	return nil
}

var errRestarted = errors.New("restarted after self-update")

// refreshArtwork rewrites the Steam library images from the ones built into
// this exe, for every account that has our shortcut, so new images reach
// friends with the next release. Best effort: failures are only logged.
func refreshArtwork(d Deps, dataDir string) {
	if d.Art == nil {
		return
	}
	root, err := d.P.SteamRoot()
	if err != nil {
		d.Log.Warn("artwork: steam not found", "err", err)
		return
	}
	users, err := steam.Users(root)
	if err != nil {
		d.Log.Warn("artwork: list users", "err", err)
		return
	}
	exe := filepath.Join(dataDir, install.ExeName)
	for _, u := range users {
		cfg := steam.UserConfigDir(root, u)
		sc, err := steam.LoadShortcuts(cfg)
		if err != nil {
			d.Log.Warn("artwork: read shortcuts", "user", u.ID3, "err", err)
			continue
		}
		for _, id := range steam.OurShortcutAppIDs(sc, exe, dataDir) {
			n, err := steam.SyncArtwork(cfg, id, d.Art)
			if err != nil {
				d.Log.Warn("artwork: write", "user", u.ID3, "err", err)
			} else if n > 0 {
				d.Log.Info("artwork refreshed", "user", u.ID3, "files", n)
			}
		}
	}
}

// update fetches the manifest and brings everything up to date, returning the
// state to launch from. Network problems are logged and skipped so the game
// still starts offline. release frees the instance lock before a restart.
func update(ctx context.Context, d Deps, dataDir string, st *syncer.State, currentVersion string, release func()) (*syncer.State, error) {
	m, _, err := install.LoadManifest(ctx, d, dataDir)
	if err != nil {
		d.Log.Warn("no manifest, launching with what is installed", "err", err)
		return st, nil
	}
	cache := &download.Cache{Dir: filepath.Join(dataDir, "cache"), HTTP: d.HTTP}

	if selfupdate.Needed(currentVersion, m) {
		d.Log.Info("self-update", "from", currentVersion, "to", m.Launcher.Version)
		if err := selfupdate.Apply(ctx, d.SelfPath, cache, m.Launcher); err != nil {
			d.Log.Warn("self-update failed, continuing with current version", "err", err)
		} else {
			release() // the new launcher must be able to take the lock
			if _, err := d.P.StartProcess(d.SelfPath, filepath.Dir(d.SelfPath), "--launch"); err != nil {
				d.Log.Warn("restart after self-update failed", "err", err)
			} else {
				return st, errRestarted
			}
		}
	}

	root, err := d.P.SteamRoot()
	if err != nil {
		d.Log.Warn("steam not found, skipping sync", "err", err)
		return st, nil
	}
	g, err := steam.FindGame(root)
	if err != nil {
		d.Log.Warn("game not found, skipping sync", "err", err)
		return st, nil
	}
	plan, err := syncer.MakePlan(st, g, m)
	if err != nil {
		d.Log.Warn("plan", "err", err)
		return st, nil
	}
	if plan.Empty() {
		return st, nil
	}
	d.Log.Info("sync", "rebuild", plan.Rebuild, "reason", plan.Reason, "fetch", len(plan.PluginsToFetch), "delete", len(plan.FilesToDelete))
	tr := progress.NewTracker()
	d.UI.Progress("Among Us Modded wird aktualisiert", tr, ui.Display{})
	s := &syncer.Syncer{Cache: cache, Tracker: tr, Log: d.Log}
	newState, err := s.Apply(ctx, plan, g, m, dataDir)
	tr.Finish()
	if errors.Is(err, syncer.ErrModdedInUse) {
		return nil, apperr.New("Among Us Modded konnte nicht aktualisiert werden, weil Dateien noch benutzt werden. Schließe das Spiel und versuche es erneut.", err)
	}
	if err != nil {
		d.Log.Error("sync failed, launching with what is installed", "err", err)
		return st, nil
	}
	if err := syncer.SaveState(dataDir, newState); err != nil {
		d.Log.Error("save state", "err", err)
	}
	keep := []string{m.Modpack.SHA256}
	for _, p := range m.Plugins {
		keep = append(keep, p.SHA256)
	}
	if err := cache.Prune(keep); err != nil {
		d.Log.Warn("prune cache", "err", err)
	}
	return newState, nil
}
