package stats

import (
	"sync"
	"testing"
	"time"
)

// TestAggregatorConcurrentAccess reproduces the TUI usage pattern where the
// stdin-reader goroutine calls Record/RecordAttempt while the tview event
// loop goroutine calls Models/Total/ForModel/Today.
//
// Run with: go test -race ./internal/stats/
func TestAggregatorConcurrentAccess(t *testing.T) {
	agg := New()

	var wg sync.WaitGroup
	wg.Add(2)

	// Writer goroutine (like tui.readStdin).
	go func() {
		defer wg.Done()
		for i := 0; i < 20000; i++ {
			agg.RecordAttempt("model-a")
			agg.Record("model-a", "1s", "10", "100", "1000", "50", time.Now())
		}
	}()

	// Reader goroutine (like the tview event loop building the stats bar).
	go func() {
		defer wg.Done()
		for i := 0; i < 20000; i++ {
			_ = agg.Models()
			_ = agg.Total()
			_ = agg.Today()
			_ = agg.ForModel("model-a")
		}
	}()

	wg.Wait()
}
