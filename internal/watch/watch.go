// Package watch decides when a SavedVariables file is safe to read: its
// modification time and size must be unchanged for a settle period
// (contract savedvariables §3.3: 5 seconds). It only calls os.Stat.
package watch

import (
	"os"
	"time"
)

// Settle is the contract's quiet period.
const Settle = 5 * time.Second

type fileInfo struct {
	mod       time.Time
	size      int64
	changedAt time.Time
	pending   bool
}

// Watcher tracks files between Scan calls. Not safe for concurrent use.
type Watcher struct {
	Settle time.Duration
	Now    func() time.Time
	files  map[string]*fileInfo
}

// New returns a watcher with the contract's settle period.
func New() *Watcher {
	return &Watcher{Settle: Settle, Now: time.Now, files: map[string]*fileInfo{}}
}

// Scan stats paths and returns the ones that changed and have now been
// stable for Settle. A file seen for the first time counts as changed.
// Paths no longer present are forgotten.
func (w *Watcher) Scan(paths []string) []string {
	now := w.Now()
	seen := map[string]bool{}
	var ready []string
	for _, p := range paths {
		seen[p] = true
		st, err := os.Stat(p)
		if err != nil || st.IsDir() {
			delete(w.files, p)
			continue
		}
		f := w.files[p]
		if f == nil {
			f = &fileInfo{mod: st.ModTime(), size: st.Size(), changedAt: now, pending: true}
			w.files[p] = f
		} else if !st.ModTime().Equal(f.mod) || st.Size() != f.size {
			f.mod, f.size, f.changedAt, f.pending = st.ModTime(), st.Size(), now, true
		}
		if f.pending && now.Sub(f.changedAt) >= w.Settle {
			f.pending = false
			ready = append(ready, p)
		}
	}
	for p := range w.files {
		if !seen[p] {
			delete(w.files, p)
		}
	}
	return ready
}

// Touch marks a file as changed now (e.g. after a read failed, or for
// "sync now"), so it is re-read after the settle period.
func (w *Watcher) Touch(path string) {
	if f := w.files[path]; f != nil {
		f.pending = true
		f.changedAt = w.Now()
	}
}

// ForceAll marks every known file ready on the next Scan, unless it is
// still changing.
func (w *Watcher) ForceAll() {
	for _, f := range w.files {
		if !f.pending {
			f.pending = true
			f.changedAt = w.Now().Add(-w.Settle)
		}
	}
}
