package staging

import (
	"context"
	"testing"
	"time"
)

func TestNewAdaptiveTransferController(t *testing.T) {
	config := DefaultAdaptationConfig()
	controller := NewAdaptiveTransferController(config)
	
	if controller == nil {
		t.Fatal("Expected non-nil AdaptiveTransferController")
	}
	
	if controller.config != config {
		t.Error("Expected config to be set correctly")
	}
	
	if controller.activeTransfers == nil {
		t.Error("Expected activeTransfers to be initialized")
	}
	
	if controller.transferCallbacks == nil {
		t.Error("Expected transferCallbacks to be initialized")
	}
	
	if controller.parameterHistory == nil {
		t.Error("Expected parameterHistory to be initialized")
	}
	
	if controller.performanceTracker == nil {
		t.Error("Expected performanceTracker to be initialized")
	}
	
	if controller.active {
		t.Error("Expected controller to be inactive initially")
	}
}

func TestAdaptiveTransferController_StartStop(t *testing.T) {
	config := DefaultAdaptationConfig()
	controller := NewAdaptiveTransferController(config)
	ctx := context.Background()
	
	// Test start
	err := controller.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start controller: %v", err)
	}
	
	if !controller.active {
		t.Error("Expected controller to be active after start")
	}
	
	// Test stop
	err = controller.Stop()
	if err != nil {
		t.Fatalf("Failed to stop controller: %v", err)
	}
	
	if controller.active {
		t.Error("Expected controller to be inactive after stop")
	}
}

func TestAdaptiveTransferController_DoubleStart(t *testing.T) {
	config := DefaultAdaptationConfig()
	controller := NewAdaptiveTransferController(config)
	ctx := context.Background()
	
	// First start should succeed
	err := controller.Start(ctx)
	if err != nil {
		t.Fatalf("First start failed: %v", err)
	}
	
	// Second start should also succeed (idempotent)
	err = controller.Start(ctx)
	if err != nil {
		t.Fatalf("Second start failed: %v", err)
	}
}

func TestAdaptiveTransferController_StartTransferSession(t *testing.T) {
	config := DefaultAdaptationConfig()
	controller := NewAdaptiveTransferController(config)
	
	sessionID := "test-session-1"
	totalBytes := int64(1024 * 1024 * 100) // 100MB
	initialParams := DefaultTransferParameters()
	
	err := controller.StartTransferSession(sessionID, totalBytes, initialParams)
	if err != nil {
		t.Fatalf("Failed to start transfer session: %v", err)
	}
	
	// Verify session was created
	session, err := controller.GetTransferSession(sessionID)
	if err != nil {
		t.Fatalf("Failed to get transfer session: %v", err)
	}
	
	if session.ID != sessionID {
		t.Error("Expected session ID to match")
	}
	
	if session.TotalBytes != totalBytes {
		t.Error("Expected total bytes to match")
	}
	
	if !session.Active {
		t.Error("Expected session to be active")
	}
	
	if session.CurrentParameters.ChunkSizeMB != initialParams.ChunkSizeMB {
		t.Error("Expected initial parameters to be set")
	}
}

func TestAdaptiveTransferController_StartTransferSessionWithNilParams(t *testing.T) {
	config := DefaultAdaptationConfig()
	controller := NewAdaptiveTransferController(config)
	
	sessionID := "test-session-2"
	totalBytes := int64(1024 * 1024 * 50) // 50MB
	
	err := controller.StartTransferSession(sessionID, totalBytes, nil)
	if err != nil {
		t.Fatalf("Failed to start transfer session with nil params: %v", err)
	}
	
	// Should use default parameters
	session, err := controller.GetTransferSession(sessionID)
	if err != nil {
		t.Fatalf("Failed to get transfer session: %v", err)
	}
	
	defaultParams := DefaultTransferParameters()
	if session.CurrentParameters.ChunkSizeMB != defaultParams.ChunkSizeMB {
		t.Error("Expected default parameters to be used")
	}
}

func TestAdaptiveTransferController_EndTransferSession(t *testing.T) {
	config := DefaultAdaptationConfig()
	controller := NewAdaptiveTransferController(config)
	
	sessionID := "test-session-3"
	totalBytes := int64(1024 * 1024 * 25) // 25MB
	
	// Start session
	err := controller.StartTransferSession(sessionID, totalBytes, nil)
	if err != nil {
		t.Fatalf("Failed to start transfer session: %v", err)
	}
	
	// End session
	err = controller.EndTransferSession(sessionID)
	if err != nil {
		t.Fatalf("Failed to end transfer session: %v", err)
	}
	
	// Session should no longer exist in active transfers
	_, err = controller.GetTransferSession(sessionID)
	if err == nil {
		t.Error("Expected error when getting ended session")
	}
}

