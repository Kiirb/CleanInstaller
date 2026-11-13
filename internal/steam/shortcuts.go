package steam

import (
	"bytes"
	"errors"
	"fmt"
	"hash/crc32"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Kiirb/CleanInstaller/internal/vdf"
)

// ShortcutName is the name shown in the Steam library.
const ShortcutName = "Among Us Modded"

// Shortcut is a non-Steam game entry. Exe and StartDir are unquoted here and
// quoted when written, as Steam does.
type Shortcut struct {
	Exe, StartDir, LaunchOptions, Icon string
}

// ShortcutAppID is Steam's id scheme for non-Steam games: CRC32 of the quoted
// exe plus the name, with the high bit set.
func ShortcutAppID(exeQuoted, name string) uint32 {
	return crc32.ChecksumIEEE([]byte(exeQuoted+name)) | 0x80000000
}

func itoa(id uint32) string { return strconv.FormatUint(uint64(id), 10) }

func quote(s string) string { return `"` + s + `"` }

func unquote(s string) string { return strings.Trim(s, `"`) }

func shortcutsNode(root *vdf.BNode) *vdf.BNode {
	sc := root.Child("shortcuts")
	if sc == nil {
		sc = &vdf.BNode{Key: "shortcuts", Type: vdf.TypeMap, Children: []*vdf.BNode{}}
		root.Children = append(root.Children, sc)
	}
	return sc
}

// RemoveShortcuts deletes entries for which match is true and renumbers the
// rest "0".."n-1" so a later append can never overwrite another entry.
func RemoveShortcuts(root *vdf.BNode, match func(entry *vdf.BNode) bool) int {
	sc := shortcutsNode(root)
	kept := sc.Children[:0]
	removed := 0
	for _, e := range sc.Children {
		if e.Type == vdf.TypeMap && match(e) {
			removed++
			continue
		}
		kept = append(kept, e)
	}
	sc.Children = kept
	for i, e := range sc.Children {
		e.Key = strconv.Itoa(i)
	}
	return removed
}

// AddShortcut appends our entry and returns its appid.
func AddShortcut(root *vdf.BNode, s Shortcut) uint32 {
	sc := shortcutsNode(root)
	exeQ := quote(s.Exe)
	id := ShortcutAppID(exeQ, ShortcutName)
	str := func(k, v string) *vdf.BNode { return &vdf.BNode{Key: k, Type: vdf.TypeString, Str: v} }
	i32 := func(k string, v int32) *vdf.BNode { return &vdf.BNode{Key: k, Type: vdf.TypeInt32, Int: v} }
	e := &vdf.BNode{Key: strconv.Itoa(len(sc.Children)), Type: vdf.TypeMap, Children: []*vdf.BNode{
		i32("appid", int32(id)),
		str("AppName", ShortcutName),
		str("Exe", exeQ),
		str("StartDir", quote(s.StartDir)),
		str("icon", s.Icon),
		str("ShortcutPath", ""),
		str("LaunchOptions", s.LaunchOptions),
		i32("IsHidden", 0),
		i32("AllowDesktopConfig", 1),
		i32("AllowOverlay", 1),
		i32("OpenVR", 0),
		i32("Devkit", 0),
		str("DevkitGameID", ""),
		i32("DevkitOverrideAppID", 0),
		i32("LastPlayTime", 0),
		str("FlatpakAppID", ""),
		str("sortas", ""),
		{Key: "tags", Type: vdf.TypeMap, Children: []*vdf.BNode{}},
	}}
	sc.Children = append(sc.Children, e)
	return id
}

