// Package state persists GF Sync's local state in %APPDATA%\GFSync.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// FileName is the state file inside the data directory.
const FileName = "state.json"

// Keep at most this many uploaded ids; the server deduplicates too, so
// forgetting very old ids is harmless (contract upload-api §2).
const maxRecords = 20000

// Record says what happened to one outbox id.
type Record struct {
	At     int64  `json:"at"`
	Status string `json:"status"` // accepted | duplicate | rejected
	Reason string `json:"reason,omitempty"`
}

// State is the on-disk document. Unknown fields from newer versions are
// dropped on save, which is fine for a local cache.
type State struct {
	Version     int     `json:"version"`
	GameDir     string  `json:"gameDir,omitempty"`
	Paused      bool    `json:"paused,omitempty"`
	LastSyncAt  int64   `json:"lastSyncAt,omitempty"`
	UploadTimes []int64 `json:"uploadTimes,omitempty"` // request times, for the hourly limit
	// Scope names the site and paired device that Uploaded belongs to
	// (see SetScope). Files from before scopes existed have none.
	Scope    string            `json:"scope,omitempty"`
	Uploaded map[string]Record `json:"uploaded"`
}

// ErrCorrupt marks a state file that was unreadable as JSON and has been
// reset. Any other error from Load means the file exists but could not be
// read (for example access denied); callers must not overwrite it then.
var ErrCorrupt = errors.New("state file was corrupted")

// New returns an empty state.
func New() *State { return &State{Version: 1, Uploaded: map[string]Record{}} }

// Load reads path. A missing file yields an empty state. A corrupted file
// is renamed to state.json.bad-<time> and an empty state is returned along
// with an error wrapping ErrCorrupt. Any other error means the file could
// not be read at all.
func Load(path string) (*State, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return New(), nil
	}
	if err != nil {
		return New(), err
	}
	s := New()
	if err := json.Unmarshal(b, s); err != nil || s.Version < 1 {
		bad := fmt.Sprintf("%s.bad-%d", path, time.Now().Unix())
		_ = os.Rename(path, bad)
		return New(), fmt.Errorf("%w and has been reset (kept as %s)", ErrCorrupt, filepath.Base(bad))
	}
	if s.Uploaded == nil {
		s.Uploaded = map[string]Record{}
	}
	return s, nil
}

// Save writes atomically (temp file + rename).
func (s *State) Save(path string) error {
	s.prune()
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// SetScope makes Uploaded belong to scope. When the scope changes (another
// API base, or pairing again as a new device), the old answers no longer
// count and are forgotten, so every entry is sent again; the site answers
// "duplicate" for ones it already has. It reports whether records were
// dropped.
func (s *State) SetScope(scope string) bool {
	if s.Scope == scope {
		return false
	}
	dropped := len(s.Uploaded) > 0
	s.Scope = scope
	s.Uploaded = map[string]Record{}
	return dropped
}

// Done reports whether id was already sent and answered by the server.
func (s *State) Done(id string) bool { _, ok := s.Uploaded[id]; return ok }

// Mark records the server's answer for id.
func (s *State) Mark(id string, r Record) { s.Uploaded[id] = r }

func (s *State) prune() {
	if len(s.Uploaded) <= maxRecords {
		return
	}
	type kv struct {
		id string
		at int64
	}
	all := make([]kv, 0, len(s.Uploaded))
	for id, r := range s.Uploaded {
		all = append(all, kv{id, r.At})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].at > all[j].at })
	for _, x := range all[maxRecords:] {
		delete(s.Uploaded, x.id)
	}
}
