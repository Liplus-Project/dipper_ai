// Package config loads and validates dipper_ai configuration.
//
// Format: shell-style key=value, # comments, blank lines ignored.
// No external dependencies.
//
// Example: see user.conf.example in the repository root.
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds all runtime configuration for dipper_ai.
type Config struct {
	// State directory (cache-equivalent storage)
	StateDir string

	// IP settings
	IPv4     bool
	IPv6     bool
	IPv4DDNS bool
	IPv6DDNS bool

	// Time gates (minutes)
	// UpdateTime:  minimum 3
	// DDNSTime:    minimum 1
	// IPCacheTime: 0 = disabled, else minimum 15
	// ErrChkTime:  0 = disabled, else minimum 1
	UpdateTime  int
	DDNSTime    int
	IPCacheTime int
	ErrChkTime  int

	// MyDNS entries (MYDNS_0_*, MYDNS_1_*, ...)
	MyDNS        []MyDNSEntry
	MyDNSIPv4URL string
	MyDNSIPv6URL string

	// Cloudflare entries (CF_0_*, CF_1_*, ...)
	Cloudflare    []CloudflareEntry
	CloudflareURL string

	// Email notification
	EmailChkDDNS bool   // EMAIL_CHK_DDNS: notify on IP change
	EmailUpDDNS  bool   // EMAIL_UP_DDNS:  notify on periodic update
	EmailAddr    string // EMAIL_ADR:       recipient address
}

// MyDNSEntry holds credentials and routing for one MyDNS domain entry.
type MyDNSEntry struct {
	ID     string
	Pass   string
	Domain string
	IPv4   bool
	IPv6   bool
}

// CloudflareEntry holds credentials and target for one Cloudflare DNS record.
type CloudflareEntry struct {
	Enabled bool
	API     string // API token (DNS:Edit permission)
	Zone    string // zone name  (e.g. "example.com")
	Domain  string // FQDN to update (e.g. "home.example.com")
	IPv4    bool
	IPv6    bool
}

// Load reads the config file from the standard location.
func Load() (*Config, error) {
	path := configFilePath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found: %s", path)
	}
	return ParseFile(path)
}

// configFilePath resolves the config file path.
// Priority: DIPPER_AI_CONFIG env → <WorkingDirectory>/user.conf
func configFilePath() string {
	if v := os.Getenv("DIPPER_AI_CONFIG"); v != "" {
		return v
	}
	wd, err := os.Getwd()
	if err != nil {
		wd = "/etc/dipper_ai"
	}
	return filepath.Join(wd, "user.conf")
}

// ParseFile parses a user.conf file and returns a validated Config.
func ParseFile(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("config: open %s: %w", path, err)
	}
	defer f.Close()

	kv := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		// Strip inline comments
		if ci := strings.Index(val, " #"); ci >= 0 {
			val = strings.TrimSpace(val[:ci])
		}
		// Strip surrounding quotes
		if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
			val = val[1 : len(val)-1]
		}
		kv[key] = val
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	return buildConfig(kv)
}

