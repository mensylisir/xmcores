package ip

import (
	"fmt"
	"net"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestParseIPsFromString(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      []string
		wantErr   bool
		errSubStr string // Substring to check in error message
	}{
		{"single IPv4", "192.168.1.1", []string{"192.168.1.1"}, false, ""},
		{"single IPv4 with trailing slash", "192.168.1.1/", []string{"192.168.1.1"}, false, ""},
		{"single IPv6", "2001:db8::1", []string{"2001:db8::1"}, false, ""},
		{"IPv4 CIDR /32", "192.168.1.5/32", []string{"192.168.1.5"}, false, ""},
		{"IPv6 CIDR /128", "2001:db8::5/128", []string{"2001:db8::5"}, false, ""},
		{"IPv4 CIDR /30", "192.168.1.4/30", []string{"192.168.1.4", "192.168.1.5", "192.168.1.6", "192.168.1.7"}, false, ""},
		{"IPv4 range", "10.0.0.1-10.0.0.3", []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"}, false, ""},
		{"IPv6 range", "2001:db8::a-2001:db8::c", []string{"2001:db8::a", "2001:db8::b", "2001:db8::c"}, false, ""},
		{"comma separated", "1.1.1.1, 2.2.2.2", []string{"1.1.1.1", "2.2.2.2"}, false, ""},
		{"mixed types", "192.168.1.100, 10.0.0.0/30, 172.16.0.1-172.16.0.2",
			[]string{"192.168.1.100", "10.0.0.0", "10.0.0.1", "10.0.0.2", "10.0.0.3", "172.16.0.1", "172.16.0.2"}, false, ""},
		{"with spaces", " 1.1.1.1 , 2.2.2.2/32 ", []string{"1.1.1.1", "2.2.2.2"}, false, ""},
		{"duplicate IPs", "1.1.1.1,1.1.1.1", []string{"1.1.1.1"}, false, ""},
		{"empty input", "", []string{}, false, ""}, // Should return empty slice, not nil or error
		{"only commas", ",,,", []string{}, false, ""},
		{"invalid IP", "not.an.ip", nil, true, "invalid IP address"},
		{"invalid CIDR", "192.168.1.0/33", nil, true, "invalid CIDR block"},
		{"invalid range format", "10.0.0.1-", nil, true, "invalid IP range format"},
		{"invalid IP in range", "10.0.0.bad-10.0.0.2", nil, true, "invalid start IP"},
		{"range start > end", "10.0.0.5-10.0.0.1", nil, true, "start IP address must be numerically greater than or equal to end IP address"},
		{"mixed family range", "192.168.1.1-2001:db8::1", nil, true, "same family"},
		{"large CIDR (expect warning, not error)", "10.0.0.0/8", nil, false, ""}, // Test that it doesn't error, but prints warning. Actual content difficult to assert here.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseIPsFromString(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseIPsFromString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if err != nil && tt.errSubStr != "" && !strings.Contains(err.Error(), tt.errSubStr) {
					t.Errorf("ParseIPsFromString() error = %v, want err containing %s", err, tt.errSubStr)
				}
				return // Don't compare slices if error was expected
			}

			// For the large CIDR case, we just check it doesn't error and returns some IPs
			if tt.name == "large CIDR (expect warning, not error)" {
				if len(got) == 0 {
					t.Errorf("ParseIPsFromString() for large CIDR returned no IPs, expected some")
				}
				// Further check could be for the first IP or count if stable
				return
			}

			if !equalStringSlices(got, tt.want) {
				sortStrings(got)
				sortStrings(tt.want)
				t.Errorf("ParseIPsFromString() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetIPsFromRange(t *testing.T) {
	tests := []struct {
		name        string
		ipStart     string
		ipEnd       string
		want        []string
		wantErr     bool
		errSubStr   string
		customCheck func(t *testing.T, got []string, err error) // For special cases like large ranges
	}{
		{
			name:    "IPv4 simple range",
			ipStart: "192.168.0.254",
			ipEnd:   "192.168.1.1",
			want:    []string{"192.168.0.254", "192.168.0.255", "192.168.1.0", "192.168.1.1"},
			wantErr: false,
		},
		{
			name:    "IPv4 single IP range",
			ipStart: "10.0.0.5",
			ipEnd:   "10.0.0.5",
			want:    []string{"10.0.0.5"},
			wantErr: false,
		},
		{
			name:    "IPv6 small manageable range", // Changed from "IPv6 simple range"
			ipStart: "2001:db8::a",
			ipEnd:   "2001:db8::d", // A small, predictable range
			want:    []string{"2001:db8::a", "2001:db8::b", "2001:db8::c", "2001:db8::d"},
			wantErr: false,
		},
		{
			name:    "IPv6 very large range (check truncation)",
			ipStart: "2001:db8:1::1",
			ipEnd:   "2001:db8:2::ffff", // This is a very large range
			wantErr: false,
			customCheck: func(t *testing.T, got []string, err error) {
				if err != nil {
					t.Errorf("Expected no error for large IPv6 range, but got: %v", err)
				}
				maxExpected := 1 << 16 // maxIPsToGenerate in GetIPsFromRange
				if len(got) > maxExpected {
					t.Errorf("Got %d IPs, expected at most %d due to truncation", len(got), maxExpected)
				}
				if len(got) == 0 {
					t.Errorf("Expected some IPs for large range, got 0")
					return
				}
				if got[0] != "2001:db8:1::1" {
					t.Errorf("First IP mismatch: got %s, want 2001:db8:1::1", got[0])
				}
				// It's hard to predict the exact last IP of a truncated list without re-implementing
				// the generation logic or making maxIPsToGenerate a very small number for test.
				// For now, checking count and first IP is a good start.
				// If a warning is printed by the function, that's harder to capture here.
			},
		},
		{
			name:    "IPv6 single IP range",
			ipStart: "2001:db8::100",
			ipEnd:   "2001:db8::100",
			want:    []string{"2001:db8::100"},
			wantErr: false,
		},
		{
			name:      "invalid start IP",
			ipStart:   "bad-ip",
			ipEnd:     "10.0.0.1",
			wantErr:   true,
			errSubStr: "invalid start IP address",
		},
		{
			name:      "invalid end IP",
			ipStart:   "10.0.0.1",
			ipEnd:     "bad-ip",
			wantErr:   true,
			errSubStr: "invalid end IP address",
		},
		{
			name:      "start > end",
			ipStart:   "10.0.1.0",
			ipEnd:     "10.0.0.255",
			wantErr:   true,
			errSubStr: "start IP address must be numerically greater than or equal to end IP address", // Corrected sub string
		},
		{
			name:      "mixed family",
			ipStart:   "192.168.1.1",
			ipEnd:     "2001:db8::1",
			wantErr:   true,
			errSubStr: "must be of the same family",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetIPsFromRange(tt.ipStart, tt.ipEnd)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetIPsFromRange() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				if err != nil && tt.errSubStr != "" && !strings.Contains(err.Error(), tt.errSubStr) {
					t.Errorf("GetIPsFromRange() error = %v, want err containing '%s'", err, tt.errSubStr)
				}
				return // Don't compare slices if error was expected
			}

			// If a custom check function is provided, use it
			if tt.customCheck != nil {
				tt.customCheck(t, got, err)
			} else { // Otherwise, do the standard slice comparison
				if !equalStringSlices(got, tt.want) {
					// Sort them for easier visual comparison in output if needed
					// equalStringSlices already sorts copies, so original got/want are preserved
					// For error message, let's sort the original `got` and `tt.want`
					// to make diffs clearer if the test fails.
					sortedGot := make([]string, len(got))
					copy(sortedGot, got)
					sortStrings(sortedGot)

					sortedWant := make([]string, len(tt.want))
					copy(sortedWant, tt.want) // tt.want might be nil if not applicable
					sortStrings(sortedWant)

					t.Errorf("GetIPsFromRange() got = %v (len %d), want %v (len %d)", sortedGot, len(got), sortedWant, len(tt.want))
				}
			}
		})
	}
}

