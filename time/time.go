// time.go
package time

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mensylisir/xmcores/common"
)

// ShortDur shortens the string representation of a time.Duration from d.String().
func ShortDur(d time.Duration) string {
	s := d.String()
	if d == 0 {
		return "0s"
	}
	if strings.HasSuffix(s, "m0s") {
		s = s[:len(s)-2]
	}
	if strings.HasSuffix(s, "h0m") {
		s = s[:len(s)-2]
	}
	return s
}

// formatDecimalNumber ensures specific padding for s, ms, µs units.
// For s, ms, µs: if fractional and naturally < 3 decimal places, pad to 3.
// Otherwise, use strconv.FormatFloat's default minimal representation.
func formatDecimalNumber(val float64, unitName string) string {
	sVal := strconv.FormatFloat(val, 'f', -1, 64)

	isTargetUnit := unitName == "s" || unitName == "ms" || unitName == "µs"

	if isTargetUnit {
		parts := strings.Split(sVal, ".")
		if len(parts) == 2 { // Has a decimal part
			integerPart := parts[0]
			decimalPart := parts[1]

			if len(decimalPart) < 3 {
				// Pad with zeros to ensure 3 decimal places
				decimalPart = decimalPart + strings.Repeat("0", 3-len(decimalPart))
				return integerPart + "." + decimalPart
			}
			// Already has 3 or more decimal places
			return sVal
		}
		// No decimal part (e.g. "2" from 2.0), return as is.
		// Tests expect "1s", not "1.000s" for whole numbers.
		return sVal
	}
	// For other units or non-fractional, return original float string
	return sVal
}

// ShortDurV2 provides a representation that omits zero intermediate units (e.g., 1h5s).
// For sub-second precision, it formats them as a decimal of the smallest displayed larger unit (s, ms, or µs).
// Does NOT convert hours to days/weeks.
func ShortDurV2(d time.Duration) string {
	if d == 0 {
		return "0s"
	}

	sign := ""
	if d < 0 {
		sign = "-"
		d = -d
	}

	var parts []string
	nanos := d.Nanoseconds()

	h := nanos / (common.NanosPerSecond * 3600)
	if h > 0 {
		parts = append(parts, fmt.Sprintf("%dh", h))
		nanos %= (common.NanosPerSecond * 3600)
	}

	m := nanos / (common.NanosPerSecond * 60)
	if m > 0 {
		parts = append(parts, fmt.Sprintf("%dm", m))
		nanos %= (common.NanosPerSecond * 60)
	} else if h > 0 && nanos > 0 {
		// Omit "0m" if h was printed and m is 0 but s/ns follows for brevity
	}

	s := nanos / common.NanosPerSecond
	nanosSubS := nanos % common.NanosPerSecond

	if len(parts) == 0 { // No hours or minutes, format s, ms, µs, or ns
		if s > 0 {
			if nanosSubS == 0 {
				parts = append(parts, fmt.Sprintf("%ds", s))
			} else {
				floatVal := float64(s) + float64(nanosSubS)/float64(common.NanosPerSecond)
				parts = append(parts, formatDecimalNumber(floatVal, "s")+"s")
			}
		} else { // Pure sub-second
			ms := nanosSubS / common.NanosPerMillisecond
			nanosSubMs := nanosSubS % common.NanosPerMillisecond
			if ms > 0 {
				if nanosSubMs == 0 {
					parts = append(parts, fmt.Sprintf("%dms", ms))
				} else {
					floatVal := float64(ms) + float64(nanosSubMs)/float64(common.NanosPerMillisecond)
					parts = append(parts, formatDecimalNumber(floatVal, "ms")+"ms")
				}
			} else {
				us := nanosSubS / common.NanosPerMicrosecond
				nanosSubUs := nanosSubS % common.NanosPerMicrosecond
				if us > 0 {
					if nanosSubUs == 0 {
						parts = append(parts, fmt.Sprintf("%dµs", us))
					} else {
						floatVal := float64(us) + float64(nanosSubUs)/float64(common.NanosPerMicrosecond)
						parts = append(parts, formatDecimalNumber(floatVal, "µs")+"µs")
					}
				} else {
					parts = append(parts, fmt.Sprintf("%dns", nanosSubS))
				}
			}
		}
	} else if s > 0 || nanosSubS > 0 { // Hours or minutes were printed, add seconds part
		if nanosSubS == 0 { // Exact seconds, no sub-second part
			parts = append(parts, fmt.Sprintf("%ds", s))
		} else {
			floatVal := float64(s) + float64(nanosSubS)/float64(common.NanosPerSecond)
			parts = append(parts, formatDecimalNumber(floatVal, "s")+"s")
		}
	}
	// else if s==0 and nanosSubS==0 and len(parts)>0: means perfectly Xh or Xm, no "0s" needed for ShortDurV2.

	if len(parts) == 0 { // Should be caught by d == 0 at start, or if nanos was 0 initially but not d.
		return "0s" // Fallback for safety, though d=0 is handled. Non-zero d should produce parts.
	}
	return sign + strings.Join(parts, "")
}

// ShortDurationConfig
type ShortDurationConfig struct {
	MaxUnits      int
	UnitSeparator string
}

