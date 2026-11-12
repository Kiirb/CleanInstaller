// Package testenv builds a fake PC (Steam, Among Us, manifest server) and a
// scripted UI for flow tests. Only test code imports it.
package testenv

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Kiirb/CleanInstaller/assets"
	"github.com/Kiirb/CleanInstaller/internal/applog"
	"github.com/Kiirb/CleanInstaller/internal/install"
	"github.com/Kiirb/CleanInstaller/internal/manifest"
	"github.com/Kiirb/CleanInstaller/internal/platform"
	"github.com/Kiirb/CleanInstaller/internal/progress"
	"github.com/Kiirb/CleanInstaller/internal/steam"
	"github.com/Kiirb/CleanInstaller/internal/ui"
)

// FakeUI answers dialogs from scripted queues and records every call.
type FakeUI struct {
	mu       sync.Mutex
	Confirms []bool
	Chooses  []int
	Infos    []int
	Calls    []string // "confirm:<title>|<body>", "choose:...", "info:...", "error:<msg>", "progress:<title>"
	Displays []ui.Display
	Options  [][]string
	Errors   []error
	Trackers []*progress.Tracker
}

func (f *FakeUI) Progress(title string, t *progress.Tracker, d ui.Display) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, "progress:"+title)
	f.Displays = append(f.Displays, d)
	f.Trackers = append(f.Trackers, t)
}

func (f *FakeUI) Confirm(title, body, ok, cancel string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, "confirm:"+title+"|"+body)
	if len(f.Confirms) == 0 {
		panic("unexpected Confirm: " + title + " " + body)
	}
	r := f.Confirms[0]
	f.Confirms = f.Confirms[1:]
	return r
}

func (f *FakeUI) Choose(title, body string, options []string) (int, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, "choose:"+title+"|"+body)
	f.Options = append(f.Options, options)
	if len(f.Chooses) == 0 {
		panic("unexpected Choose: " + title)
	}
	r := f.Chooses[0]
	f.Chooses = f.Chooses[1:]
	return r, r >= 0
}

func (f *FakeUI) Info(title, body string, buttons ...string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, "info:"+title+"|"+body)
	if len(f.Infos) == 0 {
		return len(buttons) - 1
	}
	r := f.Infos[0]
	f.Infos = f.Infos[1:]
	return r
}

func (f *FakeUI) Error(err error, logDir string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Errors = append(f.Errors, err)
}

func (f *FakeUI) Close() {}

