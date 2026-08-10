# routatic-proxy-mate

`routatic-proxy-mate` is a CLI tool that reads the stdout of `routatic-proxy serve` via stdin pipe (`|`), colorizes each log line, and outputs `streaming completed` summary statistics grouped by model.

![GitHub Release](https://img.shields.io/github/v/release/cwf818/routatic-proxy-mate)
![License](https://img.shields.io/github/license/cwf818/routatic-proxy-mate)

![Screenshot](screenshot/screenshot.png)

## Installation

Download the pre-built Windows binary from the [latest release](https://github.com/cwf818/routatic-proxy-mate/releases/latest), or install via Go:

```bash
go install github.com/cwf818/routatic-proxy-mate@latest
```

Or build from source:

```bash
git clone https://github.com/cwf818/routatic-proxy-mate.git
cd routatic-proxy-mate
go build -o routatic-proxy-mate.exe .
```

## Usage

Pipe mode (auto-enables TUI):

```bash
routatic-proxy serve | routatic-proxy-mate
```

> **PowerShell users**: run with `cmd /c` to ensure correct piping — `cmd /c "routatic-proxy serve | routatic-proxy-mate"`

Legacy mode (no TUI):

```bash
routatic-proxy-mate --no-tui < examples.log
```

### Options

| Flag | Description |
|------|-------------|
| `--no-color` | Disable ANSI color output |
| `--no-tui` | Force legacy pipe-filter mode (no interactive TUI) |
| `--row` | Only colorize lines containing this string (repeatable) |
| `--key` | Only colorize the specified log key (repeatable, e.g. `--key model --key latency`) |
| `--version` | Show version |

## TUI Mode

When stdout is a terminal, the tool enters interactive TUI mode (powered by `tview`):

- **Log area**: scrollable view showing colorized log lines
- **Stats header**: a dynamic summary bar pinned to the bottom — appears when log content overflows the visible area and the user has scrolled down
- **Auto-scroll**: follows new content by default; pausing when scrolling up, resumes when reaching the bottom
- **Keyboard**: `↑`/`↓`/`PgUp`/`PgDn` to scroll. The first `Ctrl+C` closes the TUI and keeps the pipe alive, switching to streaming raw (colorized) log lines to the current terminal; a second `Ctrl+C` (or EOF) fully exits
- **On exit**: full summary table is printed to stdout after the TUI closes

## Log Parsing

Supports 23 message types including request received, routing, streaming, fallback, and errors. The parser handles concatenated `key=value` pairs without spaces:

```
latency=3.189401sinput_tokens=0
cache_read_input_tokens=143360cache_creation_input_tokens=578
```

## Summary Statistics

All `streaming completed` entries are aggregated by model in a table with the following columns:

| Column | Description |
|--------|-------------|
| `Model` | Model name |
| `OK/Att` | Successful requests / total streaming attempts |
| `Total` | Total latency |
| `Avg` | Average latency |
| `OutTok` | Total output tokens |
| `CacheRd` | Total cache read tokens |
| `CacheCr` | Total cache creation tokens |
| `CacheHit` | Cache hit rate (CacheRd / (CacheRd + CacheCr)) |
| `SpdAvg` | Average output speed (tokens/s) |
| `SpdMax` | Max output speed (tokens/s) |
| `SpdMin` | Min output speed (tokens/s) |

The TUI stats header also displays: total input tokens, current real-time output speed, and per-model request & token info.

## License

[MIT](LICENSE)

---

[中文文档](README.zh.md)
