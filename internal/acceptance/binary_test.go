// Package acceptance runs black-box tests against the dipper_ai binary.
// The binary is built once per test run using go build.
package acceptance

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// runBinary executes dipper_ai with the given args and conf path.
func runBinary(t *testing.T, confPath string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(binaryPath, args...)
	cmd.Env = append(os.Environ(), "DIPPER_AI_CONFIG="+confPath)
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

func TestBinary_Update_ExitZero(t *testing.T) {
	stateDir := t.TempDir()
	conf := writeConf(t, stateDir, "")

	// No DDNS providers configured — update should still exit 0
	_, _, code := runBinary(t, conf, "update")
	if code != 0 {
		t.Errorf("expected exit 0 for update with no providers, got %d", code)
	}
}

func TestBinary_Check_OutputsIPLine(t *testing.T) {
	stateDir := t.TempDir()
	// IPV4=on with a positive cache time so check reads the cached IP
	// without calling dig. IPV6 stays off.
	conf := writeConf(t, stateDir, "IPV4=on\nIP_CACHE_TIME=60\n")

	// Pre-seed cached IP state.
	if err := os.WriteFile(filepath.Join(stateDir, "ip_ipv4"), []byte("1.2.3.4\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Pre-seed ip_cache gate as "just touched" so no dig call is made.
	// timegate uses time.RFC3339, not a Unix integer.
	ts := time.Now().Format(time.RFC3339)
	if err := os.WriteFile(filepath.Join(stateDir, "gate_ip_cache"), []byte(ts), 0644); err != nil {
		t.Fatal(err)
	}

	stdout, _, code := runBinary(t, conf, "check")
	if code != 0 {
		t.Errorf("expected exit 0 for check, got %d", code)
	}
	if !strings.Contains(stdout, "ipv4:") {
		t.Errorf("expected stdout to contain 'ipv4:', got: %q", stdout)
	}
}

func TestBinary_ErrMail_Disabled(t *testing.T) {
	stateDir := t.TempDir()
	// ERR_CHK_TIME=0 disables err_mail — should exit 0 immediately
	conf := writeConf(t, stateDir, "ERR_CHK_TIME=0\n")

	_, _, code := runBinary(t, conf, "err_mail")
	if code != 0 {
		t.Errorf("expected exit 0 for disabled err_mail, got %d", code)
	}
}

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
