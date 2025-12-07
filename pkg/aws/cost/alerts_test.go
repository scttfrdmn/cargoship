package cost

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/scttfrdmn/cargoship/internal/testutil"
	"github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBudgetAlertNotifier(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	cfg := DefaultBudgetAlertConfig()
	notifier := NewBudgetAlertNotifier(cfg, aws.Config{})

	assert.NotNil(t, notifier)
	assert.Equal(t, cfg, notifier.config)
	assert.NotNil(t, notifier.httpClient)
	assert.NotNil(t, notifier.lastAlertTimes)
}

func TestDefaultBudgetAlertConfig(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	cfg := DefaultBudgetAlertConfig()

	assert.True(t, cfg.Enabled)
	assert.Equal(t, 24*time.Hour, cfg.CooldownPeriod)
	assert.False(t, cfg.WebhookEnabled)
	assert.False(t, cfg.CloudWatchEnabled)
	assert.Equal(t, "CargoShip/Budget", cfg.CloudWatchNamespace)
	assert.True(t, cfg.SendProjectAlerts)
	assert.True(t, cfg.SendGlobalAlerts)
}

func TestSendWebhookAlert(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	// Create test HTTP server
	receivedAlert := &BudgetAlert{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method
		assert.Equal(t, "POST", r.Method)

		// Verify content type
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Verify custom header
		assert.Equal(t, "test-value", r.Header.Get("X-Test-Header"))

		// Read and parse request body
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		err = json.Unmarshal(body, receivedAlert)
		require.NoError(t, err)

		// Send success response
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create notifier with webhook enabled
	cfg := &BudgetAlertConfig{
		Enabled:           true,
		WebhookEnabled:    true,
		WebhookURL:        server.URL,
		WebhookHeaders:    map[string]string{
			"X-Test-Header": "test-value",
		},
		WebhookTimeout:    10 * time.Second,
		SendProjectAlerts: true, // Enable project alerts
		SendGlobalAlerts:  true,
	}
	notifier := NewBudgetAlertNotifier(cfg, aws.Config{})

	// Create test alert
	alert := &BudgetAlert{
		ID:                "test-alert-1",
		Timestamp:         time.Now(),
		Type:              AlertTypeCostThreshold,
		Severity:          SeverityWarning,
		ProjectID:         "project1",
		Description:       "Test alert",
		MaxBudget:         1000.0,
		CurrentSpend:      850.0,
		BudgetUsedPercent: 85.0,
	}

	// Send alert
	ctx := context.Background()
	err := notifier.SendAlert(ctx, alert)
	require.NoError(t, err)

	// Verify received alert
	assert.Equal(t, alert.ID, receivedAlert.ID)
	assert.Equal(t, alert.Type, receivedAlert.Type)
	assert.Equal(t, alert.Severity, receivedAlert.Severity)
	assert.Equal(t, alert.ProjectID, receivedAlert.ProjectID)
}

func TestSendWebhookAlertFailure(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	// Create test HTTP server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	// Create notifier with webhook enabled
	cfg := &BudgetAlertConfig{
		Enabled:           true,
		WebhookEnabled:    true,
		WebhookURL:        server.URL,
		WebhookTimeout:    10 * time.Second,
		SendProjectAlerts: true, // Enable project alerts
		SendGlobalAlerts:  true,
	}
	notifier := NewBudgetAlertNotifier(cfg, aws.Config{})

	// Create test alert (with ProjectID to avoid being filtered as global)
	alert := &BudgetAlert{
		ID:        "test-alert-2",
		Timestamp: time.Now(),
		Type:      AlertTypeCostThreshold,
		Severity:  SeverityWarning,
		ProjectID: "project1", // Add project ID
	}

	// Send alert (should fail)
	ctx := context.Background()
	err := notifier.SendAlert(ctx, alert)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "webhook returned non-success status")
}

func TestAlertCooldownPeriod(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	// Create test HTTP server
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create notifier with short cooldown period
	cfg := &BudgetAlertConfig{
		Enabled:           true,
		WebhookEnabled:    true,
		WebhookURL:        server.URL,
		WebhookTimeout:    10 * time.Second,
		CooldownPeriod:    100 * time.Millisecond, // Short for testing
		SendProjectAlerts: true,
	}
	notifier := NewBudgetAlertNotifier(cfg, aws.Config{})

	// Create test alert
	alert := &BudgetAlert{
		ID:        "test-alert-3",
		Timestamp: time.Now(),
		Type:      AlertTypeCostThreshold,
		Severity:  SeverityWarning,
		ProjectID: "project1",
	}

	ctx := context.Background()

	// Send first alert (should succeed)
	err := notifier.SendAlert(ctx, alert)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)

	// Send second alert immediately (should be skipped due to cooldown)
	err = notifier.SendAlert(ctx, alert)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount, "Second alert should be skipped due to cooldown")

	// Wait for cooldown period to expire
	time.Sleep(150 * time.Millisecond)

	// Send third alert (should succeed after cooldown)
	err = notifier.SendAlert(ctx, alert)
	require.NoError(t, err)
	assert.Equal(t, 2, callCount, "Third alert should succeed after cooldown")
}

