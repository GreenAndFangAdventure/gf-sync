package engine_test

// End-to-end, offline: real engine + real HTTP client + mock site
// (internal/mockapi) + a fake `_forever_` folder on disk.

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gfsync/internal/api"
	"gfsync/internal/engine"
	"gfsync/internal/mockapi"
	"gfsync/internal/secret"
	"gfsync/internal/state"
)

type harness struct {
	t       *testing.T
	mock    *mockapi.Server
	srv     *httptest.Server
	forever string
	data    string
	tokens  *secret.DPAPIStore
	cfg     engine.Config
	eng     *engine.Engine
	cancel  context.CancelFunc
	done    chan struct{}
	logMu   sync.Mutex
	logs    []string
	opened  []string
}

func newHarness(t *testing.T) *harness {
	h := &harness{t: t, mock: mockapi.New()}
	h.srv = httptest.NewServer(h.mock)
	h.mock.BaseURL = h.srv.URL
	t.Cleanup(h.srv.Close)
	root := t.TempDir()
	h.forever = filepath.Join(root, "World of Warcraft", "_forever_")
	os.MkdirAll(filepath.Join(h.forever, "Interface", "AddOns", "GreenFangToolkit"), 0o755)
	os.WriteFile(filepath.Join(h.forever, "Interface", "AddOns", "GreenFangToolkit", "GreenFangToolkit.toc"), []byte("## Title: x"), 0o644)
	h.data = filepath.Join(root, "AppData", "GFSync")
	h.tokens = secret.NewDPAPI(h.data)
	client, err := api.New(h.srv.URL, "0.1.0-test")
	if err != nil {
		t.Fatal(err)
	}
	client.Label = "TEST-PC"
	h.cfg = engine.Config{
		DataDir:  h.data,
		Client:   client,
		Tokens:   h.tokens,
		Detect:   func() string { return h.forever },
		Tick:     40 * time.Millisecond,
		Settle:   150 * time.Millisecond,
		AutoPair: true,
		OpenBrowser: func(u string) error {
			h.logMu.Lock()
			h.opened = append(h.opened, u)
			h.logMu.Unlock()
			// Simulate the player logging in and pressing "allow".
			pu, err := url.Parse(u)
			if err != nil {
				return err
			}
			go func() {
				time.Sleep(100 * time.Millisecond)
				h.mock.Approve(pu.Query().Get("code"))
			}()
			return nil
		},
		BackoffMin: 100 * time.Millisecond,
		BackoffMax: 400 * time.Millisecond,
		Logf: func(f string, a ...any) {
			h.logMu.Lock()
			h.logs = append(h.logs, fmt.Sprintf(f, a...))
			h.logMu.Unlock()
		},
	}
	return h
}

func (h *harness) start() {
	h.eng = engine.New(h.cfg)
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	h.done = make(chan struct{})
	go func() { h.eng.Run(ctx); close(h.done) }()
	h.t.Cleanup(h.stop)
}

func (h *harness) stop() {
	if h.cancel != nil {
		h.cancel()
		<-h.done
		h.cancel = nil
	}
}

func (h *harness) svPath(acct string) string {
	return filepath.Join(h.forever, "WTF", "Account", acct, "SavedVariables", "GreenFangToolkit.lua")
}

func (h *harness) writeSV(acct, content string) {
	p := h.svPath(acct)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) waitFor(what string, cond func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	h.logMu.Lock()
	defer h.logMu.Unlock()
	h.t.Fatalf("timed out waiting for %s\nstatus: %+v\nlogs:\n%s", what, h.eng.Status(), strings.Join(h.logs, "\n"))
}

func (h *harness) receivedIDs() map[string]bool {
	out := map[string]bool{}
	for _, raw := range h.mock.Received() {
		var x struct{ ID string }
		json.Unmarshal(raw, &x)
		out[x.ID] = true
	}
	return out
}

