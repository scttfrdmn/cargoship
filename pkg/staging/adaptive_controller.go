package staging

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"
)

// AdaptiveTransferController manages dynamic upload parameter tuning based on network feedback.
type AdaptiveTransferController struct {
	config              *AdaptationConfig
	activeTransfers     map[string]*TransferSession
	transferCallbacks   []TransferCallback
	parameterHistory    *ParameterHistory
	performanceTracker  *TransferPerformanceTracker
	mu                  sync.RWMutex
	ctx                 context.Context
	cancel              context.CancelFunc
	active              bool
}

// TransferSession represents an active transfer session that can be adapted.
type TransferSession struct {
	ID                    string
	StartTime             time.Time
	CurrentParameters     *TransferParameters
	PerformanceHistory    []*PerformanceSnapshot
	NetworkHistory        []*NetworkCondition
	AdaptationCount       int
	LastAdaptation        time.Time
	TransferredBytes      int64
	TotalBytes            int64
	Active                bool
	AdaptationCallbacks   []func(*TransferParameters, *TransferParameters)
}

// TransferParameters represents parameters that can be dynamically adjusted.
type TransferParameters struct {
	ChunkSizeMB         int
	Concurrency         int
	CompressionLevel    string
	BufferSizeMB        int
	RetryPolicy         *RetryPolicy
	TimeoutSettings     *TimeoutSettings
	FlowControlSettings *FlowControlSettings
}

// RetryPolicy defines retry behavior for failed transfers.
type RetryPolicy struct {
	MaxRetries      int
	InitialDelay    time.Duration
	BackoffFactor   float64
	MaxDelay        time.Duration
	JitterEnabled   bool
}

