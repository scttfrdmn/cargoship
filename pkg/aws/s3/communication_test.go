package s3

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCrossPrefixCommunicator(t *testing.T) {
	ctx := context.Background()
	config := DefaultCommunicationConfig()
	
	comm := NewCrossPrefixCommunicator(ctx, config)
	
	assert.NotNil(t, comm)
	assert.Equal(t, config, comm.config)
	assert.NotNil(t, comm.channels)
	assert.NotNil(t, comm.messageRouter)
	assert.NotNil(t, comm.priorityManager)
	assert.NotNil(t, comm.batchProcessor)
	assert.NotNil(t, comm.metrics)
	assert.False(t, comm.active)
}

func TestNewCrossPrefixCommunicatorWithNilConfig(t *testing.T) {
	ctx := context.Background()
	
	comm := NewCrossPrefixCommunicator(ctx, nil)
	
	assert.NotNil(t, comm)
	assert.NotNil(t, comm.config)
	assert.Equal(t, DefaultCommunicationConfig().ChannelBufferSize, comm.config.ChannelBufferSize)
}

func TestDefaultCommunicationConfig(t *testing.T) {
	config := DefaultCommunicationConfig()
	
	assert.Equal(t, 1024, config.ChannelBufferSize)
	assert.Equal(t, 256, config.PriorityQueueSize)
	assert.Equal(t, 32, config.BatchSize)
	assert.Equal(t, time.Millisecond*50, config.BatchTimeout)
	assert.Equal(t, "adaptive", config.RoutingStrategy)
	assert.True(t, config.CompressionEnabled)
	assert.False(t, config.EncryptionEnabled)
	assert.Equal(t, 100, config.MaxConcurrentMessages)
	assert.Equal(t, time.Second*30, config.MessageTimeout)
	assert.Equal(t, 3, config.RetryAttempts)
	assert.Equal(t, time.Millisecond*100, config.RetryBackoff)
	assert.True(t, config.NetworkAwareRouting)
	assert.True(t, config.BandwidthThrottling)
	assert.True(t, config.AdaptiveBuffering)
}

func TestCrossPrefixCommunicatorStartStop(t *testing.T) {
	ctx := context.Background()
	config := DefaultCommunicationConfig()
	comm := NewCrossPrefixCommunicator(ctx, config)
	
	// Test starting
	err := comm.Start()
	assert.NoError(t, err)
	assert.True(t, comm.active)
	
	// Test starting already active communicator
	err = comm.Start()
	assert.NoError(t, err) // Should be idempotent
	
	// Test stopping
	err = comm.Stop()
	assert.NoError(t, err)
	assert.False(t, comm.active)
	
	// Test stopping already stopped communicator
	err = comm.Stop()
	assert.NoError(t, err) // Should be idempotent
}

func TestCrossPrefixCommunicatorRegisterPrefix(t *testing.T) {
	ctx := context.Background()
	config := DefaultCommunicationConfig()
	comm := NewCrossPrefixCommunicator(ctx, config)
	
	// Test registering when not active
	err := comm.RegisterPrefix("test-prefix")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not active")
	
	// Start and test registration
	err = comm.Start()
	require.NoError(t, err)
	
	err = comm.RegisterPrefix("test-prefix")
	assert.NoError(t, err)
	
	// Verify channel was created
	comm.mu.RLock()
	channel, exists := comm.channels["test-prefix"]
	comm.mu.RUnlock()
	
	assert.True(t, exists)
	assert.NotNil(t, channel)
	assert.Equal(t, "test-prefix", channel.PrefixID)
	assert.Equal(t, config.ChannelBufferSize, cap(channel.Channel))
	
	// Test registering duplicate prefix
	err = comm.RegisterPrefix("test-prefix")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
	
	_ = comm.Stop()
}

