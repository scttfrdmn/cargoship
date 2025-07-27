/*
Package s3 RTT estimation system implements sophisticated round-trip time estimation and management.

This module provides advanced RTT estimation algorithms including smoothed RTT calculation, variance tracking,
jitter analysis, and adaptive timeout management for optimal network performance.
*/
package s3

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

// RTTEstimationSystem provides comprehensive RTT estimation and management
type RTTEstimationSystem struct {
	// Core RTT estimates
	smoothedRTT            time.Duration // SRTT - smoothed RTT
	rttVariance            time.Duration // RTTVAR - RTT variance
	minRTT                 time.Duration // Minimum observed RTT
	maxRTT                 time.Duration // Maximum observed RTT
	baselineRTT            time.Duration // Baseline RTT for comparison
	
	// Current measurements
	currentRTT             time.Duration // Latest RTT measurement
	lastUpdateTime         time.Time     // Time of last update
	
	// Estimation algorithms
	estimationAlgorithm    RTTEstimationAlgorithm
	exponentialEstimator   *ExponentialRTTEstimator
	kalmanEstimator        *KalmanRTTEstimator
	jacobsonEstimator      *JacobsonKarelsEstimator
	adaptiveEstimator      *AdaptiveRTTEstimator
	ensembleEstimator      *EnsembleRTTEstimator
	
	// Sample management
	rttSamples             []RTTMeasurementSample
	sampleWindow           time.Duration
	maxSamples             int
	sampleFilter           *RTTSampleFilter
	outlierDetector        *RTTOutlierDetector
	
	// Jitter analysis  
	jitterEstimator        *JitterEstimator
	currentJitter          time.Duration
	jitterHistory          []JitterSample
	
	// Timeout management
	timeoutCalculator      *RTTTimeoutCalculator
	retransmissionTimeout  time.Duration // RTO
	timeoutHistory         []TimeoutEvent
	adaptiveTimeout        bool
	
	// Quality assessment
	qualityAssessor        *RTTQualityAssessor
	stabilityTracker       *RTTStabilityTracker
	trendAnalyzer          *RTTTrendAnalyzer
	
	// Performance tracking
	metrics                *RTTMetrics
	estimationHistory      []RTTEstimationEvent
	accuracyTracker        *RTTAccuracyTracker
	
	// Configuration
	config                 *RTTConfig
	
	// Context and synchronization
	ctx                    context.Context
	cancel                 context.CancelFunc
	isActive               bool
	mu                     sync.RWMutex
	sampleMu               sync.Mutex
}

// RTTEstimationAlgorithm defines different RTT estimation algorithms
type RTTEstimationAlgorithm string

const (
	RTTAlgorithmExponential    RTTEstimationAlgorithm = "exponential"
	RTTAlgorithmKalman         RTTEstimationAlgorithm = "kalman"
	RTTAlgorithmJacobsonKarels RTTEstimationAlgorithm = "jacobson_karels"
	RTTAlgorithmAdaptive       RTTEstimationAlgorithm = "adaptive"
	RTTAlgorithmEnsemble       RTTEstimationAlgorithm = "ensemble"
)

// RTTMeasurementSample represents a single RTT measurement
type RTTMeasurementSample struct {
	Timestamp              time.Time
	RTT                    time.Duration
	SequenceNumber         uint64
	PacketSize             int64
	MeasurementMethod      RTTMeasurementMethod
	NetworkConditions      *RTTNetworkConditionSnapshot
	IsValid                bool
	Confidence             float64
}

// RTTMeasurementMethod defines how RTT was measured
type RTTMeasurementMethod string

const (
	RTTMethodTCPTimestamp     RTTMeasurementMethod = "tcp_timestamp"
	RTTMethodACKTiming        RTTMeasurementMethod = "ack_timing"
	RTTMethodEchoRequest      RTTMeasurementMethod = "echo_request"
	RTTMethodSynAck           RTTMeasurementMethod = "syn_ack"
	RTTMethodApplicationLevel RTTMeasurementMethod = "application_level"
)

// RTTNetworkConditionSnapshot captures network state during RTT measurement
type RTTNetworkConditionSnapshot struct {
	Bandwidth              float64
	PacketLoss             float64
	Congestion             float64
	QueueingDelay          time.Duration
	PathMTU                int
	HopCount               int
}

// JitterSample represents a jitter measurement
type JitterSample struct {
	Timestamp              time.Time
	Jitter                 time.Duration
	InterarrivalVariation  time.Duration
	PacketSizeVariation    float64
	Method                 JitterMethod
}

// JitterMethod defines how jitter was calculated
type JitterMethod string

const (
	JitterMethodRFC3550    JitterMethod = "rfc3550"    // RFC 3550 RTP jitter
	JitterMethodVariance   JitterMethod = "variance"   // Simple variance
	JitterMethodMAD        JitterMethod = "mad"        // Median Absolute Deviation
	JitterMethodIQR        JitterMethod = "iqr"        // Interquartile Range
)

// TimeoutEvent represents a timeout calculation event
type TimeoutEvent struct {
	Timestamp              time.Time
	RTTEstimate            time.Duration
	RTTVariance            time.Duration
	CalculatedTimeout      time.Duration
	ActualTimeout          time.Duration
	Method                 TimeoutMethod
	WasAccurate            bool
}

// TimeoutMethod defines timeout calculation method
type TimeoutMethod string

