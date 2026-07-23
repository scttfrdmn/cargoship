package cost

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSetGlobalBudget_Persists verifies the global/team budget round-trips
// through the store and is visible to a fresh manager (#246 PR2).
func TestSetGlobalBudget_Persists(t *testing.T) {
	t.Setenv(budgetStoreEnvOverride, filepath.Join(t.TempDir(), "budgets.json"))

	mgr, err := NewManager(testCostConfig(), aws.Config{}, nil)
	require.NoError(t, err)
	require.NoError(t, mgr.SetGlobalBudget(1000, 500, 0.8, 0.75))

	// A fresh manager reloads the persisted global budget.
	mgr2, err := NewManager(testCostConfig(), aws.Config{}, nil)
	require.NoError(t, err)
	require.NotNil(t, mgr2.config.GlobalBudget)
	assert.Equal(t, 1000.0, mgr2.config.GlobalBudget.MaxBudget)
	assert.Equal(t, 500.0, mgr2.config.GlobalBudget.MaxVolumeGB)

	st := mgr2.GetGlobalTeamBudgetStatus()
	assert.Equal(t, 1000.0, st.MaxBudget)
	assert.Equal(t, "global", st.BudgetType)
}

// TestLedgerState_GlobalBudgetBackCompat confirms a document without a global
// budget round-trips with GlobalBudget nil (pre-PR2 files unaffected).
func TestLedgerState_GlobalBudgetBackCompat(t *testing.T) {
	t.Setenv(budgetStoreEnvOverride, filepath.Join(t.TempDir(), "budgets.json"))
	store := localStore{}
	require.NoError(t, store.Save(LedgerState{
		Version:        StoreVersion,
		ProjectBudgets: map[string]config.ProjectBudget{"p": {ProjectID: "p", MaxBudget: 10}},
	}, ""))

	got, _, err := store.Load()
	require.NoError(t, err)
	assert.Nil(t, got.GlobalBudget, "absent global budget must stay nil")
}

// TestGlobalBudget_EnforcedAsCeiling proves the global ceiling blocks an upload
// that is under (or has no) project cap but would push aggregate spend over the
// org-wide budget (#246 PR2).
func TestGlobalBudget_EnforcedAsCeiling(t *testing.T) {
	t.Setenv(budgetStoreEnvOverride, filepath.Join(t.TempDir(), "budgets.json"))
	ctx := context.Background()

	mgr, err := NewManager(testCostConfig(), aws.Config{}, nil)
	require.NoError(t, err)

	// Tiny global cost ceiling, no project budget at all.
	require.NoError(t, mgr.SetGlobalBudget(0.01, 0, 0.8, 0.75))

	// A sizeable upload with no project id → falls through to the global check.
	const big = int64(50 * 1024 * 1024 * 1024) // 50 GB, well over $0.01
	err = mgr.RecordOperationCost(ctx, "upload", "f", big, config.StorageClassStandard, "us-east-1", "job", "", nil)
	require.Error(t, err, "upload over the global cost ceiling must be blocked")
	assert.Contains(t, err.Error(), "budget")
}

// TestGlobalVolumeCeiling_EnforcedAcrossProjects proves the global VOLUME
// ceiling is enforced on top of a passing per-project check.
func TestGlobalVolumeCeiling_EnforcedAcrossProjects(t *testing.T) {
	t.Setenv(budgetStoreEnvOverride, filepath.Join(t.TempDir(), "budgets.json"))
	ctx := context.Background()

	mgr, err := NewManager(testCostConfig(), aws.Config{}, nil)
	require.NoError(t, err)
	// Project with a generous volume quota, but a tiny global volume ceiling.
	require.NoError(t, mgr.SetProjectBudget("proj", 0, 10000, 0.8, 0.75))
	require.NoError(t, mgr.SetGlobalBudget(0, 1, 0.8, 0.75)) // 1 GB org-wide

	const twoGB = int64(2 * 1024 * 1024 * 1024)
	err = mgr.RecordOperationCost(ctx, "upload", "f", twoGB, config.StorageClassStandard, "us-east-1", "job", "proj", nil)
	require.Error(t, err, "upload over the global volume ceiling must be blocked even when the project quota allows it")
	assert.Contains(t, err.Error(), "quota")
}

// TestS3Store_GlobalBudgetRoundTrip verifies the S3 store persists and reloads
// the global budget in global.json.
func TestS3Store_GlobalBudgetRoundTrip(t *testing.T) {
	f := newFakeS3()
	s := newTestS3Store(f)
	_, tok, err := s.Load()
	require.NoError(t, err)

	require.NoError(t, s.Save(LedgerState{
		Version:      StoreVersion,
		GlobalBudget: &config.GlobalBudget{MaxBudget: 2500, MaxVolumeGB: 1000},
	}, tok))

	s2 := newTestS3Store(f)
	got, _, err := s2.Load()
	require.NoError(t, err)
	require.NotNil(t, got.GlobalBudget)
	assert.Equal(t, 2500.0, got.GlobalBudget.MaxBudget)
	assert.Equal(t, 1000.0, got.GlobalBudget.MaxVolumeGB)
}
