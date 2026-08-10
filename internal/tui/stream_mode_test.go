package tui

import (
	"io"
	"os"
	"strings"
	"testing"
)

// TestEmitLineStreamsToStdoutAfterRelease verifies that once the TUI has been
// left via Ctrl+C (stream mode) and the terminal has been released, emitLine
// writes to stdout instead of the log view.
func TestEmitLineStreamsToStdoutAfterRelease(t *testing.T) {
	a := New(nil, true, nil, "test")

	// Simulate the TUI having been left via Ctrl+C and the terminal restored.
	a.streamMode.Store(true)
	a.Release()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	a.emitLine("hello stream")

	w.Close()
	os.Stdout = old

	out, _ := io.ReadAll(r)
	if !strings.Contains(string(out), "hello stream") {
		t.Fatalf("expected streamed line on stdout, got %q", out)
	}
}

// TestFinishReadSignalsReaderDone verifies that EOF signals main via
// ReaderDone() in stream mode (where the TUI is already gone).
func TestFinishReadSignalsReaderDone(t *testing.T) {
	a := New(nil, true, nil, "test")

	select {
	case <-a.ReaderDone():
		t.Fatal("readerDone should not be closed yet")
	default:
	}

	a.streamMode.Store(true)
	a.finishRead()

	select {
	case <-a.ReaderDone():
		// ok
	default:
		t.Fatal("readerDone was not closed after EOF")
	}
}
