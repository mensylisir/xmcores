// logger_test.go
package logger

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mensylisir/xmcores/common"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestLogDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "logger_test_")
	require.NoError(t, err, "Failed to create temp log dir")
	return dir
}

func filesToNames(entries []fs.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

type testHook struct {
	mu      sync.Mutex
	Entries []*logrus.Entry
}

func (h *testHook) Levels() []logrus.Level { return logrus.AllLevels }
func (h *testHook) Fire(entry *logrus.Entry) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	clone := *entry
	clone.Data = make(logrus.Fields, len(entry.Data))
	for k, v := range entry.Data {
		clone.Data[k] = v
	}
	h.Entries = append(h.Entries, &clone)
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

func captureStdOutput(fn func()) string {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func assertFileContains(t *testing.T, filePath string, expectedSubstring string, msgAndArgs ...interface{}) {
	t.Helper()
	content, err := os.ReadFile(filePath)
	require.NoError(t, err, "Failed to read log file: %s", filePath)
	assert.Contains(t, string(content), expectedSubstring, msgAndArgs...)
}

func assertFileMatchesPatterns(t *testing.T, filePath string, patterns []string, msgAndArgs ...interface{}) {
	t.Helper()
	content, err := os.ReadFile(filePath)
	require.NoError(t, err, "Failed to read log file: %s", filePath)
	sContent := string(content)
	for _, pattern := range patterns {
		assert.Regexp(t, regexp.MustCompile(pattern), sContent, msgAndArgs...)
	}
}

func TestGlobalLoggerDefaultInitialization(t *testing.T) {
	require.NotNil(t, Log, "Global Log instance should be initialized by package init()")
	require.NotNil(t, Log.Logger, "Global Log.Logger should be initialized")
	assert.Equal(t, os.Stdout, Log.Out, "Global Log output should be os.Stdout by default") // As per our logger.go init
	_, ok := Log.Formatter.(*Formatter)
	assert.True(t, ok, "Global Log formatter should be of type *Formatter")

	originalHooks := Log.Hooks
	Log.ReplaceHooks(make(logrus.LevelHooks))
	hook := &testHook{}
	Log.AddHook(hook)
	defer func() {
		Log.ReplaceHooks(originalHooks)
	}()

	logMessage := "Global logger test: Info message from hook"
	Log.Info(logMessage)

	entry := hook.LastEntry()
	require.NotNil(t, entry, "Expected global Log to capture an entry via hook")
	assert.Equal(t, logrus.InfoLevel, entry.Level)
	assert.Equal(t, logMessage, entry.Message)

	t.Log("Visual check: The following line should have appeared on console during test init or normal run:")
	fmt.Printf("%s [EXPECTED CONSOLE OUTPUT] %s\n", time.Now().Format("2006-01-02 15:04:05"), logMessage)
}

func TestNewXMLogCoversDifferentConfigurations(t *testing.T) {
	baseTestDir := createTestLogDir(t)
	defer os.RemoveAll(baseTestDir)

	tests := []struct {
		name                   string
		outputPath             string
		verbose                bool
		defaultLevel           logrus.Level
		logFn                  func(l *XMLog)
		expectedConsolePattern string
		expectedFilePattern    string
		expectedFileNamePrefix string
		expectFileLink         bool
		wantErr                bool
		fileCheckTimeout       time.Duration
	}{
		{
			name:                   "Console only, verbose, Info level, Debug active",
			outputPath:             "",
			verbose:                true,
			defaultLevel:           logrus.InfoLevel,
			logFn:                  func(l *XMLog) { l.DebugfWithFields(logrus.Fields{"id": 1}, "Console debug msg %s", "verbose") },
			expectedConsolePattern: `^\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}\s\x1b\[36m\[DEBU\]\x1b\[0m(?:\s\[id:1\])?\sConsole debug msg verbose\s\s\[\S+:\d+\s\S+\]\n$`,
			expectedFilePattern:    "",
			wantErr:                false,
		},
		{
			name:                   "Console only, non-verbose, Warn level",
			outputPath:             "",
			verbose:                false,
			defaultLevel:           logrus.WarnLevel,
			logFn:                  func(l *XMLog) { l.Info("Console info (should not appear)"); l.Warn("Console warn msg") },
			expectedConsolePattern: `^\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}\s\x1b\[33m\[WARN\]\x1b\[0m\sConsole warn msg\s\s\[\S+:\d+\s\S+\]\n$`,
			wantErr:                false,
		},
		{
			name:                   "File and Console, verbose, Debug level",
			outputPath:             "file_and_console_debug",
			verbose:                true,
			defaultLevel:           logrus.DebugLevel,
			logFn:                  func(l *XMLog) { l.Debug("File and console debug") },
			expectedConsolePattern: `^\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}\s\x1b\[36m\[DEBU\]\x1b\[0m\sFile and console debug\s\s\[\S+:\d+\s\S+\]\n$`,
			expectedFilePattern:    `^\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}\.\d{3}\s\w{3,4}\s\[DEBU\]\sFile and console debug\s\s\[\S+:\d+\s\S+\]\n$`,
			expectedFileNamePrefix: "instance.log",
			expectFileLink:         true,
			wantErr:                false,
			fileCheckTimeout:       3 * time.Second,
		},
		{
			name:         "File and Console, non-verbose, Error level, with fields",
			outputPath:   "file_and_console_error",
			verbose:      false,
			defaultLevel: logrus.ErrorLevel,
			logFn: func(l *XMLog) {
				l.ErrorfWithFields(logrus.Fields{common.TaskName: "cleanup"}, "Task failed: %s", "disk full")
			},
			expectedConsolePattern: `^\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}\s\x1b\[31m\[ERRO\]\x1b\[0m\s\[` + regexp.QuoteMeta(common.TaskName) + `:cleanup\]\sTask failed: disk full\s\s\[\S+:\d+\s\S+\]\n$`,
			expectedFilePattern:    `^\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}\.\d{3}\s\w{3,4}\s\[ERRO\]\s\[` + regexp.QuoteMeta(common.TaskName) + `:cleanup\]\sTask failed: disk full\s\s\[\S+:\d+\s\S+\]\n$`,
			expectedFileNamePrefix: "instance.log",
			expectFileLink:         true,
			wantErr:                false,
			fileCheckTimeout:       3 * time.Second,
		},
		{
			name:         "Invalid output path (file exists)",
			outputPath:   "existing_file_as_dir.txt",
			verbose:      false,
			defaultLevel: logrus.InfoLevel,
			logFn:        nil,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fullOutputPath string
			if tt.outputPath != "" {
				fullOutputPath = filepath.Join(baseTestDir, tt.outputPath)
				if tt.name == "Invalid output path (file exists)" {
					err := os.WriteFile(fullOutputPath, []byte("this is a file"), 0644)
					require.NoError(t, err, "Failed to create conflicting file for test: %s", tt.name)
				}
			}

			var consoleOutput string
			var instanceLog *XMLog
			var err error

			consoleOutput = captureStdOutput(func() {
				instanceLog, err = NewXMLog(fullOutputPath, tt.verbose, tt.defaultLevel)
				if err == nil && tt.logFn != nil {
					tt.logFn(instanceLog)
					if tt.expectedFileNamePrefix != "" {
						time.Sleep(250 * time.Millisecond)
					}
				}
			})

			if tt.wantErr {
				assert.Error(t, err, "Test: %s - Expected an error", tt.name)
				t.Logf("Test: %s - Received expected error: %v", tt.name, err)
				return
			}
			require.NoError(t, err, "Test: %s - NewXMLog returned an unexpected error", tt.name)
			require.NotNil(t, instanceLog, "Test: %s - NewXMLog instance should not be nil", tt.name)

			if tt.expectedConsolePattern != "" {
				trimmedConsoleOutput := strings.TrimRight(consoleOutput, "\r\n") + "\n"
				t.Logf("TEST_CASE: %s", tt.name)
				t.Logf("QUOTED_ACTUAL_CONSOLE_OUTPUT:\n%q\nEND_QUOTED_ACTUAL", trimmedConsoleOutput)
				t.Logf("EXPECTED_CONSOLE_PATTERN:\n%s\nEND_EXPECTED_PATTERN", tt.expectedConsolePattern)
				assert.Regexp(t, regexp.MustCompile(tt.expectedConsolePattern), trimmedConsoleOutput, "Test: %s - Console output did not match pattern.\nActual Console Output (trimmed):\n%s\nExpected Pattern:\n%s", tt.name, trimmedConsoleOutput, tt.expectedConsolePattern)
			} else if strings.TrimSpace(consoleOutput) != "" && tt.logFn != nil {
				t.Logf("Test: %s - Console output was (no specific pattern expected but got output):\n%s", tt.name, consoleOutput)
			}

			if tt.expectedFileNamePrefix != "" {
				require.NotEmpty(t, fullOutputPath, "fullOutputPath must be set if expecting file output for test: %s", tt.name)
				var logFilePath string
				var foundLogFileAndContentMatched bool

				deadline := time.Now().Add(tt.fileCheckTimeout)
				var lastReadError error
				var lastContent string
				var lastFileSize int64
				var checkedFiles []string

				for time.Now().Before(deadline) {
					files, readDirErr := os.ReadDir(fullOutputPath)
					if readDirErr != nil {
						lastReadError = fmt.Errorf("error reading dir %s: %w", fullOutputPath, readDirErr)
						time.Sleep(100 * time.Millisecond)
						continue
					}
					checkedFiles = filesToNames(files)

					foundThisIteration := false
					for _, f := range files {
						if strings.HasPrefix(f.Name(), tt.expectedFileNamePrefix+".") && !f.IsDir() {
							currentCandidatePath := filepath.Join(fullOutputPath, f.Name())
							info, statErr := os.Stat(currentCandidatePath)
							if statErr != nil {
								lastReadError = fmt.Errorf("stat error for %s: %w", currentCandidatePath, statErr)
								continue
							}
							lastFileSize = info.Size()

							if info.Size() > 0 {
								contentBytes, readContentErr := os.ReadFile(currentCandidatePath)
								if readContentErr != nil {
									lastReadError = fmt.Errorf("error reading content of %s: %w", currentCandidatePath, readContentErr)
									continue
								}
								fileContentStr := strings.TrimRight(string(contentBytes), "\r\n") + "\n"
								lastContent = fileContentStr

								if tt.expectedFilePattern != "" {
									matched, _ := regexp.MatchString(tt.expectedFilePattern, fileContentStr)
									if matched {
										logFilePath = currentCandidatePath
										foundLogFileAndContentMatched = true
										foundThisIteration = true
										break
									}
								} else {
									logFilePath = currentCandidatePath
									foundLogFileAndContentMatched = true
									foundThisIteration = true
									break
								}
							}
						}
					}
					if foundThisIteration && foundLogFileAndContentMatched {
						break
					}
					time.Sleep(200 * time.Millisecond)
				}

				if !foundLogFileAndContentMatched {
					errMsg := fmt.Sprintf("Test: %s - Expected log file with prefix '%s' and matching content not found or conditions not met in %s within %v.",
						tt.name, tt.expectedFileNamePrefix, fullOutputPath, tt.fileCheckTimeout)
					if lastReadError != nil {
						errMsg += fmt.Sprintf(" Last error: %v.", lastReadError)
					}
					errMsg += fmt.Sprintf(" Files found: %v.", checkedFiles)
					if lastFileSize > 0 && tt.expectedFilePattern != "" {
						isMatch, _ := regexp.MatchString(tt.expectedFilePattern, lastContent)
						if !isMatch {
							errMsg += fmt.Sprintf(" Last file size: %d. Last content of a candidate file did NOT match pattern. Content:\n---\n%s\n---\nExpected pattern:\n%s", lastFileSize, lastContent, tt.expectedFilePattern)
						} else {
							errMsg += fmt.Sprintf(" Last file size: %d. Content was present but other conditions (e.g. multiple candidate files and none matched fully early) might have failed.", lastFileSize)
						}
					} else if lastFileSize > 0 {
						errMsg += fmt.Sprintf(" Last file size: %d, but no content pattern was checked or content was empty/mismatched.", lastFileSize)
					} else if lastFileSize == 0 && tt.expectedFileNamePrefix != "" {
						errMsg += fmt.Sprintf(" Last file size was 0 for candidate files named like '%s.*'.", tt.expectedFileNamePrefix)
					}
					require.True(t, foundLogFileAndContentMatched, errMsg)
				}

				t.Logf("Test: %s - Found and validated log file: %s", tt.name, logFilePath)

				if tt.expectFileLink {
					linkPath := filepath.Join(fullOutputPath, tt.expectedFileNamePrefix)
					targetFileBase := filepath.Base(logFilePath)
					linkTarget, errLink := os.Readlink(linkPath)
					if runtime.GOOS == "windows" && errLink != nil {
						t.Logf("Test: %s - Readlink on %s failed on Windows (often expected): %v", tt.name, linkPath, errLink)
						linkInfo, statErr := os.Stat(linkPath)
						if statErr == nil && !linkInfo.IsDir() && linkInfo.Size() > 0 {
							t.Logf("Test: %s - Link path %s on Windows is a file, content check (if pattern exists).", tt.name, linkPath)
							if tt.expectedFilePattern != "" {
								linkContentBytes, linkReadErr := os.ReadFile(linkPath)
								if linkReadErr == nil {
									trimmedLinkContent := strings.TrimRight(string(linkContentBytes), "\r\n") + "\n"
									assert.Regexp(t, regexp.MustCompile(tt.expectedFilePattern), trimmedLinkContent, "Test: %s - Link path (file) content mismatch on Windows for %s", tt.name, linkPath)
								} else {
									t.Errorf("Test: %s - Failed to read content of link path %s on Windows: %v", tt.name, linkPath, linkReadErr)
								}
							}
						}
					} else {
						require.NoError(t, errLink, "Test: %s - Expected symlink %s to exist and be readable. Files: %v", tt.name, linkPath, checkedFiles)
						assert.Equal(t, targetFileBase, filepath.Base(linkTarget), "Test: %s - Symlink %s should point to %s, but points to %s", tt.name, linkPath, targetFileBase, linkTarget)
					}
				}
			}
		})
	}
}

func filesToNamesOrError(t *testing.T, dirPath string) []string {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		t.Logf("Error reading dir %s for file listing: %v", dirPath, err)
		return []string{"<error reading dir>"}
	}
	return filesToNames(entries)
}

