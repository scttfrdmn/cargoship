package s3

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/aws/cost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func outcome(ts time.Time, thr, ratio float64, durSec int, success bool) *cost.UploadOutcome {
	return &cost.UploadOutcome{
		UploadID:         "u",
		Timestamp:        ts,
		ThroughputMBps:   thr,
		CompressionRatio: ratio,
		Duration:         time.Duration(durSec) * time.Second,
		Success:          success,
	}
}

// TestTrendAnalyzer_RisingThroughputImproves verifies a rising throughput series
// is classified as improving (higher is better).
func TestTrendAnalyzer_RisingThroughputImproves(t *testing.T) {
	ta := NewTrendAnalyzer()
	td := ta.Analyze("throughput_mbps", []float64{50, 60, 70, 80, 90}, time.Now())

	assert.Equal(t, TrendImproving, td.Direction)
	assert.Greater(t, td.Slope, 0.0)
	assert.Equal(t, 5, td.SampleCount)
	assert.InDelta(t, 90, td.RecentMean, 0.001, "recent mean = trailing quarter (last point)")
}

// TestTrendAnalyzer_RisingRatioDegrades verifies that for compression ratio
// (lower is better), a rising series is degrading.
func TestTrendAnalyzer_RisingRatioDegrades(t *testing.T) {
	ta := NewTrendAnalyzer()
	td := ta.Analyze("compression_ratio", []float64{0.3, 0.4, 0.5, 0.6}, time.Now())

	assert.Equal(t, TrendDegrading, td.Direction)
	assert.Greater(t, td.Slope, 0.0)
}

// TestTrendAnalyzer_FallingDurationImproves verifies falling duration (lower is
// better) is improving.
func TestTrendAnalyzer_FallingDurationImproves(t *testing.T) {
	ta := NewTrendAnalyzer()
	td := ta.Analyze("duration_s", []float64{100, 80, 60, 40}, time.Now())

	assert.Equal(t, TrendImproving, td.Direction)
	assert.Less(t, td.Slope, 0.0)
}

// TestTrendAnalyzer_FlatIsStable verifies a flat series is stable.
func TestTrendAnalyzer_FlatIsStable(t *testing.T) {
	ta := NewTrendAnalyzer()
	td := ta.Analyze("throughput_mbps", []float64{50, 50, 50, 50}, time.Now())

	assert.Equal(t, TrendFlat, td.Direction)
	assert.InDelta(t, 0.0, td.Slope, 1e-9)
}

// TestTrendAnalyzer_EdgeCases covers empty and single-point series.
func TestTrendAnalyzer_EdgeCases(t *testing.T) {
	ta := NewTrendAnalyzer()

	empty := ta.Analyze("throughput_mbps", nil, time.Now())
	assert.Equal(t, TrendFlat, empty.Direction)
	assert.Equal(t, 0, empty.SampleCount)

	single := ta.Analyze("throughput_mbps", []float64{42}, time.Now())
	assert.Equal(t, TrendFlat, single.Direction)
	assert.Equal(t, 1, single.SampleCount)
	assert.InDelta(t, 42, single.RecentMean, 1e-9)
}

// TestAnomalyDetector_FlagsOutlier verifies a 3-sigma outlier is flagged.
func TestAnomalyDetector_FlagsOutlier(t *testing.T) {
	ad := NewAnomalyDetector()
	base := computeBaseline([]float64{100, 101, 99, 100, 100, 101, 99})

	// Way outside baseline.
	a, found := ad.Check("throughput_mbps", 10, base, time.Now())
	require.True(t, found)
	assert.Equal(t, "throughput_mbps", a.Metric)
	assert.Less(t, a.ZScore, -3.0)

	// Within baseline.
	_, found = ad.Check("throughput_mbps", 100.5, base, time.Now())
	assert.False(t, found)
}

