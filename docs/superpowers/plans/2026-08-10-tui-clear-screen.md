# TUI Clear-Screen Key (`c`) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Pressing `c` in the TUI clears the scrollable log view and appends a hint line, leaving the stats bar and aggregation untouched.

**Architecture:** Add a `KeyRune`/`'c'` branch to the existing `SetInputCapture` in `internal/tui/tui.go` (currently only `Ctrl+C`). The branch calls a new `App.clearLogView()` method that uses tview's canonical `TextView.Clear()` then writes a muted hint line and scrolls to end. The input capture runs on the tview UI goroutine, so direct view mutation needs no `QueueUpdateDraw`.

**Tech Stack:** Go, tview v0.42.0, tcell v2.

## Global Constraints

- Only lowercase `c` triggers the clear; `Ctrl+C` behaviour is unchanged.
- Only the log view is cleared. The stats bar stays visible, the `Aggregator` is never reset, and the `logfile.Writer` keeps persisting raw lines.
- The hint line text is exactly `[gray]—— screen cleared ——\n`.
- Follow existing code style in `internal/tui/tui.go` (comments on methods, tabs for indentation).

---

### Task 1: Add `c` key handling and `clearLogView`

**Files:**
- Modify: `internal/tui/tui.go:132-138` (the `SetInputCapture` block in `Run`)
- Modify: `internal/tui/tui.go` (add `clearLogView` method, placed after `appendLine` ~line 325)
- Test: `internal/tui/clear_screen_test.go` (create)

**Interfaces:**
- Consumes: `App` struct with `logView *tview.TextView`, `agg *stats.Aggregator`, existing `updateLayout()`. `New(agg, noColor, filter, version)` constructs an `App`.
- Produces: `func (a *App) clearLogView()` — no params, no return.

- [ ] **Step 1: Write the failing test**

Create `internal/tui/clear_screen_test.go`:

```go
package tui

import (
	"strings"
	"testing"

	"routatic-proxy-mate/internal/stats"
)

// TestClearLogView verifies that clearLogView empties the log buffer and
// appends a hint line. The stats bar state is left untouched.
func TestClearLogView(t *testing.T) {
	a := New(stats.New(), true, nil, "test")
	a.logView.SetText("line one\nline two\n")

	a.clearLogView()

	got := a.logView.GetText(false)
	if !strings.Contains(got, "screen cleared") {
		t.Fatalf("expected a clear hint line, got %q", got)
	}
	if strings.Contains(got, "line one") {
		t.Fatalf("expected old lines to be cleared, got %q", got)
	}
}
```

Note: `New` needs a real aggregator (`stats.New()`, not `nil`) because `clearLogView` calls `updateLayout()` which reads `agg.Total()`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestClearLogView -v`
Expected: FAIL with `undefined: (a *App) clearLogView`

- [ ] **Step 3: Add the input-capture branch**

In `internal/tui/tui.go`, inside `Run()` (lines 132-138), extend the existing capture:

```go
	a.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlC {
			a.enterStreamMode()
			return nil
		}
		if event.Key() == tcell.KeyRune && event.Rune() == 'c' {
			a.clearLogView()
			return nil
		}
		return event
	})
```

- [ ] **Step 4: Add the `clearLogView` method**

Immediately after the `appendLine` method (ends at line 325 in `tui.go`), add:

```go
// clearLogView empties the scrollable log view and appends a hint line so the
// user knows the screen was cleared. The stats bar and aggregation are
// unaffected. It runs on the tview UI goroutine (called from the input
// capture), so it may touch the view directly.
func (a *App) clearLogView() {
	a.logView.Clear()
	fmt.Fprint(a.logView, "[gray]—— screen cleared ——\n")
	a.logView.ScrollToEnd()
	a.updateLayout()
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/tui/ -run TestClearLogView -v`
Expected: PASS

- [ ] **Step 6: Run the full test suite and build**

Run: `go test ./...`
Expected: `ok` for all packages with tests, no failures.

Run: `go build -o routatic-proxy-mate.exe .`
Expected: builds without error, `routatic-proxy-mate.exe` created.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/tui.go internal/tui/clear_screen_test.go
git commit -m "feat: add c key to clear TUI log view

Co-Authored-By: Claude <noreply@anthropic.com>"
```

### Task 2: Manual verification in a live TUI (no code)

- [ ] **Step 1: Run the pipe harness and press `c`**

Run: `go run ./upstream/serve | ./routatic-proxy-mate.exe`
Expected: log lines stream into the TUI. Press `c`: the log view clears and shows only `—— screen cleared ——`; the bottom stats bar remains visible once `streaming completed` entries arrive. New lines keep auto-scrolling after the clear. `Ctrl+C` still leaves the TUI and streams raw lines to stdout.
