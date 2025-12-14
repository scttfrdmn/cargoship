// Package cost provides budget forecasting and burn rate prediction
package cost

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// ForecastModel represents different forecasting algorithms
type ForecastModel string

const (
	// ForecastModelLinear uses simple linear regression
	ForecastModelLinear ForecastModel = "linear"
	// ForecastModelExponential uses exponential smoothing
	ForecastModelExponential ForecastModel = "exponential"
	// ForecastModelMovingAverage uses weighted moving average
	ForecastModelMovingAverage ForecastModel = "moving_average"
	// ForecastModelEnsemble combines multiple models
	ForecastModelEnsemble ForecastModel = "ensemble"
)

// BurnRateAnalysis provides burn rate metrics and predictions
type BurnRateAnalysis struct {
	// Current burn rate metrics
	CurrentDailyRate   float64 `json:"current_daily_rate"`   // $/day
	CurrentWeeklyRate  float64 `json:"current_weekly_rate"`  // $/week
	CurrentMonthlyRate float64 `json:"current_monthly_rate"` // $/month

	// Historical burn rate statistics
	AverageDailyRate float64 `json:"average_daily_rate"`
	MinDailyRate     float64 `json:"min_daily_rate"`
	MaxDailyRate     float64 `json:"max_daily_rate"`
	StdDevDailyRate  float64 `json:"std_dev_daily_rate"`
	Volatility       float64 `json:"volatility"` // Coefficient of variation (stddev/mean)

	// Burn rate trends
	TrendDirection   string  `json:"trend_direction"`   // "increasing", "decreasing", "stable"
	TrendStrength    float64 `json:"trend_strength"`    // 0.0 to 1.0
	AccelerationRate float64 `json:"acceleration_rate"` // Change in burn rate per day

	// Forecasted burn rates
	PredictedDailyRate30Days float64 `json:"predicted_daily_rate_30_days"`
	PredictedDailyRate60Days float64 `json:"predicted_daily_rate_60_days"`
	PredictedDailyRate90Days float64 `json:"predicted_daily_rate_90_days"`

	// Confidence intervals (90%, 95%, 99%)
	ConfidenceIntervals map[int]*ConfidenceInterval `json:"confidence_intervals"`
}

// ConfidenceInterval represents prediction confidence bounds
type ConfidenceInterval struct {
	ConfidenceLevel int     `json:"confidence_level"` // 90, 95, 99
	LowerBound      float64 `json:"lower_bound"`
	UpperBound      float64 `json:"upper_bound"`
	Prediction      float64 `json:"prediction"`
}

// CostForecast represents predicted future costs
type CostForecast struct {
	Model          ForecastModel `json:"model"`
	GeneratedAt    time.Time     `json:"generated_at"`
	ForecastDays   int           `json:"forecast_days"`
	BaseCost       float64       `json:"base_cost"`       // Current total cost
	BaseDate       time.Time     `json:"base_date"`       // Date from which forecast is calculated
	HistoricalDays int           `json:"historical_days"` // Days of historical data used

	// Predicted costs at specific time points
	Predicted7Days  float64 `json:"predicted_7_days"`
	Predicted14Days float64 `json:"predicted_14_days"`
	Predicted30Days float64 `json:"predicted_30_days"`
	Predicted60Days float64 `json:"predicted_60_days"`
	Predicted90Days float64 `json:"predicted_90_days"`

	// Daily predictions (day -> predicted cumulative cost)
	DailyForecasts map[int]float64 `json:"daily_forecasts"`

	// Confidence intervals for key time points
	Confidence7Days  *ConfidenceInterval `json:"confidence_7_days"`
	Confidence30Days *ConfidenceInterval `json:"confidence_30_days"`
	Confidence90Days *ConfidenceInterval `json:"confidence_90_days"`

	// Model performance metrics
	ModelAccuracy        float64 `json:"model_accuracy"`          // 0.0 to 1.0
	MeanAbsoluteError    float64 `json:"mean_absolute_error"`     // MAE
	RootMeanSquaredError float64 `json:"root_mean_squared_error"` // RMSE
	R2Score              float64 `json:"r2_score"`                // Coefficient of determination

	// Budget impact analysis
	BudgetExhaustionDate  *time.Time `json:"budget_exhaustion_date,omitempty"` // When budget will run out
	DaysUntilExhaustion   int        `json:"days_until_exhaustion"`
	ExhaustionProbability float64    `json:"exhaustion_probability"` // 0.0 to 1.0
}

