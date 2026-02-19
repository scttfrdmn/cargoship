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
				AlternativeOptions:  acs.generateFallbackOptions(rule),
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

// Additional supporting types
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

// ContentProfile describes the characteristics of content to be compressed.
type ContentProfile struct {
	ContentType      string                 `json:"content_type"`
	Size             int64                  `json:"size"`
	Entropy          float64                `json:"entropy"`
	Patterns         []ContentPattern       `json:"patterns"`
	Compressibility  float64                `json:"compressibility"`
	Metadata         map[string]interface{} `json:"metadata"`
	CompressionHints []CompressionHint      `json:"compression_hints,omitempty"`
	FileAlignment    []FileAlignment        `json:"file_alignment,omitempty"`
	EstimatedRatio   float64                `json:"estimated_ratio,omitempty"`
	AnalysisQuality  float64                `json:"analysis_quality,omitempty"`
}

// ContentPattern represents detected patterns in content.
type ContentPattern struct {
	Type            PatternType `json:"type"`
	Offset          int64       `json:"offset"`
	Length          int64       `json:"length"`
	Frequency       float64     `json:"frequency"`
	Compressibility float64     `json:"compressibility"`
	StartOffset     int64       `json:"start_offset"`
	EndOffset       int64       `json:"end_offset"`
}

// PatternType defines the type of detected pattern.
type PatternType int

const (
	PatternRepetitive PatternType = iota
	PatternStructured
	PatternText
	PatternBinary
	PatternRandom
)

// NetworkCondition describes current network conditions.
type NetworkCondition struct {
	Timestamp       time.Time    `json:"timestamp"`
	BandwidthMBps   float64      `json:"bandwidth_mbps"`
	LatencyMs       float64      `json:"latency_ms"`
	PacketLoss      float64      `json:"packet_loss"`
	Jitter          float64      `json:"jitter"`
	CongestionLevel float64      `json:"congestion_level"`
	Reliability     float64      `json:"reliability"`
	PredictedTrend  NetworkTrend `json:"predicted_trend"`
	NetworkType     string       `json:"network_type"`
	IsMetered       bool         `json:"is_metered"`
}

// NetworkTrend indicates predicted network performance direction.
type NetworkTrend int

const (
	TrendUnknown NetworkTrend = iota
	TrendImproving
	TrendDegrading
	TrendStable
	TrendVolatile
)

// CompressionLearningEngine provides machine learning based compression recommendations.
type CompressionLearningEngine struct {
	trainingData     []*CompressionTrainingPoint
	modelWeights     map[string]float64
	featureExtractor *FeatureExtractor
	predictionCache  map[string]*CachedPrediction
	config           *AdaptiveCompressionConfig
}

// CompressionTrainingPoint represents a training data point for ML learning.
type CompressionTrainingPoint struct {
	ContentProfile   *ContentProfile
	NetworkCondition *NetworkCondition
	Context          *CompressionContext
	ActualResult     *CompressionResult
	Timestamp        time.Time
}

// CachedPrediction stores cached ML predictions.
type CachedPrediction struct {
	Algorithm  string
	Confidence float64
	Reasoning  []string
	Timestamp  time.Time
}

// FeatureExtractor extracts features for ML processing.
type FeatureExtractor struct{}

// NewCompressionLearningEngine creates a new compression learning engine.
func NewCompressionLearningEngine(config *AdaptiveCompressionConfig) *CompressionLearningEngine {
	return &CompressionLearningEngine{
		trainingData:     make([]*CompressionTrainingPoint, 0),
		modelWeights:     make(map[string]float64),
		featureExtractor: NewFeatureExtractor(),
		predictionCache:  make(map[string]*CachedPrediction),
		config:           config,
	}
}

// NewFeatureExtractor creates a new feature extractor.
func NewFeatureExtractor() *FeatureExtractor {
	return &FeatureExtractor{}
}

