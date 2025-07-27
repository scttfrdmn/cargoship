/*
Tests for BBR bandwidth probing - comprehensive test suite for Google's BBR-style congestion control.
*/
package s3

import (
	"context"
	"testing"
	"time"
)

func TestNewBBRBandwidthProber(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBBRConfig()
	
	prober := NewBBRBandwidthProber(ctx, config)
	
	if prober == nil {
		t.Fatal("Expected non-nil BBR bandwidth prober")
	}
	
	if prober.state != BBRStateStartup {
		t.Errorf("Expected initial state to be startup, got %v", prober.state)
	}
	
	if prober.mode != BBRProbingModeStartup {
		t.Errorf("Expected initial mode to be startup, got %v", prober.mode)
	}
	
	if prober.maxBandwidth != 10.0 {
		t.Errorf("Expected initial bandwidth of 10.0 Mbps, got %f", prober.maxBandwidth)
	}
	
	if prober.minRTT != time.Millisecond*100 {
		t.Errorf("Expected initial RTT of 100ms, got %v", prober.minRTT)
	}
	
	if prober.congestionWindow != config.InitialCongestionWindow {
		t.Errorf("Expected initial congestion window of %d, got %d", config.InitialCongestionWindow, prober.congestionWindow)
	}
}

func TestBBRBandwidthProberStartStop(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBBRConfig()
	prober := NewBBRBandwidthProber(ctx, config)
	
	// Test starting
	err := prober.StartProbing()
	if err != nil {
		t.Fatalf("Failed to start BBR probing: %v", err)
	}
	
	if !prober.isActive {
		t.Error("Expected prober to be active after start")
	}
	
	// Test starting again (should fail)
	err = prober.StartProbing()
	if err == nil {
		t.Error("Expected error when starting already active prober")
	}
	
	// Test stopping
	err = prober.StopProbing()
	if err != nil {
		t.Fatalf("Failed to stop BBR probing: %v", err)
	}
	
	if prober.isActive {
		t.Error("Expected prober to be inactive after stop")
	}
	
	// Test stopping again (should fail)
	err = prober.StopProbing()
	if err == nil {
		t.Error("Expected error when stopping already inactive prober")
	}
}

func TestBBRPacketSentTracking(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBBRConfig()
	prober := NewBBRBandwidthProber(ctx, config)
	
	sendTime := time.Now()
	packetSize := int64(1024)
	
	// Test first packet sent
	prober.OnPacketSent(packetSize, sendTime)
	
	if prober.firstSentTime.IsZero() {
		t.Error("Expected first sent time to be set")
	}
	
	if !prober.firstSentTime.Equal(sendTime) {
		t.Errorf("Expected first sent time to be %v, got %v", sendTime, prober.firstSentTime)
	}
	
	// Test app-limited tracking
	prober.appLimited = true
	originalDelivered := prober.deliveredBytes
	
	prober.OnPacketSent(packetSize, sendTime.Add(time.Millisecond*10))
	
	expectedAppLimitedUntil := originalDelivered + packetSize
	if prober.appLimitedUntil != expectedAppLimitedUntil {
		t.Errorf("Expected app limited until %d, got %d", expectedAppLimitedUntil, prober.appLimitedUntil)
	}
}

func TestBBRPacketAcknowledged(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBBRConfig()
	prober := NewBBRBandwidthProber(ctx, config)
	
	sendTime := time.Now()
	ackTime := sendTime.Add(time.Millisecond * 50) // 50ms RTT
	packetSize := int64(1024)
	rtt := ackTime.Sub(sendTime)
	
	originalDelivered := prober.deliveredBytes
	originalSampleCount := len(prober.bandwidthSamples)
	
	prober.OnPacketAcknowledged(packetSize, sendTime, ackTime, rtt)
	
	// Check delivered bytes updated
	if prober.deliveredBytes != originalDelivered+packetSize {
		t.Errorf("Expected delivered bytes to be %d, got %d", originalDelivered+packetSize, prober.deliveredBytes)
	}
	
	// Check delivered time updated
	if !prober.deliveredTime.Equal(ackTime) {
		t.Errorf("Expected delivered time to be %v, got %v", ackTime, prober.deliveredTime)
	}
	
	// Check bandwidth sample recorded
	if len(prober.bandwidthSamples) != originalSampleCount+1 {
		t.Errorf("Expected %d bandwidth samples, got %d", originalSampleCount+1, len(prober.bandwidthSamples))
	}
	
	// Check RTT sample recorded
	if len(prober.rttSamples) == 0 {
		t.Error("Expected RTT sample to be recorded")
	}
	
	// Verify sample content
	if len(prober.bandwidthSamples) > 0 {
		sample := prober.bandwidthSamples[len(prober.bandwidthSamples)-1]
		if sample.BytesDelivered != packetSize {
			t.Errorf("Expected sample bytes delivered to be %d, got %d", packetSize, sample.BytesDelivered)
		}
		if sample.RTT != rtt {
			t.Errorf("Expected sample RTT to be %v, got %v", rtt, sample.RTT)
		}
	}
}

