package launch_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kiirb/CleanInstaller/internal/apperr"
	"github.com/Kiirb/CleanInstaller/internal/install"
	"github.com/Kiirb/CleanInstaller/internal/launch"
	"github.com/Kiirb/CleanInstaller/internal/testenv"
)

// installed returns a world after a successful install, running as the
// installed exe with a fresh scripted UI.
func installed(t *testing.T) *testenv.World {
	t.Helper()
	w := testenv.NewWorld(t, 1, true)
	w.UI.Confirms = []bool{true}
	if out, err := install.Run(context.Background(), w.Deps()); err != nil || out != install.Installed {
		t.Fatalf("install: %v %v", out, err)
	}
	w.UI = &testenv.FakeUI{}
	w.Self = w.InstalledExe()
	w.Fake.Started = nil
	return w
}

func gameExe(w *testenv.World) string { return filepath.Join(w.Game.ModdedDir, "Among Us.exe") }

func run(w *testenv.World, version string) error {
	return launch.Run(context.Background(), w.Deps(), version)
}

func TestLaunchUpToDateNoWindow(t *testing.T) {
	w := installed(t)
	if err := run(w, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if w.UI.Called("progress:") {
		t.Fatal("window shown with nothing to do")
	}
	if len(w.Fake.Started) != 1 || w.Fake.Started[0].Exe != gameExe(w) || w.Fake.Started[0].Dir != w.Game.ModdedDir {
		t.Fatalf("started %+v", w.Fake.Started)
	}
}

func TestLaunchPluginUpdate(t *testing.T) {
	w := installed(t)
	w.Files["/Reactor.dll"] = []byte("reactor-v2")
	w.Manifest.Plugins[0] = w.Asset("/Reactor.dll", "Reactor.dll", "Reactor")
	if err := run(w, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if !w.UI.Called("progress:") {
		t.Fatal("no progress window for update")
	}
	b, _ := os.ReadFile(filepath.Join(w.Game.ModdedDir, "BepInEx", "plugins", "Reactor.dll"))
	if string(b) != "reactor-v2" {
		t.Fatalf("plugin %q", b)
	}
	if len(w.Fake.Started) != 1 {
		t.Fatal("game not started")
	}
}

func TestLaunchOfflineStillStarts(t *testing.T) {
	w := installed(t)
	w.Server.Close()
	if err := run(w, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if len(w.Fake.Started) != 1 || w.Fake.Started[0].Exe != gameExe(w) {
		t.Fatalf("started %+v", w.Fake.Started)
	}
}

func TestLaunchAlreadyRunning(t *testing.T) { // Review Focus 2
	w := installed(t)
	w.Fake.Running = map[string]bool{strings.ToLower(gameExe(w)): true}
	w.Files["/Reactor.dll"] = []byte("reactor-v2")
	w.Manifest.Plugins[0] = w.Asset("/Reactor.dll", "Reactor.dll", "Reactor")
	if err := run(w, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if len(w.Fake.Started) != 0 || w.UI.Called("progress:") {
		t.Fatal("synced or started while game running")
	}
}

func TestLaunchSelfUpdateRestarts(t *testing.T) {
	w := installed(t)
	w.Files["/CleanInstaller.exe"] = []byte("self-exe-v2")
	w.Manifest.Launcher = w.Asset("/CleanInstaller.exe", "CleanInstaller.exe", "")
	w.Manifest.Launcher.Version = "2.0.0"
	if err := run(w, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(w.InstalledExe()); string(b) != "self-exe-v2" {
		t.Fatalf("exe %q", b)
	}
	if len(w.Fake.Started) != 1 || w.Fake.Started[0].Exe != w.InstalledExe() || len(w.Fake.Started[0].Args) != 1 || w.Fake.Started[0].Args[0] != "--launch" {
		t.Fatalf("started %+v", w.Fake.Started)
	}
}

func TestLaunchIgnoresJokeMode(t *testing.T) {
	w := installed(t)
	w.SetFun(true)
	w.Files["/Reactor.dll"] = []byte("reactor-v2")
	w.Manifest.Plugins[0] = w.Asset("/Reactor.dll", "Reactor.dll", "Reactor")
	if err := run(w, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if len(w.UI.Displays) != 1 || w.UI.Displays[0].JokeText || w.UI.Displays[0].ChaosBar {
		t.Fatalf("displays %+v", w.UI.Displays)
	}
}

func TestLaunchNotInstalled(t *testing.T) {
	w := testenv.NewWorld(t, 1, true)
	err := run(w, "1.0.0")
	if err == nil || !strings.Contains(apperr.Message(err), "nicht installiert") {
		t.Fatalf("%v", err)
	}
}

func TestLaunchGameUpdateRebuilds(t *testing.T) {
	w := installed(t)
	testenv.WriteFile(t, filepath.Join(w.Lib, "steamapps", "appmanifest_945360.acf"), `"AppState" { "buildid" "101" }`)
	testenv.WriteFile(t, filepath.Join(w.Game.VanillaDir, "Among Us.exe"), "game-v2")
	if err := run(w, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(gameExe(w)); string(b) != "game-v2" {
		t.Fatalf("game %q", b)
	}
}