const (
	TimeoutMethodRFC6298   TimeoutMethod = "rfc6298"   // RFC 6298
	TimeoutMethodKarn      TimeoutMethod = "karn"      // Karn's algorithm
	TimeoutMethodAdaptive  TimeoutMethod = "adaptive"  // Adaptive timeout
	TimeoutMethodML        TimeoutMethod = "ml"        // Machine learning
)

// RTTEstimationEvent represents an estimation algorithm event
type RTTEstimationEvent struct {
	Timestamp              time.Time
	Algorithm              RTTEstimationAlgorithm
	OldEstimate            time.Duration
	NewEstimate            time.Duration
	SampleRTT              time.Duration
	EstimationError        time.Duration
	Confidence             float64
	AdaptationTriggered    bool
}

// RTTConfig contains RTT estimation configuration
type RTTConfig struct {
	// Algorithm parameters
	EstimationAlgorithm    RTTEstimationAlgorithm
	
	// Exponential smoothing parameters
	Alpha                  float64   // Smoothing factor for SRTT (default: 1/8)
	Beta                   float64   // Smoothing factor for RTTVAR (default: 1/4)
	K                      float64   // Variance multiplier (default: 4)
	G                      float64   // Clock granularity
	
	// Kalman filter parameters
	ProcessNoise           float64   // Process noise variance
	MeasurementNoise       float64   // Measurement noise variance
	InitialEstimateError   float64   // Initial estimate error
	
	// Sample management
	SampleWindow           time.Duration
	MaxSamples             int
	MinSamplesForUpdate    int
	OutlierThreshold       float64
	
	// Jitter parameters
	JitterMethod           JitterMethod
	JitterSmoothingFactor  float64
	MaxJitterSamples       int
	
	// Timeout parameters
	TimeoutMethod          TimeoutMethod
	MinTimeout             time.Duration
	MaxTimeout             time.Duration
	TimeoutMultiplier      float64
	BackoffMultiplier      float64
	
	// Quality thresholds
	StabilityThreshold     float64
	AccuracyThreshold      float64
	TrendDetectionWindow   time.Duration
	
	// Adaptive parameters
	AdaptationEnabled      bool
	AdaptationSensitivity  float64
	LearningRate           float64
}

// RTTMetrics tracks RTT estimation performance
type RTTMetrics struct {
	// Basic statistics
	SampleCount            int64
	AverageRTT             time.Duration
	MedianRTT              time.Duration
	RTTStandardDeviation   time.Duration
	MinRTTObserved         time.Duration
	MaxRTTObserved         time.Duration
	
	// Estimation accuracy
	EstimationAccuracy     float64
	PredictionError        time.Duration
	VarianceExplained      float64
	ConfidenceInterval     float64
	
	// Jitter metrics
	AverageJitter          time.Duration
	JitterStandardDev      time.Duration
	MaxJitterObserved      time.Duration
	
	// Timeout metrics
	TimeoutAccuracy        float64
	FalseTimeouts          int64
	MissedTimeouts         int64
	AverageTimeoutRatio    float64
	
	// Stability metrics
	StabilityScore         float64
	TrendDirection         RTTTrendDirection
	ChangePoints           int64
	SeasonalityDetected    bool
	
	// Algorithm performance
	AlgorithmAccuracy      map[RTTEstimationAlgorithm]float64
	AlgorithmLatency       map[RTTEstimationAlgorithm]time.Duration
	BestAlgorithm          RTTEstimationAlgorithm
	
	// Quality metrics
	DataQuality            float64
	OutlierRate            float64
	NoiseLevel             float64
	
	// Timing
	LastUpdate             time.Time
	MeasurementDuration    time.Duration
}

// RTTTrendDirection defines RTT trend directions
type RTTTrendDirection string

const (
	RTTTrendIncreasing     RTTTrendDirection = "increasing"
	RTTTrendDecreasing     RTTTrendDirection = "decreasing"
	RTTTrendStable         RTTTrendDirection = "stable"
	RTTTrendVolatile       RTTTrendDirection = "volatile"
)

// Constructor
func NewRTTEstimationSystem(ctx context.Context, config *RTTConfig) *RTTEstimationSystem {
	if config == nil {
		config = NewDefaultRTTConfig()
	}
	
	rttCtx, cancel := context.WithCancel(ctx)
	
	rtt := &RTTEstimationSystem{
		smoothedRTT:            time.Millisecond * 100, // Initial estimate
		rttVariance:            time.Millisecond * 50,  // Initial variance
		minRTT:                 time.Hour,              // Start with large value
		maxRTT:                 0,
		baselineRTT:            time.Millisecond * 100,
		
		currentRTT:             0,
		lastUpdateTime:         time.Now(),
		
		estimationAlgorithm:    config.EstimationAlgorithm,
		exponentialEstimator:   NewExponentialRTTEstimator(config.Alpha, config.Beta),
		kalmanEstimator:        NewKalmanRTTEstimator(config.ProcessNoise, config.MeasurementNoise),
		jacobsonEstimator:      NewJacobsonKarelsEstimator(config.Alpha, config.Beta, config.K, config.G),
		adaptiveEstimator:      NewAdaptiveRTTEstimator(),
		ensembleEstimator:      NewEnsembleRTTEstimator(),
		
		rttSamples:             make([]RTTMeasurementSample, 0, config.MaxSamples),
		sampleWindow:           config.SampleWindow,
		maxSamples:             config.MaxSamples,
		sampleFilter:           NewRTTSampleFilter(config.OutlierThreshold),
		outlierDetector:        NewRTTOutlierDetector(),
		
		jitterEstimator:        NewJitterEstimator(config.JitterMethod, config.JitterSmoothingFactor),
		currentJitter:          0,
		jitterHistory:          make([]JitterSample, 0, config.MaxJitterSamples),
		
		timeoutCalculator:      NewRTTTimeoutCalculator(config.TimeoutMethod, config.MinTimeout, config.MaxTimeout),
		retransmissionTimeout:  config.MinTimeout * 2, // Initial conservative estimate
		timeoutHistory:         make([]TimeoutEvent, 0, 1000),
		adaptiveTimeout:        config.AdaptationEnabled,
		
		qualityAssessor:        NewRTTQualityAssessor(),
		stabilityTracker:       NewRTTStabilityTracker(),
		trendAnalyzer:          NewRTTTrendAnalyzer(config.TrendDetectionWindow),
		
		metrics:                NewRTTMetrics(),
		estimationHistory:      make([]RTTEstimationEvent, 0, 1000),
		accuracyTracker:        NewRTTAccuracyTracker(),
		
		config:                 config,
		
		ctx:                    rttCtx,
		cancel:                 cancel,
		isActive:               false,
	}
	
	return rtt
}

