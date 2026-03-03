// Package lock implements event_lock: prevents double-execution of the same mode.
// Uses a PID file with staleness detection for safe recovery after crashes.
package lock

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// EventLock prevents concurrent execution of the same dipper_ai command.
type EventLock struct {
	path string
}

// NewEventLock creates an EventLock for the given mode inside stateDir.
func NewEventLock(stateDir, mode string) *EventLock {
	return &EventLock{
		path: filepath.Join(stateDir, "lock_"+mode+".pid"),
	}
}

// Acquire attempts to acquire the lock.
// Returns an error if another instance is running; clears stale locks automatically.
func (l *EventLock) Acquire() error {
	if err := os.MkdirAll(filepath.Dir(l.path), 0755); err != nil {
		return fmt.Errorf("lock: mkdir: %w", err)
	}

	// Check for existing lock
	if data, err := os.ReadFile(l.path); err == nil {
		pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
		if parseErr == nil && processExists(pid) {
			return fmt.Errorf("pid %d is still running", pid)
		}
		// Stale lock — remove it
		_ = os.Remove(l.path)
	}

	// Write our PID
	return os.WriteFile(l.path, []byte(strconv.Itoa(os.Getpid())+"\n"), 0644)
}

// Release removes the lock file.
func (l *EventLock) Release() {
	_ = os.Remove(l.path)
}

// processExists returns true if pid is an existing, reachable process.
func processExists(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}
