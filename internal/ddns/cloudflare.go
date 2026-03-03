package ddns

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// CloudflareEntry mirrors config.CloudflareEntry to avoid import cycles.
type CloudflareEntry struct {
	API    string // API token with DNS:Edit permission
	Zone   string // zone name (e.g. "example.com")
	Domain string // FQDN of the record (e.g. "home.example.com")
}

// UpdateCloudflare updates a Cloudflare DNS record.
// recType should be "A" (IPv4) or "AAAA" (IPv6).
// zonesURL is the Cloudflare zones API base (from config.CloudflareURL).
func UpdateCloudflare(entry CloudflareEntry, ip, recType, zonesURL string) ProviderResult {
	pr := ProviderResult{Provider: "cloudflare", Domain: entry.Domain, IP: ip}

	client := &http.Client{Timeout: 15 * time.Second}

	// 1. Resolve zone name → zone ID
	zoneID, err := cfResolveZone(client, entry, zonesURL)
	if err != nil {
		pr.Err = fmt.Errorf("zone lookup: %w", err)
		return pr
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

	var result struct {
		Success bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&result); err != nil {
		pr.Err = fmt.Errorf("decode response: %w", err)
		return pr
	}
	if !result.Success {
		msgs := make([]string, len(result.Errors))
		for i, e := range result.Errors {
			msgs[i] = e.Message
		}
		pr.Err = fmt.Errorf("cloudflare API error: %v", msgs)
	}
	return pr
}

// cfResolveZone returns the zone ID for the given zone name.
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
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&result); err != nil {
		return "", err
	}
	if len(result.Result) == 0 {
		return "", fmt.Errorf("zone %q not found", entry.Zone)
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
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&result); err != nil {
		return "", err
	}
	if len(result.Result) == 0 {
		return "", fmt.Errorf("no %s record found for %s in zone %s", recType, entry.Domain, entry.Zone)
	}
	return result.Result[0].ID, nil
}
