package migrate

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Kiirb/CleanInstaller/assets"
	"github.com/Kiirb/CleanInstaller/internal/applog"
	"github.com/Kiirb/CleanInstaller/internal/steam"
	"github.com/Kiirb/CleanInstaller/internal/vdf"
)

func node(appid int32, name, exe string) *vdf.BNode {
	return &vdf.BNode{Type: vdf.TypeMap, Children: []*vdf.BNode{
		{Key: "appid", Type: vdf.TypeInt32, Int: appid},
		{Key: "AppName", Type: vdf.TypeString, Str: name},
		{Key: "Exe", Type: vdf.TypeString, Str: exe},
		{Key: "StartDir", Type: vdf.TypeString, Str: `"D:\"`},
	}}
}

func writeShortcuts(t *testing.T, cfg string, entries ...*vdf.BNode) {
	t.Helper()
	root, _ := steam.LoadShortcuts(cfg)
	sc := root.Child("shortcuts")
	for i, e := range entries {
		e.Key = string(rune('0' + i))
		sc.Children = append(sc.Children, e)
	}
	if err := steam.SaveShortcuts(cfg, root); err != nil {
		t.Fatal(err)
	}
}

var (
	userA = steam.User{ID3: 1, PersonaName: "A", MostRecent: true}
	userB = steam.User{ID3: 2, PersonaName: "B"}
)

const oldExe = `"D:\SteamLibrary\steamapps\common\Among Us Modded\CInstaller.exe"`

func TestOldShortcutMatcher(t *testing.T) {
	if !OldShortcutMatcher(node(123, "Whatever", oldExe)) {
		t.Fatal("exe match")
	}
	if !OldShortcutMatcher(node(OldAppID, "Among Us Modded", `"x.exe"`)) {
		t.Fatal("appid+name match")
	}
	if OldShortcutMatcher(node(OldAppID, "Other Game", `"x.exe"`)) || OldShortcutMatcher(node(5, "Among Us Modded", `"C:\x\CleanInstaller.exe"`)) {
		t.Fatal("false match")
	}
}

func TestDetectAllUsers(t *testing.T) {
	root := t.TempDir()
	writeShortcuts(t, steam.UserConfigDir(root, userA), node(9, "Game", `"g.exe"`))
	writeShortcuts(t, steam.UserConfigDir(root, userB), node(OldAppID, "Among Us Modded", oldExe))
	_ = os.MkdirAll(filepath.Join(steam.UserConfigDir(root, userA), "grid"), 0o755)
	_ = os.WriteFile(filepath.Join(steam.UserConfigDir(root, userA), "grid", OldArtPrefix+"p.png"), []byte("x"), 0o644)
	f, err := Detect(root, []steam.User{userA, userB}, filepath.Join(t.TempDir(), "Among Us Modded"))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.ShortcutUsers) != 1 || f.ShortcutUsers[0] != 2 || len(f.ArtUsers) != 1 || f.ArtUsers[0] != 1 || f.OldFolder != "" || !f.Any() {
		t.Fatalf("%+v", f)
	}
}

func TestDetectNothing(t *testing.T) {
	root := t.TempDir()
	f, err := Detect(root, []steam.User{userA}, filepath.Join(t.TempDir(), "Among Us Modded"))
	if err != nil || f.Any() {
		t.Fatalf("%v %+v", err, f)
	}
}

func TestDetectOldFolder(t *testing.T) {
	modded := filepath.Join(t.TempDir(), "Among Us Modded")
	_ = os.MkdirAll(modded, 0o755)
	_ = os.WriteFile(filepath.Join(modded, OldConfig), []byte("{}"), 0o644)
	f, err := Detect(t.TempDir(), nil, modded)
	if err != nil || f.OldFolder != modded || !f.Any() {
		t.Fatalf("%v %+v", err, f)
	}
}

func TestCleanSteamKeepsOtherShortcuts(t *testing.T) {
	root := t.TempDir()
	cfg := steam.UserConfigDir(root, userB)
	writeShortcuts(t, cfg, node(9, "Game1", `"g1.exe"`), node(OldAppID, "Among Us Modded", oldExe), node(10, "Game2", `"g2.exe"`))
	before, _ := steam.LoadShortcuts(cfg)
	g2 := before.Child("shortcuts").Child("2")
	var g2bytes bytes.Buffer
	_ = vdf.WriteBinary(&g2bytes, &vdf.BNode{Children: []*vdf.BNode{{Key: "x", Type: vdf.TypeMap, Children: g2.Children}}})

	f := Findings{ShortcutUsers: []uint32{2}}
	if err := CleanSteam(root, f, applog.Discard()); err != nil {
		t.Fatal(err)
	}
	after, _ := steam.LoadShortcuts(cfg)
	sc := after.Child("shortcuts").Children
	if len(sc) != 2 || sc[0].Key != "0" || sc[1].Key != "1" || sc[1].Child("AppName").Str != "Game2" {
		t.Fatalf("%+v", sc)
	}
	var now bytes.Buffer
	_ = vdf.WriteBinary(&now, &vdf.BNode{Children: []*vdf.BNode{{Key: "x", Type: vdf.TypeMap, Children: sc[1].Children}}})
	if !bytes.Equal(now.Bytes(), g2bytes.Bytes()) {
		t.Fatal("other entry changed")
	}
}

func TestCleanSteamRemovesArt(t *testing.T) {
	root := t.TempDir()
	cfg := steam.UserConfigDir(root, userA)
	if err := steam.WriteArtwork(cfg, 4294662226, assets.Steam); err != nil {
		t.Fatal(err)
	}
	if err := CleanSteam(root, Findings{ArtUsers: []uint32{1}}, applog.Discard()); err != nil {
		t.Fatal(err)
	}
	es, _ := os.ReadDir(filepath.Join(cfg, "grid"))
	if len(es) != 0 {
		t.Fatalf("left %v", es)
	}
}
