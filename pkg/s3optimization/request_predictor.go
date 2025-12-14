package s3optimization

import (
	"sort"
	"sync"
	"time"
)

// RequestPredictor predicts future S3 requests based on access patterns.
type RequestPredictor struct {
	config                *PrefetchConfig
	patterns              map[string]*AccessPattern
	predictionModels      map[string]*PredictionModel
	temporalPredictor     *TemporalPredictor
	sequentialPredictor   *SequentialPredictor
	cyclicPredictor       *CyclicPredictor
	burstPredictor        *BurstPredictor
	machineLearningEngine *MLPredictionEngine
	mu                    sync.RWMutex
}

// RequestPrediction represents a predicted future request.
type RequestPrediction struct {
	Key           string        `json:"key"`
	Bucket        string        `json:"bucket"`
	PredictedTime time.Time     `json:"predicted_time"`
	Confidence    float64       `json:"confidence"`
	EstimatedSize int64         `json:"estimated_size"`
	PatternType   PatternType   `json:"pattern_type"`
	PatternSource string        `json:"pattern_source"`
	TimeToAccess  time.Duration `json:"time_to_access"`
}

// PredictionModel represents a prediction model for a specific pattern type.
type PredictionModel struct {
	PatternType      PatternType
	Accuracy         float64
	PredictionCount  int64
	SuccessfulCount  int64
	LastUpdated      time.Time
	WeightFactor     float64
	ConfidenceAdjust float64
}

// NewRequestPredictor creates a new request predictor.
func NewRequestPredictor(config *PrefetchConfig) *RequestPredictor {
	rp := &RequestPredictor{
		config:                config,
		patterns:              make(map[string]*AccessPattern),
		predictionModels:      make(map[string]*PredictionModel),
		temporalPredictor:     NewTemporalPredictor(config),
		sequentialPredictor:   NewSequentialPredictor(config),
		cyclicPredictor:       NewCyclicPredictor(config),
		burstPredictor:        NewBurstPredictor(config),
		machineLearningEngine: NewMLPredictionEngine(config),
	}

	// Initialize prediction models
	rp.initializePredictionModels()

	return rp
}

// PredictNextRequests predicts the next likely requests for a given key.
func (rp *RequestPredictor) PredictNextRequests(currentKey string, maxPredictions int) []*RequestPrediction {
	rp.mu.RLock()
	defer rp.mu.RUnlock()

	var allPredictions []*RequestPrediction

	// Get predictions from different predictors with nil checks
	temporalPreds := rp.temporalPredictor.Predict(currentKey, rp.patterns)
	if temporalPreds != nil {
		allPredictions = append(allPredictions, temporalPreds...)
	}

	sequentialPreds := rp.sequentialPredictor.Predict(currentKey, rp.patterns)
	if sequentialPreds != nil {
		allPredictions = append(allPredictions, sequentialPreds...)
	}

	cyclicPreds := rp.cyclicPredictor.Predict(currentKey, rp.patterns)
	if cyclicPreds != nil {
		allPredictions = append(allPredictions, cyclicPreds...)
	}

	burstPreds := rp.burstPredictor.Predict(currentKey, rp.patterns)
	if burstPreds != nil {
		allPredictions = append(allPredictions, burstPreds...)
	}

	mlPreds := rp.machineLearningEngine.Predict(currentKey, rp.patterns)
	if mlPreds != nil {
		allPredictions = append(allPredictions, mlPreds...)
	}

	// Deduplicate and enhance predictions
	deduplicatedPreds := rp.deduplicatePredictions(allPredictions)
	enhancedPreds := rp.enhancePredictions(deduplicatedPreds)

	// Sort by confidence and predicted time
	sort.Slice(enhancedPreds, func(i, j int) bool {
		// First sort by confidence (higher is better)
		if enhancedPreds[i].Confidence != enhancedPreds[j].Confidence {
			return enhancedPreds[i].Confidence > enhancedPreds[j].Confidence
		}
		// Then by predicted time (sooner is better)
		return enhancedPreds[i].PredictedTime.Before(enhancedPreds[j].PredictedTime)
	})

	// Limit to max predictions
	if len(enhancedPreds) > maxPredictions {
		enhancedPreds = enhancedPreds[:maxPredictions]
	}

	// Apply confidence threshold
	var filteredPreds []*RequestPrediction
	for _, pred := range enhancedPreds {
		if pred.Confidence >= rp.config.MinPatternConfidence {
			filteredPreds = append(filteredPreds, pred)
		}
	}

	// Ensure we never return nil - return empty slice if no predictions
	if filteredPreds == nil {
		return []*RequestPrediction{}
	}
	return filteredPreds
}

