// Package acceptance runs black-box tests against the dipper_ai binary.
// The binary is built once per test run using go build.
package acceptance

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var binaryPath string

// TestMain builds the binary once before running all acceptance tests.
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "dipper_ai_bin_*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmp)

	binaryPath = filepath.Join(tmp, "dipper_ai")
	build := exec.Command("go", "build", "-o", binaryPath, "github.com/Liplus-Project/dipper_ai/cmd/dipper_ai")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "build failed: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// writeConf writes a minimal user.conf for a test and returns its path.
// IPV4 and IPV6 are off by default to avoid requiring dig in CI.
// Pass extra lines to override individual settings.
func writeConf(t *testing.T, stateDir string, extra string) string {
	t.Helper()
	content := fmt.Sprintf("STATE_DIR=%s\nIPV4=off\nIPV6=off\nUPDATE_TIME=1\nDDNS_TIME=1\nIP_CACHE_TIME=0\nERR_CHK_TIME=0\n%s", stateDir, extra)
	p := filepath.Join(t.TempDir(), "user.conf")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

// runBinary executes dipper_ai with the given args, conf path, and extra env vars.
func runBinary(t *testing.T, confPath string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	return runBinaryEnv(t, confPath, nil, args...)
}

// runBinaryEnv executes dipper_ai with additional environment variables.
func runBinaryEnv(t *testing.T, confPath string, extraEnv []string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(binaryPath, args...)
	cmd.Env = append(os.Environ(), "DIPPER_AI_CONFIG="+confPath)
	cmd.Env = append(cmd.Env, extraEnv...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode = 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			t.Fatalf("unexpected run error: %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// --- update mode ---

func TestBinary_Update_ExitZero(t *testing.T) {
	stateDir := t.TempDir()
	conf := writeConf(t, stateDir, "")

	// No DDNS providers configured — update should still exit 0
	_, _, code := runBinary(t, conf, "update")
	if code != 0 {
		t.Errorf("expected exit 0 for update with no providers, got %d", code)
	}
}

// --- check mode ---

func TestBinary_Check_NoProviders_ExitZero(t *testing.T) {
	stateDir := t.TempDir()
	// IPV4=off, IPV6=off (default from writeConf) — no providers configured.
	// check should exit 0 immediately without attempting any DNS lookup.
	conf := writeConf(t, stateDir, "")

	_, _, code := runBinary(t, conf, "check")
	if code != 0 {
		t.Errorf("expected exit 0 for check with no providers, got %d", code)
	}
}

// TestBinary_Check_AllMatch_SilentExit verifies that when all DNS records
// match the current IP, check exits 0 and produces no output.
// Uses DIPPER_AI_FAKE_IP_V4 and DIPPER_AI_FAKE_DNS to inject controlled values.
func TestBinary_Check_AllMatch_SilentExit(t *testing.T) {
	stateDir := t.TempDir()
	conf := writeConf(t, stateDir,
		"IPV4=on\nIPV4_DDNS=on\n"+
			"MYDNS_0_ID=testid\nMYDNS_0_PASS=testpass\nMYDNS_0_DOMAIN=home.example.com\nMYDNS_0_IPV4=on\n",
	)

	_, stderr, code := runBinaryEnv(t, conf,
		[]string{
			"DIPPER_AI_FAKE_IP_V4=1.2.3.4",
			"DIPPER_AI_FAKE_DNS=home.example.com=1.2.3.4", // DNS matches current IP
		},
		"check",
	)

	if code != 0 {
		t.Errorf("expected exit 0 when DNS matches, got %d (stderr: %s)", code, stderr)
	}
	if stderr != "" {
		t.Errorf("expected no stderr output when DNS matches, got: %q", stderr)
	}
}

// TestBinary_Check_Mismatch_TriggersUpdate verifies that when a domain's DNS
// record differs from the current IP, check detects the mismatch and triggers
// a DDNS update.  A local HTTP server stands in for the MyDNS endpoint.
func TestBinary_Check_Mismatch_TriggersUpdate(t *testing.T) {
	// Local HTTP server that accepts MyDNS login requests.
	updated := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		updated = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	stateDir := t.TempDir()
	conf := writeConf(t, stateDir,
		"IPV4=on\nIPV4_DDNS=on\n"+
			"MYDNS_0_ID=testid\nMYDNS_0_PASS=testpass\nMYDNS_0_DOMAIN=stale.example.com\nMYDNS_0_IPV4=on\n"+
			"MYDNS_IPV4_URL="+srv.URL+"\n",
	)

	_, stderr, code := runBinaryEnv(t, conf,
		[]string{
			"DIPPER_AI_FAKE_IP_V4=1.2.3.4",
			"DIPPER_AI_FAKE_DNS=stale.example.com=0.0.0.0", // stale — mismatch
		},
		"check",
	)

	if code != 0 {
		t.Errorf("expected exit 0 after mismatch+update, got %d (stderr: %s)", code, stderr)
	}
	if !updated {
		t.Error("expected DDNS update to be triggered on mismatch, but server was not called")
	}
	if !strings.Contains(stderr, "mismatch") {
		t.Errorf("expected mismatch log in stderr, got: %q", stderr)
	}
}

// TestBinary_Check_PartialMismatch_OnlyStaleUpdated verifies that when only
// one of two domains is stale, only that domain triggers a DDNS update.
func TestBinary_Check_PartialMismatch_OnlyStaleUpdated(t *testing.T) {
	var updatedDomains []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// MyDNS uses Basic Auth — extract user (=ID) to identify which domain was updated.
		user, _, _ := r.BasicAuth()
		updatedDomains = append(updatedDomains, user)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	stateDir := t.TempDir()
	conf := writeConf(t, stateDir,
		"IPV4=on\nIPV4_DDNS=on\n"+
			// Large UPDATE_TIME prevents forceSync so per-domain cache is the sole trigger.
			"UPDATE_TIME=1440\n"+
			// Entry 0: DNS ok (id=ok-id)
			"MYDNS_0_ID=ok-id\nMYDNS_0_PASS=pass\nMYDNS_0_DOMAIN=ok.example.com\nMYDNS_0_IPV4=on\n"+
			// Entry 1: DNS stale (id=stale-id)
			"MYDNS_1_ID=stale-id\nMYDNS_1_PASS=pass\nMYDNS_1_DOMAIN=stale.example.com\nMYDNS_1_IPV4=on\n"+
			"MYDNS_IPV4_URL="+srv.URL+"\n",
	)

	// Pre-seed the cache for ok.example.com so Update() sees no IP change for it.
	// Without this, an empty cache would cause Update() to update all entries on first run.
	if err := os.WriteFile(filepath.Join(stateDir, "cache_mydns_0_ipv4"), []byte("1.2.3.4\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, _, code := runBinaryEnv(t, conf,
		[]string{
			"DIPPER_AI_FAKE_IP_V4=1.2.3.4",
			"DIPPER_AI_FAKE_DNS=ok.example.com=1.2.3.4,stale.example.com=0.0.0.0",
		},
		"check",
	)

	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	for _, id := range updatedDomains {
		if id == "ok-id" {
			t.Error("ok.example.com should NOT have been updated (DNS already correct)")
		}
	}
	found := false
	for _, id := range updatedDomains {
		if id == "stale-id" {
			found = true
		}
	}
	if !found {
		t.Error("stale.example.com SHOULD have been updated (DNS was stale)")
	}
}

// --- err_mail mode ---

func TestBinary_ErrMail_Disabled(t *testing.T) {
	stateDir := t.TempDir()
	// ERR_CHK_TIME=0 disables err_mail — should exit 0 immediately
	conf := writeConf(t, stateDir, "ERR_CHK_TIME=0\n")

	_, _, code := runBinary(t, conf, "err_mail")
	if code != 0 {
		t.Errorf("expected exit 0 for disabled err_mail, got %d", code)
	}
}

// --- routing ---

func TestBinary_UnknownCommand_ExitOne(t *testing.T) {
	stateDir := t.TempDir()
	conf := writeConf(t, stateDir, "")

	_, stderr, code := runBinary(t, conf, "notacommand")
	if code != 1 {
		t.Errorf("expected exit 1 for unknown command, got %d", code)
	}
	if !strings.Contains(stderr, "notacommand") {
		t.Errorf("expected stderr to mention the unknown command, got: %q", stderr)
	}
}

func TestBinary_NoArgs_ExitOne(t *testing.T) {
	stateDir := t.TempDir()
	conf := writeConf(t, stateDir, "")

	_, stderr, code := runBinary(t, conf)
	if code != 1 {
		t.Errorf("expected exit 1 for no args, got %d", code)
	}
	if stderr == "" {
		t.Error("expected usage message on stderr")
	}
}