// PredictOptimalAlgorithm uses ML to predict the optimal compression algorithm.
func (cle *CompressionLearningEngine) PredictOptimalAlgorithm(
	contentProfile *ContentProfile,
	networkCondition *NetworkCondition,
	context *CompressionContext,
) *MLRecommendation {
	// Extract features
	_ = cle.featureExtractor.ExtractFeatures(contentProfile, networkCondition, context)

	// Simple rule-based prediction (would be replaced with actual ML model)
	algorithm := "zstd"
	confidence := 0.7
	reasoning := []string{"Rule-based prediction"}

	// Adjust based on content type
	if contentProfile.ContentType == "text" && contentProfile.Entropy < 4.0 {
		algorithm = "zstd-high"
		confidence = 0.8
		reasoning = append(reasoning, "High compression for low-entropy text")
	} else if networkCondition.BandwidthMBps > 100 {
		algorithm = "zstd-fast"
		confidence = 0.75
		reasoning = append(reasoning, "Fast compression for high bandwidth")
	}

	// Cache the prediction
	cacheKey := cle.generateCacheKey(contentProfile, networkCondition, context)
	cle.predictionCache[cacheKey] = &CachedPrediction{
		Algorithm:  algorithm,
		Confidence: confidence,
		Reasoning:  reasoning,
		Timestamp:  time.Now(),
	}

	return &MLRecommendation{
		Algorithm:  algorithm,
		Confidence: confidence,
		Reasoning:  reasoning,
	}
}

// ExtractFeatures extracts features from the given inputs.
func (fe *FeatureExtractor) ExtractFeatures(
	contentProfile *ContentProfile,
	networkCondition *NetworkCondition,
	context *CompressionContext,
) map[string]float64 {
	_ = fe // Mark as used
	features := make(map[string]float64)

	// Content features
	features["content_size"] = float64(contentProfile.Size)
	features["content_entropy"] = contentProfile.Entropy
	features["content_compressibility"] = contentProfile.Compressibility

	// Network features
	features["network_bandwidth"] = networkCondition.BandwidthMBps
	features["network_latency"] = networkCondition.LatencyMs
	features["network_reliability"] = networkCondition.Reliability

	// Context features
	features["system_load"] = context.SystemLoad
	features["memory_available"] = float64(context.AvailableMemoryMB)
	features["priority"] = float64(context.Priority)

	return features
}

// generateCacheKey generates a cache key for predictions.
func (cle *CompressionLearningEngine) generateCacheKey(
	contentProfile *ContentProfile,
	networkCondition *NetworkCondition,
	context *CompressionContext,
) string {
	// Simple concatenation-based key (would use better hashing in practice)
	return contentProfile.ContentType + "_" +
		string(rune(int(networkCondition.BandwidthMBps))) + "_" +
		string(rune(context.Priority))
}

// LearnFromResult learns from actual compression results.
func (cle *CompressionLearningEngine) LearnFromResult(result *CompressionResult) {
	trainingPoint := &CompressionTrainingPoint{
		ContentProfile:   &ContentProfile{ContentType: result.ContentType, Size: result.FileSize},
		NetworkCondition: result.NetworkCondition,
		Context:          result.Context,
		ActualResult:     result,
		Timestamp:        time.Now(),
	}

	cle.trainingData = append(cle.trainingData, trainingPoint)

	// Keep only recent training data
	if len(cle.trainingData) > cle.config.PerformanceHistorySize {
		cle.trainingData = cle.trainingData[1:]
	}

	// Update model weights (simplified)
	cle.updateModelWeights(trainingPoint)
}

// updateModelWeights updates the ML model weights.
func (cle *CompressionLearningEngine) updateModelWeights(trainingPoint *CompressionTrainingPoint) {
	// Simple weight update based on success
	algorithm := trainingPoint.ActualResult.Algorithm
	if trainingPoint.ActualResult.Success {
		if weight, exists := cle.modelWeights[algorithm]; exists {
			cle.modelWeights[algorithm] = weight + 0.1
		} else {
			cle.modelWeights[algorithm] = 0.6
		}
	} else {
		if weight, exists := cle.modelWeights[algorithm]; exists {
			cle.modelWeights[algorithm] = weight - 0.05
		}
	}
}

