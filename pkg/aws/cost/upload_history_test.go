package cost

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withUploadHistoryEnv points CARGOSHIP_UPLOAD_HISTORY at path for the test.
func withUploadHistoryEnv(t *testing.T, path string) {
	t.Helper()
	prev, had := os.LookupEnv(UploadHistoryEnv)
	require.NoError(t, os.Setenv(UploadHistoryEnv, path))
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(UploadHistoryEnv, prev)
		} else {
			_ = os.Unsetenv(UploadHistoryEnv)
		}
	})
}

func sampleOutcome(id string, ts time.Time) *UploadOutcome {
	return &UploadOutcome{
		UploadID:         id,
		ProjectID:        "proj",
		Timestamp:        ts,
		TotalBytes:       1024 * 1024 * 1024,
		FileCount:        42,
		FileTypeMix:      map[string]int{"txt": 40, "bin": 2},
		ChunkCount:       8,
		ShardCount:       4,
		CompressionType:  "zstd",
		CompressionLevel: 3,
		StorageClass:     "STANDARD",
		Region:           "us-west-2",
		CompressionRatio: 0.42,
		CompressedBytes:  450 * 1024 * 1024,
		Duration:         90 * time.Second,
		ThroughputMBps:   120.5,
		Success:          true,
		Cost:             1.23,
	}
}

// TestUploadHistory_DisabledByDefault verifies no store and no I/O when the
// opt-in is unset.
func TestUploadHistory_DisabledByDefault(t *testing.T) {
	prev, had := os.LookupEnv(UploadHistoryEnv)
	require.NoError(t, os.Unsetenv(UploadHistoryEnv))
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(UploadHistoryEnv, prev)
		}
	})

	store := NewUploadHistoryStore("")
	assert.False(t, store.Enabled())

	// Append and Load are no-ops when disabled.
	require.NoError(t, store.Append(sampleOutcome("u1", time.Now())))
	got, err := store.LoadOutcomes()
	require.NoError(t, err)
	assert.Empty(t, got)
}

// TestUploadHistory_OptInWritesNothingWhenOff is the issue's explicit
// "opt-in-off writes nothing" criterion: even a default-path store must not
// create a file when disabled.
func TestUploadHistory_OptInWritesNothingWhenOff(t *testing.T) {
	dir := t.TempDir()
	// Point XDG at a scratch dir so the "default path" can't touch a real home.
	t.Setenv("XDG_DATA_HOME", dir)
	prev, had := os.LookupEnv(UploadHistoryEnv)
	require.NoError(t, os.Unsetenv(UploadHistoryEnv))
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(UploadHistoryEnv, prev)
		}
	})

	store := NewUploadHistoryStore("") // config also empty => disabled
	require.NoError(t, store.Append(sampleOutcome("u1", time.Now())))

	_, err := os.Stat(filepath.Join(dir, "cargoship", "upload_history.json"))
	assert.True(t, os.IsNotExist(err), "no file should be written when opt-in is off")
}

// TestUploadHistory_RoundTrip verifies an appended outcome round-trips.
func TestUploadHistory_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upload_history.json")
	withUploadHistoryEnv(t, path)

	store := NewUploadHistoryStore("")
	require.True(t, store.Enabled())

	ts := time.Now().Truncate(time.Second)
	require.NoError(t, store.Append(sampleOutcome("u1", ts)))
	require.NoError(t, store.Append(sampleOutcome("u2", ts.Add(time.Minute))))

	// A fresh store (new process) reads the same file.
	reloaded := NewUploadHistoryStore("")
	got, err := reloaded.LoadOutcomes()
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "u1", got[0].UploadID, "records are oldest-first")
	assert.Equal(t, "u2", got[1].UploadID)
	assert.Equal(t, 0.42, got[1].CompressionRatio)
	assert.Equal(t, map[string]int{"txt": 40, "bin": 2}, got[1].FileTypeMix)
	assert.Equal(t, 90*time.Second, got[1].Duration)
}

// TestUploadHistory_ConfigLocation verifies the config field enables the store
// when the env var is unset.
func TestUploadHistory_ConfigLocation(t *testing.T) {
	prev, had := os.LookupEnv(UploadHistoryEnv)
	require.NoError(t, os.Unsetenv(UploadHistoryEnv))
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(UploadHistoryEnv, prev)
		}
	})
	path := filepath.Join(t.TempDir(), "history.json")

	store := NewUploadHistoryStore(path)
	require.True(t, store.Enabled())
	require.NoError(t, store.Append(sampleOutcome("c1", time.Now())))

	got, err := store.LoadOutcomes()
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "c1", got[0].UploadID)
}

