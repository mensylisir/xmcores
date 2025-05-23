// logger_test.go
package logger

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/mensylisir/xmcores/common"
	"io"
	"io/fs" // For fs.DirEntry
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to create a temporary directory for tests
func createTestLogDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "logger_test_")
	require.NoError(t, err, "Failed to create temp log dir")
	return dir
}

// Helper to convert []fs.DirEntry to []string for easier logging/assertion
func filesToNames(entries []fs.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// Hook to capture log entries for testing
type testHook struct {
	mu      sync.Mutex
	Entries []*logrus.Entry
}

func (h *testHook) Levels() []logrus.Level { return logrus.AllLevels }
func (h *testHook) Fire(entry *logrus.Entry) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Entries = append(h.Entries, entry)
	return nil
}
func (h *testHook) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Entries = nil
}
func (h *testHook) LastEntry() *logrus.Entry {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.Entries) == 0 {
		return nil
	}
	return h.Entries[len(h.Entries)-1]
}

// Helper to sort string slices for comparison (assuming it's defined elsewhere or here)
func sortStrings(s []string) {
	sort.Strings(s)
}

// Helper to compare two string slices ignoring order (assuming it's defined elsewhere or here)
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ac := make([]string, len(a))
	bc := make([]string, len(b))
	copy(ac, a)
	copy(bc, b)
	sortStrings(ac)
	sortStrings(bc)
	return reflect.DeepEqual(ac, bc)
}

