package manifest

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

var sha = strings.Repeat("ab", 32)

func validManifest() *Manifest {
	return &Manifest{
		Schema:      1,
		GeneratedAt: time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC),
		Launcher:    Asset{Version: "1.0.0", Tag: "v1.0.0", URL: "https://x/l.exe", SHA256: sha, Size: 1},
		Modpack:     Asset{Tag: "v5", URL: "https://x/p.zip", SHA256: sha, Size: 2},
		Plugins: []Asset{
			{ID: "Reactor", Tag: "v2", FileName: "Reactor.dll", URL: "https://x/r.dll", SHA256: sha, Size: 3},
		},
		Remove: []string{"BepInEx/plugins/Old.dll"},
		FunRaw: json.RawMessage(`{"jokeText":true,"chaosBar":false,"jokeLines":["a","b"]}`),
	}
}

func TestValidManifestOK(t *testing.T) {
	if err := validManifest().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsBadPaths(t *testing.T) { // Review Focus 5
	for _, bad := range []string{"../x.dll", "a/b.dll", `a\b.dll`, `C:\x.dll`, "", "..", "C:x.dll"} {
		m := validManifest()
		m.Plugins[0].FileName = bad
		if m.Validate() == nil {
			t.Errorf("accepted fileName %q", bad)
		}
	}
	for _, bad := range []string{"../x", `C:\x`, "/abs", `\abs`, "BepInEx/../../x", "a//b", ""} {
		m := validManifest()
		m.Remove = []string{bad}
		if m.Validate() == nil {
			t.Errorf("accepted remove %q", bad)
		}
	}
}

func TestValidateRequiresSHA(t *testing.T) {
	for _, bad := range []string{"", "abc", strings.Repeat("AB", 32), strings.Repeat("zz", 32)} {
		m := validManifest()
		m.Plugins[0].SHA256 = bad
		if m.Validate() == nil {
			t.Errorf("accepted sha %q", bad)
		}
	}
}

func TestValidateDuplicateFileName(t *testing.T) {
	m := validManifest()
	p := m.Plugins[0]
	p.ID = "Other"
	p.FileName = "reactor.DLL"
	m.Plugins = append(m.Plugins, p)
	if m.Validate() == nil {
		t.Fatal("accepted duplicate fileName")
	}
}

func TestParseSchemaTooNew(t *testing.T) {
	b, _ := Marshal(validManifest())
	b = []byte(strings.Replace(string(b), `"schema": 1`, `"schema": 2`, 1))
	if _, err := Parse(b); !errors.Is(err, ErrSchemaTooNew) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseIgnoresUnknownFields(t *testing.T) {
	b, _ := Marshal(validManifest())
	b = []byte(strings.Replace(string(b), `"schema": 1`, `"schema": 1, "future": {"x": 1}`, 1))
	if _, err := Parse(b); err != nil {
		t.Fatal(err)
	}
}

func TestFunDecode(t *testing.T) {
	f := validManifest().Fun()
	if !f.JokeText || f.ChaosBar || !reflect.DeepEqual(f.JokeLines, []string{"a", "b"}) {
		t.Fatalf("fun %+v", f)
	}
	m := validManifest()
	m.FunRaw = json.RawMessage(`{"jokeText": "yes"}`)
	if !reflect.DeepEqual(m.Fun(), Fun{}) {
		t.Fatal("invalid fun not zero")
	}
	m.FunRaw = nil
	if !reflect.DeepEqual(m.Fun(), Fun{}) {
		t.Fatal("absent fun not zero")
	}
}

func TestMarshalKeepsFunContent(t *testing.T) {
	m := validManifest()
	m.FunRaw = json.RawMessage(`{"jokeText":true,"chaosBar":true,"jokeLines":["ä"],"custom":42}`)
	b, err := Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	m2, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	var a, c any
	_ = json.Unmarshal(m.FunRaw, &a)
	_ = json.Unmarshal(m2.FunRaw, &c)
	if !reflect.DeepEqual(a, c) {
		t.Fatalf("fun changed: %s", m2.FunRaw)
	}
	if !strings.HasSuffix(string(b), "}\n") {
		t.Fatal("no trailing newline")
	}
}

func TestParseSourcesDuplicateID(t *testing.T) {
	src := `{"modpack":{"id":"P","repo":"a/b","assetPattern":".zip"},
	"launcher":{"id":"L","repo":"a/c","assetPattern":".exe"},
	"plugins":[{"id":"X","repo":"a/d"},{"id":"X","repo":"a/e"}],"remove":[]}`
	if _, err := ParseSources([]byte(src)); err == nil {
		t.Fatal("accepted duplicate id")
	}
}

func TestParseSourcesDefaultsAndRepoFormat(t *testing.T) {
	src := `{"modpack":{"id":"P","repo":"a/b","assetPattern":".zip"},
	"launcher":{"id":"L","repo":"a/c","assetPattern":".exe"},
	"plugins":[{"id":"X","repo":"a/d"}],"remove":[]}`
	s, err := ParseSources([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if s.Plugins[0].AssetPattern != ".dll" {
		t.Fatalf("default pattern %q", s.Plugins[0].AssetPattern)
	}
	bad := strings.Replace(src, `"a/d"`, `"nodash"`, 1)
	if _, err := ParseSources([]byte(bad)); err == nil {
		t.Fatal("accepted bad repo")
	}
	empty := strings.Replace(src, `"a/d"`, `""`, 1)
	if _, err := ParseSources([]byte(empty)); err == nil {
		t.Fatal("accepted empty repo")
	}
}

func TestValidateRejectsDotAndReservedElements(t *testing.T) { // final review I3
	for _, bad := range []string{"...", "BepInEx/...", "BepInEx/. ", "BepInEx/.. ", "a./b", "BepInEx/CON", "nul.txt"} {
		m := validManifest()
		m.Remove = []string{bad}
		if m.Validate() == nil {
			t.Errorf("accepted remove %q", bad)
		}
	}
	for _, bad := range []string{"...", "x.dll.", "x.dll ", "CON", "aux.dll"} {
		m := validManifest()
		m.Plugins[0].FileName = bad
		if m.Validate() == nil {
			t.Errorf("accepted fileName %q", bad)
		}
	}
}