func TestBBRPacketLoss(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBBRConfig()
	prober := NewBBRBandwidthProber(ctx, config)
	
	sendTime := time.Now()
	packetSize := int64(1024)
	
	originalLossCount := prober.consecutiveLossCount
	originalLossEvents := prober.metrics.TotalLossEvents
	
	prober.OnPacketLost(packetSize, sendTime)
	
	// Check loss count incremented
	if prober.consecutiveLossCount != originalLossCount+1 {
		t.Errorf("Expected consecutive loss count to be %d, got %d", originalLossCount+1, prober.consecutiveLossCount)
	}
	
	// Test multiple losses triggering loss handling
	for i := 0; i < int(config.LossThreshold)+1; i++ {
		prober.OnPacketLost(packetSize, sendTime)
	}
	
	// Should have triggered loss event handling
	if prober.metrics.TotalLossEvents <= originalLossEvents {
		t.Error("Expected loss event to be recorded after threshold exceeded")
	}
}

func TestBBRStateTransitions(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBBRConfig()
	prober := NewBBRBandwidthProber(ctx, config)
	
	// Test startup to drain transition
	originalState := prober.state
	
	// Simulate conditions for exiting startup
	prober.bandwidthSamples = []BBRBandwidthSample{
		{DeliveryRate: 10.0, Timestamp: time.Now()},
		{DeliveryRate: 12.0, Timestamp: time.Now()},
		{DeliveryRate: 12.1, Timestamp: time.Now()}, // Small growth
	}
	
	if prober.shouldExitStartup() {
		prober.transitionToState(BBRStateDrain)
		
		if prober.state != BBRStateDrain {
			t.Errorf("Expected state to be drain after transition, got %v", prober.state)
		}
		
		if prober.state == originalState {
			t.Error("Expected state to change from original")
		}
		
		// Check metrics updated
		if prober.metrics.StateTransitions[BBRStateDrain] == 0 {
			t.Error("Expected drain state transition to be recorded in metrics")
		}
	}
}

func TestBBRBandwidthEstimation(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBBRConfig()
	prober := NewBBRBandwidthProber(ctx, config)
	
	// Create bandwidth sample
	sample := BBRBandwidthSample{
		Timestamp:      time.Now(),
		DeliveryRate:   50.0, // 50 Mbps
		BytesDelivered: 1024,
		RTT:            time.Millisecond * 30,
		IsAppLimited:   false,
	}
	
	originalMaxBandwidth := prober.maxBandwidth
	
	prober.updateBandwidthEstimate(sample)
	
	// Should update max bandwidth if higher
	if sample.DeliveryRate > originalMaxBandwidth {
		if prober.maxBandwidth != sample.DeliveryRate {
			t.Errorf("Expected max bandwidth to be updated to %f, got %f", sample.DeliveryRate, prober.maxBandwidth)
		}
	}
	
	// Test app-limited sample (should not update max)
	appLimitedSample := sample
	appLimitedSample.IsAppLimited = true
	appLimitedSample.DeliveryRate = 100.0 // Much higher
	
	currentMax := prober.maxBandwidth
	prober.updateBandwidthEstimate(appLimitedSample)
	
	if prober.maxBandwidth != currentMax {
		t.Error("Expected app-limited sample to not update max bandwidth")
	}
}

func TestBBRRTTEstimation(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBBRConfig()
	prober := NewBBRBandwidthProber(ctx, config)
	
	timestamp := time.Now()
	rtt := time.Millisecond * 30 // 30ms
	
	originalMinRTT := prober.minRTT
	originalSampleCount := len(prober.rttSamples)
	
	prober.updateRTTEstimate(rtt, timestamp)
	
	// Should update min RTT if lower
	if rtt < originalMinRTT {
		if prober.minRTT != rtt {
			t.Errorf("Expected min RTT to be updated to %v, got %v", rtt, prober.minRTT)
		}
	}
	
	// Check RTT sample recorded
	if len(prober.rttSamples) != originalSampleCount+1 {
		t.Errorf("Expected %d RTT samples, got %d", originalSampleCount+1, len(prober.rttSamples))
	}
	
	// Verify sample content
	if len(prober.rttSamples) > 0 {
		sample := prober.rttSamples[len(prober.rttSamples)-1]
		if sample.RTT != rtt {
			t.Errorf("Expected sample RTT to be %v, got %v", rtt, sample.RTT)
		}
		if !sample.IsValid {
			t.Error("Expected RTT sample to be valid")
		}
	}
}

