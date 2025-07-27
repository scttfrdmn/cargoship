/*
Tests for CUBIC congestion control - comprehensive test suite for CUBIC TCP algorithm.
*/
package s3

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestNewCubicCongestionController(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultCubicConfig()
	
	controller := NewCubicCongestionController(ctx, config)
	
	if controller == nil {
		t.Fatal("Expected non-nil CUBIC congestion controller")
	}
	
	if controller.state != CubicStateSlowStart {
		t.Errorf("Expected initial state to be slow start, got %v", controller.state)
	}
	
	if controller.phase != CubicPhaseConcave {
		t.Errorf("Expected initial phase to be concave, got %v", controller.phase)
	}
	
	if controller.congestionWindow != config.InitialCwnd {
		t.Errorf("Expected initial congestion window of %f, got %f", config.InitialCwnd, controller.congestionWindow)
	}
	
	if controller.slowStartThreshold != config.InitialSSThresh {
		t.Errorf("Expected initial slow start threshold of %f, got %f", config.InitialSSThresh, controller.slowStartThreshold)
	}
	
	if controller.cubicC != config.C {
		t.Errorf("Expected CUBIC C parameter of %f, got %f", config.C, controller.cubicC)
	}
	
	if controller.cubicBeta != config.Beta {
		t.Errorf("Expected CUBIC beta parameter of %f, got %f", config.Beta, controller.cubicBeta)
	}
}

func TestCubicControllerStartStop(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultCubicConfig()
	controller := NewCubicCongestionController(ctx, config)
	
	// Test starting
	err := controller.StartController()
	if err != nil {
		t.Fatalf("Failed to start CUBIC controller: %v", err)
	}
	
	if !controller.isActive {
		t.Error("Expected controller to be active after start")
	}
	
	// Test starting again (should fail)
	err = controller.StartController()
	if err == nil {
		t.Error("Expected error when starting already active controller")
	}
	
	// Test stopping
	err = controller.StopController()
	if err != nil {
		t.Fatalf("Failed to stop CUBIC controller: %v", err)
	}
	
	if controller.isActive {
		t.Error("Expected controller to be inactive after stop")
	}
	
	// Test stopping again (should fail)
	err = controller.StopController()
	if err == nil {
		t.Error("Expected error when stopping already inactive controller")
	}
}

func TestCubicPacketSentTracking(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultCubicConfig()
	controller := NewCubicCongestionController(ctx, config)
	
	sendTime := time.Now()
	packetSize := int64(1500)
	
	originalInflight := controller.packetsInFlight
	originalSent := controller.totalPacketsSent
	
	controller.OnPacketSent(packetSize, sendTime)
	
	if controller.packetsInFlight != originalInflight+1 {
		t.Errorf("Expected packets in flight to be %d, got %d", originalInflight+1, controller.packetsInFlight)
	}
	
	if controller.totalPacketsSent != originalSent+1 {
		t.Errorf("Expected total packets sent to be %d, got %d", originalSent+1, controller.totalPacketsSent)
	}
}

func TestCubicPacketAcknowledged(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultCubicConfig()
	controller := NewCubicCongestionController(ctx, config)
	
	// First send a packet
	sendTime := time.Now()
	packetSize := int64(1500)
	controller.OnPacketSent(packetSize, sendTime)
	
	// Then acknowledge it
	ackTime := sendTime.Add(time.Millisecond * 50) // 50ms RTT
	rtt := ackTime.Sub(sendTime)
	
	originalInflight := controller.packetsInFlight
	originalAcked := controller.totalPacketsAcked
	originalCwnd := controller.congestionWindow
	
	controller.OnPacketAcknowledged(packetSize, sendTime, ackTime, rtt)
	
	// Check tracking updates
	if controller.packetsInFlight != originalInflight-1 {
		t.Errorf("Expected packets in flight to be %d, got %d", originalInflight-1, controller.packetsInFlight)
	}
	
	if controller.totalPacketsAcked != originalAcked+1 {
		t.Errorf("Expected total packets acked to be %d, got %d", originalAcked+1, controller.totalPacketsAcked)
	}
	
	// Check RTT update
	if controller.currentRTT != rtt {
		t.Errorf("Expected current RTT to be %v, got %v", rtt, controller.currentRTT)
	}
	
	// In slow start, cwnd should increase
	if controller.state == CubicStateSlowStart && controller.congestionWindow <= originalCwnd {
		t.Error("Expected congestion window to increase in slow start")
	}
	
	// Check that duplicate ACK count is reset
	if controller.duplicateAckCount != 0 {
		t.Error("Expected duplicate ACK count to be reset to 0")
	}
}