// GetConfidence returns confidence in predictions for given inputs.
func (cle *CompressionLearningEngine) GetConfidence(
	contentProfile *ContentProfile,
	networkCondition *NetworkCondition,
) float64 {
	// Check cache first
	cacheKey := cle.generateCacheKey(contentProfile, networkCondition, &CompressionContext{})
	if cached, exists := cle.predictionCache[cacheKey]; exists {
		// Decay confidence over time
		age := time.Since(cached.Timestamp)
		decay := 1.0 - (float64(age) / float64(time.Hour*24))
		if decay < 0 {
			decay = 0
		}
		return cached.Confidence * decay
	}

	// Base confidence on training data size
	baseConfidence := 0.5
	if len(cle.trainingData) > 100 {
		baseConfidence = 0.7
	} else if len(cle.trainingData) > 20 {
		baseConfidence = 0.6
	}

	return baseConfidence
}

// CompressionPerformancePredictor predicts compression performance metrics.
type CompressionPerformancePredictor struct {
	historicalData   map[string][]*PerformanceDataPoint
	regressionModels map[string]*RegressionModel
	config           *AdaptiveCompressionConfig
}

// RegressionModel represents a simple regression model.
type RegressionModel struct{}

// NewCompressionPerformancePredictor creates a new performance predictor.
func NewCompressionPerformancePredictor(config *AdaptiveCompressionConfig) *CompressionPerformancePredictor {
	return &CompressionPerformancePredictor{
		historicalData:   make(map[string][]*PerformanceDataPoint),
		regressionModels: make(map[string]*RegressionModel),
		config:           config,
	}
}

// PredictPerformance predicts compression performance for given inputs.
func (cpp *CompressionPerformancePredictor) PredictPerformance(
	algorithm string,
	contentProfile *ContentProfile,
	networkCondition *NetworkCondition,
) *PredictedPerformance {
	// Use historical data and simple regression
	baseRatio := cpp.getBaseCompressionRatio(algorithm, contentProfile.ContentType)
	baseSpeed := cpp.getBaseSpeed(algorithm)
	baseMemory := cpp.getBaseMemoryUsage(algorithm)

	// Adjust based on content characteristics
	ratioMultiplier := 1.0
	if contentProfile.Entropy < 2.0 {
		ratioMultiplier = 0.7 // Better compression for low entropy
	} else if contentProfile.Entropy > 6.0 {
		ratioMultiplier = 1.2 // Worse compression for high entropy
	}

	// Adjust speed based on system load
	speedMultiplier := 1.0
	if networkCondition.BandwidthMBps < 10 {
		speedMultiplier = 0.9 // Slower due to network constraints
	}

	estimatedTime := time.Duration(float64(contentProfile.Size) / (baseSpeed * speedMultiplier * 1024 * 1024) * float64(time.Second))

	return &PredictedPerformance{
		EstimatedCompressionRatio: baseRatio * ratioMultiplier,
		EstimatedSpeedMBps:        baseSpeed * speedMultiplier,
		EstimatedMemoryUsageMB:    baseMemory,
		EstimatedCompressionTime:  estimatedTime,
		Confidence:                0.7,
	}
}

// getBaseCompressionRatio returns base compression ratio for algorithm and content type.
func (cpp *CompressionPerformancePredictor) getBaseCompressionRatio(algorithm, contentType string) float64 {
	ratios := map[string]map[string]float64{
		"zstd-fast": {"text": 0.45, "binary": 0.75, "image": 0.95, "json": 0.40},
		"zstd":      {"text": 0.35, "binary": 0.60, "image": 0.95, "json": 0.30},
		"zstd-high": {"text": 0.25, "binary": 0.50, "image": 0.95, "json": 0.20},
		"none":      {"text": 1.0, "binary": 1.0, "image": 1.0, "json": 1.0},
	}

	if algoRatios, exists := ratios[algorithm]; exists {
		if ratio, exists := algoRatios[contentType]; exists {
			return ratio
		}
	}

	return 0.5 // Default ratio
}

// getBaseSpeed returns base compression speed for algorithm.
func (cpp *CompressionPerformancePredictor) getBaseSpeed(algorithm string) float64 {
	speeds := map[string]float64{
		"zstd-fast": 300,
		"zstd":      150,
		"zstd-high": 50,
		"none":      1000,
	}

	if speed, exists := speeds[algorithm]; exists {
		return speed
	}

	return 100 // Default speed MB/s
}

