package cost

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// defaultCLICostConfig is the cost config every CLI command builds its manager
// from (loadCostManager: DefaultAWSConfig), with the live pricing API off so the
// test stays offline.
func defaultCLICostConfig() *config.CostControlConfig {
	cc := config.DefaultAWSConfig().CostControl
	cc.Pricing.UseAWSPricingAPI = false
	return &cc
}

// TestNoBudgetConfigured_NeverRefuses is the #988 reproduction: with no budget
// set anywhere, the CLI's default config used to carry a hidden $1000/month
// cap, so a large upload, or any upload once the month's spend passed $1000,
// was refused with "global budget exceeded: max_budget=1000.00".
func TestNoBudgetConfigured_NeverRefuses(t *testing.T) {
	isolateBudgetStore(t)
	ctx := context.Background()
	m, err := NewManager(defaultCLICostConfig(), aws.Config{}, nil)
	require.NoError(t, err)

	assert.NoError(t, m.CheckProjectBudget("nobudget", 1500), "no budget configured: a $1500 upload must pass")
	assert.NoError(t, m.CheckProjectBudget("", 1500))
	assert.NoError(t, m.CheckProjectVolumeQuota("nobudget", 1e6))

	// A month that already recorded $5000 doesn't start refusing either.
	m.reporter.RecordCost(CostRecord{Operation: "upload", Service: "s3", Cost: 5000, ProjectID: "other"})
	assert.NoError(t, m.CheckProjectBudget("nobudget", 1))
	approval, err := m.CheckCostApproval(ctx, "upload", 1)
	assert.NoError(t, err)
	assert.False(t, approval)

	const big = int64(100_000) * 1024 * 1024 * 1024 // 100 TB: > $1000 at any class
	assert.NoError(t, m.RecordOperationCost(ctx, "upload", "f", big, config.StorageClassStandard, "us-east-1", "job", "", nil))
}

// TestNoBudgetConfigured_StatusAgrees: every status view reports no cap and not
// over budget, the same as enforcement, and stays JSON-encodable.
func TestNoBudgetConfigured_StatusAgrees(t *testing.T) {
	isolateBudgetStore(t)
	m, err := NewManager(defaultCLICostConfig(), aws.Config{}, nil)
	require.NoError(t, err)
	m.reporter.RecordCost(CostRecord{Operation: "upload", Service: "s3", Cost: 5000, ProjectID: "other"})

	team := m.GetGlobalTeamBudgetStatus()
	assert.Zero(t, team.MaxBudget)
	assert.False(t, team.OverBudget)

	legacy := m.GetBudgetStatus()
	assert.Equal(t, 0.0, legacy["max_budget"], "no hidden monthly cap")
	assert.Equal(t, false, legacy["over_budget"])
	assert.Equal(t, false, legacy["alert_triggered"])
	_, err = json.Marshal(legacy)
	assert.NoError(t, err)

	global := m.GetGlobalBudgetStatus()
	assert.False(t, global.OverBudget)
	_, err = json.Marshal(global)
	assert.NoError(t, err)

	assert.NoError(t, m.ValidateConfig(), "an unset monthly budget is a valid config")
}

// TestNoBudgetConfigured_NoAlertAttempt: recording spend with no budget set must
// not run the budget-alert path (it used to divide by the hidden cap, and by
// zero when the cap was 0).
func TestNoBudgetConfigured_NoAlertAttempt(t *testing.T) {
	isolateBudgetStore(t)
	var logs bytes.Buffer
	m, err := NewManager(defaultCLICostConfig(), aws.Config{}, slog.New(slog.NewTextHandler(&logs, nil)))
	require.NoError(t, err)
	m.reporter.RecordCost(CostRecord{Operation: "upload", Service: "s3", Cost: 5000, ProjectID: "other"})

	require.NoError(t, m.RecordCompletedUpload(context.Background(), "f", 1024*1024*1024, config.StorageClassStandard, "us-east-1", "job", "", nil))
	assert.NotContains(t, logs.String(), "Budget alert triggered")
	assert.NotContains(t, logs.String(), "Failed to check budget alerts")
}

// TestLegacyMonthlyBudget_EnforcedWhenSet: the deprecated cap still works for
// anyone who configures it explicitly.
func TestLegacyMonthlyBudget_EnforcedWhenSet(t *testing.T) {
	isolateBudgetStore(t)
	cc := defaultCLICostConfig()
	cc.MaxMonthlyBudget = 1000
	m, err := NewManager(cc, aws.Config{}, nil)
	require.NoError(t, err)

	var be *BudgetExceededError
	require.ErrorAs(t, m.CheckProjectBudget("nobudget", 1500), &be)
	assert.Equal(t, 1000.0, be.MaxBudget)

	legacy := m.GetBudgetStatus()
	assert.Equal(t, 1000.0, legacy["max_budget"])
	assert.NoError(t, m.ValidateConfig())

	cc.MaxMonthlyBudget = -1
	assert.Error(t, m.ValidateConfig(), "a negative budget is still invalid")
}