// Core RTT estimation methods
func (rtt *RTTEstimationSystem) StartEstimation() error {
	rtt.mu.Lock()
	defer rtt.mu.Unlock()
	
	if rtt.isActive {
		return fmt.Errorf("RTT estimation system already active")
	}
	
	rtt.isActive = true
	rtt.lastUpdateTime = time.Now()
	
	// Start estimation loop
	go rtt.runEstimationLoop()
	
	return nil
}

func (rtt *RTTEstimationSystem) StopEstimation() error {
	rtt.mu.Lock()
	defer rtt.mu.Unlock()
	
	if !rtt.isActive {
		return fmt.Errorf("RTT estimation system not active")
	}
	
	rtt.isActive = false
	rtt.cancel()
	
	return nil
}

func (rtt *RTTEstimationSystem) UpdateRTT(sampleRTT time.Duration, timestamp time.Time, method RTTMeasurementMethod) error {
	if sampleRTT <= 0 {
		return fmt.Errorf("invalid RTT sample: %v", sampleRTT)
	}
	
	rtt.sampleMu.Lock()
	defer rtt.sampleMu.Unlock()
	
	// Create measurement sample
	sample := RTTMeasurementSample{
		Timestamp:         timestamp,
		RTT:               sampleRTT,
		MeasurementMethod: method,
		IsValid:           true,
		Confidence:        1.0, // Default confidence
	}
	
	// Filter outliers if enabled
	if rtt.outlierDetector.IsOutlier(sample, rtt.rttSamples) {
		sample.IsValid = false
		sample.Confidence = 0.1
	}
	
	// Add sample to history
	rtt.addRTTSample(sample)
	
	// Update estimates
	return rtt.updateEstimates(sample)
}

func (rtt *RTTEstimationSystem) GetSmoothedRTT() time.Duration {
	rtt.mu.RLock()
	defer rtt.mu.RUnlock()
	
	return rtt.smoothedRTT
}

func (rtt *RTTEstimationSystem) GetRTTVariance() time.Duration {
	rtt.mu.RLock()
	defer rtt.mu.RUnlock()
	
	return rtt.rttVariance
}

func (rtt *RTTEstimationSystem) GetMinRTT() time.Duration {
	rtt.mu.RLock()
	defer rtt.mu.RUnlock()
	
	return rtt.minRTT
}

func (rtt *RTTEstimationSystem) GetCurrentJitter() time.Duration {
	rtt.mu.RLock()
	defer rtt.mu.RUnlock()
	
	return rtt.currentJitter
}

func (rtt *RTTEstimationSystem) GetRetransmissionTimeout() time.Duration {
	rtt.mu.RLock()
	defer rtt.mu.RUnlock()
	
	return rtt.retransmissionTimeout
}

func (rtt *RTTEstimationSystem) GetMetrics() *RTTMetrics {
	rtt.mu.RLock()
	defer rtt.mu.RUnlock()
	
	// Create a copy with current values
	metrics := *rtt.metrics
	metrics.LastUpdate = time.Now()
	
	return &metrics
}

// Internal estimation implementation
func (rtt *RTTEstimationSystem) runEstimationLoop() {
	ticker := time.NewTicker(time.Second) // 1 second interval
	defer ticker.Stop()
	
	for {
		select {
		case <-rtt.ctx.Done():
			return
		case <-ticker.C:
			if rtt.isActive {
				rtt.performPeriodicUpdate()
			}
		}
	}
}

func (rtt *RTTEstimationSystem) performPeriodicUpdate() {
	rtt.mu.Lock()
	defer rtt.mu.Unlock()
	
	// Update quality assessment
	rtt.qualityAssessor.AssessQuality(rtt.rttSamples)
	
	// Update stability tracking
	rtt.stabilityTracker.UpdateStability(rtt.smoothedRTT, time.Now())
	
	// Update trend analysis
	rtt.trendAnalyzer.UpdateTrends(rtt.rttSamples)
	
	// Update metrics
	rtt.updateMetrics()
	
	// Check for algorithm adaptation
	if rtt.config.AdaptationEnabled {
		rtt.checkAlgorithmAdaptation()
	}
}

