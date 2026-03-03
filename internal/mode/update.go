// Package mode implements the three dipper_ai execution modes.
package mode

import (
	"fmt"
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

	// --- Time gate: UPDATE_TIME ---
	updateGate := timegate.New(cfg.StateDir, "update", time.Duration(cfg.UpdateTime)*time.Minute)
	if !updateGate.ShouldRun() {
		return nil
	}

	// --- Fetch IPs ---
	wantV4 := cfg.IPv4 && cfg.IPv4DDNS
	wantV6 := cfg.IPv6 && cfg.IPv6DDNS
	fetched, err := ipFetch(wantV4, wantV6)
	if err != nil {
		_ = st.AppendError(fmt.Sprintf("ip_fetch_error: %v", err))
		return err
	}

	// --- IP change detection ---
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

	if !ipChanged {
		_ = updateGate.Touch()
		return nil
	}

	// --- DDNS time gate: DDNS_TIME ---
	ddnsGate := timegate.New(cfg.StateDir, "ddns", time.Duration(cfg.DDNSTime)*time.Minute)
	if !ddnsGate.ShouldRun() {
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
				updateErr = r.Err
			} else {
				_ = st.WriteDDNSResult(key, "ok")
			}
		}
		if wantV6 && entry.IPv6 && fetched.IPv6 != "" {
			r := mydnsUpdateIPv6(dnsEntry, cfg.MyDNSIPv6URL)
			key := fmt.Sprintf("mydns_%d_ipv6", i)
			if r.Err != nil {
				_ = st.WriteDDNSResult(key, "fail:"+r.Err.Error())
				_ = st.AppendError(fmt.Sprintf("ddns_error mydns[%d] ipv6: %v", i, r.Err))
				updateErr = r.Err
			} else {
				_ = st.WriteDDNSResult(key, "ok")
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
				updateErr = r.Err
			} else {
				_ = st.WriteDDNSResult(key, "ok")
			}
		}
		if wantV6 && cf.IPv6 && fetched.IPv6 != "" {
			r := cloudflareUpdate(cfEntry, fetched.IPv6, "AAAA", cfg.CloudflareURL)
			key := fmt.Sprintf("cf_%d_AAAA", i)
			if r.Err != nil {
				_ = st.WriteDDNSResult(key, "fail:"+r.Err.Error())
				_ = st.AppendError(fmt.Sprintf("ddns_error cloudflare[%d] AAAA: %v", i, r.Err))
				updateErr = r.Err
			} else {
				_ = st.WriteDDNSResult(key, "ok")
			}
		}
	}

	_ = updateGate.Touch()
	_ = ddnsGate.Touch()
	return updateErr
}
