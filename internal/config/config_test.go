package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConf(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "user.conf")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDefaults(t *testing.T) {
	path := writeConf(t, "# empty config\n")
	cfg, err := ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.UpdateTime != 1440 {
		t.Errorf("UpdateTime default: got %d, want 1440", cfg.UpdateTime)
	}
	if !cfg.IPv4 {
		t.Error("IPv4 default should be true")
	}
	if cfg.IPv6 {
		t.Error("IPv6 default should be false")
	}
	if len(cfg.MyDNS) != 0 {
		t.Error("MyDNS default should be empty")
	}
	if cfg.MyDNSIPv4URL != "https://ipv4.mydns.jp/login.html" {
		t.Errorf("MyDNSIPv4URL default: got %q", cfg.MyDNSIPv4URL)
	}
	if cfg.CloudflareURL != "https://api.cloudflare.com/client/v4/zones" {
		t.Errorf("CloudflareURL default: got %q", cfg.CloudflareURL)
	}
}

func TestBoolVariants(t *testing.T) {
	cases := []struct {
		val  string
		want bool
	}{
		{"on", true},
		{"On", true},
		{"ON", true},
		{"1", true},
		{"true", true},
		{"off", false},
		{"0", false},
		{"false", false},
	}
	for _, tc := range cases {
		path := writeConf(t, "IPV4="+tc.val+"\n")
		cfg, err := ParseFile(path)
		if err != nil {
			t.Fatalf("val=%q: unexpected error: %v", tc.val, err)
		}
		if cfg.IPv4 != tc.want {
			t.Errorf("val=%q: got %v, want %v", tc.val, cfg.IPv4, tc.want)
		}
	}
}

func TestTimegateMinimum(t *testing.T) {
	path := writeConf(t, "UPDATE_TIME=0\nDDNS_TIME=-5\n")
	cfg, err := ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.UpdateTime < 3 {
		t.Errorf("UpdateTime minimum: got %d, want >= 3", cfg.UpdateTime)
	}
	if cfg.DDNSTime < 1 {
		t.Errorf("DDNSTime minimum: got %d, want >= 1", cfg.DDNSTime)
	}
}

func TestIPCacheDisabled(t *testing.T) {
	path := writeConf(t, "IP_CACHE_TIME=0\nERR_CHK_TIME=0\n")
	cfg, err := ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.IPCacheTime != 0 {
		t.Errorf("IPCacheTime=0 should be disabled, got %d", cfg.IPCacheTime)
	}
	if cfg.ErrChkTime != 0 {
		t.Errorf("ErrChkTime=0 should be disabled, got %d", cfg.ErrChkTime)
	}
}

func TestIPCacheMinimum(t *testing.T) {
	path := writeConf(t, "IP_CACHE_TIME=5\n")
	cfg, err := ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.IPCacheTime < 15 {
		t.Errorf("IPCacheTime minimum: got %d, want >= 15", cfg.IPCacheTime)
	}
}

func TestMyDNSSingle(t *testing.T) {
	path := writeConf(t, `
MYDNS_0_ID=testid
MYDNS_0_PASS=testpass
MYDNS_0_DOMAIN=home.example.com
MYDNS_0_IPV4=on
MYDNS_0_IPV6=off
`)
	cfg, err := ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.MyDNS) != 1 {
		t.Fatalf("expected 1 MyDNS entry, got %d", len(cfg.MyDNS))
	}
	e := cfg.MyDNS[0]
	if e.ID != "testid" {
		t.Errorf("ID: got %q", e.ID)
	}
	if e.Pass != "testpass" {
		t.Errorf("Pass: got %q", e.Pass)
	}
	if !e.IPv4 {
		t.Error("IPv4 should be true")
	}
	if e.IPv6 {
		t.Error("IPv6 should be false")
	}
}

func TestMyDNSMultiple(t *testing.T) {
	path := writeConf(t, `
MYDNS_0_ID=id0
MYDNS_0_PASS=pass0
MYDNS_0_DOMAIN=a.example.com
MYDNS_1_ID=id1
MYDNS_1_PASS=pass1
MYDNS_1_DOMAIN=b.example.com
`)
	cfg, err := ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.MyDNS) != 2 {
		t.Fatalf("expected 2 MyDNS entries, got %d", len(cfg.MyDNS))
	}
	if cfg.MyDNS[0].ID != "id0" {
		t.Errorf("MyDNS[0].ID: got %q", cfg.MyDNS[0].ID)
	}
	if cfg.MyDNS[1].Domain != "b.example.com" {
		t.Errorf("MyDNS[1].Domain: got %q", cfg.MyDNS[1].Domain)
	}
}

