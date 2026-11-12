package manifestgen

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Kiirb/CleanInstaller/internal/ghrel"
	"github.com/Kiirb/CleanInstaller/internal/manifest"
)

var sha1x = strings.Repeat("11", 32)
var sha2x = strings.Repeat("22", 32)

type fakeResolver struct {
	releases map[string]*ghrel.Release // key: repo or repo@pin
	errs     map[string]error
	calls    []string
}

func (f *fakeResolver) Resolve(_ context.Context, s manifest.Source) (*ghrel.Release, error) {
	key := s.Repo
	if s.Pin != "" {
		key += "@" + s.Pin
	}
	f.calls = append(f.calls, key)
	if err := f.errs[key]; err != nil {
		return nil, err
	}
	r, ok := f.releases[key]
	if !ok {
		return nil, errors.New("not found: " + key)
	}
	return r, nil
}

type fakeHasher struct {
	sha   string
	calls []string
}

func (f *fakeHasher) Hash(_ context.Context, url string) (string, int64, error) {
	f.calls = append(f.calls, url)
	return f.sha, 42, nil
}

func rel(tag string, names ...string) *ghrel.Release {
	r := &ghrel.Release{TagName: tag}
	for _, n := range names {
		r.Assets = append(r.Assets, ghrel.ReleaseAsset{Name: n, BrowserDownloadURL: "https://dl/" + tag + "/" + n, Size: 42})
	}
	return r
}

func sources() *manifest.Sources {
	return &manifest.Sources{
		Modpack:  manifest.Source{ID: "Pack", Repo: "a/pack", AssetPattern: "-steam-itch.zip"},
		Launcher: manifest.Source{ID: "CleanInstaller", Repo: "k/ci", AssetPattern: ".exe"},
		Plugins:  []manifest.Source{{ID: "Reactor", Repo: "n/reactor", AssetPattern: ".dll"}},
	}
}

func baseResolver() *fakeResolver {
	return &fakeResolver{releases: map[string]*ghrel.Release{
		"a/pack":    rel("v5", "P-steam-itch.zip", "P-epic.zip"),
		"k/ci":      rel("v1.2.3", "CleanInstaller.exe"),
		"n/reactor": rel("v1", "Reactor.dll"),
	}}
}

var now = time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)

