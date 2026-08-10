# TUI `c` key: Clear Log View

Date: 2026-08-10
Status: Approved

## Goal

In interactive TUI mode, pressing `c` clears the scrollable log view and
appends a hint line to inform the user the screen was cleared. The bottom
stats bar and all aggregation are unaffected.

## Background

The TUI (`internal/tui/tui.go`) shows colourised log lines in a scrollable
`tview.TextView` (`logView`) capped at 500 lines (`maxLogLines`), with a
stats bar pinned to the bottom. Keyboard input is handled by a single
`SetInputCapture` in `Run()` that currently only intercepts `Ctrl+C` (to
leave the TUI and stream raw lines).

## Design

### Approach

Use tview's canonical `TextView.Clear()` — it resets the internal text
buffer and rebuilds the max-lines index, so subsequent `appendLine` writes
start counting from zero. `SetText("")` was rejected as it bypasses the
index reset; recreating the view was rejected as over-scoped.

### Keyboard handling

Add a `KeyRune`/`'c'` branch to the existing input capture in `Run()`
(`tui.go`, ~line 132). `Ctrl+C` handling is unchanged.

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

Only lowercase `c` is handled.

### New method: `clearLogView`

```go
func (a *App) clearLogView() {
    a.logView.Clear()
    fmt.Fprint(a.logView, "[gray]—— screen cleared ——\n")
    a.logView.ScrollToEnd()
    a.updateLayout()
}
```

The input capture runs on the tview UI goroutine, so direct mutation of
`logView` is safe and needs no `QueueUpdateDraw` (that pattern is only
required for the reader goroutine).

## Behavioural guarantees

- Only the log view is cleared. The stats bar stays visible and continues
  updating; the `Aggregator` is never reset.
- The `logfile.Writer` keeps persisting raw lines; clearing the screen does
  not affect the on-disk log.
- After `Clear()`, `logView` has 0 lines, so `appendLine`'s `atBottom`
  computation immediately resumes bottom-tracking and new lines auto-scroll.
- `Ctrl+C` behaviour is unchanged.

## Testing

- `go build -o routatic-proxy-mate.exe .` compiles.
- Manual verification: pipe the `upstream/` serve test harness into the
  binary (`go run ./upstream/serve | routatic-proxy-mate.exe`), press `c`,
  confirm the view clears and the hint line appears while the stats bar
  persists.
- Existing `go test ./...` stays green (no unit test is feasible for the
  input-capture path; it is UI-goroutine bound).