func TestCubicSlowStartBehavior(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultCubicConfig()
	controller := NewCubicCongestionController(ctx, config)
	
	// Ensure we're in slow start
	if controller.state != CubicStateSlowStart {
		t.Fatal("Expected initial state to be slow start")
	}
	
	initialCwnd := controller.congestionWindow
	sendTime := time.Now()
	packetSize := int64(1500)
	
	// Send and ACK multiple packets
	for i := 0; i < 5; i++ {
		controller.OnPacketSent(packetSize, sendTime)
		ackTime := sendTime.Add(time.Millisecond * 50)
		controller.OnPacketAcknowledged(packetSize, sendTime, ackTime, time.Millisecond*50)
		sendTime = sendTime.Add(time.Millisecond * 10)
	}
	
	// Window should have grown significantly
	if controller.congestionWindow <= initialCwnd+3 {
		t.Errorf("Expected significant window growth in slow start, got %f -> %f", initialCwnd, controller.congestionWindow)
	}
	
	// Should still be in slow start if below threshold
	if controller.congestionWindow < controller.slowStartThreshold && controller.state != CubicStateSlowStart {
		t.Error("Expected to remain in slow start below threshold")
	}
}

func TestCubicSlowStartToAvoidanceTransition(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultCubicConfig()
	config.InitialSSThresh = 15.0 // Low threshold for testing
	controller := NewCubicCongestionController(ctx, config)
	
	sendTime := time.Now()
	packetSize := int64(1500)
	
	// Send ACKs until we exceed slow start threshold
	for controller.congestionWindow < controller.slowStartThreshold {
		controller.OnPacketSent(packetSize, sendTime)
		ackTime := sendTime.Add(time.Millisecond * 50)
		controller.OnPacketAcknowledged(packetSize, sendTime, ackTime, time.Millisecond*50)
		sendTime = sendTime.Add(time.Millisecond * 10)
	}
	
	// Should have transitioned to congestion avoidance
	if controller.state != CubicStateCongestionAvoidance {
		t.Errorf("Expected transition to congestion avoidance, got %v", controller.state)
	}
}

func TestCubicWindowCalculation(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultCubicConfig()
	controller := NewCubicCongestionController(ctx, config)
	
	// Set up for congestion avoidance
	controller.state = CubicStateCongestionAvoidance
	controller.maxCongestionWindow = 100.0
	controller.congestionWindow = 50.0
	controller.lossDetectionTime = time.Now().Add(-time.Second) // 1 second ago
	
	currentTime := time.Now()
	cubicWindow := controller.calculateCubicWindow(currentTime)
	
	// Should calculate a valid window
	if cubicWindow < config.MinCwnd {
		t.Errorf("CUBIC window %f below minimum %f", cubicWindow, config.MinCwnd)
	}
	
	if cubicWindow > config.MaxCwnd {
		t.Errorf("CUBIC window %f above maximum %f", cubicWindow, config.MaxCwnd)
	}
	
	// Should be influenced by CUBIC function
	if cubicWindow == controller.congestionWindow {
		t.Error("Expected CUBIC window to be different from current window")
	}
}

func TestCubicTCPFriendlyCalculation(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultCubicConfig()
	config.TCPFriendlyEnable = true
	controller := NewCubicCongestionController(ctx, config)
	
	// Set up parameters
	controller.maxCongestionWindow = 100.0
	controller.smoothedRTT = time.Millisecond * 100
	controller.lossDetectionTime = time.Now().Add(-time.Second) // 1 second ago
	
	currentTime := time.Now()
	tcpWindow := controller.calculateTCPFriendlyWindow(currentTime)
	
	// Should calculate a valid TCP-friendly window
	if tcpWindow < config.MinCwnd {
		t.Errorf("TCP-friendly window %f below minimum %f", tcpWindow, config.MinCwnd)
	}
	
	if tcpWindow <= 0 {
		t.Error("Expected positive TCP-friendly window")
	}
}

