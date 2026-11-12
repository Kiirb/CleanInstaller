package ghrel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLatestSkipsPrerelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r/releases" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("auth header %q", r.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"tag_name": "v3-pre", "prerelease": true, "draft": false},
			{"tag_name": "v3-draft", "prerelease": false, "draft": true},
			{"tag_name": "v2", "prerelease": false, "draft": false,
				"assets": []map[string]any{{"name": "a.dll", "browser_download_url": "https://x/a.dll", "size": 5}}},
		})
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), Token: "tok", BaseURL: srv.URL}
	r, err := c.Latest(context.Background(), "o/r")
	if err != nil {
		t.Fatal(err)
	}
	if r.TagName != "v2" || len(r.Assets) != 1 || r.Assets[0].BrowserDownloadURL != "https://x/a.dll" || r.Assets[0].Size != 5 {
		t.Fatalf("got %+v", r)
	}
}

func TestLatestNoStable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"tag_name":"v1","prerelease":true}]`))
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), BaseURL: srv.URL}
	if _, err := c.Latest(context.Background(), "o/r"); err == nil {
		t.Fatal("expected error")
	}
}

func TestByTag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r/releases/tags/v1.0.0" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"tag_name":"v1.0.0"}`))
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), BaseURL: srv.URL}
	r, err := c.ByTag(context.Background(), "o/r", "v1.0.0")
	if err != nil || r.TagName != "v1.0.0" {
		t.Fatalf("%v %+v", err, r)
	}
	if _, err := c.ByTag(context.Background(), "o/r", "nope"); err == nil {
		t.Fatal("404 not an error")
	}
}

func TestMatchAssetExactlyOne(t *testing.T) {
	r := &Release{Assets: []ReleaseAsset{{Name: "Mod.dll"}, {Name: "Mod-steam-itch.zip"}, {Name: "Mod-epic.zip"}}}
	if a, err := MatchAsset(r, "-steam-itch.zip"); err != nil || a.Name != "Mod-steam-itch.zip" {
		t.Fatalf("%v %+v", err, a)
	}
	if _, err := MatchAsset(r, ".zip"); err == nil {
		t.Fatal("two matches accepted")
	}
	if _, err := MatchAsset(r, ".exe"); err == nil {
		t.Fatal("no match accepted")
	}
}

func TestMatchAssetCaseInsensitive(t *testing.T) {
	r := &Release{Assets: []ReleaseAsset{{Name: "TouMira.v1.7.9-x64-Steam-Epic-MsStore.zip"}, {Name: "TouMira.v1.7.9-x86-ItchIo.zip"}, {Name: "TownOfUsMira.dll"}}}
	if a, err := MatchAsset(r, "steam"); err != nil || a.Name != "TouMira.v1.7.9-x64-Steam-Epic-MsStore.zip" {
		t.Fatalf("%v %+v", err, a)
	}
}
