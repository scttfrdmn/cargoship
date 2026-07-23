package staging

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withHistoryStore points CARGOSHIP_COMPRESSION_HISTORY at a scratch file for
// the duration of a test and restores the prior value afterward.
func withHistoryStore(t *testing.T, path string) {
	t.Helper()
	prev, had := os.LookupEnv(compressionHistoryEnv)
	require.NoError(t, os.Setenv(compressionHistoryEnv, path))
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(compressionHistoryEnv, prev)
		} else {
			_ = os.Unsetenv(compressionHistoryEnv)
		}
	})
}

// TestCompressionHistory_Disabled confirms the tracker stays in-memory-only and
// writes nothing when persistence is not opted into.
func TestCompressionHistory_Disabled(t *testing.T) {
	// Ensure the env var is unset for this test.
	prev, had := os.LookupEnv(compressionHistoryEnv)
	require.NoError(t, os.Unsetenv(compressionHistoryEnv))
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(compressionHistoryEnv, prev)
		}
	})

	ch := NewCompressionHistory()
	assert.False(t, ch.store.enabled, "store should be disabled when env unset")

	ch.AddResult("text", 1000, 0.7)
	assert.InDelta(t, 0.7, ch.GetAverageRatio("text", 1000), 1e-9)
}

// TestCompressionHistory_RoundTrip verifies learned history survives a restart:
// a fresh tracker constructed against the same store recovers prior results.
func TestCompressionHistory_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compression_history.json")
	withHistoryStore(t, path)

	first := NewCompressionHistory()
	require.True(t, first.store.enabled)
	first.AddResult("text", 1000, 0.7)
	first.AddResult("text", 1000, 0.6)
	first.AddResult("json", 2000, 0.5)

	// File should exist after AddResult flushes.
	_, err := os.Stat(path)
	require.NoError(t, err, "history file should be written")

	// A new tracker (simulating a fresh process) loads the persisted history.
	second := NewCompressionHistory()
	assert.Len(t, second.GetResultsForContentType("text"), 2)
	assert.Len(t, second.GetResultsForContentType("json"), 1)
	// The learned average is recovered (recent-weighted average of 0.7 and 0.6).
	got := second.GetAverageRatio("text", 1000)
	assert.Greater(t, got, 0.0)
	assert.InDelta(t, 0.65, got, 0.05)
}

// TestCompressionHistory_DecayOnLoad verifies entries older than the decay
// window are dropped when the store is loaded.
func TestCompressionHistory_DecayOnLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compression_history.json")

	now := time.Now()
	fresh := &HistoricalCompressionResult{ContentType: "text", Size: 1000, Ratio: 0.7, Timestamp: now.Add(-24 * time.Hour)}
	stale := &HistoricalCompressionResult{ContentType: "text", Size: 1000, Ratio: 0.3, Timestamp: now.Add(-compressionHistoryDecayWindow - 24*time.Hour)}
	doc := compressionHistoryDoc{
		Version: compressionHistoryStoreVersion,
		Results: map[string][]*HistoricalCompressionResult{"text": {stale, fresh}},
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0600))

	store := compressionHistoryStore{enabled: true, path: path}
	loaded, err := store.load(now, 1000)
	require.NoError(t, err)

	require.Len(t, loaded["text"], 1, "stale entry should be pruned on load")
	assert.InDelta(t, 0.7, loaded["text"][0].Ratio, 1e-9)
}