// UpdatePatterns updates the predictor with new access patterns.
func (rp *RequestPredictor) UpdatePatterns(patterns map[string]*AccessPattern) {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	rp.patterns = patterns

	// Update individual predictors
	rp.temporalPredictor.UpdatePatterns(patterns)
	rp.sequentialPredictor.UpdatePatterns(patterns)
	rp.cyclicPredictor.UpdatePatterns(patterns)
	rp.burstPredictor.UpdatePatterns(patterns)
	rp.machineLearningEngine.UpdatePatterns(patterns)
}

// RecordPredictionResult records the result of a prediction.
func (rp *RequestPredictor) RecordPredictionResult(prediction *RequestPrediction, actualAccessTime time.Time, wasAccessed bool) {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	// Update prediction model accuracy
	modelKey := prediction.PatternType.String()
	if model, exists := rp.predictionModels[modelKey]; exists {
		model.PredictionCount++
		if wasAccessed {
			model.SuccessfulCount++

			// Calculate time accuracy
			timeDiff := actualAccessTime.Sub(prediction.PredictedTime)
			if timeDiff < 0 {
				timeDiff = -timeDiff
			}

			// Update confidence adjustment based on timing accuracy
			if timeDiff < time.Minute*5 { // Very accurate timing
				model.ConfidenceAdjust += 0.05
			} else if timeDiff < time.Minute*15 { // Reasonably accurate
				model.ConfidenceAdjust += 0.02
			} else {
				model.ConfidenceAdjust -= 0.02 // Poor timing accuracy
			}
		} else {
			model.ConfidenceAdjust -= 0.03 // Prediction was wrong
		}

		// Update overall accuracy
		model.Accuracy = float64(model.SuccessfulCount) / float64(model.PredictionCount)
		model.LastUpdated = time.Now()

		// Clamp confidence adjustment
		if model.ConfidenceAdjust > 0.2 {
			model.ConfidenceAdjust = 0.2
		} else if model.ConfidenceAdjust < -0.3 {
			model.ConfidenceAdjust = -0.3
		}
	}
}

// GetPredictionAccuracy returns prediction accuracy metrics.
func (rp *RequestPredictor) GetPredictionAccuracy() map[string]float64 {
	rp.mu.RLock()
	defer rp.mu.RUnlock()

	accuracy := make(map[string]float64)
	for modelKey, model := range rp.predictionModels {
		accuracy[modelKey] = model.Accuracy
	}

	return accuracy
}

// initializePredictionModels initializes prediction models for different pattern types.
func (rp *RequestPredictor) initializePredictionModels() {
	patternTypes := []PatternType{
		PatternSequential,
		PatternTemporal,
		PatternCyclic,
		PatternBurst,
	}

	for _, patternType := range patternTypes {
		model := &PredictionModel{
			PatternType:      patternType,
			Accuracy:         0.5, // Start with neutral accuracy
			WeightFactor:     1.0,
			ConfidenceAdjust: 0.0,
			LastUpdated:      time.Now(),
		}
		rp.predictionModels[patternType.String()] = model
	}
}

// deduplicatePredictions removes duplicate predictions and merges similar ones.
func (rp *RequestPredictor) deduplicatePredictions(predictions []*RequestPrediction) []*RequestPrediction {
	predMap := make(map[string]*RequestPrediction)

	for _, pred := range predictions {
		if existing, exists := predMap[pred.Key]; exists {
			// Merge predictions for the same key
			merged := rp.mergePredictions(existing, pred)
			predMap[pred.Key] = merged
		} else {
			predMap[pred.Key] = pred
		}
	}

	// Convert back to slice
	var result []*RequestPrediction
	for _, pred := range predMap {
		result = append(result, pred)
	}

	return result
}

