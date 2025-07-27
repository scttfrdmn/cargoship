/*
Package s3 CUBIC congestion control implements the CUBIC TCP congestion control algorithm.

This module provides sophisticated congestion control based on the CUBIC algorithm, featuring
cubic function-based window growth, TCP-friendly behavior, and optimized performance for
high bandwidth-delay product networks.
*/
package s3

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"
)

// CubicCongestionController implements the CUBIC TCP congestion control algorithm
type CubicCongestionController struct {
	// Core CUBIC state
	state                  CubicState
	phase                  CubicPhase
	stateStartTime         time.Time
	
	// Congestion window management
	congestionWindow       float64   // Current congestion window (packets)
	slowStartThreshold     float64   // Slow start threshold (packets)
	maxCongestionWindow    float64   // Maximum cwnd before last reduction
	lastMaxCwnd            float64   // Previous maximum cwnd
	
	// CUBIC function parameters
	cubicK                 float64   // Time to reach Wmax from current cwnd
	cubicOriginPoint       float64   // Time when cwnd reaches Wmax
	cubicC                 float64   // CUBIC scaling factor
	cubicBeta              float64   // Multiplicative decrease factor
	
	// TCP-friendly calculations
	tcpCwnd                float64   // TCP-friendly congestion window
	tcpFriendlyEnable      bool      // Enable TCP-friendly mode
	
	// Loss detection and recovery
	lossDetectionTime      time.Time // Time of last loss detection
	lossRecoveryPhase      bool      // In loss recovery phase
	fastRecoveryThreshold  int       // Fast recovery threshold
	duplicateAckCount      int       // Count of duplicate ACKs
	
	// RTT and timing
	minRTT                 time.Duration // Minimum observed RTT
	smoothedRTT            time.Duration // Smoothed RTT estimate
	rttVariance            time.Duration // RTT variance
	currentRTT             time.Duration // Current RTT measurement
	
	// Packet tracking
	packetsInFlight        int64     // Current packets in flight
	totalPacketsSent       int64     // Total packets sent
	totalPacketsAcked      int64     // Total packets acknowledged
	totalPacketsLost       int64     // Total packets lost
	
	// Hystart (Hybrid Slow Start) components
	hystartEnabled         bool      // Enable Hystart
	hystartAckDelta        time.Duration // ACK spacing threshold
	hystartDelayMin        time.Duration // Minimum delay increase
	hystartDelayThreshold  time.Duration // Delay increase threshold
	hystartAckTrain        []time.Time   // ACK arrival times
	hystartLastAck         time.Time     // Last ACK time
	
	// Configuration parameters
	config                 *CubicConfig
	
	// Performance tracking
	metrics                *CubicMetrics
	cwndHistory            []CubicCwndEvent
	lossHistory            []CubicLossEvent
	adaptationHistory      []CubicAdaptationEvent
	
	// Context and synchronization
	ctx                    context.Context
	cancel                 context.CancelFunc
	isActive               bool
	mu                     sync.RWMutex
	eventMu                sync.Mutex
}

// CubicState represents the current state of CUBIC
type CubicState string

const (
	CubicStateSlowStart     CubicState = "slow_start"
	CubicStateCongestionAvoidance CubicState = "congestion_avoidance" 
	CubicStateFastRecovery  CubicState = "fast_recovery"
	CubicStateFastRetransmit CubicState = "fast_retransmit"
)

// CubicPhase represents the current growth phase
type CubicPhase string

const (
	CubicPhaseConcave      CubicPhase = "concave"    // Below Wmax
	CubicPhaseConvex       CubicPhase = "convex"     // Above Wmax
	CubicPhaseTCPFriendly  CubicPhase = "tcp_friendly" // TCP-friendly mode
)

