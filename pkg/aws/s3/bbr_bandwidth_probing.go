/*
Package s3 BBR bandwidth probing implements Google's BBR-style congestion control algorithm.

This module provides sophisticated bandwidth probing based on Google's BBR (Bottleneck Bandwidth and Round-trip time)
algorithm, enabling optimal bandwidth utilization through continuous probing and adaptive rate control.
*/
package s3

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// BBRBandwidthProber implements Google's BBR-style bandwidth probing algorithm
type BBRBandwidthProber struct {
	// Core BBR state
	state                  BBRState
	mode                   BBRProbingMode
	stateStartTime         time.Time
	modeStartTime          time.Time
	
	// Bandwidth estimation
	maxBandwidth           float64 // Max observed bandwidth (Mbps)
	bandwidthFilter        *BandwidthMaxFilter
	// deliveryRateEstimator  *BBRDeliveryRateEstimator // Reserved for future use
	bandwidthSamples       []BBRBandwidthSample
	
	// RTT tracking
	minRTT                 time.Duration
	rttProbeStartTime      time.Time
	rttFilter              *RTTMinFilter
	rttSamples             []RTTSample
	
	// Probing parameters
	probeBandwidthGain     float64
	probeRTTGain           float64
	drainGain              float64
	startupGain            float64
	startupThreshold       float64
	
	// Pacing and congestion window
	pacingRate             float64 // Current pacing rate (Mbps)
	congestionWindow       int64   // Congestion window (bytes)
	sendQuantum            int64   // Send quantum size
	pacingGain             float64 // Current pacing gain
	cwndGain               float64 // Current congestion window gain
	
	// Cycle tracking
	cycleIndex             int
	cycleStartTime         time.Time
	probeBWCycleGains      []float64
	probeRTTCycleLength    time.Duration
	
	// Application limits and flow control
	appLimited             bool
	appLimitedUntil        int64
	deliveredBytes         int64
	deliveredTime          time.Time
	firstSentTime          time.Time
	
	// Loss detection
	// lossDetector           *BBRLossDetector // Reserved for future use
	lossThreshold          float64
	consecutiveLossCount   int
	
	// Configuration
	config                 *BBRConfig
	
	// Performance tracking
	metrics                *BBRMetrics
	probeHistory           []BBRProbeEvent
	adaptationHistory      []BBRAdaptationEvent
	
	// Context and synchronization
	ctx                    context.Context
	cancel                 context.CancelFunc
	isActive               bool
	mu                     sync.RWMutex
	sampleMu               sync.Mutex
}

// BBRState represents the current state of the BBR algorithm
type BBRState string

const (
	BBRStateStartup      BBRState = "startup"
	BBRStateDrain        BBRState = "drain"
	BBRStateProbeBW      BBRState = "probe_bw"
	BBRStateProbeRTT     BBRState = "probe_rtt"
)

// BBRProbingMode represents the current probing mode
type BBRProbingMode string

const (
	BBRProbingModeStartup       BBRProbingMode = "startup"
	BBRProbingModeDrain         BBRProbingMode = "drain"
	BBRProbingModeProbeBW       BBRProbingMode = "probe_bw"
	BBRProbingModeProbeRTT      BBRProbingMode = "probe_rtt"
)

// BBRBandwidthSample represents a bandwidth measurement sample
type BBRBandwidthSample struct {
	Timestamp         time.Time
	DeliveryRate      float64 // Mbps
	BytesDelivered    int64
	RTT               time.Duration
	LossRate          float64
	IsAppLimited      bool
	PacketCount       int64
	SampleRTT         time.Duration
}

// RTTSample represents an RTT measurement sample
type RTTSample struct {
	Timestamp         time.Time
	RTT               time.Duration
	MinRTT            time.Duration
	SmoothedRTT       time.Duration
	RTTVariance       time.Duration
	IsValid           bool
}

