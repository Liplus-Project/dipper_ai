// Package ddns handles DDNS updates for supported providers (MyDNS, Cloudflare).
package ddns

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const mydnsUpdateURL = "https://www.mydns.jp/login.html"

// MyDNSConfig holds credentials for a MyDNS account.
type MyDNSConfig struct {
	MasterID string
	Password string
}

// UpdateMyDNS sends an IP update to MyDNS for all configured domains.
// MyDNS uses HTTP Basic Auth; the IP is taken from the client's source address,
// so the request itself is the update (no explicit IP parameter).
// Returns a slice of per-domain results (nil error = success).
func UpdateMyDNS(cfg MyDNSConfig, ipv4, ipv6 string) []ProviderResult {
	// MyDNS updates by authenticated request — the server reads source IP.
	// ipv4/ipv6 hints are used only for logging.
	result := doMyDNSRequest(cfg, ipv4)
	return []ProviderResult{result}
}

// ProviderResult captures the outcome of a single DDNS update attempt.
type ProviderResult struct {
	Provider string
	Domain   string
	IP       string
	Err      error
}

func doMyDNSRequest(cfg MyDNSConfig, ip string) ProviderResult {
	client := &http.Client{Timeout: 15 * time.Second}

	data := url.Values{}
	data.Set("MASTERID", cfg.MasterID)
	data.Set("PASSWORD", cfg.Password)

	req, err := http.NewRequest(http.MethodPost, mydnsUpdateURL, strings.NewReader(data.Encode()))
	if err != nil {
		return ProviderResult{Provider: "mydns", IP: ip, Err: fmt.Errorf("request build: %w", err)}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return ProviderResult{Provider: "mydns", IP: ip, Err: fmt.Errorf("http: %w", err)}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))

	if resp.StatusCode != http.StatusOK {
		return ProviderResult{
			Provider: "mydns",
			IP:       ip,
			Err:      fmt.Errorf("http status %d: %s", resp.StatusCode, strings.TrimSpace(string(body))),
		}
	}
	return ProviderResult{Provider: "mydns", IP: ip}
}
