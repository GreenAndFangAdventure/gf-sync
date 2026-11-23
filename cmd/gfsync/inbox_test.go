package main

import (
	"testing"

	"gfsync/internal/api"
)

func TestInboxConfigSwitchAndKeys(t *testing.T) {
	if inboxFeature != "" {
		t.Fatal("the inbox switch must be off in normal builds")
	}
	// Off unless the build switch is on.
	for _, base := range []string{api.DefaultBase, "http://127.0.0.1:8787"} {
		if c, why := inboxConfig("", base); c != nil || why != "" {
			t.Errorf("%s: switch off but got %v %q", base, c, why)
		}
	}
	// On, production site: no production key yet, so still off.
	if c, why := inboxConfig("on", api.DefaultBase); c != nil || why == "" {
		t.Errorf("production without key: %v %q", c, why)
	}
	// The test key is only for a loopback server.
	for _, base := range []string{"https://127.0.0.1.example.com", "https://localhost.evil.test", "https://greenandfangadventure.com"} {
		if c, _ := inboxConfig("on", base); c != nil {
			t.Errorf("%s: test key accepted", base)
		}
	}
	if c, _ := inboxConfig("on", "http://127.0.0.1:8787"); c == nil || len(c.Keys) != 1 {
		t.Errorf("mock: %v", c)
	}
}
