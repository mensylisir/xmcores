package ip

import (
	"encoding/binary"
	"fmt"
	"math/big" // For IP to integer conversion, supports IPv6 better
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	// "encoding/binary" // No longer needed if networkSize is fully removed or BigInt methods are used
)

// ParseIPsFromString parses a string containing IP addresses, CIDR notations, or IP ranges
// and returns a slice of individual IP address strings.
// Supported formats:
// - Single IP: "192.168.1.1"
// - CIDR: "192.168.1.0/24" (use GetUsableIPsFromCIDR for host IPs)
// - IP Range: "192.168.1.10-192.168.1.20"
// - Comma-separated list of any of the above.
func ParseIPsFromString(input string) ([]string, error) {
	allIPs := []string{} // Initialize with empty non-nil slice
	seenIPs := make(map[string]struct{})

	trimmedInput := strings.TrimSpace(input)
	if trimmedInput == "" {
		return allIPs, nil // Handle truly empty or only whitespace input
	}

	parts := strings.Split(trimmedInput, ",")
	var effectivePartsFound int // Count parts that are not just empty strings after split by comma

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue // Skip empty parts resulting from "1.1.1.1,,2.2.2.2" or leading/trailing commas
		}
		effectivePartsFound++

		// Trim trailing slash, common in some inputs like "1.2.3.4/"
		part = strings.TrimRight(part, "/")

		var ips []string
		var err error

		if strings.Contains(part, "/") { // CIDR or IP/mask
			if strings.HasSuffix(part, "/32") && !strings.Contains(part, ":") { // IPv4 /32
				ipOnly := strings.TrimSuffix(part, "/32")
				parsedIP := net.ParseIP(ipOnly)
				if parsedIP == nil || parsedIP.To4() == nil { // Ensure it's a valid IPv4
					return nil, fmt.Errorf("invalid IPv4 address in CIDR %s", part)
				}
				ips = []string{parsedIP.String()}
			} else if strings.HasSuffix(part, "/128") && strings.Contains(part, ":") { // IPv6 /128
				ipOnly := strings.TrimSuffix(part, "/128")
				parsedIP := net.ParseIP(ipOnly)
				if parsedIP == nil || parsedIP.To4() != nil { // Ensure it's a valid IPv6
					return nil, fmt.Errorf("invalid IPv6 address in CIDR %s", part)
				}
				ips = []string{parsedIP.String()}
			} else {
				ips, err = GetAllIPsFromCIDR(part) // Handles normalization internally
				if err != nil {
					return nil, fmt.Errorf("failed to parse CIDR '%s': %w", part, err)
				}
			}
		} else if strings.Contains(part, "-") { // IP Range
			ipRangeParts := strings.SplitN(part, "-", 2)
			if len(ipRangeParts) != 2 || strings.TrimSpace(ipRangeParts[0]) == "" || strings.TrimSpace(ipRangeParts[1]) == "" {
				return nil, fmt.Errorf("invalid IP range format: '%s'", part)
			}
			ips, err = GetIPsFromRange(strings.TrimSpace(ipRangeParts[0]), strings.TrimSpace(ipRangeParts[1]))
			if err != nil {
				return nil, fmt.Errorf("failed to parse IP range '%s': %w", part, err)
			}
		} else { // Single IP
			parsedIP := net.ParseIP(part)
			if parsedIP == nil {
				return nil, fmt.Errorf("invalid IP address: %s", part)
			}
			ips = []string{parsedIP.String()} // Normalize the IP string representation
		}

		for _, ipStr := range ips {
			if _, found := seenIPs[ipStr]; !found {
				allIPs = append(allIPs, ipStr)
				seenIPs[ipStr] = struct{}{}
			}
		}
	}

	// If the input was not empty (after initial trim) but we found no valid IP segments
	// AND we didn't collect any IPs, then it's an error.
	// This handles cases like " , , " which result in effectivePartsFound = 0
	// or valid looking but empty segments like "1.1.1.1," where the last part is empty.
	if effectivePartsFound == 0 && len(allIPs) == 0 && trimmedInput != "" {
		// This case is for inputs like ",," or "   " which should have been caught by the first check.
		// The more important check is if parts were processed but none yielded IPs.
		// If effectivePartsFound > 0 but allIPs is still 0, it means all parts were invalid.
		return allIPs, nil // e.g. input like ",," should result in empty list, no error
	}

	if len(allIPs) == 0 && effectivePartsFound > 0 {
		// This means we processed some non-empty parts, but none of them were valid IP/CIDR/Range.
		return nil, fmt.Errorf("no valid IP addresses, CIDRs, or ranges found in input: '%s'", input)
	}

	return allIPs, nil
}

