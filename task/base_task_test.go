package task

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/mensylisir/xmcores/connector"
	"github.com/mensylisir/xmcores/logger" // Initialize global logger
	"github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/runner"
	"github.com/mensylisir/xmcores/step"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock Step Implementations ---

type MockStep struct {
	step.BaseStep
	executeErr error // Error to return from Execute()
	postErr    error // Error to return from Post()
	executeLog string
	initErr    error
}

func NewMockStep(name, desc string, executeErr, postErr, initErr error, executeLogOutput string) *MockStep {
	return &MockStep{
		BaseStep: step.BaseStep{
			StepName:        name,
			StepDescription: desc,
		},
		executeErr: executeErr,
		postErr:    postErr,
		executeLog: executeLogOutput,
		initErr:    initErr,
	}
}

func (ms *MockStep) Init(rt runtime.Runtime, log *logrus.Entry) error {
	log.Infof("MockStep %s Init called", ms.Name())
	return ms.initErr
}

func (ms *MockStep) Execute(rt runtime.Runtime, log *logrus.Entry) (output string, success bool, err error) {
	log.Infof("MockStep %s Execute called", ms.Name())
	if ms.executeErr != nil {
		log.Errorf("MockStep %s Execute returning error: %v", ms.Name(), ms.executeErr)
		return ms.executeLog, false, ms.executeErr
	}
	log.Infof("MockStep %s Execute succeeded", ms.Name())
	return ms.executeLog, true, nil
}

func (ms *MockStep) Post(rt runtime.Runtime, log *logrus.Entry, executeErr error) error {
	log.Infof("MockStep %s Post called", ms.Name())
	if ms.postErr != nil {
		log.Errorf("MockStep %s Post returning error: %v", ms.Name(), ms.postErr)
		return ms.postErr
	}
	log.Infof("MockStep %s Post succeeded", ms.Name())
	return nil
}

var _ step.Step = (*MockStep)(nil)

// --- Mock Runtime Implementation ---

type MockRuntime struct {
	ignoreErrorValue bool
	log              *logrus.Entry
	hosts            []connector.Host
	// We don't need a full connector/runner for these tests as steps are mocked
}

func NewMockRuntime(ignoreError bool) *MockRuntime {
	// Initialize logger for tests (can be a simple one)
	testLogger := logrus.New()
	testLogger.SetOutput(io.Discard) // Don't show log output during tests unless debugging
	entry := logrus.NewEntry(testLogger)

	// Add a dummy host for step initialization that checks for hosts
	dummyHost := connector.NewHost()
	dummyHost.SetName("mockHost")
	dummyHost.SetAddress("localhost")


	return &MockRuntime{
		ignoreErrorValue: ignoreError,
		log:              entry,
		hosts:            []connector.Host{dummyHost},
	}
}

func (mr *MockRuntime) GetPrimaryConnector() connector.Connector { return nil }
func (mr *MockRuntime) GetPrimaryRunner() runner.Runner       { return nil }
func (mr *MockRuntime) WorkDir() string                         { return "/tmp/testwork" }
func (mr *MockRuntime) ObjectName() string                      { return "TestObject" }
func (mr *MockRuntime) Verbose() bool                           { return false } // Not relevant for this test focus
func (mr *MockRuntime) IgnoreError() bool                       { return mr.ignoreErrorValue }
func (mr *MockRuntime) AllHosts() []connector.Host              { return mr.hosts }
func (mr *MockRuntime) RoleHosts() map[string][]connector.Host  { return nil }
func (mr *MockRuntime) DeprecatedHosts() []connector.Host       { return nil }
func (mr *MockRuntime) GetHostConnectorAndRunner(host connector.Host) (connector.Connector, runner.Runner, error) {
	// Return a mock runner if steps actually try to use it, though our mock steps don't
	return nil, &MockRunner{}, nil
}

var _ runtime.Runtime = (*MockRuntime)(nil)

