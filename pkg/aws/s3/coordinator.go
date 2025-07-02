/*
Package s3 provides advanced transfer coordination for cross-prefix pipeline optimization.

This module implements Globus/GridFTP-style pipeline coordination across S3 prefixes
with global scheduling and congestion control for optimal large-scale data transfers.
*/
package s3

import (
	"context"
	"sync"
	"time"
)

// PipelineCoordinator manages cross-prefix coordination for optimal upload performance.
// It implements sophisticated flow control algorithms similar to GridFTP/Globus systems.
type PipelineCoordinator struct {
	scheduler         *TransferScheduler
	congestionControl *GlobalCongestionController
	prefixChannels    map[string]chan *ScheduledUpload
	metrics           *CoordinationMetrics
	config            *CoordinationConfig
	mu                sync.RWMutex
	active            bool
	ctx               context.Context
	cancel            context.CancelFunc
}

// CoordinationConfig defines the configuration for cross-prefix coordination.
type CoordinationConfig struct {
	// Pipeline depth controls how many uploads can be prepared in advance
	PipelineDepth int `yaml:"pipeline_depth" json:"pipeline_depth"`
	
	// Global congestion window for TCP-like flow control
	GlobalCongestionWindow int `yaml:"global_congestion_window" json:"global_congestion_window"`
	
	// Coordination strategy: "tcp_like", "fair_share", "adaptive"
	Strategy string `yaml:"coordination_strategy" json:"coordination_strategy"`
	
	// Maximum number of active prefixes to coordinate
	MaxActivePrefixes int `yaml:"max_active_prefixes" json:"max_active_prefixes"`
	
	// Bandwidth allocation strategy
	BandwidthStrategy string `yaml:"bandwidth_strategy" json:"bandwidth_strategy"`
	
	// Coordination update interval
	UpdateInterval time.Duration `yaml:"update_interval" json:"update_interval"`
	
	// Enable advanced flow control features
	EnableAdvancedFlowControl bool `yaml:"enable_advanced_flow_control" json:"enable_advanced_flow_control"`
}

// DefaultCoordinationConfig returns sensible defaults for cross-prefix coordination.
func DefaultCoordinationConfig() *CoordinationConfig {
	return &CoordinationConfig{
		PipelineDepth:             16,
		GlobalCongestionWindow:    32,
		Strategy:                  "adaptive",
		MaxActivePrefixes:         16,
		BandwidthStrategy:         "fair_share",
		UpdateInterval:            time.Second * 2,
		EnableAdvancedFlowControl: true,
	}
}

// ScheduledUpload represents a coordinated upload with scheduling metadata.
type ScheduledUpload struct {
	// Archive details
	ArchivePath   string
	PrefixID      string
	Priority      int
	EstimatedSize int64
	
	// Scheduling metadata
	ScheduledAt   time.Time
	Deadline      time.Time
	Dependencies  []string
	
	// Flow control
	BandwidthAllocation float64  // MB/s allocated to this upload
	CongestionWindow    int      // Current congestion window size
	BackoffDelay        time.Duration
	
	// Coordination context
	CoordinationID string
	GroupID        string
}

// TransferScheduler implements global scheduling logic across all S3 prefixes.
type TransferScheduler struct {
	prefixMetrics    map[string]*PrefixPerformanceMetrics
	networkProfile   *NetworkProfile
	globalState      *GlobalTransferState
	loadBalancer     *PrefixLoadBalancer
	config           *CoordinationConfig
	mu               sync.RWMutex
}

// PrefixPerformanceMetrics tracks real-time performance for each S3 prefix.
type PrefixPerformanceMetrics struct {
	PrefixID           string
	ActiveUploads      int
	ThroughputMBps     float64
	LatencyMs          float64
	ErrorRate          float64
	CongestionWindow   int
	LastUpdate         time.Time
	BandwidthUtilization float64
	
	// Historical performance data
	ThroughputHistory  []float64
	LatencyHistory     []float64
	ErrorHistory       []float64
	
	// Load metrics
	QueueLength        int
	ProcessingCapacity float64
}

