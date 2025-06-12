package runtime

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/mensylisir/xmcores/connector"
	"github.com/mensylisir/xmcores/runner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock Connector ---
type mockConnector struct {
	id        string
	cfg       connector.Config
	closed    bool
	closeChan chan bool // To signal that Close was called in async tests
	fail      bool      // If true, simulates a connection failure
}

func newMockConnector(cfg connector.Config, failConnection bool) (connector.Connector, error) {
	if failConnection {
		return nil, fmt.Errorf("mock connector: simulated connection failure for host %s", cfg.Address)
	}
	return &mockConnector{
		id:        fmt.Sprintf("%s:%d", cfg.Address, cfg.Port),
		cfg:       cfg,
		closeChan: make(chan bool, 1), // Buffered channel
		fail:      failConnection,
	}, nil
}

func (mc *mockConnector) Exec(ctx context.Context, cmd string) ([]byte, []byte, int, error) { return nil, nil, 0, nil }
func (mc *mockConnector) PExec(ctx context.Context, cmd string, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, error) { return 0, nil }
func (mc *mockConnector) UploadFile(ctx context.Context, localPath string, remotePath string) error { return nil }
func (mc *mockConnector) DownloadFile(ctx context.Context, remotePath string, localPath string) error { return nil }
func (mc *mockConnector) Fetch(ctx context.Context, remotePath string) (io.ReadCloser, error) { return nil, nil }
func (mc *mockConnector) Scp(ctx context.Context, localReader io.Reader, remotePath string, sizeHint int64, mode os.FileMode) error { return nil }
func (mc *mockConnector) StatRemote(ctx context.Context, remotePath string) (os.FileInfo, error) { return nil, nil }
func (mc *mockConnector) RemoteFileExist(ctx context.Context, remotePath string) (bool, error) { return false, nil }
func (mc *mockConnector) RemoteDirExist(ctx context.Context, remotePath string) (bool, error) { return false, nil }
func (mc *mockConnector) MkDirAll(ctx context.Context, remotePath string, mode os.FileMode) error { return nil }
func (mc *mockConnector) Chmod(ctx context.Context, remotePath string, mode os.FileMode) error { return nil }
func (mc *mockConnector) GetConfig() connector.Config { return mc.cfg }
func (mc *mockConnector) Close() error {
	mc.closed = true
	if mc.closeChan != nil {
		// Non-blocking send in case channel is not listened to or already full
		select {
		case mc.closeChan <- true:
		default:
		}
	}
	return nil
}

// --- Mock Runner ---
type mockRunner struct {
	conn connector.Connector
}

func newMockRunner(conn connector.Connector) runner.Runner {
	return &mockRunner{conn: conn}
}
func (mr *mockRunner) Run(ctx context.Context, command string) (string, string, int, error) { return "", "", 0, nil }
func (mr *mockRunner) SudoRun(ctx context.Context, command string) (string, string, int, error) { return "", "", 0, nil }


// --- Test Suite ---

func TestRuntime_NewRuntime(t *testing.T) {
	cfg := Config{WorkDir: "/tmp/mywork", ObjectName: "TestObj"}
	rt, err := NewRuntime(cfg)
	require.NoError(t, err)
	require.NotNil(t, rt)

	assert.Equal(t, "/tmp/mywork", rt.WorkDir())
	assert.Equal(t, "TestObj", rt.ObjectName())
	assert.Empty(t, rt.AllHosts())

	// Test default workdir
	cfgNoWorkDir := Config{ObjectName: "TestObj2"}
	rt2, err2 := NewRuntime(cfgNoWorkDir)
	require.NoError(t, err2)
	assert.Equal(t, "./.xm_workdir", rt2.WorkDir())
}