// TestCompressionHistory_CapOnLoad verifies the per-content-type cap keeps only
// the most recent maxResults entries on load.
func TestCompressionHistory_CapOnLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compression_history.json")

	now := time.Now()
	var results []*HistoricalCompressionResult
	for i := 0; i < 10; i++ {
		results = append(results, &HistoricalCompressionResult{
			ContentType: "text",
			Size:        1000,
			Ratio:       float64(i) / 10.0,
			// Older -> newer as i grows.
			Timestamp: now.Add(-time.Duration(10-i) * time.Hour),
		})
	}
	doc := compressionHistoryDoc{Version: compressionHistoryStoreVersion, Results: map[string][]*HistoricalCompressionResult{"text": results}}
	data, err := json.MarshalIndent(doc, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0600))

	store := compressionHistoryStore{enabled: true, path: path}
	loaded, err := store.load(now, 3)
	require.NoError(t, err)

	require.Len(t, loaded["text"], 3, "should keep only the most recent 3")
	// The most recent three have ratios 0.7, 0.8, 0.9 (oldest-first order).
	assert.InDelta(t, 0.7, loaded["text"][0].Ratio, 1e-9)
	assert.InDelta(t, 0.9, loaded["text"][2].Ratio, 1e-9)
}

// TestCompressionHistory_MissingFile confirms a missing store is not an error.
func TestCompressionHistory_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")
	store := compressionHistoryStore{enabled: true, path: path}

	loaded, err := store.load(time.Now(), 1000)
	require.NoError(t, err)
	assert.Empty(t, loaded)
}

// TestCompressionHistory_CorruptFileDoesNotBlockStartup ensures a corrupt store
// is ignored by the constructor rather than panicking or losing the tracker.
func TestCompressionHistory_CorruptFileDoesNotBlockStartup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compression_history.json")
	require.NoError(t, os.WriteFile(path, []byte("{not valid json"), 0600))
	withHistoryStore(t, path)

	ch := NewCompressionHistory() // must not panic
	require.NotNil(t, ch)
	// Starts cold; still usable and re-persists cleanly.
	assert.Empty(t, ch.GetResultsForContentType("text"))
	ch.AddResult("text", 1000, 0.7)
	assert.Len(t, ch.GetResultsForContentType("text"), 1)
}

// TestCompressionHistory_SaveConvergence checks that predictions accumulate
// across simulated runs — the core "improves across restarts" goal.
func TestCompressionHistory_SaveConvergence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compression_history.json")
	withHistoryStore(t, path)

	// Three separate "processes" each learn one result for the same content.
	for i := 0; i < 3; i++ {
		ch := NewCompressionHistory()
		ch.AddResult("logs", 5000, 0.8)
	}

	final := NewCompressionHistory()
	assert.Len(t, final.GetResultsForContentType("logs"), 3,
		"results should accumulate across restarts")
	assert.InDelta(t, 0.8, final.GetAverageRatio("logs", 5000), 1e-6)
}

// TestCompressionHistoryStore_FilePermissions verifies the store is written 0600.
func TestCompressionHistoryStore_FilePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "compression_history.json")
	store := compressionHistoryStore{enabled: true, path: path}

	require.NoError(t, store.save(map[string][]*HistoricalCompressionResult{
		"text": {{ContentType: "text", Size: 1, Ratio: 0.5, Timestamp: time.Now()}},
	}))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

// TestNewCompressionHistoryStore_EnvModes covers the opt-in switch parsing.
func TestNewCompressionHistoryStore_EnvModes(t *testing.T) {
	tests := []struct {
		name        string
		env         string
		unset       bool
		wantEnabled bool
	}{
		{name: "unset", unset: true, wantEnabled: false},
		{name: "empty", env: "", wantEnabled: false},
		{name: "one", env: "1", wantEnabled: true},
		{name: "true", env: "true", wantEnabled: true},
		{name: "explicit path", env: "/tmp/x.json", wantEnabled: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.unset {
				prev, had := os.LookupEnv(compressionHistoryEnv)
				require.NoError(t, os.Unsetenv(compressionHistoryEnv))
				t.Cleanup(func() {
					if had {
						_ = os.Setenv(compressionHistoryEnv, prev)
					}
				})
			} else {
				withHistoryStore(t, tt.env)
			}
			store := newCompressionHistoryStore()
			assert.Equal(t, tt.wantEnabled, store.enabled)
			if tt.wantEnabled && tt.env != "1" && tt.env != "true" && tt.env != "" {
				assert.Equal(t, tt.env, store.path)
			}
		})
	}
}