func (rtt *RTTEstimationSystem) updateEstimates(sample RTTMeasurementSample) error {
	if !sample.IsValid {
		return nil // Skip invalid samples
	}
	
	rtt.mu.Lock()
	defer rtt.mu.Unlock()
	
	oldSRTT := rtt.smoothedRTT
	_ = rtt.rttVariance // oldRTTVAR for potential future logging
	
	// Update estimates based on selected algorithm
	switch rtt.estimationAlgorithm {
	case RTTAlgorithmExponential:
		rtt.smoothedRTT, rtt.rttVariance = rtt.exponentialEstimator.Update(sample.RTT, rtt.smoothedRTT, rtt.rttVariance)
	case RTTAlgorithmKalman:
		rtt.smoothedRTT, rtt.rttVariance = rtt.kalmanEstimator.Update(sample.RTT)
	case RTTAlgorithmJacobsonKarels:
		rtt.smoothedRTT, rtt.rttVariance = rtt.jacobsonEstimator.Update(sample.RTT, rtt.smoothedRTT, rtt.rttVariance)
	case RTTAlgorithmAdaptive:
		rtt.smoothedRTT, rtt.rttVariance = rtt.adaptiveEstimator.Update(sample, rtt.rttSamples, rtt.smoothedRTT, rtt.rttVariance)
	case RTTAlgorithmEnsemble:
		rtt.smoothedRTT, rtt.rttVariance = rtt.ensembleEstimator.Update(sample, rtt.smoothedRTT, rtt.rttVariance)
	}
	
	// Update min/max RTT
	if sample.RTT < rtt.minRTT {
		rtt.minRTT = sample.RTT
	}
	if sample.RTT > rtt.maxRTT {
		rtt.maxRTT = sample.RTT
	}
	
	// Update current values
	rtt.currentRTT = sample.RTT
	rtt.lastUpdateTime = sample.Timestamp
	
	// Update jitter
	rtt.updateJitter(sample)
	
	// Update timeout
	rtt.updateTimeout()
	
	// Record estimation event
	rtt.recordEstimationEvent(oldSRTT, rtt.smoothedRTT, sample)
	
	return nil
}

func (rtt *RTTEstimationSystem) updateJitter(sample RTTMeasurementSample) {
	jitter := rtt.jitterEstimator.CalculateJitter(sample, rtt.rttSamples)
	
	if jitter > 0 {
		rtt.currentJitter = jitter
		
		// Record jitter sample
		jitterSample := JitterSample{
			Timestamp: sample.Timestamp,
			Jitter:    jitter,
			Method:    rtt.config.JitterMethod,
		}
		
		rtt.jitterHistory = append(rtt.jitterHistory, jitterSample)
		if len(rtt.jitterHistory) > rtt.config.MaxJitterSamples {
			rtt.jitterHistory = rtt.jitterHistory[1:]
		}
	}
}

func (rtt *RTTEstimationSystem) updateTimeout() {
	oldTimeout := rtt.retransmissionTimeout
	
	newTimeout := rtt.timeoutCalculator.CalculateTimeout(rtt.smoothedRTT, rtt.rttVariance, rtt.currentJitter)
	
	if rtt.adaptiveTimeout {
		// Use adaptive timeout calculation
		newTimeout = rtt.calculateAdaptiveTimeout()
	}
	
	// Clamp to configured bounds
	if newTimeout < rtt.config.MinTimeout {
		newTimeout = rtt.config.MinTimeout
	}
	if newTimeout > rtt.config.MaxTimeout {
		newTimeout = rtt.config.MaxTimeout
	}
	
	rtt.retransmissionTimeout = newTimeout
	
	// Record timeout event
	if newTimeout != oldTimeout {
		event := TimeoutEvent{
			Timestamp:         time.Now(),
			RTTEstimate:       rtt.smoothedRTT,
			RTTVariance:       rtt.rttVariance,
			CalculatedTimeout: newTimeout,
			ActualTimeout:     newTimeout,
			Method:            rtt.config.TimeoutMethod,
		}
		
		rtt.timeoutHistory = append(rtt.timeoutHistory, event)
		if len(rtt.timeoutHistory) > 1000 {
			rtt.timeoutHistory = rtt.timeoutHistory[1:]
		}
	}
}

func (rtt *RTTEstimationSystem) calculateAdaptiveTimeout() time.Duration {
	// Adaptive timeout based on recent performance
	baseTimeout := rtt.smoothedRTT + 4*rtt.rttVariance
	
	// Adjust based on jitter
	if rtt.currentJitter > 0 {
		jitterAdjustment := time.Duration(float64(rtt.currentJitter) * 2.0)
		baseTimeout += jitterAdjustment
	}
	
	// Adjust based on stability
	stabilityScore := rtt.stabilityTracker.GetStabilityScore()
	if stabilityScore < 0.8 { // Unstable network
		baseTimeout = time.Duration(float64(baseTimeout) * 1.5)
	}
	
	return baseTimeout
}

func (rtt *RTTEstimationSystem) addRTTSample(sample RTTMeasurementSample) {
	rtt.rttSamples = append(rtt.rttSamples, sample)
	
	// Remove old samples outside window
	cutoff := sample.Timestamp.Add(-rtt.sampleWindow)
	for len(rtt.rttSamples) > 0 && rtt.rttSamples[0].Timestamp.Before(cutoff) {
		rtt.rttSamples = rtt.rttSamples[1:]
	}
	
	// Limit sample count
	if len(rtt.rttSamples) > rtt.maxSamples {
		rtt.rttSamples = rtt.rttSamples[len(rtt.rttSamples)-rtt.maxSamples:]
	}
}

