package mode

import (
	"errors"
	"os"
	"strings"
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
// DDNSTime defaults to 0 (keepalive disabled) — IP-change only.
func baseCfg(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		StateDir:   t.TempDir(),
		IPv4:       true,
		IPv4DDNS:   true,
		UpdateTime: 1,
		DDNSTime:   0, // keepalive disabled by default
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

	// IP change bypasses the DDNS gate — no need to remove any gate files.

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

// TestUpdate_Keepalive verifies that when UPDATE_TIME elapses, all MyDNS entries
// are force-updated even when the IP has not changed (MyDNS keepalive).
func TestUpdate_Keepalive(t *testing.T) {
	cfg := baseCfg(t)
	// DDNSTime=0: no rate limit. UpdateTime=1: keepalive gate interval.
	cfg.UpdateTime = 1
	cfg.MyDNS = []config.MyDNSEntry{{ID: "id0", Pass: "pass0", Domain: "home.example.com", IPv4: true}}

	overrideFetch(t, fakeFetch("1.2.3.4", ""))
	calls := captureMyDNSCalls(t)

	// First run — per-domain cache empty (0.0.0.0) → IP changed → DDNS called, gate_update touched.
	if err := Update(cfg); err != nil {
		t.Fatalf("first run: %v", err)
	}
	after1 := len(*calls)
	if after1 == 0 {
		t.Fatal("expected DDNS call on first run")
	}

	// Second run — same IP, gate_update still active → no forceSync → skip.
	if err := Update(cfg); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(*calls) != after1 {
		t.Errorf("expected no DDNS call when IP unchanged and UPDATE_TIME gate active")
	}

	// Remove gate_update to simulate UPDATE_TIME elapsed.
	_ = os.Remove(cfg.StateDir + "/gate_update")

	// Third run — same IP, but UPDATE_TIME elapsed → forceSync → MyDNS must fire.
	if err := Update(cfg); err != nil {
		t.Fatalf("keepalive run: %v", err)
	}
	if len(*calls) <= after1 {
		t.Errorf("expected keepalive DDNS call when UPDATE_TIME elapsed (IP unchanged)")
	}
}

// TestUpdate_CloudflareNoKeepalive verifies that Cloudflare entries are NOT
// updated on UPDATE_TIME keepalive — only on IP change.
func TestUpdate_CloudflareNoKeepalive(t *testing.T) {
	cfg := baseCfg(t)
	cfg.UpdateTime = 1
	cfCalls := &[]string{}
	origCF := cloudflareUpdate
	cloudflareUpdate = func(e ddns.CloudflareEntry, ip, recType, url string) ddns.ProviderResult {
		*cfCalls = append(*cfCalls, recType+":"+e.Domain)
		return ddns.ProviderResult{}
	}
	t.Cleanup(func() { cloudflareUpdate = origCF })

	cfg.Cloudflare = []config.CloudflareEntry{
		{Enabled: true, API: "tok", Zone: "example.com", Domain: "home.example.com", IPv4: true},
	}

	overrideFetch(t, fakeFetch("1.2.3.4", ""))

	// First run: cache empty → IP changed → CF updated.
	if err := Update(cfg); err != nil {
		t.Fatalf("first run: %v", err)
	}
	after1 := len(*cfCalls)
	if after1 == 0 {
		t.Fatal("expected CF call on first run (IP changed)")
	}

	// Remove gate_update to simulate UPDATE_TIME elapsed.
	_ = os.Remove(cfg.StateDir + "/gate_update")

	// Second run: same IP, UPDATE_TIME elapsed → forceSync → MyDNS fires but CF must NOT.
	if err := Update(cfg); err != nil {
		t.Fatalf("keepalive run: %v", err)
	}
	if len(*cfCalls) != after1 {
		t.Errorf("Cloudflare must NOT be updated on keepalive (forceSync); got %d extra calls", len(*cfCalls)-after1)
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

// captureMailCalls replaces sendMailFn for the duration of a test.
// Returns a pointer to a slice of recorded "to|subject|body" strings.
func captureMailCalls(t *testing.T) *[]string {
	t.Helper()
	sent := &[]string{}
	orig := sendMailFn
	sendMailFn = func(to, subject, body string) error {
		*sent = append(*sent, to+"|"+subject+"|"+body)
		return nil
	}
	t.Cleanup(func() { sendMailFn = orig })
	return sent
}

// TestUpdate_Mail_IPChanged verifies that when EMAIL_CHK_DDNS is on and the IP
// changes, a mail notification is sent containing the new IP and provider list.
func TestUpdate_Mail_IPChanged(t *testing.T) {
	cfg := baseCfg(t)
	cfg.EmailAddr = "test@example.com"
	cfg.EmailChkDDNS = true
	cfg.MyDNS = []config.MyDNSEntry{{ID: "id0", Pass: "pass0", Domain: "home.example.com", IPv4: true}}

	overrideFetch(t, fakeFetch("1.2.3.4", ""))
	captureMyDNSCalls(t)
	sent := captureMailCalls(t)

	if err := Update(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(*sent) == 0 {
		t.Fatal("expected mail to be sent on IP change")
	}
	mail := (*sent)[0]
	if !strings.Contains(mail, "1.2.3.4") {
		t.Errorf("mail body should contain new IPv4, got: %s", mail)
	}
	if !strings.Contains(mail, "IP changed") {
		t.Errorf("mail body should contain reason 'IP changed', got: %s", mail)
	}
	if !strings.Contains(mail, "home.example.com") {
		t.Errorf("mail body should contain domain, got: %s", mail)
	}
}

// TestUpdate_Mail_Keepalive verifies that EMAIL_UP_DDNS triggers mail on
// keepalive updates (IP unchanged, DDNS_TIME elapsed), not on IP-change runs.
func TestUpdate_Mail_Keepalive(t *testing.T) {
	cfg := baseCfg(t)
	cfg.UpdateTime = 1 // keepalive gate interval
	cfg.EmailAddr = "test@example.com"
	cfg.EmailUpDDNS = true // notify on keepalive only
	cfg.EmailChkDDNS = false
	cfg.MyDNS = []config.MyDNSEntry{{ID: "id0", Pass: "pass0", Domain: "home.example.com", IPv4: true}}

	overrideFetch(t, fakeFetch("1.2.3.4", ""))
	captureMyDNSCalls(t)
	sent := captureMailCalls(t)

	// First run: IP changed (0.0.0.0 → 1.2.3.4) → EMAIL_CHK_DDNS=false → no mail.
	if err := Update(cfg); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if len(*sent) != 0 {
		t.Errorf("no mail expected on IP-change run when EMAIL_CHK_DDNS=false, got %d", len(*sent))
	}

	// Simulate UPDATE_TIME elapsed (keepalive).
	_ = os.Remove(cfg.StateDir + "/gate_update")

	// Second run: same IP, UPDATE_TIME elapsed → forceSync → EMAIL_UP_DDNS=true → mail expected.
	if err := Update(cfg); err != nil {
		t.Fatalf("keepalive run: %v", err)
	}
	if len(*sent) == 0 {
		t.Error("expected mail on keepalive run when EMAIL_UP_DDNS=true")
	} else {
		mail := (*sent)[0]
		if !strings.Contains(mail, "DDNS keepalive") {
			t.Errorf("mail body should contain reason 'DDNS keepalive', got: %s", mail)
		}
	}
}

// TestUpdate_Mail_BothOff verifies that no mail is sent when both
// EMAIL_CHK_DDNS and EMAIL_UP_DDNS are false.
func TestUpdate_Mail_BothOff(t *testing.T) {
	cfg := baseCfg(t)
	cfg.EmailAddr = "test@example.com"
	cfg.EmailChkDDNS = false
	cfg.EmailUpDDNS = false
	cfg.MyDNS = []config.MyDNSEntry{{ID: "id0", Pass: "pass0", Domain: "home.example.com", IPv4: true}}

	overrideFetch(t, fakeFetch("1.2.3.4", ""))
	captureMyDNSCalls(t)
	sent := captureMailCalls(t)

	if err := Update(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(*sent) != 0 {
		t.Errorf("expected no mail when both flags off, got %d", len(*sent))
	}
}

// TestUpdate_Mail_FailureIsNonFatal verifies that a mail-send failure does not
// cause Update() to return an error (DDNS update itself succeeded).
func TestUpdate_Mail_FailureIsNonFatal(t *testing.T) {
	cfg := baseCfg(t)
	cfg.EmailAddr = "test@example.com"
	cfg.EmailChkDDNS = true
	cfg.MyDNS = []config.MyDNSEntry{{ID: "id0", Pass: "pass0", Domain: "home.example.com", IPv4: true}}

	overrideFetch(t, fakeFetch("1.2.3.4", ""))
	captureMyDNSCalls(t)

	orig := sendMailFn
	sendMailFn = func(_, _, _ string) error { return errors.New("sendmail: connection refused") }
	t.Cleanup(func() { sendMailFn = orig })

	if err := Update(cfg); err != nil {
		t.Errorf("mail failure should be non-fatal, got error: %v", err)
	}
}
