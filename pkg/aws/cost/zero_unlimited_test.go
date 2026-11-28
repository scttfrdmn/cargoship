package cost

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #1030: a limit of 0 means unlimited, for cost and volume, project and global.
// `budget set p --volume 500` (no --cost) and the documented "0 = unlimited"
// both store MaxBudget 0, which used to refuse every upload for the project.
func TestZeroLimitMeansUnlimited(t *testing.T) {
	type check struct {
		name    string
		project string
		costUSD float64 // checked with CheckProjectBudget when volGB == 0
		volGB   float64 // checked with CheckProjectVolumeQuota when > 0
		blocked bool
	}
	tests := []struct {
		name   string
		setup  func(t *testing.T, m *Manager)
		checks []check
	}{
		{
			name: "project volume-only budget",
			setup: func(t *testing.T, m *Manager) {
				require.NoError(t, m.SetProjectBudget("p", 0, 500, 0.8, 0.75))
			},
			checks: []check{
				{name: "any cost passes", project: "p", costUSD: 1500},
				{name: "volume under quota passes", project: "p", volGB: 400},
				{name: "volume over quota blocks", project: "p", volGB: 600, blocked: true},
			},
		},
		{
			name: "project cost-only budget",
			setup: func(t *testing.T, m *Manager) {
				require.NoError(t, m.SetProjectBudget("p", 100, 0, 0.8, 0.75))
			},
			checks: []check{
				{name: "any volume passes", project: "p", volGB: 1e6},
				{name: "cost under budget passes", project: "p", costUSD: 50},
				{name: "cost over budget blocks", project: "p", costUSD: 150, blocked: true},
			},
		},
		{
			name: "project fully unlimited (--cost 0 --volume 0)",
			setup: func(t *testing.T, m *Manager) {
				require.NoError(t, m.SetProjectBudget("p", 0, 0, 0.8, 0.75))
			},
			checks: []check{
				{name: "cost passes", project: "p", costUSD: 1e6},
				{name: "volume passes", project: "p", volGB: 1e6},
			},
		},
		{
			name: "global volume-only ceiling",
			setup: func(t *testing.T, m *Manager) {
				require.NoError(t, m.SetGlobalBudget(0, 500, 0.8, 0.75))
			},
			checks: []check{
				{name: "cost passes", project: "nobudget", costUSD: 1e6},
				{name: "volume over ceiling blocks", project: "nobudget", volGB: 600, blocked: true},
			},
		},
		{
			name: "global cost-only ceiling",
			setup: func(t *testing.T, m *Manager) {
				require.NoError(t, m.SetGlobalBudget(100, 0, 0.8, 0.75))
			},
			checks: []check{
				{name: "volume passes", project: "nobudget", volGB: 1e6},
				{name: "cost over ceiling blocks", project: "nobudget", costUSD: 150, blocked: true},
			},
		},
		{
			name: "budget period with no cost cap",
			setup: func(t *testing.T, m *Manager) {
				m.config.BudgetPeriods = []config.BudgetPeriod{{Type: config.BudgetPeriodMonthly, MaxBudget: 0, MaxVolumeGB: 500}}
			},
			checks: []check{
				{name: "cost passes", project: "nobudget", costUSD: 1e6},
				{name: "period volume still enforced", project: "nobudget", volGB: 600, blocked: true},
			},
		},
		{
			name: "legacy monthly budget of 0",
			setup: func(t *testing.T, m *Manager) {
				m.config.MaxMonthlyBudget = 0
				require.NoError(t, m.SetProjectBudget("vp", 0, 500, 0.8, 0.75))
			},
			checks: []check{
				{name: "cost with no project budget passes", project: "nobudget", costUSD: 1e6},
				{name: "cost on a volume-only project passes the global check too", project: "vp", costUSD: 1e6},
			},
		},
		{
			name:  "legacy monthly budget, explicitly set",
			setup: func(t *testing.T, m *Manager) { m.config.MaxMonthlyBudget = 1000 },
			checks: []check{
				{name: "cost under passes", project: "nobudget", costUSD: 999},
				{name: "cost over blocks", project: "nobudget", costUSD: 1500, blocked: true},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateBudgetStore(t)
			m, err := NewManager(testCostConfig(), aws.Config{}, nil)
			require.NoError(t, err)
			tt.setup(t, m)
			for _, c := range tt.checks {
				var err error
				if c.volGB > 0 {
					err = m.CheckProjectVolumeQuota(c.project, c.volGB)
				} else {
					err = m.CheckProjectBudget(c.project, c.costUSD)
				}
				if c.blocked {
					assert.Error(t, err, c.name)
				} else {
					assert.NoError(t, err, c.name)
				}
			}
		})
	}
}

// TestVolumeOnlyBudget_RecordOperationCost drives the recording/enforcement path
// an upload goes through.
func TestVolumeOnlyBudget_RecordOperationCost(t *testing.T) {
	isolateBudgetStore(t)
	m, err := NewManager(testCostConfig(), aws.Config{}, nil)
	require.NoError(t, err)
	require.NoError(t, m.SetProjectBudget("p", 0, 500, 0.8, 0.75))

	err = m.RecordOperationCost(context.Background(), "upload", "f", 10*1024*1024*1024,
		config.StorageClassStandard, "us-east-1", "job", "p", nil)
	assert.NoError(t, err, "a volume-only budget must not block on cost")
}

// TestVolumeOnlyBudget_StatusIsUnlimitedAndMarshals: status must agree with
// enforcement (no cost cap means not over budget) and stay JSON-encodable
// (`budget status --json`), which a 0/0 or x/0 usage ratio breaks.
func TestVolumeOnlyBudget_StatusIsUnlimitedAndMarshals(t *testing.T) {
	isolateBudgetStore(t)
	ctx := context.Background()
	m, err := NewManager(testCostConfig(), aws.Config{}, nil)
	require.NoError(t, err)
	require.NoError(t, m.SetProjectBudget("p", 0, 500, 0.8, 0.75))

	for _, withSpend := range []bool{false, true} {
		if withSpend {
			require.NoError(t, m.RecordCompletedUpload(ctx, "f", 1024*1024*1024, config.StorageClassStandard, "us-east-1", "job", "p", nil))
		}
		st, err := m.GetProjectBudgetStatus("p")
		require.NoError(t, err)
		assert.False(t, st.OverBudget, "no cost cap: never over budget")
		assert.False(t, st.AlertTriggered)
		assert.False(t, st.WillExceedBudget)
		assert.Zero(t, st.BudgetUsed)
		_, err = json.Marshal(st)
		assert.NoError(t, err, "status must marshal (spend recorded: %v)", withSpend)
	}
}
