/*
Tests for loss detection and recovery - comprehensive test suite for packet loss detection and recovery mechanisms.
*/
package s3

import (
	"context"
	"testing"
	"time"
)

func TestNewLossDetectionRecoverySystem(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultLossDetectionConfig()

	system := NewLossDetectionRecoverySystem(ctx, config)

	if system == nil {
		t.Fatal("Expected non-nil loss detection recovery system")
		return
	}

	if system.currentState != LossDetectionStateNormal {
		t.Errorf("Expected initial state to be normal, got %v", system.currentState)
	}

	if system.recoveryState != RecoveryStateNone {
		t.Errorf("Expected initial recovery state to be none, got %v", system.recoveryState)
	}

	if system.timeoutDetector == nil {
		t.Error("Expected timeout detector to be initialized")
	}

	if system.duplicateACKDetector == nil {
		t.Error("Expected duplicate ACK detector to be initialized")
	}

	if system.config.DuplicateACKThreshold != 3 {
		t.Errorf("Expected duplicate ACK threshold of 3, got %d", system.config.DuplicateACKThreshold)
	}
}

func TestLossDetectionSystemStartStop(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultLossDetectionConfig()
	system := NewLossDetectionRecoverySystem(ctx, config)

	// Test starting
	err := system.StartSystem()
	if err != nil {
		t.Fatalf("Failed to start loss detection system: %v", err)
	}

	if !system.isActive.Load() {
		t.Error("Expected system to be active after start")
	}

	// Test starting again (should fail)
	err = system.StartSystem()
	if err == nil {
		t.Error("Expected error when starting already active system")
	}

	// Test stopping
	err = system.StopSystem()
	if err != nil {
		t.Fatalf("Failed to stop loss detection system: %v", err)
	}

	if system.isActive.Load() {
		t.Error("Expected system to be inactive after stop")
	}

	// Test stopping again (should fail)
	err = system.StopSystem()
	if err == nil {
		t.Error("Expected error when stopping already inactive system")
	}
}

func TestPacketSentTracking(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultLossDetectionConfig()
	system := NewLossDetectionRecoverySystem(ctx, config)

	packetID := uint64(1)
	sendTime := time.Now()
	size := int64(1500)
	sequenceNumber := uint64(100)

	system.OnPacketSent(packetID, sendTime, size, sequenceNumber)

	// Check packet is tracked
	if packetInfo, exists := system.sentPackets[packetID]; !exists {
		t.Error("Expected packet to be tracked in sent packets")
	} else {
		if packetInfo.PacketID != packetID {
			t.Errorf("Expected packet ID %d, got %d", packetID, packetInfo.PacketID)
		}
		if packetInfo.Size != size {
			t.Errorf("Expected packet size %d, got %d", size, packetInfo.Size)
		}
		if packetInfo.SequenceNumber != sequenceNumber {
			t.Errorf("Expected sequence number %d, got %d", sequenceNumber, packetInfo.SequenceNumber)
		}
		if packetInfo.TimeoutDeadline.IsZero() {
			t.Error("Expected timeout deadline to be set")
		}
	}
}

func TestPacketAcknowledgedTracking(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultLossDetectionConfig()
	system := NewLossDetectionRecoverySystem(ctx, config)

	packetID := uint64(1)
	sendTime := time.Now()
	ackTime := sendTime.Add(time.Millisecond * 50)
	rtt := ackTime.Sub(sendTime)
	size := int64(1500)

	// First send the packet
	system.OnPacketSent(packetID, sendTime, size, 100)

	// Then acknowledge it
	sackBlocks := []SACKBlock{{StartSequence: 100, EndSequence: 200}}
	system.OnPacketAcknowledged(packetID, ackTime, rtt, sackBlocks, false)

	// Check packet moved from sent to acknowledged
	if _, exists := system.sentPackets[packetID]; exists {
		t.Error("Expected packet to be removed from sent packets after ACK")
	}

	if ackInfo, exists := system.acknowledgedPackets[packetID]; !exists {
		t.Error("Expected packet to be tracked in acknowledged packets")
	} else {
		if ackInfo.RTT != rtt {
			t.Errorf("Expected RTT %v, got %v", rtt, ackInfo.RTT)
		}
		if len(ackInfo.SACKBlocks) != 1 {
			t.Errorf("Expected 1 SACK block, got %d", len(ackInfo.SACKBlocks))
		}
		if ackInfo.ECNMarked {
			t.Error("Expected packet not to be ECN marked")
		}
	}
}