// MockRunner to satisfy GetHostConnectorAndRunner if needed by any step init/logic
type MockRunner struct{}
func (m *MockRunner) Run(ctx context.Context, command string) (string, string, int, error) { return "", "", 0, nil }
func (m *MockRunner) SudoRun(ctx context.Context, command string) (string, string, int, error) { return "", "", 0, nil }


// --- Stdout Capture Helper ---

func captureStdout(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// This WaitGroup is to ensure that all writes to the pipe are finished
	// before we restore os.Stdout and read from the pipe.
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		// This copy operation will block until the writer (w) is closed.
		// Or until it encounters an error.
		var buf bytes.Buffer
		_, err := io.Copy(&buf, r)
		if err != nil {
			// Handle error if needed, e.g. t.Errorf("Failed to copy from pipe: %v", err)
			// For now, we assume it works or test fails due to other reasons.
		}
		// Store the captured output. This part is tricky due to goroutine.
		// A channel might be better, or pass a *bytes.Buffer to this func.
		// For simplicity, we'll rely on closing w to signal EOF to Copy.
		// The captured string is returned by the outer function.
		// This means we must read from `buf` in the main goroutine after `w.Close()`.
		// The current implementation directly returns string which implies reading here.
		// Let's adjust to a more common pattern.
	}()

	// The actual function call that prints to stdout
	f()

	// Close the writer end of the pipe. This signals EOF to the io.Copy in the goroutine.
	w.Close()

	// Restore os.Stdout
	os.Stdout = old

	// Wait for the copying goroutine to finish.
	// wg.Wait() // This would deadlock if io.Copy blocks indefinitely without w.Close()
	// The reading part needs to be after w.Close()

	var buf bytes.Buffer
	// r is already closed on the write side by w.Close()
	// We need to read the remaining data from r
	_, err := io.Copy(&buf, r)
	if err != nil {
		// This indicates an issue with pipe reading itself.
		// For a test, we might panic or log a fatal error.
		fmt.Fprintf(os.Stderr, "Error reading from pipe: %v\n", err)
	}
	r.Close() // Close the read end.

	return buf.String()
}


// --- Test Function ---