// TimeoutSettings defines timeout behavior for transfers.
type TimeoutSettings struct {
	ConnectionTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

// FlowControlSettings defines flow control behavior.
type FlowControlSettings struct {
	WindowSize          int
	CongestionWindow    int
	SlowStartThreshold  int
	CongestionAlgorithm string
}

// PerformanceSnapshot captures transfer performance at a point in time.
type PerformanceSnapshot struct {
	Timestamp         time.Time
	ThroughputMBps    float64
	LatencyMs         float64
	ErrorRate         float64
	RetryRate         float64
	CongestionWindow  int
	NetworkCondition  *NetworkCondition
	ActiveParameters  *TransferParameters
}

// TransferCallback is called when transfer parameters are adapted.
type TransferCallback func(sessionID string, oldParams, newParams *TransferParameters) error

// NewAdaptiveTransferController creates a new adaptive transfer controller.
func NewAdaptiveTransferController(config *AdaptationConfig) *AdaptiveTransferController {
	return &AdaptiveTransferController{
		config:              config,
		activeTransfers:     make(map[string]*TransferSession),
		transferCallbacks:   make([]TransferCallback, 0),
		parameterHistory:    NewParameterHistory(),
		performanceTracker:  NewTransferPerformanceTracker(),
		active:              false,
	}
}

// Start begins the adaptive transfer controller.
func (atc *AdaptiveTransferController) Start(ctx context.Context) error {
	atc.mu.Lock()
	defer atc.mu.Unlock()
	
	if atc.active {
		return nil
	}
	
	atc.ctx, atc.cancel = context.WithCancel(ctx)
	atc.active = true
	
	// Start performance monitoring loop
	go atc.performanceMonitoringLoop(atc.ctx)
	
	return nil
}

// Stop gracefully shuts down the adaptive transfer controller.
func (atc *AdaptiveTransferController) Stop() error {
	atc.mu.Lock()
	defer atc.mu.Unlock()
	
	if !atc.active {
		return nil
	}
	
	atc.active = false
	atc.cancel()
	
	return nil
}

// StartTransferSession begins tracking a new transfer session.
func (atc *AdaptiveTransferController) StartTransferSession(sessionID string, totalBytes int64, initialParams *TransferParameters) error {
	atc.mu.Lock()
	defer atc.mu.Unlock()
	
	if initialParams == nil {
		initialParams = DefaultTransferParameters()
	}
	
	session := &TransferSession{
		ID:                  sessionID,
		StartTime:           time.Now(),
		CurrentParameters:   initialParams,
		PerformanceHistory:  make([]*PerformanceSnapshot, 0),
		NetworkHistory:      make([]*NetworkCondition, 0),
		AdaptationCount:     0,
		TotalBytes:          totalBytes,
		TransferredBytes:    0,
		Active:              true,
		AdaptationCallbacks: make([]func(*TransferParameters, *TransferParameters), 0),
	}
	
	atc.activeTransfers[sessionID] = session
	
	return nil
}

// EndTransferSession ends tracking of a transfer session.
func (atc *AdaptiveTransferController) EndTransferSession(sessionID string) error {
	atc.mu.Lock()
	defer atc.mu.Unlock()
	
	session, exists := atc.activeTransfers[sessionID]
	if !exists {
		return fmt.Errorf("transfer session %s not found", sessionID)
	}
	
	session.Active = false
	
	// Record session performance in history
	atc.parameterHistory.RecordSession(session)
	
	// Remove from active sessions
	delete(atc.activeTransfers, sessionID)
	
	return nil
}

// ApplyAdaptation applies a new adaptation state to active transfers.
func (atc *AdaptiveTransferController) ApplyAdaptation(adaptationState *AdaptationState) error {
	atc.mu.Lock()
	defer atc.mu.Unlock()
	
	newParams := &TransferParameters{
		ChunkSizeMB:      adaptationState.ChunkSizeMB,
		Concurrency:      adaptationState.Concurrency,
		CompressionLevel: adaptationState.CompressionLevel,
		BufferSizeMB:     adaptationState.BufferSizeMB,
		RetryPolicy:      DefaultRetryPolicy(),
		TimeoutSettings:  DefaultTimeoutSettings(),
		FlowControlSettings: DefaultFlowControlSettings(),
	}
	
	// Apply to all active transfer sessions
	for sessionID, session := range atc.activeTransfers {
		if session.Active {
			err := atc.adaptTransferSession(sessionID, newParams)
			if err != nil {
				continue // Log error but continue with other sessions
			}
		}
	}
	
	return nil
}

// adaptTransferSession adapts parameters for a specific transfer session.
func (atc *AdaptiveTransferController) adaptTransferSession(sessionID string, newParams *TransferParameters) error {
	session, exists := atc.activeTransfers[sessionID]
	if !exists {
		return fmt.Errorf("transfer session %s not found", sessionID)
	}
	
	oldParams := session.CurrentParameters
	
	// Validate new parameters
	validatedParams := atc.validateParameters(newParams, session)
	
	// Apply parameters
	session.CurrentParameters = validatedParams
	session.AdaptationCount++
	session.LastAdaptation = time.Now()
	
	// Record adaptation in history
	atc.parameterHistory.RecordAdaptation(sessionID, oldParams, validatedParams)
	
	// Notify callbacks
	atc.notifyTransferCallbacks(sessionID, oldParams, validatedParams)
	
	// Notify session callbacks
	for _, callback := range session.AdaptationCallbacks {
		go callback(oldParams, validatedParams)
	}
	
	return nil
}

// validateParameters validates and adjusts parameters for a specific session.
func (atc *AdaptiveTransferController) validateParameters(params *TransferParameters, session *TransferSession) *TransferParameters {
	validated := *params
	
	// Adjust chunk size based on remaining transfer size
	remainingBytes := session.TotalBytes - session.TransferredBytes
	maxReasonableChunks := 10 // Don't create too many small chunks
	
	if remainingBytes > 0 {
		maxChunkSizeMB := int(remainingBytes / (1024 * 1024 * int64(maxReasonableChunks)))
		if maxChunkSizeMB > 0 && validated.ChunkSizeMB > maxChunkSizeMB {
			validated.ChunkSizeMB = maxChunkSizeMB
		}
	}
	
	// Ensure minimum chunk size
	if validated.ChunkSizeMB < atc.config.MinChunkSizeMB {
		validated.ChunkSizeMB = atc.config.MinChunkSizeMB
	}
	
	// Adjust concurrency based on session progress
	transferProgress := float64(session.TransferredBytes) / float64(session.TotalBytes)
	if transferProgress > 0.9 { // Near completion, reduce concurrency for stability
		validated.Concurrency = max(1, validated.Concurrency/2)
	}
	
	return &validated
}

// UpdateTransferProgress updates the progress of a transfer session.
func (atc *AdaptiveTransferController) UpdateTransferProgress(sessionID string, transferredBytes int64, currentThroughput float64, networkCondition *NetworkCondition) error {
	atc.mu.Lock()
	defer atc.mu.Unlock()
	
	session, exists := atc.activeTransfers[sessionID]
	if !exists {
		return fmt.Errorf("transfer session %s not found", sessionID)
	}
	
	session.TransferredBytes = transferredBytes
	
	// Create performance snapshot
	snapshot := &PerformanceSnapshot{
		Timestamp:        time.Now(),
		ThroughputMBps:   currentThroughput,
		LatencyMs:        networkCondition.LatencyMs,
		NetworkCondition: networkCondition,
		ActiveParameters: session.CurrentParameters,
	}
	
	// Add to session history
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
	
	// Update performance tracker
	atc.performanceTracker.RecordPerformance(sessionID, snapshot)
	
	return nil
}

// performanceMonitoringLoop monitors transfer performance and triggers adaptations.
func (atc *AdaptiveTransferController) performanceMonitoringLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second * 10) // Monitor every 10 seconds
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			atc.evaluateSessionPerformance()
		}
	}
}

