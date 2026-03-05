package mode

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Liplus-Project/dipper_ai/internal/config"
	"github.com/Liplus-Project/dipper_ai/internal/ddns"
	"github.com/Liplus-Project/dipper_ai/internal/ip"
	"github.com/Liplus-Project/dipper_ai/internal/state"
)

// resetCheckGate removes the check gate file so the next Check() call runs.
func resetCheckGate(dir string) {
	_ = os.Remove(dir + "/gate_check")
}

// fakeFetchIP returns a fixed IPv4.
func fakeFetchIP(addr string) func(bool, bool) (*ip.Result, error) {
	return func(v4, v6 bool) (*ip.Result, error) {
		return &ip.Result{IPv4: addr}, nil
	}
}

// noopMyDNS is a DDNS update stub that succeeds silently.
func noopMyDNS(_ ddns.MyDNSEntry, _ string) ddns.ProviderResult {
	return ddns.ProviderResult{}
}

// noopCloudflare is a Cloudflare update stub that succeeds silently.
func noopCloudflare(_ ddns.CloudflareEntry, _, _, _ string) ddns.ProviderResult {
	return ddns.ProviderResult{}
}

// TestCheck_AllMatch_NoUpdate verifies that when DNS matches the current IP,
// no DDNS update is triggered and the gate is touched.
func TestCheck_AllMatch_NoUpdate(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		StateDir:   dir,
		IPv4:       true,
		IPv4DDNS:   true,
		UpdateTime: 1440,
		DDNSTime:   1,
		MyDNS: []config.MyDNSEntry{
			{ID: "u", Pass: "p", Domain: "home.example.com", IPv4: true},
		},
	}

	origFetch := ipFetch
	ipFetch = fakeFetchIP("1.2.3.4")
	t.Cleanup(func() { ipFetch = origFetch })

	origDNS := dnsLookupA
	dnsLookupA = func(domain string) (string, error) { return "1.2.3.4", nil } // matches
	t.Cleanup(func() { dnsLookupA = origDNS })

	origMyDNS := mydnsUpdateIPv4
	updateCalled := false
	mydnsUpdateIPv4 = func(e ddns.MyDNSEntry, u string) ddns.ProviderResult {
		updateCalled = true
		return ddns.ProviderResult{}
	}
	t.Cleanup(func() { mydnsUpdateIPv4 = origMyDNS })

	if err := Check(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updateCalled {
		t.Error("DDNS update should NOT have been called when DNS matches")
	}
}

// TestCheck_Mismatch_ForcesUpdate verifies that when the DNS-registered IP
// differs from the current external IP, Check resets the cache and forces an
// immediate DDNS update.
func TestCheck_Mismatch_ForcesUpdate(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		StateDir:   dir,
		IPv4:       true,
		IPv4DDNS:   true,
		UpdateTime: 1440,
		DDNSTime:   1,
		MyDNS: []config.MyDNSEntry{
			{ID: "u", Pass: "p", Domain: "home.example.com", IPv4: true},
		},
		MyDNSIPv4URL: "http://fake.invalid/login.html",
	}

	// Pre-set cache to current IP so Update() detects the reset to 0.0.0.0.
	st, _ := state.New(dir)
	_ = st.WriteIP("ipv4", "1.2.3.4")

	origFetch := ipFetch
	ipFetch = fakeFetchIP("1.2.3.4") // current IP
	t.Cleanup(func() { ipFetch = origFetch })

	origDNS := dnsLookupA
	dnsLookupA = func(domain string) (string, error) { return "0.0.0.0", nil } // stale
	t.Cleanup(func() { dnsLookupA = origDNS })

	origMyDNS := mydnsUpdateIPv4
	updateCalled := false
	mydnsUpdateIPv4 = func(e ddns.MyDNSEntry, u string) ddns.ProviderResult {
		updateCalled = true
		return ddns.ProviderResult{}
	}
	t.Cleanup(func() { mydnsUpdateIPv4 = origMyDNS })

	if err := Check(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !updateCalled {
		t.Error("DDNS update SHOULD have been called when DNS is stale")
	}

	// Verify that IP cache was written with correct IP after update.
	got, _ := st.ReadIP("ipv4")
	if got != "1.2.3.4" {
		t.Errorf("expected cache ipv4=1.2.3.4 after update, got %q", got)
	}
}