// TestUploadHistory_EnvOverridesConfig verifies the env var wins over config.
func TestUploadHistory_EnvOverridesConfig(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), "env.json")
	cfgPath := filepath.Join(t.TempDir(), "cfg.json")
	withUploadHistoryEnv(t, envPath)

	store := NewUploadHistoryStore(cfgPath)
	require.True(t, store.Enabled())
	assert.Equal(t, envPath, store.path)
}

// TestUploadHistory_Cap enforces the FIFO cap, keeping the most recent records.
func TestUploadHistory_Cap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upload_history.json")
	store := &UploadHistoryStore{enabled: true, path: path, max: 3}

	ts := time.Now()
	for i := 0; i < 6; i++ {
		require.NoError(t, store.Append(sampleOutcome(
			"u"+string(rune('0'+i)), ts.Add(time.Duration(i)*time.Minute))))
	}

	got, err := store.LoadOutcomes()
	require.NoError(t, err)
	require.Len(t, got, 3, "should keep only the most recent 3")
	assert.Equal(t, "u3", got[0].UploadID)
	assert.Equal(t, "u5", got[2].UploadID)
}

// TestUploadHistory_MissingFile confirms a missing store loads as empty.
func TestUploadHistory_MissingFile(t *testing.T) {
	store := &UploadHistoryStore{enabled: true, path: filepath.Join(t.TempDir(), "nope.json")}
	got, err := store.LoadOutcomes()
	require.NoError(t, err)
	assert.Empty(t, got)
}

// TestUploadHistory_CorruptFileSurfacedOnLoad confirms a corrupt store yields
// an error from LoadOutcomes (the upload-path caller treats it best-effort).
func TestUploadHistory_CorruptFileSurfacedOnLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upload_history.json")
	require.NoError(t, os.WriteFile(path, []byte("{not json"), 0600))
	store := &UploadHistoryStore{enabled: true, path: path}

	_, err := store.LoadOutcomes()
	assert.Error(t, err)
}

// TestUploadHistory_FilePermissions verifies the store is written 0600.
func TestUploadHistory_FilePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "upload_history.json")
	store := &UploadHistoryStore{enabled: true, path: path, max: defaultUploadHistoryMax}
	require.NoError(t, store.Append(sampleOutcome("u1", time.Now())))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	// On-disk shape carries the version.
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc uploadHistoryDoc
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.Equal(t, uploadHistoryStoreVersion, doc.Version)
	assert.Len(t, doc.Records, 1)
}

// TestNewUploadHistoryStore_SpecModes covers opt-in spec parsing.
func TestNewUploadHistoryStore_SpecModes(t *testing.T) {
	prev, had := os.LookupEnv(UploadHistoryEnv)
	require.NoError(t, os.Unsetenv(UploadHistoryEnv))
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(UploadHistoryEnv, prev)
		}
	})
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	tests := []struct {
		name        string
		config      string
		wantEnabled bool
	}{
		{name: "empty", config: "", wantEnabled: false},
		{name: "one", config: "1", wantEnabled: true},
		{name: "true", config: "true", wantEnabled: true},
		{name: "path", config: "/tmp/h.json", wantEnabled: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewUploadHistoryStore(tt.config)
			assert.Equal(t, tt.wantEnabled, store.Enabled())
			if tt.config == "/tmp/h.json" {
				assert.Equal(t, tt.config, store.path)
			}
		})
	}
}

// TestFileTypeMixFromExtensions covers the metadata-only histogram helper.
func TestFileTypeMixFromExtensions(t *testing.T) {
	assert.Nil(t, FileTypeMixFromExtensions(nil))
	got := FileTypeMixFromExtensions([]string{"txt", "txt", "", "go"})
	assert.Equal(t, map[string]int{"txt": 2, "none": 1, "go": 1}, got)
}

// TestSortOutcomesByTime covers the debug-view ordering helper.
func TestSortOutcomesByTime(t *testing.T) {
	ts := time.Now()
	recs := []*UploadOutcome{
		{UploadID: "b", Timestamp: ts.Add(time.Minute)},
		{UploadID: "a", Timestamp: ts},
	}
	sortOutcomesByTime(recs)
	assert.Equal(t, "a", recs[0].UploadID)
	assert.Equal(t, "b", recs[1].UploadID)
}