func TestRuntime_GetHostConnectorAndRunner_Caching(t *testing.T) {
	host1 := connector.NewHost()
	host1.SetName("host1")
	host1.SetAddress("192.168.1.1")
	host1.SetPort(22)
	host1.SetUser("user1")
	host1.SetPassword("pass1")
	require.NoError(t, host1.Validate())

	rtCfg := Config{AllHosts: []connector.Host{host1}}
	rt, _ := NewRuntime(rtCfg)

	// For this test, we need to ensure NewSSHConnector is not actually called,
	// or we mock its behavior. Since modifying runtime.go to inject a factory is out of scope,
	// we rely on the fact that a *real* NewSSHConnector would attempt connection.
	// We can use a dummy address that won't resolve or connect quickly to see behavior.
	// However, a better unit test would mock the connector.NewSSHConnector itself.
	// Let's assume for now that if it *were* to be called multiple times, it would create new objects.
	// The test below thus checks if the runtime *avoids* calling it multiple times.

	c1, r1, err1 := rt.GetHostConnectorAndRunner(host1)
	// This will likely fail if 192.168.1.1:22 is not connectable by test environment.
	// So, we can't require.NoError(t, err1) without a live SSH server or proper mock.
	// The focus is on caching: if c1 is not nil, c2 should be the same.
	if err1 != nil {
		t.Logf("GetHostConnectorAndRunner (call 1) for host1 failed as expected without live SSH: %v", err1)
		// If the first call fails, the resource is not cached. Subsequent calls will retry.
		// The current caching logic only caches on successful creation.
		// So, this test needs a way to make NewSSHConnector succeed.
		// This test will be more meaningful if NewSSHConnector is mocked or runtime uses a factory.
		t.Skip("Skipping caching test for connector/runner due to inability to mock NewSSHConnector without code change or live SSH server.")
		return
	}
	require.NotNil(t, c1)
	require.NotNil(t, r1)

	// Call 2 for host1 - should be cached
	c2, r2, err2 := rt.GetHostConnectorAndRunner(host1)
	require.NoError(t, err2) // This should also be no error if c1 was fine
	assert.Same(t, c1, c2, "Connector for host1 should be cached and the same instance")
	assert.Same(t, r1, r2, "Runner for host1 should be cached and the same instance")

	host2 := connector.NewHost()
	host2.SetName("host2")
	host2.SetAddress("192.168.1.2") // Different address
	host2.SetPort(22)
	host2.SetUser("user2")
	host2.SetPassword("pass2")
	require.NoError(t, host2.Validate())

	// Add host2 to runtime after initial creation (not typical but for testing)
	if baseRt, ok := rt.(*baseRuntime); ok {
		baseRt.allHosts = append(baseRt.allHosts, host2)
	} else {
		t.Fatalf("Runtime is not *baseRuntime, cannot modify internal state for test")
	}


	c3, r3, err3 := rt.GetHostConnectorAndRunner(host2)
	if err3 != nil {
		t.Logf("GetHostConnectorAndRunner (call 1) for host2 failed: %v", err3)
		t.Skip("Skipping further caching test for host2 due to connection failure.")
		return
	}
	require.NotNil(t, c3)
	require.NotNil(t, r3)
	assert.NotSame(t, c1, c3, "Connector for host2 should be different from host1")


	c4, r4, err4 := rt.GetHostConnectorAndRunner(host1)
	require.NoError(t, err4)
	assert.Same(t, c1, c4, "Connector for host1 should still be the originally cached instance")
	assert.Same(t, r1, r4, "Runner for host1 should still be the originally cached instance")

	require.NoError(t, c1.Close())
	require.NoError(t, c3.Close())
}


func TestRuntime_GetHostConnectorAndRunner_ErrorHandling(t *testing.T) {
	hostInvalidCfg := connector.NewHost()
	hostInvalidCfg.SetName("invalidConfigHost")
	hostInvalidCfg.SetAddress("10.0.0.1")
	// Missing User, which connector.NewConnection validates and causes NewSSHConnector to fail.

	rtCfg := Config{AllHosts: []connector.Host{hostInvalidCfg}}
	rt, _ := NewRuntime(rtCfg)

	_, _, err := rt.GetHostConnectorAndRunner(hostInvalidCfg)
	require.Error(t, err, "Expected an error when NewSSHConnector fails due to invalid config")
	assert.Contains(t, err.Error(), "Username is required", "Error message should indicate missing username")
	t.Logf("Got expected error for invalid config: %v", err)

	_, _, errNilHost := rt.GetHostConnectorAndRunner(nil)
	require.Error(t, errNilHost)
	assert.Contains(t, errNilHost.Error(), "host cannot be nil")

	hostEmptyID := connector.NewHost()
	hostEmptyID.SetAddress("10.0.0.2")
	hostEmptyID.SetPort(22)
	hostEmptyID.SetUser("test")
	hostEmptyID.SetPassword("test")
	// Name (ID) is empty
	_, _, errEmptyID := rt.GetHostConnectorAndRunner(hostEmptyID)
	require.Error(t, errEmptyID)
	assert.Contains(t, errEmptyID.Error(), "host ID is empty")
}

