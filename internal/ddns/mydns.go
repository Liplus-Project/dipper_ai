// Package ddns handles DDNS updates for supported providers (MyDNS, Cloudflare).
package ddns

import (
	"fmt"
	"io"
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
// MyDNS authenticates via HTTP Basic Auth; the server reads the source IP
// from the request automatically (no explicit IP parameter needed).
func UpdateMyDNSIPv4(entry MyDNSEntry, updateURL string) ProviderResult {
	return doMyDNSRequest(entry, updateURL, "ipv4")
}

// UpdateMyDNSIPv6 sends an IPv6 update for a single MyDNS entry.
func UpdateMyDNSIPv6(entry MyDNSEntry, updateURL string) ProviderResult {
	return doMyDNSRequest(entry, updateURL, "ipv6")
}

func doMyDNSRequest(entry MyDNSEntry, updateURL, proto string) ProviderResult {
	pr := ProviderResult{
		Provider: "mydns",
		Domain:   entry.Domain,
	}

	client := &http.Client{Timeout: 15 * time.Second}

	req, err := http.NewRequest(http.MethodGet, updateURL, nil)
	if err != nil {
		pr.Err = fmt.Errorf("request build: %w", err)
		return pr
	}
	req.SetBasicAuth(entry.ID, entry.Pass)

	resp, err := client.Do(req)
	if err != nil {
		pr.Err = fmt.Errorf("%s http: %w", proto, err)
		return pr
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))

	if resp.StatusCode != http.StatusOK {
		pr.Err = fmt.Errorf("%s status %d: %s", proto, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return pr
}
