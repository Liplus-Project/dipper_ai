package ddns

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- MyDNS tests ---

func TestMyDNSIPv4_Success(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	entry := MyDNSEntry{ID: "testid", Pass: "testpass", Domain: "home.example.com"}
	result := UpdateMyDNSIPv4(entry, srv.URL)

	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	// Verify Basic Auth header
	expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("testid:testpass"))
	if gotAuth != expected {
		t.Errorf("Authorization header: got %q, want %q", gotAuth, expected)
	}
}

func TestMyDNSIPv4_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	entry := MyDNSEntry{ID: "bad", Pass: "creds", Domain: "home.example.com"}
	result := UpdateMyDNSIPv4(entry, srv.URL)

	if result.Err == nil {
		t.Error("expected error on 401 response")
	}
	if !strings.Contains(result.Err.Error(), "401") {
		t.Errorf("error should mention status 401, got: %v", result.Err)
	}
}

func TestMyDNSIPv6_Success(t *testing.T) {
	// UpdateMyDNSIPv6 forces tcp6, so the mock server must listen on IPv6 loopback.
	ln, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skip("IPv6 loopback not available on this host, skipping IPv6 test")
	}

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.Listener = ln
	srv.Start()
	defer srv.Close()

	entry := MyDNSEntry{ID: "testid", Pass: "testpass", Domain: "home.example.com"}
	result := UpdateMyDNSIPv6(entry, srv.URL)

	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
}

// --- Cloudflare tests ---

// cfMockServer builds a mock Cloudflare API server.
// It handles: zone list, dns_records list, dns_records PATCH.
func cfMockServer(t *testing.T, zoneID, recordID string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		// Zone lookup: GET /zones?name=...
		case r.Method == http.MethodGet && strings.Contains(r.URL.RawQuery, "name="):
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"result":  []map[string]string{{"id": zoneID}},
			})

		// Record lookup: GET /zones/{zoneID}/dns_records?type=...
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "dns_records"):
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"result":  []map[string]string{{"id": recordID}},
			})

		// Record update: PATCH /zones/{zoneID}/dns_records/{recordID}
		case r.Method == http.MethodPatch:
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
			})

		default:
			http.NotFound(w, r)
		}
	}))
}

func TestCloudflare_Success(t *testing.T) {
	srv := cfMockServer(t, "zone123", "rec456")
	defer srv.Close()

	entry := CloudflareEntry{
		API:    "test-token",
		Zone:   "example.com",
		Domain: "home.example.com",
	}
	result := UpdateCloudflare(entry, "1.2.3.4", "A", srv.URL)

	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
}

func TestCloudflare_ZoneNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// success=true but empty result list → zone not found
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "result": []interface{}{}})
	}))
	defer srv.Close()

	entry := CloudflareEntry{API: "tok", Zone: "missing.com", Domain: "home.missing.com"}
	result := UpdateCloudflare(entry, "1.2.3.4", "A", srv.URL)

	if result.Err == nil {
		t.Error("expected error when zone is not found")
	}
	if !strings.Contains(result.Err.Error(), "zone lookup") {
		t.Errorf("error should mention zone lookup, got: %v", result.Err)
	}
}

func TestCloudflare_RecordNotFound(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		callCount++
		if callCount == 1 {
			// Zone found
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"result":  []map[string]string{{"id": "zone123"}},
			})
		} else {
			// No DNS records
			json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "result": []interface{}{}})
		}
	}))
	defer srv.Close()

	entry := CloudflareEntry{API: "tok", Zone: "example.com", Domain: "missing.example.com"}
	result := UpdateCloudflare(entry, "1.2.3.4", "A", srv.URL)

	if result.Err == nil {
		t.Error("expected error when DNS record is not found")
	}
	if !strings.Contains(result.Err.Error(), "find record") {
		t.Errorf("error should mention find record, got: %v", result.Err)
	}
}

func TestCloudflare_APIError(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		callCount++
		switch callCount {
		case 1: // zone lookup
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"result":  []map[string]string{{"id": "zone123"}},
			})
		case 2: // record lookup
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"result":  []map[string]string{{"id": "rec456"}},
			})
		default: // PATCH returns API error
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"errors":  []map[string]interface{}{{"code": 9109, "message": "invalid token"}},
			})
		}
	}))
	defer srv.Close()

	entry := CloudflareEntry{API: "bad-token", Zone: "example.com", Domain: "home.example.com"}
	result := UpdateCloudflare(entry, "1.2.3.4", "A", srv.URL)

	if result.Err == nil {
		t.Error("expected error on Cloudflare API error response")
	}
	if !strings.Contains(result.Err.Error(), "invalid token") {
		t.Errorf("error should contain API message, got: %v", result.Err)
	}
}

func TestCloudflare_BearerTokenSent(t *testing.T) {
	var gotAuth string
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if gotAuth == "" {
			gotAuth = r.Header.Get("Authorization")
		}
		callCount++
		switch callCount {
		case 1:
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"result":  []map[string]string{{"id": "zone123"}},
			})
		case 2:
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"result":  []map[string]string{{"id": "rec456"}},
			})
		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
		}
	}))
	defer srv.Close()

	entry := CloudflareEntry{API: "my-secret-token", Zone: "example.com", Domain: "home.example.com"}
	UpdateCloudflare(entry, "1.2.3.4", "A", srv.URL)

	if gotAuth != "Bearer my-secret-token" {
		t.Errorf("Authorization header: got %q, want %q", gotAuth, "Bearer my-secret-token")
	}
}

// TestCloudflare_ZoneID_SkipsLookup verifies that when ZoneID is set, the
// zone name lookup API call is skipped entirely.  This supports API tokens
// that only have DNS:Edit (no Zone:Read) permission.
func TestCloudflare_ZoneID_SkipsLookup(t *testing.T) {
	zoneLookupCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		// PATCH — record update
		case r.Method == http.MethodPatch:
			json.NewEncoder(w).Encode(map[string]interface{}{"success": true})

		// Record lookup: GET .../dns_records?...
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "dns_records"):
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"result":  []map[string]string{{"id": "rec999"}},
			})

		// Zone lookup: GET /zones?name=... (path does NOT contain dns_records)
		case r.Method == http.MethodGet:
			zoneLookupCalled = true
			json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "result": []interface{}{}})

		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	entry := CloudflareEntry{
		API:    "dns-only-token",
		ZoneID: "direct-zone-id", // provided directly — no lookup needed
		Domain: "home.example.com",
	}
	result := UpdateCloudflare(entry, "1.2.3.4", "A", srv.URL)

	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if zoneLookupCalled {
		t.Error("zone lookup API should NOT have been called when ZoneID is set")
	}
}

// TestCloudflare_ZoneNotFound_WithAPIError verifies that a proper error is
// returned when the Cloudflare API reports success=false on zone lookup.
func TestCloudflare_ZoneNotFound_WithAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"errors":  []map[string]interface{}{{"code": 9109, "message": "Invalid access token"}},
			"result":  []interface{}{},
		})
	}))
	defer srv.Close()

	entry := CloudflareEntry{API: "bad-token", Zone: "example.com", Domain: "home.example.com"}
	result := UpdateCloudflare(entry, "1.2.3.4", "A", srv.URL)

	if result.Err == nil {
		t.Fatal("expected error when API returns success=false")
	}
	if !strings.Contains(result.Err.Error(), "Invalid access token") {
		t.Errorf("error should contain API message, got: %v", result.Err)
	}
}