func build(t *testing.T, old *manifest.Manifest, r Resolver, h Hasher) *Result {
	t.Helper()
	res, err := Build(context.Background(), sources(), old, r, h, now)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestBuildFromEmpty(t *testing.T) {
	h := &fakeHasher{sha: sha1x}
	res := build(t, &manifest.Manifest{Schema: 1}, baseResolver(), h)
	m := res.Manifest
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	if m.Modpack.Tag != "v5" || m.Modpack.URL != "https://dl/v5/P-steam-itch.zip" || m.Modpack.SHA256 != sha1x || m.Modpack.Size != 42 {
		t.Fatalf("modpack %+v", m.Modpack)
	}
	if !m.GeneratedAt.Equal(now) || len(res.Changes) != 3 {
		t.Fatalf("generatedAt %v changes %v", m.GeneratedAt, res.Changes)
	}
}

func TestBuildUnchangedSkipsHash(t *testing.T) {
	first := build(t, &manifest.Manifest{Schema: 1}, baseResolver(), &fakeHasher{sha: sha1x}).Manifest
	first.GeneratedAt = now.Add(-time.Hour)
	h := &fakeHasher{sha: sha2x}
	res := build(t, first, baseResolver(), h)
	if len(h.calls) != 0 || len(res.Changes) != 0 {
		t.Fatalf("hash calls %v changes %v", h.calls, res.Changes)
	}
	if !res.Manifest.GeneratedAt.Equal(now.Add(-time.Hour)) {
		t.Fatal("generatedAt bumped without changes")
	}
}

func TestBuildNewVersion(t *testing.T) {
	first := build(t, &manifest.Manifest{Schema: 1}, baseResolver(), &fakeHasher{sha: sha1x}).Manifest
	r := baseResolver()
	r.releases["n/reactor"] = rel("v2", "Reactor.dll")
	h := &fakeHasher{sha: sha2x}
	res := build(t, first, r, h)
	if len(h.calls) != 1 || res.Manifest.Plugins[0].SHA256 != sha2x {
		t.Fatalf("calls %v plugin %+v", h.calls, res.Manifest.Plugins[0])
	}
	if len(res.Changes) != 1 || res.Changes[0] != "Reactor: v1 -> v2" {
		t.Fatalf("changes %v", res.Changes)
	}
}

func TestBuildFailureKeepsOld(t *testing.T) {
	first := build(t, &manifest.Manifest{Schema: 1}, baseResolver(), &fakeHasher{sha: sha1x}).Manifest
	r := baseResolver()
	r.errs = map[string]error{"n/reactor": errors.New("boom")}
	r.releases["a/pack"] = rel("v6", "P-steam-itch.zip")
	res := build(t, first, r, &fakeHasher{sha: sha2x})
	if res.Manifest.Plugins[0].Tag != "v1" || res.Manifest.Plugins[0].SHA256 != sha1x {
		t.Fatalf("plugin not kept: %+v", res.Manifest.Plugins[0])
	}
	if len(res.Failures) != 1 || !strings.HasPrefix(res.Failures[0], "Reactor:") {
		t.Fatalf("failures %v", res.Failures)
	}
	if res.Manifest.Modpack.Tag != "v6" {
		t.Fatal("other entries not updated")
	}
}

func TestBuildFailureNoOldEntryOmitsPlugin(t *testing.T) {
	r := baseResolver()
	r.errs = map[string]error{"n/reactor": errors.New("boom")}
	res := build(t, &manifest.Manifest{Schema: 1}, r, &fakeHasher{sha: sha1x})
	if len(res.Manifest.Plugins) != 0 || len(res.Failures) != 1 {
		t.Fatalf("plugins %v failures %v", res.Manifest.Plugins, res.Failures)
	}
}

func TestBuildPin(t *testing.T) {
	src := sources()
	src.Plugins[0].Pin = "v1.0.0"
	r := baseResolver()
	r.releases["n/reactor@v1.0.0"] = rel("v1.0.0", "Reactor.dll")
	res, err := Build(context.Background(), src, &manifest.Manifest{Schema: 1}, r, &fakeHasher{sha: sha1x}, now)
	if err != nil {
		t.Fatal(err)
	}
	if res.Manifest.Plugins[0].Tag != "v1.0.0" {
		t.Fatalf("plugin %+v calls %v", res.Manifest.Plugins[0], r.calls)
	}
}

func TestBuildPreservesFun(t *testing.T) {
	fun := json.RawMessage(`{"jokeText":true,"jokeLines":["x"]}`)
	res := build(t, &manifest.Manifest{Schema: 1, FunRaw: fun}, baseResolver(), &fakeHasher{sha: sha1x})
	if string(res.Manifest.FunRaw) != string(fun) {
		t.Fatalf("fun %s", res.Manifest.FunRaw)
	}
}

func TestBuildLauncherVersion(t *testing.T) {
	res := build(t, &manifest.Manifest{Schema: 1}, baseResolver(), &fakeHasher{sha: sha1x})
	if res.Manifest.Launcher.Version != "1.2.3" || res.Manifest.Launcher.Tag != "v1.2.3" {
		t.Fatalf("launcher %+v", res.Manifest.Launcher)
	}
}

func TestBuildPluginFileName(t *testing.T) {
	res := build(t, &manifest.Manifest{Schema: 1}, baseResolver(), &fakeHasher{sha: sha1x})
	p := res.Manifest.Plugins[0]
	if p.FileName != "Reactor.dll" || p.ID != "Reactor" {
		t.Fatalf("plugin %+v", p)
	}
}

func TestBuildRemoveConflict(t *testing.T) {
	src := sources()
	src.Remove = []string{"BepInEx/plugins/reactor.dll", "BepInEx/plugins/Old.dll"}
	res, err := Build(context.Background(), src, &manifest.Manifest{Schema: 1}, baseResolver(), &fakeHasher{sha: sha1x}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Manifest.Remove) != 1 || res.Manifest.Remove[0] != "BepInEx/plugins/Old.dll" {
		t.Fatalf("remove %v", res.Manifest.Remove)
	}
	if len(res.Failures) != 1 {
		t.Fatalf("failures %v", res.Failures)
	}
}

func TestBuildRemovedPluginIsChange(t *testing.T) {
	first := build(t, &manifest.Manifest{Schema: 1}, baseResolver(), &fakeHasher{sha: sha1x}).Manifest
	src := sources()
	src.Plugins = []manifest.Source{{ID: "Other", Repo: "o/other", AssetPattern: ".dll"}}
	r := baseResolver()
	r.releases["o/other"] = rel("v1", "Other.dll")
	res, err := Build(context.Background(), src, first, r, &fakeHasher{sha: sha2x}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Manifest.Plugins) != 1 || res.Manifest.Plugins[0].ID != "Other" {
		t.Fatalf("plugins %+v", res.Manifest.Plugins)
	}
	found := false
	for _, c := range res.Changes {
		found = found || c == "Reactor: removed"
	}
	if !found {
		t.Fatalf("changes %v", res.Changes)
	}
}