func TestInitGlobalLogger(t *testing.T) {
	originalLog := Log
	defer func() { Log = originalLog }()

	baseTmpDir := createTestLogDir(t)
	defer os.RemoveAll(baseTmpDir)

	tests := []struct {
		name                  string
		getOutputPath         func(t *testing.T) string
		verbose               bool
		defaultLevel          logrus.Level
		logMessageToTrigger   func(l *XMLog)
		expectedLogLevel      logrus.Level
		expectedFormatterDisp LevelNameDisplayMode
		expectFile            bool
		expectConsoleOut      bool
		wantErr               bool
	}{
		{
			name: "File output, verbose, Info default",
			getOutputPath: func(t *testing.T) string {
				path := filepath.Join(baseTmpDir, "file_verbose_info")
				require.NoError(t, os.MkdirAll(path, 0755))
				return path
			},
			verbose:      true,
			defaultLevel: logrus.InfoLevel,
			logMessageToTrigger: func(l *XMLog) {
				if l != nil {
					l.Debug("Test verbose log to trigger file.")
				}
			},
			expectedLogLevel:      logrus.DebugLevel,
			expectedFormatterDisp: ShowAll,
			expectFile:            true,
			expectConsoleOut:      false, // Because outputPath is set, default out should be io.Discard
			wantErr:               false,
		},
		{
			name: "File output, not verbose, Warn default",
			getOutputPath: func(t *testing.T) string {
				path := filepath.Join(baseTmpDir, "file_notverbose_warn")
				require.NoError(t, os.MkdirAll(path, 0755))
				return path
			},
			verbose:      false,
			defaultLevel: logrus.WarnLevel,
			logMessageToTrigger: func(l *XMLog) {
				if l != nil {
					l.Warn("Test warn log to trigger file.")
				}
			},
			expectedLogLevel:      logrus.WarnLevel,
			expectedFormatterDisp: ShowAboveWarn,
			expectFile:            true,
			expectConsoleOut:      false, // Because outputPath is set
			wantErr:               false,
		},
		{
			name:          "Console output, verbose, Info default",
			getOutputPath: func(t *testing.T) string { return "" },
			verbose:       true,
			defaultLevel:  logrus.InfoLevel,
			logMessageToTrigger: func(l *XMLog) {
				if l != nil {
					l.Debug("Console test")
				}
			},
			expectedLogLevel:      logrus.DebugLevel,
			expectedFormatterDisp: ShowAll,
			expectFile:            false,
			expectConsoleOut:      true,
			wantErr:               false,
		},
		{
			name:          "Console output, not verbose, Error default",
			getOutputPath: func(t *testing.T) string { return "" },
			verbose:       false,
			defaultLevel:  logrus.ErrorLevel,
			logMessageToTrigger: func(l *XMLog) {
				if l != nil {
					l.Error("Console error test")
				}
			},
			expectedLogLevel:      logrus.ErrorLevel,
			expectedFormatterDisp: ShowAboveWarn,
			expectFile:            false,
			expectConsoleOut:      true,
			wantErr:               false,
		},
		{
			name: "Invalid output path",
			getOutputPath: func(t *testing.T) string {
				if runtime.GOOS == "windows" {
					placeholderFile := filepath.Join(baseTmpDir, "placeholder_for_fail_init.txt")
					f, errCreate := os.Create(placeholderFile)
					require.NoError(t, errCreate)
					require.NoError(t, f.Close())
					return placeholderFile
				}
				return "/this/path/should/not/be/creatable/by/mkdirall/for_init"
			},
			verbose:             false,
			defaultLevel:        logrus.InfoLevel,
			logMessageToTrigger: nil,
			wantErr:             true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Log = nil // Reset global Log for each sub-test
			currentOutputPath := tt.getOutputPath(t)

			err := InitGlobalLogger(currentOutputPath, tt.verbose, tt.defaultLevel)
			if tt.wantErr {
				assert.Error(t, err, "Expected an error for outputPath: %s", currentOutputPath)
				return
			}
			require.NoError(t, err, "InitGlobalLogger failed unexpectedly for outputPath: %s", currentOutputPath)
			require.NotNil(t, Log, "Global Log instance should be initialized")
			require.NotNil(t, Log.Logger, "Embedded logrus.Logger should be initialized")

			assert.Equal(t, tt.expectedLogLevel, Log.Logger.GetLevel(), "Log level mismatch")

			formatter, ok := Log.Logger.Formatter.(*Formatter)
			require.True(t, ok, "Formatter is not of expected type *Formatter")
			assert.Equal(t, tt.expectedFormatterDisp, formatter.DisplayLevelName, "Formatter DisplayLevelName mismatch")

			if tt.expectFile {
				require.NotEmpty(t, currentOutputPath, "outputPath should not be empty")
				if tt.logMessageToTrigger != nil {
					tt.logMessageToTrigger(Log)
				} else {
					Log.Info("Default test log entry for file creation.")
				}

				// Retry mechanism to check for non-empty file
				var foundNonEmptyRotatedFile bool
				var actualLogFileName string
				var lastSize int64
				for i := 0; i < 20; i++ { // Retry for up to 20 * 50ms = 1 second
					files, listErr := os.ReadDir(currentOutputPath)
					if listErr == nil {
						for _, f := range files {
							if strings.HasPrefix(f.Name(), "app.log.") && !f.IsDir() {
								logFileStat, statErr := f.Info()
								if statErr == nil && logFileStat.Size() > 0 {
									foundNonEmptyRotatedFile = true
									actualLogFileName = f.Name()
									lastSize = logFileStat.Size()
									break
								} else if statErr == nil {
									lastSize = logFileStat.Size()
								}
							}
						}
					}
					if foundNonEmptyRotatedFile {
						break
					}
					time.Sleep(50 * time.Millisecond)
				}

				if foundNonEmptyRotatedFile {
					t.Logf("Found non-empty rotated log file: %s, size: %d", filepath.Join(currentOutputPath, actualLogFileName), lastSize)
				} else {
					files, _ := os.ReadDir(currentOutputPath) // Read again for final error message
					assert.True(t, foundNonEmptyRotatedFile, "Expected a non-empty rotated log file starting with 'app.log.' in %s. Files found: %v. Last seen size for potential log file: %d", currentOutputPath, filesToNames(files), lastSize)
				}

				linkFilePath := filepath.Join(currentOutputPath, "app.log")
				_, linkStatErr := os.Lstat(linkFilePath)
				if runtime.GOOS == "windows" {
					if linkStatErr != nil {
						t.Logf("INFO: Lstat on link file '%s' failed on Windows: %v. This is often expected.", linkFilePath, linkStatErr)
					} else {
						t.Logf("INFO: Link file '%s' found on Windows.", linkFilePath)
					}
				} else {
					assert.NoError(t, linkStatErr, "Log file/link '%s' should exist on non-Windows OS", linkFilePath)
				}
			}

			if tt.expectConsoleOut {
				assert.Equal(t, os.Stdout, Log.Logger.Out, "Expected logger output to be os.Stdout for console")
			} else if tt.expectFile {
				// After fix in InitGlobalLogger, default output should be io.Discard
				assert.Equal(t, io.Discard, Log.Logger.Out, "Expected logger default output to be io.Discard when file logging is active")
			}
		})
	}
}

