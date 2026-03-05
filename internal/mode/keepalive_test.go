package mode

import (
	"strings"
	"testing"

	"github.com/Liplus-Project/dipper_ai/internal/config"
	"github.com/Liplus-Project/dipper_ai/internal/ddns"
)

// TestKeepalive_ForceUpdate verifies that Keepalive always sends DDNS updates
// for all MyDNS entries regardless of whether the IP has changed.
func TestKeepalive_ForceUpdate(t *testing.T) {
	cfg := baseCfg(t)
	cfg.MyDNS = []config.MyDNSEntry{{ID: "id0", Pass: "pass0", Domain: "home.example.com", IPv4: true}}

	overrideFetch(t, fakeFetch("1.2.3.4", ""))
	calls := captureMyDNSCalls(t)

	// First call — seeds the domain cache.
	if err := Keepalive(cfg); err != nil {
		t.Fatalf("first keepalive: %v", err)
	}
	after1 := len(*calls)
	if after1 == 0 {
		t.Fatal("expected DDNS call on first keepalive")
	}

	// Second call — same IP, but Keepalive always fires.
	if err := Keepalive(cfg); err != nil {
		t.Fatalf("second keepalive: %v", err)
	}
	if len(*calls) <= after1 {
		t.Errorf("expected DDNS call on second keepalive (force), got none")
	}
}

// TestKeepalive_CloudflareSkipped verifies that Cloudflare entries are never
// updated by Keepalive — only MyDNS providers need periodic keepalive.
func TestKeepalive_CloudflareSkipped(t *testing.T) {
	cfg := baseCfg(t)
	cfg.MyDNS = []config.MyDNSEntry{{ID: "id0", Pass: "pass0", Domain: "home.example.com", IPv4: true}}

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
	captureMyDNSCalls(t) // mock MyDNS so it doesn't make real HTTP calls

	if err := Keepalive(cfg); err != nil {
		t.Fatalf("keepalive: %v", err)
	}
	if len(*cfCalls) != 0 {
		t.Errorf("Cloudflare must NOT be called by Keepalive; got %d call(s)", len(*cfCalls))
	}
}

// TestKeepalive_Mail verifies that EMAIL_UP_DDNS=on sends a notification after
// a successful keepalive run.
func TestKeepalive_Mail(t *testing.T) {
	cfg := baseCfg(t)
	cfg.EmailAddr = "test@example.com"
	cfg.EmailUpDDNS = true
	cfg.MyDNS = []config.MyDNSEntry{{ID: "id0", Pass: "pass0", Domain: "home.example.com", IPv4: true}}

	overrideFetch(t, fakeFetch("1.2.3.4", ""))
	captureMyDNSCalls(t)
	sent := captureMailCalls(t)

	if err := Keepalive(cfg); err != nil {
		t.Fatalf("keepalive: %v", err)
	}
	if len(*sent) == 0 {
		t.Fatal("expected mail when EMAIL_UP_DDNS=true")
	}
	mail := (*sent)[0]
	if !strings.Contains(mail, "keepalive") {
		t.Errorf("mail body should mention keepalive, got: %s", mail)
	}
}

// TestKeepalive_MailOffWhenDisabled verifies that EMAIL_UP_DDNS=false suppresses
// the keepalive notification.
func TestKeepalive_MailOffWhenDisabled(t *testing.T) {
	cfg := baseCfg(t)
	cfg.EmailAddr = "test@example.com"
	cfg.EmailUpDDNS = false
	cfg.MyDNS = []config.MyDNSEntry{{ID: "id0", Pass: "pass0", Domain: "home.example.com", IPv4: true}}

	overrideFetch(t, fakeFetch("1.2.3.4", ""))
	captureMyDNSCalls(t)
	sent := captureMailCalls(t)

	if err := Keepalive(cfg); err != nil {
		t.Fatalf("keepalive: %v", err)
	}
	if len(*sent) != 0 {
		t.Errorf("expected no mail when EMAIL_UP_DDNS=false, got %d", len(*sent))
	}
}
