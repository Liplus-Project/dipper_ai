package mode

import (
	"fmt"
	"os"
	"strings"

	"github.com/Liplus-Project/dipper_ai/internal/config"
	"github.com/Liplus-Project/dipper_ai/internal/ddns"
	"github.com/Liplus-Project/dipper_ai/internal/state"
)

// Keepalive force-updates all MyDNS entries regardless of IP change.
// Equivalent to `dipper_ai keepalive`.
//
// Logic:
//   - Triggered by its own systemd timer (dipper_ai-keepalive.timer) at
//     UPDATE_TIME interval, fully independent of the check/update timer.
//   - Fetches current external IP (needed to populate the DDNS request).
//   - All MyDNS entries are updated unconditionally; domain cache is refreshed.
//   - Cloudflare is skipped — its records persist without periodic refresh.
//   - Sends email notification when EMAIL_UP_DDNS=on.
func Keepalive(cfg *config.Config) error {
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
		fmt.Fprintf(os.Stderr, "dipper_ai keepalive: IPv4 fetch failed: %v\n", fetched.ErrIPv4)
	}
	if wantV6 && fetched.ErrIPv6 != nil {
		_ = st.AppendError(fmt.Sprintf("ip_fetch_error ipv6: %v", fetched.ErrIPv6))
		fmt.Fprintf(os.Stderr, "dipper_ai keepalive: IPv6 fetch failed: %v\n", fetched.ErrIPv6)
	}
	if fetched.IPv4 == "" && fetched.IPv6 == "" && (wantV4 || wantV6) {
		if fetched.ErrIPv4 != nil {
			return fetched.ErrIPv4
		}
		return fetched.ErrIPv6
	}

	var keepaliveErr error
	var successLines []string

	// --- MyDNS per-entry force update ---
	for i, entry := range cfg.MyDNS {
		entryKey := fmt.Sprintf("mydns_%d", i)
		dnsEntry := ddns.MyDNSEntry{
			ID:     entry.ID,
			Pass:   entry.Pass,
			Domain: entry.Domain,
		}

		if wantV4 && entry.IPv4 && fetched.IPv4 != "" {
			r := mydnsUpdateIPv4(dnsEntry, cfg.MyDNSIPv4URL)
			if r.Err != nil {
				_ = st.WriteDDNSResult(entryKey+"_ipv4", "fail:"+r.Err.Error())
				_ = st.AppendError(fmt.Sprintf("ddns_error mydns[%d] ipv4: %v", i, r.Err))
				fmt.Fprintf(os.Stderr, "dipper_ai keepalive: mydns[%d] %s ipv4: FAIL: %v\n", i, entry.Domain, r.Err)
				keepaliveErr = r.Err
			} else {
				_ = st.WriteDomainCache(entryKey, "ipv4", fetched.IPv4)
				_ = st.WriteDDNSResult(entryKey+"_ipv4", "ok")
				successLines = append(successLines, fmt.Sprintf(" mydns[%d] %s ipv4: ok", i, entry.Domain))
			}
		}

		if wantV6 && entry.IPv6 && fetched.IPv6 != "" {
			r := mydnsUpdateIPv6(dnsEntry, cfg.MyDNSIPv6URL)
			if r.Err != nil {
				_ = st.WriteDDNSResult(entryKey+"_ipv6", "fail:"+r.Err.Error())
				_ = st.AppendError(fmt.Sprintf("ddns_error mydns[%d] ipv6: %v", i, r.Err))
				fmt.Fprintf(os.Stderr, "dipper_ai keepalive: mydns[%d] %s ipv6: FAIL: %v\n", i, entry.Domain, r.Err)
				keepaliveErr = r.Err
			} else {
				_ = st.WriteDomainCache(entryKey, "ipv6", fetched.IPv6)
				_ = st.WriteDDNSResult(entryKey+"_ipv6", "ok")
				successLines = append(successLines, fmt.Sprintf(" mydns[%d] %s ipv6: ok", i, entry.Domain))
			}
		}
	}

	// Cloudflare: no keepalive needed — records persist without periodic refresh.

	if len(successLines) > 0 {
		if fetched.IPv4 != "" {
			fmt.Fprintf(os.Stderr, "dipper_ai keepalive: IPv4=%s\n", fetched.IPv4)
		}
		if fetched.IPv6 != "" {
			fmt.Fprintf(os.Stderr, "dipper_ai keepalive: IPv6=%s\n", fetched.IPv6)
		}
		for _, line := range successLines {
			fmt.Fprintf(os.Stderr, "dipper_ai keepalive:%s\n", line)
		}
	}

	// --- Email notification ---
	if cfg.EmailAddr != "" && len(successLines) > 0 && cfg.EmailUpDDNS {
		subject := "dipper_ai: DDNS keepalive"
		var ipLines []string
		if fetched.IPv4 != "" {
			ipLines = append(ipLines, "IPv4: "+fetched.IPv4)
		}
		if fetched.IPv6 != "" {
			ipLines = append(ipLines, "IPv6: "+fetched.IPv6)
		}
		body := fmt.Sprintf("%s\n\nReason: keepalive\n\nUpdated providers:\n%s\n",
			strings.Join(ipLines, "\n"),
			strings.Join(successLines, "\n"),
		)
		if mailErr := sendMailFn(cfg.EmailAddr, subject, body); mailErr != nil {
			_ = st.AppendError(fmt.Sprintf("keepalive_mail_failed: %v", mailErr))
			fmt.Fprintf(os.Stderr, "dipper_ai keepalive: mail notification failed: %v\n", mailErr)
		}
	}

	return keepaliveErr
}
