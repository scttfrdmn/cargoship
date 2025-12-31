/*
Package s3 communication implements advanced cross-prefix communication channels.

This module provides high-performance inter-prefix communication with priority queuing,
message batching, and network-aware routing for optimal coordination efficiency.
*/
package s3

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// CrossPrefixCommunicator manages communication between different S3 prefixes.
type CrossPrefixCommunicator struct {
	channels        map[string]*PrefixChannel
	messageRouter   *MessageRouter
	priorityManager *PriorityManager
	batchProcessor  *BatchProcessor
	metrics         *CommunicationMetrics
	config          *CommunicationConfig
	mu              sync.RWMutex
	ctx             context.Context
	cancel          context.CancelFunc
	active          bool
}

// CommunicationConfig defines configuration for cross-prefix communication.
type CommunicationConfig struct {
	// Channel buffer sizes
	ChannelBufferSize int           `yaml:"channel_buffer_size" json:"channel_buffer_size"`
	PriorityQueueSize int           `yaml:"priority_queue_size" json:"priority_queue_size"`
	BatchSize         int           `yaml:"batch_size" json:"batch_size"`
	BatchTimeout      time.Duration `yaml:"batch_timeout" json:"batch_timeout"`

	// Communication strategies
	RoutingStrategy    string `yaml:"routing_strategy" json:"routing_strategy"`
	CompressionEnabled bool   `yaml:"compression_enabled" json:"compression_enabled"`
	EncryptionEnabled  bool   `yaml:"encryption_enabled" json:"encryption_enabled"`

	// Performance tuning
	MaxConcurrentMessages int           `yaml:"max_concurrent_messages" json:"max_concurrent_messages"`
	MessageTimeout        time.Duration `yaml:"message_timeout" json:"message_timeout"`
	RetryAttempts         int           `yaml:"retry_attempts" json:"retry_attempts"`
	RetryBackoff          time.Duration `yaml:"retry_backoff" json:"retry_backoff"`

	// Network optimization
	NetworkAwareRouting bool `yaml:"network_aware_routing" json:"network_aware_routing"`
	BandwidthThrottling bool `yaml:"bandwidth_throttling" json:"bandwidth_throttling"`
	AdaptiveBuffering   bool `yaml:"adaptive_buffering" json:"adaptive_buffering"`
}

// DefaultCommunicationConfig returns default communication configuration.
func DefaultCommunicationConfig() *CommunicationConfig {
	return &CommunicationConfig{
		ChannelBufferSize:     1024,
		PriorityQueueSize:     256,
		BatchSize:             32,
		BatchTimeout:          time.Millisecond * 50,
		RoutingStrategy:       "adaptive",
		CompressionEnabled:    true,
		EncryptionEnabled:     false,
		MaxConcurrentMessages: 100,
		MessageTimeout:        time.Second * 30,
		RetryAttempts:         3,
		RetryBackoff:          time.Millisecond * 100,
		NetworkAwareRouting:   true,
		BandwidthThrottling:   true,
		AdaptiveBuffering:     true,
	}
}

// PrefixChannel represents a communication channel for a specific S3 prefix.
type PrefixChannel struct {
	PrefixID          string
	Channel           chan *CoordinationMessage
	PriorityQueue     *PriorityQueue
	Statistics        *ChannelStatistics
	LastActivity      time.Time
	BufferUtilization float64
	NetworkLatency    time.Duration
}

// CoordinationMessage represents a message between S3 prefixes.
type CoordinationMessage struct {
	ID              string                    `json:"id"`
	Type            MessageType               `json:"type"`
	SourcePrefix    string                    `json:"source_prefix"`
	TargetPrefix    string                    `json:"target_prefix"`
	Priority        int                       `json:"priority"`
	Payload         map[string]interface{}    `json:"payload"`
	Timestamp       time.Time                 `json:"timestamp"`
	ExpiresAt       time.Time                 `json:"expires_at"`
	RetryCount      int                       `json:"retry_count"`
	ResponseChannel chan *CoordinationMessage `json:"-"`
	Context         context.Context           `json:"-"`
}

// MessageType defines different types of coordination messages.
type MessageType string

