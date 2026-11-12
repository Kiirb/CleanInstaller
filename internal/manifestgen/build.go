// Package manifestgen turns sources.json plus the previous manifest into a new
// manifest, downloading and hashing only assets that changed.
package manifestgen

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Kiirb/CleanInstaller/internal/ghrel"
	"github.com/Kiirb/CleanInstaller/internal/manifest"
)

// Resolver finds the release a source should use.
type Resolver interface {
	Resolve(ctx context.Context, src manifest.Source) (*ghrel.Release, error)
}

// Hasher downloads a URL and returns its SHA-256 and size.
type Hasher interface {
	Hash(ctx context.Context, url string) (sha256 string, size int64, err error)
}

// Result of a Build.
type Result struct {
	Manifest *manifest.Manifest
	Changes  []string // e.g. "Reactor: v2.3.0 -> v2.4.0"
	Failures []string // "<id>: <error>"; the previous entry is kept
}

// Build resolves every source. A failing source keeps its previous entry and
// is reported in Failures; the others still update.
func Build(ctx context.Context, src *manifest.Sources, old *manifest.Manifest, r Resolver, h Hasher, now time.Time) (*Result, error) {
	if old == nil {
		old = &manifest.Manifest{Schema: manifest.SchemaVersion}
	}
	res := &Result{Manifest: &manifest.Manifest{
		Schema:      manifest.SchemaVersion,
		GeneratedAt: old.GeneratedAt,
		FunRaw:      old.FunRaw,
	}}
	m := res.Manifest

	resolve := func(s manifest.Source, prev manifest.Asset) (manifest.Asset, bool) {
		a, change, err := resolveOne(ctx, s, prev, r, h)
		if err != nil {
			res.Failures = append(res.Failures, fmt.Sprintf("%s: %v", s.ID, err))
			return prev, !prev.Empty()
		}
		if change != "" {
			res.Changes = append(res.Changes, change)
		}
		return a, true
	}

	m.Modpack, _ = resolve(src.Modpack, old.Modpack)
	m.Launcher, _ = resolve(src.Launcher, old.Launcher)
	if !m.Launcher.Empty() {
		m.Launcher.Version = strings.TrimPrefix(strings.TrimPrefix(m.Launcher.Tag, "v"), "V")
	}

	oldPlugins := map[string]manifest.Asset{}
	for _, p := range old.Plugins {
		oldPlugins[p.ID] = p
	}
	m.Plugins = []manifest.Asset{}
	pluginFiles := map[string]bool{}
	for _, s := range src.Plugins {
		a, ok := resolve(s, oldPlugins[s.ID])
		if !ok {
			continue
		}
		a.ID = s.ID
		m.Plugins = append(m.Plugins, a)
		pluginFiles[strings.ToLower("BepInEx/plugins/"+a.FileName)] = true
	}
	// Removed sources are a change even if nothing was re-hashed.
	for _, p := range old.Plugins {
		if !hasSource(src.Plugins, p.ID) {
			res.Changes = append(res.Changes, p.ID+": removed")
		}
	}

	m.Remove = []string{}
	for _, p := range src.Remove {
		if pluginFiles[strings.ToLower(p)] {
			res.Failures = append(res.Failures, fmt.Sprintf("remove %s: conflicts with a managed plugin, dropped", p))
			continue
		}
		m.Remove = append(m.Remove, p)
	}
	if strings.Join(m.Remove, "\n") != strings.Join(old.Remove, "\n") {
		res.Changes = append(res.Changes, "remove list changed")
	}

	if len(res.Changes) > 0 {
		m.GeneratedAt = now.UTC()
	}
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("generated manifest is invalid: %w", err)
	}
	return res, nil
}

func hasSource(ss []manifest.Source, id string) bool {
	for _, s := range ss {
		if s.ID == id {
			return true
		}
	}
	return false
}

func resolveOne(ctx context.Context, s manifest.Source, prev manifest.Asset, r Resolver, h Hasher) (manifest.Asset, string, error) {
	rel, err := r.Resolve(ctx, s)
	if err != nil {
		return prev, "", err
	}
	ra, err := ghrel.MatchAsset(rel, s.AssetPattern)
	if err != nil {
		return prev, "", err
	}
	if !prev.Empty() && prev.Tag == rel.TagName && prev.URL == ra.BrowserDownloadURL {
		return prev, "", nil
	}
	sum, size, err := h.Hash(ctx, ra.BrowserDownloadURL)
	if err != nil {
		return prev, "", fmt.Errorf("download %s: %w", ra.Name, err)
	}
	a := manifest.Asset{Tag: rel.TagName, URL: ra.BrowserDownloadURL, SHA256: sum, Size: size, FileName: ra.Name}
	from := prev.Tag
	if prev.Empty() {
		from = "(new)"
	}
	return a, fmt.Sprintf("%s: %s -> %s", s.ID, from, rel.TagName), nil
}

// ClientResolver resolves sources through the GitHub API.
type ClientResolver struct{ C *ghrel.Client }

// Resolve uses the pinned tag if set, otherwise the latest stable release.
func (cr ClientResolver) Resolve(ctx context.Context, s manifest.Source) (*ghrel.Release, error) {
	if s.Pin != "" {
		return cr.C.ByTag(ctx, s.Repo, s.Pin)
	}
	return cr.C.Latest(ctx, s.Repo)
}

// HTTPHasher downloads and hashes over HTTP.
type HTTPHasher struct{ HTTP *http.Client }

// Hash streams the URL through SHA-256.
func (hh HTTPHasher) Hash(ctx context.Context, url string) (string, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("User-Agent", "CleanInstaller-manifestgen")
	resp, err := hh.HTTP.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	sum := sha256.New()
	n, err := io.Copy(sum, resp.Body)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(sum.Sum(nil)), n, nil
}
