// Package engine is GF Sync without a user interface: it watches the
// SavedVariables files, pairs the device and uploads the outbox. The tray
// app and the end-to-end tests both drive it.
package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"time"

	"gfsync/internal/api"
	"gfsync/internal/gamedir"
	"gfsync/internal/luasv"
	"gfsync/internal/secret"
	"gfsync/internal/state"
	"gfsync/internal/svdoc"
	"gfsync/internal/watch"
)

// Config wires the engine. Zero fields get defaults in New.
type Config struct {
	DataDir     string // %APPDATA%\GFSync
	Client      *api.Client
	Tokens      secret.Store
	OpenBrowser func(url string) error
	Detect      func() string // finds _forever_; default gamedir.Detect
	Now         func() time.Time
	Logf        func(format string, a ...any)

	Tick        time.Duration // scan interval (default 2s)
	Settle      time.Duration // quiet period before reading (default 5s)
	HourlyLimit int           // upload requests per hour (default 50, contract max 60)
	BackoffMin  time.Duration // default 15s
	BackoffMax  time.Duration // default 30m
	DetectEvery time.Duration // re-run Detect while no folder is known (default 30s)

	AutoPair bool // start pairing on Run when there is no token
}

// Status is a snapshot for the UI.
type Status struct {
	// PairEnded: the last pairing attempt ended without success (code
	// expired, denied or could not start). LastError says which; the UI
	// offers "pair again".
	PairEnded   bool
	PairExpired bool // PairEnded because the code expired
	Pairing     bool // a pairing attempt is running (maybe still waiting for a code)
	Ready       bool // state and token have been loaded (Run has started)
	GameDir     string
	NeedGameDir bool
	Files       int
	Paired      bool
	PairingCode string
	PairingURL  string
	Paused      bool
	LastSyncAt  time.Time
	LastError   string
	Pending     int
	Notices     []string
	RetryAt     time.Time
}

type pairInfo struct {
	code, url string
	cancel    context.CancelFunc
}

// Engine is safe for concurrent use by the UI.
type Engine struct {
	cfg Config

	mu          sync.Mutex
	st          *state.State
	token       string
	gameDir     string
	files       []string
	w           *watch.Watcher
	pending     map[string][]svdoc.Entry // by file
	notices     map[string]string        // by file
	lastErr     string
	backoff     time.Duration
	retryAt     time.Time // after a network/server error
	rateUntil   time.Time // after 429 or our own hourly cap
	batchMax    int
	pairing     *pairInfo
	lastDetect  time.Time
	runCtx      context.Context
	kick        chan bool // true = force
	onChange    func(Status)
	lastStatus  Status
	statePath   string
	initialized bool
	pairEnded   bool
	pairExpired bool
}

// New builds an engine; call Run to start it.
func New(cfg Config) *Engine {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	if cfg.Detect == nil {
		cfg.Detect = gamedir.Detect
	}
	if cfg.OpenBrowser == nil {
		cfg.OpenBrowser = func(string) error { return nil }
	}
	if cfg.Tick <= 0 {
		cfg.Tick = 2 * time.Second
	}
	if cfg.Settle <= 0 {
		cfg.Settle = watch.Settle
	}
	if cfg.HourlyLimit <= 0 || cfg.HourlyLimit > api.HourlyLimit {
		cfg.HourlyLimit = 50
	}
	if cfg.BackoffMin <= 0 {
		cfg.BackoffMin = 15 * time.Second
	}
	if cfg.BackoffMax <= 0 {
		cfg.BackoffMax = 30 * time.Minute
	}
	if cfg.DetectEvery <= 0 {
		cfg.DetectEvery = 30 * time.Second
	}
	w := watch.New()
	w.Settle, w.Now = cfg.Settle, cfg.Now
	return &Engine{
		cfg:       cfg,
		w:         w,
		pending:   map[string][]svdoc.Entry{},
		notices:   map[string]string{},
		batchMax:  api.MaxItems,
		kick:      make(chan bool, 1),
		statePath: filepath.Join(cfg.DataDir, state.FileName),
		st:        state.New(),
	}
}

// OnChange registers a callback for status changes (called from the
// engine goroutine, never while holding the lock).
func (e *Engine) OnChange(f func(Status)) {
	e.mu.Lock()
	e.onChange = f
	e.mu.Unlock()
}

func (e *Engine) init() {
	st, err := state.Load(e.statePath)
	if err != nil {
		e.cfg.Logf("state: %v", err)
	}
	e.st = st
	tok, err := e.cfg.Tokens.Load()
	if err != nil {
		e.cfg.Logf("token: cannot decrypt, pairing again: %v", err)
		_ = e.cfg.Tokens.Clear()
		tok = ""
	}
	e.token = tok
	if d := gamedir.Normalize(st.GameDir); d != "" {
		e.gameDir = d
	}
	e.initialized = true
}

