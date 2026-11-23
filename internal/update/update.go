// Package update checks whether a newer GF Sync has been released
// (docs/UPDATE_MANIFEST.md). It only reads: it never downloads or runs an
// installer. Installing is for a later, signed version (docs/AUTO_UPDATE.md
// §4.3).
//
// The manifest is a GitHub Release asset made by CI. The site serves the
// same bytes; GF Sync trusts a copy only if its SHA-256 is listed in the
// SHA256SUMS.txt of the same Release in the pinned repository, and the
// installer hash in the manifest is listed there too. Changing only the site
// or only one file therefore does not work.
package update

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Pinned locations. They are not read from the network.
const (
	Repo         = "GreenAndFangAdventure/gf-sync"
	ManifestName = "gfsync-manifest.json"
	SumsName     = "SHA256SUMS.txt"
	// SitePath is where the site serves the manifest, byte for byte the
	// Release asset of the latest release.
	SitePath = "/api/sync/gfsync-manifest.json"
	// GitHub is the default GitHub base.
	GitHub = "https://github.com"
)

// Limits on what is read.
const (
	maxManifest = 64 << 10
	maxSums     = 64 << 10
)

// Manifest is gfsync-manifest.json, schema 1.
type Manifest struct {
	Schema         int    `json:"schema"`
	Product        string `json:"product"`
	Version        string `json:"version"`
	Channel        string `json:"channel"`
	Released       string `json:"released"`
	URL            string `json:"url"`
	SHA256         string `json:"sha256"`
	Size           int64  `json:"size"`
	MinGFSync      string `json:"minGFSync"`
	Changelog      string `json:"changelog"`
	PrivacyChanged bool   `json:"privacyChanged"`
	Notes          string `json:"notes"`
}

// SetupName is the installer file name for version v.
func SetupName(v string) string { return "GFSync-Setup-" + v + ".exe" }

// ReleasePage is the public page of the release (what the tray opens).
func ReleasePage(v string) string {
	return GitHub + "/" + Repo + "/releases/tag/v" + v
}

// Result of a check.
type Result struct {
	Manifest Manifest
	Current  string
	Newer    bool   // Manifest.Version > Current
	Required bool   // Current < Manifest.MinGFSync: the site may refuse this version
	Source   string // "site" or "github"
}

// Checker fetches and verifies the manifest.
type Checker struct {
	HTTP    *http.Client
	Site    *url.URL // the API base (https, or http on loopback for tests)
	GitHub  *url.URL // default GitHub; tests point it at a local server
	Current string   // running version
}

// New returns a checker for the site base and the running version.
func New(site, current string) (*Checker, error) {
	s, err := parseBase(site)
	if err != nil {
		return nil, err
	}
	g, _ := url.Parse(GitHub)
	return &Checker{Site: s, GitHub: g, Current: current}, nil
}

func parseBase(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimRight(raw, "/"))
	if err != nil {
		return nil, err
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback(u.Hostname())) {
		return nil, fmt.Errorf("update: base URL must be https: %s", raw)
	}
	return u, nil
}

func loopback(h string) bool { return h == "localhost" || h == "127.0.0.1" || h == "::1" }

// Errors. ErrNotFound means no manifest is published yet (not a problem).
var (
	ErrNotFound = errors.New("update: no manifest published")
	ErrInvalid  = errors.New("update: manifest rejected")
)

// Check fetches the manifest from the site, or from the latest GitHub
// Release when the site has none (404), and verifies it.
func (c *Checker) Check(ctx context.Context) (*Result, error) {
	raw, err := c.get(ctx, c.Site, SitePath, maxManifest)
	src := "site"
	if errors.Is(err, ErrNotFound) {
		raw, err = c.get(ctx, c.GitHub, "/"+Repo+"/releases/latest/download/"+ManifestName, maxManifest)
		src = "github"
	}
	if err != nil {
		return nil, err
	}
	m, err := c.verify(ctx, raw)
	if err != nil {
		return nil, err
	}
	r := &Result{Manifest: *m, Current: c.Current, Source: src}
	r.Newer = Compare(m.Version, c.Current) > 0
	r.Required = m.MinGFSync != "" && Compare(c.Current, m.MinGFSync) < 0
	return r, nil
}