func TestDuplicateACKDetection(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultLossDetectionConfig()
	system := NewLossDetectionRecoverySystem(ctx, config)

	packetID := uint64(1)
	sendTime := time.Now()
	ackTime := sendTime.Add(time.Millisecond * 50)

	// Send packet
	system.OnPacketSent(packetID, sendTime, 1500, 100)

	originalLossEvents := len(system.lossEvents)

	// Send duplicate ACKs (below threshold)
	for i := 0; i < config.DuplicateACKThreshold-1; i++ {
		system.OnDuplicateACK(packetID, ackTime, i+1)
	}

	// Should not have triggered loss detection yet
	if len(system.lossEvents) != originalLossEvents {
		t.Error("Expected no loss detection before threshold")
	}

	// Send one more duplicate ACK to reach threshold
	system.OnDuplicateACK(packetID, ackTime, config.DuplicateACKThreshold)

	// Should have triggered loss detection
	if len(system.lossEvents) != originalLossEvents+1 {
		t.Error("Expected loss detection after duplicate ACK threshold")
	}

	// Check loss event details
	if len(system.lossEvents) > 0 {
		lossEvent := system.lossEvents[len(system.lossEvents)-1]
		if lossEvent.LossType != LossTypeDuplicateACK {
			t.Errorf("Expected duplicate ACK loss type, got %v", lossEvent.LossType)
		}
		if lossEvent.PacketID != packetID {
			t.Errorf("Expected packet ID %d, got %d", packetID, lossEvent.PacketID)
		}
	}
}

func TestTimeoutDetection(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultLossDetectionConfig()
	config.MinTimeout = time.Millisecond * 10 // Very short for testing
	system := NewLossDetectionRecoverySystem(ctx, config)

	err := system.StartSystem()
	if err != nil {
		t.Fatalf("Failed to start system: %v", err)
	}
	defer func() { _ = system.StopSystem() }()

	packetID := uint64(1)
	sendTime := time.Now()

	// Send packet - timeout will be calculated based on RTT (100ms * 4.0 = 400ms)
	system.OnPacketSent(packetID, sendTime, 1500, 100)

	originalLossEvents := len(system.GetLossEvents(0))

	// Wait for timeout (actual timeout is 400ms based on RTT)
	time.Sleep(time.Millisecond * 450)

	// Give system time to detect timeout
	time.Sleep(time.Millisecond * 50)

	// Should have detected timeout loss
	currentLossEvents := system.GetLossEvents(0)
	if len(currentLossEvents) <= originalLossEvents {
		t.Errorf("Expected timeout loss detection. Loss events: %d -> %d", originalLossEvents, len(currentLossEvents))
	}

	// Check loss event details
	if len(currentLossEvents) > 0 {
		lossEvent := currentLossEvents[len(currentLossEvents)-1]
		if lossEvent.LossType != LossTypeTimeout {
			t.Errorf("Expected timeout loss type, got %v", lossEvent.LossType)
		}
	}
}

func TestLossRecoveryInitiation(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultLossDetectionConfig()
	system := NewLossDetectionRecoverySystem(ctx, config)

	// Create a loss event
	lossEvent := LossDetectionEvent{
		Timestamp:        time.Now(),
		PacketID:         1,
		LossType:         LossTypeDuplicateACK,
		DetectionLatency: time.Millisecond * 10,
		RTTAtLoss:        time.Millisecond * 50,
		CwndAtLoss:       65536,
		InflightAtLoss:   10,
		LossRate:         0.01,
		Confidence:       0.9,
		RecoveryAction:   RecoveryTypeFast,
	}

	originalRecoveryEvents := len(system.recoveryEvents)

	// Initiate recovery
	system.initiateRecovery(lossEvent)

	// Should have created recovery event
	if len(system.recoveryEvents) != originalRecoveryEvents+1 {
		t.Error("Expected recovery event to be created")
	}

	// Check recovery event details
	if len(system.recoveryEvents) > 0 {
		recoveryEvent := system.recoveryEvents[len(system.recoveryEvents)-1]
		if recoveryEvent.RecoveryType != RecoveryTypeFast {
			t.Errorf("Expected fast recovery type, got %v", recoveryEvent.RecoveryType)
		}
		if recoveryEvent.TriggerEvent.PacketID != lossEvent.PacketID {
			t.Error("Expected recovery event to reference triggering loss event")
		}
	}
}

