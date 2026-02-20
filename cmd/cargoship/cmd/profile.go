package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	runtimetrace "runtime/trace"
	"time"

	"github.com/spf13/cobra"
)

var (
	profileOutputDir string
	profileDuration  int
	profileMemory    bool
	profileCPU       bool
	profileGoroutine bool
	profileBlock     bool
	profileMutex     bool
	profileTrace     bool
	profileAllocs    bool
)

// NewProfileCmd creates a command for performance profiling operations
func NewProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Performance profiling and diagnostics tools",
		Long: `Performance profiling and diagnostics tools for CargoShip.

This command provides comprehensive profiling capabilities to help diagnose
performance issues and optimize CargoShip operations.

Available profile types:
  • CPU: CPU usage profiling (--cpu)
  • Memory: Memory allocation profiling (--memory)
  • Goroutine: Goroutine stack traces (--goroutine)
  • Block: Blocking operations profiling (--block)
  • Mutex: Mutex contention profiling (--mutex)
  • Trace: Execution trace for detailed analysis (--trace)
  • Allocs: Memory allocation profiling (--allocs)

Examples:
  # Capture CPU profile for 30 seconds
  cargoship profile collect --cpu --duration 30

  # Capture memory profile
  cargoship profile collect --memory

  # Capture all profiles
  cargoship profile collect --cpu --memory --goroutine --duration 60

  # List available profile files
  cargoship profile list

  # Show current runtime statistics
  cargoship profile stats`,
	}

	cmd.AddCommand(
		newProfileCollectCmd(),
		newProfileListCmd(),
		newProfileStatsCmd(),
	)

	return cmd
}

func newProfileCollectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "collect",
		Short: "Collect performance profiles",
		Long: `Collect various performance profiles for analysis.

By default, profiles are saved to a temporary directory. Use --output-dir
to specify a custom location.`,
		RunE: runProfileCollect,
	}

	cmd.Flags().StringVarP(&profileOutputDir, "output-dir", "o", "", "Output directory for profile files (default: temp dir)")
	cmd.Flags().IntVarP(&profileDuration, "duration", "d", 30, "Duration in seconds for CPU profiling")
	cmd.Flags().BoolVar(&profileCPU, "cpu", false, "Collect CPU profile")
	cmd.Flags().BoolVar(&profileMemory, "memory", false, "Collect memory profile")
	cmd.Flags().BoolVar(&profileGoroutine, "goroutine", false, "Collect goroutine profile")
	cmd.Flags().BoolVar(&profileBlock, "block", false, "Collect blocking operations profile")
	cmd.Flags().BoolVar(&profileMutex, "mutex", false, "Collect mutex contention profile")
	cmd.Flags().BoolVar(&profileTrace, "trace", false, "Collect execution trace")
	cmd.Flags().BoolVar(&profileAllocs, "allocs", false, "Collect allocation profile")

	return cmd
}

func newProfileListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available profile files",
		Long:  `List all profile files in the default profile directory.`,
		RunE:  runProfileList,
	}

	cmd.Flags().StringVarP(&profileOutputDir, "dir", "d", "", "Directory to list profiles from (default: temp dir)")

	return cmd
}

func newProfileStatsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show current runtime statistics",
		Long: `Show current runtime statistics including memory usage,
goroutines, and GC statistics.`,
		RunE: runProfileStats,
	}

	return cmd
}

