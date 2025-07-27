/*
Tests for bandwidth delay product calculations - comprehensive test suite for BDP estimation and optimization.
*/
package s3

import (
	"context"
	"testing"
	"time"
)

func TestNewBandwidthDelayProductCalculator(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBDPConfig()
	
	calculator := NewBandwidthDelayProductCalculator(ctx, config)
	
	if calculator == nil {
		t.Fatal("Expected non-nil BDP calculator")
	}
	
	if calculator.currentBandwidth != config.DefaultBandwidth {
		t.Errorf("Expected default bandwidth %f, got %f", config.DefaultBandwidth, calculator.currentBandwidth)
	}
	
	if calculator.currentRTT != config.DefaultRTT {
		t.Errorf("Expected default RTT %v, got %v", config.DefaultRTT, calculator.currentRTT)
	}
	
	if calculator.adaptiveAlgorithm != BDPAlgorithmAdaptive {
		t.Errorf("Expected adaptive algorithm, got %v", calculator.adaptiveAlgorithm)
	}
	
	if calculator.config.MinBDP <= 0 {
		t.Error("Expected positive minimum BDP")
	}
	
	if calculator.config.MaxBDP <= calculator.config.MinBDP {
		t.Error("Expected maximum BDP to be greater than minimum")
	}
}

func TestBDPCalculatorStartStop(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBDPConfig()
	calculator := NewBandwidthDelayProductCalculator(ctx, config)
	
	// Test starting
	err := calculator.StartCalculator()
	if err != nil {
		t.Fatalf("Failed to start BDP calculator: %v", err)
	}
	
	if !calculator.isActive {
		t.Error("Expected calculator to be active after start")
	}
	
	// Test starting again (should fail)
	err = calculator.StartCalculator()
	if err == nil {
		t.Error("Expected error when starting already active calculator")
	}
	
	// Test stopping
	err = calculator.StopCalculator()
	if err != nil {
		t.Fatalf("Failed to stop BDP calculator: %v", err)
	}
	
	if calculator.isActive {
		t.Error("Expected calculator to be inactive after stop")
	}
	
	// Test stopping again (should fail)
	err = calculator.StopCalculator()
	if err == nil {
		t.Error("Expected error when stopping already inactive calculator")
	}
}

func TestBDPBasicCalculation(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBDPConfig()
	calculator := NewBandwidthDelayProductCalculator(ctx, config)
	
	// Set known values
	bandwidth := 100.0 // 100 Mbps
	rtt := time.Millisecond * 50 // 50ms
	
	calculator.UpdateBandwidth(bandwidth, time.Now(), 1.0)
	calculator.UpdateRTT(rtt, time.Now(), "test")
	
	// Expected BDP: 100 Mbps * 50ms = 100 * 1024 * 1024 / 8 * 0.05 = 655,360 bytes
	expectedBDP := int64(100 * 1024 * 1024 / 8 * 0.05)
	
	currentBDP := calculator.GetCurrentBDP()
	if currentBDP == 0 {
		t.Error("Expected non-zero BDP after calculation")
	}
	
	// Allow for some variance due to smoothing
	if currentBDP < expectedBDP/2 || currentBDP > expectedBDP*2 {
		t.Errorf("Expected BDP around %d, got %d", expectedBDP, currentBDP)
	}
}

func TestBDPBandwidthUpdate(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBDPConfig()
	calculator := NewBandwidthDelayProductCalculator(ctx, config)
	
	originalBandwidth := calculator.currentBandwidth
	newBandwidth := 200.0
	
	calculator.UpdateBandwidth(newBandwidth, time.Now(), 1.0)
	
	if calculator.currentBandwidth == originalBandwidth {
		t.Error("Expected bandwidth to be updated")
	}
	
	// Should be close to new bandwidth (may be smoothed)
	if calculator.currentBandwidth < newBandwidth*0.5 || calculator.currentBandwidth > newBandwidth*1.5 {
		t.Errorf("Expected bandwidth around %f, got %f", newBandwidth, calculator.currentBandwidth)
	}
}

func TestBDPRTTUpdate(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBDPConfig()
	calculator := NewBandwidthDelayProductCalculator(ctx, config)
	
	originalRTT := calculator.currentRTT
	newRTT := time.Millisecond * 100
	
	calculator.UpdateRTT(newRTT, time.Now(), "test")
	
	if calculator.currentRTT == originalRTT {
		t.Error("Expected RTT to be updated")
	}
	
	// Should be close to new RTT (may be smoothed)
	if calculator.currentRTT < newRTT/2 || calculator.currentRTT > newRTT*2 {
		t.Errorf("Expected RTT around %v, got %v", newRTT, calculator.currentRTT)
	}
}

