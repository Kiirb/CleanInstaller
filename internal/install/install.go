// Package install runs installer mode: install, repair, uninstall, and
// cleanup of the old CInstaller.
package install

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Kiirb/CleanInstaller/internal/apperr"
	"github.com/Kiirb/CleanInstaller/internal/download"
	"github.com/Kiirb/CleanInstaller/internal/manifest"
	"github.com/Kiirb/CleanInstaller/internal/migrate"
	"github.com/Kiirb/CleanInstaller/internal/platform"
	"github.com/Kiirb/CleanInstaller/internal/progress"
	"github.com/Kiirb/CleanInstaller/internal/steam"
	"github.com/Kiirb/CleanInstaller/internal/syncer"
	"github.com/Kiirb/CleanInstaller/internal/ui"
)

// ExeName is the installed exe's file name.
const ExeName = "CleanInstaller.exe"

const (
	setupCopyName = "CleanInstaller-setup.exe"
	manifestCache = "manifest.cache.json"
	title         = "CleanInstaller"
)

// Deps are everything the flows need from the outside world.
type Deps struct {
	P           platform.Platform
	UI          ui.UI
	HTTP        *http.Client
	ManifestURL string
	SelfPath    string // os.Executable()
	Log         *slog.Logger
	TempDir     string // os.TempDir()
	Art         fs.FS  // assets.Steam
}

// Outcome of an installer run.
type Outcome int

const (
	Installed Outcome = iota + 1
	Repaired
	Uninstalled
	Cancelled
	Relaunched
)

// Run executes installer mode.
func Run(ctx context.Context, d Deps) (Outcome, error) {
	dataDir, err := d.P.DataDir()
	if err != nil {
		return 0, apperr.New("Der Programmordner konnte nicht angelegt werden.", err)
	}

	// Never work from a folder we may delete: relaunch from a temp copy.
	if inside(d.SelfPath, dataDir) || inModdedFolder(d.SelfPath) {
		return relaunch(d)
	}

	m, live, err := LoadManifest(ctx, d, dataDir)
	if err != nil {
		return 0, err
	}
	if m.Modpack.Empty() {
		return 0, apperr.New("Die Mod-Liste ist noch nicht fertig eingerichtet. Bitte später erneut versuchen.", syncer.ErrNoModpack)
	}

	root, err := d.P.SteamRoot()
	if err != nil {
		return 0, apperr.New("Steam wurde nicht gefunden. Bitte installiere Steam und starte den Installer erneut.", err)
	}
	g, err := steam.FindGame(root)
	if errors.Is(err, steam.ErrGameNotInstalled) {
		if d.UI.Confirm(title, "Among Us ist nicht installiert. Jetzt über Steam installieren?", "Installieren", "Abbrechen") {
			if err := d.P.OpenURL("steam://install/" + steam.AppID); err != nil {
				d.Log.Warn("open steam install link", "err", err)
			}
			d.UI.Info(title, "Starte den Installer erneut, sobald Among Us installiert ist.", "OK")
		}
		return Cancelled, nil
	}
	if err != nil {
		return 0, apperr.New("Die Steam-Bibliotheken konnten nicht gelesen werden.", err)
	}
	users, err := steam.Users(root)
	if err != nil || len(users) == 0 {
		return 0, apperr.New("Kein Steam-Konto gefunden. Bitte einmal in Steam anmelden.", err)
	}
	findings, err := migrate.Detect(root, users, g.ModdedDir)
	if err != nil {
		d.Log.Warn("detect old install", "err", err) // cleanup is best effort; install can continue
	}
	st, err := syncer.LoadState(dataDir)
	if err != nil {
		d.Log.Warn("unreadable state, treating as not installed", "err", err)
		st = nil
	}

	repair := false
	if st != nil {
		i, ok := d.UI.Choose(title, "Among Us Modded ist bereits installiert. Was möchtest du tun?", []string{"Reparieren", "Deinstallieren", "Abbrechen"})
		switch {
		case !ok || i == 2:
			return Cancelled, nil
		case i == 1:
			return uninstall(ctx, d, root, users, g, dataDir, findings)
		}
		repair = true
	}

	user, ok := chooseUser(d.UI, users)
	if !ok {
		return Cancelled, nil
	}
	if !repair {
		body := "Das passiert jetzt:\n" +
			"• Erstellt den Ordner „Among Us Modded“ neben deinem Among Us (dein normales Among Us bleibt unverändert)\n" +
			"• Fügt in Steam den Eintrag „Among Us Modded“ hinzu\n" +
			"• Kopiert CleanInstaller nach %LOCALAPPDATA%\\CleanInstaller"
		if findings.Any() {
			body += "\n• Ersetzt deine alte CInstaller-Installation"
		}
		if !d.UI.Confirm(title, body, "Installieren", "Abbrechen") {
			return Cancelled, nil
		}
	}

	// Sync (always a full rebuild here: fresh install, repair or migration).
	if err := syncer.DeleteState(dataDir); err != nil {
		return 0, apperr.New("Der alte Installationsstand konnte nicht zurückgesetzt werden.", err)
	}
	display := ui.Display{}
	if f := m.Fun(); live {
		display = ui.Display{JokeText: f.JokeText, ChaosBar: f.ChaosBar, JokeLines: f.JokeLines}
	}
	if err := runSync(ctx, d, g, m, dataDir, display); err != nil {
		return 0, err
	}

	exe := filepath.Join(dataDir, ExeName)
	if !samePath(d.SelfPath, exe) {
		if err := copyFile(d.SelfPath, exe); err != nil {
			return 0, apperr.New("CleanInstaller konnte nicht nach %LOCALAPPDATA% kopiert werden.", err)
		}
	}

	ok, err = closeSteam(ctx, d, root)
	if err != nil || !ok {
		return Cancelled, err
	}
	if err := migrate.CleanSteam(root, findings, d.Log); err != nil {
		d.Log.Warn("clean old shortcuts", "err", err)
	}
	if err := addShortcut(d, root, user, exe, dataDir); err != nil {
		return 0, apperr.New("Der Steam-Eintrag konnte nicht angelegt werden.", err)
	}

	if d.UI.Info(title, "Fertig! Starte „Among Us Modded“ in Steam.", "Steam starten", "Schließen") == 0 {
		if err := d.P.StartSteam(root); err != nil {
			d.Log.Warn("start steam", "err", err)
		}
	}
	if repair {
		return Repaired, nil
	}
	return Installed, nil
}