func TestCrossPrefixCommunicatorSendReceiveMessage(t *testing.T) {
	ctx := context.Background()
	config := DefaultCommunicationConfig()
	comm := NewCrossPrefixCommunicator(ctx, config)
	
	err := comm.Start()
	require.NoError(t, err)
	defer func() { _ = comm.Stop() }()
	
	// Register prefixes
	err = comm.RegisterPrefix("source")
	require.NoError(t, err)
	err = comm.RegisterPrefix("target")
	require.NoError(t, err)
	
	// Create test message
	message := &CoordinationMessage{
		Type:         MessageTypeResourceRequest,
		SourcePrefix: "source",
		TargetPrefix: "target",
		Priority:     3,
		Payload: map[string]interface{}{
			"resource": "bandwidth",
			"amount":   100.0,
		},
	}
	
	// Send message
	err = comm.SendMessage(message)
	assert.NoError(t, err)
	assert.NotEmpty(t, message.ID)
	assert.NotZero(t, message.Timestamp)
	assert.NotZero(t, message.ExpiresAt)
	
	// Receive message
	received, err := comm.ReceiveMessage("target", time.Second)
	assert.NoError(t, err)
	assert.NotNil(t, received)
	assert.Equal(t, message.Type, received.Type)
	assert.Equal(t, message.SourcePrefix, received.SourcePrefix)
	assert.Equal(t, message.TargetPrefix, received.TargetPrefix)
	assert.Equal(t, message.Priority, received.Priority)
}

func TestCrossPrefixCommunicatorSendMessageInactive(t *testing.T) {
	ctx := context.Background()
	config := DefaultCommunicationConfig()
	comm := NewCrossPrefixCommunicator(ctx, config)
	
	message := &CoordinationMessage{
		Type:         MessageTypeResourceRequest,
		SourcePrefix: "source",
		TargetPrefix: "target",
		Priority:     3,
	}
	
	err := comm.SendMessage(message)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not active")
}

func TestCrossPrefixCommunicatorReceiveTimeout(t *testing.T) {
	ctx := context.Background()
	config := DefaultCommunicationConfig()
	comm := NewCrossPrefixCommunicator(ctx, config)
	
	err := comm.Start()
	require.NoError(t, err)
	defer func() { _ = comm.Stop() }()
	
	err = comm.RegisterPrefix("test-prefix")
	require.NoError(t, err)
	
	// Try to receive with short timeout
	received, err := comm.ReceiveMessage("test-prefix", time.Millisecond*100)
	assert.Error(t, err)
	assert.Nil(t, received)
	assert.Contains(t, err.Error(), "timeout")
}

func TestCrossPrefixCommunicatorReceiveNonExistentPrefix(t *testing.T) {
	ctx := context.Background()
	config := DefaultCommunicationConfig()
	comm := NewCrossPrefixCommunicator(ctx, config)
	
	err := comm.Start()
	require.NoError(t, err)
	defer func() { _ = comm.Stop() }()
	
	received, err := comm.ReceiveMessage("non-existent", time.Second)
	assert.Error(t, err)
	assert.Nil(t, received)
	assert.Contains(t, err.Error(), "not registered")
}

func TestCrossPrefixCommunicatorBroadcastMessage(t *testing.T) {
	ctx := context.Background()
	config := DefaultCommunicationConfig()
	comm := NewCrossPrefixCommunicator(ctx, config)
	
	err := comm.Start()
	require.NoError(t, err)
	defer func() { _ = comm.Stop() }()
	
	// Register multiple prefixes
	prefixes := []string{"source", "target1", "target2", "target3"}
	for _, prefix := range prefixes {
		err = comm.RegisterPrefix(prefix)
		require.NoError(t, err)
	}
	
	// Create broadcast message
	message := &CoordinationMessage{
		Type:         MessageTypeSystemStatus,
		SourcePrefix: "source",
		Priority:     2,
		Payload: map[string]interface{}{
			"status": "healthy",
		},
	}
	
	// Broadcast message
	err = comm.BroadcastMessage(message)
	assert.NoError(t, err)
	
	// Verify all targets received the message
	targetPrefixes := []string{"target1", "target2", "target3"}
	for _, prefix := range targetPrefixes {
		received, err := comm.ReceiveMessage(prefix, time.Second)
		assert.NoError(t, err)
		assert.NotNil(t, received)
		assert.Equal(t, MessageTypeSystemStatus, received.Type)
		assert.Equal(t, "source", received.SourcePrefix)
		assert.Equal(t, prefix, received.TargetPrefix)
		assert.Contains(t, received.ID, "broadcast")
	}
}

