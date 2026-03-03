package mode

import (
	"testing"

	"github.com/Liplus-Project/dipper_ai/internal/config"
	"github.com/Liplus-Project/dipper_ai/internal/state"
)

func baseCfgErrMail(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		StateDir:     t.TempDir(),
		ErrChkTime:   1,
		EmailAddr:    "test@example.com",
		EmailChkDDNS: true,
	}
}

func TestErrMail_Disabled_ErrChkTimeZero(t *testing.T) {
	cfg := baseCfgErrMail(t)
	cfg.ErrChkTime = 0

	called := false
	orig := sendMailFn
	sendMailFn = func(to, subject, body string) error {
		called = true
		return nil
	}
	t.Cleanup(func() { sendMailFn = orig })

	if err := ErrMail(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Error("sendMail should not be called when ErrChkTime=0")
	}
}

func TestErrMail_Disabled_NoEmailAddr(t *testing.T) {
	cfg := baseCfgErrMail(t)
	cfg.EmailAddr = ""

	called := false
	orig := sendMailFn
	sendMailFn = func(to, subject, body string) error { called = true; return nil }
	t.Cleanup(func() { sendMailFn = orig })

	if err := ErrMail(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Error("sendMail should not be called with empty EmailAddr")
	}
}

func TestErrMail_NoErrors_NoMail(t *testing.T) {
	cfg := baseCfgErrMail(t)

	called := false
	orig := sendMailFn
	sendMailFn = func(to, subject, body string) error { called = true; return nil }
	t.Cleanup(func() { sendMailFn = orig })

	if err := ErrMail(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Error("sendMail should not be called when there are no errors")
	}
}

func TestErrMail_WithErrors_SendsMail(t *testing.T) {
	cfg := baseCfgErrMail(t)

	// Write some errors to state
	st, _ := state.New(cfg.StateDir)
	_ = st.AppendError("ddns_error mydns[0]: timeout")

	var sentTo, sentBody string
	orig := sendMailFn
	sendMailFn = func(to, subject, body string) error {
		sentTo = to
		sentBody = body
		return nil
	}
	t.Cleanup(func() { sendMailFn = orig })

	if err := ErrMail(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sentTo != "test@example.com" {
		t.Errorf("expected recipient test@example.com, got %q", sentTo)
	}
	if sentBody == "" {
		t.Error("expected non-empty mail body")
	}

	// Errors should be cleared after send
	errs, _ := st.ReadErrors()
	if len(errs) != 0 {
		t.Errorf("expected errors cleared after send, got %d remaining", len(errs))
	}
}