// BBRConfig contains configuration parameters for BBR
type BBRConfig struct {
	// Core parameters
	HighGain              float64
	DrainGain             float64
	StartupThreshold      float64
	ProbeRTTDuration      time.Duration
	
	// Bandwidth probing
	ProbeBWCycleLength    int
	ProbeBWGainCycle      []float64
	BandwidthWindowLength time.Duration
	
	// RTT probing
	RTTWindowLength       time.Duration
	ProbeRTTInterval      time.Duration
	MinRTTExpiry          time.Duration
	
	// Loss handling
	LossThreshold         float64
	BetaLoss              float64
	
	// Pacing
	PacingMarginPercent   float64
	MaxPacingRate         float64
	MinPacingRate         float64
	
	// Congestion window
	InitialCongestionWindow int64
	MinCongestionWindow     int64
	MaxCongestionWindow     int64
	CwndGain               float64
}

// BBRMetrics tracks BBR performance metrics
type BBRMetrics struct {
	// State transitions
	StateTransitions       map[BBRState]int64
	ModeTransitions        map[BBRProbingMode]int64
	TotalProbes            int64
	SuccessfulProbes       int64
	
	// Bandwidth metrics
	MaxBandwidthObserved   float64
	AverageBandwidth       float64
	BandwidthUtilization   float64
	ProbingOverhead        float64
	
	// RTT metrics
	MinRTTObserved         time.Duration
	AverageRTT             time.Duration
	RTTVariation           time.Duration
	RTTStability           float64
	
	// Loss metrics
	TotalLossEvents        int64
	LossRate               float64
	RecoveryTime           time.Duration
	
	// Performance metrics
	Throughput             float64
	Efficiency             float64
	Fairness               float64
	Responsiveness         float64
	
	// Timing
	LastUpdate             time.Time
	ProbeInterval          time.Duration
	AdaptationLatency      time.Duration
}

// BBRProbeEvent represents a bandwidth probing event
type BBRProbeEvent struct {
	Timestamp             time.Time
	ProbeType             BBRProbeType
	StartBandwidth        float64
	EndBandwidth          float64
	RTTMeasured           time.Duration
	LossDetected          bool
	ProbeSuccess          bool
	AdaptationTriggered   bool
	StateTransition       BBRState
}

// BBRProbeType defines different types of probes
type BBRProbeType string

const (
	BBRProbeTypeStartup   BBRProbeType = "startup"
	BBRProbeTypeBandwidth BBRProbeType = "bandwidth"
	BBRProbeTypeRTT       BBRProbeType = "rtt"
	BBRProbeTypeDrain     BBRProbeType = "drain"
)

// BBRAdaptationEvent represents an adaptation decision
type BBRAdaptationEvent struct {
	Timestamp             time.Time
	Trigger               string
	OldState              BBRState
	NewState              BBRState
	BandwidthChange       float64
	RTTChange             time.Duration
	CongestionWindowChange int64
	PacingRateChange      float64
	Reason               string
}

