package tui

import (
	"strings"
	"testing"
	"time"

	"routatic-proxy-mate/internal/stats"
)

// TestClearLogView verifies that clearLogView empties the log buffer and
// appends the exact clear hint line. It also pins the constraint that only
// the log view is cleared: the aggregator is not reset and the stats bar is
// still rendered after the clear.
func TestClearLogView(t *testing.T) {
	a := New(stats.New(), true, nil, "test")
	a.logView.SetText("line one\nline two\n")

	// Seed the aggregator so a streaming-completed entry exists and the
	// stats bar would render. The assertions below pin the "only the log
	// view is cleared" constraint: aggregation survives and the stats bar
	// is still rendered, untouched by clearLogView.
	a.agg.Record("model-a", "1s", "10", "100", "1000", "50", time.Now())

	a.clearLogView()

	got := a.logView.GetText(false)
	if got != "[gray]—— screen cleared ——\n" {
		t.Fatalf("expected exact clear hint, got %q", got)
	}
	if strings.Contains(got, "line one") {
		t.Fatalf("expected old lines to be cleared, got %q", got)
	}

	// The stats bar and Aggregator must be untouched: aggregation is not
	// reset, and the stats bar is still rendered (not cleared).
	if got := a.agg.Total().Requests; got != 1 {
		t.Fatalf("expected aggregator to keep 1 request after clear, got %d", got)
	}
	if got := a.statsView.GetText(false); !strings.Contains(got, "Streaming Completed") {
		t.Fatalf("expected stats bar to remain rendered after clear, got %q", got)
	}
}
