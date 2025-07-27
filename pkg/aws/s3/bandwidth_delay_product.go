/*
Package s3 bandwidth delay product calculations implements sophisticated BDP estimation and management
for optimal network resource utilization and flow control.

This module provides advanced bandwidth-delay product calculations including dynamic BDP estimation,
optimal window sizing, buffer management, and adaptive flow control optimization for high-performance
cloud storage transfers.
*/
package s3

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"
)

// BandwidthDelayProductCalculator manages comprehensive BDP calculations and optimization
type BandwidthDelayProductCalculator struct {
	// Core BDP components
	currentBDP             int64           // Current BDP estimate (bytes)
	optimalBDP             int64           // Optimal BDP for current conditions
	maxObservedBDP         int64           // Maximum observed BDP
	minObservedBDP         int64           // Minimum observed BDP
	
	// Bandwidth and RTT tracking
	bandwidthEstimator     *BDPBandwidthEstimator
	rttEstimator           *BDPRTTEstimator
	currentBandwidth       float64         // Current bandwidth (Mbps)
	currentRTT             time.Duration   // Current RTT
	
	// Dynamic calculations
	smoothedBDP            int64           // Smoothed BDP estimate
	bdpVariance            float64         // BDP variance for stability
	adaptationRate         float64         // Rate of BDP adaptation
	
	// Window and buffer management
	optimalWindowSize      int64           // Optimal congestion window size
	optimalBufferSize      int64           // Optimal buffer size
	receiveWindowSize      int64           // Receive window size
	sendWindowSize         int64           // Send window size
	
	// Network condition tracking
	networkConditions      *BDPNetworkConditions
	pathCharacteristics    *BDPPathCharacteristics
	trafficProfile         *BDPTrafficProfile
	
	// Historical data
	bdpHistory             []BDPSample
	performanceHistory     []BDPPerformanceSample
	optimizationHistory    []BDPOptimizationEvent
	
	// Configuration and tuning
	config                 *BDPConfig
	tuningParameters       *BDPTuningParameters
	adaptiveAlgorithm      BDPAdaptationAlgorithm
	
	// Performance metrics
	metrics                *BDPMetrics
	efficiency             float64         // BDP utilization efficiency
	accuracy               float64         // BDP estimation accuracy
	
	// Context and synchronization
	ctx                    context.Context
	cancel                 context.CancelFunc
	isActive               bool
	mu                     sync.RWMutex
	historyMu              sync.Mutex
}

// BDP-related data structures
type BDPSample struct {
	Timestamp              time.Time
	Bandwidth              float64         // Mbps
	RTT                    time.Duration
	CalculatedBDP          int64          // bytes
	ActualThroughput       float64        // Mbps
	WindowUtilization      float64        // percentage
	BufferOccupancy        float64        // percentage
	NetworkConditionScore  float64        // 0-1 scale
}

type BDPPerformanceSample struct {
	Timestamp              time.Time
	Throughput             float64        // Achieved throughput (Mbps)
	Efficiency             float64        // BDP utilization efficiency
	PacketLoss             float64        // Packet loss rate
	Retransmissions        int64          // Number of retransmissions
	WindowFullEvents       int64          // Window full events
	BufferOverruns         int64          // Buffer overrun events
	OptimalityScore        float64        // How close to optimal (0-1)
}

type BDPOptimizationEvent struct {
	Timestamp              time.Time
	EventType              BDPOptimizationType
	OldBDP                 int64
	NewBDP                 int64
	Trigger                string
	PerformanceImprovement float64
	Confidence             float64
}

type BDPOptimizationType string

const (
	BDPOptimizationIncrease    BDPOptimizationType = "increase"
	BDPOptimizationDecrease    BDPOptimizationType = "decrease"
	BDPOptimizationFinetuning  BDPOptimizationType = "finetuning"
	BDPOptimizationReset       BDPOptimizationType = "reset"
)

type BDPAdaptationAlgorithm string

const (
	BDPAlgorithmClassic        BDPAdaptationAlgorithm = "classic"
	BDPAlgorithmAdaptive       BDPAdaptationAlgorithm = "adaptive"
	BDPAlgorithmMachineLearning BDPAdaptationAlgorithm = "machine_learning"
	BDPAlgorithmHybrid         BDPAdaptationAlgorithm = "hybrid"
)

// Network condition structures
type BDPNetworkConditions struct {
	Congestion             float64        // Congestion level (0-1)
	PacketLoss             float64        // Packet loss rate
	Jitter                 time.Duration   // Network jitter
	QueueingDelay          time.Duration   // Queueing delay
	BottleneckBandwidth    float64        // Bottleneck bandwidth (Mbps)
	PathMTU                int64          // Path MTU
	ECNCapable             bool           // ECN capability
}

