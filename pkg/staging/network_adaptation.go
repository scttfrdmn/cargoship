package staging

import (
	"context"
	"math"
	"sync"
	"time"
)

// NetworkAdaptationEngine provides real-time network condition monitoring and parameter adjustment.
type NetworkAdaptationEngine struct {
	conditionMonitor    *NetworkConditionMonitor
	transferController  *AdaptiveTransferController
	bandwidthOptimizer  *BandwidthOptimizer
	adaptationHistory   *AdaptationHistory
	config              *AdaptationConfig
	currentAdaptation   *AdaptationState
	adaptationCallbacks []AdaptationCallback
	mu                  sync.RWMutex
	ctx                 context.Context
	cancel              context.CancelFunc
	active              bool
}

// AdaptationConfig configures the network adaptation behavior.
type AdaptationConfig struct {
	// Monitoring intervals
	MonitoringInterval      time.Duration `yaml:"monitoring_interval" json:"monitoring_interval"`
	AdaptationInterval      time.Duration `yaml:"adaptation_interval" json:"adaptation_interval"`
	
	// Adaptation thresholds
	BandwidthChangeThreshold float64 `yaml:"bandwidth_change_threshold" json:"bandwidth_change_threshold"`
	LatencyChangeThreshold   float64 `yaml:"latency_change_threshold" json:"latency_change_threshold"`
	LossChangeThreshold      float64 `yaml:"loss_change_threshold" json:"loss_change_threshold"`
	
	// Adaptation limits
	MinChunkSizeMB          int     `yaml:"min_chunk_size_mb" json:"min_chunk_size_mb"`
	MaxChunkSizeMB          int     `yaml:"max_chunk_size_mb" json:"max_chunk_size_mb"`
	MinConcurrency          int     `yaml:"min_concurrency" json:"min_concurrency"`
	MaxConcurrency          int     `yaml:"max_concurrency" json:"max_concurrency"`
	
	// Adaptation sensitivity
	AggressiveAdaptation    bool    `yaml:"aggressive_adaptation" json:"aggressive_adaptation"`
	ConservativeMode        bool    `yaml:"conservative_mode" json:"conservative_mode"`
	AdaptationSensitivity   float64 `yaml:"adaptation_sensitivity" json:"adaptation_sensitivity"`
	
	// Performance targets
	TargetThroughputMBps    float64 `yaml:"target_throughput_mbps" json:"target_throughput_mbps"`
	TargetLatencyMs         float64 `yaml:"target_latency_ms" json:"target_latency_ms"`
	MaxTolerableLoss        float64 `yaml:"max_tolerable_loss" json:"max_tolerable_loss"`
}

// DefaultAdaptationConfig returns default network adaptation configuration.
func DefaultAdaptationConfig() *AdaptationConfig {
	return &AdaptationConfig{
		MonitoringInterval:       time.Second * 2,  // Monitor every 2 seconds
		AdaptationInterval:       time.Second * 5,  // Adapt every 5 seconds
		BandwidthChangeThreshold: 0.1,              // 10% bandwidth change triggers adaptation
		LatencyChangeThreshold:   0.2,              // 20% latency change triggers adaptation
		LossChangeThreshold:      0.001,            // 0.1% loss change triggers adaptation
		MinChunkSizeMB:          5,                 // 5MB minimum chunk size
		MaxChunkSizeMB:          100,               // 100MB maximum chunk size
		MinConcurrency:          1,                 // Minimum 1 concurrent upload
		MaxConcurrency:          8,                 // Maximum 8 concurrent uploads
		AggressiveAdaptation:    false,             // Conservative by default
		ConservativeMode:        true,              // Prefer stability over optimization
		AdaptationSensitivity:   1.0,               // Standard sensitivity
		TargetThroughputMBps:    50.0,              // Target 50 MB/s throughput
		TargetLatencyMs:         50.0,              // Target 50ms latency
		MaxTolerableLoss:        0.01,              // Maximum 1% packet loss
	}
}

// AdaptationState represents the current adaptation state.
type AdaptationState struct {
	Timestamp           time.Time
	ChunkSizeMB         int
	Concurrency         int
	CompressionLevel    string
	BufferSizeMB        int
	NetworkCondition    *NetworkCondition
	PerformanceMetrics  *PerformanceMetrics
	AdaptationReason    string
	AdaptationScore     float64
	PredictedImprovement float64
}

