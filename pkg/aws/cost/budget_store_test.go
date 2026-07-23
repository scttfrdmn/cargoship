package cost

import (
	"path/filepath"
	"testing"

	"github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBudgetStore_SaveLoadRoundTrip(t *testing.T) {
	t.Setenv(budgetStoreEnvOverride, filepath.Join(t.TempDir(), "budgets.json"))

	want := map[string]config.ProjectBudget{
		"proj-a": {ProjectID: "proj-a", MaxBudget: 100, MaxVolumeGB: 50, AlertThreshold: 0.8},
		"proj-b": {ProjectID: "proj-b", MaxBudget: 250},
	}
	require.NoError(t, saveProjectBudgets(want))

	got, err := loadProjectBudgets()
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestBudgetStore_LoadMissingFileIsEmpty(t *testing.T) {
	t.Setenv(budgetStoreEnvOverride, filepath.Join(t.TempDir(), "does-not-exist.json"))

	got, err := loadProjectBudgets()
	require.NoError(t, err, "a missing store must not be an error")
	assert.Empty(t, got)
}

func TestBudgetStore_SaveOverwrites(t *testing.T) {
	t.Setenv(budgetStoreEnvOverride, filepath.Join(t.TempDir(), "budgets.json"))

	require.NoError(t, saveProjectBudgets(map[string]config.ProjectBudget{
		"proj-a": {ProjectID: "proj-a", MaxBudget: 100},
	}))
	// A second save with different contents must fully replace the first.
	require.NoError(t, saveProjectBudgets(map[string]config.ProjectBudget{
		"proj-b": {ProjectID: "proj-b", MaxBudget: 200},
	}))

	got, err := loadProjectBudgets()
	require.NoError(t, err)
	_, hasA := got["proj-a"]
	assert.False(t, hasA, "proj-a should have been overwritten")
	assert.Equal(t, float64(200), got["proj-b"].MaxBudget)
}
