# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`routatic-proxy-mate` is a CLI tool that reads the stdout of `routatic-proxy serve` via stdin pipe (`|`), colorizes each log line, and outputs `streaming completed` summary statistics grouped by model. The `examples/` directory contains sample raw log files for development and testing.

## Project Structure

```
routatic-proxy-mate/
├── main.go                    # Entry point: stdin reading, signal handling, orchestration
├── internal/
│   ├── parser/parser.go       # Log line parser (key=value with quoted and concatenated values)
│   ├── output/output.go       # ANSI color output + summary table formatter
│   └── stats/aggregator.go    # Per-model streaming completed stats aggregation
└── examples/
    ├── 命令提示符1 - routatic-proxy  serve.txt
    └── 命令提示符2 - routatic-proxy  serve.txt
```

## Build & Run

- **Build**: `go build -o routatic-proxy-mate.exe .`
- **Run with example data**: `./routatic-proxy-mate.exe --no-color < "examples/命令提示符1 - routatic-proxy  serve.txt"`
- **Run as pipe** (production): `routatic-proxy serve | routatic-proxy-mate`
- **Disable color**: `--no-color`
- **Version**: `--version`

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