// ForecastEngine generates cost forecasts using multiple models
type ForecastEngine struct {
	reporter *CostReporter
}

// NewForecastEngine creates a new forecast engine
func NewForecastEngine(reporter *CostReporter) *ForecastEngine {
	return &ForecastEngine{
		reporter: reporter,
	}
}

// AnalyzeBurnRate analyzes current and historical burn rates
func (fe *ForecastEngine) AnalyzeBurnRate(projectID string, days int) (*BurnRateAnalysis, error) {
	fe.reporter.mu.RLock()
	defer fe.reporter.mu.RUnlock()

	// Filter records for time period and project
	cutoffDate := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	var records []CostRecord
	for _, r := range fe.reporter.costs {
		if r.Timestamp.After(cutoffDate) {
			if projectID == "" || r.ProjectID == projectID {
				records = append(records, r)
			}
		}
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("no cost records found for burn rate analysis")
	}

	// Sort by timestamp
	sort.Slice(records, func(i, j int) bool {
		return records[i].Timestamp.Before(records[j].Timestamp)
	})

	// Calculate daily costs
	dailyCosts := make(map[string]float64)
	for _, r := range records {
		day := r.Timestamp.Format("2006-01-02")
		dailyCosts[day] += r.Cost
	}

	// Convert to sorted slice
	type dailyCost struct {
		date string
		cost float64
	}
	var costs []dailyCost
	for date, cost := range dailyCosts {
		costs = append(costs, dailyCost{date: date, cost: cost})
	}
	sort.Slice(costs, func(i, j int) bool {
		return costs[i].date < costs[j].date
	})

	// Calculate burn rate statistics
	analysis := &BurnRateAnalysis{
		ConfidenceIntervals: make(map[int]*ConfidenceInterval),
	}

	// Average daily rate
	totalCost := 0.0
	rates := make([]float64, len(costs))
	for i, c := range costs {
		rates[i] = c.cost
		totalCost += c.cost
	}
	analysis.AverageDailyRate = totalCost / float64(len(costs))

	// Current rates (last 1, 7, 30 days)
	if len(costs) > 0 {
		analysis.CurrentDailyRate = costs[len(costs)-1].cost
	}
	if len(costs) >= 7 {
		weekTotal := 0.0
		for i := len(costs) - 7; i < len(costs); i++ {
			weekTotal += costs[i].cost
		}
		analysis.CurrentWeeklyRate = weekTotal
	}
	if len(costs) >= 30 {
		monthTotal := 0.0
		for i := len(costs) - 30; i < len(costs); i++ {
			monthTotal += costs[i].cost
		}
		analysis.CurrentMonthlyRate = monthTotal
	}

	// Min/max rates
	analysis.MinDailyRate = rates[0]
	analysis.MaxDailyRate = rates[0]
	for _, r := range rates {
		if r < analysis.MinDailyRate {
			analysis.MinDailyRate = r
		}
		if r > analysis.MaxDailyRate {
			analysis.MaxDailyRate = r
		}
	}

	// Standard deviation and volatility
	sumSquaredDiff := 0.0
	for _, r := range rates {
		diff := r - analysis.AverageDailyRate
		sumSquaredDiff += diff * diff
	}
	analysis.StdDevDailyRate = math.Sqrt(sumSquaredDiff / float64(len(rates)))
	if analysis.AverageDailyRate > 0 {
		analysis.Volatility = analysis.StdDevDailyRate / analysis.AverageDailyRate
	}

	// Trend analysis using linear regression
	n := float64(len(rates))
	sumX := 0.0
	sumY := 0.0
	sumXY := 0.0
	sumX2 := 0.0
	for i, r := range rates {
		x := float64(i)
		y := r
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	// Calculate slope (trend)
	slope := (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)
	analysis.AccelerationRate = slope

	// Determine trend direction and strength
	if math.Abs(slope) < 0.01*analysis.AverageDailyRate {
		analysis.TrendDirection = "stable"
		analysis.TrendStrength = 0.0
	} else if slope > 0 {
		analysis.TrendDirection = "increasing"
		analysis.TrendStrength = math.Min(1.0, math.Abs(slope)/analysis.AverageDailyRate)
	} else {
		analysis.TrendDirection = "decreasing"
		analysis.TrendStrength = math.Min(1.0, math.Abs(slope)/analysis.AverageDailyRate)
	}

	// Predict future burn rates using linear extrapolation
	intercept := (sumY - slope*sumX) / n
	analysis.PredictedDailyRate30Days = intercept + slope*float64(len(rates)+30)
	analysis.PredictedDailyRate60Days = intercept + slope*float64(len(rates)+60)
	analysis.PredictedDailyRate90Days = intercept + slope*float64(len(rates)+90)

	// Calculate confidence intervals (using normal distribution assumption)
	// 90% = 1.645 * stddev, 95% = 1.96 * stddev, 99% = 2.576 * stddev
	confidenceLevels := map[int]float64{
		90: 1.645,
		95: 1.96,
		99: 2.576,
	}

	for level, z := range confidenceLevels {
		margin := z * analysis.StdDevDailyRate
		analysis.ConfidenceIntervals[level] = &ConfidenceInterval{
			ConfidenceLevel: level,
			LowerBound:      math.Max(0, analysis.AverageDailyRate-margin),
			UpperBound:      analysis.AverageDailyRate + margin,
			Prediction:      analysis.AverageDailyRate,
		}
	}

	return analysis, nil
}

// GenerateForecast creates a cost forecast using the specified model
func (fe *ForecastEngine) GenerateForecast(projectID string, model ForecastModel, days int) (*CostForecast, error) {
	switch model {
	case ForecastModelLinear:
		return fe.generateLinearForecast(projectID, days)
	case ForecastModelExponential:
		return fe.generateExponentialForecast(projectID, days)
	case ForecastModelMovingAverage:
		return fe.generateMovingAverageForecast(projectID, days)
	case ForecastModelEnsemble:
		return fe.generateEnsembleForecast(projectID, days)
	default:
		return nil, fmt.Errorf("unsupported forecast model: %s", model)
	}
}

// generateLinearForecast uses linear regression for forecasting
func (fe *ForecastEngine) generateLinearForecast(projectID string, forecastDays int) (*CostForecast, error) {
	fe.reporter.mu.RLock()
	defer fe.reporter.mu.RUnlock()

	// Get historical data (last 90 days or all available)
	historicalDays := 90
	cutoffDate := time.Now().Add(-time.Duration(historicalDays) * 24 * time.Hour)
	var records []CostRecord
	for _, r := range fe.reporter.costs {
		if r.Timestamp.After(cutoffDate) {
			if projectID == "" || r.ProjectID == projectID {
				records = append(records, r)
			}
		}
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("no historical cost records found")
	}

	// Sort by timestamp
	sort.Slice(records, func(i, j int) bool {
		return records[i].Timestamp.Before(records[j].Timestamp)
	})

	// Calculate cumulative daily costs
	dailyCosts := make(map[string]float64)
	for _, r := range records {
		day := r.Timestamp.Format("2006-01-02")
		dailyCosts[day] += r.Cost
	}

	// Convert to sorted cumulative costs
	type dayCost struct {
		day            string
		cumulativeCost float64
	}
	var costs []dayCost
	cumulative := 0.0
	for _, r := range records {
		day := r.Timestamp.Format("2006-01-02")
		// Check if we already processed this day
		found := false
		for _, dc := range costs {
			if dc.day == day {
				found = true
				break
			}
		}
		if !found {
			cumulative += dailyCosts[day]
			costs = append(costs, dayCost{day: day, cumulativeCost: cumulative})
		}
	}

	if len(costs) < 2 {
		return nil, fmt.Errorf("insufficient historical data for forecasting (need at least 2 days)")
	}

	// Linear regression: y = mx + b
	// where x = day index, y = cumulative cost
	n := float64(len(costs))
	sumX := 0.0
	sumY := 0.0
	sumXY := 0.0
	sumX2 := 0.0
	for i, c := range costs {
		x := float64(i)
		y := c.cumulativeCost
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	slope := (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)
	intercept := (sumY - slope*sumX) / n

	// Create forecast
	forecast := &CostForecast{
		Model:          ForecastModelLinear,
		GeneratedAt:    time.Now(),
		ForecastDays:   forecastDays,
		BaseCost:       costs[len(costs)-1].cumulativeCost,
		BaseDate:       time.Now(),
		HistoricalDays: len(costs),
		DailyForecasts: make(map[int]float64),
	}

	// Generate daily forecasts
	baseIndex := float64(len(costs) - 1)
	for day := 1; day <= forecastDays; day++ {
		predictedCost := intercept + slope*float64(baseIndex+float64(day))
		forecast.DailyForecasts[day] = predictedCost
	}

	// Set specific time point predictions
	if forecastDays >= 7 {
		forecast.Predicted7Days = intercept + slope*(baseIndex+7)
	}
	if forecastDays >= 14 {
		forecast.Predicted14Days = intercept + slope*(baseIndex+14)
	}
	if forecastDays >= 30 {
		forecast.Predicted30Days = intercept + slope*(baseIndex+30)
	}
	if forecastDays >= 60 {
		forecast.Predicted60Days = intercept + slope*(baseIndex+60)
	}
	if forecastDays >= 90 {
		forecast.Predicted90Days = intercept + slope*(baseIndex+90)
	}

	// Calculate confidence intervals
	// Standard error of prediction
	sumSquaredResiduals := 0.0
	for i, c := range costs {
		predicted := intercept + slope*float64(i)
		residual := c.cumulativeCost - predicted
		sumSquaredResiduals += residual * residual
	}
	stdError := math.Sqrt(sumSquaredResiduals / (n - 2))

	// Confidence intervals at key time points (95% confidence, z = 1.96)
	z := 1.96
	if forecastDays >= 7 {
		margin := z * stdError * math.Sqrt(1+1/n+7*7/(n*sumX2/n))
		forecast.Confidence7Days = &ConfidenceInterval{
			ConfidenceLevel: 95,
			Prediction:      forecast.Predicted7Days,
			LowerBound:      math.Max(0, forecast.Predicted7Days-margin),
			UpperBound:      forecast.Predicted7Days + margin,
		}
	}
	if forecastDays >= 30 {
		margin := z * stdError * math.Sqrt(1+1/n+30*30/(n*sumX2/n))
		forecast.Confidence30Days = &ConfidenceInterval{
			ConfidenceLevel: 95,
			Prediction:      forecast.Predicted30Days,
			LowerBound:      math.Max(0, forecast.Predicted30Days-margin),
			UpperBound:      forecast.Predicted30Days + margin,
		}
	}
	if forecastDays >= 90 {
		margin := z * stdError * math.Sqrt(1+1/n+90*90/(n*sumX2/n))
		forecast.Confidence90Days = &ConfidenceInterval{
			ConfidenceLevel: 95,
			Prediction:      forecast.Predicted90Days,
			LowerBound:      math.Max(0, forecast.Predicted90Days-margin),
			UpperBound:      forecast.Predicted90Days + margin,
		}
	}

	// Calculate R² score (coefficient of determination)
	meanY := sumY / n
	ssTotal := 0.0
	ssResidual := 0.0
	for i, c := range costs {
		predicted := intercept + slope*float64(i)
		diff1 := c.cumulativeCost - meanY
		diff2 := c.cumulativeCost - predicted
		ssTotal += diff1 * diff1
		ssResidual += diff2 * diff2
	}
	if ssTotal > 0 {
		forecast.R2Score = 1 - (ssResidual / ssTotal)
		forecast.ModelAccuracy = math.Max(0, forecast.R2Score) // Clamp to 0-1
	}

	// MAE and RMSE
	sumAbsError := 0.0
	sumSquaredError := 0.0
	for i, c := range costs {
		predicted := intercept + slope*float64(i)
		error := math.Abs(c.cumulativeCost - predicted)
		sumAbsError += error
		sumSquaredError += error * error
	}
	forecast.MeanAbsoluteError = sumAbsError / n
	forecast.RootMeanSquaredError = math.Sqrt(sumSquaredError / n)

	return forecast, nil
}

// generateExponentialForecast uses exponential smoothing
func (fe *ForecastEngine) generateExponentialForecast(projectID string, forecastDays int) (*CostForecast, error) {
	// TODO: Implement exponential smoothing (alpha = 0.3 for level, beta = 0.1 for trend)
	// For now, fall back to linear forecast
	forecast, err := fe.generateLinearForecast(projectID, forecastDays)
	if err != nil {
		return nil, err
	}
	forecast.Model = ForecastModelExponential
	return forecast, nil
}

// generateMovingAverageForecast uses weighted moving average
func (fe *ForecastEngine) generateMovingAverageForecast(projectID string, forecastDays int) (*CostForecast, error) {
	// TODO: Implement weighted moving average (window = 7 days, exponential weights)
	// For now, fall back to linear forecast
	forecast, err := fe.generateLinearForecast(projectID, forecastDays)
	if err != nil {
		return nil, err
	}
	forecast.Model = ForecastModelMovingAverage
	return forecast, nil
}

// generateEnsembleForecast combines multiple models
func (fe *ForecastEngine) generateEnsembleForecast(projectID string, forecastDays int) (*CostForecast, error) {
	// Generate forecasts from all models
	linearForecast, err := fe.generateLinearForecast(projectID, forecastDays)
	if err != nil {
		return nil, err
	}

	exponentialForecast, _ := fe.generateExponentialForecast(projectID, forecastDays)
	movingAvgForecast, _ := fe.generateMovingAverageForecast(projectID, forecastDays)

	// Combine predictions using weighted average (linear=50%, exponential=30%, moving_avg=20%)
	ensemble := &CostForecast{
		Model:          ForecastModelEnsemble,
		GeneratedAt:    time.Now(),
		ForecastDays:   forecastDays,
		BaseCost:       linearForecast.BaseCost,
		BaseDate:       linearForecast.BaseDate,
		HistoricalDays: linearForecast.HistoricalDays,
		DailyForecasts: make(map[int]float64),
	}

	// Weighted average of predictions
	w1, w2, w3 := 0.5, 0.3, 0.2
	for day := 1; day <= forecastDays; day++ {
		prediction := w1 * linearForecast.DailyForecasts[day]
		if exponentialForecast != nil {
			prediction += w2 * exponentialForecast.DailyForecasts[day]
		}
		if movingAvgForecast != nil {
			prediction += w3 * movingAvgForecast.DailyForecasts[day]
		}
		ensemble.DailyForecasts[day] = prediction
	}

	// Set specific time point predictions
	if forecastDays >= 7 {
		ensemble.Predicted7Days = ensemble.DailyForecasts[7]
	}
	if forecastDays >= 14 {
		ensemble.Predicted14Days = ensemble.DailyForecasts[14]
	}
	if forecastDays >= 30 {
		ensemble.Predicted30Days = ensemble.DailyForecasts[30]
	}
	if forecastDays >= 60 {
		ensemble.Predicted60Days = ensemble.DailyForecasts[60]
	}
	if forecastDays >= 90 {
		ensemble.Predicted90Days = ensemble.DailyForecasts[90]
	}

	// Use linear forecast's confidence intervals as baseline
	ensemble.Confidence7Days = linearForecast.Confidence7Days
	ensemble.Confidence30Days = linearForecast.Confidence30Days
	ensemble.Confidence90Days = linearForecast.Confidence90Days

	// Average model accuracy metrics
	ensemble.ModelAccuracy = linearForecast.ModelAccuracy
	ensemble.MeanAbsoluteError = linearForecast.MeanAbsoluteError
	ensemble.RootMeanSquaredError = linearForecast.RootMeanSquaredError
	ensemble.R2Score = linearForecast.R2Score

	return ensemble, nil
}

// PredictBudgetExhaustion calculates when a budget will be exhausted
func (fe *ForecastEngine) PredictBudgetExhaustion(projectID string, currentBudget, spentAmount float64) (*CostForecast, error) {
	// Generate 90-day forecast
	forecast, err := fe.GenerateForecast(projectID, ForecastModelLinear, 90)
	if err != nil {
		return nil, err
	}

	remainingBudget := currentBudget - spentAmount
	if remainingBudget <= 0 {
		// Budget already exhausted
		exhaustionDate := time.Now()
		forecast.BudgetExhaustionDate = &exhaustionDate
		forecast.DaysUntilExhaustion = 0
		forecast.ExhaustionProbability = 1.0
		return forecast, nil
	}

	// Find day when forecast exceeds remaining budget
	for day := 1; day <= 90; day++ {
		predictedCost := forecast.DailyForecasts[day] - forecast.BaseCost
		if predictedCost >= remainingBudget {
			exhaustionDate := time.Now().Add(time.Duration(day) * 24 * time.Hour)
			forecast.BudgetExhaustionDate = &exhaustionDate
			forecast.DaysUntilExhaustion = day

			// Calculate exhaustion probability based on confidence intervals
			if forecast.Confidence30Days != nil && day <= 30 {
				margin := forecast.Confidence30Days.UpperBound - forecast.Confidence30Days.Prediction
				// Probability that cost exceeds budget (using normal distribution approximation)
				z := (remainingBudget - predictedCost) / margin
				forecast.ExhaustionProbability = 0.5 + 0.5*math.Erf(z/math.Sqrt(2))
			} else {
				forecast.ExhaustionProbability = 0.5 // Default to 50% if no confidence interval
			}
			return forecast, nil
		}
	}

	// Budget won't be exhausted within 90 days
	forecast.DaysUntilExhaustion = -1 // Indicates > 90 days
	forecast.ExhaustionProbability = 0.0
	return forecast, nil
}