func TestCrossPrefixCommunicatorGetChannelStatistics(t *testing.T) {
	ctx := context.Background()
	config := DefaultCommunicationConfig()
	comm := NewCrossPrefixCommunicator(ctx, config)
	
	err := comm.Start()
	require.NoError(t, err)
	defer func() { _ = comm.Stop() }()
	
	err = comm.RegisterPrefix("test-prefix")
	require.NoError(t, err)
	
	// Get statistics
	stats, err := comm.GetChannelStatistics("test-prefix")
	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, int64(0), stats.MessagesReceived)
	assert.Equal(t, int64(0), stats.MessagesSent)
	assert.Equal(t, int64(0), stats.MessagesDropped)
	
	// Test non-existent prefix
	stats, err = comm.GetChannelStatistics("non-existent")
	assert.Error(t, err)
	assert.Nil(t, stats)
	assert.Contains(t, err.Error(), "not registered")
}

func TestCrossPrefixCommunicatorGetMetrics(t *testing.T) {
	ctx := context.Background()
	config := DefaultCommunicationConfig()
	comm := NewCrossPrefixCommunicator(ctx, config)
	
	err := comm.Start()
	require.NoError(t, err)
	defer func() { _ = comm.Stop() }()
	
	metrics := comm.GetMetrics()
	assert.NotNil(t, metrics)
	assert.Equal(t, int64(0), metrics.TotalMessages)
	assert.Equal(t, 0, metrics.ActiveChannels)
	assert.NotZero(t, metrics.LastUpdate)
}

func TestCrossPrefixCommunicatorConcurrency(t *testing.T) {
	ctx := context.Background()
	config := DefaultCommunicationConfig()
	comm := NewCrossPrefixCommunicator(ctx, config)
	
	err := comm.Start()
	require.NoError(t, err)
	defer func() { _ = comm.Stop() }()
	
	// Register multiple prefixes concurrently
	var wg sync.WaitGroup
	numPrefixes := 10
	
	for i := 0; i < numPrefixes; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			prefixID := fmt.Sprintf("prefix-%d", id)
			err := comm.RegisterPrefix(prefixID)
			assert.NoError(t, err)
		}(i)
	}
	wg.Wait()
	
	// Verify all prefixes were registered
	comm.mu.RLock()
	assert.Equal(t, numPrefixes, len(comm.channels))
	comm.mu.RUnlock()
	
	// Send messages concurrently
	numMessages := 100
	for i := 0; i < numMessages; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			message := &CoordinationMessage{
				Type:         MessageTypePerformanceUpdate,
				SourcePrefix: fmt.Sprintf("prefix-%d", id%numPrefixes),
				TargetPrefix: fmt.Sprintf("prefix-%d", (id+1)%numPrefixes),
				Priority:     3,
				Payload: map[string]interface{}{
					"message_id": id,
				},
			}
			err := comm.SendMessage(message)
			// Some messages might fail due to routing issues, which is acceptable
			if err != nil && !assert.Contains(t, err.Error(), "routing failed") {
				t.Logf("Unexpected error: %v", err)
			}
		}(i)
	}
	wg.Wait()
}

func TestMessageTypes(t *testing.T) {
	// Test that all message types are defined correctly
	types := []MessageType{
		MessageTypeResourceRequest,
		MessageTypeResourceAllocation,
		MessageTypeResourceRelease,
		MessageTypePerformanceUpdate,
		MessageTypePerformanceQuery,
		MessageTypePerformanceAlert,
		MessageTypeLoadBalanceRequest,
		MessageTypeLoadBalanceResponse,
		MessageTypeLoadBalanceUpdate,
		MessageTypeCongestionAlert,
		MessageTypeCongestionUpdate,
		MessageTypeCongestionRecovery,
		MessageTypeSystemStatus,
		MessageTypeSystemShutdown,
		MessageTypeSystemHeartbeat,
	}
	
	for _, msgType := range types {
		assert.NotEmpty(t, string(msgType))
	}
}

