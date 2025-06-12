package util

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"sort"
	"strings"
	"sync"
	"text/template"

	"github.com/pkg/errors"
)

// Data is a generic map type for template rendering context.
type Data map[string]interface{}

// Render executes the given template with the provided variables.
func Render(tmpl *template.Template, variables Data) (string, error) {
	var buf strings.Builder
	if err := tmpl.Execute(&buf, variables); err != nil {
		return "", errors.Wrap(err, "failed to render template")
	}
	return buf.String(), nil
}

// RenderString parses and executes the given template string with the provided variables.
func RenderString(tmplStr string, variables Data) (string, error) {
	tmpl, err := template.New("").Parse(tmplStr)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse template string")
	}
	return Render(tmpl, variables)
}

// MustRender executes the given template, panicking on error.
func MustRender(tmpl *template.Template, variables Data) string {
	s, err := Render(tmpl, variables)
	if err != nil {
		panic(err)
	}
	return s
}

// MustRenderString parses and executes the given template string, panicking on error.
func MustRenderString(tmplStr string, variables Data) string {
	s, err := RenderString(tmplStr, variables)
	if err != nil {
		panic(err)
	}
	return s
}

var (
	homeDir     string
	homeDirErr  error
	homeDirOnce sync.Once
)

// Home returns the home directory for the current user.
// It caches the result for subsequent calls.
func Home() (string, error) {
	homeDirOnce.Do(func() {
		u, err := user.Current()
		if err == nil && u.HomeDir != "" {
			homeDir = u.HomeDir
			return
		}
		// Fallback if user.Current() fails or doesn't provide HomeDir
		if "windows" == runtime.GOOS {
			homeDir, homeDirErr = homeWindows()
		} else {
			homeDir, homeDirErr = homeUnix()
		}
	})
	return homeDir, homeDirErr
}

func homeUnix() (string, error) {
	if home := os.Getenv("HOME"); home != "" {
		return home, nil
	}

	var stdout bytes.Buffer
	// Using 'getent passwd $(id -u)' is often more robust than 'eval echo ~$USER'
	// as it doesn't rely on shell expansion of $USER, which might not be set.
	// However, `id -u` itself might not be available everywhere.
	// Sticking to the original if `user.Current()` and `os.Getenv("HOME")` fail.
	cmd := exec.Command("sh", "-c", "eval echo ~$USER")
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", errors.Wrap(err, "failed to run shell command for home directory")
	}

	result := strings.TrimSpace(stdout.String())
	if result == "" {
		return "", errors.New("blank output when reading home directory via shell")
	}
	return result, nil
}

func homeWindows() (string, error) {
	drive := os.Getenv("HOMEDRIVE")
	path := os.Getenv("HOMEPATH")
	home := drive + path
	if drive == "" || path == "" {
		home = os.Getenv("USERPROFILE")
	}
	if home == "" {
		return "", errors.New("HOMEDRIVE, HOMEPATH, and USERPROFILE environment variables are blank")
	}
	return home, nil
}

// NormalizeArgs merges a base map of arguments with a list of override arguments (in "key=value" format).
// It returns a sorted slice of "key=value" strings representing the final merged arguments,
// and the final merged map.
// Arguments in overrideArgsList take precedence over baseArgs.
func NormalizeArgs(baseArgs map[string]string, overrideArgsList []string) ([]string, map[string]string) {
	finalArgsMap := make(map[string]string)
	if baseArgs != nil {
		for k, v := range baseArgs {
			finalArgsMap[k] = v
		}
	}

	for _, arg := range overrideArgsList {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			if key == "" { // Skip if key is empty after trim
				continue
			}
			finalArgsMap[key] = parts[1]
		}
		// Args without '=' are ignored for map population, but will be in the slice if they were originally there.
		// The original implementation implicitly added them to the slice if they were in `args`.
		// This revised version only adds key=value pairs from the map to the final slice.
		// If non-key-value args from overrideArgsList should be preserved, the logic would need adjustment.
		// For now, assuming overrideArgsList is primarily for "key=value" overrides.
	}

	finalArgsSlice := make([]string, 0, len(finalArgsMap))
	for k, v := range finalArgsMap {
		finalArgsSlice = append(finalArgsSlice, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(finalArgsSlice)

	return finalArgsSlice, finalArgsMap
}

// Round returns the result of rounding 'val' to 'precision' decimal places.
// Precision can be negative or zero.
// Handles NaN and Inf inputs by returning them as is.
func Round(val float64, precision int) float64 {
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return val
	}
	p := math.Pow10(precision)
	if math.IsInf(p, 0) { // Precision too large or too small
		if p > 0 { // Inf
			return val // Cannot scale, effectively
		}
		return 0 // Pow10 resulted in 0
	}
	return math.Floor(val*p+0.5) / p
}

const (
	ArchAMD64   = "amd64"
	ArchARM64   = "arm64"
	ArchARM     = "arm"
	Arch386     = "386"
	ArchPPC64LE = "ppc64le"
	ArchS390X   = "s390x"
	ArchRISCV64 = "riscv64"
)

// ArchAlias returns a common alias for a given Go CPU architecture.
// For example, "amd64" becomes "x86_64".
// Returns an empty string if no common alias is defined or input is unknown.
func ArchAlias(goArch string) string {
	switch strings.ToLower(goArch) {
	case ArchAMD64:
		return "x86_64"
	case ArchARM64:
		return "aarch64"
	case ArchARM: // Common 32-bit ARM
		return "armhf" // or "armel", depends on convention, armhf is common for hard-float
	case Arch386:
		return "i386" // or "i686"
	case ArchPPC64LE:
		return "ppc64le"
	case ArchS390X:
		return "s390x"
	case ArchRISCV64:
		return "riscv64"
	default:
		return ""
	}
}

