// Package mockapi is an offline stand-in for the site's /api/sync/*
// endpoints (upload API v1), used by end-to-end tests and
// by cmd/gfsync-mock for manual runs. It keeps everything in memory.
package mockapi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"gfsync/internal/update"
)

// Guild is the only accepted guild (upload API v1 §3.1).
const Guild = "綠皮與獠牙冒險事務所"

type pairing struct {
	deviceCode, userCode string
	label, clientVersion string
	expires              time.Time
	approved, denied     bool
	tokenIssued          bool
	token                string
}

type device struct {
	id       int
	revoked  bool
	uploads  []time.Time
	lastSeen time.Time
}

type failure struct {
	status     int
	retryAfter int
}

// Server is the mock. Zero value is not usable; call New.
type Server struct {
	mu        sync.Mutex
	Now       func() time.Time
	Interval  int // seconds suggested to the poller
	ExpiresIn int
	BaseURL   string // used to build verifyUrl; set by the caller

	unknownKinds map[string]bool // kinds answered with "unknown_kind" (an old site)
	addonTooOld  map[string]bool // kinds answered with "addon_too_old"
	minClient    string          // GF Sync below this gets 426 client_too_old

	pairings      map[string]*pairing // by deviceCode
	devices       map[string]*device  // by token hash
	seen          map[string]string   // id -> first status
	received      []json.RawMessage
	labels        []string
	calls         int
	failures      []failure
	startFailures []failure
	nextID        int
}

// New returns a mock with short intervals.
func New() *Server {
	return &Server{
		Now: time.Now, Interval: 1, ExpiresIn: 600,
		pairings: map[string]*pairing{}, devices: map[string]*device{}, seen: map[string]string{},
	}
}

func randCode(n int, alphabet string) string {
	b := make([]byte, n)
	rand.Read(b)
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}

func hash(tok string) string {
	h := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(h[:])
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// ServeHTTP routes requests.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "POST" && r.URL.Path == "/api/sync/device/start":
		s.start(w, r)
	case r.Method == "POST" && r.URL.Path == "/api/sync/device/poll":
		s.poll(w, r)
	case r.Method == "POST" && r.URL.Path == "/api/sync/upload":
		s.upload(w, r)
	case r.URL.Path == "/pages/sync-pair":
		s.pairPage(w, r)
	case r.URL.Path == "/" || r.URL.Path == "/pages/forever":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<h1>GF Sync mock server</h1>")
	default:
		writeJSON(w, 404, map[string]string{"error": "not_found"})
	}
}