// GlobalCongestionController implements TCP-like congestion control across all prefixes.
type GlobalCongestionController struct {
	globalCongestionWindow    int
	slowStartThreshold        int
	prefixAllocation         map[string]*PrefixAllocation
	totalBandwidthMBps       float64
	congestionState          CongestionState
	lastCongestionEvent      time.Time
	adaptiveParameters       *AdaptiveParameters
	mu                       sync.RWMutex
}

// PrefixAllocation tracks bandwidth and resource allocation for a specific prefix.
type PrefixAllocation struct {
	PrefixID              string
	AllocatedBandwidthMBps float64
	CongestionWindow      int
	InFlight              int
	Utilization           float64
	Priority              int
	LastAdjustment        time.Time
}

// CongestionState represents the current global congestion state.
type CongestionState int

const (
	CongestionStateNormal CongestionState = iota
	CongestionStateSlowStart
	CongestionStateAvoidance
	CongestionStateRecovery
	CongestionStateFastRecovery
)

// AdaptiveParameters controls the adaptive behavior of the congestion controller.
type AdaptiveParameters struct {
	LearningRate           float64
	BandwidthProbingRate   float64
	CongestionSensitivity  float64
	RecoveryAggressiveness float64
	
	// BBR-style parameters
	BTLBandwidthFilter    *BandwidthFilter
	RTTMin                time.Duration
	CycleLength           time.Duration
}

// BandwidthFilter implements a windowed max filter for bandwidth estimation.
type BandwidthFilter struct {
	samples    []BandwidthSample
	maxWindow  time.Duration
	currentMax float64
	mu         sync.RWMutex
}

// BandwidthSample represents a single bandwidth measurement.
type BandwidthSample struct {
	Timestamp   time.Time
	BandwidthMBps float64
	RTT         time.Duration
	InFlight    int
}

// NetworkProfile maintains a profile of network characteristics for optimization.
type NetworkProfile struct {
	// Network characteristics
	EstimatedBandwidthMBps float64
	BaselineRTT            time.Duration
	NetworkVariance        float64
	CongestionThreshold    float64
	
	// Historical learning
	SessionHistory         []*TransferSession
	MaxHistorySize         int
	LastProfileUpdate      time.Time
	
	// Adaptive learning
	BandwidthTrend         TrendDirection
	LatencyTrend           TrendDirection
	LearningConfidence     float64
}

// TransferSession captures performance data from a complete transfer session.
type TransferSession struct {
	SessionID            string
	StartTime            time.Time
	EndTime              time.Time
	TotalBytes           int64
	AverageThroughputMBps float64
	PeakThroughputMBps   float64
	AverageLatency       time.Duration
	ErrorCount           int
	PrefixCount          int
	CoordinationEnabled  bool
}

// TrendDirection indicates the direction of a performance trend.
type TrendDirection int

const (
	TrendUnknown TrendDirection = iota
	TrendIncreasing
	TrendDecreasing
	TrendStable
)

// GlobalTransferState maintains global state across all active transfers.
type GlobalTransferState struct {
	ActivePrefixes        map[string]bool
	TotalActiveUploads    int
	GlobalThroughputMBps  float64
	GlobalErrorRate       float64
	SystemLoad            float64
	
	// Resource utilization
	NetworkUtilization    float64
	CPUUtilization        float64
	MemoryUtilization     float64
	
	// Coordination metrics
	CoordinationOverhead  float64
	LoadBalanceEfficiency float64
	PipelineUtilization   float64
	
	LastUpdate            time.Time
}

// PrefixLoadBalancer implements intelligent load balancing across S3 prefixes.
type PrefixLoadBalancer struct {
	strategy            LoadBalanceStrategy
	prefixWeights       map[string]float64
	prefixCapacities    map[string]float64
	rebalanceThreshold  float64
	rebalanceInterval   time.Duration
	lastRebalance       time.Time
	mu                  sync.RWMutex
}

