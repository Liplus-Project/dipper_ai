// Package ddns handles DDNS updates for supported providers (MyDNS, Cloudflare).
package ddns

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// MyDNSEntry mirrors config.MyDNSEntry to avoid import cycles.
type MyDNSEntry struct {
	ID     string
	Pass   string
	Domain string
	IPv4   bool
	IPv6   bool
}

// ProviderResult captures the outcome of a single DDNS update attempt.
type ProviderResult struct {
	Provider string
	Domain   string
	IP       string
	Err      error
}

// UpdateMyDNSIPv4 sends an IPv4 update for a single MyDNS entry.
// MyDNS reads the source IP of the request automatically.
// The HTTP client is forced to connect via TCP4 so that the source address
// seen by MyDNS is always an IPv4 address, even on dual-stack hosts.
func UpdateMyDNSIPv4(entry MyDNSEntry, updateURL string) ProviderResult {
	return doMyDNSRequest(entry, updateURL, "tcp4")
}

// UpdateMyDNSIPv6 sends an IPv6 update for a single MyDNS entry.
// The HTTP client is forced to connect via TCP6 so that MyDNS sees the
// source IPv6 address.
func UpdateMyDNSIPv6(entry MyDNSEntry, updateURL string) ProviderResult {
	return doMyDNSRequest(entry, updateURL, "tcp6")
}

// doMyDNSRequest performs the actual HTTP GET to the MyDNS login endpoint.
// network must be "tcp4" or "tcp6" to ensure the correct source address
// family is used (MyDNS registers the source IP, not a parameter).
func doMyDNSRequest(entry MyDNSEntry, updateURL, network string) ProviderResult {
	proto := strings.TrimPrefix(network, "tcp") // "4" or "6"
	pr := ProviderResult{
		Provider: "mydns",
		Domain:   entry.Domain,
	}

	// Force the specified IP family so MyDNS sees the right source address.
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, addr)
		},
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
	}

	req, err := http.NewRequest(http.MethodGet, updateURL, nil)
	if err != nil {
		pr.Err = fmt.Errorf("ipv%s request build: %w", proto, err)
		return pr
	}
	req.SetBasicAuth(entry.ID, entry.Pass)

	resp, err := client.Do(req)
	if err != nil {
		pr.Err = fmt.Errorf("ipv%s http: %w", proto, err)
		return pr
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))

	if resp.StatusCode != http.StatusOK {
		pr.Err = fmt.Errorf("ipv%s status %d: %s", proto, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return pr
}
