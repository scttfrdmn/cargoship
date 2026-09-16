package cmd

import (
	"context"

	cargoconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/scttfrdmn/cargoship/pkg/aws/cost"
)

// enforceBudgetCaps is the pre-write cost/volume gate (#629): it refuses an upload or
// sync cycle that would exceed a configured volume quota or cost budget for projectID,
// returning the typed *cost.VolumeQuotaExceededError / *cost.BudgetExceededError so the
// caller can abort BEFORE any bytes move.
//
// It is a no-op when mgr is nil or no cap is configured for projectID (the check
// functions fall through to the global cap, then to "allow"). The volume quota is
// enforced exactly; the $ budget is best-effort — if the cost estimate fails (e.g. a
// pricing lookup is unavailable) it fails OPEN rather than blocking a backup, since
// volume is the exact gate.
func enforceBudgetCaps(ctx context.Context, mgr *cost.Manager, projectID string, sizeGB float64, sc cargoconfig.StorageClass, region string) error {
	if mgr == nil {
		return nil
	}
	if err := mgr.CheckProjectVolumeQuota(projectID, sizeGB); err != nil {
		return err
	}
	if est, err := mgr.EstimateOperationCost(ctx, "upload", sizeGB, sc, region); err == nil && est != nil {
		if berr := mgr.CheckProjectBudget(projectID, est.TotalCost); berr != nil {
			return berr
		}
	}
	return nil
}