// evaluateSessionPerformance evaluates performance of all active sessions.
func (atc *AdaptiveTransferController) evaluateSessionPerformance() {
	atc.mu.RLock()
	sessions := make([]*TransferSession, 0, len(atc.activeTransfers))
	for _, session := range atc.activeTransfers {
		if session.Active {
			sessions = append(sessions, session)
		}
	}
	atc.mu.RUnlock()
	
	for _, session := range sessions {
		atc.evaluateSessionAdaptation(session)
	}
}

// evaluateSessionAdaptation evaluates if a session needs parameter adaptation.
func (atc *AdaptiveTransferController) evaluateSessionAdaptation(session *TransferSession) {
	if len(session.PerformanceHistory) < 2 {
		return // Need at least 2 data points
	}
	
	// Check if enough time has passed since last adaptation
	if time.Since(session.LastAdaptation) < time.Minute {
		return // Don't adapt too frequently
	}
	
	// Analyze recent performance
	recentSnapshots := session.PerformanceHistory
	if len(recentSnapshots) > 5 {
		recentSnapshots = recentSnapshots[len(recentSnapshots)-5:]
	}
	
	avgThroughput := atc.calculateAverageThroughput(recentSnapshots)
	throughputTrend := atc.calculateThroughputTrend(recentSnapshots)
	
	// Get current network conditions
	var currentCondition *NetworkCondition
	if len(session.NetworkHistory) > 0 {
		currentCondition = session.NetworkHistory[len(session.NetworkHistory)-1]
	}
	
	// Determine if adaptation is needed
	adaptationNeeded := false
	reason := ""
	
	// Check for poor performance
	expectedThroughput := atc.calculateExpectedThroughput(currentCondition, session.CurrentParameters)
	if avgThroughput < expectedThroughput*0.7 {
		adaptationNeeded = true
		reason = "poor_performance"
	}
	
	// Check for declining performance
	if throughputTrend < -0.1 { // 10% decline
		adaptationNeeded = true
		reason = "declining_performance"
	}
	
	// Check for high error/retry rates
	errorRate := atc.calculateRecentErrorRate(session)
	if errorRate > 0.05 { // 5% error rate
		adaptationNeeded = true
		reason = "high_error_rate"
	}
	
	if adaptationNeeded {
		atc.performSessionAdaptation(session, reason, currentCondition)
	}
}

// performSessionAdaptation performs adaptation for a specific session.
func (atc *AdaptiveTransferController) performSessionAdaptation(session *TransferSession, reason string, condition *NetworkCondition) {
	newParams := atc.generateAdaptedParameters(session, reason, condition)
	if newParams == nil {
		return
	}
	
	atc.mu.Lock()
	err := atc.adaptTransferSession(session.ID, newParams)
	atc.mu.Unlock()
	
	if err != nil {
		// Log error but continue
		return
	}
}

