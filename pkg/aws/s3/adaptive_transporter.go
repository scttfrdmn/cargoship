package s3

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	awsconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/scttfrdmn/cargoship/pkg/staging"
)

// AdaptiveTransporter extends staging transporter with real-time network adaptation.
type AdaptiveTransporter struct {
	*StagingTransporter
	adaptationEngine   *staging.NetworkAdaptationEngine
	transferController *staging.AdaptiveTransferController
	bandwidthOptimizer *staging.BandwidthOptimizer
	adaptationConfig   *staging.AdaptationConfig
	activeSessions     map[string]*AdaptiveSession
	logger             *slog.Logger
	mu                 sync.RWMutex
}

// AdaptiveSession represents an upload session with real-time adaptation.
type AdaptiveSession struct {
	ID                  string
	Archive             Archive
	StartTime           time.Time
	TotalSize           int64
	TransferredSize     int64
	CurrentParameters   *staging.TransferParameters
	PerformanceHistory  []*staging.PerformanceSnapshot
	AdaptationHistory   []*staging.AdaptationRecord
	NetworkHistory      []*staging.NetworkCondition
	Active              bool
	LastAdaptation      time.Time
	AdaptationCount     int
}

// AdaptiveTransporterConfig configures the adaptive transporter behavior.
type AdaptiveTransporterConfig struct {
	*StagingConfig
	*staging.AdaptationConfig
	EnableRealTimeAdaptation bool    `yaml:"enable_realtime_adaptation" json:"enable_realtime_adaptation"`
	AdaptationSensitivity    float64 `yaml:"adaptation_sensitivity" json:"adaptation_sensitivity"`
	MinAdaptationInterval    time.Duration `yaml:"min_adaptation_interval" json:"min_adaptation_interval"`
	MaxAdaptationsPerSession int     `yaml:"max_adaptations_per_session" json:"max_adaptations_per_session"`
}

// NewAdaptiveTransporter creates a new adaptive S3 transporter with real-time network adaptation.
func NewAdaptiveTransporter(ctx context.Context, client *s3.Client, s3Config awsconfig.S3Config, config *AdaptiveTransporterConfig, logger *slog.Logger) (*AdaptiveTransporter, error) {
	if config == nil {
		config = DefaultAdaptiveTransporterConfig()
	}
	
	if logger == nil {
		logger = slog.Default()
	}
	
	// Create staging transporter
	stagingTransporter, err := NewStagingTransporter(ctx, client, s3Config, config.StagingConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create staging transporter: %w", err)
	}
	
	at := &AdaptiveTransporter{
		StagingTransporter: stagingTransporter,
		adaptationConfig:   config.AdaptationConfig,
		activeSessions:     make(map[string]*AdaptiveSession),
		logger:             logger,
	}
	
	// Initialize adaptation components if enabled
	if config.EnableRealTimeAdaptation {
		// Initialize network adaptation engine
		at.adaptationEngine = staging.NewNetworkAdaptationEngine(ctx, config.AdaptationConfig)
		
		// Initialize adaptive transfer controller
		at.transferController = staging.NewAdaptiveTransferController(config.AdaptationConfig)
		
		// Initialize bandwidth optimizer
		at.bandwidthOptimizer = staging.NewBandwidthOptimizer(config.AdaptationConfig)
		
		// Register adaptation callbacks
		at.setupAdaptationCallbacks()
		
		// Start adaptation systems
		err = at.startAdaptationSystems(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to start adaptation systems: %w", err)
		}
		
		logger.Info("adaptive S3 transporter initialized",
			"realtime_adaptation", true,
			"adaptation_sensitivity", config.AdaptationSensitivity,
			"min_adaptation_interval", config.MinAdaptationInterval)
	} else {
		logger.Info("adaptive S3 transporter initialized", "realtime_adaptation", false)
	}
	
	return at, nil
}

// DefaultAdaptiveTransporterConfig returns default adaptive configuration.
func DefaultAdaptiveTransporterConfig() *AdaptiveTransporterConfig {
	return &AdaptiveTransporterConfig{
		StagingConfig:            DefaultStagingConfig(),
		AdaptationConfig:         staging.DefaultAdaptationConfig(),
		EnableRealTimeAdaptation: true,
		AdaptationSensitivity:    1.0,
		MinAdaptationInterval:    time.Second * 10,
		MaxAdaptationsPerSession: 10,
	}
}

