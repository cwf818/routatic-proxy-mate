package tui

import (
	"bufio"
	"fmt"
	"hash/fnv"
	"io"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"routatic-proxy-mate/internal/output"
	"routatic-proxy-mate/internal/parser"
	"routatic-proxy-mate/internal/stats"
)

// App wraps a tview Application providing a scrollable log viewer with a
// stats bar pinned to the bottom that appears once streaming completed
// entries exist.
type App struct {
	*tview.Application

	logView   *tview.TextView
	statsView *tview.TextView
	flex      *tview.Flex
	agg       *stats.Aggregator
	noColor   bool

	statsShown bool
	done       bool

	// The main event loop signals this channel when Run() returns.
	exited chan struct{}
}

// New creates a new TUI application.  The aggregator is read periodically to
// build the stats bar.
func New(agg *stats.Aggregator, noColor bool) *App {
	a := &App{
		Application: tview.NewApplication(),
		agg:         agg,
		noColor:     noColor,
		exited:      make(chan struct{}),
	}

	// ---- stats bar (hidden until entries arrive) ----
	a.statsView = tview.NewTextView()
	a.statsView.SetBackgroundColor(tcell.ColorDarkCyan)
	a.statsView.SetTextAlign(tview.AlignCenter)
	a.statsView.SetDynamicColors(true)

	// ---- scrollable log view ----
	a.logView = tview.NewTextView()
	a.logView.SetDynamicColors(true)
	a.logView.SetScrollable(true)
	a.logView.SetBorder(true)
	a.logView.SetTitle(" Output Log ")
	a.logView.SetTitleAlign(tview.AlignLeft)
	a.logView.ScrollToEnd()

	// ---- layout: log on top, stats bar pinned to bottom ----
	a.flex = tview.NewFlex().SetDirection(tview.FlexRow)
	a.flex.AddItem(a.logView, 0, 1, true)
	a.flex.AddItem(a.statsView, 0, 0, false) // zero height until shown

	return a
}

// Run starts the TUI.  It reads from stdin in a background goroutine and
// blocks until the application exits (Ctrl+C / EOF).
func (a *App) Run(stdin io.Reader) error {
	go a.readStdin(stdin)

	// Periodically check scroll offset and refresh stats.
	go a.layoutLoop()

	// Intercept Ctrl+C to shut down gracefully.
	a.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlC {
			a.Stop()
			return nil
		}
		return event
	})

	err := a.SetRoot(a.flex, true).Run()
	close(a.exited)
	return err
}

// Done returns a channel that is closed when Run() returns.
func (a *App) Done() <-chan struct{} {
	return a.exited
}

// ---------------------------------------------------------------------------
// stdin reader
// ---------------------------------------------------------------------------

func (a *App) readStdin(stdin io.Reader) {
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 64*1024), 256*1024)

	for scanner.Scan() {
		line := scanner.Text()

		entry, err := parser.ParseLine(line)
		if err != nil || entry == nil {
			if line != "" {
				colored := output.ColorizeFallback(line, a.noColor)
				a.appendLine(colored)
			}
			continue
		}

		// Record streaming completed stats for the summary header.
		if parser.ClassifyMessage(entry.Message) == parser.MsgStreamingCompleted {
			a.agg.Record(
				entry.Fields["model"],
				entry.Fields["latency"],
				entry.Fields["input_tokens"],
				entry.Fields["output_tokens"],
				entry.Fields["cache_read_input_tokens"],
				entry.Fields["cache_creation_input_tokens"],
			)
		}

		colored := output.ColorizeRawLine(line, a.noColor)
		a.appendLine(colored)
	}

	// EOF — append the full summary to the log view, then exit after a short
	// pause so the user can read it.
	a.QueueUpdateDraw(func() {
		a.showFinalSummary()
		a.done = true
	})
	time.Sleep(3 * time.Second)
	a.Stop()
}

