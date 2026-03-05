// Package mode implements the three dipper_ai execution modes.
package mode

import (
	"fmt"
	"os"
	"strings"

	"github.com/Liplus-Project/dipper_ai/internal/config"
	"github.com/Liplus-Project/dipper_ai/internal/ddns"
	"github.com/Liplus-Project/dipper_ai/internal/ip"
	"github.com/Liplus-Project/dipper_ai/internal/state"
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
//
// Logic:
//   - Current external IP is fetched on every invocation.
//   - Per-domain IP cache: each provider entry independently tracks the last
//     IP it was sent. Only entries whose cached IP differs from the current
//     IP are updated ("changed domains only").
//   - Timing is controlled by the daemon's internal ticker (DDNS_TIME interval).
//     Update() itself has no rate-limiting gate — the caller is responsible.
//   - Keepalive is handled separately by Keepalive() on its own ticker.
//   - Cloudflare: no keepalive — API records persist until explicitly changed.
func Update(cfg *config.Config) error {
	st, err := state.New(cfg.StateDir)
	if err != nil {
		return err
	}

	// --- Fetch current external IP ---
	wantV4 := cfg.IPv4 && cfg.IPv4DDNS
	wantV6 := cfg.IPv6 && cfg.IPv6DDNS
	fetched, _ := ipFetch(wantV4, wantV6)

	if wantV4 && fetched.ErrIPv4 != nil {
		_ = st.AppendError(fmt.Sprintf("ip_fetch_error ipv4: %v", fetched.ErrIPv4))
		fmt.Fprintf(os.Stderr, "dipper_ai update: IPv4 fetch failed: %v\n", fetched.ErrIPv4)
	}
	if wantV6 && fetched.ErrIPv6 != nil {
		_ = st.AppendError(fmt.Sprintf("ip_fetch_error ipv6: %v", fetched.ErrIPv6))
		fmt.Fprintf(os.Stderr, "dipper_ai update: IPv6 fetch failed: %v\n", fetched.ErrIPv6)
	}
	if fetched.IPv4 == "" && fetched.IPv6 == "" && (wantV4 || wantV6) {
		if fetched.ErrIPv4 != nil {
			return fetched.ErrIPv4
		}
		return fetched.ErrIPv6
	}

	var updateErr error
	var successLines []string
	anyUpdate := false
	anyIPChange := false // at least one domain updated due to IP change

	// --- MyDNS per-entry updates ---
	// Each entry is updated independently based on its own per-domain cache.
	for i, entry := range cfg.MyDNS {
		entryKey := fmt.Sprintf("mydns_%d", i)
		dnsEntry := ddns.MyDNSEntry{
			ID:     entry.ID,
			Pass:   entry.Pass,
			Domain: entry.Domain,
		}

		if wantV4 && entry.IPv4 && fetched.IPv4 != "" {
			cached, _ := st.ReadDomainCache(entryKey, "ipv4")
			if fetched.IPv4 != cached {
				r := mydnsUpdateIPv4(dnsEntry, cfg.MyDNSIPv4URL)
				if r.Err != nil {
					_ = st.WriteDDNSResult(entryKey+"_ipv4", "fail:"+r.Err.Error())
					_ = st.AppendError(fmt.Sprintf("ddns_error mydns[%d] ipv4: %v", i, r.Err))
					fmt.Fprintf(os.Stderr, "dipper_ai update: mydns[%d] %s ipv4: FAIL: %v\n", i, entry.Domain, r.Err)
					updateErr = r.Err
				} else {
					_ = st.WriteDomainCache(entryKey, "ipv4", fetched.IPv4)
					_ = st.WriteDDNSResult(entryKey+"_ipv4", "ok")
					successLines = append(successLines, fmt.Sprintf(" mydns[%d] %s ipv4: ok", i, entry.Domain))
					anyUpdate = true
					anyIPChange = true
				}
			}
		}

		if wantV6 && entry.IPv6 && fetched.IPv6 != "" {
			cached, _ := st.ReadDomainCache(entryKey, "ipv6")
			if fetched.IPv6 != cached {
				r := mydnsUpdateIPv6(dnsEntry, cfg.MyDNSIPv6URL)
				if r.Err != nil {
					_ = st.WriteDDNSResult(entryKey+"_ipv6", "fail:"+r.Err.Error())
					_ = st.AppendError(fmt.Sprintf("ddns_error mydns[%d] ipv6: %v", i, r.Err))
					fmt.Fprintf(os.Stderr, "dipper_ai update: mydns[%d] %s ipv6: FAIL: %v\n", i, entry.Domain, r.Err)
					updateErr = r.Err
				} else {
					_ = st.WriteDomainCache(entryKey, "ipv6", fetched.IPv6)
					_ = st.WriteDDNSResult(entryKey+"_ipv6", "ok")
					successLines = append(successLines, fmt.Sprintf(" mydns[%d] %s ipv6: ok", i, entry.Domain))
					anyUpdate = true
					anyIPChange = true
				}
			}
		}
	}

	// --- Cloudflare per-entry updates (IP change only — no keepalive) ---
	for i, cf := range cfg.Cloudflare {
		if !cf.Enabled {
			continue
		}
		entryKey := fmt.Sprintf("cf_%d", i)
		cfEntry := ddns.CloudflareEntry{
			API:    cf.API,
			Zone:   cf.Zone,
			ZoneID: cf.ZoneID,
			Domain: cf.Domain,
		}

		if wantV4 && cf.IPv4 && fetched.IPv4 != "" {
			cached, _ := st.ReadDomainCache(entryKey, "A")
			if fetched.IPv4 != cached {
				r := cloudflareUpdate(cfEntry, fetched.IPv4, "A", cfg.CloudflareURL)
				if r.Err != nil {
					_ = st.WriteDDNSResult(entryKey+"_A", "fail:"+r.Err.Error())
					_ = st.AppendError(fmt.Sprintf("ddns_error cloudflare[%d] A: %v", i, r.Err))
					fmt.Fprintf(os.Stderr, "dipper_ai update: cf[%d] %s A: FAIL: %v\n", i, cf.Domain, r.Err)
					updateErr = r.Err
				} else {
					_ = st.WriteDomainCache(entryKey, "A", fetched.IPv4)
					_ = st.WriteDDNSResult(entryKey+"_A", "ok")
					successLines = append(successLines, fmt.Sprintf(" cf[%d] %s A: ok", i, cf.Domain))
					anyUpdate = true
					anyIPChange = true
				}
			}
		}

		if wantV6 && cf.IPv6 && fetched.IPv6 != "" {
			cached, _ := st.ReadDomainCache(entryKey, "AAAA")
			if fetched.IPv6 != cached {
				r := cloudflareUpdate(cfEntry, fetched.IPv6, "AAAA", cfg.CloudflareURL)
				if r.Err != nil {
					_ = st.WriteDDNSResult(entryKey+"_AAAA", "fail:"+r.Err.Error())
					_ = st.AppendError(fmt.Sprintf("ddns_error cloudflare[%d] AAAA: %v", i, r.Err))
					fmt.Fprintf(os.Stderr, "dipper_ai update: cf[%d] %s AAAA: FAIL: %v\n", i, cf.Domain, r.Err)
					updateErr = r.Err
				} else {
					_ = st.WriteDomainCache(entryKey, "AAAA", fetched.IPv6)
					_ = st.WriteDDNSResult(entryKey+"_AAAA", "ok")
					successLines = append(successLines, fmt.Sprintf(" cf[%d] %s AAAA: ok", i, cf.Domain))
					anyUpdate = true
					anyIPChange = true
				}
			}
		}
	}

	// Log only when at least one update was attempted.
	// Silent output when nothing changed keeps the systemd journal clean.
	if anyUpdate {
		if fetched.IPv4 != "" {
			fmt.Fprintf(os.Stderr, "dipper_ai update: IPv4=%s\n", fetched.IPv4)
		}
		if fetched.IPv6 != "" {
			fmt.Fprintf(os.Stderr, "dipper_ai update: IPv6=%s\n", fetched.IPv6)
		}
		for _, line := range successLines {
			fmt.Fprintf(os.Stderr, "dipper_ai update:%s\n", line)
		}
	}

	// --- Email notification ---
	if cfg.EmailAddr != "" && anyIPChange && cfg.EmailChkDDNS {
		if mailErr := sendUpdateNotification(cfg, fetched, successLines); mailErr != nil {
			_ = st.AppendError(fmt.Sprintf("update_mail_failed: %v", mailErr))
			fmt.Fprintf(os.Stderr, "dipper_ai update: mail notification failed: %v\n", mailErr)
		}
	}

	return updateErr
}

// sendUpdateNotification composes and sends an IP-update notification email.
func sendUpdateNotification(cfg *config.Config, fetched *ip.Result, successLines []string) error {
	var ipLines []string
	if fetched.IPv4 != "" {
		ipLines = append(ipLines, "IPv4: "+fetched.IPv4)
	}
	if fetched.IPv6 != "" {
		ipLines = append(ipLines, "IPv6: "+fetched.IPv6)
	}

	subject := "dipper_ai: IP updated"
	body := fmt.Sprintf("%s\n\nReason: IP changed\n\nUpdated providers:\n%s\n",
		strings.Join(ipLines, "\n"),
		strings.Join(successLines, "\n"),
	)

	return sendMailFn(cfg.EmailAddr, subject, body)
}