// CubicConfig contains CUBIC algorithm configuration
type CubicConfig struct {
	// Core CUBIC parameters
	C                      float64   // Scaling factor (default: 0.4)
	Beta                   float64   // Multiplicative decrease (default: 0.7)
	
	// Window parameters
	InitialCwnd            float64   // Initial congestion window (packets)
	MinCwnd                float64   // Minimum congestion window (packets)
	MaxCwnd                float64   // Maximum congestion window (packets)
	InitialSSThresh        float64   // Initial slow start threshold
	
	// TCP-friendly parameters
	TCPFriendlyEnable      bool      // Enable TCP-friendly behavior
	TCPFriendlyAlpha       float64   // TCP-friendly alpha parameter
	TCPFriendlyBeta        float64   // TCP-friendly beta parameter
	
	// Hystart parameters
	HystartEnable          bool      // Enable Hybrid Slow Start
	HystartAckDelta        time.Duration // ACK spacing threshold
	HystartDelayMin        time.Duration // Minimum delay increase
	HystartDelayThreshold  time.Duration // Delay increase threshold
	HystartAckTrainLength  int       // Length of ACK train to track
	
	// Loss detection parameters
	DuplicateAckThreshold  int       // Threshold for fast retransmit
	FastRecoveryEnable     bool      // Enable fast recovery
	
	// RTT parameters
	RTTSmoothingFactor     float64   // RTT smoothing factor (alpha)
	RTTVarianceFactor      float64   // RTT variance factor (beta)
	MinRTT                 time.Duration // Minimum RTT value
	MaxRTT                 time.Duration // Maximum RTT value
	
	// Performance tuning
	PacketSize             int64     // Average packet size (bytes)
	MaxBurstSize           float64   // Maximum burst size (packets)
	CongestionThreshold    float64   // Congestion detection threshold
}

// CubicMetrics tracks CUBIC performance metrics
type CubicMetrics struct {
	// State tracking
	StateTransitions       map[CubicState]int64
	PhaseTransitions       map[CubicPhase]int64
	TimeInSlowStart        time.Duration
	TimeInCongestionAvoidance time.Duration
	
	// Window metrics
	AverageWindow          float64
	MaxWindowReached       float64
	WindowUtilization      float64
	WindowGrowthRate       float64
	
	// Loss metrics
	TotalLossEvents        int64
	LossRate               float64
	RecoveryTime           time.Duration
	FastRecoveries         int64
	TimeoutRecoveries      int64
	
	// RTT metrics  
	AverageRTT             time.Duration
	RTTVariation           time.Duration
	MinRTTObserved         time.Duration
	MaxRTTObserved         time.Duration
	
	// Throughput metrics
	AverageThroughput      float64
	PeakThroughput         float64
	ThroughputEfficiency   float64
	
	// CUBIC-specific metrics
	CubicK                 float64
	TCPFriendliness        float64
	HystartExits           int64
	CubicVsTCPRatio        float64
	
	// Performance metrics
	Fairness               float64
	Responsiveness         float64
	ConvergenceTime        time.Duration
	
	// Timing
	LastUpdate             time.Time
	MeasurementDuration    time.Duration
}

// CubicCwndEvent represents a congestion window change event
type CubicCwndEvent struct {
	Timestamp              time.Time
	OldCwnd                float64
	NewCwnd                float64
	Trigger                string
	State                  CubicState
	Phase                  CubicPhase
	RTT                    time.Duration
	PacketsInFlight        int64
}

// CubicLossEvent represents a packet loss event
type CubicLossEvent struct {
	Timestamp              time.Time
	LossType               CubicLossType
	PacketsLost            int64
	CwndBeforeLoss         float64
	CwndAfterLoss          float64
	RTT                    time.Duration
	RecoveryMethod         string
}

// CubicLossType defines different types of loss detection
type CubicLossType string

const (
	CubicLossTimeout       CubicLossType = "timeout"
	CubicLossFastRetransmit CubicLossType = "fast_retransmit"
	CubicLossECN           CubicLossType = "ecn"
)

// CubicAdaptationEvent represents an algorithm adaptation
type CubicAdaptationEvent struct {
	Timestamp              time.Time
	AdaptationType         string
	OldParameters          map[string]float64
	NewParameters          map[string]float64
	Trigger                string
	PerformanceImprovement float64
}