// startAdaptationSystems starts all adaptation subsystems.
func (at *AdaptiveTransporter) startAdaptationSystems(ctx context.Context) error {
	// Start network adaptation engine
	if err := at.adaptationEngine.Start(); err != nil {
		return fmt.Errorf("failed to start adaptation engine: %w", err)
	}
	
	// Start transfer controller
	if err := at.transferController.Start(ctx); err != nil {
		return fmt.Errorf("failed to start transfer controller: %w", err)
	}
	
	// Start bandwidth optimizer
	if err := at.bandwidthOptimizer.Start(ctx); err != nil {
		return fmt.Errorf("failed to start bandwidth optimizer: %w", err)
	}
	
	return nil
}

// setupAdaptationCallbacks sets up callbacks for adaptation events.
func (at *AdaptiveTransporter) setupAdaptationCallbacks() {
	// Register adaptation callback
	at.adaptationEngine.RegisterAdaptationCallback(func(oldState, newState *staging.AdaptationState) error {
		return at.handleAdaptationChange(oldState, newState)
	})
	
	// Register bandwidth optimization callback
	at.bandwidthOptimizer.RegisterOptimizationCallback(func(util *staging.BandwidthUtilization, rec *staging.OptimizationRecommendation) error {
		return at.handleBandwidthOptimization(util, rec)
	})
	
	// Register transfer callback
	at.transferController.RegisterTransferCallback(func(sessionID string, oldParams, newParams *staging.TransferParameters) error {
		return at.handleTransferParameterChange(sessionID, oldParams, newParams)
	})
}

// UploadWithAdaptation uploads an archive using real-time network adaptation.
func (at *AdaptiveTransporter) UploadWithAdaptation(ctx context.Context, archive Archive) (*UploadResult, error) {
	sessionID := fmt.Sprintf("upload-%d", time.Now().UnixNano())
	
	// Create adaptive session
	session := &AdaptiveSession{
		ID:                  sessionID,
		Archive:             archive,
		StartTime:           time.Now(),
		TotalSize:           archive.Size,
		TransferredSize:     0,
		CurrentParameters:   staging.DefaultTransferParameters(),
		PerformanceHistory:  make([]*staging.PerformanceSnapshot, 0),
		AdaptationHistory:   make([]*staging.AdaptationRecord, 0),
		NetworkHistory:      make([]*staging.NetworkCondition, 0),
		Active:              true,
		LastAdaptation:      time.Now(),
		AdaptationCount:     0,
	}
	
	// Register session
	at.mu.Lock()
	at.activeSessions[sessionID] = session
	at.mu.Unlock()
	
	// Start transfer session tracking
	if at.transferController != nil {
		err := at.transferController.StartTransferSession(sessionID, archive.Size, session.CurrentParameters)
		if err != nil {
			at.logger.Warn("failed to start transfer session tracking", "error", err)
		}
	}
	
	// Perform adaptive upload
	result, err := at.performAdaptiveUpload(ctx, session)
	
	// End session
	at.endAdaptiveSession(sessionID, err == nil)
	
	if err != nil {
		return nil, err
	}
	
	at.logger.Info("adaptive upload completed",
		"session_id", sessionID,
		"duration", result.Duration,
		"throughput_mbps", result.Throughput,
		"adaptations", session.AdaptationCount,
		"final_efficiency", at.calculateSessionEfficiency(session))
	
	return result, nil
}

// performAdaptiveUpload performs the upload with real-time adaptation.
func (at *AdaptiveTransporter) performAdaptiveUpload(ctx context.Context, session *AdaptiveSession) (*UploadResult, error) {
	// Start with regular staging upload but monitor for adaptation opportunities
	adaptiveCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	
	// Start adaptation monitoring for this session
	go at.monitorSessionAdaptation(adaptiveCtx, session)
	
	// Perform upload with staging
	if at.StagingTransporter != nil {
		return at.UploadWithStaging(ctx, session.Archive)
	}
	
	// Fallback to regular upload
	return at.Upload(ctx, session.Archive)
}

