package install_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Kiirb/CleanInstaller/internal/apperr"
	. "github.com/Kiirb/CleanInstaller/internal/install"
	"github.com/Kiirb/CleanInstaller/internal/migrate"
	"github.com/Kiirb/CleanInstaller/internal/steam"
	"github.com/Kiirb/CleanInstaller/internal/testenv"
	"github.com/Kiirb/CleanInstaller/internal/vdf"
)

func run(t *testing.T, w *testenv.World) (Outcome, error) {
	t.Helper()
	return Run(context.Background(), w.Deps())
}

func TestInstallFresh(t *testing.T) {
	w := testenv.NewWorld(t, 1, true)
	w.UI.Confirms = []bool{true}
	w.UI.Infos = []int{1}
	out, err := run(t, w)
	if err != nil || out != Installed {
		t.Fatalf("%v %v calls=%v", out, err, w.UI.Calls)
	}
	b, _ := os.ReadFile(w.InstalledExe())
	if string(b) != "self-exe-v1" {
		t.Fatal("exe not copied")
	}
	if !testenv.Exists(filepath.Join(w.Data, "state.json")) || !testenv.Exists(filepath.Join(w.Game.ModdedDir, "BepInEx", "plugins", "Reactor.dll")) {
		t.Fatal("not synced")
	}
	names := w.ShortcutNames(w.Users[0])
	want := `Among Us Modded|"` + w.InstalledExe() + `"|--launch`
	if len(names) != 1 || names[0] != want {
		t.Fatalf("shortcuts %v want %s", names, want)
	}
	id := steam.ShortcutAppID(`"`+w.InstalledExe()+`"`, steam.ShortcutName)
	if !testenv.Exists(steam.IconPath(steam.UserConfigDir(w.Root, w.Users[0]), id)) {
		t.Fatal("artwork missing")
	}
	if !strings.Contains(w.UI.Calls[0], "Among Us Modded") || !strings.Contains(w.UI.Calls[0], "%LOCALAPPDATA%") {
		t.Fatalf("first screen does not disclose changes: %s", w.UI.Calls[0])
	}
}

func TestInstallFinishStartsSteam(t *testing.T) {
	w := testenv.NewWorld(t, 1, true)
	w.UI.Confirms = []bool{true}
	w.UI.Infos = []int{0}
	if _, err := run(t, w); err != nil {
		t.Fatal(err)
	}
	if w.Fake.SteamStarted != 1 {
		t.Fatal("steam not started")
	}
}

func TestInstallGameMissingStops(t *testing.T) {
	w := testenv.NewWorld(t, 1, false)
	w.UI.Confirms = []bool{true}
	out, err := run(t, w)
	if err != nil || out != Cancelled {
		t.Fatalf("%v %v", out, err)
	}
	if len(w.Fake.OpenedURLs) != 1 || w.Fake.OpenedURLs[0] != "steam://install/945360" {
		t.Fatalf("urls %v", w.Fake.OpenedURLs)
	}
	if testenv.Exists(w.Game.ModdedDir) || testenv.Exists(w.InstalledExe()) {
		t.Fatal("continued after missing game")
	}
}

func TestInstallSteamNotFound(t *testing.T) {
	w := testenv.NewWorld(t, 1, true)
	w.Fake.Root = ""
	_, err := run(t, w)
	if err == nil || !strings.Contains(apperr.Message(err), "Steam wurde nicht gefunden") {
		t.Fatalf("%v", err)
	}
}

func TestInstallSteamRunningCancel(t *testing.T) {
	w := testenv.NewWorld(t, 1, true)
	w.Fake.SteamUp = true
	w.UI.Confirms = []bool{true, false}
	out, err := run(t, w)
	if err != nil || out != Cancelled {
		t.Fatalf("%v %v", out, err)
	}
	if testenv.Exists(filepath.Join(steam.UserConfigDir(w.Root, w.Users[0]), "shortcuts.vdf")) {
		t.Fatal("shortcuts touched")
	}
	if w.Fake.SteamShutdown != 0 {
		t.Fatal("steam shut down anyway")
	}
}

