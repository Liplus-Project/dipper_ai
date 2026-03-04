package ddns

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// CloudflareEntry mirrors config.CloudflareEntry to avoid import cycles.
type CloudflareEntry struct {
	API    string // API token with DNS:Edit permission
	Zone   string // zone name (e.g. "example.com")
	ZoneID string // zone ID (optional; skips zone-name lookup when set)
	Domain string // FQDN of the record (e.g. "home.example.com")
}

// UpdateCloudflare updates a Cloudflare DNS record.
// recType should be "A" (IPv4) or "AAAA" (IPv6).
// zonesURL is the Cloudflare zones API base (from config.CloudflareURL).
func UpdateCloudflare(entry CloudflareEntry, ip, recType, zonesURL string) ProviderResult {
	pr := ProviderResult{Provider: "cloudflare", Domain: entry.Domain, IP: ip}

	client := &http.Client{Timeout: 15 * time.Second}

	// 1. Resolve zone name → zone ID (skip when ZoneID is already known)
	zoneID := entry.ZoneID
	if zoneID == "" {
		var err error
		zoneID, err = cfResolveZone(client, entry, zonesURL)
		if err != nil {
			pr.Err = fmt.Errorf("zone lookup: %w", err)
			return pr
		}
	}

	// 2. Find the existing DNS record ID
	recID, err := cfFindRecord(client, entry, zoneID, recType, zonesURL)
	if err != nil {
		pr.Err = fmt.Errorf("find record: %w", err)
		return pr
	}

	// 3. PATCH the record
	payload := map[string]interface{}{
		"type":    recType,
		"name":    entry.Domain,
		"content": ip,
		"ttl":     1, // auto
		"proxied": false,
	}
	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/%s/dns_records/%s", zonesURL, zoneID, recID)

	req, _ := http.NewRequest(http.MethodPatch, url, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+entry.API)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		pr.Err = fmt.Errorf("patch: %w", err)
		return pr
	}
	defer resp.Body.Close()

	var result cfResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&result); err != nil {
		pr.Err = fmt.Errorf("decode response: %w", err)
		return pr
	}
	if !result.Success {
		pr.Err = fmt.Errorf("cloudflare API error: %s", result.errorString())
	}
	return pr
}

// cfResolveZone returns the zone ID for the given zone name.
// This requires the API token to have Zone:Read permission.
// If your token only has DNS:Edit, set CF_x_ZONE_ID in user.conf instead.
func cfResolveZone(client *http.Client, entry CloudflareEntry, zonesURL string) (string, error) {
	url := fmt.Sprintf("%s?name=%s", zonesURL, entry.Zone)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+entry.API)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		cfResponse
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&result); err != nil {
		return "", err
	}
	if !result.Success {
		return "", fmt.Errorf("zones API error (token may lack Zone:Read permission): %s", result.errorString())
	}
	if len(result.Result) == 0 {
		return "", fmt.Errorf("zone %q not found — check zone name or set CF_x_ZONE_ID in user.conf", entry.Zone)
	}
	return result.Result[0].ID, nil
}

// cfFindRecord returns the DNS record ID for the given zone/domain/type.
func cfFindRecord(client *http.Client, entry CloudflareEntry, zoneID, recType, zonesURL string) (string, error) {
	url := fmt.Sprintf("%s/%s/dns_records?type=%s&name=%s", zonesURL, zoneID, recType, entry.Domain)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+entry.API)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		cfResponse
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&result); err != nil {
		return "", err
	}
	if !result.Success {
		return "", fmt.Errorf("dns_records API error: %s", result.errorString())
	}
	if len(result.Result) == 0 {
		return "", fmt.Errorf("no %s record found for %s in zone %s", recType, entry.Domain, entry.Zone)
	}
	return result.Result[0].ID, nil
}

// cfResponse is the common envelope returned by all Cloudflare API calls.
type cfResponse struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

func (r cfResponse) errorString() string {
	if len(r.Errors) == 0 {
		return "(no error detail)"
	}
	msgs := make([]string, len(r.Errors))
	for i, e := range r.Errors {
		msgs[i] = fmt.Sprintf("[%d] %s", e.Code, e.Message)
	}
	return strings.Join(msgs, "; ")
}
