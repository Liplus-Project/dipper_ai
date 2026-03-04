package mode

import (
	"fmt"
	"os"
	"testing"

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

// TestUpdate_DDNSGate_BypassedOnIPChange verifies that a real IP change
// bypasses the DDNS_TIME gate even when the gate is still active.
func TestUpdate_DDNSGate_BypassedOnIPChange(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		StateDir:     dir,
		IPv4:         true,
		IPv4DDNS:     true,
		UpdateTime:   1440,
		DDNSTime:     1440, // very long — gate stays active
		MyDNSIPv4URL: "http://fake.invalid/login.html",
		MyDNS: []config.MyDNSEntry{
			{ID: "u", Pass: "p", Domain: "home.example.com", IPv4: true},
		},
	}

	// Pre-cache an old IP so the first update() run touches the ddnsGate.
	st, _ := state.New(dir)
	_ = st.WriteIP("ipv4", "1.1.1.1")

	origFetch := ipFetch
	ipFetch = fakeFetchIP("1.1.1.1") // no change yet
	t.Cleanup(func() { ipFetch = origFetch })

	origMyDNS := mydnsUpdateIPv4
	callCount := 0
	mydnsUpdateIPv4 = func(e ddns.MyDNSEntry, u string) ddns.ProviderResult {
		callCount++
		return ddns.ProviderResult{}
	}
	t.Cleanup(func() { mydnsUpdateIPv4 = origMyDNS })

	// First update: IP unchanged → gate should block (DDNSTime=1440 → gate active after touch)
	// Actually on first run gate_ddns doesn't exist yet → ShouldRun=true → update fires
	_ = Update(cfg)
	after1 := callCount

	// Now simulate IP change — gate_ddns is now active (just touched).
	ipFetch = fakeFetchIP("9.9.9.9") // new IP

	_ = Update(cfg)
	after2 := callCount

	if after2 <= after1 {
		t.Errorf("IP change should bypass DDNS gate; callCount before=%d after=%d", after1, after2)
	}
}