func TestAdaptiveTransferController_EndNonExistentSession(t *testing.T) {
	config := DefaultAdaptationConfig()
	controller := NewAdaptiveTransferController(config)
	
	err := controller.EndTransferSession("non-existent-session")
	if err == nil {
		t.Error("Expected error when ending non-existent session")
	}
}

func TestAdaptiveTransferController_UpdateTransferProgress(t *testing.T) {
	config := DefaultAdaptationConfig()
	controller := NewAdaptiveTransferController(config)
	
	sessionID := "test-session-4"
	totalBytes := int64(1024 * 1024 * 50) // 50MB
	
	// Start session
	err := controller.StartTransferSession(sessionID, totalBytes, nil)
	if err != nil {
		t.Fatalf("Failed to start transfer session: %v", err)
	}
	defer func() { _ = controller.EndTransferSession(sessionID) }()
	
	// Update progress
	transferredBytes := int64(1024 * 1024 * 10) // 10MB
	currentThroughput := 25.0                   // 25 MB/s
	networkCondition := &NetworkCondition{
		Timestamp:     time.Now(),
		BandwidthMBps: 100.0,
		LatencyMs:     15.0,
		PacketLoss:    0.001,
	}
	
	err = controller.UpdateTransferProgress(sessionID, transferredBytes, currentThroughput, networkCondition)
	if err != nil {
		t.Fatalf("Failed to update transfer progress: %v", err)
	}
	
	// Verify progress was updated
	session, err := controller.GetTransferSession(sessionID)
	if err != nil {
		t.Fatalf("Failed to get transfer session: %v", err)
	}
	
	if session.TransferredBytes != transferredBytes {
		t.Error("Expected transferred bytes to be updated")
	}
	
	if len(session.PerformanceHistory) == 0 {
		t.Error("Expected performance history to have entries")
	}
	
	if len(session.NetworkHistory) == 0 {
		t.Error("Expected network history to have entries")
	}
	
	lastSnapshot := session.PerformanceHistory[len(session.PerformanceHistory)-1]
	if lastSnapshot.ThroughputMBps != currentThroughput {
		t.Error("Expected throughput to be recorded in snapshot")
	}
}

func TestAdaptiveTransferController_ApplyAdaptation(t *testing.T) {
	config := DefaultAdaptationConfig()
	controller := NewAdaptiveTransferController(config)
	
	sessionID := "test-session-5"
	totalBytes := int64(1024 * 1024 * 1000) // 1000MB (1GB)
	
	// Start session
	err := controller.StartTransferSession(sessionID, totalBytes, nil)
	if err != nil {
		t.Fatalf("Failed to start transfer session: %v", err)
	}
	defer func() { _ = controller.EndTransferSession(sessionID) }()
	
	// Create adaptation state
	adaptationState := &AdaptationState{
		ChunkSizeMB:      64,
		Concurrency:      8,
		CompressionLevel: "zstd-fast",
		BufferSizeMB:     512,
	}
	
	// Apply adaptation
	err = controller.ApplyAdaptation(adaptationState)
	if err != nil {
		t.Fatalf("Failed to apply adaptation: %v", err)
	}
	
	// Verify adaptation was applied
	session, err := controller.GetTransferSession(sessionID)
	if err != nil {
		t.Fatalf("Failed to get transfer session: %v", err)
	}
	
	if session.CurrentParameters.ChunkSizeMB != 64 {
		t.Errorf("Expected chunk size to be 64, got %d", session.CurrentParameters.ChunkSizeMB)
	}
	
	if session.CurrentParameters.Concurrency != 8 {
		t.Error("Expected concurrency to be updated")
	}
	
	if session.CurrentParameters.CompressionLevel != "zstd-fast" {
		t.Error("Expected compression level to be updated")
	}
	
	if session.AdaptationCount != 1 {
		t.Error("Expected adaptation count to be incremented")
	}
}

