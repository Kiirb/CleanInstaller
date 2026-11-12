package steam

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func copyTestdata(t *testing.T, name, dst string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "vdf", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeSteam builds a Steam root whose libraryfolders.vdf points library "1"
// at lib (a temp dir) holding Among Us.
func fakeSteam(t *testing.T, withGame bool) (root, lib string) {
	t.Helper()
	root, lib = t.TempDir(), t.TempDir()
	vdfText := `"libraryfolders"
{
	"0" { "path" "` + strings.ReplaceAll(root, `\`, `\\`) + `" "apps" { "228980" "1" } }
	"1" { "path" "` + strings.ReplaceAll(lib, `\`, `\\`) + `" "apps" { "945360" "1" } }
}`
	if err := os.MkdirAll(filepath.Join(root, "steamapps"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, "steamapps", "libraryfolders.vdf"), []byte(vdfText), 0o644)
	if withGame {
		copyTestdata(t, "appmanifest_945360.acf", filepath.Join(lib, "steamapps", "appmanifest_945360.acf"))
		_ = os.MkdirAll(filepath.Join(lib, "steamapps", "common", "Among Us"), 0o755)
	}
	return root, lib
}

func TestFindGameSecondLibrary(t *testing.T) {
	root, lib := fakeSteam(t, true)
	g, err := FindGame(root)
	if err != nil {
		t.Fatal(err)
	}
	common := filepath.Join(lib, "steamapps", "common")
	if g.LibraryDir != lib || g.CommonDir != common ||
		g.VanillaDir != filepath.Join(common, "Among Us") ||
		g.ModdedDir != filepath.Join(common, "Among Us Modded") || g.BuildID != "16000001" {
		t.Fatalf("game %+v", g)
	}
}

func TestFindGameNotInstalled(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "steamapps"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "steamapps", "libraryfolders.vdf"), []byte(`"libraryfolders" { "0" { "path" "x" "apps" { } } }`), 0o644)
	if _, err := FindGame(root); !errors.Is(err, ErrGameNotInstalled) {
		t.Fatalf("err %v", err)
	}
}

func TestFindGameListedButFolderMissing(t *testing.T) {
	root, _ := fakeSteam(t, false)
	if _, err := FindGame(root); !errors.Is(err, ErrGameNotInstalled) {
		t.Fatalf("err %v", err)
	}
}

func TestUsersID3(t *testing.T) {
	root := t.TempDir()
	copyTestdata(t, "loginusers.vdf", filepath.Join(root, "config", "loginusers.vdf"))
	_ = os.MkdirAll(filepath.Join(root, "userdata", "39734272"), 0o755)
	_ = os.MkdirAll(filepath.Join(root, "userdata", "39734273"), 0o755)
	us, err := Users(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(us) != 2 || us[0].ID3 != 39734272 || us[0].ID64 != 76561198000000000 || us[0].PersonaName != "Alice" || us[0].MostRecent {
		t.Fatalf("users %+v", us)
	}
	if !us[1].MostRecent || us[1].PersonaName != "Bob" {
		t.Fatalf("user1 %+v", us[1])
	}
	if got := UserConfigDir(root, us[0]); got != filepath.Join(root, "userdata", "39734272", "config") {
		t.Fatal(got)
	}
}

func TestUsersFiltersMissingUserdata(t *testing.T) {
	root := t.TempDir()
	copyTestdata(t, "loginusers.vdf", filepath.Join(root, "config", "loginusers.vdf"))
	_ = os.MkdirAll(filepath.Join(root, "userdata", "39734273"), 0o755)
	us, err := Users(root)
	if err != nil || len(us) != 1 || us[0].PersonaName != "Bob" {
		t.Fatalf("%v %+v", err, us)
	}
}