// monitorSessionAdaptation monitors a session for adaptation opportunities.
func (at *AdaptiveTransporter) monitorSessionAdaptation(ctx context.Context, session *AdaptiveSession) {
	ticker := time.NewTicker(time.Second * 2) // Monitor every 2 seconds
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			at.updateSessionMetrics(session)
			at.evaluateSessionAdaptation(session)
		}
	}
}

// updateSessionMetrics updates performance metrics for a session.
func (at *AdaptiveTransporter) updateSessionMetrics(session *AdaptiveSession) {
	// Get current network condition
	var networkCondition *staging.NetworkCondition
	if at.adaptationEngine != nil {
		currentAdaptation := at.adaptationEngine.GetCurrentAdaptation()
		if currentAdaptation != nil {
			networkCondition = currentAdaptation.NetworkCondition
		}
	}
	
	if networkCondition == nil {
		// Create default network condition
		networkCondition = &staging.NetworkCondition{
			Timestamp:       time.Now(),
			BandwidthMBps:   50.0,
			LatencyMs:       20.0,
			PacketLoss:      0.001,
			Jitter:          2.0,
			CongestionLevel: 0.1,
			Reliability:     0.95,
		}
	}
	
	// Calculate current throughput
	elapsed := time.Since(session.StartTime).Seconds()
	currentThroughput := 0.0
	if elapsed > 0 {
		currentThroughput = float64(session.TransferredSize) / (1024 * 1024) / elapsed
	}
	
	// Create performance snapshot
	snapshot := &staging.PerformanceSnapshot{
		Timestamp:        time.Now(),
		ThroughputMBps:   currentThroughput,
		LatencyMs:        networkCondition.LatencyMs,
		NetworkCondition: networkCondition,
		ActiveParameters: session.CurrentParameters,
	}
	
	// Add to session history
	at.mu.Lock()
	session.PerformanceHistory = append(session.PerformanceHistory, snapshot)
	session.NetworkHistory = append(session.NetworkHistory, networkCondition)
	
	// Limit history size
	maxHistory := 100
	if len(session.PerformanceHistory) > maxHistory {
		session.PerformanceHistory = session.PerformanceHistory[1:]
	}
	if len(session.NetworkHistory) > maxHistory {
		session.NetworkHistory = session.NetworkHistory[1:]
	}
	at.mu.Unlock()
	
	// Update transfer controller
	if at.transferController != nil {
		err := at.transferController.UpdateTransferProgress(session.ID, session.TransferredSize, currentThroughput, networkCondition)
		if err != nil {
			at.logger.Warn("failed to update transfer progress", "error", err)
		}
	}
}

// evaluateSessionAdaptation evaluates if the session needs adaptation.
func (at *AdaptiveTransporter) evaluateSessionAdaptation(session *AdaptiveSession) {
	at.mu.RLock()
	defer at.mu.RUnlock()
	
	// Check if adaptation is needed
	if !at.shouldAdaptSession(session) {
		return
	}
	
	// Get current adaptation state
	if at.adaptationEngine == nil {
		return
	}
	
	currentAdaptation := at.adaptationEngine.GetCurrentAdaptation()
	if currentAdaptation == nil {
		return
	}
	
	// Apply adaptation to session
	at.applyAdaptationToSession(session, currentAdaptation)
}

// shouldAdaptSession determines if a session should be adapted.
func (at *AdaptiveTransporter) shouldAdaptSession(session *AdaptiveSession) bool {
	// Check adaptation limits
	if session.AdaptationCount >= 10 { // Max 10 adaptations per session
		return false
	}
	
	// Check minimum interval
	if time.Since(session.LastAdaptation) < time.Second*10 {
		return false
	}
	
	// Check if performance is poor
	if len(session.PerformanceHistory) < 3 {
		return false // Need some history
	}
	
	// Calculate recent performance
	recentSnapshots := session.PerformanceHistory
	if len(recentSnapshots) > 5 {
		recentSnapshots = recentSnapshots[len(recentSnapshots)-5:]
	}
	
	avgThroughput := at.calculateAverageThroughput(recentSnapshots)
	expectedThroughput := 30.0 // Expected minimum throughput
	
	// Adapt if performance is significantly below expectations
	return avgThroughput < expectedThroughput*0.7
}