func TestCheckBudgetStatusCostOverBudget(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	now := time.Now()
	cfg := &config.CostControlConfig{
		BudgetPeriods: []config.BudgetPeriod{
			{
				Type:           config.BudgetPeriodMonthly,
				MaxBudget:      1000.0,
				AlertThreshold: 0.8,
				StartDate:      &now,
			},
		},
		ActiveBudgetPeriodIndex: 0,
		ProjectBudgets: map[string]config.ProjectBudget{
			"project1": {
				ProjectID:      "project1",
				MaxBudget:      500.0,
				AlertThreshold: 0.8,
			},
		},
	}
	manager, err := NewManager(cfg, aws.Config{}, nil)
	require.NoError(t, err)

	// Record costs that exceed budget ($500 budget)
	// S3 Standard = ~$0.023/GB, so we need ~22,000 GB to exceed $500
	// Use 10 files of 2,500 GB each (2.5 TB) = 25,000 GB total ≈ $575
	reporter := manager.GetReporter()
	ctx := context.Background()
	fileSizeBytes := int64(2500 * 1024 * 1024 * 1024) // 2.5 TB per file
	for i := 0; i < 10; i++ {
		_ = reporter.RecordArchivalCost(ctx, "file.dat", fileSizeBytes, config.StorageClassStandard, "us-east-1", "job1", "project1", nil)
	}

	// Check budget status
	alert, err := manager.CheckBudgetStatus(ctx, "project1")
	require.NoError(t, err)
	require.NotNil(t, alert)

	// Verify alert
	assert.Equal(t, AlertTypeCostOverBudget, alert.Type)
	assert.Equal(t, SeverityCritical, alert.Severity)
	assert.Equal(t, "project1", alert.ProjectID)
	assert.True(t, alert.ActionRequired)
	assert.Equal(t, 500.0, alert.MaxBudget)
	assert.Greater(t, alert.CurrentSpend, 500.0)
}

func TestCheckBudgetStatusCostThreshold(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	now := time.Now()
	cfg := &config.CostControlConfig{
		BudgetPeriods: []config.BudgetPeriod{
			{
				Type:           config.BudgetPeriodMonthly,
				MaxBudget:      1000.0,
				AlertThreshold: 0.8,
				StartDate:      &now,
			},
		},
		ActiveBudgetPeriodIndex: 0,
		ProjectBudgets: map[string]config.ProjectBudget{
			"project1": {
				ProjectID:      "project1",
				MaxBudget:      500.0,
				AlertThreshold: 0.8,
			},
		},
	}
	manager, err := NewManager(cfg, aws.Config{}, nil)
	require.NoError(t, err)

	// Record costs that reach alert threshold (85% of $500 = $425)
	// S3 Standard = ~$0.023/GB, so we need ~18,500 GB to reach $425
	// Use 8 files of 2,300 GB each = 18,400 GB total ≈ $423
	reporter := manager.GetReporter()
	ctx := context.Background()
	fileSizeBytes := int64(2300 * 1024 * 1024 * 1024) // 2.3 TB per file
	for i := 0; i < 8; i++ {
		_ = reporter.RecordArchivalCost(ctx, "file.dat", fileSizeBytes, config.StorageClassStandard, "us-east-1", "job1", "project1", nil)
	}

	// Check budget status
	alert, err := manager.CheckBudgetStatus(ctx, "project1")
	require.NoError(t, err)
	require.NotNil(t, alert)

	// Verify alert
	assert.Equal(t, AlertTypeCostThreshold, alert.Type)
	assert.Equal(t, SeverityWarning, alert.Severity)
	assert.Equal(t, "project1", alert.ProjectID)
	assert.False(t, alert.ActionRequired)
	assert.Equal(t, 500.0, alert.MaxBudget)
	assert.Greater(t, alert.CurrentSpend, 400.0)
	assert.Less(t, alert.CurrentSpend, 500.0)
}

