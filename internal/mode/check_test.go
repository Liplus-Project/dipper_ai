package mode

import (
	"os"
	"testing"

	"github.com/Liplus-Project/dipper_ai/internal/config"
	"github.com/Liplus-Project/dipper_ai/internal/ip"
	"github.com/Liplus-Project/dipper_ai/internal/state"
)

func TestCheck_CacheDisabled_AlwaysFetches(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		StateDir:    dir,
		IPv4:        true,
		DDNSTime:    1,
		IPCacheTime: 0, // disabled
	}

	fetchCount := 0
	orig := ipFetch
	ipFetch = func(v4, v6 bool) (*ip.Result, error) {
		fetchCount++
		return &ip.Result{IPv4: "1.2.3.4"}, nil
	}
	t.Cleanup(func() { ipFetch = orig })

	// Run 3 times, resetting the check gate each time so timegate passes.
	for i := 0; i < 3; i++ {
		_ = os.Remove(dir + "/gate_check") // reset gate
		if err := Check(cfg); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	if fetchCount < 3 {
		t.Errorf("expected at least 3 fetches with IPCacheTime=0, got %d", fetchCount)
	}
}

func TestCheck_CacheEnabled_SkipsFetch(t *testing.T) {
	cfg := &config.Config{
		StateDir:    t.TempDir(),
		IPv4:        true,
		DDNSTime:    1,
		IPCacheTime: 60, // 60 minutes
	}

	fetchCount := 0
	orig := ipFetch
	ipFetch = func(v4, v6 bool) (*ip.Result, error) {
		fetchCount++
		return &ip.Result{IPv4: "1.2.3.4"}, nil
	}
	t.Cleanup(func() { ipFetch = orig })

	// First run fetches
	if err := Check(cfg); err != nil {
		t.Fatalf("first run: %v", err)
	}
	// Second run should use cached gate (not yet expired)
	if err := Check(cfg); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if fetchCount > 1 {
		t.Errorf("expected 1 fetch with cache enabled, got %d", fetchCount)
	}
}

func TestCheck_WritesIPToState(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		StateDir:    dir,
		IPv4:        true,
		DDNSTime:    1,
		IPCacheTime: 0,
	}

	orig := ipFetch
	ipFetch = func(v4, v6 bool) (*ip.Result, error) {
		return &ip.Result{IPv4: "9.8.7.6"}, nil
	}
	t.Cleanup(func() { ipFetch = orig })

	if err := Check(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	st, _ := state.New(dir)
	got, _ := st.ReadIP("ipv4")
	if got != "9.8.7.6" {
		t.Errorf("expected state ipv4=9.8.7.6, got %q", got)
	}
}
