package syncer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/Kiirb/CleanInstaller/internal/download"
	"github.com/Kiirb/CleanInstaller/internal/fsutil"
	"github.com/Kiirb/CleanInstaller/internal/manifest"
	"github.com/Kiirb/CleanInstaller/internal/progress"
	"github.com/Kiirb/CleanInstaller/internal/steam"
)

// ErrModdedInUse means the modded folder could not be replaced because files
// in it are open (game running, antivirus scan). The old folder is intact.
var ErrModdedInUse = errors.New("modded game is in use")

// ErrNoModpack means the manifest has no mod pack yet.
var ErrNoModpack = errors.New("manifest has no modpack")

const pluginsRel = "BepInEx/plugins/"

// Plan is what Apply will do.
type Plan struct {
	Rebuild        bool
	Reason         string
	PluginsToFetch []manifest.Asset
	FilesToDelete  []string // absolute paths
}

// Empty reports whether nothing needs doing.
func (p *Plan) Empty() bool {
	return !p.Rebuild && len(p.PluginsToFetch) == 0 && len(p.FilesToDelete) == 0
}

func pluginDir(modded string) string { return filepath.Join(modded, "BepInEx", "plugins") }

// MakePlan compares state, disk and manifest.
func MakePlan(st *State, g *steam.Game, m *manifest.Manifest) (*Plan, error) {
	if m.Modpack.Empty() {
		return nil, ErrNoModpack
	}
	p := &Plan{}
	switch {
	case st == nil:
		p.Rebuild, p.Reason = true, "not installed"
	case !strings.EqualFold(filepath.Clean(st.ModdedDir), filepath.Clean(g.ModdedDir)):
		p.Rebuild, p.Reason = true, "modded folder moved"
	case !exists(filepath.Join(g.ModdedDir, steam.GameExe)):
		p.Rebuild, p.Reason = true, "modded folder missing"
	case st.BuildID != g.BuildID:
		p.Rebuild, p.Reason = true, fmt.Sprintf("game updated (%s -> %s)", st.BuildID, g.BuildID)
	case st.ModpackSHA != m.Modpack.SHA256:
		p.Rebuild, p.Reason = true, "mod pack changed"
	}
	if p.Rebuild {
		p.PluginsToFetch = append(p.PluginsToFetch, m.Plugins...)
		return p, nil
	}

	dir := pluginDir(g.ModdedDir)
	managed := map[string]bool{}
	for _, a := range m.Plugins {
		managed[strings.ToLower(a.FileName)] = true
		sum, err := download.FileSHA256(filepath.Join(dir, a.FileName))
		if err != nil || sum != a.SHA256 {
			p.PluginsToFetch = append(p.PluginsToFetch, a)
		}
	}
	for _, n := range st.PackPlugins {
		managed[strings.ToLower(n)] = true
	}
	es, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, e := range es {
		if !e.IsDir() && !managed[strings.ToLower(e.Name())] {
			p.FilesToDelete = append(p.FilesToDelete, filepath.Join(dir, e.Name()))
		}
	}
	for _, r := range m.Remove {
		if managed[strings.ToLower(strings.TrimPrefix(r, pluginsRel))] && strings.HasPrefix(r, pluginsRel) {
			continue
		}
		abs := filepath.Join(g.ModdedDir, filepath.FromSlash(r))
		if exists(abs) {
			p.FilesToDelete = append(p.FilesToDelete, abs)
		}
	}
	return p, nil
}

// Syncer applies plans.
type Syncer struct {
	Cache   *download.Cache
	Tracker *progress.Tracker
	Log     *slog.Logger
}

// Apply executes p and returns the new state (the caller saves it).
func (s *Syncer) Apply(ctx context.Context, p *Plan, g *steam.Game, m *manifest.Manifest, dataDir string) (*State, error) {
	prev, _ := LoadState(dataDir)
	if p.Rebuild {
		return s.rebuild(ctx, p, g, m)
	}
	s.Tracker.Plan(100)
	s.Tracker.Begin("Mods werden aktualisiert")
	dir := pluginDir(g.ModdedDir)
	if err := s.fetchPlugins(ctx, p.PluginsToFetch, dir); err != nil {
		return nil, err
	}
	for _, f := range p.FilesToDelete {
		s.Log.Info("delete", "path", f)
		if err := os.RemoveAll(f); err != nil {
			return nil, fmt.Errorf("delete %s: %w", f, err)
		}
	}
	s.Tracker.Set(1)
	st := &State{BuildID: g.BuildID, ModpackSHA: m.Modpack.SHA256, ModdedDir: g.ModdedDir, Plugins: pluginState(m)}
	if prev != nil {
		st.PackPlugins = prev.PackPlugins
	}
	return st, nil
}

func pluginState(m *manifest.Manifest) map[string]string {
	out := map[string]string{}
	for _, a := range m.Plugins {
		out[a.FileName] = a.SHA256
	}
	return out
}

