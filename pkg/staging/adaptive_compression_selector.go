package staging

import (
	"fmt"
	"sync"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/compression"
	"github.com/scttfrdmn/cargoship/pkg/detection"
)

// AdaptiveCompressionSelector provides intelligent compression algorithm selection
// based on file types, content characteristics, network conditions, and performance history.
type AdaptiveCompressionSelector struct {
	compressionProfiles  map[string]*CompressionProfile        // Content type -> profile
	algorithmPerformance map[string]*AlgorithmPerformance      // Algorithm -> performance stats
	fileTypeRules        map[string]*CompressionRule           // File extension -> rule
	networkAdapters      map[string]*NetworkCompressionAdapter // Network type -> adapter
	learningEngine       *CompressionLearningEngine
	performancePredictor *CompressionPerformancePredictor
	contextualOptimizer  *ContextualCompressionOptimizer
	realtimeMonitor      *RealtimeCompressionMonitor
	config               *AdaptiveCompressionConfig
	mu                   sync.RWMutex
}

// AdaptiveCompressionConfig configures adaptive compression behavior.
type AdaptiveCompressionConfig struct {
	// Algorithm selection parameters
	EnableLearning               bool `yaml:"enable_learning" json:"enable_learning"`
	EnableNetworkAdaptation      bool `yaml:"enable_network_adaptation" json:"enable_network_adaptation"`
	EnableContextualOptimization bool `yaml:"enable_contextual_optimization" json:"enable_contextual_optimization"`
	EnableRealtimeMonitoring     bool `yaml:"enable_realtime_monitoring" json:"enable_realtime_monitoring"`

	// Performance thresholds
	MinCompressionRatio        float64           `yaml:"min_compression_ratio" json:"min_compression_ratio"`
	MaxCompressionTime         time.Duration     `yaml:"max_compression_time" json:"max_compression_time"`
	NetworkBandwidthThresholds NetworkThresholds `yaml:"network_thresholds" json:"network_thresholds"`

	// Learning parameters
	LearningWindowSize     int `yaml:"learning_window_size" json:"learning_window_size"`
	MinSamplesForLearning  int `yaml:"min_samples_for_learning" json:"min_samples_for_learning"`
	PerformanceHistorySize int `yaml:"performance_history_size" json:"performance_history_size"`

	// Contextual optimization
	EnableFileTypeSpecialization      bool `yaml:"enable_file_type_specialization" json:"enable_file_type_specialization"`
	EnableSizeBasedOptimization       bool `yaml:"enable_size_based_optimization" json:"enable_size_based_optimization"`
	EnableContentAnalysisOptimization bool `yaml:"enable_content_analysis_optimization" json:"enable_content_analysis_optimization"`
}

// DefaultAdaptiveCompressionConfig returns sensible defaults.
func DefaultAdaptiveCompressionConfig() *AdaptiveCompressionConfig {
	return &AdaptiveCompressionConfig{
		EnableLearning:               true,
		EnableNetworkAdaptation:      true,
		EnableContextualOptimization: true,
		EnableRealtimeMonitoring:     true,
		MinCompressionRatio:          0.05,
		MaxCompressionTime:           time.Second * 30,
		NetworkBandwidthThresholds: NetworkThresholds{
			LowBandwidth:    1.0,  // < 1 MB/s
			MediumBandwidth: 10.0, // 1-10 MB/s
			HighBandwidth:   50.0, // > 10 MB/s
		},
		LearningWindowSize:                1000,
		MinSamplesForLearning:             10,
		PerformanceHistorySize:            5000,
		EnableFileTypeSpecialization:      true,
		EnableSizeBasedOptimization:       true,
		EnableContentAnalysisOptimization: true,
	}
}

// CompressionProfile defines compression characteristics for a content type.
type CompressionProfile struct {
	ContentType            string
	PreferredAlgorithms    []string                         // Ordered by preference
	AlgorithmEffectiveness map[string]*EffectivenessMetrics // Algorithm -> metrics
	FileTypeRules          []*FileTypeRule                  // Specific rules for file types
	ContentPatternRules    []*PatternRule                   // Rules based on content patterns
	SizeThresholds         *SizeBasedRules                  // Size-based algorithm selection
	NetworkOptimizations   *NetworkOptimizationRules        // Network-specific optimizations
	LastUpdated            time.Time
	SampleCount            int64
}