// buildConfig constructs and validates a Config from raw key-value pairs.
func buildConfig(kv map[string]string) (*Config, error) {
	c := &Config{}
	var errs []string

	strOr := func(key, def string) string {
		if v, ok := kv[key]; ok && v != "" {
			return v
		}
		return def
	}
	boolVal := func(key string, def bool) bool {
		v, ok := kv[key]
		if !ok {
			return def
		}
		lower := strings.ToLower(v)
		return lower == "on" || lower == "1" || lower == "true"
	}
	// intMin enforces a non-zero minimum (used for UpdateTime, DDNSTime).
	intMin := func(key string, def, min int) int {
		v, ok := kv[key]
		if !ok {
			return def
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: invalid integer %q", key, v))
			return def
		}
		if n < min {
			return min
		}
		return n
	}
	// intGate allows 0 (= feature disabled); enforces minimum only when > 0.
	intGate := func(key string, def, min int) int {
		v, ok := kv[key]
		if !ok {
			return def
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: invalid integer %q", key, v))
			return def
		}
		if n == 0 {
			return 0
		}
		if n < min {
			return min
		}
		return n
	}

	// --- Core ---
	c.StateDir = strOr("STATE_DIR", "/etc/dipper_ai/state")

	// --- IP settings ---
	c.IPv4 = boolVal("IPV4", true)
	c.IPv6 = boolVal("IPV6", false)
	c.IPv4DDNS = boolVal("IPV4_DDNS", true)
	c.IPv6DDNS = boolVal("IPV6_DDNS", true)

	// --- Time gates ---
	c.UpdateTime = intMin("UPDATE_TIME", 1440, 3)
	c.DDNSTime = intMin("DDNS_TIME", 1, 1)
	c.IPCacheTime = intGate("IP_CACHE_TIME", 0, 15)
	c.ErrChkTime = intGate("ERR_CHK_TIME", 0, 1)

	// --- MyDNS entries (MYDNS_0_*, MYDNS_1_*, ...) ---
	c.MyDNSIPv4URL = strOr("MYDNS_IPV4_URL", "https://ipv4.mydns.jp/login.html")
	c.MyDNSIPv6URL = strOr("MYDNS_IPV6_URL", "https://ipv6.mydns.jp/login.html")

	for i := 0; ; i++ {
		prefix := fmt.Sprintf("MYDNS_%d_", i)
		id := strOr(prefix+"ID", "")
		if id == "" {
			break
		}
		e := MyDNSEntry{
			ID:     id,
			Pass:   strOr(prefix+"PASS", ""),
			Domain: strOr(prefix+"DOMAIN", ""),
			IPv4:   boolVal(prefix+"IPV4", true),
			IPv6:   boolVal(prefix+"IPV6", false),
		}
		if e.Pass == "" {
			errs = append(errs, fmt.Sprintf("%sPASS: required", prefix))
		}
		c.MyDNS = append(c.MyDNS, e)
	}

	// --- Cloudflare entries (CF_0_*, CF_1_*, ...) ---
	c.CloudflareURL = strOr("CLOUDFLARE_URL", "https://api.cloudflare.com/client/v4/zones")

	for i := 0; ; i++ {
		prefix := fmt.Sprintf("CF_%d_", i)
		if _, exists := kv[prefix+"ENABLED"]; !exists {
			break
		}
		e := CloudflareEntry{
			Enabled: boolVal(prefix+"ENABLED", false),
			API:     strOr(prefix+"API", ""),
			Zone:    strOr(prefix+"ZONE", ""),
			Domain:  strOr(prefix+"DOMAIN", ""),
			IPv4:    boolVal(prefix+"IPV4", true),
			IPv6:    boolVal(prefix+"IPV6", false),
		}
		if e.Enabled {
			if e.API == "" {
				errs = append(errs, fmt.Sprintf("%sAPI: required when enabled", prefix))
			}
			if e.Zone == "" {
				errs = append(errs, fmt.Sprintf("%sZONE: required when enabled", prefix))
			}
			if e.Domain == "" {
				errs = append(errs, fmt.Sprintf("%sDOMAIN: required when enabled", prefix))
			}
		}
		c.Cloudflare = append(c.Cloudflare, e)
	}

	// --- Email notification ---
	c.EmailChkDDNS = boolVal("EMAIL_CHK_DDNS", false)
	c.EmailUpDDNS = boolVal("EMAIL_UP_DDNS", false)
	c.EmailAddr = strOr("EMAIL_ADR", "")
	if (c.EmailChkDDNS || c.EmailUpDDNS) && c.EmailAddr == "" {
		errs = append(errs, "EMAIL_ADR: required when EMAIL_CHK_DDNS or EMAIL_UP_DDNS is on")
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("config errors:\n  %s", strings.Join(errs, "\n  "))
	}
	return c, nil
}