// GetIPsFromRange generates a list of IP addresses within a given start and end IP range (inclusive).
// Supports both IPv4 and IPv6.
func GetIPsFromRange(ipStartStr, ipEndStr string) ([]string, error) {
	startIP := net.ParseIP(ipStartStr)
	endIP := net.ParseIP(ipEndStr)

	if startIP == nil {
		return nil, fmt.Errorf("invalid start IP address: '%s'", ipStartStr)
	}
	if endIP == nil {
		return nil, fmt.Errorf("invalid end IP address: '%s'", ipEndStr)
	}

	// Ensure IPs are of the same family (both IPv4 or both IPv6)
	isStartIPv4 := startIP.To4() != nil
	isEndIPv4 := endIP.To4() != nil
	if isStartIPv4 != isEndIPv4 {
		return nil, errors.New("start and end IP addresses must be of the same family (IPv4 or IPv6)")
	}

	var ips []string
	startNum := IPToBigInt(startIP)
	endNum := IPToBigInt(endIP)

	if startNum.Cmp(endNum) > 0 {
		return nil, errors.New("start IP address must be numerically greater than or equal to end IP address")
	}

	currentNum := new(big.Int).Set(startNum)
	one := big.NewInt(1)
	maxIPsToGenerate := 1 << 16 // Safety limit (65536 IPs)
	count := 0

	for currentNum.Cmp(endNum) <= 0 {
		if count >= maxIPsToGenerate {
			fmt.Printf("Warning: GetIPsFromRange: Range %s-%s is too large, returning first %d IPs\n", ipStartStr, ipEndStr, maxIPsToGenerate)
			break
		}
		ip := BigIntToIP(currentNum, isStartIPv4) // Pass whether it's IPv4
		if ip == nil {
			return nil, fmt.Errorf("failed to convert number %s back to IP for range %s-%s", currentNum.String(), ipStartStr, ipEndStr)
		}
		ips = append(ips, ip.String())
		currentNum.Add(currentNum, one)
		count++
	}
	return ips, nil
}