// mergePredictions merges two predictions for the same key.
func (rp *RequestPredictor) mergePredictions(pred1, pred2 *RequestPrediction) *RequestPrediction {
	// Use the prediction with higher confidence as base
	var base, other *RequestPrediction
	if pred1.Confidence >= pred2.Confidence {
		base, other = pred1, pred2
	} else {
		base, other = pred2, pred1
	}

	// Create merged prediction
	merged := &RequestPrediction{
		Key:           base.Key,
		Bucket:        base.Bucket,
		PatternType:   base.PatternType,
		PatternSource: base.PatternSource + "+" + other.PatternSource,
	}

	// Weighted average of confidence
	weight1 := base.Confidence
	weight2 := other.Confidence
	totalWeight := weight1 + weight2

	merged.Confidence = (base.Confidence*weight1 + other.Confidence*weight2) / totalWeight

	// Choose earlier predicted time
	if base.PredictedTime.Before(other.PredictedTime) {
		merged.PredictedTime = base.PredictedTime
	} else {
		merged.PredictedTime = other.PredictedTime
	}

	// Use larger estimated size
	if base.EstimatedSize > other.EstimatedSize {
		merged.EstimatedSize = base.EstimatedSize
	} else {
		merged.EstimatedSize = other.EstimatedSize
	}

	merged.TimeToAccess = time.Until(merged.PredictedTime)

	return merged
}

// enhancePredictions enhances predictions with additional information.
func (rp *RequestPredictor) enhancePredictions(predictions []*RequestPrediction) []*RequestPrediction {
	for _, pred := range predictions {
		// Adjust confidence based on model accuracy
		modelKey := pred.PatternType.String()
		if model, exists := rp.predictionModels[modelKey]; exists {
			// Apply confidence adjustment
			pred.Confidence += model.ConfidenceAdjust

			// Weight by model accuracy
			pred.Confidence *= model.Accuracy

			// Clamp confidence to valid range
			if pred.Confidence > 1.0 {
				pred.Confidence = 1.0
			} else if pred.Confidence < 0.0 {
				pred.Confidence = 0.0
			}
		}

		// Update time to access
		pred.TimeToAccess = time.Until(pred.PredictedTime)

		// Estimate size if not set
		if pred.EstimatedSize == 0 {
			pred.EstimatedSize = rp.estimateObjectSize(pred.Key)
		}

		// Set bucket if not set
		if pred.Bucket == "" {
			pred.Bucket = rp.extractBucketFromKey(pred.Key)
		}
	}

	return predictions
}

// estimateObjectSize estimates the size of an object based on historical data.
func (rp *RequestPredictor) estimateObjectSize(key string) int64 {
	// Default size estimation (would use historical data in real implementation)
	defaultSize := int64(1024 * 1024) // 1MB default

	// Simple heuristic based on file extension
	if len(key) > 4 {
		ext := key[len(key)-4:]
		switch ext {
		case ".jpg", ".png", ".gif":
			return defaultSize * 5 // Image files ~5MB
		case ".mp4", ".avi", ".mov":
			return defaultSize * 100 // Video files ~100MB
		case ".pdf", ".doc":
			return defaultSize * 2 // Document files ~2MB
		case ".zip", ".tar", ".gz":
			return defaultSize * 20 // Archive files ~20MB
		}
	}

	return defaultSize
}

// extractBucketFromKey extracts bucket name from a key (simple heuristic).
func (rp *RequestPredictor) extractBucketFromKey(key string) string {
	// Simple extraction - would use more sophisticated logic in real implementation
	if len(key) > 0 && key[0] == '/' {
		parts := splitString(key[1:], "/")
		if len(parts) > 0 {
			return parts[0]
		}
	}
	return "default-bucket"
}

// TemporalPredictor predicts based on temporal patterns.
type TemporalPredictor struct {
	config *PrefetchConfig
}

func NewTemporalPredictor(config *PrefetchConfig) *TemporalPredictor {
	return &TemporalPredictor{config: config}
}