func TestXMLog_StructuredMethods(t *testing.T) {
	logger := logrus.New() // Create a base logger for the hook
	hook := &testHook{}
	logger.AddHook(hook)
	logger.SetOutput(io.Discard)
	logger.SetLevel(logrus.TraceLevel)
	xmLogger, err := NewXMLog("", true, logrus.TraceLevel)
	require.NoError(t, err)
	require.NotNil(t, xmLogger)

	xmLogger.SetLevel(logrus.TraceLevel)

	xmLogger.Logger.ReplaceHooks(make(logrus.LevelHooks))
	xmLogger.Logger.AddHook(hook)

	xmLogger.SetReportCaller(true)

	testCases := []struct {
		name           string
		logAction      func(xl *XMLog)
		expectedMsg    string
		expectedLvl    logrus.Level
		expectedFields logrus.Fields
		expectCaller   bool
	}{
		{
			name: "InfoPipeline with dynamic fields",
			logAction: func(xl *XMLog) {
				xl.InfoPipeline("deploy-app", "Pipeline started", logrus.Fields{"version": "1.0", "user": "admin"})
			},
			expectedMsg:    "Pipeline started",
			expectedLvl:    logrus.InfoLevel,
			expectedFields: logrus.Fields{common.PipelineName: "deploy-app", "version": "1.0", "user": "admin"},
			expectCaller:   true,
		},
		{
			name: "ErrorfTask with error and format args",
			logAction: func(xl *XMLog) {
				testErr := errors.New("network timeout")
				xl.ErrorfTask("user-sync", testErr, "Sync failed after %d retries for ID %s", 3, "user123")
			},
			expectedMsg:    "Sync failed after 3 retries for ID user123",
			expectedLvl:    logrus.ErrorLevel,
			expectedFields: logrus.Fields{common.TaskName: "user-sync", logrus.ErrorKey: errors.New("network timeout")},
			expectCaller:   true,
		},
		{
			name: "DebugfModule with multiple args",
			logAction: func(xl *XMLog) {
				xl.DebugfModule("auth-svc", "Request from %s to %s", "client-A", "resource-B")
			},
			expectedMsg:    "Request from client-A to resource-B",
			expectedLvl:    logrus.DebugLevel,
			expectedFields: logrus.Fields{common.ModuleName: "auth-svc"},
			expectCaller:   true,
		},
		{
			name: "WarnStep with no dynamic fields",
			logAction: func(xl *XMLog) {
				xl.WarnStep("data-validation", "Schema mismatch detected")
			},
			expectedMsg:    "Schema mismatch detected",
			expectedLvl:    logrus.WarnLevel,
			expectedFields: logrus.Fields{common.StepName: "data-validation"},
			expectCaller:   true,
		},
		{
			name: "ErrorNode with nil error and dynamic fields",
			logAction: func(xl *XMLog) {
				xl.ErrorNode("node-3", nil, "State inconsistent", logrus.Fields{"expected": "ready", "actual": "pending"})
			},
			expectedMsg:    "State inconsistent",
			expectedLvl:    logrus.ErrorLevel,
			expectedFields: logrus.Fields{common.NodeName: "node-3", "expected": "ready", "actual": "pending"},
			expectCaller:   true,
		},
		{
			name: "Generic Info method",
			logAction: func(xl *XMLog) {
				xl.Info("This is a generic info message")
			},
			expectedMsg:    "This is a generic info message",
			expectedLvl:    logrus.InfoLevel,
			expectedFields: logrus.Fields{},
			expectCaller:   true,
		},
		{
			name: "Generic Errorf method with error field from WithFields",
			logAction: func(xl *XMLog) {
				xl.WithField(logrus.ErrorKey, errors.New("generic error")).Errorf("Details: %s", "something went wrong")
			},
			expectedMsg:    "Details: something went wrong",
			expectedLvl:    logrus.ErrorLevel,
			expectedFields: logrus.Fields{logrus.ErrorKey: errors.New("generic error")},
			expectCaller:   true,
		},
		{
			name: "LogAtLevel Trace",
			logAction: func(xl *XMLog) {
				xl.LogAtLevel(logrus.TraceLevel, "Trace via LogAtLevel", logrus.Fields{"detail": "specifics"})
			},
			expectedMsg:    "Trace via LogAtLevel",
			expectedLvl:    logrus.TraceLevel,
			expectedFields: logrus.Fields{"detail": "specifics"},
			expectCaller:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			hook.Reset()
			tc.logAction(xmLogger)

			entry := hook.LastEntry()
			require.NotNil(t, entry, "Expected a log entry to be captured")

			assert.Equal(t, tc.expectedLvl, entry.Level, "Log level mismatch")
			assert.Equal(t, tc.expectedMsg, entry.Message, "Log message mismatch")

			assertFieldsEqual(t, tc.expectedFields, entry.Data, "Log fields mismatch")

			if tc.expectCaller {
				assert.NotNil(t, entry.Caller, "Expected caller data in log entry for: %s", tc.name)
				if entry.Caller != nil {
					t.Logf("Test: %s, Caller: %s:%d %s", tc.name, filepath.Base(entry.Caller.File), entry.Caller.Line, filepath.Base(entry.Caller.Function))
					if tc.name == "Generic Errorf method with error field from WithFields" {
						assert.True(t, strings.HasSuffix(entry.Caller.File, "logger_test.go"),
							"Caller file for chained direct log methods should be from the test file (logger_test.go) for test: %s. Got: %s", tc.name, entry.Caller.File)
					} else {
						assert.True(t, strings.HasSuffix(entry.Caller.File, "logger.go"),
							"Caller file for XMLog methods should be from the logger package (logger.go) for test: %s. Got: %s", tc.name, entry.Caller.File)
					}
				}
			} else {
				assert.Nil(t, entry.Caller, "Expected no caller data in log entry for: %s", tc.name)
			}
		})
	}
}

