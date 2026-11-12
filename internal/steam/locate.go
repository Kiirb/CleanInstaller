// Package steam finds the Steam libraries, the Among Us install, Steam users,
// and edits non-Steam shortcuts.
package steam

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Kiirb/CleanInstaller/internal/vdf"
)

// Among Us and Steam constants.
const (
	AppID          = "945360"
	GameFolder     = "Among Us"
	GameExe        = "Among Us.exe"
	ModdedFolder   = "Among Us Modded"
	SteamID64Base  = 76561197960265728
	appManifestFmt = "appmanifest_%s.acf"
)

// ErrGameNotInstalled means no Steam library holds Among Us.
var ErrGameNotInstalled = errors.New("among us not installed")

// Game describes where vanilla and modded Among Us live.
type Game struct {
	LibraryDir string
	CommonDir  string
	VanillaDir string
	ModdedDir  string
	BuildID    string
}

// FindGame locates the library holding Among Us and reads its build id.
func FindGame(steamRoot string) (*Game, error) {
	f, err := os.Open(filepath.Join(steamRoot, "steamapps", "libraryfolders.vdf"))
	if err != nil {
		return nil, fmt.Errorf("read libraryfolders.vdf: %w", err)
	}
	defer f.Close()
	root, err := vdf.ParseText(f)
	if err != nil {
		return nil, err
	}
	for _, lib := range root.Child("libraryfolders").Children {
		if lib.Child("apps").Child(AppID) == nil {
			continue
		}
		path, ok := lib.Get("path")
		if !ok {
			continue
		}
		g, err := gameIn(filepath.Clean(path))
		if err == nil {
			return g, nil
		}
		if !errors.Is(err, ErrGameNotInstalled) {
			return nil, err
		}
	}
	return nil, ErrGameNotInstalled
}

func gameIn(lib string) (*Game, error) {
	common := filepath.Join(lib, "steamapps", "common")
	g := &Game{
		LibraryDir: lib,
		CommonDir:  common,
		VanillaDir: filepath.Join(common, GameFolder),
		ModdedDir:  filepath.Join(common, ModdedFolder),
	}
	if st, err := os.Stat(g.VanillaDir); err != nil || !st.IsDir() {
		return nil, ErrGameNotInstalled
	}
	f, err := os.Open(filepath.Join(lib, "steamapps", fmt.Sprintf(appManifestFmt, AppID)))
	if os.IsNotExist(err) {
		return nil, ErrGameNotInstalled
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	am, err := vdf.ParseText(f)
	if err != nil {
		return nil, fmt.Errorf("appmanifest: %w", err)
	}
	g.BuildID, _ = am.Get("AppState", "buildid")
	return g, nil
}