// AlgorithmPerformance tracks performance metrics for compression algorithms.
type AlgorithmPerformance struct {
	Algorithm               string
	TotalCompressions       int64
	AverageCompressionRatio float64
	AverageSpeedMBps        float64
	AverageMemoryUsageMB    float64
	SuccessRate             float64
	NetworkPerformance      map[string]*NetworkPerformanceMetrics
	FileTypePerformance     map[string]*FileTypePerformanceMetrics
	SizeRangePerformance    map[string]*SizeRangePerformanceMetrics
	RecentPerformance       []*PerformanceDataPoint
	LastUpdated             time.Time
}

// CompressionRule defines rules for compression algorithm selection.
type CompressionRule struct {
	Name                    string
	Priority                int              // Higher priority rules take precedence
	Conditions              []*RuleCondition // Conditions that must be met
	RecommendedAlgorithm    string           // Algorithm to use when conditions met
	FallbackAlgorithms      []string         // Fallback options
	PerformanceRequirements *PerformanceRequirements
	ApplicableFileTypes     []string
	ApplicableNetworkTypes  []string
	Enabled                 bool
}

// CompressionDecision represents the result of algorithm selection.
type CompressionDecision struct {
	SelectedAlgorithm    string
	Confidence           float64
	ReasoningChain       []string
	PredictedPerformance *PredictedPerformance
	AlternativeOptions   []*AlgorithmOption
	RecommendedSettings  *CompressionSettings
	ContextualFactors    *ContextualFactors
	DecisionMetadata     map[string]interface{}
}

// NewAdaptiveCompressionSelector creates a new adaptive compression selector.
func NewAdaptiveCompressionSelector(config *AdaptiveCompressionConfig) *AdaptiveCompressionSelector {
	if config == nil {
		config = DefaultAdaptiveCompressionConfig()
	}

	selector := &AdaptiveCompressionSelector{
		compressionProfiles:  make(map[string]*CompressionProfile),
		algorithmPerformance: make(map[string]*AlgorithmPerformance),
		fileTypeRules:        make(map[string]*CompressionRule),
		networkAdapters:      make(map[string]*NetworkCompressionAdapter),
		config:               config,
	}

	// Initialize components
	if config.EnableLearning {
		selector.learningEngine = NewCompressionLearningEngine(config)
	}

	selector.performancePredictor = NewCompressionPerformancePredictor(config)

	if config.EnableContextualOptimization {
		selector.contextualOptimizer = NewContextualCompressionOptimizer(config)
	}

	if config.EnableRealtimeMonitoring {
		selector.realtimeMonitor = NewRealtimeCompressionMonitor(config)
	}

	// Initialize default compression profiles
	selector.initializeDefaultProfiles()

	// Initialize default file type rules
	selector.initializeFileTypeRules()

	return selector
}

