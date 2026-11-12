// Package launch runs launcher mode (the Steam shortcut): self-update, sync,
// start the game. It never shows a window unless there is work to do, and it
// never uses joke mode.
package launch

import (
	"context"
	"errors"
	"os"
	"path/filepath"

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
	gameExe := filepath.Join(st.ModdedDir, steam.GameExe)

	// Started again while playing: don't touch files in use, don't start twice.
	if d.P.ProcessRunning(gameExe) {
		d.Log.Info("modded game already running")
		return nil
	}

	if err := update(ctx, d, dataDir, st, currentVersion); err != nil {
		if errors.Is(err, errRestarted) {
			return nil
		}
		return err
	}

	if _, err := os.Stat(gameExe); err != nil {
		return notInstalled(err)
	}
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

// update fetches the manifest and brings everything up to date. Network
// problems are logged and skipped so the game still starts offline.
func update(ctx context.Context, d Deps, dataDir string, st *syncer.State, currentVersion string) error {
	m, _, err := install.LoadManifest(ctx, d, dataDir)
	if err != nil {
		d.Log.Warn("no manifest, launching with what is installed", "err", err)
		return nil
	}
	cache := &download.Cache{Dir: filepath.Join(dataDir, "cache"), HTTP: d.HTTP}

	if selfupdate.Needed(currentVersion, m) {
		d.Log.Info("self-update", "from", currentVersion, "to", m.Launcher.Version)
		if err := selfupdate.Apply(ctx, d.SelfPath, cache, m.Launcher); err != nil {
			d.Log.Warn("self-update failed, continuing with current version", "err", err)
		} else if _, err := d.P.StartProcess(d.SelfPath, filepath.Dir(d.SelfPath), "--launch"); err != nil {
			d.Log.Warn("restart after self-update failed", "err", err)
		} else {
			return errRestarted
		}
	}

	root, err := d.P.SteamRoot()
	if err != nil {
		d.Log.Warn("steam not found, skipping sync", "err", err)
		return nil
	}
	g, err := steam.FindGame(root)
	if err != nil {
		d.Log.Warn("game not found, skipping sync", "err", err)
		return nil
	}
	plan, err := syncer.MakePlan(st, g, m)
	if err != nil {
		d.Log.Warn("plan", "err", err)
		return nil
	}
	if plan.Empty() {
		return nil
	}
	d.Log.Info("sync", "rebuild", plan.Rebuild, "reason", plan.Reason, "fetch", len(plan.PluginsToFetch), "delete", len(plan.FilesToDelete))
	tr := progress.NewTracker()
	d.UI.Progress("Among Us Modded wird aktualisiert", tr, ui.Display{})
	s := &syncer.Syncer{Cache: cache, Tracker: tr, Log: d.Log}
	newState, err := s.Apply(ctx, plan, g, m, dataDir)
	tr.Finish()
	if errors.Is(err, syncer.ErrModdedInUse) {
		return apperr.New("Among Us Modded konnte nicht aktualisiert werden, weil Dateien noch benutzt werden. Schließe das Spiel und versuche es erneut.", err)
	}
	if err != nil {
		d.Log.Error("sync failed, launching with what is installed", "err", err)
		return nil
	}
	if err := syncer.SaveState(dataDir, newState); err != nil {
		d.Log.Error("save state", "err", err)
	}
	keep := []string{m.Modpack.SHA256, m.Launcher.SHA256}
	for _, p := range m.Plugins {
		keep = append(keep, p.SHA256)
	}
	if err := cache.Prune(keep); err != nil {
		d.Log.Warn("prune cache", "err", err)
	}
	return nil
}
