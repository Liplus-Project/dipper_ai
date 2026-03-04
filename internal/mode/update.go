// Package mode implements the three dipper_ai execution modes.
package mode

import (
	"fmt"
	"os"
	"time"

	"github.com/Liplus-Project/dipper_ai/internal/config"
	"github.com/Liplus-Project/dipper_ai/internal/ddns"
	"github.com/Liplus-Project/dipper_ai/internal/ip"
	"github.com/Liplus-Project/dipper_ai/internal/state"
	"github.com/Liplus-Project/dipper_ai/internal/timegate"
)

// Package-level function variables — overridable in tests.
var (
	ipFetch          = ip.Fetch
	mydnsUpdateIPv4  = ddns.UpdateMyDNSIPv4
	mydnsUpdateIPv6  = ddns.UpdateMyDNSIPv6
	cloudflareUpdate = ddns.UpdateCloudflare
)

// Update is the main DDNS update mode.
// Equivalent to `dipper update`.
func Update(cfg *config.Config) error {
	st, err := state.New(cfg.StateDir)
	if err != nil {
		return err
	}

	// --- Fetch IPs (always — IP must be checked on every run) ---
	wantV4 := cfg.IPv4 && cfg.IPv4DDNS
	wantV6 := cfg.IPv6 && cfg.IPv6DDNS
	fetched, _ := ipFetch(wantV4, wantV6)

	// Log per-family fetch errors; IPv6 failure alone is non-fatal.
	if wantV4 && fetched.ErrIPv4 != nil {
		_ = st.AppendError(fmt.Sprintf("ip_fetch_error ipv4: %v", fetched.ErrIPv4))
		fmt.Fprintf(os.Stderr, "dipper_ai update: IPv4 fetch failed: %v\n", fetched.ErrIPv4)
	}
	if wantV6 && fetched.ErrIPv6 != nil {
		_ = st.AppendError(fmt.Sprintf("ip_fetch_error ipv6: %v", fetched.ErrIPv6))
		fmt.Fprintf(os.Stderr, "dipper_ai update: IPv6 fetch failed: %v\n", fetched.ErrIPv6)
	}
	// Abort only when nothing usable was fetched at all.
	if fetched.IPv4 == "" && fetched.IPv6 == "" && (wantV4 || wantV6) {
		if fetched.ErrIPv4 != nil {
			return fetched.ErrIPv4
		}
		return fetched.ErrIPv6
	}

	if fetched.IPv4 != "" {
		fmt.Fprintf(os.Stderr, "dipper_ai update: IPv4=%s\n", fetched.IPv4)
	}
	if fetched.IPv6 != "" {
		fmt.Fprintf(os.Stderr, "dipper_ai update: IPv6=%s\n", fetched.IPv6)
	}

	// --- IP change detection ---
	// Cache is initialised to 0.0.0.0 / :: so first run always triggers DDNS.
	ipChanged := false

	if fetched.IPv4 != "" {
		cached, _ := st.ReadIP("ipv4")
		if fetched.IPv4 != cached {
			ipChanged = true
			if err := st.WriteIP("ipv4", fetched.IPv4); err != nil {
				return err
			}
		}
	}
	if fetched.IPv6 != "" {
		cached, _ := st.ReadIP("ipv6")
		if fetched.IPv6 != cached {
			ipChanged = true
			if err := st.WriteIP("ipv6", fetched.IPv6); err != nil {
				return err
			}
		}
	}

	// UPDATE_TIME: force re-sync even when IP is unchanged (catches external
	// DDNS edits).  ipChanged bypasses this gate so IP changes are acted on
	// immediately regardless of how recently the last sync ran.
	updateGate := timegate.New(cfg.StateDir, "update", time.Duration(cfg.UpdateTime)*time.Minute)
	forceSync := updateGate.ShouldRun()

	if !ipChanged && !forceSync {
		fmt.Fprintln(os.Stderr, "dipper_ai update: IP unchanged, skipping DDNS")
		return nil
	}
	if !ipChanged && forceSync {
		fmt.Fprintln(os.Stderr, "dipper_ai update: forcing periodic re-sync")
	}

	// --- DDNS time gate: DDNS_TIME ---
	// Bypassed when IP has changed (act immediately) or when force-sync is set.
	// Only applies when IP is unchanged and no force-sync requested.
	ddnsGate := timegate.New(cfg.StateDir, "ddns", time.Duration(cfg.DDNSTime)*time.Minute)
	if !ipChanged && !forceSync && !ddnsGate.ShouldRun() {
		fmt.Fprintln(os.Stderr, "dipper_ai update: DDNS gate active, skipping")
		return nil
	}

	var updateErr error

	// --- MyDNS per-entry updates ---
	for i, entry := range cfg.MyDNS {
		dnsEntry := ddns.MyDNSEntry{
			ID:     entry.ID,
			Pass:   entry.Pass,
			Domain: entry.Domain,
		}
		if wantV4 && entry.IPv4 && fetched.IPv4 != "" {
			r := mydnsUpdateIPv4(dnsEntry, cfg.MyDNSIPv4URL)
			key := fmt.Sprintf("mydns_%d_ipv4", i)
			if r.Err != nil {
				_ = st.WriteDDNSResult(key, "fail:"+r.Err.Error())
				_ = st.AppendError(fmt.Sprintf("ddns_error mydns[%d] ipv4: %v", i, r.Err))
				fmt.Fprintf(os.Stderr, "dipper_ai update: mydns[%d] %s ipv4: FAIL: %v\n", i, entry.Domain, r.Err)
				updateErr = r.Err
			} else {
				_ = st.WriteDDNSResult(key, "ok")
				fmt.Fprintf(os.Stderr, "dipper_ai update: mydns[%d] %s ipv4: ok\n", i, entry.Domain)
			}
		}
		if wantV6 && entry.IPv6 && fetched.IPv6 != "" {
			r := mydnsUpdateIPv6(dnsEntry, cfg.MyDNSIPv6URL)
			key := fmt.Sprintf("mydns_%d_ipv6", i)
			if r.Err != nil {
				_ = st.WriteDDNSResult(key, "fail:"+r.Err.Error())
				_ = st.AppendError(fmt.Sprintf("ddns_error mydns[%d] ipv6: %v", i, r.Err))
				fmt.Fprintf(os.Stderr, "dipper_ai update: mydns[%d] %s ipv6: FAIL: %v\n", i, entry.Domain, r.Err)
				updateErr = r.Err
			} else {
				_ = st.WriteDDNSResult(key, "ok")
				fmt.Fprintf(os.Stderr, "dipper_ai update: mydns[%d] %s ipv6: ok\n", i, entry.Domain)
			}
		}
	}

	// --- Cloudflare per-entry updates ---
	for i, cf := range cfg.Cloudflare {
		if !cf.Enabled {
			continue
		}
		cfEntry := ddns.CloudflareEntry{
			API:    cf.API,
			Zone:   cf.Zone,
			Domain: cf.Domain,
		}
		if wantV4 && cf.IPv4 && fetched.IPv4 != "" {
			r := cloudflareUpdate(cfEntry, fetched.IPv4, "A", cfg.CloudflareURL)
			key := fmt.Sprintf("cf_%d_A", i)
			if r.Err != nil {
				_ = st.WriteDDNSResult(key, "fail:"+r.Err.Error())
				_ = st.AppendError(fmt.Sprintf("ddns_error cloudflare[%d] A: %v", i, r.Err))
				fmt.Fprintf(os.Stderr, "dipper_ai update: cf[%d] %s A: FAIL: %v\n", i, cf.Domain, r.Err)
				updateErr = r.Err
			} else {
				_ = st.WriteDDNSResult(key, "ok")
				fmt.Fprintf(os.Stderr, "dipper_ai update: cf[%d] %s A: ok\n", i, cf.Domain)
			}
		}
		if wantV6 && cf.IPv6 && fetched.IPv6 != "" {
			r := cloudflareUpdate(cfEntry, fetched.IPv6, "AAAA", cfg.CloudflareURL)
			key := fmt.Sprintf("cf_%d_AAAA", i)
			if r.Err != nil {
				_ = st.WriteDDNSResult(key, "fail:"+r.Err.Error())
				_ = st.AppendError(fmt.Sprintf("ddns_error cloudflare[%d] AAAA: %v", i, r.Err))
				fmt.Fprintf(os.Stderr, "dipper_ai update: cf[%d] %s AAAA: FAIL: %v\n", i, cf.Domain, r.Err)
				updateErr = r.Err
			} else {
				_ = st.WriteDDNSResult(key, "ok")
				fmt.Fprintf(os.Stderr, "dipper_ai update: cf[%d] %s AAAA: ok\n", i, cf.Domain)
			}
		}
	}

	if forceSync {
		_ = updateGate.Touch()
	}
	_ = ddnsGate.Touch()
	return updateErr
}