// getBaseMemoryUsage returns base memory usage for algorithm.
func (cpp *CompressionPerformancePredictor) getBaseMemoryUsage(algorithm string) float64 {
	memory := map[string]float64{
		"zstd-fast": 16,
		"zstd":      32,
		"zstd-high": 64,
		"none":      0,
	}

	if mem, exists := memory[algorithm]; exists {
		return mem
	}

	return 32 // Default memory MB
}

// UpdateWithResult updates the predictor with actual results.
func (cpp *CompressionPerformancePredictor) UpdateWithResult(result *CompressionResult) {
	dataPoint := &PerformanceDataPoint{
		Timestamp:        time.Now(),
		CompressionRatio: result.CompressionRatio,
		SpeedMBps:        result.SpeedMBps,
		MemoryUsageMB:    result.MemoryUsageMB,
		Success:          result.Success,
	}

	if cpp.historicalData[result.Algorithm] == nil {
		cpp.historicalData[result.Algorithm] = make([]*PerformanceDataPoint, 0)
	}

	cpp.historicalData[result.Algorithm] = append(cpp.historicalData[result.Algorithm], dataPoint)

	// Keep only recent data
	if len(cpp.historicalData[result.Algorithm]) > cpp.config.PerformanceHistorySize {
		cpp.historicalData[result.Algorithm] = cpp.historicalData[result.Algorithm][1:]
	}
}

// ContextualCompressionOptimizer provides context-aware compression optimization.
type ContextualCompressionOptimizer struct {
	contextRules      []*ContextRule
	optimizationHints map[string]*OptimizationHint
	config            *AdaptiveCompressionConfig
}

// ContextRule defines context-based optimization rules.
type ContextRule struct {
	Name        string
	Condition   func(*ContentProfile, *NetworkCondition, *CompressionContext) bool
	Algorithm   string
	Weight      float64
	Description string
}

// OptimizationHint provides optimization hints for specific contexts.
type OptimizationHint struct {
	Algorithm  string
	Settings   *CompressionSettings
	Reasoning  string
	Confidence float64
}

// NewContextualCompressionOptimizer creates a new contextual optimizer.
func NewContextualCompressionOptimizer(config *AdaptiveCompressionConfig) *ContextualCompressionOptimizer {
	optimizer := &ContextualCompressionOptimizer{
		contextRules:      make([]*ContextRule, 0),
		optimizationHints: make(map[string]*OptimizationHint),
		config:            config,
	}

	optimizer.initializeDefaultRules()
	return optimizer
}

// initializeDefaultRules initializes default contextual optimization rules.
func (cco *ContextualCompressionOptimizer) initializeDefaultRules() {
	// High memory, low latency rule
	cco.contextRules = append(cco.contextRules, &ContextRule{
		Name: "HighMemoryLowLatency",
		Condition: func(cp *ContentProfile, nc *NetworkCondition, cc *CompressionContext) bool {
			return cc.AvailableMemoryMB > 1024 && nc.LatencyMs < 50
		},
		Algorithm:   "zstd-high",
		Weight:      0.8,
		Description: "Use high compression when memory is abundant and latency is low",
	})

	// Low memory rule
	cco.contextRules = append(cco.contextRules, &ContextRule{
		Name: "LowMemory",
		Condition: func(cp *ContentProfile, nc *NetworkCondition, cc *CompressionContext) bool {
			return cc.AvailableMemoryMB < 256
		},
		Algorithm:   "zstd-fast",
		Weight:      0.9,
		Description: "Use fast compression when memory is limited",
	})

	// High priority rule
	cco.contextRules = append(cco.contextRules, &ContextRule{
		Name: "HighPriority",
		Condition: func(cp *ContentProfile, nc *NetworkCondition, cc *CompressionContext) bool {
			return cc.Priority >= 8
		},
		Algorithm:   "zstd-fast",
		Weight:      0.7,
		Description: "Use fast compression for high priority tasks",
	})
}