type BDPPathCharacteristics struct {
	HopCount               int            // Number of hops
	GeographicDistance     float64        // Approximate distance (km)
	NetworkType            string         // Network type (LAN, WAN, etc.)
	ISPCharacteristics     string         // ISP-specific characteristics
	TimeOfDay              string         // Time-based variations
	LoadBalancing          bool           // Multi-path capability
}

type BDPTrafficProfile struct {
	FlowType               string         // Flow type (bulk, interactive, etc.)
	DataPattern            string         // Data pattern (sequential, random)
	BurstCharacteristics   *BurstProfile  // Burst characteristics
	Priority               int            // Traffic priority
	QoSRequirements        *QoSProfile    // QoS requirements
}

type BurstProfile struct {
	BurstSize              int64          // Average burst size
	BurstFrequency         float64        // Bursts per second
	BurstDuration          time.Duration   // Average burst duration
	InterBurstGap          time.Duration   // Gap between bursts
}

type QoSProfile struct {
	MinBandwidth           float64        // Minimum bandwidth requirement
	MaxLatency             time.Duration   // Maximum acceptable latency
	MaxJitter              time.Duration   // Maximum acceptable jitter
	MaxLossRate            float64        // Maximum acceptable loss rate
}

// Configuration structures
type BDPConfig struct {
	// Core BDP parameters
	DefaultBandwidth       float64        // Default bandwidth assumption (Mbps)
	DefaultRTT             time.Duration   // Default RTT assumption
	MinBDP                 int64          // Minimum BDP (bytes)
	MaxBDP                 int64          // Maximum BDP (bytes)
	
	// Estimation parameters
	SmoothingFactor        float64        // BDP smoothing factor
	VarianceThreshold      float64        // Variance threshold for stability
	AdaptationRate         float64        // Rate of adaptation to changes
	
	// Window sizing
	WindowSizingFactor     float64        // Factor for window sizing
	BufferMultiplier       float64        // Buffer size multiplier
	MaxWindowSize          int64          // Maximum window size
	MinWindowSize          int64          // Minimum window size
	
	// Performance optimization
	OptimizationInterval   time.Duration   // Optimization check interval
	PerformanceThreshold   float64        // Performance improvement threshold
	StabilityPeriod        time.Duration   // Stability observation period
	
	// Advanced features
	EnableAdaptiveBDP      bool           // Enable adaptive BDP calculation
	EnableMLOptimization   bool           // Enable ML-based optimization
	EnablePathPrediction   bool           // Enable path characteristic prediction
}

type BDPTuningParameters struct {
	// Sensitivity parameters
	BandwidthSensitivity   float64        // Sensitivity to bandwidth changes
	RTTSensitivity         float64        // Sensitivity to RTT changes
	LossSensitivity        float64        // Sensitivity to packet loss
	
	// Stability parameters
	StabilityFactor        float64        // Factor for maintaining stability
	OscillationDamping     float64        // Damping for oscillations
	ConvergenceRate        float64        // Rate of convergence to optimal
	
	// Optimization parameters
	ExplorationRate        float64        // Rate of exploring new values
	ExploitationRate       float64        // Rate of exploiting known good values
	LearningRate           float64        // Learning rate for ML algorithms
}

// Performance metrics
type BDPMetrics struct {
	// Calculation metrics
	TotalCalculations      int64          // Total BDP calculations
	AverageCalculationTime time.Duration   // Average calculation time
	EstimationAccuracy     float64        // Accuracy of BDP estimation
	PredictionAccuracy     float64        // Accuracy of BDP prediction
	
	// Utilization metrics
	AverageBDPUtilization  float64        // Average BDP utilization
	PeakBDPUtilization     float64        // Peak BDP utilization
	OptimalUtilizationTime float64        // Time at optimal utilization (%)
	
	// Performance impact
	ThroughputImprovement  float64        // Throughput improvement (%)
	LatencyReduction       time.Duration   // Latency reduction
	PacketLossReduction    float64        // Packet loss reduction (%)
	
	// Optimization metrics
	TotalOptimizations     int64          // Total optimization events
	SuccessfulOptimizations int64         // Successful optimizations
	OptimizationEfficiency float64        // Optimization efficiency
	
	// Stability metrics
	BDPVariance            float64        // BDP variance
	StabilityScore         float64        // Overall stability score
	AdaptationLatency      time.Duration   // Time to adapt to changes
	
	// Timing
	LastUpdate             time.Time      // Last metrics update
	MeasurementPeriod      time.Duration   // Measurement period
}

