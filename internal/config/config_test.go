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
	if cfg.UpdateTime != 60 {
		t.Errorf("UpdateTime default: got %d, want 60", cfg.UpdateTime)
	}
	if !cfg.IPv4 {
		t.Error("IPv4 default should be true")
	}
	if !cfg.IPv6 {
		t.Error("IPv6 default should be true")
	}
	if cfg.MyDNS.Enabled {
		t.Error("MYDNS_ENABLED default should be false")
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
	if cfg.UpdateTime < 1 {
		t.Errorf("UpdateTime minimum: got %d", cfg.UpdateTime)
	}
	if cfg.DDNSTime < 1 {
		t.Errorf("DDNSTime minimum: got %d", cfg.DDNSTime)
	}
}

func TestMyDNSRequired(t *testing.T) {
	path := writeConf(t, "MYDNS_ENABLED=on\n")
	_, err := ParseFile(path)
	if err == nil {
		t.Error("expected error when MYDNS_ENABLED=on but credentials missing")
	}
}

func TestMyDNSFull(t *testing.T) {
	path := writeConf(t, `
MYDNS_ENABLED=on
MYDNS_MASTERID=testid
MYDNS_PASSWORD=testpass
`)
	cfg, err := ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.MyDNS.Enabled {
		t.Error("MyDNS.Enabled should be true")
	}
	if cfg.MyDNS.MasterID != "testid" {
		t.Errorf("MasterID: got %q", cfg.MyDNS.MasterID)
	}
}

func TestCloudflareMultiple(t *testing.T) {
	path := writeConf(t, `
CF_0_ENABLED=on
CF_0_TOKEN=tok0
CF_0_ZONE_ID=zone0
CF_0_NAME=a.example.com

CF_1_ENABLED=on
CF_1_TOKEN=tok1
CF_1_ZONE_ID=zone1
CF_1_NAME=b.example.com
`)
	cfg, err := ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Cloudflare) != 2 {
		t.Fatalf("expected 2 CF entries, got %d", len(cfg.Cloudflare))
	}
	if cfg.Cloudflare[0].Name != "a.example.com" {
		t.Errorf("CF[0].Name: got %q", cfg.Cloudflare[0].Name)
	}
	if cfg.Cloudflare[1].Token != "tok1" {
		t.Errorf("CF[1].Token: got %q", cfg.Cloudflare[1].Token)
	}
}

func TestCloudflareDisabledNoValidation(t *testing.T) {
	// Disabled entry should not require token/zone/name
	path := writeConf(t, `
CF_0_ENABLED=off
CF_0_TOKEN=
CF_0_ZONE_ID=
CF_0_NAME=
`)
	_, err := ParseFile(path)
	if err != nil {
		t.Errorf("disabled CF entry should not require fields: %v", err)
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

func TestErrMailRequired(t *testing.T) {
	path := writeConf(t, "ERR_MAIL_ENABLED=on\n")
	_, err := ParseFile(path)
	if err == nil {
		t.Error("expected error: ERR_MAIL_ENABLED=on without MAIL_TO")
	}
}