func TestInstallSteamRunningClosesAndRestarts(t *testing.T) {
	w := testenv.NewWorld(t, 1, true)
	w.Fake.SteamUp = true
	w.UI.Confirms = []bool{true, true}
	w.UI.Infos = []int{0}
	out, err := run(t, w)
	if err != nil || out != Installed || w.Fake.SteamShutdown != 1 || w.Fake.SteamStarted != 1 {
		t.Fatalf("%v %v shutdown=%d started=%d", out, err, w.Fake.SteamShutdown, w.Fake.SteamStarted)
	}
}

func TestInstallMultipleUsersChoose(t *testing.T) {
	w := testenv.NewWorld(t, 2, true)
	w.UI.Chooses = []int{1}
	w.UI.Confirms = []bool{true}
	if _, err := run(t, w); err != nil {
		t.Fatal(err)
	}
	if got := w.UI.Options[0]; len(got) != 2 || got[0] != "Alice" || got[1] != "Bob" {
		t.Fatalf("options %v", got)
	}
	if len(w.ShortcutNames(w.Users[0])) != 0 || len(w.ShortcutNames(w.Users[1])) != 1 {
		t.Fatal("shortcut for wrong user")
	}
}

func TestInstallMigratesOld(t *testing.T) {
	w := testenv.NewWorld(t, 2, true)
	oldExe := `"` + filepath.Join(w.Game.ModdedDir, "CInstaller.exe") + `"`
	cfgA := steam.UserConfigDir(w.Root, w.Users[0])
	rootA, _ := steam.LoadShortcuts(cfgA)
	sc := rootA.Child("shortcuts")
	sc.Children = append(sc.Children,
		&vdf.BNode{Key: "0", Type: vdf.TypeMap, Children: []*vdf.BNode{{Key: "AppName", Type: vdf.TypeString, Str: "Other"}, {Key: "Exe", Type: vdf.TypeString, Str: `"o.exe"`}}},
		&vdf.BNode{Key: "1", Type: vdf.TypeMap, Children: []*vdf.BNode{{Key: "appid", Type: vdf.TypeInt32, Int: migrate.OldAppID}, {Key: "AppName", Type: vdf.TypeString, Str: "Among Us Modded"}, {Key: "Exe", Type: vdf.TypeString, Str: oldExe}}})
	if err := steam.SaveShortcuts(cfgA, rootA); err != nil {
		t.Fatal(err)
	}
	testenv.WriteFile(t, filepath.Join(cfgA, "grid", migrate.OldArtPrefix+"p.png"), "old-art")
	testenv.WriteFile(t, filepath.Join(w.Game.ModdedDir, migrate.OldConfig), "{}")
	testenv.WriteFile(t, filepath.Join(w.Game.ModdedDir, "CInstaller.exe"), "old")
	testenv.WriteFile(t, filepath.Join(w.Game.ModdedDir, "Among Us.exe"), "game")
	testenv.WriteFile(t, filepath.Join(w.Game.ModdedDir, "BepInEx", "config", "x.cfg"), "user-setting")

	w.UI.Chooses = []int{1} // install for Bob
	w.UI.Confirms = []bool{true}
	out, err := run(t, w)
	if err != nil || out != Installed {
		t.Fatalf("%v %v", out, err)
	}
	if !strings.Contains(w.UI.Calls[1], "Ersetzt deine alte CInstaller-Installation") {
		t.Fatalf("migration not disclosed: %v", w.UI.Calls)
	}
	if names := w.ShortcutNames(w.Users[0]); len(names) != 1 || !strings.HasPrefix(names[0], "Other|") {
		t.Fatalf("user A shortcuts %v", names)
	}
	if testenv.Exists(filepath.Join(cfgA, "grid", migrate.OldArtPrefix+"p.png")) {
		t.Fatal("old art kept")
	}
	b, _ := os.ReadFile(filepath.Join(w.Game.ModdedDir, "BepInEx", "config", "x.cfg"))
	if string(b) != "user-setting" {
		t.Fatal("user config lost")
	}
	if testenv.Exists(filepath.Join(w.Game.ModdedDir, "CInstaller.exe")) || testenv.Exists(filepath.Join(w.Game.ModdedDir, migrate.OldConfig)) {
		t.Fatal("old folder not rebuilt")
	}
	if len(w.ShortcutNames(w.Users[1])) != 1 {
		t.Fatal("new shortcut missing")
	}
}

