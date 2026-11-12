package fsutil

import (
	"archive/zip"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

func TestCopyTreeExcludes(t *testing.T) {
	src, dst := t.TempDir(), filepath.Join(t.TempDir(), "out")
	write(t, filepath.Join(src, "Among Us.exe"), "game")
	write(t, filepath.Join(src, "winhttp.dll"), "x")
	write(t, filepath.Join(src, "WINHTTP.DLL.bak"), "keep")
	write(t, filepath.Join(src, "BepInEx", "core", "a.dll"), "x")
	write(t, filepath.Join(src, "data", "BepInEx", "b.txt"), "nested")
	var lastDone, lastTotal int64
	if err := CopyTree(src, dst, VanillaExcludes, func(d, tot int64) { lastDone, lastTotal = d, tot }); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(dst, "Among Us.exe")) != "game" || read(t, filepath.Join(dst, "data", "BepInEx", "b.txt")) != "nested" {
		t.Fatal("missing copied files")
	}
	if exists(filepath.Join(dst, "winhttp.dll")) || exists(filepath.Join(dst, "BepInEx")) {
		t.Fatal("excluded files copied")
	}
	if !exists(filepath.Join(dst, "WINHTTP.DLL.bak")) {
		t.Fatal("non-excluded file skipped")
	}
	if lastDone != lastTotal || lastTotal == 0 {
		t.Fatalf("progress %d/%d", lastDone, lastTotal)
	}
}

func makeZip(t *testing.T, entries map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "x.zip")
	f, _ := os.Create(p)
	zw := zip.NewWriter(f)
	names := make([]string, 0, len(entries))
	for n := range entries {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(entries[n]))
	}
	_ = zw.Close()
	_ = f.Close()
	return p
}

func TestExtractZipStripsSingleRoot(t *testing.T) {
	z := makeZip(t, map[string]string{"Pack/": "", "Pack/BepInEx/plugins/x.dll": "x", "Pack/winhttp.dll": "w"})
	dst := t.TempDir()
	files, err := ExtractZip(z, dst, nil)
	if err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(dst, "BepInEx", "plugins", "x.dll")) != "x" || !exists(filepath.Join(dst, "winhttp.dll")) {
		t.Fatal("not stripped")
	}
	sort.Strings(files)
	if len(files) != 2 || files[0] != "BepInEx/plugins/x.dll" || files[1] != "winhttp.dll" {
		t.Fatalf("files %v", files)
	}
}

func TestExtractZipNoStripMultiRoot(t *testing.T) {
	z := makeZip(t, map[string]string{"BepInEx/a.dll": "a", "winhttp.dll": "w"})
	dst := t.TempDir()
	if _, err := ExtractZip(z, dst, nil); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(dst, "BepInEx", "a.dll")) || !exists(filepath.Join(dst, "winhttp.dll")) {
		t.Fatal("wrong layout")
	}
}

func TestExtractZipSingleTopLevelFileNotStripped(t *testing.T) {
	z := makeZip(t, map[string]string{"only.dll": "a"})
	dst := t.TempDir()
	if _, err := ExtractZip(z, dst, nil); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(dst, "only.dll")) {
		t.Fatal("file lost")
	}
}

func TestExtractZipSlip(t *testing.T) {
	parent := t.TempDir()
	dst := filepath.Join(parent, "dst")
	for _, bad := range []string{"../evil.txt", "a/../../evil.txt", `..\evil.txt`, "/abs.txt", "C:/x.txt"} {
		z := makeZip(t, map[string]string{"ok.txt": "o", bad: "evil"})
		if _, err := ExtractZip(z, dst, nil); !errors.Is(err, ErrZipSlip) {
			t.Errorf("%q: err %v", bad, err)
		}
		if exists(filepath.Join(parent, "evil.txt")) {
			t.Fatalf("%q escaped", bad)
		}
	}
}

func TestMergeDirKeepExisting(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "keybinds.cfg"), "pack default")
	write(t, filepath.Join(src, "sub", "new.cfg"), "new")
	write(t, filepath.Join(dst, "keybinds.cfg"), "user")
	if err := MergeDirKeepExisting(src, dst); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(dst, "keybinds.cfg")) != "user" || read(t, filepath.Join(dst, "sub", "new.cfg")) != "new" {
		t.Fatal("merge wrong")
	}
	if err := MergeDirKeepExisting(filepath.Join(src, "missing"), dst); err != nil {
		t.Fatalf("missing src: %v", err)
	}
}