// TestAnomalyDetector_InsufficientBaseline verifies too-small or zero-variance
// baselines are not judged.
func TestAnomalyDetector_InsufficientBaseline(t *testing.T) {
	ad := NewAnomalyDetector()

	_, found := ad.Check("m", 100, baselineStat{Mean: 50, StdDev: 10, Count: 1}, time.Now())
	assert.False(t, found, "count < 2 can't be judged")

	_, found = ad.Check("m", 100, baselineStat{Mean: 50, StdDev: 0, Count: 5}, time.Now())
	assert.False(t, found, "zero variance can't be judged")
}

// TestComputeBaseline verifies mean/stddev.
func TestComputeBaseline(t *testing.T) {
	b := computeBaseline([]float64{2, 4, 4, 4, 5, 5, 7, 9})
	assert.InDelta(t, 5.0, b.Mean, 1e-9)
	assert.InDelta(t, 2.0, b.StdDev, 1e-9) // population stddev
	assert.Equal(t, 8, b.Count)

	assert.Equal(t, 0, computeBaseline(nil).Count)
}

// TestIngestOutcomes_BuildsTrends verifies the analyzer builds forward-in-time
// trends from an unordered corpus and reflects the latest throughput.
func TestIngestOutcomes_BuildsTrends(t *testing.T) {
	pa := NewPerformanceAnalyzer()
	base := time.Now().Add(-10 * time.Hour)

	// Deliberately out of order; throughput rising over time.
	outcomes := []*cost.UploadOutcome{
		outcome(base.Add(3*time.Hour), 80, 0.5, 40, true),
		outcome(base.Add(1*time.Hour), 60, 0.5, 60, true),
		outcome(base.Add(4*time.Hour), 90, 0.5, 30, true),
		outcome(base.Add(2*time.Hour), 70, 0.5, 50, true),
	}
	anomalies := pa.IngestOutcomes(outcomes)

	tp, ok := pa.GetTrend("throughput_mbps")
	require.True(t, ok)
	assert.Equal(t, TrendImproving, tp.Direction, "sorted oldest-first, throughput rises")
	assert.Equal(t, 4, tp.SampleCount)

	dur, ok := pa.GetTrend("duration_s")
	require.True(t, ok)
	assert.Equal(t, TrendImproving, dur.Direction, "duration falling over time is good")

	// Latest throughput (90) reflected into currentPerformance.
	assert.InDelta(t, 90, pa.AnalyzeCurrentPerformance().ThroughputMBps, 1e-9)

	// A steady series produces no anomalies.
	assert.Empty(t, anomalies)
}

// TestIngestOutcomes_DetectsAnomaly verifies a wildly different final upload is
// flagged as anomalous.
func TestIngestOutcomes_DetectsAnomaly(t *testing.T) {
	pa := NewPerformanceAnalyzer()
	base := time.Now().Add(-10 * time.Hour)

	// Prior history clusters near 100 MB/s with small, realistic variance.
	priorThroughputs := []float64{98, 101, 99, 102, 100, 97, 103, 100}
	var outcomes []*cost.UploadOutcome
	for i, thr := range priorThroughputs {
		outcomes = append(outcomes, outcome(base.Add(time.Duration(i)*time.Hour), thr, 0.5, 50, true))
	}
	// Final upload: throughput collapses far outside the baseline.
	outcomes = append(outcomes, outcome(base.Add(9*time.Hour), 5, 0.5, 50, true))

	anomalies := pa.IngestOutcomes(outcomes)
	require.NotEmpty(t, anomalies)
	found := false
	for _, a := range anomalies {
		if a.Metric == "throughput_mbps" {
			found = true
			assert.Less(t, a.ZScore, 0.0)
		}
	}
	assert.True(t, found, "collapsed throughput should be flagged")
}

