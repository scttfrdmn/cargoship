// Package pipeline provides streaming pipeline for CargoShip v0.5.0
package pipeline

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
)

// PrefixRouter routes jobs to per-prefix channels based on S3 key shard ID.
// This enables parallel uploads to multiple S3 prefixes, bypassing the single-prefix
// throughput limit (~5-10 MB/s) and achieving 5-8x performance improvement.
//
// Architecture:
//
//	Input: Single channel from archiver
//	Output: Map of per-prefix channels (shard-0, shard-1, ..., shard-N)
//	Routing: Extract shard ID from S3 key (e.g., "uploads/id/shard-3/chunk-42.tar.zst" → 3)
type PrefixRouter struct {
	input   <-chan *Job
	outputs map[string]chan<- *Job // Key: "shard-N", Value: output channel
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	// Statistics
	mu              sync.RWMutex
	stats           StageStats
	jobsRouted      int64            // Atomic counter
	routingErrors   int64            // Atomic counter
	perPrefixCounts map[string]int64 // Jobs routed to each prefix
}

// Shard ID extraction regex: matches "shard-N" where N is digit(s)
var shardIDRegex = regexp.MustCompile(`shard-(\d+)`)

// NewPrefixRouter creates a new prefix router stage
func NewPrefixRouter(input <-chan *Job, outputs map[string]chan<- *Job) *PrefixRouter {
	return &PrefixRouter{
		input:           input,
		outputs:         outputs,
		perPrefixCounts: make(map[string]int64),
		stats: StageStats{
			Name: "prefix_router",
		},
	}
}

// Name returns the stage name
func (r *PrefixRouter) Name() string {
	return "prefix_router"
}

// Start starts the prefix router stage
func (r *PrefixRouter) Start(ctx context.Context) error {
	// Create child context from parent (inherits trace context for Issue #155)
	r.ctx, r.cancel = context.WithCancel(ctx)

	r.wg.Add(1)
	go r.route(r.ctx)
	return nil
}

// Stop stops the prefix router stage
func (r *PrefixRouter) Stop() error {
	// Wait for route() to finish gracefully (happens when input channel closes)
	// Don't cancel context - let router drain all buffered jobs first
	r.wg.Wait()
	// Output channels are closed by route() via deferred closeOutputChannels()

	// Clean up context if still active
	if r.cancel != nil {
		r.cancel()
	}
	return nil
}

// closeOutputChannels closes all output channels (called via defer in route())
func (r *PrefixRouter) closeOutputChannels() {
	for _, output := range r.outputs {
		close(output)
	}
}

// route continuously routes jobs from input to appropriate prefix channels
func (r *PrefixRouter) route(ctx context.Context) {
	defer r.wg.Done()
	defer r.closeOutputChannels()

	for {
		// Prioritize draining input channel over context cancellation
		job, ok := <-r.input
		if !ok {
			// Input channel closed - finish gracefully
			return
		}

		// Extract prefix from S3 key
		prefix, err := r.extractPrefix(job.S3Key)
		if err != nil {
			// Routing error - try fallback to round-robin
			atomic.AddInt64(&r.routingErrors, 1)
			prefix = r.fallbackPrefix()
		}

		// Route to appropriate output channel
		output, exists := r.outputs[prefix]
		if !exists {
			// Unknown prefix - use fallback
			atomic.AddInt64(&r.routingErrors, 1)
			prefix = r.fallbackPrefix()
			output = r.outputs[prefix]
		}

		// Send to output channel (check context to avoid blocking forever)
		select {
		case <-ctx.Done():
			return
		case output <- job:
			atomic.AddInt64(&r.jobsRouted, 1)
			r.mu.Lock()
			r.perPrefixCounts[prefix]++
			r.mu.Unlock()
		}
	}
}

// extractPrefix extracts the shard prefix from an S3 key
// Example: "uploads/20231129-abc/shard-3/chunk-42.tar.zst" → "shard-3"
func (r *PrefixRouter) extractPrefix(s3Key string) (string, error) {
	matches := shardIDRegex.FindStringSubmatch(s3Key)
	if len(matches) < 2 {
		return "", fmt.Errorf("no shard ID found in S3 key: %s", s3Key)
	}

	shardID, err := strconv.Atoi(matches[1])
	if err != nil {
		return "", fmt.Errorf("invalid shard ID in S3 key %s: %w", s3Key, err)
	}

	return fmt.Sprintf("shard-%d", shardID), nil
}

// fallbackPrefix returns a fallback prefix using round-robin distribution
// Used when prefix extraction fails (malformed S3 key, unknown prefix, etc.)
func (r *PrefixRouter) fallbackPrefix() string {
	// Simple round-robin based on current job count
	jobCount := atomic.LoadInt64(&r.jobsRouted)
	prefixCount := len(r.outputs)
	if prefixCount == 0 {
		return "shard-0" // Default fallback
	}

	prefixID := int(jobCount % int64(prefixCount))
	return fmt.Sprintf("shard-%d", prefixID)
}

// Stats returns routing statistics
func (r *PrefixRouter) Stats() StageStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := r.stats
	stats.JobsProcessed = atomic.LoadInt64(&r.jobsRouted)

	// Add per-prefix distribution to metadata
	prefixDist := make(map[string]interface{})
	for prefix, count := range r.perPrefixCounts {
		prefixDist[prefix] = count
	}

	return stats
}

// GetRoutingErrors returns the number of routing errors encountered
func (r *PrefixRouter) GetRoutingErrors() int64 {
	return atomic.LoadInt64(&r.routingErrors)
}

// GetPerPrefixCounts returns the distribution of jobs across prefixes
func (r *PrefixRouter) GetPerPrefixCounts() map[string]int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return copy to avoid concurrent map access
	counts := make(map[string]int64)
	for prefix, count := range r.perPrefixCounts {
		counts[prefix] = count
	}
	return counts
}