func (tp *TemporalPredictor) Predict(currentKey string, patterns map[string]*AccessPattern) []*RequestPrediction {
	var predictions []*RequestPrediction

	for _, pattern := range patterns {
		if pattern.Type == PatternTemporal {
			for _, key := range pattern.Keys {
				if key == currentKey {
					// Predict other keys in the temporal pattern
					for _, otherKey := range pattern.Keys {
						if otherKey != currentKey {
							prediction := &RequestPrediction{
								Key:           otherKey,
								PredictedTime: time.Now().Add(time.Minute * 2), // Predict access in 2 minutes
								Confidence:    pattern.Confidence,
								PatternType:   PatternTemporal,
								PatternSource: "temporal",
							}
							predictions = append(predictions, prediction)
						}
					}
					break
				}
			}
		}
	}

	return predictions
}

func (tp *TemporalPredictor) UpdatePatterns(patterns map[string]*AccessPattern) {
	// Update logic for temporal predictor
}

// SequentialPredictor predicts based on sequential patterns.
type SequentialPredictor struct {
	config *PrefetchConfig
}

func NewSequentialPredictor(config *PrefetchConfig) *SequentialPredictor {
	return &SequentialPredictor{config: config}
}

func (sp *SequentialPredictor) Predict(currentKey string, patterns map[string]*AccessPattern) []*RequestPrediction {
	var predictions []*RequestPrediction

	for _, pattern := range patterns {
		if pattern.Type == PatternSequential {
			for i, key := range pattern.Keys {
				if key == currentKey && i < len(pattern.Keys)-1 {
					// Predict next key in sequence
					nextKey := pattern.Keys[i+1]
					prediction := &RequestPrediction{
						Key:           nextKey,
						PredictedTime: time.Now().Add(pattern.AverageInterval),
						Confidence:    pattern.Confidence,
						PatternType:   PatternSequential,
						PatternSource: "sequential",
					}
					predictions = append(predictions, prediction)
				}
			}
		}
	}

	return predictions
}

func (sp *SequentialPredictor) UpdatePatterns(patterns map[string]*AccessPattern) {
	// Update logic for sequential predictor
}

// CyclicPredictor predicts based on cyclic patterns.
type CyclicPredictor struct {
	config *PrefetchConfig
}

func NewCyclicPredictor(config *PrefetchConfig) *CyclicPredictor {
	return &CyclicPredictor{config: config}
}

func (cp *CyclicPredictor) Predict(currentKey string, patterns map[string]*AccessPattern) []*RequestPrediction {
	var predictions []*RequestPrediction

	for _, pattern := range patterns {
		if pattern.Type == PatternCyclic {
			for _, key := range pattern.Keys {
				if key == currentKey {
					// Predict next cycle
					prediction := &RequestPrediction{
						Key:           currentKey, // Same key in cycle
						PredictedTime: time.Now().Add(pattern.AverageInterval),
						Confidence:    pattern.Confidence,
						PatternType:   PatternCyclic,
						PatternSource: "cyclic",
					}
					predictions = append(predictions, prediction)
				}
			}
		}
	}

	return predictions
}

func (cp *CyclicPredictor) UpdatePatterns(patterns map[string]*AccessPattern) {
	// Update logic for cyclic predictor
}

// BurstPredictor predicts based on burst patterns.
type BurstPredictor struct {
	config *PrefetchConfig
}

func NewBurstPredictor(config *PrefetchConfig) *BurstPredictor {
	return &BurstPredictor{config: config}
}

func (bp *BurstPredictor) Predict(currentKey string, patterns map[string]*AccessPattern) []*RequestPrediction {
	var predictions []*RequestPrediction

	for _, pattern := range patterns {
		if pattern.Type == PatternBurst {
			for _, key := range pattern.Keys {
				if key == currentKey {
					// Predict other keys in burst
					for _, otherKey := range pattern.Keys {
						if otherKey != currentKey {
							prediction := &RequestPrediction{
								Key:           otherKey,
								PredictedTime: time.Now().Add(time.Second * 30), // Burst within 30 seconds
								Confidence:    pattern.Confidence,
								PatternType:   PatternBurst,
								PatternSource: "burst",
							}
							predictions = append(predictions, prediction)
						}
					}
					break
				}
			}
		}
	}

	return predictions
}

func (bp *BurstPredictor) UpdatePatterns(patterns map[string]*AccessPattern) {
	// Update logic for burst predictor
}

