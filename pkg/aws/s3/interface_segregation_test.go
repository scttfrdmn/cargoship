package s3

import (
	"context"
	"testing"
	
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInterfaceSegregation(t *testing.T) {
	ctx := context.Background()
	
	// Create the concrete implementation
	config := DefaultCoordinationConfig()
	fullController := NewGlobalCongestionController(config)
	fullController.Start(ctx)
	
	t.Run("BasicCongestionController", func(t *testing.T) {
		// Verify that we can treat it as a BasicCongestionController
		var basicController BasicCongestionController = fullController
		
		// Test core functionality
		basicController.RegisterPrefix("test-prefix", 100.0)
		
		upload := &ScheduledUpload{
			ArchivePath:   "test.tar",
			PrefixID:      "test-prefix",
			Priority:      3,
			EstimatedSize: 1024 * 1024,
		}
		
		allocation, err := basicController.AllocateResources(upload)
		require.NoError(t, err)
		assert.NotNil(t, allocation)
		
		metrics := basicController.GetMetrics()
		assert.NotNil(t, metrics)
	})
	
	t.Run("CongestionMetricsProvider", func(t *testing.T) {
		// Verify that we can treat it as just a metrics provider
		var metricsProvider CongestionMetricsProvider = fullController
		
		metrics := metricsProvider.GetMetrics()
		assert.NotNil(t, metrics)
		
		enhancedMetrics := metricsProvider.GetEnhancedMetrics()
		assert.NotNil(t, enhancedMetrics)
	})
	
	t.Run("AdvancedCongestionController", func(t *testing.T) {
		// Verify that we can treat it as an advanced controller
		var advancedController AdvancedCongestionController = fullController
		
		// Can use all basic + advanced methods
		advancedController.RegisterPrefix("advanced-prefix", 200.0)
		
		upload := &ScheduledUpload{
			ArchivePath:   "advanced.tar",
			PrefixID:      "advanced-prefix",
			Priority:      5,
			EstimatedSize: 2 * 1024 * 1024,
		}
		
		allocation, err := advancedController.AllocateResources(upload)
		require.NoError(t, err)
		assert.NotNil(t, allocation)
		
		metrics := advancedController.GetMetrics()
		assert.NotNil(t, metrics)
	})
	
	t.Run("StatisticsAnalyzer", func(t *testing.T) {
		// Verify that we can treat it as just a statistics analyzer
		var statsAnalyzer StatisticsAnalyzer = fullController
		
		// Test statistics methods
		utilization := statsAnalyzer.calculateAverageUtilization()
		assert.GreaterOrEqual(t, utilization, 0.0)
		
		totalUtilization := statsAnalyzer.calculateTotalUtilization()
		assert.GreaterOrEqual(t, totalUtilization, 0.0)
		
		fairnessIndex := statsAnalyzer.calculateFairnessIndex()
		assert.GreaterOrEqual(t, fairnessIndex, 0.0)
		assert.LessOrEqual(t, fairnessIndex, 1.0)
	})
}

func TestInterfaceSegregationFactory(t *testing.T) {
	ctx := context.Background()
	factory := &InterfaceSegregatedFactory{}
	
	components, err := factory.CreateComponents(ctx)
	require.NoError(t, err)
	assert.NotNil(t, components)
	
	// Verify all components are created
	assert.NotNil(t, components.SimpleManager)
	assert.NotNil(t, components.Monitor)
	assert.NotNil(t, components.Optimizer)
	assert.NotNil(t, components.BandwidthOptimizer)
	assert.NotNil(t, components.PrefixCoordinator)
	assert.NotNil(t, components.CongestionPerformanceAnalyzer)
	
	// Test integrated functionality
	upload := &ScheduledUpload{
		ArchivePath:   "integrated.tar",
		PrefixID:      "integrated-prefix", // Add prefix ID
		Priority:      3,
		EstimatedSize: 1024 * 1024,
	}
	
	err = components.ProcessTransfer(ctx, upload)
	assert.NoError(t, err)
}

func TestComponentFactoryInterfaceSegregation(t *testing.T) {
	ctx := context.Background()
	factory := NewComponentFactory()
	
	t.Run("BasicCongestionController", func(t *testing.T) {
		basicController := factory.CreateBasicCongestionController(ctx)
		assert.NotNil(t, basicController)
		
		// Verify it implements the basic interface
		var _ BasicCongestionController = basicController
		
		// Test basic functionality
		basicController.RegisterPrefix("basic-test", 50.0)
		metrics := basicController.GetMetrics()
		assert.NotNil(t, metrics)
	})
	
	t.Run("AdvancedCongestionController", func(t *testing.T) {
		advancedController := factory.CreateAdvancedCongestionController(ctx)
		assert.NotNil(t, advancedController)
		
		// Verify it implements the advanced interface
		var _ AdvancedCongestionController = advancedController
		
		// Test advanced functionality
		advancedController.RegisterPrefix("advanced-test", 100.0)
		metrics := advancedController.GetMetrics()
		assert.NotNil(t, metrics)
	})
	
	t.Run("MetricsOnlyCongestionController", func(t *testing.T) {
		metricsController := factory.CreateMetricsOnlyCongestionController(ctx)
		assert.NotNil(t, metricsController)
		
		// Verify it implements the metrics interface
		var _ CongestionMetricsProvider = metricsController
		
		// Test metrics functionality
		metrics := metricsController.GetMetrics()
		assert.NotNil(t, metrics)
		
		enhancedMetrics := metricsController.GetEnhancedMetrics()
		assert.NotNil(t, enhancedMetrics)
	})
}

// Example test showing the benefits of interface segregation
func TestInterfaceSegregationBenefits(t *testing.T) {
	ctx := context.Background()
	config := DefaultCoordinationConfig()
	fullController := NewGlobalCongestionController(config)
	fullController.Start(ctx)
	
	// Before ISP: Components would depend on the full 58-method interface
	// After ISP: Components depend only on what they need
	
	t.Run("ComponentWithMinimalNeeds", func(t *testing.T) {
		// This component only needs metrics
		component := NewCongestionMonitor(fullController)
		
		// It can only access metrics methods, not all 58 methods
		metrics := component.GetCurrentMetrics()
		assert.NotNil(t, metrics)
		
		// This enforces the interface segregation principle:
		// "No client should be forced to depend on methods it does not use"
	})
	
	t.Run("ComponentWithSpecificNeeds", func(t *testing.T) {
		// This component needs basic congestion control
		component := NewSimpleUploadManager(fullController)
		
		// First register a prefix
		var basicController BasicCongestionController = fullController
		basicController.RegisterPrefix("specific-prefix", 100.0)
		
		upload := &ScheduledUpload{
			ArchivePath:   "specific.tar",
			PrefixID:      "specific-prefix",
			Priority:      2,
			EstimatedSize: 512 * 1024,
		}
		
		// It can access only the methods it needs for its functionality
		err := component.ProcessUpload(upload)
		assert.NoError(t, err)
	})
}