// Constructor
func NewBBRBandwidthProber(ctx context.Context, config *BBRConfig) *BBRBandwidthProber {
	if config == nil {
		config = NewDefaultBBRConfig()
	}
	
	proberCtx, cancel := context.WithCancel(ctx)
	
	bbr := &BBRBandwidthProber{
		state:                BBRStateStartup,
		mode:                 BBRProbingModeStartup,
		stateStartTime:      time.Now(),
		modeStartTime:       time.Now(),
		
		maxBandwidth:        10.0, // Initial 10 Mbps
		bandwidthFilter:     NewBandwidthMaxFilter(config.BandwidthWindowLength),
		// deliveryRateEstimator: NewBBRDeliveryRateEstimator(), // Reserved for future use
		bandwidthSamples:    make([]BBRBandwidthSample, 0, 1000),
		
		minRTT:              time.Millisecond * 100, // Initial 100ms
		rttFilter:           NewRTTMinFilter(config.RTTWindowLength),
		rttSamples:          make([]RTTSample, 0, 1000),
		
		probeBandwidthGain:  config.HighGain,
		probeRTTGain:        1.0,
		drainGain:           config.DrainGain,
		startupGain:         config.HighGain,
		startupThreshold:    config.StartupThreshold,
		
		pacingRate:          10.0, // Initial 10 Mbps
		congestionWindow:    config.InitialCongestionWindow,
		sendQuantum:         1024 * 1024, // 1MB
		pacingGain:          1.0,
		cwndGain:            config.CwndGain,
		
		cycleIndex:          0,
		probeBWCycleGains:   config.ProbeBWGainCycle,
		probeRTTCycleLength: config.ProbeRTTDuration,
		
		appLimited:          false,
		deliveredBytes:      0,
		deliveredTime:       time.Now(),
		firstSentTime:       time.Now(),
		
		// lossDetector:        NewBBRLossDetector(), // Reserved for future use
		lossThreshold:       config.LossThreshold,
		consecutiveLossCount: 0,
		
		config:             config,
		metrics:            NewBBRMetrics(),
		probeHistory:       make([]BBRProbeEvent, 0, 1000),
		adaptationHistory:  make([]BBRAdaptationEvent, 0, 1000),
		
		ctx:                proberCtx,
		cancel:             cancel,
		isActive:           false,
	}
	
	return bbr
}

// Core BBR algorithm methods
func (bbr *BBRBandwidthProber) StartProbing() error {
	bbr.mu.Lock()
	defer bbr.mu.Unlock()
	
	if bbr.isActive {
		return fmt.Errorf("BBR probing already active")
	}
	
	bbr.isActive = true
	bbr.stateStartTime = time.Now()
	bbr.modeStartTime = time.Now()
	
	// Start the main BBR loop
	go bbr.runBBRLoop()
	
	return nil
}

func (bbr *BBRBandwidthProber) StopProbing() error {
	bbr.mu.Lock()
	defer bbr.mu.Unlock()
	
	if !bbr.isActive {
		return fmt.Errorf("BBR probing not active")
	}
	
	bbr.isActive = false
	bbr.cancel()
	
	return nil
}

func (bbr *BBRBandwidthProber) OnPacketSent(packetSize int64, sendTime time.Time) {
	bbr.sampleMu.Lock()
	defer bbr.sampleMu.Unlock()
	
	if bbr.firstSentTime.IsZero() {
		bbr.firstSentTime = sendTime
	}
	
	// Update application-limited tracking
	if bbr.appLimited {
		bbr.appLimitedUntil = bbr.deliveredBytes + packetSize
	}
}

func (bbr *BBRBandwidthProber) OnPacketAcknowledged(packetSize int64, sendTime, ackTime time.Time, rtt time.Duration) {
	bbr.sampleMu.Lock()
	defer bbr.sampleMu.Unlock()
	
	// Update delivered bytes
	bbr.deliveredBytes += packetSize
	bbr.deliveredTime = ackTime
	
	// Calculate delivery rate
	deliveryRate := bbr.calculateDeliveryRate(packetSize, sendTime, ackTime)
	
	// Create bandwidth sample
	sample := BBRBandwidthSample{
		Timestamp:      ackTime,
		DeliveryRate:   deliveryRate,
		BytesDelivered: packetSize,
		RTT:            rtt,
		LossRate:       0.0, // Will be updated by loss detector
		IsAppLimited:   bbr.isAppLimited(),
		PacketCount:    1,
		SampleRTT:      rtt,
	}
	
	// Update bandwidth and RTT estimates
	bbr.updateBandwidthEstimate(sample)
	bbr.updateRTTEstimate(rtt, ackTime)
	
	// Record sample
	bbr.recordBandwidthSample(sample)
}