func (rtt *RTTEstimationSystem) checkAlgorithmAdaptation() {
	// Check if current algorithm is performing well
	currentAccuracy := rtt.accuracyTracker.GetAccuracy(rtt.estimationAlgorithm)
	
	// Try other algorithms and compare
	bestAlgorithm := rtt.estimationAlgorithm
	bestAccuracy := currentAccuracy
	
	algorithms := []RTTEstimationAlgorithm{
		RTTAlgorithmExponential,
		RTTAlgorithmKalman,
		RTTAlgorithmJacobsonKarels,
		RTTAlgorithmAdaptive,
		RTTAlgorithmEnsemble,
	}
	
	for _, algorithm := range algorithms {
		if algorithm == rtt.estimationAlgorithm {
			continue
		}
		
		accuracy := rtt.accuracyTracker.GetAccuracy(algorithm)
		if accuracy > bestAccuracy+rtt.config.AdaptationSensitivity {
			bestAlgorithm = algorithm
			bestAccuracy = accuracy
		}
	}
	
	// Switch algorithm if significantly better
	if bestAlgorithm != rtt.estimationAlgorithm {
		rtt.estimationAlgorithm = bestAlgorithm
		rtt.metrics.BestAlgorithm = bestAlgorithm
	}
}

func (rtt *RTTEstimationSystem) recordEstimationEvent(oldSRTT, newSRTT time.Duration, sample RTTMeasurementSample) {
	estimationError := time.Duration(0)
	if sample.RTT > newSRTT {
		estimationError = sample.RTT - newSRTT
	} else {
		estimationError = newSRTT - sample.RTT
	}
	
	event := RTTEstimationEvent{
		Timestamp:       sample.Timestamp,
		Algorithm:       rtt.estimationAlgorithm,
		OldEstimate:     oldSRTT,
		NewEstimate:     newSRTT,
		SampleRTT:       sample.RTT,
		EstimationError: estimationError,
		Confidence:      sample.Confidence,
	}
	
	rtt.estimationHistory = append(rtt.estimationHistory, event)
	if len(rtt.estimationHistory) > 1000 {
		rtt.estimationHistory = rtt.estimationHistory[1:]
	}
	
	// Update accuracy tracker
	rtt.accuracyTracker.RecordEstimation(rtt.estimationAlgorithm, estimationError, sample.Confidence)
}

func (rtt *RTTEstimationSystem) updateMetrics() {
	rtt.metrics.LastUpdate = time.Now()
	rtt.metrics.SampleCount = int64(len(rtt.rttSamples))
	
	if len(rtt.rttSamples) > 0 {
		// Calculate basic statistics
		rttValues := make([]time.Duration, 0, len(rtt.rttSamples))
		totalRTT := time.Duration(0)
		
		for _, sample := range rtt.rttSamples {
			if sample.IsValid {
				rttValues = append(rttValues, sample.RTT)
				totalRTT += sample.RTT
			}
		}
		
		if len(rttValues) > 0 {
			rtt.metrics.AverageRTT = totalRTT / time.Duration(len(rttValues))
			
			// Calculate median
			sort.Slice(rttValues, func(i, j int) bool {
				return rttValues[i] < rttValues[j]
			})
			if len(rttValues)%2 == 0 {
				rtt.metrics.MedianRTT = (rttValues[len(rttValues)/2-1] + rttValues[len(rttValues)/2]) / 2
			} else {
				rtt.metrics.MedianRTT = rttValues[len(rttValues)/2]
			}
			
			// Calculate standard deviation
			if len(rttValues) > 1 {
				variance := time.Duration(0)
				for _, value := range rttValues {
					diff := value - rtt.metrics.AverageRTT
					variance += time.Duration(int64(diff) * int64(diff))
				}
				variance /= time.Duration(len(rttValues) - 1)
				rtt.metrics.RTTStandardDeviation = time.Duration(math.Sqrt(float64(variance)))
			}
		}
	}
	
	// Update other metrics
	rtt.metrics.MinRTTObserved = rtt.minRTT
	rtt.metrics.MaxRTTObserved = rtt.maxRTT
	rtt.metrics.AverageJitter = rtt.currentJitter
	rtt.metrics.StabilityScore = rtt.stabilityTracker.GetStabilityScore()
	rtt.metrics.TrendDirection = rtt.trendAnalyzer.GetTrendDirection()
	
	// Update algorithm performance
	if rtt.metrics.AlgorithmAccuracy == nil {
		rtt.metrics.AlgorithmAccuracy = make(map[RTTEstimationAlgorithm]float64)
		rtt.metrics.AlgorithmLatency = make(map[RTTEstimationAlgorithm]time.Duration)
	}
	
	for _, algorithm := range []RTTEstimationAlgorithm{
		RTTAlgorithmExponential, RTTAlgorithmKalman, RTTAlgorithmJacobsonKarels,
		RTTAlgorithmAdaptive, RTTAlgorithmEnsemble,
	} {
		rtt.metrics.AlgorithmAccuracy[algorithm] = rtt.accuracyTracker.GetAccuracy(algorithm)
	}
}

// Supporting component implementations
type ExponentialRTTEstimator struct {
	alpha  float64 // Smoothing factor for SRTT
	beta   float64 // Smoothing factor for RTTVAR
}

func NewExponentialRTTEstimator(alpha, beta float64) *ExponentialRTTEstimator {
	return &ExponentialRTTEstimator{
		alpha: alpha,
		beta:  beta,
	}
}