func TestRecoveryTypeSelection(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultLossDetectionConfig()
	system := NewLossDetectionRecoverySystem(ctx, config)

	testCases := []struct {
		lossType         LossType
		expectedRecovery RecoveryType
	}{
		{LossTypeTimeout, RecoveryTypeTimeout},
		{LossTypeDuplicateACK, RecoveryTypeFast},
		{LossTypeEarlyRetransmit, RecoveryTypeFast},
		{LossTypeECN, RecoveryTypeCongestion},
	}

	for _, tc := range testCases {
		lossEvent := LossDetectionEvent{
			LossType:   tc.lossType,
			Confidence: 0.9,
		}

		recoveryType := system.determineRecoveryType(lossEvent)
		if recoveryType != tc.expectedRecovery {
			t.Errorf("For loss type %v, expected recovery type %v, got %v",
				tc.lossType, tc.expectedRecovery, recoveryType)
		}
	}
}

func TestSACKBasedDetection(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultLossDetectionConfig()
	config.SACKEnable = true
	system := NewLossDetectionRecoverySystem(ctx, config)

	packetID := uint64(1)
	sendTime := time.Now()
	ackTime := sendTime.Add(time.Millisecond * 50)

	// Send packet
	system.OnPacketSent(packetID, sendTime, 1500, 100)

	// Acknowledge with SACK blocks indicating loss
	sackBlocks := []SACKBlock{
		{StartSequence: 200, EndSequence: 300}, // Gap indicates loss at 100-199
	}

	system.OnPacketAcknowledged(packetID+1, ackTime, time.Millisecond*50, sackBlocks, false)

	// SACK detector should have updated state
	if system.sackDetector == nil {
		t.Error("Expected SACK detector to be initialized")
	}

	// Check that SACK blocks were recorded
	if len(system.sackDetector.sackBlocks) != 1 {
		t.Errorf("Expected 1 SACK block, got %d", len(system.sackDetector.sackBlocks))
	}
}

func TestECNBasedDetection(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultLossDetectionConfig()
	config.ECNEnable = true
	system := NewLossDetectionRecoverySystem(ctx, config)

	packetID := uint64(1)
	sendTime := time.Now()
	ackTime := sendTime.Add(time.Millisecond * 50)

	// Send packet
	system.OnPacketSent(packetID, sendTime, 1500, 100)

	// Acknowledge with ECN marking
	system.OnPacketAcknowledged(packetID, ackTime, time.Millisecond*50, nil, true)

	// ECN detector should have updated marking rate
	if system.ecnDetector.ecnMarkingRate <= 0 {
		t.Error("Expected ECN marking rate to increase after marked packet")
	}
}

func TestMetricsTracking(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultLossDetectionConfig()
	system := NewLossDetectionRecoverySystem(ctx, config)

	// Test initial metrics
	metrics := system.GetMetrics()
	if metrics == nil {
		t.Fatal("Expected non-nil metrics")
		return
	}

	if metrics.TotalLossEvents != 0 {
		t.Error("Expected zero initial loss events")
	}

	if metrics.TotalRecoveryEvents != 0 {
		t.Error("Expected zero initial recovery events")
	}

	// Create and record a loss event
	lossEvent := LossDetectionEvent{
		Timestamp: time.Now(),
		PacketID:  1,
		LossType:  LossTypeDuplicateACK,
	}

	system.recordLossEvent(lossEvent)

	// Check updated metrics
	updatedMetrics := system.GetMetrics()
	if updatedMetrics.TotalLossEvents != 1 {
		t.Errorf("Expected 1 loss event, got %d", updatedMetrics.TotalLossEvents)
	}

	if updatedMetrics.LossEventsByType[LossTypeDuplicateACK] != 1 {
		t.Error("Expected duplicate ACK loss to be tracked by type")
	}
}