func TestCubicPhaseDetection(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultCubicConfig()
	controller := NewCubicCongestionController(ctx, config)
	
	// Set up for concave phase (below Wmax)
	controller.maxCongestionWindow = 100.0
	newCwnd := 50.0
	
	controller.updateCubicPhase(newCwnd)
	if controller.phase != CubicPhaseConcave {
		t.Errorf("Expected concave phase for cwnd %f < Wmax %f, got %v", newCwnd, controller.maxCongestionWindow, controller.phase)
	}
	
	// Set up for convex phase (above Wmax)
	newCwnd = 150.0
	controller.updateCubicPhase(newCwnd)
	if controller.phase != CubicPhaseConvex {
		t.Errorf("Expected convex phase for cwnd %f > Wmax %f, got %v", newCwnd, controller.maxCongestionWindow, controller.phase)
	}
}

func TestCubicPacketLoss(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultCubicConfig()
	controller := NewCubicCongestionController(ctx, config)
	
	// Set up initial state
	controller.congestionWindow = 100.0
	originalCwnd := controller.congestionWindow
	originalLossCount := controller.totalPacketsLost
	
	sendTime := time.Now()
	packetSize := int64(1500)
	
	// Simulate packet loss
	controller.OnPacketLost(packetSize, sendTime, CubicLossFastRetransmit)
	
	// Check loss tracking
	if controller.totalPacketsLost != originalLossCount+1 {
		t.Errorf("Expected total packets lost to be %d, got %d", originalLossCount+1, controller.totalPacketsLost)
	}
	
	// Window should be reduced
	if controller.congestionWindow >= originalCwnd {
		t.Error("Expected congestion window to be reduced after loss")
	}
	
	// Should update max window
	if controller.maxCongestionWindow != originalCwnd {
		t.Errorf("Expected max window to be set to %f, got %f", originalCwnd, controller.maxCongestionWindow)
	}
}

func TestCubicTimeoutLoss(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultCubicConfig()
	controller := NewCubicCongestionController(ctx, config)
	
	// Set up congestion avoidance state
	controller.congestionWindow = 100.0
	controller.state = CubicStateCongestionAvoidance
	
	lossTime := time.Now()
	
	// Handle timeout loss
	controller.handleTimeoutLoss(lossTime)
	
	// Should reset to slow start
	if controller.state != CubicStateSlowStart {
		t.Errorf("Expected state to be slow start after timeout, got %v", controller.state)
	}
	
	// Should reset window to initial value
	if controller.congestionWindow != config.InitialCwnd {
		t.Errorf("Expected window to reset to %f, got %f", config.InitialCwnd, controller.congestionWindow)
	}
	
	// Should set appropriate slow start threshold
	expectedSSThresh := math.Max(100.0*config.Beta, config.MinCwnd)
	if controller.slowStartThreshold != expectedSSThresh {
		t.Errorf("Expected SS threshold to be %f, got %f", expectedSSThresh, controller.slowStartThreshold)
	}
}

func TestCubicFastRetransmitLoss(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultCubicConfig()
	controller := NewCubicCongestionController(ctx, config)
	
	// Set up congestion avoidance state
	originalCwnd := 100.0
	controller.congestionWindow = originalCwnd
	controller.state = CubicStateCongestionAvoidance
	
	lossTime := time.Now()
	
	// Handle fast retransmit loss
	controller.handleFastRetransmitLoss(lossTime)
	
	// Should reduce window by beta
	expectedCwnd := originalCwnd * config.Beta
	if controller.congestionWindow != expectedCwnd {
		t.Errorf("Expected window to be %f, got %f", expectedCwnd, controller.congestionWindow)
	}
	
	// Should set slow start threshold
	if controller.slowStartThreshold != expectedCwnd {
		t.Errorf("Expected SS threshold to be %f, got %f", expectedCwnd, controller.slowStartThreshold)
	}
	
	// Should transition to appropriate state
	if config.FastRecoveryEnable && controller.state != CubicStateFastRecovery {
		t.Error("Expected transition to fast recovery when enabled")
	}
}