func TestBBRPacingRateCalculation(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBBRConfig()
	prober := NewBBRBandwidthProber(ctx, config)
	
	// Set test values
	prober.maxBandwidth = 100.0 // 100 Mbps
	prober.pacingGain = 1.25    // 25% gain
	
	_ = prober.pacingRate // originalPacingRate for potential future use
	
	prober.updatePacingRate()
	
	// Calculate expected rate
	expectedRate := prober.maxBandwidth * prober.pacingGain
	margin := 1.0 + (config.PacingMarginPercent / 100.0)
	expectedRate *= margin
	
	if prober.pacingRate != expectedRate {
		t.Errorf("Expected pacing rate to be %f, got %f", expectedRate, prober.pacingRate)
	}
	
	// Test rate limiting
	prober.maxBandwidth = 10000.0 // Very high
	prober.updatePacingRate()
	
	if prober.pacingRate > config.MaxPacingRate {
		t.Errorf("Expected pacing rate to be capped at %f, got %f", config.MaxPacingRate, prober.pacingRate)
	}
	
	// Test minimum rate
	prober.maxBandwidth = 0.1 // Very low
	prober.updatePacingRate()
	
	if prober.pacingRate < config.MinPacingRate {
		t.Errorf("Expected pacing rate to be at least %f, got %f", config.MinPacingRate, prober.pacingRate)
	}
}

func TestBBRCongestionWindowCalculation(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBBRConfig()
	prober := NewBBRBandwidthProber(ctx, config)
	
	// Set test values
	prober.maxBandwidth = 100.0 // 100 Mbps
	prober.minRTT = time.Millisecond * 50 // 50ms
	prober.cwndGain = 2.0
	
	_ = prober.congestionWindow // originalCwnd for potential future use
	
	prober.updateCongestionWindow()
	
	// Calculate expected BDP and congestion window
	expectedBDP := prober.calculateBDP()
	expectedCwnd := int64(float64(expectedBDP) * prober.cwndGain)
	
	if expectedCwnd < prober.sendQuantum {
		expectedCwnd = prober.sendQuantum
	}
	if expectedCwnd < config.MinCongestionWindow {
		expectedCwnd = config.MinCongestionWindow
	}
	if expectedCwnd > config.MaxCongestionWindow {
		expectedCwnd = config.MaxCongestionWindow
	}
	
	if prober.congestionWindow != expectedCwnd {
		t.Errorf("Expected congestion window to be %d, got %d", expectedCwnd, prober.congestionWindow)
	}
}

func TestBBRBDPCalculation(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBBRConfig()
	prober := NewBBRBandwidthProber(ctx, config)
	
	// Set test values
	prober.maxBandwidth = 100.0 // 100 Mbps
	prober.minRTT = time.Millisecond * 50 // 50ms
	
	bdp := prober.calculateBDP()
	
	// Calculate expected BDP
	bandwidthBps := prober.maxBandwidth * 1024 * 1024 / 8 // Convert Mbps to Bps
	rttSeconds := prober.minRTT.Seconds()
	expectedBDP := int64(bandwidthBps * rttSeconds)
	
	if bdp != expectedBDP {
		t.Errorf("Expected BDP to be %d, got %d", expectedBDP, bdp)
	}
	
	// Test with different values
	prober.maxBandwidth = 50.0
	prober.minRTT = time.Millisecond * 100
	
	newBDP := prober.calculateBDP()
	
	if newBDP == bdp {
		t.Error("Expected BDP to change with different bandwidth and RTT")
	}
}

func TestBBRDeliveryRateCalculation(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBBRConfig()
	prober := NewBBRBandwidthProber(ctx, config)
	
	sendTime := time.Now()
	ackTime := sendTime.Add(time.Millisecond * 50) // 50ms
	packetSize := int64(1024) // 1KB
	
	deliveryRate := prober.calculateDeliveryRate(packetSize, sendTime, ackTime)
	
	// Calculate expected rate
	duration := ackTime.Sub(sendTime)
	expectedBps := float64(packetSize*8) / duration.Seconds()
	expectedMbps := expectedBps / (1024 * 1024)
	
	if deliveryRate != expectedMbps {
		t.Errorf("Expected delivery rate to be %f Mbps, got %f", expectedMbps, deliveryRate)
	}
	
	// Test with zero duration
	zeroRate := prober.calculateDeliveryRate(packetSize, sendTime, sendTime)
	if zeroRate != 0.0 {
		t.Errorf("Expected zero delivery rate for zero duration, got %f", zeroRate)
	}
}

