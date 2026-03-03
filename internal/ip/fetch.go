// Package ip fetches the current public IPv4 and/or IPv6 addresses.
// Uses external `dig` (or fallback `curl`) consistent with dipper's approach.
package ip

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
)

// Result holds the fetched IP addresses.
type Result struct {
	IPv4 string // empty if fetch failed or disabled
	IPv6 string // empty if fetch failed or disabled
}

// Fetch retrieves the current public IPs according to the flags.
// fetchV4 / fetchV6 enable/disable each address family.
// Returns an error if any enabled address family fails to resolve a valid IP.
func Fetch(fetchV4, fetchV6 bool) (*Result, error) {
	r := &Result{}

	if fetchV4 {
		ip, err := fetchIPv4()
		if err != nil {
			return nil, fmt.Errorf("IPv4 fetch failed: %w", err)
		}
		r.IPv4 = ip
	}

	if fetchV6 {
		ip, err := fetchIPv6()
		if err != nil {
			return nil, fmt.Errorf("IPv6 fetch failed: %w", err)
		}
		r.IPv6 = ip
	}

	return r, nil
}

// fetchIPv4 returns the current public IPv4 via dig (myip.opendns.com).
func fetchIPv4() (string, error) {
	// TODO: make resolver configurable via config
	out, err := exec.Command("dig", "-4", "+short", "myip.opendns.com", "@resolver1.opendns.com").Output()
	if err != nil {
		return "", fmt.Errorf("dig ipv4: %w", err)
	}
	return validateIP(strings.TrimSpace(string(out)), false)
}

// fetchIPv6 returns the current public IPv6 via dig.
func fetchIPv6() (string, error) {
	out, err := exec.Command("dig", "-6", "+short", "myip.opendns.com", "aaaa", "@resolver1.opendns.com").Output()
	if err != nil {
		return "", fmt.Errorf("dig ipv6: %w", err)
	}
	return validateIP(strings.TrimSpace(string(out)), true)
}

// validateIP checks that s is a valid IP of the expected family.
func validateIP(s string, expectV6 bool) (string, error) {
	if s == "" {
		return "", fmt.Errorf("empty response from resolver")
	}
	ip := net.ParseIP(s)
	if ip == nil {
		return "", fmt.Errorf("invalid IP: %q", s)
	}
	isV6 := ip.To4() == nil
	if expectV6 && !isV6 {
		return "", fmt.Errorf("expected IPv6, got %q", s)
	}
	if !expectV6 && isV6 {
		return "", fmt.Errorf("expected IPv4, got %q", s)
	}
	return s, nil
}
