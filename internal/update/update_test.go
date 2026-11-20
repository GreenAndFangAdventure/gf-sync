package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const setupHash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func manifestJSON(version, min string) string {
	return fmt.Sprintf(`{"schema":1,"product":"GFSync","version":%q,"channel":"release",`+
		`"released":"2026-11-20T00:00:00Z","url":"https://github.com/GreenAndFangAdventure/gf-sync/releases/download/v%s/GFSync-Setup-%s.exe",`+
		`"sha256":%q,"size":1234,"minGFSync":%q,"privacyChanged":false,"notes":"x"}`, version, version, version, setupHash, min)
}

func hexSum(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

// fake serves the site and GitHub on one loopback server.
type fake struct {
	site     string // "" = 404
	latest   string // GitHub latest/download manifest; "" = 404
	sums     map[string]string
	requests []string
}

func (f *fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.requests = append(f.requests, r.URL.Path)
	switch {
	case r.URL.Path == SitePath && f.site != "":
		fmt.Fprint(w, f.site)
	case r.URL.Path == "/"+Repo+"/releases/latest/download/"+ManifestName && f.latest != "":
		// Like GitHub: redirect to the versioned asset.
		http.Redirect(w, r, "/"+Repo+"/releases/download/v-latest/"+ManifestName, http.StatusFound)
	case r.URL.Path == "/"+Repo+"/releases/download/v-latest/"+ManifestName && f.latest != "":
		fmt.Fprint(w, f.latest)
	case strings.HasSuffix(r.URL.Path, "/"+SumsName):
		if s, ok := f.sums[r.URL.Path]; ok {
			fmt.Fprint(w, s)
			return
		}
		http.NotFound(w, r)
	default:
		http.NotFound(w, r)
	}
}

func sumsPath(v string) string { return "/" + Repo + "/releases/download/v" + v + "/" + SumsName }

func goodSums(manifest, v string) string {
	return fmt.Sprintf("\ufeff%s  GFSync.exe\r\n%s  %s\r\n%s  %s\r\n",
		strings.Repeat("a", 64), setupHash, SetupName(v), hexSum(manifest), ManifestName)
}

func checker(t *testing.T, f *fake, current string) *Checker {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, current)
	if err != nil {
		t.Fatal(err)
	}
	c.GitHub, _ = url.Parse(srv.URL)
	return c
}

func TestCheckFromSite(t *testing.T) {
	m := manifestJSON("0.2.0", "0.1.1")
	f := &fake{site: m, sums: map[string]string{sumsPath("0.2.0"): goodSums(m, "0.2.0")}}
	for _, c := range []struct {
		current         string
		newer, required bool
	}{
		{"0.1.0", true, true},
		{"0.1.1", true, false},
		{"0.2.0", false, false},
		{"0.3.0", false, false},
		{"0.2.0-beta1", true, false},
	} {
		r, err := checker(t, f, c.current).Check(context.Background())
		if err != nil {
			t.Fatalf("%s: %v", c.current, err)
		}
		if r.Newer != c.newer || r.Required != c.required || r.Source != "site" || r.Manifest.Version != "0.2.0" {
			t.Errorf("%s: %+v", c.current, r)
		}
	}
}

func TestCheckFallsBackToGitHub(t *testing.T) {
	m := manifestJSON("0.2.0", "")
	f := &fake{latest: m, sums: map[string]string{sumsPath("0.2.0"): goodSums(m, "0.2.0")}}
	r, err := checker(t, f, "0.1.1").Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !r.Newer || r.Required || r.Source != "github" {
		t.Errorf("%+v", r)
	}
}

func TestCheckNothingPublished(t *testing.T) {
	_, err := checker(t, &fake{}, "0.1.1").Check(context.Background())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// Every way a manifest can be wrong is refused, and nothing newer is
// reported.
func TestCheckRejects(t *testing.T) {
	good := manifestJSON("0.2.0", "")
	cases := map[string]struct{ site, sums string }{
		// The site copy was changed (one byte): its hash is not in SUMS.
		"edited copy": {strings.Replace(good, `"notes":"x"`, `"notes":"y"`, 1), goodSums(good, "0.2.0")},
		// The installer hash differs from SUMS.
		"installer hash": {strings.Replace(good, setupHash, strings.Repeat("b", 64), 1), ""},
		"other repo":     {strings.Replace(good, "GreenAndFangAdventure/gf-sync", "someone/gf-sync", 1), ""},
		"http url":       {strings.Replace(good, "https://github.com", "http://github.com", 1), ""},
		"other file":     {strings.Replace(good, "GFSync-Setup-0.2.0.exe", "evil.exe", 1), ""},
		"product":        {strings.Replace(good, `"GFSync"`, `"Other"`, 1), ""},
		"channel":        {strings.Replace(good, `"release"`, `"beta"`, 1), ""},
		"schema":         {strings.Replace(good, `"schema":1`, `"schema":2`, 1), ""},
		"ci version":     {manifestJSON("0.0.0-ci.5", ""), ""},
		"bad version":    {manifestJSON("latest", ""), ""},
		"bad min":        {manifestJSON("0.2.0", "soon"), ""},
		"not json":       {"<html>", ""},
		"no sums":        {good, "-"},
	}
	for name, c := range cases {
		sums := c.sums
		if sums == "" {
			sums = goodSums(c.site, "0.2.0")
		}
		f := &fake{site: c.site, sums: map[string]string{}}
		if sums != "-" {
			f.sums[sumsPath("0.2.0")] = sums
		}
		r, err := checker(t, f, "0.1.1").Check(context.Background())
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: result %+v, err %v; want ErrInvalid", name, r, err)
		}
	}
}

func TestCompare(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"0.1.0", "0.1.0", 0},
		{"0.1.0", "0.1.1", -1},
		{"0.10.0", "0.9.9", 1},
		{"1.0.0", "0.99.99", 1},
		{"0.2.0-beta1", "0.2.0", -1},
		{"0.2.0", "0.2.0-beta1", 1},
		{"0.2.0-beta1", "0.2.0-beta2", -1},
		{"v0.2.0", "0.2.0", 0},
		{"0.2.0+abc", "0.2.0", 0},
		{"0.1.0-test", "0.1.0", -1},
		{"garbage", "0.0.1", -1},
		{"0.0.0-dev", "0.1.0", -1},
	} {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestNewRequiresHTTPS(t *testing.T) {
	if _, err := New("http://example.com", "0.1.0"); err == nil {
		t.Error("http to a non-loopback host must be refused")
	}
	if _, err := New("https://greenandfangadventure.com", "0.1.0"); err != nil {
		t.Error(err)
	}
}