// PerformanceMetrics tracks current transfer performance.
type PerformanceMetrics struct {
	CurrentThroughputMBps  float64
	AverageThroughputMBps  float64
	PeakThroughputMBps     float64
	EffectiveBandwidth     float64
	TransferEfficiency     float64
	RetryRate              float64
	ErrorRate              float64
	CongestionDetected     bool
	NetworkUtilization     float64
	LastUpdate             time.Time
}

// AdaptationCallback is called when network adaptation occurs.
type AdaptationCallback func(oldState, newState *AdaptationState) error

// NewNetworkAdaptationEngine creates a new network adaptation engine.
func NewNetworkAdaptationEngine(ctx context.Context, config *AdaptationConfig) *NetworkAdaptationEngine {
	if config == nil {
		config = DefaultAdaptationConfig()
	}
	
	adaptationCtx, cancel := context.WithCancel(ctx)
	
	engine := &NetworkAdaptationEngine{
		conditionMonitor:   NewNetworkConditionMonitor(nil), // Use default staging config
		adaptationHistory:  NewAdaptationHistory(),
		config:             config,
		currentAdaptation:  NewDefaultAdaptationState(),
		adaptationCallbacks: make([]AdaptationCallback, 0),
		ctx:                adaptationCtx,
		cancel:             cancel,
		active:             false,
	}
	
	// Initialize sub-components
	engine.transferController = NewAdaptiveTransferController(config)
	engine.bandwidthOptimizer = NewBandwidthOptimizer(config)
	
	return engine
}

// Start begins real-time network adaptation.
func (nae *NetworkAdaptationEngine) Start() error {
	nae.mu.Lock()
	defer nae.mu.Unlock()
	
	if nae.active {
		return nil // Already active
	}
	
	nae.active = true
	
	// Start network condition monitoring
	go nae.conditionMonitor.Start(nae.ctx)
	
	// Start adaptation monitoring loop
	go nae.adaptationLoop(nae.ctx)
	
	// Start bandwidth optimization loop
	go func() {
		_ = nae.bandwidthOptimizer.Start(nae.ctx)
	}()
	
	return nil
}

// Stop gracefully shuts down the network adaptation engine.
func (nae *NetworkAdaptationEngine) Stop() error {
	nae.mu.Lock()
	defer nae.mu.Unlock()
	
	if !nae.active {
		return nil // Already stopped
	}
	
	nae.active = false
	nae.cancel()
	
	return nil
}

// adaptationLoop runs the main network adaptation monitoring loop.
func (nae *NetworkAdaptationEngine) adaptationLoop(ctx context.Context) {
	monitorTicker := time.NewTicker(nae.config.MonitoringInterval)
	adaptationTicker := time.NewTicker(nae.config.AdaptationInterval)
	defer monitorTicker.Stop()
	defer adaptationTicker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-monitorTicker.C:
			nae.updatePerformanceMetrics()
		case <-adaptationTicker.C:
			nae.evaluateAndAdapt()
		}
	}
}

// updatePerformanceMetrics updates current performance metrics.
func (nae *NetworkAdaptationEngine) updatePerformanceMetrics() {
	nae.mu.Lock()
	defer nae.mu.Unlock()
	
	// Get current network condition
	networkCondition := nae.conditionMonitor.GetCurrentCondition()
	
	// Update adaptation state with current network condition
	nae.currentAdaptation.NetworkCondition = networkCondition
	nae.currentAdaptation.Timestamp = time.Now()
	
	// Calculate performance metrics
	if nae.currentAdaptation.PerformanceMetrics == nil {
		nae.currentAdaptation.PerformanceMetrics = &PerformanceMetrics{}
	}
	
	metrics := nae.currentAdaptation.PerformanceMetrics
	metrics.CurrentThroughputMBps = networkCondition.BandwidthMBps
	metrics.EffectiveBandwidth = nae.calculateEffectiveBandwidth(networkCondition)
	metrics.CongestionDetected = networkCondition.CongestionLevel > 0.3
	metrics.NetworkUtilization = nae.calculateNetworkUtilization(networkCondition)
	metrics.LastUpdate = time.Now()
	
	// Update averages
	nae.updateThroughputAverages(metrics)
}