func (s *Syncer) fetchPlugins(ctx context.Context, list []manifest.Asset, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for i, a := range list {
		s.Log.Info("plugin", "file", a.FileName, "tag", a.Tag)
		cached, err := s.Cache.Get(ctx, a, nil)
		if err != nil {
			return fmt.Errorf("plugin %s: %w", a.FileName, err)
		}
		if err := copyFile(cached, filepath.Join(dir, a.FileName)); err != nil {
			return fmt.Errorf("install plugin %s: %w", a.FileName, err)
		}
		s.Tracker.Set(float64(i+1) / float64(len(list)))
	}
	return nil
}

func (s *Syncer) rebuild(ctx context.Context, p *Plan, g *steam.Game, m *manifest.Manifest) (*State, error) {
	s.Log.Info("rebuild", "reason", p.Reason, "dir", g.ModdedDir)
	s.Tracker.Plan(25, 40, 15, 20)

	// 1. Download first: if this fails (offline), nothing on disk changed.
	s.Tracker.Begin("Mod-Paket wird heruntergeladen")
	pack, err := s.Cache.Get(ctx, m.Modpack, func(done, total int64) {
		if total > 0 {
			s.Tracker.Set(float64(done) / float64(total))
		}
	})
	if err != nil {
		return nil, fmt.Errorf("mod pack: %w", err)
	}

	newDir := g.ModdedDir + ".new"
	if err := os.RemoveAll(newDir); err != nil {
		return nil, fmt.Errorf("clean %s: %w", newDir, err)
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(newDir) // best effort; logged by caller via returned error
		}
	}()

	// 2. Copy the vanilla game.
	s.Tracker.Begin("Among Us wird kopiert")
	if err := fsutil.CopyTree(g.VanillaDir, newDir, fsutil.VanillaExcludes, func(done, total int64) {
		if total > 0 {
			s.Tracker.Set(float64(done) / float64(total))
		}
	}); err != nil {
		return nil, fmt.Errorf("copy game: %w", err)
	}

	// 3. Extract the mod pack.
	s.Tracker.Begin("Mod-Paket wird entpackt")
	files, err := fsutil.ExtractZip(pack, newDir, func(done, total int) { s.Tracker.Set(float64(done) / float64(total)) })
	if err != nil {
		return nil, fmt.Errorf("extract mod pack: %w", err)
	}
	var packPlugins []string
	for _, f := range files {
		if rest, found := strings.CutPrefix(f, pluginsRel); found && !strings.Contains(rest, "/") {
			packPlugins = append(packPlugins, strings.ToLower(rest))
		}
	}
	appid := filepath.Join(newDir, "steam_appid.txt")
	if !exists(appid) {
		if err := os.WriteFile(appid, []byte(steam.AppID), 0o644); err != nil {
			return nil, err
		}
	}

	// 4. Keep the user's settings: their config files win over the pack's.
	oldCfg := filepath.Join(g.ModdedDir, "BepInEx", "config")
	if exists(oldCfg) {
		if err := fsutil.CopyTree(oldCfg, filepath.Join(newDir, "BepInEx", "config"), nil, nil); err != nil {
			return nil, fmt.Errorf("keep config: %w", err)
		}
	}

	// 5. Plugins and remove list.
	s.Tracker.Begin("Mods werden installiert")
	if err := s.fetchPlugins(ctx, p.PluginsToFetch, pluginDir(newDir)); err != nil {
		return nil, err
	}
	for _, r := range m.Remove {
		if err := os.RemoveAll(filepath.Join(newDir, filepath.FromSlash(r))); err != nil {
			return nil, fmt.Errorf("remove %s: %w", r, err)
		}
	}

	// 6. Swap. If the old folder can't be moved, it stays as it was.
	oldDir := g.ModdedDir + ".old"
	_ = os.RemoveAll(oldDir)
	if exists(g.ModdedDir) {
		if err := os.Rename(g.ModdedDir, oldDir); err != nil {
			s.Log.Warn("swap failed", "err", err)
			return nil, fmt.Errorf("%w: %v", ErrModdedInUse, err)
		}
	}
	if err := os.Rename(newDir, g.ModdedDir); err != nil {
		if exists(oldDir) {
			_ = os.Rename(oldDir, g.ModdedDir) // restore
		}
		return nil, fmt.Errorf("%w: %v", ErrModdedInUse, err)
	}
	ok = true
	if err := os.RemoveAll(oldDir); err != nil {
		s.Log.Warn("could not delete old folder", "dir", oldDir, "err", err)
	}
	s.Tracker.Set(1)
	return &State{BuildID: g.BuildID, ModpackSHA: m.Modpack.SHA256, ModdedDir: g.ModdedDir,
		Plugins: pluginState(m), PackPlugins: packPlugins}, nil
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}