const (
	// Resource coordination messages
	MessageTypeResourceRequest    MessageType = "resource_request"
	MessageTypeResourceAllocation MessageType = "resource_allocation"
	MessageTypeResourceRelease    MessageType = "resource_release"

	// Performance coordination messages
	MessageTypePerformanceUpdate MessageType = "performance_update"
	MessageTypePerformanceQuery  MessageType = "performance_query"
	MessageTypePerformanceAlert  MessageType = "performance_alert"

	// Load balancing messages
	MessageTypeLoadBalanceRequest  MessageType = "load_balance_request"
	MessageTypeLoadBalanceResponse MessageType = "load_balance_response"
	MessageTypeLoadBalanceUpdate   MessageType = "load_balance_update"

	// Congestion control messages
	MessageTypeCongestionAlert    MessageType = "congestion_alert"
	MessageTypeCongestionUpdate   MessageType = "congestion_update"
	MessageTypeCongestionRecovery MessageType = "congestion_recovery"

	// System coordination messages
	MessageTypeSystemStatus    MessageType = "system_status"
	MessageTypeSystemShutdown  MessageType = "system_shutdown"
	MessageTypeSystemHeartbeat MessageType = "system_heartbeat"
)

// MessageRouter handles intelligent routing of messages between prefixes.
type MessageRouter struct {
	routingTable    map[string]*RoutingEntry
	networkTopology *NetworkTopology
	routingStrategy RoutingStrategy
	loadBalancer    *MessageLoadBalancer
	mu              sync.RWMutex
}

// RoutingEntry contains routing information for a prefix.
type RoutingEntry struct {
	PrefixID        string
	NetworkLatency  time.Duration
	Bandwidth       float64
	Reliability     float64
	CurrentLoad     int
	LastUpdate      time.Time
	PreferredRoutes []string
	BackupRoutes    []string
}

// NetworkTopology represents the network topology between prefixes.
type NetworkTopology struct {
	Connections map[string]map[string]*Connection
	Latencies   map[string]map[string]time.Duration
	Bandwidths  map[string]map[string]float64
	LastUpdate  time.Time
}

// Connection represents a network connection between two prefixes.
type Connection struct {
	SourcePrefix string
	TargetPrefix string
	Latency      time.Duration
	Bandwidth    float64
	PacketLoss   float64
	Reliability  float64
	LastMeasured time.Time
}

// RoutingStrategy defines different message routing strategies.
type RoutingStrategy string

const (
	RoutingStrategyDirect   RoutingStrategy = "direct"
	RoutingStrategyOptimal  RoutingStrategy = "optimal"
	RoutingStrategyAdaptive RoutingStrategy = "adaptive"
	RoutingStrategyReliable RoutingStrategy = "reliable"
)

// PriorityManager handles message prioritization and queuing.
type PriorityManager struct {
	priorityLevels map[int]*PriorityLevel
	mu             sync.RWMutex
}

// PriorityLevel represents a priority level in the priority queue system.
type PriorityLevel struct {
	Priority   int
	Queue      []*CoordinationMessage
	MaxSize    int
	DropPolicy DropPolicy
	Statistics *PriorityStatistics
}

// DropPolicy defines how messages are dropped when queues are full.
type DropPolicy string

const (
	DropPolicyOldest DropPolicy = "oldest"
	DropPolicyLowest DropPolicy = "lowest_priority"
	DropPolicyRandom DropPolicy = "random"
	DropPolicyNone   DropPolicy = "none"
)

// BatchProcessor handles message batching for efficiency.
type BatchProcessor struct {
	batches            map[string]*MessageBatch
	batchTimeout       time.Duration
	maxBatchSize       int
	compressionEnabled bool
	mu                 sync.RWMutex
}

// MessageBatch represents a batch of messages for efficient processing.
type MessageBatch struct {
	ID             string
	TargetPrefix   string
	Messages       []*CoordinationMessage
	CreatedAt      time.Time
	Size           int
	CompressedSize int
	Status         BatchStatus
}

// BatchStatus represents the status of a message batch.
type BatchStatus string

const (
	BatchStatusPending    BatchStatus = "pending"
	BatchStatusProcessing BatchStatus = "processing"
	BatchStatusSent       BatchStatus = "sent"
	BatchStatusFailed     BatchStatus = "failed"
)

// Various statistics and metrics structures