// generateAdaptedParameters generates new parameters based on session analysis.
func (atc *AdaptiveTransferController) generateAdaptedParameters(session *TransferSession, reason string, condition *NetworkCondition) *TransferParameters {
	current := session.CurrentParameters
	newParams := *current
	
	switch reason {
	case "poor_performance":
		newParams = *atc.adaptForPoorPerformance(&newParams, session, condition)
	case "declining_performance":
		newParams = *atc.adaptForDecliningPerformance(&newParams, session, condition)
	case "high_error_rate":
		newParams = *atc.adaptForHighErrors(&newParams, session, condition)
	}
	
	// Validate the new parameters
	return atc.validateParameters(&newParams, session)
}

// adaptForPoorPerformance adapts parameters when performance is poor.
func (atc *AdaptiveTransferController) adaptForPoorPerformance(params *TransferParameters, session *TransferSession, condition *NetworkCondition) *TransferParameters {
	// Try increasing concurrency if network allows
	if condition != nil && condition.CongestionLevel < 0.3 && params.Concurrency < atc.config.MaxConcurrency {
		params.Concurrency++
	}
	
	// Try optimizing chunk size
	if condition != nil {
		optimalChunkSize := atc.calculateOptimalChunkSizeForSession(session, condition)
		params.ChunkSizeMB = optimalChunkSize
	}
	
	// Use faster compression if bottlenecked by CPU
	if condition != nil && condition.BandwidthMBps > 50 {
		params.CompressionLevel = "zstd-fast"
	}
	
	return params
}

// adaptForDecliningPerformance adapts parameters when performance is declining.
func (atc *AdaptiveTransferController) adaptForDecliningPerformance(params *TransferParameters, session *TransferSession, condition *NetworkCondition) *TransferParameters {
	// Reduce concurrency to avoid overwhelming the network
	if params.Concurrency > 1 {
		params.Concurrency = max(1, params.Concurrency-1)
	}
	
	// Use smaller chunks for better adaptability
	params.ChunkSizeMB = max(atc.config.MinChunkSizeMB, params.ChunkSizeMB-5)
	
	// Adjust retry policy to be more aggressive
	params.RetryPolicy.MaxRetries = min(params.RetryPolicy.MaxRetries+1, 5)
	params.RetryPolicy.InitialDelay = time.Millisecond * 500
	
	return params
}

// adaptForHighErrors adapts parameters when error rate is high.
func (atc *AdaptiveTransferController) adaptForHighErrors(params *TransferParameters, session *TransferSession, condition *NetworkCondition) *TransferParameters {
	// Reduce concurrency to minimize connection issues
	params.Concurrency = max(1, params.Concurrency/2)
	
	// Use smaller chunks to reduce the impact of failed transfers
	params.ChunkSizeMB = max(atc.config.MinChunkSizeMB, params.ChunkSizeMB/2)
	
	// Increase timeouts
	params.TimeoutSettings.ConnectionTimeout *= 2
	params.TimeoutSettings.ReadTimeout *= 2
	params.TimeoutSettings.WriteTimeout *= 2
	
	// More aggressive retry policy
	params.RetryPolicy.MaxRetries = 5
	params.RetryPolicy.BackoffFactor = 2.0
	params.RetryPolicy.MaxDelay = time.Minute * 5
	
	return params
}

// Helper calculation methods

// calculateAverageThroughput calculates average throughput from snapshots.
func (atc *AdaptiveTransferController) calculateAverageThroughput(snapshots []*PerformanceSnapshot) float64 {
	if len(snapshots) == 0 {
		return 0
	}
	
	total := 0.0
	for _, snapshot := range snapshots {
		total += snapshot.ThroughputMBps
	}
	
	return total / float64(len(snapshots))
}

// calculateThroughputTrend calculates throughput trend from snapshots.
func (atc *AdaptiveTransferController) calculateThroughputTrend(snapshots []*PerformanceSnapshot) float64 {
	if len(snapshots) < 2 {
		return 0
	}
	
	// Simple linear trend calculation
	firstHalf := snapshots[:len(snapshots)/2]
	secondHalf := snapshots[len(snapshots)/2:]
	
	firstAvg := atc.calculateAverageThroughput(firstHalf)
	secondAvg := atc.calculateAverageThroughput(secondHalf)
	
	if firstAvg == 0 {
		return 0
	}
	
	return (secondAvg - firstAvg) / firstAvg
}