func (bbr *BBRBandwidthProber) OnPacketLost(packetSize int64, sendTime time.Time) {
	bbr.sampleMu.Lock()
	defer bbr.sampleMu.Unlock()
	
	// Update loss tracking
	bbr.consecutiveLossCount++
	
	// Trigger loss-based adaptation if threshold exceeded
	if float64(bbr.consecutiveLossCount) > bbr.lossThreshold {
		bbr.handleLossEvent()
	}
}

func (bbr *BBRBandwidthProber) GetCurrentBandwidth() float64 {
	bbr.mu.RLock()
	defer bbr.mu.RUnlock()
	
	return bbr.maxBandwidth
}

func (bbr *BBRBandwidthProber) GetCurrentRTT() time.Duration {
	bbr.mu.RLock()
	defer bbr.mu.RUnlock()
	
	return bbr.minRTT
}

func (bbr *BBRBandwidthProber) GetPacingRate() float64 {
	bbr.mu.RLock()
	defer bbr.mu.RUnlock()
	
	return bbr.pacingRate
}

func (bbr *BBRBandwidthProber) GetCongestionWindow() int64 {
	bbr.mu.RLock()
	defer bbr.mu.RUnlock()
	
	return bbr.congestionWindow
}

func (bbr *BBRBandwidthProber) GetMetrics() *BBRMetrics {
	bbr.mu.RLock()
	defer bbr.mu.RUnlock()
	
	// Create a copy of current metrics
	metrics := *bbr.metrics
	metrics.LastUpdate = time.Now()
	
	return &metrics
}

// Internal BBR algorithm implementation
func (bbr *BBRBandwidthProber) runBBRLoop() {
	ticker := time.NewTicker(time.Millisecond * 100) // 100ms interval
	defer ticker.Stop()
	
	for {
		select {
		case <-bbr.ctx.Done():
			return
		case <-ticker.C:
			if bbr.isActive {
				bbr.updateBBRState()
			}
		}
	}
}

func (bbr *BBRBandwidthProber) updateBBRState() {
	bbr.mu.Lock()
	defer bbr.mu.Unlock()
	
	oldState := bbr.state
	
	// State machine transitions
	switch bbr.state {
	case BBRStateStartup:
		bbr.updateStartupState()
	case BBRStateDrain:
		bbr.updateDrainState()
	case BBRStateProbeBW:
		bbr.updateProbeBWState()
	case BBRStateProbeRTT:
		bbr.updateProbeRTTState()
	}
	
	// Update pacing and congestion window
	bbr.updatePacingRate()
	bbr.updateCongestionWindow()
	
	// Record state transition if changed
	if oldState != bbr.state {
		bbr.recordStateTransition(oldState, bbr.state)
	}
	
	// Update metrics
	bbr.updateMetrics()
}

func (bbr *BBRBandwidthProber) updateStartupState() {
	// Check if we should exit startup
	if bbr.shouldExitStartup() {
		bbr.transitionToState(BBRStateDrain)
		return
	}
	
	// Continue aggressive probing in startup
	bbr.pacingGain = bbr.startupGain
	bbr.cwndGain = bbr.startupGain
}

func (bbr *BBRBandwidthProber) updateDrainState() {
	// Drain the bottleneck buffer
	bbr.pacingGain = bbr.drainGain
	bbr.cwndGain = bbr.startupGain // Keep high cwnd during drain
	
	// Exit drain when inflight drops to BDP
	if bbr.shouldExitDrain() {
		bbr.transitionToState(BBRStateProbeBW)
	}
}

func (bbr *BBRBandwidthProber) updateProbeBWState() {
	// Cycle through different probing gains
	bbr.updateProbeBWCycle()
	
	// Check for RTT probing
	if bbr.shouldProbeRTT() {
		bbr.transitionToState(BBRStateProbeRTT)
		return
	}
	
	// Set gains based on current cycle position
	if bbr.cycleIndex < len(bbr.probeBWCycleGains) {
		bbr.pacingGain = bbr.probeBWCycleGains[bbr.cycleIndex]
	} else {
		bbr.pacingGain = 1.0 // Default gain
	}
	bbr.cwndGain = bbr.config.CwndGain
}