// evaluateAndAdapt evaluates current conditions and adapts parameters if needed.
func (nae *NetworkAdaptationEngine) evaluateAndAdapt() {
	nae.mu.Lock()
	defer nae.mu.Unlock()
	
	// Get current network condition
	networkCondition := nae.conditionMonitor.GetCurrentCondition()
	
	// Determine if adaptation is needed
	adaptationNeeded, reason := nae.shouldAdapt(networkCondition)
	if !adaptationNeeded {
		return
	}
	
	// Create new adaptation state
	newState := nae.generateAdaptedState(networkCondition, reason)
	if newState == nil {
		return // No valid adaptation found
	}
	
	// Apply adaptation
	oldState := nae.currentAdaptation
	err := nae.applyAdaptation(oldState, newState)
	if err != nil {
		return // Adaptation failed
	}
	
	// Update current state
	nae.currentAdaptation = newState
	
	// Record adaptation in history
	nae.adaptationHistory.RecordAdaptation(oldState, newState, reason)
	
	// Notify callbacks
	nae.notifyAdaptationCallbacks(oldState, newState)
}

// shouldAdapt determines if network adaptation is needed.
func (nae *NetworkAdaptationEngine) shouldAdapt(networkCondition *NetworkCondition) (bool, string) {
	if nae.currentAdaptation.NetworkCondition == nil {
		return true, "initial_adaptation"
	}
	
	prev := nae.currentAdaptation.NetworkCondition
	
	// Check bandwidth change
	bandwidthChange := math.Abs(networkCondition.BandwidthMBps-prev.BandwidthMBps) / prev.BandwidthMBps
	if bandwidthChange > nae.config.BandwidthChangeThreshold {
		if networkCondition.BandwidthMBps > prev.BandwidthMBps {
			return true, "bandwidth_increase"
		}
		return true, "bandwidth_decrease"
	}
	
	// Check latency change
	latencyChange := math.Abs(networkCondition.LatencyMs-prev.LatencyMs) / prev.LatencyMs
	if latencyChange > nae.config.LatencyChangeThreshold {
		if networkCondition.LatencyMs > prev.LatencyMs {
			return true, "latency_increase"
		}
		return true, "latency_decrease"
	}
	
	// Check packet loss change
	lossChange := math.Abs(networkCondition.PacketLoss - prev.PacketLoss)
	if lossChange > nae.config.LossChangeThreshold {
		if networkCondition.PacketLoss > prev.PacketLoss {
			return true, "packet_loss_increase"
		}
		return true, "packet_loss_decrease"
	}
	
	// Check congestion level change
	if networkCondition.CongestionLevel > 0.5 && prev.CongestionLevel <= 0.5 {
		return true, "congestion_detected"
	}
	if networkCondition.CongestionLevel <= 0.3 && prev.CongestionLevel > 0.3 {
		return true, "congestion_cleared"
	}
	
	// Check if performance is below targets
	if nae.currentAdaptation.PerformanceMetrics != nil {
		metrics := nae.currentAdaptation.PerformanceMetrics
		if metrics.CurrentThroughputMBps < nae.config.TargetThroughputMBps*0.8 {
			return true, "throughput_below_target"
		}
		if networkCondition.LatencyMs > nae.config.TargetLatencyMs*1.5 {
			return true, "latency_above_target"
		}
		if networkCondition.PacketLoss > nae.config.MaxTolerableLoss {
			return true, "loss_above_threshold"
		}
	}
	
	return false, ""
}

// generateAdaptedState generates a new adaptation state based on current conditions.
func (nae *NetworkAdaptationEngine) generateAdaptedState(networkCondition *NetworkCondition, reason string) *AdaptationState {
	newState := &AdaptationState{
		Timestamp:        time.Now(),
		NetworkCondition: networkCondition,
		AdaptationReason: reason,
	}
	
	// Start with current values
	current := nae.currentAdaptation
	newState.ChunkSizeMB = current.ChunkSizeMB
	newState.Concurrency = current.Concurrency
	newState.CompressionLevel = current.CompressionLevel
	newState.BufferSizeMB = current.BufferSizeMB
	
	// Adapt based on reason
	switch reason {
	case "bandwidth_increase":
		newState = nae.adaptForBandwidthIncrease(newState, networkCondition)
	case "bandwidth_decrease":
		newState = nae.adaptForBandwidthDecrease(newState, networkCondition)
	case "latency_increase":
		newState = nae.adaptForLatencyIncrease(newState, networkCondition)
	case "latency_decrease":
		newState = nae.adaptForLatencyDecrease(newState, networkCondition)
	case "congestion_detected":
		newState = nae.adaptForCongestion(newState, networkCondition)
	case "congestion_cleared":
		newState = nae.adaptForCongestionClear(newState, networkCondition)
	case "packet_loss_increase":
		newState = nae.adaptForPacketLoss(newState, networkCondition)
	case "throughput_below_target":
		newState = nae.adaptForLowThroughput(newState, networkCondition)
	default:
		newState = nae.adaptForGeneral(newState, networkCondition)
	}
	
	// Validate adaptation limits
	newState = nae.validateAdaptationLimits(newState)
	
	// Calculate adaptation score
	newState.AdaptationScore = nae.calculateAdaptationScore(newState, networkCondition)
	newState.PredictedImprovement = nae.predictPerformanceImprovement(current, newState)
	
	// Only return adaptation if it's beneficial
	if newState.PredictedImprovement > 0.05 || nae.config.AggressiveAdaptation {
		return newState
	}
	
	return nil
}