// calculateExpectedThroughput calculates expected throughput for given conditions.
func (atc *AdaptiveTransferController) calculateExpectedThroughput(condition *NetworkCondition, params *TransferParameters) float64 {
	if condition == nil {
		return 25.0 // Default expectation
	}
	
	// Start with available bandwidth
	expected := condition.BandwidthMBps
	
	// Adjust for concurrency efficiency
	concurrencyEfficiency := 1.0
	if params.Concurrency > 1 {
		concurrencyEfficiency = 0.8 + 0.2/float64(params.Concurrency)
	}
	expected *= concurrencyEfficiency
	
	// Adjust for network conditions
	expected *= (1.0 - condition.CongestionLevel*0.5)
	expected *= (1.0 - condition.PacketLoss*10.0)
	
	return math.Max(expected, condition.BandwidthMBps*0.1)
}

// calculateRecentErrorRate calculates recent error rate for a session.
func (atc *AdaptiveTransferController) calculateRecentErrorRate(session *TransferSession) float64 {
	// This would be implemented based on actual error tracking
	// For now, return a simplified calculation
	return 0.0
}

// calculateOptimalChunkSizeForSession calculates optimal chunk size for a specific session.
func (atc *AdaptiveTransferController) calculateOptimalChunkSizeForSession(session *TransferSession, condition *NetworkCondition) int {
	// Base calculation on bandwidth-delay product
	bandwidthDelayProduct := condition.BandwidthMBps * (condition.LatencyMs / 1000.0)
	
	// Adjust for remaining transfer size
	remainingMB := (session.TotalBytes - session.TransferredBytes) / (1024 * 1024)
	if remainingMB < int64(bandwidthDelayProduct) && remainingMB > 0 {
		// Don't use chunks larger than remaining data
		bandwidthDelayProduct = float64(remainingMB)
	}
	
	// Apply network condition adjustments
	optimal := int(bandwidthDelayProduct * (1.0 - condition.CongestionLevel*0.3))
	
	// Apply bounds
	if optimal < atc.config.MinChunkSizeMB {
		optimal = atc.config.MinChunkSizeMB
	}
	if optimal > atc.config.MaxChunkSizeMB {
		optimal = atc.config.MaxChunkSizeMB
	}
	
	return optimal
}

// notifyTransferCallbacks notifies registered transfer callbacks.
func (atc *AdaptiveTransferController) notifyTransferCallbacks(sessionID string, oldParams, newParams *TransferParameters) {
	for _, callback := range atc.transferCallbacks {
		go func(cb TransferCallback) {
			_ = cb(sessionID, oldParams, newParams)
		}(callback)
	}
}

// Public API methods

// RegisterTransferCallback registers a callback for transfer parameter changes.
func (atc *AdaptiveTransferController) RegisterTransferCallback(callback TransferCallback) {
	atc.mu.Lock()
	defer atc.mu.Unlock()
	
	atc.transferCallbacks = append(atc.transferCallbacks, callback)
}

// GetActiveTransfers returns information about active transfer sessions.
func (atc *AdaptiveTransferController) GetActiveTransfers() map[string]*TransferSession {
	atc.mu.RLock()
	defer atc.mu.RUnlock()
	
	// Return copies to prevent race conditions
	result := make(map[string]*TransferSession)
	for id, session := range atc.activeTransfers {
		sessionCopy := *session
		result[id] = &sessionCopy
	}
	
	return result
}

// GetTransferSession returns information about a specific transfer session.
func (atc *AdaptiveTransferController) GetTransferSession(sessionID string) (*TransferSession, error) {
	atc.mu.RLock()
	defer atc.mu.RUnlock()
	
	session, exists := atc.activeTransfers[sessionID]
	if !exists {
		return nil, fmt.Errorf("transfer session %s not found", sessionID)
	}
	
	// Return a copy
	sessionCopy := *session
	return &sessionCopy, nil
}

// Default parameter functions

// DefaultTransferParameters returns default transfer parameters.
func DefaultTransferParameters() *TransferParameters {
	return &TransferParameters{
		ChunkSizeMB:         32,
		Concurrency:         4,
		CompressionLevel:    "zstd",
		BufferSizeMB:        256,
		RetryPolicy:         DefaultRetryPolicy(),
		TimeoutSettings:     DefaultTimeoutSettings(),
		FlowControlSettings: DefaultFlowControlSettings(),
	}
}

