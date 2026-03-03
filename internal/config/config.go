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

	// Time gates (minutes; minimum 1)
	UpdateTime  int // UPDATE_TIME
	DDNSTime    int // DDNS_TIME
	IPCacheTime int // IP_CACHE_TIME
	ErrChkTime  int // ERR_CHK_TIME

	// IP settings
	IPv4     bool
	IPv6     bool
	IPv4DDNS bool
	IPv6DDNS bool

	// DDNS providers
	MyDNS      MyDNSConfig
	Cloudflare []CloudflareEntry

	// Error notification
	ErrMailEnabled bool
	ErrThreshold   int
	MailTo         string
}

// MyDNSConfig holds MyDNS credentials.
type MyDNSConfig struct {
	Enabled  bool
	MasterID string
	Password string
}

// CloudflareEntry holds Cloudflare credentials for one DNS record.
type CloudflareEntry struct {
	Enabled bool
	Token   string
	ZoneID  string
	Name    string // FQDN
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
		// Strip inline comments (value must not contain " #")
		if ci := strings.Index(val, " #"); ci >= 0 {
			val = strings.TrimSpace(val[:ci])
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
	intVal := func(key string, def, min int) int {
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
	require := func(key string) string {
		v, ok := kv[key]
		if !ok || v == "" {
			errs = append(errs, fmt.Sprintf("%s: required but not set", key))
			return ""
		}
		return v
	}

	// --- Core ---
	c.StateDir = strOr("STATE_DIR", "/etc/dipper_ai/state")

	// --- Time gates ---
	c.UpdateTime = intVal("UPDATE_TIME", 60, 1)
	c.DDNSTime = intVal("DDNS_TIME", 10, 1)
	c.IPCacheTime = intVal("IP_CACHE_TIME", 30, 1)
	c.ErrChkTime = intVal("ERR_CHK_TIME", 120, 1)

	// --- IP settings ---
	c.IPv4 = boolVal("IPV4", true)
	c.IPv6 = boolVal("IPV6", true)
	c.IPv4DDNS = boolVal("IPV4_DDNS", true)
	c.IPv6DDNS = boolVal("IPV6_DDNS", true)

	// --- MyDNS ---
	c.MyDNS.Enabled = boolVal("MYDNS_ENABLED", false)
	if c.MyDNS.Enabled {
		c.MyDNS.MasterID = require("MYDNS_MASTERID")
		c.MyDNS.Password = require("MYDNS_PASSWORD")
	}

	// --- Cloudflare (CF_0_*, CF_1_*, ...) ---
	for i := 0; ; i++ {
		prefix := fmt.Sprintf("CF_%d_", i)
		if _, exists := kv[prefix+"ENABLED"]; !exists {
			break
		}
		e := CloudflareEntry{
			Enabled: boolVal(prefix+"ENABLED", false),
			Token:   strOr(prefix+"TOKEN", ""),
			ZoneID:  strOr(prefix+"ZONE_ID", ""),
			Name:    strOr(prefix+"NAME", ""),
		}
		if e.Enabled {
			if e.Token == "" {
				errs = append(errs, fmt.Sprintf("%sTOKEN: required when enabled", prefix))
			}
			if e.ZoneID == "" {
				errs = append(errs, fmt.Sprintf("%sZONE_ID: required when enabled", prefix))
			}
			if e.Name == "" {
				errs = append(errs, fmt.Sprintf("%sNAME: required when enabled", prefix))
			}
		}
		c.Cloudflare = append(c.Cloudflare, e)
	}

	// --- Error mail ---
	c.ErrMailEnabled = boolVal("ERR_MAIL_ENABLED", false)
	if c.ErrMailEnabled {
		c.MailTo = require("MAIL_TO")
	}
	c.ErrThreshold = intVal("ERR_THRESHOLD", 1, 1)

	if len(errs) > 0 {
		return nil, fmt.Errorf("config errors:\n  %s", strings.Join(errs, "\n  "))
	}
	return c, nil
}