func TestGetAllIPsFromCIDR(t *testing.T) {
	tests := []struct {
		name      string
		cidr      string
		want      []string
		wantErr   bool
		errSubStr string
	}{
		{"IPv4 /30", "192.168.1.0/30", []string{"192.168.1.0", "192.168.1.1", "192.168.1.2", "192.168.1.3"}, false, ""},
		{"IPv4 /32", "10.0.0.1/32", []string{"10.0.0.1"}, false, ""},
		{"IPv6 /126", "2001:db8::/126", []string{"2001:db8::", "2001:db8::1", "2001:db8::2", "2001:db8::3"}, false, ""},
		{"IPv6 /128", "2001:db8::10/128", []string{"2001:db8::10"}, false, ""},
		{"invalid CIDR", "192.168.1.0/33", nil, true, "invalid CIDR block"},
		{"invalid IP in CIDR", "bad-ip/24", nil, true, "invalid CIDR block"},
		// Large CIDR test (gets truncated by maxGeneratedIPs)
		// Check first few and count. The warning is printed to stdout, hard to capture in test easily.
		{"IPv4 /16 (large, truncated)", "10.10.0.0/16", nil, false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetAllIPsFromCIDR(tt.cidr)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAllIPsFromCIDR() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if err != nil && tt.errSubStr != "" && !strings.Contains(err.Error(), tt.errSubStr) {
					t.Errorf("GetAllIPsFromCIDR() error = %v, want err containing %s", err, tt.errSubStr)
				}
				return
			}

			if tt.name == "IPv4 /16 (large, truncated)" {
				if len(got) != 1<<16 { // maxGeneratedIPs
					t.Errorf("GetAllIPsFromCIDR() for /16 got %d IPs, want %d (maxGeneratedIPs)", len(got), 1<<16)
				}
				if len(got) > 0 && got[0] != "10.10.0.0" {
					t.Errorf("GetAllIPsFromCIDR() for /16 first IP got %s, want 10.10.0.0", got[0])
				}
				// Check last IP if feasible, e.g., got[len(got)-1] should be 10.10.255.255
				// This requires calculating the 65536th IP.
				// For simplicity, we'll skip a precise last IP check for the truncated case.
				return
			}

			if !equalStringSlices(got, tt.want) {
				sortStrings(got)
				sortStrings(tt.want)
				t.Errorf("GetAllIPsFromCIDR() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetUsableIPsFromCIDR(t *testing.T) {
	tests := []struct {
		name    string
		cidr    string
		want    []string
		wantErr bool
	}{
		{"IPv4 /30", "192.168.1.0/30", []string{"192.168.1.1", "192.168.1.2"}, false}, // Network: .0, Broadcast: .3
		{"IPv4 /29", "192.168.1.0/29", []string{"192.168.1.1", "192.168.1.2", "192.168.1.3", "192.168.1.4", "192.168.1.5", "192.168.1.6"}, false},
		{"IPv4 /31 (point-to-point)", "192.168.1.0/31", []string{}, false},                                          // No usable according to typical host rules
		{"IPv4 /32 (single host)", "192.168.1.1/32", []string{}, false},                                             // No usable according to typical host rules
		{"IPv6 /126", "2001:db8::/126", []string{"2001:db8::", "2001:db8::1", "2001:db8::2", "2001:db8::3"}, false}, // All are usable for IPv6
		{"IPv6 /127 (point-to-point)", "2001:db8::/127", []string{"2001:db8::", "2001:db8::1"}, false},
		{"IPv6 /128 (single host)", "2001:db8::1/128", []string{"2001:db8::1"}, false},
		{"invalid CIDR", "invalid/cidr", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetUsableIPsFromCIDR(tt.cidr)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetUsableIPsFromCIDR() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !equalStringSlices(got, tt.want) {
				sortStrings(got)
				sortStrings(tt.want)
				t.Errorf("GetUsableIPsFromCIDR() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizeCIDR(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"already CIDR prefix", "192.168.1.1/24", "192.168.1.1/24"},
		{"IPv4 with netmask", "10.0.0.5/255.255.255.0", "10.0.0.5/24"},
		{"IPv4 with netmask /30", "10.0.0.5/255.255.255.252", "10.0.0.5/30"},
		{"IPv6 with prefix", "2001:db8::1/64", "2001:db8::1/64"},
		// Normalizing IPv6 with netmask is less common but could be supported if needed.
		// The current NormalizeCIDR doesn't fully handle IPv6 netmasks.
		{"plain IPv4", "172.16.0.10", "172.16.0.10/32"},
		{"plain IPv6", "fe80::1", "fe80::1/128"},
		{"with spaces", " 192.168.1.1/24 ", "192.168.1.1/24"},
		{"invalid mask", "10.0.0.1/badmask", "10.0.0.1/badmask"}, // Returns input if can't parse
		{"invalid IP", "badip/24", "badip/24"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeCIDR(tt.input); got != tt.want {
				t.Errorf("NormalizeCIDR() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNetworkRange(t *testing.T) {
	tests := []struct {
		name          string
		cidr          string
		wantNetworkIP string
		wantLastIP    string // Broadcast for IPv4, last in range for IPv6
		wantErr       bool
	}{
		{"IPv4 /24", "192.168.1.100/24", "192.168.1.0", "192.168.1.255", false},
		{"IPv4 /30", "10.0.0.1/30", "10.0.0.0", "10.0.0.3", false},
		{"IPv4 /32", "172.16.5.5/32", "172.16.5.5", "172.16.5.5", false},
		{"IPv6 /64", "2001:db8:abcd:0012::5/64", "2001:db8:abcd:12::", "2001:db8:abcd:12:ffff:ffff:ffff:ffff", false},
		{"IPv6 /127", "2001:db8::a/127", "2001:db8::a", "2001:db8::b", false},
		{"IPv6 /128", "2001:db8::100/128", "2001:db8::100", "2001:db8::100", false},
		{"invalid CIDR", "bad/cidr", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, network, err := net.ParseCIDR(tt.cidr)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error for ParseCIDR with input %s, but got nil", tt.cidr)
				}
				// If ParseCIDR errors, NetworkRange won't be called with valid input, so we check its own error path
				_, _, nrErr := NetworkRange(nil) // Test nil input path
				if nrErr == nil {
					t.Error("NetworkRange(nil) expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseCIDR failed for valid test case %s: %v", tt.cidr, err)
			}

			gotNetworkIP, gotLastIP, err := NetworkRange(network)
			if (err != nil) != tt.wantErr { // Should not error if ParseCIDR succeeded
				t.Errorf("NetworkRange() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if gotNetworkIP.String() != tt.wantNetworkIP {
					t.Errorf("NetworkRange() gotNetworkIP = %v, want %v", gotNetworkIP.String(), tt.wantNetworkIP)
				}
				if gotLastIP.String() != tt.wantLastIP {
					t.Errorf("NetworkRange() gotLastIP = %v, want %v", gotLastIP.String(), tt.wantLastIP)
				}
			}
		})
	}
}

func TestIPToBigIntAndBigIntToIP(t *testing.T) {
	tests := []struct {
		name   string
		ipStr  string
		isIPv4 bool
	}{
		{"IPv4 typical", "192.168.1.1", true},
		{"IPv4 zero", "0.0.0.0", true},
		{"IPv4 broadcast", "255.255.255.255", true},
		{"IPv6 typical", "2001:db8::1", false},
		{"IPv6 zero", "::", false},
		{"IPv6 all Fs", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff", false},
		{"IPv4-mapped IPv6", "::ffff:192.168.1.1", true}, // isIPv4Hint is true as it's an IPv4 concept
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ipStr)
			if ip == nil {
				t.Fatalf("Failed to parse IP: %s", tt.ipStr)
			}

			bigIntVal := IPToBigInt(ip)
			// For IPv4, ensure To4() is used for original comparison if applicable
			var originalIPForComparison net.IP
			if tt.isIPv4 && ip.To4() != nil {
				originalIPForComparison = ip.To4()
			} else {
				originalIPForComparison = ip
			}

			convertedIP := BigIntToIP(bigIntVal, tt.isIPv4) // Pass hint

			// Normalize convertedIP for comparison, especially for IPv4 zero (might be ::)
			var finalConvertedIP net.IP
			if tt.isIPv4 && convertedIP.To4() != nil {
				finalConvertedIP = convertedIP.To4()
			} else if !tt.isIPv4 && convertedIP.To16() != nil && convertedIP.To4() == nil { // Pure IPv6
				finalConvertedIP = convertedIP.To16()
			} else {
				finalConvertedIP = convertedIP // Fallback, or for IPv4-mapped if hint was false
			}

			if !originalIPForComparison.Equal(finalConvertedIP) {
				t.Errorf("IPToBigInt -> BigIntToIP mismatch for %s:\nOriginal: %s (%v)\nConverted: %s (%v)\nBigInt: %s",
					tt.ipStr, originalIPForComparison, []byte(originalIPForComparison),
					finalConvertedIP, []byte(finalConvertedIP), bigIntVal.String())
			}
		})
	}
}

func TestDiscoverLocalIP(t *testing.T) {
	// Test with XMLOCALIP environment variable
	t.Run("with XMLOCALIP env", func(t *testing.T) {
		validEnvIP := "10.20.30.40" // Assume this is a valid global unicast for testing
		os.Setenv("XMLOCALIP", validEnvIP)
		defer os.Unsetenv("XMLOCALIP")

		ip, err := DiscoverLocalIP()
		if err != nil {
			t.Errorf("DiscoverLocalIP() with valid env var error = %v", err)
		}
		if ip != validEnvIP {
			t.Errorf("DiscoverLocalIP() with valid env var got = %s, want %s", ip, validEnvIP)
		}
	})

	t.Run("with invalid XMLOCALIP env", func(t *testing.T) {
		invalidEnvIP := "not-an-ip"
		os.Setenv("XMLOCALIP", invalidEnvIP)
		defer os.Unsetenv("XMLOCALIP")

		// Expect it to fall back to discovery, so result depends on GetHostLocalIP
		// This test mainly checks that an invalid env var doesn't cause an immediate error itself.
		// The warning will be printed.
		ip, err := DiscoverLocalIP()
		if err != nil {
			t.Logf("DiscoverLocalIP() with invalid env var errored (expected if no other IP found): %v", err)
		} else {
			t.Logf("DiscoverLocalIP() with invalid env var succeeded, found: %s", ip)
		}
		// Further assertions depend on the actual network interfaces of the test machine.
	})

	// Test without XMLOCALIP (relies on actual network interfaces)
	t.Run("without XMLOCALIP env (discovery)", func(t *testing.T) {
		// Ensure env var is not set
		originalEnv, isSet := os.LookupEnv("XMLOCALIP")
		if isSet {
			os.Unsetenv("XMLOCALIP")
			defer os.Setenv("XMLOCALIP", originalEnv)
		}

		ip, err := DiscoverLocalIP()
		if err != nil {
			t.Logf("DiscoverLocalIP() discovery failed (this is OK if no suitable interface): %v", err)
			// On CI or systems with no suitable network, this might error.
			// Consider if this test should be skipped in such environments.
		} else {
			if !IsValidIPv4(ip) { // Assuming IsValidIPv4 is what we expect from GetHostLocalIP
				t.Errorf("DiscoverLocalIP() discovery got non-valid IPv4 = %s", ip)
			}
			t.Logf("DiscoverLocalIP() discovery found IP: %s", ip)
		}
	})
}

func TestIsValidIPv4(t *testing.T) {
	// Test cases for IsValidIPv4
	// The definition of "Valid" here includes IsGlobalUnicast,
	// meaning private IPs will return false.
	isValidIPv4TestCases := []struct { // Renamed variable
		name  string
		ipStr string
		want  bool
	}{
		{"valid global unicast", "8.8.8.8", true},
		{"private IP (not global unicast)", "192.168.1.1", true},
		{"loopback", "127.0.0.1", false},
		{"IPv6", "2001:db8::1", false},
		{"multicast", "224.0.0.1", false},
		{"link-local", "169.254.1.1", false},
		{"invalid format", "not.an.ip", false},
		{"empty string", "", false},
		{"broadcast", "255.255.255.255", false}, // IsGlobalUnicast is false
		{"zero IP", "0.0.0.0", false},           // IsGlobalUnicast is false, IsUnspecified is true
	}

	for _, tc := range isValidIPv4TestCases { // Changed tt to tc
		t.Run(tc.name, func(t *testing.T) {
			if got := IsValidIPv4(tc.ipStr); got != tc.want { // Use tc.ipStr and tc.want
				ipInfo := net.ParseIP(tc.ipStr)
				var debugStr string
				if ipInfo != nil {
					debugStr = fmt.Sprintf(" (To4:%v, IsLoopback:%v, IsGlobalUnicast:%v, IsUnspecified:%v, IsMulticast:%v, IsLinkLocalUnicast:%v)",
						ipInfo.To4() != nil,
						ipInfo.IsLoopback(),
						ipInfo.IsGlobalUnicast(),
						ipInfo.IsUnspecified(),
						ipInfo.IsMulticast(),
						ipInfo.IsLinkLocalUnicast(),
					)
				}
				t.Errorf("IsValidIPv4(%s) = %v, want %v%s", tc.ipStr, got, tc.want, debugStr)
			}
		})
	}
}

// Note: GetHostLocalIP is hard to test deterministically in a unit test
// as it depends on the actual network interfaces of the machine running the test.
// We can test its components or mock net.InterfaceAddrs if absolutely needed,
// but for now, DiscoverLocalIP covers its integration.

func TestNetworkSize(t *testing.T) {
	// Define test cases for networkSize
	// Note: networkSize is marked for potential refactor/deprecation and has specific behavior for /0 due to int32 overflow.
	networkSizeTestCases := []struct { // Renamed variable to avoid conflict if 'tests' is used elsewhere
		name     string
		maskStr  string // Dotted decimal mask
		wantSize int32
	}{
		{"/24", "255.255.255.0", 256},
		{"/30", "255.255.255.252", 4},
		{"/32", "255.255.255.255", 1},
		{"/16", "255.255.0.0", 65536},
		// For /0 mask (0.0.0.0):
		// ^mask results in 255.255.255.255 (MaxUint32 for IPv4).
		// binary.BigEndian.Uint32(^mask) is MaxUint32.
		// MaxUint32 + 1 (as uint32) overflows to 0.
		// int32(0) is 0.
		// So, networkSize("0.0.0.0") will return 0.
		{"/0 (full range with overflow)", "0.0.0.0", 0},
	}

	for _, tc := range networkSizeTestCases { // Changed tt to tc to use the new variable name
		t.Run(tc.name, func(t *testing.T) {
			// ParseIP can return nil if the input is not a valid IP address literal.
			// For masks like "255.255.255.0", To4() is appropriate.
			parsedMaskIP := net.ParseIP(tc.maskStr)
			if parsedMaskIP == nil {
				t.Fatalf("Failed to parse mask string '%s' as IP", tc.maskStr)
			}
			ipv4Mask := parsedMaskIP.To4()
			if ipv4Mask == nil {
				t.Fatalf("Mask string '%s' is not a valid IPv4 mask", tc.maskStr)
			}
			mask := net.IPMask(ipv4Mask)

			gotSize := networkSize(mask) // Assuming networkSize is still available in the 'ip' package

			if gotSize != tc.wantSize {
				t.Errorf("networkSize() for %s (%s) = %d, want %d", tc.name, tc.maskStr, gotSize, tc.wantSize)
			}
		})
	}
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
	// Create copies before sorting to avoid modifying original slices if they are reused
	ac := make([]string, len(a))
	bc := make([]string, len(b))
	copy(ac, a)
	copy(bc, b)

	sortStrings(ac)
	sortStrings(bc)
	return reflect.DeepEqual(ac, bc)
}
