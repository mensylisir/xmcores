package logger

import (
	"bytes"
	"encoding/json" // For Prettyfier example
	"fmt"
	"github.com/sirupsen/logrus"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const (
	resetColorCode = 0
	// Default field separator
	defaultFieldSeparator = " | "
	// Default timestamp format
	defaultTimestampFormat = time.RFC3339 // "2006-01-02T15:04:05Z07:00"
)

// Formatter implements logrus.Formatter interface.
type Formatter struct {
	// TimestampFormat specifies the format of the timestamp. Default: time.RFC3339.
	TimestampFormat string
	// NoColors disables colorized output.
	NoColors bool
	// ForceColors forces colorized output, even if TTY is not detected (useful for some CI environments).
	ForceColors bool
	// DisableTimestamp disables timestamp output.
	DisableTimestamp bool
	// DisplayLevelName configures how log level names are displayed.
	// - ShowAll: Show all level names (e.g., [INFO], [DEBUG]).
	// - ShowAboveWarn: Show level names for WARN and above (ERROR, FATAL, PANIC).
	// - ShowAboveError: Show level names for ERROR and above (FATAL, PANIC).
	// - HideAll: Never show level names.
	DisplayLevelName LevelNameDisplayMode
	// ShowFullLevel shows the full level name (e.g., "WARNING") instead of a shortened version (e.g., "WARN").
	ShowFullLevel bool
	// NoUppercaseLevel prevents uppercasing of the level name.
	NoUppercaseLevel bool
	// HideKeys hides field keys, showing only field values (e.g., "[fieldValue]" instead of "[fieldKey:fieldValue]").
	HideKeys bool
	// FieldsDisplayWithOrder specifies a list of field keys to display in a specific order.
	// Fields not in this list will be appended alphabetically after the ordered fields.
	// If nil or empty, all fields are displayed alphabetically.
	FieldsDisplayWithOrder []string
	// FieldSeparator defines the separator string used between fields. Default: " | ".
	FieldSeparator string
	// CallerFirst places caller information (if available) before the log level and message.
	CallerFirst bool
	// DisableCaller disables caller information output.
	DisableCaller bool
	// CustomCallerFormatter allows a custom function to format caller information.
	CustomCallerFormatter func(*runtime.Frame) string
	// MaxFieldValueLength specifies the maximum length for a field value string. Longer values will be truncated.
	// 0 means no truncation.
	MaxFieldValueLength int
	// Prettyfier can be used to pretty-print field values, e.g., for structs or complex types.
	// If set, it overrides default value formatting.
	Prettyfier func(key string, value interface{}) string
}

// LevelNameDisplayMode defines how log level names are displayed.
type LevelNameDisplayMode int

const (
	// ShowAll shows all level names.
	ShowAll LevelNameDisplayMode = iota
	// ShowAboveWarn shows level names for WARN, ERROR, FATAL, PANIC.
	ShowAboveWarn
	// ShowAboveError shows level names for ERROR, FATAL, PANIC.
	ShowAboveError
	// HideAll hides all level names.
	HideAll
)

// Format formats the log entry.
func (f *Formatter) Format(entry *logrus.Entry) ([]byte, error) {
	b := &bytes.Buffer{}

	// Timestamp
	if !f.DisableTimestamp {
		timestampFormat := f.TimestampFormat
		if timestampFormat == "" {
			timestampFormat = defaultTimestampFormat
		}
		b.WriteString(entry.Time.Format(timestampFormat))
		b.WriteString(" ")
	}

	// Caller info (if CallerFirst)
	if f.CallerFirst && !f.DisableCaller {
		f.writeCaller(b, entry)
		if entry.HasCaller() {
			b.WriteString(" ")
		}
	}

	// Level
	showLevelName := false
	switch f.DisplayLevelName {
	case ShowAll:
		showLevelName = true
	case ShowAboveWarn:
		showLevelName = entry.Level <= logrus.WarnLevel
	case ShowAboveError:
		showLevelName = entry.Level <= logrus.ErrorLevel
	case HideAll:
		showLevelName = false
	}

	useColors := !f.NoColors
	if f.ForceColors {
		useColors = true
	}

	if showLevelName {
		levelColor := getColorByLevel(entry.Level)
		if useColors {
			fmt.Fprintf(b, "\x1b[%dm", levelColor)
		}

		var levelStr string
		rawLevelStr := entry.Level.String()
		if f.ShowFullLevel {
			levelStr = rawLevelStr
		} else {
			if len(rawLevelStr) >= 4 {
				levelStr = rawLevelStr[:4]
			} else {
				levelStr = rawLevelStr
			}
		}

		if !f.NoUppercaseLevel {
			levelStr = strings.ToUpper(levelStr)
		}

		fmt.Fprintf(b, "[%s]", levelStr) // Removed trailing space, add one after color reset or if no level

		if useColors {
			fmt.Fprintf(b, "\x1b[%dm", resetColorCode)
		}
		b.WriteString(" ") // Space after level (or where level would be)
	}

	// Fields
	fieldSeparator := f.FieldSeparator
	if fieldSeparator == "" {
		fieldSeparator = defaultFieldSeparator
	}

	if len(entry.Data) > 0 {
		b.WriteString("[")
		if f.FieldsDisplayWithOrder == nil || len(f.FieldsDisplayWithOrder) == 0 {
			f.writeFieldsAlphabetically(b, entry, fieldSeparator)
		} else {
			f.writeOrderedFields(b, entry, fieldSeparator)
		}
		b.WriteString("] ")
	}

	// Message
	b.WriteString(entry.Message)

	// Caller info (if not CallerFirst and not disabled)
	if !f.CallerFirst && !f.DisableCaller && entry.HasCaller() {
		b.WriteString(" ")
		f.writeCaller(b, entry)
	}

	// Final color reset if colors were used and not reset by level display
	// This is more of a safeguard if the last colored element wasn't the level.
	// However, current logic resets after level. If message or fields were colored, this would be needed.
	// For now, assuming only level is colored.

	b.WriteByte('\n')
	return b.Bytes(), nil
}

func (f *Formatter) writeFieldsAlphabetically(b *bytes.Buffer, entry *logrus.Entry, separator string) {
	fields := make([]string, 0, len(entry.Data))
	for field := range entry.Data {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	for i, field := range fields {
		f.writeKeyValue(b, field, entry.Data[field])
		if i < len(fields)-1 {
			b.WriteString(separator)
		}
	}
}

func (f *Formatter) writeOrderedFields(b *bytes.Buffer, entry *logrus.Entry, separator string) {
	displayedCount := 0
	totalFields := len(entry.Data)

	foundInOrder := make(map[string]bool)
	for _, field := range f.FieldsDisplayWithOrder {
		if value, ok := entry.Data[field]; ok {
			if displayedCount > 0 {
				b.WriteString(separator)
			}
			f.writeKeyValue(b, field, value)
			foundInOrder[field] = true
			displayedCount++
		}
	}

	if displayedCount < totalFields {
		remainingFields := make([]string, 0, totalFields-displayedCount)
		for field := range entry.Data {
			if !foundInOrder[field] {
				remainingFields = append(remainingFields, field)
			}
		}
		sort.Strings(remainingFields)

		for _, field := range remainingFields {
			if displayedCount > 0 {
				b.WriteString(separator)
			}
			f.writeKeyValue(b, field, entry.Data[field])
			displayedCount++
		}
	}
}

func (f *Formatter) writeKeyValue(b *bytes.Buffer, key string, value interface{}) {
	var valStr string
	if f.Prettyfier != nil {
		valStr = f.Prettyfier(key, value)
	} else {
		valStr = fmt.Sprintf("%v", value)
	}

	if f.MaxFieldValueLength > 0 && len(valStr) > f.MaxFieldValueLength {
		valStr = valStr[:f.MaxFieldValueLength] + "..."
	}

	if f.HideKeys {
		b.WriteString(valStr)
	} else {
		fmt.Fprintf(b, "%s:%s", key, valStr)
	}
}

func (f *Formatter) writeCaller(b *bytes.Buffer, entry *logrus.Entry) {
	if !entry.HasCaller() {
		return
	}
	if f.CustomCallerFormatter != nil {
		fmt.Fprint(b, f.CustomCallerFormatter(entry.Caller))
	} else {
		callerFile := filepath.Base(entry.Caller.File)
		callerFunc := filepath.Base(entry.Caller.Function) // Show only function name
		// Remove package path from function name for even more conciseness if desired
		if parts := strings.Split(callerFunc, "."); len(parts) > 1 {
			callerFunc = parts[len(parts)-1]
		}
		fmt.Fprintf(b, "(%s:%d %s)", callerFile, entry.Caller.Line, callerFunc)
	}
}

// getColorByLevel remains the same
func getColorByLevel(level logrus.Level) int {
	switch level {
	case logrus.TraceLevel:
		return colorGray
	case logrus.DebugLevel:
		return colorBlue
	case logrus.WarnLevel:
		return colorYellow
	case logrus.ErrorLevel, logrus.FatalLevel, logrus.PanicLevel:
		return colorRed
	default: // InfoLevel
		return colorGray
	}
}

// Color constants remain the same
const (
	colorRed    = 31
	colorYellow = 33
	colorBlue   = 36
	colorGray   = 37
)

// Example Prettyfier function
func JSONPrettyfier(key string, value interface{}) string {
	// For specific keys or types, you might want different pretty printing.
	// This is a generic JSON marshaller for complex types.
	if _, ok := value.(string); ok { // Avoid JSON marshalling simple strings
		return fmt.Sprintf("%v", value)
	}
	if _, ok := value.(fmt.Stringer); ok { // Use String() method if available
		return fmt.Sprintf("%v", value)
	}

	// Attempt to marshal complex types as JSON
	// Be careful with types that might cause marshalling errors or have sensitive data.
	bytes, err := json.Marshal(value)
	if err == nil {
		return string(bytes)
	}
	return fmt.Sprintf("%+v", value) // Fallback to Go's default detailed print
}
