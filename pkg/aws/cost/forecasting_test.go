package cost

import (
	"log/slog"
	"math"
	"testing"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test helper to create a cost reporter with test data
func createTestReporterWithHistoricalData(t *testing.T, days int, dailyCost float64, trend string) *CostReporter {
	t.Helper()

	cfg := &config.CostReportingConfig{
		Enabled: true,
	}
	// Create pricing manager with nil config (will use defaults)
	reporter := NewCostReporter(cfg, nil, nil, slog.Default())

	now := time.Now()

	// Generate historical cost records based on trend
	for i := 0; i < days; i++ {
		dayOffset := -days + i
		timestamp := now.Add(time.Duration(dayOffset) * 24 * time.Hour)

		var cost float64
		switch trend {
		case "increasing":
			// Linear increase: 10% growth per day
			cost = dailyCost * (1.0 + float64(i)*0.10)
		case "decreasing":
			// Linear decrease: 5% decrease per day
			cost = dailyCost * math.Max(0.1, 1.0 - float64(i)*0.05)
		case "stable":
			// Stable with small random variation (±5%)
			variation := 0.95 + (float64(i%10) / 100.0) // Deterministic "random"
			cost = dailyCost * variation
		case "volatile":
			// High volatility: alternating high/low
			if i%2 == 0 {
				cost = dailyCost * 1.5
			} else {
				cost = dailyCost * 0.5
			}
		default:
			cost = dailyCost
		}

		reporter.RecordCost(CostRecord{
			Timestamp:    timestamp,
			Operation:    "upload",
			Service:      "s3",
			Region:       "us-east-1",
			StorageClass: "STANDARD",
			SizeBytes:    1024 * 1024 * 100, // 100 MB
			SizeGB:       0.1,
			Cost:         cost,
			Currency:     "USD",
			ProjectID:    "test-project",
		})
	}

	return reporter
}

// TestNewForecastEngine tests forecast engine creation
func TestNewForecastEngine(t *testing.T) {
	reporter := createTestReporterWithHistoricalData(t, 30, 10.0, "stable")
	engine := NewForecastEngine(reporter)

	assert.NotNil(t, engine)
	assert.Equal(t, reporter, engine.reporter)
}

// TestAnalyzeBurnRate_StablePattern tests burn rate analysis with stable costs
func TestAnalyzeBurnRate_StablePattern(t *testing.T) {
	reporter := createTestReporterWithHistoricalData(t, 30, 10.0, "stable")
	engine := NewForecastEngine(reporter)

	analysis, err := engine.AnalyzeBurnRate("test-project", 30)
	require.NoError(t, err)
	require.NotNil(t, analysis)

	// Stable pattern should have low volatility
	assert.InDelta(t, 10.0, analysis.AverageDailyRate, 2.0, "Average daily rate should be ~10")
	assert.Less(t, analysis.Volatility, 0.2, "Volatility should be low for stable pattern")
	assert.Equal(t, "stable", analysis.TrendDirection, "Should detect stable trend")
	assert.Less(t, analysis.TrendStrength, 0.3, "Trend strength should be weak for stable pattern")

	// Confidence intervals should be present
	assert.Contains(t, analysis.ConfidenceIntervals, 90)
	assert.Contains(t, analysis.ConfidenceIntervals, 95)
	assert.Contains(t, analysis.ConfidenceIntervals, 99)

	t.Logf("Stable Pattern Analysis: avg_rate=%.2f, volatility=%.2f, trend=%s (%.2f)",
		analysis.AverageDailyRate, analysis.Volatility, analysis.TrendDirection, analysis.TrendStrength)
}

// TestAnalyzeBurnRate_IncreasingPattern tests burn rate analysis with increasing costs
func TestAnalyzeBurnRate_IncreasingPattern(t *testing.T) {
	reporter := createTestReporterWithHistoricalData(t, 30, 10.0, "increasing")
	engine := NewForecastEngine(reporter)

	analysis, err := engine.AnalyzeBurnRate("test-project", 30)
	require.NoError(t, err)
	require.NotNil(t, analysis)

	// Increasing pattern should show upward trend
	assert.Equal(t, "increasing", analysis.TrendDirection, "Should detect increasing trend")
	assert.Greater(t, analysis.TrendStrength, 0.0, "Trend strength should be positive")
	assert.Greater(t, analysis.AccelerationRate, 0.0, "Acceleration should be positive")

	// Future burn rates should be higher
	assert.Greater(t, analysis.PredictedDailyRate30Days, analysis.AverageDailyRate,
		"Predicted rate should be higher than average")
	assert.Greater(t, analysis.PredictedDailyRate90Days, analysis.PredictedDailyRate30Days,
		"90-day prediction should be higher than 30-day")

	t.Logf("Increasing Pattern Analysis: avg_rate=%.2f, accel=%.2f, predicted_30d=%.2f, predicted_90d=%.2f",
		analysis.AverageDailyRate, analysis.AccelerationRate,
		analysis.PredictedDailyRate30Days, analysis.PredictedDailyRate90Days)
}

// TestAnalyzeBurnRate_DecreasingPattern tests burn rate analysis with decreasing costs
func TestAnalyzeBurnRate_DecreasingPattern(t *testing.T) {
	reporter := createTestReporterWithHistoricalData(t, 30, 20.0, "decreasing")
	engine := NewForecastEngine(reporter)

	analysis, err := engine.AnalyzeBurnRate("test-project", 30)
	require.NoError(t, err)
	require.NotNil(t, analysis)

	// Decreasing pattern should show downward trend
	assert.Equal(t, "decreasing", analysis.TrendDirection, "Should detect decreasing trend")
	assert.Greater(t, analysis.TrendStrength, 0.0, "Trend strength should be positive")
	assert.Less(t, analysis.AccelerationRate, 0.0, "Acceleration should be negative")

	t.Logf("Decreasing Pattern Analysis: avg_rate=%.2f, accel=%.2f, trend_strength=%.2f",
		analysis.AverageDailyRate, analysis.AccelerationRate, analysis.TrendStrength)
}

// TestAnalyzeBurnRate_VolatilePattern tests burn rate analysis with volatile costs
func TestAnalyzeBurnRate_VolatilePattern(t *testing.T) {
	reporter := createTestReporterWithHistoricalData(t, 30, 10.0, "volatile")
	engine := NewForecastEngine(reporter)

	analysis, err := engine.AnalyzeBurnRate("test-project", 30)
	require.NoError(t, err)
	require.NotNil(t, analysis)

	// Volatile pattern should have high volatility
	assert.Greater(t, analysis.Volatility, 0.3, "Volatility should be high for volatile pattern")
	assert.Greater(t, analysis.MaxDailyRate, analysis.MinDailyRate*1.5,
		"Max should be significantly higher than min")

	// Wide confidence intervals
	ci95 := analysis.ConfidenceIntervals[95]
	intervalWidth := ci95.UpperBound - ci95.LowerBound
	assert.Greater(t, intervalWidth, analysis.AverageDailyRate*0.5,
		"Confidence interval should be wide for volatile data")

	t.Logf("Volatile Pattern Analysis: volatility=%.2f, min=%.2f, max=%.2f, CI_width=%.2f",
		analysis.Volatility, analysis.MinDailyRate, analysis.MaxDailyRate, intervalWidth)
}

// TestAnalyzeBurnRate_NoData tests error handling with no cost records
func TestAnalyzeBurnRate_NoData(t *testing.T) {
	cfg := &config.CostReportingConfig{Enabled: true}
	reporter := NewCostReporter(cfg, nil, nil, slog.Default())
	engine := NewForecastEngine(reporter)

	analysis, err := engine.AnalyzeBurnRate("test-project", 30)

	assert.Error(t, err)
	assert.Nil(t, analysis)
	assert.Contains(t, err.Error(), "no cost records found")
}

// TestGenerateForecast_Linear tests linear regression forecasting
func TestGenerateForecast_Linear(t *testing.T) {
	reporter := createTestReporterWithHistoricalData(t, 30, 10.0, "increasing")
	engine := NewForecastEngine(reporter)

	forecast, err := engine.GenerateForecast("test-project", ForecastModelLinear, 90)
	require.NoError(t, err)
	require.NotNil(t, forecast)

	// Verify forecast structure
	assert.Equal(t, ForecastModelLinear, forecast.Model)
	assert.Equal(t, 90, forecast.ForecastDays)
	assert.Greater(t, forecast.BaseCost, 0.0)
	assert.Equal(t, 30, forecast.HistoricalDays)

	// Verify key time point predictions
	assert.Greater(t, forecast.Predicted7Days, forecast.BaseCost, "7-day forecast should exceed base")
	assert.Greater(t, forecast.Predicted30Days, forecast.Predicted7Days, "30-day should exceed 7-day")
	assert.Greater(t, forecast.Predicted90Days, forecast.Predicted30Days, "90-day should exceed 30-day")

	// Verify daily forecasts
	assert.Len(t, forecast.DailyForecasts, 90)
	for day := 1; day < 90; day++ {
		assert.Greater(t, forecast.DailyForecasts[day+1], forecast.DailyForecasts[day],
			"Daily costs should be increasing")
	}

	// Verify confidence intervals
	assert.NotNil(t, forecast.Confidence7Days)
	assert.NotNil(t, forecast.Confidence30Days)
	assert.NotNil(t, forecast.Confidence90Days)

	// Confidence intervals should widen over time
	width7 := forecast.Confidence7Days.UpperBound - forecast.Confidence7Days.LowerBound
	width30 := forecast.Confidence30Days.UpperBound - forecast.Confidence30Days.LowerBound
	assert.Greater(t, width30, width7, "30-day confidence interval should be wider than 7-day")

	// Model accuracy should be reasonable (R² > 0.5 for linear trend)
	assert.Greater(t, forecast.ModelAccuracy, 0.5, "Model accuracy should be good for linear trend")
	assert.Greater(t, forecast.R2Score, 0.5, "R² score should be good")

	t.Logf("Linear Forecast: base=%.2f, 7d=%.2f, 30d=%.2f, 90d=%.2f, accuracy=%.2f, R²=%.2f",
		forecast.BaseCost, forecast.Predicted7Days, forecast.Predicted30Days,
		forecast.Predicted90Days, forecast.ModelAccuracy, forecast.R2Score)
}

// TestGenerateForecast_Ensemble tests ensemble forecasting
func TestGenerateForecast_Ensemble(t *testing.T) {
	reporter := createTestReporterWithHistoricalData(t, 30, 10.0, "stable")
	engine := NewForecastEngine(reporter)

	forecast, err := engine.GenerateForecast("test-project", ForecastModelEnsemble, 90)
	require.NoError(t, err)
	require.NotNil(t, forecast)

	// Verify ensemble model
	assert.Equal(t, ForecastModelEnsemble, forecast.Model)
	assert.Greater(t, forecast.Predicted30Days, 0.0)
	assert.Greater(t, forecast.Predicted90Days, 0.0)

	// Ensemble should have all daily forecasts
	assert.Len(t, forecast.DailyForecasts, 90)

	t.Logf("Ensemble Forecast: 30d=%.2f, 90d=%.2f, accuracy=%.2f",
		forecast.Predicted30Days, forecast.Predicted90Days, forecast.ModelAccuracy)
}

// TestGenerateForecast_InsufficientData tests error handling with minimal data
func TestGenerateForecast_InsufficientData(t *testing.T) {
	// Only 1 day of data
	reporter := createTestReporterWithHistoricalData(t, 1, 10.0, "stable")
	engine := NewForecastEngine(reporter)

	forecast, err := engine.GenerateForecast("test-project", ForecastModelLinear, 30)

	assert.Error(t, err)
	assert.Nil(t, forecast)
	assert.Contains(t, err.Error(), "insufficient historical data")
}

// TestPredictBudgetExhaustion tests budget exhaustion prediction
func TestPredictBudgetExhaustion(t *testing.T) {
	// Create increasing cost pattern: $10/day growing at 10% per day
	reporter := createTestReporterWithHistoricalData(t, 30, 10.0, "increasing")
	engine := NewForecastEngine(reporter)

	// Budget: $1000, Already spent: $500, Remaining: $500
	forecast, err := engine.PredictBudgetExhaustion("test-project", 1000.0, 500.0)
	require.NoError(t, err)
	require.NotNil(t, forecast)

	// Should predict exhaustion within 90 days
	assert.NotNil(t, forecast.BudgetExhaustionDate, "Should predict exhaustion date")
	assert.Greater(t, forecast.DaysUntilExhaustion, 0, "Should have positive days until exhaustion")
	assert.LessOrEqual(t, forecast.DaysUntilExhaustion, 90, "Should exhaust within 90 days")
	assert.Greater(t, forecast.ExhaustionProbability, 0.0, "Should have non-zero exhaustion probability")

	t.Logf("Budget Exhaustion: days=%d, date=%s, probability=%.2f%%",
		forecast.DaysUntilExhaustion,
		forecast.BudgetExhaustionDate.Format("2006-01-02"),
		forecast.ExhaustionProbability*100)
}

// TestPredictBudgetExhaustion_AlreadyExhausted tests when budget is already exceeded
func TestPredictBudgetExhaustion_AlreadyExhausted(t *testing.T) {
	reporter := createTestReporterWithHistoricalData(t, 30, 10.0, "stable")
	engine := NewForecastEngine(reporter)

	// Budget: $100, Already spent: $150 (over budget)
	forecast, err := engine.PredictBudgetExhaustion("test-project", 100.0, 150.0)
	require.NoError(t, err)
	require.NotNil(t, forecast)

	// Should immediately show exhaustion
	assert.NotNil(t, forecast.BudgetExhaustionDate)
	assert.Equal(t, 0, forecast.DaysUntilExhaustion, "Should be exhausted today")
	assert.Equal(t, 1.0, forecast.ExhaustionProbability, "Should be 100% probability")

	t.Logf("Already Exhausted: days=%d, probability=%.2f%%",
		forecast.DaysUntilExhaustion, forecast.ExhaustionProbability*100)
}

// TestPredictBudgetExhaustion_NeverExhausted tests when budget won't be exhausted
func TestPredictBudgetExhaustion_NeverExhausted(t *testing.T) {
	// Decreasing costs
	reporter := createTestReporterWithHistoricalData(t, 30, 5.0, "decreasing")
	engine := NewForecastEngine(reporter)

	// Budget: $10000, Already spent: $100 (plenty remaining)
	forecast, err := engine.PredictBudgetExhaustion("test-project", 10000.0, 100.0)
	require.NoError(t, err)
	require.NotNil(t, forecast)

	// Should not predict exhaustion within 90 days
	assert.Equal(t, -1, forecast.DaysUntilExhaustion, "Should indicate >90 days")
	assert.Equal(t, 0.0, forecast.ExhaustionProbability, "Should be 0% probability")

	t.Logf("Never Exhausted (>90 days): days=%d, probability=%.2f%%",
		forecast.DaysUntilExhaustion, forecast.ExhaustionProbability*100)
}

// TestForecast_AccuracyMetrics tests model accuracy calculations
func TestForecast_AccuracyMetrics(t *testing.T) {
	// Perfect linear trend
	reporter := createTestReporterWithHistoricalData(t, 30, 10.0, "increasing")
	engine := NewForecastEngine(reporter)

	forecast, err := engine.GenerateForecast("test-project", ForecastModelLinear, 30)
	require.NoError(t, err)

	// Linear model should have good accuracy on linear data
	assert.Greater(t, forecast.ModelAccuracy, 0.8, "Should have high accuracy on linear data")
	assert.Greater(t, forecast.R2Score, 0.8, "R² should be high for linear trend")
	assert.Greater(t, forecast.MeanAbsoluteError, 0.0, "MAE should be positive")
	assert.Greater(t, forecast.RootMeanSquaredError, 0.0, "RMSE should be positive")

	// RMSE should be >= MAE
	assert.GreaterOrEqual(t, forecast.RootMeanSquaredError, forecast.MeanAbsoluteError,
		"RMSE should be >= MAE")

	t.Logf("Accuracy Metrics: R²=%.3f, MAE=%.2f, RMSE=%.2f, accuracy=%.2f%%",
		forecast.R2Score, forecast.MeanAbsoluteError, forecast.RootMeanSquaredError,
		forecast.ModelAccuracy*100)
}

// TestConfidenceIntervals tests confidence interval calculations
func TestConfidenceIntervals(t *testing.T) {
	reporter := createTestReporterWithHistoricalData(t, 30, 10.0, "stable")
	engine := NewForecastEngine(reporter)

	analysis, err := engine.AnalyzeBurnRate("test-project", 30)
	require.NoError(t, err)

	// Test all confidence levels
	levels := []int{90, 95, 99}
	for _, level := range levels {
		ci, exists := analysis.ConfidenceIntervals[level]
		require.True(t, exists, "Should have %d%% confidence interval", level)

		// Bounds should be reasonable
		assert.Greater(t, ci.UpperBound, ci.LowerBound, "Upper bound > lower bound")
		assert.GreaterOrEqual(t, ci.LowerBound, 0.0, "Lower bound should be non-negative")

		// Prediction should be within bounds
		assert.LessOrEqual(t, ci.LowerBound, ci.Prediction, "Prediction >= lower bound")
		assert.GreaterOrEqual(t, ci.UpperBound, ci.Prediction, "Prediction <= upper bound")

		t.Logf("%d%% CI: [%.2f, %.2f], prediction=%.2f",
			level, ci.LowerBound, ci.UpperBound, ci.Prediction)
	}

	// Higher confidence levels should have wider intervals
	ci90 := analysis.ConfidenceIntervals[90]
	ci95 := analysis.ConfidenceIntervals[95]
	ci99 := analysis.ConfidenceIntervals[99]

	width90 := ci90.UpperBound - ci90.LowerBound
	width95 := ci95.UpperBound - ci95.LowerBound
	width99 := ci99.UpperBound - ci99.LowerBound

	assert.Greater(t, width95, width90, "95% CI should be wider than 90%")
	assert.Greater(t, width99, width95, "99% CI should be wider than 95%")
}

// BenchmarkAnalyzeBurnRate benchmarks burn rate analysis
func BenchmarkAnalyzeBurnRate(b *testing.B) {
	reporter := createTestReporterWithHistoricalData(&testing.T{}, 90, 10.0, "increasing")
	engine := NewForecastEngine(reporter)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = engine.AnalyzeBurnRate("test-project", 90)
	}
}

// BenchmarkGenerateForecast_Linear benchmarks linear forecasting
func BenchmarkGenerateForecast_Linear(b *testing.B) {
	reporter := createTestReporterWithHistoricalData(&testing.T{}, 90, 10.0, "increasing")
	engine := NewForecastEngine(reporter)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = engine.GenerateForecast("test-project", ForecastModelLinear, 90)
	}
}

// BenchmarkGenerateForecast_Ensemble benchmarks ensemble forecasting
func BenchmarkGenerateForecast_Ensemble(b *testing.B) {
	reporter := createTestReporterWithHistoricalData(&testing.T{}, 90, 10.0, "stable")
	engine := NewForecastEngine(reporter)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = engine.GenerateForecast("test-project", ForecastModelEnsemble, 90)
	}
}