func TestInstallRelaunchFromDataDir(t *testing.T) {
	w := testenv.NewWorld(t, 1, true)
	w.Self = w.InstalledExe()
	testenv.WriteFile(t, w.Self, "installed-exe")
	out, err := run(t, w)
	if err != nil || out != Relaunched {
		t.Fatalf("%v %v", out, err)
	}
	tmp := filepath.Join(w.Temp, "CleanInstaller-setup.exe")
	if len(w.Fake.Started) != 1 || w.Fake.Started[0].Exe != tmp || len(w.Fake.Started[0].Args) != 0 || !testenv.Exists(tmp) {
		t.Fatalf("started %+v", w.Fake.Started)
	}
}

func TestInstallRelaunchFromOldModdedFolder(t *testing.T) {
	w := testenv.NewWorld(t, 1, true)
	w.Self = filepath.Join(w.Game.ModdedDir, "CInstaller.exe")
	testenv.WriteFile(t, w.Self, "old-location")
	out, err := run(t, w)
	if err != nil || out != Relaunched {
		t.Fatalf("%v %v", out, err)
	}
}

func TestInstallJokeOnlyWhenLive(t *testing.T) {
	w := testenv.NewWorld(t, 1, true)
	w.SetFun(true)
	w.UI.Confirms = []bool{true}
	if _, err := run(t, w); err != nil {
		t.Fatal(err)
	}
	if !w.UI.Displays[0].JokeText || len(w.UI.Displays[0].JokeLines) != 2 {
		t.Fatalf("live display %+v", w.UI.Displays[0])
	}
	// Second run: server down, manifest from cache -> no joke mode.
	w.Server.Close()
	w.UI = &testenv.FakeUI{Chooses: []int{0}} // Reparieren
	if _, err := run(t, w); err != nil {
		t.Fatal(err)
	}
	if w.UI.Displays[0].JokeText {
		t.Fatal("joke mode from cached manifest")
	}
}

func TestInstallOffline(t *testing.T) {
	w := testenv.NewWorld(t, 1, true)
	w.Server.Close()
	_, err := run(t, w)
	if err == nil || !strings.HasPrefix(apperr.Message(err), "Keine Internetverbindung") {
		t.Fatalf("%v", err)
	}
}

func TestUninstall(t *testing.T) {
	w := testenv.NewWorld(t, 2, true)
	w.UI.Chooses = []int{0}
	w.UI.Confirms = []bool{true}
	if _, err := run(t, w); err != nil {
		t.Fatal(err)
	}
	w.UI = &testenv.FakeUI{Chooses: []int{1}} // Deinstallieren
	out, err := run(t, w)
	if err != nil || out != Uninstalled {
		t.Fatalf("%v %v", out, err)
	}
	for _, u := range w.Users {
		if n := w.ShortcutNames(u); len(n) != 0 {
			t.Fatalf("shortcuts left %v", n)
		}
	}
	if testenv.Exists(w.Game.ModdedDir) || testenv.Exists(w.InstalledExe()) || testenv.Exists(filepath.Join(w.Data, "state.json")) {
		t.Fatal("files left")
	}
	if !testenv.Exists(filepath.Join(w.Game.VanillaDir, "Among Us.exe")) {
		t.Fatal("vanilla touched")
	}
}