// Run blocks until ctx is cancelled.
func (e *Engine) Run(ctx context.Context) {
	e.mu.Lock()
	e.runCtx = ctx
	e.init()
	autoPair := e.cfg.AutoPair && e.token == ""
	e.mu.Unlock()
	if autoPair {
		e.StartPairing()
	}
	t := time.NewTicker(e.cfg.Tick)
	defer t.Stop()
	e.step(ctx, false)
	for {
		select {
		case <-ctx.Done():
			e.mu.Lock()
			if e.pairing != nil {
				e.pairing.cancel()
			}
			e.mu.Unlock()
			return
		case <-t.C:
			e.step(ctx, false)
		case force := <-e.kick:
			e.step(ctx, force)
		}
	}
}

func (e *Engine) poke(force bool) {
	select {
	case e.kick <- force:
	default:
		if force {
			// replace a pending non-forced kick
			select {
			case <-e.kick:
			default:
			}
			select {
			case e.kick <- true:
			default:
			}
		}
	}
}

// SyncNow re-reads every file and uploads at once (still honouring the
// server's rate limit).
func (e *Engine) SyncNow() { e.poke(true) }

// SetPaused stops or resumes uploading. Watching continues.
func (e *Engine) SetPaused(p bool) {
	e.mu.Lock()
	e.st.Paused = p
	e.saveLocked()
	e.mu.Unlock()
	e.poke(false)
}

// SetGameDir sets the folder the player picked.
func (e *Engine) SetGameDir(dir string) error {
	f := gamedir.Normalize(dir)
	if f == "" {
		return fmt.Errorf("這個資料夾裡找不到 %s", gamedir.ForeverDir)
	}
	e.mu.Lock()
	e.gameDir = f
	e.st.GameDir = f
	e.saveLocked()
	e.mu.Unlock()
	e.poke(true)
	return nil
}

// Unpair forgets the local token.
func (e *Engine) Unpair() {
	e.mu.Lock()
	e.token = ""
	_ = e.cfg.Tokens.Clear()
	e.mu.Unlock()
	e.poke(false)
}

// Status returns the current snapshot.
func (e *Engine) Status() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.statusLocked()
}

func (e *Engine) statusLocked() Status {
	s := Status{
		GameDir:     e.gameDir,
		Ready:       e.initialized,
		PairEnded:   e.pairEnded,
		PairExpired: e.pairExpired,
		Pairing:     e.pairing != nil,
		NeedGameDir: e.initialized && e.gameDir == "",
		Files:       len(e.files),
		Paired:      e.token != "",
		Paused:      e.st.Paused,
		LastError:   e.lastErr,
		Pending:     e.pendingCountLocked(),
	}
	if e.st.LastSyncAt > 0 {
		s.LastSyncAt = time.Unix(e.st.LastSyncAt, 0)
	}
	if e.pairing != nil {
		s.PairingCode, s.PairingURL = e.pairing.code, e.pairing.url
	}
	if e.retryAt.After(e.rateUntil) {
		s.RetryAt = e.retryAt
	} else {
		s.RetryAt = e.rateUntil
	}
	keys := make([]string, 0, len(e.notices))
	for k := range e.notices {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		s.Notices = append(s.Notices, e.notices[k])
	}
	return s
}

func (e *Engine) notify() {
	e.mu.Lock()
	s := e.statusLocked()
	f := e.onChange
	changed := !reflect.DeepEqual(s, e.lastStatus)
	e.lastStatus = s
	e.mu.Unlock()
	if f != nil && changed {
		f(s)
	}
}

func (e *Engine) saveLocked() {
	if err := e.st.Save(e.statePath); err != nil {
		e.cfg.Logf("state: save failed: %v", err)
	}
}

func (e *Engine) pendingLocked() []svdoc.Entry {
	seen := map[string]bool{}
	var out []svdoc.Entry
	for _, f := range e.files {
		for _, en := range e.pending[f] {
			if seen[en.ID] || e.st.Done(en.ID) {
				continue
			}
			seen[en.ID] = true
			out = append(out, en)
		}
	}
	return out
}

func (e *Engine) pendingCountLocked() int { return len(e.pendingLocked()) }