func assertFieldsEqual(t *testing.T, expected, actual logrus.Fields, msgAndArgs ...interface{}) {
	t.Helper()
	if len(expected) == 0 && len(actual) == 0 {
		return
	}

	cleanedActual := make(logrus.Fields)
	for k, v := range actual {
		if k == "file" || k == "func" || k == "line" {
			if _, expect := expected[k]; !expect {
				continue
			}
		}
		cleanedActual[k] = v
	}

	expectedComparable := make(map[string]interface{})
	for k, v := range expected {
		if err, ok := v.(error); ok {
			expectedComparable[k] = err.Error()
		} else {
			expectedComparable[k] = v
		}
	}

	actualComparable := make(map[string]interface{})
	for k, v := range cleanedActual {
		if err, ok := v.(error); ok {
			actualComparable[k] = err.Error()
		} else {
			actualComparable[k] = v
		}
	}
	assert.Equal(t, expectedComparable, actualComparable, msgAndArgs...)
}

func TestLogRotation(t *testing.T) {
	logDir := createTestLogDir(t)
	defer os.RemoveAll(logDir)

	logger, err := NewXMLog(logDir, false, logrus.InfoLevel)
	require.NoError(t, err)
	require.NotNil(t, logger)

	logFileName := "instance.log"

	// Log on "day 1"
	logger.Info("Message on day 1")
	time.Sleep(100 * time.Millisecond)

	filesDay1, err := os.ReadDir(logDir)
	require.NoError(t, err)
	t.Logf("Files after day 1 log: %v", filesToNames(filesDay1))
	foundRotatedDay1 := false
	var day1RotatedName string
	for _, f := range filesDay1 {
		if strings.HasPrefix(f.Name(), logFileName+".") {
			foundRotatedDay1 = true
			day1RotatedName = f.Name()
			break
		}
	}
	require.True(t, foundRotatedDay1, "Expected a rotated log file after first log message")
	assertFileContains(t, filepath.Join(logDir, day1RotatedName), "Message on day 1")

	symlinkPath := filepath.Join(logDir, logFileName)
	linkTarget, err := os.Readlink(symlinkPath)
	if runtime.GOOS == "windows" && err != nil {
		t.Logf("Readlink on %s failed on Windows (often expected): %v", symlinkPath, err)
	} else {
		require.NoError(t, err, "Failed to read symlink")
		assert.Equal(t, day1RotatedName, filepath.Base(linkTarget), "Symlink should point to the day 1 rotated file")
	}

	logger.Info("Message on day 2 (same actual day, testing continued logging)")
	time.Sleep(100 * time.Millisecond)
	assertFileContains(t, filepath.Join(logDir, day1RotatedName), "Message on day 2")
}