// SelectCompressionAlgorithm intelligently selects the best compression algorithm
// for the given content and conditions.
func (acs *AdaptiveCompressionSelector) SelectCompressionAlgorithm(
	contentProfile *ContentProfile,
	networkCondition *NetworkCondition,
	context *CompressionContext,
) (*CompressionDecision, error) {

	acs.mu.RLock()
	defer acs.mu.RUnlock()

	// Issue #34 Phase 3.2: Fast path for files with obvious compression needs
	// Skip expensive analysis for common file types with high-priority rules (50-70% faster)
	if context.FileExtension != "" {
		if rule, exists := acs.fileTypeRules[context.FileExtension]; exists && rule.Enabled && rule.Priority >= 90 {
			// High-confidence rule - return immediately without full analysis
			return &CompressionDecision{
				SelectedAlgorithm: rule.RecommendedAlgorithm,
				Confidence:        0.95, // High confidence for explicit rules
				ReasoningChain: []string{
					fmt.Sprintf("Fast path: Extension %s → %s (priority %d)",
						context.FileExtension, rule.RecommendedAlgorithm, rule.Priority),
				},
				AlternativeOptions: acs.generateFallbackOptions(rule),
				RecommendedSettings: acs.getOptimalSettings(rule.RecommendedAlgorithm, contentProfile, networkCondition),
				ContextualFactors: &ContextualFactors{
					FileSize:         context.FileSize,
					ContentEntropy:   contentProfile.Entropy,
					NetworkLatency:   networkCondition.LatencyMs,
					NetworkBandwidth: networkCondition.BandwidthMBps,
					Priority:         context.Priority,
				},
				DecisionMetadata: map[string]interface{}{
					"fast_path": true,
					"rule_name": rule.Name,
				},
			}, nil
		}
	}

	decision := &CompressionDecision{
		ReasoningChain:     make([]string, 0),
		AlternativeOptions: make([]*AlgorithmOption, 0),
		DecisionMetadata:   make(map[string]interface{}),
	}

	// Step 1: Get content type-based profile (with Magika metadata support - Issue #30)
	effectiveContentType := acs.getContentTypeWithMagikaMetadata(contentProfile)
	profile := acs.getCompressionProfile(effectiveContentType)

	// Log Magika detection if used
	if contentProfile.Metadata != nil {
		if magikaType, ok := contentProfile.Metadata["magika_type"].(string); ok && magikaType != "" {
			decision.ReasoningChain = append(decision.ReasoningChain,
				fmt.Sprintf("Magika AI detected type: %s → %s", magikaType, effectiveContentType))
		}
	}

	decision.ReasoningChain = append(decision.ReasoningChain,
		fmt.Sprintf("Retrieved compression profile for content type: %s", effectiveContentType))

	// Step 2: Apply file type specific rules
	fileTypeAlgorithm := acs.applyFileTypeRules(context.FileName, context.FileExtension)
	if fileTypeAlgorithm != "" {
		decision.ReasoningChain = append(decision.ReasoningChain,
			fmt.Sprintf("File type rule suggests algorithm: %s", fileTypeAlgorithm))
	}

	// Step 3: Analyze network conditions
	networkOptimization := acs.analyzeNetworkOptimization(networkCondition, contentProfile)
	decision.ReasoningChain = append(decision.ReasoningChain,
		fmt.Sprintf("Network analysis completed - bandwidth: %.2f MB/s", networkCondition.BandwidthMBps))

	// Step 4: Use machine learning predictions if available
	var mlRecommendation *MLRecommendation
	if acs.learningEngine != nil {
		mlRecommendation = acs.learningEngine.PredictOptimalAlgorithm(contentProfile, networkCondition, context)
		if mlRecommendation.Confidence > 0.7 {
			decision.ReasoningChain = append(decision.ReasoningChain,
				fmt.Sprintf("ML engine recommends: %s (confidence: %.2f)", mlRecommendation.Algorithm, mlRecommendation.Confidence))
		}
	}

	// Step 5: Apply contextual optimization
	var contextualRecommendation *ContextualRecommendation
	if acs.contextualOptimizer != nil {
		contextualRecommendation = acs.contextualOptimizer.OptimizeSelection(
			contentProfile, networkCondition, context, profile)
	}

	// Step 6: Combine all recommendations using weighted decision making
	finalAlgorithm := acs.combineRecommendations(
		profile, fileTypeAlgorithm, networkOptimization,
		mlRecommendation, contextualRecommendation, context)

	decision.SelectedAlgorithm = finalAlgorithm
	decision.Confidence = acs.calculateConfidence(finalAlgorithm, contentProfile, networkCondition)
	decision.PredictedPerformance = acs.performancePredictor.PredictPerformance(
		finalAlgorithm, contentProfile, networkCondition)

	// Generate alternative options
	decision.AlternativeOptions = acs.generateAlternativeOptions(
		finalAlgorithm, contentProfile, networkCondition, context)

	// Set recommended compression settings
	decision.RecommendedSettings = acs.getOptimalSettings(finalAlgorithm, contentProfile, networkCondition)

	// Capture contextual factors
	decision.ContextualFactors = &ContextualFactors{
		FileSize:           context.FileSize,
		ContentEntropy:     contentProfile.Entropy,
		NetworkLatency:     networkCondition.LatencyMs,
		NetworkBandwidth:   networkCondition.BandwidthMBps,
		NetworkReliability: networkCondition.Reliability,
		SystemLoad:         context.SystemLoad,
		MemoryAvailable:    context.AvailableMemoryMB,
		Priority:           context.Priority,
	}

	decision.ReasoningChain = append(decision.ReasoningChain,
		fmt.Sprintf("Final selection: %s with %.2f confidence", finalAlgorithm, decision.Confidence))

	return decision, nil
}

// getContentTypeWithMagikaMetadata determines the best content type to use for compression
// selection, prioritizing Magika AI detection if available (Issue #30)
func (acs *AdaptiveCompressionSelector) getContentTypeWithMagikaMetadata(contentProfile *ContentProfile) string {
	// Priority 1: Check for Magika metadata
	if contentProfile.Metadata != nil {
		if magikaType, ok := contentProfile.Metadata["magika_type"].(string); ok && magikaType != "" {
			// Map Magika label to compression content type
			compressionType := detection.MapMagikaToCompression(magikaType)

			// Convert compression.ContentType to string for profile lookup
			mappedType := acs.mapCompressionTypeToProfileKey(compressionType)
			if mappedType != "" {
				return mappedType
			}
		}
	}

	// Priority 2: Fall back to provided content type
	return contentProfile.ContentType
}