// GoArchFromAlias returns a Go architecture string from a common alias.
// For example, "x86_64" becomes "amd64".
// Returns an empty string if the alias is unknown.
func GoArchFromAlias(alias string) string {
	switch strings.ToLower(alias) {
	case "x86_64", "x64":
		return ArchAMD64
	case "aarch64", "arm64v8":
		return ArchARM64
	case "armhf", "armv7l": // armhf is a common hard-float ABI for ARMv7
		return ArchARM
	case "armel": // armel is a common soft-float ABI for ARMv5/ARMv6
		return ArchARM
	case "i386", "i686", "x86":
		return Arch386
	// Add other common aliases if needed
	case "ppc64le":
		return ArchPPC64LE
	case "s390x":
		return ArchS390X
	case "riscv64":
		return ArchRISCV64
	default:
		return ""
	}
}

// --- New Utility Functions ---

// FileExists checks if a file exists at the given path and is not a directory.
func FileExists(filePath string) bool {
	info, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		// Other error (e.g., permission denied), conservatively return false or handle error
		return false
	}
	return !info.IsDir()
}

// DirExists checks if a directory exists at the given path.
func DirExists(dirPath string) bool {
	info, err := os.Stat(dirPath)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		return false
	}
	return info.IsDir()
}

// EnsureDir creates a directory if it does not already exist.
// It's similar to `mkdir -p`.
func EnsureDir(dirPath string) error {
	err := os.MkdirAll(dirPath, os.ModePerm) // os.ModePerm (0777) is often masked by umask
	if err != nil {
		return errors.Wrapf(err, "failed to create directory %s", dirPath)
	}
	return nil
}

// GetenvOrDefault retrieves the value of the environment variable named by the key.
// If the variable is not present or empty, it returns the defaultValue.
func GetenvOrDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// TruncateString shortens a string to a maximum length, appending an ellipsis if truncation occurs.
// If the string is shorter than or equal to maxLength, it's returned unchanged.
// The ellipsis counts towards the maxLength. If maxLength is too small for the ellipsis,
// the string might be truncated more severely or only the ellipsis returned.
func TruncateString(s string, maxLength int, ellipsis string) string {
	if len(s) <= maxLength {
		return s
	}
	if maxLength <= len(ellipsis) {
		if maxLength < 0 {
			maxLength = 0
		}
		return ellipsis[:maxLength]
	}
	return s[:maxLength-len(ellipsis)] + ellipsis
}

// ContainsString checks if a slice of strings contains the given string.
func ContainsString(slice []string, str string) bool {
	for _, item := range slice {
		if item == str {
			return true
		}
	}
	return false
}

// UniqueStrings returns a new slice containing only the unique strings from the input slice.
// The order of the first appearance of each string is preserved.
func UniqueStrings(slice []string) []string {
	if len(slice) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(slice))
	result := make([]string, 0, len(slice))
	for _, str := range slice {
		if _, ok := seen[str]; !ok {
			seen[str] = struct{}{}
			result = append(result, str)
		}
	}
	return result
}

// CombineErrors takes multiple errors and returns a single error.
// If no errors or all errors are nil, it returns nil.
// Otherwise, it returns a new error that concatenates the messages of non-nil errors.
// Note: For Go 1.20+, `errors.Join` is the standard way to do this.
func CombineErrors(errs ...error) error {
	var errStrings []string
	for _, err := range errs {
		if err != nil {
			errStrings = append(errStrings, err.Error())
		}
	}
	if len(errStrings) == 0 {
		return nil
	}
	return fmt.Errorf(strings.Join(errStrings, "; "))
}

// StringPtr returns a pointer to the string value.
func StringPtr(s string) *string {
	return &s
}

// IntPtr returns a pointer to the int value.
func IntPtr(i int) *int {
	return &i
}

// BoolPtr returns a pointer to the bool value.
func BoolPtr(b bool) *bool {
	return &b
}

// FirstNonEmpty returns the first non-empty string from a list of strings.
// If all strings are empty, it returns an empty string.
func FirstNonEmpty(strs ...string) string {
	for _, s := range strs {
		if s != "" {
			return s
		}
	}
	return ""
}

// ReadFileToString reads the entire file into a string.
// Returns an empty string and an error if reading fails.
func ReadFileToString(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read file %s", filePath)
	}
	return string(data), nil
}

// WriteStringToFile writes a string to a file with the given permissions.
// Creates the file if it doesn't exist, truncates it if it does.
func WriteStringToFile(filePath string, content string, perm fs.FileMode) error {
	err := os.WriteFile(filePath, []byte(content), perm)
	if err != nil {
		return errors.Wrapf(err, "failed to write to file %s", filePath)
	}
	return nil
}

func IsErrPipeClosed(err error) bool {
	return errors.Is(err, os.ErrClosed) || // For os.Pipe
		errors.Is(err, io.ErrClosedPipe) || // For io.Pipe
		errors.Is(err, io.EOF) || // Often signals closed pipe from reader's perspective
		(err != nil && strings.Contains(err.Error(), "file already closed")) ||
		(err != nil && strings.Contains(err.Error(), "pipe already closed"))
}
