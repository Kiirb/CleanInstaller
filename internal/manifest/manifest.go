// Package manifest defines manifest.json (what every client installs) and
// sources.json (what the workflow resolves into the manifest).
package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// SchemaVersion is the newest manifest schema this build understands.
const SchemaVersion = 1

// ErrSchemaTooNew means the manifest was written for a newer client.
var ErrSchemaTooNew = errors.New("manifest schema too new")

// Asset is one downloadable file pinned by checksum.
type Asset struct {
	Version  string `json:"version,omitempty"`  // launcher only, semver without "v"
	Tag      string `json:"tag,omitempty"`      // release tag it came from
	FileName string `json:"fileName,omitempty"` // plugins only
	ID       string `json:"id,omitempty"`       // plugins only
	URL      string `json:"url"`
	SHA256   string `json:"sha256"` // lowercase hex
	Size     int64  `json:"size"`
}

// Empty reports whether the asset has not been resolved yet.
func (a Asset) Empty() bool { return a.URL == "" }

// Fun is the hand-edited joke-mode block (installer only).
type Fun struct {
	JokeText  bool     `json:"jokeText"`
	ChaosBar  bool     `json:"chaosBar"`
	JokeLines []string `json:"jokeLines"`
}

// Manifest is the resolved, pinned setup every client installs.
type Manifest struct {
	Schema      int             `json:"schema"`
	GeneratedAt time.Time       `json:"generatedAt"`
	Launcher    Asset           `json:"launcher"`
	Modpack     Asset           `json:"modpack"`
	Plugins     []Asset         `json:"plugins"`
	Remove      []string        `json:"remove"`
	FunRaw      json.RawMessage `json:"fun,omitempty"`
}

// Fun decodes the fun block. An absent or invalid block yields the zero Fun,
// which means "joke mode off".
func (m *Manifest) Fun() Fun {
	var f Fun
	if len(m.FunRaw) == 0 || json.Unmarshal(m.FunRaw, &f) != nil {
		return Fun{}
	}
	return f
}

// Parse decodes and validates a manifest.
func Parse(b []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("manifest: invalid JSON: %w", err)
	}
	if m.Schema > SchemaVersion {
		return nil, fmt.Errorf("%w: %d > %d", ErrSchemaTooNew, m.Schema, SchemaVersion)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// Marshal encodes with 2-space indent and a trailing newline. Whitespace in
// the fun block may be normalised; its content is kept.
func Marshal(m *Manifest) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

var shaRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Validate checks every entry before anything is written to disk.
func (m *Manifest) Validate() error {
	if m.Schema < 1 {
		return fmt.Errorf("manifest: missing schema")
	}
	if !m.Launcher.Empty() {
		if err := checkAsset("launcher", m.Launcher); err != nil {
			return err
		}
		if m.Launcher.Version == "" {
			return fmt.Errorf("manifest: launcher has no version")
		}
	}
	if !m.Modpack.Empty() {
		if err := checkAsset("modpack", m.Modpack); err != nil {
			return err
		}
	}
	seen := map[string]bool{}
	for _, p := range m.Plugins {
		if err := checkAsset("plugin "+p.ID, p); err != nil {
			return err
		}
		if err := checkFileName(p.FileName); err != nil {
			return fmt.Errorf("manifest: plugin %s: %w", p.ID, err)
		}
		k := strings.ToLower(p.FileName)
		if seen[k] {
			return fmt.Errorf("manifest: duplicate plugin fileName %q", p.FileName)
		}
		seen[k] = true
	}
	for _, r := range m.Remove {
		if err := CheckRelPath(r); err != nil {
			return fmt.Errorf("manifest: remove %q: %w", r, err)
		}
	}
	return nil
}

func checkAsset(what string, a Asset) error {
	if !strings.HasPrefix(a.URL, "https://") {
		return fmt.Errorf("manifest: %s: url must be https", what)
	}
	if !shaRe.MatchString(a.SHA256) {
		return fmt.Errorf("manifest: %s: invalid sha256 %q", what, a.SHA256)
	}
	if a.Size <= 0 {
		return fmt.Errorf("manifest: %s: invalid size", what)
	}
	return nil
}

func checkFileName(name string) error {
	if name == "" || name == "." || name == ".." ||
		strings.ContainsAny(name, `/\:`) {
		return fmt.Errorf("invalid file name %q", name)
	}
	return nil
}

// CheckRelPath accepts a forward-slash relative path with no empty, "." or
// ".." elements, no backslashes and no drive letters.
func CheckRelPath(p string) error {
	if p == "" || strings.ContainsAny(p, `\:`) || strings.HasPrefix(p, "/") {
		return fmt.Errorf("invalid relative path")
	}
	for _, el := range strings.Split(p, "/") {
		if el == "" || el == "." || el == ".." {
			return fmt.Errorf("invalid path element %q", el)
		}
	}
	return nil
}

// Source is one entry of sources.json.
type Source struct {
	ID           string `json:"id"`
	Repo         string `json:"repo"`         // "owner/name"
	AssetPattern string `json:"assetPattern"` // substring match on asset name
	Pin          string `json:"pin,omitempty"`
}

// Sources is sources.json, edited by hand.
type Sources struct {
	Modpack  Source   `json:"modpack"`
	Plugins  []Source `json:"plugins"`
	Launcher Source   `json:"launcher"`
	Remove   []string `json:"remove"`
}

var repoRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// ParseSources decodes sources.json, applies defaults and validates it.
func ParseSources(b []byte) (*Sources, error) {
	var s Sources
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("sources: invalid JSON: %w", err)
	}
	check := func(what string, src *Source, defPattern string) error {
		if !repoRe.MatchString(src.Repo) {
			return fmt.Errorf("sources: %s: invalid repo %q", what, src.Repo)
		}
		if src.AssetPattern == "" {
			if defPattern == "" {
				return fmt.Errorf("sources: %s: assetPattern required", what)
			}
			src.AssetPattern = defPattern
		}
		return nil
	}
	if err := check("modpack", &s.Modpack, ""); err != nil {
		return nil, err
	}
	if err := check("launcher", &s.Launcher, ""); err != nil {
		return nil, err
	}
	ids := map[string]bool{}
	for i := range s.Plugins {
		p := &s.Plugins[i]
		if p.ID == "" {
			return nil, fmt.Errorf("sources: plugin %d has no id", i)
		}
		if ids[p.ID] {
			return nil, fmt.Errorf("sources: duplicate plugin id %q", p.ID)
		}
		ids[p.ID] = true
		if err := check("plugin "+p.ID, p, ".dll"); err != nil {
			return nil, err
		}
	}
	for _, r := range s.Remove {
		if err := CheckRelPath(r); err != nil {
			return nil, fmt.Errorf("sources: remove %q: %w", r, err)
		}
	}
	return &s, nil
}