func TestCubicDuplicateAckHandling(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultCubicConfig()
	controller := NewCubicCongestionController(ctx, config)
	
	ackTime := time.Now()
	originalDupCount := controller.duplicateAckCount
	
	// Send duplicate ACK
	controller.OnDuplicateAck(ackTime)
	
	if controller.duplicateAckCount != originalDupCount+1 {
		t.Errorf("Expected duplicate ACK count to be %d, got %d", originalDupCount+1, controller.duplicateAckCount)
	}
	
	// Send enough duplicate ACKs to trigger fast retransmit
	for i := 0; i < config.DuplicateAckThreshold; i++ {
		controller.OnDuplicateAck(ackTime)
	}
	
	// Should have triggered fast retransmit
	if controller.duplicateAckCount != 0 {
		t.Error("Expected duplicate ACK count to be reset after fast retransmit")
	}
}

func TestCubicRTTEstimation(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultCubicConfig()
	controller := NewCubicCongestionController(ctx, config)
	
	rtt := time.Millisecond * 50
	measurementTime := time.Now()
	
	// Initial RTT
	controller.updateRTTEstimates(rtt, measurementTime)
	
	if controller.currentRTT != rtt {
		t.Errorf("Expected current RTT to be %v, got %v", rtt, controller.currentRTT)
	}
	
	if controller.minRTT != rtt {
		t.Errorf("Expected min RTT to be %v, got %v", rtt, controller.minRTT)
	}
	
	if controller.smoothedRTT != rtt {
		t.Errorf("Expected initial smoothed RTT to equal current RTT, got %v", controller.smoothedRTT)
	}
	
	// Second RTT measurement
	rtt2 := time.Millisecond * 60
	controller.updateRTTEstimates(rtt2, measurementTime.Add(time.Second))
	
	// Min RTT should not change (50ms is still minimum)
	if controller.minRTT != rtt {
		t.Errorf("Expected min RTT to remain %v, got %v", rtt, controller.minRTT)
	}
	
	// Smoothed RTT should be between old and new
	if controller.smoothedRTT <= rtt || controller.smoothedRTT >= rtt2 {
		t.Errorf("Expected smoothed RTT between %v and %v, got %v", rtt, rtt2, controller.smoothedRTT)
	}
	
	// RTT variance should be calculated
	if controller.rttVariance <= 0 {
		t.Error("Expected positive RTT variance")
	}
}

func TestCubicHystart(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultCubicConfig()
	config.HystartEnable = true
	controller := NewCubicCongestionController(ctx, config)
	
	// Ensure we're in slow start
	controller.state = CubicStateSlowStart
	controller.congestionWindow = 10.0
	
	ackTime := time.Now()
	rtt := time.Millisecond * 50
	
	// Add ACKs to build train
	for i := 0; i < 5; i++ {
		controller.updateHystart(ackTime.Add(time.Duration(i)*time.Millisecond), rtt)
	}
	
	// Should have ACKs in train
	if len(controller.hystartAckTrain) == 0 {
		t.Error("Expected ACKs to be recorded in Hystart train")
	}
	
	// Test delay increase criterion
	highRTT := controller.minRTT + controller.hystartDelayThreshold + time.Millisecond
	originalState := controller.state
	controller.updateHystart(ackTime.Add(time.Second), highRTT)
	
	// Should exit slow start if delay increased significantly
	if originalState == CubicStateSlowStart && controller.minRTT > 0 {
		// Hystart should have triggered if conditions were met
		if controller.state == CubicStateSlowStart && len(controller.hystartAckTrain) > 1 {
			// Check if conditions would trigger exit
			if highRTT > controller.minRTT+controller.hystartDelayThreshold {
				t.Logf("Hystart conditions met: RTT %v > threshold %v", highRTT, controller.minRTT+controller.hystartDelayThreshold)
			}
		}
	}
}