// TestIngestOutcomes_SkipsFailedAndZero verifies failed uploads and zero-valued
// metrics are excluded from the series.
func TestIngestOutcomes_SkipsFailedAndZero(t *testing.T) {
	pa := NewPerformanceAnalyzer()
	base := time.Now().Add(-5 * time.Hour)

	outcomes := []*cost.UploadOutcome{
		outcome(base.Add(1*time.Hour), 60, 0.5, 60, true),
		outcome(base.Add(2*time.Hour), 999, 0.5, 60, false), // failed: excluded
		outcome(base.Add(3*time.Hour), 0, 0.5, 60, true),    // zero throughput: excluded
		outcome(base.Add(4*time.Hour), 80, 0.5, 60, true),
	}
	pa.IngestOutcomes(outcomes)

	tp, ok := pa.GetTrend("throughput_mbps")
	require.True(t, ok)
	assert.Equal(t, 2, tp.SampleCount, "only the two valid successful throughputs")
}

// TestIngestOutcomes_Idempotent verifies re-ingesting replaces (not accumulates)
// trend state.
func TestIngestOutcomes_Idempotent(t *testing.T) {
	pa := NewPerformanceAnalyzer()
	base := time.Now().Add(-5 * time.Hour)
	outcomes := []*cost.UploadOutcome{
		outcome(base.Add(1*time.Hour), 60, 0.5, 60, true),
		outcome(base.Add(2*time.Hour), 80, 0.5, 60, true),
	}
	pa.IngestOutcomes(outcomes)
	pa.IngestOutcomes(outcomes)

	tp, ok := pa.GetTrend("throughput_mbps")
	require.True(t, ok)
	assert.Equal(t, 2, tp.SampleCount, "state replaced, not doubled")
}

// TestPrimeFromHistory_DisabledIsNoop verifies priming does nothing when the
// history store is disabled (default).
func TestPrimeFromHistory_DisabledIsNoop(t *testing.T) {
	t.Setenv("CARGOSHIP_UPLOAD_HISTORY", "")
	as := NewAdaptiveStaging(context.Background())

	anomalies, err := as.PrimeFromHistory()
	require.NoError(t, err)
	assert.Nil(t, anomalies)
	assert.Empty(t, as.GetPerformanceTrends())
}

// TestPrimeFromHistory_LoadsCorpus verifies priming ingests a persisted corpus
// end-to-end through the #261 store.
func TestPrimeFromHistory_LoadsCorpus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upload_history.json")
	t.Setenv("CARGOSHIP_UPLOAD_HISTORY", path)

	store := cost.NewUploadHistoryStore("")
	require.True(t, store.Enabled())
	base := time.Now().Add(-4 * time.Hour)
	for i := 0; i < 4; i++ {
		require.NoError(t, store.Append(outcome(
			base.Add(time.Duration(i)*time.Hour), float64(50+i*10), 0.5, 60, true)))
	}

	as := NewAdaptiveStaging(context.Background())
	_, err := as.PrimeFromHistory()
	require.NoError(t, err)

	trends := as.GetPerformanceTrends()
	require.Contains(t, trends, "throughput_mbps")
	assert.Equal(t, TrendImproving, trends["throughput_mbps"].Direction)
	assert.Equal(t, 4, trends["throughput_mbps"].SampleCount)
}

// TestPrimeFromHistory_LoadErrorSurfaced verifies a corrupt store surfaces an
// error rather than panicking.
func TestPrimeFromHistory_LoadErrorSurfaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upload_history.json")
	require.NoError(t, os.WriteFile(path, []byte("{corrupt"), 0600))
	t.Setenv("CARGOSHIP_UPLOAD_HISTORY", path)

	as := NewAdaptiveStaging(context.Background())
	_, err := as.PrimeFromHistory()
	assert.Error(t, err)
}

// TestHigherIsBetter documents the metric polarity map.
func TestHigherIsBetter(t *testing.T) {
	assert.True(t, higherIsBetter("throughput_mbps"))
	assert.False(t, higherIsBetter("compression_ratio"))
	assert.False(t, higherIsBetter("duration_s"))
	assert.False(t, higherIsBetter("unknown_metric"))
}
