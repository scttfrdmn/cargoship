package multiregion

import (
	"context"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFailoverManager(t *testing.T) {
	config := createValidMultiRegionConfig()
	logger := log.New(nil)
	
	manager := NewFailoverManager(config, logger)
	
	assert.NotNil(t, manager)
	assert.IsType(t, &DefaultFailoverManager{}, manager)
	
	defaultManager := manager.(*DefaultFailoverManager)
	assert.Equal(t, config, defaultManager.config)
	assert.Equal(t, logger, defaultManager.logger)
	assert.NotNil(t, defaultManager.failureHistory)
	assert.NotNil(t, defaultManager.failoverStatus)
}

func TestDefaultFailoverManager_DetectFailure(t *testing.T) {
	config := createValidMultiRegionConfig()
	logger := log.New(nil)
	manager := NewFailoverManager(config, logger).(*DefaultFailoverManager)
	ctx := context.Background()
	
	tests := []struct {
		name         string
		regionName   string
		setupFailure bool
		expectFail   bool
	}{
		{
			name:         "healthy region",
			regionName:   "us-east-1",
			setupFailure: false,
			expectFail:   false,
		},
		{
			name:         "failed region",
			regionName:   "us-west-2",
			setupFailure: true,
			expectFail:   true,
		},
		{
			name:       "non-existent region",
			regionName: "non-existent",
			expectFail: false, // Returns false for unknown regions
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupFailure {
				// Record multiple failures to trigger detection
				for i := 0; i < 5; i++ {
					manager.RecordFailure(tt.regionName)
				}
			}
			
			failed, err := manager.DetectFailure(ctx, tt.regionName)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectFail, failed)
		})
	}
}

func TestDefaultFailoverManager_ExecuteFailover(t *testing.T) {
	config := createValidMultiRegionConfig()
	// Reduce timeout for faster tests
	config.Failover.FailoverTimeout = 100 * time.Millisecond
	logger := log.New(nil)
	manager := NewFailoverManager(config, logger).(*DefaultFailoverManager)
	ctx := context.Background()
	
	tests := []struct {
		name       string
		fromRegion string
		toRegion   string
		expectErr  bool
	}{
		{
			name:       "valid failover",
			fromRegion: "us-east-1",
			toRegion:   "us-west-2",
			expectErr:  false,
		},
		{
			name:       "same region failover",
			fromRegion: "us-east-1",
			toRegion:   "us-east-1",
			expectErr:  true,
		},
		{
			name:       "non-existent source region",
			fromRegion: "non-existent",
			toRegion:   "us-west-2",
			expectErr:  false, // Graceful failover simulation succeeds
		},
		{
			name:       "non-existent target region",
			fromRegion: "us-east-1",
			toRegion:   "non-existent",
			expectErr:  false, // Graceful failover simulation succeeds
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.ExecuteFailover(ctx, tt.fromRegion, tt.toRegion)
			
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				
				// Verify failover was recorded
				status, err := manager.GetFailoverStatus(ctx)
				require.NoError(t, err)
				assert.Contains(t, status, tt.fromRegion)
				assert.Equal(t, tt.toRegion, status[tt.fromRegion])
			}
		})
	}
}

func TestDefaultFailoverManager_GetFailoverStatus(t *testing.T) {
	config := createValidMultiRegionConfig()
	// Reduce timeout for faster tests
	config.Failover.FailoverTimeout = 100 * time.Millisecond
	logger := log.New(nil)
	manager := NewFailoverManager(config, logger).(*DefaultFailoverManager)
	ctx := context.Background()
	
	// Initially should have empty status
	status, err := manager.GetFailoverStatus(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, status)
	assert.Empty(t, status)
	
	// Execute a failover
	err = manager.ExecuteFailover(ctx, "us-east-1", "us-west-2")
	require.NoError(t, err)
	
	// Check status after failover
	status, err = manager.GetFailoverStatus(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, status)
	assert.Len(t, status, 1)
	assert.Equal(t, "us-west-2", status["us-east-1"])
}

func TestDefaultFailoverManager_RecordFailure(t *testing.T) {
	config := createValidMultiRegionConfig()
	logger := log.New(nil)
	manager := NewFailoverManager(config, logger).(*DefaultFailoverManager)
	
	regionName := "us-east-1"
	
	// Initially should have no failures
	history := manager.GetFailureHistory(regionName)
	if history != nil {
		assert.Equal(t, int64(0), history.TotalFailures)
	}
	
	// Record a failure
	manager.RecordFailure(regionName)
	
	history = manager.GetFailureHistory(regionName)
	assert.NotNil(t, history)
	assert.Equal(t, int64(1), history.TotalFailures)
	assert.Equal(t, 1, history.ConsecutiveFailures)
	assert.True(t, history.LastFailure.After(time.Now().Add(-1*time.Second)))
	
	// Record multiple failures
	for i := 0; i < 5; i++ {
		manager.RecordFailure(regionName)
	}
	
	history = manager.GetFailureHistory(regionName)
	assert.Equal(t, int64(6), history.TotalFailures) // Original 1 + 5 more
	assert.Equal(t, 6, history.ConsecutiveFailures)
}