func TestBBRSmoothingCalculations(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBBRConfig()
	prober := NewBBRBandwidthProber(ctx, config)
	
	// Test smoothed RTT calculation
	currentRTT := time.Millisecond * 50
	
	// First RTT (no history)
	smoothedRTT := prober.calculateSmoothedRTT(currentRTT)
	if smoothedRTT != currentRTT {
		t.Errorf("Expected first smoothed RTT to equal current RTT, got %v", smoothedRTT)
	}
	
	// Add a sample and test smoothing
	prober.rttSamples = append(prober.rttSamples, RTTSample{
		SmoothedRTT: time.Millisecond * 40,
		RTTVariance: time.Millisecond * 5,
	})
	
	newRTT := time.Millisecond * 60
	newSmoothedRTT := prober.calculateSmoothedRTT(newRTT)
	
	// Should be between old smoothed and new RTT
	if newSmoothedRTT <= time.Millisecond*40 || newSmoothedRTT >= newRTT {
		t.Errorf("Expected smoothed RTT to be between old and new values, got %v", newSmoothedRTT)
	}
	
	// Test RTT variance calculation
	variance := prober.calculateRTTVariance(newRTT)
	if variance <= 0 {
		t.Error("Expected positive RTT variance")
	}
}

func TestBBRAppLimitedDetection(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBBRConfig()
	prober := NewBBRBandwidthProber(ctx, config)
	
	// Test not app limited initially
	if prober.isAppLimited() {
		t.Error("Expected not app limited initially")
	}
	
	// Set app limited state
	prober.appLimited = true
	prober.deliveredBytes = 1000
	prober.appLimitedUntil = 2000
	
	if !prober.isAppLimited() {
		t.Error("Expected to be app limited")
	}
	
	// Advance delivered bytes beyond limit
	prober.deliveredBytes = 2500
	
	if prober.isAppLimited() {
		t.Error("Expected not to be app limited after exceeding limit")
	}
}

func TestBBRMetricsTracking(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBBRConfig()
	prober := NewBBRBandwidthProber(ctx, config)
	
	// Test initial metrics
	metrics := prober.GetMetrics()
	if metrics == nil {
		t.Fatal("Expected non-nil metrics")
	}
	
	if metrics.TotalProbes < 0 {
		t.Error("Expected non-negative total probes")
	}
	
	// Test metrics update
	originalProbes := metrics.TotalProbes
	
	prober.updateMetrics()
	
	newMetrics := prober.GetMetrics()
	if newMetrics.TotalProbes <= originalProbes {
		t.Error("Expected total probes to increase after update")
	}
	
	// Test state transition tracking
	prober.transitionToState(BBRStateDrain)
	
	updatedMetrics := prober.GetMetrics()
	if updatedMetrics.StateTransitions[BBRStateDrain] == 0 {
		t.Error("Expected drain state transition to be recorded")
	}
}

func TestBBRConfigDefaults(t *testing.T) {
	config := NewDefaultBBRConfig()
	
	if config == nil {
		t.Fatal("Expected non-nil default config")
	}
	
	if config.HighGain <= 1.0 {
		t.Error("Expected high gain to be greater than 1.0")
	}
	
	if config.DrainGain >= 1.0 {
		t.Error("Expected drain gain to be less than 1.0")
	}
	
	if config.StartupThreshold <= 1.0 {
		t.Error("Expected startup threshold to be greater than 1.0")
	}
	
	if config.ProbeRTTDuration <= 0 {
		t.Error("Expected positive probe RTT duration")
	}
	
	if len(config.ProbeBWGainCycle) == 0 {
		t.Error("Expected non-empty probe bandwidth gain cycle")
	}
	
	if config.InitialCongestionWindow <= 0 {
		t.Error("Expected positive initial congestion window")
	}
}

