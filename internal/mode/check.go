package mode

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/Liplus-Project/dipper_ai/internal/config"
	"github.com/Liplus-Project/dipper_ai/internal/state"
	"github.com/Liplus-Project/dipper_ai/internal/timegate"
)

// Package-level DNS lookup functions — overridable in tests.
var (
	dnsLookupA    = lookupARecord
	dnsLookupAAAA = lookupAAAARecord
)

// Check resolves the DNS-registered IP for each configured DDNS domain and
// compares it with the current external IP.  If any domain has a stale or
// wrong registration, Check resets the IP cache and forces an immediate DDNS
// update via Update().
//
// Equivalent to `dipper check`.
func Check(cfg *config.Config) error {
	// Check runs on its own schedule — use UpdateTime so it does not fire on
	// every timer tick (which would cause spurious DNS lookups and race
	// conditions immediately after a fresh update).
	checkGate := timegate.New(cfg.StateDir, "check", time.Duration(cfg.UpdateTime)*time.Minute)
	if !checkGate.ShouldRun() {
		return nil
	}

	wantV4 := cfg.IPv4 && cfg.IPv4DDNS
	wantV6 := cfg.IPv6 && cfg.IPv6DDNS

	// Fetch current external IP.
	fetched, _ := ipFetch(wantV4, wantV6)
	if wantV4 && fetched.ErrIPv4 != nil {
		fmt.Fprintf(os.Stderr, "dipper_ai check: IPv4 fetch error: %v\n", fetched.ErrIPv4)
	}
	if wantV6 && fetched.ErrIPv6 != nil {
		fmt.Fprintf(os.Stderr, "dipper_ai check: IPv6 fetch error: %v\n", fetched.ErrIPv6)
	}
	if fetched.IPv4 == "" && fetched.IPv6 == "" && (wantV4 || wantV6) {
		return fmt.Errorf("check: could not fetch current external IP")
	}

	mismatch := false

	// --- Verify MyDNS domains ---
	for _, m := range cfg.MyDNS {
		if m.Domain == "" {
			continue
		}
		if wantV4 && m.IPv4 && fetched.IPv4 != "" {
			registered, err := dnsLookupA(m.Domain)
			if err != nil {
				fmt.Fprintf(os.Stderr, "dipper_ai check: DNS A %s: %v → scheduling update\n", m.Domain, err)
				mismatch = true
			} else if registered != fetched.IPv4 {
				fmt.Fprintf(os.Stderr, "dipper_ai check: %s A=%s want=%s → mismatch\n", m.Domain, registered, fetched.IPv4)
				mismatch = true
			} else {
				fmt.Fprintf(os.Stderr, "dipper_ai check: %s A=%s ok\n", m.Domain, registered)
			}
		}
		if wantV6 && m.IPv6 && fetched.IPv6 != "" {
			registered, err := dnsLookupAAAA(m.Domain)
			if err != nil {
				fmt.Fprintf(os.Stderr, "dipper_ai check: DNS AAAA %s: %v → scheduling update\n", m.Domain, err)
				mismatch = true
			} else if registered != fetched.IPv6 {
				fmt.Fprintf(os.Stderr, "dipper_ai check: %s AAAA=%s want=%s → mismatch\n", m.Domain, registered, fetched.IPv6)
				mismatch = true
			} else {
				fmt.Fprintf(os.Stderr, "dipper_ai check: %s AAAA=%s ok\n", m.Domain, registered)
			}
		}
	}

	// --- Verify Cloudflare domains ---
	for _, cf := range cfg.Cloudflare {
		if !cf.Enabled || cf.Domain == "" {
			continue
		}
		if wantV4 && cf.IPv4 && fetched.IPv4 != "" {
			registered, err := dnsLookupA(cf.Domain)
			if err != nil {
				fmt.Fprintf(os.Stderr, "dipper_ai check: DNS A %s: %v → scheduling update\n", cf.Domain, err)
				mismatch = true
			} else if registered != fetched.IPv4 {
				fmt.Fprintf(os.Stderr, "dipper_ai check: %s A=%s want=%s → mismatch\n", cf.Domain, registered, fetched.IPv4)
				mismatch = true
			} else {
				fmt.Fprintf(os.Stderr, "dipper_ai check: %s A=%s ok\n", cf.Domain, registered)
			}
		}
		if wantV6 && cf.IPv6 && fetched.IPv6 != "" {
			registered, err := dnsLookupAAAA(cf.Domain)
			if err != nil {
				fmt.Fprintf(os.Stderr, "dipper_ai check: DNS AAAA %s: %v → scheduling update\n", cf.Domain, err)
				mismatch = true
			} else if registered != fetched.IPv6 {
				fmt.Fprintf(os.Stderr, "dipper_ai check: %s AAAA=%s want=%s → mismatch\n", cf.Domain, registered, fetched.IPv6)
				mismatch = true
			} else {
				fmt.Fprintf(os.Stderr, "dipper_ai check: %s AAAA=%s ok\n", cf.Domain, registered)
			}
		}
	}

	if !mismatch {
		fmt.Fprintln(os.Stderr, "dipper_ai check: all domains match current IP")
		_ = checkGate.Touch()
		return nil
	}

	// Mismatch detected: reset per-domain caches for mismatched entries so
	// the next Update() call sees a cache miss and re-sends to those providers.
	// Also delete gate_ddns so DDNS_TIME rate-limit does not delay the fix.
	fmt.Fprintln(os.Stderr, "dipper_ai check: mismatch detected — forcing DDNS update")
	st, err := state.New(cfg.StateDir)
	if err != nil {
		return err
	}
	for i, m := range cfg.MyDNS {
		entryKey := fmt.Sprintf("mydns_%d", i)
		if wantV4 && m.IPv4 && fetched.IPv4 != "" {
			_ = st.ResetDomainCache(entryKey, "ipv4")
		}
		if wantV6 && m.IPv6 && fetched.IPv6 != "" {
			_ = st.ResetDomainCache(entryKey, "ipv6")
		}
	}
	for i, cf := range cfg.Cloudflare {
		if !cf.Enabled {
			continue
		}
		entryKey := fmt.Sprintf("cf_%d", i)
		if wantV4 && cf.IPv4 && fetched.IPv4 != "" {
			_ = st.ResetDomainCache(entryKey, "A")
		}
		if wantV6 && cf.IPv6 && fetched.IPv6 != "" {
			_ = st.ResetDomainCache(entryKey, "AAAA")
		}
	}

	// Remove DDNS_TIME gate so Update() is not rate-limited on this forced run.
	_ = os.Remove(cfg.StateDir + "/gate_ddns")

	updateErr := Update(cfg)
	_ = checkGate.Touch()
	return updateErr
}

// lookupARecord resolves the IPv4 A record for domain.
func lookupARecord(domain string) (string, error) {
	ips, err := net.LookupHost(domain)
	if err != nil {
		return "", err
	}
	for _, s := range ips {
		if ip := net.ParseIP(s); ip != nil {
			if v4 := ip.To4(); v4 != nil {
				return v4.String(), nil
			}
		}
	}
	return "", fmt.Errorf("no A record for %s", domain)
}

// lookupAAAARecord resolves the IPv6 AAAA record for domain.
func lookupAAAARecord(domain string) (string, error) {
	ips, err := net.LookupHost(domain)
	if err != nil {
		return "", err
	}
	for _, s := range ips {
		if ip := net.ParseIP(s); ip != nil && ip.To4() == nil {
			return ip.String(), nil
		}
	}
	return "", fmt.Errorf("no AAAA record for %s", domain)
}
