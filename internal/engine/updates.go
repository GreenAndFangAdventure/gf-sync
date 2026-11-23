package engine

import (
	"context"
	"errors"
	"math/rand"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"gfsync/internal/api"
	"gfsync/internal/update"
)

// ErrUpdatesOff: this build does not check for updates.
var ErrUpdatesOff = errors.New("update: checks are off in this build")

func isTooOld(err error) bool { _, ok := api.ClientTooOld(err); return ok }

var reShownVersion = regexp.MustCompile(`^\d{1,4}\.\d{1,4}\.\d{1,4}(-[0-9A-Za-z.-]{1,32})?$`)

// okVersion: a version from the network that may be shown to the player.
func okVersion(v string) bool { return reShownVersion.MatchString(v) }

// oneLine trims network text for a notification: one line, at most n
// characters.
func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

// maybeCheckUpdate starts a scheduled check in the background.
func (e *Engine) maybeCheckUpdate(ctx context.Context) {
	e.mu.Lock()
	due := e.cfg.CheckUpdate != nil && !e.updChecking && !e.nextUpdate.IsZero() && !e.cfg.Now().Before(e.nextUpdate)
	if due {
		e.updChecking = true
	}
	e.mu.Unlock()
	if due {
		go func() { _, _ = e.runUpdateCheck(ctx) }()
	}
}

// CheckUpdateNow checks at once (the tray's 「檢查更新」) and returns the
// newer version, or nil when this one is the latest. It returns
// ErrUpdatesOff when checks are off (development builds) and
// update.ErrNotFound when nothing is published yet.
func (e *Engine) CheckUpdateNow(ctx context.Context) (*Update, error) {
	e.mu.Lock()
	if e.cfg.CheckUpdate == nil {
		e.mu.Unlock()
		return nil, ErrUpdatesOff
	}
	if e.updChecking {
		e.mu.Unlock()
		return nil, errors.New("update: a check is already running")
	}
	e.updChecking = true
	e.mu.Unlock()
	if _, err := e.runUpdateCheck(ctx); err != nil {
		return nil, err
	}
	return e.Status().Update, nil
}

func (e *Engine) runUpdateCheck(ctx context.Context) (*update.Result, error) {
	cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	r, err := e.cfg.CheckUpdate(cctx)
	cancel()
	e.mu.Lock()
	now := e.cfg.Now()
	e.updChecking = false
	jitter := time.Duration(rand.Int63n(int64(e.cfg.UpdateEvery)/12 + 1))
	e.nextUpdate = now.Add(e.cfg.UpdateEvery + jitter)
	switch {
	case errors.Is(err, update.ErrNotFound):
		e.cfg.Logf("update: no release manifest published yet")
	case err != nil:
		e.cfg.Logf("update: check failed: %v", err)
	default:
		e.upd = r
		if r.Newer {
			e.cfg.Logf("update: %s available (running %s, from %s, required %v); notify only", r.Manifest.Version, r.Current, r.Source, r.Required)
		} else {
			e.cfg.Logf("update: up to date (%s, latest %s)", r.Current, r.Manifest.Version)
		}
	}
	e.mu.Unlock()
	e.notify()
	return r, err
}
