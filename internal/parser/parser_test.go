package parser

import "testing"

func TestParseReasonField(t *testing.T) {
	line := `time=2026-08-10T00:00:32.861+08:00 level=INFO msg="model skipped by capacity filter" model=deepseek-v4-pro reason=vision_not_supported`
	e, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.Fields["model"]; got != "deepseek-v4-pro" {
		t.Errorf("model = %q, want %q", got, "deepseek-v4-pro")
	}
	if got := e.Fields["reason"]; got != "vision_not_supported" {
		t.Errorf("reason = %q, want %q", got, "vision_not_supported")
	}
}

// TestParseConcatenatedKeys guards the disambiguation of concatenated
// key=value pairs such as latency=3.1sinput_tokens=0.
func TestParseConcatenatedKeys(t *testing.T) {
	line := `time=2026-08-10T00:00:32.861+08:00 level=INFO msg="streaming completed" model=deepseek-v4 latency=3.189401sinput_tokens=0`
	e, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.Fields["latency"]; got != "3.189401s" {
		t.Errorf("latency = %q, want %q", got, "3.189401s")
	}
	if got := e.Fields["input_tokens"]; got != "0" {
		t.Errorf("input_tokens = %q, want %q", got, "0")
	}
}