func TestAdaptiveTransferController_RegisterTransferCallback(t *testing.T) {
	config := DefaultAdaptationConfig()
	controller := NewAdaptiveTransferController(config)
	
	callbackCalled := false
	var capturedSessionID string
	var capturedOldParams, capturedNewParams *TransferParameters
	
	callback := func(sessionID string, oldParams, newParams *TransferParameters) error {
		callbackCalled = true
		capturedSessionID = sessionID
		capturedOldParams = oldParams
		capturedNewParams = newParams
		return nil
	}
	
	controller.RegisterTransferCallback(callback)
	
	// Start session
	sessionID := "test-session-6"
	totalBytes := int64(1024 * 1024 * 50) // 50MB
	
	err := controller.StartTransferSession(sessionID, totalBytes, nil)
	if err != nil {
		t.Fatalf("Failed to start transfer session: %v", err)
	}
	defer func() { _ = controller.EndTransferSession(sessionID) }()
	
	// Apply adaptation to trigger callback
	adaptationState := &AdaptationState{
		ChunkSizeMB:      32,
		Concurrency:      4,
		CompressionLevel: "zstd",
		BufferSizeMB:     256,
	}
	
	err = controller.ApplyAdaptation(adaptationState)
	if err != nil {
		t.Fatalf("Failed to apply adaptation: %v", err)
	}
	
	// Allow time for callback to be called
	time.Sleep(50 * time.Millisecond)
	
	if !callbackCalled {
		t.Error("Expected callback to be called")
	}
	
	if capturedSessionID != sessionID {
		t.Error("Expected session ID to be passed to callback")
	}
	
	if capturedOldParams == nil || capturedNewParams == nil {
		t.Error("Expected old and new params to be passed to callback")
	}
}

func TestAdaptiveTransferController_GetActiveTransfers(t *testing.T) {
	config := DefaultAdaptationConfig()
	controller := NewAdaptiveTransferController(config)
	
	// Start multiple sessions
	sessionIDs := []string{"session-1", "session-2", "session-3"}
	for _, sessionID := range sessionIDs {
		err := controller.StartTransferSession(sessionID, 1024*1024*10, nil)
		if err != nil {
			t.Fatalf("Failed to start session %s: %v", sessionID, err)
		}
	}
	
	// Get active transfers
	activeTransfers := controller.GetActiveTransfers()
	
	if len(activeTransfers) != len(sessionIDs) {
		t.Errorf("Expected %d active transfers, got %d", len(sessionIDs), len(activeTransfers))
	}
	
	for _, sessionID := range sessionIDs {
		if _, exists := activeTransfers[sessionID]; !exists {
			t.Errorf("Expected session %s to be in active transfers", sessionID)
		}
	}
	
	// Clean up
	for _, sessionID := range sessionIDs {
		_ = controller.EndTransferSession(sessionID)
	}
}

func TestDefaultTransferParameters(t *testing.T) {
	params := DefaultTransferParameters()
	
	if params == nil {
		t.Fatal("Expected non-nil transfer parameters")
	}
	
	if params.ChunkSizeMB != 32 {
		t.Error("Expected default chunk size to be 32 MB")
	}
	
	if params.Concurrency != 4 {
		t.Error("Expected default concurrency to be 4")
	}
	
	if params.CompressionLevel != "zstd" {
		t.Error("Expected default compression level to be zstd")
	}
	
	if params.BufferSizeMB != 256 {
		t.Error("Expected default buffer size to be 256 MB")
	}
	
	if params.RetryPolicy == nil {
		t.Error("Expected retry policy to be set")
	}
	
	if params.TimeoutSettings == nil {
		t.Error("Expected timeout settings to be set")
	}
	
	if params.FlowControlSettings == nil {
		t.Error("Expected flow control settings to be set")
	}
}

func TestDefaultRetryPolicy(t *testing.T) {
	policy := DefaultRetryPolicy()
	
	if policy == nil {
		t.Fatal("Expected non-nil retry policy")
	}
	
	if policy.MaxRetries != 3 {
		t.Error("Expected default max retries to be 3")
	}
	
	if policy.InitialDelay != time.Second {
		t.Error("Expected default initial delay to be 1 second")
	}
	
	if policy.BackoffFactor != 2.0 {
		t.Error("Expected default backoff factor to be 2.0")
	}
	
	if policy.MaxDelay != time.Minute {
		t.Error("Expected default max delay to be 1 minute")
	}
	
	if !policy.JitterEnabled {
		t.Error("Expected jitter to be enabled by default")
	}
}

func TestDefaultTimeoutSettings(t *testing.T) {
	settings := DefaultTimeoutSettings()
	
	if settings == nil {
		t.Fatal("Expected non-nil timeout settings")
	}
	
	if settings.ConnectionTimeout != time.Second*30 {
		t.Error("Expected default connection timeout to be 30 seconds")
	}
	
	if settings.ReadTimeout != time.Minute*5 {
		t.Error("Expected default read timeout to be 5 minutes")
	}
	
	if settings.WriteTimeout != time.Minute*5 {
		t.Error("Expected default write timeout to be 5 minutes")
	}
	
	if settings.IdleTimeout != time.Minute*10 {
		t.Error("Expected default idle timeout to be 10 minutes")
	}
}