// Constructor
func NewCubicCongestionController(ctx context.Context, config *CubicConfig) *CubicCongestionController {
	if config == nil {
		config = NewDefaultCubicConfig()
	}
	
	cubicCtx, cancel := context.WithCancel(ctx)
	
	cubic := &CubicCongestionController{
		state:                CubicStateSlowStart,
		phase:                CubicPhaseConcave,
		stateStartTime:       time.Now(),
		
		congestionWindow:     config.InitialCwnd,
		slowStartThreshold:   config.InitialSSThresh,
		maxCongestionWindow:  config.InitialCwnd,
		lastMaxCwnd:          config.InitialCwnd,
		
		cubicK:               0.0,
		cubicOriginPoint:     0.0,
		cubicC:               config.C,
		cubicBeta:            config.Beta,
		
		tcpCwnd:              config.InitialCwnd,
		tcpFriendlyEnable:    config.TCPFriendlyEnable,
		
		lossDetectionTime:    time.Time{},
		lossRecoveryPhase:    false,
		fastRecoveryThreshold: config.DuplicateAckThreshold,
		duplicateAckCount:    0,
		
		minRTT:               config.MinRTT,
		smoothedRTT:          time.Millisecond * 100, // Initial estimate
		rttVariance:          time.Millisecond * 50,  // Initial variance
		currentRTT:           time.Millisecond * 100,
		
		packetsInFlight:      0,
		totalPacketsSent:     0,
		totalPacketsAcked:    0,
		totalPacketsLost:     0,
		
		hystartEnabled:       config.HystartEnable,
		hystartAckDelta:      config.HystartAckDelta,
		hystartDelayMin:      config.HystartDelayMin,
		hystartDelayThreshold: config.HystartDelayThreshold,
		hystartAckTrain:      make([]time.Time, 0, config.HystartAckTrainLength),
		
		config:               config,
		metrics:              NewCubicMetrics(),
		cwndHistory:          make([]CubicCwndEvent, 0, 1000),
		lossHistory:          make([]CubicLossEvent, 0, 1000),
		adaptationHistory:    make([]CubicAdaptationEvent, 0, 1000),
		
		ctx:                  cubicCtx,
		cancel:               cancel,
		isActive:             false,
	}
	
	// Initialize CUBIC parameters
	cubic.initializeCubicParameters()
	
	return cubic
}

// Core CUBIC algorithm methods
func (cc *CubicCongestionController) StartController() error {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	
	if cc.isActive {
		return fmt.Errorf("CUBIC controller already active")
	}
	
	cc.isActive = true
	cc.stateStartTime = time.Now()
	
	// Start the main control loop
	go cc.runControlLoop()
	
	return nil
}

func (cc *CubicCongestionController) StopController() error {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	
	if !cc.isActive {
		return fmt.Errorf("CUBIC controller not active")
	}
	
	cc.isActive = false
	cc.cancel()
	
	return nil
}

func (cc *CubicCongestionController) OnPacketSent(packetSize int64, sendTime time.Time) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	
	cc.packetsInFlight++
	cc.totalPacketsSent++
}

func (cc *CubicCongestionController) OnPacketAcknowledged(packetSize int64, sendTime, ackTime time.Time, rtt time.Duration) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	
	cc.packetsInFlight--
	cc.totalPacketsAcked++
	
	// Update RTT estimates
	cc.updateRTTEstimates(rtt, ackTime)
	
	// Process ACK based on current state
	switch cc.state {
	case CubicStateSlowStart:
		cc.processSlowStartAck(ackTime)
	case CubicStateCongestionAvoidance:
		cc.processCongestionAvoidanceAck(ackTime)
	case CubicStateFastRecovery:
		cc.processFastRecoveryAck(ackTime)
	}
	
	// Update Hystart if enabled
	if cc.hystartEnabled && cc.state == CubicStateSlowStart {
		cc.updateHystart(ackTime, rtt)
	}
	
	// Reset duplicate ACK count on new ACK
	cc.duplicateAckCount = 0
}

func (cc *CubicCongestionController) OnPacketLost(packetSize int64, sendTime time.Time, lossType CubicLossType) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	
	cc.packetsInFlight--
	cc.totalPacketsLost++
	
	// Handle loss based on type and current state
	cc.handlePacketLoss(lossType, sendTime)
	
	// Record loss event
	cc.recordLossEvent(lossType, 1, sendTime)
}

func (cc *CubicCongestionController) OnDuplicateAck(ackTime time.Time) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	
	cc.duplicateAckCount++
	
	// Check for fast retransmit threshold
	if cc.duplicateAckCount >= cc.fastRecoveryThreshold {
		cc.enterFastRetransmit(ackTime)
	}
}

func (cc *CubicCongestionController) GetCongestionWindow() float64 {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	
	return cc.congestionWindow
}

func (cc *CubicCongestionController) GetSlowStartThreshold() float64 {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	
	return cc.slowStartThreshold
}

func (cc *CubicCongestionController) GetCurrentState() CubicState {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	
	return cc.state
}