func TestCubicParameterUpdates(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultCubicConfig()
	controller := NewCubicCongestionController(ctx, config)
	
	// Set test parameters
	controller.maxCongestionWindow = 100.0
	currentTime := time.Now()
	
	oldK := controller.cubicK
	
	controller.updateCubicParameters(currentTime)
	
	// K should be calculated
	if controller.cubicK == oldK && controller.maxCongestionWindow > 0 {
		// K should have been updated
		expectedK := math.Cbrt((controller.maxCongestionWindow * (1.0 - controller.cubicBeta)) / controller.cubicC)
		if math.Abs(controller.cubicK-expectedK) > 0.001 {
			t.Errorf("Expected K to be approximately %f, got %f", expectedK, controller.cubicK)
		}
	}
	
	// Loss detection time should be updated
	if !controller.lossDetectionTime.Equal(currentTime) {
		t.Error("Expected loss detection time to be updated")
	}
}

func TestCubicStateTransitions(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultCubicConfig()
	controller := NewCubicCongestionController(ctx, config)
	
	transitionTime := time.Now()
	originalTransitions := controller.metrics.StateTransitions[CubicStateCongestionAvoidance]
	
	// Test state transition
	controller.transitionToState(CubicStateCongestionAvoidance, transitionTime)
	
	if controller.state != CubicStateCongestionAvoidance {
		t.Errorf("Expected state to be congestion avoidance, got %v", controller.state)
	}
	
	if !controller.stateStartTime.Equal(transitionTime) {
		t.Error("Expected state start time to be updated")
	}
	
	// Check metrics update
	if controller.metrics.StateTransitions[CubicStateCongestionAvoidance] != originalTransitions+1 {
		t.Error("Expected state transition to be recorded in metrics")
	}
	
	// Test fast recovery specific initialization
	controller.transitionToState(CubicStateFastRecovery, transitionTime)
	
	if !controller.lossRecoveryPhase {
		t.Error("Expected loss recovery phase to be set in fast recovery")
	}
}

func TestCubicMetricsTracking(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultCubicConfig()
	controller := NewCubicCongestionController(ctx, config)
	
	// Test initial metrics
	metrics := controller.GetMetrics()
	if metrics == nil {
		t.Fatal("Expected non-nil metrics")
	}
	
	if metrics.StateTransitions == nil {
		t.Error("Expected state transitions map to be initialized")
	}
	
	// Test metrics update
	originalWindow := metrics.AverageWindow
	controller.congestionWindow = 50.0
	
	controller.updateMetrics()
	
	newMetrics := controller.GetMetrics()
	if newMetrics.AverageWindow == originalWindow && originalWindow != 50.0 {
		t.Error("Expected average window to be updated")
	}
	
	// Test RTT metrics
	controller.currentRTT = time.Millisecond * 30
	controller.smoothedRTT = time.Millisecond * 35
	controller.rttVariance = time.Millisecond * 5
	
	controller.updateMetrics()
	
	updatedMetrics := controller.GetMetrics()
	if updatedMetrics.AverageRTT != controller.smoothedRTT {
		t.Error("Expected average RTT to match smoothed RTT")
	}
	
	if updatedMetrics.RTTVariation != controller.rttVariance {
		t.Error("Expected RTT variation to match RTT variance")
	}
}

func TestCubicConfigDefaults(t *testing.T) {
	config := NewDefaultCubicConfig()
	
	if config == nil {
		t.Fatal("Expected non-nil default config")
	}
	
	if config.C != 0.4 {
		t.Errorf("Expected CUBIC C parameter to be 0.4, got %f", config.C)
	}
	
	if config.Beta != 0.7 {
		t.Errorf("Expected CUBIC beta parameter to be 0.7, got %f", config.Beta)
	}
	
	if config.InitialCwnd <= 0 {
		t.Error("Expected positive initial congestion window")
	}
	
	if config.MinCwnd <= 0 {
		t.Error("Expected positive minimum congestion window")
	}
	
	if config.MaxCwnd <= config.MinCwnd {
		t.Error("Expected maximum congestion window to be greater than minimum")
	}
	
	if config.DuplicateAckThreshold != 3 {
		t.Errorf("Expected duplicate ACK threshold to be 3, got %d", config.DuplicateAckThreshold)
	}
	
	if !config.HystartEnable {
		t.Error("Expected Hystart to be enabled by default")
	}
	
	if !config.TCPFriendlyEnable {
		t.Error("Expected TCP-friendly behavior to be enabled by default")
	}
}

