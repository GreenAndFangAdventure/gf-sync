// Package api is the client for the guild website's sync upload API v1 (docs/FORMAT.md).
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultBase is the production site.
const DefaultBase = "https://greenandfangadventure.com"

// Contract limits (§2).
const (
	MaxItems     = 50
	MaxBodyBytes = 1 << 20
	HourlyLimit  = 60
)

// Client talks to the site.
type Client struct {
	Base    *url.URL
	HTTP    *http.Client
	Name    string // client.name, "GFSync"
	Version string // client.version
	Label   string // computer name shown on the pairing page
}

// New parses base and returns a client. Only https is allowed, except
// http for loopback hosts (tests and the local mock server).
func New(base, version string) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil {
		return nil, err
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && isLoopback(u.Hostname())) {
		return nil, fmt.Errorf("api: base URL must be https: %s", base)
	}
	return &Client{
		Base:    u,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
		Name:    "GFSync",
		Version: version,
	}, nil
}

func isLoopback(h string) bool {
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

// HTTPError is a non-2xx response.
type HTTPError struct {
	Status     int
	RetryAfter time.Duration
	Message    string
	// Code is the body's "error"; MinVersion and Download come with
	// 426 client_too_old (docs/AUTO_UPDATE.md §3).
	Code       string
	MinVersion string
	Download   string
}

// StatusClientTooOld is what the site answers a GF Sync it no longer
// accepts. v0.1.0 treats it like a server error (backs off, keeps the
// data), which is why it is not 400 or 403.
const StatusClientTooOld = http.StatusUpgradeRequired // 426

// ClientTooOld reports whether err is the site refusing this version.
func ClientTooOld(err error) (*HTTPError, bool) {
	var he *HTTPError
	if errors.As(err, &he) && he.Status == StatusClientTooOld {
		return he, true
	}
	return nil, false
}

// SafeDownloadURL is the page to open for a 426: the site's "download" if
// it is on the API host, otherwise the GF Sync release list.
func (c *Client) SafeDownloadURL(he *HTTPError) string {
	if he != nil {
		if u, err := url.Parse(he.Download); err == nil && he.Download != "" &&
			u.Scheme == c.Base.Scheme && strings.EqualFold(u.Host, c.Base.Host) {
			return u.String()
		}
	}
	return "https://github.com/GreenAndFangAdventure/gf-sync/releases/latest"
}

func (e *HTTPError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("HTTP %d: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("HTTP %d", e.Status)
}

// ErrUnauthorized means the token is missing, wrong or revoked.
var ErrUnauthorized = errors.New("api: device token rejected")

// ErrPairingEnded means the pairing code expired or was denied.
var ErrPairingEnded = errors.New("api: pairing code expired or denied")

// ErrPairingDenied is returned (wrapped together with ErrPairingEnded) when
// the player pressed "deny" on the pairing page.
var ErrPairingDenied = errors.New("api: pairing denied")

func (c *Client) endpoint(p string) string {
	u := *c.Base
	u.Path = strings.TrimRight(u.Path, "/") + p
	return u.String()
}

func (c *Client) do(ctx context.Context, method, path, token string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint(path), rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.Name+"/"+c.Version)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		he := &HTTPError{Status: resp.StatusCode, RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
		var eb struct {
			Error      string `json:"error"`
			Message    string `json:"message"`
			MinVersion string `json:"minVersion"`
			Download   string `json:"download"`
		}
		if json.Unmarshal(data, &eb) == nil {
			he.Message = firstNonEmpty(eb.Message, eb.Error)
			he.Code, he.MinVersion, he.Download = eb.Error, eb.MinVersion, eb.Download
		}
		return he
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("api: bad JSON from %s: %w", path, err)
		}
	}
	return nil
}

func firstNonEmpty(a ...string) string {
	for _, s := range a {
		if s != "" {
			return s
		}
	}
	return ""
}

func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
		return time.Duration(n) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

// DeviceStart is the §1 start response.
type DeviceStart struct {
	DeviceCode string `json:"deviceCode"`
	UserCode   string `json:"userCode"`
	VerifyURL  string `json:"verifyUrl"`
	Interval   int    `json:"interval"`
	ExpiresIn  int    `json:"expiresIn"`
}

// StartRequest is the §1 start body.
type StartRequest struct {
	Label         string `json:"label"`
	ClientVersion string `json:"clientVersion"`
}

