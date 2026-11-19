package engine_test

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gfsync/internal/api"
	"gfsync/internal/mockapi"
)

// LOG #058: answers from one site must not stop uploads to another.
// Switching the API base (staging -> production) and pairing there sends
// every entry again.
func TestE2ESwitchSiteSendsAgain(t *testing.T) {
	const id = "site:1:character.self"
	h := newHarness(t)
	h.writeSV("A", sv(2, true, id))
	h.start()
	h.waitFor("upload to the first site", func() bool { return h.receivedIDs()[id] })
	h.stop()

	second := mockapi.New()
	srv := httptest.NewServer(second)
	t.Cleanup(srv.Close)
	second.BaseURL = srv.URL
	client, err := api.New(srv.URL, "0.1.0-test")
	if err != nil {
		t.Fatal(err)
	}
	client.Label = "TEST-PC"
	h.mock = second // OpenBrowser approves on the current mock
	h.cfg.Client = client
	h.start()
	// The scope changed with the base alone, before any new pairing.
	h.waitForLog("answered by another site or device")
	// The first site's token is unknown here: pair again from the tray.
	h.waitFor("unpaired", func() bool {
		s := h.eng.Status()
		return s.Ready && !s.Paired && strings.Contains(s.LastError, "重新配對")
	})
	h.eng.StartPairing()
	h.waitFor("upload to the second site", func() bool { return h.receivedIDs()[id] })
}

// Pairing again as a new device on the same site also sends everything
// again; the site answers duplicate, which is safe.
func TestE2ENewDeviceSendsAgain(t *testing.T) {
	const id = "dev:1:character.self"
	h := newHarness(t)
	h.writeSV("A", sv(2, true, id))
	h.start()
	h.waitFor("first upload", func() bool { return h.receivedIDs()[id] })
	h.stop()

	// Same token, restart: nothing is re-sent.
	calls := h.mock.Calls()
	h.start()
	time.Sleep(600 * time.Millisecond)
	if h.mock.Calls() != calls {
		t.Fatalf("same device re-uploaded (%d -> %d calls)", calls, h.mock.Calls())
	}
	h.stop()

	if err := h.tokens.Clear(); err != nil {
		t.Fatal(err)
	}
	h.start()
	h.waitFor("re-send after pairing again", func() bool { return h.mock.Calls() > calls })
	h.waitForLog("answered by another site or device")
	h.waitFor("no pending", func() bool { s := h.eng.Status(); return s.Paired && s.Pending == 0 })
	if got := h.mock.Status(id); got != "accepted" {
		t.Fatalf("first answer %q", got)
	}
}
