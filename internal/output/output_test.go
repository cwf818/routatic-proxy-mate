package output

import "testing"

// TestValueColorLevelCaseInsensitive verifies that the level value color is
// matched case-insensitively, so error/ERROR render red and warn/WARN yellow.
func TestValueColorLevelCaseInsensitive(t *testing.T) {
	cases := []struct {
		value string
		want  string
	}{
		{"ERROR", RedBold},
		{"error", RedBold},
		{"WARN", Yellow},
		{"warn", Yellow},
		{"INFO", Reset},
		{"info", Reset},
	}
	for _, c := range cases {
		if got := valueColor("level", c.value); got != c.want {
			t.Errorf("valueColor(level, %q) = %q, want %q", c.value, got, c.want)
		}
	}
}
