package cost

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testCostConfig returns a minimal CostControlConfig that uses the static
// fallback pricing table (no AWS Pricing API calls) so tests are hermetic.
func testCostConfig() *config.CostControlConfig {
	return &config.CostControlConfig{
		Pricing: config.PricingConfig{
			UseAWSPricingAPI:     false,
			Currency:             "USD",
			PricingCacheDuration: "24h",
		},
	}
}

func TestLedgerStore_RoundTrip(t *testing.T) {
	t.Setenv(budgetStoreEnvOverride, filepath.Join(t.TempDir(), "budgets.json"))

	want := LedgerState{
		Version: StoreVersion,
		ProjectBudgets: map[string]config.ProjectBudget{
			"proj-a": {ProjectID: "proj-a", MaxBudget: 100, MaxVolumeGB: 50},
		},
		Records: []CostRecord{
			{Timestamp: time.Unix(1000, 0).UTC(), Operation: "upload", ProjectID: "proj-a", Cost: 1.5, SizeGB: 10},
		},
	}
	store := localStore{}
	require.NoError(t, store.Save(want, ""))

	got, tok, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, Token(""), tok, "local store uses no concurrency token")
	assert.Equal(t, StoreVersion, got.Version)
	assert.Equal(t, want.ProjectBudgets, got.ProjectBudgets)
	require.Len(t, got.Records, 1)
	assert.Equal(t, 1.5, got.Records[0].Cost)
}

func TestLedgerStore_MissingFileIsEmpty(t *testing.T) {
	t.Setenv(budgetStoreEnvOverride, filepath.Join(t.TempDir(), "nope.json"))

	got, _, err := localStore{}.Load()
	require.NoError(t, err)
	assert.Equal(t, StoreVersion, got.Version)
	assert.Empty(t, got.ProjectBudgets)
	assert.Empty(t, got.Records)
}

// TestLedgerStore_MigratesLegacyFormat writes a pre-#246 bare-map file and
// asserts Load migrates it to the versioned shape without losing limits, then
// the next Save rewrites it in the new format.
func TestLedgerStore_MigratesLegacyFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "budgets.json")
	t.Setenv(budgetStoreEnvOverride, path)

	// Legacy on-disk shape: a bare map[string]ProjectBudget, no "version" key.
	legacy := map[string]config.ProjectBudget{
		"proj-a": {ProjectID: "proj-a", MaxBudget: 100},
		"proj-b": {ProjectID: "proj-b", MaxBudget: 250, MaxVolumeGB: 30},
	}
	data, err := json.MarshalIndent(legacy, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0600))

	store := localStore{}
	got, _, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, StoreVersion, got.Version, "legacy file migrated to current version")
	assert.Equal(t, legacy, got.ProjectBudgets, "limits preserved through migration")
	assert.Empty(t, got.Records)

	// Persisting rewrites the file in the new versioned shape.
	require.NoError(t, store.Save(got, ""))
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var probe map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &probe))
	_, hasVersion := probe["version"]
	assert.True(t, hasVersion, "rewritten file must carry a version field")
}

// TestManager_RehydratesLedger seeds a store with records and asserts a freshly
// constructed Manager surfaces the persisted spend/volume.
func TestManager_RehydratesLedger(t *testing.T) {
	t.Setenv(budgetStoreEnvOverride, filepath.Join(t.TempDir(), "budgets.json"))

	seed := LedgerState{
		Version:        StoreVersion,
		ProjectBudgets: map[string]config.ProjectBudget{"proj-x": {ProjectID: "proj-x", MaxBudget: 100}},
		Records: []CostRecord{
			{Timestamp: time.Unix(1, 0).UTC(), Operation: "upload", ProjectID: "proj-x", Cost: 2.0, SizeGB: 5},
			{Timestamp: time.Unix(2, 0).UTC(), Operation: "upload", ProjectID: "proj-x", Cost: 3.0, SizeGB: 7},
		},
	}
	require.NoError(t, localStore{}.Save(seed, ""))

	mgr, err := NewManager(testCostConfig(), aws.Config{}, nil)
	require.NoError(t, err)
	assert.InDelta(t, 5.0, mgr.reporter.GetProjectCosts("proj-x"), 1e-9)
	assert.InDelta(t, 12.0, mgr.reporter.GetProjectVolume("proj-x"), 1e-9)
}

