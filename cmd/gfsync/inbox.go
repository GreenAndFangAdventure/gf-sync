package main

import (
	"crypto/ed25519"
	"net/url"

	"gfsync/internal/engine"
	"gfsync/internal/inbox"
)

// inboxFeature turns on writing web orders into GreenFangToolkit_Inbox
// (D-018, version 1.5). It is a build switch, off in every normal build:
//
//	go build -ldflags "-X main.inboxFeature=on" ./cmd/gfsync
//
// Even when on, the feature needs a key: the production keys in
// inbox.SiteKeys (none yet), or the mock's test key, which is accepted
// only from a loopback API (gfsync-mock).
var inboxFeature = ""

// inboxConfig returns the engine setting for this build and API base, or
// nil and the reason it stays off.
func inboxConfig(feature, base string) (*engine.InboxConfig, string) {
	if feature != "on" {
		return nil, ""
	}
	keys := append([]ed25519.PublicKey{}, inbox.SiteKeys...)
	if u, err := url.Parse(base); err == nil && u.Scheme == "http" &&
		(u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1") {
		if k := inbox.TestPublicKey(); k != nil {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return nil, "inbox: built with the inbox switch but there is no site key yet; off"
	}
	return &engine.InboxConfig{Keys: keys}, ""
}