func (bbr *BBRBandwidthProber) updateProbeRTTState() {
	// Reduce congestion window to probe for minimum RTT
	bbr.pacingGain = 1.0
	bbr.cwndGain = 0.5 // Reduce cwnd to drain queues
	
	// Exit RTT probing after duration
	if time.Since(bbr.stateStartTime) >= bbr.config.ProbeRTTDuration {
		if bbr.shouldReturnToStartup() {
			bbr.transitionToState(BBRStateStartup)
		} else {
			bbr.transitionToState(BBRStateProbeBW)
		}
	}
}

func (bbr *BBRBandwidthProber) shouldExitStartup() bool {
	// Exit startup when bandwidth growth slows
	if len(bbr.bandwidthSamples) < 3 {
		return false
	}
	
	recent := bbr.bandwidthSamples[len(bbr.bandwidthSamples)-3:]
	bandwidthGrowth := recent[2].DeliveryRate / recent[0].DeliveryRate
	
	return bandwidthGrowth < bbr.startupThreshold
}

func (bbr *BBRBandwidthProber) shouldExitDrain() bool {
	// Exit drain when estimated inflight <= BDP
	bdp := bbr.calculateBDP()
	estimatedInflight := bbr.estimateInflightBytes()
	
	return estimatedInflight <= bdp
}

func (bbr *BBRBandwidthProber) shouldProbeRTT() bool {
	// Probe RTT periodically
	return time.Since(bbr.rttProbeStartTime) >= bbr.config.ProbeRTTInterval
}

func (bbr *BBRBandwidthProber) shouldReturnToStartup() bool {
	// Return to startup if bandwidth seems underutilized
	currentUtilization := bbr.pacingRate / bbr.maxBandwidth
	return currentUtilization < 0.8 // Less than 80% utilization
}

func (bbr *BBRBandwidthProber) updateProbeBWCycle() {
	// Advance cycle based on RTT measurements
	if time.Since(bbr.cycleStartTime) >= bbr.minRTT {
		bbr.cycleIndex = (bbr.cycleIndex + 1) % len(bbr.probeBWCycleGains)
		bbr.cycleStartTime = time.Now()
	}
}

func (bbr *BBRBandwidthProber) transitionToState(newState BBRState) {
	oldState := bbr.state
	bbr.state = newState
	bbr.stateStartTime = time.Now()
	
	// State-specific initialization
	switch newState {
	case BBRStateProbeRTT:
		bbr.rttProbeStartTime = time.Now()
	case BBRStateProbeBW:
		bbr.cycleIndex = 0
		bbr.cycleStartTime = time.Now()
	}
	
	// Update metrics
	bbr.metrics.StateTransitions[newState]++
	
	// Record adaptation event
	bbr.recordAdaptationEvent(oldState, newState, "state_transition")
}

func (bbr *BBRBandwidthProber) updatePacingRate() {
	// Calculate target pacing rate
	targetRate := bbr.maxBandwidth * bbr.pacingGain
	
	// Apply pacing margin
	margin := 1.0 + (bbr.config.PacingMarginPercent / 100.0)
	targetRate *= margin
	
	// Clamp to configured limits
	if targetRate < bbr.config.MinPacingRate {
		targetRate = bbr.config.MinPacingRate
	}
	if targetRate > bbr.config.MaxPacingRate {
		targetRate = bbr.config.MaxPacingRate
	}
	
	bbr.pacingRate = targetRate
}

func (bbr *BBRBandwidthProber) updateCongestionWindow() {
	// Calculate target congestion window
	bdp := bbr.calculateBDP()
	targetCwnd := int64(float64(bdp) * bbr.cwndGain)
	
	// Apply minimum quantum
	if targetCwnd < bbr.sendQuantum {
		targetCwnd = bbr.sendQuantum
	}
	
	// Clamp to configured limits
	if targetCwnd < bbr.config.MinCongestionWindow {
		targetCwnd = bbr.config.MinCongestionWindow
	}
	if targetCwnd > bbr.config.MaxCongestionWindow {
		targetCwnd = bbr.config.MaxCongestionWindow
	}
	
	bbr.congestionWindow = targetCwnd
}

