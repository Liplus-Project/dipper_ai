package mode

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/Liplus-Project/dipper_ai/internal/config"
	"github.com/Liplus-Project/dipper_ai/internal/state"
	"github.com/Liplus-Project/dipper_ai/internal/timegate"
)

// ErrMail aggregates errors and sends a notification if the threshold is met.
// Equivalent to `dipper err_mail`.
func ErrMail(cfg *config.Config) error {
	if cfg.ErrChkTime == 0 || cfg.EmailAddr == "" {
		return nil // disabled or no recipient configured
	}

	// --- Time gate: ERR_CHK_TIME ---
	errGate := timegate.New(cfg.StateDir, "err_mail", time.Duration(cfg.ErrChkTime)*time.Minute)
	if !errGate.ShouldRun() {
		return nil
	}

	st, err := state.New(cfg.StateDir)
	if err != nil {
		return err
	}

	errors, err := st.ReadErrors()
	if err != nil {
		return err
	}

	if len(errors) == 0 {
		_ = errGate.Touch()
		return nil
	}

	// --- Send notification ---
	body := fmt.Sprintf("dipper_ai error report (%d errors):\n\n%s\n",
		len(errors), strings.Join(errors, "\n"))

	if err := sendMail(cfg.EmailAddr, "dipper_ai: error notification", body); err != nil {
		_ = st.AppendError(fmt.Sprintf("sendmail_failed: %v", err))
		return fmt.Errorf("sendmail: %w", err)
	}

	_ = st.ClearErrors()
	_ = errGate.Touch()
	return nil
}

// sendMail delivers a message via sendmail(1).
// In CI/tests this is stubbed by replacing the sendmail binary in PATH.
func sendMail(to, subject, body string) error {
	msg := fmt.Sprintf("To: %s\nSubject: %s\n\n%s", to, subject, body)
	cmd := exec.Command("sendmail", "-t")
	cmd.Stdin = strings.NewReader(msg)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
