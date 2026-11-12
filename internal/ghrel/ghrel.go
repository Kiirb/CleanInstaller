// Package ghrel is a minimal GitHub Releases client. It is used only by the
// manifest workflow; the shipped exe never calls the GitHub API.
package ghrel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ReleaseAsset is one file attached to a release.
type ReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// Release is one GitHub release.
type Release struct {
	TagName    string         `json:"tag_name"`
	Draft      bool           `json:"draft"`
	Prerelease bool           `json:"prerelease"`
	Assets     []ReleaseAsset `json:"assets"`
}

// Client talks to the GitHub REST API.
type Client struct {
	HTTP    *http.Client
	Token   string
	BaseURL string // default "https://api.github.com"
}

// Latest returns the newest release that is neither a draft nor a prerelease.
func (c *Client) Latest(ctx context.Context, repo string) (*Release, error) {
	var rels []Release
	if err := c.get(ctx, "/repos/"+repo+"/releases?per_page=30", &rels); err != nil {
		return nil, err
	}
	for i := range rels {
		if !rels[i].Draft && !rels[i].Prerelease {
			return &rels[i], nil
		}
	}
	return nil, fmt.Errorf("ghrel: %s has no stable release", repo)
}

// ByTag returns the release with the given tag.
func (c *Client) ByTag(ctx context.Context, repo, tag string) (*Release, error) {
	var r Release
	if err := c.get(ctx, "/repos/"+repo+"/releases/tags/"+url.PathEscape(tag), &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	base := c.BaseURL
	if base == "" {
		base = "https://api.github.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "CleanInstaller-manifestgen")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	h := c.HTTP
	if h == nil {
		h = http.DefaultClient
	}
	resp, err := h.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("ghrel: GET %s: %s: %s", path, resp.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// MatchAsset returns the single asset whose name contains pattern.
func MatchAsset(r *Release, pattern string) (ReleaseAsset, error) {
	var found []ReleaseAsset
	for _, a := range r.Assets {
		if strings.Contains(a.Name, pattern) {
			found = append(found, a)
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return ReleaseAsset{}, fmt.Errorf("no asset in %s matches %q", r.TagName, pattern)
	}
	names := make([]string, len(found))
	for i, a := range found {
		names[i] = a.Name
	}
	return ReleaseAsset{}, fmt.Errorf("%d assets in %s match %q: %s", len(found), r.TagName, pattern, strings.Join(names, ", "))
}