// Sub-component estimators
type BDPBandwidthEstimator struct {
	samples                []BDPBandwidthSample
	currentBandwidth       float64
	smoothedBandwidth      float64
	bandwidthVariance      float64
	estimationMethod       BandwidthEstimationMethod
	mu                     sync.RWMutex
}

type BDPBandwidthSample struct {
	timestamp              time.Time
	bandwidth              float64
	confidence             float64
}

type BandwidthEstimationMethod string

const (
	BandwidthMethodPacketPair     BandwidthEstimationMethod = "packet_pair"
	BandwidthMethodThroughput     BandwidthEstimationMethod = "throughput"
	BandwidthMethodDeliveryRate   BandwidthEstimationMethod = "delivery_rate"
	BandwidthMethodHybrid         BandwidthEstimationMethod = "hybrid"
)

type BDPRTTEstimator struct {
	samples                []RTTSampleBDP
	currentRTT             time.Duration
	smoothedRTT            time.Duration
	rttVariance            time.Duration
	minRTT                 time.Duration
	mu                     sync.RWMutex
}

type RTTSampleBDP struct {
	timestamp              time.Time
	rtt                    time.Duration
	measurement_type       string
}

// Constructor
func NewBandwidthDelayProductCalculator(ctx context.Context, config *BDPConfig) *BandwidthDelayProductCalculator {
	if config == nil {
		config = NewDefaultBDPConfig()
	}
	
	bdpCtx, cancel := context.WithCancel(ctx)
	
	calculator := &BandwidthDelayProductCalculator{
		currentBDP:             0,
		optimalBDP:             0,
		maxObservedBDP:         0,
		minObservedBDP:         math.MaxInt64,
		
		bandwidthEstimator:     NewBDPBandwidthEstimator(),
		rttEstimator:           NewBDPRTTEstimator(),
		currentBandwidth:       config.DefaultBandwidth,
		currentRTT:             config.DefaultRTT,
		
		smoothedBDP:            0,
		bdpVariance:            0.0,
		adaptationRate:         config.AdaptationRate,
		
		optimalWindowSize:      config.MinWindowSize,
		optimalBufferSize:      config.MinWindowSize * 2,
		receiveWindowSize:      config.MinWindowSize,
		sendWindowSize:         config.MinWindowSize,
		
		networkConditions:      NewBDPNetworkConditions(),
		pathCharacteristics:    NewBDPPathCharacteristics(),
		trafficProfile:         NewBDPTrafficProfile(),
		
		bdpHistory:             make([]BDPSample, 0, 1000),
		performanceHistory:     make([]BDPPerformanceSample, 0, 1000),
		optimizationHistory:    make([]BDPOptimizationEvent, 0, 1000),
		
		config:                 config,
		tuningParameters:       NewDefaultBDPTuningParameters(),
		adaptiveAlgorithm:      BDPAlgorithmAdaptive,
		
		metrics:                NewBDPMetrics(),
		efficiency:             0.0,
		accuracy:               0.0,
		
		ctx:                    bdpCtx,
		cancel:                 cancel,
		isActive:               false,
	}
	
	return calculator
}

// Core BDP calculation methods
func (bdp *BandwidthDelayProductCalculator) StartCalculator() error {
	bdp.mu.Lock()
	defer bdp.mu.Unlock()
	
	if bdp.isActive {
		return fmt.Errorf("BDP calculator already active")
	}
	
	bdp.isActive = true
	
	// Start the main calculation loop
	go bdp.runCalculationLoop()
	go bdp.runOptimizationLoop()
	
	return nil
}

func (bdp *BandwidthDelayProductCalculator) StopCalculator() error {
	bdp.mu.Lock()
	defer bdp.mu.Unlock()
	
	if !bdp.isActive {
		return fmt.Errorf("BDP calculator not active")
	}
	
	bdp.isActive = false
	bdp.cancel()
	
	return nil
}

func (bdp *BandwidthDelayProductCalculator) UpdateBandwidth(bandwidth float64, timestamp time.Time, confidence float64) {
	bdp.mu.Lock()
	defer bdp.mu.Unlock()
	
	// Update bandwidth estimator
	bdp.bandwidthEstimator.AddSample(bandwidth, timestamp, confidence)
	bdp.currentBandwidth = bdp.bandwidthEstimator.GetCurrentBandwidth()
	
	// Recalculate BDP
	bdp.calculateBDP()
}