func TestCheckBudgetStatusVolumeOverQuota(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	now := time.Now()
	cfg := &config.CostControlConfig{
		BudgetPeriods: []config.BudgetPeriod{
			{
				Type:           config.BudgetPeriodMonthly,
				MaxBudget:      10000.0,
				MaxVolumeGB:    1000.0,
				AlertThreshold: 0.8,
				StartDate:      &now,
			},
		},
		ActiveBudgetPeriodIndex: 0,
		ProjectBudgets: map[string]config.ProjectBudget{
			"project1": {
				ProjectID:            "project1",
				MaxBudget:            5000.0,
				MaxVolumeGB:          100.0,
				AlertThreshold:       0.8,
				VolumeAlertThreshold: 0.75,
			},
		},
	}
	manager, err := NewManager(cfg, aws.Config{}, nil)
	require.NoError(t, err)

	// Record archival that exceeds volume quota (150 GB > 100 GB)
	reporter := manager.GetReporter()
	ctx := context.Background()
	_ = reporter.RecordArchivalCost(ctx, "file.dat", 150*1024*1024*1024, config.StorageClassStandard, "us-east-1", "job1", "project1", nil) // 150 GB

	// Check budget status
	alert, err := manager.CheckBudgetStatus(ctx, "project1")
	require.NoError(t, err)
	require.NotNil(t, alert)

	// Verify alert
	assert.Equal(t, AlertTypeVolumeOverQuota, alert.Type)
	assert.Equal(t, SeverityCritical, alert.Severity)
	assert.Equal(t, "project1", alert.ProjectID)
	assert.True(t, alert.ActionRequired)
	assert.Equal(t, 100.0, alert.MaxVolumeGB)
	assert.Greater(t, alert.CurrentVolumeGB, 100.0)
}

func TestCheckBudgetStatusVolumeThreshold(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	now := time.Now()
	cfg := &config.CostControlConfig{
		BudgetPeriods: []config.BudgetPeriod{
			{
				Type:           config.BudgetPeriodMonthly,
				MaxBudget:      10000.0,
				MaxVolumeGB:    1000.0,
				AlertThreshold: 0.8,
				StartDate:      &now,
			},
		},
		ActiveBudgetPeriodIndex: 0,
		ProjectBudgets: map[string]config.ProjectBudget{
			"project1": {
				ProjectID:            "project1",
				MaxBudget:            5000.0,
				MaxVolumeGB:          100.0,
				AlertThreshold:       0.8,
				VolumeAlertThreshold: 0.75,
			},
		},
	}
	manager, err := NewManager(cfg, aws.Config{}, nil)
	require.NoError(t, err)

	// Record archival that reaches volume alert threshold (80 GB > 75% of 100 GB)
	reporter := manager.GetReporter()
	ctx := context.Background()
	_ = reporter.RecordArchivalCost(ctx, "file.dat", 80*1024*1024*1024, config.StorageClassStandard, "us-east-1", "job1", "project1", nil) // 80 GB

	// Check budget status
	alert, err := manager.CheckBudgetStatus(ctx, "project1")
	require.NoError(t, err)
	require.NotNil(t, alert)

	// Verify alert
	assert.Equal(t, AlertTypeVolumeThreshold, alert.Type)
	assert.Equal(t, SeverityWarning, alert.Severity)
	assert.Equal(t, "project1", alert.ProjectID)
	assert.False(t, alert.ActionRequired)
	assert.Equal(t, 100.0, alert.MaxVolumeGB)
	assert.Greater(t, alert.CurrentVolumeGB, 75.0)
	assert.Less(t, alert.CurrentVolumeGB, 100.0)
}

func TestCheckBudgetStatusNoAlert(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	now := time.Now()
	cfg := &config.CostControlConfig{
		BudgetPeriods: []config.BudgetPeriod{
			{
				Type:           config.BudgetPeriodMonthly,
				MaxBudget:      1000.0,
				AlertThreshold: 0.8,
				StartDate:      &now,
			},
		},
		ActiveBudgetPeriodIndex: 0,
		ProjectBudgets: map[string]config.ProjectBudget{
			"project1": {
				ProjectID:      "project1",
				MaxBudget:      500.0,
				AlertThreshold: 0.8,
			},
		},
	}
	manager, err := NewManager(cfg, aws.Config{}, nil)
	require.NoError(t, err)

	// Record costs well below threshold
	reporter := manager.GetReporter()
	ctx := context.Background()
	_ = reporter.RecordArchivalCost(ctx, "file.dat", 100, config.StorageClassStandard, "us-east-1", "job1", "project1", nil)

	// Check budget status
	alert, err := manager.CheckBudgetStatus(ctx, "project1")
	require.NoError(t, err)
	assert.Nil(t, alert, "No alert should be triggered when budget is healthy")
}

