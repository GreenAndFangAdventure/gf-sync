package mockapi

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"

	"gfsync/internal/inbox"
)

// InboxTestKey is the mock site's inbox signing key, derived from a fixed,
// public seed. It is a TEST key: anyone can sign with it. GF Sync accepts
// its public half (inbox.TestPublicKeyHex) only from a loopback server.
func InboxTestKey() ed25519.PrivateKey {
	seed := sha256.Sum256([]byte("gfsync-mock inbox test key; never use in production"))
	return ed25519.NewKeyFromSeed(seed[:])
}

// SignInbox builds a signed GFIN1 string from body ("H;...~C;...~O;...")
// with key, the way the site would (docs/INBOX_SIGNING.md).
func SignInbox(key ed25519.PrivateKey, body string) string {
	sum := inbox.Sum(body)
	sig := ed25519.Sign(key, inbox.SignedMessage(sum, body))
	return inbox.Version + "~" + sum + "~" + base64.RawURLEncoding.EncodeToString(sig) + "~" + body
}

// SetInbox makes GET /api/sync/inbox answer body signed with the test key.
func (s *Server) SetInbox(body string) { s.SetInboxRaw(SignInbox(InboxTestKey(), body)) }

// SetInboxRaw makes the endpoint answer exactly s (tests: bad signatures,
// tampered strings). "" answers 404 (a site without the feature).
func (s *Server) SetInboxRaw(str string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inbox = str
}

// InboxCalls returns how many inbox requests were made (304s included).
func (s *Server) InboxCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inboxCalls
}

func (s *Server) inboxGet(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inboxCalls++
	tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if d := s.devices[hash(tok)]; !ok || d == nil || d.revoked {
		writeJSON(w, 401, map[string]string{"error": "invalid_token"})
		return
	}
	if s.inbox == "" {
		writeJSON(w, 404, map[string]string{"error": "not_found"})
		return
	}
	h := sha256.Sum256([]byte(s.inbox))
	etag := `"` + hex.EncodeToString(h[:8]) + `"`
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("ETag", etag)
	writeJSON(w, 200, map[string]string{"inbox": s.inbox})
}
