package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"unsafe"

	"routatic-proxy-mate/internal/output"
	"routatic-proxy-mate/internal/parser"
	"routatic-proxy-mate/internal/stats"
	"routatic-proxy-mate/internal/tui"
)

func main() {
	noColor := flag.Bool("no-color", false, "disable ANSI color output")
	showVersion := flag.Bool("version", false, "show version")
	noTUI := flag.Bool("no-tui", false, "disable TUI mode (legacy pipe filter)")
	flag.Parse()

	if *showVersion {
		fmt.Println("routatic-proxy-mate v0.2.0")
		return
	}

	// Check stdin is a pipe (identical for both modes).
	info, err := os.Stdin.Stat()
	if err != nil || (info.Mode()&os.ModeCharDevice) != 0 {
		fmt.Fprintln(os.Stderr, "Usage: routatic-proxy serve | routatic-proxy-mate")
		fmt.Fprintln(os.Stderr, "  (stdin must be a pipe)")
		os.Exit(1)
	}

	agg := stats.New()

	// When stdout is a terminal we use the TUI; otherwise fall back to the
	// original line-by-line pipe filter.
	if !*noTUI && isTerminal() {
		runTUI(os.Stdin, agg, *noColor)
	} else {
		runLegacy(os.Stdin, agg, *noColor)
	}
}

// ---------------------------------------------------------------------------
// TUI mode
// ---------------------------------------------------------------------------

func runTUI(stdin io.Reader, agg *stats.Aggregator, noColor bool) {
	app := tui.New(agg, noColor)
	if err := app.Run(stdin); err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
	}
	// After the TUI has exited, print the summary to stdout so the user
	// sees it in the terminal scrollback.
	if !noColor {
		enableWindowsANSI()
	}
	output.WriteSummary(agg, noColor)
}

// ---------------------------------------------------------------------------
// Legacy pipe-filter mode
// ---------------------------------------------------------------------------

func runLegacy(stdin io.Reader, agg *stats.Aggregator, noColor bool) {
	if !noColor {
		enableWindowsANSI()
	}

	// Trap interrupts so we can show the summary before exit.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	done := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(stdin)
		// Increase buffer for long lines (errors may be verbose).
		scanner.Buffer(make([]byte, 64*1024), 256*1024)
		for scanner.Scan() {
			line := scanner.Text()
			entry, err := parser.ParseLine(line)
			if err != nil || entry == nil {
				if line != "" {
					fmt.Println(output.ColorizeFallback(line, noColor))
				}
				continue
			}

			// Record streaming completed stats.
			if parser.ClassifyMessage(entry.Message) == parser.MsgStreamingCompleted {
				agg.Record(
					entry.Fields["model"],
					entry.Fields["latency"],
					entry.Fields["input_tokens"],
					entry.Fields["output_tokens"],
					entry.Fields["cache_read_input_tokens"],
					entry.Fields["cache_creation_input_tokens"],
				)
			}

			// Print colourised log line.
			fmt.Println(output.ColorizeRawLine(line, noColor))
		}
		close(done)
	}()

	select {
	case <-done:
		// Normal EOF — print summary.
	case <-sigCh:
		// Interrupted — print partial summary.
		fmt.Fprintln(os.Stderr, "\n(interrupted — partial summary follows)")
	}

	output.WriteSummary(agg, noColor)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// isTerminal reports whether stdout is a terminal (rather than a pipe or
// file redirection).
func isTerminal() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

// enableWindowsANSI attempts to enable ANSI virtual-terminal processing on
// Windows. It is a no-op on other platforms and when the call fails.
func enableWindowsANSI() {
	const (
		enableVirtualTerminalProcessing = 0x0004
		stdOutputHandle                 = 0xFFFFFFF5 // (DWORD)-11 = STD_OUTPUT_HANDLE
	)

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getStdHandle := kernel32.NewProc("GetStdHandle")
	getConsoleMode := kernel32.NewProc("GetConsoleMode")
	setConsoleMode := kernel32.NewProc("SetConsoleMode")

	handle, _, _ := getStdHandle.Call(uintptr(stdOutputHandle))
	if handle == 0 || handle == ^uintptr(0) {
		return
	}

	var mode uint32
	ret, _, _ := getConsoleMode.Call(handle, uintptr(unsafe.Pointer(&mode)))
	if ret == 0 {
		return
	}

	mode |= enableVirtualTerminalProcessing
	setConsoleMode.Call(handle, uintptr(mode))
}
