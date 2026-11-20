// Package engine is GF Sync without a user interface: it watches the
// SavedVariables files, pairs the device and uploads the outbox. The tray
// app and the end-to-end tests both drive it.
package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"gfsync/internal/redact"
	"gfsync/internal/secret"
	"gfsync/internal/state"
	"gfsync/internal/svdoc"
	"gfsync/internal/update"
	"gfsync/internal/watch"
)

// Config wires the engine. Zero fields get defaults in New.
type Config struct {
	DataDir     string // %APPDATA%\GFSync
	Client      *api.Client
	Tokens      secret.Store
	OpenBrowser func(url string) error
	// Detect finds _forever_ (default gamedir.Detect). "" with an
	// ErrAccessDenied error means a folder exists but cannot be read.
	Detect func() (string, error)
	Now    func() time.Time
	Logf   func(format string, a ...any)
	// LogErr reports whether the log file could not be opened (nil = fine).
	LogErr func() error

	Tick        time.Duration // scan interval (default 2s)
	Settle      time.Duration // quiet period before reading (default 5s)
	HourlyLimit int           // upload requests per hour (default 50, contract max 60)
	BackoffMin  time.Duration // default 15s
	BackoffMax  time.Duration // default 30m
	DetectEvery time.Duration // re-run Detect while no folder is known (default 30s)

	AutoPair bool // start pairing on Run when there is no token

	// CheckUpdate looks for a newer GF Sync (update.Checker.Check). nil
	// turns update checks off (development builds, most tests). It only
	// reads; nothing is downloaded or run (docs/AUTO_UPDATE.md §4.3).
	CheckUpdate func(ctx context.Context) (*update.Result, error)
	UpdateFirst time.Duration // first check after Run (default 2m)
	UpdateEvery time.Duration // then every this long plus up to 1/12 jitter (default 24h)
}

// Update is a newer GF Sync the site and GitHub agree on.
type Update struct {
	Version        string
	Page           string // release page to open; never an installer
	Required       bool   // this version is below the manifest's minGFSync
	PrivacyChanged bool
	Notes          string // short, one line
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
	// NeedGameDir: no game folder was found anywhere; the UI asks the
	// player to pick one. Never set together with GameDirDenied.
	NeedGameDir bool
	// GameDirDenied: the game folder (or its saved files) exists but GF
	// Sync is not allowed to read it, usually because of security
	// software. Picking the folder again would not help.
	GameDirDenied bool
	// Storage lists GF Sync's own files that cannot be opened:
	// StorageLog, StorageState, StorageToken.
	Storage []string
	// TokenUnreadable: token.dat exists but cannot be read, so whether
	// this computer is paired is unknown. The token is kept.
	TokenUnreadable bool
	Files           int
	Paired          bool
	PairingCode     string
	PairingURL      string
	Paused          bool
	LastSyncAt      time.Time
	LastError       string
	Pending         int
	Notices         []string
	RetryAt         time.Time

	// Update is set when a newer GF Sync is available (notify only).
	Update *Update
	// ClientTooOld: the site answered 426 and stopped taking uploads from
	// this version. Nothing is lost; uploads wait. DownloadURL is the page
	// to open, MinVersion what the site asks for ("" if not given).
	ClientTooOld bool
	MinVersion   string
	DownloadURL  string
	// AddonTooOld: the site refused some entries because the AddOn that
	// wrote them is too old (retried later, like unknown_kind).
	AddonTooOld bool
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

	gameDirDenied   bool
	stateUnreadable bool  // state.json exists but cannot be read: never overwrite it
	stateSaveErr    error // last state.json save failed
	tokenUnreadable bool  // token.dat exists but cannot be read: never delete it
	tokenSaveFailed bool  // saving a new token failed
	lastStoreRetry  time.Time

	nextUpdate  time.Time      // next scheduled update check
	updChecking bool           // a check is running
	upd         *update.Result // last successful check
	tooOld      *api.HTTPError // last 426, until an upload succeeds
	addonTooOld bool
}

// Names for Status.Storage.
const (
	StorageLog   = "log"
	StorageState = "state"
	StorageToken = "token"
)