// GetAllIPsFromCIDR returns all IP addresses within a given CIDR block,
// including the network and broadcast addresses.
func GetAllIPsFromCIDR(cidrStr string) ([]string, error) {
	// NormalizeCIDR first to handle different mask formats
	normalizedCidr := NormalizeCIDR(cidrStr)
	ip, ipNet, err := net.ParseCIDR(normalizedCidr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR block '%s' (normalized to '%s'): %w", cidrStr, normalizedCidr, err)
	}

	var ips []string
	isIPv4 := ip.To4() != nil
	currentIPNum := IPToBigInt(ipNet.IP) // Start with the network address

	maskLen, totalBits := ipNet.Mask.Size()
	if maskLen == 0 && totalBits == 0 { // Should be caught by ParseCIDR, but defensive
		return nil, fmt.Errorf("invalid mask for CIDR %s", cidrStr)
	}

	var hostBits int
	if isIPv4 {
		hostBits = 32 - maskLen
	} else {
		hostBits = 128 - maskLen
	}
	if hostBits < 0 { // e.g. /33 for IPv4
		return nil, fmt.Errorf("invalid prefix length %d for %s", maskLen, map[bool]string{true: "IPv4", false: "IPv6"}[isIPv4])
	}

	maxGeneratedIPs := 1 << 16 // 65536
	count := 0
	one := big.NewInt(1)

	// Loop while the current IP is contained in the network.
	// For very large networks (small prefix), this loop can be very long.
	// The Contains check is essential.
	for ; ipNet.Contains(BigIntToIP(currentIPNum, isIPv4)); currentIPNum.Add(currentIPNum, one) {
		// Apply limit only for subnets that could genuinely produce more than maxGeneratedIPs.
		// hostBits > 16 means more than 2^16 = 65536 addresses.
		if count >= maxGeneratedIPs && hostBits > 16 {
			fmt.Printf("Warning: GetAllIPsFromCIDR: CIDR %s is too large, returning first %d IPs\n", cidrStr, maxGeneratedIPs)
			break
		}
		ipToAdd := BigIntToIP(currentIPNum, isIPv4)
		if ipToAdd == nil { // Should not happen if currentIPNum is valid
			return nil, fmt.Errorf("failed to convert number %s to IP for CIDR %s", currentIPNum.String(), cidrStr)
		}
		ips = append(ips, ipToAdd.String())
		count++
		if count == 0 { // Safety break for int overflow if maxGeneratedIPs was much larger
			break
		}
	}
	return ips, nil
}

// GetUsableIPsFromCIDR returns usable host IP addresses within a given CIDR block.
// For IPv4, it excludes the network and broadcast addresses.
// For IPv6, all addresses are typically considered usable (no dedicated broadcast).
func GetUsableIPsFromCIDR(cidrStr string) ([]string, error) {
	allIPs, err := GetAllIPsFromCIDR(cidrStr)
	if err != nil {
		return nil, err
	}

	if len(allIPs) == 0 {
		return []string{}, nil
	}

	// Check if it's IPv4 to exclude network & broadcast
	// We need to parse an IP from the CIDR string itself to determine family,
	// as allIPs[0] might be an IPv6 representation of an IPv4 network.
	ip, _, err := net.ParseCIDR(NormalizeCIDR(cidrStr))
	if err != nil {
		// Should have been caught by GetAllIPsFromCIDR, but defensive
		return nil, fmt.Errorf("internal error parsing CIDR for family check: %w", err)
	}

	if ip.To4() != nil { // It's an IPv4 CIDR
		if len(allIPs) <= 2 { // e.g., /31 (2 IPs) or /32 (1 IP) for IPv4
			return []string{}, nil // No usable host IPs by common convention
		}
		return allIPs[1 : len(allIPs)-1], nil // Exclude first (network) and last (broadcast)
	}

	// For IPv6, all IPs are generally usable
	return allIPs, nil
}

// NormalizeCIDR ensures a CIDR string is in "ip/prefixlen" format.
// It converts "ip/netmask" to "ip/prefixlen".
// If input is a plain IP, it appends /32 for IPv4 or /128 for IPv6.
func NormalizeCIDR(ipAddressOrCIDR string) string {
	ipAddressOrCIDR = strings.TrimSpace(ipAddressOrCIDR)
	if !strings.Contains(ipAddressOrCIDR, "/") {
		ip := net.ParseIP(ipAddressOrCIDR)
		if ip != nil {
			if ip.To4() != nil {
				return ipAddressOrCIDR + "/32"
			}
			return ipAddressOrCIDR + "/128" // Plain IPv6 becomes /128
		}
		return ipAddressOrCIDR // Let ParseCIDR handle an invalid plain IP later
	}

	parts := strings.SplitN(ipAddressOrCIDR, "/", 2)
	if len(parts) != 2 {
		return ipAddressOrCIDR // Invalid format, e.g. "1.2.3.4/"
	}
	ipPartStr, maskPartStr := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])

	// If maskPartStr is already a number (prefix length), reconstruct and return
	if _, err := strconv.Atoi(maskPartStr); err == nil {
		return fmt.Sprintf("%s/%s", ipPartStr, maskPartStr)
	}

	// If maskPartStr is a dotted decimal netmask (for IPv4)
	if strings.Contains(maskPartStr, ".") {
		maskIP := net.ParseIP(maskPartStr)
		// It must be a valid IP to be a mask, and for IPv4, To4() should be non-nil
		if maskIP != nil && maskIP.To4() != nil {
			ipv4Mask := net.IPMask(maskIP.To4())
			prefixLen, _ := ipv4Mask.Size()
			return fmt.Sprintf("%s/%d", ipPartStr, prefixLen)
		}
		// If maskIP is nil or not an IPv4 representation, it's an invalid mask for this path
	}

	// If it's an IPv6 address and maskPart is an IPv6 mask (less common but possible)
	// This is more complex as IPv6 masks are usually just prefix lengths.
	// For now, if it's not a number or a dotted quad, let ParseCIDR handle it.
	return ipAddressOrCIDR // Return original if mask format is not recognized here
}