// FormatDuration formats a time.Duration with MaxUnits and Separator.
// It formats each primary unit as an integer. If a unit is the last one to be displayed
// due to MaxUnits or being the smallest significant unit, its remainder is formatted
// using strconv.FormatFloat for concise decimal representation.
func FormatDuration(d time.Duration, config *ShortDurationConfig) string {
	if d == 0 {
		return "0s"
	}

	cfg := ShortDurationConfig{MaxUnits: 0, UnitSeparator: ""}
	if config != nil {
		cfg = *config
	}

	sign := ""
	if d < 0 {
		sign = "-"
		d = -d
	}

	var parts []string
	remainingNanos := d.Nanoseconds()

	units := []struct {
		name string
		val  int64
	}{
		{"w", 7 * 24 * 3600 * common.NanosPerSecond},
		{"d", 24 * 3600 * common.NanosPerSecond},
		{"h", 3600 * common.NanosPerSecond},
		{"m", 60 * common.NanosPerSecond},
		{"s", common.NanosPerSecond},
		{"ms", common.NanosPerMillisecond},
		{"µs", common.NanosPerMicrosecond},
		{"ns", 1},
	}

	for _, unit := range units {
		if remainingNanos == 0 {
			break
		}
		if cfg.MaxUnits > 0 && len(parts) >= cfg.MaxUnits {
			break
		}

		if remainingNanos < unit.val && unit.name != "ns" {
			continue
		}

		count := remainingNanos / unit.val
		remainderForUnitLevel := remainingNanos % unit.val
		isLastUnitToDisplayByMax := (cfg.MaxUnits > 0 && len(parts)+1 == cfg.MaxUnits)

		if isLastUnitToDisplayByMax {
			if remainingNanos > 0 {
				floatVal := float64(remainingNanos) / float64(unit.val)
				parts = append(parts, formatDecimalNumber(floatVal, unit.name)+unit.name)
			}
			remainingNanos = 0
			break
		} else if count > 0 {
			if unit.name == "ns" {
				parts = append(parts, strconv.FormatInt(count, 10)+unit.name)
				remainingNanos = remainderForUnitLevel
			} else if unit.name == "w" || unit.name == "d" || unit.name == "h" || unit.name == "m" {
				parts = append(parts, strconv.FormatInt(count, 10)+unit.name)
				remainingNanos = remainderForUnitLevel
			} else { // unit.name is s, ms, or µs
				if remainderForUnitLevel == 0 {
					parts = append(parts, strconv.FormatInt(count, 10)+unit.name)
					remainingNanos = 0
				} else {
					shouldFloatRemainder := false
					if unit.name == "s" {
						// 's' floats its remainder if:
						// 1. The remainder is purely nanoseconds (less than 1 microsecond).
						// 2. The remainder is purely milliseconds (remainder % NanosPerMillisecond == 0).
						if remainderForUnitLevel < common.NanosPerMicrosecond { // Purely ns
							shouldFloatRemainder = true
						} else if remainderForUnitLevel%common.NanosPerMillisecond == 0 { // Purely ms
							shouldFloatRemainder = true
						}
						// Otherwise (e.g. contains µs and is not purely ms), it does not float.
					} else if unit.name == "ms" {
						// 'ms' always floats its remainder (which could be µs and/or ns).
						shouldFloatRemainder = true
					} else if unit.name == "µs" {
						// 'µs' always floats its remainder (which is purely ns).
						shouldFloatRemainder = true
					}

					if shouldFloatRemainder {
						floatVal := float64(count) + float64(remainderForUnitLevel)/float64(unit.val)
						parts = append(parts, formatDecimalNumber(floatVal, unit.name)+unit.name)
						remainingNanos = 0
					} else {
						parts = append(parts, strconv.FormatInt(count, 10)+unit.name)
						remainingNanos = remainderForUnitLevel
					}
				}
			}
		} else if count == 0 && len(parts) == 0 && remainingNanos > 0 {
			// This path is taken when the duration is smaller than the current unit,
			// no parts have been added yet, and MaxUnits doesn't force this to be the only unit shown as a float.
			// The initial `if remainingNanos < unit.val` skip usually handles this for default behavior,
			// ensuring 500ms becomes "500ms" not "0.500s".
			// This block primarily addresses MaxUnits scenarios where a small duration must be
			// represented as a fraction of a larger unit that isn't the *only* unit.
			// However, `isLastUnitToDisplayByMax` at the top handles the "only unit" float case.
			// For default (nil config), this specific path leads to "0.xxxUnit" for the first applicable unit.
			// Given tests expect "500ms" for 500ms (nil config), the top skip is doing its job.
			// This block's main utility might be if `MaxUnits > 1` and we want the *first* of those MaxUnits
			// to be a float if it's small. e.g. MaxUnits=2, duration=0.5s -> "0.500s" (and no second unit).
			// This seems to be the implied behavior from its original structure.
			floatVal := float64(remainingNanos) / float64(unit.val)
			parts = append(parts, formatDecimalNumber(floatVal, unit.name)+unit.name)
			remainingNanos = 0
		}
	}

	if len(parts) == 0 {
		if d.Nanoseconds() > 0 {
			return sign + strconv.FormatInt(d.Nanoseconds(), 10) + "ns"
		}
		return "0s"
	}

	return sign + strings.Join(parts, cfg.UnitSeparator)
}
