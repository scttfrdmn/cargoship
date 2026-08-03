package launch

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cargoshipconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
)

// #347 deleted pkg/launch's dead Agent/LocalArchiver/FileWatcher/AstrapiLauncher
// surface, which took the package's only test file with it. GhostShip is what
// actually runs — cmd/ghost-ship builds nothing else — and had no tests at all.
//
// Autonomous archival is deferred, not retired, so this is the surface a future
// revival builds on and it should be the tested one. These cover file selection,
// the part that decides what gets uploaded and deleted: an over-broad match
// archives data the operator never nominated, and DeleteAfterArchive makes a
// false positive destructive.

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

// validGhostShipConfig returns the minimum config NewGhostShip accepts.
func validGhostShipConfig(watchDir string) *GhostShipConfig {
	return &GhostShipConfig{
		ID:       "test-ghost",
		Name:     "Test Ghost Ship",
		S3Config: cargoshipconfig.S3Config{Bucket: "test-bucket"},
		WatchPaths: []WatchPath{
			{Path: watchDir, Recursive: true},
		},
		ArchivalRules: []ArchivalRule{
			{Name: "everything", FilePattern: "*", Enabled: true},
		},
	}
}

func TestValidateGhostShipConfig(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*GhostShipConfig)
		wantErr string
	}{
		{"valid", func(*GhostShipConfig) {}, ""},
		{"missing ID", func(c *GhostShipConfig) { c.ID = "" }, "ID cannot be empty"},
		{"missing bucket", func(c *GhostShipConfig) { c.S3Config.Bucket = "" }, "S3 bucket must be configured"},
		{"no watch paths", func(c *GhostShipConfig) { c.WatchPaths = nil }, "at least one watch path"},
		{"no archival rules", func(c *GhostShipConfig) { c.ArchivalRules = nil }, "at least one archival rule"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := validGhostShipConfig(t.TempDir())
			tt.mutate(config)

			err := validateGhostShipConfig(config)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)

			// A valid config must come back with every zero-valued tunable
			// defaulted — a zero ScanInterval would busy-loop the scanner.
			assert.Equal(t, 5*time.Minute, config.ScanInterval)
			assert.Equal(t, 5, config.MaxConcurrentJobs)
			assert.Equal(t, 3, config.WorkerPoolSize)
			assert.Equal(t, time.Minute, config.ReportInterval)
		})
	}
}

func TestValidateGhostShipConfigKeepsExplicitValues(t *testing.T) {
	config := validGhostShipConfig(t.TempDir())
	config.ScanInterval = 90 * time.Second
	config.MaxConcurrentJobs = 11
	config.WorkerPoolSize = 7
	config.ReportInterval = 30 * time.Second

	require.NoError(t, validateGhostShipConfig(config))

	assert.Equal(t, 90*time.Second, config.ScanInterval, "defaulting must not overwrite an explicit value")
	assert.Equal(t, 11, config.MaxConcurrentJobs)
	assert.Equal(t, 7, config.WorkerPoolSize)
	assert.Equal(t, 30*time.Second, config.ReportInterval)
}

func TestNewGhostShipRejectsBadConfig(t *testing.T) {
	gs, err := NewGhostShip(nil, testLogger())
	require.Error(t, err)
	assert.Nil(t, gs)
	assert.Contains(t, err.Error(), "cannot be nil")

	gs, err = NewGhostShip(&GhostShipConfig{}, testLogger())
	require.Error(t, err)
	assert.Nil(t, gs)
}

func TestExpandBracePattern(t *testing.T) {
	gs := &GhostShip{}

	tests := []struct {
		name    string
		pattern string
		want    []string
	}{
		{"no braces", "*.fasta", []string{"*.fasta"}},
		{"expands", "*.{fasta,vcf,fastq}", []string{"*.fasta", "*.vcf", "*.fastq"}},
		{"trims spaces", "*.{fasta, vcf}", []string{"*.fasta", "*.vcf"}},
		{"single option", "*.{gz}", []string{"*.gz"}},
		// Malformed patterns must degrade to a literal, not panic or produce
		// a pattern that matches more than the operator asked for.
		{"unclosed brace", "*.{fasta", []string{"*.{fasta"}},
		{"reversed braces", "*.}fasta{", []string{"*.}fasta{"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, gs.expandBracePattern(tt.pattern))
		})
	}
}

func TestFileMatchesRule(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.fasta")
	require.NoError(t, os.WriteFile(path, make([]byte, 2048), 0o600))
	info, err := os.Stat(path)
	require.NoError(t, err)

	gs := &GhostShip{logger: testLogger()}

	tests := []struct {
		name string
		rule ArchivalRule
		want bool
	}{
		{"empty rule matches", ArchivalRule{}, true},
		{"pattern matches", ArchivalRule{FilePattern: "*.fasta"}, true},
		{"pattern does not match", ArchivalRule{FilePattern: "*.vcf"}, false},
		{"brace pattern matches one alternative", ArchivalRule{FilePattern: "*.{vcf,fasta}"}, true},
		{"brace pattern matches none", ArchivalRule{FilePattern: "*.{vcf,bam}"}, false},
		{"above min size", ArchivalRule{MinSize: 1024}, true},
		{"below min size", ArchivalRule{MinSize: 4096}, false},
		{"under max size", ArchivalRule{MaxSize: 4096}, true},
		{"over max size", ArchivalRule{MaxSize: 1024}, false},
		// The file was just written, so it is younger than any positive MinAge.
		// This is the guard that stops a ghost ship archiving a file still
		// being written to.
		{"younger than min age", ArchivalRule{MinAge: time.Hour}, false},
		{"older than max age", ArchivalRule{MaxAge: time.Nanosecond}, false},
		{"within max age", ArchivalRule{MaxAge: time.Hour}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, gs.fileMatchesRule(path, tt.rule, info))
		})
	}
}