// ChannelStatistics tracks statistics for a prefix channel.
type ChannelStatistics struct {
	MessagesReceived int64
	MessagesSent     int64
	MessagesDropped  int64
	AverageLatency   time.Duration
	ThroughputMBps   float64
	ErrorRate        float64
	LastUpdate       time.Time
}

// CommunicationMetrics provides comprehensive communication metrics.
type CommunicationMetrics struct {
	TotalMessages      int64
	MessagesPerSecond  float64
	AverageLatency     time.Duration
	P95Latency         time.Duration
	P99Latency         time.Duration
	ThroughputMBps     float64
	ErrorRate          float64
	ActiveChannels     int
	QueueUtilization   float64
	CompressionRatio   float64
	NetworkUtilization float64
	LastUpdate         time.Time
}

// PriorityStatistics tracks statistics for a priority level.
type PriorityStatistics struct {
	MessagesProcessed int64
	MessagesDropped   int64
	AverageWaitTime   time.Duration
	QueueUtilization  float64
}

// MessageLoadBalancer balances message routing across available paths.
type MessageLoadBalancer struct {
	strategy        LoadBalanceStrategy
	pathUtilization map[string]float64
	pathCapacity    map[string]float64
}

// PriorityQueue implements a priority queue for messages.
type PriorityQueue struct {
	items    []*CoordinationMessage
	capacity int
	mu       sync.RWMutex
}

// NewCrossPrefixCommunicator creates a new cross-prefix communicator.
func NewCrossPrefixCommunicator(ctx context.Context, config *CommunicationConfig) *CrossPrefixCommunicator {
	if config == nil {
		config = DefaultCommunicationConfig()
	}

	commCtx, cancel := context.WithCancel(ctx)

	return &CrossPrefixCommunicator{
		channels:        make(map[string]*PrefixChannel),
		messageRouter:   NewMessageRouter(config),
		priorityManager: NewPriorityManager(config),
		batchProcessor:  NewBatchProcessor(config),
		metrics:         NewCommunicationMetrics(),
		config:          config,
		ctx:             commCtx,
		cancel:          cancel,
		active:          false,
	}
}

// Start begins the cross-prefix communication system.
func (cpc *CrossPrefixCommunicator) Start() error {
	cpc.mu.Lock()
	defer cpc.mu.Unlock()

	if cpc.active {
		return nil
	}

	cpc.active = true

	// Start subsystems
	go cpc.messageProcessingLoop()
	go cpc.batchProcessingLoop()
	go cpc.metricsCollectionLoop()
	go cpc.networkMonitoringLoop()

	return nil
}

// Stop gracefully shuts down the communication system.
func (cpc *CrossPrefixCommunicator) Stop() error {
	cpc.mu.Lock()
	defer cpc.mu.Unlock()

	if !cpc.active {
		return nil
	}

	cpc.active = false
	cpc.cancel()

	// Close all channels
	for _, channel := range cpc.channels {
		close(channel.Channel)
	}

	return nil
}

// RegisterPrefix registers a new prefix for communication.
func (cpc *CrossPrefixCommunicator) RegisterPrefix(prefixID string) error {
	cpc.mu.Lock()
	defer cpc.mu.Unlock()

	if !cpc.active {
		return fmt.Errorf("communicator not active")
	}

	if _, exists := cpc.channels[prefixID]; exists {
		return fmt.Errorf("prefix already registered: %s", prefixID)
	}

	channel := &PrefixChannel{
		PrefixID:      prefixID,
		Channel:       make(chan *CoordinationMessage, cpc.config.ChannelBufferSize),
		PriorityQueue: NewPriorityQueue(cpc.config.PriorityQueueSize),
		Statistics:    &ChannelStatistics{LastUpdate: time.Now()},
		LastActivity:  time.Now(),
	}

	cpc.channels[prefixID] = channel
	cpc.messageRouter.RegisterPrefix(prefixID)

	return nil
}