func TestDefaultFlowControlSettings(t *testing.T) {
	settings := DefaultFlowControlSettings()
	
	if settings == nil {
		t.Fatal("Expected non-nil flow control settings")
	}
	
	if settings.WindowSize != 64 {
		t.Error("Expected default window size to be 64")
	}
	
	if settings.CongestionWindow != 10 {
		t.Error("Expected default congestion window to be 10")
	}
	
	if settings.SlowStartThreshold != 32 {
		t.Error("Expected default slow start threshold to be 32")
	}
	
	if settings.CongestionAlgorithm != "cubic" {
		t.Error("Expected default congestion algorithm to be cubic")
	}
}

func TestParameterHistory_RecordAdaptation(t *testing.T) {
	history := NewParameterHistory()
	
	sessionID := "test-session"
	oldParams := &TransferParameters{ChunkSizeMB: 32}
	newParams := &TransferParameters{ChunkSizeMB: 64}
	
	history.RecordAdaptation(sessionID, oldParams, newParams)
	
	if len(history.adaptations) != 1 {
		t.Error("Expected one adaptation record")
	}
	
	record := history.adaptations[0]
	if record.SessionID != sessionID {
		t.Error("Expected session ID to be recorded")
	}
	
	if record.OldParams.ChunkSizeMB != 32 {
		t.Error("Expected old params to be recorded")
	}
	
	if record.NewParams.ChunkSizeMB != 64 {
		t.Error("Expected new params to be recorded")
	}
}

func TestParameterHistory_RecordSession(t *testing.T) {
	history := NewParameterHistory()
	
	session := &TransferSession{
		ID:        "test-session",
		StartTime: time.Now(),
		TotalBytes: 1024 * 1024 * 100,
		Active:    false,
	}
	
	history.RecordSession(session)
	
	if len(history.sessions) != 1 {
		t.Error("Expected one session record")
	}
	
	if history.sessions[0] != session {
		t.Error("Expected session to be recorded correctly")
	}
}

func TestParameterHistory_MaxHistoryLimit(t *testing.T) {
	history := NewParameterHistory()
	
	// Add more than max history
	for i := 0; i < 600; i++ {
		record := &ParameterAdaptationRecord{
			Timestamp: time.Now(),
			SessionID: "session",
			OldParams: &TransferParameters{ChunkSizeMB: 32},
			NewParams: &TransferParameters{ChunkSizeMB: 64},
		}
		history.adaptations = append(history.adaptations, record)
	}
	
	// Trigger record adaptation to test limit
	history.RecordAdaptation("new-session", &TransferParameters{}, &TransferParameters{})
	
	if len(history.adaptations) > history.maxHistory {
		t.Errorf("Expected adaptations to be limited to %d, got %d", history.maxHistory, len(history.adaptations))
	}
}

func TestTransferPerformanceTracker_RecordPerformance(t *testing.T) {
	tracker := NewTransferPerformanceTracker()
	
	sessionID := "test-session"
	snapshot := &PerformanceSnapshot{
		Timestamp:      time.Now(),
		ThroughputMBps: 50.0,
		LatencyMs:      15.0,
	}
	
	tracker.RecordPerformance(sessionID, snapshot)
	
	if len(tracker.performanceData[sessionID]) != 1 {
		t.Error("Expected one performance record for session")
	}
	
	if tracker.performanceData[sessionID][0] != snapshot {
		t.Error("Expected snapshot to be recorded correctly")
	}
}

func TestTransferPerformanceTracker_MaxPerformanceLimit(t *testing.T) {
	tracker := NewTransferPerformanceTracker()
	
	sessionID := "test-session"
	
	// Add more than max per session
	for i := 0; i < 250; i++ {
		snapshot := &PerformanceSnapshot{
			Timestamp:      time.Now(),
			ThroughputMBps: float64(i),
		}
		tracker.RecordPerformance(sessionID, snapshot)
	}
	
	maxPerSession := 200
	if len(tracker.performanceData[sessionID]) > maxPerSession {
		t.Errorf("Expected performance data to be limited to %d per session, got %d", maxPerSession, len(tracker.performanceData[sessionID]))
	}
}

func TestUtilityFunctions(t *testing.T) {
	// Test max function
	if max(5, 3) != 5 {
		t.Error("Expected max(5, 3) to be 5")
	}
	
	if max(2, 8) != 8 {
		t.Error("Expected max(2, 8) to be 8")
	}
	
	if max(4, 4) != 4 {
		t.Error("Expected max(4, 4) to be 4")
	}
	
	// Test min function
	if min(5, 3) != 3 {
		t.Error("Expected min(5, 3) to be 3")
	}
	
	if min(2, 8) != 2 {
		t.Error("Expected min(2, 8) to be 2")
	}
	
	if min(4, 4) != 4 {
		t.Error("Expected min(4, 4) to be 4")
	}
}