// StartDevice begins pairing.
func (c *Client) StartDevice(ctx context.Context) (*DeviceStart, error) {
	var out DeviceStart
	label := c.Label
	if label == "" {
		label = "GF Sync"
	}
	if err := c.do(ctx, "POST", "/api/sync/device/start", "", StartRequest{
		Label: label, ClientVersion: c.Version,
	}, &out); err != nil {
		return nil, err
	}
	if out.DeviceCode == "" || out.UserCode == "" {
		return nil, errors.New("api: start response missing codes")
	}
	if out.Interval <= 0 {
		out.Interval = 5
	}
	if out.ExpiresIn <= 0 {
		out.ExpiresIn = 600
	}
	return &out, nil
}

// SafeVerifyURL returns the page the browser should open. The server's
// verifyUrl is used only if it is on the same scheme and host as the API
// base, so a tampered response cannot send the player elsewhere.
func (c *Client) SafeVerifyURL(s *DeviceStart) string {
	if u, err := url.Parse(s.VerifyURL); err == nil && s.VerifyURL != "" &&
		u.Scheme == c.Base.Scheme && strings.EqualFold(u.Host, c.Base.Host) {
		return u.String()
	}
	u := *c.Base
	u.Path = strings.TrimRight(u.Path, "/") + "/pages/sync-pair"
	u.RawQuery = url.Values{"code": {s.UserCode}}.Encode()
	return u.String()
}

// PollResult is the §1 poll response.
type PollResult struct {
	Pending  bool   `json:"pending"`
	Interval int    `json:"interval"` // seconds, may come with pending
	Token    string `json:"token"`
	DeviceID any    `json:"deviceId"` // informational only
	Error    string `json:"error"`
	SlowDown bool   `json:"-"`
}

// PollDevice checks whether the player approved.
func (c *Client) PollDevice(ctx context.Context, deviceCode string) (*PollResult, error) {
	var out PollResult
	err := c.do(ctx, "POST", "/api/sync/device/poll", "", map[string]string{"deviceCode": deviceCode}, &out)
	var he *HTTPError
	if errors.As(err, &he) {
		switch {
		case he.Status == 429:
			return &PollResult{Pending: true, SlowDown: true}, nil
		case he.Status == 403:
			return nil, fmt.Errorf("%w: %w (%v)", ErrPairingEnded, ErrPairingDenied, he)
		case he.Status >= 400 && he.Status < 500:
			return nil, fmt.Errorf("%w (%v)", ErrPairingEnded, he)
		}
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	switch out.Error {
	case "":
	case "authorization_pending":
		out.Pending = true
	case "slow_down":
		out.Pending, out.SlowDown = true, true
	case "denied", "access_denied":
		return nil, fmt.Errorf("%w: %w (%s)", ErrPairingEnded, ErrPairingDenied, out.Error)
	default:
		return nil, fmt.Errorf("%w (%s)", ErrPairingEnded, out.Error)
	}
	if !out.Pending && out.Token == "" {
		return nil, errors.New("api: poll response has neither pending nor token")
	}
	return &out, nil
}

// ItemResult is one entry of the §2 response.
type ItemResult struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
	// Skipped (roster.snapshot only) is informational and ignored.
	Skipped int `json:"skipped,omitempty"`
}

// UploadRequest is the §2 body.
type UploadRequest struct {
	Client map[string]string `json:"client"`
	Items  []json.RawMessage `json:"items"`
}

// NewUploadRequest builds the body for items. addonVersion is the
// SavedVariables addonVersion of the file the items come from (all items
// of one request share it); "" leaves client.addonVersion out.
func (c *Client) NewUploadRequest(addonVersion string, items []json.RawMessage) UploadRequest {
	cl := map[string]string{"name": c.Name, "version": c.Version}
	if addonVersion != "" {
		cl["addonVersion"] = addonVersion
	}
	return UploadRequest{Client: cl, Items: items}
}

// BodySize returns the encoded size of a request.
func BodySize(r UploadRequest) int {
	b, _ := json.Marshal(r)
	return len(b)
}

// Upload sends one batch; see NewUploadRequest for addonVersion.
func (c *Client) Upload(ctx context.Context, token, addonVersion string, items []json.RawMessage) ([]ItemResult, error) {
	var out struct {
		Results []ItemResult `json:"results"`
	}
	err := c.do(ctx, "POST", "/api/sync/upload", token, c.NewUploadRequest(addonVersion, items), &out)
	var he *HTTPError
	if errors.As(err, &he) && (he.Status == 401 || he.Status == 403) {
		return nil, fmt.Errorf("%w (%v)", ErrUnauthorized, he)
	}
	if err != nil {
		return nil, err
	}
	return out.Results, nil
}