var (
	reVersion = regexp.MustCompile(`^\d{1,4}\.\d{1,4}\.\d{1,4}(-[0-9A-Za-z.-]{1,32})?$`)
	reHash    = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// verify parses raw and checks it against the Release's SHA256SUMS.txt.
func (c *Checker) verify(ctx context.Context, raw []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("%w: bad JSON: %v", ErrInvalid, err)
	}
	switch {
	case m.Schema != 1:
		return nil, fmt.Errorf("%w: schema %d", ErrInvalid, m.Schema)
	case m.Product != "GFSync":
		return nil, fmt.Errorf("%w: product %q", ErrInvalid, m.Product)
	case m.Channel != "release":
		return nil, fmt.Errorf("%w: channel %q", ErrInvalid, m.Channel)
	case !reVersion.MatchString(m.Version) || strings.HasPrefix(m.Version, "0.0.0"):
		return nil, fmt.Errorf("%w: version %q", ErrInvalid, m.Version)
	case m.MinGFSync != "" && !reVersion.MatchString(m.MinGFSync):
		return nil, fmt.Errorf("%w: minGFSync %q", ErrInvalid, m.MinGFSync)
	case !reHash.MatchString(m.SHA256):
		return nil, fmt.Errorf("%w: sha256", ErrInvalid)
	}
	want := GitHub + "/" + Repo + "/releases/download/v" + m.Version + "/" + SetupName(m.Version)
	if m.URL != want {
		return nil, fmt.Errorf("%w: url is not the pinned release asset", ErrInvalid)
	}
	sums, err := c.get(ctx, c.GitHub, "/"+Repo+"/releases/download/v"+m.Version+"/"+SumsName, maxSums)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalid, SumsName, err)
	}
	list := parseSums(sums)
	sum := sha256.Sum256(raw)
	if list[ManifestName] != hex.EncodeToString(sum[:]) {
		return nil, fmt.Errorf("%w: manifest is not the one in %s", ErrInvalid, SumsName)
	}
	if list[SetupName(m.Version)] != m.SHA256 {
		return nil, fmt.Errorf("%w: installer hash differs from %s", ErrInvalid, SumsName)
	}
	return &m, nil
}

// parseSums reads "hash  name" lines (sha256sum format, also "hash *name").
func parseSums(b []byte) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		f := strings.Fields(strings.TrimPrefix(sc.Text(), "\ufeff"))
		if len(f) != 2 {
			continue
		}
		h, name := strings.ToLower(f[0]), strings.TrimPrefix(f[1], "*")
		if reHash.MatchString(h) {
			out[name] = h
		}
	}
	return out
}

func (c *Checker) client() *http.Client {
	base := c.HTTP
	if base == nil {
		base = &http.Client{Timeout: 30 * time.Second}
	}
	cl := *base
	// Follow redirects (GitHub sends release downloads to its file host)
	// but never away from https, except loopback in tests.
	cl.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("update: too many redirects")
		}
		if req.URL.Scheme != "https" && !(req.URL.Scheme == "http" && loopback(req.URL.Hostname())) {
			return fmt.Errorf("update: redirect to %s refused", req.URL.Scheme)
		}
		return nil
	}
	return &cl
}

func (c *Checker) get(ctx context.Context, base *url.URL, path string, limit int64) ([]byte, error) {
	u := *base
	u.Path = strings.TrimRight(u.Path, "/") + path
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "GFSync/"+c.Current)
	resp, err := c.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update: HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("update: response larger than %d bytes", limit)
	}
	return b, nil
}
