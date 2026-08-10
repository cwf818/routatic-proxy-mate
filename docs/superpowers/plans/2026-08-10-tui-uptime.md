# TUI Stats-Bar UpTime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show the program's running time in the TUI stats bar immediately after `ApiTime`, in dhms format, ticking live as the bar refreshes.

**Architecture:** Record the process start time at the top of `main()`, thread it through `runTUI` into `tui.New`, and store it on `App.started`. `buildStatsText` line 2 appends `| UpTime: <fmtDuration(time.Since(a.started))>` after `ApiTime`. Only the stats bar changes; summary tables and legacy mode are untouched.

**Tech Stack:** Go, tview v0.42.0.

## Global Constraints

- UpTime appears only on TUI stats bar line 2, immediately after `ApiTime`, and is formatted with the existing `fmtDuration` (compact dhms, e.g. `1d2h3m4s`).
- The runtime reference is the process start time recorded at the top of `main()`.
- Summary tables (TUI exit table, legacy `output.WriteSummary`) are NOT changed.
- Follow existing code style in `internal/tui/tui.go` and `main.go` (tabs, method comments).

---

### Task 1: Thread process start time and display UpTime

**Files:**
- Modify: `main.go` (top of `main()`, `runTUI` signature + call)
- Modify: `internal/tui/tui.go` (App struct, `New` signature, `buildStatsText` line 2)
- Test: `internal/tui/uptime_test.go` (create)
- Modify: `internal/tui/stream_mode_test.go:14,41` (`New` calls + `time` import)
- Modify: `internal/tui/clear_screen_test.go:16` (`New` call)

**Interfaces:**
- Consumes: existing `App` (`logView`, `agg`, `updateLayout`, `buildStatsText`), existing `fmtDuration(time.Duration) string`, existing `stats.New()` / `agg.Record(...)`.
- Produces: `func New(agg *stats.Aggregator, noColor bool, filter *output.ColorFilter, version string, started time.Time) *App` — sets `App.started`. `runTUI(stdin io.Reader, agg *stats.Aggregator, noColor bool, filter *output.ColorFilter, logWriter *logfile.Writer, started time.Time)`.

- [ ] **Step 1: Write the failing test**

Create `internal/tui/uptime_test.go`:

```go
package tui

import (
	"strings"
	"testing"
	"time"

	"routatic-proxy-mate/internal/stats"
)

// TestBuildStatsTextShowsUpTime verifies the stats bar shows UpTime right
// after ApiTime, formatted as a dhms duration since the app started.
func TestBuildStatsTextShowsUpTime(t *testing.T) {
	a := New(stats.New(), true, nil, "test", time.Now())
	// Seed one model so buildStatsText renders the full stats bar (it
	// early-returns a "waiting" message when no models exist).
	a.agg.Record("model-a", "1s", "10", "100", "1000", "50", time.Now())
	// Pin the start time 2 hours in the past so the UpTime is deterministic.
	a.started = time.Now().Add(-2 * time.Hour)

	got := a.buildStatsText()
	if !strings.Contains(got, "UpTime: 2h") {
		t.Fatalf("expected UpTime after ApiTime, got %q", got)
	}
	if strings.Index(got, "ApiTime") > strings.Index(got, "UpTime") {
		t.Fatalf("expected UpTime to follow ApiTime, got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestBuildStatsTextShowsUpTime`
Expected: FAIL with a compile error — `New` currently takes 4 arguments, the test passes 5 (`too many arguments in call to New`).

- [ ] **Step 3: Add `started` to `App` and change `New`**

In `internal/tui/tui.go`:
- Add a field to the `App` struct (near the `statsShown`/`done` fields, around line 48):

```go
	started    time.Time
```

- Change the `New` signature and set the field. Current signature (line 78):

```go
func New(agg *stats.Aggregator, noColor bool, filter *output.ColorFilter, version string) *App {
	a := &App{
		Application: tview.NewApplication(),
		agg:         agg,
		noColor:     noColor,
		filter:      filter,
		exited:      make(chan struct{}),
		levelCounts: make(map[string]int64),
		released:    make(chan struct{}),
		readerDone:  make(chan struct{}),
	}
```