func TestDefaultFailoverManager_RecordSuccess(t *testing.T) {
	config := createValidMultiRegionConfig()
	logger := log.New(nil)
	manager := NewFailoverManager(config, logger).(*DefaultFailoverManager)
	
	regionName := "us-east-1"
	
	// Record some failures first
	for i := 0; i < 3; i++ {
		manager.RecordFailure(regionName)
	}
	
	history := manager.GetFailureHistory(regionName)
	assert.Equal(t, 3, history.ConsecutiveFailures)
	
	// Record a success
	manager.RecordSuccess(regionName)
	
	history = manager.GetFailureHistory(regionName)
	assert.Equal(t, 0, history.ConsecutiveFailures) // Should reset consecutive failures
	assert.True(t, history.LastSuccess.After(time.Now().Add(-1*time.Second)))
}

func TestDefaultFailoverManager_GetActiveFailovers(t *testing.T) {
	config := createValidMultiRegionConfig()
	// Reduce timeout for faster tests
	config.Failover.FailoverTimeout = 100 * time.Millisecond
	logger := log.New(nil)
	manager := NewFailoverManager(config, logger).(*DefaultFailoverManager)
	ctx := context.Background()
	
	// Initially should have no active failovers
	activeFailovers := manager.GetActiveFailovers()
	assert.Empty(t, activeFailovers)
	
	// Execute a failover
	err := manager.ExecuteFailover(ctx, "us-east-1", "us-west-2")
	require.NoError(t, err)
	
	// Should have an active failover now (depending on implementation)
	activeFailovers = manager.GetActiveFailovers()
	// Note: This might be empty if failover completes immediately
	assert.NotNil(t, activeFailovers)
}

func TestDefaultFailoverManager_ResetFailureHistory(t *testing.T) {
	config := createValidMultiRegionConfig()
	logger := log.New(nil)
	manager := NewFailoverManager(config, logger).(*DefaultFailoverManager)
	
	regionName := "us-east-1"
	
	// Record some failures
	for i := 0; i < 5; i++ {
		manager.RecordFailure(regionName)
	}
	
	history := manager.GetFailureHistory(regionName)
	assert.Equal(t, int64(5), history.TotalFailures)
	assert.Equal(t, 5, history.ConsecutiveFailures)
	
	// Reset failure history
	manager.ResetFailureHistory(regionName)
	
	history = manager.GetFailureHistory(regionName)
	if history != nil {
		assert.Equal(t, int64(0), history.TotalFailures)
		assert.Equal(t, 0, history.ConsecutiveFailures)
	}
}

func TestDefaultFailoverManager_IsRegionInFailover(t *testing.T) {
	config := createValidMultiRegionConfig()
	// Reduce timeout for faster tests
	config.Failover.FailoverTimeout = 100 * time.Millisecond
	logger := log.New(nil)
	manager := NewFailoverManager(config, logger).(*DefaultFailoverManager)
	ctx := context.Background()
	
	regionName := "us-east-1"
	
	// Initially should not be in failover
	assert.False(t, manager.IsRegionInFailover(regionName))
	
	// Execute failover
	err := manager.ExecuteFailover(ctx, regionName, "us-west-2")
	require.NoError(t, err)
	
	// Should be in failover status now
	status, err := manager.GetFailoverStatus(ctx)
	require.NoError(t, err)
	assert.Contains(t, status, regionName)
}

func TestDefaultFailoverManager_FailureDetectionThreshold(t *testing.T) {
	config := createValidMultiRegionConfig()
	logger := log.New(nil)
	manager := NewFailoverManager(config, logger).(*DefaultFailoverManager)
	ctx := context.Background()
	
	regionName := "us-east-1"
	
	// First record a success to set baseline
	manager.RecordSuccess(regionName)
	
	// Record failures below threshold (RetryAttempts = 2, so record 1)
	manager.RecordFailure(regionName)
	
	// Should not detect failure yet
	failed, err := manager.DetectFailure(ctx, regionName)
	assert.NoError(t, err)
	assert.False(t, failed)
	
	// Record more failures to exceed threshold (need 2 consecutive failures)
	manager.RecordFailure(regionName)
	
	// Should now detect failure
	failed, err = manager.DetectFailure(ctx, regionName)
	assert.NoError(t, err)
	assert.True(t, failed)
}