func (e *ExponentialRTTEstimator) Update(sampleRTT, smoothedRTT, rttVariance time.Duration) (time.Duration, time.Duration) {
	// RFC 6298 algorithm
	if smoothedRTT == 0 {
		// First measurement
		smoothedRTT = sampleRTT
		rttVariance = sampleRTT / 2
	} else {
		// Subsequent measurements
		rttDiff := sampleRTT - smoothedRTT
		if rttDiff < 0 {
			rttDiff = -rttDiff
		}
		
		rttVariance = time.Duration((1.0-e.beta)*float64(rttVariance) + e.beta*float64(rttDiff))
		smoothedRTT = time.Duration((1.0-e.alpha)*float64(smoothedRTT) + e.alpha*float64(sampleRTT))
	}
	
	return smoothedRTT, rttVariance
}

func (e *ExponentialRTTEstimator) GetAccuracy() float64 {
	return 0.85 // Default accuracy for exponential smoothing
}

type KalmanRTTEstimator struct {
	processNoise     float64
	measurementNoise float64
	estimate         float64
	errorCovariance  float64
	// TODO: Add mutex for thread safety
	// mu               sync.RWMutex
}

func NewKalmanRTTEstimator(processNoise, measurementNoise float64) *KalmanRTTEstimator {
	return &KalmanRTTEstimator{
		processNoise:     processNoise,
		measurementNoise: measurementNoise,
		estimate:         100.0, // Initial estimate in milliseconds
		errorCovariance:  1000.0, // Initial error covariance
	}
}

func (k *KalmanRTTEstimator) Update(measurement time.Duration) (time.Duration, time.Duration) {
	// Kalman filter prediction step
	k.errorCovariance += k.processNoise
	
	// Kalman filter update step
	measurementMs := float64(measurement.Nanoseconds()) / 1e6
	kalmanGain := k.errorCovariance / (k.errorCovariance + k.measurementNoise)
	
	k.estimate = k.estimate + kalmanGain*(measurementMs-k.estimate)
	k.errorCovariance = (1.0 - kalmanGain) * k.errorCovariance
	
	estimatedRTT := time.Duration(k.estimate * 1e6) // Convert back to duration
	variance := time.Duration(k.errorCovariance * 1e6)
	
	return estimatedRTT, variance
}

type JacobsonKarelsEstimator struct {
	alpha float64 // SRTT smoothing factor
	beta  float64 // RTTVAR smoothing factor
	k     float64 // Variance multiplier
	g     float64 // Clock granularity
}

func NewJacobsonKarelsEstimator(alpha, beta, k, g float64) *JacobsonKarelsEstimator {
	return &JacobsonKarelsEstimator{
		alpha: alpha,
		beta:  beta,
		k:     k,
		g:     g,
	}
}

func (j *JacobsonKarelsEstimator) Update(sampleRTT, smoothedRTT, rttVariance time.Duration) (time.Duration, time.Duration) {
	// Jacobson/Karels algorithm with improvements
	if smoothedRTT == 0 {
		smoothedRTT = sampleRTT
		rttVariance = sampleRTT / 2
	} else {
		err := sampleRTT - smoothedRTT
		smoothedRTT = time.Duration(float64(smoothedRTT) + j.alpha*float64(err))
		
		if err < 0 {
			err = -err
		}
		rttVariance = time.Duration(float64(rttVariance) + j.beta*(float64(err)-float64(rttVariance)))
	}
	
	return smoothedRTT, rttVariance
}

func (j *JacobsonKarelsEstimator) GetAccuracy() float64 {
	return 0.90 // High accuracy for Jacobson-Karels
}

type AdaptiveRTTEstimator struct {
	windowSize     int
	learningRate   float64
	adaptationRate float64
	// TODO: Add mutex for thread safety
	// mu             sync.RWMutex
}

func NewAdaptiveRTTEstimator() *AdaptiveRTTEstimator {
	return &AdaptiveRTTEstimator{
		windowSize:     20,
		learningRate:   0.1,
		adaptationRate: 0.05,
	}
}

func (a *AdaptiveRTTEstimator) Update(sample RTTMeasurementSample, samples []RTTMeasurementSample, smoothedRTT, rttVariance time.Duration) (time.Duration, time.Duration) {
	// Adaptive estimation based on recent samples
	if len(samples) < 2 {
		return sample.RTT, sample.RTT / 4
	}
	
	// Calculate adaptive smoothing factor based on variance
	recentSamples := samples
	if len(samples) > a.windowSize {
		recentSamples = samples[len(samples)-a.windowSize:]
	}
	
	variance := a.calculateVariance(recentSamples)
	adaptiveFactor := a.learningRate * (1.0 + variance)
	
	// Update estimates
	newSRTT := time.Duration((1.0-adaptiveFactor)*float64(smoothedRTT) + adaptiveFactor*float64(sample.RTT))
	
	rttDiff := sample.RTT - newSRTT
	if rttDiff < 0 {
		rttDiff = -rttDiff
	}
	newRTTVAR := time.Duration((1.0-adaptiveFactor)*float64(rttVariance) + adaptiveFactor*float64(rttDiff))
	
	return newSRTT, newRTTVAR
}

func (a *AdaptiveRTTEstimator) calculateVariance(samples []RTTMeasurementSample) float64 {
	if len(samples) < 2 {
		return 0.0
	}
	
	// Calculate sample variance
	sum := time.Duration(0)
	for _, sample := range samples {
		sum += sample.RTT
	}
	mean := sum / time.Duration(len(samples))
	
	variance := 0.0
	for _, sample := range samples {
		diff := float64(sample.RTT - mean)
		variance += diff * diff
	}
	variance /= float64(len(samples) - 1)
	
	// Normalize variance
	return variance / (float64(mean) * float64(mean))
}

