package mode

import (
	"fmt"
	"net"
	"os"

	"github.com/Liplus-Project/dipper_ai/internal/config"
	"github.com/Liplus-Project/dipper_ai/internal/state"
)

// Package-level DNS lookup functions — overridable in tests.
var (
	dnsLookupA    = lookupARecord
	dnsLookupAAAA = lookupAAAARecord
)

// domainMismatch records which address families mismatched for a single entry.
type domainMismatch struct {
	ipv4 bool
	ipv6 bool
}

// Check resolves the DNS-registered IP for each configured DDNS domain and
// compares it with the current external IP.  If any domain has a stale or
// wrong registration, Check resets the per-domain cache for only those
// mismatched entries and forces an immediate DDNS update via Update().
//
// Only domains whose DNS record differs from the current IP have their cache
// reset; domains that are already correct retain their cache so Update() does
// not send unnecessary API requests for them.
//
// No internal gate: the systemd timer (every 5 min) is the effective rate
// limit.  Running check on every tick means external DNS changes (e.g. manual
// record edits) are detected and corrected within one timer interval.
//
// Equivalent to `dipper check`.
func Check(cfg *config.Config) error {
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

	// Per-entry mismatch tracking — only mismatched entries get cache-reset.
	mydnsMismatch := make([]domainMismatch, len(cfg.MyDNS))
	cfMismatch := make([]domainMismatch, len(cfg.Cloudflare))
	mismatch := false

	// --- Verify MyDNS domains ---
	for i, m := range cfg.MyDNS {
		if m.Domain == "" {
			continue
		}
		if wantV4 && m.IPv4 && fetched.IPv4 != "" {
			registered, err := dnsLookupA(m.Domain)
			if err != nil {
				fmt.Fprintf(os.Stderr, "dipper_ai check: DNS A %s: %v → scheduling update\n", m.Domain, err)
				mydnsMismatch[i].ipv4 = true
				mismatch = true
			} else if registered != fetched.IPv4 {
				fmt.Fprintf(os.Stderr, "dipper_ai check: %s A=%s want=%s → mismatch\n", m.Domain, registered, fetched.IPv4)
				mydnsMismatch[i].ipv4 = true
				mismatch = true
			} else {
				fmt.Fprintf(os.Stderr, "dipper_ai check: %s A=%s ok\n", m.Domain, registered)
			}
		}
		if wantV6 && m.IPv6 && fetched.IPv6 != "" {
			registered, err := dnsLookupAAAA(m.Domain)
			if err != nil {
				fmt.Fprintf(os.Stderr, "dipper_ai check: DNS AAAA %s: %v → scheduling update\n", m.Domain, err)
				mydnsMismatch[i].ipv6 = true
				mismatch = true
			} else if registered != fetched.IPv6 {
				fmt.Fprintf(os.Stderr, "dipper_ai check: %s AAAA=%s want=%s → mismatch\n", m.Domain, registered, fetched.IPv6)
				mydnsMismatch[i].ipv6 = true
				mismatch = true
			} else {
				fmt.Fprintf(os.Stderr, "dipper_ai check: %s AAAA=%s ok\n", m.Domain, registered)
			}
		}
	}

	// --- Verify Cloudflare domains ---
	for i, cf := range cfg.Cloudflare {
		if !cf.Enabled || cf.Domain == "" {
			continue
		}
		if wantV4 && cf.IPv4 && fetched.IPv4 != "" {
			registered, err := dnsLookupA(cf.Domain)
			if err != nil {
				fmt.Fprintf(os.Stderr, "dipper_ai check: DNS A %s: %v → scheduling update\n", cf.Domain, err)
				cfMismatch[i].ipv4 = true
				mismatch = true
			} else if registered != fetched.IPv4 {
				fmt.Fprintf(os.Stderr, "dipper_ai check: %s A=%s want=%s → mismatch\n", cf.Domain, registered, fetched.IPv4)
				cfMismatch[i].ipv4 = true
				mismatch = true
			} else {
				fmt.Fprintf(os.Stderr, "dipper_ai check: %s A=%s ok\n", cf.Domain, registered)
			}
		}
		if wantV6 && cf.IPv6 && fetched.IPv6 != "" {
			registered, err := dnsLookupAAAA(cf.Domain)
			if err != nil {
				fmt.Fprintf(os.Stderr, "dipper_ai check: DNS AAAA %s: %v → scheduling update\n", cf.Domain, err)
				cfMismatch[i].ipv6 = true
				mismatch = true
			} else if registered != fetched.IPv6 {
				fmt.Fprintf(os.Stderr, "dipper_ai check: %s AAAA=%s want=%s → mismatch\n", cf.Domain, registered, fetched.IPv6)
				cfMismatch[i].ipv6 = true
				mismatch = true
			} else {
				fmt.Fprintf(os.Stderr, "dipper_ai check: %s AAAA=%s ok\n", cf.Domain, registered)
			}
		}
	}

	if !mismatch {
		fmt.Fprintln(os.Stderr, "dipper_ai check: all domains match current IP")
		return nil
	}

	// Mismatch detected: reset per-domain caches for mismatched entries ONLY.
	// Domains whose DNS record is already correct keep their cache intact so
	// Update() does not send redundant API requests for them.
	// Also delete gate_ddns so DDNS_TIME rate-limit does not delay the fix.
	fmt.Fprintln(os.Stderr, "dipper_ai check: mismatch detected — forcing DDNS update for affected domains")
	st, err := state.New(cfg.StateDir)
	if err != nil {
		return err
	}
	for i, m := range cfg.MyDNS {
		entryKey := fmt.Sprintf("mydns_%d", i)
		if mydnsMismatch[i].ipv4 && m.IPv4 {
			_ = st.ResetDomainCache(entryKey, "ipv4")
		}
		if mydnsMismatch[i].ipv6 && m.IPv6 {
			_ = st.ResetDomainCache(entryKey, "ipv6")
		}
	}
	for i, cf := range cfg.Cloudflare {
		if !cf.Enabled {
			continue
		}
		entryKey := fmt.Sprintf("cf_%d", i)
		if cfMismatch[i].ipv4 && cf.IPv4 {
			_ = st.ResetDomainCache(entryKey, "A")
		}
		if cfMismatch[i].ipv6 && cf.IPv6 {
			_ = st.ResetDomainCache(entryKey, "AAAA")
		}
	}

	// Remove DDNS_TIME gate so Update() is not rate-limited on this forced run.
	_ = os.Remove(cfg.StateDir + "/gate_ddns")

	return Update(cfg)
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