func TestFatalLogsAndExits(t *testing.T) {
	logger := logrus.New()
	hook := &testHook{}
	logger.AddHook(hook)
	logger.SetOutput(io.Discard)

	originalExitFunc := logrus.StandardLogger().ExitFunc
	var exitCode int = -1
	mockExit := func(code int) {
		exitCode = code
	}
	logger.ExitFunc = mockExit
	defer func() { logrus.StandardLogger().ExitFunc = originalExitFunc }()

	xmLogger := &XMLog{Logger: logger}

	xmLogger.Fatal("This is a fatal error")

	assert.Equal(t, 1, exitCode, "Expected os.Exit to be called with code 1")
	entry := hook.LastEntry()
	require.NotNil(t, entry)
	assert.Equal(t, logrus.FatalLevel, entry.Level)
	assert.Equal(t, "This is a fatal error", entry.Message)
}

func TestPanicLogsAndPanics(t *testing.T) {
	logger := logrus.New()
	hook := &testHook{}
	logger.AddHook(hook)
	logger.SetOutput(io.Discard)

	xmLogger := &XMLog{Logger: logger}

	defer func() {
		r := recover()
		require.NotNil(t, r, "Expected a panic")

		var panicMsg string
		if entry, ok := r.(*logrus.Entry); ok {
			panicMsg = entry.Message
		} else if msg, ok := r.(string); ok {
			panicMsg = msg
		} else {
			panicMsg = fmt.Sprintf("%v", r)
		}

		assert.Equal(t, "This is a panic situation", panicMsg, "Panic message mismatch")

		entry := hook.LastEntry()
		require.NotNil(t, entry)
		assert.Equal(t, logrus.PanicLevel, entry.Level)
		assert.Equal(t, "This is a panic situation", entry.Message)
	}()

	xmLogger.Panic("This is a panic situation")
}