func (cc *CubicCongestionController) GetMetrics() *CubicMetrics {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	
	// Create a copy with current values
	metrics := *cc.metrics
	metrics.LastUpdate = time.Now()
	metrics.AverageWindow = cc.congestionWindow
	metrics.MaxWindowReached = cc.maxCongestionWindow
	
	return &metrics
}

// Internal CUBIC algorithm implementation
func (cc *CubicCongestionController) runControlLoop() {
	ticker := time.NewTicker(time.Millisecond * 50) // 50ms interval
	defer ticker.Stop()
	
	for {
		select {
		case <-cc.ctx.Done():
			return
		case <-ticker.C:
			if cc.isActive {
				cc.updateCubicState()
			}
		}
	}
}

func (cc *CubicCongestionController) updateCubicState() {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	
	// Update metrics
	cc.updateMetrics()
	
	// Check for state transitions
	cc.checkStateTransitions()
}

func (cc *CubicCongestionController) processSlowStartAck(ackTime time.Time) {
	oldCwnd := cc.congestionWindow
	
	// Standard slow start: cwnd += 1 per ACK
	cc.congestionWindow++
	
	// Check if we should exit slow start
	if cc.congestionWindow >= cc.slowStartThreshold {
		cc.transitionToState(CubicStateCongestionAvoidance, ackTime)
	}
	
	// Record window change
	cc.recordCwndEvent(oldCwnd, cc.congestionWindow, "slow_start_ack", ackTime)
}

func (cc *CubicCongestionController) processCongestionAvoidanceAck(ackTime time.Time) {
	oldCwnd := cc.congestionWindow
	
	// CUBIC congestion avoidance
	newCwnd := cc.calculateCubicWindow(ackTime)
	
	// TCP-friendly check if enabled
	if cc.tcpFriendlyEnable {
		tcpCwnd := cc.calculateTCPFriendlyWindow(ackTime)
		if tcpCwnd > newCwnd {
			newCwnd = tcpCwnd
			cc.phase = CubicPhaseTCPFriendly
		} else {
			cc.updateCubicPhase(newCwnd)
		}
	} else {
		cc.updateCubicPhase(newCwnd)
	}
	
	cc.congestionWindow = newCwnd
	
	// Record window change
	cc.recordCwndEvent(oldCwnd, cc.congestionWindow, "cubic_ack", ackTime)
}

func (cc *CubicCongestionController) processFastRecoveryAck(ackTime time.Time) {
	// Inflate window for each ACK in fast recovery
	cc.congestionWindow++
	
	// Check for recovery completion
	if cc.packetsInFlight <= int64(cc.slowStartThreshold) {
		cc.exitFastRecovery(ackTime)
	}
}

func (cc *CubicCongestionController) calculateCubicWindow(currentTime time.Time) float64 {
	// Time since last congestion event
	t := currentTime.Sub(cc.lossDetectionTime).Seconds()
	
	// CUBIC function: W(t) = C*(t-K)^3 + Wmax
	// Where K = cube_root(Wmax * beta / C)
	
	// Calculate cubic term using manual expansion to avoid staticcheck warning
	diff := t - cc.cubicK
	cubicTerm := cc.cubicC * diff * diff * diff
	targetCwnd := cubicTerm + cc.maxCongestionWindow
	
	// Ensure minimum window
	if targetCwnd < cc.config.MinCwnd {
		targetCwnd = cc.config.MinCwnd
	}
	
	// Ensure maximum window
	if targetCwnd > cc.config.MaxCwnd {
		targetCwnd = cc.config.MaxCwnd
	}
	
	return targetCwnd
}

func (cc *CubicCongestionController) calculateTCPFriendlyWindow(currentTime time.Time) float64 {
	// TCP-friendly window calculation
	// W_tcp = Wmax * beta + 3 * (1-beta) / (1+beta) * t / RTT
	
	if cc.smoothedRTT <= 0 {
		return cc.congestionWindow
	}
	
	t := currentTime.Sub(cc.lossDetectionTime).Seconds()
	rttSeconds := cc.smoothedRTT.Seconds()
	
	beta := cc.cubicBeta
	tcpTerm := 3.0 * (1.0 - beta) / (1.0 + beta) * t / rttSeconds
	tcpCwnd := cc.maxCongestionWindow*beta + tcpTerm
	
	return math.Max(tcpCwnd, cc.config.MinCwnd)
}

