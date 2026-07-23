package output

import (
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"routatic-proxy-mate/internal/parser"
	"routatic-proxy-mate/internal/stats"
)

// ANSI color codes
const (
	Reset     = "\033[0m"
	Bold      = "\033[1m"
	Dim       = "\033[2m"
	Red       = "\033[31m"
	Green     = "\033[32m"
	Yellow    = "\033[33m"
	Blue      = "\033[34m"
	Magenta   = "\033[35m"
	Cyan      = "\033[38;5;51m" // bright pure cyan
	White     = "\033[37m"
	RedBold   = Red + Bold
	GreenDim  = Green + Dim
	YellowDim = Yellow + Dim
	CyanBold  = Cyan + Bold
)

// keyColor returns the ANSI color for a key name.  Only time and level get a
// fixed Dim color; everything else gets a deterministic hash-based color.
func keyColor(key string) string {
	if key == "time" || key == "level" {
		return Dim
	}
	return hashColor(key)
}

// valueColor returns the ANSI color for a value, based on key and content.
// Only level and latency get fixed semantic colors; numeric values get Cyan/White;
// time values get Dim; everything else defaults to a deterministic hash-based color.
func valueColor(key, value string) string {
	// 1. Level — semantic color.
	if key == "level" {
		switch value {
		case "WARN":
			return Yellow
		case "ERROR":
			return RedBold
		default:
			return Reset
		}
	}

	// 2. Time — dim, not hash-colored.
	if key == "time" {
		return Dim
	}

	// 3. Latency — fixed green.
	if key == "latency" {
		return Green
	}

	// 4. Numeric values — cyan for non-zero, white for zero.
	if isNumeric(value) {
		if value == "0" {
			return White
		}
		return Cyan
	}

	// 5. Default — hash-based deterministic color.
	return hashColor(value)
}

// isNumeric reports whether s consists entirely of decimal digits.
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// hashPalette is a set of distinct terminal-friendly colors used for
// hash-based value coloring (model names, provider names, etc.).
var hashPalette = []string{
	Green,
	Cyan,       // bright pure cyan
	"\033[94m", // bright blue
	"\033[95m", // bright magenta
	"\033[92m", // bright green
	Yellow,
	"\033[38;5;214m", // orange
	"\033[38;5;208m", // bright orange (was brown)
}

// hashColor returns a deterministic ANSI color for a string value using
// the FNV-1a hash.  The same input always produces the same color.
func hashColor(s string) string {
	h := fnv.New32a()
	h.Write([]byte(s))
	return hashPalette[h.Sum32()%uint32(len(hashPalette))]
}

// eqColor returns the color for the '=' separator.
func eqColor() string { return Dim }

// ColorizeRawLine returns the raw log line with separate ANSI color applied
// to the key, the '=', and the value.  When noColor is true the raw line
// is returned verbatim.
func ColorizeRawLine(raw string, noColor bool) string {
	if noColor || raw == "" {
		return raw
	}

	spans := parser.Spans(raw)

	var b strings.Builder
	b.Grow(len(raw) + len(spans)*18)

	pos := 0
	for _, sp := range spans {
		// Text before this span
		if sp.Start > pos {
			b.WriteString(raw[pos:sp.Start])
		}

		// Key
		kc := keyColor(sp.Key)
		b.WriteString(kc)
		b.WriteString(raw[sp.Start:sp.EqPos])
		b.WriteString(Reset)

		// =
		b.WriteString(eqColor())
		b.WriteByte('=')
		b.WriteString(Reset)

		// Value
		valStart := sp.EqPos + 1
		vc := valueColor(sp.Key, sp.Value)
		b.WriteString(vc)
		b.WriteString(raw[valStart:sp.End])
		b.WriteString(Reset)

		pos = sp.End
	}
	// Trailing text
	if pos < len(raw) {
		b.WriteString(raw[pos:])
	}
	return b.String()
}

// ColorizeFallback applies a dim color to an unparseable / startup line.
func ColorizeFallback(line string, noColor bool) string {
	if noColor || line == "" {
		return line
	}
	return White + Dim + line + Reset
}

// visualLen returns the visual cell width of s for terminal display.
// This counts runes (not bytes); all characters we use (∅↑↓…─µ) are
// single-width, so the rune count equals the display width.
func visualLen(s string) int { return len([]rune(s)) }

