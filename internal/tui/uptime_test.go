package tui

import (
	"strings"
	"testing"
	"time"

	"routatic-proxy-mate/internal/stats"
)

// TestBuildStatsTextShowsUpTime verifies the stats bar shows UpTime right
// after ApiTime, formatted as a dhms duration since the app started.
func TestBuildStatsTextShowsUpTime(t *testing.T) {
	a := New(stats.New(), true, nil, "test", time.Now())
	// Seed one model so buildStatsText renders the full stats bar (it
	// early-returns a "waiting" message when no models exist).
	a.agg.Record("model-a", "1s", "10", "100", "1000", "50", time.Now())
	// Pin the start time 2 hours in the past so the UpTime is deterministic.
	a.started = time.Now().Add(-2 * time.Hour)

	got := a.buildStatsText()
	// Note: the tview colour tag [lime] sits between the label and value, so
	// the assertion matches the rendered output "UpTime: [lime]2h".
	if !strings.Contains(got, "UpTime: [lime]2h") {
		t.Fatalf("expected UpTime after ApiTime, got %q", got)
	}
	if strings.Index(got, "ApiTime") > strings.Index(got, "UpTime") {
		t.Fatalf("expected UpTime to follow ApiTime, got %q", got)
	}
}