Change to:

```go
func New(agg *stats.Aggregator, noColor bool, filter *output.ColorFilter, version string, started time.Time) *App {
	a := &App{
		Application: tview.NewApplication(),
		agg:         agg,
		noColor:     noColor,
		filter:      filter,
		started:     started,
		exited:      make(chan struct{}),
		levelCounts: make(map[string]int64),
		released:    make(chan struct{}),
		readerDone:  make(chan struct{}),
	}
```

- [ ] **Step 4: Append UpTime to stats bar line 2**

In `buildStatsText` (`tui.go`, around line 403), change the line-2 `fmt.Sprintf` to append `| UpTime` after `ApiTime`:

```go
	b.WriteString(fmt.Sprintf(
		"[white::b]OutSpeed: Current: [%s]%.1f/s  "+
			"[white]|  Avg: [%s]%.1f/s  "+
			"[white]|  Max: [lime]%.1f/s  "+
			"[white]|  Min: [lime]%.1f/s  "+
			"[white]|  ApiTime: [lime]%s  "+
			"[white]|  UpTime: [lime]%s",
		speedTag(tt.CurrentOutSpeed), tt.CurrentOutSpeed,
		speedTag(tt.AvgOutSpeed()), tt.AvgOutSpeed(),
		tt.MaxOutSpeed,
		tt.MinOutSpeed,
		fmtDuration(tt.TotalLatency),
		fmtDuration(time.Since(a.started)),
	))
```

`tui.go` already imports `time`.

- [ ] **Step 5: Thread start time through `main.go`**

- At the very top of `main()` (line 34, before the flag setup), add:

```go
	started := time.Now()
```

- Change the `runTUI` call (line 81):

```go
		runTUI(os.Stdin, agg, *noColor, filter, logWriter, started)
```

- Change the `runTUI` signature (line 91) and the `tui.New` call inside it (line 92):

```go
func runTUI(stdin io.Reader, agg *stats.Aggregator, noColor bool, filter *output.ColorFilter, logWriter *logfile.Writer, started time.Time) {
	app := tui.New(agg, noColor, filter, Version, started)
```

`main.go` already imports `time`.

- [ ] **Step 6: Update the existing `New` test calls**

- `internal/tui/stream_mode_test.go` lines 14 and 41:
  - `a := New(nil, true, nil, "test")` → `a := New(nil, true, nil, "test", time.Now())` (both occurrences).
  - Add `"time"` to the import block.
- `internal/tui/clear_screen_test.go` line ~16:
  - `a := New(stats.New(), true, nil, "test")` → `a := New(stats.New(), true, nil, "test", time.Now())`.
  - (`time` is already imported there.)

- [ ] **Step 7: Run the new test to verify it passes**

Run: `go test ./internal/tui/ -run TestBuildStatsTextShowsUpTime -v`
Expected: PASS

- [ ] **Step 8: Run the full suite and build**

Run: `go test ./...`
Expected: `ok` for all packages with tests, no failures.

Run: `go build -o routatic-proxy-mate.exe .`
Expected: builds without error.

- [ ] **Step 9: Commit**

```bash
git add main.go internal/tui/tui.go internal/tui/uptime_test.go internal/tui/stream_mode_test.go internal/tui/clear_screen_test.go
git commit -m "feat: show UpTime (program runtime) in TUI stats bar

Co-Authored-By: Claude <noreply@anthropic.com>"
```

### Task 2: Manual verification in a live TUI (no code)

- [ ] **Step 1: Run the pipe harness and watch UpTime**

Run: `go run ./upstream/serve | ./routatic-proxy-mate.exe`
Expected: once `streaming completed` entries arrive, the stats bar line 2 shows `ApiTime: <d> | UpTime: <dhms>` and the UpTime value ticks upward every second. Everything else (log view, `c` clear, Ctrl+C) behaves as before.