type EnsembleRTTEstimator struct {
	estimators []RTTSystemEstimator
	weights    []float64
	// TODO: Add mutex for thread safety
	// mu         sync.RWMutex
}

type RTTSystemEstimator interface {
	Update(sampleRTT, smoothedRTT, rttVariance time.Duration) (time.Duration, time.Duration)
	GetAccuracy() float64
}

func NewEnsembleRTTEstimator() *EnsembleRTTEstimator {
	ensemble := &EnsembleRTTEstimator{
		estimators: make([]RTTSystemEstimator, 0),
		weights:    make([]float64, 0),
	}
	
	// Add different estimators
	ensemble.estimators = append(ensemble.estimators, NewExponentialRTTEstimator(0.125, 0.25))
	ensemble.weights = append(ensemble.weights, 0.4)
	
	ensemble.estimators = append(ensemble.estimators, NewJacobsonKarelsEstimator(0.125, 0.25, 4.0, 0.1))
	ensemble.weights = append(ensemble.weights, 0.6)
	
	return ensemble
}

func (e *EnsembleRTTEstimator) Update(sample RTTMeasurementSample, smoothedRTT, rttVariance time.Duration) (time.Duration, time.Duration) {
	// Weighted combination of estimator outputs
	totalWeight := 0.0
	weightedSRTT := 0.0
	weightedRTTVAR := 0.0
	
	for i, estimator := range e.estimators {
		srtt, rttvar := estimator.Update(sample.RTT, smoothedRTT, rttVariance)
		weight := e.weights[i]
		
		weightedSRTT += weight * float64(srtt)
		weightedRTTVAR += weight * float64(rttvar)
		totalWeight += weight
	}
	
	if totalWeight > 0 {
		weightedSRTT /= totalWeight
		weightedRTTVAR /= totalWeight
	}
	
	return time.Duration(weightedSRTT), time.Duration(weightedRTTVAR)
}

// Supporting types
type RTTSampleFilter struct {
	outlierThreshold float64
}

func NewRTTSampleFilter(threshold float64) *RTTSampleFilter {
	return &RTTSampleFilter{
		outlierThreshold: threshold,
	}
}

type RTTOutlierDetector struct {
	detectionMethod string
}

func NewRTTOutlierDetector() *RTTOutlierDetector {
	return &RTTOutlierDetector{
		detectionMethod: "iqr",
	}
}

func (rod *RTTOutlierDetector) IsOutlier(sample RTTMeasurementSample, samples []RTTMeasurementSample) bool {
	if len(samples) < 10 {
		return false // Need sufficient samples
	}
	
	// Simple IQR-based outlier detection
	rttValues := make([]float64, 0, len(samples))
	for _, s := range samples {
		if s.IsValid {
			rttValues = append(rttValues, float64(s.RTT))
		}
	}
	
	if len(rttValues) < 4 {
		return false
	}
	
	sort.Float64s(rttValues)
	
	q1 := rttValues[len(rttValues)/4]
	q3 := rttValues[3*len(rttValues)/4]
	iqr := q3 - q1
	
	lowerBound := q1 - 1.5*iqr
	upperBound := q3 + 1.5*iqr
	
	sampleValue := float64(sample.RTT)
	return sampleValue < lowerBound || sampleValue > upperBound
}

type JitterEstimator struct {
	method          JitterMethod
	smoothingFactor float64
	lastArrival     time.Time
	smoothedJitter  float64
}

func NewJitterEstimator(method JitterMethod, smoothingFactor float64) *JitterEstimator {
	return &JitterEstimator{
		method:          method,
		smoothingFactor: smoothingFactor,
		smoothedJitter:  0.0,
	}
}

func (je *JitterEstimator) CalculateJitter(sample RTTMeasurementSample, samples []RTTMeasurementSample) time.Duration {
	switch je.method {
	case JitterMethodRFC3550:
		return je.calculateRFC3550Jitter(sample)
	case JitterMethodVariance:
		return je.calculateVarianceJitter(samples)
	default:
		return je.calculateRFC3550Jitter(sample)
	}
}

func (je *JitterEstimator) calculateRFC3550Jitter(sample RTTMeasurementSample) time.Duration {
	// RFC 3550 jitter calculation
	if je.lastArrival.IsZero() {
		je.lastArrival = sample.Timestamp
		return 0
	}
	
	// Calculate interarrival jitter
	arrivalDiff := sample.Timestamp.Sub(je.lastArrival)
	transitDiff := math.Abs(float64(sample.RTT) - float64(arrivalDiff))
	
	je.smoothedJitter += (transitDiff - je.smoothedJitter) / 16.0
	je.lastArrival = sample.Timestamp
	
	return time.Duration(je.smoothedJitter)
}

func (je *JitterEstimator) calculateVarianceJitter(samples []RTTMeasurementSample) time.Duration {
	if len(samples) < 2 {
		return 0
	}
	
	// Calculate RTT variance as jitter
	sum := time.Duration(0)
	for _, sample := range samples {
		sum += sample.RTT
	}
	mean := sum / time.Duration(len(samples))
	
	variance := 0.0
	for _, sample := range samples {
		diff := float64(sample.RTT - mean)
		variance += diff * diff
	}
	variance /= float64(len(samples) - 1)
	
	return time.Duration(math.Sqrt(variance))
}

type RTTTimeoutCalculator struct {
	method     TimeoutMethod
	minTimeout time.Duration
	maxTimeout time.Duration
}