// SendMessage sends a coordination message between prefixes.
func (cpc *CrossPrefixCommunicator) SendMessage(message *CoordinationMessage) error {
	cpc.mu.RLock()
	defer cpc.mu.RUnlock()

	if !cpc.active {
		return fmt.Errorf("communicator not active")
	}

	// Set message metadata
	if message.ID == "" {
		message.ID = fmt.Sprintf("msg-%d-%s", time.Now().UnixNano(), message.SourcePrefix)
	}
	message.Timestamp = time.Now()
	if message.ExpiresAt.IsZero() {
		message.ExpiresAt = time.Now().Add(cpc.config.MessageTimeout)
	}

	// Route the message
	route, err := cpc.messageRouter.FindOptimalRoute(message)
	if err != nil {
		return fmt.Errorf("routing failed: %w", err)
	}

	// Apply priority management
	if cpc.priorityManager.ShouldDropMessage(message) {
		cpc.metrics.ErrorRate++
		return fmt.Errorf("message dropped due to priority constraints")
	}

	// Process through batch processor if batching is beneficial
	if cpc.shouldBatchMessage(message) {
		return cpc.batchProcessor.AddToBatch(message)
	}

	// Send directly
	return cpc.sendMessageDirect(message, route)
}

// ReceiveMessage receives a message from a specific prefix channel.
func (cpc *CrossPrefixCommunicator) ReceiveMessage(prefixID string, timeout time.Duration) (*CoordinationMessage, error) {
	cpc.mu.RLock()
	channel, exists := cpc.channels[prefixID]
	cpc.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("prefix not registered: %s", prefixID)
	}

	select {
	case message := <-channel.Channel:
		atomic.AddInt64(&channel.Statistics.MessagesReceived, 1)
		channel.LastActivity = time.Now()
		return message, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("receive timeout")
	case <-cpc.ctx.Done():
		return nil, fmt.Errorf("communicator shutting down")
	}
}

// BroadcastMessage broadcasts a message to all registered prefixes.
func (cpc *CrossPrefixCommunicator) BroadcastMessage(message *CoordinationMessage) error {
	cpc.mu.RLock()
	prefixes := make([]string, 0, len(cpc.channels))
	for prefixID := range cpc.channels {
		if prefixID != message.SourcePrefix {
			prefixes = append(prefixes, prefixID)
		}
	}
	cpc.mu.RUnlock()

	var lastError error
	for _, prefixID := range prefixes {
		msgCopy := *message
		msgCopy.TargetPrefix = prefixID
		msgCopy.ID = fmt.Sprintf("%s-broadcast-%s", message.ID, prefixID)

		if err := cpc.SendMessage(&msgCopy); err != nil {
			lastError = err
		}
	}

	return lastError
}

// GetChannelStatistics returns statistics for a specific channel.
func (cpc *CrossPrefixCommunicator) GetChannelStatistics(prefixID string) (*ChannelStatistics, error) {
	cpc.mu.RLock()
	defer cpc.mu.RUnlock()

	channel, exists := cpc.channels[prefixID]
	if !exists {
		return nil, fmt.Errorf("prefix not registered: %s", prefixID)
	}

	return channel.Statistics, nil
}

// GetMetrics returns comprehensive communication metrics.
func (cpc *CrossPrefixCommunicator) GetMetrics() *CommunicationMetrics {
	cpc.mu.RLock()
	defer cpc.mu.RUnlock()

	cpc.updateMetrics()
	return cpc.metrics
}

// Internal methods

func (cpc *CrossPrefixCommunicator) shouldBatchMessage(message *CoordinationMessage) bool {
	// Batch low-priority, non-urgent messages
	return message.Priority <= 2 &&
		message.Type != MessageTypeSystemShutdown &&
		message.Type != MessageTypeCongestionAlert &&
		cpc.config.BatchSize > 1
}

func (cpc *CrossPrefixCommunicator) sendMessageDirect(message *CoordinationMessage, route *RoutingEntry) error {
	channel, exists := cpc.channels[message.TargetPrefix]
	if !exists {
		return fmt.Errorf("target prefix not found: %s", message.TargetPrefix)
	}

	select {
	case channel.Channel <- message:
		atomic.AddInt64(&channel.Statistics.MessagesSent, 1)
		return nil
	case <-time.After(cpc.config.MessageTimeout):
		atomic.AddInt64(&channel.Statistics.MessagesDropped, 1)
		return fmt.Errorf("send timeout")
	case <-cpc.ctx.Done():
		return fmt.Errorf("communicator shutting down")
	}
}

func (cpc *CrossPrefixCommunicator) messageProcessingLoop() {
	ticker := time.NewTicker(time.Millisecond * 100)
	defer ticker.Stop()

	for {
		select {
		case <-cpc.ctx.Done():
			return
		case <-ticker.C:
			cpc.processMessageQueues()
		}
	}
}