// LoadShortcuts reads <userConfigDir>\shortcuts.vdf; a missing file yields an
// empty root.
func LoadShortcuts(userConfigDir string) (*vdf.BNode, error) {
	f, err := os.Open(filepath.Join(userConfigDir, "shortcuts.vdf"))
	if errors.Is(err, fs.ErrNotExist) {
		root := &vdf.BNode{Type: vdf.TypeMap}
		shortcutsNode(root)
		return root, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	root, err := vdf.ReadBinary(f)
	if err != nil {
		return nil, fmt.Errorf("shortcuts.vdf: %w", err)
	}
	shortcutsNode(root)
	return root, nil
}

// SaveShortcuts writes via a temp file and keeps one backup of the old file.
func SaveShortcuts(userConfigDir string, root *vdf.BNode) error {
	if err := os.MkdirAll(userConfigDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(userConfigDir, "shortcuts.vdf")
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := vdf.WriteBinary(f, root); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if _, err := os.Stat(path); err == nil {
		if err := copyFile(path, path+".bak"); err != nil {
			os.Remove(tmp)
			return err
		}
	}
	return os.Rename(tmp, path)
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

var artFiles = []struct{ src, suffix string }{
	{"steam/grid.png", "p.png"},
	{"steam/hero.png", "_hero.png"},
	{"steam/logo.png", "_logo.png"},
	{"steam/icon.png", "_icon.png"},
	{"steam/logo.json", ".json"},
}

// WriteArtwork writes the custom library images for appID.
func WriteArtwork(userConfigDir string, appID uint32, art fs.FS) error {
	grid := filepath.Join(userConfigDir, "grid")
	if err := os.MkdirAll(grid, 0o755); err != nil {
		return err
	}
	for _, a := range artFiles {
		b, err := fs.ReadFile(art, a.src)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(grid, itoa(appID)+a.suffix), b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// SyncArtwork rewrites only the artwork files that are missing or differ
// from art and returns how many it wrote. It never touches shortcuts.vdf, so
// it is safe while Steam is running.
func SyncArtwork(userConfigDir string, appID uint32, art fs.FS) (int, error) {
	grid := filepath.Join(userConfigDir, "grid")
	if err := os.MkdirAll(grid, 0o755); err != nil {
		return 0, err
	}
	written := 0
	for _, a := range artFiles {
		want, err := fs.ReadFile(art, a.src)
		if err != nil {
			return written, err
		}
		path := filepath.Join(grid, itoa(appID)+a.suffix)
		if have, err := os.ReadFile(path); err == nil && bytes.Equal(have, want) {
			continue
		}
		if err := os.WriteFile(path, want, 0o644); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}

// OurShortcutAppIDs returns the appids of our entries in a shortcuts root.
func OurShortcutAppIDs(root *vdf.BNode, exePath, startDir string) []uint32 {
	match := OurShortcutMatcher(exePath, startDir)
	var ids []uint32
	for _, e := range shortcutsNode(root).Children {
		if e.Type != vdf.TypeMap || !match(e) {
			continue
		}
		if id := e.Child("appid"); id != nil && id.Type == vdf.TypeInt32 {
			ids = append(ids, uint32(id.Int))
		}
	}
	return ids
}

// IconPath is where WriteArtwork puts the icon for appID.
func IconPath(userConfigDir string, appID uint32) string {
	return filepath.Join(userConfigDir, "grid", itoa(appID)+"_icon.png")
}

// RemoveArtwork deletes the five artwork files named with prefix.
func RemoveArtwork(userConfigDir, prefix string) error {
	for _, a := range artFiles {
		err := os.Remove(filepath.Join(userConfigDir, "grid", prefix+a.suffix))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

// OurShortcutMatcher matches entries whose Exe or StartDir point at us.
func OurShortcutMatcher(exePath, startDir string) func(*vdf.BNode) bool {
	exe, dir := filepath.Clean(exePath), filepath.Clean(startDir)
	return func(e *vdf.BNode) bool {
		if x := e.Child("Exe"); x != nil && strings.EqualFold(filepath.Clean(unquote(x.Str)), exe) {
			return true
		}
		if d := e.Child("StartDir"); d != nil && strings.EqualFold(filepath.Clean(unquote(d.Str)), dir) {
			return true
		}
		return false
	}
}