func (bbr *BBRBandwidthProber) calculateBDP() int64 {
	// BDP = Bandwidth * RTT
	bandwidthBps := bbr.maxBandwidth * 1024 * 1024 / 8 // Convert Mbps to Bps
	rttSeconds := bbr.minRTT.Seconds()
	
	return int64(bandwidthBps * rttSeconds)
}

func (bbr *BBRBandwidthProber) estimateInflightBytes() int64 {
	// Estimate bytes currently in flight
	// This would be maintained by the transport layer
	return bbr.congestionWindow / 2 // Simplified estimate
}

func (bbr *BBRBandwidthProber) calculateDeliveryRate(packetSize int64, sendTime, ackTime time.Time) float64 {
	// Calculate delivery rate based on ACK timing
	duration := ackTime.Sub(sendTime)
	if duration <= 0 {
		return 0.0
	}
	
	// Convert to Mbps
	bps := float64(packetSize*8) / duration.Seconds()
	mbps := bps / (1024 * 1024)
	
	return mbps
}

func (bbr *BBRBandwidthProber) updateBandwidthEstimate(sample BBRBandwidthSample) {
	// Update max bandwidth filter
	if !sample.IsAppLimited {
		bbr.bandwidthFilter.Update(sample.DeliveryRate, sample.Timestamp)
		newMax := bbr.bandwidthFilter.GetMax()
		
		if newMax > bbr.maxBandwidth {
			bbr.maxBandwidth = newMax
		}
	}
}

func (bbr *BBRBandwidthProber) updateRTTEstimate(rtt time.Duration, timestamp time.Time) {
	// Update min RTT filter
	bbr.rttFilter.Update(rtt, timestamp)
	newMin := bbr.rttFilter.GetMin()
	
	if newMin < bbr.minRTT {
		bbr.minRTT = newMin
	}
	
	// Create RTT sample
	rttSample := RTTSample{
		Timestamp:   timestamp,
		RTT:         rtt,
		MinRTT:      bbr.minRTT,
		SmoothedRTT: bbr.calculateSmoothedRTT(rtt),
		RTTVariance: bbr.calculateRTTVariance(rtt),
		IsValid:     true,
	}
	
	bbr.recordRTTSample(rttSample)
}

func (bbr *BBRBandwidthProber) calculateSmoothedRTT(currentRTT time.Duration) time.Duration {
	// EWMA smoothing
	alpha := 0.125 // Standard TCP alpha
	if len(bbr.rttSamples) == 0 {
		return currentRTT
	}
	
	lastSmoothed := bbr.rttSamples[len(bbr.rttSamples)-1].SmoothedRTT
	smoothed := time.Duration(float64(lastSmoothed)*(1-alpha) + float64(currentRTT)*alpha)
	
	return smoothed
}

func (bbr *BBRBandwidthProber) calculateRTTVariance(currentRTT time.Duration) time.Duration {
	// Calculate RTT variance for jitter estimation
	if len(bbr.rttSamples) == 0 {
		return 0
	}
	
	lastSmoothed := bbr.rttSamples[len(bbr.rttSamples)-1].SmoothedRTT
	diff := currentRTT - lastSmoothed
	if diff < 0 {
		diff = -diff
	}
	
	beta := 0.25 // Standard TCP beta
	lastVariance := bbr.rttSamples[len(bbr.rttSamples)-1].RTTVariance
	variance := time.Duration(float64(lastVariance)*(1-beta) + float64(diff)*beta)
	
	return variance
}

func (bbr *BBRBandwidthProber) isAppLimited() bool {
	return bbr.appLimited && bbr.deliveredBytes < bbr.appLimitedUntil
}