func (bdp *BandwidthDelayProductCalculator) UpdateRTT(rtt time.Duration, timestamp time.Time, measurementType string) {
	bdp.mu.Lock()
	defer bdp.mu.Unlock()
	
	// Update RTT estimator
	bdp.rttEstimator.AddSample(rtt, timestamp, measurementType)
	bdp.currentRTT = bdp.rttEstimator.GetCurrentRTT()
	
	// Recalculate BDP
	bdp.calculateBDP()
}

func (bdp *BandwidthDelayProductCalculator) UpdateNetworkConditions(conditions *BDPNetworkConditions) {
	bdp.mu.Lock()
	defer bdp.mu.Unlock()
	
	bdp.networkConditions = conditions
	
	// Adjust BDP based on network conditions
	bdp.adjustBDPForConditions()
}

// Core BDP calculation
func (bdp *BandwidthDelayProductCalculator) calculateBDP() {
	if bdp.currentBandwidth <= 0 || bdp.currentRTT <= 0 {
		return
	}
	
	// Basic BDP calculation: BDP = Bandwidth * RTT
	bandwidthBps := bdp.currentBandwidth * 1024 * 1024 / 8 // Convert Mbps to Bps
	rttSeconds := bdp.currentRTT.Seconds()
	rawBDP := int64(bandwidthBps * rttSeconds)
	
	// Apply bounds
	if rawBDP < bdp.config.MinBDP {
		rawBDP = bdp.config.MinBDP
	}
	if rawBDP > bdp.config.MaxBDP {
		rawBDP = bdp.config.MaxBDP
	}
	
	// Smooth the BDP estimate
	if bdp.currentBDP == 0 {
		bdp.currentBDP = rawBDP
		bdp.smoothedBDP = rawBDP
	} else {
		// Exponential smoothing
		bdp.smoothedBDP = int64(float64(bdp.smoothedBDP)*(1-bdp.config.SmoothingFactor) + 
			float64(rawBDP)*bdp.config.SmoothingFactor)
		bdp.currentBDP = bdp.smoothedBDP
	}
	
	// Update variance
	diff := float64(rawBDP - bdp.smoothedBDP)
	bdp.bdpVariance = bdp.bdpVariance*0.9 + diff*diff*0.1
	
	// Update bounds tracking
	if bdp.currentBDP > bdp.maxObservedBDP {
		bdp.maxObservedBDP = bdp.currentBDP
	}
	if bdp.currentBDP < bdp.minObservedBDP {
		bdp.minObservedBDP = bdp.currentBDP
	}
	
	// Calculate optimal window and buffer sizes
	bdp.calculateOptimalSizes()
	
	// Record sample
	bdp.recordBDPSample()
	
	// Update metrics
	bdp.metrics.TotalCalculations++
}

func (bdp *BandwidthDelayProductCalculator) calculateOptimalSizes() {
	// Optimal window size based on BDP and conditions
	baseWindowSize := int64(float64(bdp.currentBDP) * bdp.config.WindowSizingFactor)
	
	// Adjust for network conditions
	if bdp.networkConditions.PacketLoss > 0.01 { // > 1% loss
		baseWindowSize = int64(float64(baseWindowSize) * (1.0 - bdp.networkConditions.PacketLoss))
	}
	
	// Apply bounds
	if baseWindowSize < bdp.config.MinWindowSize {
		baseWindowSize = bdp.config.MinWindowSize
	}
	if baseWindowSize > bdp.config.MaxWindowSize {
		baseWindowSize = bdp.config.MaxWindowSize
	}
	
	bdp.optimalWindowSize = baseWindowSize
	
	// Optimal buffer size
	bdp.optimalBufferSize = int64(float64(bdp.optimalWindowSize) * bdp.config.BufferMultiplier)
	
	// Set send and receive windows
	bdp.sendWindowSize = bdp.optimalWindowSize
	bdp.receiveWindowSize = bdp.optimalWindowSize
}

func (bdp *BandwidthDelayProductCalculator) adjustBDPForConditions() {
	if bdp.networkConditions == nil {
		return
	}
	
	adjustmentFactor := 1.0
	
	// Adjust for congestion
	if bdp.networkConditions.Congestion > 0.5 {
		adjustmentFactor *= (1.0 - bdp.networkConditions.Congestion*0.5)
	}
	
	// Adjust for packet loss
	if bdp.networkConditions.PacketLoss > 0.01 {
		adjustmentFactor *= (1.0 - bdp.networkConditions.PacketLoss)
	}
	
	// Apply adjustment
	bdp.currentBDP = int64(float64(bdp.currentBDP) * adjustmentFactor)
	bdp.calculateOptimalSizes()
}

