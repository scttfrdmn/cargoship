package cmd

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	cargoconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/scttfrdmn/cargoship/pkg/aws/cost"
)

// TestAlertsConfig_HidesWebhookPath (#1041): `alerts config` shows where the
// webhook goes, never the path or query that carry its secret.
func TestAlertsConfig_HidesWebhookPath(t *testing.T) {
	t.Setenv("CARGOSHIP_BUDGET_STORE", filepath.Join(t.TempDir(), "budgets.json"))
	t.Setenv("CARGOSHIP_WEBHOOK_URL", "https://alerts.example.com/hooks/PATHSECRET?token=QUERYSECRET")

	dc := cargoconfig.DefaultAWSConfig()
	dc.CostControl.Pricing.UseAWSPricingAPI = false
	mgr, err := cost.NewManager(&dc.CostControl, aws.Config{}, nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	cfg := cost.DefaultBudgetAlertConfig()
	cfg.WebhookEnabled = true
	cfg.WebhookTimeout = time.Second
	if err := mgr.UpdateAlertConfig(cfg); err != nil {
		t.Fatalf("UpdateAlertConfig: %v", err)
	}

	var runErr error
	out := captureStdout(t, func() { runErr = runAlertsConfig(context.Background(), false) })
	if runErr != nil {
		t.Fatalf("alerts config: %v", runErr)
	}
	if !strings.Contains(out, "https://alerts.example.com") {
		t.Errorf("the endpoint host should be shown:\n%s", out)
	}
	for _, s := range []string{"PATHSECRET", "QUERYSECRET", "/hooks/"} {
		if strings.Contains(out, s) {
			t.Errorf("alerts config printed %q:\n%s", s, out)
		}
	}
}