// DefaultRetryPolicy returns default retry policy.
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxRetries:    3,
		InitialDelay:  time.Second,
		BackoffFactor: 2.0,
		MaxDelay:      time.Minute,
		JitterEnabled: true,
	}
}

// DefaultTimeoutSettings returns default timeout settings.
func DefaultTimeoutSettings() *TimeoutSettings {
	return &TimeoutSettings{
		ConnectionTimeout: time.Second * 30,
		ReadTimeout:       time.Minute * 5,
		WriteTimeout:      time.Minute * 5,
		IdleTimeout:       time.Minute * 10,
	}
}

// DefaultFlowControlSettings returns default flow control settings.
func DefaultFlowControlSettings() *FlowControlSettings {
	return &FlowControlSettings{
		WindowSize:          64,
		CongestionWindow:    10,
		SlowStartThreshold:  32,
		CongestionAlgorithm: "cubic",
	}
}

// ParameterHistory tracks the history of parameter adaptations.
type ParameterHistory struct {
	adaptations []*ParameterAdaptationRecord
	sessions    []*TransferSession
	maxHistory  int
	mu          sync.RWMutex
}

// ParameterAdaptationRecord represents a parameter adaptation event.
type ParameterAdaptationRecord struct {
	Timestamp    time.Time
	SessionID    string
	OldParams    *TransferParameters
	NewParams    *TransferParameters
	Reason       string
	Effectiveness float64
}

// NewParameterHistory creates a new parameter history tracker.
func NewParameterHistory() *ParameterHistory {
	return &ParameterHistory{
		adaptations: make([]*ParameterAdaptationRecord, 0),
		sessions:    make([]*TransferSession, 0),
		maxHistory:  500,
	}
}

// RecordAdaptation records a parameter adaptation.
func (ph *ParameterHistory) RecordAdaptation(sessionID string, oldParams, newParams *TransferParameters) {
	ph.mu.Lock()
	defer ph.mu.Unlock()
	
	record := &ParameterAdaptationRecord{
		Timestamp: time.Now(),
		SessionID: sessionID,
		OldParams: oldParams,
		NewParams: newParams,
	}
	
	ph.adaptations = append(ph.adaptations, record)
	
	// Limit history size
	if len(ph.adaptations) > ph.maxHistory {
		// Remove oldest entries to get back to max size
		excess := len(ph.adaptations) - ph.maxHistory
		ph.adaptations = ph.adaptations[excess:]
	}
}

// RecordSession records a completed transfer session.
func (ph *ParameterHistory) RecordSession(session *TransferSession) {
	ph.mu.Lock()
	defer ph.mu.Unlock()
	
	ph.sessions = append(ph.sessions, session)
	
	// Limit session history
	if len(ph.sessions) > ph.maxHistory/5 {
		ph.sessions = ph.sessions[1:]
	}
}

// TransferPerformanceTracker tracks transfer performance across sessions.
type TransferPerformanceTracker struct {
	performanceData map[string][]*PerformanceSnapshot
	mu              sync.RWMutex
}

// NewTransferPerformanceTracker creates a new performance tracker.
func NewTransferPerformanceTracker() *TransferPerformanceTracker {
	return &TransferPerformanceTracker{
		performanceData: make(map[string][]*PerformanceSnapshot),
	}
}

// RecordPerformance records a performance snapshot for a session.
func (tpt *TransferPerformanceTracker) RecordPerformance(sessionID string, snapshot *PerformanceSnapshot) {
	tpt.mu.Lock()
	defer tpt.mu.Unlock()
	
	if tpt.performanceData[sessionID] == nil {
		tpt.performanceData[sessionID] = make([]*PerformanceSnapshot, 0)
	}
	
	tpt.performanceData[sessionID] = append(tpt.performanceData[sessionID], snapshot)
	
	// Limit history per session
	maxPerSession := 200
	if len(tpt.performanceData[sessionID]) > maxPerSession {
		tpt.performanceData[sessionID] = tpt.performanceData[sessionID][1:]
	}
}

// Utility functions

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}