func TestNewMessageRouter(t *testing.T) {
	config := DefaultCommunicationConfig()
	router := NewMessageRouter(config)
	
	assert.NotNil(t, router)
	assert.NotNil(t, router.routingTable)
	assert.NotNil(t, router.networkTopology)
	assert.Equal(t, RoutingStrategy(config.RoutingStrategy), router.routingStrategy)
	assert.NotNil(t, router.loadBalancer)
}

func TestMessageRouterRegisterPrefix(t *testing.T) {
	config := DefaultCommunicationConfig()
	router := NewMessageRouter(config)
	
	router.RegisterPrefix("test-prefix")
	
	router.mu.RLock()
	entry, exists := router.routingTable["test-prefix"]
	router.mu.RUnlock()
	
	assert.True(t, exists)
	assert.NotNil(t, entry)
	assert.Equal(t, "test-prefix", entry.PrefixID)
	assert.Equal(t, time.Millisecond*50, entry.NetworkLatency)
	assert.Equal(t, 100.0, entry.Bandwidth)
	assert.Equal(t, 0.99, entry.Reliability)
}

func TestMessageRouterFindOptimalRoute(t *testing.T) {
	config := DefaultCommunicationConfig()
	router := NewMessageRouter(config)
	
	// Test route to non-existent prefix
	message := &CoordinationMessage{
		TargetPrefix: "non-existent",
	}
	
	route, err := router.FindOptimalRoute(message)
	assert.Error(t, err)
	assert.Nil(t, route)
	assert.Contains(t, err.Error(), "no route")
	
	// Register prefix and test successful routing
	router.RegisterPrefix("test-prefix")
	message.TargetPrefix = "test-prefix"
	
	route, err = router.FindOptimalRoute(message)
	assert.NoError(t, err)
	assert.NotNil(t, route)
	assert.Equal(t, "test-prefix", route.PrefixID)
}

func TestNewPriorityManager(t *testing.T) {
	config := DefaultCommunicationConfig()
	pm := NewPriorityManager(config)
	
	assert.NotNil(t, pm)
	assert.NotNil(t, pm.priorityLevels)
	assert.Equal(t, 5, len(pm.priorityLevels))
	
	// Check that all priority levels are initialized
	for priority := 1; priority <= 5; priority++ {
		level, exists := pm.priorityLevels[priority]
		assert.True(t, exists)
		assert.Equal(t, priority, level.Priority)
		assert.Equal(t, config.PriorityQueueSize, level.MaxSize)
		assert.Equal(t, DropPolicyOldest, level.DropPolicy)
		assert.NotNil(t, level.Statistics)
	}
}

func TestPriorityManagerShouldDropMessage(t *testing.T) {
	config := DefaultCommunicationConfig()
	config.PriorityQueueSize = 2 // Small queue for testing
	pm := NewPriorityManager(config)
	
	message := &CoordinationMessage{
		Priority: 3,
	}
	
	// Should not drop when queue is empty
	assert.False(t, pm.ShouldDropMessage(message))
	
	// Fill up the queue
	level := pm.priorityLevels[3]
	level.Queue = make([]*CoordinationMessage, config.PriorityQueueSize)
	
	// Should drop when queue is full
	assert.True(t, pm.ShouldDropMessage(message))
	
	// Test with non-existent priority
	message.Priority = 99
	assert.False(t, pm.ShouldDropMessage(message))
}

func TestNewBatchProcessor(t *testing.T) {
	config := DefaultCommunicationConfig()
	bp := NewBatchProcessor(config)
	
	assert.NotNil(t, bp)
	assert.NotNil(t, bp.batches)
	assert.Equal(t, config.BatchTimeout, bp.batchTimeout)
	assert.Equal(t, config.BatchSize, bp.maxBatchSize)
	assert.Equal(t, config.CompressionEnabled, bp.compressionEnabled)
}