func entry(id string, capturedAt int64) string {
	return fmt.Sprintf(`
			{
				["id"] = %q,
				["kind"] = "character.self",
				["capturedAt"] = %d,
				["game"] = { ["world"] = "forever", ["interface"] = 16001, ["realm"] = "虛構伺服器", ["locale"] = "zhTW" },
				["uploader"] = { ["guid"] = "Player-1-1", ["name"] = "虛構", ["realm"] = "虛構伺服器", ["faction"] = "Horde", ["guild"] = %q },
				["payload"] = { ["level"] = 10, ["zone"] = "不該上傳" },
			},`, id, capturedAt, mockapi.Guild)
}

func sv(schema int, enabled bool, ids ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "GreenFangToolkitDB = {\n\t[\"schema\"] = \"gf.toolkit\",\n\t[\"schemaVersion\"] = %d,\n\t[\"sync\"] = {\n\t\t[\"enabled\"] = %v,\n\t\t[\"outbox\"] = {", schema, enabled)
	for i, id := range ids {
		b.WriteString(entry(id, 1763370000+int64(i)))
	}
	b.WriteString("\n\t\t},\n\t},\n}\n")
	return b.String()
}

// snapshot hashes every file under the game folder.
func snapshot(t *testing.T, root string) map[string][32]byte {
	out := map[string][32]byte{}
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := os.ReadFile(p)
			out[p] = sha256.Sum256(b)
		}
		return nil
	})
	return out
}

func TestE2EPairAndUpload(t *testing.T) {
	h := newHarness(t)
	fixture, err := os.ReadFile(filepath.Join("..", "svdoc", "testdata", "GreenFangToolkit.contract.lua"))
	if err != nil {
		t.Fatal(err)
	}
	h.writeSV("ACCOUNT1", string(fixture))
	h.writeSV("ACCOUNT2", sv(2, true, "acct2:1:character.self"))
	before := snapshot(t, h.forever)

	h.start()
	h.waitFor("pairing", func() bool { return h.eng.Status().Paired })
	h.waitFor("3 uploads", func() bool { return len(h.receivedIDs()) == 3 })

	got := h.receivedIDs()
	for _, id := range []string{
		"Player-9999-0000AAAA:1763370000:character.self",
		"Player-9999-0000AAAA:1763370100:roster.snapshot",
		"acct2:1:character.self",
	} {
		if !got[id] || h.mock.Status(id) != "accepted" {
			t.Errorf("%s: status %q", id, h.mock.Status(id))
		}
	}
	for _, raw := range h.mock.Received() {
		if strings.Contains(string(raw), "location.share") {
			t.Error("unknown kind must not be uploaded")
		}
	}
	if l := h.mock.Labels(); len(l) != 1 || l[0] != "TEST-PC" {
		t.Errorf("pairing labels %v", l)
	}
	if len(h.opened) != 1 || !strings.HasPrefix(h.opened[0], h.srv.URL+"/pages/sync-pair?code=") {
		t.Errorf("browser opened %v", h.opened)
	}
	st := h.eng.Status()
	if st.LastSyncAt.IsZero() || st.LastError != "" || st.Pending != 0 || st.GameDir != h.forever || st.Files != 2 {
		t.Errorf("status %+v", st)
	}
	h.stop()

	// state.json records ids; token is encrypted on disk.
	s, err := state.Load(filepath.Join(h.data, state.FileName))
	if err != nil || len(s.Uploaded) != 3 || s.GameDir != h.forever {
		t.Fatalf("state %+v %v", s, err)
	}
	tokRaw, _ := os.ReadFile(filepath.Join(h.data, secret.FileName))
	tok, _ := h.tokens.Load()
	if tok == "" || strings.Contains(string(tokRaw), tok) {
		t.Fatal("token must be stored encrypted")
	}
	for _, line := range h.logs {
		if strings.Contains(line, tok) {
			t.Fatal("token leaked into logs")
		}
	}

	// GF Sync never writes into the game folder.
	after := snapshot(t, h.forever)
	if len(after) != len(before) {
		t.Fatalf("files under _forever_ changed: %d -> %d", len(before), len(after))
	}
	for p, sum := range before {
		if after[p] != sum {
			t.Fatalf("%s was modified", p)
		}
	}

	// Restart: nothing is re-sent.
	calls := h.mock.Calls()
	h.start()
	time.Sleep(600 * time.Millisecond)
	if h.mock.Calls() != calls {
		t.Fatalf("restart re-uploaded (%d -> %d calls)", calls, h.mock.Calls())
	}
	if len(h.opened) != 1 {
		t.Fatal("must not pair again when a token exists")
	}

	// The game writes a new entry: only that one goes up.
	h.writeSV("ACCOUNT2", sv(2, true, "acct2:1:character.self", "acct2:2:character.self"))
	h.waitFor("new entry", func() bool { return h.receivedIDs()["acct2:2:character.self"] })
	if h.mock.Calls() != calls+1 {
		t.Fatalf("expected exactly one more upload, got %d", h.mock.Calls()-calls)
	}
}

