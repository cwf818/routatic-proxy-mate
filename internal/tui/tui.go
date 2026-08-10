package tui

import (
	"bufio"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
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
	filter    *output.ColorFilter

	statsShown bool
	done       bool

	// levelCounts tracks all log levels seen (key=level string, value=count).
	// Protected by levelMu for concurrent access from reader / event goroutines.
	levelMu     sync.Mutex
	levelCounts map[string]int64

	// The main event loop signals this channel when Run() returns.
	exited chan struct{}
}

// New creates a new TUI application.  The aggregator is read periodically to
// build the stats bar. version is displayed in the log view title.
func New(agg *stats.Aggregator, noColor bool, filter *output.ColorFilter, version string) *App {
	a := &App{
		Application: tview.NewApplication(),
		agg:         agg,
		noColor:     noColor,
		filter:      filter,
		exited:      make(chan struct{}),
		levelCounts: make(map[string]int64),
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
	a.logView.SetMaxLines(3000)
	a.logView.SetBorder(true)
	a.logView.SetTitle(" cwf818/routatic-proxy-mate " + version + " ")
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

	// Forward OS signals to a graceful Stop so tcell.Fini() restores the
	// console modes.  Without this, a SIGTERM (e.g. from killing the process
	// or the terminal tab closing on Windows) would terminate the process
	// while the console is still in the alternate-screen buffer / raw mode,
	// leaving the terminal looking dead.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		select {
		case <-sigCh:
			a.Stop()
		case <-a.exited:
		}
	}()

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
	// A panic in this goroutine would crash the whole process while the
	// console is still in the alternate-screen buffer / raw mode, leaving
	// the terminal looking dead.  Recover so a single bad line can't take
	// the app (and the console) down.
	defer func() {
		if r := recover(); r != nil {
			a.QueueUpdateDraw(func() {
				fmt.Fprintf(a.logView, "\n[red]reader panic: %v\n", r)
				a.done = true
			})
			time.Sleep(2 * time.Second)
			a.Stop()
		}
	}()

	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 64*1024), 256*1024)

	for scanner.Scan() {
		line := scanner.Text()

		entry, err := parser.ParseLine(line)
		if err != nil || entry == nil {
			if line != "" {
				colored := output.ColorizeFallback(line, a.noColor, a.filter)
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
				entry.Time,
			)
		}


			// Record attempting streaming model stats.
			if parser.ClassifyMessage(entry.Message) == parser.MsgAttemptingStreaming {
				a.agg.RecordAttempt(entry.Fields["model"])
			}
		// Track per-level log counts for the summary bar.
		if entry.Level != "" {
			a.levelMu.Lock()
			a.levelCounts[entry.Level]++
			a.levelMu.Unlock()
		}

		colored := output.ColorizeRawLine(line, a.noColor, a.filter)
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
	today := a.agg.Today()

	var b strings.Builder

	// Line 1: today/cumulative pairs for counts and token sums.
	b.WriteString(fmt.Sprintf(
		"[white::b]Streaming Completed: [lime]%s/%s  "+
			"[white]|  In: [lime]%s/%s  "+
			"[white]|  Out: [lime]%s/%s  "+
			"[white]|  Cache Rd: [lime]%s/%s  "+
			"[white]|  Cache Cr: [lime]%s/%s",
		abbreviate(int64(today.Requests)), abbreviate(int64(tt.Requests)),
		abbreviate(today.TotalInputTokens), abbreviate(tt.TotalInputTokens),
		abbreviate(today.TotalOutputTokens), abbreviate(tt.TotalOutputTokens),
		abbreviate(today.TotalCacheReadTokens), abbreviate(tt.TotalCacheReadTokens),
		abbreviate(today.TotalCacheCreateTokens), abbreviate(tt.TotalCacheCreateTokens),
	))
	b.WriteByte('\n')

	// Line 2: OutSpeed with band colours for Current and Avg + Total at end.
	b.WriteString(fmt.Sprintf(
		"[white::b]OutSpeed: Current: [%s]%.1f/s  "+
			"[white]|  Avg: [%s]%.1f/s  "+
			"[white]|  Max: [lime]%.1f/s  "+
			"[white]|  Min: [lime]%.1f/s  "+
			"[white]|  ApiTime: [lime]%s",
		speedTag(tt.CurrentOutSpeed), tt.CurrentOutSpeed,
		speedTag(tt.AvgOutSpeed()), tt.AvgOutSpeed(),
		tt.MaxOutSpeed,
		tt.MinOutSpeed,
		fmtDuration(tt.TotalLatency),
	))
	b.WriteByte('\n')

	// Line 3: averages, cache hit rate and non-INFO level counts.
	chToday := cacheHitRate(today.TotalCacheReadTokens, today.TotalCacheCreateTokens)
	chTotal := cacheHitRate(tt.TotalCacheReadTokens, tt.TotalCacheCreateTokens)
	// Gather non-INFO level counts.
	a.levelMu.Lock()
	var levelPairs []string
	for lv, cnt := range a.levelCounts {
		if lv != "INFO" {
			levelPairs = append(levelPairs, fmt.Sprintf("[yellow]%s[white]: [lime]%d", lv, cnt))
		}
	}
	a.levelMu.Unlock()
	var levelStr string
	if len(levelPairs) > 0 {
		sort.Strings(levelPairs)
		levelStr = "  [white]|  " + strings.Join(levelPairs, "  [white]|  ")
	}
	b.WriteString(fmt.Sprintf(
		"[white::b]Avg Latency: [lime]%s  "+
			"[white]|  Avg Out/Req: [lime]%s  "+
			"[white]|  Hit: [%s]%.1f%%/%.1f%%"+
			"%s",
		fmtDuration(tt.AvgLatency()),
		abbreviate(int64(float64(tt.TotalOutputTokens)/float64(tt.Requests))),
		cacheHitTag(chTotal), chToday, chTotal,
		levelStr,
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
		b.WriteString(fmt.Sprintf("[%s]%s[white]([lime]%d/%d[white])", tag, m, s.Requests, s.Attempts))
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
		wReq   = 13
		wDur   = 11
		wNum   = 12
		wAbbr  = 10
		wPct   = 9
		wSpd   = 10
	)

	var b strings.Builder
	line := func(fmtStr string, args ...interface{}) {
		b.WriteString(fmt.Sprintf(fmtStr, args...))
	}

	b.WriteString("\n")
	line("[white::b]─────────────────────────────────────────────────────────────────────────────────────\n")
	line("[white::b]  Streaming Completed Summary (by model)\n\n")
	line("  [cyan::b]%-*s[white::b] %*s %*s %*s %*s %*s %*s %*s %*s %*s %*s\n",
		wModel, "Model",
		wReq, "OK/Att",
		wDur, "ApiTime",
		wDur, "Avg",
		wNum, "OutTok",
		wAbbr, "CacheRd",
		wAbbr, "CacheCr",
		wPct, "CacheHit",
		wSpd, "SpdAvg",
		wSpd, "SpdMax",
		wSpd, "SpdMin",
	)
	line("  [gray::d]%s\n", strings.Repeat("─", 145))

	for _, m := range models {
		s := a.agg.ForModel(m)
		ch := cacheHitRate(s.TotalCacheReadTokens, s.TotalCacheCreateTokens)
		line("  [yellow]%-*s[white] %*s %*s %*s %*d %*s %*s %*s [%s]%*s[white] [%s]%*s[white] [%s]%*s[white]\n",
			wModel, truncate(m, wModel),
			wReq, fmt.Sprintf("%d/%d", s.Requests, s.Attempts),
			wDur, fmtDuration(s.TotalLatency),
			wDur, fmtDuration(s.AvgLatency()),
			wNum, s.TotalOutputTokens,
			wAbbr, abbreviate(s.TotalCacheReadTokens),
			wAbbr, abbreviate(s.TotalCacheCreateTokens),
			wPct, fmtPct(ch),
			speedTag(saneSpeed(s.AvgOutSpeed())), wSpd, fmtSpeed(saneSpeed(s.AvgOutSpeed())),
			speedTag(saneSpeed(s.MaxOutSpeed)), wSpd, fmtSpeed(saneSpeed(s.MaxOutSpeed)),
			speedTag(saneSpeed(s.MinOutSpeed)), wSpd, fmtSpeed(saneSpeed(s.MinOutSpeed)),
		)
	}

	line("  [gray::d]%s\n", strings.Repeat("─", 145))
	ch := cacheHitRate(tt.TotalCacheReadTokens, tt.TotalCacheCreateTokens)
	line("  [yellow]%-*s[white] %*s %*s %*s %*d %*s %*s %*s [%s]%*s[white] [%s]%*s[white] [%s]%*s[white]\n",
		wModel, "TOTAL",
		wReq, fmt.Sprintf("%d/%d", tt.Requests, tt.Attempts),
		wDur, fmtDuration(tt.TotalLatency),
		wDur, fmtDuration(tt.AvgLatency()),
		wNum, tt.TotalOutputTokens,
		wAbbr, abbreviate(tt.TotalCacheReadTokens),
		wAbbr, abbreviate(tt.TotalCacheCreateTokens),
		wPct, fmtPct(ch),
		speedTag(saneSpeed(tt.AvgOutSpeed())), wSpd, fmtSpeed(saneSpeed(tt.AvgOutSpeed())),
		speedTag(saneSpeed(tt.MaxOutSpeed)), wSpd, fmtSpeed(saneSpeed(tt.MaxOutSpeed)),
		speedTag(saneSpeed(tt.MinOutSpeed)), wSpd, fmtSpeed(saneSpeed(tt.MinOutSpeed)),
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

// fmtSpeed formats an OutSpeed value (tokens/s) for table display.
func fmtSpeed(speed float64) string {
	return fmt.Sprintf("%.1f/s", speed)
}

// fmtPct formats a percentage value for table display.
func fmtPct(pct float64) string {
	return fmt.Sprintf("%.1f%%", pct)
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

// speedTag returns a tview colour tag for the given OutSpeed value using a
// 4-band scheme: ≤20 red, ≤40 orange, ≤60 yellow, >60 green.
func speedTag(speed float64) string {
	switch {
	case speed <= 20:
		return "red"
	case speed <= 40:
		return "#ff8700" // orange
	case speed <= 60:
		return "yellow"
	default:
		return "lime"
	}
}

// cacheHitRate returns the cache hit percentage:
//
//	cacheRd / (cacheRd + cacheCr) * 100
func saneSpeed(speed float64) float64 {
	if speed >= 1e200 {
		return 0
	}
	return speed
}

func cacheHitRate(cacheRd, cacheCr int64) float64 {
	total := cacheRd + cacheCr
	if total == 0 {
		return 0
	}
	return float64(cacheRd) / float64(total) * 100
}

// cacheHitTag returns a tview colour tag for a cache hit percentage using a
// 4-band scheme: ≤80 red, ≤90 orange, ≤95 yellow, >95 green.
func cacheHitTag(pct float64) string {
	switch {
	case pct <= 80:
		return "red"
	case pct <= 90:
		return "#ff8700"
	case pct <= 95:
		return "yellow"
	default:
		return "lime"
	}
}