// TestCheck_DNSError_ForcesUpdate verifies that a DNS lookup failure is
// treated as a mismatch and triggers a DDNS update.
func TestCheck_DNSError_ForcesUpdate(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		StateDir:   dir,
		IPv4:       true,
		IPv4DDNS:   true,
		UpdateTime: 1440,
		DDNSTime:   1,
		MyDNS: []config.MyDNSEntry{
			{ID: "u", Pass: "p", Domain: "home.example.com", IPv4: true},
		},
		MyDNSIPv4URL: "http://fake.invalid/login.html",
	}

	origFetch := ipFetch
	ipFetch = fakeFetchIP("5.6.7.8")
	t.Cleanup(func() { ipFetch = origFetch })

	origDNS := dnsLookupA
	dnsLookupA = func(domain string) (string, error) { return "", fmt.Errorf("nxdomain") }
	t.Cleanup(func() { dnsLookupA = origDNS })

	origMyDNS := mydnsUpdateIPv4
	updateCalled := false
	mydnsUpdateIPv4 = func(e ddns.MyDNSEntry, u string) ddns.ProviderResult {
		updateCalled = true
		return ddns.ProviderResult{}
	}
	t.Cleanup(func() { mydnsUpdateIPv4 = origMyDNS })

	if err := Check(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !updateCalled {
		t.Error("DDNS update SHOULD have been called when DNS lookup fails")
	}
}

// TestCheck_Gate_SkipsWhenRecent verifies that the check gate prevents
// re-running before UpdateTime minutes have elapsed.
func TestCheck_Gate_SkipsWhenRecent(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		StateDir:   dir,
		IPv4:       true,
		IPv4DDNS:   true,
		UpdateTime: 1440,
		DDNSTime:   1,
		MyDNS: []config.MyDNSEntry{
			{ID: "u", Pass: "p", Domain: "home.example.com", IPv4: true},
		},
	}

	origFetch := ipFetch
	fetchCount := 0
	ipFetch = func(v4, v6 bool) (*ip.Result, error) {
		fetchCount++
		return &ip.Result{IPv4: "1.2.3.4"}, nil
	}
	t.Cleanup(func() { ipFetch = origFetch })

	origDNS := dnsLookupA
	dnsLookupA = func(domain string) (string, error) { return "1.2.3.4", nil }
	t.Cleanup(func() { dnsLookupA = origDNS })

	// First run: gate passes, fetch occurs.
	if err := Check(cfg); err != nil {
		t.Fatalf("first run: %v", err)
	}
	// Second run: gate active (not enough time has passed) → skips.
	if err := Check(cfg); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if fetchCount > 1 {
		t.Errorf("expected 1 fetch (gate should block second run), got %d", fetchCount)
	}
}

// TestUpdate_PerDomain_OnlyChangedEntry verifies that only the entry whose
// per-domain cache differs from the current IP is updated; unchanged entries
// are skipped even when the timer fires.
func TestUpdate_PerDomain_OnlyChangedEntry(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		StateDir:     dir,
		IPv4:         true,
		IPv4DDNS:     true,
		UpdateTime:   1440,
		MyDNSIPv4URL: "http://fake.invalid/login.html",
		MyDNS: []config.MyDNSEntry{
			{ID: "u0", Pass: "p0", Domain: "entry0.example.com", IPv4: true},
			{ID: "u1", Pass: "p1", Domain: "entry1.example.com", IPv4: true},
		},
	}

	// Pre-seed entry0 cache with current IP (already up-to-date).
	// entry1 cache is empty (0.0.0.0) → needs update.
	st, _ := state.New(dir)
	_ = st.WriteDomainCache("mydns_0", "ipv4", "1.2.3.4")
	// mydns_1 left at default (0.0.0.0)

	// Pre-touch gate_update so forceSync does not fire on this run.
	gateFile := dir + "/gate_update"
	_ = os.WriteFile(gateFile, []byte(time.Now().Format(time.RFC3339)), 0644)

	origFetch := ipFetch
	ipFetch = fakeFetchIP("1.2.3.4")
	t.Cleanup(func() { ipFetch = origFetch })

	updated := &[]string{}
	origMyDNS := mydnsUpdateIPv4
	mydnsUpdateIPv4 = func(e ddns.MyDNSEntry, u string) ddns.ProviderResult {
		*updated = append(*updated, e.Domain)
		return ddns.ProviderResult{}
	}
	t.Cleanup(func() { mydnsUpdateIPv4 = origMyDNS })

	if err := Update(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only entry1 should have been updated.
	for _, d := range *updated {
		if d == "entry0.example.com" {
			t.Error("entry0 should NOT be updated (cache already matches)")
		}
	}
	found1 := false
	for _, d := range *updated {
		if d == "entry1.example.com" {
			found1 = true
		}
	}
	if !found1 {
		t.Error("entry1 should have been updated (cache was 0.0.0.0)")
	}
}