func TestXMLog_WithFields_Chaining(t *testing.T) {
	hook := &testHook{}
	baseLogger := logrus.New()
	baseLogger.SetOutput(io.Discard)
	baseLogger.AddHook(hook)
	baseLogger.SetLevel(logrus.DebugLevel)

	xl := &XMLog{Logger: baseLogger}

	xl.WithField("key1", "val1").WithField("key2", "val2").Info("Chained fields info")
	entry := hook.LastEntry()
	require.NotNil(t, entry)
	assert.Equal(t, logrus.InfoLevel, entry.Level)
	assert.Equal(t, "Chained fields info", entry.Message)
	assertFieldsEqual(t, logrus.Fields{"key1": "val1", "key2": "val2"}, entry.Data)
	hook.Reset()

	fieldsMap := logrus.Fields{"map_key1": "map_val1", "map_key2": 123}
	xl.WithFields(fieldsMap).Debug("WithFields map")
	entry = hook.LastEntry()
	require.NotNil(t, entry)
	assert.Equal(t, logrus.DebugLevel, entry.Level)
	assert.Equal(t, "WithFields map", entry.Message)
	assertFieldsEqual(t, fieldsMap, entry.Data)
}

func (xl *XMLog) DebugfWithFields(fields logrus.Fields, format string, args ...interface{}) {
	xl.WithFields(fields).Debugf(format, args...)
}
func (xl *XMLog) ErrorfWithFields(fields logrus.Fields, format string, args ...interface{}) {
	xl.WithFields(fields).Errorf(format, args...)
}