// mapCompressionTypeToProfileKey maps compression.ContentType to profile keys
func (acs *AdaptiveCompressionSelector) mapCompressionTypeToProfileKey(ct compression.ContentType) string {
	switch ct {
	case compression.ContentTypeText:
		return "text"
	case compression.ContentTypeCode:
		return "json" // Code files compress similarly to JSON
	case compression.ContentTypeDocument:
		return "text" // Documents compress like text
	case compression.ContentTypeImage:
		return "image"
	case compression.ContentTypeVideo:
		return "compressed" // Already compressed
	case compression.ContentTypeAudio:
		return "compressed" // Already compressed
	case compression.ContentTypeArchive:
		return "compressed" // Already compressed
	case compression.ContentTypeBinary:
		return "binary"
	default:
		return "" // Unknown, use provided content type
	}
}

// LearnFromCompressionResult learns from actual compression performance.
func (acs *AdaptiveCompressionSelector) LearnFromCompressionResult(result *CompressionResult) {
	acs.mu.Lock()
	defer acs.mu.Unlock()

	// Update algorithm performance
	acs.updateAlgorithmPerformance(result)

	// Update compression profile
	acs.updateCompressionProfile(result)

	// Feed to learning engine
	if acs.learningEngine != nil {
		acs.learningEngine.LearnFromResult(result)
	}

	// Update performance predictor
	acs.performancePredictor.UpdateWithResult(result)
}

// initializeDefaultProfiles sets up default compression profiles for common content types.
func (acs *AdaptiveCompressionSelector) initializeDefaultProfiles() {
	profiles := map[string]*CompressionProfile{
		"text": {
			ContentType:         "text",
			PreferredAlgorithms: []string{"zstd-high", "zstd", "zstd-fast"},
			AlgorithmEffectiveness: map[string]*EffectivenessMetrics{
				"zstd-high": {CompressionRatio: 0.25, Speed: 50, MemoryUsage: 64},
				"zstd":      {CompressionRatio: 0.35, Speed: 150, MemoryUsage: 32},
				"zstd-fast": {CompressionRatio: 0.45, Speed: 400, MemoryUsage: 16},
			},
		},
		"binary": {
			ContentType:         "binary",
			PreferredAlgorithms: []string{"zstd", "zstd-fast", "zstd-high"},
			AlgorithmEffectiveness: map[string]*EffectivenessMetrics{
				"zstd":      {CompressionRatio: 0.60, Speed: 120, MemoryUsage: 32},
				"zstd-fast": {CompressionRatio: 0.75, Speed: 350, MemoryUsage: 16},
				"zstd-high": {CompressionRatio: 0.50, Speed: 40, MemoryUsage: 64},
			},
		},
		"image": {
			ContentType:         "image",
			PreferredAlgorithms: []string{"zstd-fast", "none"},
			AlgorithmEffectiveness: map[string]*EffectivenessMetrics{
				"zstd-fast": {CompressionRatio: 0.95, Speed: 500, MemoryUsage: 8},
				"none":      {CompressionRatio: 1.0, Speed: 1000, MemoryUsage: 0},
			},
		},
		"compressed": {
			ContentType:         "compressed",
			PreferredAlgorithms: []string{"none", "zstd-fast"},
			AlgorithmEffectiveness: map[string]*EffectivenessMetrics{
				"none":      {CompressionRatio: 1.0, Speed: 1000, MemoryUsage: 0},
				"zstd-fast": {CompressionRatio: 0.98, Speed: 500, MemoryUsage: 8},
			},
		},
		"json": {
			ContentType:         "json",
			PreferredAlgorithms: []string{"zstd-high", "zstd", "zstd-fast"},
			AlgorithmEffectiveness: map[string]*EffectivenessMetrics{
				"zstd-high": {CompressionRatio: 0.20, Speed: 60, MemoryUsage: 48},
				"zstd":      {CompressionRatio: 0.30, Speed: 180, MemoryUsage: 24},
				"zstd-fast": {CompressionRatio: 0.40, Speed: 450, MemoryUsage: 12},
			},
		},
	}

	for contentType, profile := range profiles {
		profile.LastUpdated = time.Now()
		acs.compressionProfiles[contentType] = profile
	}
}