func (f *FakeUI) Called(prefix string) bool {
	for _, c := range f.Calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func SHA(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func WriteFile(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func Exists(p string) bool { _, err := os.Stat(p); return err == nil }

func vdfPath(p string) string { return strings.ReplaceAll(p, `\`, `\\`) }

// World is a fake PC: Steam root, a library with Among Us, users, a manifest
// server, and a fake self exe.
type World struct {
	T        *testing.T
	Root     string
	Lib      string
	Game     *steam.Game
	Users    []steam.User
	Data     string
	Temp     string
	Self     string
	Fake     *platform.Fake
	UI       *FakeUI
	Server   *httptest.Server
	Files    map[string][]byte
	Manifest *manifest.Manifest
}

func NewWorld(t *testing.T, users int, withGame bool) *World {
	t.Helper()
	w := &World{T: t, Root: t.TempDir(), Lib: t.TempDir(), Data: filepath.Join(t.TempDir(), "CleanInstaller"), Temp: t.TempDir(), UI: &FakeUI{}, Files: map[string][]byte{}}
	WriteFile(t, filepath.Join(w.Root, "steam.exe"), "steam")
	WriteFile(t, filepath.Join(w.Root, "steamapps", "libraryfolders.vdf"), `"libraryfolders" { "0" { "path" "`+vdfPath(w.Root)+`" "apps" { } } "1" { "path" "`+vdfPath(w.Lib)+`" "apps" { "945360" "1" } } }`)
	common := filepath.Join(w.Lib, "steamapps", "common")
	w.Game = &steam.Game{LibraryDir: w.Lib, CommonDir: common, VanillaDir: filepath.Join(common, "Among Us"), ModdedDir: filepath.Join(common, "Among Us Modded"), BuildID: "100"}
	if withGame {
		WriteFile(t, filepath.Join(w.Lib, "steamapps", "appmanifest_945360.acf"), `"AppState" { "appid" "945360" "buildid" "100" }`)
		WriteFile(t, filepath.Join(w.Game.VanillaDir, "Among Us.exe"), "game")
	}
	login := `"users" {`
	for i := 0; i < users; i++ {
		id3 := uint32(1000 + i)
		u := steam.User{ID3: id3, ID64: uint64(steam.SteamID64Base) + uint64(id3), PersonaName: []string{"Alice", "Bob", "Carol"}[i], MostRecent: i == 0}
		w.Users = append(w.Users, u)
		mr := "0"
		if u.MostRecent {
			mr = "1"
		}
		login += ` "` + itoa64(u.ID64) + `" { "PersonaName" "` + u.PersonaName + `" "MostRecent" "` + mr + `" }`
		_ = os.MkdirAll(steam.UserConfigDir(w.Root, u), 0o755)
	}
	WriteFile(t, filepath.Join(w.Root, "config", "loginusers.vdf"), login+" }")

	w.Self = filepath.Join(t.TempDir(), "Downloads", "CleanInstaller.exe")
	WriteFile(t, w.Self, "self-exe-v1")

	w.Files["/pack.zip"] = ZipOf(t, map[string]string{"P/winhttp.dll": "doorstop", "P/BepInEx/plugins/MiraAPI.dll": "mira", "P/BepInEx/config/BepInEx.cfg": "pack-cfg"})
	w.Files["/Reactor.dll"] = []byte("reactor")
	w.Server = httptest.NewTLSServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/manifest.json" {
			b, _ := manifest.Marshal(w.Manifest)
			_, _ = rw.Write(b)
			return
		}
		b, ok := w.Files[r.URL.Path]
		if !ok {
			http.NotFound(rw, r)
			return
		}
		_, _ = rw.Write(b)
	}))
	t.Cleanup(w.Server.Close)
	asset := func(p, file, id string) manifest.Asset {
		return manifest.Asset{ID: id, FileName: file, Tag: "v1", URL: w.Server.URL + p, SHA256: SHA(w.Files[p]), Size: int64(len(w.Files[p]))}
	}
	w.Manifest = &manifest.Manifest{Schema: 1, Modpack: asset("/pack.zip", "", ""), Plugins: []manifest.Asset{asset("/Reactor.dll", "Reactor.dll", "Reactor")}, Remove: []string{}}
	w.Fake = &platform.Fake{Root: w.Root, Data: w.Data}
	return w
}

func itoa64(v uint64) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func ZipOf(t *testing.T, entries map[string]string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for n, c := range entries {
		f, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.Write([]byte(c))
	}
	_ = zw.Close()
	return buf.Bytes()
}

func (w *World) SetFun(jokeText bool) {
	w.Manifest.FunRaw = json.RawMessage(`{"jokeText":` + map[bool]string{true: "true", false: "false"}[jokeText] + `,"chaosBar":false,"jokeLines":["x","y"]}`)
}

func (w *World) Deps() install.Deps {
	return install.Deps{P: w.Fake, UI: w.UI, HTTP: w.Server.Client(), ManifestURL: w.Server.URL + "/manifest.json",
		SelfPath: w.Self, Log: applog.Discard(), TempDir: w.Temp, Art: assets.Steam}
}

func (w *World) InstalledExe() string { return filepath.Join(w.Data, "CleanInstaller.exe") }

func (w *World) ShortcutNames(u steam.User) []string {
	root, err := steam.LoadShortcuts(steam.UserConfigDir(w.Root, u))
	if err != nil {
		w.T.Fatal(err)
	}
	var out []string
	for _, e := range root.Child("shortcuts").Children {
		str := func(k string) string {
			if c := e.Child(k); c != nil {
				return c.Str
			}
			return ""
		}
		out = append(out, str("AppName")+"|"+str("Exe")+"|"+str("LaunchOptions"))
	}
	return out
}

// Asset describes a file served by the world's server at path.
func (w *World) Asset(path, file, id string) manifest.Asset {
	return manifest.Asset{ID: id, FileName: file, Tag: "v1", URL: w.Server.URL + path, SHA256: SHA(w.Files[path]), Size: int64(len(w.Files[path]))}
}
