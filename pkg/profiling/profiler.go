// Package profiling provides integrated profiling capabilities for CargoShip
// performance analysis and optimization.
package profiling

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"time"
)

// ProfileType represents the type of profile to collect
type ProfileType string

const (
	// ProfileCPU collects CPU profiling data
	ProfileCPU ProfileType = "cpu"

	// ProfileMemory collects heap memory profiling data
	ProfileMemory ProfileType = "memory"

	// ProfileGoroutine collects goroutine profiling data
	ProfileGoroutine ProfileType = "goroutine"

	// ProfileBlock collects blocking profiling data
	ProfileBlock ProfileType = "block"

	// ProfileMutex collects mutex contention profiling data
	ProfileMutex ProfileType = "mutex"

	// ProfileTrace collects execution trace data
	ProfileTrace ProfileType = "trace"
)

// Profiler orchestrates profiling operations
type Profiler struct {
	// Output directory for profile files
	outputDir string

	// Active profiles
	cpuFile   *os.File
	traceFile *os.File

	// Start time
	startTime time.Time

	// Profile types being collected
	activeProfiles []ProfileType
}

// Config configures the profiler
type Config struct {
	// OutputDir where profile files will be written
	OutputDir string

	// Profiles to collect
	Profiles []ProfileType

	// BlockRate for block profiling (default: 1)
	BlockRate int

	// MutexFraction for mutex profiling (default: 1)
	MutexFraction int
}

// New creates a new profiler
func New(config Config) (*Profiler, error) {
	// Create output directory if it doesn't exist
	if err := os.MkdirAll(config.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	return &Profiler{
		outputDir:      config.OutputDir,
		activeProfiles: config.Profiles,
	}, nil
}

// Start begins profiling
func (p *Profiler) Start() error {
	p.startTime = time.Now()

	for _, profileType := range p.activeProfiles {
		if err := p.startProfile(profileType); err != nil {
			// Clean up any started profiles
			_ = p.Stop()
			return fmt.Errorf("failed to start %s profile: %w", profileType, err)
		}
	}

	return nil
}

// Stop stops profiling and writes results
func (p *Profiler) Stop() error {
	var firstErr error

	for _, profileType := range p.activeProfiles {
		if err := p.stopProfile(profileType); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

// startProfile starts a specific profile type
func (p *Profiler) startProfile(profileType ProfileType) error {
	switch profileType {
	case ProfileCPU:
		return p.startCPUProfile()
	case ProfileTrace:
		return p.startTrace()
	case ProfileBlock:
		runtime.SetBlockProfileRate(1)
		return nil
	case ProfileMutex:
		runtime.SetMutexProfileFraction(1)
		return nil
	case ProfileMemory, ProfileGoroutine:
		// These are captured at stop time
		return nil
	default:
		return fmt.Errorf("unknown profile type: %s", profileType)
	}
}

// stopProfile stops a specific profile type
func (p *Profiler) stopProfile(profileType ProfileType) error {
	switch profileType {
	case ProfileCPU:
		return p.stopCPUProfile()
	case ProfileMemory:
		return p.writeMemoryProfile()
	case ProfileGoroutine:
		return p.writeGoroutineProfile()
	case ProfileBlock:
		defer runtime.SetBlockProfileRate(0)
		return p.writeBlockProfile()
	case ProfileMutex:
		defer runtime.SetMutexProfileFraction(0)
		return p.writeMutexProfile()
	case ProfileTrace:
		return p.stopTrace()
	default:
		return fmt.Errorf("unknown profile type: %s", profileType)
	}
}

// startCPUProfile starts CPU profiling
func (p *Profiler) startCPUProfile() error {
	filename := filepath.Join(p.outputDir, fmt.Sprintf("cpu-%s.prof", timestamp()))
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create CPU profile: %w", err)
	}

	if err := pprof.StartCPUProfile(f); err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to start CPU profile: %w", err)
	}

	p.cpuFile = f
	return nil
}

// stopCPUProfile stops CPU profiling
func (p *Profiler) stopCPUProfile() error {
	if p.cpuFile == nil {
		return nil
	}

	pprof.StopCPUProfile()
	if err := p.cpuFile.Close(); err != nil {
		return fmt.Errorf("failed to close CPU profile: %w", err)
	}

	p.cpuFile = nil
	return nil
}

// writeMemoryProfile writes heap memory profile
func (p *Profiler) writeMemoryProfile() error {
	filename := filepath.Join(p.outputDir, fmt.Sprintf("memory-%s.prof", timestamp()))
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create memory profile: %w", err)
	}
	defer func() { _ = f.Close() }()

	runtime.GC() // Get up-to-date statistics
	if err := pprof.WriteHeapProfile(f); err != nil {
		return fmt.Errorf("failed to write memory profile: %w", err)
	}

	return nil
}