func TestRuntime_GetHostConnectorAndRunner_Concurrency(t *testing.T) {
	hostConc := connector.NewHost()
	hostConc.SetName("host_concurrency_test")
	hostConc.SetAddress("127.0.0.1")
	hostConc.SetPort(2222) // Non-standard, likely to fail connection quickly
	hostConc.SetUser("concUser")
	hostConc.SetPassword("concPass")
	require.NoError(t, hostConc.Validate())

	rtCfg := Config{AllHosts: []connector.Host{hostConc}}
	rt, _ := NewRuntime(rtCfg)

	var wg sync.WaitGroup
	numGoroutines := 20 // Increased goroutines

	connectors := make(chan connector.Connector, numGoroutines)
	errorsChan := make(chan error, numGoroutines) // Renamed to avoid conflict

	// Initial call may fail if 127.0.0.1:2222 is not available.
	// The test focuses on the runtime's locking, not actual connection success.
	firstC, _, firstErr := rt.GetHostConnectorAndRunner(hostConc)
	if firstErr != nil {
		t.Logf("Initial GetHostConnectorAndRunner failed (expected if dummy host not connectable): %v", firstErr)
	}

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(routineID int) {
			defer wg.Done()
			t.Logf("Goroutine %d: Calling GetHostConnectorAndRunner", routineID)
			c, _, err := rt.GetHostConnectorAndRunner(hostConc)
			if err != nil {
				t.Logf("Goroutine %d: Error: %v", routineID, err)
				errorsChan <- err
				return
			}
			t.Logf("Goroutine %d: Success", routineID)
			connectors <- c
		}(i)
	}

	wg.Wait()
	close(connectors)
	close(errorsChan)

	var retrievedConnectors []connector.Connector
	for c := range connectors {
		retrievedConnectors = append(retrievedConnectors, c)
	}
	var retrievedErrors []error
	for e := range errorsChan {
		retrievedErrors = append(retrievedErrors, e)
	}

	if firstErr != nil { // Initial call failed
		assert.Len(t, retrievedConnectors, 0, "No connectors should be successful if the first attempt failed and it's not cached")
		assert.Len(t, retrievedErrors, numGoroutines, "All goroutines should report an error if the first attempt failed")
	} else { // Initial call succeeded
		assert.Len(t, retrievedErrors, 0, "No errors expected in goroutines if first call succeeded")
		assert.Len(t, retrievedConnectors, numGoroutines, "All goroutines should have retrieved a connector")
		for _, c := range retrievedConnectors {
			assert.Same(t, firstC, c, "All goroutines should get the same cached connector instance")
		}
		require.NoError(t, firstC.Close())
	}
	t.Logf("Concurrency test finished. Connectors retrieved: %d, Errors: %d", len(retrievedConnectors), len(retrievedErrors))
}


func TestRuntime_Getters(t *testing.T) {
	// Use mockConnector which doesn't try to connect
	mockConnInstance, _ := newMockConnector(connector.Config{Address: "primary"}, false)

	host1 := connector.NewHost(); host1.SetName("h1"); host1.SetAddress("1.1.1.1"); host1.SetPort(22); host1.SetUser("u1"); host1.SetPassword("p1")
	host2 := connector.NewHost(); host2.SetName("h2"); host2.SetAddress("2.2.2.2"); host2.SetPort(22); host2.SetUser("u2"); host2.SetPassword("p2")

	all := []connector.Host{host1, host2}
	roles := map[string][]connector.Host{"roleA": {host1}}
	deprecated := []connector.Host{host2}

	rtCfg := Config{
		PrimaryConnector: mockConnInstance,
		PrimaryRunner:    newMockRunner(nil),
		WorkDir:          "/my/work",
		ObjectName:       "GetterTest",
		Verbose:          true,
		IgnoreError:      true,
		AllHosts:         all,
		RoleHosts:        roles,
		DeprecatedHosts:  deprecated,
	}
	rt, _ := NewRuntime(rtCfg)

	assert.Equal(t, rtCfg.PrimaryConnector, rt.GetPrimaryConnector())
	assert.Equal(t, rtCfg.PrimaryRunner, rt.GetPrimaryRunner())
	assert.Equal(t, "/my/work", rt.WorkDir())
	assert.Equal(t, "GetterTest", rt.ObjectName())
	assert.True(t, rt.Verbose())
	assert.True(t, rt.IgnoreError())

	assert.EqualValues(t, all, rt.AllHosts(), "AllHosts getter mismatch")

	// Test that modifications to the slice returned by AllHosts() do not affect the internal slice
    allHostsReturned := rt.AllHosts()
    if len(allHostsReturned) > 0 {
         // Try to modify a field of the first host in the returned slice
         // This test is more about Go's slice/struct semantics than the getter itself.
         // If Host is a struct, allHostsReturned contains copies.
         // If Host was *Host (pointer), then this would test if the getter returns copies of pointers or same pointers.
         // For structs, this modification won't affect rt.allHosts.
        allHostsReturned[0].SetName("modifiedInReturnedCopy")
    }
	assert.Equal(t, "h1", rt.AllHosts()[0].GetName(), "Internal AllHosts should not be modified by changes to the returned slice's elements")


	assert.EqualValues(t, roles, rt.RoleHosts(), "RoleHosts getter mismatch")
	assert.EqualValues(t, deprecated, rt.DeprecatedHosts(), "DeprecatedHosts getter mismatch")
}
