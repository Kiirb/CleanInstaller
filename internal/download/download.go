// Package download fetches files and keeps a cache keyed by SHA-256. A file
// is only ever used after its checksum has been verified.
package download

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Kiirb/CleanInstaller/internal/manifest"
)

// UserAgent is sent with every request; main sets the version.
var UserAgent = "CleanInstaller"

// ErrChecksum means the downloaded bytes did not match the manifest.
var ErrChecksum = errors.New("checksum mismatch")

// ProgressFunc reports bytes done of total.
type ProgressFunc func(done, total int64)

// Cache stores verified downloads named by their SHA-256.
type Cache struct {
	Dir  string
	HTTP *http.Client
}

// Get returns the path of the verified file for a, downloading it if it is
// absent or corrupt. A mismatching download is retried once.
func (c *Cache) Get(ctx context.Context, a manifest.Asset, p ProgressFunc) (string, error) {
	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return "", err
	}
	final := filepath.Join(c.Dir, a.SHA256)
	if sum, err := fileSHA(final); err == nil && sum == a.SHA256 {
		return final, nil
	}
	_ = os.Remove(final)

	var err error
	for attempt := 0; attempt < 2; attempt++ {
		err = c.fetch(ctx, a, final, p)
		if err == nil {
			return final, nil
		}
		if !errors.Is(err, ErrChecksum) || ctx.Err() != nil {
			break
		}
	}
	return "", err
}

func (c *Cache) fetch(ctx context.Context, a manifest.Asset, final string, p ProgressFunc) (err error) {
	part := final + ".part"
	defer func() {
		if err != nil {
			_ = os.Remove(part)
		}
	}()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := c.client().Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", a.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", a.URL, resp.Status)
	}
	f, err := os.Create(part)
	if err != nil {
		return err
	}
	h := sha256.New()
	pr := &progressReader{r: resp.Body, total: a.Size, p: p}
	_, copyErr := io.Copy(io.MultiWriter(f, h), pr)
	closeErr := f.Close()
	if copyErr != nil {
		return fmt.Errorf("download %s: %w", a.URL, copyErr)
	}
	if closeErr != nil {
		return closeErr
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != a.SHA256 || pr.done != a.Size {
		return fmt.Errorf("%w for %s (got %s, %d bytes)", ErrChecksum, a.URL, got, pr.done)
	}
	return os.Rename(part, final)
}

func (c *Cache) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

// Prune deletes cached files whose SHA is not in keep, plus stray .part files.
func (c *Cache) Prune(keep []string) error {
	k := map[string]bool{}
	for _, s := range keep {
		k[s] = true
	}
	es, err := os.ReadDir(c.Dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var firstErr error
	for _, e := range es {
		if e.IsDir() || (k[e.Name()] && !strings.HasSuffix(e.Name(), ".part")) {
			continue
		}
		if err := os.Remove(filepath.Join(c.Dir, e.Name())); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// FetchManifest downloads the manifest with a 15 s timeout.
func FetchManifest(ctx context.Context, h *http.Client, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Cache-Control", "no-cache")
	if h == nil {
		h = http.DefaultClient
	}
	resp, err := h.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manifest: %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}

func fileSHA(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// FileSHA256 returns the hex SHA-256 of a file.
func FileSHA256(path string) (string, error) { return fileSHA(path) }

type progressReader struct {
	r     io.Reader
	done  int64
	total int64
	p     ProgressFunc
}

func (pr *progressReader) Read(b []byte) (int, error) {
	n, err := pr.r.Read(b)
	pr.done += int64(n)
	if pr.p != nil && n > 0 {
		pr.p(pr.done, pr.total)
	}
	return n, err
}
