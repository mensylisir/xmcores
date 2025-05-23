// util_test.go
package util

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"text/template"
)

// Helper to create a temporary file with content
func createTempFile(t *testing.T, dir, pattern, content string) string {
	t.Helper()
	tmpFile, err := os.CreateTemp(dir, pattern)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	if _, err := tmpFile.WriteString(content); err != nil {
		_ = tmpFile.Close() // Attempt to close before failing
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}
	return tmpFile.Name()
}

func TestRender(t *testing.T) {
	tmplStr := "Hello, {{.Name}}! You are {{.Age}} years old."
	tmpl, err := template.New("test").Parse(tmplStr)
	if err != nil {
		t.Fatalf("Failed to parse template: %v", err)
	}

	tests := []struct {
		name      string
		tmpl      *template.Template
		variables Data
		want      string
		wantErr   bool
	}{
		{
			name:      "Simple render",
			tmpl:      tmpl,
			variables: Data{"Name": "World", "Age": 30},
			want:      "Hello, World! You are 30 years old.",
			wantErr:   false,
		},
		{
			name:      "Missing variable",
			tmpl:      tmpl,
			variables: Data{"Name": "Test"}, // Age is missing, template will insert <no value>
			want:      "Hello, Test! You are <no value> years old.",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(tt.tmpl, tt.variables)
			if (err != nil) != tt.wantErr {
				t.Errorf("Render() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Render() got = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderString(t *testing.T) {
	tests := []struct {
		name      string
		tmplStr   string
		variables Data
		want      string
		wantErr   bool
	}{
		{
			name:      "Simple render string",
			tmplStr:   "Amount: {{.Value}}",
			variables: Data{"Value": 100},
			want:      "Amount: 100",
			wantErr:   false,
		},
		{
			name:      "Invalid template string",
			tmplStr:   "Hello, {{.Name", // Unclosed brace
			variables: Data{"Name": "Test"},
			want:      "",
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderString(tt.tmplStr, tt.variables)
			if (err != nil) != tt.wantErr {
				t.Errorf("RenderString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("RenderString() got = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMustRender(t *testing.T) {
	tmplStr := "Value: {{.X}}"
	tmpl, _ := template.New("must").Parse(tmplStr)

	t.Run("Successful MustRender", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("MustRender() panicked unexpectedly: %v", r)
			}
		}()
		got := MustRender(tmpl, Data{"X": "Test"})
		if got != "Value: Test" {
			t.Errorf("MustRender() got = %q, want %q", got, "Value: Test")
		}
	})
}

func TestMustRenderString(t *testing.T) {
	t.Run("Successful MustRenderString", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("MustRenderString() panicked unexpectedly: %v", r)
			}
		}()
		got := MustRenderString("Item: {{.Item}}", Data{"Item": "Book"})
		if got != "Item: Book" {
			t.Errorf("MustRenderString() got = %q, want %q", got, "Item: Book")
		}
	})

	t.Run("Panic on invalid template MustRenderString", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("MustRenderString() did not panic on invalid template")
			}
		}()
		_ = MustRenderString("Item: {{.Item", Data{"Item": "Book"}) // Invalid template
	})
}

func TestHome(t *testing.T) {
	// Reset homeDir and homeDirErr for consistent testing if Home is called multiple times across test functions.
	// This is generally not necessary if tests are independent, but good for this specific cache test.
	homeDirOnce = sync.Once{} // Reset the Once for re-evaluation in this test context
	homeDir = ""
	homeDirErr = nil

	home, err := Home()
	if err != nil {
		if runtime.GOOS != "windows" && os.Getenv("HOME") == "" && (os.Getenv("USER") == "" && os.Getenv("LOGNAME") == "") {
			t.Logf("Home() failed, but HOME and USER/LOGNAME env vars are not set: %v. This might be expected in some CI.", err)
		} else if runtime.GOOS == "windows" && os.Getenv("USERPROFILE") == "" && (os.Getenv("HOMEDRIVE") == "" || os.Getenv("HOMEPATH") == "") {
			t.Logf("Home() failed, but USERPROFILE and HOMEDRIVE/HOMEPATH env vars are not set: %v. This might be expected in some CI.", err)
		} else {
			t.Errorf("Home() error = %v", err)
		}
		return
	}
	if home == "" {
		t.Errorf("Home() returned an empty string")
	}
	t.Logf("Home directory found: %s", home)

	// Test caching
	homeAgain, errAgain := Home()
	if errAgain != err { // Compare original error state too
		t.Errorf("Home() on second call error = %v, want error %v", errAgain, err)
	}
	if homeAgain != home {
		t.Errorf("Home() on second call got %q, want %q (caching test)", homeAgain, home)
	}
}