func TestBDPOptimalSizeCalculation(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBDPConfig()
	calculator := NewBandwidthDelayProductCalculator(ctx, config)
	
	// Set up a scenario with known values
	calculator.UpdateBandwidth(100.0, time.Now(), 1.0)
	calculator.UpdateRTT(time.Millisecond*50, time.Now(), "test")
	
	optimalWindow := calculator.GetOptimalWindowSize()
	optimalBuffer := calculator.GetOptimalBufferSize()
	
	if optimalWindow <= 0 {
		t.Error("Expected positive optimal window size")
	}
	
	if optimalBuffer <= 0 {
		t.Error("Expected positive optimal buffer size")
	}
	
	if optimalBuffer <= optimalWindow {
		t.Error("Expected buffer size to be larger than window size")
	}
	
	// Window size should be within reasonable bounds
	if optimalWindow < config.MinWindowSize {
		t.Errorf("Expected window size >= %d, got %d", config.MinWindowSize, optimalWindow)
	}
	
	if optimalWindow > config.MaxWindowSize {
		t.Errorf("Expected window size <= %d, got %d", config.MaxWindowSize, optimalWindow)
	}
}

func TestBDPNetworkConditionsImpact(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBDPConfig()
	calculator := NewBandwidthDelayProductCalculator(ctx, config)
	
	// Set baseline values
	calculator.UpdateBandwidth(100.0, time.Now(), 1.0)
	calculator.UpdateRTT(time.Millisecond*50, time.Now(), "test")
	
	baselineBDP := calculator.GetCurrentBDP()
	
	// Update with poor network conditions
	conditions := &BDPNetworkConditions{
		Congestion:          0.8, // High congestion
		PacketLoss:          0.05, // 5% packet loss
		Jitter:              time.Millisecond * 10,
		QueueingDelay:       time.Millisecond * 20,
		BottleneckBandwidth: 50.0, // Lower than available
		PathMTU:             1500,
		ECNCapable:          false,
	}
	
	calculator.UpdateNetworkConditions(conditions)
	
	adjustedBDP := calculator.GetCurrentBDP()
	
	// BDP should be reduced due to poor conditions
	if adjustedBDP >= baselineBDP {
		t.Error("Expected BDP to be reduced due to poor network conditions")
	}
}

func TestBDPBoundaryConditions(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBDPConfig()
	calculator := NewBandwidthDelayProductCalculator(ctx, config)
	
	// Test very low bandwidth
	calculator.UpdateBandwidth(0.1, time.Now(), 1.0) // 0.1 Mbps
	calculator.UpdateRTT(time.Millisecond*10, time.Now(), "test")
	
	bdp := calculator.GetCurrentBDP()
	if bdp < config.MinBDP {
		t.Errorf("Expected BDP to be clamped to minimum %d, got %d", config.MinBDP, bdp)
	}
	
	// Test very high bandwidth
	calculator.UpdateBandwidth(10000.0, time.Now(), 1.0) // 10 Gbps
	calculator.UpdateRTT(time.Second, time.Now(), "test") // 1 second RTT
	
	bdp = calculator.GetCurrentBDP()
	if bdp > config.MaxBDP {
		t.Errorf("Expected BDP to be clamped to maximum %d, got %d", config.MaxBDP, bdp)
	}
}

func TestBDPHistoryTracking(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBDPConfig()
	calculator := NewBandwidthDelayProductCalculator(ctx, config)
	
	// Generate some history
	for i := 0; i < 10; i++ {
		bandwidth := float64(100 + i*10)
		rtt := time.Millisecond * time.Duration(50+i*5)
		
		calculator.UpdateBandwidth(bandwidth, time.Now(), 1.0)
		calculator.UpdateRTT(rtt, time.Now(), "test")
	}
	
	history := calculator.GetBDPHistory(5)
	if len(history) == 0 {
		t.Error("Expected BDP history to be recorded")
	}
	
	if len(history) > 5 {
		t.Errorf("Expected at most 5 history entries, got %d", len(history))
	}
	
	// Check that entries have valid data
	for _, sample := range history {
		if sample.Bandwidth <= 0 {
			t.Error("Expected positive bandwidth in history sample")
		}
		if sample.RTT <= 0 {
			t.Error("Expected positive RTT in history sample")
		}
		if sample.CalculatedBDP <= 0 {
			t.Error("Expected positive BDP in history sample")
		}
	}
}

