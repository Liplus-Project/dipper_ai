package mode

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Liplus-Project/dipper_ai/internal/config"
)

const (
	defaultCheckInterval = 5 * time.Minute
	startupDelay         = 10 * time.Second
)

// Daemon runs dipper_ai as a long-lived process with two independent tickers:
//   - Check ticker  (DDNS_TIME interval): fetch IP → update if changed → DNS verify
//   - Keepalive ticker (UPDATE_TIME interval): force-update all MyDNS entries
//
// Design rationale:
//   Both intervals are handled internally by goroutine tickers, so any
//   combination of DDNS_TIME and UPDATE_TIME works correctly — including
//   DDNS_TIME=1d with UPDATE_TIME=2m, which was impossible with the previous
//   single-timer systemd approach.
//
//   A single process also means a single log stream: `journalctl -u dipper_ai`
//   shows all activity without needing to distinguish between timer units.
//
// Shutdown: SIGTERM or SIGINT triggers a clean exit.
func Daemon(cfg *config.Config) error {
	// --- Check interval ---
	checkInterval := time.Duration(cfg.DDNSTime) * time.Minute
	if checkInterval <= 0 {
		checkInterval = defaultCheckInterval
	}

	fmt.Fprintf(os.Stderr, "dipper_ai daemon: starting (check=%v", checkInterval)
	if cfg.UpdateTime > 0 {
		fmt.Fprintf(os.Stderr, ", keepalive=%v", time.Duration(cfg.UpdateTime)*time.Minute)
	} else {
		fmt.Fprintf(os.Stderr, ", keepalive=disabled")
	}
	fmt.Fprintf(os.Stderr, ")\n")

	// Short startup delay — gives the network stack time to come up after boot.
	time.Sleep(startupDelay)

	// Run first cycle immediately on startup.
	runCycle(cfg)

	checkTicker := time.NewTicker(checkInterval)
	defer checkTicker.Stop()

	// Keepalive ticker — nil channel blocks forever when keepalive is disabled.
	var keepaliveCh <-chan time.Time
	if cfg.UpdateTime > 0 {
		kt := time.NewTicker(time.Duration(cfg.UpdateTime) * time.Minute)
		defer kt.Stop()
		keepaliveCh = kt.C
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	for {
		select {
		case <-checkTicker.C:
			runCycle(cfg)
		case <-keepaliveCh:
			_ = Keepalive(cfg)
		case sig := <-sigCh:
			fmt.Fprintf(os.Stderr, "dipper_ai daemon: received %v, shutting down\n", sig)
			return nil
		}
	}
}

// runCycle executes one full check-and-update cycle: update → check → err_mail.
func runCycle(cfg *config.Config) {
	if err := Update(cfg); err != nil {
		// Update() already logged the error; non-fatal for the daemon.
		_ = err
	}
	if err := Check(cfg); err != nil {
		_ = err
	}
	if err := ErrMail(cfg); err != nil {
		_ = err
	}
}