// applyAdaptationToSession applies adaptation state to a session.
func (at *AdaptiveTransporter) applyAdaptationToSession(session *AdaptiveSession, adaptation *staging.AdaptationState) {
	// Update session parameters
	oldParams := session.CurrentParameters
	newParams := &staging.TransferParameters{
		ChunkSizeMB:      adaptation.ChunkSizeMB,
		Concurrency:      adaptation.Concurrency,
		CompressionLevel: adaptation.CompressionLevel,
		BufferSizeMB:     adaptation.BufferSizeMB,
		RetryPolicy:      staging.DefaultRetryPolicy(),
		TimeoutSettings:  staging.DefaultTimeoutSettings(),
		FlowControlSettings: staging.DefaultFlowControlSettings(),
	}
	
	session.CurrentParameters = newParams
	session.LastAdaptation = time.Now()
	session.AdaptationCount++
	
	// Create adaptation record
	record := &staging.AdaptationRecord{
		Timestamp: time.Now(),
		OldState:  &staging.AdaptationState{
			ChunkSizeMB:      oldParams.ChunkSizeMB,
			Concurrency:      oldParams.Concurrency,
			CompressionLevel: oldParams.CompressionLevel,
			BufferSizeMB:     oldParams.BufferSizeMB,
		},
		NewState: adaptation,
		Reason:   adaptation.AdaptationReason,
	}
	
	session.AdaptationHistory = append(session.AdaptationHistory, record)
	
	at.logger.Info("session adapted",
		"session_id", session.ID,
		"reason", adaptation.AdaptationReason,
		"old_chunk_size", oldParams.ChunkSizeMB,
		"new_chunk_size", newParams.ChunkSizeMB,
		"old_concurrency", oldParams.Concurrency,
		"new_concurrency", newParams.Concurrency)
}

// endAdaptiveSession ends an adaptive session.
func (at *AdaptiveTransporter) endAdaptiveSession(sessionID string, success bool) {
	at.mu.Lock()
	session, exists := at.activeSessions[sessionID]
	if exists {
		session.Active = false
		delete(at.activeSessions, sessionID)
	}
	at.mu.Unlock()
	
	// End transfer session tracking
	if at.transferController != nil && exists {
		err := at.transferController.EndTransferSession(sessionID)
		if err != nil {
			at.logger.Warn("failed to end transfer session", "error", err)
		}
	}
	
	if exists {
		at.logger.Debug("adaptive session ended",
			"session_id", sessionID,
			"success", success,
			"duration", time.Since(session.StartTime),
			"adaptations", session.AdaptationCount)
	}
}

// Adaptation event handlers

// handleAdaptationChange handles adaptation state changes.
func (at *AdaptiveTransporter) handleAdaptationChange(oldState, newState *staging.AdaptationState) error {
	at.logger.Debug("network adaptation occurred",
		"reason", newState.AdaptationReason,
		"chunk_size_change", fmt.Sprintf("%d -> %d", oldState.ChunkSizeMB, newState.ChunkSizeMB),
		"concurrency_change", fmt.Sprintf("%d -> %d", oldState.Concurrency, newState.Concurrency),
		"predicted_improvement", newState.PredictedImprovement)
	
	return nil
}

// handleBandwidthOptimization handles bandwidth optimization recommendations.
func (at *AdaptiveTransporter) handleBandwidthOptimization(util *staging.BandwidthUtilization, rec *staging.OptimizationRecommendation) error {
	at.logger.Debug("bandwidth optimization recommendation",
		"reason", rec.Reason,
		"priority", rec.Priority,
		"confidence", rec.Confidence,
		"predicted_improvement", rec.PredictedImprovement,
		"current_utilization", util.UtilizationRatio,
		"efficiency_score", util.EfficiencyScore)
	
	return nil
}

// handleTransferParameterChange handles transfer parameter changes.
func (at *AdaptiveTransporter) handleTransferParameterChange(sessionID string, oldParams, newParams *staging.TransferParameters) error {
	at.logger.Debug("transfer parameters changed",
		"session_id", sessionID,
		"chunk_size_change", fmt.Sprintf("%d -> %d", oldParams.ChunkSizeMB, newParams.ChunkSizeMB),
		"concurrency_change", fmt.Sprintf("%d -> %d", oldParams.Concurrency, newParams.Concurrency),
		"compression_change", fmt.Sprintf("%s -> %s", oldParams.CompressionLevel, newParams.CompressionLevel))
	
	return nil
}