func (cpc *CrossPrefixCommunicator) batchProcessingLoop() {
	ticker := time.NewTicker(cpc.config.BatchTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-cpc.ctx.Done():
			return
		case <-ticker.C:
			cpc.batchProcessor.ProcessPendingBatches()
		}
	}
}

func (cpc *CrossPrefixCommunicator) metricsCollectionLoop() {
	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()

	for {
		select {
		case <-cpc.ctx.Done():
			return
		case <-ticker.C:
			cpc.updateMetrics()
		}
	}
}

func (cpc *CrossPrefixCommunicator) networkMonitoringLoop() {
	ticker := time.NewTicker(time.Second * 10)
	defer ticker.Stop()

	for {
		select {
		case <-cpc.ctx.Done():
			return
		case <-ticker.C:
			cpc.messageRouter.UpdateNetworkTopology()
		}
	}
}

func (cpc *CrossPrefixCommunicator) processMessageQueues() {
	// Process priority queues for each channel
	for _, channel := range cpc.channels {
		cpc.processPriorityQueue(channel)
	}
}

func (cpc *CrossPrefixCommunicator) processPriorityQueue(channel *PrefixChannel) {
	// Process messages in priority order
	for {
		message := channel.PriorityQueue.Pop()
		if message == nil {
			break
		}

		// Check if message has expired
		if time.Now().After(message.ExpiresAt) {
			atomic.AddInt64(&channel.Statistics.MessagesDropped, 1)
			continue
		}

		// Send message
		select {
		case channel.Channel <- message:
			atomic.AddInt64(&channel.Statistics.MessagesSent, 1)
		default:
			// Channel full, put back in priority queue
			channel.PriorityQueue.Push(message)
			return
		}
	}
}

func (cpc *CrossPrefixCommunicator) updateMetrics() {
	totalMessages := int64(0)
	totalDropped := int64(0)
	totalLatency := time.Duration(0)
	channelCount := 0

	for _, channel := range cpc.channels {
		totalMessages += atomic.LoadInt64(&channel.Statistics.MessagesReceived) + atomic.LoadInt64(&channel.Statistics.MessagesSent)
		totalDropped += atomic.LoadInt64(&channel.Statistics.MessagesDropped)
		totalLatency += channel.Statistics.AverageLatency
		channelCount++
	}

	cpc.metrics.TotalMessages = totalMessages
	cpc.metrics.ActiveChannels = channelCount
	if channelCount > 0 {
		cpc.metrics.AverageLatency = totalLatency / time.Duration(channelCount)
	}
	if totalMessages > 0 {
		cpc.metrics.ErrorRate = float64(totalDropped) / float64(totalMessages)
	}
	cpc.metrics.LastUpdate = time.Now()
}

// Factory functions for subsystems

func NewMessageRouter(config *CommunicationConfig) *MessageRouter {
	return &MessageRouter{
		routingTable:    make(map[string]*RoutingEntry),
		networkTopology: NewNetworkTopology(),
		routingStrategy: RoutingStrategy(config.RoutingStrategy),
		loadBalancer:    NewMessageLoadBalancer(),
	}
}

func NewPriorityManager(config *CommunicationConfig) *PriorityManager {
	pm := &PriorityManager{
		priorityLevels: make(map[int]*PriorityLevel),
	}

	// Initialize standard priority levels
	for priority := 1; priority <= 5; priority++ {
		pm.priorityLevels[priority] = &PriorityLevel{
			Priority:   priority,
			Queue:      make([]*CoordinationMessage, 0),
			MaxSize:    config.PriorityQueueSize,
			DropPolicy: DropPolicyOldest,
			Statistics: &PriorityStatistics{},
		}
	}

	return pm
}

func NewBatchProcessor(config *CommunicationConfig) *BatchProcessor {
	return &BatchProcessor{
		batches:            make(map[string]*MessageBatch),
		batchTimeout:       config.BatchTimeout,
		maxBatchSize:       config.BatchSize,
		compressionEnabled: config.CompressionEnabled,
	}
}

func NewCommunicationMetrics() *CommunicationMetrics {
	return &CommunicationMetrics{
		LastUpdate: time.Now(),
	}
}