func TestFindArchivalCandidatesSkipsDirectoriesAndNonMatches(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "keep.fasta"), []byte("data"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "nested", "deep.fasta"), []byte("data"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "skip.txt"), []byte("data"), 0o600))

	gs := &GhostShip{
		logger: testLogger(),
		config: &GhostShipConfig{WatchPaths: []WatchPath{{Path: dir}}},
	}

	candidates, err := gs.findArchivalCandidates(ArchivalRule{Name: "fasta", FilePattern: "*.fasta"})
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{
		filepath.Join(dir, "keep.fasta"),
		filepath.Join(dir, "nested", "deep.fasta"),
	}, candidates, "the walk must recurse and must not return the directory itself")
}

func TestFindArchivalCandidatesToleratesMissingWatchPath(t *testing.T) {
	gs := &GhostShip{
		logger: testLogger(),
		config: &GhostShipConfig{WatchPaths: []WatchPath{{Path: filepath.Join(t.TempDir(), "absent")}}},
	}

	// A watch path that does not exist must not abort the scan — a NAS share can
	// be unmounted at any time, and a ghost ship has to keep running.
	candidates, err := gs.findArchivalCandidates(ArchivalRule{Name: "any", FilePattern: "*"})
	require.NoError(t, err)
	assert.Empty(t, candidates)
}

func TestGenerateS3KeyIsDatePartitioned(t *testing.T) {
	gs := &GhostShip{id: "ghost-01"}

	key := gs.generateS3Key("/volume1/data/reads.fastq", "genomics")

	now := time.Now()
	want := filepath.Join("ghost-01", "genomics",
		now.Format("2006"), now.Format("01"), now.Format("02"), "reads.fastq")
	assert.Equal(t, want, key)
}

func TestCreateArchivalJob(t *testing.T) {
	gs := &GhostShip{id: "ghost-01"}
	rule := ArchivalRule{Name: "docs", Destination: "s3://bucket/docs"}

	job := gs.createArchivalJob("/data/report.pdf", rule)

	assert.Equal(t, "ghost-01", job.GhostShipID)
	assert.Equal(t, "docs", job.RuleName)
	assert.Equal(t, "/data/report.pdf", job.SourcePath)
	assert.Equal(t, "s3://bucket/docs", job.Destination)
	assert.Equal(t, JobStatePending, job.State, "a new job must start pending or the processor will skip it")
	assert.NotEmpty(t, job.ID)
	assert.False(t, job.CreatedAt.IsZero())
}

func TestGetStatusReportsDerivedCounts(t *testing.T) {
	gs := &GhostShip{
		config: &GhostShipConfig{
			WatchPaths: []WatchPath{{Path: "/a"}, {Path: "/b"}},
			ArchivalRules: []ArchivalRule{
				{Name: "on", Enabled: true},
				{Name: "off", Enabled: false},
				{Name: "also-on", Enabled: true},
			},
		},
		activeJobs: map[string]*ArchivalJob{
			"j1": {ID: "j1", State: JobStatePending},
			"j2": {ID: "j2", State: JobStateRunning},
		},
		status: GhostShipStatus{State: GhostShipStateRunning, StartTime: time.Now().Add(-time.Minute)},
	}

	status := gs.GetStatus()

	assert.Equal(t, GhostShipStateRunning, status.State)
	assert.Equal(t, 2, status.ActiveJobs)
	assert.Equal(t, 2, status.WatchedPaths)
	assert.Equal(t, 2, status.ActiveRules, "disabled rules must not be counted as active")
	assert.Positive(t, status.Uptime)
}

func TestCountActiveRules(t *testing.T) {
	assert.Equal(t, 0, countActiveRules(nil))
	assert.Equal(t, 0, countActiveRules([]ArchivalRule{{Enabled: false}}))
	assert.Equal(t, 2, countActiveRules([]ArchivalRule{
		{Enabled: true}, {Enabled: false}, {Enabled: true},
	}))
}

func TestWatchPathMatchesRule(t *testing.T) {
	gs := &GhostShip{}

	// An empty PathPattern means the rule applies to every watch path.
	assert.True(t, gs.watchPathMatchesRule(WatchPath{Path: "/volume1/data"}, ArchivalRule{}))

	// A pattern that cannot match must exclude the path rather than defaulting
	// to true — otherwise a scoped rule silently becomes global.
	assert.False(t, gs.watchPathMatchesRule(
		WatchPath{Path: "/volume1/data"},
		ArchivalRule{PathPattern: "/other/*"},
	))
}