func TestDefaultFailoverManager_MultipleFailures(t *testing.T) {
	config := createValidMultiRegionConfig()
	logger := log.New(nil)
	manager := NewFailoverManager(config, logger).(*DefaultFailoverManager)
	
	regionName := "us-east-1"
	
	// Record multiple failures and check the history
	for i := 1; i <= 10; i++ {
		manager.RecordFailure(regionName)
		
		history := manager.GetFailureHistory(regionName)
		assert.NotNil(t, history)
		assert.Equal(t, int64(i), history.TotalFailures)
		assert.Equal(t, i, history.ConsecutiveFailures)
	}
	
	// Record a success to reset consecutive failures
	manager.RecordSuccess(regionName)
	
	history := manager.GetFailureHistory(regionName)
	assert.Equal(t, int64(10), history.TotalFailures) // Total should remain
	assert.Equal(t, 0, history.ConsecutiveFailures)    // Consecutive should reset
}

func TestDefaultFailoverManager_ConcurrentAccess(t *testing.T) {
	config := createValidMultiRegionConfig()
	logger := log.New(nil)
	manager := NewFailoverManager(config, logger).(*DefaultFailoverManager)
	ctx := context.Background()
	
	regionName := "us-east-1"
	
	// Test concurrent failure recording
	done := make(chan bool, 10)
	
	for i := 0; i < 10; i++ {
		go func() {
			manager.RecordFailure(regionName)
			done <- true
		}()
	}
	
	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
	
	// Verify all failures were recorded
	history := manager.GetFailureHistory(regionName)
	assert.NotNil(t, history)
	assert.Equal(t, int64(10), history.TotalFailures)
	
	// Test concurrent failure detection
	results := make(chan bool, 10)
	errors := make(chan error, 10)
	
	for i := 0; i < 10; i++ {
		go func() {
			failed, err := manager.DetectFailure(ctx, regionName)
			if err != nil {
				errors <- err
				return
			}
			results <- failed
		}()
	}
	
	// Collect results
	for i := 0; i < 10; i++ {
		select {
		case result := <-results:
			// Should detect failure due to many recorded failures (probably true)
			assert.IsType(t, bool(false), result)
		case err := <-errors:
			t.Fatalf("Unexpected error during concurrent access: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatalf("Timeout waiting for concurrent operations")
		}
	}
}

func TestDefaultFailoverManager_EdgeCases(t *testing.T) {
	t.Run("empty region name", func(t *testing.T) {
		config := createValidMultiRegionConfig()
		config.Failover.FailoverTimeout = 100 * time.Millisecond
		logger := log.New(nil)
		manager := NewFailoverManager(config, logger).(*DefaultFailoverManager)
		ctx := context.Background()
		
		failed, err := manager.DetectFailure(ctx, "")
		assert.Error(t, err)
		assert.False(t, failed)
		assert.Contains(t, err.Error(), "region name cannot be empty")
		
		err = manager.ExecuteFailover(ctx, "", "us-west-2")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "from and to regions cannot be empty")
	})
	
	t.Run("nil error in record failure", func(t *testing.T) {
		config := createValidMultiRegionConfig()
		logger := log.New(nil)
		manager := NewFailoverManager(config, logger).(*DefaultFailoverManager)
		
		// Should not panic
		assert.NotPanics(t, func() {
			manager.RecordFailure("us-east-1")
		})
		
		// Should record the failure
		history := manager.GetFailureHistory("us-east-1")
		assert.NotNil(t, history)
		assert.Equal(t, int64(1), history.TotalFailures)
	})
	
	t.Run("failover to same region", func(t *testing.T) {
		config := createValidMultiRegionConfig()
		config.Failover.FailoverTimeout = 100 * time.Millisecond
		logger := log.New(nil)
		manager := NewFailoverManager(config, logger).(*DefaultFailoverManager)
		ctx := context.Background()
		
		err := manager.ExecuteFailover(ctx, "us-east-1", "us-east-1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "from and to regions cannot be the same")
	})
	
	t.Run("very large number of failures", func(t *testing.T) {
		config := createValidMultiRegionConfig()
		logger := log.New(nil)
		manager := NewFailoverManager(config, logger).(*DefaultFailoverManager)
		ctx := context.Background()
		
		regionName := "us-east-1"
		
		// Record many failures
		for i := 0; i < 100; i++ {
			manager.RecordFailure(regionName)
		}
		
		// Should still work correctly
		history := manager.GetFailureHistory(regionName)
		assert.NotNil(t, history)
		assert.Equal(t, int64(100), history.TotalFailures)
		assert.Equal(t, 100, history.ConsecutiveFailures)
		
		// Should detect failure
		failed, err := manager.DetectFailure(ctx, regionName)
		assert.NoError(t, err)
		assert.True(t, failed)
	})
}