package cost

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// On Windows, replacing budgets.json by rename fails with "Access is denied"
// while any other handle has the file open without FILE_SHARE_DELETE. Go's
// os.Open, and so every concurrent Load, opens it that way, and so do
// antivirus scanners and the search indexer. The failed rename dropped the
// save; on the best-effort ledger path that was a silently lost spend record
// (main 2d3b3c6, TestManager_ConcurrentGoroutineWriters: 5 of 6 records).
// These tests hold such a handle across a save. On Unix the rename never
// conflicts, so they pass there either way.

// holdOpen opens path the way os.Open does and closes it after d.
func holdOpen(t *testing.T, path string, d time.Duration) {
	t.Helper()
	f, err := os.Open(path) //nolint:gosec // test-owned temp path
	require.NoError(t, err)
	closed := make(chan struct{})
	go func() {
		time.Sleep(d)
		_ = f.Close()
		close(closed)
	}()
	t.Cleanup(func() { <-closed })
}

func TestLocalStore_SaveOutlastsAnOpenHandle(t *testing.T) {
	isolateBudgetStore(t)
	store := localStore{}
	require.NoError(t, store.Save(LedgerState{ProjectBudgets: map[string]config.ProjectBudget{"before": {ProjectID: "before"}}}, ""))
	path, err := budgetStorePath()
	require.NoError(t, err)

	holdOpen(t, path, 150*time.Millisecond)
	require.NoError(t, store.Save(LedgerState{ProjectBudgets: map[string]config.ProjectBudget{"after": {ProjectID: "after"}}}, ""),
		"a save must wait out another handle on the store, not fail")

	got, _, err := store.Load()
	require.NoError(t, err)
	assert.Contains(t, got.ProjectBudgets, "after")
}

// TestManager_RecordPersistsWhileTheStoreIsOpen is the production symptom: the
// ledger save is best-effort, so a failed rename lost the record silently.
func TestManager_RecordPersistsWhileTheStoreIsOpen(t *testing.T) {
	isolateBudgetStore(t)
	mgr, err := NewManager(testCostConfig(), aws.Config{}, nil)
	require.NoError(t, err)
	require.NoError(t, mgr.SetProjectBudget("p", 100, 0, 0.8, 0.75))
	path, err := budgetStorePath()
	require.NoError(t, err)

	holdOpen(t, path, 150*time.Millisecond)
	require.NoError(t, mgr.RecordCompletedUpload(context.Background(), "f", gib, config.StorageClassStandard, "us-east-1", "job-open", "p", nil))

	got, _, err := localStore{}.Load()
	require.NoError(t, err)
	require.Len(t, got.Records, 1, "the spend record must reach the store")
	assert.Equal(t, "job-open", got.Records[0].JobID)
}