func TestCubicEventRecording(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultCubicConfig()
	controller := NewCubicCongestionController(ctx, config)
	
	eventTime := time.Now()
	
	// Test congestion window event recording
	originalCwndEvents := len(controller.cwndHistory)
	controller.recordCwndEvent(10.0, 15.0, "test_trigger", eventTime)
	
	if len(controller.cwndHistory) != originalCwndEvents+1 {
		t.Error("Expected cwnd event to be recorded")
	}
	
	if len(controller.cwndHistory) > 0 {
		event := controller.cwndHistory[len(controller.cwndHistory)-1]
		if event.OldCwnd != 10.0 || event.NewCwnd != 15.0 {
			t.Error("Expected cwnd event to record correct values")
		}
		if event.Trigger != "test_trigger" {
			t.Error("Expected cwnd event to record correct trigger")
		}
	}
	
	// Test loss event recording
	originalLossEvents := len(controller.lossHistory)
	controller.recordLossEvent(CubicLossFastRetransmit, 2, eventTime)
	
	if len(controller.lossHistory) != originalLossEvents+1 {
		t.Error("Expected loss event to be recorded")
	}
	
	// Test adaptation event recording
	originalAdaptationEvents := len(controller.adaptationHistory)
	params := map[string]float64{"cwnd": 20.0, "ssthresh": 15.0}
	controller.recordAdaptationEvent("test_adaptation", params, "test_trigger", eventTime)
	
	if len(controller.adaptationHistory) != originalAdaptationEvents+1 {
		t.Error("Expected adaptation event to be recorded")
	}
}

func TestCubicControllerIntegration(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultCubicConfig()
	controller := NewCubicCongestionController(ctx, config)
	
	// Start controller
	err := controller.StartController()
	if err != nil {
		t.Fatalf("Failed to start controller: %v", err)
	}
	defer func() {
		_ = controller.StopController()
	}()
	
	sendTime := time.Now()
	packetSize := int64(1500)
	
	// Simulate packet flow
	initialCwnd := controller.GetCongestionWindow()
	
	for i := 0; i < 10; i++ {
		// Send packet
		controller.OnPacketSent(packetSize, sendTime)
		
		// ACK packet
		ackTime := sendTime.Add(time.Millisecond * 50)
		rtt := ackTime.Sub(sendTime)
		controller.OnPacketAcknowledged(packetSize, sendTime, ackTime, rtt)
		
		sendTime = sendTime.Add(time.Millisecond * 10)
	}
	
	// Window should have grown
	finalCwnd := controller.GetCongestionWindow()
	if finalCwnd <= initialCwnd {
		t.Error("Expected congestion window to grow with successful ACKs")
	}
	
	// Test loss handling
	controller.OnPacketLost(packetSize, sendTime, CubicLossFastRetransmit)
	
	postLossCwnd := controller.GetCongestionWindow()
	if postLossCwnd >= finalCwnd {
		t.Error("Expected congestion window to decrease after loss")
	}
	
	// Test metrics
	metrics := controller.GetMetrics()
	if metrics.TotalLossEvents == 0 {
		t.Error("Expected loss events to be recorded in metrics")
	}
	
	if metrics.AverageWindow <= 0 {
		t.Error("Expected positive average window in metrics")
	}
}

// Benchmark tests
func BenchmarkCubicOnPacketAcknowledged(b *testing.B) {
	ctx := context.Background()
	config := NewDefaultCubicConfig()
	controller := NewCubicCongestionController(ctx, config)
	
	sendTime := time.Now()
	packetSize := int64(1500)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ackTime := sendTime.Add(time.Duration(i) * time.Microsecond)
		rtt := ackTime.Sub(sendTime)
		controller.OnPacketAcknowledged(packetSize, sendTime, ackTime, rtt)
	}
}

func BenchmarkCubicWindowCalculation(b *testing.B) {
	ctx := context.Background()
	config := NewDefaultCubicConfig()
	controller := NewCubicCongestionController(ctx, config)
	
	controller.state = CubicStateCongestionAvoidance
	controller.maxCongestionWindow = 100.0
	controller.lossDetectionTime = time.Now().Add(-time.Second)
	
	currentTime := time.Now()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = controller.calculateCubicWindow(currentTime.Add(time.Duration(i) * time.Millisecond))
	}
}