// adaptForBandwidthIncrease adapts parameters when bandwidth increases.
func (nae *NetworkAdaptationEngine) adaptForBandwidthIncrease(state *AdaptationState, condition *NetworkCondition) *AdaptationState {
	// Increase chunk size for better efficiency
	newChunkSize := int(float64(state.ChunkSizeMB) * 1.2)
	state.ChunkSizeMB = newChunkSize
	
	// Increase concurrency if network can handle it
	if condition.CongestionLevel < 0.3 {
		state.Concurrency = int(math.Min(float64(state.Concurrency+1), float64(nae.config.MaxConcurrency)))
	}
	
	// Use faster compression if bandwidth is high
	if condition.BandwidthMBps > 100 {
		state.CompressionLevel = "zstd-fast"
	}
	
	return state
}

// adaptForBandwidthDecrease adapts parameters when bandwidth decreases.
func (nae *NetworkAdaptationEngine) adaptForBandwidthDecrease(state *AdaptationState, condition *NetworkCondition) *AdaptationState {
	// Decrease chunk size for better responsiveness
	newChunkSize := int(float64(state.ChunkSizeMB) * 0.8)
	state.ChunkSizeMB = newChunkSize
	
	// Decrease concurrency to reduce congestion
	state.Concurrency = int(math.Max(float64(state.Concurrency-1), float64(nae.config.MinConcurrency)))
	
	// Use higher compression to reduce transfer size
	if condition.BandwidthMBps < 20 {
		state.CompressionLevel = "zstd-high"
	}
	
	return state
}

// adaptForLatencyIncrease adapts parameters when latency increases.
func (nae *NetworkAdaptationEngine) adaptForLatencyIncrease(state *AdaptationState, condition *NetworkCondition) *AdaptationState {
	// Increase chunk size to amortize latency overhead
	newChunkSize := int(float64(state.ChunkSizeMB) * 1.3)
	state.ChunkSizeMB = newChunkSize
	
	// Reduce concurrency to avoid overwhelming the network
	state.Concurrency = int(math.Max(float64(state.Concurrency-1), float64(nae.config.MinConcurrency)))
	
	return state
}

// adaptForLatencyDecrease adapts parameters when latency decreases.
func (nae *NetworkAdaptationEngine) adaptForLatencyDecrease(state *AdaptationState, condition *NetworkCondition) *AdaptationState {
	// Can use smaller chunks with lower latency overhead
	newChunkSize := int(float64(state.ChunkSizeMB) * 0.9)
	state.ChunkSizeMB = newChunkSize
	
	// Increase concurrency to utilize better network conditions
	if condition.CongestionLevel < 0.2 {
		state.Concurrency = int(math.Min(float64(state.Concurrency+1), float64(nae.config.MaxConcurrency)))
	}
	
	return state
}

// adaptForCongestion adapts parameters when congestion is detected.
func (nae *NetworkAdaptationEngine) adaptForCongestion(state *AdaptationState, condition *NetworkCondition) *AdaptationState {
	// Reduce concurrency to ease congestion
	state.Concurrency = int(math.Max(float64(state.Concurrency-2), float64(nae.config.MinConcurrency)))
	
	// Use smaller chunks to be more responsive to network changes
	newChunkSize := int(float64(state.ChunkSizeMB) * 0.7)
	state.ChunkSizeMB = newChunkSize
	
	// Use higher compression to reduce network load
	state.CompressionLevel = "zstd-high"
	
	return state
}