// Helper methods

// calculateAverageThroughput calculates average throughput from performance snapshots.
func (at *AdaptiveTransporter) calculateAverageThroughput(snapshots []*staging.PerformanceSnapshot) float64 {
	if len(snapshots) == 0 {
		return 0
	}
	
	total := 0.0
	for _, snapshot := range snapshots {
		total += snapshot.ThroughputMBps
	}
	
	return total / float64(len(snapshots))
}

// calculateSessionEfficiency calculates the efficiency of a completed session.
func (at *AdaptiveTransporter) calculateSessionEfficiency(session *AdaptiveSession) float64 {
	if len(session.PerformanceHistory) == 0 {
		return 0.5 // Default efficiency
	}
	
	avgThroughput := at.calculateAverageThroughput(session.PerformanceHistory)
	
	// Calculate efficiency based on achieved vs expected throughput
	expectedThroughput := 50.0 // Baseline expectation
	efficiency := avgThroughput / expectedThroughput
	
	// Bonus for successful adaptations
	if session.AdaptationCount > 0 {
		efficiency += float64(session.AdaptationCount) * 0.05 // 5% bonus per adaptation
	}
	
	return math.Min(efficiency, 1.0)
}

// Public API methods

// GetAdaptationMetrics returns current adaptation metrics.
func (at *AdaptiveTransporter) GetAdaptationMetrics() *AdaptationMetrics {
	metrics := &AdaptationMetrics{
		Timestamp: time.Now(),
	}
	
	// Get current adaptation state
	if at.adaptationEngine != nil {
		metrics.CurrentAdaptation = at.adaptationEngine.GetCurrentAdaptation()
	}
	
	// Get bandwidth utilization
	if at.bandwidthOptimizer != nil {
		metrics.BandwidthUtilization = at.bandwidthOptimizer.GetCurrentUtilization()
	}
	
	// Get active sessions
	at.mu.RLock()
	metrics.ActiveSessions = len(at.activeSessions)
	at.mu.RUnlock()
	
	// Get staging metrics
	if at.StagingTransporter != nil {
		metrics.StagingMetrics = at.GetStagingMetrics()
	}
	
	return metrics
}

// GetActiveSessions returns information about active adaptive sessions.
func (at *AdaptiveTransporter) GetActiveSessions() map[string]*AdaptiveSession {
	at.mu.RLock()
	defer at.mu.RUnlock()
	
	// Return copies to prevent race conditions
	result := make(map[string]*AdaptiveSession)
	for id, session := range at.activeSessions {
		sessionCopy := *session
		result[id] = &sessionCopy
	}
	
	return result
}

// ForceAdaptation forces immediate adaptation evaluation.
func (at *AdaptiveTransporter) ForceAdaptation() {
	if at.adaptationEngine != nil {
		at.adaptationEngine.ForceAdaptation()
	}
	if at.bandwidthOptimizer != nil {
		at.bandwidthOptimizer.ForceOptimization()
	}
}

// Stop gracefully shuts down the adaptive transporter.
func (at *AdaptiveTransporter) Stop() error {
	// Stop adaptation systems
	if at.adaptationEngine != nil {
		if err := at.adaptationEngine.Stop(); err != nil {
			at.logger.Warn("failed to stop adaptation engine", "error", err)
		}
	}
	
	if at.transferController != nil {
		if err := at.transferController.Stop(); err != nil {
			at.logger.Warn("failed to stop transfer controller", "error", err)
		}
	}
	
	if at.bandwidthOptimizer != nil {
		if err := at.bandwidthOptimizer.Stop(); err != nil {
			at.logger.Warn("failed to stop bandwidth optimizer", "error", err)
		}
	}
	
	// Stop staging transporter
	if at.StagingTransporter != nil {
		return at.StagingTransporter.Stop()
	}
	
	return nil
}

// AdaptationMetrics provides comprehensive metrics for adaptive transfers.
type AdaptationMetrics struct {
	Timestamp            time.Time
	CurrentAdaptation    *staging.AdaptationState
	BandwidthUtilization *staging.BandwidthUtilization
	StagingMetrics       *staging.StagingMetrics
	ActiveSessions       int
}