func NewRTTTimeoutCalculator(method TimeoutMethod, minTimeout, maxTimeout time.Duration) *RTTTimeoutCalculator {
	return &RTTTimeoutCalculator{
		method:     method,
		minTimeout: minTimeout,
		maxTimeout: maxTimeout,
	}
}

func (rtc *RTTTimeoutCalculator) CalculateTimeout(smoothedRTT, rttVariance, jitter time.Duration) time.Duration {
	switch rtc.method {
	case TimeoutMethodRFC6298:
		return rtc.calculateRFC6298Timeout(smoothedRTT, rttVariance)
	case TimeoutMethodKarn:
		return rtc.calculateKarnTimeout(smoothedRTT, rttVariance)
	case TimeoutMethodAdaptive:
		return rtc.calculateAdaptiveTimeout(smoothedRTT, rttVariance, jitter)
	default:
		return rtc.calculateRFC6298Timeout(smoothedRTT, rttVariance)
	}
}

func (rtc *RTTTimeoutCalculator) calculateRFC6298Timeout(smoothedRTT, rttVariance time.Duration) time.Duration {
	// RTO = SRTT + max(G, K * RTTVAR)
	timeout := smoothedRTT + 4*rttVariance
	
	if timeout < rtc.minTimeout {
		timeout = rtc.minTimeout
	}
	if timeout > rtc.maxTimeout {
		timeout = rtc.maxTimeout
	}
	
	return timeout
}

func (rtc *RTTTimeoutCalculator) calculateKarnTimeout(smoothedRTT, rttVariance time.Duration) time.Duration {
	// Karn's algorithm with backoff
	timeout := smoothedRTT + 2*rttVariance
	
	if timeout < rtc.minTimeout {
		timeout = rtc.minTimeout
	}
	if timeout > rtc.maxTimeout {
		timeout = rtc.maxTimeout
	}
	
	return timeout
}

func (rtc *RTTTimeoutCalculator) calculateAdaptiveTimeout(smoothedRTT, rttVariance, jitter time.Duration) time.Duration {
	// Adaptive timeout incorporating jitter
	baseTimeout := smoothedRTT + 4*rttVariance
	jitterComponent := time.Duration(float64(jitter) * 2.0)
	
	timeout := baseTimeout + jitterComponent
	
	if timeout < rtc.minTimeout {
		timeout = rtc.minTimeout
	}
	if timeout > rtc.maxTimeout {
		timeout = rtc.maxTimeout
	}
	
	return timeout
}

// Stub implementations for remaining components
type RTTQualityAssessor struct{}
type RTTStabilityTracker struct{}
type RTTTrendAnalyzer struct{}
type RTTAccuracyTracker struct{}

func NewRTTQualityAssessor() *RTTQualityAssessor { return &RTTQualityAssessor{} }
func NewRTTStabilityTracker() *RTTStabilityTracker { return &RTTStabilityTracker{} }
func NewRTTTrendAnalyzer(window time.Duration) *RTTTrendAnalyzer { return &RTTTrendAnalyzer{} }
func NewRTTAccuracyTracker() *RTTAccuracyTracker { return &RTTAccuracyTracker{} }

func (rqa *RTTQualityAssessor) AssessQuality(samples []RTTMeasurementSample) float64 { return 0.8 }
func (rst *RTTStabilityTracker) UpdateStability(rtt time.Duration, timestamp time.Time) {}
func (rst *RTTStabilityTracker) GetStabilityScore() float64 { return 0.8 }
func (rta *RTTTrendAnalyzer) UpdateTrends(samples []RTTMeasurementSample) {}
func (rta *RTTTrendAnalyzer) GetTrendDirection() RTTTrendDirection { return RTTTrendStable }
func (rat *RTTAccuracyTracker) RecordEstimation(algorithm RTTEstimationAlgorithm, error time.Duration, confidence float64) {}
func (rat *RTTAccuracyTracker) GetAccuracy(algorithm RTTEstimationAlgorithm) float64 { return 0.8 }

func NewRTTMetrics() *RTTMetrics {
	return &RTTMetrics{
		AlgorithmAccuracy: make(map[RTTEstimationAlgorithm]float64),
		AlgorithmLatency:  make(map[RTTEstimationAlgorithm]time.Duration),
		LastUpdate:        time.Now(),
	}
}

func NewDefaultRTTConfig() *RTTConfig {
	return &RTTConfig{
		EstimationAlgorithm:    RTTAlgorithmJacobsonKarels,
		Alpha:                  0.125, // RFC 6298
		Beta:                   0.25,  // RFC 6298
		K:                      4.0,   // RFC 6298
		G:                      0.1,   // 100ms clock granularity
		ProcessNoise:           0.1,
		MeasurementNoise:       1.0,
		InitialEstimateError:   100.0,
		SampleWindow:           time.Minute * 5,
		MaxSamples:             1000,
		MinSamplesForUpdate:    3,
		OutlierThreshold:       2.0,
		JitterMethod:           JitterMethodRFC3550,
		JitterSmoothingFactor:  0.1,
		MaxJitterSamples:       100,
		TimeoutMethod:          TimeoutMethodRFC6298,
		MinTimeout:             time.Second,
		MaxTimeout:             time.Second * 60,
		TimeoutMultiplier:      2.0,
		BackoffMultiplier:      2.0,
		StabilityThreshold:     0.8,
		AccuracyThreshold:      0.9,
		TrendDetectionWindow:   time.Minute * 2,
		AdaptationEnabled:      true,
		AdaptationSensitivity:  0.1,
		LearningRate:           0.01,
	}
}