// initializeFileTypeRules sets up default file type-specific compression rules.
func (acs *AdaptiveCompressionSelector) initializeFileTypeRules() {
	rules := map[string]*CompressionRule{
		".txt": {
			Name:                 "Text File Rule",
			Priority:             100,
			RecommendedAlgorithm: "zstd-high",
			FallbackAlgorithms:   []string{"zstd", "zstd-fast"},
			Enabled:              true,
		},
		".log": {
			Name:                 "Log File Rule",
			Priority:             90,
			RecommendedAlgorithm: "zstd-high",
			FallbackAlgorithms:   []string{"zstd"},
			Enabled:              true,
		},
		".json": {
			Name:                 "JSON File Rule",
			Priority:             95,
			RecommendedAlgorithm: "zstd-high",
			FallbackAlgorithms:   []string{"zstd"},
			Enabled:              true,
		},
		".xml": {
			Name:                 "XML File Rule",
			Priority:             95,
			RecommendedAlgorithm: "zstd-high",
			FallbackAlgorithms:   []string{"zstd"},
			Enabled:              true,
		},
		".jpg": {
			Name:                 "JPEG Image Rule",
			Priority:             100,
			RecommendedAlgorithm: "none",
			FallbackAlgorithms:   []string{"zstd-fast"},
			Enabled:              true,
		},
		".png": {
			Name:                 "PNG Image Rule",
			Priority:             100,
			RecommendedAlgorithm: "none",
			FallbackAlgorithms:   []string{"zstd-fast"},
			Enabled:              true,
		},
		".zip": {
			Name:                 "ZIP Archive Rule",
			Priority:             100,
			RecommendedAlgorithm: "none",
			FallbackAlgorithms:   []string{},
			Enabled:              true,
		},
		".gz": {
			Name:                 "Gzip Archive Rule",
			Priority:             100,
			RecommendedAlgorithm: "none",
			FallbackAlgorithms:   []string{},
			Enabled:              true,
		},
	}

	for extension, rule := range rules {
		acs.fileTypeRules[extension] = rule
	}
}

// getCompressionProfile retrieves or creates a compression profile for a content type.
func (acs *AdaptiveCompressionSelector) getCompressionProfile(contentType string) *CompressionProfile {
	if profile, exists := acs.compressionProfiles[contentType]; exists {
		return profile
	}

	// Create default profile for unknown content type
	return &CompressionProfile{
		ContentType:         contentType,
		PreferredAlgorithms: []string{"zstd", "zstd-fast"},
		AlgorithmEffectiveness: map[string]*EffectivenessMetrics{
			"zstd":      {CompressionRatio: 0.50, Speed: 100, MemoryUsage: 32},
			"zstd-fast": {CompressionRatio: 0.70, Speed: 300, MemoryUsage: 16},
		},
		LastUpdated: time.Now(),
		SampleCount: 0,
	}
}

// applyFileTypeRules applies file type specific rules.
func (acs *AdaptiveCompressionSelector) applyFileTypeRules(fileName, fileExtension string) string {
	if rule, exists := acs.fileTypeRules[fileExtension]; exists && rule.Enabled {
		return rule.RecommendedAlgorithm
	}
	return ""
}

// analyzeNetworkOptimization analyzes network conditions for optimal compression.
func (acs *AdaptiveCompressionSelector) analyzeNetworkOptimization(
	networkCondition *NetworkCondition,
	contentProfile *ContentProfile) *NetworkOptimization {

	optimization := &NetworkOptimization{}

	// High bandwidth - favor speed over compression ratio
	if networkCondition.BandwidthMBps > acs.config.NetworkBandwidthThresholds.HighBandwidth {
		optimization.Strategy = "speed-optimized"
		optimization.PreferredAlgorithms = []string{"zstd-fast", "none"}
		optimization.Reasoning = "High bandwidth network - prioritizing speed"
	} else if networkCondition.BandwidthMBps < acs.config.NetworkBandwidthThresholds.LowBandwidth {
		// Low bandwidth - favor compression ratio over speed
		optimization.Strategy = "compression-optimized"
		optimization.PreferredAlgorithms = []string{"zstd-high", "zstd"}
		optimization.Reasoning = "Low bandwidth network - prioritizing compression ratio"
	} else {
		// Medium bandwidth - balanced approach
		optimization.Strategy = "balanced"
		optimization.PreferredAlgorithms = []string{"zstd", "zstd-fast"}
		optimization.Reasoning = "Medium bandwidth network - balanced approach"
	}

	// Adjust for network reliability
	if networkCondition.Reliability < 0.8 {
		// Unreliable network - use faster compression to reduce exposure time
		optimization.PreferredAlgorithms = []string{"zstd-fast", "none"}
		optimization.Reasoning += " | Unreliable network - using faster compression"
	}

	return optimization
}

