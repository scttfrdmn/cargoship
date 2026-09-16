package cost

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/scttfrdmn/cargoship/pkg/aws/config"
)

func newAlertTestManager(t *testing.T, storePath string) *Manager {
	t.Helper()
	t.Setenv("CARGOSHIP_BUDGET_STORE", storePath)
	dc := config.DefaultAWSConfig()
	mgr, err := NewManager(&dc.CostControl, aws.Config{}, slog.Default())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return mgr
}

// TestAlertConfigPersistence: UpdateAlertConfig persists the non-secret config (and
// NOT the secret), and a fresh Manager on the same store rehydrates it (#630).
func TestAlertConfigPersistence(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "budgets.json")
	t.Setenv("CARGOSHIP_SLACK_WEBHOOK_URL", "https://hooks.example/SECRET")

	m1 := newAlertTestManager(t, storePath)
	cfg := DefaultBudgetAlertConfig()
	cfg.SlackEnabled = true
	cfg.SlackChannel = "#backups"
	if err := m1.UpdateAlertConfig(cfg); err != nil {
		t.Fatalf("UpdateAlertConfig: %v", err)
	}

	data, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("read store: %v", err)
	}
	if strings.Contains(string(data), "hooks.example") || strings.Contains(string(data), "SECRET") {
		t.Fatalf("persisted store leaked the webhook secret:\n%s", data)
	}
	if !strings.Contains(string(data), `"slack_enabled": true`) {
		t.Fatalf("expected slack_enabled persisted:\n%s", data)
	}

	// Fresh process on the same store reflects the persisted (non-secret) config,
	// and the secret is sourced from the environment.
	m2 := newAlertTestManager(t, storePath)
	got := m2.GetAlertConfig()
	if !got.SlackEnabled || got.SlackChannel != "#backups" {
		t.Fatalf("persisted config not rehydrated: %+v", got)
	}
	if got.SlackWebhookURL != "https://hooks.example/SECRET" {
		t.Fatalf("secret should come from env, got %q", got.SlackWebhookURL)
	}
}

// TestUpdateAlertConfig_ValidatesWithEnvSecret: an enabled channel requires its secret
// via a flag OR the environment.
func TestUpdateAlertConfig_ValidatesWithEnvSecret(t *testing.T) {
	m := newAlertTestManager(t, filepath.Join(t.TempDir(), "b.json"))
	cfg := DefaultBudgetAlertConfig()
	cfg.SlackEnabled = true

	if err := m.UpdateAlertConfig(cfg); err == nil {
		t.Fatal("slack enabled without a webhook (flag or env) should error")
	}
	t.Setenv("CARGOSHIP_SLACK_WEBHOOK_URL", "https://hooks.example/x")
	if err := m.UpdateAlertConfig(cfg); err != nil {
		t.Fatalf("slack enabled with an env webhook should pass: %v", err)
	}
}

func TestWithSecretsFromEnv(t *testing.T) {
	t.Setenv("CARGOSHIP_SMTP_PASSWORD", "pw")
	t.Setenv("CARGOSHIP_SLACK_WEBHOOK_URL", "sl")
	t.Setenv("CARGOSHIP_WEBHOOK_URL", "wh")
	got := withSecretsFromEnv(DefaultBudgetAlertConfig())
	if got.SMTPPassword != "pw" || got.SlackWebhookURL != "sl" || got.WebhookURL != "wh" {
		t.Fatalf("env secrets not applied: %+v", got)
	}
	if withSecretsFromEnv(nil) != nil {
		t.Error("nil in → nil out")
	}
}
