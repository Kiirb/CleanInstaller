// Package migrate finds and removes what the old C# CInstaller left behind:
// its Steam shortcut, its artwork, and its "Among Us Modded" folder layout.
package migrate

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/Kiirb/CleanInstaller/internal/steam"
	"github.com/Kiirb/CleanInstaller/internal/vdf"
)

// What the old tool wrote (see Kiirb/CInstaller).
const (
	OldAppID     int32 = -305070
	OldArtPrefix       = "4294662226"
	OldExe             = "CInstaller.exe"
	OldConfig          = "CInstallerConfig.json"
)

// Findings lists what was found, by Steam user ID3.
type Findings struct {
	ShortcutUsers []uint32
	ArtUsers      []uint32
	OldFolder     string // set if moddedDir holds the old tool's config
}

// Any reports whether there is anything to migrate.
func (f Findings) Any() bool {
	return len(f.ShortcutUsers) > 0 || len(f.ArtUsers) > 0 || f.OldFolder != ""
}

// OldShortcutMatcher matches the old tool's shortcut entries.
func OldShortcutMatcher(e *vdf.BNode) bool {
	if x := e.Child("Exe"); x != nil {
		exe := strings.Trim(x.Str, `"`)
		if strings.EqualFold(filepath.Base(exe), OldExe) {
			return true
		}
	}
	id, name := e.Child("appid"), e.Child("AppName")
	return id != nil && name != nil && id.Int == OldAppID && name.Str == steam.ShortcutName
}

// Detect checks every user, not only the one being installed for.
func Detect(steamRoot string, users []steam.User, moddedDir string) (Findings, error) {
	var f Findings
	for _, u := range users {
		cfg := steam.UserConfigDir(steamRoot, u)
		root, err := steam.LoadShortcuts(cfg)
		if err != nil {
			return f, err
		}
		for _, e := range root.Child("shortcuts").Children {
			if e.Type == vdf.TypeMap && OldShortcutMatcher(e) {
				f.ShortcutUsers = append(f.ShortcutUsers, u.ID3)
				break
			}
		}
		if _, err := os.Stat(filepath.Join(cfg, "grid", OldArtPrefix+"p.png")); err == nil {
			f.ArtUsers = append(f.ArtUsers, u.ID3)
		}
	}
	if _, err := os.Stat(filepath.Join(moddedDir, OldConfig)); err == nil {
		f.OldFolder = moddedDir
	}
	return f, nil
}

// CleanSteam removes old shortcuts and artwork. Steam must not be running.
func CleanSteam(steamRoot string, f Findings, log *slog.Logger) error {
	for _, id := range f.ShortcutUsers {
		cfg := steam.UserConfigDir(steamRoot, steam.User{ID3: id})
		root, err := steam.LoadShortcuts(cfg)
		if err != nil {
			return err
		}
		n := steam.RemoveShortcuts(root, OldShortcutMatcher)
		log.Info("removed old CInstaller shortcuts", "user", id, "count", n)
		if err := steam.SaveShortcuts(cfg, root); err != nil {
			return err
		}
	}
	for _, id := range f.ArtUsers {
		cfg := steam.UserConfigDir(steamRoot, steam.User{ID3: id})
		if err := steam.RemoveArtwork(cfg, OldArtPrefix); err != nil {
			return err
		}
		log.Info("removed old CInstaller artwork", "user", id)
	}
	return nil
}