// combineRecommendations combines multiple recommendation sources using weighted decision making.
func (acs *AdaptiveCompressionSelector) combineRecommendations(
	profile *CompressionProfile,
	fileTypeAlgorithm string,
	networkOptimization *NetworkOptimization,
	mlRecommendation *MLRecommendation,
	contextualRecommendation *ContextualRecommendation,
	context *CompressionContext,
) string {

	// Weight different recommendation sources
	algorithmScores := make(map[string]float64)

	// File type rules have high weight
	if fileTypeAlgorithm != "" {
		algorithmScores[fileTypeAlgorithm] += 0.3
	}

	// Network optimization
	for _, alg := range networkOptimization.PreferredAlgorithms {
		algorithmScores[alg] += 0.25
	}

	// ML recommendation (if confident)
	if mlRecommendation != nil && mlRecommendation.Confidence > 0.6 {
		weight := mlRecommendation.Confidence * 0.3
		algorithmScores[mlRecommendation.Algorithm] += weight
	}

	// Contextual recommendation
	if contextualRecommendation != nil {
		algorithmScores[contextualRecommendation.Algorithm] += contextualRecommendation.Weight * 0.15
	}

	// Content profile preferred algorithms
	for i, alg := range profile.PreferredAlgorithms {
		weight := 0.1 * (1.0 - float64(i)*0.1) // Decreasing weight by preference order
		algorithmScores[alg] += weight
	}

	// Find algorithm with highest score
	var bestAlgorithm string
	var bestScore float64
	for algorithm, score := range algorithmScores {
		if score > bestScore {
			bestScore = score
			bestAlgorithm = algorithm
		}
	}

	// Fallback to profile default if no clear winner
	if bestAlgorithm == "" && len(profile.PreferredAlgorithms) > 0 {
		bestAlgorithm = profile.PreferredAlgorithms[0]
	}

	// Ultimate fallback
	if bestAlgorithm == "" {
		bestAlgorithm = "zstd"
	}

	return bestAlgorithm
}

// calculateConfidence calculates confidence in the algorithm selection.
func (acs *AdaptiveCompressionSelector) calculateConfidence(
	algorithm string,
	contentProfile *ContentProfile,
	networkCondition *NetworkCondition,
) float64 {
	baseConfidence := 0.5

	// Higher confidence for well-known content types
	if profile := acs.compressionProfiles[contentProfile.ContentType]; profile != nil {
		if profile.SampleCount > 100 {
			baseConfidence += 0.2
		} else if profile.SampleCount > 10 {
			baseConfidence += 0.1
		}
	}

	// Higher confidence for stable network conditions
	if networkCondition.Reliability > 0.9 {
		baseConfidence += 0.1
	}

	// ML confidence boost
	if acs.learningEngine != nil {
		mlConfidence := acs.learningEngine.GetConfidence(contentProfile, networkCondition)
		baseConfidence += mlConfidence * 0.2
	}

	// Cap at 0.95
	if baseConfidence > 0.95 {
		baseConfidence = 0.95
	}

	return baseConfidence
}

// generateAlternativeOptions generates alternative algorithm options.
func (acs *AdaptiveCompressionSelector) generateAlternativeOptions(
	selectedAlgorithm string,
	contentProfile *ContentProfile,
	networkCondition *NetworkCondition,
	context *CompressionContext,
) []*AlgorithmOption {

	alternatives := make([]*AlgorithmOption, 0)
	profile := acs.getCompressionProfile(contentProfile.ContentType)

	for _, algorithm := range profile.PreferredAlgorithms {
		if algorithm != selectedAlgorithm {
			predictedPerf := acs.performancePredictor.PredictPerformance(
				algorithm, contentProfile, networkCondition)

			alternatives = append(alternatives, &AlgorithmOption{
				Algorithm:   algorithm,
				Confidence:  0.7, // Default confidence for alternatives
				Performance: predictedPerf,
				Reasoning:   fmt.Sprintf("Alternative from %s profile", contentProfile.ContentType),
			})
		}
	}

	return alternatives
}

// generateFallbackOptions creates alternative options from rule fallback algorithms
// Issue #34 Phase 3.2: Helper for fast path to generate simple alternatives
func (acs *AdaptiveCompressionSelector) generateFallbackOptions(rule *CompressionRule) []*AlgorithmOption {
	alternatives := make([]*AlgorithmOption, 0, len(rule.FallbackAlgorithms))

	for _, algorithm := range rule.FallbackAlgorithms {
		alternatives = append(alternatives, &AlgorithmOption{
			Algorithm:  algorithm,
			Confidence: 0.7, // Lower confidence for fallbacks
			Reasoning:  fmt.Sprintf("Fallback option from %s rule", rule.Name),
		})
	}

	return alternatives
}