func TestNormalizeArgs(t *testing.T) {
	tests := []struct {
		name             string
		baseArgs         map[string]string
		overrideArgsList []string
		wantSlice        []string
		wantMap          map[string]string
	}{
		{
			name:             "Nil inputs",
			baseArgs:         nil,
			overrideArgsList: nil,
			wantSlice:        []string{},
			wantMap:          map[string]string{},
		},
		{
			name:             "Empty inputs",
			baseArgs:         map[string]string{},
			overrideArgsList: []string{},
			wantSlice:        []string{},
			wantMap:          map[string]string{},
		},
		{
			name:             "Only base args",
			baseArgs:         map[string]string{"key1": "baseVal1", "key2": "baseVal2"},
			overrideArgsList: []string{},
			wantSlice:        []string{"key1=baseVal1", "key2=baseVal2"},
			wantMap:          map[string]string{"key1": "baseVal1", "key2": "baseVal2"},
		},
		{
			name:             "Only override args",
			baseArgs:         nil,
			overrideArgsList: []string{"keyA=overrideA", "keyB=overrideB"},
			wantSlice:        []string{"keyA=overrideA", "keyB=overrideB"},
			wantMap:          map[string]string{"keyA": "overrideA", "keyB": "overrideB"},
		},
		{
			name:             "Mix and override",
			baseArgs:         map[string]string{"common": "base", "onlyBase": "valBase"},
			overrideArgsList: []string{"common=override", "onlyOverride=valOverride", "noValueKey", "=emptyKeyVal"},
			wantSlice:        []string{"common=override", "onlyBase=valBase", "onlyOverride=valOverride"},
			wantMap:          map[string]string{"common": "override", "onlyBase": "valBase", "onlyOverride": "valOverride"},
		},
		{
			name:             "Override with spaces in value, key trimmed",
			baseArgs:         nil,
			overrideArgsList: []string{"  keySpaced  =  value with spaces  "},
			wantSlice:        []string{"keySpaced=  value with spaces  "},
			wantMap:          map[string]string{"keySpaced": "  value with spaces  "},
		},
		{
			name:             "Empty key in override (ignored)",
			baseArgs:         map[string]string{"a": "b"},
			overrideArgsList: []string{"=valForEmptyKey"},
			wantSlice:        []string{"a=b"},
			wantMap:          map[string]string{"a": "b"},
		},
		{
			name:             "Override with no value part",
			baseArgs:         map[string]string{"x": "y"},
			overrideArgsList: []string{"justkey"}, // This is ignored by current map logic
			wantSlice:        []string{"x=y"},
			wantMap:          map[string]string{"x": "y"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSlice, gotMap := NormalizeArgs(tt.baseArgs, tt.overrideArgsList)
			sort.Strings(gotSlice)
			sort.Strings(tt.wantSlice)

			if !reflect.DeepEqual(gotSlice, tt.wantSlice) {
				t.Errorf("NormalizeArgs() gotSlice = %v, want %v", gotSlice, tt.wantSlice)
			}
			if !reflect.DeepEqual(gotMap, tt.wantMap) {
				t.Errorf("NormalizeArgs() gotMap = %v, want %v", gotMap, tt.wantMap)
			}
		})
	}
}

