package s3

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDynamicParameterAdjuster(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	po := NewRealTimeParameterOptimizer(ctx, nm)
	defer func() { _ = po.Shutdown() }()

	dpa := NewDynamicParameterAdjuster(ctx, nm, po)
	defer func() { _ = dpa.Shutdown() }()

	assert.NotNil(t, dpa)
	assert.Equal(t, time.Second*30, dpa.adjustmentInterval)
	assert.Equal(t, time.Minute*2, dpa.stabilityWindow)
	assert.Equal(t, 0.1, dpa.adjustmentThreshold)
	assert.NotNil(t, dpa.networkMonitor)
	assert.NotNil(t, dpa.parameterOptimizer)
	assert.NotNil(t, dpa.uploadSessionManager)
	assert.NotNil(t, dpa.adjustmentEngine)
	assert.NotNil(t, dpa.transitionController)
	assert.NotNil(t, dpa.activeSessions)
	assert.NotNil(t, dpa.sessionMetrics)
	assert.NotNil(t, dpa.adjustmentHistory)
	assert.True(t, dpa.adjustmentActive)
	assert.Equal(t, AdjustmentAdaptive, dpa.adjustmentStrategy)
	assert.NotNil(t, dpa.performanceTracker)
	assert.NotNil(t, dpa.impactAnalyzer)
	assert.NotNil(t, dpa.riskAssessment)
}