// LoadManifest fetches the live manifest (caching it) or falls back to the
// cached copy. live reports whether it came from the network in this run.
func LoadManifest(ctx context.Context, d Deps, dataDir string) (m *manifest.Manifest, live bool, err error) {
	cache := filepath.Join(dataDir, manifestCache)
	b, err := download.FetchManifest(ctx, d.HTTP, d.ManifestURL)
	if err == nil {
		if m, err = manifest.Parse(b); err == nil {
			if werr := os.WriteFile(cache, b, 0o644); werr != nil {
				d.Log.Warn("cache manifest", "err", werr)
			}
			return m, true, nil
		}
	}
	d.Log.Warn("live manifest unavailable, trying cache", "err", err)
	if cb, cerr := os.ReadFile(cache); cerr == nil {
		if m, cerr := manifest.Parse(cb); cerr == nil {
			return m, false, nil
		}
	}
	return nil, false, apperr.New("Keine Internetverbindung oder GitHub nicht erreichbar. Bitte später erneut versuchen.", err)
}

func runSync(ctx context.Context, d Deps, g *steam.Game, m *manifest.Manifest, dataDir string, display ui.Display) error {
	plan, err := syncer.MakePlan(nil, g, m)
	if err != nil {
		return apperr.New("Die Installation konnte nicht vorbereitet werden.", err)
	}
	tr := progress.NewTracker()
	d.UI.Progress("Among Us Modded wird installiert", tr, display)
	s := &syncer.Syncer{Cache: &download.Cache{Dir: filepath.Join(dataDir, "cache"), HTTP: d.HTTP}, Tracker: tr, Log: d.Log}
	st, err := s.Apply(ctx, plan, g, m, dataDir)
	tr.Finish()
	if errors.Is(err, syncer.ErrModdedInUse) {
		return apperr.New("„Among Us Modded“ wird gerade benutzt. Schließe das Spiel und versuche es erneut.", err)
	}
	if err != nil {
		return apperr.New("Die Installation ist fehlgeschlagen. Möglicherweise hat ein Virenscanner eine Datei entfernt, oder die Internetverbindung wurde unterbrochen.", err)
	}
	if err := syncer.SaveState(dataDir, st); err != nil {
		return apperr.New("Der Installationsstand konnte nicht gespeichert werden.", err)
	}
	keep := []string{m.Modpack.SHA256}
	for _, p := range m.Plugins {
		keep = append(keep, p.SHA256)
	}
	if err := s.Cache.Prune(keep); err != nil {
		d.Log.Warn("prune cache", "err", err)
	}
	return nil
}

