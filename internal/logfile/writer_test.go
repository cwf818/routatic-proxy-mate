package logfile

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestWriter(t *testing.T, prefix string) (*Writer, string) {
	t.Helper()
	dir := t.TempDir()
	w, err := newWithDir(dir, prefix, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w, dir
}

func flush(w *Writer) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.flushLocked()
}

func TestWriteLinePersistsToTodayFile(t *testing.T) {
	w, dir := newTestWriter(t, "mate")
	w.WriteLine("first line")
	w.WriteLine("second line")
	flush(w)

	today := time.Now().Format("2006-01-02")
	data, err := os.ReadFile(filepath.Join(dir, "mate-"+today+".log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "first line\nsecond line\n" {
		t.Fatalf("unexpected content: %q", data)
	}
}

func TestRotatesOnDayChange(t *testing.T) {
	w, dir := newTestWriter(t, "mate")

	day1 := time.Date(2026, 8, 1, 12, 0, 0, 0, time.Local)
	day2 := time.Date(2026, 8, 2, 12, 0, 0, 0, time.Local)
	w.now = func() time.Time { return day1 }
	w.WriteLine("day one")

	w.now = func() time.Time { return day2 }
	w.WriteLine("day two")

	flush(w)

	d1, err := os.ReadFile(filepath.Join(dir, "mate-2026-08-01.log"))
	if err != nil {
		t.Fatalf("day1 file missing: %v", err)
	}
	if string(d1) != "day one\n" {
		t.Fatalf("day1 content: %q", d1)
	}
	d2, err := os.ReadFile(filepath.Join(dir, "mate-2026-08-02.log"))
	if err != nil {
		t.Fatalf("day2 file missing: %v", err)
	}
	if string(d2) != "day two\n" {
		t.Fatalf("day2 content: %q", d2)
	}
}

func TestCleanupRemovesExpiredFiles(t *testing.T) {
	dir := t.TempDir()
	oldFile := filepath.Join(dir, "mate-2026-07-01.log")
	newFile := filepath.Join(dir, "mate-2026-08-01.log")
	for _, p := range []string{oldFile, newFile} {
		if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tenDaysAgo := time.Now().AddDate(0, 0, -10)
	if err := os.Chtimes(oldFile, tenDaysAgo, tenDaysAgo); err != nil {
		t.Fatal(err)
	}

	w, err := newWithDir(dir, "mate", 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Close() }()

	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatalf("old file should be removed, got %v", err)
	}
	if _, err := os.Stat(newFile); err != nil {
		t.Fatalf("recent file should remain: %v", err)
	}
}

func TestCloseIdempotent(t *testing.T) {
	w, _ := newTestWriter(t, "mate")
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal("second Close should return nil")
	}
}

func TestWriteLineAfterCloseNoPanic(t *testing.T) {
	w, _ := newTestWriter(t, "mate")
	w.Close()
	w.WriteLine("ignored") // must not panic
}

// TestWriteFailureReportsOnceAndContinues verifies that a broken log file
// (write failure) does not panic and is reported at most once, so the caller's
// pipe is never killed by a log-file write error.
func TestWriteFailureReportsOnceAndContinues(t *testing.T) {
	w, _ := newTestWriter(t, "mate")

	// Force a real write error: create a file, close it, then write through a
	// buffer over the now-closed file descriptor. The buffer must actually hold
	// data, otherwise bufio.Writer.Flush is a no-op and never touches the file.
	f, err := os.CreateTemp(t.TempDir(), "broken-*.log")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	w.mu.Lock()
	w.buf = bufio.NewWriter(f)
	w.mu.Unlock()

	// First failure: WriteLine buffers a line (no error yet), then the flush
	// writes to the closed file, errors, and is reported once.
	w.WriteLine("first line")
	flush(w)
	if !w.errOnce {
		t.Fatal("reportOnce should have fired after first flush failure")
	}

	// Second failure: must not panic and must not be reported again (once-only).
	w.WriteLine("second line")
	flush(w)
	if !w.errOnce {
		t.Fatal("errOnce should remain set")
	}
}