func (s *Server) start(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Label         string `json:"label"`
		ClientVersion string `json:"clientVersion"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body) != nil || body.Label == "" || body.ClientVersion == "" {
		writeJSON(w, 400, map[string]string{"error": "label_and_clientVersion_required"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tooOldLocked(w, body.ClientVersion) {
		return
	}
	if len(s.startFailures) > 0 {
		f := s.startFailures[0]
		s.startFailures = s.startFailures[1:]
		writeJSON(w, f.status, map[string]string{"error": "test_failure"})
		return
	}
	s.labels = append(s.labels, body.Label)
	p := &pairing{
		label: body.Label, clientVersion: body.ClientVersion,
		deviceCode: randCode(32, "abcdefghijklmnopqrstuvwxyz0123456789"),
		userCode:   randCode(8, "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"),
		expires:    s.Now().Add(time.Duration(s.ExpiresIn) * time.Second),
	}
	s.pairings[p.deviceCode] = p
	writeJSON(w, 200, map[string]any{
		"deviceCode": p.deviceCode, "userCode": p.userCode,
		"verifyUrl": strings.TrimRight(s.BaseURL, "/") + "/pages/sync-pair?code=" + p.userCode,
		"interval":  s.Interval, "expiresIn": s.ExpiresIn,
	})
}

func (s *Server) poll(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DeviceCode string `json:"deviceCode"`
	}
	json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body)
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.pairings[body.DeviceCode]
	switch {
	case p == nil:
		writeJSON(w, 404, map[string]string{"error": "unknown_device_code"})
	case p.denied:
		writeJSON(w, 403, map[string]string{"error": "denied"})
	case s.Now().After(p.expires):
		writeJSON(w, 410, map[string]string{"error": "expired_token"})
	case !p.approved:
		writeJSON(w, 200, map[string]any{"pending": true, "interval": s.Interval})
	case p.tokenIssued:
		// token is returned only once
		writeJSON(w, 410, map[string]string{"error": "already_used"})
	default:
		raw := make([]byte, 32)
		rand.Read(raw)
		tok := base64.RawURLEncoding.EncodeToString(raw)
		p.tokenIssued = true
		s.nextID++
		s.devices[hash(tok)] = &device{id: s.nextID}
		writeJSON(w, 200, map[string]any{"token": tok, "deviceId": s.nextID})
	}
}

func (s *Server) pairPage(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Method == "POST" {
		r.ParseForm()
		code = r.FormValue("code")
		if s.Approve(code) {
			fmt.Fprint(w, "<p>已允許，這台電腦可以同步了。可以關閉這個分頁。</p>")
		} else {
			w.WriteHeader(400)
			fmt.Fprint(w, "<p>配對碼無效或已過期。</p>")
		}
		return
	}
	fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>GF Sync 配對（模擬）</title>
<h1>GF Sync 配對（模擬伺服器）</h1><p>電腦：<b>%s</b></p><p>配對碼：<b>%s</b></p>
<form method="post"><input type="hidden" name="code" value="%s"><button>允許這台電腦同步</button></form>`,
		html.EscapeString(s.labelFor(code)), html.EscapeString(code), html.EscapeString(code))
}

func (s *Server) labelFor(userCode string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.pairings {
		if p.userCode == userCode {
			return p.label
		}
	}
	return ""
}

// Labels returns the computer names sent to /device/start.
func (s *Server) Labels() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.labels...)
}

// Approve simulates the player pressing "allow" for userCode.
func (s *Server) Approve(userCode string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.pairings {
		if p.userCode == userCode && !s.Now().After(p.expires) && !p.denied {
			p.approved = true
			return true
		}
	}
	return false
}

// Deny simulates the player refusing.
func (s *Server) Deny(userCode string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.pairings {
		if p.userCode == userCode {
			p.denied = true
		}
	}
}

// SetUnknownKinds makes the mock behave like a site that does not know
// these kinds yet: it answers rejected / "unknown_kind" and does not
// remember the id, as upload-api v1.1 §2 expects. No kinds = all known.
func (s *Server) SetUnknownKinds(kinds ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unknownKinds = map[string]bool{}
	for _, k := range kinds {
		s.unknownKinds[k] = true
	}
}

// SetMinClient makes the mock refuse GF Sync versions below v with 426
// client_too_old on start and upload (docs/AUTO_UPDATE.md §3). "" = off.
func (s *Server) SetMinClient(v string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.minClient = v
}

// SetAddonTooOld answers entries of these kinds with rejected /
// "addon_too_old", without remembering the id. No kinds = off.
func (s *Server) SetAddonTooOld(kinds ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addonTooOld = map[string]bool{}
	for _, k := range kinds {
		s.addonTooOld[k] = true
	}
}

// tooOldLocked writes the 426 answer if version is below minClient.
func (s *Server) tooOldLocked(w http.ResponseWriter, version string) bool {
	if s.minClient == "" || update.Compare(version, s.minClient) >= 0 {
		return false
	}
	writeJSON(w, 426, map[string]string{
		"error":      "client_too_old",
		"minVersion": s.minClient,
		"download":   s.BaseURL + "/pages/sync",
	})
	return true
}

// knownKinds are the kinds this mock (upload-api v1.1) accepts.
var knownKinds = map[string]bool{
	"character.self": true, "roster.snapshot": true, "member.report": true,
	"member.share": true, "recipe.names": true,
}

