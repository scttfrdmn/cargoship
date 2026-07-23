package cmd

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/pipeline"
)

// TestRecordUploadCost_PersistsAcrossManagers verifies the #246 upload hook:
// recordUploadCost writes a completed upload's spend to the budget store, and a
// freshly loaded cost manager sees it (durable across "processes").
func TestRecordUploadCost_PersistsAcrossManagers(t *testing.T) {
	t.Setenv("CARGOSHIP_BUDGET_STORE", filepath.Join(t.TempDir(), "budgets.json"))
	ctx := context.Background()

	cmd := &cobra.Command{}
	result := &pipeline.Result{
		UploadID:   "20260101-abc123",
		TotalBytes: 10 * 1024 * 1024 * 1024, // 10 GB
		TotalFiles: 3,
		Success:    true,
	}
	recordUploadCost(ctx, cmd, "proj-record", "STANDARD", "us-east-1", result, nil)

	// A fresh cost manager must see the persisted spend.
	mgr, err := loadCostManager(ctx)
	require.NoError(t, err)
	spend := mgr.GetReporter().GetProjectCosts("proj-record")
	assert.Greater(t, spend, 0.0, "recorded upload spend must persist and reload")
	assert.InDelta(t, 10.0, mgr.GetReporter().GetProjectVolume("proj-record"), 1e-6)
}