func runProfileCollect(cmd *cobra.Command, args []string) error {
	slog.Info("Starting profile collection")

	// Determine output directory
	outDir := profileOutputDir
	if outDir == "" {
		var err error
		outDir, err = os.MkdirTemp("", "cargoship-profile-*")
		if err != nil {
			return fmt.Errorf("failed to create temp directory: %w", err)
		}
	}

	// Ensure directory exists with restricted permissions: profile data is
	// for the owner only (may contain memory layout / performance data).
	if err := os.MkdirAll(outDir, 0700); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	slog.Info("Profile output directory", "path", outDir)

	// Collect enabled profiles
	var collected []string

	// CPU profile
	if profileCPU {
		if err := collectCPUProfile(outDir, profileDuration); err != nil {
			slog.Error("Failed to collect CPU profile", "error", err)
		} else {
			collected = append(collected, "CPU")
		}
	}

	// Memory profile
	if profileMemory {
		if err := collectMemoryProfile(outDir); err != nil {
			slog.Error("Failed to collect memory profile", "error", err)
		} else {
			collected = append(collected, "Memory")
		}
	}

	// Goroutine profile
	if profileGoroutine {
		if err := collectGoroutineProfile(outDir); err != nil {
			slog.Error("Failed to collect goroutine profile", "error", err)
		} else {
			collected = append(collected, "Goroutine")
		}
	}

	// Block profile
	if profileBlock {
		if err := collectBlockProfile(outDir); err != nil {
			slog.Error("Failed to collect block profile", "error", err)
		} else {
			collected = append(collected, "Block")
		}
	}

	// Mutex profile
	if profileMutex {
		if err := collectMutexProfile(outDir); err != nil {
			slog.Error("Failed to collect mutex profile", "error", err)
		} else {
			collected = append(collected, "Mutex")
		}
	}

	// Execution trace
	if profileTrace {
		if err := collectExecutionTrace(outDir, profileDuration); err != nil {
			slog.Error("Failed to collect execution trace", "error", err)
		} else {
			collected = append(collected, "Trace")
		}
	}

	// Allocation profile
	if profileAllocs {
		if err := collectAllocProfile(outDir); err != nil {
			slog.Error("Failed to collect allocation profile", "error", err)
		} else {
			collected = append(collected, "Allocs")
		}
	}

	// Check if any profiles were collected
	if len(collected) == 0 {
		fmt.Println("No profile types selected. Use --cpu, --memory, --goroutine, etc.")
		fmt.Println("Run 'cargoship profile collect --help' for more information.")
		return nil
	}

	fmt.Println("\n╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║           Profile Collection Complete                       ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Printf("\nCollected profiles: %v\n", collected)
	fmt.Printf("Output directory: %s\n", outDir)
	fmt.Println("\nAnalyze with:")
	fmt.Printf("  go tool pprof %s/<profile-file>\n", outDir)
	fmt.Printf("  go tool trace %s/<trace-file>\n", outDir)
	fmt.Println()

	return nil
}

func collectCPUProfile(outDir string, duration int) error {
	filename := filepath.Join(outDir, fmt.Sprintf("cpu-%s.prof", time.Now().Format("20060102-150405")))

	slog.Info("Collecting CPU profile", "duration", duration, "file", filename)
	fmt.Printf("Collecting CPU profile for %d seconds...\n", duration)

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create CPU profile: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := pprof.StartCPUProfile(f); err != nil {
		return fmt.Errorf("failed to start CPU profile: %w", err)
	}

	time.Sleep(time.Duration(duration) * time.Second)

	pprof.StopCPUProfile()

	slog.Info("CPU profile collected", "file", filename)
	fmt.Printf("✅ CPU profile saved: %s\n", filename)
	return nil
}

func collectMemoryProfile(outDir string) error {
	filename := filepath.Join(outDir, fmt.Sprintf("memory-%s.prof", time.Now().Format("20060102-150405")))

	slog.Info("Collecting memory profile", "file", filename)
	fmt.Println("Collecting memory profile...")

	// Force GC to get accurate stats
	runtime.GC()

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create memory profile: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := pprof.WriteHeapProfile(f); err != nil {
		return fmt.Errorf("failed to write memory profile: %w", err)
	}

	slog.Info("Memory profile collected", "file", filename)
	fmt.Printf("✅ Memory profile saved: %s\n", filename)
	return nil
}

func collectGoroutineProfile(outDir string) error {
	filename := filepath.Join(outDir, fmt.Sprintf("goroutine-%s.prof", time.Now().Format("20060102-150405")))

	slog.Info("Collecting goroutine profile", "file", filename)
	fmt.Println("Collecting goroutine profile...")

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create goroutine profile: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := pprof.Lookup("goroutine").WriteTo(f, 0); err != nil {
		return fmt.Errorf("failed to write goroutine profile: %w", err)
	}

	slog.Info("Goroutine profile collected", "file", filename)
	fmt.Printf("✅ Goroutine profile saved: %s\n", filename)
	return nil
}

func collectBlockProfile(outDir string) error {
	filename := filepath.Join(outDir, fmt.Sprintf("block-%s.prof", time.Now().Format("20060102-150405")))

	slog.Info("Collecting block profile", "file", filename)
	fmt.Println("Collecting block profile...")

	// Enable block profiling
	runtime.SetBlockProfileRate(1)
	defer runtime.SetBlockProfileRate(0)

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create block profile: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := pprof.Lookup("block").WriteTo(f, 0); err != nil {
		return fmt.Errorf("failed to write block profile: %w", err)
	}

	slog.Info("Block profile collected", "file", filename)
	fmt.Printf("✅ Block profile saved: %s\n", filename)
	return nil
}

func collectMutexProfile(outDir string) error {
	filename := filepath.Join(outDir, fmt.Sprintf("mutex-%s.prof", time.Now().Format("20060102-150405")))

	slog.Info("Collecting mutex profile", "file", filename)
	fmt.Println("Collecting mutex profile...")

	// Enable mutex profiling
	runtime.SetMutexProfileFraction(1)
	defer runtime.SetMutexProfileFraction(0)

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create mutex profile: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := pprof.Lookup("mutex").WriteTo(f, 0); err != nil {
		return fmt.Errorf("failed to write mutex profile: %w", err)
	}

	slog.Info("Mutex profile collected", "file", filename)
	fmt.Printf("✅ Mutex profile saved: %s\n", filename)
	return nil
}

func collectExecutionTrace(outDir string, duration int) error {
	filename := filepath.Join(outDir, fmt.Sprintf("trace-%s.out", time.Now().Format("20060102-150405")))

	slog.Info("Collecting execution trace", "duration", duration, "file", filename)
	fmt.Printf("Collecting execution trace for %d seconds...\n", duration)

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create trace file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := runtimetrace.Start(f); err != nil {
		return fmt.Errorf("failed to start trace: %w", err)
	}

	time.Sleep(time.Duration(duration) * time.Second)

	runtimetrace.Stop()

	slog.Info("Execution trace collected", "file", filename)
	fmt.Printf("✅ Execution trace saved: %s\n", filename)
	return nil
}

func collectAllocProfile(outDir string) error {
	filename := filepath.Join(outDir, fmt.Sprintf("allocs-%s.prof", time.Now().Format("20060102-150405")))

	slog.Info("Collecting allocation profile", "file", filename)
	fmt.Println("Collecting allocation profile...")

	// Force GC to get accurate stats
	runtime.GC()

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create allocation profile: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := pprof.Lookup("allocs").WriteTo(f, 0); err != nil {
		return fmt.Errorf("failed to write allocation profile: %w", err)
	}

	slog.Info("Allocation profile collected", "file", filename)
	fmt.Printf("✅ Allocation profile saved: %s\n", filename)
	return nil
}

func runProfileList(cmd *cobra.Command, args []string) error {
	dir := profileOutputDir
	if dir == "" {
		dir = os.TempDir()
	}

	slog.Debug("Listing profile files", "directory", dir)

	// Find profile files
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	var profileFiles []os.DirEntry
	for _, entry := range entries {
		if !entry.IsDir() {
			name := entry.Name()
			if filepath.Ext(name) == ".prof" || filepath.Ext(name) == ".out" {
				profileFiles = append(profileFiles, entry)
			}
		}
	}

	if len(profileFiles) == 0 {
		fmt.Printf("No profile files found in: %s\n", dir)
		return nil
	}

	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║                 Available Profile Files                     ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Printf("\nDirectory: %s\n\n", dir)

	for _, entry := range profileFiles {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		fmt.Printf("  • %s (%s, modified: %s)\n",
			entry.Name(),
			formatSize(info.Size()),
			info.ModTime().Format("2006-01-02 15:04:05"))
	}

	fmt.Println()
	return nil
}

func runProfileStats(cmd *cobra.Command, args []string) error {
	slog.Debug("Collecting runtime statistics")

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║              Runtime Statistics                              ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Memory statistics
	fmt.Println("Memory Statistics:")
	fmt.Printf("  • Allocated:     %s\n", formatSize(int64(m.Alloc)))
	fmt.Printf("  • Total Alloc:   %s\n", formatSize(int64(m.TotalAlloc)))
	fmt.Printf("  • Sys:           %s\n", formatSize(int64(m.Sys)))
	fmt.Printf("  • Heap Alloc:    %s\n", formatSize(int64(m.HeapAlloc)))
	fmt.Printf("  • Heap Sys:      %s\n", formatSize(int64(m.HeapSys)))
	fmt.Printf("  • Heap Idle:     %s\n", formatSize(int64(m.HeapIdle)))
	fmt.Printf("  • Heap In Use:   %s\n", formatSize(int64(m.HeapInuse)))
	fmt.Printf("  • Heap Released: %s\n", formatSize(int64(m.HeapReleased)))
	fmt.Printf("  • Heap Objects:  %d\n", m.HeapObjects)
	fmt.Println()

	// GC statistics
	fmt.Println("Garbage Collection:")
	fmt.Printf("  • Num GC:        %d\n", m.NumGC)
	fmt.Printf("  • Pause Total:   %v\n", time.Duration(m.PauseTotalNs))
	fmt.Printf("  • Last Pause:    %v\n", time.Duration(m.PauseNs[(m.NumGC+255)%256]))
	fmt.Printf("  • GC CPU %%:      %.2f%%\n", m.GCCPUFraction*100)
	fmt.Println()

	// Goroutine statistics
	fmt.Println("Goroutines:")
	fmt.Printf("  • Active:        %d\n", runtime.NumGoroutine())
	fmt.Printf("  • Threads:       %d\n", runtime.GOMAXPROCS(0))
	fmt.Println()

	// System statistics
	fmt.Println("System:")
	fmt.Printf("  • NumCPU:        %d\n", runtime.NumCPU())
	fmt.Printf("  • GOMAXPROCS:    %d\n", runtime.GOMAXPROCS(0))
	fmt.Printf("  • Go Version:    %s\n", runtime.Version())
	fmt.Printf("  • OS/Arch:       %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Println()

	return nil
}

func formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
