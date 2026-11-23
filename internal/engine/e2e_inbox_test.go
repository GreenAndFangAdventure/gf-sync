package engine_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gfsync/internal/engine"
	"gfsync/internal/inbox"
	"gfsync/internal/mockapi"
)

// Made-up realm/guild and GUIDs.
func inboxBody(at string, orders int) string {
	b := "H;realm=%E6%B8%AC%E8%A9%A6;guild=%E7%B6%A0%E7%9A%AE;at=" + at + ";n=" + string(rune('0'+orders)) +
		"~C;cu=Player-0000-0000000B;t=1763899000"
	for i := 0; i < orders; i++ {
		b += "~O;id=w1" + string(rune('0'+i)) + ";cr=1763890000;cf=*;rc=2660;q=5;fe=500;st=open;sa=1763890000"
	}
	return b
}

func withInbox(h *harness) string {
	h.cfg.Inbox = &engine.InboxConfig{
		Keys:  []ed25519.PublicKey{inbox.TestPublicKey()},
		First: 50 * time.Millisecond,
		Every: 80 * time.Millisecond,
	}
	return filepath.Join(h.forever, "Interface", "AddOns", inbox.Folder)
}

func fileIs(path, want string) func() bool {
	return func() bool {
		b, err := os.ReadFile(path)
		return err == nil && string(b) == want
	}
}

func TestE2EInboxWrittenAndKept(t *testing.T) {
	h := newHarness(t)
	dir := withInbox(h)
	lua, toc := filepath.Join(dir, inbox.LuaName), filepath.Join(dir, inbox.TOCName)
	first := mockapi.SignInbox(mockapi.InboxTestKey(), inboxBody("1763900000", 2))
	h.mock.SetInbox(inboxBody("1763900000", 2))
	h.start()
	h.waitFor("Inbox.lua written", fileIs(lua, string(inbox.LuaFile(first))))
	h.waitFor(".toc written", fileIs(toc, inbox.TOC))
	h.waitForLog("inbox: wrote 2 web orders")

	// Someone edits the files: put back on the next round (the site
	// answers 304 meanwhile).
	os.WriteFile(toc, []byte(inbox.TOC+"Evil.lua\n"), 0o644)
	os.WriteFile(lua, []byte("GreenFangToolkit_Inbox = os.execute\n"), 0o644)
	h.waitFor(".toc restored", fileIs(toc, inbox.TOC))
	h.waitFor("Inbox.lua restored", fileIs(lua, string(inbox.LuaFile(first))))

	// A newer list replaces it.
	second := mockapi.SignInbox(mockapi.InboxTestKey(), inboxBody("1763900600", 1))
	h.mock.SetInbox(inboxBody("1763900600", 1))
	h.waitFor("newer list", fileIs(lua, string(inbox.LuaFile(second))))

	// Refused lists leave the file alone.
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	calls := h.mock.InboxCalls()
	for _, bad := range []string{
		mockapi.SignInbox(other, inboxBody("1763901200", 1)),                                                        // not the site's key
		strings.Replace(mockapi.SignInbox(mockapi.InboxTestKey(), inboxBody("1763901200", 1)), "fe=500", "fe=1", 1), // edited on the way
		mockapi.SignInbox(mockapi.InboxTestKey(), inboxBody("1763800000", 1)),                                       // older than the one written
		mockapi.SignInbox(mockapi.InboxTestKey(), `H;realm=..\..\WTF;guild=x;at=1763901200;n=0`),
	} {
		h.mock.SetInboxRaw(bad)
		h.waitFor("site asked again", func() bool { return h.mock.InboxCalls() >= calls+2 })
		calls = h.mock.InboxCalls()
		if b, _ := os.ReadFile(lua); string(b) != string(inbox.LuaFile(second)) {
			t.Fatalf("refused list was written: %q", b)
		}
	}
	h.waitForLog("list refused, files unchanged: inbox: signature does not verify")
	h.waitForLog("list refused, older than the one written")
	h.stop()

	// The log has counts, never the list itself.
	for _, l := range h.logs {
		if strings.Contains(l, "GFIN1") || strings.Contains(l, "Player-0000") || strings.Contains(l, "%E6") {
			t.Errorf("list content in the log: %q", l)
		}
	}
	// Only the inbox folder was added next to the toolkit.
	ents, _ := os.ReadDir(filepath.Join(h.forever, "Interface", "AddOns"))
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "GreenFangToolkit,"+inbox.Folder {
		t.Fatalf("AddOns = %v", names)
	}
}

func TestE2EInboxSiteWithoutFeature(t *testing.T) {
	h := newHarness(t)
	dir := withInbox(h)
	h.start() // the mock answers 404: no list yet
	h.waitForLog("the site has no pending-orders list yet")
	h.waitFor(".toc there before any list", fileIs(filepath.Join(dir, inbox.TOCName), inbox.TOC))
	time.Sleep(300 * time.Millisecond)
	h.stop()
	if _, err := os.Stat(filepath.Join(dir, inbox.LuaName)); err == nil {
		t.Fatal("Inbox.lua written without a signed list")
	}
	n := 0
	for _, l := range h.logs {
		if strings.Contains(l, "no pending-orders list") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("404 logged %d times", n)
	}
}

// Off by default: no InboxConfig, nothing in AddOns.
func TestE2EInboxOffByDefault(t *testing.T) {
	h := newHarness(t)
	h.mock.SetInbox(inboxBody("1763900000", 0))
	h.writeSV("A", sv(2, true, "a:1:character.self"))
	h.start()
	h.waitFor("upload", func() bool { return h.receivedIDs()["a:1:character.self"] })
	time.Sleep(200 * time.Millisecond)
	h.stop()
	if h.mock.InboxCalls() != 0 {
		t.Fatal("asked for the inbox while off")
	}
	if _, err := os.Stat(filepath.Join(h.forever, "Interface", "AddOns", inbox.Folder)); err == nil {
		t.Fatal("inbox folder created while off")
	}
}