func TestCheckAndNotifyBudgetStatus(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	// Create test HTTP server
	alertReceived := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		alertReceived = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create manager with budget that will trigger alert
	now := time.Now()
	cfg := &config.CostControlConfig{
		BudgetPeriods: []config.BudgetPeriod{
			{
				Type:           config.BudgetPeriodMonthly,
				MaxBudget:      1000.0,
				AlertThreshold: 0.8,
				StartDate:      &now,
			},
		},
		ActiveBudgetPeriodIndex: 0,
		ProjectBudgets: map[string]config.ProjectBudget{
			"project1": {
				ProjectID:      "project1",
				MaxBudget:      100.0,
				AlertThreshold: 0.8,
			},
		},
	}
	manager, err := NewManager(cfg, aws.Config{}, nil)
	require.NoError(t, err)

	// Record costs that exceed budget ($100 budget)
	// S3 Standard = ~$0.023/GB, so we need ~4,500 GB to exceed $100
	// Use 5 files of 1,000 GB each = 5,000 GB total ≈ $115
	reporter := manager.GetReporter()
	ctx := context.Background()
	fileSizeBytes := int64(1000 * 1024 * 1024 * 1024) // 1 TB per file
	for i := 0; i < 5; i++ {
		_ = reporter.RecordArchivalCost(ctx, "file.dat", fileSizeBytes, config.StorageClassStandard, "us-east-1", "job1", "project1", nil)
	}

	// Create notifier
	notifierCfg := &BudgetAlertConfig{
		Enabled:           true,
		WebhookEnabled:    true,
		WebhookURL:        server.URL,
		WebhookTimeout:    10 * time.Second,
		CooldownPeriod:    time.Hour,
		SendProjectAlerts: true,
	}
	notifier := NewBudgetAlertNotifier(notifierCfg, aws.Config{})

	// Check and notify
	err = manager.CheckAndNotifyBudgetStatus(ctx, "project1", notifier)
	require.NoError(t, err)

	// Verify alert was sent
	assert.True(t, alertReceived, "Webhook should have received alert")
}

func TestMonitorAllBudgets(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	// Create test HTTP server
	alertCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		alertCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create manager with multiple projects
	now := time.Now()
	cfg := &config.CostControlConfig{
		BudgetPeriods: []config.BudgetPeriod{
			{
				Type:           config.BudgetPeriodMonthly,
				MaxBudget:      1000.0,
				AlertThreshold: 0.8,
				StartDate:      &now,
			},
		},
		ActiveBudgetPeriodIndex: 0,
		ProjectBudgets: map[string]config.ProjectBudget{
			"project1": {
				ProjectID:      "project1",
				MaxBudget:      100.0,
				AlertThreshold: 0.8,
			},
			"project2": {
				ProjectID:      "project2",
				MaxBudget:      200.0,
				AlertThreshold: 0.8,
			},
		},
	}
	manager, err := NewManager(cfg, aws.Config{}, nil)
	require.NoError(t, err)

	// Record costs that exceed budget for both projects
	// Project1: $100 budget, need ~4,500 GB
	// Project2: $200 budget, need ~9,000 GB
	reporter := manager.GetReporter()
	ctx := context.Background()
	fileSizeBytes1 := int64(1000 * 1024 * 1024 * 1024) // 1 TB per file
	for i := 0; i < 5; i++ {
		_ = reporter.RecordArchivalCost(ctx, "file1.dat", fileSizeBytes1, config.StorageClassStandard, "us-east-1", "job1", "project1", nil)
	}
	fileSizeBytes2 := int64(2000 * 1024 * 1024 * 1024) // 2 TB per file
	for i := 0; i < 5; i++ {
		_ = reporter.RecordArchivalCost(ctx, "file2.dat", fileSizeBytes2, config.StorageClassStandard, "us-east-1", "job2", "project2", nil)
	}

	// Create notifier
	notifierCfg := &BudgetAlertConfig{
		Enabled:           true,
		WebhookEnabled:    true,
		WebhookURL:        server.URL,
		WebhookTimeout:    10 * time.Second,
		CooldownPeriod:    time.Hour,
		SendProjectAlerts: true,
		SendGlobalAlerts:  false, // Disable global to only count project alerts
	}
	notifier := NewBudgetAlertNotifier(notifierCfg, aws.Config{})

	// Monitor all budgets
	err = manager.MonitorAllBudgets(ctx, notifier)
	require.NoError(t, err)

	// Verify alerts were sent for both projects
	assert.Equal(t, 2, alertCount, "Should have received alerts for both projects")
}