// writeGoroutineProfile writes goroutine profile
func (p *Profiler) writeGoroutineProfile() error {
	filename := filepath.Join(p.outputDir, fmt.Sprintf("goroutine-%s.prof", timestamp()))
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create goroutine profile: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := pprof.Lookup("goroutine").WriteTo(f, 0); err != nil {
		return fmt.Errorf("failed to write goroutine profile: %w", err)
	}

	return nil
}

// writeBlockProfile writes block profile
func (p *Profiler) writeBlockProfile() error {
	filename := filepath.Join(p.outputDir, fmt.Sprintf("block-%s.prof", timestamp()))
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create block profile: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := pprof.Lookup("block").WriteTo(f, 0); err != nil {
		return fmt.Errorf("failed to write block profile: %w", err)
	}

	return nil
}

// writeMutexProfile writes mutex profile
func (p *Profiler) writeMutexProfile() error {
	filename := filepath.Join(p.outputDir, fmt.Sprintf("mutex-%s.prof", timestamp()))
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create mutex profile: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := pprof.Lookup("mutex").WriteTo(f, 0); err != nil {
		return fmt.Errorf("failed to write mutex profile: %w", err)
	}

	return nil
}

// startTrace starts execution tracing
func (p *Profiler) startTrace() error {
	filename := filepath.Join(p.outputDir, fmt.Sprintf("trace-%s.out", timestamp()))
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create trace file: %w", err)
	}

	if err := trace.Start(f); err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to start trace: %w", err)
	}

	p.traceFile = f
	return nil
}

// stopTrace stops execution tracing
func (p *Profiler) stopTrace() error {
	if p.traceFile == nil {
		return nil
	}

	trace.Stop()
	if err := p.traceFile.Close(); err != nil {
		return fmt.Errorf("failed to close trace file: %w", err)
	}

	p.traceFile = nil
	return nil
}

// timestamp returns a formatted timestamp for file naming
func timestamp() string {
	return time.Now().Format("20060102-150405")
}

// ProfileResults contains the results of profiling
type ProfileResults struct {
	// OutputDir where profiles were written
	OutputDir string

	// Duration of profiling session
	Duration time.Duration

	// Profiles that were collected
	Profiles []ProfileInfo

	// MemoryStats final memory statistics
	MemoryStats runtime.MemStats
}

// ProfileInfo contains information about a single profile
type ProfileInfo struct {
	// Type of profile
	Type ProfileType

	// Filename where profile was written
	Filename string

	// Size in bytes
	Size int64
}

// GetResults returns profiling results
func (p *Profiler) GetResults() (*ProfileResults, error) {
	duration := time.Since(p.startTime)

	results := &ProfileResults{
		OutputDir: p.outputDir,
		Duration:  duration,
		Profiles:  make([]ProfileInfo, 0),
	}

	// Get memory stats
	runtime.ReadMemStats(&results.MemoryStats)

	// List profile files
	entries, err := os.ReadDir(p.outputDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read output directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Determine profile type from filename
		profileType := determineProfileType(entry.Name())

		results.Profiles = append(results.Profiles, ProfileInfo{
			Type:     profileType,
			Filename: filepath.Join(p.outputDir, entry.Name()),
			Size:     info.Size(),
		})
	}

	return results, nil
}

// determineProfileType determines profile type from filename
func determineProfileType(filename string) ProfileType {
	switch {
	case containsString(filename, "cpu"):
		return ProfileCPU
	case containsString(filename, "memory") || containsString(filename, "heap"):
		return ProfileMemory
	case containsString(filename, "goroutine"):
		return ProfileGoroutine
	case containsString(filename, "block"):
		return ProfileBlock
	case containsString(filename, "mutex"):
		return ProfileMutex
	case containsString(filename, "trace"):
		return ProfileTrace
	default:
		return ProfileType("unknown")
	}
}

// containsString checks if s contains substr
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr ||
		len(s) > len(substr) && containsStringHelper(s[1:], substr)
}

func containsStringHelper(s, substr string) bool {
	if len(s) < len(substr) {
		return false
	}
	if s[:len(substr)] == substr {
		return true
	}
	return containsStringHelper(s[1:], substr)
}

// DefaultConfig returns a sensible default configuration
func DefaultConfig() Config {
	return Config{
		OutputDir: "profiles",
		Profiles: []ProfileType{
			ProfileCPU,
			ProfileMemory,
			ProfileGoroutine,
		},
		BlockRate:     1,
		MutexFraction: 1,
	}
}

// AllProfilesConfig returns config for all profile types
func AllProfilesConfig() Config {
	return Config{
		OutputDir: "profiles",
		Profiles: []ProfileType{
			ProfileCPU,
			ProfileMemory,
			ProfileGoroutine,
			ProfileBlock,
			ProfileMutex,
			ProfileTrace,
		},
		BlockRate:     1,
		MutexFraction: 1,
	}
}