// adaptForCongestionClear adapts parameters when congestion clears.
func (nae *NetworkAdaptationEngine) adaptForCongestionClear(state *AdaptationState, condition *NetworkCondition) *AdaptationState {
	// Gradually increase concurrency
	state.Concurrency = int(math.Min(float64(state.Concurrency+1), float64(nae.config.MaxConcurrency)))
	
	// Increase chunk size for better efficiency
	newChunkSize := int(float64(state.ChunkSizeMB) * 1.1)
	state.ChunkSizeMB = newChunkSize
	
	// Use balanced compression
	state.CompressionLevel = "zstd"
	
	return state
}

// adaptForPacketLoss adapts parameters when packet loss increases.
func (nae *NetworkAdaptationEngine) adaptForPacketLoss(state *AdaptationState, condition *NetworkCondition) *AdaptationState {
	// Reduce concurrency to minimize retransmissions
	state.Concurrency = int(math.Max(float64(state.Concurrency-1), float64(nae.config.MinConcurrency)))
	
	// Use smaller chunks to reduce impact of packet loss
	newChunkSize := int(float64(state.ChunkSizeMB) * 0.8)
	state.ChunkSizeMB = newChunkSize
	
	return state
}

// adaptForLowThroughput adapts parameters when throughput is below target.
func (nae *NetworkAdaptationEngine) adaptForLowThroughput(state *AdaptationState, condition *NetworkCondition) *AdaptationState {
	// Try increasing concurrency if not at max
	if state.Concurrency < nae.config.MaxConcurrency && condition.CongestionLevel < 0.4 {
		state.Concurrency++
	}
	
	// Optimize chunk size based on network conditions
	optimalChunkSize := nae.calculateOptimalChunkSize(condition)
	state.ChunkSizeMB = optimalChunkSize
	
	// Use compression that balances speed and ratio
	if condition.BandwidthMBps < 30 {
		state.CompressionLevel = "zstd-high"
	} else {
		state.CompressionLevel = "zstd-fast"
	}
	
	return state
}

// adaptForGeneral performs general adaptation based on current conditions.
func (nae *NetworkAdaptationEngine) adaptForGeneral(state *AdaptationState, condition *NetworkCondition) *AdaptationState {
	// Calculate optimal parameters based on current network state
	optimalChunkSize := nae.calculateOptimalChunkSize(condition)
	optimalConcurrency := nae.calculateOptimalConcurrency(condition)
	
	state.ChunkSizeMB = optimalChunkSize
	state.Concurrency = optimalConcurrency
	state.CompressionLevel = nae.selectOptimalCompression(condition)
	
	return state
}

// Helper methods for adaptation calculations

// calculateOptimalChunkSize calculates optimal chunk size for current network conditions.
func (nae *NetworkAdaptationEngine) calculateOptimalChunkSize(condition *NetworkCondition) int {
	// Base size on bandwidth-delay product
	bandwidthDelayProduct := condition.BandwidthMBps * (condition.LatencyMs / 1000.0)
	
	// Adjust for congestion
	congestionFactor := 1.0 - condition.CongestionLevel*0.5
	
	// Adjust for packet loss
	lossFactor := 1.0 - condition.PacketLoss*10.0
	
	optimalSize := int(bandwidthDelayProduct * congestionFactor * lossFactor)
	
	// Apply bounds
	if optimalSize < nae.config.MinChunkSizeMB {
		optimalSize = nae.config.MinChunkSizeMB
	}
	if optimalSize > nae.config.MaxChunkSizeMB {
		optimalSize = nae.config.MaxChunkSizeMB
	}
	
	return optimalSize
}

// calculateOptimalConcurrency calculates optimal concurrency for current network conditions.
func (nae *NetworkAdaptationEngine) calculateOptimalConcurrency(condition *NetworkCondition) int {
	// Base concurrency on bandwidth
	baseConcurrency := int(condition.BandwidthMBps / 25.0) // 25 MB/s per stream
	
	// Adjust for congestion
	if condition.CongestionLevel > 0.3 {
		baseConcurrency = int(float64(baseConcurrency) * (1.0 - condition.CongestionLevel))
	}
	
	// Adjust for latency
	if condition.LatencyMs > 100 {
		baseConcurrency = int(math.Max(float64(baseConcurrency-1), 1))
	}
	
	// Apply bounds
	if baseConcurrency < nae.config.MinConcurrency {
		baseConcurrency = nae.config.MinConcurrency
	}
	if baseConcurrency > nae.config.MaxConcurrency {
		baseConcurrency = nae.config.MaxConcurrency
	}
	
	return baseConcurrency
}

