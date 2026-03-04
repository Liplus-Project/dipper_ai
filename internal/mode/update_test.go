package mode

import (
	"errors"
	"os"
	"testing"

	"github.com/Liplus-Project/dipper_ai/internal/config"
	"github.com/Liplus-Project/dipper_ai/internal/ddns"
	"github.com/Liplus-Project/dipper_ai/internal/ip"
)

// fakeFetch returns a fixed IP result.
func fakeFetch(v4, v6 string) func(bool, bool) (*ip.Result, error) {
	return func(wantV4, wantV6 bool) (*ip.Result, error) {
		r := &ip.Result{}
		if wantV4 {
			r.IPv4 = v4
		}
		if wantV6 {
			r.IPv6 = v6
		}
		return r, nil
	}
}

// fakeFetchIPv6Err returns IPv4 successfully but simulates an IPv6 fetch error.
// This replicates the issue #11 scenario (host has no IPv6 connectivity).
func fakeFetchIPv6Err(v4 string, v6Err error) func(bool, bool) (*ip.Result, error) {
	return func(wantV4, wantV6 bool) (*ip.Result, error) {
		r := &ip.Result{}
		if wantV4 {
			r.IPv4 = v4
		}
		if wantV6 {
			r.ErrIPv6 = v6Err
		}
		return r, nil
	}
}

// overrideFetch replaces ipFetch for the duration of a test.
func overrideFetch(t *testing.T, fn func(bool, bool) (*ip.Result, error)) {
	t.Helper()
	orig := ipFetch
	ipFetch = fn
	t.Cleanup(func() { ipFetch = orig })
}

// captureMyDNSCalls replaces mydnsUpdateIPv4/IPv6 and records calls.
func captureMyDNSCalls(t *testing.T) *[]string {
	t.Helper()
	calls := &[]string{}
	origV4, origV6 := mydnsUpdateIPv4, mydnsUpdateIPv6
	mydnsUpdateIPv4 = func(e ddns.MyDNSEntry, url string) ddns.ProviderResult {
		*calls = append(*calls, "ipv4:"+e.Domain)
		return ddns.ProviderResult{}
	}
	mydnsUpdateIPv6 = func(e ddns.MyDNSEntry, url string) ddns.ProviderResult {
		*calls = append(*calls, "ipv6:"+e.Domain)
		return ddns.ProviderResult{}
	}
	t.Cleanup(func() { mydnsUpdateIPv4 = origV4; mydnsUpdateIPv6 = origV6 })
	return calls
}

// baseCfg builds a minimal Config for update tests.
func baseCfg(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		StateDir:   t.TempDir(),
		IPv4:       true,
		IPv4DDNS:   true,
		UpdateTime: 1,
		DDNSTime:   1,
	}
}

func TestUpdate_NoChange(t *testing.T) {
	cfg := baseCfg(t)
	cfg.MyDNS = []config.MyDNSEntry{{ID: "id0", Pass: "pass0", Domain: "home.example.com", IPv4: true}}

	// Pre-seed cached IP
	ip0 := "1.2.3.4"
	overrideFetch(t, fakeFetch(ip0, ""))
	calls := captureMyDNSCalls(t)

	// First run: IP written as new
	if err := Update(cfg); err != nil {
		t.Fatalf("first run error: %v", err)
	}
	firstCalls := len(*calls)

	// Second run with same IP: no DDNS call expected
	if err := Update(cfg); err != nil {
		t.Fatalf("second run error: %v", err)
	}
	if len(*calls) != firstCalls {
		t.Errorf("no-change: expected no additional DDNS calls after first run, got %d total", len(*calls))
	}
}

func TestUpdate_IPChanged(t *testing.T) {
	cfg := baseCfg(t)
	cfg.MyDNS = []config.MyDNSEntry{{ID: "id0", Pass: "pass0", Domain: "home.example.com", IPv4: true}}

	calls := captureMyDNSCalls(t)

	// First run
	overrideFetch(t, fakeFetch("1.2.3.4", ""))
	if err := Update(cfg); err != nil {
		t.Fatalf("first run: %v", err)
	}

	// Reset update gate so second run is allowed
	_ = os.Remove(cfg.StateDir + "/gate_update")
	_ = os.Remove(cfg.StateDir + "/gate_ddns")

	// Second run with different IP
	overrideFetch(t, fakeFetch("5.6.7.8", ""))
	if err := Update(cfg); err != nil {
		t.Fatalf("second run: %v", err)
	}

	if len(*calls) < 2 {
		t.Errorf("expected DDNS calls on both runs, got %d", len(*calls))
	}
}

func TestUpdate_DDNSError_RecordedInState(t *testing.T) {
	cfg := baseCfg(t)
	cfg.MyDNS = []config.MyDNSEntry{{ID: "id0", Pass: "pass0", Domain: "home.example.com", IPv4: true}}

	overrideFetch(t, fakeFetch("1.2.3.4", ""))

	orig := mydnsUpdateIPv4
	mydnsUpdateIPv4 = func(e ddns.MyDNSEntry, url string) ddns.ProviderResult {
		return ddns.ProviderResult{Err: errors.New("timeout")}
	}
	t.Cleanup(func() { mydnsUpdateIPv4 = orig })

	err := Update(cfg)
	if err == nil {
		t.Error("expected error from DDNS failure")
	}
}

