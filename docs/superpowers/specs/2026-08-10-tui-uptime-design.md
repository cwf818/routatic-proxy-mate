# TUI Stats Bar: Add UpTime After ApiTime

Date: 2026-08-10
Status: Approved

## Goal

Show the program's running time in the TUI stats bar, immediately after
`ApiTime`, in days/hours/minutes/seconds format. The value ticks live as the
stats bar refreshes.

## Scope

Only the TUI bottom stats bar line 2 is changed. The summary tables (TUI
exit table and legacy `output.WriteSummary`) are untouched: their `ApiTime`
is a per-model value, and a global UpTime column would repeat the same number
on every row.

## Design

### Runtime reference

Record the process start time at the top of `main()` and thread it through
`runTUI` into `tui.New`, so UpTime measures the program's wall-clock runtime
from launch (not TUI start).

### Changes

**`main.go`**
- `main()`: `started := time.Now()` at the top.
- `runTUI(...)`: accept `started time.Time`, pass it to `tui.New`.

**`internal/tui/tui.go`**
- `App` gains `started time.Time`.
- `New(agg, noColor, filter, version string, started time.Time)` sets it.
- `buildStatsText` line 2 appends `[white]|  UpTime: [lime]%s` after
  `ApiTime`, value `fmtDuration(time.Since(a.started))`.

**Format:** reuse the existing `fmtDuration`, which already emits a
compact dhms form (`1d2h3m4s`, leading zero units omitted), consistent with
how `ApiTime` renders.

**Tests**
- Update the three existing `New(...)` calls in `internal/tui` tests
  (`stream_mode_test.go` ×2, `clear_screen_test.go` ×1) to pass a
  `time.Now()` start time.
- New test: set `a.started = time.Now().Add(-2 * time.Hour)`, call
  `buildStatsText`, assert the output contains `UpTime: 2h`.

## Behavioural guarantees

- UpTime appears only on stats bar line 2, after ApiTime, formatted with the
  same dhms style as other durations.
- It updates live (stats bar refreshes every 200 ms via `layoutLoop`).
- No other output (summary tables, legacy mode) changes.
