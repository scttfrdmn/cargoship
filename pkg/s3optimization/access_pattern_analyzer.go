package s3optimization

import (
	"sort"
	"sync"
	"time"
)

// AccessPatternAnalyzer analyzes S3 access patterns to predict future requests.
type AccessPatternAnalyzer struct {
	config          *PrefetchConfig
	accessHistory   map[string]*AccessHistory
	patterns        map[string]*AccessPattern
	sequentialSets  map[string]*SequentialSet
	temporalSets    map[string]*TemporalSet
	mu              sync.RWMutex
	
	// Analysis parameters
	minSequenceLength int
	maxPatternAge     time.Duration
	confidenceThreshold float64
}

// AccessHistory tracks access history for a specific key.
type AccessHistory struct {
	Key             string
	AccessTimes     []time.Time
	AccessCount     int64
	FirstAccess     time.Time
	LastAccess      time.Time
	AverageInterval time.Duration
	AccessFrequency float64 // Accesses per hour
}

// AccessPattern represents a detected access pattern.
type AccessPattern struct {
	Type            PatternType
	Keys            []string
	Confidence      float64
	PredictedNext   []string
	AverageInterval time.Duration
	LastUpdated     time.Time
	UsageCount      int64
}

// PatternType defines the type of access pattern.
type PatternType int

const (
	PatternSequential PatternType = iota // A -> B -> C -> D
	PatternTemporal                      // Accessed together in time windows
	PatternCyclic                        // Repeating cycles A -> B -> A -> B
	PatternBurst                         // Burst access to related objects
	PatternRandom                        // No clear pattern
)

// String returns the string representation of PatternType.
func (pt PatternType) String() string {
	switch pt {
	case PatternSequential:
		return "sequential"
	case PatternTemporal:
		return "temporal"
	case PatternCyclic:
		return "cyclic"
	case PatternBurst:
		return "burst"
	case PatternRandom:
		return "random"
	default:
		return "unknown"
	}
}

// SequentialSet represents a sequence of related objects.
type SequentialSet struct {
	Name       string
	Sequence   []string
	Confidence float64
	LastSeen   time.Time
	Count      int64
}

// TemporalSet represents objects accessed within time windows.
type TemporalSet struct {
	Name        string
	Objects     []string
	TimeWindow  time.Duration
	Confidence  float64
	LastSeen    time.Time
	Count       int64
}

// NewAccessPatternAnalyzer creates a new access pattern analyzer.
func NewAccessPatternAnalyzer(config *PrefetchConfig) *AccessPatternAnalyzer {
	return &AccessPatternAnalyzer{
		config:              config,
		accessHistory:       make(map[string]*AccessHistory),
		patterns:            make(map[string]*AccessPattern),
		sequentialSets:      make(map[string]*SequentialSet),
		temporalSets:        make(map[string]*TemporalSet),
		minSequenceLength:   3,
		maxPatternAge:       time.Hour * 24,
		confidenceThreshold: config.MinPatternConfidence,
	}
}

// RecordAccess records an access to a specific key.
func (apa *AccessPatternAnalyzer) RecordAccess(key string, accessTime time.Time) {
	apa.mu.Lock()
	defer apa.mu.Unlock()
	
	// Update or create access history
	history, exists := apa.accessHistory[key]
	if !exists {
		history = &AccessHistory{
			Key:         key,
			AccessTimes: make([]time.Time, 0),
			FirstAccess: accessTime,
		}
		apa.accessHistory[key] = history
	}
	
	// Add new access time
	history.AccessTimes = append(history.AccessTimes, accessTime)
	history.AccessCount++
	history.LastAccess = accessTime
	
	// Limit history size
	maxHistorySize := 100
	if len(history.AccessTimes) > maxHistorySize {
		history.AccessTimes = history.AccessTimes[len(history.AccessTimes)-maxHistorySize:]
	}
	
	// Update statistics
	apa.updateAccessStatistics(history)
	
	// Trigger pattern detection
	apa.detectPatternsForKey(key, accessTime)
}

// UpdatePatterns updates all detected patterns.
func (apa *AccessPatternAnalyzer) UpdatePatterns() {
	apa.mu.Lock()
	defer apa.mu.Unlock()
	
	// Remove old patterns
	apa.removeExpiredPatterns()
	
	// Detect new patterns
	apa.detectSequentialPatterns()
	apa.detectTemporalPatterns()
	apa.detectCyclicPatterns()
	apa.detectBurstPatterns()
}