func TestE2EDisabledAndUnsupported(t *testing.T) {
	h := newHarness(t)
	h.writeSV("OFF", sv(2, false, "off:1:character.self"))
	h.writeSV("NEW", sv(3, true, "new:1:character.self"))
	h.writeSV("OLD", sv(1, true, "old:1:character.self"))
	h.writeSV("BROKEN", `GreenFangToolkitDB = { schemaVersion = 2, sync = { enabled = os.execute("calc") } }`)
	h.writeSV("OK", sv(2, true, "ok:1:character.self"))
	h.start()
	h.waitFor("ok upload", func() bool { return h.receivedIDs()["ok:1:character.self"] })
	h.waitFor("notices", func() bool { return len(h.eng.Status().Notices) == 3 })
	got := h.receivedIDs()
	if got["off:1:character.self"] || got["new:1:character.self"] || got["old:1:character.self"] {
		t.Fatal("disabled or unsupported files must not upload")
	}
	n := strings.Join(h.eng.Status().Notices, "\n")
	if strings.Count(n, "請更新 GF Sync") != 2 || !strings.Contains(n, "BROKEN") {
		t.Fatalf("notices: %s", n)
	}
	// Player turns sync off in game after entries were queued.
	h.writeSV("OK", sv(2, false, "ok:1:character.self", "ok:2:character.self"))
	time.Sleep(800 * time.Millisecond)
	if h.receivedIDs()["ok:2:character.self"] {
		t.Fatal("upload after /gf sync off")
	}
}

func TestE2ERetryWithBackoff(t *testing.T) {
	h := newHarness(t)
	h.writeSV("A", sv(2, true, "a:1:character.self"))
	h.mock.FailNext(503, 0, 3)
	h.start()
	h.waitFor("upload after retries", func() bool { return h.receivedIDs()["a:1:character.self"] })
	if c := h.mock.Calls(); c != 4 {
		t.Fatalf("calls = %d, want 4 (3 failures + success)", c)
	}
	if h.eng.Status().LastError != "" {
		t.Fatal("error should clear after success")
	}
}

func TestE2ENetworkDown(t *testing.T) {
	h := newHarness(t)
	h.writeSV("A", sv(2, true, "a:1:character.self"))
	h.tokens.Save("pre-existing-token")
	// Point the client at a closed port.
	dead := httptest.NewServer(nil)
	deadURL := dead.URL
	dead.Close()
	c, _ := api.New(deadURL, "t")
	c.HTTP.Timeout = 300 * time.Millisecond
	h.cfg.Client = c
	h.cfg.AutoPair = false
	h.start()
	h.waitFor("network error shown", func() bool {
		s := h.eng.Status()
		return strings.Contains(s.LastError, "無法連線") && !s.RetryAt.IsZero() && s.Pending == 1
	})
}

func TestE2ERateLimit429(t *testing.T) {
	h := newHarness(t)
	h.writeSV("A", sv(2, true, "a:1:character.self"))
	h.mock.FailNext(429, 1, 1)
	h.start()
	h.waitFor("rate limited", func() bool { return h.mock.Calls() == 1 })
	t0 := time.Now()
	h.eng.SyncNow() // must not bypass Retry-After
	h.waitFor("upload after Retry-After", func() bool { return h.receivedIDs()["a:1:character.self"] })
	if time.Since(t0) < 700*time.Millisecond {
		t.Fatalf("Retry-After not honoured (%s)", time.Since(t0))
	}
}

