// Package syncer makes the "Among Us Modded" folder match the manifest. Both
// the installer and the launcher use it.
package syncer

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

const stateFile = "state.json"

// State records what was installed, so the next run knows what changed.
type State struct {
	BuildID     string            `json:"buildId"`
	ModpackSHA  string            `json:"modpackSha"`
	Plugins     map[string]string `json:"plugins"` // fileName -> sha256
	ModdedDir   string            `json:"moddedDir"`
	PackPlugins []string          `json:"packPlugins"` // lower-case file names the mod pack put in BepInEx/plugins
}

// LoadState returns (nil, nil) when no state exists yet.
func LoadState(dataDir string) (*State, error) {
	b, err := os.ReadFile(filepath.Join(dataDir, stateFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// SaveState writes state.json atomically.
func SaveState(dataDir string, s *State) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	p := filepath.Join(dataDir, stateFile)
	if err := os.WriteFile(p+".tmp", b, 0o644); err != nil {
		return err
	}
	return os.Rename(p+".tmp", p)
}

// DeleteState forces the next sync to rebuild.
func DeleteState(dataDir string) error {
	err := os.Remove(filepath.Join(dataDir, stateFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
