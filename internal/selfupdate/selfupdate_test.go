package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Kiirb/CleanInstaller/internal/download"
	"github.com/Kiirb/CleanInstaller/internal/manifest"
)

func TestNeeded(t *testing.T) {
	m := func(v string) *manifest.Manifest {
		return &manifest.Manifest{Launcher: manifest.Asset{Version: v, URL: "https://x"}}
	}
	cases := []struct {
		cur, latest string
		want        bool
	}{
		{"1.0.0", "1.0.1", true},
		{"1.0.1", "1.0.1", false},
		{"1.2.0", "1.0.9", false},
		{"0.0.0-dev", "1.0.0", true},
		{"garbage", "1.0.0", false},
		{"1.0.0", "garbage", false},
	}
	for _, c := range cases {
		if got := Needed(c.cur, m(c.latest)); got != c.want {
			t.Errorf("Needed(%s, %s)=%v", c.cur, c.latest, got)
		}
	}
	if Needed("1.0.0", &manifest.Manifest{}) {
		t.Error("empty launcher entry")
	}
}

func TestApplyRenamesAndReplaces(t *testing.T) {
	newExe := []byte("new-exe-bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(newExe) }))
	defer srv.Close()
	dir := t.TempDir()
	self := filepath.Join(dir, "CleanInstaller.exe")
	_ = os.WriteFile(self, []byte("old-exe"), 0o644)
	sum := sha256.Sum256(newExe)
	a := manifest.Asset{Version: "2.0.0", URL: srv.URL, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(newExe))}
	if err := Apply(context.Background(), self, &download.Cache{Dir: filepath.Join(dir, "cache"), HTTP: srv.Client()}, a); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(self); string(b) != "new-exe-bytes" {
		t.Fatalf("self %q", b)
	}
	if b, _ := os.ReadFile(self + ".old"); string(b) != "old-exe" {
		t.Fatalf("old %q", b)
	}
}

func TestApplyBadDownloadKeepsSelf(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("tampered")) }))
	defer srv.Close()
	dir := t.TempDir()
	self := filepath.Join(dir, "CleanInstaller.exe")
	_ = os.WriteFile(self, []byte("old-exe"), 0o644)
	a := manifest.Asset{Version: "2.0.0", URL: srv.URL, SHA256: hex.EncodeToString(make([]byte, 32)), Size: 8}
	if err := Apply(context.Background(), self, &download.Cache{Dir: filepath.Join(dir, "cache"), HTTP: srv.Client()}, a); err == nil {
		t.Fatal("expected error")
	}
	if b, _ := os.ReadFile(self); string(b) != "old-exe" {
		t.Fatal("self replaced by bad download")
	}
}

func TestCleanupOld(t *testing.T) {
	dir := t.TempDir()
	self := filepath.Join(dir, "a.exe")
	_ = os.WriteFile(self+".old", []byte("x"), 0o644)
	if err := CleanupOld(self); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(self + ".old"); err == nil {
		t.Fatal("not removed")
	}
	if err := CleanupOld(self); err != nil {
		t.Fatal("missing .old is an error")
	}
}
