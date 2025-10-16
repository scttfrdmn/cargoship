# CargoShip Performance Profiling Guide

This guide covers how to profile CargoShip, analyze performance bottlenecks, and interpret results.

## Table of Contents

- [Quick Start](#quick-start)
- [Running Benchmarks](#running-benchmarks)
- [Profiling Types](#profiling-types)
- [Analyzing Profiles](#analyzing-profiles)
- [Regression Detection](#regression-detection)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)

## Quick Start

### Run All Benchmarks

```bash
# Run comprehensive benchmark suite
./scripts/run-benchmarks.sh

# Run short benchmarks (faster)
./scripts/run-benchmarks.sh --short

# Run long benchmarks (more accurate)
./scripts/run-benchmarks.sh --long
```

### Analyze Specific Profile

```bash
# View CPU profile interactively
go tool pprof profiles/cpu-20251016-143022.prof

# View memory profile
go tool pprof profiles/mem-20251016-143022.prof

# View execution trace
go tool trace profiles/trace-20251016-143022.out
```

## Running Benchmarks

### Benchmark Suite

CargoShip includes a comprehensive benchmark suite covering all file sizes and optimization scenarios:

**Small Files (1-10MB)**
- Focus: Latency and request overhead
- Scenarios: 1MB, 5MB, 10MB
- Key metrics: Latency percentiles, request latency

**Medium Files (10-100MB)**
- Focus: Multipart efficiency with low concurrency
- Scenarios: 10MB (2 concurrent), 50MB (2 concurrent), 100MB (4 concurrent)
- Key metrics: Throughput, multipart performance

**Large Files (100MB-1GB)**
- Focus: Throughput with high concurrency
- Scenarios: 100MB (4 concurrent), 500MB (8 concurrent), 1GB (8 concurrent)
- Key metrics: Sustained throughput, concurrency scaling

**XL Files (1GB+)**
- Focus: Memory efficiency and sustained throughput
- Scenarios: 2GB (10 concurrent), 5GB (10 concurrent)
- Key metrics: Memory usage, sustained throughput

### Running Specific Scenarios

```bash
# Run only small file benchmarks
go test -bench=BenchmarkSmallFileUpload ./pkg/benchmarks/scenarios

# Run only large file benchmarks
go test -bench=BenchmarkLargeFileUpload ./pkg/benchmarks/scenarios

# Run with profiling
go test -bench=BenchmarkMediumFileUpload \
  -cpuprofile=cpu.prof \
  -memprofile=mem.prof \
  ./pkg/benchmarks/scenarios
```

### Benchmark Parameters

Control benchmark behavior with environment variables:

```bash
# Set benchmark duration (default: 3s)
export BENCH_TIME=10s

# Set iteration count (default: 5)
export BENCH_COUNT=10

# Set S3 bucket
export BENCHMARK_BUCKET=my-benchmark-bucket

# Set AWS region
export AWS_REGION=us-east-1

# Run benchmarks
./scripts/run-benchmarks.sh
```

## Profiling Types

### CPU Profiling

**What it measures**: Where your program spends CPU time

**When to use**:
- Identifying hot paths and performance bottlenecks
- Optimizing compute-intensive operations
- Understanding function call overhead

**How to collect**:
```bash
go test -bench=. -cpuprofile=cpu.prof ./pkg/benchmarks/scenarios
```

**How to analyze**:
```bash
# Interactive mode
go tool pprof cpu.prof

# Common pprof commands:
# - top      : Show top CPU consumers
# - list     : Show annotated source code
# - web      : Generate call graph (requires graphviz)
# - pdf      : Generate PDF call graph
# - flames   : Generate flame graph

# Top 20 functions by CPU time
go tool pprof -top -nodecount=20 cpu.prof

# Generate flame graph
go tool pprof -http=:8080 cpu.prof
```

### Memory Profiling

**What it measures**: Heap allocations and memory usage

**When to use**:
- Investigating memory leaks
- Reducing GC pressure
- Optimizing allocation patterns

**How to collect**:
```bash
go test -bench=. -memprofile=mem.prof ./pkg/benchmarks/scenarios
```

**How to analyze**:
```bash
# Interactive mode
go tool pprof mem.prof

# Top allocations by bytes
go tool pprof -top -alloc_space mem.prof

# Top allocations by count
go tool pprof -top -alloc_objects mem.prof

# Show allocations in specific function
go tool pprof -list=FunctionName mem.prof

# Generate visual call graph
go tool pprof -http=:8080 mem.prof
```

### Goroutine Profiling

**What it measures**: Active goroutines and their stack traces

**When to use**:
- Investigating goroutine leaks
- Understanding concurrency patterns
- Debugging deadlocks

**How to collect**:
```go
// In your code
import "runtime/pprof"

f, _ := os.Create("goroutine.prof")
defer f.Close()
pprof.Lookup("goroutine").WriteTo(f, 0)
```

**How to analyze**:
```bash
go tool pprof goroutine.prof
```

### Block Profiling

**What it measures**: Time spent blocked on synchronization primitives

**When to use**:
- Identifying contention bottlenecks
- Optimizing lock usage
- Debugging blocking operations

**How to collect**:
```go
import "runtime"

// Enable block profiling
runtime.SetBlockProfileRate(1)

// Run your code...

// Save profile
f, _ := os.Create("block.prof")
defer f.Close()
pprof.Lookup("block").WriteTo(f, 0)
```

### Mutex Profiling

**What it measures**: Contention on mutexes

**When to use**:
- Optimizing lock contention
- Identifying synchronization bottlenecks
- Improving concurrent performance

**How to collect**:
```go
import "runtime"

// Enable mutex profiling
runtime.SetMutexProfileFraction(1)

// Run your code...

// Save profile
f, _ := os.Create("mutex.prof")
defer f.Close()
pprof.Lookup("mutex").WriteTo(f, 0)
```

### Execution Tracing

**What it measures**: Detailed execution timeline including goroutine scheduling, syscalls, and GC events

**When to use**:
- Understanding goroutine scheduling
- Debugging concurrency issues
- Analyzing GC behavior

**How to collect**:
```bash
go test -bench=. -trace=trace.out ./pkg/benchmarks/scenarios
```

**How to analyze**:
```bash
# Open trace viewer (opens in browser)
go tool trace trace.out

# Trace viewer features:
# - View: Goroutine analysis
# - View: Network blocking profile
# - View: Synchronization blocking profile
# - View: Syscall blocking profile
# - View: Scheduler latency profile
```

## Analyzing Profiles

### CPU Profile Analysis

**1. Identify Hot Paths**

```bash
# Show top 10 CPU consumers
go tool pprof -top -nodecount=10 cpu.prof
```

Look for:
- Functions consuming >10% of CPU time
- Unexpected functions in top list
- Recursive calls with high cumulative time

**2. Analyze Call Chains**

```bash
# Show call graph
go tool pprof -http=:8080 cpu.prof
```

In the web interface:
- Use "View > Top" to see hottest functions
- Use "View > Flame Graph" to visualize call stacks
- Click on functions to see callers and callees

**3. Source Code Inspection**

```bash
# In interactive mode
(pprof) list FunctionName
```

Shows annotated source with CPU time per line.

### Memory Profile Analysis

**1. Find Allocation Hotspots**

```bash
# Top 20 allocators by bytes
go tool pprof -top -alloc_space -nodecount=20 mem.prof
```

Look for:
- Large allocations (>1MB)
- High allocation counts (>1000/op)
- Unexpected allocations in hot paths

**2. Analyze Allocation Patterns**

```bash
# Show allocations in context
go tool pprof -http=:8080 mem.prof
```

Use the web interface to:
- Identify allocation chains
- Find sources of GC pressure
- Locate memory leaks

**3. Compare Allocations**

```bash
# Compare two memory profiles
go tool pprof -base=baseline.prof current.prof
```

Shows delta between profiles - useful for regression detection.

### Trace Analysis

**1. Goroutine Analysis**

In the trace viewer:
1. Click "View: Goroutine analysis"
2. Look for:
   - Long-running goroutines
   - Goroutines with high execution time
   - Blocked goroutines

**2. Network Analysis**

1. Click "View: Network blocking profile"
2. Identify:
   - Network I/O bottlenecks
   - Slow operations
   - Connection pool exhaustion

**3. GC Analysis**

Look at the trace timeline:
- GC pause frequency
- GC pause duration
- Goroutine activity during GC

## Regression Detection

### Establishing Baselines

Before making optimizations, establish performance baselines:

```bash
# Run benchmarks and save results
./scripts/run-benchmarks.sh --long > baseline-v0.4.5.txt

# The results are automatically saved to:
# - benchmark-reports/benchmark-TIMESTAMP.txt
# - profiles/cpu-TIMESTAMP.prof
# - profiles/mem-TIMESTAMP.prof
```

### Automated Regression Detection

CargoShip includes a regression detection system:

```go
// Load baseline
baseline, err := benchmarks.LoadBaseline("pkg/benchmarks/baseline.json")

// Create detector with thresholds
detector := benchmarks.NewRegressionDetector(baseline, benchmarks.Thresholds{
    ThroughputDelta: -0.05,  // -5% throughput loss acceptable
    LatencyDelta:    0.10,   // +10% latency increase acceptable
    MemoryDelta:     0.15,   // +15% memory increase acceptable
    AllocationDelta: 0.20,   // +20% allocation increase acceptable
})

// Check current results
report, err := detector.Check(results)

// Print regressions
if len(report.Regressions) > 0 {
    fmt.Println(benchmarks.FormatRegressionReport(report))
}
```

### Comparing Results Manually

```bash
# Run benchmarks before changes
go test -bench=. -benchmem ./pkg/benchmarks/scenarios > before.txt

# Make your changes...

# Run benchmarks after changes
go test -bench=. -benchmem ./pkg/benchmarks/scenarios > after.txt

# Compare results
benchstat before.txt after.txt
```

Install benchstat:
```bash
go install golang.org/x/perf/cmd/benchstat@latest
```

## Best Practices

### Benchmark Design

1. **Isolate what you're measuring**
   - Profile one operation at a time
   - Avoid mixing different workloads
   - Use realistic data sizes

2. **Run sufficient iterations**
   - Use `-benchtime=10s` for stable results
   - Run multiple iterations with `-count=10`
   - Discard warmup iterations

3. **Control your environment**
   - Disable CPU frequency scaling
   - Close unnecessary applications
   - Use dedicated hardware for CI benchmarks

4. **Measure the right things**
   - Throughput for I/O operations
   - Latency for user-facing operations
   - Memory for long-running services
   - Allocations for GC-sensitive workloads

### Profiling Best Practices

1. **CPU Profiling**
   - Profile realistic workloads
   - Run long enough to capture steady state
   - Avoid profiling initialization code

2. **Memory Profiling**
   - Profile after warmup period
   - Look for cumulative allocations
   - Check for object retention

3. **Trace Analysis**
   - Keep traces short (< 1 second)
   - Focus on specific operations
   - Use trace for concurrency issues

### Optimization Workflow

1. **Establish baseline**
   ```bash
   ./scripts/run-benchmarks.sh --long
   ```

2. **Identify bottleneck**
   ```bash
   go tool pprof -http=:8080 profiles/cpu-TIMESTAMP.prof
   ```

3. **Make targeted changes**
   - Optimize one thing at a time
   - Use benchmark-driven development
   - Document assumptions

4. **Measure improvement**
   ```bash
   ./scripts/run-benchmarks.sh --long
   benchstat baseline.txt current.txt
   ```

5. **Verify no regressions**
   - Check all metrics, not just optimized one
   - Test edge cases
   - Validate in production-like environment

## Troubleshooting

### Benchmarks Taking Too Long

```bash
# Run shorter benchmarks
./scripts/run-benchmarks.sh --short

# Or run specific scenarios
go test -bench=BenchmarkSmallFileUpload ./pkg/benchmarks/scenarios
```

### AWS Credentials Not Working

```bash
# Check credentials
aws sts get-caller-identity

# Set profile
export AWS_PROFILE=your-profile

# Set region
export AWS_REGION=us-west-2
```

### S3 Bucket Access Denied

```bash
# Create bucket manually
aws s3 mb s3://cargoship-benchmark-test

# Or use different bucket
export BENCHMARK_BUCKET=my-bucket
./scripts/run-benchmarks.sh
```

### Profile Analysis Fails

```bash
# Ensure pprof is installed
go install github.com/google/pprof@latest

# Try text-based analysis
go tool pprof -text cpu.prof
```

### Trace Viewer Won't Open

```bash
# Try specifying port
go tool trace -http=localhost:8081 trace.out

# Or use trace tool commands
go tool trace trace.out
# Then click "View trace" in the list
```

## Example Session

Here's a complete profiling session:

```bash
# 1. Establish baseline
./scripts/run-benchmarks.sh --long
cp benchmark-reports/benchmark-*.txt baseline-v0.4.5.txt

# 2. Make optimization changes
# ... edit code ...

# 3. Run benchmarks again
./scripts/run-benchmarks.sh --long

# 4. Compare results
benchstat baseline-v0.4.5.txt benchmark-reports/benchmark-*.txt

# 5. Analyze CPU profile
go tool pprof -http=:8080 profiles/cpu-*.prof

# 6. Analyze memory profile
go tool pprof -http=:8081 profiles/mem-*.prof

# 7. View execution trace
go tool trace profiles/trace-*.out

# 8. Document findings and commit changes
```

## Additional Resources

- [Go Performance Tuning](https://github.com/golang/go/wiki/Performance)
- [Profiling Go Programs](https://go.dev/blog/pprof)
- [Execution Tracer](https://go.dev/doc/diagnostics#execution-tracer)
- [Benchstat Documentation](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat)

## See Also

- [Phase 1 Progress Summary](/tmp/phase1-progress-summary.md) - v0.5.0 Phase 1 status
- [v0.4.6 and v0.5.0 Plan](/tmp/v0.4.6-and-v0.5.0-detailed-plan.md) - Detailed roadmap
- [API Stability Guide](api-stability.md) - API guarantees
- [Versioning Guide](versioning.md) - Release process