// GetPatterns returns current access patterns.
func (apa *AccessPatternAnalyzer) GetPatterns() map[string]*AccessPattern {
	apa.mu.RLock()
	defer apa.mu.RUnlock()
	
	// Return a copy
	patterns := make(map[string]*AccessPattern)
	for k, v := range apa.patterns {
		patternCopy := *v
		patterns[k] = &patternCopy
	}
	
	return patterns
}

// GetAccessFrequency returns the access frequency for a key.
func (apa *AccessPatternAnalyzer) GetAccessFrequency(key string) float64 {
	apa.mu.RLock()
	defer apa.mu.RUnlock()
	
	if history, exists := apa.accessHistory[key]; exists {
		return history.AccessFrequency
	}
	return 0
}

// PredictNextAccess predicts the next likely access for a key.
func (apa *AccessPatternAnalyzer) PredictNextAccess(key string) []string {
	apa.mu.RLock()
	defer apa.mu.RUnlock()
	
	var predictions []string
	
	// Check sequential patterns
	for _, pattern := range apa.patterns {
		if pattern.Type == PatternSequential {
			for i, k := range pattern.Keys {
				if k == key && i < len(pattern.Keys)-1 {
					predictions = append(predictions, pattern.Keys[i+1])
				}
			}
		}
	}
	
	// Check temporal patterns
	for _, pattern := range apa.patterns {
		if pattern.Type == PatternTemporal {
			for _, k := range pattern.Keys {
				if k == key {
					// Add other keys in the temporal set
					for _, otherKey := range pattern.Keys {
						if otherKey != key {
							predictions = append(predictions, otherKey)
						}
					}
					break
				}
			}
		}
	}
	
	// Remove duplicates and sort by confidence
	result := apa.deduplicateAndSort(predictions)
	// Ensure we never return nil - return empty slice if no predictions
	if result == nil {
		return []string{}
	}
	return result
}

// updateAccessStatistics updates statistics for an access history.
func (apa *AccessPatternAnalyzer) updateAccessStatistics(history *AccessHistory) {
	if len(history.AccessTimes) < 2 {
		return
	}
	
	// Calculate average interval
	totalInterval := time.Duration(0)
	intervals := 0
	
	for i := 1; i < len(history.AccessTimes); i++ {
		interval := history.AccessTimes[i].Sub(history.AccessTimes[i-1])
		totalInterval += interval
		intervals++
	}
	
	if intervals > 0 {
		history.AverageInterval = totalInterval / time.Duration(intervals)
		
		// Calculate access frequency (accesses per hour)
		totalDuration := history.LastAccess.Sub(history.FirstAccess)
		if totalDuration.Hours() > 0 {
			history.AccessFrequency = float64(history.AccessCount) / totalDuration.Hours()
		}
	}
}

// detectPatternsForKey detects patterns triggered by a specific key access.
func (apa *AccessPatternAnalyzer) detectPatternsForKey(key string, accessTime time.Time) {
	// Look for sequential patterns involving this key
	apa.detectSequentialForKey(key, accessTime)
	
	// Look for temporal patterns involving this key
	apa.detectTemporalForKey(key, accessTime)
}

// detectSequentialForKey detects sequential patterns for a specific key.
func (apa *AccessPatternAnalyzer) detectSequentialForKey(key string, accessTime time.Time) {
	// Get recent accesses (last 10 minutes)
	recentWindow := time.Minute * 10
	recentAccesses := apa.getRecentAccesses(accessTime.Add(-recentWindow), accessTime)
	
	if len(recentAccesses) < apa.minSequenceLength {
		return
	}
	
	// Look for sequences ending with this key
	for i := len(recentAccesses) - 1; i >= apa.minSequenceLength-1; i-- {
		if recentAccesses[i].Key == key {
			// Extract sequence
			sequenceKeys := make([]string, apa.minSequenceLength)
			for j := 0; j < apa.minSequenceLength; j++ {
				sequenceKeys[j] = recentAccesses[i-apa.minSequenceLength+1+j].Key
			}
			
			// Create or update pattern
			patternID := generateSequenceID(sequenceKeys)
			apa.updateSequentialPattern(patternID, sequenceKeys, accessTime)
		}
	}
}