// TestUpdate_IPv6FetchFail_IPv4Proceeds is the regression test for issue #11.
// When IPv6 fetch fails (e.g. the host has no IPv6 connectivity), the update
// must still proceed and update DDNS for IPv4.
func TestUpdate_IPv6FetchFail_IPv4Proceeds(t *testing.T) {
	cfg := baseCfg(t)
	cfg.IPv6 = true
	cfg.IPv6DDNS = true
	cfg.MyDNS = []config.MyDNSEntry{
		{ID: "id0", Pass: "pass0", Domain: "home.example.com", IPv4: true, IPv6: true},
	}

	overrideFetch(t, fakeFetchIPv6Err("1.2.3.4", errors.New("dig ipv6: exit status 1")))
	calls := captureMyDNSCalls(t)

	if err := Update(cfg); err != nil {
		t.Fatalf("IPv6 failure should not abort the update: %v", err)
	}

	hasV4 := false
	for _, c := range *calls {
		if c == "ipv4:home.example.com" {
			hasV4 = true
		}
		if c == "ipv6:home.example.com" {
			t.Error("IPv6 DDNS must not be called when IPv6 fetch failed")
		}
	}
	if !hasV4 {
		t.Error("IPv4 DDNS must still run even when IPv6 fetch failed")
	}
}

// TestUpdate_ForceSync verifies that UPDATE_TIME triggers a DDNS re-sync even
// when the IP has not changed (catches external DDNS edits / drift).
func TestUpdate_ForceSync(t *testing.T) {
	cfg := baseCfg(t)
	cfg.MyDNS = []config.MyDNSEntry{{ID: "id0", Pass: "pass0", Domain: "home.example.com", IPv4: true}}

	overrideFetch(t, fakeFetch("1.2.3.4", ""))
	calls := captureMyDNSCalls(t)

	// First run — IP written to cache, DDNS called.
	if err := Update(cfg); err != nil {
		t.Fatalf("first run: %v", err)
	}
	after1 := len(*calls)
	if after1 == 0 {
		t.Fatal("expected DDNS call on first run")
	}

	// Second run — same IP, gate_update still active → skip.
	if err := Update(cfg); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(*calls) != after1 {
		t.Errorf("expected no DDNS call when IP unchanged and gate active")
	}

	// Remove only the update gate to simulate UPDATE_TIME elapsed.
	_ = os.Remove(cfg.StateDir + "/gate_update")

	// Third run — same IP, but force-sync gate elapsed → DDNS must be called.
	if err := Update(cfg); err != nil {
		t.Fatalf("force-sync run: %v", err)
	}
	if len(*calls) <= after1 {
		t.Errorf("expected DDNS call on force-sync run (IP unchanged but UPDATE_TIME elapsed)")
	}
}

// TestUpdate_InitialCacheIsZero verifies that a fresh install (no state files)
// always triggers a DDNS update, because the implicit cache is 0.0.0.0.
func TestUpdate_InitialCacheIsZero(t *testing.T) {
	cfg := baseCfg(t)
	cfg.MyDNS = []config.MyDNSEntry{{ID: "id0", Pass: "pass0", Domain: "home.example.com", IPv4: true}}

	overrideFetch(t, fakeFetch("1.2.3.4", ""))
	calls := captureMyDNSCalls(t)

	// State dir is empty (t.TempDir) — no ip_ipv4 file exists.
	if err := Update(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(*calls) == 0 {
		t.Error("expected DDNS call on first run with empty state (cache=0.0.0.0)")
	}
}

func TestUpdate_PerEntryIPv4IPv6(t *testing.T) {
	cfg := baseCfg(t)
	cfg.IPv6 = true
	cfg.IPv6DDNS = true
	cfg.MyDNS = []config.MyDNSEntry{
		{ID: "id0", Pass: "pass0", Domain: "a.example.com", IPv4: true, IPv6: false},
		{ID: "id1", Pass: "pass1", Domain: "b.example.com", IPv4: false, IPv6: true},
	}

	overrideFetch(t, fakeFetch("1.2.3.4", "::1"))
	calls := captureMyDNSCalls(t)

	if err := Update(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hasV4a := false
	hasV6b := false
	for _, c := range *calls {
		if c == "ipv4:a.example.com" {
			hasV4a = true
		}
		if c == "ipv6:b.example.com" {
			hasV6b = true
		}
	}
	if !hasV4a {
		t.Error("expected IPv4 update for a.example.com")
	}
	if !hasV6b {
		t.Error("expected IPv6 update for b.example.com")
	}
	// Ensure IPv6 wasn't called for entry[0] (IPv6=false)
	for _, c := range *calls {
		if c == "ipv6:a.example.com" {
			t.Error("IPv6 should be skipped for a.example.com (IPv6=false)")
		}
	}
}
