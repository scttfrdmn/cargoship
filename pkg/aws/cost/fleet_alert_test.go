package cost

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/scttfrdmn/cargoship/internal/testutil"
	"github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// staleWriterWebhookManager builds a Manager whose notifier delivers to a test
// webhook server, plus a way to read the last alert it received.
func staleWriterWebhookManager(t *testing.T, srvURL string, cooldown time.Duration) *Manager {
	t.Helper()
	mgr, err := NewManager(&config.CostControlConfig{}, aws.Config{}, nil)
	require.NoError(t, err)
	require.NoError(t, mgr.UpdateAlertConfig(&BudgetAlertConfig{
		Enabled:           true,
		WebhookEnabled:    true,
		WebhookURL:        srvURL,
		WebhookTimeout:    5 * time.Second,
		CooldownPeriod:    cooldown,
		SendProjectAlerts: true,
	}))
	return mgr
}

func TestNotifyStaleWriter_DeliversViaWebhook(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	var got BudgetAlert
	received := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		received <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mgr := staleWriterWebhookManager(t, server.URL, time.Hour)
	require.NoError(t, mgr.NotifyStaleWriter(context.Background(), "lab-nas-1", 3*time.Hour, 2*time.Hour))

	select {
	case <-received:
	case <-time.After(2 * time.Second):
		t.Fatal("webhook did not receive the stale-writer alert")
	}
	assert.Equal(t, AlertTypeStaleWriter, got.Type)
	assert.Equal(t, "lab-nas-1", got.ProjectID, "writer id is carried in ProjectID")
	assert.Equal(t, SeverityCritical, got.Severity)
	assert.Greater(t, got.StaleSeconds, 0.0)
	assert.True(t, got.ActionRequired)
}

func TestNotifyStaleWriter_PerWriterCooldown(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	var count int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&count, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mgr := staleWriterWebhookManager(t, server.URL, time.Hour)
	ctx := context.Background()

	// Same writer twice within the cooldown → delivered once.
	require.NoError(t, mgr.NotifyStaleWriter(ctx, "w1", 3*time.Hour, 2*time.Hour))
	require.NoError(t, mgr.NotifyStaleWriter(ctx, "w1", 4*time.Hour, 2*time.Hour))
	// A different writer is keyed separately → delivered.
	require.NoError(t, mgr.NotifyStaleWriter(ctx, "w2", 3*time.Hour, 2*time.Hour))

	assert.Equal(t, int32(2), atomic.LoadInt32(&count),
		"cooldown should suppress the second alert for w1 but not w2")
}

func TestSendCloudWatchAlert_StaleWriterIsSupported(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	notifier := NewBudgetAlertNotifier(&BudgetAlertConfig{
		Enabled:             true,
		CloudWatchEnabled:   true,
		CloudWatchNamespace: "CargoShip/Fleet",
		SendProjectAlerts:   true,
	}, aws.Config{})

	alert := &BudgetAlert{
		ID:           "stale-w1",
		Timestamp:    time.Now(),
		Type:         AlertTypeStaleWriter,
		Severity:     SeverityCritical,
		ProjectID:    "w1",
		StaleSeconds: 7200,
	}
	// Without real AWS creds the PutMetricData may fail, but the type must be
	// recognized — it must NOT be rejected as an unsupported CloudWatch alert type.
	if err := notifier.SendAlert(context.Background(), alert); err != nil {
		assert.NotContains(t, err.Error(), "unsupported alert type",
			"stale_writer must be a supported CloudWatch alert type")
	}
}

func TestMonitorBudgets_UsesInternalNotifier(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	var count int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&count, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	now := time.Now()
	mgr, err := NewManager(&config.CostControlConfig{
		ProjectBudgets: map[string]config.ProjectBudget{
			"project1": {ProjectID: "project1", MaxBudget: 100.0, AlertThreshold: 0.8},
		},
		BudgetPeriods: []config.BudgetPeriod{
			{Type: config.BudgetPeriodMonthly, MaxBudget: 1000.0, AlertThreshold: 0.8, StartDate: &now},
		},
	}, aws.Config{}, nil)
	require.NoError(t, err)
	require.NoError(t, mgr.UpdateAlertConfig(&BudgetAlertConfig{
		Enabled:           true,
		WebhookEnabled:    true,
		WebhookURL:        server.URL,
		WebhookTimeout:    5 * time.Second,
		CooldownPeriod:    time.Hour,
		SendProjectAlerts: true,
		SendGlobalAlerts:  true,
	}))

	// Drive project1 over its $100 budget.
	reporter := mgr.GetReporter()
	ctx := context.Background()
	oneTB := int64(1000 * 1024 * 1024 * 1024)
	for i := 0; i < 5; i++ {
		_ = reporter.RecordArchivalCost(ctx, "f.dat", oneTB, config.StorageClassStandard, "us-east-1", "job1", "project1", nil)
	}

	require.NoError(t, mgr.MonitorBudgets(ctx))
	assert.Positive(t, atomic.LoadInt32(&count),
		"MonitorBudgets should deliver a budget alert through the Manager's own notifier")
}
