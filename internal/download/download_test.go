package download

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/Kiirb/CleanInstaller/internal/manifest"
)

var payload = []byte("hello plugin bytes, a reasonably long payload for testing")

func shaOf(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func server(t *testing.T, body func(w http.ResponseWriter)) (*httptest.Server, *int32) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		body(w)
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

func asset(url string) manifest.Asset {
	return manifest.Asset{URL: url, SHA256: shaOf(payload), Size: int64(len(payload))}
}

func onlyFiles(t *testing.T, dir string) []string {
	t.Helper()
	es, _ := os.ReadDir(dir)
	var out []string
	for _, e := range es {
		out = append(out, e.Name())
	}
	return out
}

func TestGetDownloadsAndVerifies(t *testing.T) {
	srv, _ := server(t, func(w http.ResponseWriter) { _, _ = w.Write(payload) })
	c := &Cache{Dir: t.TempDir(), HTTP: srv.Client()}
	var lastDone, lastTotal int64
	p, err := c.Get(context.Background(), asset(srv.URL+"/a.dll"), func(d, tot int64) { lastDone, lastTotal = d, tot })
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p) != shaOf(payload) || filepath.Dir(p) != c.Dir {
		t.Fatalf("path %s", p)
	}
	b, _ := os.ReadFile(p)
	if string(b) != string(payload) {
		t.Fatal("content")
	}
	if lastDone != int64(len(payload)) || lastTotal != int64(len(payload)) {
		t.Fatalf("progress %d/%d", lastDone, lastTotal)
	}
}

func TestGetUsesCache(t *testing.T) {
	srv, n := server(t, func(w http.ResponseWriter) { _, _ = w.Write(payload) })
	c := &Cache{Dir: t.TempDir(), HTTP: srv.Client()}
	for i := 0; i < 2; i++ {
		if _, err := c.Get(context.Background(), asset(srv.URL), nil); err != nil {
			t.Fatal(err)
		}
	}
	if *n != 1 {
		t.Fatalf("requests %d", *n)
	}
}

func TestGetChecksumMismatchRetriesThenFails(t *testing.T) {
	srv, n := server(t, func(w http.ResponseWriter) {
		_, _ = w.Write([]byte("wrong bytes of the same-ish length...................."))
	})
	c := &Cache{Dir: t.TempDir(), HTTP: srv.Client()}
	_, err := c.Get(context.Background(), asset(srv.URL), nil)
	if !errors.Is(err, ErrChecksum) {
		t.Fatalf("err %v", err)
	}
	if *n != 2 {
		t.Fatalf("requests %d", *n)
	}
	if f := onlyFiles(t, c.Dir); len(f) != 0 {
		t.Fatalf("leftovers %v", f)
	}
}

func TestGetInterruptedNeverUsed(t *testing.T) { // Review Focus 3
	srv, _ := server(t, func(w http.ResponseWriter) {
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write(payload[:10])
		// handler returns: connection closes short of Content-Length
	})
	c := &Cache{Dir: t.TempDir(), HTTP: srv.Client()}
	if _, err := c.Get(context.Background(), asset(srv.URL), nil); err == nil {
		t.Fatal("expected error")
	}
	if f := onlyFiles(t, c.Dir); len(f) != 0 {
		t.Fatalf("leftovers %v", f)
	}
}

func TestGetHTTPErrorStatus(t *testing.T) {
	srv, _ := server(t, func(w http.ResponseWriter) { w.WriteHeader(http.StatusNotFound) })
	c := &Cache{Dir: t.TempDir(), HTTP: srv.Client()}
	if _, err := c.Get(context.Background(), asset(srv.URL), nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestGetCorruptCacheRedownloads(t *testing.T) {
	srv, n := server(t, func(w http.ResponseWriter) { _, _ = w.Write(payload) })
	c := &Cache{Dir: t.TempDir(), HTTP: srv.Client()}
	_ = os.WriteFile(filepath.Join(c.Dir, shaOf(payload)), []byte("corrupt"), 0o644)
	p, err := c.Get(context.Background(), asset(srv.URL), nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != string(payload) || *n != 1 {
		t.Fatalf("content %q requests %d", b, *n)
	}
}

func TestPrune(t *testing.T) {
	dir := t.TempDir()
	keep, drop := shaOf([]byte("k")), shaOf([]byte("d"))
	for _, f := range []string{keep, drop, drop + ".part"} {
		_ = os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644)
	}
	c := &Cache{Dir: dir}
	if err := c.Prune([]string{keep}); err != nil {
		t.Fatal(err)
	}
	if f := onlyFiles(t, dir); len(f) != 1 || f[0] != keep {
		t.Fatalf("left %v", f)
	}
}

func TestFetchManifest(t *testing.T) {
	srv, _ := server(t, func(w http.ResponseWriter) { _, _ = w.Write([]byte(`{"schema":1}`)) })
	b, err := FetchManifest(context.Background(), srv.Client(), srv.URL)
	if err != nil || string(b) != `{"schema":1}` {
		t.Fatalf("%v %s", err, b)
	}
	bad, _ := server(t, func(w http.ResponseWriter) { w.WriteHeader(500) })
	if _, err := FetchManifest(context.Background(), bad.Client(), bad.URL); err == nil {
		t.Fatal("500 accepted")
	}
}