// LoadBalanceStrategy defines different load balancing approaches.
type LoadBalanceStrategy int

const (
	LoadBalanceRoundRobin LoadBalanceStrategy = iota
	LoadBalanceLeastLoaded
	LoadBalanceWeighted
	LoadBalanceAdaptive
	LoadBalancePredictive
)

// CoordinationMetrics provides comprehensive metrics for coordination performance.
type CoordinationMetrics struct {
	// Coordination efficiency
	CoordinationOverheadPercent float64
	LoadBalanceEfficiency       float64
	PipelineUtilization         float64
	
	// Performance metrics
	GlobalThroughputMBps        float64
	CoordinationEfficiencyGain  float64
	LatencyReduction            float64
	
	// Resource utilization
	ActivePrefixes              int
	AverageQueueLength          float64
	BandwidthUtilization        float64
	
	// Congestion control metrics
	CongestionEvents            int
	RecoveryTime                time.Duration
	AdaptationRate              float64
	
	// Comparison metrics
	BaselinePerformance         *PerformanceBaseline
	CoordinatedPerformance      *PerformanceBaseline
	ImprovementFactor           float64
	
	LastUpdate                  time.Time
}

// PerformanceBaseline captures baseline performance metrics for comparison.
type PerformanceBaseline struct {
	ThroughputMBps    float64
	LatencyMs         float64
	ErrorRate         float64
	EfficiencyRating  float64
	Timestamp         time.Time
}

// NewPipelineCoordinator creates a new cross-prefix pipeline coordinator.
func NewPipelineCoordinator(ctx context.Context, config *CoordinationConfig) *PipelineCoordinator {
	if config == nil {
		config = DefaultCoordinationConfig()
	}
	
	coordCtx, cancel := context.WithCancel(ctx)
	
	pc := &PipelineCoordinator{
		scheduler:         NewTransferScheduler(config),
		congestionControl: NewGlobalCongestionController(config),
		prefixChannels:    make(map[string]chan *ScheduledUpload),
		metrics:           NewCoordinationMetrics(),
		config:            config,
		active:            false,
		ctx:               coordCtx,
		cancel:            cancel,
	}
	
	return pc
}

// NewTransferScheduler creates a new global transfer scheduler.
func NewTransferScheduler(config *CoordinationConfig) *TransferScheduler {
	return &TransferScheduler{
		prefixMetrics:  make(map[string]*PrefixPerformanceMetrics),
		networkProfile: NewNetworkProfile(),
		globalState:    NewGlobalTransferState(),
		loadBalancer:   NewPrefixLoadBalancer(LoadBalanceAdaptive),
		config:         config,
	}
}

// NewGlobalCongestionController creates a new global congestion controller.
func NewGlobalCongestionController(config *CoordinationConfig) *GlobalCongestionController {
	return &GlobalCongestionController{
		globalCongestionWindow: config.GlobalCongestionWindow,
		slowStartThreshold:     config.GlobalCongestionWindow / 2,
		prefixAllocation:      make(map[string]*PrefixAllocation),
		totalBandwidthMBps:    0,
		congestionState:       CongestionStateSlowStart,
		lastCongestionEvent:   time.Now(),
		adaptiveParameters:    NewAdaptiveParameters(),
	}
}

// NewNetworkProfile creates a new network profile for adaptive optimization.
func NewNetworkProfile() *NetworkProfile {
	return &NetworkProfile{
		EstimatedBandwidthMBps: 100.0, // Start with reasonable default
		BaselineRTT:           time.Millisecond * 50,
		NetworkVariance:       0.1,
		CongestionThreshold:   0.8,
		SessionHistory:        make([]*TransferSession, 0),
		MaxHistorySize:        50,
		LastProfileUpdate:     time.Now(),
		BandwidthTrend:        TrendUnknown,
		LatencyTrend:          TrendUnknown,
		LearningConfidence:    0.5,
	}
}

