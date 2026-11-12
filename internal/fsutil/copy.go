// Package fsutil holds the file operations the sync needs: copying the game,
// extracting the mod pack and merging config folders.
package fsutil

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// VanillaExcludes are root-level names left out when copying the vanilla game,
// so leftovers of an earlier BepInEx install never reach the modded copy.
var VanillaExcludes = []string{"winhttp.dll", "doorstop_config.ini", "changelog.txt", ".doorstop_version", "BepInEx"}

// CopyTree copies src to dst, skipping root-level names in excludeRoot
// (case-insensitive). progress receives bytes copied so far and the total.
func CopyTree(src, dst string, excludeRoot []string, progress func(done, total int64)) error {
	excluded := func(rel string) bool {
		first := strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]
		for _, e := range excludeRoot {
			if strings.EqualFold(first, e) {
				return true
			}
		}
		return false
	}
	type item struct {
		rel  string
		size int64
	}
	var files []item
	var total int64
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if rel == "." {
			return nil
		}
		if excluded(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		files = append(files, item{rel, info.Size()})
		total += info.Size()
		return nil
	})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	var done int64
	for _, f := range files {
		if err := copyFile(filepath.Join(src, f.rel), filepath.Join(dst, f.rel)); err != nil {
			return err
		}
		done += f.size
		if progress != nil {
			progress(done, total)
		}
	}
	return nil
}

// MergeDirKeepExisting copies files from src into dst only where dst has no
// file at that relative path. A missing src is not an error.
func MergeDirKeepExisting(src, dst string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	}
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if _, err := os.Stat(target); err == nil {
			return nil
		}
		return copyFile(p, target)
	})
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
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