// TestManager_LedgerSurvivesSetProjectBudget guards the clobber trap: recording
// spend then setting a budget for another project must not drop the ledger.
func TestManager_LedgerSurvivesSetProjectBudget(t *testing.T) {
	t.Setenv(budgetStoreEnvOverride, filepath.Join(t.TempDir(), "budgets.json"))

	// Seed a record for proj-x directly, then persist via a Manager.
	require.NoError(t, localStore{}.Save(LedgerState{
		Version: StoreVersion,
		Records: []CostRecord{{Timestamp: time.Unix(1, 0).UTC(), ProjectID: "proj-x", Cost: 4.0, SizeGB: 8}},
	}, ""))

	mgr, err := NewManager(testCostConfig(), aws.Config{}, nil)
	require.NoError(t, err)
	// Set a budget for a DIFFERENT project — must preserve proj-x's records.
	require.NoError(t, mgr.SetProjectBudget("proj-y", 500, 0, 0.8, 0.75))

	got, _, err := localStore{}.Load()
	require.NoError(t, err)
	require.Len(t, got.Records, 1, "recorded ledger must survive a budget set")
	assert.Equal(t, "proj-x", got.Records[0].ProjectID)
	_, hasY := got.ProjectBudgets["proj-y"]
	assert.True(t, hasY, "new budget must be persisted alongside the ledger")
}

// TestManager_TwoProcessDurability proves spend recorded by one Manager is
// visible to a fresh Manager reading the same store — the core #246 promise.
func TestManager_TwoProcessDurability(t *testing.T) {
	t.Setenv(budgetStoreEnvOverride, filepath.Join(t.TempDir(), "budgets.json"))
	ctx := context.Background()

	// "Process 1": record a completed upload for proj-x.
	mgrA, err := NewManager(testCostConfig(), aws.Config{}, nil)
	require.NoError(t, err)
	const sizeBytes = int64(20 * 1024 * 1024 * 1024) // 20 GB
	require.NoError(t, mgrA.RecordCompletedUpload(ctx, "upload-1", sizeBytes, config.StorageClassStandard, "us-east-1", "upload-1", "proj-x", nil))
	spentA := mgrA.reporter.GetProjectCosts("proj-x")
	require.Greater(t, spentA, 0.0, "recording should produce non-zero spend")

	// "Process 2": a fresh Manager reloads from disk and sees the same spend.
	mgrB, err := NewManager(testCostConfig(), aws.Config{}, nil)
	require.NoError(t, err)
	assert.InDelta(t, spentA, mgrB.reporter.GetProjectCosts("proj-x"), 1e-9,
		"spend must be durable across Manager instances (process restarts)")
	assert.InDelta(t, 20.0, mgrB.reporter.GetProjectVolume("proj-x"), 1e-6)
}

// TestManager_LedgerRotation asserts the persisted ledger is capped (FIFO).
func TestManager_LedgerRotation(t *testing.T) {
	t.Setenv(budgetStoreEnvOverride, filepath.Join(t.TempDir(), "budgets.json"))

	// Seed just over the cap with monotonically increasing markers.
	records := make([]CostRecord, maxLedgerRecords+5)
	for i := range records {
		records[i] = CostRecord{Timestamp: time.Unix(int64(i), 0).UTC(), ProjectID: "p", Cost: float64(i)}
	}
	require.NoError(t, localStore{}.Save(LedgerState{Version: StoreVersion, Records: records}, ""))

	mgr, err := NewManager(testCostConfig(), aws.Config{}, nil)
	require.NoError(t, err)
	// Trigger a save (which applies rotation) via SetProjectBudget.
	require.NoError(t, mgr.SetProjectBudget("p", 1, 0, 0.8, 0.75))

	got, _, err := localStore{}.Load()
	require.NoError(t, err)
	assert.LessOrEqual(t, len(got.Records), maxLedgerRecords, "ledger must be capped")
	// The newest record (highest marker) must be retained; the oldest dropped.
	last := got.Records[len(got.Records)-1]
	assert.Equal(t, float64(maxLedgerRecords+4), last.Cost, "newest record retained after FIFO rotation")
}
