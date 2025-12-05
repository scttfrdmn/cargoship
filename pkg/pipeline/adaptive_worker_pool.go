// Package pipeline provides streaming pipeline for CargoShip v0.5.0
package pipeline

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// AdaptiveWorkerPool manages a dynamically scaling worker pool based on throughput monitoring
// Inspired by MinIO mc's bandwidth-based scaling and s5cmd's user-controlled parallelism
type AdaptiveWorkerPool struct {
	// Configuration
	initialWorkers int           // Starting worker count (default: runtime.NumCPU())
	maxWorkers     int           // Maximum worker count (default: 256)
	minWorkers     int           // Minimum worker count (default: 2)
	monitorPeriod  time.Duration // Bandwidth monitoring interval (default: 4s)
	scalingFactor  int           // Workers to add per scaling cycle (default: runtime.GOMAXPROCS(0))

	// State
	workers      int32         // Current worker count (atomic)
	semaphore    chan struct{} // Semaphore for worker concurrency
	wg           sync.WaitGroup
	ctx          context.Context
	cancel       context.CancelFunc
	mu           sync.RWMutex

	// Throughput monitoring
	totalBytes     int64     // Total bytes processed (atomic)
	lastBytes      int64     // Bytes at last check
	lastCheck      time.Time // Last monitoring check time
	maxThroughput  int64     // Maximum throughput observed (bytes/sec)
	plateauRetries int       // Consecutive throughput plateaus

	// Adaptive scaling control
	enableAdaptive bool          // Enable adaptive scaling
	stopMonitor    chan struct{} // Signal to stop monitoring
	monitorWg      sync.WaitGroup
}

// AdaptiveWorkerPoolConfig configures the adaptive worker pool
type AdaptiveWorkerPoolConfig struct {
	InitialWorkers int           // Starting worker count (0 = runtime.NumCPU())
	MaxWorkers     int           // Maximum worker count (0 = 256)
	MinWorkers     int           // Minimum worker count (0 = 2)
	MonitorPeriod  time.Duration // Bandwidth monitoring interval (0 = 4s)
	ScalingFactor  int           // Workers to add per cycle (0 = runtime.GOMAXPROCS(0))
	EnableAdaptive bool          // Enable adaptive scaling (default: true)
}

// NewAdaptiveWorkerPool creates a new adaptive worker pool
func NewAdaptiveWorkerPool(ctx context.Context, config *AdaptiveWorkerPoolConfig) *AdaptiveWorkerPool {
	if config == nil {
		config = &AdaptiveWorkerPoolConfig{}
	}

	// Set defaults
	initialWorkers := config.InitialWorkers
	if initialWorkers <= 0 {
		initialWorkers = runtime.NumCPU()
	}

	maxWorkers := config.MaxWorkers
	if maxWorkers <= 0 {
		maxWorkers = 256 // Match s5cmd typical usage
	}

	minWorkers := config.MinWorkers
	if minWorkers <= 0 {
		minWorkers = 2
	}

	monitorPeriod := config.MonitorPeriod
	if monitorPeriod <= 0 {
		monitorPeriod = 4 * time.Second // Match mc's monitor period
	}

	scalingFactor := config.ScalingFactor
	if scalingFactor <= 0 {
		scalingFactor = runtime.GOMAXPROCS(0) // Match mc's scaling factor
	}

	enableAdaptive := config.EnableAdaptive
	// Default to true if not explicitly disabled
	if config.InitialWorkers == 0 && config.MaxWorkers == 0 {
		enableAdaptive = true
	}

	ctx, cancel := context.WithCancel(ctx)

	pool := &AdaptiveWorkerPool{
		initialWorkers: initialWorkers,
		maxWorkers:     maxWorkers,
		minWorkers:     minWorkers,
		monitorPeriod:  monitorPeriod,
		scalingFactor:  scalingFactor,
		workers:        int32(initialWorkers),
		semaphore:      make(chan struct{}, maxWorkers), // Pre-allocate for max workers
		ctx:            ctx,
		cancel:         cancel,
		lastCheck:      time.Now(),
		enableAdaptive: enableAdaptive,
		stopMonitor:    make(chan struct{}),
	}

	// Start adaptive scaling monitor if enabled
	if enableAdaptive {
		pool.startMonitor()
	}

	return pool
}

