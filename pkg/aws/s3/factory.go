/*
Package s3 factory provides dependency injection and factory patterns for S3 components.

This file implements the Factory pattern to reduce coupling between components
and enable easier testing and configuration management.
*/
package s3

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/scttfrdmn/cargoship/pkg/aws/config"
)

// ComponentFactory creates S3 components with proper dependency injection
type ComponentFactory struct {
	// Shared dependencies
	congestionController CongestionController
	communicationService CommunicationService
}

// NewComponentFactory creates a new factory with shared dependencies
func NewComponentFactory() *ComponentFactory {
	return &ComponentFactory{}
}

// SetCongestionController configures the congestion controller dependency
func (f *ComponentFactory) SetCongestionController(controller CongestionController) *ComponentFactory {
	f.congestionController = controller
	return f
}

// SetCommunicationService configures the communication service dependency
func (f *ComponentFactory) SetCommunicationService(service CommunicationService) *ComponentFactory {
	f.communicationService = service
	return f
}

// CreateBasicTransporter creates a basic S3 transporter
func (f *ComponentFactory) CreateBasicTransporter(client *s3.Client, cfg config.S3Config) BasicTransporter {
	return NewTransporter(client, cfg)
}

// CreateCongestionController creates a congestion controller with default configuration
func (f *ComponentFactory) CreateCongestionController(ctx context.Context) CongestionController {
	config := DefaultCoordinationConfig()
	controller := NewGlobalCongestionController(config)
	
	// Start the controller with the provided context
	controller.Start(ctx)
	
	return controller
}

// CreateCommunicationService creates a communication service
func (f *ComponentFactory) CreateCommunicationService(ctx context.Context) CommunicationService {
	config := DefaultCommunicationConfig()
	service := NewCrossPrefixCommunicator(ctx, config)
	
	return service
}

// CreatePipelineCoordinator creates a pipeline coordinator with dependency injection
func (f *ComponentFactory) CreatePipelineCoordinator(ctx context.Context, congestionController CongestionController) *PipelineCoordinator {
	config := DefaultCoordinationConfig()
	return NewPipelineCoordinator(ctx, config, congestionController)
}

// Interface Segregation Examples:

// CreateBasicCongestionController creates a congestion controller with basic functionality
// This is suitable for simple use cases that only need core congestion control
func (f *ComponentFactory) CreateBasicCongestionController(ctx context.Context) BasicCongestionController {
	config := DefaultCoordinationConfig()
	controller := NewGlobalCongestionController(config)
	controller.Start(ctx)
	return controller
}

// CreateAdvancedCongestionController creates a congestion controller with advanced functionality
// This is suitable for sophisticated use cases requiring algorithm management and adaptation
func (f *ComponentFactory) CreateAdvancedCongestionController(ctx context.Context) AdvancedCongestionController {
	config := DefaultCoordinationConfig()
	controller := NewGlobalCongestionController(config)
	controller.Start(ctx)
	return controller
}

// CreateMetricsOnlyCongestionController creates a congestion controller for metrics collection
// This is suitable for monitoring and observability use cases
func (f *ComponentFactory) CreateMetricsOnlyCongestionController(ctx context.Context) CongestionMetricsProvider {
	config := DefaultCoordinationConfig()
	controller := NewGlobalCongestionController(config)
	controller.Start(ctx)
	return controller
}

// Dependencies provides access to shared dependencies
type Dependencies struct {
	CongestionController CongestionController
	CommunicationService CommunicationService
	ComponentFactory     *ComponentFactory
}

// NewDependencies creates a new dependency container
func NewDependencies(ctx context.Context) *Dependencies {
	factory := NewComponentFactory()
	
	// Create shared dependencies
	congestionController := factory.CreateCongestionController(ctx)
	communicationService := factory.CreateCommunicationService(ctx)
	
	// Configure the factory with the shared dependencies
	factory.SetCongestionController(congestionController).
		SetCommunicationService(communicationService)
	
	return &Dependencies{
		CongestionController: congestionController,
		CommunicationService: communicationService,
		ComponentFactory:     factory,
	}
}

// GetTransporter creates a new transporter using the configured dependencies
func (d *Dependencies) GetTransporter(client *s3.Client, cfg config.S3Config) BasicTransporter {
	return d.ComponentFactory.CreateBasicTransporter(client, cfg)
}

// IntegratedTransporter combines multiple S3 transport capabilities with dependency injection
type IntegratedTransporter struct {
	BasicTransporter
	congestionController CongestionController
	communicationService CommunicationService
}

// NewIntegratedTransporter creates a fully integrated transporter with all dependencies
func (f *ComponentFactory) NewIntegratedTransporter(client *s3.Client, cfg config.S3Config) *IntegratedTransporter {
	// Create the basic transporter
	basicTransporter := f.CreateBasicTransporter(client, cfg)
	
	return &IntegratedTransporter{
		BasicTransporter:     basicTransporter,
		congestionController: f.congestionController,
		communicationService: f.communicationService,
	}
}

// GetCongestionController returns the injected congestion controller
func (it *IntegratedTransporter) GetCongestionController() CongestionController {
	return it.congestionController
}

// GetCommunicationService returns the injected communication service
func (it *IntegratedTransporter) GetCommunicationService() CommunicationService {
	return it.communicationService
}

// RegisterForCoordination registers this transporter for cross-prefix coordination
func (it *IntegratedTransporter) RegisterForCoordination(prefixID string, capacity float64) error {
	// Register with congestion controller
	if it.congestionController != nil {
		it.congestionController.RegisterPrefix(prefixID, capacity)
	}
	
	// Register with communication service
	if it.communicationService != nil {
		return it.communicationService.RegisterPrefix(prefixID)
	}
	
	return nil
}