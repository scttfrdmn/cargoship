package pipeline

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/chunking"
)

func TestPrefixRouter_ExtractPrefix(t *testing.T) {
	tests := []struct {
		name      string
		s3Key     string
		wantShard string
		wantErr   bool
	}{
		{
			name:      "valid shard-0",
			s3Key:     "uploads/20231129-abc/shard-0/chunk-42.tar.zst",
			wantShard: "shard-0",
			wantErr:   false,
		},
		{
			name:      "valid shard-7",
			s3Key:     "uploads/20231129-abc/shard-7/chunk-123.tar.zst",
			wantShard: "shard-7",
			wantErr:   false,
		},
		{
			name:      "valid shard-15",
			s3Key:     "uploads/20231129-abc/shard-15/chunk-999.tar.zst",
			wantShard: "shard-15",
			wantErr:   false,
		},
		{
			name:      "no shard in key",
			s3Key:     "uploads/20231129-abc/chunk-42.tar.zst",
			wantShard: "",
			wantErr:   true,
		},
		{
			name:      "malformed shard",
			s3Key:     "uploads/20231129-abc/shard-/chunk-42.tar.zst",
			wantShard: "",
			wantErr:   true,
		},
		{
			name:      "empty key",
			s3Key:     "",
			wantShard: "",
			wantErr:   true,
		},
	}

	input := make(chan *Job)
	outputs := map[string]chan<- *Job{
		"shard-0": make(chan *Job, 1),
	}
	router := NewPrefixRouter(input, outputs)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := router.extractPrefix(tt.s3Key)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractPrefix() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.wantShard {
				t.Errorf("extractPrefix() = %v, want %v", got, tt.wantShard)
			}
		})
	}
}

func TestPrefixRouter_FallbackPrefix(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const numShards = 4
	const numJobs = 12

	// Create channels
	input := make(chan *Job, numJobs)
	outputChans := make(map[string]chan *Job)
	outputs := make(map[string]chan<- *Job)
	for i := 0; i < numShards; i++ {
		shardName := fmt.Sprintf("shard-%d", i)
		ch := make(chan *Job, numJobs)
		outputChans[shardName] = ch
		outputs[shardName] = ch
	}
	router := NewPrefixRouter(input, outputs)
	if err := router.Start(ctx); err != nil {
		t.Fatalf("Failed to start router: %v", err)
	}

	// Send jobs with malformed keys to trigger fallback routing
	go func() {
		for i := 0; i < numJobs; i++ {
			job := &Job{
				ID:    i,
				S3Key: fmt.Sprintf("uploads/test/chunk-%d.tar.zst", i), // No shard in key
				Chunk: chunking.Chunk{
					Files: []chunking.File{{Path: fmt.Sprintf("file-%d.txt", i)}},
				},
			}
			input <- job
		}
		close(input)
	}()

	// Collect jobs from each shard channel to verify round-robin distribution
	seen := make(map[string]bool)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for shardName, ch := range outputChans {
		wg.Add(1)
		go func(name string, channel chan *Job) {
			defer wg.Done()
			for range channel {
				mu.Lock()
				seen[name] = true
				mu.Unlock()
			}
		}(shardName, ch)
	}

	// Stop router (which will close output channels when routing is done)
	if err := router.Stop(); err != nil {
		t.Errorf("Failed to stop router: %v", err)
	}

	// Wait for all consumers to finish
	wg.Wait()

	// Verify all shards were used (round-robin)
	if len(seen) < 4 {
		t.Errorf("fallbackPrefix() didn't cycle through all shards, got %d unique shards, want 4", len(seen))
	}
}