func TestInstallChooseCancel(t *testing.T) {
	w := testenv.NewWorld(t, 1, true)
	w.UI.Confirms = []bool{true}
	if _, err := run(t, w); err != nil {
		t.Fatal(err)
	}
	w.UI = &testenv.FakeUI{Chooses: []int{2}} // Abbrechen
	out, err := run(t, w)
	if err != nil || out != Cancelled {
		t.Fatalf("%v %v", out, err)
	}
}

func TestRepairLockedKeepsInstallUsable(t *testing.T) { // final review I1
	if runtime.GOOS != "windows" {
		t.Skip("windows file locking")
	}
	w := testenv.NewWorld(t, 1, true)
	w.UI.Confirms = []bool{true}
	if _, err := Run(context.Background(), w.Deps()); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filepath.Join(w.Game.ModdedDir, "Among Us.exe"))
	if err != nil {
		t.Fatal(err)
	}
	w.UI = &testenv.FakeUI{Chooses: []int{0}} // Reparieren
	_, err = Run(context.Background(), w.Deps())
	f.Close()
	if err == nil || !strings.Contains(apperr.Message(err), "benutzt") {
		t.Fatalf("%v", err)
	}
	if !testenv.Exists(filepath.Join(w.Data, "state.json")) {
		t.Fatal("state deleted by failed repair: launcher would say 'nicht installiert'")
	}
}

func TestInstallDisclosesExistingFolder(t *testing.T) { // final review I5
	w := testenv.NewWorld(t, 1, true)
	testenv.WriteFile(t, filepath.Join(w.Game.ModdedDir, "Among Us.exe"), "someone else's copy")
	w.UI.Confirms = []bool{false}
	if _, err := Run(context.Background(), w.Deps()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(w.UI.Calls[0], "Ersetzt den vorhandenen Ordner") {
		t.Fatalf("not disclosed: %s", w.UI.Calls[0])
	}
}

func TestUninstallWhileGameRunning(t *testing.T) { // final review I6
	w := testenv.NewWorld(t, 1, true)
	w.UI.Confirms = []bool{true}
	if _, err := Run(context.Background(), w.Deps()); err != nil {
		t.Fatal(err)
	}
	w.Fake.Running = map[string]bool{strings.ToLower(filepath.Join(w.Game.ModdedDir, "Among Us.exe")): true}
	w.UI = &testenv.FakeUI{Chooses: []int{1}}
	_, err := Run(context.Background(), w.Deps())
	if err == nil || !strings.Contains(apperr.Message(err), "Schließe") {
		t.Fatalf("%v", err)
	}
	if len(w.ShortcutNames(w.Users[0])) != 1 || !testenv.Exists(w.Game.ModdedDir) {
		t.Fatal("uninstall went ahead while game runs")
	}
}

func TestInstallAsksSteamCloseBeforeSync(t *testing.T) { // final review I8
	w := testenv.NewWorld(t, 1, true)
	w.Fake.SteamUp = true
	w.UI.Confirms = []bool{true, false}
	if out, err := Run(context.Background(), w.Deps()); err != nil || out != Cancelled {
		t.Fatalf("%v %v", out, err)
	}
	if testenv.Exists(w.Game.ModdedDir) || testenv.Exists(filepath.Join(w.Data, "state.json")) {
		t.Fatal("synced before asking about Steam")
	}
}

func TestInstallSecondInstanceRefused(t *testing.T) { // final review I2
	w := testenv.NewWorld(t, 1, true)
	w.Fake.LockHeld = true
	_, err := Run(context.Background(), w.Deps())
	if err == nil || !strings.Contains(apperr.Message(err), "läuft bereits") {
		t.Fatalf("%v", err)
	}
}

func TestInstallReleasesLock(t *testing.T) {
	w := testenv.NewWorld(t, 1, true)
	w.UI.Confirms = []bool{true}
	if _, err := Run(context.Background(), w.Deps()); err != nil {
		t.Fatal(err)
	}
	if w.Fake.LockHeld {
		t.Fatal("lock not released")
	}
}