func TestXMLog_StructuredMethods(t *testing.T) {
	logger := logrus.New()
	hook := &testHook{}
	logger.AddHook(hook)
	logger.SetOutput(io.Discard)
	logger.SetLevel(logrus.TraceLevel)
	logger.SetReportCaller(true)

	xmLogger := &XMLog{Logger: logger}

	testCases := []struct {
		name           string
		logAction      func()
		expectedMsg    string
		expectedLvl    logrus.Level
		expectedFields logrus.Fields
	}{
		{
			name: "InfoPipeline",
			logAction: func() {
				xmLogger.InfoPipeline("deploy-app", "Pipeline started", logrus.Fields{"version": "1.0"})
			},
			expectedMsg:    "Pipeline started",
			expectedLvl:    logrus.InfoLevel,
			expectedFields: logrus.Fields{common.PipelineName: "deploy-app", "version": "1.0"},
		},
		{
			name: "ErrorfTask with error",
			logAction: func() {
				testErr := errors.New("network timeout")
				xmLogger.ErrorfTask("user-sync", testErr, "Sync failed after %d retries", 3)
			},
			expectedMsg:    "Sync failed after 3 retries",
			expectedLvl:    logrus.ErrorLevel,
			expectedFields: logrus.Fields{common.TaskName: "user-sync", "error": errors.New("network timeout")},
		},
		{
			name: "DebugfModule",
			logAction: func() {
				xmLogger.DebugfModule("auth-service", "Token received: %s", "abc-xyz")
			},
			expectedMsg:    "Token received: abc-xyz",
			expectedLvl:    logrus.DebugLevel,
			expectedFields: logrus.Fields{common.ModuleName: "auth-service"},
		},
		{
			name: "WarnStep without dynamic fields",
			logAction: func() {
				xmLogger.WarnStep("validation-step", "Input validation warning")
			},
			expectedMsg:    "Input validation warning",
			expectedLvl:    logrus.WarnLevel,
			expectedFields: logrus.Fields{common.StepName: "validation-step"},
		},
		{
			name: "ErrorNode with nil error",
			logAction: func() {
				xmLogger.ErrorNode("worker-1", nil, "Unexpected state", logrus.Fields{"state": "corrupted"})
			},
			expectedMsg:    "Unexpected state",
			expectedLvl:    logrus.ErrorLevel,
			expectedFields: logrus.Fields{common.NodeName: "worker-1", "state": "corrupted"},
		},
		{
			name: "Generic Info with fields",
			logAction: func() {
				xmLogger.WithFields(logrus.Fields{"component": "general", "id": 123}).Info("General info message")
			},
			expectedMsg:    "General info message",
			expectedLvl:    logrus.InfoLevel,
			expectedFields: logrus.Fields{"component": "general", "id": 123},
		},
		{
			name: "Generic Tracef",
			logAction: func() {
				xmLogger.Tracef("Trace value: %d", 42)
			},
			expectedMsg:    "Trace value: 42",
			expectedLvl:    logrus.TraceLevel,
			expectedFields: logrus.Fields{},
		},
		{
			name: "TracePipeline",
			logAction: func() {
				xmLogger.TracePipeline("init-sequence", "Trace point A", logrus.Fields{"detail": "extra"})
			},
			expectedMsg:    "Trace point A",
			expectedLvl:    logrus.TraceLevel,
			expectedFields: logrus.Fields{common.PipelineName: "init-sequence", "detail": "extra"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			hook.Reset()
			tc.logAction()

			entry := hook.LastEntry()
			require.NotNil(t, entry, "Expected a log entry")

			assert.Equal(t, tc.expectedLvl, entry.Level, "Log level mismatch")
			assert.Equal(t, tc.expectedMsg, entry.Message, "Log message mismatch")

			filteredGotFields := make(logrus.Fields)
			for k, v := range entry.Data {
				if errVal, ok := v.(error); ok && k == "error" {
					filteredGotFields[k] = errVal.Error()
				} else {
					filteredGotFields[k] = v
				}
			}

			filteredWantFields := make(logrus.Fields)
			for k, v := range tc.expectedFields {
				if errVal, ok := v.(error); ok && k == "error" {
					filteredWantFields[k] = errVal.Error()
				} else {
					filteredWantFields[k] = v
				}
			}
			assert.Equal(t, filteredWantFields, filteredGotFields, "Log fields mismatch")
		})
	}
}