// Submit submits work to the pool (non-blocking, spawns goroutine immediately)
func (p *AdaptiveWorkerPool) Submit(fn func(context.Context) error) error {
	select {
	case <-p.ctx.Done():
		return p.ctx.Err()
	default:
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			_ = fn(p.ctx)
		}()
		return nil
	}
}

// AddBytes records bytes processed (for throughput monitoring)
func (p *AdaptiveWorkerPool) AddBytes(bytes int64) {
	atomic.AddInt64(&p.totalBytes, bytes)
}

// startMonitor starts the adaptive scaling monitor
func (p *AdaptiveWorkerPool) startMonitor() {
	p.monitorWg.Add(1)
	go p.monitorThroughput()
}

// monitorThroughput monitors throughput and scales workers accordingly
func (p *AdaptiveWorkerPool) monitorThroughput() {
	defer p.monitorWg.Done()

	ticker := time.NewTicker(p.monitorPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-p.stopMonitor:
			return
		case <-ticker.C:
			p.checkAndScale()
		}
	}
}

// checkAndScale checks throughput and scales workers if beneficial
func (p *AdaptiveWorkerPool) checkAndScale() {
	// Calculate current throughput
	currentBytes := atomic.LoadInt64(&p.totalBytes)
	elapsed := time.Since(p.lastCheck).Seconds()
	if elapsed == 0 {
		return
	}

	throughput := int64(float64(currentBytes-p.lastBytes) / elapsed)

	p.mu.Lock()
	defer p.mu.Unlock()

	// Check if throughput improved
	if throughput <= p.maxThroughput {
		p.plateauRetries++
		// Stop scaling after 2 consecutive plateaus (match mc behavior)
		if p.plateauRetries > 2 {
			// Throughput plateaued, stop adaptive scaling
			close(p.stopMonitor)
			return
		}
	} else {
		// Throughput improved, reset retry counter
		p.plateauRetries = 0
		p.maxThroughput = throughput
	}

	// Add more workers
	currentWorkers := atomic.LoadInt32(&p.workers)
	if int(currentWorkers) >= p.maxWorkers {
		// Already at maximum, stop monitoring
		close(p.stopMonitor)
		return
	}

	// Add scalingFactor workers (default: GOMAXPROCS)
	workersToAdd := p.scalingFactor
	newWorkerCount := int(currentWorkers) + workersToAdd
	if newWorkerCount > p.maxWorkers {
		workersToAdd = p.maxWorkers - int(currentWorkers)
	}

	for i := 0; i < workersToAdd; i++ {
		p.addWorker()
	}

	// Update state for next check
	p.lastBytes = currentBytes
	p.lastCheck = time.Now()
}

// addWorker adds a new worker to the pool (increases parallelism limit)
func (p *AdaptiveWorkerPool) addWorker() {
	currentWorkers := atomic.LoadInt32(&p.workers)
	if int(currentWorkers) >= p.maxWorkers {
		return
	}

	// Atomically increment worker count (increases parallelism)
	atomic.AddInt32(&p.workers, 1)
}

// Wait waits for all workers to complete
func (p *AdaptiveWorkerPool) Wait() {
	p.wg.Wait()
}

// Stop stops the worker pool and monitoring
func (p *AdaptiveWorkerPool) Stop() {
	p.cancel()

	// Stop monitoring if still running
	select {
	case <-p.stopMonitor:
		// Already stopped
	default:
		close(p.stopMonitor)
	}

	p.monitorWg.Wait() // Wait for monitor to stop
	p.wg.Wait()        // Wait for all workers to finish
}

// GetWorkerCount returns the current number of workers
func (p *AdaptiveWorkerPool) GetWorkerCount() int {
	return int(atomic.LoadInt32(&p.workers))
}

// GetTotalBytes returns total bytes processed
func (p *AdaptiveWorkerPool) GetTotalBytes() int64 {
	return atomic.LoadInt64(&p.totalBytes)
}

// GetThroughput returns current throughput in bytes/sec
func (p *AdaptiveWorkerPool) GetThroughput() int64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.maxThroughput
}