func TestDynamicParameterAdjusterRegisterUploadSession(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	po := NewRealTimeParameterOptimizer(ctx, nm)
	defer func() { _ = po.Shutdown() }()

	dpa := NewDynamicParameterAdjuster(ctx, nm, po)
	defer func() { _ = dpa.Shutdown() }()

	sessionID := "test-session-1"
	initialParams := NewDefaultActiveUploadParameters()
	uploadCtx := &UploadContext{
		TotalBytes:        1024 * 1024 * 100, // 100MB
		TotalChunks:       10,
		EstimatedDuration: time.Minute * 5,
		Priority:          PriorityNormal,
		ContentType:       "application/octet-stream",
	}

	err := dpa.RegisterUploadSession(sessionID, initialParams, uploadCtx)
	require.NoError(t, err)

	// Verify session was registered
	assert.Contains(t, dpa.activeSessions, sessionID)
	assert.Contains(t, dpa.sessionMetrics, sessionID)

	session := dpa.activeSessions[sessionID]
	assert.Equal(t, sessionID, session.ID)
	assert.Equal(t, uploadCtx.TotalBytes, session.TotalBytes)
	assert.Equal(t, int64(0), session.UploadedBytes)
	assert.Equal(t, uploadCtx.TotalBytes, session.RemainingBytes)
	assert.Equal(t, 0.0, session.UploadProgress)
	assert.True(t, session.AdjustmentEnabled)
	assert.False(t, session.TransitionInProgress)

	// Test duplicate registration
	err = dpa.RegisterUploadSession(sessionID, initialParams, uploadCtx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestDynamicParameterAdjusterUpdateSessionProgress(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	po := NewRealTimeParameterOptimizer(ctx, nm)
	defer func() { _ = po.Shutdown() }()

	dpa := NewDynamicParameterAdjuster(ctx, nm, po)
	defer func() { _ = dpa.Shutdown() }()

	sessionID := "test-session-2"
	initialParams := NewDefaultActiveUploadParameters()
	uploadCtx := &UploadContext{
		TotalBytes:        1024 * 1024 * 100, // 100MB
		TotalChunks:       10,
		EstimatedDuration: time.Minute * 5,
		Priority:          PriorityNormal,
		ContentType:       "application/octet-stream",
	}

	err := dpa.RegisterUploadSession(sessionID, initialParams, uploadCtx)
	require.NoError(t, err)

	// Update session progress
	progressUpdate := &SessionProgressUpdate{
		UploadedBytes:     1024 * 1024 * 25, // 25MB uploaded
		CurrentThroughput: 10.0,             // 10 MB/s
		LatencyMs:         50.0,
		ErrorRate:         0.02,
		CompletedChunks:   3,
		FailedChunks:      0,
		ActiveConnections: 8,
	}

	err = dpa.UpdateSessionProgress(sessionID, progressUpdate)
	require.NoError(t, err)

	// Verify session was updated
	session := dpa.activeSessions[sessionID]
	assert.Equal(t, progressUpdate.UploadedBytes, session.UploadedBytes)
	assert.Equal(t, uploadCtx.TotalBytes-progressUpdate.UploadedBytes, session.RemainingBytes)
	assert.Equal(t, 0.25, session.UploadProgress) // 25% progress
	assert.Equal(t, progressUpdate.CurrentThroughput, session.CurrentThroughput)
	assert.Equal(t, progressUpdate.ErrorRate, session.ErrorRate)
	assert.Equal(t, progressUpdate.CompletedChunks, session.CompletedChunks)
	assert.Greater(t, session.AverageThroughput, 0.0)
	assert.Greater(t, session.EstimatedTimeLeft, time.Duration(0))

	// Test progress update for non-existent session
	err = dpa.UpdateSessionProgress("non-existent", progressUpdate)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestDynamicParameterAdjusterAdjustSessionParameters(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	po := NewRealTimeParameterOptimizer(ctx, nm)
	defer func() { _ = po.Shutdown() }()

	dpa := NewDynamicParameterAdjuster(ctx, nm, po)
	defer func() { _ = dpa.Shutdown() }()

	sessionID := "test-session-3"
	initialParams := NewDefaultActiveUploadParameters()
	uploadCtx := &UploadContext{
		TotalBytes:        1024 * 1024 * 100,
		TotalChunks:       10,
		EstimatedDuration: time.Minute * 5,
		Priority:          PriorityNormal,
		ContentType:       "application/octet-stream",
	}

	err := dpa.RegisterUploadSession(sessionID, initialParams, uploadCtx)
	require.NoError(t, err)

	// Test parameter adjustment
	adjustmentRequest := &ParameterAdjustmentRequest{
		ParameterName: "ConcurrentConnections",
		NewValue:      12,
		Type:          AdjustmentOptimize,
		Reason:        "test_adjustment",
		Priority:      5,
	}

	result, err := dpa.AdjustSessionParameters(sessionID, adjustmentRequest)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Greater(t, result.ParameterVersion, int64(1))

	// Verify parameter was updated
	session := dpa.activeSessions[sessionID]
	assert.Equal(t, 12, session.CurrentParameters.ConcurrentConnections)
	assert.Equal(t, 1, session.AdjustmentCount)
	assert.Greater(t, len(dpa.adjustmentHistory), 0)

	// Test adjustment for non-existent session
	_, err = dpa.AdjustSessionParameters("non-existent", adjustmentRequest)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Test invalid parameter adjustment
	invalidRequest := &ParameterAdjustmentRequest{
		ParameterName: "InvalidParameter",
		NewValue:      "invalid",
		Type:          AdjustmentOptimize,
		Reason:        "test_invalid",
		Priority:      5,
	}

	_, err = dpa.AdjustSessionParameters(sessionID, invalidRequest)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid adjustment request")
}

func TestDynamicParameterAdjusterGetSessionStatus(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	po := NewRealTimeParameterOptimizer(ctx, nm)
	defer func() { _ = po.Shutdown() }()

	dpa := NewDynamicParameterAdjuster(ctx, nm, po)
	defer func() { _ = dpa.Shutdown() }()

	sessionID := "test-session-4"
	initialParams := NewDefaultActiveUploadParameters()
	uploadCtx := &UploadContext{
		TotalBytes:        1024 * 1024 * 100,
		TotalChunks:       10,
		EstimatedDuration: time.Minute * 5,
		Priority:          PriorityNormal,
		ContentType:       "application/octet-stream",
	}

	err := dpa.RegisterUploadSession(sessionID, initialParams, uploadCtx)
	require.NoError(t, err)

	// Update progress
	progressUpdate := &SessionProgressUpdate{
		UploadedBytes:     1024 * 1024 * 50,
		CurrentThroughput: 15.0,
		LatencyMs:         30.0,
		ErrorRate:         0.01,
		CompletedChunks:   5,
		FailedChunks:      0,
		ActiveConnections: 8,
	}

	err = dpa.UpdateSessionProgress(sessionID, progressUpdate)
	require.NoError(t, err)

	// Get session status
	status, err := dpa.GetSessionStatus(sessionID)
	require.NoError(t, err)
	assert.NotNil(t, status)

	assert.Equal(t, sessionID, status.SessionID)
	assert.Equal(t, 0.5, status.UploadProgress)
	assert.Equal(t, 15.0, status.CurrentThroughput)
	assert.Greater(t, status.AverageThroughput, 0.0)
	assert.Greater(t, status.EstimatedTimeLeft, time.Duration(0))
	assert.Equal(t, 0.01, status.ErrorRate)
	assert.Equal(t, 0, status.AdjustmentCount)
	assert.NotNil(t, status.CurrentParameters)
	assert.False(t, status.TransitionInProgress)
	assert.True(t, status.AdjustmentEnabled)

	// Test status for non-existent session
	_, err = dpa.GetSessionStatus("non-existent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestDynamicParameterAdjusterUnregisterUploadSession(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	po := NewRealTimeParameterOptimizer(ctx, nm)
	defer func() { _ = po.Shutdown() }()

	dpa := NewDynamicParameterAdjuster(ctx, nm, po)
	defer func() { _ = dpa.Shutdown() }()

	sessionID := "test-session-5"
	initialParams := NewDefaultActiveUploadParameters()
	uploadCtx := &UploadContext{
		TotalBytes:        1024 * 1024 * 100,
		TotalChunks:       10,
		EstimatedDuration: time.Minute * 5,
		Priority:          PriorityNormal,
		ContentType:       "application/octet-stream",
	}

	err := dpa.RegisterUploadSession(sessionID, initialParams, uploadCtx)
	require.NoError(t, err)

	// Verify session exists
	assert.Contains(t, dpa.activeSessions, sessionID)
	assert.Contains(t, dpa.sessionMetrics, sessionID)

	// Unregister session
	err = dpa.UnregisterUploadSession(sessionID)
	require.NoError(t, err)

	// Verify session was removed
	assert.NotContains(t, dpa.activeSessions, sessionID)
	assert.NotContains(t, dpa.sessionMetrics, sessionID)

	// Test unregistering non-existent session
	err = dpa.UnregisterUploadSession("non-existent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestDynamicParameterAdjusterShutdown(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	po := NewRealTimeParameterOptimizer(ctx, nm)
	defer func() { _ = po.Shutdown() }()

	dpa := NewDynamicParameterAdjuster(ctx, nm, po)

	// Register a session
	sessionID := "test-session-6"
	initialParams := NewDefaultActiveUploadParameters()
	uploadCtx := &UploadContext{
		TotalBytes:        1024 * 1024 * 100,
		TotalChunks:       10,
		EstimatedDuration: time.Minute * 5,
		Priority:          PriorityNormal,
		ContentType:       "application/octet-stream",
	}

	err := dpa.RegisterUploadSession(sessionID, initialParams, uploadCtx)
	require.NoError(t, err)

	// Verify initial state
	assert.True(t, dpa.adjustmentActive)
	assert.Contains(t, dpa.activeSessions, sessionID)

	// Shutdown
	err = dpa.Shutdown()
	assert.NoError(t, err)
	assert.False(t, dpa.adjustmentActive)

	// Verify context is cancelled
	select {
	case <-dpa.ctx.Done():
		// Context properly cancelled
	default:
		t.Error("Context should be cancelled after shutdown")
	}

	// Verify sessions were cleaned up
	assert.Equal(t, 0, len(dpa.activeSessions))
	assert.Equal(t, 0, len(dpa.sessionMetrics))
}

func TestNewDefaultActiveUploadParameters(t *testing.T) {
	params := NewDefaultActiveUploadParameters()

	assert.NotNil(t, params)
	assert.Equal(t, 16.0, params.ChunkSizeMB)
	assert.Equal(t, 8, params.ConcurrentConnections)
	assert.Equal(t, 30.0, params.RequestTimeoutSec)
	assert.Equal(t, 3, params.RetryAttempts)
	assert.Equal(t, 1000.0, params.RetryBackoffMs)
	assert.Equal(t, 10, params.ConnectionPoolSize)
	assert.Equal(t, 256.0, params.BufferSizeMB)
	assert.Equal(t, 6, params.CompressionLevel)
	assert.Equal(t, 65536, params.TCPWindowSize)
	assert.Equal(t, time.Second*30, params.KeepAliveInterval)
	assert.Equal(t, time.Second*10, params.ConnectionTimeout)
	assert.Equal(t, time.Second*30, params.IdleTimeout)
	assert.Equal(t, PriorityNormal, params.PriorityLevel)
	assert.Equal(t, 0.8, params.ResourceAllocation)
	assert.Equal(t, 0.0, params.BandwidthLimit)
	assert.Equal(t, int64(1), params.ParameterVersion)
	assert.NotZero(t, params.LastModified)
	assert.Equal(t, "initial_parameters", params.ModificationReason)
	assert.Equal(t, 0.0, params.ExpectedImprovement)
}

func TestActiveUploadParametersCopy(t *testing.T) {
	original := NewDefaultActiveUploadParameters()
	original.ChunkSizeMB = 32.0
	original.ConcurrentConnections = 16

	copied := copyActiveUploadParameters(original)

	assert.Equal(t, original.ChunkSizeMB, copied.ChunkSizeMB)
	assert.Equal(t, original.ConcurrentConnections, copied.ConcurrentConnections)

	// Verify it's a deep copy
	copied.ChunkSizeMB = 64.0
	assert.NotEqual(t, original.ChunkSizeMB, copied.ChunkSizeMB)

	// Test copy with nil
	nilCopy := copyActiveUploadParameters(nil)
	assert.NotNil(t, nilCopy)
	assert.Equal(t, 16.0, nilCopy.ChunkSizeMB)
}

func TestParameterValidation(t *testing.T) {
	// Test valid parameter names
	validParams := []string{
		"ChunkSizeMB", "ConcurrentConnections", "RequestTimeoutSec",
		"RetryAttempts", "RetryBackoffMs", "ConnectionPoolSize",
		"BufferSizeMB", "CompressionLevel", "TCPWindowSize",
		"KeepAliveInterval", "ConnectionTimeout", "IdleTimeout",
		"ResourceAllocation", "BandwidthLimit",
	}

	for _, param := range validParams {
		assert.True(t, isValidParameterName(param), "Parameter %s should be valid", param)
	}

	// Test invalid parameter names
	invalidParams := []string{"InvalidParam", "NonExistent", ""}
	for _, param := range invalidParams {
		assert.False(t, isValidParameterName(param), "Parameter %s should be invalid", param)
	}
}

func TestParameterGetSetValues(t *testing.T) {
	params := NewDefaultActiveUploadParameters()

	// Test getting values
	chunkSize := getParameterValue(params, "ChunkSizeMB")
	assert.Equal(t, 16.0, chunkSize)

	concurrency := getParameterValue(params, "ConcurrentConnections")
	assert.Equal(t, 8, concurrency)

	timeout := getParameterValue(params, "RequestTimeoutSec")
	assert.Equal(t, 30.0, timeout)

	// Test getting invalid parameter
	invalid := getParameterValue(params, "InvalidParameter")
	assert.Nil(t, invalid)

	// Test setting values
	err := setParameterValue(params, "ChunkSizeMB", 32.0)
	assert.NoError(t, err)
	assert.Equal(t, 32.0, params.ChunkSizeMB)

	err = setParameterValue(params, "ConcurrentConnections", 16)
	assert.NoError(t, err)
	assert.Equal(t, 16, params.ConcurrentConnections)

	// Test setting invalid parameter
	err = setParameterValue(params, "InvalidParameter", "value")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown parameter")

	// Test setting wrong type
	err = setParameterValue(params, "ChunkSizeMB", "invalid_type")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid type")
}

func TestInterpolateValue(t *testing.T) {
	// Test float64 interpolation
	result := interpolateValue(10.0, 20.0, 0.5)
	assert.Equal(t, 15.0, result)

	result = interpolateValue(10.0, 20.0, 0.0)
	assert.Equal(t, 10.0, result)

	result = interpolateValue(10.0, 20.0, 1.0)
	assert.Equal(t, 20.0, result)

	// Test int interpolation
	result = interpolateValue(10, 20, 0.5)
	assert.Equal(t, 15, result)

	// Test unsupported type fallback
	result = interpolateValue("start", "end", 0.5)
	assert.Equal(t, "end", result)
}

func TestAdjustmentTypes(t *testing.T) {
	// Test AdjustmentStrategy enum values
	assert.Equal(t, "gradual", string(AdjustmentGradual))
	assert.Equal(t, "adaptive", string(AdjustmentAdaptive))
	assert.Equal(t, "aggressive", string(AdjustmentAggressive))
	assert.Equal(t, "conservative", string(AdjustmentConservative))
	assert.Equal(t, "emergency", string(AdjustmentEmergency))

	// Test AdjustmentAlgorithm enum values
	assert.Equal(t, "hill_climbing", string(AlgorithmHillClimbing))
	assert.Equal(t, "gradient_based", string(AlgorithmGradientBased))
	assert.Equal(t, "reinforcement_ml", string(AlgorithmReinforcementML))
	assert.Equal(t, "rule_based", string(AlgorithmRuleBased))
	assert.Equal(t, "hybrid_ml", string(AlgorithmHybridML))

	// Test UploadPriority enum values
	assert.Equal(t, "low", string(PriorityLow))
	assert.Equal(t, "normal", string(PriorityNormal))
	assert.Equal(t, "high", string(PriorityHigh))
	assert.Equal(t, "critical", string(PriorityCritical))

	// Test TransitionStrategy enum values
	assert.Equal(t, "immediate", string(TransitionImmediate))
	assert.Equal(t, "gradual", string(TransitionGradual))
	assert.Equal(t, "phased", string(TransitionPhased))
	assert.Equal(t, "queued", string(TransitionQueued))

	// Test AdjustmentType enum values
	assert.Equal(t, "increase", string(AdjustmentIncrease))
	assert.Equal(t, "decrease", string(AdjustmentDecrease))
	assert.Equal(t, "optimize", string(AdjustmentOptimize))
	assert.Equal(t, "emergency", string(AdjustmentEmergency))
	assert.Equal(t, "rollback", string(AdjustmentRollback))

	// Test ChunkStatus enum values
	assert.Equal(t, "pending", string(ChunkStatusPending))
	assert.Equal(t, "uploading", string(ChunkStatusUploading))
	assert.Equal(t, "completed", string(ChunkStatusCompleted))
	assert.Equal(t, "failed", string(ChunkStatusFailed))
	assert.Equal(t, "retrying", string(ChunkStatusRetrying))
	assert.Equal(t, "cancelled", string(ChunkStatusCancelled))
}

func TestSupportingConstructors(t *testing.T) {
	// Test NewUploadSessionManager
	usm := NewUploadSessionManager()
	assert.NotNil(t, usm)
	assert.NotNil(t, usm.sessions)

	// Test NewParameterAdjustmentEngine
	pae := NewParameterAdjustmentEngine()
	assert.NotNil(t, pae)
	assert.Equal(t, AlgorithmHybridML, pae.algorithm)
	assert.Equal(t, 0.5, pae.aggressiveness)
	assert.Equal(t, 0.7, pae.cautionLevel)
	assert.NotNil(t, pae.gradualAdjustment)
	assert.NotNil(t, pae.adaptiveAdjustment)
	assert.NotNil(t, pae.emergencyAdjustment)
	assert.NotNil(t, pae.predictiveAdjustment)
	assert.NotNil(t, pae.constraintEnforcer)
	assert.NotNil(t, pae.safetyLimits)
	assert.NotNil(t, pae.rollbackMechanism)
	assert.NotNil(t, pae.decisionTree)
	assert.NotNil(t, pae.impactPredictor)
	assert.NotNil(t, pae.conflictResolver)

	// Test NewGracefulTransitionController
	gtc := NewGracefulTransitionController()
	assert.NotNil(t, gtc)
	assert.NotNil(t, gtc.transitionStrategies)
	assert.NotNil(t, gtc.activeTransitions)
	assert.NotNil(t, gtc.transitionQueue)
	assert.Equal(t, 3, gtc.maxConcurrentTransitions)
	assert.Equal(t, time.Minute*5, gtc.transitionTimeout)
	assert.Equal(t, time.Minute*2, gtc.rollbackTimeout)
	assert.NotNil(t, gtc.transitionValidator)
	assert.NotNil(t, gtc.stateManager)
	assert.NotNil(t, gtc.coordinationEngine)

	// Test NewAdjustmentPerformanceTracker
	apt := NewAdjustmentPerformanceTracker()
	assert.NotNil(t, apt)
	assert.NotNil(t, apt.sessionMetrics)

	// Test NewAdjustmentImpactAnalyzer
	aia := NewAdjustmentImpactAnalyzer()
	assert.NotNil(t, aia)
	assert.NotNil(t, aia.impactModels)

	// Test NewAdjustmentRiskAssessment
	ara := NewAdjustmentRiskAssessment()
	assert.NotNil(t, ara)
	assert.NotNil(t, ara.riskFactors)
}

func TestAdjustmentEngineGenerateRecommendations(t *testing.T) {
	pae := NewParameterAdjustmentEngine()

	session := &ManagedUploadSession{
		ID:                "test-session",
		CurrentThroughput: 5.0,  // Low throughput
		AverageThroughput: 10.0, // Higher average
		ErrorRate:         0.08, // High error rate
		CurrentParameters: NewDefaultActiveUploadParameters(),
	}

	networkConditions := &RealTimeNetworkConditions{
		ConnectionStability: 0.95, // High stability
		BandwidthMBps:       100.0,
		LatencyMs:           30.0,
		NetworkQuality:      0.8,
	}

	optimizationParams := NewDefaultRealTimeOptimizationParameters()

	recommendations := pae.GenerateAdjustmentRecommendations(session, networkConditions, optimizationParams)

	assert.Greater(t, len(recommendations), 0)

	// Should recommend increasing concurrency due to stable network and low throughput
	foundConcurrencyRecommendation := false
	foundChunkSizeRecommendation := false

	for _, rec := range recommendations {
		if rec.ParameterName == "ConcurrentConnections" {
			foundConcurrencyRecommendation = true
			assert.Greater(t, rec.RecommendedValue.(int), session.CurrentParameters.ConcurrentConnections)
			assert.Contains(t, rec.Reason, "increase concurrency")
		}
		if rec.ParameterName == "ChunkSizeMB" {
			foundChunkSizeRecommendation = true
			assert.Less(t, rec.RecommendedValue.(float64), session.CurrentParameters.ChunkSizeMB)
			assert.Contains(t, rec.Reason, "reduce chunk size")
		}
	}

	assert.True(t, foundConcurrencyRecommendation, "Should recommend increasing concurrency")
	assert.True(t, foundChunkSizeRecommendation, "Should recommend reducing chunk size due to high error rate")
}

func TestConstraintEnforcerValidateParameterValue(t *testing.T) {
	ace := &AdjustmentConstraintEnforcer{}

	// Test ChunkSizeMB validation
	assert.True(t, ace.ValidateParameterValue("ChunkSizeMB", 16.0))
	assert.True(t, ace.ValidateParameterValue("ChunkSizeMB", 1.0))
	assert.True(t, ace.ValidateParameterValue("ChunkSizeMB", 64.0))
	assert.False(t, ace.ValidateParameterValue("ChunkSizeMB", 0.5))
	assert.False(t, ace.ValidateParameterValue("ChunkSizeMB", 128.0))

	// Test ConcurrentConnections validation
	assert.True(t, ace.ValidateParameterValue("ConcurrentConnections", 8))
	assert.True(t, ace.ValidateParameterValue("ConcurrentConnections", 1))
	assert.True(t, ace.ValidateParameterValue("ConcurrentConnections", 32))
	assert.False(t, ace.ValidateParameterValue("ConcurrentConnections", 0))
	assert.False(t, ace.ValidateParameterValue("ConcurrentConnections", 64))

	// Test RequestTimeoutSec validation
	assert.True(t, ace.ValidateParameterValue("RequestTimeoutSec", 30.0))
	assert.True(t, ace.ValidateParameterValue("RequestTimeoutSec", 5.0))
	assert.True(t, ace.ValidateParameterValue("RequestTimeoutSec", 300.0))
	assert.False(t, ace.ValidateParameterValue("RequestTimeoutSec", 2.0))
	assert.False(t, ace.ValidateParameterValue("RequestTimeoutSec", 500.0))

	// Test unknown parameter (should return true)
	assert.True(t, ace.ValidateParameterValue("UnknownParameter", "any_value"))
}

func TestSafetyLimitsCheckSafetyLimits(t *testing.T) {
	asl := &AdjustmentSafetyLimits{}

	session := &ManagedUploadSession{
		AdjustmentCount: 5,
		StartTime:       time.Now().Add(-time.Minute * 15), // 15 minutes ago
	}

	request := &ParameterAdjustmentRequest{
		ParameterName: "ConcurrentConnections",
		NewValue:      12,
	}

	// Should pass safety limits
	assert.True(t, asl.CheckSafetyLimits(session, request))

	// Too many adjustments in short time
	session.AdjustmentCount = 15
	session.StartTime = time.Now().Add(-time.Minute * 5) // 5 minutes ago
	assert.False(t, asl.CheckSafetyLimits(session, request))
}

func TestTransitionControllerCreateTransitionPlan(t *testing.T) {
	gtc := NewGracefulTransitionController()

	session := &ManagedUploadSession{
		ID:                "test-session",
		CurrentParameters: NewDefaultActiveUploadParameters(),
	}

	// Test gradual transition for most parameters
	request := &ParameterAdjustmentRequest{
		ParameterName: "ConcurrentConnections",
		NewValue:      12,
	}

	plan := gtc.CreateTransitionPlan(session, request)
	assert.NotNil(t, plan)
	assert.Equal(t, TransitionGradual, plan.Strategy)
	assert.Equal(t, time.Second*30, plan.TotalDuration)
	assert.Equal(t, 5, plan.TransitionSteps)
	assert.NotNil(t, plan.RollbackPlan)
	assert.True(t, plan.RollbackPlan.Enabled)

	// Test immediate transition for compression level
	request.ParameterName = "CompressionLevel"
	request.NewValue = 8

	plan = gtc.CreateTransitionPlan(session, request)
	assert.Equal(t, TransitionImmediate, plan.Strategy)
	assert.Equal(t, time.Millisecond*100, plan.TotalDuration)
	assert.Equal(t, 1, plan.TransitionSteps)
}

func TestImpactAnalyzerAssessAdjustmentImpact(t *testing.T) {
	aia := NewAdjustmentImpactAnalyzer()

	session := &ManagedUploadSession{
		ID:                "test-session",
		CurrentParameters: NewDefaultActiveUploadParameters(),
	}

	request := &ParameterAdjustmentRequest{
		ParameterName: "ConcurrentConnections",
		NewValue:      12,
	}

	assessment := aia.AssessAdjustmentImpact(session, request)
	assert.NotNil(t, assessment)
	assert.Equal(t, 0.1, assessment.ExpectedImprovement)
	assert.Equal(t, 0.7, assessment.ConfidenceLevel)
	assert.Equal(t, 0.2, assessment.RiskLevel)
	assert.NotNil(t, assessment.SideEffects)
}

func TestRiskAssessmentAssessAdjustmentRisk(t *testing.T) {
	ara := NewAdjustmentRiskAssessment()

	session := &ManagedUploadSession{
		ID:                "test-session",
		AdjustmentCount:   3,
		UploadProgress:    0.5, // 50% complete
		CurrentParameters: NewDefaultActiveUploadParameters(),
	}

	request := &ParameterAdjustmentRequest{
		ParameterName: "ConcurrentConnections",
		NewValue:      12,
	}

	risk := ara.AssessAdjustmentRisk(session, request)
	assert.GreaterOrEqual(t, risk, 0.0)
	assert.LessOrEqual(t, risk, 1.0)
	assert.InDelta(t, 0.1, risk, 0.001) // Base risk for this scenario

	// Test higher risk scenarios
	session.AdjustmentCount = 8 // Many adjustments
	risk = ara.AssessAdjustmentRisk(session, request)
	assert.InDelta(t, 0.3, risk, 0.001) // Base + adjustment penalty

	session.UploadProgress = 0.95 // Nearly complete
	risk = ara.AssessAdjustmentRisk(session, request)
	assert.InDelta(t, 0.6, risk, 0.001) // Base + adjustment + completion penalties
}

func TestPerformanceTrackerRecordSessionMetrics(t *testing.T) {
	apt := NewAdjustmentPerformanceTracker()

	sessionID := "test-session"
	metrics := &SessionPerformanceMetrics{
		SessionID:      sessionID,
		Timestamp:      time.Now(),
		ThroughputMBps: 15.0,
		LatencyMs:      25.0,
		ErrorRate:      0.02,
		UploadProgress: 0.6,
	}

	// Record metrics
	apt.RecordSessionMetrics(sessionID, metrics)

	// Verify metrics were recorded
	assert.Contains(t, apt.sessionMetrics, sessionID)
	assert.Equal(t, 1, len(apt.sessionMetrics[sessionID]))
	assert.Equal(t, metrics, apt.sessionMetrics[sessionID][0])

	// Record more metrics to test history limit
	for i := 0; i < 150; i++ {
		newMetrics := &SessionPerformanceMetrics{
			SessionID:      sessionID,
			Timestamp:      time.Now(),
			ThroughputMBps: float64(i),
		}
		apt.RecordSessionMetrics(sessionID, newMetrics)
	}

	// Verify history is limited to 100
	assert.LessOrEqual(t, len(apt.sessionMetrics[sessionID]), 100)
}

func TestDynamicParameterAdjusterCompleteWorkflow(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	po := NewRealTimeParameterOptimizer(ctx, nm)
	defer func() { _ = po.Shutdown() }()

	dpa := NewDynamicParameterAdjuster(ctx, nm, po)
	defer func() { _ = dpa.Shutdown() }()

	// Start network monitoring
	err := nm.StartMonitoring()
	require.NoError(t, err)
	defer func() { _ = nm.StopMonitoring() }()

	// Register session
	sessionID := "workflow-test"
	initialParams := NewDefaultActiveUploadParameters()
	uploadCtx := &UploadContext{
		TotalBytes:        1024 * 1024 * 200, // 200MB
		TotalChunks:       20,
		EstimatedDuration: time.Minute * 10,
		Priority:          PriorityHigh,
		ContentType:       "application/octet-stream",
	}

	err = dpa.RegisterUploadSession(sessionID, initialParams, uploadCtx)
	require.NoError(t, err)

	// Simulate upload progress with performance issues
	progressUpdates := []*SessionProgressUpdate{
		{
			UploadedBytes:     1024 * 1024 * 40, // 40MB
			CurrentThroughput: 8.0,              // Good throughput
			LatencyMs:         30.0,
			ErrorRate:         0.01,
			CompletedChunks:   4,
			FailedChunks:      0,
			ActiveConnections: 8,
		},
		{
			UploadedBytes:     1024 * 1024 * 80, // 80MB
			CurrentThroughput: 4.0,              // Degraded throughput
			LatencyMs:         60.0,
			ErrorRate:         0.03,
			CompletedChunks:   8,
			FailedChunks:      1,
			ActiveConnections: 6,
		},
		{
			UploadedBytes:     1024 * 1024 * 120, // 120MB
			CurrentThroughput: 2.0,               // Poor throughput
			LatencyMs:         100.0,
			ErrorRate:         0.08, // High error rate
			CompletedChunks:   10,
			FailedChunks:      3,
			ActiveConnections: 4,
		},
	}

	for i, update := range progressUpdates {
		err = dpa.UpdateSessionProgress(sessionID, update)
		require.NoError(t, err)

		// Check session status after each update
		status, err := dpa.GetSessionStatus(sessionID)
		require.NoError(t, err)

		expectedProgress := float64(update.UploadedBytes) / float64(uploadCtx.TotalBytes)
		assert.InDelta(t, expectedProgress, status.UploadProgress, 0.01)
		assert.Equal(t, update.CurrentThroughput, status.CurrentThroughput)
		assert.Equal(t, update.ErrorRate, status.ErrorRate)

		// For the final update with high error rate, manually trigger adjustment
		if i == len(progressUpdates)-1 {
			adjustmentRequest := &ParameterAdjustmentRequest{
				ParameterName: "ChunkSizeMB",
				NewValue:      8.0, // Reduce chunk size due to high error rate
				Type:          AdjustmentOptimize,
				Reason:        "high_error_rate_mitigation",
				Priority:      8,
			}

			result, err := dpa.AdjustSessionParameters(sessionID, adjustmentRequest)
			require.NoError(t, err)
			assert.True(t, result.Success)

			// Verify adjustment was applied
			updatedStatus, err := dpa.GetSessionStatus(sessionID)
			require.NoError(t, err)
			assert.Equal(t, 8.0, updatedStatus.CurrentParameters.ChunkSizeMB)
			assert.Equal(t, 1, updatedStatus.AdjustmentCount)
		}
	}

	// Verify adjustment history
	assert.Greater(t, len(dpa.adjustmentHistory), 0)

	// Final status check
	finalStatus, err := dpa.GetSessionStatus(sessionID)
	require.NoError(t, err)
	assert.Greater(t, finalStatus.UploadProgress, 0.5)
	assert.Equal(t, 1, finalStatus.AdjustmentCount)
	assert.Equal(t, 8.0, finalStatus.CurrentParameters.ChunkSizeMB)

	// Unregister session
	err = dpa.UnregisterUploadSession(sessionID)
	require.NoError(t, err)
}
