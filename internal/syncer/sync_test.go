package syncer

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/Kiirb/CleanInstaller/internal/applog"
	"github.com/Kiirb/CleanInstaller/internal/download"
	"github.com/Kiirb/CleanInstaller/internal/manifest"
	"github.com/Kiirb/CleanInstaller/internal/progress"
	"github.com/Kiirb/CleanInstaller/internal/steam"
)

type env struct {
	game  *steam.Game
	man   *manifest.Manifest
	data  string
	sync  *Syncer
	files map[string][]byte // served by URL path
}

func shaOf(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func zipBytes(t *testing.T, entries map[string]string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	names := make([]string, 0, len(entries))
	for n := range entries {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(entries[n]))
	}
	_ = zw.Close()
	return buf.Bytes()
}

func writeFile(t *testing.T, p, s string) {
	t.Helper()
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func newEnv(t *testing.T) *env {
	t.Helper()
	common := filepath.Join(t.TempDir(), "steamapps", "common")
	g := &steam.Game{CommonDir: common, VanillaDir: filepath.Join(common, "Among Us"), ModdedDir: filepath.Join(common, "Among Us Modded"), BuildID: "100"}
	writeFile(t, filepath.Join(g.VanillaDir, "Among Us.exe"), "game-v1")
	writeFile(t, filepath.Join(g.VanillaDir, "Among Us_Data", "data.bin"), "data")
	writeFile(t, filepath.Join(g.VanillaDir, "winhttp.dll"), "old-bepinex")
	writeFile(t, filepath.Join(g.VanillaDir, "BepInEx", "plugins", "Stale.dll"), "stale")

	e := &env{game: g, data: t.TempDir(), files: map[string][]byte{}}
	e.files["/pack.zip"] = zipBytes(t, map[string]string{
		"Pack/winhttp.dll":                   "doorstop",
		"Pack/BepInEx/core/BepInEx.Core.dll": "core",
		"Pack/BepInEx/plugins/MiraAPI.dll":   "mira-from-pack",
		"Pack/BepInEx/plugins/Reactor.dll":   "reactor-from-pack",
		"Pack/BepInEx/config/BepInEx.cfg":    "pack-default-cfg",
		"Pack/BepInEx/config/PackOnly.cfg":   "pack-only",
	})
	e.files["/Reactor.dll"] = []byte("reactor-v2")
	e.files["/Extra.dll"] = []byte("extra-v1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, ok := e.files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)
	asset := func(path, file, id string) manifest.Asset {
		return manifest.Asset{ID: id, FileName: file, Tag: "v1", URL: srv.URL + path, SHA256: shaOf(e.files[path]), Size: int64(len(e.files[path]))}
	}
	e.man = &manifest.Manifest{Schema: 1,
		Modpack: asset("/pack.zip", "", ""),
		Plugins: []manifest.Asset{asset("/Reactor.dll", "Reactor.dll", "Reactor"), asset("/Extra.dll", "Extra.dll", "Extra")},
	}
	// srv.URL is http; Validate requires https, so the tests skip Validate.
	e.sync = &Syncer{Cache: &download.Cache{Dir: filepath.Join(e.data, "cache"), HTTP: srv.Client()}, Tracker: progress.NewTracker(), Log: applog.Discard()}
	return e
}

func (e *env) refreshAsset(path string, a *manifest.Asset) {
	a.SHA256 = shaOf(e.files[path])
	a.Size = int64(len(e.files[path]))
}

func (e *env) install(t *testing.T) *State {
	t.Helper()
	p, err := MakePlan(nil, e.game, e.man)
	if err != nil {
		t.Fatal(err)
	}
	st, err := e.sync.Apply(context.Background(), p, e.game, e.man, e.data)
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveState(e.data, st); err != nil {
		t.Fatal(err)
	}
	return st
}

func plugins(m string) string { return filepath.Join(m, "BepInEx", "plugins") }

func TestPlanFreshInstallRebuilds(t *testing.T) {
	e := newEnv(t)
	p, err := MakePlan(nil, e.game, e.man)
	if err != nil || !p.Rebuild || p.Empty() {
		t.Fatalf("%v %+v", err, p)
	}
}

func TestApplyFreshInstallLayout(t *testing.T) {
	e := newEnv(t)
	st := e.install(t)
	m := e.game.ModdedDir
	if readFile(t, filepath.Join(m, "Among Us.exe")) != "game-v1" || readFile(t, filepath.Join(m, "winhttp.dll")) != "doorstop" {
		t.Fatal("game/pack files wrong")
	}
	if _, err := os.Stat(filepath.Join(plugins(m), "Stale.dll")); err == nil {
		t.Fatal("vanilla BepInEx leftovers copied")
	}
	if readFile(t, filepath.Join(plugins(m), "Reactor.dll")) != "reactor-v2" {
		t.Fatal("manifest plugin did not override pack plugin")
	}
	if readFile(t, filepath.Join(plugins(m), "MiraAPI.dll")) != "mira-from-pack" {
		t.Fatal("pack-shipped plugin deleted")
	}
	if readFile(t, filepath.Join(plugins(m), "Extra.dll")) != "extra-v1" {
		t.Fatal("extra plugin missing")
	}
	if st.BuildID != "100" || st.ModpackSHA != e.man.Modpack.SHA256 || st.Plugins["Extra.dll"] != e.man.Plugins[1].SHA256 {
		t.Fatalf("state %+v", st)
	}
	if s := e.sync.Tracker.Snapshot(); s.Percent < 99.9 {
		t.Fatalf("progress not complete: %+v", s)
	}
}

func TestPlanUpToDateEmpty(t *testing.T) {
	e := newEnv(t)
	st := e.install(t)
	p, err := MakePlan(st, e.game, e.man)
	if err != nil || !p.Empty() {
		t.Fatalf("%v %+v", err, p)
	}
}

func TestPlanBuildIDChangeRebuilds(t *testing.T) {
	e := newEnv(t)
	st := e.install(t)
	e.game.BuildID = "101"
	p, _ := MakePlan(st, e.game, e.man)
	if !p.Rebuild {
		t.Fatal("no rebuild on game update")
	}
}

func TestPlanModpackChangeRebuilds(t *testing.T) {
	e := newEnv(t)
	st := e.install(t)
	e.man.Modpack.SHA256 = strings.Repeat("0", 64)
	p, _ := MakePlan(st, e.game, e.man)
	if !p.Rebuild {
		t.Fatal("no rebuild on modpack change")
	}
}

func TestPlanWrongPluginHashFetches(t *testing.T) {
	e := newEnv(t)
	st := e.install(t)
	writeFile(t, filepath.Join(plugins(e.game.ModdedDir), "Extra.dll"), "tampered")
	p, _ := MakePlan(st, e.game, e.man)
	if p.Rebuild || len(p.PluginsToFetch) != 1 || p.PluginsToFetch[0].FileName != "Extra.dll" {
		t.Fatalf("%+v", p)
	}
	if _, err := e.sync.Apply(context.Background(), p, e.game, e.man, e.data); err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(plugins(e.game.ModdedDir), "Extra.dll")) != "extra-v1" {
		t.Fatal("not repaired")
	}
}

func TestPlanExtraPluginDeletedPackPluginKept(t *testing.T) {
	e := newEnv(t)
	st := e.install(t)
	writeFile(t, filepath.Join(plugins(e.game.ModdedDir), "UserAdded.dll"), "x")
	p, _ := MakePlan(st, e.game, e.man)
	if len(p.FilesToDelete) != 1 || filepath.Base(p.FilesToDelete[0]) != "UserAdded.dll" {
		t.Fatalf("%+v", p.FilesToDelete)
	}
	if _, err := e.sync.Apply(context.Background(), p, e.game, e.man, e.data); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(plugins(e.game.ModdedDir), "UserAdded.dll")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("extra plugin not deleted")
	}
	if _, err := os.Stat(filepath.Join(plugins(e.game.ModdedDir), "MiraAPI.dll")); err != nil {
		t.Fatal("pack plugin deleted")
	}
}

