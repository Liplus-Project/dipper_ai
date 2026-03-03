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
		return nil // gate not elapsed, exit silently
	}

	// --- Fetch IPs ---
	fetched, err := ip.Fetch(cfg.IPv4 && cfg.IPv4DDNS, cfg.IPv6 && cfg.IPv6DDNS)
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
		return nil // no change, skip DDNS
	}

	// --- DDNS time gate: DDNS_TIME ---
	ddnsGate := timegate.New(cfg.StateDir, "ddns", time.Duration(cfg.DDNSTime)*time.Minute)
	if !ddnsGate.ShouldRun() {
		return nil
	}

	// --- Update DDNS providers ---
	var updateErr error

	if cfg.MyDNS.Enabled {
		results := ddns.UpdateMyDNS(ddns.MyDNSConfig{
			MasterID: cfg.MyDNS.MasterID,
			Password: cfg.MyDNS.Password,
		}, fetched.IPv4, fetched.IPv6)
		for _, r := range results {
			key := "mydns"
			if r.Err != nil {
				_ = st.WriteDDNSResult(key, "fail:"+r.Err.Error())
				_ = st.AppendError(fmt.Sprintf("ddns_error mydns: %v", r.Err))
				updateErr = r.Err
			} else {
				_ = st.WriteDDNSResult(key, "ok")
			}
		}
	}

	for i, cf := range cfg.Cloudflare {
		if !cf.Enabled {
			continue
		}
		entries := []ddns.CloudflareEntry{{Token: cf.Token, ZoneID: cf.ZoneID, Name: cf.Name}}
		if fetched.IPv4 != "" {
			for _, r := range ddns.UpdateCloudflare(entries, fetched.IPv4, "A") {
				key := fmt.Sprintf("cf_%d_A", i)
				if r.Err != nil {
					_ = st.WriteDDNSResult(key, "fail:"+r.Err.Error())
					_ = st.AppendError(fmt.Sprintf("ddns_error cloudflare[%d] A: %v", i, r.Err))
					updateErr = r.Err
				} else {
					_ = st.WriteDDNSResult(key, "ok")
				}
			}
		}
		if fetched.IPv6 != "" {
			for _, r := range ddns.UpdateCloudflare(entries, fetched.IPv6, "AAAA") {
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
	}

	_ = updateGate.Touch()
	_ = ddnsGate.Touch()
	return updateErr
}
