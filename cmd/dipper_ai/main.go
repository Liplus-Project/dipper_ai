package main

import (
	"fmt"
	"os"

	"github.com/Liplus-Project/dipper_ai/internal/config"
	"github.com/Liplus-Project/dipper_ai/internal/lock"
	"github.com/Liplus-Project/dipper_ai/internal/mode"
)

const usage = `Usage: dipper_ai <command>

Commands:
  update    Fetch IP, update DDNS if changed
  check     Check current IP and DDNS status
  keepalive Force-update all DDNS providers (MyDNS keepalive)
  err_mail  Aggregate errors and send notification if threshold met
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}

	cmd := os.Args[1]

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	el := lock.NewEventLock(cfg.StateDir, cmd)
	if err := el.Acquire(); err != nil {
		fmt.Fprintf(os.Stderr, "already running: %v\n", err)
		os.Exit(0)
	}
	defer el.Release()

	var runErr error
	switch cmd {
	case "update":
		runErr = mode.Update(cfg)
	case "check":
		runErr = mode.Check(cfg)
	case "keepalive":
		runErr = mode.Keepalive(cfg)
	case "err_mail":
		runErr = mode.ErrMail(cfg)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n%s", cmd, usage)
		os.Exit(1)
	}

	if runErr != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", runErr)
		os.Exit(1)
	}
}
