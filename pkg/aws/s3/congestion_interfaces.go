/*
Package s3 congestion interfaces provides segregated interfaces for congestion control.

This file implements Interface Segregation Principle by breaking down the large
GlobalCongestionController interface (58 methods) into smaller, focused interfaces.
*/
package s3

import (
	"context"
	"time"
)

// Core Congestion Control Operations
// CongestionControlCore provides essential congestion control operations
type CongestionControlCore interface {
	// Lifecycle management
	Start(ctx context.Context)
	
	// Resource allocation
	RegisterPrefix(prefixID string, capacity float64)
	AllocateResources(upload *ScheduledUpload) (*PrefixAllocation, error)
	UpdatePrefixPerformance(prefixID string, metrics *PrefixPerformanceMetrics)
}

// Metrics and Monitoring Interface
// CongestionMetricsProvider provides metrics and monitoring capabilities
type CongestionMetricsProvider interface {
	GetMetrics() *CongestionMetrics
	GetEnhancedMetrics() *EnhancedCongestionMetrics
}

// Algorithm Management Interface
// CongestionAlgorithmManager manages different congestion control algorithms
type CongestionAlgorithmManager interface {
	// Traditional algorithms
	applyCongestionControl(allocation *PrefixAllocation, metrics *PrefixPerformanceMetrics)
	applySlowStart(allocation *PrefixAllocation, metrics *PrefixPerformanceMetrics)
	applyCongestionAvoidance(allocation *PrefixAllocation, metrics *PrefixPerformanceMetrics)
	applyRecovery(allocation *PrefixAllocation, metrics *PrefixPerformanceMetrics)
	applyFastRecovery(allocation *PrefixAllocation, metrics *PrefixPerformanceMetrics)
	
	// BBR algorithms
	ApplyBBRCongestionControl(allocation *PrefixAllocation, metrics *PrefixPerformanceMetrics)
	applyBBRStartup(allocation *PrefixAllocation, metrics *PrefixPerformanceMetrics)
	applyBBRDrain(allocation *PrefixAllocation, metrics *PrefixPerformanceMetrics)
	applyBBRProbeBW(allocation *PrefixAllocation, metrics *PrefixPerformanceMetrics)
	applyBBRProbeRTT(allocation *PrefixAllocation, metrics *PrefixPerformanceMetrics)
}

// Event Detection and Handling Interface
// CongestionEventManager handles detection and response to congestion events
type CongestionEventManager interface {
	detectCongestionEvents(allocation *PrefixAllocation, metrics *PrefixPerformanceMetrics)
	handleCongestionDetected(allocation *PrefixAllocation)
	handleTimeoutCongestion(allocation *PrefixAllocation)
	handleBandwidthCongestion(allocation *PrefixAllocation)
	shouldUseFastRecovery(allocation *PrefixAllocation) bool
}

// Bandwidth Management Interface
// BandwidthManager handles bandwidth estimation and allocation
type BandwidthManager interface {
	updateGlobalBandwidthEstimate(metrics *PrefixPerformanceMetrics)
	redistributeBandwidthFairly(coordinator *CrossPrefixCongestionCoordinator)
	shouldRedistributeBandwidth(utilization, fairness float64) bool
	rebalanceAllocations()
}

// Adaptive Control Interface
// AdaptiveController handles parameter adaptation and learning
type AdaptiveController interface {
	updateAdaptiveParameters()
	updateGlobalCongestionWindow()
	updatePacingAndCongestionWindow(allocation *PrefixAllocation, metrics *PrefixPerformanceMetrics)
	calculateBackoffDelay(allocation *PrefixAllocation) time.Duration
	calculatePriorityMultiplier(priority int) float64
}

// Background Processing Interface
// BackgroundProcessor manages background control loops
type BackgroundProcessor interface {
	congestionControlLoop(ctx context.Context)
	bandwidthProbingLoop(ctx context.Context)
	adaptiveRecoveryLoop(ctx context.Context)
	performCongestionControlUpdates()
	performBandwidthProbing()
	performAdaptiveRecovery()
}

// Cross-Prefix Coordination Interface
// CrossPrefixCoordinator handles coordination across multiple prefixes
type CrossPrefixCoordinator interface {
	SetCommunicator(communicator *CrossPrefixCommunicator)
	CoordinatedRegisterPrefix(prefixID string, capacity float64) error
	runCrossPrefixCoordination(ctx context.Context, coordinator *CrossPrefixCongestionCoordinator)
	performCrossPrefixOptimization(coordinator *CrossPrefixCongestionCoordinator)
	
	// Message handling
	handleCongestionCoordinationMessage(coordinator *CrossPrefixCongestionCoordinator, msg *CongestionCoordinationMessage)
	handleBandwidthRequest(coordinator *CrossPrefixCongestionCoordinator, msg *CongestionCoordinationMessage)
	handleCongestionAlert(coordinator *CrossPrefixCongestionCoordinator, msg *CongestionCoordinationMessage)
	handleLoadUpdate(coordinator *CrossPrefixCongestionCoordinator, msg *CongestionCoordinationMessage)
	
	// Message sending
	sendCrossePrefixUpdates(coordinator *CrossPrefixCongestionCoordinator)
	sendBandwidthReallocationMessage(coordinator *CrossPrefixCongestionCoordinator, fromPrefix, toPrefix string, amount float64)
	sendBandwidthGrantMessage(prefixID string, amount float64)
	handleGlobalCongestionResponse(prefixID string, congestionLevel float64)
}

// Statistics and Analysis Interface
// StatisticsAnalyzer provides analytical capabilities for congestion control
type StatisticsAnalyzer interface {
	findLeastUtilizedPrefix() *PrefixAllocation
	calculateAverageUtilization() float64
	calculateTotalUtilization() float64
	calculateFairnessIndex() float64
	countCongestionEvents() int
	calculateOverheadPercent() float64
	calculateSystemStability() float64
	calculateRecentCongestionFrequency() float64
	getMaxDeliveryRate() float64
}

// Composite Interface for Full Functionality
// FullCongestionController combines all congestion control capabilities
// This is the interface that external components should depend on if they need all functionality
type FullCongestionController interface {
	CongestionControlCore
	CongestionMetricsProvider
	CongestionAlgorithmManager
	CongestionEventManager
	BandwidthManager
	AdaptiveController
	BackgroundProcessor
	CrossPrefixCoordinator
	StatisticsAnalyzer
}

// Simplified Interface for Basic Use Cases
// BasicCongestionController provides only essential functionality
// This should be used by components that only need basic congestion control
type BasicCongestionController interface {
	CongestionControlCore
	CongestionMetricsProvider
}

// Advanced Interface for Sophisticated Control
// AdvancedCongestionController provides sophisticated control capabilities
// This should be used by components that need advanced features
type AdvancedCongestionController interface {
	CongestionControlCore
	CongestionMetricsProvider
	CongestionAlgorithmManager
	CongestionEventManager
	BandwidthManager
	AdaptiveController
}