// Main calculation and optimization loops
func (bdp *BandwidthDelayProductCalculator) runCalculationLoop() {
	ticker := time.NewTicker(time.Millisecond * 100) // 100ms interval
	defer ticker.Stop()
	
	for {
		select {
		case <-bdp.ctx.Done():
			return
		case <-ticker.C:
			if bdp.isActive {
				bdp.updateCalculations()
			}
		}
	}
}

func (bdp *BandwidthDelayProductCalculator) runOptimizationLoop() {
	ticker := time.NewTicker(bdp.config.OptimizationInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-bdp.ctx.Done():
			return
		case <-ticker.C:
			if bdp.isActive {
				bdp.performOptimization()
			}
		}
	}
}

func (bdp *BandwidthDelayProductCalculator) updateCalculations() {
	bdp.mu.Lock()
	defer bdp.mu.Unlock()
	
	// Update estimators
	bdp.bandwidthEstimator.Update()
	bdp.rttEstimator.Update()
	
	// Recalculate if needed
	newBandwidth := bdp.bandwidthEstimator.GetCurrentBandwidth()
	newRTT := bdp.rttEstimator.GetCurrentRTT()
	
	if math.Abs(newBandwidth-bdp.currentBandwidth) > bdp.currentBandwidth*0.1 ||
		math.Abs(float64(newRTT-bdp.currentRTT)) > float64(bdp.currentRTT)*0.1 {
		
		bdp.currentBandwidth = newBandwidth
		bdp.currentRTT = newRTT
		bdp.calculateBDP()
	}
	
	// Update metrics
	bdp.updateMetrics()
}

func (bdp *BandwidthDelayProductCalculator) performOptimization() {
	bdp.mu.Lock()
	defer bdp.mu.Unlock()
	
	// Analyze recent performance
	performanceScore := bdp.analyzePerformance()
	
	// Check if optimization is needed
	if performanceScore < bdp.config.PerformanceThreshold {
		bdp.optimizeBDP(performanceScore)
	}
}

func (bdp *BandwidthDelayProductCalculator) analyzePerformance() float64 {
	if len(bdp.performanceHistory) < 10 {
		return 0.5 // Neutral score if insufficient data
	}
	
	// Analyze recent performance samples
	recentSamples := bdp.performanceHistory[len(bdp.performanceHistory)-10:]
	
	totalEfficiency := 0.0
	totalOptimality := 0.0
	
	for _, sample := range recentSamples {
		totalEfficiency += sample.Efficiency
		totalOptimality += sample.OptimalityScore
	}
	
	avgEfficiency := totalEfficiency / float64(len(recentSamples))
	avgOptimality := totalOptimality / float64(len(recentSamples))
	
	// Combined performance score
	return (avgEfficiency + avgOptimality) / 2.0
}

func (bdp *BandwidthDelayProductCalculator) optimizeBDP(currentScore float64) {
	oldBDP := bdp.currentBDP
	
	// Choose optimization strategy based on adaptive algorithm
	switch bdp.adaptiveAlgorithm {
	case BDPAlgorithmClassic:
		bdp.classicOptimization()
	case BDPAlgorithmAdaptive:
		bdp.adaptiveOptimization(currentScore)
	case BDPAlgorithmMachineLearning:
		bdp.mlOptimization()
	case BDPAlgorithmHybrid:
		bdp.hybridOptimization(currentScore)
	}
	
	// Record optimization event
	if bdp.currentBDP != oldBDP {
		bdp.recordOptimizationEvent(oldBDP, bdp.currentBDP, "performance_optimization")
	}
}

func (bdp *BandwidthDelayProductCalculator) classicOptimization() {
	// Simple multiplicative increase/decrease
	if bdp.networkConditions.PacketLoss > 0.01 {
		// Decrease BDP due to loss
		bdp.currentBDP = int64(float64(bdp.currentBDP) * 0.9)
	} else if bdp.efficiency < 0.8 {
		// Increase BDP for better utilization
		bdp.currentBDP = int64(float64(bdp.currentBDP) * 1.1)
	}
}

