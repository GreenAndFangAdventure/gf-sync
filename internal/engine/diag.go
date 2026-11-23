package engine

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"gfsync/internal/api"
)

// DiagEvery is how often the same kind of unrecognized error response
// (same HTTP status and CF-Ray data center) is described in the log.
// During a long outage the backoff retries many times; the body is
// logged once per hour, the short "HTTP 503; retry in …" line every time.
const DiagEvery = time.Hour

// diagLimiter remembers when each kind of error response was last
// described in the log.
type diagLimiter struct {
	mu   sync.Mutex
	last map[string]time.Time
}

// allow reports whether key may be logged at now, and records it.
func (d *diagLimiter) allow(key string, now time.Time) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if t, ok := d.last[key]; ok && now.Sub(t) < DiagEvery && !now.Before(t) {
		return false
	}
	if d.last == nil {
		d.last = map[string]time.Time{}
	}
	d.last[key] = now
	return true
}

// diagKey groups error responses: the status and the CF-Ray's data
// center ("8f00…a1b2-TPE" → "TPE"). The hex part of a CF-Ray is new for
// every request, so keying on it would log the body on every retry.
func diagKey(he *api.HTTPError) string {
	colo := "-"
	if i := strings.LastIndexByte(he.CFRay, '-'); i >= 0 && i < len(he.CFRay)-1 {
		colo = he.CFRay[i+1:]
	} else if he.CFRay != "" {
		colo = "?"
	}
	return fmt.Sprintf("%d %s", he.Status, colo)
}

// logHTTPDiag logs what the site sent back when err is an HTTP error
// whose body we did not understand (api.HTTPError.Diag), at most once per
// DiagEvery for the same diagKey. Safe to call with or without e.mu held.
func (e *Engine) logHTTPDiag(op string, err error) {
	var he *api.HTTPError
	if !errors.As(err, &he) || he.Diag == "" {
		return
	}
	if !e.diag.allow(diagKey(he), e.cfg.Now()) {
		return
	}
	e.cfg.Logf("%s: HTTP %d response: %s (same kind not repeated for %s)", op, he.Status, he.Diag, DiagEvery)
}