// getOptimalSettings returns optimal compression settings for the algorithm.
func (acs *AdaptiveCompressionSelector) getOptimalSettings(
	algorithm string,
	contentProfile *ContentProfile,
	networkCondition *NetworkCondition,
) *CompressionSettings {

	settings := &CompressionSettings{
		Algorithm: algorithm,
	}

	// Set compression level based on algorithm and conditions
	switch algorithm {
	case "zstd-fast":
		settings.Level = 1
		settings.WindowSize = 16 * 1024 // 16KB
		settings.ThreadCount = 2
	case "zstd":
		settings.Level = 3
		settings.WindowSize = 64 * 1024 // 64KB
		settings.ThreadCount = 4
	case "zstd-high":
		settings.Level = 9
		settings.WindowSize = 256 * 1024 // 256KB
		settings.ThreadCount = 6
	case "zstd-max":
		settings.Level = 19
		settings.WindowSize = 1024 * 1024 // 1MB
		settings.ThreadCount = 8
	case "none":
		settings.Level = 0
		settings.WindowSize = 0
		settings.ThreadCount = 0
	}

	// Adjust for network conditions
	if networkCondition.BandwidthMBps < 1.0 {
		// Low bandwidth - increase compression level
		if settings.Level > 0 && settings.Level < 15 {
			settings.Level += 2
		}
	} else if networkCondition.BandwidthMBps > 50.0 {
		// High bandwidth - decrease compression level for speed
		if settings.Level > 3 {
			settings.Level -= 2
		}
	}

	return settings
}

// updateAlgorithmPerformance updates performance statistics for an algorithm.
func (acs *AdaptiveCompressionSelector) updateAlgorithmPerformance(result *CompressionResult) {
	perf, exists := acs.algorithmPerformance[result.Algorithm]
	if !exists {
		perf = &AlgorithmPerformance{
			Algorithm:            result.Algorithm,
			NetworkPerformance:   make(map[string]*NetworkPerformanceMetrics),
			FileTypePerformance:  make(map[string]*FileTypePerformanceMetrics),
			SizeRangePerformance: make(map[string]*SizeRangePerformanceMetrics),
			RecentPerformance:    make([]*PerformanceDataPoint, 0),
		}
		acs.algorithmPerformance[result.Algorithm] = perf
	}

	// Update running averages
	count := float64(perf.TotalCompressions)
	perf.AverageCompressionRatio = (perf.AverageCompressionRatio*count + result.CompressionRatio) / (count + 1)
	perf.AverageSpeedMBps = (perf.AverageSpeedMBps*count + result.SpeedMBps) / (count + 1)
	perf.AverageMemoryUsageMB = (perf.AverageMemoryUsageMB*count + result.MemoryUsageMB) / (count + 1)

	// Update success rate
	if result.Success {
		perf.SuccessRate = (perf.SuccessRate*count + 1.0) / (count + 1)
	} else {
		perf.SuccessRate = (perf.SuccessRate*count + 0.0) / (count + 1)
	}

	perf.TotalCompressions++
	perf.LastUpdated = time.Now()

	// Add to recent performance (keep last 100 data points)
	dataPoint := &PerformanceDataPoint{
		Timestamp:        time.Now(),
		CompressionRatio: result.CompressionRatio,
		SpeedMBps:        result.SpeedMBps,
		MemoryUsageMB:    result.MemoryUsageMB,
		Success:          result.Success,
	}

	perf.RecentPerformance = append(perf.RecentPerformance, dataPoint)
	if len(perf.RecentPerformance) > 100 {
		perf.RecentPerformance = perf.RecentPerformance[1:]
	}
}

// updateCompressionProfile updates the compression profile based on results.
func (acs *AdaptiveCompressionSelector) updateCompressionProfile(result *CompressionResult) {
	profile := acs.getCompressionProfile(result.ContentType)

	// Update algorithm effectiveness
	if profile.AlgorithmEffectiveness == nil {
		profile.AlgorithmEffectiveness = make(map[string]*EffectivenessMetrics)
	}

	effectiveness, exists := profile.AlgorithmEffectiveness[result.Algorithm]
	if !exists {
		effectiveness = &EffectivenessMetrics{}
		profile.AlgorithmEffectiveness[result.Algorithm] = effectiveness
	}

	// Update metrics using exponential moving average
	alpha := 0.1 // Learning rate
	effectiveness.CompressionRatio = (1-alpha)*effectiveness.CompressionRatio + alpha*result.CompressionRatio
	effectiveness.Speed = (1-alpha)*effectiveness.Speed + alpha*result.SpeedMBps
	effectiveness.MemoryUsage = (1-alpha)*effectiveness.MemoryUsage + alpha*result.MemoryUsageMB

	profile.SampleCount++
	profile.LastUpdated = time.Now()

	// Store back to map
	acs.compressionProfiles[result.ContentType] = profile
}