// selectOptimalCompression selects optimal compression level for current conditions.
func (nae *NetworkAdaptationEngine) selectOptimalCompression(condition *NetworkCondition) string {
	// For high bandwidth, prefer speed
	if condition.BandwidthMBps > 100 {
		return "zstd-fast"
	}
	
	// For low bandwidth, prefer compression ratio
	if condition.BandwidthMBps < 20 {
		return "zstd-high"
	}
	
	// For high latency, prefer compression ratio to reduce transfer time
	if condition.LatencyMs > 100 {
		return "zstd-high"
	}
	
	// Default balanced compression
	return "zstd"
}

// validateAdaptationLimits ensures adaptation state stays within configured limits.
func (nae *NetworkAdaptationEngine) validateAdaptationLimits(state *AdaptationState) *AdaptationState {
	// Validate chunk size
	if state.ChunkSizeMB < nae.config.MinChunkSizeMB {
		state.ChunkSizeMB = nae.config.MinChunkSizeMB
	}
	if state.ChunkSizeMB > nae.config.MaxChunkSizeMB {
		state.ChunkSizeMB = nae.config.MaxChunkSizeMB
	}
	
	// Validate concurrency
	if state.Concurrency < nae.config.MinConcurrency {
		state.Concurrency = nae.config.MinConcurrency
	}
	if state.Concurrency > nae.config.MaxConcurrency {
		state.Concurrency = nae.config.MaxConcurrency
	}
	
	// Validate compression level
	validCompressions := []string{"zstd-fast", "zstd", "zstd-high", "zstd-max"}
	isValid := false
	for _, valid := range validCompressions {
		if state.CompressionLevel == valid {
			isValid = true
			break
		}
	}
	if !isValid {
		state.CompressionLevel = "zstd"
	}
	
	return state
}

// calculateAdaptationScore calculates a score for the adaptation quality.
func (nae *NetworkAdaptationEngine) calculateAdaptationScore(state *AdaptationState, condition *NetworkCondition) float64 {
	score := 0.0
	
	// Score based on how well parameters match network conditions
	chunkScore := nae.scoreChunkSize(state.ChunkSizeMB, condition)
	concurrencyScore := nae.scoreConcurrency(state.Concurrency, condition)
	compressionScore := nae.scoreCompression(state.CompressionLevel, condition)
	
	score = (chunkScore + concurrencyScore + compressionScore) / 3.0
	
	return math.Min(math.Max(score, 0.0), 1.0)
}

// predictPerformanceImprovement predicts performance improvement from adaptation.
func (nae *NetworkAdaptationEngine) predictPerformanceImprovement(oldState, newState *AdaptationState) float64 {
	if oldState.PerformanceMetrics == nil {
		return 0.1 // Assume small improvement if no baseline
	}
	
	// Predict improvement based on parameter changes
	improvement := 0.0
	
	// Chunk size improvement
	if newState.ChunkSizeMB != oldState.ChunkSizeMB {
		improvement += nae.predictChunkSizeImprovement(oldState.ChunkSizeMB, newState.ChunkSizeMB, newState.NetworkCondition)
	}
	
	// Concurrency improvement
	if newState.Concurrency != oldState.Concurrency {
		improvement += nae.predictConcurrencyImprovement(oldState.Concurrency, newState.Concurrency, newState.NetworkCondition)
	}
	
	// Compression improvement
	if newState.CompressionLevel != oldState.CompressionLevel {
		improvement += nae.predictCompressionImprovement(oldState.CompressionLevel, newState.CompressionLevel, newState.NetworkCondition)
	}
	
	return improvement
}

// Helper methods for scoring and prediction

// scoreChunkSize scores chunk size appropriateness for network conditions.
func (nae *NetworkAdaptationEngine) scoreChunkSize(chunkSizeMB int, condition *NetworkCondition) float64 {
	optimal := nae.calculateOptimalChunkSize(condition)
	diff := math.Abs(float64(chunkSizeMB - optimal))
	maxDiff := float64(nae.config.MaxChunkSizeMB - nae.config.MinChunkSizeMB)
	
	return 1.0 - (diff / maxDiff)
}

