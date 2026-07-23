# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`routatic-proxy-mate` is a CLI tool that reads the stdout of `routatic-proxy serve` via stdin pipe (`|`), colorizes each log line, and outputs `streaming completed` summary statistics grouped by model. The `examples/` directory contains sample raw log files for development and testing.

## Project Structure

```
routatic-proxy-mate/
├── main.go                    # Entry point: stdin reading, TUI vs legacy dispatch
├── internal/
│   ├── parser/parser.go       # Log line parser (key=value with quoted and concatenated values)
│   ├── output/output.go       # ANSI color output + summary table formatter
│   ├── stats/aggregator.go    # Per-model streaming completed stats aggregation
│   └── tui/tui.go             # TUI application: scrollable log view, dynamic stats header
└── examples/
    ├── 命令提示符1 - routatic-proxy  serve.txt
    └── 命令提示符2 - routatic-proxy  serve.txt
```

## Build & Run

- **Build**: `go build -o routatic-proxy-mate.exe .`
- **Run with example data** (legacy mode): `./routatic-proxy-mate.exe --no-tui --no-color < "examples/命令提示符1 - routatic-proxy  serve.txt"`
- **Run as pipe** (automatic TUI when stdout is terminal): `routatic-proxy serve | routatic-proxy-mate`
- **Flags**:
  - `--no-color` — disable ANSI color output
  - `--no-tui` — force legacy pipe-filter mode (no interactive TUI)
  - `--version` — show version

## TUI Mode

When stdout is a terminal, the tool enters interactive TUI mode (using `tview`):

- **Log area**: scrollable view showing colourised log lines.
- **Stats header**: a dynamic summary bar pinned to the bottom. It appears when:
  1. The log content overflows the visible area, AND
  2. The user has scrolled down (not at the top).
- **Auto-scroll**: the view follows new content unless the user scrolls up. Scrolling to the bottom re-enables tracking.
- **Keyboard**: `↑`/`↓`/`PgUp`/`PgDn` to scroll, `Ctrl+C` to exit.
- **On exit**: the full summary table is printed to stdout after the TUI closes.

When stdout is piped or the `--no-tui` flag is set, the original line-by-line filter mode is used instead.

## Log Format

All logs use structured key=value pairs: `time=... level=INFO|WARN|ERROR msg="..." [fields...]`

### Known Message Types (23 types)

| Mnemonic | message(s) | Description |
|----------|------------|-------------|
| RECEIVED | `received request` | Request enters proxy |
| ROUTING | `routing request` | Model/provider selected |
| STREAMING | `attempting streaming model` | Stream call initiated |
| COMPLETED | `streaming completed` | Stream finished successfully |
| STREAM FAIL | `streaming request failed via provider` | Stream failed, fallback queued |
| FALLBACK | `attempting model` | Non-streaming fallback attempt (fields: `model`, `attempt`, `total`) |
| FATAL | `non-retryable error (skipping circuit breaker), trying fallback` | Provider error, trying next (fields: `model`, `error`, `remaining`) |
| ERROR | `request error` | All attempts exhausted (fields: `status`, `message`, `error`) |
| SEND ERR | `sending stream error` | Error streaming response back |
| CANCELED | `request context canceled during/after...` | Context canceled mid-request |
| OPENAI ERR | `openai streaming failed` | OpenAI-specific streaming failure |
| OK | `model succeeded` | Non-streaming fallback succeeded |
| DONE | `request completed` | Request finished via fallback |
| SYSTEM | config/log lifecycle messages | Server startup, config reload, shutdown |

### Parsing Edge Cases

The log format has concatenated key=value pairs without spaces, e.g.:
```
latency=3.189401sinput_tokens=0
cache_read_input_tokens=143360cache_creation_input_tokens=578
```

The parser uses a known-keys dictionary (in `parser.go`) to disambiguate these by checking whether a maximal word token preceding `=` ends with a known key name.

## Aggregation

The summary table aggregates all `streaming completed` entries by `model` with: request count, total/avg/min/max latency, total output tokens, total cache read tokens, and total cache creation tokens.
