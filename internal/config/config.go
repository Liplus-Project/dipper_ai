// Package config loads and validates dipper_ai configuration.
// Config file format is intentionally different from dipper (MAY differ per spec).
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Config holds all runtime configuration for dipper_ai.
type Config struct {
	// State directory (cache-equivalent storage)
	StateDir string

	// Time gates (minutes)
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

	// Notification
	ErrMailEnabled bool
	ErrThreshold   int
	MailTo         string
}

// MyDNSConfig holds MyDNS credentials and domains.
type MyDNSConfig struct {
	Enabled  bool
	MasterID string
	Password string
	Domains  []string
}

// CloudflareEntry holds Cloudflare credentials for one domain.
type CloudflareEntry struct {
	Enabled bool
	Token   string
	ZoneID  string
	Name    string
}

// Load reads the config file. The path is resolved from the working directory
// or a well-known system location. It follows systemd WorkingDirectory semantics.
func Load() (*Config, error) {
	path := configFilePath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found: %s", path)
	}
	return parseFile(path)
}

// configFilePath returns the resolved config file path.
func configFilePath() string {
	if v := os.Getenv("DIPPER_AI_CONFIG"); v != "" {
		return v
	}
	return filepath.Join(workingDir(), "user.conf")
}

func workingDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return "/etc/dipper_ai"
	}
	return wd
}

// parseFile parses a user.conf-format file into Config.
// TODO: implement full parser (key=value, array syntax for Cloudflare entries)
func parseFile(path string) (*Config, error) {
	_ = path
	return nil, fmt.Errorf("parseFile: not yet implemented")
}
