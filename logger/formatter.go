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
	resetColorCode         = 0
	defaultFieldSeparator  = " | "
	defaultTimestampFormat = time.RFC3339 // "2006-01-02T15:04:05Z07:00"
)

type Formatter struct {
	TimestampFormat        string
	NoColors               bool
	ForceColors            bool
	DisableTimestamp       bool
	DisplayLevelName       LevelNameDisplayMode
	ShowFullLevel          bool
	NoUppercaseLevel       bool
	HideKeys               bool
	FieldsDisplayWithOrder []string
	FieldSeparator         string
	CallerFirst            bool
	DisableCaller          bool
	CustomCallerFormatter  func(*runtime.Frame) string
	MaxFieldValueLength    int
	Prettyfier             func(key string, value interface{}) string
}

type LevelNameDisplayMode int

const (
	ShowAll LevelNameDisplayMode = iota
	ShowAboveWarn
	ShowAboveError
	HideAll
)

func (f *Formatter) Format(entry *logrus.Entry) ([]byte, error) {
	b := &bytes.Buffer{}

	if !f.DisableTimestamp {
		timestampFormat := f.TimestampFormat
		if timestampFormat == "" {
			timestampFormat = defaultTimestampFormat
		}
		b.WriteString(entry.Time.Format(timestampFormat))
		b.WriteString(" ")
	}

	if f.CallerFirst && !f.DisableCaller {
		f.writeCaller(b, entry)
		if entry.HasCaller() {
			b.WriteString(" ")
		}
	}

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

		fmt.Fprintf(b, "[%s]", levelStr)

		if useColors {
			fmt.Fprintf(b, "\x1b[%dm", resetColorCode)
		}
		b.WriteString(" ")
	}

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

	b.WriteString(entry.Message)

	if !f.CallerFirst && !f.DisableCaller && entry.HasCaller() {
		b.WriteString(" ")
		f.writeCaller(b, entry)
	}

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
		callerFunc := filepath.Base(entry.Caller.Function)
		if parts := strings.Split(callerFunc, "."); len(parts) > 1 {
			callerFunc = parts[len(parts)-1]
		}
		fmt.Fprintf(b, "(%s:%d %s)", callerFile, entry.Caller.Line, callerFunc)
	}
}

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

const (
	colorRed    = 31
	colorYellow = 33
	colorBlue   = 36
	colorGray   = 37
)

func JSONPrettyfier(key string, value interface{}) string {
	if _, ok := value.(string); ok {
		return fmt.Sprintf("%v", value)
	}
	if _, ok := value.(fmt.Stringer); ok {
		return fmt.Sprintf("%v", value)
	}
	bytesVar, err := json.Marshal(value)
	if err == nil {
		return string(bytesVar)
	}
	return fmt.Sprintf("%+v", value)
}