func TestStateTransitions(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultLossDetectionConfig()
	system := NewLossDetectionRecoverySystem(ctx, config)

	// Test initial state
	if system.GetCurrentState() != LossDetectionStateNormal {
		t.Error("Expected initial normal state")
	}

	if system.GetRecoveryState() != RecoveryStateNone {
		t.Error("Expected initial no recovery state")
	}

	// Trigger fast recovery
	system.updateRecoveryState(RecoveryTypeFast)

	if system.GetCurrentState() != LossDetectionStateFastRecovery {
		t.Errorf("Expected fast recovery state, got %v", system.GetCurrentState())
	}

	if system.GetRecoveryState() != RecoveryStateFastRecovery {
		t.Errorf("Expected fast recovery state, got %v", system.GetRecoveryState())
	}
}

func TestEventRetrieval(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultLossDetectionConfig()
	system := NewLossDetectionRecoverySystem(ctx, config)

	// Add some loss events
	for i := 0; i < 5; i++ {
		lossEvent := LossDetectionEvent{
			Timestamp: time.Now(),
			PacketID:  uint64(i + 1),
			LossType:  LossTypeDuplicateACK,
		}
		system.recordLossEvent(lossEvent)
	}

	// Test retrieving all events
	allEvents := system.GetLossEvents(0)
	if len(allEvents) != 5 {
		t.Errorf("Expected 5 loss events, got %d", len(allEvents))
	}

	// Test retrieving limited events
	limitedEvents := system.GetLossEvents(3)
	if len(limitedEvents) != 3 {
		t.Errorf("Expected 3 loss events, got %d", len(limitedEvents))
	}

	// Test retrieving more than available
	moreEvents := system.GetLossEvents(10)
	if len(moreEvents) != 5 {
		t.Errorf("Expected 5 loss events (all available), got %d", len(moreEvents))
	}
}

func TestRTTEstimation(t *testing.T) {
	rttEst := NewLossDetectionRTTEstimator()

	// Test initial values
	if rttEst.GetSmoothedRTT() != time.Millisecond*100 {
		t.Errorf("Expected initial smoothed RTT of 100ms, got %v", rttEst.GetSmoothedRTT())
	}

	// Test first RTT update
	newRTT := time.Millisecond * 80
	rttEst.UpdateRTT(newRTT, time.Now())

	// The RTT estimator starts with 100ms initial value and uses exponential smoothing
	smoothedAfterFirst := rttEst.GetSmoothedRTT()
	if smoothedAfterFirst < time.Millisecond*75 || smoothedAfterFirst > time.Millisecond*105 {
		t.Errorf("Expected smoothed RTT to be influenced by initial 100ms and new 80ms RTT, got %v", smoothedAfterFirst)
	}

	// Test subsequent RTT update (should be smoothed)
	secondRTT := time.Millisecond * 120
	rttEst.UpdateRTT(secondRTT, time.Now())

	smoothedRTT := rttEst.GetSmoothedRTT()
	if smoothedRTT == newRTT || smoothedRTT == secondRTT {
		t.Error("Expected smoothed RTT to be between measurements")
	}

	if smoothedRTT < newRTT || smoothedRTT > secondRTT {
		t.Errorf("Expected smoothed RTT between %v and %v, got %v", newRTT, secondRTT, smoothedRTT)
	}
}

func TestTimeoutCalculation(t *testing.T) {
	config := NewDefaultLossDetectionConfig()
	calc := NewTimeoutCalculator(config)

	smoothedRTT := time.Millisecond * 100

	timeout := calc.CalculateTimeout(smoothedRTT)

	// Should be at least the multiplier times the RTT
	expectedMin := time.Duration(float64(smoothedRTT) * config.TimeoutMultiplier)
	if timeout < expectedMin {
		t.Errorf("Expected timeout to be at least %v, got %v", expectedMin, timeout)
	}

	// Should respect minimum timeout
	if timeout < config.MinTimeout {
		t.Errorf("Expected timeout to be at least %v (min), got %v", config.MinTimeout, timeout)
	}

	// Should respect maximum timeout
	if timeout > config.MaxTimeout {
		t.Errorf("Expected timeout to be at most %v (max), got %v", config.MaxTimeout, timeout)
	}
}

