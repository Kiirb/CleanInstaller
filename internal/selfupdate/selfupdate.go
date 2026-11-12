// Package selfupdate replaces the running exe with a newer, verified one.
// Windows allows renaming a running exe, so no helper process is needed: the
// old file is renamed to .old and deleted on the next start.
package selfupdate

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"

	"golang.org/x/mod/semver"

	"github.com/Kiirb/CleanInstaller/internal/download"
	"github.com/Kiirb/CleanInstaller/internal/manifest"
)

// CleanupOld deletes a leftover "<self>.old" from a previous update.
func CleanupOld(selfPath string) error {
	err := os.Remove(selfPath + ".old")
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// Needed reports whether the manifest's launcher is newer than current.
func Needed(current string, m *manifest.Manifest) bool {
	if m.Launcher.Empty() {
		return false
	}
	cur, latest := "v"+current, "v"+m.Launcher.Version
	if !semver.IsValid(cur) || !semver.IsValid(latest) {
		return false
	}
	return semver.Compare(latest, cur) > 0
}

// Apply downloads and verifies a, then swaps it in for selfPath.
func Apply(ctx context.Context, selfPath string, cache *download.Cache, a manifest.Asset) error {
	src, err := cache.Get(ctx, a, nil)
	if err != nil {
		return err
	}
	// Stage the full new exe next to us first, so an interruption can never
	// leave a truncated CleanInstaller.exe behind.
	staged := selfPath + ".new"
	if err := copyFile(src, staged); err != nil {
		return err
	}
	old := selfPath + ".old"
	_ = os.Remove(old) // a stale .old from an earlier crash; Rename below reports real problems
	if err := os.Rename(selfPath, old); err != nil {
		os.Remove(staged)
		return err
	}
	if err := os.Rename(staged, selfPath); err != nil {
		_ = os.Rename(old, selfPath) // put the working exe back
		os.Remove(staged)
		return err
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}