// RevokeAll revokes every device (as /pages/sync-devices would).
func (s *Server) RevokeAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.devices {
		d.revoked = true
	}
}

// FailNext makes the next upload calls fail with status (and Retry-After
// seconds when > 0), one call per entry.
func (s *Server) FailNext(status, retryAfter, times int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := 0; i < times; i++ {
		s.failures = append(s.failures, failure{status, retryAfter})
	}
}

// FailStartNext makes the next device/start calls fail with status.
func (s *Server) FailStartNext(status, times int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := 0; i < times; i++ {
		s.startFailures = append(s.startFailures, failure{status: status})
	}
}

// DeviceCodes returns every device code handed out (tests check that it
// never leaks into the client's status).
func (s *Server) DeviceCodes() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for c := range s.pairings {
		out = append(out, c)
	}
	return out
}

// Received returns every accepted item so far.
func (s *Server) Received() []json.RawMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]json.RawMessage(nil), s.received...)
}

// Calls returns the number of upload requests (including failures).
func (s *Server) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// Status returns the first answer given for id ("" if never seen).
func (s *Server) Status(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seen[id]
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if len(s.failures) > 0 {
		f := s.failures[0]
		s.failures = s.failures[1:]
		if f.retryAfter > 0 {
			w.Header().Set("Retry-After", fmt.Sprint(f.retryAfter))
		}
		writeJSON(w, f.status, map[string]string{"error": "injected"})
		return
	}
	auth := r.Header.Get("Authorization")
	tok, ok := strings.CutPrefix(auth, "Bearer ")
	d := s.devices[hash(tok)]
	if !ok || d == nil || d.revoked {
		writeJSON(w, 401, map[string]string{"error": "invalid_token"})
		return
	}
	now := s.Now()
	recent := d.uploads[:0]
	for _, t := range d.uploads {
		if now.Sub(t) < time.Hour {
			recent = append(recent, t)
		}
	}
	d.uploads = recent
	if len(d.uploads) >= 60 {
		w.Header().Set("Retry-After", fmt.Sprint(int(time.Hour-now.Sub(d.uploads[0]))/int(time.Second)+1))
		writeJSON(w, 429, map[string]string{"error": "rate_limited"})
		return
	}
	d.uploads = append(d.uploads, now)
	d.lastSeen = now
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20+1))
	if err != nil || len(body) > 1<<20 {
		writeJSON(w, 413, map[string]string{"error": "too_large"})
		return
	}
	var req struct {
		Client struct{ Name, Version string } `json:"client"`
		Items  []json.RawMessage              `json:"items"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Client.Name == "" {
		writeJSON(w, 400, map[string]string{"error": "bad_request"})
		return
	}
	if s.tooOldLocked(w, req.Client.Version) {
		return
	}
	if len(req.Items) > 50 {
		writeJSON(w, 413, map[string]string{"error": "too_many_items"})
		return
	}
	type result struct {
		ID      string `json:"id"`
		Status  string `json:"status"`
		Reason  string `json:"reason,omitempty"`
		Skipped *int   `json:"skipped,omitempty"`
	}
	results := []result{}
	for _, raw := range req.Items {
		var it struct {
			ID         string  `json:"id"`
			Kind       string  `json:"kind"`
			CapturedAt float64 `json:"capturedAt"`
			// v1.1: preferred over capturedAt when it is a number.
			CapturedAtServer any `json:"capturedAtServer"`
			Game             struct {
				World string `json:"world"`
			} `json:"game"`
			Uploader struct {
				Guild string `json:"guild"`
			} `json:"uploader"`
		}
		if json.Unmarshal(raw, &it) != nil || it.ID == "" {
			results = append(results, result{Status: "rejected", Reason: "malformed"})
			continue
		}
		if !knownKinds[it.Kind] || s.unknownKinds[it.Kind] {
			// Not remembered: once the site learns the kind, the same id
			// is accepted (upload-api v1.1 §2).
			results = append(results, result{ID: it.ID, Status: "rejected", Reason: "unknown_kind"})
			continue
		}
		if s.addonTooOld[it.Kind] {
			results = append(results, result{ID: it.ID, Status: "rejected", Reason: "addon_too_old"})
			continue
		}
		if _, dup := s.seen[it.ID]; dup {
			results = append(results, result{ID: it.ID, Status: "duplicate"})
			continue
		}
		when := it.CapturedAt
		if t, ok := it.CapturedAtServer.(float64); ok {
			when = t
		}
		reason := ""
		switch {
		case it.Game.World != "forever":
			reason = "wrong_world"
		case it.Uploader.Guild != Guild:
			reason = "wrong_guild"
		case when > float64(now.Add(10*time.Minute).Unix()):
			reason = "captured_in_future"
		case hasPrivateField(raw, it.Kind):
			reason = "private_field"
		case (it.Kind == "member.report" || it.Kind == "member.share") && !subjectOnly(raw):
			reason = "bad_subject"
		}
		if reason != "" {
			s.seen[it.ID] = "rejected"
			results = append(results, result{ID: it.ID, Status: "rejected", Reason: reason})
			continue
		}
		s.seen[it.ID] = "accepted"
		s.received = append(s.received, raw)
		res := result{ID: it.ID, Status: "accepted"}
		if it.Kind == "roster.snapshot" {
			zero := 0
			res.Skipped = &zero // the site sends this; GF Sync ignores it
		}
		results = append(results, res)
	}
	writeJSON(w, 200, map[string]any{"results": results})
}

// hasPrivateField reports zone, online or location anywhere in an item,
// except a character.self or member.report payload.location holding only
// the zone and optional subZone strings (decisions D-009, D-011).
func hasPrivateField(raw []byte, kind string) bool {
	var item map[string]any
	if json.Unmarshal(raw, &item) != nil {
		return true
	}
	if kind == "member.share" {
		if p, ok := item["payload"].(map[string]any); ok {
			// withdrawn.location is a time, not a place (upload-api v1.1 §3.7).
			delete(p, "withdrawn")
			if loc, ok := p["location"]; ok {
				m, _ := loc.(map[string]any)
				if m == nil || !ownLocationOnly(m["value"]) {
					return true
				}
				for k := range m {
					if k != "value" && k != "recordedAt" && k != "via" {
						return true
					}
				}
				delete(p, "location")
			}
		}
	}
	if kind == "character.self" || kind == "member.report" {
		if p, ok := item["payload"].(map[string]any); ok {
			if loc, ok := p["location"]; ok {
				if !ownLocationOnly(loc) {
					return true
				}
				delete(p, "location")
			}
		}
	}
	return anyPrivateKey(item)
}

func ownLocationOnly(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	if _, ok := m["zone"].(string); !ok {
		return false
	}
	for k, val := range m {
		if _, ok := val.(string); !ok || (k != "zone" && k != "subZone") {
			return false
		}
	}
	return true
}

func anyPrivateKey(v any) bool {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if k == "zone" || k == "online" || k == "location" || anyPrivateKey(val) {
				return true
			}
		}
	case []any:
		for _, val := range t {
			if anyPrivateKey(val) {
				return true
			}
		}
	}
	return false
}

// subjectOnly reports whether a member.report payload.subject is present
// with a name and nothing but name, realm and guid strings (D-011).
func subjectOnly(raw []byte) bool {
	var item struct {
		Payload struct {
			Subject map[string]any `json:"subject"`
		} `json:"payload"`
	}
	if json.Unmarshal(raw, &item) != nil {
		return false
	}
	if n, _ := item.Payload.Subject["name"].(string); n == "" {
		return false
	}
	for k, v := range item.Payload.Subject {
		if _, ok := v.(string); !ok || (k != "name" && k != "realm" && k != "guid") {
			return false
		}
	}
	return true
}