func TestRecoveryManagers(t *testing.T) {
	config := NewDefaultLossDetectionConfig()

	// Test Fast Recovery Manager
	frm := NewFastRecoveryManager(config)
	lossEvent := LossDetectionEvent{
		PacketID:   1,
		CwndAtLoss: 65536,
	}

	recoveryEvent := frm.InitiateRecovery(lossEvent)
	if recoveryEvent.RecoveryType != RecoveryTypeFast {
		t.Error("Expected fast recovery type")
	}

	if recoveryEvent.CwndReduction != lossEvent.CwndAtLoss/2 {
		t.Error("Expected congestion window to be halved in fast recovery")
	}

	// Test Timeout Recovery Manager
	trm := NewTimeoutRecoveryManager(config)
	timeoutRecovery := trm.InitiateRecovery(lossEvent)
	if timeoutRecovery.RecoveryType != RecoveryTypeTimeout {
		t.Error("Expected timeout recovery type")
	}

	// Timeout recovery should be more aggressive than fast recovery
	if timeoutRecovery.CwndReduction <= recoveryEvent.CwndReduction {
		t.Error("Expected timeout recovery to be more aggressive than fast recovery")
	}

	// Test Congestion Recovery Manager
	crm := NewCongestionRecoveryManager(config)
	congestionRecovery := crm.InitiateRecovery(lossEvent)
	if congestionRecovery.RecoveryType != RecoveryTypeCongestion {
		t.Error("Expected congestion recovery type")
	}

	// ECN-based recovery should be gentler
	if congestionRecovery.CwndReduction >= recoveryEvent.CwndReduction {
		t.Error("Expected congestion recovery to be gentler than fast recovery")
	}
}

func TestConfigDefaults(t *testing.T) {
	config := NewDefaultLossDetectionConfig()

	if config == nil {
		t.Fatal("Expected non-nil default config")
		return
	}

	if config.DuplicateACKThreshold != 3 {
		t.Errorf("Expected duplicate ACK threshold of 3, got %d", config.DuplicateACKThreshold)
	}

	if config.TimeoutMultiplier != 4.0 {
		t.Errorf("Expected timeout multiplier of 4.0, got %f", config.TimeoutMultiplier)
	}

	if !config.FastRecoveryEnable {
		t.Error("Expected fast recovery to be enabled by default")
	}

	if !config.SACKEnable {
		t.Error("Expected SACK to be enabled by default")
	}

	if !config.ECNEnable {
		t.Error("Expected ECN to be enabled by default")
	}

	if config.MinTimeout <= 0 {
		t.Error("Expected positive minimum timeout")
	}

	if config.MaxTimeout <= config.MinTimeout {
		t.Error("Expected maximum timeout to be greater than minimum")
	}
}

func TestLossRateCalculation(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultLossDetectionConfig()
	system := NewLossDetectionRecoverySystem(ctx, config)

	// Add some packets to different states
	system.sentPackets[1] = &SentPacketInfo{PacketID: 1}
	system.sentPackets[2] = &SentPacketInfo{PacketID: 2}
	system.acknowledgedPackets[3] = &AcknowledgedPacketInfo{PacketID: 3}
	system.acknowledgedPackets[4] = &AcknowledgedPacketInfo{PacketID: 4}
	system.lostPackets[5] = &LostPacketInfo{PacketID: 5}

	lossRate := system.calculateCurrentLossRate()
	expectedLossRate := 1.0 / 5.0 // 1 lost out of 5 total

	if lossRate != expectedLossRate {
		t.Errorf("Expected loss rate of %f, got %f", expectedLossRate, lossRate)
	}

	// Test with no packets
	system.sentPackets = make(map[uint64]*SentPacketInfo)
	system.acknowledgedPackets = make(map[uint64]*AcknowledgedPacketInfo)
	system.lostPackets = make(map[uint64]*LostPacketInfo)

	emptyLossRate := system.calculateCurrentLossRate()
	if emptyLossRate != 0.0 {
		t.Errorf("Expected zero loss rate with no packets, got %f", emptyLossRate)
	}
}