func (a *App) appendLine(line string) {
	a.QueueUpdateDraw(func() {
		// Only auto-scroll if the view was already tracking the bottom,
		// so the user can scroll up without being yanked back.
		_, _, _, height := a.logView.GetInnerRect()
		lineCount := a.logView.GetOriginalLineCount()
		scrollRow, _ := a.logView.GetScrollOffset()
		atBottom := lineCount <= height || scrollRow >= lineCount-height-1

		// Escape any literal [ in the ANSI-coloured string that could be
		// misinterpreted as a tview colour tag, then convert ANSI codes to
		// native tview tags.
		safe := tview.Escape(line)
		converted := tview.TranslateANSI(safe)
		fmt.Fprint(a.logView, converted+"\n")

		if atBottom {
			a.logView.ScrollToEnd()
		}
		a.updateLayout()
	})
}

// ---------------------------------------------------------------------------
// layout management
// ---------------------------------------------------------------------------

func (a *App) layoutLoop() {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.QueueUpdateDraw(func() {
				if a.done {
					return
				}
				a.updateLayout()
			})
		case <-a.exited:
			return
		}
	}
}

func (a *App) updateLayout() {
	if a.done {
		return
	}

	// Show the stats bar once at least one "streaming completed" entry exists.
	shouldShow := a.agg.Total().Requests > 0

	if shouldShow != a.statsShown {
		a.statsShown = shouldShow
		if shouldShow {
			a.statsView.SetText(a.buildStatsText())
			a.flex.ResizeItem(a.statsView, 4, 0)
		} else {
			a.flex.ResizeItem(a.statsView, 0, 0)
		}
	} else if a.statsShown {
		// Live-update even when visibility hasn't changed.
		a.statsView.SetText(a.buildStatsText())
	}
}

// ---------------------------------------------------------------------------
// summary text builders
// ---------------------------------------------------------------------------

func (a *App) buildStatsText() string {
	models := a.agg.Models()
	if len(models) == 0 {
		return "[white]Waiting for streaming completed data …"
	}

	tt := a.agg.Total()

	var b strings.Builder

	// Line 1: aggregated counts.
	b.WriteString(fmt.Sprintf(
		"[white::b]Streaming Completed: [lime]%d  "+
			"[white]|  Total: [lime]%s  "+
			"[white]|  Out: [lime]%s  "+
			"[white]|  Cache Rd: [lime]%s  "+
			"[white]|  Cache Cr: [lime]%s",
		tt.Requests,
		fmtDuration(tt.TotalLatency),
		abbreviate(tt.TotalOutputTokens),
		abbreviate(tt.TotalCacheReadTokens),
		abbreviate(tt.TotalCacheCreateTokens),
	))
	b.WriteByte('\n')

	// Line 2: OutSpeed.
	b.WriteString(fmt.Sprintf(
		"[white::b]OutSpeed: Avg: [lime]%.1f/s  "+
			"[white]|  Max: [lime]%.1f/s  "+
			"[white]|  Min: [lime]%.1f/s",
		tt.AvgOutSpeed(),
		tt.MaxOutSpeed,
		tt.MinOutSpeed,
	))
	b.WriteByte('\n')

	// Line 3: averages.
	b.WriteString(fmt.Sprintf(
		"[white::b]Avg Latency: [lime]%s  "+
			"[white]|  Avg Out/Req: [lime]%s  "+
			"[white]|  Total Input: [lime]%s",
		fmtDuration(tt.AvgLatency()),
		abbreviate(int64(float64(tt.TotalOutputTokens)/float64(tt.Requests))),
		abbreviate(tt.TotalInputTokens),
	))
	b.WriteByte('\n')

	// Line 4: per-model breakdown with hash-based colours.
	b.WriteString("[white::b]Models:  ")
	for i, m := range models {
		if i > 0 {
			b.WriteString("  ")
		}
		s := a.agg.ForModel(m)
		tag := tviewModelTag(m)
		b.WriteString(fmt.Sprintf("[%s]%s[white]([lime]%d[white])", tag, m, s.Requests))
	}

	return b.String()
}