func TestPlanRemoveList(t *testing.T) {
	e := newEnv(t)
	st := e.install(t)
	e.man.Remove = []string{"BepInEx/core/BepInEx.Core.dll", "does/not/exist.txt"}
	p, _ := MakePlan(st, e.game, e.man)
	if len(p.FilesToDelete) != 1 {
		t.Fatalf("%+v", p.FilesToDelete)
	}
}

func TestApplyRebuildKeepsUserConfig(t *testing.T) {
	e := newEnv(t)
	st := e.install(t)
	cfg := filepath.Join(e.game.ModdedDir, "BepInEx", "config")
	writeFile(t, filepath.Join(cfg, "BepInEx.cfg"), "user-edited")
	writeFile(t, filepath.Join(cfg, "keybinds.cfg"), "user-keys")
	e.game.BuildID = "101"
	p, _ := MakePlan(st, e.game, e.man)
	if _, err := e.sync.Apply(context.Background(), p, e.game, e.man, e.data); err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(cfg, "BepInEx.cfg")) != "user-edited" || readFile(t, filepath.Join(cfg, "keybinds.cfg")) != "user-keys" ||
		readFile(t, filepath.Join(cfg, "PackOnly.cfg")) != "pack-only" {
		t.Fatal("config not preserved/merged")
	}
	if _, err := os.Stat(e.game.ModdedDir + ".new"); err == nil {
		t.Fatal(".new left behind")
	}
	if _, err := os.Stat(e.game.ModdedDir + ".old"); err == nil {
		t.Fatal(".old left behind")
	}
}