// GetAlgorithmPerformance returns performance statistics for an algorithm.
func (acs *AdaptiveCompressionSelector) GetAlgorithmPerformance(algorithm string) *AlgorithmPerformance {
	acs.mu.RLock()
	defer acs.mu.RUnlock()

	if perf, exists := acs.algorithmPerformance[algorithm]; exists {
		return perf
	}
	return nil
}

// GetCompressionProfile returns the compression profile for a content type.
func (acs *AdaptiveCompressionSelector) GetCompressionProfile(contentType string) *CompressionProfile {
	acs.mu.RLock()
	defer acs.mu.RUnlock()

	return acs.getCompressionProfile(contentType)
}

// UpdateCompressionRule updates or adds a compression rule.
func (acs *AdaptiveCompressionSelector) UpdateCompressionRule(extension string, rule *CompressionRule) {
	acs.mu.Lock()
	defer acs.mu.Unlock()

	acs.fileTypeRules[extension] = rule
}

// ClearPerformanceHistory clears performance history for all algorithms.
func (acs *AdaptiveCompressionSelector) ClearPerformanceHistory() {
	acs.mu.Lock()
	defer acs.mu.Unlock()

	for _, perf := range acs.algorithmPerformance {
		perf.RecentPerformance = make([]*PerformanceDataPoint, 0)
	}
}

// Supporting types for the adaptive compression selector

// NetworkThresholds defines bandwidth thresholds for different network types.
type NetworkThresholds struct {
	LowBandwidth    float64 `yaml:"low_bandwidth" json:"low_bandwidth"`
	MediumBandwidth float64 `yaml:"medium_bandwidth" json:"medium_bandwidth"`
	HighBandwidth   float64 `yaml:"high_bandwidth" json:"high_bandwidth"`
}

// EffectivenessMetrics tracks algorithm effectiveness metrics.
type EffectivenessMetrics struct {
	CompressionRatio float64 `json:"compression_ratio"`
	Speed            float64 `json:"speed_mbps"`
	MemoryUsage      float64 `json:"memory_usage_mb"`
}

// Additional supporting types will be implemented in separate files for better organization
type (
	FileTypeRule                struct{}
	PatternRule                 struct{}
	SizeBasedRules              struct{}
	NetworkOptimizationRules    struct{}
	NetworkPerformanceMetrics   struct{}
	FileTypePerformanceMetrics  struct{}
	SizeRangePerformanceMetrics struct{}
	PerformanceDataPoint        struct {
		Timestamp        time.Time
		CompressionRatio float64
		SpeedMBps        float64
		MemoryUsageMB    float64
		Success          bool
	}
	RuleCondition           struct{}
	PerformanceRequirements struct{}
	PredictedPerformance    struct {
		EstimatedCompressionRatio float64
		EstimatedSpeedMBps        float64
		EstimatedMemoryUsageMB    float64
		EstimatedCompressionTime  time.Duration
		Confidence                float64
	}
	AlgorithmOption struct {
		Algorithm   string
		Confidence  float64
		Performance *PredictedPerformance
		Reasoning   string
	}
	CompressionSettings struct {
		Algorithm   string
		Level       int
		WindowSize  int
		ThreadCount int
	}
	ContextualFactors struct {
		FileSize           int64
		ContentEntropy     float64
		NetworkLatency     float64
		NetworkBandwidth   float64
		NetworkReliability float64
		SystemLoad         float64
		MemoryAvailable    int64
		Priority           int
	}
	CompressionContext struct {
		FileName          string
		FileExtension     string
		FileSize          int64
		SystemLoad        float64
		AvailableMemoryMB int64
		Priority          int
		Deadline          time.Time
	}
	NetworkOptimization struct {
		Strategy            string
		PreferredAlgorithms []string
		Reasoning           string
	}
	MLRecommendation struct {
		Algorithm  string
		Confidence float64
		Reasoning  []string
	}
	ContextualRecommendation struct {
		Algorithm string
		Weight    float64
		Factors   []string
	}
	CompressionResult struct {
		Algorithm        string
		ContentType      string
		FileSize         int64
		CompressionRatio float64
		SpeedMBps        float64
		MemoryUsageMB    float64
		CompressionTime  time.Duration
		Success          bool
		ErrorMessage     string
		NetworkCondition *NetworkCondition
		Context          *CompressionContext
		Timestamp        time.Time
	}
)