// showFinalSummary writes the post-run summary table into the log view.
func (a *App) showFinalSummary() {
	models := a.agg.Models()
	if len(models) == 0 {
		fmt.Fprint(a.logView, "\n[yellow]No streaming completed data recorded.\n")
		return
	}

	tt := a.agg.Total()

	const (
		wModel = 24
		wReq   = 8
		wDur   = 11
		wNum   = 12
		wAbbr  = 10
	)

	var b strings.Builder
	line := func(fmtStr string, args ...interface{}) {
		b.WriteString(fmt.Sprintf(fmtStr, args...))
	}

	b.WriteString("\n")
	line("[white::b]─────────────────────────────────────────────────────────────────────\n")
	line("[white::b]  Streaming Completed Summary (by model)\n\n")
	line("  [cyan::b]%-*s[white::b] %*s %*s %*s %*s %*s %*s %*s %*s\n",
		wModel, "Model",
		wReq, "Req",
		wDur, "Total",
		wDur, "Avg",
		wDur, "Min",
		wDur, "Max",
		wNum, "OutTok",
		wAbbr, "CacheRd",
		wAbbr, "CacheCr",
	)
	line("  [gray::d]%s\n", strings.Repeat("─", 130))

	for _, m := range models {
		s := a.agg.ForModel(m)
		line("  [yellow]%-*s[white] %*d %*s %*s %*s %*s %*d %*s %*s\n",
			wModel, truncate(m, wModel),
			wReq, s.Requests,
			wDur, fmtDuration(s.TotalLatency),
			wDur, fmtDuration(s.AvgLatency()),
			wDur, fmtDuration(s.MinLatency),
			wDur, fmtDuration(s.MaxLatency),
			wNum, s.TotalOutputTokens,
			wAbbr, abbreviate(s.TotalCacheReadTokens),
			wAbbr, abbreviate(s.TotalCacheCreateTokens),
		)
	}

	line("  [gray::d]%s\n", strings.Repeat("─", 130))
	line("  [yellow]%-*s[white] %*d %*s %*s %*s %*s %*d %*s %*s\n",
		wModel, "TOTAL",
		wReq, tt.Requests,
		wDur, fmtDuration(tt.TotalLatency),
		wDur, fmtDuration(tt.AvgLatency()),
		wDur, fmtDuration(tt.MinLatency),
		wDur, fmtDuration(tt.MaxLatency),
		wNum, tt.TotalOutputTokens,
		wAbbr, abbreviate(tt.TotalCacheReadTokens),
		wAbbr, abbreviate(tt.TotalCacheCreateTokens),
	)

	fmt.Fprint(a.logView, tview.Escape(b.String()))
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func fmtDuration(d time.Duration) string {
	if d == 0 {
		return "0"
	}
	switch {
	case d >= time.Minute:
		return fmt.Sprintf("%dm%ds", d/time.Minute, (d%time.Minute)/time.Second)
	case d >= time.Second:
		return fmt.Sprintf("%.1fs", float64(d)/float64(time.Second))
	case d >= time.Millisecond:
		return fmt.Sprintf("%dms", d/time.Millisecond)
	default:
		return fmt.Sprintf("%dµs", d/time.Microsecond)
	}
}

func abbreviate(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// tviewModelTag returns a tview colour tag for a model name, deterministically
// chosen from a palette that mirrors output.hashPalette.  The colours are
// distinct and work well on a dark-cyan background.
func tviewModelTag(model string) string {
	palette := []string{
		"green", "#00ffff", "blue", "fuchsia",
		"lime", "yellow", "#ffaf00", "#ff8700",
	}
	h := fnv.New32a()
	h.Write([]byte(model))
	return palette[h.Sum32()%uint32(len(palette))]
}