// OptimizeSelection provides contextual optimization recommendations.
func (cco *ContextualCompressionOptimizer) OptimizeSelection(
	contentProfile *ContentProfile,
	networkCondition *NetworkCondition,
	context *CompressionContext,
	profile *CompressionProfile,
) *ContextualRecommendation {
	var bestRule *ContextRule
	var bestWeight float64

	// Evaluate all rules
	for _, rule := range cco.contextRules {
		if rule.Condition(contentProfile, networkCondition, context) {
			if rule.Weight > bestWeight {
				bestWeight = rule.Weight
				bestRule = rule
			}
		}
	}

	if bestRule != nil {
		return &ContextualRecommendation{
			Algorithm: bestRule.Algorithm,
			Weight:    bestRule.Weight,
			Factors:   []string{bestRule.Description},
		}
	}

	// No specific rule matched, return default
	return &ContextualRecommendation{
		Algorithm: "zstd",
		Weight:    0.5,
		Factors:   []string{"Default algorithm selection"},
	}
}

// RealtimeCompressionMonitor monitors compression performance in real-time.
type RealtimeCompressionMonitor struct {
	metrics    map[string]*RealtimeMetrics
	alerts     []*Alert
	thresholds *PerformanceThresholds
	config     *AdaptiveCompressionConfig
}

// RealtimeMetrics tracks real-time compression metrics.
type RealtimeMetrics struct {
	Algorithm           string
	ActiveCompressions  int
	AverageLatency      time.Duration
	ThroughputMBps      float64
	ErrorRate           float64
	ResourceUtilization float64
	LastUpdated         time.Time
}

// Alert represents a performance alert.
type Alert struct {
	Type      AlertType
	Algorithm string
	Message   string
	Severity  AlertSeverity
	Timestamp time.Time
	Resolved  bool
}

// AlertType defines types of alerts.
type AlertType int

const (
	AlertHighLatency AlertType = iota
	AlertLowThroughput
	AlertHighErrorRate
	AlertHighResourceUsage
)

// AlertSeverity defines alert severity levels.
type AlertSeverity int

const (
	SeverityInfo AlertSeverity = iota
	SeverityWarning
	SeverityCritical
)

// PerformanceThresholds defines performance alert thresholds.
type PerformanceThresholds struct {
	MaxLatency       time.Duration
	MinThroughput    float64
	MaxErrorRate     float64
	MaxResourceUsage float64
}

// NewRealtimeCompressionMonitor creates a new real-time monitor.
func NewRealtimeCompressionMonitor(config *AdaptiveCompressionConfig) *RealtimeCompressionMonitor {
	return &RealtimeCompressionMonitor{
		metrics: make(map[string]*RealtimeMetrics),
		alerts:  make([]*Alert, 0),
		thresholds: &PerformanceThresholds{
			MaxLatency:       time.Second * 30,
			MinThroughput:    1.0,
			MaxErrorRate:     0.05,
			MaxResourceUsage: 0.8,
		},
		config: config,
	}
}

// NetworkCompressionAdapter adapts compression for specific network types.
type NetworkCompressionAdapter struct {
	networkType string
	adaptations map[string]*NetworkAdaptation
	config      *AdaptiveCompressionConfig
}

// NetworkAdaptation defines network-specific adaptations.
type NetworkAdaptation struct {
	PreferredAlgorithms []string
	Settings            *CompressionSettings
	BufferSize          int
	ChunkSize           int
}

// NewNetworkCompressionAdapter creates a new network adapter.
func NewNetworkCompressionAdapter(networkType string, config *AdaptiveCompressionConfig) *NetworkCompressionAdapter {
	return &NetworkCompressionAdapter{
		networkType: networkType,
		adaptations: make(map[string]*NetworkAdaptation),
		config:      config,
	}
}

// CompressionHint provides hints for optimal compression.
type CompressionHint struct {
	Algorithm      string
	WindowSize     int
	Dictionary     []byte
	EstimatedRatio float64
}

// FileAlignment represents file boundary information.
type FileAlignment struct {
	Offset   int64
	FileName string
	FileSize int64
	FileType string
}
