package cost

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestRedactURL(t *testing.T) {
	tests := []struct{ in, want string }{
		{"https://hooks.slack.com/services/T/B/SECRET", "https://hooks.slack.com"},
		{"https://user:pw@example.com:8443/x?token=SECRET#f", "https://example.com:8443"},
		{"http://127.0.0.1:1/hooks", "http://127.0.0.1:1"},
		{"not a url", "<redacted URL>"},
		{"http://example.invalid/%zzSECRET", "<redacted URL>"},
		{"", "<redacted URL>"},
	}
	for _, tt := range tests {
		got := RedactURL(tt.in)
		if got != tt.want {
			t.Errorf("RedactURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
		if strings.Contains(got, "SECRET") || strings.Contains(got, "pw") {
			t.Errorf("RedactURL(%q) kept a secret: %q", tt.in, got)
		}
	}
}

func TestRedactAlertSecrets(t *testing.T) {
	raw := "https://bot:PASSWORD@hooks.example.com/services/PATHSECRET?sig=QUERYSECRET"
	tests := []struct {
		name string
		err  error
	}{
		{"whole URL", fmt.Errorf("upstream said: %s is gone", raw)},
		{"path only", errors.New("404 for /services/PATHSECRET")},
		{"query only", errors.New("bad signature sig=QUERYSECRET")},
		{"password", errors.New("auth PASSWORD rejected")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactAlertSecrets(tt.err, raw).Error()
			for _, s := range []string{"PATHSECRET", "QUERYSECRET", "PASSWORD"} {
				if strings.Contains(got, s) {
					t.Errorf("%q still contains %s", got, s)
				}
			}
		})
	}

	clean := errors.New("connection refused")
	if got := redactAlertSecrets(clean, raw); got != clean {
		t.Errorf("an error with nothing to redact must be returned as is (keeping its chain), got %v", got)
	}
	if redactAlertSecrets(nil, raw) != nil || redactURLError(nil, raw) != nil {
		t.Error("nil stays nil")
	}
}

// TestRedactURLError_KeepsCause: the underlying cause stays matchable after
// redaction (errors.Is through the rewritten *url.Error).
func TestRedactURLError_KeepsCause(t *testing.T) {
	cfg := &BudgetAlertConfig{Enabled: true, SendProjectAlerts: true, WebhookEnabled: true,
		WebhookURL: "http://10.255.255.1/hooks/SECRET", WebhookTimeout: 5 * time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := NewBudgetAlertNotifier(cfg, aws.Config{}).sendWebhookAlert(ctx, testAlert())
	if err == nil {
		t.Fatal("a cancelled context must fail the send")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("cause lost: %v", err)
	}
	if strings.Contains(err.Error(), "SECRET") {
		t.Errorf("leaked: %v", err)
	}
}