func TestXMLog_FatalMethods(t *testing.T) {
	logger := logrus.New()
	hook := &testHook{}
	logger.AddHook(hook)
	logger.SetOutput(io.Discard)
	logger.SetLevel(logrus.TraceLevel)
	logger.SetReportCaller(true)

	originalGlobalExitFunc := logrus.StandardLogger().ExitFunc
	var exitedWithCode = -1

	logger.ExitFunc = func(code int) {
		exitedWithCode = code
	}
	logrus.StandardLogger().ExitFunc = func(code int) { // Also mock global for safety, though instance specific should be hit
		exitedWithCode = code
	}

	defer func() {
		logrus.StandardLogger().ExitFunc = originalGlobalExitFunc
	}()

	xmLogger := &XMLog{Logger: logger}

	tests := []struct {
		name             string
		fatalAction      func()
		expectedMsg      string
		expectedFields   logrus.Fields
		expectedExitCode int
	}{
		{
			name: "FatalPipeline",
			fatalAction: func() {
				xmLogger.FatalPipeline("setup-cluster", errors.New("critical init fail"), "Cannot proceed with cluster setup", logrus.Fields{"phase": 1})
			},
			expectedMsg:      "Cannot proceed with cluster setup",
			expectedFields:   logrus.Fields{common.PipelineName: "setup-cluster", "error": errors.New("critical init fail"), "phase": 1},
			expectedExitCode: 1,
		},
		{
			name: "Generic Fatalf",
			fatalAction: func() {
				xmLogger.Fatalf("System integrity compromised: %s", "checksum mismatch")
			},
			expectedMsg:      "System integrity compromised: checksum mismatch",
			expectedFields:   logrus.Fields{},
			expectedExitCode: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hook.Reset()
			exitedWithCode = -1

			tc.fatalAction()

			assert.Equal(t, tc.expectedExitCode, exitedWithCode, "Exit code mismatch")

			entry := hook.LastEntry()
			require.NotNil(t, entry, "Expected a log entry for Fatal call")
			assert.Equal(t, logrus.FatalLevel, entry.Level)
			assert.Equal(t, tc.expectedMsg, entry.Message)

			filteredGotFields := make(logrus.Fields)
			for k, v := range entry.Data {
				if errVal, ok := v.(error); ok && k == "error" {
					filteredGotFields[k] = errVal.Error()
				} else {
					filteredGotFields[k] = v
				}
			}
			filteredWantFields := make(logrus.Fields)
			for k, v := range tc.expectedFields {
				if errVal, ok := v.(error); ok && k == "error" {
					filteredWantFields[k] = errVal.Error()
				} else {
					filteredWantFields[k] = v
				}
			}
			assert.Equal(t, filteredWantFields, filteredGotFields, "Log fields mismatch for Fatal call")
		})
	}
}

