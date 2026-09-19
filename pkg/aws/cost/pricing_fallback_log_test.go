package cost

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

// TestLogPricingFallback_WarnsOnceThenDebug is the #657 guard: falling back to the
// static pricing table is designed, non-fatal behavior (a write-only fleet agent has
// no pricing:GetProducts by design), so it must warn ONCE per manager and log at debug
// afterwards — otherwise a healthy unattended backup emits pricing warnings forever.
func TestLogPricingFallback_WarnsOnceThenDebug(t *testing.T) {
	var buf bytes.Buffer
	// Level Debug so both the warning and the subsequent debug lines are captured.
	pm := &PricingManager{
		logger: slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})),
	}

	err := errors.New("AccessDeniedException: not authorized to perform: pricing:GetProducts")
	for i := 0; i < 5; i++ {
		pm.logPricingFallback("AWS storage pricing lookup failed", err)
	}

	out := buf.String()
	if got := strings.Count(out, "level=WARN"); got != 1 {
		t.Errorf("want exactly 1 WARN across 5 fallbacks, got %d\n%s", got, out)
	}
	if got := strings.Count(out, "level=DEBUG"); got != 4 {
		t.Errorf("want 4 DEBUG lines after the first warning, got %d\n%s", got, out)
	}
	// The one warning must still say what happened and that it's non-fatal.
	if !strings.Contains(out, "static fallback table") {
		t.Errorf("warning should explain the fallback:\n%s", out)
	}
}

// TestLogPricingFallback_PerManager confirms the once-only warning is scoped to a
// manager instance, not process-global — a second manager must still warn once.
func TestLogPricingFallback_PerManager(t *testing.T) {
	newPM := func(buf *bytes.Buffer) *PricingManager {
		return &PricingManager{logger: slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))}
	}
	var a, b bytes.Buffer
	pmA, pmB := newPM(&a), newPM(&b)
	err := errors.New("boom")

	pmA.logPricingFallback("x", err)
	pmA.logPricingFallback("x", err)
	pmB.logPricingFallback("x", err)

	if got := strings.Count(a.String(), "level=WARN"); got != 1 {
		t.Errorf("manager A: want 1 WARN, got %d", got)
	}
	if got := strings.Count(b.String(), "level=WARN"); got != 1 {
		t.Errorf("manager B should warn independently: want 1 WARN, got %d", got)
	}
}