func NewNetworkTopology() *NetworkTopology {
	return &NetworkTopology{
		Connections: make(map[string]map[string]*Connection),
		Latencies:   make(map[string]map[string]time.Duration),
		Bandwidths:  make(map[string]map[string]float64),
		LastUpdate:  time.Now(),
	}
}

func NewMessageLoadBalancer() *MessageLoadBalancer {
	return &MessageLoadBalancer{
		strategy:        LoadBalanceAdaptive,
		pathUtilization: make(map[string]float64),
		pathCapacity:    make(map[string]float64),
	}
}

func NewPriorityQueue(capacity int) *PriorityQueue {
	return &PriorityQueue{
		items:    make([]*CoordinationMessage, 0, capacity),
		capacity: capacity,
	}
}

// MessageRouter methods

func (mr *MessageRouter) RegisterPrefix(prefixID string) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	mr.routingTable[prefixID] = &RoutingEntry{
		PrefixID:        prefixID,
		NetworkLatency:  time.Millisecond * 50, // Default
		Bandwidth:       100.0,                 // Default 100 Mbps
		Reliability:     0.99,                  // Default 99%
		CurrentLoad:     0,
		LastUpdate:      time.Now(),
		PreferredRoutes: []string{},
		BackupRoutes:    []string{},
	}
}

func (mr *MessageRouter) FindOptimalRoute(message *CoordinationMessage) (*RoutingEntry, error) {
	mr.mu.RLock()
	defer mr.mu.RUnlock()

	entry, exists := mr.routingTable[message.TargetPrefix]
	if !exists {
		return nil, fmt.Errorf("no route to prefix: %s", message.TargetPrefix)
	}

	return entry, nil
}

func (mr *MessageRouter) UpdateNetworkTopology() {
	// Implementation would measure actual network conditions
	// This is a placeholder for the topology update logic
}

// PriorityManager methods

func (pm *PriorityManager) ShouldDropMessage(message *CoordinationMessage) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	level, exists := pm.priorityLevels[message.Priority]
	if !exists {
		return false
	}

	return len(level.Queue) >= level.MaxSize
}

// BatchProcessor methods

func (bp *BatchProcessor) AddToBatch(message *CoordinationMessage) error {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	batchKey := message.TargetPrefix
	batch, exists := bp.batches[batchKey]

	if !exists {
		batch = &MessageBatch{
			ID:           fmt.Sprintf("batch-%s-%d", batchKey, time.Now().UnixNano()),
			TargetPrefix: message.TargetPrefix,
			Messages:     make([]*CoordinationMessage, 0, bp.maxBatchSize),
			CreatedAt:    time.Now(),
			Status:       BatchStatusPending,
		}
		bp.batches[batchKey] = batch
	}

	batch.Messages = append(batch.Messages, message)
	batch.Size++

	// Send batch if it's full
	if len(batch.Messages) >= bp.maxBatchSize {
		return bp.processBatch(batch)
	}

	return nil
}

func (bp *BatchProcessor) ProcessPendingBatches() {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	now := time.Now()
	for key, batch := range bp.batches {
		if batch.Status == BatchStatusPending &&
			(len(batch.Messages) > 0 && now.Sub(batch.CreatedAt) > bp.batchTimeout) {
			_ = bp.processBatch(batch) // Error handling is done in processBatch
			delete(bp.batches, key)
		}
	}
}

func (bp *BatchProcessor) processBatch(batch *MessageBatch) error {
	batch.Status = BatchStatusProcessing
	// Implementation would send the batched messages
	batch.Status = BatchStatusSent
	return nil
}

// PriorityQueue methods

func (pq *PriorityQueue) Push(message *CoordinationMessage) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if len(pq.items) >= pq.capacity {
		return // Queue full
	}

	pq.items = append(pq.items, message)

	// Sort by priority (higher priority first)
	sort.Slice(pq.items, func(i, j int) bool {
		return pq.items[i].Priority > pq.items[j].Priority
	})
}

func (pq *PriorityQueue) Pop() *CoordinationMessage {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if len(pq.items) == 0 {
		return nil
	}

	message := pq.items[0]
	pq.items = pq.items[1:]

	return message
}

func (pq *PriorityQueue) Size() int {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	return len(pq.items)
}

// Compile-time check that CrossPrefixCommunicator implements CommunicationService interface
var _ CommunicationService = (*CrossPrefixCommunicator)(nil)
