# upstream/serve

A tiny test harness that simulates `routatic-proxy serve` streaming to stdout,
so you can exercise `routatic-proxy-mate` through the pipe without a real
server.

## Build

```sh
go build -o serve.exe ./upstream
```

## Usage

```sh
# Loop a sample log forever: ~0.5-1.5s between lines, extra 3-5s rest every 10
serve.exe "examples/example1 - routatic-proxy  serve.txt" | routatic-proxy-mate.exe

# Single pass then exit -> exercises the EOF/summary path
serve.exe --once "examples/example1 - routatic-proxy  serve.txt" | routatic-proxy-mate.exe --no-tui

# Faster stream for quicker testing
serve.exe --min 0.1 --max 0.2 --rest-min 0.5 --rest-max 1 "examples/example1 - routatic-proxy  serve.txt" | routatic-proxy-mate.exe
```

## Flags

| Flag | Default | Meaning |
|------|---------|---------|
| `--min`  | `0.5` | minimum seconds between lines |
| `--max`  | `1.5` | maximum seconds between lines |
| `--every` | `10` | take an extra rest after every N lines |
| `--rest-min` | `3` | minimum extra rest seconds |
| `--rest-max` | `5` | maximum extra rest seconds |
| `--once` | `false` | emit one pass through the file, then exit (EOF) |
