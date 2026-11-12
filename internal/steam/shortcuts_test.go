package steam

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Kiirb/CleanInstaller/assets"
	"github.com/Kiirb/CleanInstaller/internal/vdf"
)

func entry(appName, exe, dir string) *vdf.BNode {
	return &vdf.BNode{Type: vdf.TypeMap, Children: []*vdf.BNode{
		{Key: "AppName", Type: vdf.TypeString, Str: appName},
		{Key: "Exe", Type: vdf.TypeString, Str: exe},
		{Key: "StartDir", Type: vdf.TypeString, Str: dir},
	}}
}

func rootWith(entries ...*vdf.BNode) *vdf.BNode {
	sc := &vdf.BNode{Key: "shortcuts", Type: vdf.TypeMap}
	for i, e := range entries {
		e.Key = string(rune('0' + i))
		sc.Children = append(sc.Children, e)
	}
	return &vdf.BNode{Type: vdf.TypeMap, Children: []*vdf.BNode{sc}}
}

func keysAndNames(root *vdf.BNode) (keys, names []string) {
	for _, c := range root.Child("shortcuts").Children {
		keys = append(keys, c.Key)
		names = append(names, c.Child("AppName").Str)
	}
	return
}

func TestShortcutAppIDHighBit(t *testing.T) {
	a := ShortcutAppID(`"C:\x\CleanInstaller.exe"`, ShortcutName)
	if a&0x80000000 == 0 {
		t.Fatalf("%x", a)
	}
	if a != ShortcutAppID(`"C:\x\CleanInstaller.exe"`, ShortcutName) {
		t.Fatal("not deterministic")
	}
	if a == ShortcutAppID(`"C:\y\CleanInstaller.exe"`, ShortcutName) {
		t.Fatal("no dependence on exe")
	}
}

func TestRemoveShortcutsRenumbers(t *testing.T) {
	root := rootWith(entry("A", "a", "d"), entry("B", "b", "d"), entry("C", "c", "d"))
	n := RemoveShortcuts(root, func(e *vdf.BNode) bool { return e.Child("AppName").Str == "B" })
	keys, names := keysAndNames(root)
	if n != 1 || len(keys) != 2 || keys[0] != "0" || keys[1] != "1" || names[1] != "C" {
		t.Fatalf("n=%d keys=%v names=%v", n, keys, names)
	}
}

func TestAddShortcutNoCollision(t *testing.T) {
	root := rootWith(entry("A", "a", "d"), entry("B", "b", "d"), entry("C", "c", "d"))
	RemoveShortcuts(root, func(e *vdf.BNode) bool { return e.Child("AppName").Str == "A" })
	id := AddShortcut(root, Shortcut{Exe: `C:\x\CleanInstaller.exe`, StartDir: `C:\x`, LaunchOptions: "--launch", Icon: `C:\i.png`})
	keys, names := keysAndNames(root)
	if len(keys) != 3 || keys[2] != "2" || names[0] != "B" || names[1] != "C" || names[2] != ShortcutName {
		t.Fatalf("keys=%v names=%v", keys, names)
	}
	e := root.Child("shortcuts").Child("2")
	if uint32(e.Child("appid").Int) != id || e.Child("Exe").Str != `"C:\x\CleanInstaller.exe"` || e.Child("StartDir").Str != `"C:\x"` ||
		e.Child("LaunchOptions").Str != "--launch" || e.Child("icon").Str != `C:\i.png` || e.Child("tags").Type != vdf.TypeMap {
		t.Fatalf("entry %+v", e)
	}
	if id != ShortcutAppID(`"C:\x\CleanInstaller.exe"`, ShortcutName) {
		t.Fatal("appid mismatch")
	}
}

func TestLoadShortcutsMissingFile(t *testing.T) { // Review Focus 4
	cfg := filepath.Join(t.TempDir(), "userdata", "1", "config") // does not exist yet
	root, err := LoadShortcuts(cfg)
	if err != nil {
		t.Fatal(err)
	}
	id := AddShortcut(root, Shortcut{Exe: `C:\x\a.exe`, StartDir: `C:\x`, LaunchOptions: "--launch", Icon: IconPath(cfg, 1)})
	if err := SaveShortcuts(cfg, root); err != nil {
		t.Fatal(err)
	}
	if err := WriteArtwork(cfg, id, assets.Steam); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"shortcuts.vdf", filepath.Join("grid", itoa(id)+"p.png"), filepath.Join("grid", itoa(id)+"_hero.png"),
		filepath.Join("grid", itoa(id)+"_logo.png"), filepath.Join("grid", itoa(id)+"_icon.png"), filepath.Join("grid", itoa(id)+".json")} {
		if _, err := os.Stat(filepath.Join(cfg, f)); err != nil {
			t.Errorf("missing %s", f)
		}
	}
	if IconPath(cfg, id) != filepath.Join(cfg, "grid", itoa(id)+"_icon.png") {
		t.Fatal(IconPath(cfg, id))
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	cfg := t.TempDir()
	root := rootWith(entry("Other", `"D:\o.exe"`, `"D:\"`))
	AddShortcut(root, Shortcut{Exe: `C:\x\a.exe`, StartDir: `C:\x`, LaunchOptions: "--launch"})
	if err := SaveShortcuts(cfg, root); err != nil {
		t.Fatal(err)
	}
	// second save keeps a backup of the previous file
	if err := SaveShortcuts(cfg, root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cfg, "shortcuts.vdf.bak")); err != nil {
		t.Fatal("no backup")
	}
	got, err := LoadShortcuts(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var a, b bytes.Buffer
	_ = vdf.WriteBinary(&a, root)
	_ = vdf.WriteBinary(&b, got)
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Fatal("round trip differs")
	}
}

func TestOurShortcutMatcherQuoted(t *testing.T) {
	m := OurShortcutMatcher(`C:\Users\a\AppData\Local\CleanInstaller\CleanInstaller.exe`, `C:\Users\a\AppData\Local\CleanInstaller`)
	if !m(entry("x", `"c:\users\a\appdata\local\cleaninstaller\CLEANINSTALLER.exe"`, `"elsewhere"`)) {
		t.Fatal("exe match failed")
	}
	if !m(entry("x", `"other.exe"`, `"C:\Users\a\AppData\Local\CleanInstaller"`)) {
		t.Fatal("startdir match failed")
	}
	if m(entry("x", `"other.exe"`, `"D:\"`)) {
		t.Fatal("false match")
	}
}

func TestRemoveArtwork(t *testing.T) {
	cfg := t.TempDir()
	if err := WriteArtwork(cfg, 4294662226, assets.Steam); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(cfg, "grid", "123p.png")
	_ = os.WriteFile(keep, []byte("x"), 0o644)
	if err := RemoveArtwork(cfg, "4294662226"); err != nil {
		t.Fatal(err)
	}
	es, _ := os.ReadDir(filepath.Join(cfg, "grid"))
	if len(es) != 1 || es[0].Name() != "123p.png" {
		t.Fatalf("left %v", es)
	}
	if err := RemoveArtwork(filepath.Join(cfg, "missing"), "1"); err != nil {
		t.Fatalf("missing dir: %v", err)
	}
}
