/*
Package s3 interfaces defines core abstractions for AWS S3 transport components.

This file establishes interface boundaries to reduce coupling between components
and enable dependency injection, testing, and modular architecture.
*/
package s3

import (
	"context"

	"github.com/scttfrdmn/cargoship/pkg/aws/config"
)

// Core Transport Interfaces

// BasicTransporter defines the basic interface for S3 data transport
type BasicTransporter interface {
	// Upload uploads an archive to S3
	Upload(ctx context.Context, archive Archive) (*UploadResult, error)

	// GetConfig returns the current transport configuration
	GetConfig() config.S3Config
}

// Congestion Control Interface

// CongestionController manages congestion control across S3 prefixes
// This is the main interface that components should depend on for basic congestion control
// For more specialized needs, see congestion_interfaces.go for segregated interfaces
type CongestionController interface {
	// Start begins congestion control with the given context
	Start(ctx context.Context)

	// RegisterPrefix registers a new S3 prefix for congestion control
	RegisterPrefix(prefixID string, capacity float64)

	// AllocateResources allocates bandwidth and congestion window for an upload
	AllocateResources(upload *ScheduledUpload) (*PrefixAllocation, error)

	// UpdatePrefixPerformance updates performance metrics for a prefix
	UpdatePrefixPerformance(prefixID string, metrics *PrefixPerformanceMetrics)

	// GetMetrics returns current congestion control metrics
	GetMetrics() *CongestionMetrics
}

// Communication Interface

// CommunicationService handles message passing between prefixes and coordination
type CommunicationService interface {
	// Start begins the communication service
	Start() error

	// Stop halts the communication service
	Stop() error

	// RegisterPrefix registers a prefix for communication
	RegisterPrefix(prefixID string) error

	// SendMessage sends a coordination message
	SendMessage(message *CoordinationMessage) error
}

// Coordination Interface

// UploadCoordinator manages scheduling and coordination of uploads
type UploadCoordinator interface {
	// ScheduleUpload schedules an upload for coordination
	ScheduleUpload(upload *ScheduledUpload) error

	// GetScheduledUploads returns currently scheduled uploads
	GetScheduledUploads() []*ScheduledUpload
}
