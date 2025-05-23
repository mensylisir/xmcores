// time_test.go
package time

import (
	"github.com/mensylisir/xmcores/common"
	"testing"
	"time"
)

func TestShortDur(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{"zero", 0, "0s"},
		{"1 second", 1 * time.Second, "1s"},
		{"59 seconds", 59 * time.Second, "59s"},
		{"1 minute 0 seconds", 1 * time.Minute, "1m"},
		{"1 minute 30 seconds", 1*time.Minute + 30*time.Second, "1m30s"},
		{"59 minutes 0 seconds", 59 * time.Minute, "59m"},
		{"1 hour 0 minutes 0 seconds", 1 * time.Hour, "1h"},
		{"1 hour 30 minutes 0 seconds", 1*time.Hour + 30*time.Minute, "1h30m"},
		{"1 hour 0 minutes 30 seconds", 1*time.Hour + 30*time.Second, "1h0m30s"}, // ShortDur does not omit 0m if seconds follow
		{"2 hours 5 minutes 10 seconds", 2*time.Hour + 5*time.Minute + 10*time.Second, "2h5m10s"},
		{"500 milliseconds", 500 * time.Millisecond, "500ms"},
		{"1 second 500 milliseconds", 1*time.Second + 500*time.Millisecond, "1.5s"}, // Standard time.Duration.String() behavior
		{"1 minute 1 second 500 milliseconds", 1*time.Minute + 1*time.Second + 500*time.Millisecond, "1m1.5s"},
		{"just under 1m (has ms)", 59*time.Second + 900*time.Millisecond, "59.9s"},
		{"just under 1h (has s)", 59*time.Minute + 59*time.Second, "59m59s"},
		{"negative 1 minute", -1 * time.Minute, "-1m"},
		{"negative 1 hour", -1 * time.Hour, "-1h"},
		{"negative 1h30m", -(1*time.Hour + 30*time.Minute), "-1h30m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = common.NanosPerSecond

			if got := ShortDur(tt.duration); got != tt.want {
				t.Errorf("ShortDur(%v) = %q, want %q (original: %q)", tt.duration, got, tt.want, tt.duration.String())
			}
		})
	}
}