// NewGlobalTransferState creates a new global transfer state tracker.
func NewGlobalTransferState() *GlobalTransferState {
	return &GlobalTransferState{
		ActivePrefixes:        make(map[string]bool),
		TotalActiveUploads:    0,
		GlobalThroughputMBps:  0,
		GlobalErrorRate:       0,
		SystemLoad:            0,
		NetworkUtilization:    0,
		CPUUtilization:        0,
		MemoryUtilization:     0,
		CoordinationOverhead:  0,
		LoadBalanceEfficiency: 1.0,
		PipelineUtilization:   0,
		LastUpdate:            time.Now(),
	}
}

// NewPrefixLoadBalancer creates a new prefix load balancer.
func NewPrefixLoadBalancer(strategy LoadBalanceStrategy) *PrefixLoadBalancer {
	return &PrefixLoadBalancer{
		strategy:           strategy,
		prefixWeights:      make(map[string]float64),
		prefixCapacities:   make(map[string]float64),
		rebalanceThreshold: 0.2, // 20% imbalance triggers rebalancing
		rebalanceInterval:  time.Second * 30,
		lastRebalance:      time.Now(),
	}
}

// NewAdaptiveParameters creates default adaptive parameters for congestion control.
func NewAdaptiveParameters() *AdaptiveParameters {
	return &AdaptiveParameters{
		LearningRate:           0.1,
		BandwidthProbingRate:   0.05,
		CongestionSensitivity:  0.8,
		RecoveryAggressiveness: 1.2,
		BTLBandwidthFilter:     NewBandwidthFilter(time.Second * 10),
		RTTMin:                time.Millisecond * 10,
		CycleLength:           time.Second * 8,
	}
}

// NewBandwidthFilter creates a new bandwidth filter for bottleneck estimation.
func NewBandwidthFilter(windowSize time.Duration) *BandwidthFilter {
	return &BandwidthFilter{
		samples:    make([]BandwidthSample, 0),
		maxWindow:  windowSize,
		currentMax: 0,
	}
}

// NewCoordinationMetrics creates a new coordination metrics tracker.
func NewCoordinationMetrics() *CoordinationMetrics {
	return &CoordinationMetrics{
		CoordinationOverheadPercent: 0,
		LoadBalanceEfficiency:       1.0,
		PipelineUtilization:         0,
		GlobalThroughputMBps:        0,
		CoordinationEfficiencyGain:  0,
		LatencyReduction:            0,
		ActivePrefixes:              0,
		AverageQueueLength:          0,
		BandwidthUtilization:        0,
		CongestionEvents:            0,
		RecoveryTime:                0,
		AdaptationRate:              0,
		BaselinePerformance:         nil,
		CoordinatedPerformance:      nil,
		ImprovementFactor:           1.0,
		LastUpdate:                  time.Now(),
	}
}

// Start begins the pipeline coordination process.
func (pc *PipelineCoordinator) Start() error {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	
	if pc.active {
		return nil // Already active
	}
	
	pc.active = true
	
	// Start coordination subsystems
	go pc.scheduler.Start(pc.ctx)
	go pc.congestionControl.Start(pc.ctx)
	go pc.metricsCollector(pc.ctx)
	
	return nil
}

// Stop gracefully shuts down the pipeline coordinator.
func (pc *PipelineCoordinator) Stop() error {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	
	if !pc.active {
		return nil // Already stopped
	}
	
	pc.active = false
	pc.cancel()
	
	// Close all prefix channels
	for _, ch := range pc.prefixChannels {
		close(ch)
	}
	
	return nil
}

// RegisterPrefix registers a new S3 prefix for coordination.
func (pc *PipelineCoordinator) RegisterPrefix(prefixID string, capacity float64) error {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	
	if !pc.active {
		return &CoordinationError{
			Type:    "coordinator_inactive",
			Message: "pipeline coordinator is not active",
		}
	}
	
	// Create communication channel for this prefix
	pc.prefixChannels[prefixID] = make(chan *ScheduledUpload, pc.config.PipelineDepth)
	
	// Register with scheduler and congestion controller
	pc.scheduler.RegisterPrefix(prefixID, capacity)
	pc.congestionControl.RegisterPrefix(prefixID, capacity)
	
	return nil
}

