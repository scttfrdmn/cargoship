package s3

import (
	"math"
	"sort"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/aws/cost"
)

// This file implements the trend + anomaly layer of the staging
// PerformanceAnalyzer (#140). It turns the persisted per-upload outcome corpus
// (#261) into per-metric trends and anomaly flags that adaptation logic can
// consult. It is pure analysis over recorded history — it observes, it does not
// change any optimization decision, and the ML prediction model remains
// deferred to Option L / #137.

// TrendQuality classifies whether a metric's recent movement is good or bad for
// upload performance (distinct from coordinator.go's raw TrendDirection, which
// only records increasing/decreasing without a notion of "better").
type TrendQuality string

const (
	TrendImproving TrendQuality = "improving"
	TrendDegrading TrendQuality = "degrading"
	TrendFlat      TrendQuality = "stable"
)

// TrendData is the computed trend for a single named metric over the analyzed
// window. Slope is in metric-units per sample (oldest→newest).
type TrendData struct {
	Metric      string       `json:"metric"`
	Direction   TrendQuality `json:"direction"`
	Slope       float64      `json:"slope"`
	RecentMean  float64      `json:"recent_mean"`
	SampleCount int          `json:"sample_count"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// baselineStat is the rolling mean/stddev of one metric, the reference an
// anomaly is measured against.
type baselineStat struct {
	Mean   float64
	StdDev float64
	Count  int
}

// BaselineMetrics holds per-metric baselines derived from the corpus.
type BaselineMetrics struct {
	stats map[string]baselineStat
}

// NewBaselineMetrics returns an empty baseline set.
func NewBaselineMetrics() *BaselineMetrics {
	return &BaselineMetrics{stats: make(map[string]baselineStat)}
}

// TrendAnalyzer computes least-squares slope over an ordered metric series.
type TrendAnalyzer struct {
	// stableThreshold is the fraction of the series mean below which an
	// absolute slope is treated as "stable" (no meaningful trend).
	stableThreshold float64
}

// NewTrendAnalyzer returns a trend analyzer with sensible defaults.
func NewTrendAnalyzer() *TrendAnalyzer {
	return &TrendAnalyzer{stableThreshold: 0.01}
}

// higherIsBetter reports whether a rising value of the metric is an improvement.
// Throughput rising is good; latency or duration rising is bad. Compression
// ratio here is compressed/original, so a falling ratio (smaller output) is the
// improvement.
func higherIsBetter(metric string) bool {
	switch metric {
	case "throughput_mbps":
		return true
	default:
		// latency_ms, duration_s, compression_ratio, error_rate: lower is better.
		return false
	}
}

// Analyze computes a TrendData for the given metric over an oldest-first series.
// Fewer than two points yields a stable trend (nothing to slope over).
func (ta *TrendAnalyzer) Analyze(metric string, series []float64, now time.Time) *TrendData {
	n := len(series)
	td := &TrendData{Metric: metric, Direction: TrendFlat, SampleCount: n, UpdatedAt: now}
	if n == 0 {
		return td
	}

	// Recent mean over the trailing quarter (min 1) of the series.
	recentN := n / 4
	if recentN < 1 {
		recentN = 1
	}
	var recentSum float64
	for _, v := range series[n-recentN:] {
		recentSum += v
	}
	td.RecentMean = recentSum / float64(recentN)

	if n < 2 {
		return td
	}

	// Least-squares slope of value vs index.
	fn := float64(n)
	var sumX, sumY, sumXY, sumX2 float64
	for i, y := range series {
		x := float64(i)
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}
	denom := fn*sumX2 - sumX*sumX
	if denom == 0 {
		return td
	}
	td.Slope = (fn*sumXY - sumX*sumY) / denom

	mean := sumY / fn
	// Treat near-flat slopes (relative to the mean magnitude) as stable.
	if mean == 0 || math.Abs(td.Slope) < ta.stableThreshold*math.Abs(mean) {
		td.Direction = TrendFlat
		return td
	}
	rising := td.Slope > 0
	if rising == higherIsBetter(metric) {
		td.Direction = TrendImproving
	} else {
		td.Direction = TrendDegrading
	}
	return td
}

// Anomaly flags a single observation that deviates from a metric's baseline.
type Anomaly struct {
	Metric   string    `json:"metric"`
	Value    float64   `json:"value"`
	Baseline float64   `json:"baseline"`
	ZScore   float64   `json:"z_score"`
	At       time.Time `json:"at"`
}

// AnomalyDetector flags observations beyond a z-score threshold from baseline.
type AnomalyDetector struct {
	// zThreshold is the |z-score| beyond which a value is anomalous.
	zThreshold float64
}

// NewAnomalyDetector returns a detector with a 3-sigma default threshold.
func NewAnomalyDetector() *AnomalyDetector {
	return &AnomalyDetector{zThreshold: 3.0}
}

// Check reports whether value is anomalous relative to the baseline. A baseline
// with fewer than 2 samples or zero variance can't be judged (returns false).
func (ad *AnomalyDetector) Check(metric string, value float64, base baselineStat, now time.Time) (*Anomaly, bool) {
	if base.Count < 2 || base.StdDev == 0 {
		return nil, false
	}
	z := (value - base.Mean) / base.StdDev
	if math.Abs(z) < ad.zThreshold {
		return nil, false
	}
	return &Anomaly{Metric: metric, Value: value, Baseline: base.Mean, ZScore: z, At: now}, true
}

// metricSeries extracts the four analyzed metrics from an oldest-first slice of
// successful outcomes. Failed uploads are skipped for throughput/compression
// (their measurements are not representative) but counted for error rate.
func metricSeries(outcomes []*cost.UploadOutcome) map[string][]float64 {
	series := map[string][]float64{
		"throughput_mbps":   {},
		"compression_ratio": {},
		"duration_s":        {},
	}
	for _, o := range outcomes {
		if o == nil {
			continue
		}
		if o.Success {
			if o.ThroughputMBps > 0 {
				series["throughput_mbps"] = append(series["throughput_mbps"], o.ThroughputMBps)
			}
			if o.CompressionRatio > 0 {
				series["compression_ratio"] = append(series["compression_ratio"], o.CompressionRatio)
			}
			if o.Duration > 0 {
				series["duration_s"] = append(series["duration_s"], o.Duration.Seconds())
			}
		}
	}
	return series
}

// computeBaseline returns the mean/stddev of a series.
func computeBaseline(series []float64) baselineStat {
	n := len(series)
	if n == 0 {
		return baselineStat{}
	}
	var sum float64
	for _, v := range series {
		sum += v
	}
	mean := sum / float64(n)
	var ss float64
	for _, v := range series {
		d := v - mean
		ss += d * d
	}
	return baselineStat{Mean: mean, StdDev: math.Sqrt(ss / float64(n)), Count: n}
}

// IngestOutcomes rebuilds the analyzer's trends and baselines from a corpus of
// per-upload outcomes (#261). Outcomes are sorted oldest-first so the trend
// slope points forward in time. It replaces (not merges) prior trend state, so
// repeated calls with a growing corpus are idempotent. Returns the anomalies
// found in the most recent successful upload relative to the historical
// baseline (empty if there's too little history).
func (pa *PerformanceAnalyzer) IngestOutcomes(outcomes []*cost.UploadOutcome) []*Anomaly {
	now := time.Now()

	// Defensive copy + stable sort oldest-first.
	sorted := make([]*cost.UploadOutcome, 0, len(outcomes))
	for _, o := range outcomes {
		if o != nil {
			sorted = append(sorted, o)
		}
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})

	series := metricSeries(sorted)

	pa.mu.Lock()
	defer pa.mu.Unlock()

	pa.performanceTrends = make(map[string]*TrendData, len(series))
	if pa.baselineMetrics == nil {
		pa.baselineMetrics = NewBaselineMetrics()
	}
	pa.baselineMetrics.stats = make(map[string]baselineStat, len(series))

	for metric, values := range series {
		pa.performanceTrends[metric] = pa.trendAnalyzer.Analyze(metric, values, now)
		pa.baselineMetrics.stats[metric] = computeBaseline(values)
	}

	// Reflect the latest throughput into currentPerformance so existing
	// consumers (AnalyzeCurrentPerformance) see corpus-derived numbers.
	if tp := series["throughput_mbps"]; len(tp) > 0 {
		latest := tp[len(tp)-1]
		if pa.currentPerformance == nil {
			pa.currentPerformance = &PerformanceMetrics{TargetThroughput: 100.0, Reliability: 0.95}
		}
		pa.currentPerformance.ThroughputMBps = latest
	}

	// Anomaly check on the most recent successful upload. The reference
	// baseline is built from PRIOR history only (excluding the last upload) —
	// including a genuine outlier in its own baseline inflates the stddev and
	// masks it. This needs at least a few points of prior history to be
	// meaningful (AnomalyDetector.Check requires count >= 2).
	var anomalies []*Anomaly
	if len(sorted) > 0 {
		last := sorted[len(sorted)-1]
		if last.Success {
			priorSeries := metricSeries(sorted[:len(sorted)-1])
			latest := map[string]float64{
				"throughput_mbps":   last.ThroughputMBps,
				"compression_ratio": last.CompressionRatio,
			}
			if last.Duration > 0 {
				latest["duration_s"] = last.Duration.Seconds()
			}
			for metric, v := range latest {
				if v == 0 {
					continue
				}
				priorBase := computeBaseline(priorSeries[metric])
				if a, found := pa.anomalyDetector.Check(metric, v, priorBase, now); found {
					anomalies = append(anomalies, a)
				}
			}
		}
	}
	return anomalies
}

// PrimeFromHistory loads the persisted upload-outcome corpus (#261) and rebuilds
// the performance analyzer's trends and baselines from it, returning any
// anomalies detected in the most recent upload. It is a no-op returning nil when
// the history store is disabled (the opt-in default), so callers can invoke it
// unconditionally at startup. Reads only — it never changes staging behavior.
func (as *AdaptiveStaging) PrimeFromHistory() ([]*Anomaly, error) {
	store := cost.NewUploadHistoryStore("")
	if !store.Enabled() {
		return nil, nil
	}
	outcomes, err := store.LoadOutcomes()
	if err != nil {
		return nil, err
	}
	if len(outcomes) == 0 {
		return nil, nil
	}
	return as.performanceAnalyzer.IngestOutcomes(outcomes), nil
}

// GetPerformanceTrends returns a copy of the analyzer's current per-metric
// trends, for status reporting.
func (as *AdaptiveStaging) GetPerformanceTrends() map[string]*TrendData {
	return as.performanceAnalyzer.GetTrends()
}

// GetTrend returns the computed trend for a metric, if analyzed.
func (pa *PerformanceAnalyzer) GetTrend(metric string) (*TrendData, bool) {
	pa.mu.RLock()
	defer pa.mu.RUnlock()
	td, ok := pa.performanceTrends[metric]
	return td, ok
}

// GetTrends returns a copy of all computed trends.
func (pa *PerformanceAnalyzer) GetTrends() map[string]*TrendData {
	pa.mu.RLock()
	defer pa.mu.RUnlock()
	out := make(map[string]*TrendData, len(pa.performanceTrends))
	for k, v := range pa.performanceTrends {
		out[k] = v
	}
	return out
}
