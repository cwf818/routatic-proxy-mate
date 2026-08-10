package tui

import (
	"strings"
	"testing"
	"time"

	"routatic-proxy-mate/internal/stats"
)

// TestRecordLevelIsCaseInsensitive verifies that recordLevel normalises the
// level to upper case, so info and INFO share a single bucket and lowercase
// levels never appear as a separate "exception" in the summary bar.
func TestRecordLevelIsCaseInsensitive(t *testing.T) {
	a := New(stats.New(), true, nil, "test", time.Now())
	a.recordLevel("info")
	a.recordLevel("INFO")
	a.recordLevel("info")

	a.levelMu.Lock()
	defer a.levelMu.Unlock()
	if got := a.levelCounts["INFO"]; got != 3 {
		t.Fatalf("INFO count = %d, want 3", got)
	}
	if _, ok := a.levelCounts["info"]; ok {
		t.Fatal("lowercase 'info' key should not exist")
	}
}

// TestBuildStatsTextLevelColors verifies the summary bar renders each non-INFO
// level with a unified colour for label and count: ERROR red, others yellow.
func TestBuildStatsTextLevelColors(t *testing.T) {
	a := New(stats.New(), true, nil, "test", time.Now())
	// Seed one model so buildStatsText renders the full stats bar.
	a.agg.Record("model-a", "1s", "10", "100", "1000", "50", time.Now())
	a.levelMu.Lock()
	a.levelCounts["ERROR"] = 44
	a.levelCounts["WARN"] = 33
	a.levelCounts["INFO"] = 100
	a.levelMu.Unlock()

	got := a.buildStatsText()
	if !strings.Contains(got, "[red]ERROR[white]: [red]44") {
		t.Errorf("expected red ERROR label+count, got %q", got)
	}
	if !strings.Contains(got, "[yellow]WARN[white]: [yellow]33") {
		t.Errorf("expected yellow WARN label+count, got %q", got)
	}
	if strings.Contains(got, "info") {
		t.Errorf("lowercase 'info' should not appear in the summary bar, got %q", got)
	}
}