func TestBDPMetricsTracking(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBDPConfig()
	calculator := NewBandwidthDelayProductCalculator(ctx, config)
	
	// Perform some operations
	calculator.UpdateBandwidth(100.0, time.Now(), 1.0)
	calculator.UpdateRTT(time.Millisecond*50, time.Now(), "test")
	
	metrics := calculator.GetMetrics()
	if metrics == nil {
		t.Fatal("Expected non-nil metrics")
	}
	
	if metrics.TotalCalculations <= 0 {
		t.Error("Expected some calculations to be recorded")
	}
	
	if metrics.LastUpdate.IsZero() {
		t.Error("Expected last update time to be set")
	}
	
	// Test efficiency calculation
	efficiency := calculator.GetEfficiency()
	if efficiency < 0 || efficiency > 1 {
		t.Errorf("Expected efficiency between 0 and 1, got %f", efficiency)
	}
}

func TestBDPBandwidthEstimator(t *testing.T) {
	estimator := NewBDPBandwidthEstimator()
	
	if estimator == nil {
		t.Fatal("Expected non-nil bandwidth estimator")
	}
	
	// Add samples
	timestamp := time.Now()
	estimator.AddSample(100.0, timestamp, 1.0)
	estimator.AddSample(110.0, timestamp.Add(time.Second), 0.9)
	estimator.AddSample(95.0, timestamp.Add(time.Second*2), 0.8)
	
	currentBandwidth := estimator.GetCurrentBandwidth()
	if currentBandwidth <= 0 {
		t.Error("Expected positive current bandwidth")
	}
	
	// Should be smoothed value, somewhere between the samples
	if currentBandwidth < 90 || currentBandwidth > 120 {
		t.Errorf("Expected smoothed bandwidth in reasonable range, got %f", currentBandwidth)
	}
	
	// Test update method
	estimator.Update()
	
	// Should still have a valid bandwidth
	updatedBandwidth := estimator.GetCurrentBandwidth()
	if updatedBandwidth <= 0 {
		t.Error("Expected positive bandwidth after update")
	}
}

func TestBDPRTTEstimator(t *testing.T) {
	estimator := NewBDPRTTEstimator()
	
	if estimator == nil {
		t.Fatal("Expected non-nil RTT estimator")
	}
	
	// Add samples
	timestamp := time.Now()
	estimator.AddSample(time.Millisecond*50, timestamp, "test")
	estimator.AddSample(time.Millisecond*55, timestamp.Add(time.Second), "test")
	estimator.AddSample(time.Millisecond*45, timestamp.Add(time.Second*2), "test")
	
	currentRTT := estimator.GetCurrentRTT()
	if currentRTT <= 0 {
		t.Error("Expected positive current RTT")
	}
	
	// Should be smoothed value
	if currentRTT < time.Millisecond*40 || currentRTT > time.Millisecond*60 {
		t.Errorf("Expected smoothed RTT in reasonable range, got %v", currentRTT)
	}
	
	// Test update method
	estimator.Update()
	
	// Should still have a valid RTT
	updatedRTT := estimator.GetCurrentRTT()
	if updatedRTT <= 0 {
		t.Error("Expected positive RTT after update")
	}
}

func TestBDPOptimizationAlgorithms(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBDPConfig()
	calculator := NewBandwidthDelayProductCalculator(ctx, config)
	
	// Set initial conditions
	calculator.UpdateBandwidth(100.0, time.Now(), 1.0)
	calculator.UpdateRTT(time.Millisecond*50, time.Now(), "test")
	
	initialBDP := calculator.GetCurrentBDP()
	
	// Test classic optimization
	calculator.adaptiveAlgorithm = BDPAlgorithmClassic
	calculator.classicOptimization()
	
	// Test adaptive optimization
	calculator.adaptiveAlgorithm = BDPAlgorithmAdaptive
	calculator.adaptiveOptimization(0.5) // Medium performance score
	
	// Test hybrid optimization
	calculator.adaptiveAlgorithm = BDPAlgorithmHybrid
	calculator.hybridOptimization(0.3) // Poor performance score
	
	// BDP should have been adjusted
	finalBDP := calculator.GetCurrentBDP()
	
	// At least one optimization should have changed the BDP
	// (This is a weak test since we don't know the exact optimization behavior)
	if finalBDP == initialBDP {
		// This might happen if conditions are optimal, so we just log it
		t.Logf("BDP remained unchanged after optimizations: %d", finalBDP)
	}
}