func (bdp *BandwidthDelayProductCalculator) adaptiveOptimization(performanceScore float64) {
	// Adaptive optimization based on recent performance
	if performanceScore < 0.3 {
		// Poor performance - more aggressive adjustment
		if bdp.networkConditions.PacketLoss > 0.005 {
			bdp.currentBDP = int64(float64(bdp.currentBDP) * 0.8)
		} else {
			bdp.currentBDP = int64(float64(bdp.currentBDP) * 1.2)
		}
	} else if performanceScore < 0.7 {
		// Moderate performance - gentle adjustment
		if bdp.networkConditions.PacketLoss > 0.005 {
			bdp.currentBDP = int64(float64(bdp.currentBDP) * 0.95)
		} else {
			bdp.currentBDP = int64(float64(bdp.currentBDP) * 1.05)
		}
	}
	// Good performance (>= 0.7) - no adjustment needed
}

func (bdp *BandwidthDelayProductCalculator) mlOptimization() {
	// Placeholder for ML-based optimization
	// Would use historical data to predict optimal BDP
	if bdp.config.EnableMLOptimization {
		// ML algorithm would go here
		bdp.adaptiveOptimization(0.5) // Fallback to adaptive for now
	}
}

func (bdp *BandwidthDelayProductCalculator) hybridOptimization(performanceScore float64) {
	// Hybrid approach combining multiple strategies
	bdp.adaptiveOptimization(performanceScore)
	
	// Add path prediction if enabled
	if bdp.config.EnablePathPrediction {
		bdp.adjustForPathPrediction()
	}
}

func (bdp *BandwidthDelayProductCalculator) adjustForPathPrediction() {
	// Adjust BDP based on predicted path characteristics
	if bdp.pathCharacteristics.NetworkType == "WAN" {
		// WAN connections typically need larger buffers
		bdp.currentBDP = int64(float64(bdp.currentBDP) * 1.1)
	}
}

// Data recording methods
func (bdp *BandwidthDelayProductCalculator) recordBDPSample() {
	sample := BDPSample{
		Timestamp:             time.Now(),
		Bandwidth:             bdp.currentBandwidth,
		RTT:                   bdp.currentRTT,
		CalculatedBDP:         bdp.currentBDP,
		ActualThroughput:      bdp.calculateActualThroughput(),
		WindowUtilization:     bdp.calculateWindowUtilization(),
		BufferOccupancy:       bdp.calculateBufferOccupancy(),
		NetworkConditionScore: bdp.calculateNetworkConditionScore(),
	}
	
	bdp.historyMu.Lock()
	bdp.bdpHistory = append(bdp.bdpHistory, sample)
	if len(bdp.bdpHistory) > 1000 {
		bdp.bdpHistory = bdp.bdpHistory[1:]
	}
	bdp.historyMu.Unlock()
}

func (bdp *BandwidthDelayProductCalculator) recordOptimizationEvent(oldBDP, newBDP int64, trigger string) {
	event := BDPOptimizationEvent{
		Timestamp:             time.Now(),
		EventType:             bdp.determineOptimizationType(oldBDP, newBDP),
		OldBDP:                oldBDP,
		NewBDP:                newBDP,
		Trigger:               trigger,
		PerformanceImprovement: 0.0, // Would be calculated
		Confidence:            0.8,
	}
	
	bdp.historyMu.Lock()
	bdp.optimizationHistory = append(bdp.optimizationHistory, event)
	if len(bdp.optimizationHistory) > 1000 {
		bdp.optimizationHistory = bdp.optimizationHistory[1:]
	}
	bdp.historyMu.Unlock()
	
	// Update metrics
	bdp.metrics.TotalOptimizations++
}

// Helper calculation methods
func (bdp *BandwidthDelayProductCalculator) calculateActualThroughput() float64 {
	// Placeholder - would measure actual achieved throughput
	return bdp.currentBandwidth * bdp.efficiency
}

func (bdp *BandwidthDelayProductCalculator) calculateWindowUtilization() float64 {
	// Placeholder - would measure how much of the window is utilized
	return 0.85 // Example value
}

func (bdp *BandwidthDelayProductCalculator) calculateBufferOccupancy() float64 {
	// Placeholder - would measure buffer occupancy
	return 0.60 // Example value
}

func (bdp *BandwidthDelayProductCalculator) calculateNetworkConditionScore() float64 {
	if bdp.networkConditions == nil {
		return 0.5
	}
	
	// Calculate overall network condition score
	score := 1.0
	score -= bdp.networkConditions.Congestion * 0.3
	score -= bdp.networkConditions.PacketLoss * 0.4
	score -= math.Min(float64(bdp.networkConditions.Jitter)/float64(time.Millisecond*10), 1.0) * 0.3
	
	return math.Max(score, 0.0)
}

func (bdp *BandwidthDelayProductCalculator) determineOptimizationType(oldBDP, newBDP int64) BDPOptimizationType {
	if newBDP > oldBDP {
		return BDPOptimizationIncrease
	} else if newBDP < oldBDP {
		return BDPOptimizationDecrease
	}
	return BDPOptimizationFinetuning
}

