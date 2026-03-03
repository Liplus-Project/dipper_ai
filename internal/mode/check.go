package mode

import (
	"fmt"
	"time"

	"github.com/Liplus-Project/dipper_ai/internal/config"
	"github.com/Liplus-Project/dipper_ai/internal/ip"
	"github.com/Liplus-Project/dipper_ai/internal/state"
	"github.com/Liplus-Project/dipper_ai/internal/timegate"
)

// Check reports the current IP and cached DDNS state.
// Equivalent to `dipper check`.
func Check(cfg *config.Config) error {
	st, err := state.New(cfg.StateDir)
	if err != nil {
		return err
	}

	// --- Time gate: DDNS_TIME (check gate) ---
	checkGate := timegate.New(cfg.StateDir, "check", time.Duration(cfg.DDNSTime)*time.Minute)
	if !checkGate.ShouldRun() {
		return nil
	}

	// --- IP cache gate (0 = disabled: always refresh) ---
	shouldRefresh := true
	if cfg.IPCacheTime > 0 {
		ipCacheGate := timegate.New(cfg.StateDir, "ip_cache", time.Duration(cfg.IPCacheTime)*time.Minute)
		shouldRefresh = ipCacheGate.ShouldRun()
		if shouldRefresh {
			defer func() { _ = ipCacheGate.Touch() }()
		}
	}
	if shouldRefresh {
		fetched, err := ip.Fetch(cfg.IPv4, cfg.IPv6)
		if err != nil {
			_ = st.AppendError(fmt.Sprintf("check_ip_fetch_error: %v", err))
			return err
		}
		if fetched.IPv4 != "" {
			_ = st.WriteIP("ipv4", fetched.IPv4)
		}
		if fetched.IPv6 != "" {
			_ = st.WriteIP("ipv6", fetched.IPv6)
		}
	}

	// Output current state to stdout
	if cfg.IPv4 {
		v4, _ := st.ReadIP("ipv4")
		fmt.Printf("ipv4: %s\n", v4)
	}
	if cfg.IPv6 {
		v6, _ := st.ReadIP("ipv6")
		fmt.Printf("ipv6: %s\n", v6)
	}

	_ = checkGate.Touch()
	return nil
}