func TestApplyRebuildWritesSteamAppID(t *testing.T) {
	e := newEnv(t)
	e.install(t)
	if strings.TrimSpace(readFile(t, filepath.Join(e.game.ModdedDir, "steam_appid.txt"))) != "945360" {
		t.Fatal("steam_appid.txt")
	}
}

func treeHash(t *testing.T, dir string) string {
	h := sha256.New()
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		h.Write([]byte(p))
		if !d.IsDir() {
			b, _ := os.ReadFile(p)
			h.Write(b)
		}
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))
}

func TestApplyDoesNotTouchVanilla(t *testing.T) {
	e := newEnv(t)
	before := treeHash(t, e.game.VanillaDir)
	st := e.install(t)
	e.game.BuildID = "101"
	p, _ := MakePlan(st, e.game, e.man)
	if _, err := e.sync.Apply(context.Background(), p, e.game, e.man, e.data); err != nil {
		t.Fatal(err)
	}
	if treeHash(t, e.game.VanillaDir) != before {
		t.Fatal("vanilla changed")
	}
}

func TestApplyLockedKeepsOldFolder(t *testing.T) { // Review Focus 2
	if runtime.GOOS != "windows" {
		t.Skip("file locking semantics are Windows-specific")
	}
	e := newEnv(t)
	st := e.install(t)
	before := treeHash(t, e.game.ModdedDir)
	f, err := os.Open(filepath.Join(e.game.ModdedDir, "Among Us.exe"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	e.game.BuildID = "101"
	p, _ := MakePlan(st, e.game, e.man)
	_, err = e.sync.Apply(context.Background(), p, e.game, e.man, e.data)
	if !errors.Is(err, ErrModdedInUse) {
		t.Fatalf("err %v", err)
	}
	f.Close()
	if treeHash(t, e.game.ModdedDir) != before {
		t.Fatal("modded folder changed")
	}
	if _, err := os.Stat(e.game.ModdedDir + ".new"); err == nil {
		t.Fatal(".new left behind")
	}
}

func TestApplyOfflineRebuildLeavesFolderIntact(t *testing.T) {
	e := newEnv(t)
	st := e.install(t)
	before := treeHash(t, e.game.ModdedDir)
	e.files["/pack.zip"] = zipBytes(t, map[string]string{"Pack/new.txt": "n"})
	e.refreshAsset("/pack.zip", &e.man.Modpack)
	delete(e.files, "/pack.zip") // download now 404s
	p, _ := MakePlan(st, e.game, e.man)
	if _, err := e.sync.Apply(context.Background(), p, e.game, e.man, e.data); err == nil {
		t.Fatal("expected error")
	}
	if treeHash(t, e.game.ModdedDir) != before {
		t.Fatal("modded folder changed")
	}
}

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if st, err := LoadState(dir); st != nil || err != nil {
		t.Fatalf("%v %v", st, err)
	}
	in := &State{BuildID: "1", ModpackSHA: "a", Plugins: map[string]string{"x.dll": "b"}, ModdedDir: `C:\m`, PackPlugins: []string{"mira.dll"}}
	if err := SaveState(dir, in); err != nil {
		t.Fatal(err)
	}
	out, err := LoadState(dir)
	if err != nil || out.BuildID != "1" || out.Plugins["x.dll"] != "b" || out.PackPlugins[0] != "mira.dll" {
		t.Fatalf("%v %+v", err, out)
	}
}
