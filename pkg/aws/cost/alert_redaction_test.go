package cost

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #1041: a webhook or Slack URL is the credential. A failed delivery used to
// wrap *url.Error with %w, printing the whole URL (fleet monitor repeated it
// every interval), and a failed alert started the 24h cooldown anyway.

func testAlert() *BudgetAlert {
	return &BudgetAlert{ID: "a", Timestamp: time.Now(), Type: AlertTypeCostThreshold, Severity: SeverityWarning, ProjectID: "p"}
}

func TestAlertDeliveryErrors_DoNotLeakURL(t *testing.T) {
	// Port 1 on loopback refuses at once; the paths, query and userinfo are
	// the secrets.
	tests := []struct {
		name    string
		cfg     BudgetAlertConfig
		secrets []string
	}{
		{
			name: "webhook unreachable",
			cfg: BudgetAlertConfig{WebhookEnabled: true,
				WebhookURL: "http://127.0.0.1:1/hooks/T000/WEBHOOKSECRET?token=QUERYSECRET"},
			secrets: []string{"WEBHOOKSECRET", "QUERYSECRET", "/hooks/"},
		},
		{
			name: "slack unreachable",
			cfg: BudgetAlertConfig{SlackEnabled: true,
				SlackWebhookURL: "http://bot:PASSWORDSECRET@127.0.0.1:1/services/T000/B000/SLACKSECRET"},
			secrets: []string{"SLACKSECRET", "PASSWORDSECRET", "/services/"},
		},
		{
			name: "webhook URL that does not parse",
			cfg: BudgetAlertConfig{WebhookEnabled: true,
				WebhookURL: "http://example.invalid/%zzPARSESECRET"},
			secrets: []string{"PARSESECRET"},
		},
		{
			name: "slack URL that does not parse",
			cfg: BudgetAlertConfig{SlackEnabled: true,
				SlackWebhookURL: "http://example.invalid/%zzSLACKPARSESECRET"},
			secrets: []string{"SLACKPARSESECRET"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.cfg
			cfg.Enabled, cfg.SendProjectAlerts, cfg.WebhookTimeout = true, true, 2*time.Second
			err := NewBudgetAlertNotifier(&cfg, aws.Config{}).SendAlert(context.Background(), testAlert())
			require.Error(t, err)
			for _, s := range tt.secrets {
				assert.NotContains(t, err.Error(), s, "the alert error leaks the URL")
			}
		})
	}
}

// TestAlertDeliveryError_NamesHost: the redacted error still says where it
// failed to deliver.
func TestAlertDeliveryError_NamesHost(t *testing.T) {
	cfg := &BudgetAlertConfig{Enabled: true, SendProjectAlerts: true, WebhookEnabled: true,
		WebhookURL: "http://127.0.0.1:1/hooks/SECRET", WebhookTimeout: 2 * time.Second}
	err := NewBudgetAlertNotifier(cfg, aws.Config{}).SendAlert(context.Background(), testAlert())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "http://127.0.0.1:1")
}

// flakyServer fails its first `fails` requests with 500, then answers 200.
func flakyServer(t *testing.T, fails int32) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) <= fails {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestFailedAlertDoesNotStartCooldown(t *testing.T) {
	srv, calls := flakyServer(t, 1)
	n := NewBudgetAlertNotifier(&BudgetAlertConfig{Enabled: true, SendProjectAlerts: true,
		WebhookEnabled: true, WebhookURL: srv.URL, WebhookTimeout: 5 * time.Second, CooldownPeriod: time.Hour}, aws.Config{})
	ctx := context.Background()

	require.Error(t, n.SendAlert(ctx, testAlert()), "first delivery fails")
	require.NoError(t, n.SendAlert(ctx, testAlert()), "a failed alert must be retried, not suppressed for the cooldown")
	assert.Equal(t, int32(2), calls.Load())

	require.NoError(t, n.SendAlert(ctx, testAlert()))
	assert.Equal(t, int32(2), calls.Load(), "a delivered alert starts the cooldown")
}

// TestCooldownIsPerChannel: when one channel fails and another delivers, the
// retry goes to the failed channel only, so the working one isn't spammed.
func TestCooldownIsPerChannel(t *testing.T) {
	slack, slackCalls := flakyServer(t, 1)
	hook, hookCalls := flakyServer(t, 0)
	n := NewBudgetAlertNotifier(&BudgetAlertConfig{Enabled: true, SendProjectAlerts: true, CooldownPeriod: time.Hour,
		WebhookEnabled: true, WebhookURL: hook.URL, WebhookTimeout: 5 * time.Second,
		SlackEnabled: true, SlackWebhookURL: slack.URL}, aws.Config{})
	ctx := context.Background()

	require.Error(t, n.SendAlert(ctx, testAlert()))
	require.NoError(t, n.SendAlert(ctx, testAlert()))
	assert.Equal(t, int32(2), slackCalls.Load(), "the failed channel is retried")
	assert.Equal(t, int32(1), hookCalls.Load(), "the delivered channel is in cooldown")
}