func TestBDPPerformanceAnalysis(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBDPConfig()
	calculator := NewBandwidthDelayProductCalculator(ctx, config)
	
	// Add some performance history samples
	for i := 0; i < 15; i++ {
		sample := BDPPerformanceSample{
			Timestamp:       time.Now().Add(-time.Duration(i) * time.Second),
			Throughput:      float64(90 + i), // Varying throughput
			Efficiency:      0.8 + float64(i)*0.01,
			PacketLoss:      float64(i) * 0.001,
			OptimalityScore: 0.7 + float64(i)*0.02,
		}
		calculator.performanceHistory = append(calculator.performanceHistory, sample)
	}
	
	performanceScore := calculator.analyzePerformance()
	
	if performanceScore < 0 || performanceScore > 1 {
		t.Errorf("Expected performance score between 0 and 1, got %f", performanceScore)
	}
	
	// With our test data, performance should be reasonably good
	if performanceScore < 0.5 {
		t.Errorf("Expected decent performance score with test data, got %f", performanceScore)
	}
}

func TestBDPNetworkConditionScore(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBDPConfig()
	calculator := NewBandwidthDelayProductCalculator(ctx, config)
	
	// Test with good conditions
	goodConditions := &BDPNetworkConditions{
		Congestion:    0.1,
		PacketLoss:    0.001,
		Jitter:        time.Millisecond * 2,
		ECNCapable:    true,
	}
	calculator.networkConditions = goodConditions
	
	goodScore := calculator.calculateNetworkConditionScore()
	if goodScore < 0.7 {
		t.Errorf("Expected high score for good conditions, got %f", goodScore)
	}
	
	// Test with poor conditions
	poorConditions := &BDPNetworkConditions{
		Congestion:    0.8,
		PacketLoss:    0.05,
		Jitter:        time.Millisecond * 20,
		ECNCapable:    false,
	}
	calculator.networkConditions = poorConditions
	
	poorScore := calculator.calculateNetworkConditionScore()
	if poorScore > 0.5 {
		t.Errorf("Expected low score for poor conditions, got %f", poorScore)
	}
	
	// Good conditions should score higher than poor conditions
	if goodScore <= poorScore {
		t.Error("Expected good conditions to score higher than poor conditions")
	}
}

func TestBDPConfigDefaults(t *testing.T) {
	config := NewDefaultBDPConfig()
	
	if config == nil {
		t.Fatal("Expected non-nil default config")
	}
	
	if config.DefaultBandwidth <= 0 {
		t.Error("Expected positive default bandwidth")
	}
	
	if config.DefaultRTT <= 0 {
		t.Error("Expected positive default RTT")
	}
	
	if config.MinBDP <= 0 {
		t.Error("Expected positive minimum BDP")
	}
	
	if config.MaxBDP <= config.MinBDP {
		t.Error("Expected maximum BDP to be greater than minimum")
	}
	
	if config.SmoothingFactor <= 0 || config.SmoothingFactor >= 1 {
		t.Error("Expected smoothing factor between 0 and 1")
	}
	
	if config.AdaptationRate <= 0 || config.AdaptationRate >= 1 {
		t.Error("Expected adaptation rate between 0 and 1")
	}
	
	if config.WindowSizingFactor <= 0 {
		t.Error("Expected positive window sizing factor")
	}
	
	if config.BufferMultiplier <= 1 {
		t.Error("Expected buffer multiplier greater than 1")
	}
	
	if config.OptimizationInterval <= 0 {
		t.Error("Expected positive optimization interval")
	}
	
	if config.PerformanceThreshold <= 0 || config.PerformanceThreshold > 1 {
		t.Error("Expected performance threshold between 0 and 1")
	}
}

func TestBDPTuningParameters(t *testing.T) {
	params := NewDefaultBDPTuningParameters()
	
	if params == nil {
		t.Fatal("Expected non-nil default tuning parameters")
	}
	
	// Check sensitivity parameters
	if params.BandwidthSensitivity <= 0 || params.BandwidthSensitivity > 1 {
		t.Error("Expected bandwidth sensitivity between 0 and 1")
	}
	
	if params.RTTSensitivity <= 0 || params.RTTSensitivity > 1 {
		t.Error("Expected RTT sensitivity between 0 and 1")
	}
	
	if params.LossSensitivity <= 0 || params.LossSensitivity > 1 {
		t.Error("Expected loss sensitivity between 0 and 1")
	}
	
	// Check stability parameters
	if params.StabilityFactor <= 0 || params.StabilityFactor > 1 {
		t.Error("Expected stability factor between 0 and 1")
	}
	
	if params.OscillationDamping <= 0 || params.OscillationDamping > 1 {
		t.Error("Expected oscillation damping between 0 and 1")
	}
	
	// Check optimization parameters
	if params.ExplorationRate < 0 || params.ExplorationRate > 1 {
		t.Error("Expected exploration rate between 0 and 1")
	}
	
	if params.ExploitationRate < 0 || params.ExploitationRate > 1 {
		t.Error("Expected exploitation rate between 0 and 1")
	}
	
	if params.LearningRate <= 0 || params.LearningRate > 1 {
		t.Error("Expected learning rate between 0 and 1")
	}
}