// WriteSummary writes the per-model streaming-completed summary table to
// stdout.  Columns are fixed-width with manual padding (ANSI-safe).
func WriteSummary(a *stats.Aggregator, noColor bool) {
	models := a.Models()
	if len(models) == 0 {
		return
	}

	colorCell := func(s, c string, width int) string {
		if c == "" || noColor {
			return s + strings.Repeat(" ", width-visualLen(s))
		}
		return c + s + Reset + strings.Repeat(" ", width-visualLen(s))
	}
	plain := func(s string, width int) string {
		return s + strings.Repeat(" ", width-visualLen(s))
	}
	header := func(s string, width int) string { return colorCell(s, Cyan+Bold, width) }
	val := func(s string, width int) string   { return colorCell(s, Green, width) }
	lbl := func(s string, width int) string   { return colorCell(s, Yellow, width) }

	const (
		wModel = 24
		wReq   = 8
		wDur   = 11
		wNum   = 12
		wAbbr  = 10
		wPct   = 9
		wSpd   = 10
	)

	fmt.Println()
	fmt.Println(colorCell("Streaming Completed Summary (by model)", Cyan+Bold, 80))

	var sb strings.Builder

	// Header
	sb.WriteString("  ")
	sb.WriteString(header("Model", wModel))
	sb.WriteString(header("Req", wReq))
	sb.WriteString(header("Total", wDur))
	sb.WriteString(header("Avg", wDur))
	sb.WriteString(header("OutTok", wNum))
	sb.WriteString(header("CacheRd", wAbbr))
	sb.WriteString(header("CacheCr", wAbbr))
	sb.WriteString(header("CacheHit", wPct))
	sb.WriteString(header("SpdAvg", wSpd))
	sb.WriteString(header("SpdMax", wSpd))
	sb.WriteString(header("SpdMin", wSpd))
	sb.WriteByte('\n')

	// Separator
	sep := "  " + strings.Repeat("─", 135)
	if !noColor {
		sep = White + Dim + sep + Reset
	}
	sb.WriteString(sep)
	sb.WriteByte('\n')

	// Data rows
	for _, m := range models {
		s := a.ForModel(m)
		sb.WriteString("  ")
		sb.WriteString(lbl(trunc(m, wModel), wModel))
		sb.WriteString(plain(fmt.Sprintf("%d", s.Requests), wReq))
		sb.WriteString(val(fmtDur(s.TotalLatency), wDur))
		sb.WriteString(val(fmtDur(s.AvgLatency()), wDur))
		sb.WriteString(plain(fmt.Sprintf("%d", s.TotalOutputTokens), wNum))
		sb.WriteString(hashAbbr(s.TotalCacheReadTokens, wAbbr, noColor))
		sb.WriteString(hashAbbr(s.TotalCacheCreateTokens, wAbbr, noColor))
		sb.WriteString(plain(fmtPct(cacheHitRate(s.TotalCacheReadTokens, s.TotalCacheCreateTokens)), wPct))
		sb.WriteString(colorCell(fmtSpeed(s.AvgOutSpeed()), speedBandColor(s.AvgOutSpeed()), wSpd))
		sb.WriteString(val(fmtSpeed(s.MaxOutSpeed), wSpd))
		sb.WriteString(val(fmtSpeed(s.MinOutSpeed), wSpd))
		sb.WriteByte('\n')
	}

	// Total row
	tt := a.Total()
	sb.WriteString(sep)
	sb.WriteByte('\n')
	sb.WriteString("  ")
	sb.WriteString(lbl("TOTAL", wModel))
	sb.WriteString(plain(fmt.Sprintf("%d", tt.Requests), wReq))
	sb.WriteString(val(fmtDur(tt.TotalLatency), wDur))
	sb.WriteString(val(fmtDur(tt.AvgLatency()), wDur))
	sb.WriteString(plain(fmt.Sprintf("%d", tt.TotalOutputTokens), wNum))
	sb.WriteString(hashAbbr(tt.TotalCacheReadTokens, wAbbr, noColor))
	sb.WriteString(hashAbbr(tt.TotalCacheCreateTokens, wAbbr, noColor))
	sb.WriteString(plain(fmtPct(cacheHitRate(tt.TotalCacheReadTokens, tt.TotalCacheCreateTokens)), wPct))
	sb.WriteString(colorCell(fmtSpeed(tt.AvgOutSpeed()), speedBandColor(tt.AvgOutSpeed()), wSpd))
	sb.WriteString(val(fmtSpeed(tt.MaxOutSpeed), wSpd))
	sb.WriteString(val(fmtSpeed(tt.MinOutSpeed), wSpd))
	sb.WriteByte('\n')

	fmt.Print(sb.String())
}

// hashAbbr is like abbr() but accepts noColor and width for padding.
func hashAbbr(n int64, width int, noColor bool) string {
	s := abbr(n)
	pad := width - visualLen(s)
	if pad < 0 {
		pad = 0
	}
	if noColor {
		return s + strings.Repeat(" ", pad)
	}
	return hashColor(s) + s + Reset + strings.Repeat(" ", pad)
}

// fmtDuration is like fmtDur but exported for use by TUI.
func fmtDur(d time.Duration) string {
	if d == 0 {
		return "0"
	}
	switch {
	case d >= 60_000_000_000:
		return fmt.Sprintf("%dm%ds", d/60_000_000_000, (d%60_000_000_000)/1_000_000_000)
	case d >= 1_000_000_000:
		return fmt.Sprintf("%.1fs", float64(d)/1_000_000_000)
	case d >= 1_000_000:
		return fmt.Sprintf("%dms", d/1_000_000)
	default:
		return fmt.Sprintf("%dµs", d/1000)
	}
}

func abbr(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// fmtSpeed formats an OutSpeed value (tokens/s) for table display.
func fmtSpeed(speed float64) string {
	return fmt.Sprintf("%.1f/s", speed)
}

// fmtPct formats a percentage value for table display.
func fmtPct(pct float64) string {
	return fmt.Sprintf("%.1f%%", pct)
}

// speedBandColor returns the ANSI colour for a speed value using a
// 4-band scheme: ≤20 red, ≤40 orange (#ff8700 as ANSI 256-colour),
// ≤60 yellow, >60 green.
func speedBandColor(speed float64) string {
	switch {
	case speed <= 20:
		return Red
	case speed <= 40:
		return "[38;5;208m" // orange
	case speed <= 60:
		return Yellow
	default:
		return Green
	}
}

// cacheHitRate returns the cache hit percentage:
//
//	cacheRd / (cacheRd + cacheCr) * 100
func cacheHitRate(cacheRd, cacheCr int64) float64 {
	total := cacheRd + cacheCr
	if total == 0 {
		return 0
	}
	return float64(cacheRd) / float64(total) * 100
}
