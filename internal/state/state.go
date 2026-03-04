// Package state manages the persistent state (cache-equivalent) for dipper_ai.
// State files reside in StateDir and represent observable output for test comparison.
package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Manager provides read/write access to state files.
type Manager struct {
	Dir string
}

// New creates a Manager rooted at dir, creating it if needed.
func New(dir string) (*Manager, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("state: cannot create dir %s: %w", dir, err)
	}
	return &Manager{Dir: dir}, nil
}

// ReadIP returns the cached IP for the given key ("ipv4" or "ipv6").
// Returns ("0.0.0.0" / "::" , nil) when not yet cached so that any real
// IP address is always treated as a change on first run.
func (m *Manager) ReadIP(key string) (string, error) {
	data, err := os.ReadFile(m.path("ip_" + key))
	if os.IsNotExist(err) {
		if key == "ipv6" {
			return "::", nil
		}
		return "0.0.0.0", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// WriteIP persists the IP for the given key.
func (m *Manager) WriteIP(key, ip string) error {
	return os.WriteFile(m.path("ip_"+key), []byte(ip+"\n"), 0644)
}

// ReadDDNSResult returns the last DDNS result for the given provider+domain key.
func (m *Manager) ReadDDNSResult(key string) (string, error) {
	data, err := os.ReadFile(m.path("ddns_" + key))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// WriteDDNSResult persists the DDNS result ("ok" or "fail:<reason>").
func (m *Manager) WriteDDNSResult(key, result string) error {
	return os.WriteFile(m.path("ddns_"+key), []byte(result+"\n"), 0644)
}

// AppendError appends an error entry to the error log for err_mail aggregation.
func (m *Manager) AppendError(entry string) error {
	f, err := os.OpenFile(m.path("errors.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, entry)
	return err
}

// ReadErrors returns all error entries since the last clear.
func (m *Manager) ReadErrors() ([]string, error) {
	data, err := os.ReadFile(m.path("errors.log"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, l := range strings.Split(string(data), "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines, nil
}

// ClearErrors removes the error log (called after successful err_mail dispatch).
func (m *Manager) ClearErrors() error {
	err := os.Remove(m.path("errors.log"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (m *Manager) path(name string) string {
	return filepath.Join(m.Dir, name)
}