// NetworkRange returns the first (network) and last (broadcast or last in range) IP addresses of a given IP network.
// Deprecated: For network address use `ipNet.IP`. For broadcast/last IP, iterate or use IP math.
func NetworkRange(network *net.IPNet) (net.IP, net.IP, error) {
	if network == nil {
		return nil, nil, errors.New("input network is nil")
	}
	ip := network.IP
	mask := network.Mask

	networkAddress := ip.Mask(mask) // This is ipNet.IP essentially

	lastIP := make(net.IP, len(ip))
	for i := 0; i < len(ip); i++ {
		lastIP[i] = ip[i] | ^mask[i]
	}

	// Ensure consistency for IPv4 addresses that might be in 16-byte form
	if networkAddress.To4() != nil {
		networkAddress = networkAddress.To4()
		lastIP = lastIP.To4()
	}

	return networkAddress, lastIP, nil
}

// IPToBigInt converts a net.IP to a *big.Int.
func IPToBigInt(ip net.IP) *big.Int {
	// Ensure IPv4 is represented as 4 bytes for consistent integer conversion
	// if its To4() form is available. Otherwise, use its native length (usually 16 for IPv6).
	if ipv4 := ip.To4(); ipv4 != nil {
		return new(big.Int).SetBytes(ipv4)
	}
	return new(big.Int).SetBytes(ip)
}

// BigIntToIP converts a *big.Int to a net.IP.
// isIPv4Hint helps determine if the original IP was IPv4.
func BigIntToIP(n *big.Int, isIPv4Hint bool) net.IP {
	ipBytes := n.Bytes()

	if isIPv4Hint {
		// For IPv4, construct a 4-byte net.IP.
		// This ensures net.IP.String() formats it as an IPv4 address.
		if n.IsUint64() && n.Uint64() <= 0xffffffff { // Check if it fits in uint32
			b := make([]byte, net.IPv4len)
			binary.BigEndian.PutUint32(b, uint32(n.Uint64()))
			return net.IP(b)
		}
		// If the number is too large for uint32 but hinted as IPv4,
		// this indicates an issue with the hint or the number.
		// Fallback: try to form a 16-byte IP, which might be an IPv4-mapped IPv6.
		// This path should ideally not be hit if inputs are consistent.
		if len(ipBytes) < net.IPv6len {
			fullIP := make([]byte, net.IPv6len)
			copy(fullIP[net.IPv6len-len(ipBytes):], ipBytes)
			return net.IP(fullIP)
		}
		return net.IP(ipBytes) // Return as is (could be > 16 bytes)
	}

	// For IPv6, construct a 16-byte net.IP.
	if len(ipBytes) < net.IPv6len {
		fullIP := make([]byte, net.IPv6len)
		copy(fullIP[net.IPv6len-len(ipBytes):], ipBytes)
		return net.IP(fullIP)
	}
	// If ipBytes is exactly net.IPv6len or longer (net.IP will truncate if > 16)
	return net.IP(ipBytes)
}