// scoreConcurrency scores concurrency appropriateness for network conditions.
func (nae *NetworkAdaptationEngine) scoreConcurrency(concurrency int, condition *NetworkCondition) float64 {
	optimal := nae.calculateOptimalConcurrency(condition)
	diff := math.Abs(float64(concurrency - optimal))
	maxDiff := float64(nae.config.MaxConcurrency - nae.config.MinConcurrency)
	
	return 1.0 - (diff / maxDiff)
}

// scoreCompression scores compression level appropriateness for network conditions.
func (nae *NetworkAdaptationEngine) scoreCompression(compression string, condition *NetworkCondition) float64 {
	optimal := nae.selectOptimalCompression(condition)
	if compression == optimal {
		return 1.0
	}
	
	// Partial scores for similar compression levels
	compressionLevels := map[string]int{
		"zstd-fast": 1,
		"zstd":      2,
		"zstd-high": 3,
		"zstd-max":  4,
	}
	
	currentLevel, exists1 := compressionLevels[compression]
	optimalLevel, exists2 := compressionLevels[optimal]
	
	if !exists1 || !exists2 {
		return 0.5
	}
	
	diff := math.Abs(float64(currentLevel - optimalLevel))
	return 1.0 - (diff / 3.0) // Max difference is 3
}

// predictChunkSizeImprovement predicts improvement from chunk size change.
func (nae *NetworkAdaptationEngine) predictChunkSizeImprovement(oldSize, newSize int, condition *NetworkCondition) float64 {
	optimal := nae.calculateOptimalChunkSize(condition)
	
	oldDiff := math.Abs(float64(oldSize - optimal))
	newDiff := math.Abs(float64(newSize - optimal))
	
	if newDiff < oldDiff {
		return (oldDiff - newDiff) / float64(optimal) * 0.1 // Max 10% improvement
	}
	
	return 0.0
}

// predictConcurrencyImprovement predicts improvement from concurrency change.
func (nae *NetworkAdaptationEngine) predictConcurrencyImprovement(oldConcurrency, newConcurrency int, condition *NetworkCondition) float64 {
	// Simplified prediction based on network capacity
	if condition.CongestionLevel > 0.5 && newConcurrency < oldConcurrency {
		return 0.05 // 5% improvement from reducing congestion
	}
	
	if condition.CongestionLevel < 0.2 && newConcurrency > oldConcurrency && condition.BandwidthMBps > 50 {
		return 0.1 // 10% improvement from better utilization
	}
	
	return 0.0
}

// predictCompressionImprovement predicts improvement from compression change.
func (nae *NetworkAdaptationEngine) predictCompressionImprovement(oldCompression, newCompression string, condition *NetworkCondition) float64 {
	optimal := nae.selectOptimalCompression(condition)
	
	if newCompression == optimal && oldCompression != optimal {
		return 0.05 // 5% improvement from optimal compression
	}
	
	return 0.0
}

// calculateEffectiveBandwidth calculates effective bandwidth considering network conditions.
func (nae *NetworkAdaptationEngine) calculateEffectiveBandwidth(condition *NetworkCondition) float64 {
	effective := condition.BandwidthMBps
	
	// Reduce for congestion
	effective *= (1.0 - condition.CongestionLevel*0.5)
	
	// Reduce for packet loss
	effective *= (1.0 - condition.PacketLoss*5.0)
	
	// Reduce for high jitter
	if condition.Jitter > 10.0 {
		effective *= 0.9
	}
	
	return math.Max(effective, condition.BandwidthMBps*0.1) // At least 10% of nominal
}

// calculateNetworkUtilization calculates current network utilization.
func (nae *NetworkAdaptationEngine) calculateNetworkUtilization(condition *NetworkCondition) float64 {
	// Simplified utilization calculation
	baseUtilization := condition.CongestionLevel
	
	// Adjust for latency (higher latency may indicate higher utilization)
	if condition.LatencyMs > 50.0 {
		baseUtilization += (condition.LatencyMs - 50.0) / 200.0 // Add up to 0.5 for 150ms latency
	}
	
	// Adjust for packet loss
	baseUtilization += condition.PacketLoss * 10.0
	
	return math.Min(baseUtilization, 1.0)
}