func TestPrefixRouter_RoutingDistribution(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const numShards = 8
	const jobsPerShard = 10
	const totalJobs = numShards * jobsPerShard

	// Create channels
	input := make(chan *Job, totalJobs)
	outputChans := make(map[string]chan *Job)
	outputs := make(map[string]chan<- *Job)
	for i := 0; i < numShards; i++ {
		shardName := fmt.Sprintf("shard-%d", i)
		ch := make(chan *Job, jobsPerShard)
		outputChans[shardName] = ch
		outputs[shardName] = ch
	}

	// Create router
	router := NewPrefixRouter(input, outputs)
	if err := router.Start(ctx); err != nil {
		t.Fatalf("Failed to start router: %v", err)
	}

	// Send jobs with proper shard distribution
	go func() {
		for i := 0; i < totalJobs; i++ {
			shardID := i % numShards
			job := &Job{
				ID:    i,
				S3Key: fmt.Sprintf("uploads/test/shard-%d/chunk-%d.tar.zst", shardID, i),
				Chunk: chunking.Chunk{
					Files: []chunking.File{{Path: fmt.Sprintf("file-%d.txt", i)}},
				},
			}
			input <- job
		}
		close(input)
	}()

	// Collect jobs from each shard channel
	shardCounts := make(map[string]int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for shardName, ch := range outputChans {
		wg.Add(1)
		go func(name string, channel chan *Job) {
			defer wg.Done()
			count := 0
			for range channel {
				count++
			}
			mu.Lock()
			shardCounts[name] = count
			mu.Unlock()
		}(shardName, ch)
	}

	// Stop router (which will close output channels when routing is done)
	if err := router.Stop(); err != nil {
		t.Errorf("Failed to stop router: %v", err)
	}

	// Wait for all consumers to finish
	wg.Wait()

	// Lock for reading shardCounts
	mu.Lock()
	defer mu.Unlock()

	// Verify distribution
	for i := 0; i < numShards; i++ {
		shardName := fmt.Sprintf("shard-%d", i)
		count := shardCounts[shardName]
		if count != jobsPerShard {
			t.Errorf("Shard %s received %d jobs, want %d", shardName, count, jobsPerShard)
		}
	}

	// Verify router stats
	stats := router.Stats()
	if stats.JobsProcessed != totalJobs {
		t.Errorf("Router processed %d jobs, want %d", stats.JobsProcessed, totalJobs)
	}

	// Verify per-prefix counts
	prefixCounts := router.GetPerPrefixCounts()
	for i := 0; i < numShards; i++ {
		shardName := fmt.Sprintf("shard-%d", i)
		count := prefixCounts[shardName]
		if count != jobsPerShard {
			t.Errorf("Prefix count for %s = %d, want %d", shardName, count, jobsPerShard)
		}
	}
}

func TestPrefixRouter_MalformedKeys(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const numShards = 4
	const numJobs = 20

	// Create channels
	input := make(chan *Job, numJobs)
	outputChans := make(map[string]chan *Job)
	outputs := make(map[string]chan<- *Job)
	for i := 0; i < numShards; i++ {
		shardName := fmt.Sprintf("shard-%d", i)
		ch := make(chan *Job, numJobs)
		outputChans[shardName] = ch
		outputs[shardName] = ch
	}

	// Create router
	router := NewPrefixRouter(input, outputs)
	if err := router.Start(ctx); err != nil {
		t.Fatalf("Failed to start router: %v", err)
	}

	// Send jobs with malformed keys (should use fallback routing)
	go func() {
		for i := 0; i < numJobs; i++ {
			job := &Job{
				ID:    i,
				S3Key: fmt.Sprintf("uploads/test/chunk-%d.tar.zst", i), // No shard in key
				Chunk: chunking.Chunk{
					Files: []chunking.File{{Path: fmt.Sprintf("file-%d.txt", i)}},
				},
			}
			input <- job
		}
		close(input)
	}()

	// Collect all jobs
	var totalReceived int64
	var wg sync.WaitGroup
	for _, ch := range outputChans {
		wg.Add(1)
		go func(channel chan *Job) {
			defer wg.Done()
			for range channel {
				atomic.AddInt64(&totalReceived, 1)
			}
		}(ch)
	}

	// Stop router (which will close output channels when routing is done)
	if err := router.Stop(); err != nil {
		t.Errorf("Failed to stop router: %v", err)
	}

	// Wait for all consumers to finish
	wg.Wait()

	// Verify all jobs were routed via fallback
	if int(totalReceived) != numJobs {
		t.Errorf("Received %d jobs, want %d", totalReceived, numJobs)
	}

	// Verify routing errors were tracked
	routingErrors := router.GetRoutingErrors()
	if routingErrors != numJobs {
		t.Errorf("Routing errors = %d, want %d", routingErrors, numJobs)
	}
}

func TestPrefixRouter_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create channels
	input := make(chan *Job, 10)
	outputs := map[string]chan<- *Job{
		"shard-0": make(chan *Job, 10),
		"shard-1": make(chan *Job, 10),
	}

	// Create and start router
	router := NewPrefixRouter(input, outputs)
	if err := router.Start(ctx); err != nil {
		t.Fatalf("Failed to start router: %v", err)
	}

	// Send a few jobs
	for i := 0; i < 5; i++ {
		job := &Job{
			ID:    i,
			S3Key: fmt.Sprintf("uploads/test/shard-%d/chunk-%d.tar.zst", i%2, i),
		}
		input <- job
	}

	// Cancel context
	cancel()

	// Give router time to shut down
	time.Sleep(100 * time.Millisecond)

	// Verify router stopped
	if err := router.Stop(); err != nil {
		t.Errorf("Failed to stop router: %v", err)
	}
}

func TestPrefixRouter_EmptyOutputChannels(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Create channels
	input := make(chan *Job, 1)
	outputs := map[string]chan<- *Job{} // Empty outputs

	// Create router with empty outputs
	router := NewPrefixRouter(input, outputs)
	if err := router.Start(ctx); err != nil {
		t.Fatalf("Failed to start router: %v", err)
	}

	// Send a job
	job := &Job{
		ID:    1,
		S3Key: "uploads/test/shard-0/chunk-1.tar.zst",
	}
	input <- job
	close(input)

	// Give router time to process
	time.Sleep(100 * time.Millisecond)

	// Stop router - should not panic
	if err := router.Stop(); err != nil {
		t.Errorf("Failed to stop router: %v", err)
	}
}

func BenchmarkPrefixRouter_Routing(b *testing.B) {
	ctx := context.Background()

	const numShards = 8

	// Create channels
	input := make(chan *Job, 1000)
	outputChans := make(map[string]chan *Job)
	outputs := make(map[string]chan<- *Job)
	for i := 0; i < numShards; i++ {
		shardName := fmt.Sprintf("shard-%d", i)
		ch := make(chan *Job, 1000)
		outputChans[shardName] = ch
		outputs[shardName] = ch
	}

	// Create router
	router := NewPrefixRouter(input, outputs)
	if err := router.Start(ctx); err != nil {
		b.Fatalf("Failed to start router: %v", err)
	}

	// Drain output channels
	var wg sync.WaitGroup
	for _, ch := range outputChans {
		wg.Add(1)
		go func(channel chan *Job) {
			defer wg.Done()
			for range channel {
			}
		}(ch)
	}

	b.ResetTimer()

	// Send jobs
	go func() {
		for i := 0; i < b.N; i++ {
			shardID := i % numShards
			job := &Job{
				ID:    i,
				S3Key: fmt.Sprintf("uploads/test/shard-%d/chunk-%d.tar.zst", shardID, i),
			}
			input <- job
		}
		close(input)
	}()

	wg.Wait()
	b.StopTimer()

	if err := router.Stop(); err != nil {
		b.Errorf("Failed to stop router: %v", err)
	}
}