// MLPredictionEngine provides machine learning based predictions.
type MLPredictionEngine struct {
	config        *PrefetchConfig
	featureVector map[string]float64
	modelWeights  map[string]float64
	learningRate  float64
}

func NewMLPredictionEngine(config *PrefetchConfig) *MLPredictionEngine {
	return &MLPredictionEngine{
		config:        config,
		featureVector: make(map[string]float64),
		modelWeights:  make(map[string]float64),
		learningRate:  config.LearningRate,
	}
}

func (ml *MLPredictionEngine) Predict(currentKey string, patterns map[string]*AccessPattern) []*RequestPrediction {
	var predictions []*RequestPrediction

	// Extract features
	features := ml.extractFeatures(currentKey, patterns)

	// Generate ML-based predictions
	for key, score := range ml.scorePredictions(features) {
		if score > 0.5 && key != currentKey {
			prediction := &RequestPrediction{
				Key:           key,
				PredictedTime: time.Now().Add(time.Minute * 5), // ML prediction window
				Confidence:    score,
				PatternType:   PatternRandom, // ML-based prediction
				PatternSource: "ml",
			}
			predictions = append(predictions, prediction)
		}
	}

	return predictions
}

func (ml *MLPredictionEngine) UpdatePatterns(patterns map[string]*AccessPattern) {
	// Update ML model with new patterns
	ml.updateModelWeights(patterns)
}

func (ml *MLPredictionEngine) extractFeatures(currentKey string, patterns map[string]*AccessPattern) map[string]float64 {
	features := make(map[string]float64)

	// Basic features
	features["key_length"] = float64(len(currentKey))
	features["pattern_count"] = float64(len(patterns))

	// Pattern-based features
	var sequentialScore, temporalScore, cyclicScore, burstScore float64
	for _, pattern := range patterns {
		for _, key := range pattern.Keys {
			if key == currentKey {
				switch pattern.Type {
				case PatternSequential:
					sequentialScore += pattern.Confidence
				case PatternTemporal:
					temporalScore += pattern.Confidence
				case PatternCyclic:
					cyclicScore += pattern.Confidence
				case PatternBurst:
					burstScore += pattern.Confidence
				}
			}
		}
	}

	features["sequential_score"] = sequentialScore
	features["temporal_score"] = temporalScore
	features["cyclic_score"] = cyclicScore
	features["burst_score"] = burstScore

	return features
}

func (ml *MLPredictionEngine) scorePredictions(features map[string]float64) map[string]float64 {
	scores := make(map[string]float64)

	// Simple linear model (would be more sophisticated in real implementation)
	baseScore := 0.3

	// Apply feature weights
	for feature, value := range features {
		if weight, exists := ml.modelWeights[feature]; exists {
			baseScore += value * weight
		}
	}

	// Generate scores for potential keys (simplified)
	candidates := []string{"key1", "key2", "key3"} // Would be derived from patterns
	for _, candidate := range candidates {
		scores[candidate] = baseScore + randomFloat()*0.2 // Add some variation
	}

	return scores
}

func (ml *MLPredictionEngine) updateModelWeights(patterns map[string]*AccessPattern) {
	// Update weights based on pattern performance (simplified implementation)
	for _, pattern := range patterns {
		weightKey := pattern.Type.String() + "_weight"
		if _, exists := ml.modelWeights[weightKey]; !exists {
			ml.modelWeights[weightKey] = 0.1
		}

		// Adjust weight based on pattern confidence
		adjustment := (pattern.Confidence - 0.5) * ml.learningRate
		ml.modelWeights[weightKey] += adjustment

		// Clamp weights
		if ml.modelWeights[weightKey] > 1.0 {
			ml.modelWeights[weightKey] = 1.0
		} else if ml.modelWeights[weightKey] < -1.0 {
			ml.modelWeights[weightKey] = -1.0
		}
	}
}

// Helper functions
func splitString(s, sep string) []string {
	if s == "" {
		return []string{}
	}

	var parts []string
	current := ""

	for _, char := range s {
		if string(char) == sep {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(char)
		}
	}

	if current != "" {
		parts = append(parts, current)
	}

	return parts
}

func randomFloat() float64 {
	// Simple pseudo-random number generator
	return 0.5 // Simplified for this implementation
}