// updateThroughputAverages updates running throughput averages.
func (nae *NetworkAdaptationEngine) updateThroughputAverages(metrics *PerformanceMetrics) {
	// Update peak throughput
	if metrics.CurrentThroughputMBps > metrics.PeakThroughputMBps {
		metrics.PeakThroughputMBps = metrics.CurrentThroughputMBps
	}
	
	// Update average throughput (exponential moving average)
	alpha := 0.1 // Smoothing factor
	if metrics.AverageThroughputMBps == 0 {
		metrics.AverageThroughputMBps = metrics.CurrentThroughputMBps
	} else {
		metrics.AverageThroughputMBps = alpha*metrics.CurrentThroughputMBps + (1-alpha)*metrics.AverageThroughputMBps
	}
	
	// Calculate transfer efficiency
	if metrics.PeakThroughputMBps > 0 {
		metrics.TransferEfficiency = metrics.CurrentThroughputMBps / metrics.PeakThroughputMBps
	}
}

// applyAdaptation applies the new adaptation state.
func (nae *NetworkAdaptationEngine) applyAdaptation(oldState, newState *AdaptationState) error {
	// This would integrate with transfer controller to apply changes
	if nae.transferController != nil {
		return nae.transferController.ApplyAdaptation(newState)
	}
	return nil
}

// notifyAdaptationCallbacks notifies registered callbacks of adaptation changes.
func (nae *NetworkAdaptationEngine) notifyAdaptationCallbacks(oldState, newState *AdaptationState) {
	for _, callback := range nae.adaptationCallbacks {
		go func(cb AdaptationCallback) {
			_ = cb(oldState, newState)
		}(callback)
	}
}

// Public API methods

// GetCurrentAdaptation returns the current adaptation state.
func (nae *NetworkAdaptationEngine) GetCurrentAdaptation() *AdaptationState {
	nae.mu.RLock()
	defer nae.mu.RUnlock()
	
	// Return a copy to prevent race conditions
	state := *nae.currentAdaptation
	return &state
}

// RegisterAdaptationCallback registers a callback for adaptation events.
func (nae *NetworkAdaptationEngine) RegisterAdaptationCallback(callback AdaptationCallback) {
	nae.mu.Lock()
	defer nae.mu.Unlock()
	
	nae.adaptationCallbacks = append(nae.adaptationCallbacks, callback)
}

// ForceAdaptation forces immediate adaptation evaluation.
func (nae *NetworkAdaptationEngine) ForceAdaptation() {
	nae.evaluateAndAdapt()
}

// NewDefaultAdaptationState creates a default adaptation state.
func NewDefaultAdaptationState() *AdaptationState {
	return &AdaptationState{
		Timestamp:           time.Now(),
		ChunkSizeMB:         32,       // 32MB default
		Concurrency:         4,        // 4 concurrent uploads
		CompressionLevel:    "zstd",   // Balanced compression
		BufferSizeMB:        256,      // 256MB buffer
		AdaptationReason:    "initial",
		AdaptationScore:     0.5,
		PredictedImprovement: 0.0,
	}
}

// AdaptationHistory tracks the history of network adaptations.
type AdaptationHistory struct {
	adaptations    []*AdaptationRecord
	maxHistory     int
	mu             sync.RWMutex
}

// AdaptationRecord represents a single adaptation event.
type AdaptationRecord struct {
	Timestamp    time.Time
	OldState     *AdaptationState
	NewState     *AdaptationState
	Reason       string
	Effectiveness float64 // Measured effectiveness (set later)
}

// NewAdaptationHistory creates a new adaptation history tracker.
func NewAdaptationHistory() *AdaptationHistory {
	return &AdaptationHistory{
		adaptations: make([]*AdaptationRecord, 0),
		maxHistory:  100,
	}
}

// RecordAdaptation records a new adaptation event.
func (ah *AdaptationHistory) RecordAdaptation(oldState, newState *AdaptationState, reason string) {
	ah.mu.Lock()
	defer ah.mu.Unlock()
	
	record := &AdaptationRecord{
		Timestamp: time.Now(),
		OldState:  oldState,
		NewState:  newState,
		Reason:    reason,
	}
	
	ah.adaptations = append(ah.adaptations, record)
	
	// Limit history size
	if len(ah.adaptations) > ah.maxHistory {
		ah.adaptations = ah.adaptations[1:]
	}
}

// GetRecentAdaptations returns recent adaptation history.
func (ah *AdaptationHistory) GetRecentAdaptations(count int) []*AdaptationRecord {
	ah.mu.RLock()
	defer ah.mu.RUnlock()
	
	if count > len(ah.adaptations) {
		count = len(ah.adaptations)
	}
	
	start := len(ah.adaptations) - count
	recent := make([]*AdaptationRecord, count)
	copy(recent, ah.adaptations[start:])
	
	return recent
}