// New builds an engine; call Run to start it.
func New(cfg Config) *Engine {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	// Every log line is masked: account folder names inside paths and
	// character GUIDs (see package redact). Bare account names are masked
	// at the call sites with redact.Account.
	logf := cfg.Logf
	cfg.Logf = func(format string, a ...any) {
		logf("%s", redact.Text(fmt.Sprintf(format, a...)))
	}
	if cfg.LogErr == nil {
		cfg.LogErr = func() error { return nil }
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
	if cfg.UpdateFirst <= 0 {
		cfg.UpdateFirst = 2 * time.Minute
	}
	if cfg.UpdateEvery <= 0 {
		cfg.UpdateEvery = 24 * time.Hour
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
	switch {
	case err == nil:
	case errors.Is(err, state.ErrCorrupt):
		e.cfg.Logf("state: %v", err)
	default:
		// Exists but cannot be read (access denied…). Keep the file as it
		// is: saving now would replace the player's folder and upload
		// history with an empty state.
		e.cfg.Logf("state: cannot read %s, leaving it untouched: %v", state.FileName, err)
		e.stateUnreadable = true
	}
	e.st = st
	e.loadTokenLocked()
	e.scopeLocked()
	if st.GameDir != "" {
		d, err := gamedir.Check(st.GameDir)
		switch {
		case d != "":
			e.gameDir = d
		case gamedir.IsAccessDenied(err):
			// The saved folder is still there; we are just not allowed in.
			e.gameDir = st.GameDir
			e.gameDirDenied = true
			e.cfg.Logf("game folder: access denied: %v", err)
		}
	}
	e.initialized = true
}

// ReasonUnknownKind is the site's answer for a kind it does not know yet.
// Such entries are retried after UnknownKindRetry instead of being
// recorded as answered (upload-api v1.1 §2).
const ReasonUnknownKind = "unknown_kind"

// UnknownKindRetry is how long to wait before sending an unknown_kind
// entry again.
const UnknownKindRetry = 24 * time.Hour

// ReasonAddonTooOld is the site refusing an entry because the AddOn that
// wrote it is too old. It is retried like unknown_kind: after the player
// updates the AddOn the site may take it, and nothing is lost meanwhile.
const ReasonAddonTooOld = "addon_too_old"

// TooOldRetry is how long uploads wait after a 426 before trying again
// (the site may lower its minimum; a new GF Sync restarts anyway).
const TooOldRetry = 6 * time.Hour

// Messages for a version the site no longer takes.
const (
	MsgClientTooOld = "這版 GF Sync 太舊，網站暫時不收資料。請更新 GF Sync（資料會先留在電腦上）"
	MsgAddonTooOld  = "綠皮冒險工具箱的版本太舊，網站暫時不收它的部分資料。請更新工具箱（資料會先留著）"
)

// noticeAddon is the notices key for MsgAddonTooOld (never a file path).
const noticeAddon = "~addon"

// uploadScope identifies the site and paired device that upload answers
// belong to: the API base plus a one-way fingerprint of the device token.
// Pairing again gives a new token, so a new device starts with no answers.
// The fingerprint is salted so it never equals the hash the site stores.
func uploadScope(base, token string) string {
	sum := sha256.Sum256([]byte("gfsync-upload-scope|" + token))
	return base + " device:" + hex.EncodeToString(sum[:8])
}

// scopeLocked points the upload records at the current site and device.
// Without a token nothing changes: the records wait for the next pairing,
// which decides whether they still count. It reports whether old answers
// were dropped.
func (e *Engine) scopeLocked() bool {
	if e.token == "" || e.cfg.Client == nil || e.cfg.Client.Base == nil {
		return false
	}
	if !e.st.SetScope(uploadScope(e.cfg.Client.Base.String(), e.token)) {
		return false
	}
	e.cfg.Logf("state: uploads were answered by another site or device; sending them again (the site answers duplicate for ones it has)")
	return true
}

// loadTokenLocked reads token.dat. A token that cannot be decrypted is
// deleted (pair again); a file that cannot be read is kept.
func (e *Engine) loadTokenLocked() {
	tok, err := e.cfg.Tokens.Load()
	switch {
	case err == nil:
		e.token, e.tokenUnreadable = tok, false
	case errors.Is(err, secret.ErrUnreadable):
		if !e.tokenUnreadable {
			e.cfg.Logf("token: cannot read %s, keeping it: %v", secret.FileName, err)
		}
		e.token, e.tokenUnreadable = "", true
	default:
		e.cfg.Logf("token: cannot decrypt, pairing again: %v", err)
		_ = e.cfg.Tokens.Clear()
		e.token, e.tokenUnreadable = "", false
	}
}

// retryStorageLocked tries again to read state.json / token.dat that could
// not be read at start (for example once security software lets GF Sync in).
func (e *Engine) retryStorageLocked(now time.Time, force bool) {
	if !e.stateUnreadable && !e.tokenUnreadable {
		return
	}
	if !force && now.Sub(e.lastStoreRetry) < e.cfg.DetectEvery {
		return
	}
	e.lastStoreRetry = now
	if e.tokenUnreadable {
		e.loadTokenLocked()
		if !e.tokenUnreadable {
			e.cfg.Logf("token: readable again")
			e.scopeLocked()
		}
	}
	if e.stateUnreadable {
		st, err := state.Load(e.statePath)
		if err != nil && !errors.Is(err, state.ErrCorrupt) {
			return
		}
		// Keep what happened while the file was unreadable, but only
		// answers from the same site and device count.
		if e.st.Scope != "" {
			st.SetScope(e.st.Scope)
		}
		for id, r := range e.st.Uploaded {
			if !st.Has(id) {
				st.Mark(id, r)
			}
		}
		if st.GameDir == "" {
			st.GameDir = e.st.GameDir
		}
		if st.LastSyncAt < e.st.LastSyncAt {
			st.LastSyncAt = e.st.LastSyncAt
		}
		if e.gameDir == "" {
			if d := gamedir.Normalize(st.GameDir); d != "" {
				e.gameDir = d
			}
		}
		e.st = st
		e.stateUnreadable = false
		e.cfg.Logf("state: readable again")
		e.scopeLocked()
		e.saveLocked()
	}
}

// Run blocks until ctx is cancelled.
func (e *Engine) Run(ctx context.Context) {
	e.mu.Lock()
	e.runCtx = ctx
	e.init()
	// Do not pair over a token we merely cannot read right now.
	autoPair := e.cfg.AutoPair && e.token == "" && !e.tokenUnreadable
	e.nextUpdate = e.cfg.Now().Add(e.cfg.UpdateFirst)
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
	f, err := gamedir.Check(dir)
	if f == "" {
		if gamedir.IsAccessDenied(err) {
			e.cfg.Logf("game folder: picked folder: %v", err)
			return errors.New(MsgFolderDenied)
		}
		return fmt.Errorf("這個資料夾裡找不到 %s", gamedir.ForeverDir)
	}
	e.mu.Lock()
	e.gameDir = f
	e.gameDirDenied = false
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
		GameDir:         e.gameDir,
		Ready:           e.initialized,
		PairEnded:       e.pairEnded,
		PairExpired:     e.pairExpired,
		Pairing:         e.pairing != nil,
		NeedGameDir:     e.initialized && e.gameDir == "" && !e.gameDirDenied,
		GameDirDenied:   e.gameDirDenied,
		TokenUnreadable: e.tokenUnreadable,
		Files:           len(e.files),
		Paired:          e.token != "",
		Paused:          e.st.Paused,
		LastError:       e.lastErr,
		Pending:         e.pendingCountLocked(),
		AddonTooOld:     e.addonTooOld,
	}
	if e.tooOld != nil {
		s.ClientTooOld = true
		s.DownloadURL = e.cfg.Client.SafeDownloadURL(e.tooOld)
		if okVersion(e.tooOld.MinVersion) {
			s.MinVersion = e.tooOld.MinVersion
		}
	}
	if u := e.upd; u != nil && u.Newer {
		s.Update = &Update{
			Version:        u.Manifest.Version,
			Page:           update.ReleasePage(u.Manifest.Version),
			Required:       u.Required,
			PrivacyChanged: u.Manifest.PrivacyChanged,
			Notes:          oneLine(u.Manifest.Notes, 80),
		}
	}
	if e.cfg.LogErr() != nil {
		s.Storage = append(s.Storage, StorageLog)
	}
	if e.stateUnreadable || e.stateSaveErr != nil {
		s.Storage = append(s.Storage, StorageState)
	}
	if e.tokenUnreadable || e.tokenSaveFailed {
		s.Storage = append(s.Storage, StorageToken)
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
	if e.stateUnreadable {
		return // never overwrite a state file we could not read
	}
	err := e.st.Save(e.statePath)
	if err != nil && e.stateSaveErr == nil {
		e.cfg.Logf("state: save failed: %v", err)
	}
	if err == nil && e.stateSaveErr != nil {
		e.cfg.Logf("state: saved again")
	}
	e.stateSaveErr = err
}

// pendingLocked lists what is still to upload, member.share last: with a
// large guild there can be about 1,000 of them per file (EVAL-1000), and
// the player's own character, the roster and the rest should not wait
// behind them. Nothing is dropped: whatever does not fit in this hour's
// requests stays pending and goes in a later run.
func (e *Engine) pendingLocked() []svdoc.Entry {
	seen := map[string]bool{}
	var out, shares []svdoc.Entry
	now := e.cfg.Now().Unix()
	for _, f := range e.files {
		for _, en := range e.pending[f] {
			if seen[en.ID] || e.st.Done(en.ID, now) {
				continue
			}
			seen[en.ID] = true
			if en.Kind == svdoc.KindMemberShare {
				shares = append(shares, en)
			} else {
				out = append(out, en)
			}
		}
	}
	return append(out, shares...)
}

func (e *Engine) pendingCountLocked() int { return len(e.pendingLocked()) }

// step: detect folder, scan files, read settled ones, upload one batch.
func (e *Engine) step(ctx context.Context, force bool) {
	defer e.notify()
	now := e.cfg.Now()

	e.mu.Lock()
	e.retryStorageLocked(now, force)
	if e.gameDir == "" && (force || now.Sub(e.lastDetect) >= e.cfg.DetectEvery) {
		e.lastDetect = now
		e.mu.Unlock()
		d, derr := e.cfg.Detect()
		e.mu.Lock()
		if d != "" && e.gameDir == "" {
			e.gameDir = d
			e.st.GameDir = d
			e.saveLocked()
			e.cfg.Logf("game folder detected: %s", d)
		}
		if e.gameDir == "" {
			e.setDeniedLocked(gamedir.IsAccessDenied(derr), derr)
		}
	}
	dir := e.gameDir
	e.mu.Unlock()

	var files []string
	var scanErr error
	if dir != "" {
		files, scanErr = gamedir.ScanSavedVariables(dir)
	}
	e.mu.Lock()
	if dir != "" && dir == e.gameDir {
		e.setDeniedLocked(scanErr != nil, scanErr)
	}
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
	e.maybeCheckUpdate(ctx)
	e.flush(ctx)
}

// setDeniedLocked records whether the game folder is blocked, logging
// only when that changes.
func (e *Engine) setDeniedLocked(denied bool, err error) {
	if denied && !e.gameDirDenied {
		e.cfg.Logf("game folder: access denied (not \"not found\"): %v", err)
	}
	if !denied && e.gameDirDenied {
		e.cfg.Logf("game folder: readable again")
	}
	e.gameDirDenied = denied
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
		e.cfg.Logf("read %s: %v", redact.Account(acct), err)
		if gamedir.IsAccessDenied(err) {
			e.notices[path] = fmt.Sprintf("帳號 %s 的存檔沒有讀取權限，可能是防毒軟體擋住了 GF Sync", acct)
		} else {
			e.notices[path] = fmt.Sprintf("帳號 %s 的存檔讀取失敗，下次存檔時會再試一次", acct)
		}
		e.pending[path] = nil
		return
	}
	delete(e.notices, path)
	switch doc.Status {
	case svdoc.StatusOK:
		e.pending[path] = doc.Entries
		e.cfg.Logf("read %s: %d entries (%d skipped)", redact.Account(acct), len(doc.Entries), doc.Skipped)
		if doc.OtherGuild > 0 {
			// Count only: never the guild or character names.
			e.cfg.Logf("read %s: skipped %d entries (not our guild)", redact.Account(acct), doc.OtherGuild)
		}
		if doc.OverLimit > 0 {
			e.cfg.Logf("read %s: skipped %d older member.share entries (more than %d in one file)", redact.Account(acct), doc.OverLimit, svdoc.MaxMemberShare)
		}
	case svdoc.StatusDisabled:
		e.pending[path] = nil
		e.cfg.Logf("read %s: not uploaded, sync.enabled is not true (an older AddOn had sync turned off)", redact.Account(acct))
	case svdoc.StatusUnsupported:
		e.pending[path] = nil
		e.notices[path] = fmt.Sprintf("帳號 %s 的插件資料格式（版本 %v）這版 GF Sync 看不懂，請更新 GF Sync", acct, doc.SchemaVersion)
		e.cfg.Logf("read %s: unsupported schemaVersion %v", redact.Account(acct), doc.SchemaVersion)
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
				e.cfg.Logf("upload: %s is larger than 1MB, skipped", redact.EntryID(en.ID))
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
		answered, accepted, unknownKind, addonOld := 0, 0, 0, 0
		e.tooOld = nil
		for _, r := range results {
			if !inBatch[r.ID] {
				continue
			}
			switch r.Status {
			case "accepted", "duplicate", "rejected":
			default:
				continue
			}
			if r.Status == "rejected" && r.Reason == ReasonAddonTooOld {
				// Same handling as unknown_kind: not an answer, retried.
				e.st.Mark(r.ID, state.Record{At: now.Unix(), Status: r.Status, Reason: r.Reason,
					RetryAt: now.Add(UnknownKindRetry).Unix()})
				addonOld++
				continue
			}
			if r.Status == "rejected" && r.Reason == ReasonUnknownKind {
				// The site does not know this kind yet (it is updated
				// before GF Sync and the AddOn, but just in case): not an
				// answer. Try again in 24 hours (upload-api v1.1 §2).
				e.st.Mark(r.ID, state.Record{At: now.Unix(), Status: r.Status, Reason: r.Reason,
					RetryAt: now.Add(UnknownKindRetry).Unix()})
				unknownKind++
				continue
			}
			e.st.Mark(r.ID, state.Record{At: now.Unix(), Status: r.Status, Reason: r.Reason})
			answered++
			if r.Status == "accepted" {
				accepted++
			}
			if r.Status == "rejected" {
				e.cfg.Logf("upload: %s rejected: %s", redact.EntryID(r.ID), r.Reason)
			}
		}
		e.cfg.Logf("upload: %d sent, %d answered, %d accepted", len(batch), answered, accepted)
		if unknownKind > 0 {
			e.cfg.Logf("upload: %d entries of a kind the site does not know yet; retry in %s", unknownKind, UnknownKindRetry)
		}
		switch {
		case addonOld > 0:
			e.cfg.Logf("upload: %d entries from an AddOn the site finds too old; retry in %s", addonOld, UnknownKindRetry)
			e.addonTooOld = true
			e.notices[noticeAddon] = MsgAddonTooOld
		case accepted > 0:
			e.addonTooOld = false
			delete(e.notices, noticeAddon)
		}
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
	case isTooOld(err):
		// The site no longer takes this version. Keep everything, stop
		// sending (no backoff churn, no hourly budget) and look for an
		// update now.
		he, _ := api.ClientTooOld(err)
		e.tooOld = he
		e.rateUntil = now.Add(TooOldRetry)
		e.lastErr = MsgClientTooOld
		e.nextUpdate = now
		e.cfg.Logf("upload: site refused this GF Sync version (HTTP 426, minimum %q); uploads wait %s", he.MinVersion, TooOldRetry)
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
			e.cfg.Logf("upload: %s refused with HTTP %d, skipped", redact.EntryID(batch[0].ID), he.Status)
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

// MsgFolderDenied is returned by SetGameDir when the picked folder exists
// but cannot be read.
const MsgFolderDenied = "GF Sync 沒有權限讀取這個資料夾，可能是防毒軟體擋住了。資料夾本身沒有問題，不需要重新選擇。"

// MsgTokenSaveFailed: pairing was approved but token.dat could not be written.
const MsgTokenSaveFailed = "無法保存配對資料，可能是防毒軟體擋住了 GF Sync 寫入自己的資料"

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
		if he, ok := api.ClientTooOld(err); ok {
			e.cfg.Logf("pair: site refused this GF Sync version (HTTP 426, minimum %q)", he.MinVersion)
			e.mu.Lock()
			e.tooOld = he
			e.nextUpdate = e.cfg.Now()
			e.mu.Unlock()
			e.endPairing(MsgClientTooOld)
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
			e.mu.Lock()
			e.tokenSaveFailed = true
			e.mu.Unlock()
			e.endPairing(MsgTokenSaveFailed)
			return
		}
		e.mu.Lock()
		e.token = r.Token
		e.tokenSaveFailed = false
		e.tokenUnreadable = false
		e.lastErr = ""
		if e.scopeLocked() {
			e.saveLocked()
		}
		e.mu.Unlock()
		e.cfg.Logf("pair: device paired (%v)", stats)
		e.endPairing("")
		e.poke(true)
		return
	}
}