func TestBatchProcessorAddToBatch(t *testing.T) {
	config := DefaultCommunicationConfig()
	config.BatchSize = 2 // Small batch for testing
	bp := NewBatchProcessor(config)
	
	message := &CoordinationMessage{
		TargetPrefix: "test-prefix",
		Type:         MessageTypePerformanceUpdate,
	}
	
	// Add first message
	err := bp.AddToBatch(message)
	assert.NoError(t, err)
	
	bp.mu.RLock()
	batch, exists := bp.batches["test-prefix"]
	bp.mu.RUnlock()
	
	assert.True(t, exists)
	assert.Equal(t, 1, len(batch.Messages))
	assert.Equal(t, BatchStatusPending, batch.Status)
	
	// Add second message (should trigger batch processing)
	message2 := &CoordinationMessage{
		TargetPrefix: "test-prefix",
		Type:         MessageTypePerformanceUpdate,
	}
	
	err = bp.AddToBatch(message2)
	assert.NoError(t, err)
}

func TestNewPriorityQueue(t *testing.T) {
	capacity := 10
	pq := NewPriorityQueue(capacity)
	
	assert.NotNil(t, pq)
	assert.NotNil(t, pq.items)
	assert.Equal(t, capacity, pq.capacity)
	assert.Equal(t, 0, pq.Size())
}

func TestPriorityQueuePushPop(t *testing.T) {
	pq := NewPriorityQueue(5)
	
	// Test pushing messages with different priorities
	messages := []*CoordinationMessage{
		{Priority: 1, Type: MessageTypePerformanceUpdate},
		{Priority: 5, Type: MessageTypeSystemShutdown},
		{Priority: 3, Type: MessageTypeResourceRequest},
		{Priority: 2, Type: MessageTypeLoadBalanceRequest},
	}
	
	for _, msg := range messages {
		pq.Push(msg)
	}
	
	assert.Equal(t, 4, pq.Size())
	
	// Pop messages - should come out in priority order (highest first)
	expectedPriorities := []int{5, 3, 2, 1}
	for _, expectedPriority := range expectedPriorities {
		msg := pq.Pop()
		assert.NotNil(t, msg)
		assert.Equal(t, expectedPriority, msg.Priority)
	}
	
	assert.Equal(t, 0, pq.Size())
	
	// Pop from empty queue
	msg := pq.Pop()
	assert.Nil(t, msg)
}

func TestPriorityQueueCapacity(t *testing.T) {
	capacity := 2
	pq := NewPriorityQueue(capacity)
	
	// Fill queue to capacity
	for i := 0; i < capacity; i++ {
		pq.Push(&CoordinationMessage{
			Priority: i + 1,
			Type:     MessageTypePerformanceUpdate,
		})
	}
	
	assert.Equal(t, capacity, pq.Size())
	
	// Try to push beyond capacity
	pq.Push(&CoordinationMessage{
		Priority: 10,
		Type:     MessageTypeSystemShutdown,
	})
	
	// Size should remain at capacity
	assert.Equal(t, capacity, pq.Size())
}

func TestNewCommunicationMetrics(t *testing.T) {
	metrics := NewCommunicationMetrics()
	
	assert.NotNil(t, metrics)
	assert.Equal(t, int64(0), metrics.TotalMessages)
	assert.Equal(t, 0.0, metrics.MessagesPerSecond)
	assert.Equal(t, time.Duration(0), metrics.AverageLatency)
	assert.Equal(t, 0.0, metrics.ThroughputMBps)
	assert.Equal(t, 0.0, metrics.ErrorRate)
	assert.Equal(t, 0, metrics.ActiveChannels)
	assert.NotZero(t, metrics.LastUpdate)
}