// isIPv4 is a helper (already provided, assumed to be correct)
func isIPv4(ipBytes []byte) bool {
	if len(ipBytes) == net.IPv4len {
		return true
	}
	if len(ipBytes) == net.IPv6len {
		prefix := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff}
		isMapped := true
		for i := 0; i < len(prefix); i++ {
			if ipBytes[i] != prefix[i] {
				isMapped = false
				break
			}
		}
		return isMapped
	}
	return false
}

// GetHostLocalIP attempts to find a non-loopback, global unicast IPv4 address for the host.
// If none found, it tries any non-loopback IPv4.
func GetHostLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", errors.Wrap(err, "failed to get interface addresses")
	}

	var firstPrivateIPv4 string
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			ipv4 := ipnet.IP.To4()
			if ipv4 != nil {
				if ipv4.IsGlobalUnicast() {
					return ipv4.String(), nil // Prefer global unicast IPv4
				}
				if firstPrivateIPv4 == "" && (ipv4.IsPrivate() || ipv4.IsLinkLocalUnicast()) { // Store first private or link-local
					firstPrivateIPv4 = ipv4.String()
				}
			}
		}
	}

	if firstPrivateIPv4 != "" {
		return firstPrivateIPv4, nil // Fallback to first private/link-local IPv4
	}

	return "", errors.New("no suitable local IPv4 address found (global unicast, private, or link-local)")
}

// IsValidIPv4 checks if the string is a valid, non-loopback, global unicast IPv4 address.
// Note: This definition means private IPs (192.168.x.x, 10.x.x.x, 172.16-31.x.x) will return false.
func IsValidIPv4(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	// An IP is valid for this purpose if:
	// 1. It's a valid IP address.
	// 2. It's an IPv4 address.
	// 3. It's not a loopback address.
	// 4. It's globally unicast (not link-local, not multicast, not private for some definitions).
	return ip != nil && ip.To4() != nil && !ip.IsLoopback() && ip.IsGlobalUnicast()
}

// DiscoverLocalIP finds the local IP address.
// It first checks the "XMLOCALIP" environment variable (expecting a global unicast IPv4).
// If not set or invalid by IsValidIPv4's definition, it calls GetHostLocalIP.
// Returns an error if no IP can be found.
func DiscoverLocalIP() (string, error) {
	if envIP := os.Getenv("XMLOCALIP"); envIP != "" {
		// IsValidIPv4 checks for global unicast. If env var is private, it will fail this check.
		// Depending on requirements, one might want a different validation for env var.
		if IsValidIPv4(envIP) {
			return envIP, nil
		}
		fmt.Printf("Warning: Environment variable XMLOCALIP ('%s') is not a valid global unicast IPv4 by IsValidIPv4 definition. Attempting discovery.\n", envIP)
	}

	localIP, err := GetHostLocalIP()
	if err != nil {
		return "", fmt.Errorf("failed to discover local IP: %w", err)
	}
	return localIP, nil
}

// networkSize is specific to IPv4 logic and returns total addresses.
// It's kept for compatibility with tests but consider replacing its usage.
func networkSize(mask net.IPMask) int32 {
	// Ensure mask is 4 bytes for IPv4 logic
	if len(mask) != net.IPv4len {
		if len(mask) == net.IPv6len && isIPv4(mask) { // IPv4-mapped IPv6 mask
			mask = mask[12:]
		} else {
			// Cannot determine IPv4 network size from non-IPv4 mask
			return 0 // Or an error
		}
	}
	m := net.IPv4Mask(0, 0, 0, 0) // Create a 4-byte slice
	for i := 0; i < net.IPv4len; i++ {
		m[i] = ^mask[i]
	}
	// This calculates (2^host_bits - 1) + 1 = 2^host_bits, total number of addresses.
	// For /0, ^mask is 255.255.255.255 (MaxUint32). MaxUint32 + 1 overflows to 0 for uint32.
	return int32(binary.BigEndian.Uint32(m)) + 1
}