func (cc *CubicCongestionController) updateCubicPhase(newCwnd float64) {
	if newCwnd < cc.maxCongestionWindow {
		cc.phase = CubicPhaseConcave
	} else {
		cc.phase = CubicPhaseConvex
	}
}

func (cc *CubicCongestionController) handlePacketLoss(lossType CubicLossType, lossTime time.Time) {
	oldCwnd := cc.congestionWindow
	
	switch lossType {
	case CubicLossTimeout:
		cc.handleTimeoutLoss(lossTime)
	case CubicLossFastRetransmit:
		cc.handleFastRetransmitLoss(lossTime)
	case CubicLossECN:
		cc.handleECNLoss(lossTime)
	}
	
	// Record loss adaptation
	cc.recordCwndEvent(oldCwnd, cc.congestionWindow, string(lossType), lossTime)
}

func (cc *CubicCongestionController) handleTimeoutLoss(lossTime time.Time) {
	// Timeout: reset to slow start
	cc.maxCongestionWindow = cc.congestionWindow
	cc.slowStartThreshold = math.Max(cc.congestionWindow*cc.cubicBeta, cc.config.MinCwnd)
	cc.congestionWindow = cc.config.InitialCwnd
	
	cc.transitionToState(CubicStateSlowStart, lossTime)
	cc.updateCubicParameters(lossTime)
}

func (cc *CubicCongestionController) handleFastRetransmitLoss(lossTime time.Time) {
	// Fast retransmit: multiplicative decrease
	cc.maxCongestionWindow = cc.congestionWindow
	cc.congestionWindow *= cc.cubicBeta
	cc.slowStartThreshold = cc.congestionWindow
	
	if cc.config.FastRecoveryEnable {
		cc.transitionToState(CubicStateFastRecovery, lossTime)
	} else {
		cc.transitionToState(CubicStateCongestionAvoidance, lossTime)
	}
	
	cc.updateCubicParameters(lossTime)
}

func (cc *CubicCongestionController) handleECNLoss(lossTime time.Time) {
	// ECN: similar to fast retransmit but less aggressive
	cc.maxCongestionWindow = cc.congestionWindow
	cc.congestionWindow *= (cc.cubicBeta + 0.1) // Slightly less aggressive
	cc.slowStartThreshold = cc.congestionWindow
	
	cc.transitionToState(CubicStateCongestionAvoidance, lossTime)
	cc.updateCubicParameters(lossTime)
}

func (cc *CubicCongestionController) enterFastRetransmit(ackTime time.Time) {
	if cc.state != CubicStateFastRecovery {
		cc.handlePacketLoss(CubicLossFastRetransmit, ackTime)
		cc.duplicateAckCount = 0
	}
}

func (cc *CubicCongestionController) exitFastRecovery(ackTime time.Time) {
	cc.congestionWindow = cc.slowStartThreshold
	cc.transitionToState(CubicStateCongestionAvoidance, ackTime)
	cc.lossRecoveryPhase = false
}

func (cc *CubicCongestionController) updateHystart(ackTime time.Time, rtt time.Duration) {
	// Add ACK to train
	cc.hystartAckTrain = append(cc.hystartAckTrain, ackTime)
	if len(cc.hystartAckTrain) > cc.config.HystartAckTrainLength {
		cc.hystartAckTrain = cc.hystartAckTrain[1:]
	}
	
	// Check ACK spacing criterion
	if len(cc.hystartAckTrain) >= 2 {
		lastTwo := cc.hystartAckTrain[len(cc.hystartAckTrain)-2:]
		ackSpacing := lastTwo[1].Sub(lastTwo[0])
		
		if ackSpacing > cc.hystartAckDelta {
			cc.exitSlowStartViaHystart(ackTime, "ack_spacing")
			return
		}
	}
	
	// Check delay increase criterion
	if rtt > cc.minRTT+cc.hystartDelayThreshold {
		cc.exitSlowStartViaHystart(ackTime, "delay_increase")
	}
}

func (cc *CubicCongestionController) exitSlowStartViaHystart(exitTime time.Time, reason string) {
	cc.slowStartThreshold = cc.congestionWindow
	cc.transitionToState(CubicStateCongestionAvoidance, exitTime)
	cc.metrics.HystartExits++
	
	// Record adaptation event
	cc.recordAdaptationEvent("hystart_exit", map[string]float64{
		"cwnd": cc.congestionWindow,
		"ssthresh": cc.slowStartThreshold,
	}, reason, exitTime)
}

