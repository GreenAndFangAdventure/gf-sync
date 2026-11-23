package engine

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"strings"
	"time"

	"gfsync/internal/api"
	"gfsync/internal/inbox"
)

// InboxConfig turns on writing the site's pending web orders into
// GreenFangToolkit_Inbox (D-018, version 1.5). nil in Config = off, which
// is the default until the site has the API and a production key.
type InboxConfig struct {
	// Keys are the site's Ed25519 public keys; a list is written only if
	// one of them verifies its signature. Empty = the feature stays off.
	Keys  []ed25519.PublicKey
	First time.Duration // first check after Run (default 30s)
	Every time.Duration // then every this long (default 10m)
}

// inboxState belongs to inboxLoop alone (no lock).
type inboxState struct {
	last     string // last verified string written ("" = none yet)
	etag     string
	at       int64  // H.at of last: an older list is never written over it
	no404    bool   // "site has no inbox" already logged
	lastErrs string // last logged error: each is logged once until it changes
}

func (e *Engine) inboxLoop(ctx context.Context) {
	c := e.cfg.Inbox
	first, every := c.First, c.Every
	if first <= 0 {
		first = 30 * time.Second
	}
	if every <= 0 {
		every = 10 * time.Minute
	}
	var st inboxState
	t := time.NewTimer(first)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			e.inboxOnce(ctx, &st)
			t.Reset(every)
		}
	}
}

// inboxOnce keeps the folder right (fixed .toc, last verified Inbox.lua;
// anything changed by hand is put back), then asks the site for a newer
// list and writes it only if its signature verifies.
func (e *Engine) inboxOnce(ctx context.Context, st *inboxState) {
	e.mu.Lock()
	token, dir, paused := e.token, e.gameDir, e.st.Paused
	e.mu.Unlock()
	if dir == "" {
		return
	}
	w := inbox.NewWriter(dir)
	if wrote, err := w.Sync(st.last); err != nil {
		e.inboxLogOnce(st, "inbox: cannot keep "+inbox.Folder+": %v", err)
		return
	} else if len(wrote) > 0 {
		e.cfg.Logf("inbox: wrote %s", strings.Join(wrote, ", "))
	}
	if token == "" || paused {
		return
	}
	res, err := e.cfg.Client.Inbox(ctx, token, st.etag)
	var he *api.HTTPError
	switch {
	case errors.As(err, &he) && he.Status == 404:
		if !st.no404 {
			st.no404 = true
			e.cfg.Logf("inbox: the site has no pending-orders list yet (HTTP 404)")
		}
		return
	case err != nil:
		e.inboxLogOnce(st, "inbox: %v", err)
		e.logHTTPDiag("inbox", err)
		return
	case res.NotModified:
		return
	}
	st.no404 = false
	info, err := inbox.Verify(res.Inbox, e.cfg.Inbox.Keys)
	if err != nil {
		e.inboxLogOnce(st, "inbox: list refused, files unchanged: %v", err)
		return
	}
	if info.At < st.at {
		e.inboxLogOnce(st, "inbox: list refused, older than the one written (%v)", time.Unix(info.At, 0).UTC())
		return
	}
	wrote, err := w.Sync(res.Inbox)
	if err != nil {
		e.inboxLogOnce(st, "inbox: writing failed, previous file kept: %v", err)
		return
	}
	st.last, st.etag, st.at, st.lastErrs = res.Inbox, res.ETag, info.At, ""
	if len(wrote) > 0 {
		// Counts only: the list holds GUIDs and the guild's name.
		e.cfg.Logf("inbox: wrote %d web orders (%s)", info.Orders, strings.Join(wrote, ", "))
	}
}

// inboxLogOnce logs a message once until a different one happens.
func (e *Engine) inboxLogOnce(st *inboxState, format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	if msg == st.lastErrs {
		return
	}
	st.lastErrs = msg
	e.cfg.Logf("%s", msg)
}
