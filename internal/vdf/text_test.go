package vdf

import (
	"os"
	"strings"
	"testing"
)

func mustParse(t *testing.T, path string) *KV {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	root, err := ParseText(f)
	if err != nil {
		t.Fatalf("ParseText(%s): %v", path, err)
	}
	return root
}

func TestParseLibraryFolders(t *testing.T) {
	root := mustParse(t, "testdata/libraryfolders.vdf")
	lib1 := root.Child("libraryfolders").Child("1")
	if lib1 == nil {
		t.Fatal("library 1 missing")
	}
	p, ok := lib1.Get("path")
	if !ok || p != `D:\Spiele\Steam Bibliothek ä` { // Review Focus 1
		t.Fatalf("path %q", p)
	}
	if lib1.Child("apps").Child("945360") == nil {
		t.Fatal("app missing")
	}
	if root.Child("libraryfolders").Child("0").Child("apps").Child("945360") != nil {
		t.Fatal("app wrongly found in library 0")
	}
}

func TestParseGetPath(t *testing.T) {
	root := mustParse(t, "testdata/appmanifest_945360.acf")
	if v, ok := root.Get("AppState", "buildid"); !ok || v != "16000001" {
		t.Fatalf("buildid %q %v", v, ok)
	}
	if _, ok := root.Get("AppState", "nope"); ok {
		t.Fatal("missing key reported present")
	}
}

func TestParseCaseInsensitiveChild(t *testing.T) {
	root := mustParse(t, "testdata/loginusers.vdf")
	u := root.Child("USERS").Child("76561198000000001")
	if v, _ := u.Get("mostrecent"); v != "1" {
		t.Fatalf("MostRecent %q", v)
	}
}

func TestParseEscapes(t *testing.T) {
	root, err := ParseText(strings.NewReader(`"k" "a\"b\\c"`))
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := root.Get("k"); v != `a"b\c` {
		t.Fatalf("got %q", v)
	}
}

func TestParseComments(t *testing.T) {
	src := "// header comment\n\"a\"\n{\n  // inner\n  \"b\" \"1\" // trailing\n}\n"
	root, err := ParseText(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := root.Get("a", "b"); v != "1" {
		t.Fatalf("got %q", v)
	}
}

func TestParseConditional(t *testing.T) {
	root, err := ParseText(strings.NewReader(`"a" "1" [$WIN32] "b" "2"`))
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := root.Get("b"); v != "2" {
		t.Fatalf("got %q", v)
	}
}

func TestParseUnterminated(t *testing.T) {
	for _, src := range []string{`"a" {`, `"a" "b`, `"a"`, `}`} {
		if _, err := ParseText(strings.NewReader(src)); err == nil {
			t.Errorf("no error for %q", src)
		}
	}
}