func (cc *CubicCongestionController) transitionToState(newState CubicState, transitionTime time.Time) {
	// oldState := cc.state // TODO: Use for logging/metrics if needed
	cc.state = newState
	cc.stateStartTime = transitionTime
	
	// Update metrics
	cc.metrics.StateTransitions[newState]++
	
	// State-specific initialization
	switch newState {
	case CubicStateFastRecovery:
		cc.lossRecoveryPhase = true
		cc.congestionWindow += float64(cc.fastRecoveryThreshold) // Inflate window
	case CubicStateCongestionAvoidance:
		cc.lossRecoveryPhase = false
	}
}

func (cc *CubicCongestionController) checkStateTransitions() {
	// Check for timeout-based transitions
	if cc.shouldTransitionFromTimeout() {
		cc.handleTimeoutLoss(time.Now())
	}
}

func (cc *CubicCongestionController) shouldTransitionFromTimeout() bool {
	// Simple timeout detection based on lack of ACKs
	if cc.packetsInFlight > 0 && time.Since(cc.hystartLastAck) > cc.smoothedRTT*4 {
		return true
	}
	return false
}

func (cc *CubicCongestionController) initializeCubicParameters() {
	// Calculate initial K value
	cc.updateCubicParameters(time.Now())
}

func (cc *CubicCongestionController) updateCubicParameters(currentTime time.Time) {
	// Update K: time to reach Wmax from current cwnd
	if cc.cubicC > 0 {
		cc.cubicK = math.Cbrt((cc.maxCongestionWindow * (1.0 - cc.cubicBeta)) / cc.cubicC)
	}
	
	// Update origin point
	cc.cubicOriginPoint = currentTime.Add(time.Duration(cc.cubicK * float64(time.Second))).Sub(currentTime).Seconds()
	
	// Update loss detection time
	cc.lossDetectionTime = currentTime
}

func (cc *CubicCongestionController) updateRTTEstimates(rtt time.Duration, measurementTime time.Time) {
	cc.currentRTT = rtt
	
	// Update minimum RTT
	if rtt < cc.minRTT || cc.minRTT == 0 {
		cc.minRTT = rtt
	}
	
	// Update smoothed RTT (EWMA)
	alpha := cc.config.RTTSmoothingFactor
	if cc.smoothedRTT == 0 {
		cc.smoothedRTT = rtt
	} else {
		cc.smoothedRTT = time.Duration(float64(cc.smoothedRTT)*(1-alpha) + float64(rtt)*alpha)
	}
	
	// Update RTT variance
	beta := cc.config.RTTVarianceFactor
	rttDiff := rtt - cc.smoothedRTT
	if rttDiff < 0 {
		rttDiff = -rttDiff
	}
	
	if cc.rttVariance == 0 {
		cc.rttVariance = rttDiff
	} else {
		cc.rttVariance = time.Duration(float64(cc.rttVariance)*(1-beta) + float64(rttDiff)*beta)
	}
	
	// Update last ACK time for timeout detection
	cc.hystartLastAck = measurementTime
}

func (cc *CubicCongestionController) recordCwndEvent(oldCwnd, newCwnd float64, trigger string, eventTime time.Time) {
	event := CubicCwndEvent{
		Timestamp:       eventTime,
		OldCwnd:         oldCwnd,
		NewCwnd:         newCwnd,
		Trigger:         trigger,
		State:           cc.state,
		Phase:           cc.phase,
		RTT:             cc.currentRTT,
		PacketsInFlight: cc.packetsInFlight,
	}
	
	cc.eventMu.Lock()
	cc.cwndHistory = append(cc.cwndHistory, event)
	if len(cc.cwndHistory) > 1000 {
		cc.cwndHistory = cc.cwndHistory[1:]
	}
	cc.eventMu.Unlock()
}

func (cc *CubicCongestionController) recordLossEvent(lossType CubicLossType, packetsLost int64, lossTime time.Time) {
	event := CubicLossEvent{
		Timestamp:        lossTime,
		LossType:         lossType,
		PacketsLost:      packetsLost,
		CwndBeforeLoss:   cc.maxCongestionWindow,
		CwndAfterLoss:    cc.congestionWindow,
		RTT:              cc.currentRTT,
		RecoveryMethod:   string(cc.state),
	}
	
	cc.eventMu.Lock()
	cc.lossHistory = append(cc.lossHistory, event)
	if len(cc.lossHistory) > 1000 {
		cc.lossHistory = cc.lossHistory[1:]
	}
	cc.eventMu.Unlock()
	
	// Update metrics
	cc.metrics.TotalLossEvents++
}