func TestMyDNSPassRequired(t *testing.T) {
	path := writeConf(t, "MYDNS_0_ID=testid\n")
	_, err := ParseFile(path)
	if err == nil {
		t.Error("expected error when MYDNS_0_PASS is missing")
	}
}

func TestMyDNSCustomURL(t *testing.T) {
	path := writeConf(t, `
MYDNS_IPV4_URL=https://custom.ipv4.example.com/login
MYDNS_IPV6_URL=https://custom.ipv6.example.com/login
`)
	cfg, err := ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MyDNSIPv4URL != "https://custom.ipv4.example.com/login" {
		t.Errorf("MyDNSIPv4URL: got %q", cfg.MyDNSIPv4URL)
	}
}

func TestCloudflareMultiple(t *testing.T) {
	path := writeConf(t, `
CF_0_ENABLED=on
CF_0_API=tok0
CF_0_ZONE=example.com
CF_0_DOMAIN=a.example.com

CF_1_ENABLED=on
CF_1_API=tok1
CF_1_ZONE=other.com
CF_1_DOMAIN=b.other.com
CF_1_IPV6=on
`)
	cfg, err := ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Cloudflare) != 2 {
		t.Fatalf("expected 2 CF entries, got %d", len(cfg.Cloudflare))
	}
	if cfg.Cloudflare[0].Zone != "example.com" {
		t.Errorf("CF[0].Zone: got %q", cfg.Cloudflare[0].Zone)
	}
	if cfg.Cloudflare[1].API != "tok1" {
		t.Errorf("CF[1].API: got %q", cfg.Cloudflare[1].API)
	}
	if !cfg.Cloudflare[1].IPv6 {
		t.Error("CF[1].IPv6 should be true")
	}
}

func TestCloudflareDisabledNoValidation(t *testing.T) {
	path := writeConf(t, `
CF_0_ENABLED=off
CF_0_API=
CF_0_ZONE=
CF_0_DOMAIN=
`)
	_, err := ParseFile(path)
	if err != nil {
		t.Errorf("disabled CF entry should not require fields: %v", err)
	}
}

func TestCloudflareEnabledRequiresFields(t *testing.T) {
	path := writeConf(t, "CF_0_ENABLED=on\n")
	_, err := ParseFile(path)
	if err == nil {
		t.Error("expected error: CF_0_ENABLED=on without API/ZONE/DOMAIN")
	}
}

func TestEmailChkDDNS(t *testing.T) {
	path := writeConf(t, `
EMAIL_CHK_DDNS=on
EMAIL_ADR=test@example.com
`)
	cfg, err := ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.EmailChkDDNS {
		t.Error("EmailChkDDNS should be true")
	}
	if cfg.EmailAddr != "test@example.com" {
		t.Errorf("EmailAddr: got %q", cfg.EmailAddr)
	}
}

func TestEmailAddrRequired(t *testing.T) {
	path := writeConf(t, "EMAIL_UP_DDNS=on\n")
	_, err := ParseFile(path)
	if err == nil {
		t.Error("expected error: EMAIL_UP_DDNS=on without EMAIL_ADR")
	}
}

func TestInlineComment(t *testing.T) {
	path := writeConf(t, "UPDATE_TIME=30 # every 30 min\n")
	cfg, err := ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.UpdateTime != 30 {
		t.Errorf("inline comment stripping failed: got %d", cfg.UpdateTime)
	}
}

func TestQuotedValue(t *testing.T) {
	path := writeConf(t, "MYDNS_0_ID=\"quotedid\"\nMYDNS_0_PASS=\"quotedpass\"\n")
	cfg, err := ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.MyDNS) == 1 && cfg.MyDNS[0].ID != "quotedid" {
		t.Errorf("quoted value stripping failed: got %q", cfg.MyDNS[0].ID)
	}
}

func TestStateDir(t *testing.T) {
	path := writeConf(t, "STATE_DIR=/tmp/mystate\n")
	cfg, err := ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.StateDir != "/tmp/mystate" {
		t.Errorf("StateDir: got %q", cfg.StateDir)
	}
}