func (bdp *BandwidthDelayProductCalculator) updateMetrics() {
	bdp.metrics.LastUpdate = time.Now()
	
	// Calculate current efficiency
	if bdp.currentBDP > 0 {
		bdp.efficiency = math.Min(bdp.calculateActualThroughput()/bdp.currentBandwidth, 1.0)
		bdp.metrics.AverageBDPUtilization = bdp.efficiency
	}
	
	// Update BDP variance
	bdp.metrics.BDPVariance = bdp.bdpVariance
	
	// Calculate stability score
	if bdp.bdpVariance < bdp.config.VarianceThreshold {
		bdp.metrics.StabilityScore = 1.0 - bdp.bdpVariance/bdp.config.VarianceThreshold
	} else {
		bdp.metrics.StabilityScore = 0.0
	}
}

// Public interface methods
func (bdp *BandwidthDelayProductCalculator) GetCurrentBDP() int64 {
	bdp.mu.RLock()
	defer bdp.mu.RUnlock()
	return bdp.currentBDP
}

func (bdp *BandwidthDelayProductCalculator) GetOptimalBDP() int64 {
	bdp.mu.RLock()
	defer bdp.mu.RUnlock()
	return bdp.optimalBDP
}

func (bdp *BandwidthDelayProductCalculator) GetOptimalWindowSize() int64 {
	bdp.mu.RLock()
	defer bdp.mu.RUnlock()
	return bdp.optimalWindowSize
}

func (bdp *BandwidthDelayProductCalculator) GetOptimalBufferSize() int64 {
	bdp.mu.RLock()
	defer bdp.mu.RUnlock()
	return bdp.optimalBufferSize
}

func (bdp *BandwidthDelayProductCalculator) GetEfficiency() float64 {
	bdp.mu.RLock()
	defer bdp.mu.RUnlock()
	return bdp.efficiency
}

func (bdp *BandwidthDelayProductCalculator) GetMetrics() *BDPMetrics {
	bdp.mu.RLock()
	defer bdp.mu.RUnlock()
	
	// Return a copy of current metrics
	metrics := *bdp.metrics
	return &metrics
}

func (bdp *BandwidthDelayProductCalculator) GetBDPHistory(limit int) []BDPSample {
	bdp.historyMu.Lock()
	defer bdp.historyMu.Unlock()
	
	if limit <= 0 || limit > len(bdp.bdpHistory) {
		limit = len(bdp.bdpHistory)
	}
	
	history := make([]BDPSample, limit)
	copy(history, bdp.bdpHistory[len(bdp.bdpHistory)-limit:])
	return history
}

// Sub-component implementations
func NewBDPBandwidthEstimator() *BDPBandwidthEstimator {
	return &BDPBandwidthEstimator{
		samples:           make([]BDPBandwidthSample, 0, 100),
		currentBandwidth:  0.0,
		smoothedBandwidth: 0.0,
		bandwidthVariance: 0.0,
		estimationMethod:  BandwidthMethodHybrid,
	}
}

func (be *BDPBandwidthEstimator) AddSample(bandwidth float64, timestamp time.Time, confidence float64) {
	be.mu.Lock()
	defer be.mu.Unlock()
	
	sample := BDPBandwidthSample{
		timestamp:  timestamp,
		bandwidth:  bandwidth,
		confidence: confidence,
	}
	
	be.samples = append(be.samples, sample)
	if len(be.samples) > 100 {
		be.samples = be.samples[1:]
	}
	
	be.currentBandwidth = bandwidth
	
	// Update smoothed bandwidth
	if be.smoothedBandwidth == 0 {
		be.smoothedBandwidth = bandwidth
	} else {
		alpha := 0.1 * confidence // Weight by confidence
		be.smoothedBandwidth = be.smoothedBandwidth*(1-alpha) + bandwidth*alpha
	}
}

func (be *BDPBandwidthEstimator) Update() {
	be.mu.Lock()
	defer be.mu.Unlock()
	
	// Clean old samples
	cutoff := time.Now().Add(-time.Minute * 5)
	newSamples := make([]BDPBandwidthSample, 0, len(be.samples))
	for _, sample := range be.samples {
		if sample.timestamp.After(cutoff) {
			newSamples = append(newSamples, sample)
		}
	}
	be.samples = newSamples
}

func (be *BDPBandwidthEstimator) GetCurrentBandwidth() float64 {
	be.mu.RLock()
	defer be.mu.RUnlock()
	return be.smoothedBandwidth
}