func TestRound(t *testing.T) {
	tests := []struct {
		name      string
		val       float64
		precision int
		want      float64
	}{
		{"Round down standard", 3.14159, 2, 3.14}, // Standard library behavior, 3.14159 * 100 = 314.159 + 0.5 = 314.659 => floor = 314 => 3.14
		{"Round up standard", 3.14159, 3, 3.142},  // 3.14159 * 1000 = 3141.59 + 0.5 = 3142.09 => floor = 3142 => 3.142
		{"No change", 3.0, 2, 3.0},
		{"Integer precision", 123.456, 0, 123.0},
		{"Negative precision round half down", 123.456, -1, 120.0}, // 123.456 * 0.1 = 12.3456 + 0.5 = 12.8456 => floor = 12 => 120
		{"Negative precision round half up", 127.0, -1, 130.0},     // 127 * 0.1 = 12.7 + 0.5 = 13.2 => floor = 13 => 130
		{"Zero precision", 0.123, 1, 0.1},
		{"Large precision", 1.23456789, 7, 1.2345679},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Round(tt.val, tt.precision); got != tt.want {
				t.Errorf("Round() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestArchAlias(t *testing.T) {
	tests := []struct {
		name   string
		goArch string
		want   string
	}{
		{"amd64", ArchAMD64, "x86_64"},
		{"arm64", ArchARM64, "aarch64"},
		{"arm", ArchARM, "armhf"},
		{"386", Arch386, "i386"},
		{"ppc64le", ArchPPC64LE, "ppc64le"},
		{"s390x", ArchS390X, "s390x"},
		{"riscv64", ArchRISCV64, "riscv64"},
		{"unknown", "unknown_arch", ""},
		{"empty", "", ""},
		{"AMD64 uppercase", "AMD64", "x86_64"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ArchAlias(tt.goArch); got != tt.want {
				t.Errorf("ArchAlias() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGoArchFromAlias(t *testing.T) {
	testCases := []struct { // Renamed to avoid conflict if 'tests' is used above
		name  string
		alias string
		want  string
	}{
		{"x86_64", "x86_64", ArchAMD64},
		{"aarch64", "aarch64", ArchARM64},
		{"armhf", "armhf", ArchARM},
		{"armv7l", "armv7l", ArchARM},
		{"armel", "armel", ArchARM},
		{"i386", "i386", Arch386},
		{"i686", "i686", Arch386},
		{"x86", "x86", Arch386},
		{"x64", "x64", ArchAMD64},
		{"ppc64le alias", "ppc64le", ArchPPC64LE},
		{"s390x alias", "s390x", ArchS390X},
		{"riscv64 alias", "riscv64", ArchRISCV64},
		{"unknown", "unknown_alias", ""},
		{"empty", "", ""},
		{"X86_64 uppercase", "X86_64", ArchAMD64},
	}
	for _, tc := range testCases { // Use tc (test case) here
		t.Run(tc.name, func(t *testing.T) {
			if got := GoArchFromAlias(tc.alias); got != tc.want {
				t.Errorf("GoArchFromAlias() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFileDirExists(t *testing.T) {
	tempDir := t.TempDir()

	if !DirExists(tempDir) {
		t.Errorf("DirExists() failed for existing temp directory: %s", tempDir)
	}
	if DirExists(filepath.Join(tempDir, "nonexistent_dir")) {
		t.Errorf("DirExists() succeeded for nonexistent directory")
	}

	tempFile := createTempFile(t, tempDir, "testfile*.txt", "hello")
	// No explicit defer os.Remove(tempFile) needed as t.TempDir() handles cleanup of its contents.

	if !FileExists(tempFile) {
		t.Errorf("FileExists() failed for existing temp file: %s", tempFile)
	}
	if FileExists(filepath.Join(tempDir, "nonexistent_file.txt")) {
		t.Errorf("FileExists() succeeded for nonexistent file")
	}

	if FileExists(tempDir) {
		t.Errorf("FileExists() succeeded for a directory path: %s", tempDir)
	}
	if DirExists(tempFile) {
		t.Errorf("DirExists() succeeded for a file path: %s", tempFile)
	}
}

func TestEnsureDir(t *testing.T) {
	baseTempDir := t.TempDir()
	testDirPath := filepath.Join(baseTempDir, "test_ensure", "subdir")

	err := EnsureDir(testDirPath)
	if err != nil {
		t.Fatalf("EnsureDir() failed to create directory: %v", err)
	}
	if !DirExists(testDirPath) {
		t.Errorf("EnsureDir() created directory, but DirExists() reports false")
	}

	err = EnsureDir(testDirPath) // Call again on existing dir
	if err != nil {
		t.Errorf("EnsureDir() failed for existing directory: %v", err)
	}

	tempFile := createTempFile(t, baseTempDir, "filepart*.txt", "content")
	err = EnsureDir(filepath.Join(tempFile, "sub")) // Path where a component is a file
	if err == nil {
		t.Errorf("EnsureDir() did not fail when part of the path is a file")
	}
}

func TestGetenvOrDefault(t *testing.T) {
	const testEnvKey = "MY_TEST_ENV_VAR_XYZ123_UTIL" // Unique key
	const defaultValue = "this_is_default_val"
	const setValue = "this_is_a_set_value"

	originalValue, wasSet := os.LookupEnv(testEnvKey) // Save original state
	t.Cleanup(func() {                                // Restore original state after test
		if wasSet {
			os.Setenv(testEnvKey, originalValue)
		} else {
			os.Unsetenv(testEnvKey)
		}
	})

	os.Unsetenv(testEnvKey)
	if got := GetenvOrDefault(testEnvKey, defaultValue); got != defaultValue {
		t.Errorf("GetenvOrDefault() got %q, want %q when var not set", got, defaultValue)
	}

	os.Setenv(testEnvKey, "")
	if got := GetenvOrDefault(testEnvKey, defaultValue); got != defaultValue {
		t.Errorf("GetenvOrDefault() got %q, want %q when var is empty string", got, defaultValue)
	}

	os.Setenv(testEnvKey, setValue)
	if got := GetenvOrDefault(testEnvKey, defaultValue); got != setValue {
		t.Errorf("GetenvOrDefault() got %q, want %q when var is set", got, setValue)
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name      string
		s         string
		maxLength int
		ellipsis  string
		want      string
	}{
		{"No truncation", "hello", 10, "...", "hello"},
		{"Exact length", "hello", 5, "...", "hello"},
		{"Simple truncation", "hello world", 8, "...", "hello..."},
		{"Short maxLength for ellipsis", "hello world", 3, "...", "..."},
		{"maxLength smaller than ellipsis", "hello world", 2, "...", ".."},
		{"maxLength zero", "hello world", 0, "...", ""},
		{"Empty string", "", 5, "...", ""},
		{"Empty ellipsis", "hello world", 5, "", "hello"},
		{"maxLength negative", "hello world", -1, "...", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TruncateString(tt.s, tt.maxLength, tt.ellipsis); got != tt.want {
				t.Errorf("TruncateString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestContainsString(t *testing.T) {
	slice := []string{"apple", "banana", "cherry"}
	tests := []struct {
		name  string
		slice []string // Allow testing different slices
		str   string
		want  bool
	}{
		{"Contains existing", slice, "banana", true},
		{"Does not contain", slice, "grape", false},
		{"Empty string in slice", []string{"a", "", "c"}, "", true},
		{"Search empty string in slice without it", slice, "", false},
		{"Empty slice", []string{}, "a", false},
		{"Nil slice", nil, "a", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ContainsString(tt.slice, tt.str); got != tt.want {
				t.Errorf("ContainsString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUniqueStrings(t *testing.T) {
	tests := []struct {
		name  string
		slice []string
		want  []string
	}{
		{"Empty slice", []string{}, []string{}},
		{"All unique", []string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{"Duplicates", []string{"a", "b", "a", "c", "b", "b"}, []string{"a", "b", "c"}},
		{"Duplicates at end", []string{"x", "y", "x", "x"}, []string{"x", "y"}},
		{"All same", []string{"z", "z", "z"}, []string{"z"}},
		{"Nil slice", nil, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UniqueStrings(tt.slice)
			if tt.slice == nil && len(got) != 0 { // Special check for nil input to ensure non-nil empty slice output
				t.Errorf("UniqueStrings(nil) = %v, want []string{}", got)
			} else if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("UniqueStrings() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCombineErrors(t *testing.T) {
	err1 := fmt.Errorf("error one")
	err2 := fmt.Errorf("error two")

	tests := []struct {
		name string
		errs []error
		want string
	}{
		{"No errors", []error{}, ""},
		{"Nil errors", []error{nil, nil}, ""},
		{"One error", []error{err1}, "error one"},
		{"Multiple errors", []error{err1, err2}, "error one; error two"},
		{"Mixed nil and errors", []error{nil, err1, nil, err2}, "error one; error two"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := CombineErrors(tt.errs...)
			if tt.want == "" {
				if gotErr != nil {
					t.Errorf("CombineErrors() got error %v, want nil", gotErr)
				}
			} else {
				if gotErr == nil {
					t.Errorf("CombineErrors() got nil, want error containing %q", tt.want)
				} else if gotErr.Error() != tt.want {
					t.Errorf("CombineErrors() got error string %q, want %q", gotErr.Error(), tt.want)
				}
			}
		})
	}
}

func TestPtrHelpers(t *testing.T) {
	t.Run("StringPtr", func(t *testing.T) {
		s := "hello"
		sp := StringPtr(s)
		if sp == nil {
			t.Fatal("StringPtr returned nil")
		}
		if *sp != s {
			t.Errorf("StringPtr dereferenced = %q, want %q", *sp, s)
		}
		*sp = "world" // Modify the pointed-to value
		if s == "world" {
			t.Error("StringPtr modification affected original 's' (should not for string, as it creates a copy of 's' on stack for StringPtr's argument)")
			// StringPtr takes 's' by value, so 's' in this scope is unchanged. The pointer 'sp' points to a new memory location.
			// This test as written actually checks if `s` is NOT modified, which is correct.
			// If we wanted to show that the value *pointed to* by sp changed:
			// sCopy := "hello"
			// sp := StringPtr(sCopy)
			// *sp = "world"
			// if *sp != "world" { t.Error("Value pointed to by sp did not change") }
		}
		s = "original again" // Reset s for clarity that sp has its own copy
		if *sp == s {
			t.Error("StringPtr points to original stack var s after s changed; should point to its own copy")
		}

	})

	t.Run("IntPtr", func(t *testing.T) {
		i := 123
		ip := IntPtr(i)
		if ip == nil {
			t.Fatal("IntPtr returned nil")
		}
		if *ip != i {
			t.Errorf("IntPtr dereferenced = %d, want %d", *ip, i)
		}
	})

	t.Run("BoolPtr", func(t *testing.T) {
		b := true
		bp := BoolPtr(b)
		if bp == nil {
			t.Fatal("BoolPtr returned nil")
		}
		if *bp != b {
			t.Errorf("BoolPtr dereferenced = %v, want %v", *bp, b)
		}
	})
}

func TestFirstNonEmpty(t *testing.T) {
	tests := []struct {
		name string
		strs []string
		want string
	}{
		{"All empty", []string{"", "", ""}, ""},
		{"First non-empty", []string{"", "hello", "world"}, "hello"},
		{"First is non-empty", []string{"first", "second"}, "first"},
		{"Last non-empty", []string{"", "", "last"}, "last"},
		{"No args", []string{}, ""},
		{"Single empty", []string{""}, ""},
		{"Single non-empty", []string{"single"}, "single"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FirstNonEmpty(tt.strs...); got != tt.want {
				t.Errorf("FirstNonEmpty() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFileReadWrite(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test_rw_file.txt")
	content := "Hello, KubeSphere Utilities!"
	var perm os.FileMode = 0644

	t.Run("WriteStringToFile and ReadFileToString", func(t *testing.T) {
		err := WriteStringToFile(filePath, content, perm)
		if err != nil {
			t.Fatalf("WriteStringToFile() failed: %v", err)
		}

		if !FileExists(filePath) {
			t.Fatalf("FileExists() reports false after WriteStringToFile")
		}

		readContent, err := ReadFileToString(filePath)
		if err != nil {
			t.Fatalf("ReadFileToString() failed: %v", err)
		}
		if readContent != content {
			t.Errorf("ReadFileToString() got %q, want %q", readContent, content)
		}

		_, err = ReadFileToString(filepath.Join(tempDir, "nonexistent.txt"))
		if err == nil {
			t.Errorf("ReadFileToString() expected error for nonexistent file, got nil")
		}
	})

	t.Run("WriteStringToFile error (bad path)", func(t *testing.T) {
		err := WriteStringToFile(tempDir, "should fail", perm) // tempDir is a directory
		if err == nil {
			t.Errorf("WriteStringToFile() did not return an error when writing to a directory path")
		} else {
			errMsg := err.Error()
			// Check for common OS-specific error messages
			if !(strings.Contains(strings.ToLower(errMsg), "is a directory") ||
				strings.Contains(strings.ToLower(errMsg), "access is denied") || // Windows
				strings.Contains(strings.ToLower(errMsg), "permission denied")) { // Unix-like
				t.Logf("WriteStringToFile() to a directory path returned error as expected, but message was: %q. This can be OS-dependent.", errMsg)
			}
		}
	})
}
