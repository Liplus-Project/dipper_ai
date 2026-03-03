package ddns

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const cfAPIBase = "https://api.cloudflare.com/client/v4"

// CloudflareEntry holds credentials and target for one Cloudflare DNS record.
type CloudflareEntry struct {
	Token  string // API token with DNS:Edit permission
	ZoneID string
	Name   string // FQDN of the record (e.g. "home.example.com")
}

// UpdateCloudflare updates all given Cloudflare entries to point to ip.
// recType should be "A" (IPv4) or "AAAA" (IPv6).
func UpdateCloudflare(entries []CloudflareEntry, ip, recType string) []ProviderResult {
	results := make([]ProviderResult, 0, len(entries))
	for _, e := range entries {
		results = append(results, updateCFEntry(e, ip, recType))
	}
	return results
}

func updateCFEntry(e CloudflareEntry, ip, recType string) ProviderResult {
	pr := ProviderResult{Provider: "cloudflare", Domain: e.Name, IP: ip}

	client := &http.Client{Timeout: 15 * time.Second}

	// 1. Find the record ID
	recID, err := cfFindRecord(client, e, recType)
	if err != nil {
		pr.Err = fmt.Errorf("find record: %w", err)
		return pr
	}

	// 2. PATCH the record
	payload := map[string]interface{}{
		"type":    recType,
		"name":    e.Name,
		"content": ip,
		"ttl":     1, // auto
		"proxied": false,
	}
	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/zones/%s/dns_records/%s", cfAPIBase, e.ZoneID, recID)

	req, _ := http.NewRequest(http.MethodPatch, url, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+e.Token)
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

// cfFindRecord returns the DNS record ID for the given zone/name/type.
func cfFindRecord(client *http.Client, e CloudflareEntry, recType string) (string, error) {
	url := fmt.Sprintf("%s/zones/%s/dns_records?type=%s&name=%s", cfAPIBase, e.ZoneID, recType, e.Name)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+e.Token)

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
		return "", fmt.Errorf("no %s record found for %s in zone %s", recType, e.Name, e.ZoneID)
	}
	return result.Result[0].ID, nil
}