func TestXMLog_PanicMethods(t *testing.T) {
	logger := logrus.New()
	hook := &testHook{}
	logger.AddHook(hook)
	logger.SetOutput(io.Discard)
	logger.SetLevel(logrus.TraceLevel)
	logger.SetReportCaller(true)

	xmLogger := &XMLog{Logger: logger}

	tests := []struct {
		name             string
		panicAction      func()
		expectedMsg      string
		expectedFields   logrus.Fields
		expectedPanicVal interface{}
	}{
		{
			name: "PanicfTask",
			panicAction: func() {
				xmLogger.PanicfTask("data-corruption", errors.New("checksum error"), "Unrecoverable data corruption in task, value: %d", 123)
			},
			expectedMsg:      "Unrecoverable data corruption in task, value: 123",
			expectedFields:   logrus.Fields{common.TaskName: "data-corruption", "error": errors.New("checksum error")},
			expectedPanicVal: "Unrecoverable data corruption in task, value: 123",
		},
		{
			name: "Generic Panic",
			panicAction: func() {
				xmLogger.Panic("System invariant violated")
			},
			expectedMsg:      "System invariant violated",
			expectedFields:   logrus.Fields{},
			expectedPanicVal: "System invariant violated",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hook.Reset()
			var recoveredVal interface{}

			func() {
				defer func() {
					recoveredVal = recover()
				}()
				tc.panicAction()
			}()

			assert.NotNil(t, recoveredVal, "Expected a panic")
			if recoveredVal != nil {
				// Logrus panics with the *logrus.Entry or the message string depending on hooks/setup.
				// If it's an *logrus.Entry, its Message field would contain the string.
				// Checking if string representation contains the message is safer.
				assert.Contains(t, fmt.Sprintf("%v", recoveredVal), tc.expectedMsg, "Panic value should contain the log message")
			}

			entry := hook.LastEntry()
			require.NotNil(t, entry, "Expected a log entry for Panic call")
			assert.Equal(t, logrus.PanicLevel, entry.Level)
			assert.Equal(t, tc.expectedMsg, entry.Message)

			filteredGotFields := make(logrus.Fields)
			for k, v := range entry.Data {
				if errVal, ok := v.(error); ok && k == "error" {
					filteredGotFields[k] = errVal.Error()
				} else {
					filteredGotFields[k] = v
				}
			}
			filteredWantFields := make(logrus.Fields)
			for k, v := range tc.expectedFields {
				if errVal, ok := v.(error); ok && k == "error" {
					filteredWantFields[k] = errVal.Error()
				} else {
					filteredWantFields[k] = v
				}
			}
			assert.Equal(t, filteredWantFields, filteredGotFields, "Log fields mismatch for Panic call")
		})
	}
}