func TestE2EHourlyCap(t *testing.T) {
	h := newHarness(t)
	var ids []string
	for i := 0; i < 120; i++ {
		ids = append(ids, fmt.Sprintf("bulk:%d:character.self", i))
	}
	h.writeSV("A", sv(2, true, ids...))
	h.cfg.HourlyLimit = 2
	h.start()
	h.waitFor("two batches", func() bool { return len(h.receivedIDs()) == 100 })
	time.Sleep(500 * time.Millisecond)
	if h.mock.Calls() != 2 || len(h.receivedIDs()) != 100 {
		t.Fatalf("calls %d received %d", h.mock.Calls(), len(h.receivedIDs()))
	}
	st := h.eng.Status()
	if st.Pending != 20 || !strings.Contains(st.LastError, "上限") {
		t.Fatalf("status %+v", st)
	}
}

func TestE2EBatchSplitOn413(t *testing.T) {
	h := newHarness(t)
	var ids []string
	for i := 0; i < 10; i++ {
		ids = append(ids, fmt.Sprintf("s:%d:character.self", i))
	}
	h.writeSV("A", sv(2, true, ids...))
	h.mock.FailNext(413, 0, 1)
	h.start()
	h.waitFor("all after split", func() bool { return len(h.receivedIDs()) == 10 })
}

func TestE2ERevokedToken(t *testing.T) {
	h := newHarness(t)
	h.writeSV("A", sv(2, true, "a:1:character.self"))
	h.start()
	h.waitFor("first upload", func() bool { return h.receivedIDs()["a:1:character.self"] })
	h.mock.RevokeAll()
	h.writeSV("A", sv(2, true, "a:1:character.self", "a:2:character.self"))
	h.waitFor("unpaired", func() bool {
		s := h.eng.Status()
		return !s.Paired && strings.Contains(s.LastError, "重新配對")
	})
	if tok, _ := h.tokens.Load(); tok != "" {
		t.Fatal("revoked token should be deleted")
	}
	// Pair again from the tray menu; the queued entry goes up.
	h.eng.StartPairing()
	h.waitFor("re-paired upload", func() bool { return h.receivedIDs()["a:2:character.self"] })
}

func TestE2EPause(t *testing.T) {
	h := newHarness(t)
	h.writeSV("A", sv(2, true, "a:1:character.self"))
	h.cfg.AutoPair = false
	h.start()
	h.eng.SetPaused(true)
	h.eng.StartPairing()
	h.waitFor("paired", func() bool { return h.eng.Status().Paired })
	time.Sleep(500 * time.Millisecond)
	if h.mock.Calls() != 0 {
		t.Fatal("uploaded while paused")
	}
	if st := h.eng.Status(); !st.Paused || st.Pending != 1 {
		t.Fatalf("%+v", st)
	}
	h.eng.SetPaused(false)
	h.waitFor("upload after resume", func() bool { return h.receivedIDs()["a:1:character.self"] })
}

func TestE2EPairingDenied(t *testing.T) {
	h := newHarness(t)
	h.cfg.OpenBrowser = func(u string) error {
		pu, _ := url.Parse(u)
		h.mock.Deny(pu.Query().Get("code"))
		return nil
	}
	h.start()
	h.waitFor("denied", func() bool {
		s := h.eng.Status()
		return !s.Paired && s.PairingCode == "" && strings.Contains(s.LastError, "配對沒有完成")
	})
}

func TestE2ENoGameFolderThenPicked(t *testing.T) {
	h := newHarness(t)
	h.cfg.Detect = func() string { return "" }
	h.writeSV("A", sv(2, true, "a:1:character.self"))
	h.start()
	h.waitFor("need folder", func() bool { return h.eng.Status().NeedGameDir })
	if err := h.eng.SetGameDir(t.TempDir()); err == nil {
		t.Fatal("random folder must be refused")
	}
	// The player picks the WoW root; GF Sync finds _forever_ under it.
	if err := h.eng.SetGameDir(filepath.Dir(h.forever)); err != nil {
		t.Fatal(err)
	}
	h.waitFor("upload", func() bool { return h.receivedIDs()["a:1:character.self"] })
}