func chooseUser(u ui.UI, users []steam.User) (steam.User, bool) {
	if len(users) == 1 {
		return users[0], true
	}
	names := make([]string, len(users))
	for i, us := range users {
		names[i] = us.PersonaName
	}
	i, ok := u.Choose("Steam-Konto wählen", "Für welches Steam-Konto soll der Eintrag „Among Us Modded“ angelegt werden?", names)
	if !ok || i < 0 || i >= len(users) {
		return steam.User{}, false
	}
	return users[i], true
}

// closeSteam asks to close Steam if it runs. ok=false means the user declined.
func closeSteam(ctx context.Context, d Deps, root string) (bool, error) {
	if !d.P.SteamRunning() {
		return true, nil
	}
	if !d.UI.Confirm(title, "Steam muss kurz geschlossen werden, um den Eintrag zu ändern. Jetzt schließen?", "Steam schließen", "Abbrechen") {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if err := d.P.ShutdownSteam(ctx, root); err != nil {
		return false, apperr.New("Steam konnte nicht geschlossen werden. Bitte schließe Steam selbst und starte den Installer erneut.", err)
	}
	return true, nil
}

func addShortcut(d Deps, root string, u steam.User, exe, startDir string) error {
	cfg := steam.UserConfigDir(root, u)
	sc, err := steam.LoadShortcuts(cfg)
	if err != nil {
		return err
	}
	steam.RemoveShortcuts(sc, steam.OurShortcutMatcher(exe, startDir))
	id := steam.ShortcutAppID(`"`+exe+`"`, steam.ShortcutName)
	steam.AddShortcut(sc, steam.Shortcut{Exe: exe, StartDir: startDir, LaunchOptions: "--launch", Icon: steam.IconPath(cfg, id)})
	if err := steam.WriteArtwork(cfg, id, d.Art); err != nil {
		return err
	}
	return steam.SaveShortcuts(cfg, sc)
}

func uninstall(ctx context.Context, d Deps, root string, users []steam.User, g *steam.Game, dataDir string, findings migrate.Findings) (Outcome, error) {
	ok, err := closeSteam(ctx, d, root)
	if err != nil || !ok {
		return Cancelled, err
	}
	exe := filepath.Join(dataDir, ExeName)
	id := steam.ShortcutAppID(`"`+exe+`"`, steam.ShortcutName)
	for _, u := range users {
		cfg := steam.UserConfigDir(root, u)
		sc, err := steam.LoadShortcuts(cfg)
		if err != nil {
			return 0, apperr.New("Der Steam-Eintrag konnte nicht entfernt werden.", err)
		}
		if steam.RemoveShortcuts(sc, steam.OurShortcutMatcher(exe, dataDir)) > 0 {
			if err := steam.SaveShortcuts(cfg, sc); err != nil {
				return 0, apperr.New("Der Steam-Eintrag konnte nicht entfernt werden.", err)
			}
		}
		if err := steam.RemoveArtwork(cfg, fmt.Sprint(id)); err != nil {
			d.Log.Warn("remove artwork", "user", u.ID3, "err", err)
		}
	}
	if err := migrate.CleanSteam(root, findings, d.Log); err != nil {
		d.Log.Warn("clean old shortcuts", "err", err)
	}
	if err := os.RemoveAll(g.ModdedDir); err != nil {
		return 0, apperr.New("„Among Us Modded“ konnte nicht gelöscht werden. Ist das Spiel noch geöffnet?", err)
	}
	// Remove our folder except the logs (the current log file is open).
	es, _ := os.ReadDir(dataDir)
	for _, e := range es {
		if e.Name() == "logs" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dataDir, e.Name())); err != nil {
			d.Log.Warn("remove", "path", e.Name(), "err", err)
		}
	}
	d.UI.Info(title, "CleanInstaller wurde entfernt. Dein normales Among Us ist unverändert.", "OK")
	return Uninstalled, nil
}

func relaunch(d Deps) (Outcome, error) {
	tmp := filepath.Join(d.TempDir, setupCopyName)
	if err := copyFile(d.SelfPath, tmp); err != nil {
		return 0, apperr.New("Der Installer konnte nicht gestartet werden.", err)
	}
	if _, err := d.P.StartProcess(tmp, d.TempDir); err != nil {
		return 0, apperr.New("Der Installer konnte nicht gestartet werden.", err)
	}
	d.Log.Info("relaunched from temp copy", "path", tmp)
	return Relaunched, nil
}

func inside(path, dir string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func inModdedFolder(path string) bool {
	for _, part := range strings.Split(filepath.Clean(path), string(filepath.Separator)) {
		if strings.EqualFold(part, steam.ModdedFolder) {
			return true
		}
	}
	return false
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
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
