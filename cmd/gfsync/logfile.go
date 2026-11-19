package main

import (
	"os"
	"path/filepath"
	"sync"
)

const maxLogBytes = 1 << 20

// logFile is gfsync.log. If it cannot be opened (for example security
// software blocks GF Sync's own folder), writes are dropped instead of going
// silently to stderr, Err reports the problem so the tray can show it, and
// every later write tries to open the file again.
type logFile struct {
	path string

	mu  sync.Mutex
	f   *os.File
	err error
}

func newLogFile(dir string) *logFile {
	l := &logFile{path: filepath.Join(dir, "gfsync.log")}
	l.mu.Lock()
	l.openLocked(true)
	l.mu.Unlock()
	return l
}

func (l *logFile) openLocked(rotate bool) {
	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		l.err = err
		return
	}
	if rotate {
		if st, err := os.Stat(l.path); err == nil && st.Size() > maxLogBytes {
			_ = os.Rename(l.path, l.path+".old")
		}
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	l.f, l.err = f, err
}

func (l *logFile) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		l.openLocked(false)
		if l.f == nil {
			return len(p), nil // reported through Err, not stderr
		}
	}
	if _, err := l.f.Write(p); err != nil {
		l.err = err
		_ = l.f.Close()
		l.f = nil
		return len(p), nil
	}
	l.err = nil
	return len(p), nil
}

// Err is nil while the log is being written.
func (l *logFile) Err() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.err
}