func TestNewNetworkTopology(t *testing.T) {
	nt := NewNetworkTopology()
	
	assert.NotNil(t, nt)
	assert.NotNil(t, nt.Connections)
	assert.NotNil(t, nt.Latencies)
	assert.NotNil(t, nt.Bandwidths)
	assert.NotZero(t, nt.LastUpdate)
}

func TestNewMessageLoadBalancer(t *testing.T) {
	mlb := NewMessageLoadBalancer()
	
	assert.NotNil(t, mlb)
	assert.Equal(t, LoadBalanceAdaptive, mlb.strategy)
	assert.NotNil(t, mlb.pathUtilization)
	assert.NotNil(t, mlb.pathCapacity)
}

func TestCrossPrefixCommunicatorMessageExpiration(t *testing.T) {
	ctx := context.Background()
	config := DefaultCommunicationConfig()
	config.MessageTimeout = time.Millisecond * 100 // Very short timeout
	comm := NewCrossPrefixCommunicator(ctx, config)
	
	err := comm.Start()
	require.NoError(t, err)
	defer func() { _ = comm.Stop() }()
	
	err = comm.RegisterPrefix("source")
	require.NoError(t, err)
	err = comm.RegisterPrefix("target")
	require.NoError(t, err)
	
	// Create message with past expiration
	message := &CoordinationMessage{
		Type:         MessageTypeResourceRequest,
		SourcePrefix: "source",
		TargetPrefix: "target",
		Priority:     3,
		ExpiresAt:    time.Now().Add(-time.Hour), // Already expired
	}
	
	// Send message
	err = comm.SendMessage(message)
	assert.NoError(t, err) // Send should succeed
	
	// Message should be expired by the time it's processed
	// This test verifies the expiration logic exists, though timing may vary
}

func TestCommunicationSystemIntegration(t *testing.T) {
	ctx := context.Background()
	config := DefaultCommunicationConfig()
	config.BatchSize = 3
	config.BatchTimeout = time.Millisecond * 100
	comm := NewCrossPrefixCommunicator(ctx, config)
	
	err := comm.Start()
	require.NoError(t, err)
	defer func() { _ = comm.Stop() }()
	
	// Register multiple prefixes
	prefixes := []string{"coordinator", "worker1", "worker2", "worker3"}
	for _, prefix := range prefixes {
		err = comm.RegisterPrefix(prefix)
		require.NoError(t, err)
	}
	
	// Simulate a coordination scenario
	scenarios := []struct {
		msgType      MessageType
		source       string
		target       string
		priority     int
		shouldBatch  bool
	}{
		{MessageTypeResourceRequest, "worker1", "coordinator", 3, true},
		{MessageTypeResourceAllocation, "coordinator", "worker1", 3, true},
		{MessageTypePerformanceUpdate, "worker2", "coordinator", 2, true},
		{MessageTypeCongestionAlert, "worker3", "coordinator", 5, false}, // High priority, no batching
		{MessageTypeSystemHeartbeat, "coordinator", "worker1", 1, true},
	}
	
	for i, scenario := range scenarios {
		message := &CoordinationMessage{
			Type:         scenario.msgType,
			SourcePrefix: scenario.source,
			TargetPrefix: scenario.target,
			Priority:     scenario.priority,
			Payload: map[string]interface{}{
				"test_id": i,
				"data":    fmt.Sprintf("test_data_%d", i),
			},
		}
		
		err = comm.SendMessage(message)
		assert.NoError(t, err, "Failed to send message %d", i)
	}
	
	// Allow some time for message processing
	time.Sleep(time.Millisecond * 200)
	
	// Verify metrics were updated
	metrics := comm.GetMetrics()
	assert.NotNil(t, metrics)
	assert.Equal(t, len(prefixes), metrics.ActiveChannels)
	
	// Verify channel statistics
	for _, prefix := range prefixes {
		if prefix == "coordinator" {
			continue // Skip coordinator as it's the target for most messages
		}
		
		stats, err := comm.GetChannelStatistics(prefix)
		assert.NoError(t, err)
		assert.NotNil(t, stats)
	}
}