func TestBaseTask_Execute_ErrorHandlingAndSummary(t *testing.T) {
	// Initialize the global logger for consistent log handling in tests
	// You might want a more sophisticated setup or pass logger to BaseTask if it's refactored
	logger.Init(logrus.InfoLevel, true, "") // Adjust level as needed for debugging tests

	stepSucceed := NewMockStep("StepSucceed", "This step always succeeds", nil, nil, nil, "Success output")
	stepFailExecute := NewMockStep("StepFailExecute", "This step fails in Execute", errors.New("execute failed"), nil, nil, "Execute failure output")
	stepFailPost := NewMockStep("StepFailPost", "This step fails in Post", nil, errors.New("post failed"), nil, "Post failure output")
	stepFailBoth := NewMockStep("StepFailBoth", "This step fails in Execute and Post", errors.New("exec failed badly"), errors.New("post also failed"), nil, "Both failure output")
	stepSucceed2 := NewMockStep("StepSucceedAgain", "This step also succeeds", nil, nil, nil, "Another success output")


	task := NewBaseTask("TestSummaryTask", "A task to test summary reporting")
	task.AddStep(stepSucceed)
	task.AddStep(stepFailExecute)
	task.AddStep(stepFailPost)
	task.AddStep(stepFailBoth)
	task.AddStep(stepSucceed2)

	testLogEntry := logger.Log.WithField("test", "TestBaseTask_Execute")

	// Scenario 1: IgnoreError = false
	t.Run("IgnoreErrorFalse", func(t *testing.T) {
		mockRtFalse := NewMockRuntime(false)

		// Initialize steps (important!)
		err := task.Init(mockRtFalse, testLogEntry)
		require.NoError(t, err, "Task Init should succeed")

		var capturedOutput string
		// There's a problem with the simple captureStdout if fmt.Printf is used inside goroutines
		// or if the logger also writes to stdout. For now, let's assume it works for direct fmt.Printf.
		// A more robust solution involves passing an io.Writer to the functions that print.
		// The logger used by BaseTask might also print to os.Stdout.
		// Redirecting logger output for the test duration:
		originalLogOut := logger.Log.Out
		var logBuf bytes.Buffer
		logger.Log.SetOutput(&logBuf) // Capture log output
		defer logger.Log.SetOutput(originalLogOut) // Restore

		// Capture os.Stdout for fmt.Printf calls within BaseTask.Execute's summary
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		execErr := task.Execute(mockRtFalse, testLogEntry)

		w.Close()
		os.Stdout = oldStdout
		var stdOutSummaryBuf bytes.Buffer
		io.Copy(&stdOutSummaryBuf, r)
		r.Close()
		capturedOutput = stdOutSummaryBuf.String()


		require.Error(t, execErr, "Execute should return an error when IgnoreError is false and steps fail")
		assert.Contains(t, execErr.Error(), "task TestSummaryTask completed with one or more errors", "Error message should indicate task failure")

		t.Logf("Captured Stdout (IgnoreError=false):\n%s", capturedOutput)
		t.Logf("Captured Log Output (IgnoreError=false):\n%s", logBuf.String())


		require.Contains(t, capturedOutput, "--- Task Execution Summary for 'TestSummaryTask' ---", "Summary header missing")
		assert.Contains(t, capturedOutput, "Step 'StepSucceed': SUCCEEDED", "StepSucceed status incorrect")
		assert.Contains(t, capturedOutput, "Step 'StepFailExecute': FAILED (Error: execute failed)", "StepFailExecute status/error incorrect")
		assert.Contains(t, capturedOutput, "Step 'StepFailPost': FAILED (Error: post-execute error: post failed)", "StepFailPost status/error incorrect")
		assert.Contains(t, capturedOutput, "Step 'StepFailBoth': FAILED (Error: exec failed badly; post-execute error: post also failed)", "StepFailBoth status/error incorrect")
		assert.Contains(t, capturedOutput, "Step 'StepSucceedAgain': SUCCEEDED", "StepSucceedAgain status incorrect")
	})

	// Scenario 2: IgnoreError = true
	t.Run("IgnoreErrorTrue", func(t *testing.T) {
		mockRtTrue := NewMockRuntime(true)

		// Initialize steps (important!)
		err := task.Init(mockRtTrue, testLogEntry)
		require.NoError(t, err, "Task Init should succeed")

		originalLogOut := logger.Log.Out
		var logBuf bytes.Buffer
		logger.Log.SetOutput(&logBuf)
		defer logger.Log.SetOutput(originalLogOut)

		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		execErr := task.Execute(mockRtTrue, testLogEntry)

		w.Close()
		os.Stdout = oldStdout
		var stdOutSummaryBuf bytes.Buffer
		io.Copy(&stdOutSummaryBuf, r)
		r.Close()
		capturedOutput := stdOutSummaryBuf.String()

		require.NoError(t, execErr, "Execute should return nil when IgnoreError is true, even if steps fail")

		t.Logf("Captured Stdout (IgnoreError=true):\n%s", capturedOutput)
		t.Logf("Captured Log Output (IgnoreError=true):\n%s", logBuf.String())

		require.Contains(t, capturedOutput, "--- Task Execution Summary for 'TestSummaryTask' ---", "Summary header missing")
		assert.Contains(t, capturedOutput, "Step 'StepSucceed': SUCCEEDED", "StepSucceed status incorrect")
		assert.Contains(t, capturedOutput, "Step 'StepFailExecute': FAILED (Error: execute failed)", "StepFailExecute status/error incorrect")
		assert.Contains(t, capturedOutput, "Step 'StepFailPost': FAILED (Error: post-execute error: post failed)", "StepFailPost status/error incorrect")
		assert.Contains(t, capturedOutput, "Step 'StepFailBoth': FAILED (Error: exec failed badly; post-execute error: post also failed)", "StepFailBoth status/error incorrect")
		assert.Contains(t, capturedOutput, "Step 'StepSucceedAgain': SUCCEEDED", "StepSucceedAgain status incorrect")
	})
}
