package stats

import (
	"math"
	"sort"
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
type Aggregator struct {
	models map[string]*StreamingStats
	total  StreamingStats
}

// New creates a new Aggregator.
func New() *Aggregator {
	return &Aggregator{
		models: make(map[string]*StreamingStats),
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
	cacheReadStr, cacheCreateStr string) {

	latency, _ := time.ParseDuration(latencyStr)
	inputTokens := parseInt(inputTokensStr)
	outputTokens := parseInt(outputTokensStr)
	cacheRead := parseInt(cacheReadStr)
	cacheCreate := parseInt(cacheCreateStr)

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
}

// Models returns a sorted list of model names.
func (a *Aggregator) Models() []string {
	names := make([]string, 0, len(a.models))
	for n := range a.models {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ForModel returns stats for a specific model.
func (a *Aggregator) ForModel(model string) *StreamingStats {
	return a.models[model].Clone()
}

// Total returns the aggregate stats across all models.
func (a *Aggregator) Total() *StreamingStats {
	return a.total.Clone()
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