func NewBDPRTTEstimator() *BDPRTTEstimator {
	return &BDPRTTEstimator{
		samples:     make([]RTTSampleBDP, 0, 100),
		currentRTT:  0,
		smoothedRTT: 0,
		rttVariance: 0,
		minRTT:      time.Hour, // Start with high value
	}
}

func (re *BDPRTTEstimator) AddSample(rtt time.Duration, timestamp time.Time, measurementType string) {
	re.mu.Lock()
	defer re.mu.Unlock()
	
	sample := RTTSampleBDP{
		timestamp:        timestamp,
		rtt:              rtt,
		measurement_type: measurementType,
	}
	
	re.samples = append(re.samples, sample)
	if len(re.samples) > 100 {
		re.samples = re.samples[1:]
	}
	
	re.currentRTT = rtt
	
	// Update min RTT
	if rtt < re.minRTT {
		re.minRTT = rtt
	}
	
	// Update smoothed RTT
	if re.smoothedRTT == 0 {
		re.smoothedRTT = rtt
	} else {
		alpha := 0.125 // Standard TCP alpha
		re.smoothedRTT = time.Duration(float64(re.smoothedRTT)*(1-alpha) + float64(rtt)*alpha)
	}
}

func (re *BDPRTTEstimator) Update() {
	re.mu.Lock()
	defer re.mu.Unlock()
	
	// Clean old samples
	cutoff := time.Now().Add(-time.Minute * 5)
	newSamples := make([]RTTSampleBDP, 0, len(re.samples))
	for _, sample := range re.samples {
		if sample.timestamp.After(cutoff) {
			newSamples = append(newSamples, sample)
		}
	}
	re.samples = newSamples
}

func (re *BDPRTTEstimator) GetCurrentRTT() time.Duration {
	re.mu.RLock()
	defer re.mu.RUnlock()
	return re.smoothedRTT
}

// Configuration constructors
func NewDefaultBDPConfig() *BDPConfig {
	return &BDPConfig{
		DefaultBandwidth:       100.0, // 100 Mbps
		DefaultRTT:             time.Millisecond * 50,
		MinBDP:                 1024 * 1024,      // 1 MB
		MaxBDP:                 1024 * 1024 * 100, // 100 MB
		
		SmoothingFactor:        0.1,
		VarianceThreshold:      1000000.0, // 1MB variance
		AdaptationRate:         0.05,
		
		WindowSizingFactor:     1.0,
		BufferMultiplier:       2.0,
		MaxWindowSize:          1024 * 1024 * 64, // 64 MB
		MinWindowSize:          1024 * 64,        // 64 KB
		
		OptimizationInterval:   time.Second * 5,
		PerformanceThreshold:   0.7,
		StabilityPeriod:        time.Second * 30,
		
		EnableAdaptiveBDP:      true,
		EnableMLOptimization:   false,
		EnablePathPrediction:   true,
	}
}

func NewDefaultBDPTuningParameters() *BDPTuningParameters {
	return &BDPTuningParameters{
		BandwidthSensitivity:   0.1,
		RTTSensitivity:         0.1,
		LossSensitivity:        0.3,
		
		StabilityFactor:        0.8,
		OscillationDamping:     0.2,
		ConvergenceRate:        0.05,
		
		ExplorationRate:        0.1,
		ExploitationRate:       0.9,
		LearningRate:           0.01,
	}
}

func NewBDPNetworkConditions() *BDPNetworkConditions {
	return &BDPNetworkConditions{
		Congestion:          0.0,
		PacketLoss:          0.0,
		Jitter:              0,
		QueueingDelay:       0,
		BottleneckBandwidth: 0.0,
		PathMTU:             1500,
		ECNCapable:          false,
	}
}

func NewBDPPathCharacteristics() *BDPPathCharacteristics {
	return &BDPPathCharacteristics{
		HopCount:           0,
		GeographicDistance: 0.0,
		NetworkType:        "LAN",
		ISPCharacteristics: "unknown",
		TimeOfDay:          "unknown",
		LoadBalancing:      false,
	}
}

func NewBDPTrafficProfile() *BDPTrafficProfile {
	return &BDPTrafficProfile{
		FlowType:             "bulk",
		DataPattern:          "sequential",
		BurstCharacteristics: &BurstProfile{},
		Priority:             0,
		QoSRequirements:      &QoSProfile{},
	}
}

func NewBDPMetrics() *BDPMetrics {
	return &BDPMetrics{
		LastUpdate:        time.Now(),
		MeasurementPeriod: time.Second * 30,
	}
}