// detectTemporalForKey detects temporal patterns for a specific key.
func (apa *AccessPatternAnalyzer) detectTemporalForKey(key string, accessTime time.Time) {
	// Look for objects accessed within a time window
	timeWindow := time.Minute * 5
	windowStart := accessTime.Add(-timeWindow)
	windowEnd := accessTime.Add(timeWindow)
	
	windowAccesses := apa.getAccessesInWindow(windowStart, windowEnd)
	if len(windowAccesses) < 2 {
		return
	}
	
	// Group by time windows
	uniqueKeys := make(map[string]bool)
	for _, access := range windowAccesses {
		uniqueKeys[access.Key] = true
	}
	
	if len(uniqueKeys) >= 2 {
		keys := make([]string, 0, len(uniqueKeys))
		for k := range uniqueKeys {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		
		patternID := generateTemporalID(keys)
		apa.updateTemporalPattern(patternID, keys, timeWindow, accessTime)
	}
}

// detectSequentialPatterns detects sequential access patterns.
func (apa *AccessPatternAnalyzer) detectSequentialPatterns() {
	// Analyze access history for sequential patterns
	analysisWindow := time.Hour * 2
	cutoff := time.Now().Add(-analysisWindow)
	
	recentAccesses := apa.getRecentAccesses(cutoff, time.Now())
	if len(recentAccesses) < apa.minSequenceLength {
		return
	}
	
	// Sliding window approach to find sequences
	for i := 0; i <= len(recentAccesses)-apa.minSequenceLength; i++ {
		sequence := make([]string, apa.minSequenceLength)
		for j := 0; j < apa.minSequenceLength; j++ {
			sequence[j] = recentAccesses[i+j].Key
		}
		
		// Check if this is a valid sequence (not all same key)
		if apa.isValidSequence(sequence) {
			patternID := generateSequenceID(sequence)
			apa.updateSequentialPattern(patternID, sequence, recentAccesses[i+apa.minSequenceLength-1].Time)
		}
	}
}

// detectTemporalPatterns detects temporal co-access patterns.
func (apa *AccessPatternAnalyzer) detectTemporalPatterns() {
	timeWindow := time.Minute * 5
	analysisWindow := time.Hour * 6
	cutoff := time.Now().Add(-analysisWindow)
	
	recentAccesses := apa.getRecentAccesses(cutoff, time.Now())
	
	// Group accesses into time windows
	windows := apa.groupIntoTimeWindows(recentAccesses, timeWindow)
	
	for _, window := range windows {
		if len(window) >= 2 {
			uniqueKeys := make(map[string]bool)
			for _, access := range window {
				uniqueKeys[access.Key] = true
			}
			
			if len(uniqueKeys) >= 2 {
				keys := make([]string, 0, len(uniqueKeys))
				for k := range uniqueKeys {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				
				patternID := generateTemporalID(keys)
				apa.updateTemporalPattern(patternID, keys, timeWindow, window[len(window)-1].Time)
			}
		}
	}
}

// detectCyclicPatterns detects cyclic access patterns.
func (apa *AccessPatternAnalyzer) detectCyclicPatterns() {
	// Look for repeating sequences in access history
	for key, history := range apa.accessHistory {
		if len(history.AccessTimes) < 6 { // Need at least 2 cycles of 3
			continue
		}
		
		// Check for regular intervals
		if history.AverageInterval > 0 {
			confidence := apa.calculateCyclicConfidence(history)
			if confidence >= apa.confidenceThreshold {
				patternID := "cyclic_" + key
				pattern := &AccessPattern{
					Type:            PatternCyclic,
					Keys:            []string{key},
					Confidence:      confidence,
					PredictedNext:   []string{key},
					AverageInterval: history.AverageInterval,
					LastUpdated:     time.Now(),
					UsageCount:      1,
				}
				apa.patterns[patternID] = pattern
			}
		}
	}
}

// detectBurstPatterns detects burst access patterns.
func (apa *AccessPatternAnalyzer) detectBurstPatterns() {
	// Look for burst patterns (multiple accesses in short time)
	burstWindow := time.Minute * 2
	minBurstSize := 3
	
	analysisWindow := time.Hour * 1
	cutoff := time.Now().Add(-analysisWindow)
	recentAccesses := apa.getRecentAccesses(cutoff, time.Now())
	
	// Group into burst windows
	bursts := apa.groupIntoBursts(recentAccesses, burstWindow, minBurstSize)
	
	for _, burst := range bursts {
		uniqueKeys := make(map[string]bool)
		for _, access := range burst {
			uniqueKeys[access.Key] = true
		}
		
		if len(uniqueKeys) >= minBurstSize {
			keys := make([]string, 0, len(uniqueKeys))
			for k := range uniqueKeys {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			
			patternID := generateBurstID(keys)
			confidence := float64(len(burst)) / float64(len(uniqueKeys))
			
			pattern := &AccessPattern{
				Type:          PatternBurst,
				Keys:          keys,
				Confidence:    confidence,
				PredictedNext: keys,
				LastUpdated:   time.Now(),
				UsageCount:    1,
			}
			apa.patterns[patternID] = pattern
		}
	}
}

// updateSequentialPattern updates or creates a sequential pattern.
func (apa *AccessPatternAnalyzer) updateSequentialPattern(patternID string, sequence []string, accessTime time.Time) {
	if pattern, exists := apa.patterns[patternID]; exists {
		pattern.UsageCount++
		pattern.LastUpdated = accessTime
		pattern.Confidence = apa.calculateSequentialConfidence(pattern.UsageCount)
		
		// Update predicted next
		if len(sequence) > 0 {
			pattern.PredictedNext = []string{sequence[len(sequence)-1]}
		}
	} else {
		confidence := apa.calculateSequentialConfidence(1)
		if confidence >= apa.confidenceThreshold {
			predictedNext := make([]string, 0)
			if len(sequence) > 0 {
				predictedNext = append(predictedNext, sequence[len(sequence)-1])
			}
			
			pattern := &AccessPattern{
				Type:          PatternSequential,
				Keys:          sequence,
				Confidence:    confidence,
				PredictedNext: predictedNext,
				LastUpdated:   accessTime,
				UsageCount:    1,
			}
			apa.patterns[patternID] = pattern
		}
	}
}

// updateTemporalPattern updates or creates a temporal pattern.
func (apa *AccessPatternAnalyzer) updateTemporalPattern(patternID string, keys []string, timeWindow time.Duration, accessTime time.Time) {
	if pattern, exists := apa.patterns[patternID]; exists {
		pattern.UsageCount++
		pattern.LastUpdated = accessTime
		pattern.Confidence = apa.calculateTemporalConfidence(pattern.UsageCount, len(keys))
	} else {
		confidence := apa.calculateTemporalConfidence(1, len(keys))
		if confidence >= apa.confidenceThreshold {
			pattern := &AccessPattern{
				Type:          PatternTemporal,
				Keys:          keys,
				Confidence:    confidence,
				PredictedNext: keys, // All keys are potential next accesses
				LastUpdated:   accessTime,
				UsageCount:    1,
			}
			apa.patterns[patternID] = pattern
		}
	}
}

// Helper functions

func (apa *AccessPatternAnalyzer) getRecentAccesses(start, end time.Time) []AccessInfo {
	var accesses []AccessInfo
	
	for _, history := range apa.accessHistory {
		for _, accessTime := range history.AccessTimes {
			if accessTime.After(start) && accessTime.Before(end) {
				accesses = append(accesses, AccessInfo{
					Key:  history.Key,
					Time: accessTime,
				})
			}
		}
	}
	
	// Sort by time
	sort.Slice(accesses, func(i, j int) bool {
		return accesses[i].Time.Before(accesses[j].Time)
	})
	
	return accesses
}

func (apa *AccessPatternAnalyzer) getAccessesInWindow(start, end time.Time) []AccessInfo {
	return apa.getRecentAccesses(start, end)
}

func (apa *AccessPatternAnalyzer) isValidSequence(sequence []string) bool {
	// Check if sequence has at least 2 different keys
	uniqueKeys := make(map[string]bool)
	for _, key := range sequence {
		uniqueKeys[key] = true
	}
	return len(uniqueKeys) >= 2
}

func (apa *AccessPatternAnalyzer) groupIntoTimeWindows(accesses []AccessInfo, window time.Duration) [][]AccessInfo {
	if len(accesses) == 0 {
		return nil
	}
	
	var groups [][]AccessInfo
	currentGroup := []AccessInfo{accesses[0]}
	windowStart := accesses[0].Time
	
	for i := 1; i < len(accesses); i++ {
		if accesses[i].Time.Sub(windowStart) <= window {
			currentGroup = append(currentGroup, accesses[i])
		} else {
			if len(currentGroup) > 0 {
				groups = append(groups, currentGroup)
			}
			currentGroup = []AccessInfo{accesses[i]}
			windowStart = accesses[i].Time
		}
	}
	
	if len(currentGroup) > 0 {
		groups = append(groups, currentGroup)
	}
	
	return groups
}

func (apa *AccessPatternAnalyzer) groupIntoBursts(accesses []AccessInfo, window time.Duration, minSize int) [][]AccessInfo {
	windows := apa.groupIntoTimeWindows(accesses, window)
	
	var bursts [][]AccessInfo
	for _, w := range windows {
		if len(w) >= minSize {
			bursts = append(bursts, w)
		}
	}
	
	return bursts
}

func (apa *AccessPatternAnalyzer) calculateSequentialConfidence(usageCount int64) float64 {
	// Confidence increases with usage count but plateaus
	base := 0.5
	growth := float64(usageCount) * 0.1
	max := 0.95
	
	confidence := base + growth
	if confidence > max {
		confidence = max
	}
	
	return confidence
}

func (apa *AccessPatternAnalyzer) calculateTemporalConfidence(usageCount int64, keyCount int) float64 {
	// Confidence based on usage count and number of keys in pattern
	base := 0.4
	usageFactor := float64(usageCount) * 0.05
	keyFactor := float64(keyCount) * 0.05
	
	confidence := base + usageFactor + keyFactor
	if confidence > 0.9 {
		confidence = 0.9
	}
	
	return confidence
}

func (apa *AccessPatternAnalyzer) calculateCyclicConfidence(history *AccessHistory) float64 {
	if len(history.AccessTimes) < 3 {
		return 0
	}
	
	// Calculate variance in intervals
	intervals := make([]time.Duration, 0, len(history.AccessTimes)-1)
	for i := 1; i < len(history.AccessTimes); i++ {
		intervals = append(intervals, history.AccessTimes[i].Sub(history.AccessTimes[i-1]))
	}
	
	// Calculate coefficient of variation
	variance := calculateIntervalVariance(intervals, history.AverageInterval)
	cv := variance / float64(history.AverageInterval)
	
	// Lower variance = higher confidence
	confidence := 1.0 - cv
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 0.9 {
		confidence = 0.9
	}
	
	return confidence
}

func (apa *AccessPatternAnalyzer) removeExpiredPatterns() {
	now := time.Now()
	for patternID, pattern := range apa.patterns {
		if now.Sub(pattern.LastUpdated) > apa.maxPatternAge {
			delete(apa.patterns, patternID)
		}
	}
}

func (apa *AccessPatternAnalyzer) deduplicateAndSort(predictions []string) []string {
	// Remove duplicates
	unique := make(map[string]bool)
	var result []string
	
	for _, pred := range predictions {
		if !unique[pred] {
			unique[pred] = true
			result = append(result, pred)
		}
	}
	
	// Sort alphabetically for consistency
	sort.Strings(result)
	return result
}

// Helper types and functions

type AccessInfo struct {
	Key  string
	Time time.Time
}

func generateSequenceID(sequence []string) string {
	return "seq_" + joinStrings(sequence, "_")
}

func generateTemporalID(keys []string) string {
	return "temp_" + joinStrings(keys, "_")
}

func generateBurstID(keys []string) string {
	return "burst_" + joinStrings(keys, "_")
}

func joinStrings(strings []string, separator string) string {
	if len(strings) == 0 {
		return ""
	}
	
	result := strings[0]
	for i := 1; i < len(strings); i++ {
		result += separator + strings[i]
	}
	return result
}

func calculateIntervalVariance(intervals []time.Duration, mean time.Duration) float64 {
	if len(intervals) <= 1 {
		return 0
	}
	
	var sumSquaredDiffs float64
	for _, interval := range intervals {
		diff := float64(interval - mean)
		sumSquaredDiffs += diff * diff
	}
	
	return sumSquaredDiffs / float64(len(intervals))
}