func TestBandwidthMaxFilter(t *testing.T) {
	windowLength := time.Second * 5
	filter := NewBandwidthMaxFilter(windowLength)
	
	if filter == nil {
		t.Fatal("Expected non-nil bandwidth max filter")
	}
	
	// Test initial state
	if filter.GetMax() != 0.0 {
		t.Error("Expected initial max to be 0.0")
	}
	
	// Add samples
	now := time.Now()
	filter.Update(10.0, now)
	filter.Update(15.0, now.Add(time.Second))
	filter.Update(12.0, now.Add(time.Second*2))
	
	// Should track maximum
	if filter.GetMax() != 15.0 {
		t.Errorf("Expected max to be 15.0, got %f", filter.GetMax())
	}
	
	// Test window expiry
	filter.Update(8.0, now.Add(time.Second*10)) // Outside window
	
	// Old samples should be removed, new max should be 8.0
	if filter.GetMax() != 8.0 {
		t.Errorf("Expected max to be 8.0 after window expiry, got %f", filter.GetMax())
	}
}

func TestRTTMinFilter(t *testing.T) {
	windowLength := time.Second * 5
	filter := NewRTTMinFilter(windowLength)
	
	if filter == nil {
		t.Fatal("Expected non-nil RTT min filter")
	}
	
	// Test initial state (should be very high)
	if filter.GetMin() < time.Minute {
		t.Error("Expected initial min to be very high")
	}
	
	// Add samples
	now := time.Now()
	filter.Update(time.Millisecond*100, now)
	filter.Update(time.Millisecond*50, now.Add(time.Second))
	filter.Update(time.Millisecond*75, now.Add(time.Second*2))
	
	// Should track minimum
	if filter.GetMin() != time.Millisecond*50 {
		t.Errorf("Expected min to be 50ms, got %v", filter.GetMin())
	}
	
	// Test window expiry
	filter.Update(time.Millisecond*200, now.Add(time.Second*10)) // Outside window
	
	// Old samples should be removed, new min should be 200ms
	if filter.GetMin() != time.Millisecond*200 {
		t.Errorf("Expected min to be 200ms after window expiry, got %v", filter.GetMin())
	}
}

func TestBBRProberIntegration(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultBBRConfig()
	prober := NewBBRBandwidthProber(ctx, config)
	
	// Start probing
	err := prober.StartProbing()
	if err != nil {
		t.Fatalf("Failed to start probing: %v", err)
	}
	defer func() {
		_ = prober.StopProbing()
	}()
	
	// Simulate packet flow
	sendTime := time.Now()
	packetSize := int64(1024)
	
	// Send packet
	prober.OnPacketSent(packetSize, sendTime)
	
	// ACK packet
	ackTime := sendTime.Add(time.Millisecond * 50)
	rtt := ackTime.Sub(sendTime)
	prober.OnPacketAcknowledged(packetSize, sendTime, ackTime, rtt)
	
	// Check state updates
	if len(prober.bandwidthSamples) == 0 {
		t.Error("Expected bandwidth samples to be recorded")
	}
	
	if len(prober.rttSamples) == 0 {
		t.Error("Expected RTT samples to be recorded")
	}
	
	// Get current estimates
	bandwidth := prober.GetCurrentBandwidth()
	currentRTT := prober.GetCurrentRTT()
	pacingRate := prober.GetPacingRate()
	cwnd := prober.GetCongestionWindow()
	
	if bandwidth <= 0 {
		t.Error("Expected positive bandwidth estimate")
	}
	
	if currentRTT <= 0 {
		t.Error("Expected positive RTT estimate")
	}
	
	if pacingRate <= 0 {
		t.Error("Expected positive pacing rate")
	}
	
	if cwnd <= 0 {
		t.Error("Expected positive congestion window")
	}
	
	// Test metrics
	metrics := prober.GetMetrics()
	if metrics.TotalProbes == 0 {
		t.Error("Expected some probes to be recorded")
	}
}

// Benchmark tests
func BenchmarkBBROnPacketAcknowledged(b *testing.B) {
	ctx := context.Background()
	config := NewDefaultBBRConfig()
	prober := NewBBRBandwidthProber(ctx, config)
	
	sendTime := time.Now()
	packetSize := int64(1024)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ackTime := sendTime.Add(time.Duration(i) * time.Microsecond)
		rtt := ackTime.Sub(sendTime)
		prober.OnPacketAcknowledged(packetSize, sendTime, ackTime, rtt)
	}
}

func BenchmarkBBRUpdateBandwidthEstimate(b *testing.B) {
	ctx := context.Background()
	config := NewDefaultBBRConfig()
	prober := NewBBRBandwidthProber(ctx, config)
	
	sample := BBRBandwidthSample{
		Timestamp:      time.Now(),
		DeliveryRate:   50.0,
		BytesDelivered: 1024,
		RTT:            time.Millisecond * 30,
		IsAppLimited:   false,
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sample.Timestamp = sample.Timestamp.Add(time.Microsecond)
		prober.updateBandwidthEstimate(sample)
	}
}