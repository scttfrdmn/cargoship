package cmd

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"

	cargoconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/scttfrdmn/cargoship/pkg/aws/cost"
)

func TestEnforceBudgetCaps(t *testing.T) {
	// Isolate the local budget store so SetProjectBudget doesn't touch the real one.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	dc := cargoconfig.DefaultAWSConfig()
	mgr, err := cost.NewManager(&dc.CostControl, aws.Config{}, slog.Default())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	ctx := context.Background()

	// nil manager and an unconfigured project are both no-ops.
	if err := enforceBudgetCaps(ctx, nil, "p", 1.0, "STANDARD", "us-east-1"); err != nil {
		t.Fatalf("nil mgr should pass: %v", err)
	}
	if err := enforceBudgetCaps(ctx, mgr, "p", 1.0, "STANDARD", "us-east-1"); err != nil {
		t.Fatalf("no configured cap should pass: %v", err)
	}

	// A tiny volume quota must refuse a 1 GB cycle with the typed error, before write.
	if err := mgr.SetProjectBudget("p", 1e6, 0.001, 0.8, 0.8); err != nil {
		t.Fatalf("SetProjectBudget: %v", err)
	}
	err = enforceBudgetCaps(ctx, mgr, "p", 1.0, "STANDARD", "us-east-1")
	var volErr *cost.VolumeQuotaExceededError
	if !errors.As(err, &volErr) {
		t.Fatalf("1 GB over a 0.001 GB quota should return *VolumeQuotaExceededError, got: %v", err)
	}

	// Within the quota → allowed (the $ estimate path fails open offline).
	if err := enforceBudgetCaps(ctx, mgr, "p", 0.0001, "STANDARD", "us-east-1"); err != nil {
		t.Fatalf("within quota should pass: %v", err)
	}
}