func (bbr *BBRBandwidthProber) handleLossEvent() {
	// Handle packet loss with BBR-appropriate response
	bbr.consecutiveLossCount = 0 // Reset counter
	
	// BBR is less aggressive than traditional loss-based algorithms
	// Mainly use loss for validation rather than primary congestion signal
	if bbr.state == BBRStateStartup {
		// In startup, loss might indicate we've found the bottleneck
		bbr.transitionToState(BBRStateDrain)
	}
	
	// Record loss event
	bbr.metrics.TotalLossEvents++
}

func (bbr *BBRBandwidthProber) recordBandwidthSample(sample BBRBandwidthSample) {
	bbr.bandwidthSamples = append(bbr.bandwidthSamples, sample)
	if len(bbr.bandwidthSamples) > 1000 {
		bbr.bandwidthSamples = bbr.bandwidthSamples[1:]
	}
}

func (bbr *BBRBandwidthProber) recordRTTSample(sample RTTSample) {
	bbr.rttSamples = append(bbr.rttSamples, sample)
	if len(bbr.rttSamples) > 1000 {
		bbr.rttSamples = bbr.rttSamples[1:]
	}
}

func (bbr *BBRBandwidthProber) recordStateTransition(oldState, newState BBRState) {
	event := BBRAdaptationEvent{
		Timestamp:   time.Now(),
		Trigger:     "state_machine",
		OldState:    oldState,
		NewState:    newState,
		Reason:      fmt.Sprintf("Transition from %s to %s", oldState, newState),
	}
	
	bbr.adaptationHistory = append(bbr.adaptationHistory, event)
	if len(bbr.adaptationHistory) > 1000 {
		bbr.adaptationHistory = bbr.adaptationHistory[1:]
	}
}

func (bbr *BBRBandwidthProber) recordAdaptationEvent(oldState, newState BBRState, trigger string) {
	event := BBRAdaptationEvent{
		Timestamp:             time.Now(),
		Trigger:               trigger,
		OldState:              oldState,
		NewState:              newState,
		BandwidthChange:       0.0, // Would be calculated
		RTTChange:             0,    // Would be calculated
		CongestionWindowChange: 0,   // Would be calculated
		PacingRateChange:      0.0, // Would be calculated
		Reason:                fmt.Sprintf("BBR adaptation: %s", trigger),
	}
	
	bbr.adaptationHistory = append(bbr.adaptationHistory, event)
	if len(bbr.adaptationHistory) > 1000 {
		bbr.adaptationHistory = bbr.adaptationHistory[1:]
	}
}

func (bbr *BBRBandwidthProber) updateMetrics() {
	bbr.metrics.LastUpdate = time.Now()
	bbr.metrics.MaxBandwidthObserved = bbr.maxBandwidth
	bbr.metrics.MinRTTObserved = bbr.minRTT
	bbr.metrics.TotalProbes++
	
	// Calculate averages
	if len(bbr.bandwidthSamples) > 0 {
		totalBandwidth := 0.0
		for _, sample := range bbr.bandwidthSamples {
			totalBandwidth += sample.DeliveryRate
		}
		bbr.metrics.AverageBandwidth = totalBandwidth / float64(len(bbr.bandwidthSamples))
	}
	
	if len(bbr.rttSamples) > 0 {
		totalRTT := time.Duration(0)
		for _, sample := range bbr.rttSamples {
			totalRTT += sample.RTT
		}
		bbr.metrics.AverageRTT = totalRTT / time.Duration(len(bbr.rttSamples))
	}
}

// Supporting component implementations
type BandwidthMaxFilter struct {
	windowLength    time.Duration
	samples         []timestampedValue
	maxValue        float64
	maxTimestamp    time.Time
	// TODO: Add mutex for thread safety
	// mu              sync.RWMutex
}

type timestampedValue struct {
	value     float64
	timestamp time.Time
}