func TestFormatterOutput(t *testing.T) {
	fixedTime, _ := time.Parse(time.RFC3339, "2023-10-27T10:30:45Z")

	testCases := []struct {
		name            string
		formatter       *Formatter
		entrySetup      func(entry *logrus.Entry)
		fields          logrus.Fields
		message         string
		expectedPattern string
	}{
		{
			name: "Console verbose with colors, Info level shown, basic fields",
			formatter: &Formatter{
				TimestampFormat:  "15:04:05",
				NoColors:         false,
				DisplayLevelName: ShowAll,
				DisableCaller:    true,
			},
			entrySetup: func(entry *logrus.Entry) {
				entry.Level = logrus.InfoLevel
				entry.Time = fixedTime
			},
			fields:          logrus.Fields{"key1": "val1"},
			message:         "Console verbose test",
			expectedPattern: "10:30:45 \x1b[37m[INFO]\x1b[0m [key1:val1] Console verbose test\n",
		},
		{
			name: "File no colors, Warn level, Ordered Fields, Caller",
			formatter: &Formatter{
				TimestampFormat:  "2006/01/02 15:04:05.000 MST",
				NoColors:         true,
				DisplayLevelName: ShowAboveWarn,
				// **ASSUMING common constants are TitleCase based on previous error output**
				// If your common.TaskName is "task", change "Task" to "task" below
				FieldsDisplayWithOrder: []string{common.TaskName, "status", common.ModuleName},
				FieldSeparator:         " | ",
				DisableCaller:          false,
				CustomCallerFormatter:  nil,
			},
			entrySetup: func(entry *logrus.Entry) {
				entry.Level = logrus.WarnLevel
				entry.Time = fixedTime // UTC
			},
			fields: logrus.Fields{
				// **ASSUMING common constants are TitleCase**
				common.TaskName:   "backup-db", // If common.TaskName = "Task", this is "Task":"backup-db"
				"status":          "in_progress",
				"extra":           "details",      // Sorted after ordered fields
				common.ModuleName: "database_ops", // If common.ModuleName = "Module", this is "Module":"database_ops"
			},
			message: "File warning with specific field order",
			// Expected: "YYYY/MM/DD HH:MM:SS.mmm UTC [WARN] [Task:backup-db | status:in_progress | Module:database_ops | extra:details] Message (file:line func)\n"
			// Adjusted to match likely common constant casing and actual caller output:
			expectedPattern: "2023/10/27 10:30:45.000 UTC [WARN] [Task:backup-db | status:in_progress | Module:database_ops | extra:details] File warning with specific field order (logger_test.go:",
		},
		{
			name: "Hide level name, HideKeys, MaxFieldValueLength, No Timestamp, No Caller",
			formatter: &Formatter{
				DisableTimestamp:    true,
				NoColors:            true,
				DisplayLevelName:    HideAll,
				HideKeys:            true,
				DisableCaller:       true,
				MaxFieldValueLength: 7,
				FieldSeparator:      " - ",
			},
			entrySetup: func(entry *logrus.Entry) {
				entry.Level = logrus.DebugLevel
			},
			fields:          logrus.Fields{"long_field_name": "thisisverylongdata", "short_field": "abc"},
			message:         "Minimal output test",
			expectedPattern: "[thisisv... - abc] Minimal output test\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := logrus.New()
			logger.SetOutput(&buf)
			logger.SetFormatter(tc.formatter)
			logger.SetLevel(logrus.TraceLevel)

			if !tc.formatter.DisableCaller {
				logger.SetReportCaller(true)
			}

			entry := logrus.NewEntry(logger)
			if tc.entrySetup != nil {
				tc.entrySetup(entry)
			}

			entry.WithFields(tc.fields).Log(entry.Level, tc.message)

			output := buf.String()
			t.Logf("Formatted output for '%s':\n>>>\n%s<<<", tc.name, output)

			if !strings.Contains(output, tc.expectedPattern) {
				t.Errorf("Formatted output did not contain expected pattern.\nGot:\n%s\nWant pattern:\n%s", output, tc.expectedPattern)
			}

			if (!tc.formatter.NoColors || tc.formatter.ForceColors) && tc.formatter.DisplayLevelName != HideAll {
				levelIsShown := false
				switch tc.formatter.DisplayLevelName {
				case ShowAll:
					levelIsShown = true
				case ShowAboveWarn:
					levelIsShown = entry.Level <= logrus.WarnLevel
				case ShowAboveError:
					levelIsShown = entry.Level <= logrus.ErrorLevel
				}
				if levelIsShown {
					colorCode := fmt.Sprintf("\x1b[%dm", getColorByLevel(entry.Level))
					resetCode := fmt.Sprintf("\x1b[%dm", resetColorCode)
					assert.Contains(t, output, colorCode, "Should contain start color code for level")
					assert.Contains(t, output, resetCode, "Should contain reset color code after level")
				}
			}
		})
	}
}