// step: detect folder, scan files, read settled ones, upload one batch.
func (e *Engine) step(ctx context.Context, force bool) {
	defer e.notify()
	now := e.cfg.Now()

	e.mu.Lock()
	if e.gameDir == "" && (force || now.Sub(e.lastDetect) >= e.cfg.DetectEvery) {
		e.lastDetect = now
		e.mu.Unlock()
		d := e.cfg.Detect()
		e.mu.Lock()
		if d != "" && e.gameDir == "" {
			e.gameDir = d
			e.st.GameDir = d
			e.saveLocked()
			e.cfg.Logf("game folder detected: %s", d)
		}
	}
	dir := e.gameDir
	e.mu.Unlock()

	var files []string
	if dir != "" {
		files = gamedir.SavedVariablesFiles(dir)
	}
	e.mu.Lock()
	e.files = files
	for f := range e.pending {
		if !contains(files, f) {
			delete(e.pending, f)
			delete(e.notices, f)
		}
	}
	if force {
		e.w.ForceAll()
		e.retryAt = time.Time{}
		e.backoff = 0
	}
	ready := e.w.Scan(files)
	e.mu.Unlock()

	for _, f := range ready {
		e.readFile(f)
	}
	e.flush(ctx)
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func (e *Engine) readFile(path string) {
	acct := gamedir.AccountOf(path)
	var doc *svdoc.Doc
	st, err := os.Stat(path)
	if err == nil && st.Size() > int64(luasv.DefaultLimits.MaxBytes) {
		err = fmt.Errorf("file too large (%d bytes)", st.Size())
	}
	if err == nil {
		var b []byte
		// Read-only open; GF Sync never writes under WTF.
		b, err = os.ReadFile(path)
		if err == nil {
			doc, err = svdoc.Parse(b)
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err != nil {
		e.cfg.Logf("read %s: %v", acct, err)
		e.notices[path] = fmt.Sprintf("帳號 %s 的存檔讀取失敗，下次存檔時會再試一次", acct)
		e.pending[path] = nil
		return
	}
	delete(e.notices, path)
	switch doc.Status {
	case svdoc.StatusOK:
		e.pending[path] = doc.Entries
		e.cfg.Logf("read %s: %d entries (%d skipped)", acct, len(doc.Entries), doc.Skipped)
	case svdoc.StatusDisabled:
		e.pending[path] = nil
		e.cfg.Logf("read %s: sync disabled in game (/gf sync off)", acct)
	case svdoc.StatusUnsupported:
		e.pending[path] = nil
		e.notices[path] = fmt.Sprintf("帳號 %s 的插件資料格式（版本 %v）這版 GF Sync 看不懂，請更新 GF Sync", acct, doc.SchemaVersion)
		e.cfg.Logf("read %s: unsupported schemaVersion %v", acct, doc.SchemaVersion)
	default:
		e.pending[path] = nil
	}
}

// flush sends at most one batch.
func (e *Engine) flush(ctx context.Context) {
	e.mu.Lock()
	now := e.cfg.Now()
	if e.token == "" || e.st.Paused || now.Before(e.retryAt) || now.Before(e.rateUntil) {
		e.mu.Unlock()
		return
	}
	pend := e.pendingLocked()
	if len(pend) == 0 {
		e.mu.Unlock()
		return
	}
	// Our own hourly cap, persisted across restarts.
	cut := now.Add(-time.Hour).Unix()
	times := e.st.UploadTimes[:0]
	for _, t := range e.st.UploadTimes {
		if t > cut {
			times = append(times, t)
		}
	}
	e.st.UploadTimes = times
	if len(times) >= e.cfg.HourlyLimit {
		e.rateUntil = time.Unix(times[0], 0).Add(time.Hour)
		e.lastErr = "已達每小時上傳次數上限，稍後自動繼續"
		e.mu.Unlock()
		return
	}
	var batch []svdoc.Entry
	var items []json.RawMessage
	for _, en := range pend {
		if len(batch) >= e.batchMax {
			break
		}
		trial := append(append([]json.RawMessage(nil), items...), en.JSON)
		if api.BodySize(e.cfg.Client.NewUploadRequest(trial)) > api.MaxBodyBytes {
			if len(batch) == 0 {
				e.st.Mark(en.ID, state.Record{At: now.Unix(), Status: "rejected", Reason: "too_large_local"})
				e.cfg.Logf("upload: %s is larger than 1MB, skipped", en.ID)
				continue
			}
			break
		}
		batch, items = append(batch, en), trial
	}
	if len(batch) == 0 {
		e.saveLocked()
		e.mu.Unlock()
		return
	}
	e.st.UploadTimes = append(e.st.UploadTimes, now.Unix())
	e.saveLocked()
	token := e.token
	e.mu.Unlock()

	results, err := e.cfg.Client.Upload(ctx, token, items)

	e.mu.Lock()
	defer e.mu.Unlock()
	now = e.cfg.Now()
	var he *api.HTTPError
	switch {
	case err == nil:
		inBatch := map[string]bool{}
		for _, b := range batch {
			inBatch[b.ID] = true
		}
		answered, accepted := 0, 0
		for _, r := range results {
			if !inBatch[r.ID] {
				continue
			}
			switch r.Status {
			case "accepted", "duplicate", "rejected":
			default:
				continue
			}
			e.st.Mark(r.ID, state.Record{At: now.Unix(), Status: r.Status, Reason: r.Reason})
			answered++
			if r.Status == "accepted" {
				accepted++
			}
			if r.Status == "rejected" {
				e.cfg.Logf("upload: %s rejected: %s", r.ID, r.Reason)
			}
		}
		e.cfg.Logf("upload: %d sent, %d answered, %d accepted", len(batch), answered, accepted)
		e.st.LastSyncAt = now.Unix()
		e.lastErr = ""
		e.backoff = 0
		e.retryAt = time.Time{}
		if answered < len(batch) {
			// Server skipped some ids; try them again later, gently.
			e.retryAt = now.Add(e.cfg.BackoffMin)
		}
		if e.batchMax < api.MaxItems {
			e.batchMax *= 2
			if e.batchMax > api.MaxItems {
				e.batchMax = api.MaxItems
			}
		}
		e.saveLocked()
		if len(e.pendingLocked()) > 0 {
			e.poke(false)
		}
	case errors.Is(err, api.ErrUnauthorized):
		e.cfg.Logf("upload: token rejected, device must pair again")
		e.token = ""
		_ = e.cfg.Tokens.Clear()
		e.lastErr = "這台電腦的配對已失效，請重新配對"
	case errors.As(err, &he) && he.Status == 429:
		wait := he.RetryAfter
		if wait <= 0 {
			wait = 10 * time.Minute
		}
		e.rateUntil = now.Add(wait)
		e.lastErr = "網站暫時限制上傳次數，稍後自動重試"
		e.cfg.Logf("upload: rate limited for %s", wait)
	case errors.As(err, &he) && (he.Status == 413 || he.Status == 400):
		if len(batch) > 1 {
			e.batchMax = max(1, len(batch)/2)
			e.cfg.Logf("upload: HTTP %d, batch size now %d", he.Status, e.batchMax)
			e.poke(false)
		} else {
			e.st.Mark(batch[0].ID, state.Record{At: now.Unix(), Status: "rejected", Reason: fmt.Sprintf("http_%d", he.Status)})
			e.cfg.Logf("upload: %s refused with HTTP %d, skipped", batch[0].ID, he.Status)
			e.saveLocked()
		}
	default:
		if e.backoff == 0 {
			e.backoff = e.cfg.BackoffMin
		} else {
			e.backoff *= 2
		}
		if e.backoff > e.cfg.BackoffMax {
			e.backoff = e.cfg.BackoffMax
		}
		jitter := time.Duration(rand.Int63n(int64(e.backoff)/5 + 1))
		e.retryAt = now.Add(e.backoff + jitter)
		e.lastErr = "無法連線到公會網站，稍後自動重試"
		e.cfg.Logf("upload: %v; retry in %s", err, e.backoff+jitter)
	}
}

// StartPairing begins (or re-opens) the device pairing flow (contract §1).
func (e *Engine) StartPairing() {
	e.mu.Lock()
	if e.pairing != nil {
		url := e.pairing.url
		e.mu.Unlock()
		if url != "" {
			_ = e.cfg.OpenBrowser(url)
		}
		return
	}
	parent := e.runCtx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	e.pairing = &pairInfo{cancel: cancel}
	e.pairEnded, e.pairExpired = false, false
	e.mu.Unlock()
	e.cfg.Logf("pair: starting")
	e.notify()
	go e.pair(ctx)
}

// pairRetryMax caps the wait between attempts to start pairing.
const pairRetryMax = 5 * time.Minute

// retryableStart reports whether a failed device/start is worth retrying:
// network errors, 429 and 5xx. Other HTTP errors (4xx) will not fix themselves.
func retryableStart(err error) bool {
	var he *api.HTTPError
	if errors.As(err, &he) {
		return he.Status == 429 || he.Status >= 500
	}
	return true
}

func (e *Engine) endPairing(msg string) { e.endPairingAs(msg, false) }

// endPairingAs ends the pairing attempt. A non-empty msg means it failed.
func (e *Engine) endPairingAs(msg string, expired bool) {
	e.mu.Lock()
	if e.pairing != nil {
		e.pairing.cancel()
	}
	e.pairing = nil
	if msg != "" {
		e.lastErr = msg
		e.pairEnded = e.token == ""
		e.pairExpired = expired && e.pairEnded
	}
	e.mu.Unlock()
	e.notify()
}

// MsgPairExpired is shown in the tray tooltip and menu after the code expired.
const MsgPairExpired = "配對碼已過期，點這裡重新配對"

// pollStats counts poll outcomes for the log (never codes or tokens).
type pollStats struct{ pending, slowDown, errors int }

func (p pollStats) String() string {
	return fmt.Sprintf("%d pending, %d slow_down, %d errors", p.pending, p.slowDown, p.errors)
}

func (e *Engine) pair(ctx context.Context) {
	c := e.cfg.Client
	// Starting can fail right after boot (autostart runs before the network
	// is up) or while the site is briefly down. Keep retrying those cases
	// quietly so a new player never has to find the tray menu.
	wait := e.cfg.BackoffMin
	var ds *api.DeviceStart
	for {
		var err error
		ds, err = c.StartDevice(ctx)
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			return
		}
		if !retryableStart(err) {
			e.cfg.Logf("pair: start failed: %v", err)
			e.endPairing("無法開始配對，請稍後在選單按「配對這台電腦」")
			return
		}
		e.cfg.Logf("pair: start failed: %v; retry in %s", err, wait)
		e.mu.Lock()
		e.lastErr = "無法連線到公會網站，連上網路後會自動開始配對"
		e.mu.Unlock()
		e.notify()
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		if wait *= 2; wait > pairRetryMax {
			wait = pairRetryMax
		}
	}
	e.mu.Lock()
	e.lastErr = ""
	e.mu.Unlock()
	// Log timings only: the user code and device code stay out of the log.
	e.cfg.Logf("pair: start ok, code expires in %ds, poll every %ds", ds.ExpiresIn, ds.Interval)
	url := c.SafeVerifyURL(ds)
	e.mu.Lock()
	if e.pairing != nil {
		e.pairing.code, e.pairing.url = ds.UserCode, url
	}
	e.mu.Unlock()
	e.notify()
	if err := e.cfg.OpenBrowser(url); err != nil {
		e.cfg.Logf("pair: open browser failed: %v", err)
	} else {
		e.cfg.Logf("pair: browser opened")
	}
	deadline := time.Now().Add(time.Duration(ds.ExpiresIn) * time.Second)
	interval := time.Duration(ds.Interval) * time.Second
	var stats pollStats
	last := ""
	class := func(c string) { // log each change of poll outcome, not every poll
		if c != last {
			e.cfg.Logf("pair: poll: %s", c)
			last = c
		}
	}
	for {
		select {
		case <-ctx.Done():
			e.cfg.Logf("pair: cancelled (%v)", stats)
			return
		case <-time.After(interval):
		}
		if time.Now().After(deadline) {
			e.cfg.Logf("pair: code expired (%v)", stats)
			e.endPairingAs(MsgPairExpired, true)
			return
		}
		r, err := c.PollDevice(ctx, ds.DeviceCode)
		switch {
		case errors.Is(err, api.ErrPairingDenied):
			e.cfg.Logf("pair: denied on the website (%v)", stats)
			e.endPairing("配對被拒絕了。要同步這台電腦，請從選單重新配對")
			return
		case errors.Is(err, api.ErrPairingEnded):
			// 410 and friends: expired or already used.
			e.cfg.Logf("pair: ended by the website: %v (%v)", err, stats)
			e.endPairingAs(MsgPairExpired, true)
			return
		case err != nil:
			if ctx.Err() != nil {
				e.cfg.Logf("pair: cancelled (%v)", stats)
				return
			}
			stats.errors++
			class("error")
			e.cfg.Logf("pair: poll error: %v", err)
			continue
		case r.SlowDown:
			stats.slowDown++
			class("slow_down")
			interval += 5 * time.Second
			continue
		case r.Pending:
			stats.pending++
			class("waiting for approval")
			if s := time.Duration(r.Interval) * time.Second; s > interval && s <= time.Minute {
				interval = s
			}
			continue
		}
		if err := e.cfg.Tokens.Save(r.Token); err != nil {
			e.cfg.Logf("pair: save token: %v", err)
			e.endPairing("無法保存配對資料")
			return
		}
		e.mu.Lock()
		e.token = r.Token
		e.lastErr = ""
		e.mu.Unlock()
		e.cfg.Logf("pair: device paired (%v)", stats)
		e.endPairing("")
		e.poke(true)
		return
	}
}