func TestShortDurV2(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{"zero", 0, "0s"},
		{"1 nanosecond", 1 * time.Nanosecond, "1ns"},
		{"500 nanoseconds", 500 * time.Nanosecond, "500ns"},
		{"1 microsecond", 1 * time.Microsecond, "1µs"}, // Expect "1µs", not "1.000µs" for exact
		{"1 microsecond 50 ns", 1*time.Microsecond + 50*time.Nanosecond, "1.050µs"},
		{"500 microseconds", 500 * time.Microsecond, "500µs"},
		{"1 millisecond", 1 * time.Millisecond, "1ms"},
		{"1 millisecond 500 µs", 1*time.Millisecond + 500*time.Microsecond, "1.500ms"},
		{"500 milliseconds", 500 * time.Millisecond, "500ms"},
		{"1 second", 1 * time.Second, "1s"},
		{"1 second 5 ms", 1*time.Second + 5*time.Millisecond, "1.005s"},
		{"1 second 50 ms", 1*time.Second + 50*time.Millisecond, "1.050s"},
		{"1 second 500 ms", 1*time.Second + 500*time.Millisecond, "1.500s"},
		{"59 seconds", 59 * time.Second, "59s"},
		{"1 minute 0 seconds", 1 * time.Minute, "1m"},
		{"1 minute 30 seconds", 1*time.Minute + 30*time.Second, "1m30s"}, // Updated ShortDurV2 to output 30s not 30.000s
		{"1 minute 0.5 seconds", 1*time.Minute + 500*time.Millisecond, "1m0.500s"},
		{"59 minutes 0 seconds", 59 * time.Minute, "59m"},
		{"1 hour 0 minutes 0 seconds", 1 * time.Hour, "1h"},
		{"1 hour 30 minutes 0 seconds", 1*time.Hour + 30*time.Minute, "1h30m"},
		{"1 hour 0 minutes 30 seconds", 1*time.Hour + 30*time.Second, "1h30s"}, // Updated ShortDurV2 logic
		{"1 hour 5 seconds", 1*time.Hour + 5*time.Second, "1h5s"},
		{"2 hours 5 minutes 10 seconds", 2*time.Hour + 5*time.Minute + 10*time.Second, "2h5m10s"},
		{"negative 1m30s", -(1*time.Minute + 30*time.Second), "-1m30s"},
		{"negative 1h5s", -(1*time.Hour + 5*time.Second), "-1h5s"},
		{"25h expected", 25 * time.Hour, "25h"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = common.NanosPerSecond
			if got := ShortDurV2(tt.duration); got != tt.want {
				t.Errorf("ShortDurV2(%v) = %q, want %q (original string: %q)", tt.duration, got, tt.want, tt.duration.String())
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		config   *ShortDurationConfig
		want     string
	}{
		{"zero", 0, nil, "0s"},
		{"1s", 1 * time.Second, nil, "1s"}, // Expect "1s", not "1.000s"
		{"1m", 1 * time.Minute, nil, "1m"},
		{"1h", 1 * time.Hour, nil, "1h"},
		{"1d", 24 * time.Hour, nil, "1d"},
		{"1w", 7 * 24 * time.Hour, nil, "1w"},
		{"1w 1d 1h 1m 1s", (7*24+24)*time.Hour + 1*time.Hour + 1*time.Minute + 1*time.Second, nil, "1w1d1h1m1s"},
		{"1h30m", 1*time.Hour + 30*time.Minute, nil, "1h30m"},
		{"1m30s", 1*time.Minute + 30*time.Second, nil, "1m30s"},
		{"1.5s", 1*time.Second + 500*time.Millisecond, nil, "1.500s"},
		{"500ms", 500 * time.Millisecond, nil, "500ms"},
		{"1.234ms", 1*time.Millisecond + 234*time.Microsecond, nil, "1.234ms"},
		{"500µs", 500 * time.Microsecond, nil, "500µs"},
		{"1.234µs", 1*time.Microsecond + 234*time.Nanosecond, nil, "1.234µs"},
		{"250ns", 250 * time.Nanosecond, nil, "250ns"},
		{"negative 1h30m", -(1*time.Hour + 30*time.Minute), nil, "-1h30m"},

		// Test MaxUnits: if MaxUnits is hit, the last unit displayed is floated with its remainder.
		{"1h30m20s MaxUnits=2", 1*time.Hour + 30*time.Minute + 20*time.Second, &ShortDurationConfig{MaxUnits: 2}, "1h30.333333333333332m"}, // 20s = 1/3 min = 0.333... min. Total 30.333...m for the second part.
		{"1d1h30m MaxUnits=1", (24*time.Hour + 1*time.Hour + 30*time.Minute), &ShortDurationConfig{MaxUnits: 1}, "1.0625d"},                // 25.5h. 25.5/24 = 1.0625d.
		{"1w1d1h MaxUnits=2", (7*24+24)*time.Hour + 1*time.Hour, &ShortDurationConfig{MaxUnits: 2}, "1w1.0416666666666667d"},               // 1w + 25h. 25/24 = 1.0416...d.

		// Test revised MaxUnits logic based on problem description (float last part)
		{"1h30m20s MaxUnits=2 revised", 1*time.Hour + 30*time.Minute + 20*time.Second, &ShortDurationConfig{MaxUnits: 2}, "1h30.333333333333332m"}, // Adjusted to ...332m
		{"1d1h30m MaxUnits=1 revised", (24*time.Hour + 1*time.Hour + 30*time.Minute), &ShortDurationConfig{MaxUnits: 1}, "1.0625d"},
		{"1w1d1h MaxUnits=2 revised", (7*24+24)*time.Hour + 1*time.Hour, &ShortDurationConfig{MaxUnits: 2}, "1w1.0416666666666667d"},

		{"1.234567s MaxUnits=1", 1*time.Second + 234*time.Millisecond + 567*time.Microsecond, &ShortDurationConfig{MaxUnits: 1}, "1.234567s"},

		{"1h30m10s Separator=' '", 1*time.Hour + 30*time.Minute + 10*time.Second, &ShortDurationConfig{UnitSeparator: " "}, "1h 30m 10s"},
		{"1d1h Separator='-'", 24*time.Hour + 1*time.Hour, &ShortDurationConfig{UnitSeparator: "-"}, "1d-1h"},

		// Complex multi-part s,ms,µs,ns cases
		{"1s1.123µs (1s + 1µs + 123ns)", 1*time.Second + 1*time.Microsecond + 123*time.Nanosecond, nil, "1s1.123µs"},
		{"1m1.123456ms (1m + 1ms + 123µs + 456ns)", 1*time.Minute + 1*time.Millisecond + 123*time.Microsecond + 456*time.Nanosecond, nil, "1m1.123456ms"},
		{"1h1m1.000000123s (1h + 1m + 1s + 123ns)", 1*time.Hour + 1*time.Minute + 1*time.Second + 123*time.Nanosecond, nil, "1h1m1.000000123s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = common.NanosPerSecond
			if got := FormatDuration(tt.duration, tt.config); got != tt.want {
				t.Errorf("FormatDuration(%v, %#v) = %q, want %q (original string: %q)", tt.duration, tt.config, got, tt.want, tt.duration.String())
			}
		})
	}
}