func TestDeliveryRateCalculation(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultLossDetectionConfig()
	system := NewLossDetectionRecoverySystem(ctx, config)

	sendTime := time.Now()
	ackTime := sendTime.Add(time.Millisecond * 50) // 50ms
	packetSize := int64(1500)                      // 1500 bytes
	packetID := uint64(1)

	// Add packet to sent packets
	system.sentPackets[packetID] = &SentPacketInfo{
		PacketID: packetID,
		SendTime: sendTime,
		Size:     packetSize,
	}

	deliveryRate := system.calculateDeliveryRate(packetID, ackTime)

	// Calculate expected rate: 1500 bytes * 8 bits/byte / 0.05 seconds = 240,000 bps = ~0.229 Mbps
	expectedBps := float64(packetSize*8) / (time.Millisecond * 50).Seconds()
	expectedMbps := expectedBps / (1024 * 1024)

	if deliveryRate != expectedMbps {
		t.Errorf("Expected delivery rate of %f Mbps, got %f", expectedMbps, deliveryRate)
	}

	// Test with non-existent packet
	nonExistentRate := system.calculateDeliveryRate(999, ackTime)
	if nonExistentRate != 0.0 {
		t.Errorf("Expected zero delivery rate for non-existent packet, got %f", nonExistentRate)
	}
}

func TestSystemIntegration(t *testing.T) {
	ctx := context.Background()
	config := NewDefaultLossDetectionConfig()
	config.MinTimeout = time.Millisecond * 20 // Short timeout for testing
	system := NewLossDetectionRecoverySystem(ctx, config)

	// Start system
	err := system.StartSystem()
	if err != nil {
		t.Fatalf("Failed to start system: %v", err)
	}
	defer func() { _ = system.StopSystem() }()

	// Simulate packet flow with loss
	packetID := uint64(1)
	sendTime := time.Now()

	// Send packet
	system.OnPacketSent(packetID, sendTime, 1500, 100)

	// Simulate multiple duplicate ACKs to trigger fast recovery
	ackTime := sendTime.Add(time.Millisecond * 10)
	for i := 0; i < config.DuplicateACKThreshold; i++ {
		system.OnDuplicateACK(packetID, ackTime, i+1)
	}

	// Wait for system to process
	time.Sleep(time.Millisecond * 30)

	// Check that loss was detected and recovery initiated
	metrics := system.GetMetrics()
	if metrics.TotalLossEvents == 0 {
		t.Error("Expected loss events to be recorded")
	}

	if metrics.TotalRecoveryEvents == 0 {
		t.Error("Expected recovery events to be recorded")
	}

	// Check state transitions
	if system.GetCurrentState() == LossDetectionStateNormal {
		t.Error("Expected system to transition from normal state after loss detection")
	}

	// Retrieve events
	lossEvents := system.GetLossEvents(10)
	if len(lossEvents) == 0 {
		t.Error("Expected to retrieve loss events")
	}

	recoveryEvents := system.GetRecoveryEvents(10)
	if len(recoveryEvents) == 0 {
		t.Error("Expected to retrieve recovery events")
	}
}

// Benchmark tests
func BenchmarkLossDetection(b *testing.B) {
	ctx := context.Background()
	config := NewDefaultLossDetectionConfig()
	system := NewLossDetectionRecoverySystem(ctx, config)

	sendTime := time.Now()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		packetID := uint64(i)
		system.OnPacketSent(packetID, sendTime, 1500, uint64(i))

		// Simulate ACK half the time
		if i%2 == 0 {
			ackTime := sendTime.Add(time.Millisecond * 50)
			system.OnPacketAcknowledged(packetID, ackTime, time.Millisecond*50, nil, false)
		}
	}
}

func BenchmarkRecoveryInitiation(b *testing.B) {
	ctx := context.Background()
	config := NewDefaultLossDetectionConfig()
	system := NewLossDetectionRecoverySystem(ctx, config)

	lossEvent := LossDetectionEvent{
		Timestamp:      time.Now(),
		PacketID:       1,
		LossType:       LossTypeDuplicateACK,
		CwndAtLoss:     65536,
		RecoveryAction: RecoveryTypeFast,
		Confidence:     0.9,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lossEvent.PacketID = uint64(i)
		system.initiateRecovery(lossEvent)
	}
}
