// Package logfile persists raw log lines to per-day files in a directory,
// retaining them for a fixed period.  Writes are buffered and flushed
// periodically so logging never blocks the caller's hot path.
package logfile

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Writer appends raw log lines to a per-day file.  Safe for concurrent use.
type Writer struct {
	mu        sync.Mutex
	dir       string
	prefix    string
	retention time.Duration

	now   func() time.Time // injectable clock for tests
	file  *os.File
	buf   *bufio.Writer
	today string // current day key "2006-01-02"

	flushInterval time.Duration
	stop          chan struct{}
	done          chan struct{}
	closeOnce     sync.Once
	closed        bool
	errOnce       bool
}

// New creates a Writer writing files named <prefix>-2006-01-02.log in the
// system temp directory.  It opens today's file, starts a background flush
// loop, and removes files older than retention.
func New(prefix string, retention time.Duration) (*Writer, error) {
	return newWithDir(os.TempDir(), prefix, retention)
}

// newWithDir is New with an explicit directory (used by tests).
func newWithDir(dir, prefix string, retention time.Duration) (*Writer, error) {
	w := &Writer{
		dir:           dir,
		prefix:        prefix,
		retention:     retention,
		now:           time.Now,
		flushInterval: 30 * time.Second,
		stop:          make(chan struct{}),
		done:          make(chan struct{}),
	}
	if err := w.openToday(); err != nil {
		return nil, err
	}
	w.cleanup()
	go w.flushLoop()
	return w, nil
}

// WriteLine appends one raw log line to today's file (a trailing newline is
// added).  If the calendar day has changed, it rotates to a new file.  Errors
// are reported once on stderr and otherwise ignored so the caller's pipe is
// never blocked or killed by a broken log file.
func (w *Writer) WriteLine(line string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return // writer already closed: never reopen files
	}
	if today := w.now().Format("2006-01-02"); today != w.today {
		w.rotate(today)
	}
	if w.buf == nil {
		return // file unavailable (open failed / already closed)
	}
	if _, err := w.buf.WriteString(line + "\n"); err != nil {
		w.reportOnce(err)
	}
}

// Close stops the background flush loop, flushes buffered data and closes the
// current file.  It is safe to call more than once.
func (w *Writer) Close() error {
	var err error
	w.closeOnce.Do(func() {
		close(w.stop)
		<-w.done // wait for the flush loop to exit
		w.mu.Lock()
		defer w.mu.Unlock()
		w.flushLocked()
		w.buf = nil
		if w.file != nil {
			err = w.file.Close()
			w.file = nil
		}
		w.closed = true
	})
	return err
}

// flushLoop periodically flushes buffered lines to disk.
func (w *Writer) flushLoop() {
	ticker := time.NewTicker(w.flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			w.mu.Lock()
			w.flushLocked()
			w.mu.Unlock()
		case <-w.stop:
			close(w.done)
			return
		}
	}
}

// flushLocked flushes the buffered writer.  Caller must hold w.mu.
func (w *Writer) flushLocked() {
	if w.buf != nil {
		if err := w.buf.Flush(); err != nil {
			w.reportOnce(err)
		}
	}
}

// rotate flushes and closes the current file, then opens the new day's file
// and prunes expired files.  Caller must hold w.mu.
func (w *Writer) rotate(today string) {
	w.flushLocked()
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
		w.buf = nil
	}
	w.today = today
	if err := w.openToday(); err != nil {
		w.reportOnce(err)
	}
	w.cleanup()
}

// openToday opens (creating if needed) today's file in append mode.
func (w *Writer) openToday() error {
	w.today = w.now().Format("2006-01-02")
	name := fmt.Sprintf("%s-%s.log", w.prefix, w.today)
	f, err := os.OpenFile(filepath.Join(w.dir, name), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	w.file = f
	w.buf = bufio.NewWriter(f)
	return nil
}

// cleanup removes same-prefix log files whose mtime is older than retention.
func (w *Writer) cleanup() {
	cutoff := w.now().Add(-w.retention)
	matches, err := filepath.Glob(filepath.Join(w.dir, w.prefix+"-*.log"))
	if err != nil {
		return
	}
	for _, m := range matches {
		if fi, err := os.Stat(m); err == nil && fi.ModTime().Before(cutoff) {
			_ = os.Remove(m)
		}
	}
}

// reportOnce prints a log-file error to stderr the first time it happens, so a
// persistent problem does not spam the terminal.
func (w *Writer) reportOnce(err error) {
	if w.errOnce {
		return
	}
	w.errOnce = true
	fmt.Fprintf(os.Stderr, "(log file: %v)\n", err)
}
