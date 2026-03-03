// Package timegate implements the time-gate (throttle) logic equivalent to dipper.
// Gates determine whether an action should be executed based on elapsed time.
package timegate

import (
	"os"
	"path/filepath"
	"time"
)

// Gate represents a named time gate backed by a timestamp file.
type Gate struct {
	StateDir string
	Name     string
	Interval time.Duration
}

// New creates a Gate.
func New(stateDir, name string, interval time.Duration) *Gate {
	return &Gate{StateDir: stateDir, Name: name, Interval: interval}
}

// ShouldRun returns true if the gate has elapsed or never been triggered.
func (g *Gate) ShouldRun() bool {
	ts, err := g.readTimestamp()
	if err != nil {
		// No timestamp → gate never triggered → should run
		return true
	}
	return time.Since(ts) >= g.Interval
}

// Touch records the current time as the gate's last-run timestamp.
func (g *Gate) Touch() error {
	return os.WriteFile(g.stateFile(), []byte(time.Now().Format(time.RFC3339)), 0644)
}

// stateFile returns the path of the timestamp file.
func (g *Gate) stateFile() string {
	return filepath.Join(g.StateDir, "gate_"+g.Name)
}

// readTimestamp reads and parses the timestamp file.
func (g *Gate) readTimestamp() (time.Time, error) {
	data, err := os.ReadFile(g.stateFile())
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339, string(data))
}
