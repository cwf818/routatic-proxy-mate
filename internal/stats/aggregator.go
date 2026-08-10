package stats

import (
	"math"
	"sort"
	"sync"
	"time"
)

// StreamingStats holds per-model aggregated statistics for "streaming completed"
// entries.
type StreamingStats struct {
	Requests              int
	Attempts              int           // number of "attempting streaming model" events
	TotalLatency          time.Duration
	MinLatency            time.Duration
	MaxLatency            time.Duration
	TotalOutputTokens     int64
	TotalInputTokens      int64
	TotalCacheReadTokens  int64
	TotalCacheCreateTokens int64
	TotalOutSpeed         float64  // sum of output_tokens/latency_sec per request
	CurrentOutSpeed       float64  // last-recorded speed
	MaxOutSpeed           float64
	MinOutSpeed           float64
}

// Clone returns a deep copy.
func (s *StreamingStats) Clone() *StreamingStats {
	if s == nil {
		return nil
	}
	c := *s
	return &c
}

// AvgLatency returns the average latency, or 0 if no requests.
func (s *StreamingStats) AvgLatency() time.Duration {
	if s.Requests == 0 {
		return 0
	}
	return time.Duration(s.TotalLatency.Nanoseconds() / int64(s.Requests))
}

// AvgOutputTokens returns the average output token count.
func (s *StreamingStats) AvgOutputTokens() float64 {
	if s.Requests == 0 {
		return 0
	}
	return float64(s.TotalOutputTokens) / float64(s.Requests)
}

// AvgOutSpeed returns the average output speed (tokens/s).
func (s *StreamingStats) AvgOutSpeed() float64 {
	if s.Requests == 0 {
		return 0
	}
	return s.TotalOutSpeed / float64(s.Requests)
}

// Aggregator collects streaming-completed events.
//
// The aggregator is shared between the stdin-reader goroutine (which calls
// Record/RecordAttempt) and the TUI event-loop goroutine (which calls
// Models/Total/ForModel/Today to build the stats bar).  All access is
// guarded by mu so the two goroutines never touch the maps concurrently.
type Aggregator struct {
	mu     sync.Mutex
	models map[string]*StreamingStats
	total  StreamingStats
	days   map[string]*StreamingStats // key: "2006-01-02"
}

// New creates a new Aggregator.
func New() *Aggregator {
	return &Aggregator{
		models: make(map[string]*StreamingStats),
		days:   make(map[string]*StreamingStats),
		total: StreamingStats{
			MinLatency:  math.MaxInt64,
			MinOutSpeed: math.MaxFloat64,
		},
	}
}

// RecordAttempt records an "attempting streaming model" event for the given
// model.  It initialises the per-model entry if needed.
func (a *Aggregator) RecordAttempt(model string) {
	if model == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.models[model]
	if s == nil {
		s = &StreamingStats{
			MinLatency:  math.MaxInt64,
			MinOutSpeed: math.MaxFloat64,
		}
		a.models[model] = s
	}
	s.Attempts++
	a.total.Attempts++
}

// Record records a streaming completed event.
func (a *Aggregator) Record(model, latencyStr, inputTokensStr, outputTokensStr,
	cacheReadStr, cacheCreateStr string, t time.Time) {

	latency, _ := time.ParseDuration(latencyStr)
	inputTokens := parseInt(inputTokensStr)
	outputTokens := parseInt(outputTokensStr)
	cacheRead := parseInt(cacheReadStr)
	cacheCreate := parseInt(cacheCreateStr)

	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.models[model]
	if s == nil {
		s = &StreamingStats{
			MinLatency:  math.MaxInt64,
			MinOutSpeed: math.MaxFloat64,
		}
		a.models[model] = s
	}

	s.Requests++
	s.TotalLatency += latency
	if latency < s.MinLatency {
		s.MinLatency = latency
	}
	if latency > s.MaxLatency {
		s.MaxLatency = latency
	}
	s.TotalInputTokens += inputTokens
	s.TotalOutputTokens += outputTokens
	s.TotalCacheReadTokens += cacheRead
	s.TotalCacheCreateTokens += cacheCreate

	// OutSpeed = output_tokens / latency_seconds
	latencySec := latency.Seconds()
	if latencySec > 0 {
		speed := float64(outputTokens) / latencySec
		s.CurrentOutSpeed = speed
		s.TotalOutSpeed += speed
		if speed > s.MaxOutSpeed {
			s.MaxOutSpeed = speed
		}
		if speed < s.MinOutSpeed {
			s.MinOutSpeed = speed
		}
	}

	// Update totals
	a.total.Requests++
	a.total.TotalLatency += latency
	if latency < a.total.MinLatency {
		a.total.MinLatency = latency
	}
	if latency > a.total.MaxLatency {
		a.total.MaxLatency = latency
	}
	a.total.TotalInputTokens += inputTokens
	a.total.TotalOutputTokens += outputTokens
	a.total.TotalCacheReadTokens += cacheRead
	a.total.TotalCacheCreateTokens += cacheCreate
	if latencySec > 0 {
		speed := float64(outputTokens) / latencySec
		a.total.CurrentOutSpeed = speed
		a.total.TotalOutSpeed += speed
		if speed > a.total.MaxOutSpeed {
			a.total.MaxOutSpeed = speed
		}
		if speed < a.total.MinOutSpeed {
			a.total.MinOutSpeed = speed
		}
	}

	// Record daily stats.
	if !t.IsZero() {
		dateKey := t.Format("2006-01-02")
		d, ok := a.days[dateKey]
		if !ok {
			d = &StreamingStats{}
			a.days[dateKey] = d
		}
		d.Requests++
		d.TotalInputTokens += inputTokens
		d.TotalOutputTokens += outputTokens
		d.TotalCacheReadTokens += cacheRead
		d.TotalCacheCreateTokens += cacheCreate
	}
}

// Models returns a sorted list of model names.
func (a *Aggregator) Models() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	names := make([]string, 0, len(a.models))
	for n := range a.models {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ForModel returns stats for a specific model.
func (a *Aggregator) ForModel(model string) *StreamingStats {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.models[model].Clone()
}

// Total returns the aggregate stats across all models.
func (a *Aggregator) Total() *StreamingStats {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.total.Clone()
}

// Today returns the stats for the current calendar day.  Returns an empty
// StreamingStats when no data has been recorded for today.
func (a *Aggregator) Today() *StreamingStats {
	dateKey := time.Now().Format("2006-01-02")
	return a.Daily(dateKey)
}

// Daily returns the stats for the given date (format "2006-01-02").
func (a *Aggregator) Daily(date string) *StreamingStats {
	a.mu.Lock()
	defer a.mu.Unlock()
	if s, ok := a.days[date]; ok {
		return s.Clone()
	}
	return &StreamingStats{}
}

func parseInt(s string) int64 {
	if s == "" {
		return 0
	}
	var n int64
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int64(c-'0')
		}
	}
	return n
}