func (cc *CubicCongestionController) recordAdaptationEvent(adaptationType string, parameters map[string]float64, trigger string, eventTime time.Time) {
	event := CubicAdaptationEvent{
		Timestamp:              eventTime,
		AdaptationType:         adaptationType,
		NewParameters:          parameters,
		Trigger:                trigger,
		PerformanceImprovement: 0.0, // Would be calculated
	}
	
	cc.eventMu.Lock()
	cc.adaptationHistory = append(cc.adaptationHistory, event)
	if len(cc.adaptationHistory) > 1000 {
		cc.adaptationHistory = cc.adaptationHistory[1:]
	}
	cc.eventMu.Unlock()
}

func (cc *CubicCongestionController) updateMetrics() {
	cc.metrics.LastUpdate = time.Now()
	
	// Update window metrics
	cc.metrics.AverageWindow = cc.congestionWindow
	if cc.congestionWindow > cc.metrics.MaxWindowReached {
		cc.metrics.MaxWindowReached = cc.congestionWindow
	}
	
	// Update RTT metrics
	cc.metrics.AverageRTT = cc.smoothedRTT
	cc.metrics.RTTVariation = cc.rttVariance
	if cc.minRTT < cc.metrics.MinRTTObserved || cc.metrics.MinRTTObserved == 0 {
		cc.metrics.MinRTTObserved = cc.minRTT
	}
	if cc.currentRTT > cc.metrics.MaxRTTObserved {
		cc.metrics.MaxRTTObserved = cc.currentRTT
	}
	
	// Calculate loss rate
	if cc.totalPacketsSent > 0 {
		cc.metrics.LossRate = float64(cc.totalPacketsLost) / float64(cc.totalPacketsSent)
	}
	
	// Update CUBIC-specific metrics
	cc.metrics.CubicK = cc.cubicK
	
	// Calculate TCP-friendliness ratio
	if cc.tcpCwnd > 0 {
		cc.metrics.CubicVsTCPRatio = cc.congestionWindow / cc.tcpCwnd
	}
}

// Configuration and helper functions
func NewCubicMetrics() *CubicMetrics {
	return &CubicMetrics{
		StateTransitions: make(map[CubicState]int64),
		PhaseTransitions: make(map[CubicPhase]int64),
		LastUpdate:       time.Now(),
	}
}

func NewDefaultCubicConfig() *CubicConfig {
	return &CubicConfig{
		// Core CUBIC parameters
		C:                     0.4,  // Standard CUBIC scaling factor
		Beta:                  0.7,  // 30% reduction on loss (less than TCP's 50%)
		
		// Window parameters
		InitialCwnd:           10.0, // 10 packets initial window
		MinCwnd:               2.0,  // Minimum 2 packets
		MaxCwnd:               1000.0, // Maximum 1000 packets
		InitialSSThresh:       65535.0, // Large initial threshold
		
		// TCP-friendly parameters
		TCPFriendlyEnable:     true,
		TCPFriendlyAlpha:      0.125,
		TCPFriendlyBeta:       0.25,
		
		// Hystart parameters
		HystartEnable:         true,
		HystartAckDelta:       time.Millisecond * 2,  // 2ms ACK spacing threshold
		HystartDelayMin:       time.Millisecond * 4,  // 4ms minimum delay increase
		HystartDelayThreshold: time.Millisecond * 8,  // 8ms delay increase threshold
		HystartAckTrainLength: 8,
		
		// Loss detection parameters
		DuplicateAckThreshold: 3, // Standard fast retransmit threshold
		FastRecoveryEnable:    true,
		
		// RTT parameters
		RTTSmoothingFactor:    0.125, // Standard TCP alpha
		RTTVarianceFactor:     0.25,  // Standard TCP beta
		MinRTT:                time.Millisecond,      // 1ms minimum
		MaxRTT:                time.Second * 10,     // 10s maximum
		
		// Performance tuning
		PacketSize:            1500,  // Standard MTU
		MaxBurstSize:          10.0,  // 10 packets max burst
		CongestionThreshold:   0.8,   // 80% threshold
	}
}