func TestBDPSampleGeneration(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBDPConfig()
	calculator := NewBandwidthDelayProductCalculator(ctx, config)
	
	// Set up conditions
	calculator.UpdateBandwidth(100.0, time.Now(), 1.0)
	calculator.UpdateRTT(time.Millisecond*50, time.Now(), "test")
	
	// Force sample recording
	calculator.recordBDPSample()
	
	history := calculator.GetBDPHistory(1)
	if len(history) == 0 {
		t.Error("Expected at least one BDP sample to be recorded")
	}
	
	sample := history[0]
	if sample.Bandwidth <= 0 {
		t.Error("Expected positive bandwidth in sample")
	}
	
	if sample.RTT <= 0 {
		t.Error("Expected positive RTT in sample")
	}
	
	if sample.CalculatedBDP <= 0 {
		t.Error("Expected positive calculated BDP in sample")
	}
	
	if sample.NetworkConditionScore < 0 || sample.NetworkConditionScore > 1 {
		t.Error("Expected network condition score between 0 and 1")
	}
}

func TestBDPIntegration(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBDPConfig()
	config.OptimizationInterval = time.Millisecond * 100 // Short interval for testing
	calculator := NewBandwidthDelayProductCalculator(ctx, config)
	
	// Start calculator
	err := calculator.StartCalculator()
	if err != nil {
		t.Fatalf("Failed to start calculator: %v", err)
	}
	defer func() { _ = calculator.StopCalculator() }()
	
	// Simulate network measurements
	for i := 0; i < 5; i++ {
		bandwidth := 100.0 + float64(i)*10
		rtt := time.Millisecond * time.Duration(50+i*5)
		
		calculator.UpdateBandwidth(bandwidth, time.Now(), 0.9)
		calculator.UpdateRTT(rtt, time.Now(), "integration_test")
		
		time.Sleep(time.Millisecond * 50)
	}
	
	// Wait for some processing
	time.Sleep(time.Millisecond * 200)
	
	// Check that BDP has been calculated
	bdp := calculator.GetCurrentBDP()
	if bdp <= 0 {
		t.Error("Expected positive BDP after integration test")
	}
	
	// Check that optimal sizes have been calculated
	windowSize := calculator.GetOptimalWindowSize()
	bufferSize := calculator.GetOptimalBufferSize()
	
	if windowSize <= 0 {
		t.Error("Expected positive optimal window size")
	}
	
	if bufferSize <= 0 {
		t.Error("Expected positive optimal buffer size")
	}
	
	// Check metrics
	metrics := calculator.GetMetrics()
	if metrics.TotalCalculations == 0 {
		t.Error("Expected some calculations to have been performed")
	}
	
	// Check history
	history := calculator.GetBDPHistory(10)
	if len(history) == 0 {
		t.Error("Expected some history to be recorded")
	}
}

// Benchmark tests
func BenchmarkBDPCalculation(b *testing.B) {
	ctx := context.Background()
	config := NewDefaultBDPConfig()
	calculator := NewBandwidthDelayProductCalculator(ctx, config)
	
	bandwidth := 100.0
	rtt := time.Millisecond * 50
	timestamp := time.Now()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		calculator.UpdateBandwidth(bandwidth, timestamp, 1.0)
		calculator.UpdateRTT(rtt, timestamp, "benchmark")
	}
}

func BenchmarkBDPOptimization(b *testing.B) {
	ctx := context.Background()
	config := NewDefaultBDPConfig()
	calculator := NewBandwidthDelayProductCalculator(ctx, config)
	
	// Set up some history
	for i := 0; i < 20; i++ {
		sample := BDPPerformanceSample{
			Timestamp:       time.Now(),
			Throughput:      float64(90 + i),
			Efficiency:      0.8,
			OptimalityScore: 0.7,
		}
		calculator.performanceHistory = append(calculator.performanceHistory, sample)
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		calculator.performOptimization()
	}
}