func NewBandwidthMaxFilter(windowLength time.Duration) *BandwidthMaxFilter {
	return &BandwidthMaxFilter{
		windowLength: windowLength,
		samples:      make([]timestampedValue, 0, 100),
		maxValue:     0.0,
	}
}

func (f *BandwidthMaxFilter) Update(value float64, timestamp time.Time) {
	// Add new sample
	f.samples = append(f.samples, timestampedValue{value, timestamp})
	
	// Remove old samples outside window
	cutoff := timestamp.Add(-f.windowLength)
	for len(f.samples) > 0 && f.samples[0].timestamp.Before(cutoff) {
		f.samples = f.samples[1:]
	}
	
	// Update max
	f.maxValue = 0.0
	for _, sample := range f.samples {
		if sample.value > f.maxValue {
			f.maxValue = sample.value
			f.maxTimestamp = sample.timestamp
		}
	}
}

func (f *BandwidthMaxFilter) GetMax() float64 {
	return f.maxValue
}

type RTTMinFilter struct {
	windowLength    time.Duration
	samples         []timestampedDuration
	minValue        time.Duration
	minTimestamp    time.Time
	// TODO: Add mutex for thread safety
	// mu              sync.RWMutex
}

type timestampedDuration struct {
	value     time.Duration
	timestamp time.Time
}

func NewRTTMinFilter(windowLength time.Duration) *RTTMinFilter {
	return &RTTMinFilter{
		windowLength: windowLength,
		samples:      make([]timestampedDuration, 0, 100),
		minValue:     time.Hour, // Start with very high value
	}
}

func (f *RTTMinFilter) Update(value time.Duration, timestamp time.Time) {
	// Add new sample
	f.samples = append(f.samples, timestampedDuration{value, timestamp})
	
	// Remove old samples outside window
	cutoff := timestamp.Add(-f.windowLength)
	for len(f.samples) > 0 && f.samples[0].timestamp.Before(cutoff) {
		f.samples = f.samples[1:]
	}
	
	// Update min
	f.minValue = time.Hour
	for _, sample := range f.samples {
		if sample.value < f.minValue {
			f.minValue = sample.value
			f.minTimestamp = sample.timestamp
		}
	}
}

func (f *RTTMinFilter) GetMin() time.Duration {
	return f.minValue
}

// BBRDeliveryRateEstimator is reserved for future delivery rate estimation
// Currently using the built-in bandwidth tracking in BBRBandwidthProber

// BBRLossDetector is reserved for future loss detection integration
// Currently using the loss tracking in the main BBRBandwidthProber

func NewBBRMetrics() *BBRMetrics {
	return &BBRMetrics{
		StateTransitions: make(map[BBRState]int64),
		ModeTransitions:  make(map[BBRProbingMode]int64),
		LastUpdate:       time.Now(),
	}
}

func NewDefaultBBRConfig() *BBRConfig {
	return &BBRConfig{
		HighGain:              2.885, // ln(2 * BDP) / ln(BDP)
		DrainGain:             1.0 / 2.885,
		StartupThreshold:      1.25,
		ProbeRTTDuration:      time.Millisecond * 200,
		
		ProbeBWCycleLength:    8,
		ProbeBWGainCycle:      []float64{1.25, 0.75, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0},
		BandwidthWindowLength: time.Second * 10,
		
		RTTWindowLength:       time.Second * 10,
		ProbeRTTInterval:      time.Second * 10,
		MinRTTExpiry:          time.Second * 10,
		
		LossThreshold:         5.0,
		BetaLoss:              0.7,
		
		PacingMarginPercent:   1.0,
		MaxPacingRate:         1000.0, // 1 Gbps
		MinPacingRate:         1.0,    // 1 Mbps
		
		InitialCongestionWindow: 10 * 1024, // 10KB
		MinCongestionWindow:     4 * 1024,  // 4KB
		MaxCongestionWindow:     1024 * 1024 * 10, // 10MB
		CwndGain:               2.0,
	}
}