// ScheduleUpload schedules an upload through the coordination system.
func (pc *PipelineCoordinator) ScheduleUpload(upload *ScheduledUpload) error {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	
	if !pc.active {
		return &CoordinationError{
			Type:    "coordinator_inactive",
			Message: "pipeline coordinator is not active",
		}
	}
	
	// Get optimal prefix for this upload
	optimalPrefix, err := pc.scheduler.SelectOptimalPrefix(upload)
	if err != nil {
		return err
	}
	
	upload.PrefixID = optimalPrefix
	upload.ScheduledAt = time.Now()
	
	// Apply congestion control
	allocation, err := pc.congestionControl.AllocateResources(upload)
	if err != nil {
		return err
	}
	
	upload.BandwidthAllocation = allocation.AllocatedBandwidthMBps
	upload.CongestionWindow = allocation.CongestionWindow
	
	// Schedule the upload
	select {
	case pc.prefixChannels[optimalPrefix] <- upload:
		return nil
	default:
		return &CoordinationError{
			Type:    "prefix_queue_full",
			Message: "prefix upload queue is full",
			PrefixID: optimalPrefix,
		}
	}
}

// GetMetrics returns current coordination metrics.
func (pc *PipelineCoordinator) GetMetrics() *CoordinationMetrics {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	
	return pc.metrics
}

// UpdatePrefixMetrics updates performance metrics for a specific prefix.
func (pc *PipelineCoordinator) UpdatePrefixMetrics(prefixID string, metrics *PrefixPerformanceMetrics) {
	pc.scheduler.UpdatePrefixMetrics(prefixID, metrics)
	pc.congestionControl.UpdatePrefixPerformance(prefixID, metrics)
}

// metricsCollector runs periodic metrics collection and coordination updates.
func (pc *PipelineCoordinator) metricsCollector(ctx context.Context) {
	ticker := time.NewTicker(pc.config.UpdateInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pc.updateCoordinationMetrics()
		}
	}
}

// updateCoordinationMetrics updates the coordination metrics.
func (pc *PipelineCoordinator) updateCoordinationMetrics() {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	
	// Collect metrics from all subsystems
	schedulerMetrics := pc.scheduler.GetMetrics()
	congestionMetrics := pc.congestionControl.GetMetrics()
	
	// Update coordination metrics
	pc.metrics.ActivePrefixes = len(pc.prefixChannels)
	pc.metrics.GlobalThroughputMBps = schedulerMetrics.GlobalThroughputMBps
	pc.metrics.LoadBalanceEfficiency = schedulerMetrics.LoadBalanceEfficiency
	pc.metrics.CoordinationOverheadPercent = congestionMetrics.OverheadPercent
	pc.metrics.CongestionEvents = congestionMetrics.CongestionEvents
	pc.metrics.LastUpdate = time.Now()
	
	// Calculate efficiency gains
	if pc.metrics.BaselinePerformance != nil && pc.metrics.CoordinatedPerformance != nil {
		pc.metrics.ImprovementFactor = pc.metrics.CoordinatedPerformance.ThroughputMBps / 
										pc.metrics.BaselinePerformance.ThroughputMBps
		pc.metrics.CoordinationEfficiencyGain = (pc.metrics.ImprovementFactor - 1.0) * 100
	}
}

// CoordinationError represents an error in the coordination system.
type CoordinationError struct {
	Type     string
	Message  string
	PrefixID string
	Details  map[string]interface{}
}

func (e *CoordinationError) Error() string {
	if e.PrefixID != "" {
		return e.Type + " [" + e.PrefixID + "]: " + e.Message
	}
	return e.Type + ": " + e.Message
}