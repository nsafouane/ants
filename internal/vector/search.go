package vector

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"
)

// SearchEngine provides advanced similarity search with intelligent ranking.
type SearchEngine struct {
	embeddingService EmbeddingGenerator
	vectorBridge     *Bridge
	queries          QueryInterface

	// Search configuration
	defaultLimit    int
	maxLimit        int
	scoringWeights  *ScoringWeights
	filterOptimizer *FilterOptimizer
}

// ScoringWeights defines weights for different ranking factors.
type ScoringWeights struct {
	VectorSimilarity float64 `json:"vector_similarity"` // Vector cosine similarity
	AnalysisTier     float64 `json:"analysis_tier"`     // Analysis tier completion bonus
	CodeQuality      float64 `json:"code_quality"`      // Has tests/docs bonus
	Recency          float64 `json:"recency"`           // Recently modified bonus
	Complexity       float64 `json:"complexity"`        // Complexity penalty/bonus
	RiskScore        float64 `json:"risk_score"`        // Risk score penalty
	PopularityBoost  float64 `json:"popularity_boost"`  // Entry point/public boost
	SymbolMatch      float64 `json:"symbol_match"`      // Symbol name match bonus
}

// FilterOptimizer optimizes filter application order for performance.
type FilterOptimizer struct {
	sessionFilters []string // High-selectivity filters
	booleanFilters []string // Fast boolean filters
	rangeFilters   []string // Numeric range filters
	textFilters    []string // Expensive text filters
}

// SmartSearchOptions provides intelligent search configuration.
type SmartSearchOptions struct {
	// Query parameters
	Query          string    `json:"query"`           // Text query for embedding
	QueryEmbedding []float32 `json:"query_embedding"` // Pre-computed embedding

	// Filtering
	SessionID       *string  `json:"session_id,omitempty"`
	Languages       []string `json:"languages,omitempty"`
	Kinds           []string `json:"kinds,omitempty"`
	MinAnalysisTier *int     `json:"min_analysis_tier,omitempty"`
	RequiredTags    []string `json:"required_tags,omitempty"`
	ExcludedTags    []string `json:"excluded_tags,omitempty"`

	// Quality filters
	RequireTests      *bool `json:"require_tests,omitempty"`
	RequireDocs       *bool `json:"require_docs,omitempty"`
	PublicOnly        *bool `json:"public_only,omitempty"`
	ExcludeDeprecated *bool `json:"exclude_deprecated,omitempty"`

	// Complexity and risk
	MaxComplexity *int     `json:"max_complexity,omitempty"`
	MaxRiskScore  *float64 `json:"max_risk_score,omitempty"`

	// Result configuration
	Limit           int     `json:"limit"`            // Max results
	MinScore        float64 `json:"min_score"`        // Minimum similarity score
	IncludeMetadata bool    `json:"include_metadata"` // Include full metadata
	BoostRecent     bool    `json:"boost_recent"`     // Boost recently modified
	BoostQuality    bool    `json:"boost_quality"`    // Boost high-quality code

	// Advanced options
	DiversityFactor float64         `json:"diversity_factor"` // Result diversity (0-1)
	ExplainScoring  bool            `json:"explain_scoring"`  // Include score explanation
	CustomWeights   *ScoringWeights `json:"custom_weights"`   // Custom scoring weights
}

// SmartSearchResult represents an enhanced search result with scoring details.
type SmartSearchResult struct {
	// Basic result data
	*HybridSearchResult

	// Enhanced scoring
	FinalScore     float64         `json:"final_score"`     // Combined score
	ScoreBreakdown *ScoreBreakdown `json:"score_breakdown"` // Score explanation
	MatchReasons   []string        `json:"match_reasons"`   // Why this result matched
	QualitySignals *QualitySignals `json:"quality_signals"` // Quality indicators

	// Context information
	RelatedResults []int64         `json:"related_results"` // Related node IDs
	SessionContext *SessionContext `json:"session_context"` // Session-specific context
}

// ScoreBreakdown explains how the final score was calculated.
type ScoreBreakdown struct {
	VectorScore     float64 `json:"vector_score"`     // Raw vector similarity
	TierBonus       float64 `json:"tier_bonus"`       // Analysis tier bonus
	QualityBonus    float64 `json:"quality_bonus"`    // Quality indicators bonus
	RecencyBonus    float64 `json:"recency_bonus"`    // Recency bonus
	ComplexityScore float64 `json:"complexity_score"` // Complexity adjustment
	RiskPenalty     float64 `json:"risk_penalty"`     // Risk score penalty
	PopularityBonus float64 `json:"popularity_bonus"` // Popularity boost
	SymbolBonus     float64 `json:"symbol_bonus"`     // Symbol match bonus
	DiversityAdjust float64 `json:"diversity_adjust"` // Diversity adjustment
}

// QualitySignals contains indicators of code quality.
type QualitySignals struct {
	HasTests        bool     `json:"has_tests"`
	HasDocs         bool     `json:"has_docs"`
	IsPublic        bool     `json:"is_public"`
	IsEntry         bool     `json:"is_entry"`
	ComplexityLevel string   `json:"complexity_level"` // low, medium, high
	RiskLevel       string   `json:"risk_level"`       // low, medium, high
	AnalysisTier    int      `json:"analysis_tier"`
	Tags            []string `json:"tags"`
}

// SessionContext provides session-specific context for results.
type SessionContext struct {
	TotalNodesInSession int      `json:"total_nodes_in_session"`
	SessionAnalysisTier int      `json:"session_analysis_tier"` // Average tier
	SessionLanguages    []string `json:"session_languages"`     // Languages in session
	RelatedFileCount    int      `json:"related_file_count"`    // Files in same directory
}

// DefaultScoringWeights returns balanced weights for general code search.
func DefaultScoringWeights() *ScoringWeights {
	return &ScoringWeights{
		VectorSimilarity: 1.0,  // Primary signal
		AnalysisTier:     0.3,  // Tier 3 analysis gets +30% boost
		CodeQuality:      0.2,  // Quality code gets +20% boost
		Recency:          0.1,  // Recent changes get +10% boost
		Complexity:       -0.1, // High complexity gets -10% penalty
		RiskScore:        -0.2, // High risk gets -20% penalty
		PopularityBoost:  0.15, // Public/entry points get +15% boost
		SymbolMatch:      0.25, // Symbol name matches get +25% boost
	}
}

// DefaultFilterOptimizer returns optimized filter application order.
func DefaultFilterOptimizer() *FilterOptimizer {
	return &FilterOptimizer{
		sessionFilters: []string{"session_id"},                                          // Apply first (high selectivity)
		booleanFilters: []string{"has_tests", "has_docs", "is_public", "is_deprecated"}, // Fast boolean checks
		rangeFilters:   []string{"complexity", "risk_score", "tier_completed"},          // Numeric comparisons
		textFilters:    []string{"symbol", "path", "tags"},                              // Text matching (slowest)
	}
}

// NewSearchEngine creates a new advanced search engine.
func NewSearchEngine(
	embeddingService EmbeddingGenerator,
	vectorBridge *Bridge,
	queries QueryInterface,
) *SearchEngine {
	return &SearchEngine{
		embeddingService: embeddingService,
		vectorBridge:     vectorBridge,
		queries:          queries,
		defaultLimit:     10,
		maxLimit:         100,
		scoringWeights:   DefaultScoringWeights(),
		filterOptimizer:  DefaultFilterOptimizer(),
	}
}

// SmartSearch performs intelligent similarity search with advanced ranking.
func (se *SearchEngine) SmartSearch(ctx context.Context, options *SmartSearchOptions) ([]*SmartSearchResult, error) {
	if options == nil {
		return nil, fmt.Errorf("search options cannot be nil")
	}

	startTime := time.Now()

	slog.Debug("Starting smart search",
		"query", options.Query,
		"has_embedding", len(options.QueryEmbedding) > 0,
		"session_id", options.SessionID,
		"limit", options.Limit)

	// Step 1: Prepare query embedding
	queryEmbedding, err := se.prepareQueryEmbedding(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare query embedding: %w", err)
	}

	// Step 2: Configure search parameters
	searchConfig := se.buildSearchConfig(options)

	// Step 3: Perform hybrid search with optimized filters
	hybridResults, err := se.performOptimizedSearch(ctx, queryEmbedding, searchConfig)
	if err != nil {
		return nil, fmt.Errorf("hybrid search failed: %w", err)
	}

	if len(hybridResults) == 0 {
		slog.Debug("No hybrid search results found")
		return []*SmartSearchResult{}, nil
	}

	// Step 4: Enhance results with smart scoring
	enhancedResults, err := se.enhanceResultsWithScoring(ctx, hybridResults, options)
	if err != nil {
		return nil, fmt.Errorf("result enhancement failed: %w", err)
	}

	// Step 5: Apply intelligent ranking
	se.applyIntelligentRanking(enhancedResults, options)

	// Step 6: Apply diversity filtering if requested
	if options.DiversityFactor > 0 {
		enhancedResults = se.applyDiversityFiltering(enhancedResults, options.DiversityFactor)
	}

	// Step 7: Limit results
	finalLimit := options.Limit
	if finalLimit <= 0 {
		finalLimit = se.defaultLimit
	}
	if finalLimit > se.maxLimit {
		finalLimit = se.maxLimit
	}

	if len(enhancedResults) > finalLimit {
		enhancedResults = enhancedResults[:finalLimit]
	}

	slog.Info("Smart search completed",
		"query", options.Query,
		"results_found", len(enhancedResults),
		"processing_time", time.Since(startTime),
		"session_id", options.SessionID)

	return enhancedResults, nil
}

// prepareQueryEmbedding gets or generates the query embedding.
func (se *SearchEngine) prepareQueryEmbedding(ctx context.Context, options *SmartSearchOptions) ([]float32, error) {
	// Use provided embedding if available
	if len(options.QueryEmbedding) > 0 {
		return options.QueryEmbedding, nil
	}

	// Generate embedding from query text
	if options.Query == "" {
		return nil, fmt.Errorf("either query text or query embedding must be provided")
	}

	embedding, err := se.embeddingService.GenerateEmbedding(ctx, options.Query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	return embedding, nil
}

// buildSearchConfig converts SmartSearchOptions to HybridSearchOptions.
func (se *SearchEngine) buildSearchConfig(options *SmartSearchOptions) *HybridSearchOptions {
	// Start with a larger vector limit for better post-processing
	vectorLimit := options.Limit * 3
	if vectorLimit < 50 {
		vectorLimit = 50
	}
	if vectorLimit > 200 {
		vectorLimit = 200
	}

	config := &HybridSearchOptions{
		VectorLimit: vectorLimit,
		FinalLimit:  options.Limit * 2, // Get more for smart ranking
		SessionID:   options.SessionID,
		Languages:   options.Languages,
		Kinds:       options.Kinds,
	}

	// Add complexity filters
	if options.MaxComplexity != nil {
		config.MaxComplexity = options.MaxComplexity
	}

	return config
}

// performOptimizedSearch executes the hybrid search with filter optimization.
func (se *SearchEngine) performOptimizedSearch(ctx context.Context, embedding []float32, config *HybridSearchOptions) ([]*HybridSearchResult, error) {
	// Set the query vector
	config.Vector = embedding

	// Perform the hybrid search using our existing bridge
	return se.vectorBridge.Search(ctx, config)
}

// enhanceResultsWithScoring adds smart scoring and metadata to results.
func (se *SearchEngine) enhanceResultsWithScoring(ctx context.Context, hybridResults []*HybridSearchResult, options *SmartSearchOptions) ([]*SmartSearchResult, error) {
	enhanced := make([]*SmartSearchResult, 0, len(hybridResults))

	// Get session context if needed
	var sessionContext *SessionContext
	if options.SessionID != nil {
		sessionCtx, err := se.buildSessionContext(ctx, *options.SessionID)
		if err != nil {
			slog.Warn("Failed to build session context", "error", err)
			// Continue without session context
		} else {
			sessionContext = sessionCtx
		}
	}

	for _, result := range hybridResults {
		// Build quality signals
		qualitySignals := se.buildQualitySignals(result)

		// Apply smart scoring
		finalScore, breakdown := se.calculateSmartScore(result, qualitySignals, options)

		// Build match reasons
		matchReasons := se.buildMatchReasons(result, qualitySignals, options)

		// Create enhanced result
		enhanced = append(enhanced, &SmartSearchResult{
			HybridSearchResult: result,
			FinalScore:         finalScore,
			ScoreBreakdown:     breakdown,
			MatchReasons:       matchReasons,
			QualitySignals:     qualitySignals,
			SessionContext:     sessionContext,
		})
	}

	return enhanced, nil
}

// calculateSmartScore computes the final score using multiple factors.
func (se *SearchEngine) calculateSmartScore(result *HybridSearchResult, quality *QualitySignals, options *SmartSearchOptions) (float64, *ScoreBreakdown) {
	weights := se.scoringWeights
	if options.CustomWeights != nil {
		weights = options.CustomWeights
	}

	// Base vector similarity score
	vectorScore := float64(result.Score)

	// Analysis tier bonus
	tierBonus := 0.0
	if quality.AnalysisTier > 0 {
		tierBonus = weights.AnalysisTier * float64(quality.AnalysisTier) / 3.0 // Normalize to 0-1
	}

	// Quality bonus
	qualityBonus := 0.0
	if quality.HasTests {
		qualityBonus += weights.CodeQuality * 0.5
	}
	if quality.HasDocs {
		qualityBonus += weights.CodeQuality * 0.5
	}

	// Recency bonus (based on last modified)
	recencyBonus := 0.0
	if options.BoostRecent && result.CodeNode != nil {
		// Calculate recency based on update timestamp
		// This would need actual timestamp data from the code node
		recencyBonus = weights.Recency * 0.5 // Placeholder
	}

	// Complexity penalty/bonus
	complexityScore := 0.0
	if complexity := se.extractComplexity(result); complexity > 0 {
		// Normalize complexity (assuming 1-20 range)
		normalizedComplexity := float64(complexity) / 20.0
		complexityScore = weights.Complexity * normalizedComplexity
	}

	// Risk penalty
	riskPenalty := 0.0
	if riskScore := se.extractRiskScore(result); riskScore > 0 {
		riskPenalty = weights.RiskScore * float64(riskScore)
	}

	// Popularity bonus
	popularityBonus := 0.0
	if quality.IsPublic {
		popularityBonus += weights.PopularityBoost * 0.5
	}
	if quality.IsEntry {
		popularityBonus += weights.PopularityBoost * 0.5
	}

	// Symbol match bonus
	symbolBonus := 0.0
	if options.Query != "" && result.CodeNode != nil && result.CodeNode.Symbol.Valid {
		if se.isSymbolMatch(options.Query, result.CodeNode.Symbol.String) {
			symbolBonus = weights.SymbolMatch
		}
	}

	// Calculate final score
	finalScore := vectorScore + tierBonus + qualityBonus + recencyBonus + complexityScore + riskPenalty + popularityBonus + symbolBonus

	// Ensure score is non-negative
	if finalScore < 0 {
		finalScore = 0
	}

	breakdown := &ScoreBreakdown{
		VectorScore:     vectorScore,
		TierBonus:       tierBonus,
		QualityBonus:    qualityBonus,
		RecencyBonus:    recencyBonus,
		ComplexityScore: complexityScore,
		RiskPenalty:     riskPenalty,
		PopularityBonus: popularityBonus,
		SymbolBonus:     symbolBonus,
	}

	return finalScore, breakdown
}

// buildQualitySignals extracts quality indicators from a result.
func (se *SearchEngine) buildQualitySignals(result *HybridSearchResult) *QualitySignals {
	signals := &QualitySignals{
		Tags: []string{},
	}

	if result.VectorMetadata != nil {
		// Extract boolean signals
		if hasTests, ok := result.VectorMetadata["has_tests"].(bool); ok {
			signals.HasTests = hasTests
		}
		if hasDocs, ok := result.VectorMetadata["has_docs"].(bool); ok {
			signals.HasDocs = hasDocs
		}
		if isPublic, ok := result.VectorMetadata["is_public"].(bool); ok {
			signals.IsPublic = isPublic
		}
		if isEntry, ok := result.VectorMetadata["is_entry"].(bool); ok {
			signals.IsEntry = isEntry
		}

		// Extract numeric signals
		if tier, ok := result.VectorMetadata["tier_completed"].(int); ok {
			signals.AnalysisTier = tier
		}

		// Extract complexity level
		if complexity := se.extractComplexity(result); complexity > 0 {
			signals.ComplexityLevel = se.categorizeComplexity(complexity)
		}

		// Extract risk level
		if riskScore := se.extractRiskScore(result); riskScore > 0 {
			signals.RiskLevel = se.categorizeRisk(riskScore)
		}

		// Extract tags
		if tags, ok := result.VectorMetadata["tags"].([]string); ok {
			signals.Tags = tags
		}
	}

	return signals
}

// buildMatchReasons explains why a result matched the query.
func (se *SearchEngine) buildMatchReasons(result *HybridSearchResult, quality *QualitySignals, options *SmartSearchOptions) []string {
	reasons := []string{}

	// Vector similarity
	if result.Score > 0.8 {
		reasons = append(reasons, "High semantic similarity")
	} else if result.Score > 0.6 {
		reasons = append(reasons, "Moderate semantic similarity")
	} else {
		reasons = append(reasons, "Low semantic similarity")
	}

	// Quality indicators
	if quality.HasTests && quality.HasDocs {
		reasons = append(reasons, "High-quality code with tests and documentation")
	} else if quality.HasTests {
		reasons = append(reasons, "Code includes tests")
	} else if quality.HasDocs {
		reasons = append(reasons, "Code includes documentation")
	}

	// Analysis tier
	if quality.AnalysisTier >= 3 {
		reasons = append(reasons, "Fully analyzed code (Tier 3)")
	} else if quality.AnalysisTier >= 2 {
		reasons = append(reasons, "Partially analyzed code (Tier 2)")
	}

	// Symbol match
	if options.Query != "" && result.CodeNode != nil && result.CodeNode.Symbol.Valid {
		if se.isSymbolMatch(options.Query, result.CodeNode.Symbol.String) {
			reasons = append(reasons, "Symbol name matches query")
		}
	}

	// Language match
	if len(options.Languages) > 0 && result.CodeNode != nil && result.CodeNode.Language.Valid {
		for _, lang := range options.Languages {
			if result.CodeNode.Language.String == lang {
				reasons = append(reasons, fmt.Sprintf("Written in %s", lang))
				break
			}
		}
	}

	return reasons
}

// buildSessionContext creates session-specific context information.
func (se *SearchEngine) buildSessionContext(ctx context.Context, sessionID string) (*SessionContext, error) {
	// Count total nodes in session
	totalNodes, err := se.queries.CountCodeNodesBySession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to count session nodes: %w", err)
	}

	// TODO: Implement additional session context gathering
	// This would include analysis tier statistics, language distribution, etc.

	return &SessionContext{
		TotalNodesInSession: int(totalNodes),
		SessionAnalysisTier: 2,              // Placeholder
		SessionLanguages:    []string{"go"}, // Placeholder
		RelatedFileCount:    0,              // Placeholder
	}, nil
}

// applyIntelligentRanking sorts results by final score.
func (se *SearchEngine) applyIntelligentRanking(results []*SmartSearchResult, options *SmartSearchOptions) {
	sort.Slice(results, func(i, j int) bool {
		return results[i].FinalScore > results[j].FinalScore
	})
}

// applyDiversityFiltering reduces redundant results.
func (se *SearchEngine) applyDiversityFiltering(results []*SmartSearchResult, diversityFactor float64) []*SmartSearchResult {
	if diversityFactor <= 0 || len(results) <= 1 {
		return results
	}

	filtered := []*SmartSearchResult{results[0]} // Always include top result

	for i := 1; i < len(results); i++ {
		candidate := results[i]

		// Check diversity against already selected results
		isDiverse := true
		for _, selected := range filtered {
			if se.calculateDiversityScore(candidate, selected) < diversityFactor {
				isDiverse = false
				break
			}
		}

		if isDiverse {
			filtered = append(filtered, candidate)
		}
	}

	return filtered
}

// calculateDiversityScore computes diversity between two results (0-1, 1=completely different).
func (se *SearchEngine) calculateDiversityScore(a, b *SmartSearchResult) float64 {
	score := 0.0
	factors := 0

	// Path diversity
	if a.CodeNode != nil && b.CodeNode != nil {
		if a.CodeNode.Path != b.CodeNode.Path {
			score += 1.0
		}
		factors++

		// Language diversity
		if a.CodeNode.Language.Valid && b.CodeNode.Language.Valid {
			if a.CodeNode.Language.String != b.CodeNode.Language.String {
				score += 1.0
			}
			factors++
		}

		// Kind diversity
		if a.CodeNode.Kind.Valid && b.CodeNode.Kind.Valid {
			if a.CodeNode.Kind.String != b.CodeNode.Kind.String {
				score += 0.5
			}
			factors++
		}
	}

	if factors == 0 {
		return 1.0 // Default to diverse if we can't compare
	}

	return score / float64(factors)
}

// Helper methods for extracting metadata values

func (se *SearchEngine) extractComplexity(result *HybridSearchResult) int {
	if result.VectorMetadata != nil {
		if complexity, ok := result.VectorMetadata["complexity"].(float64); ok {
			return int(complexity)
		}
		if complexity, ok := result.VectorMetadata["complexity"].(int); ok {
			return complexity
		}
	}
	return 0
}

func (se *SearchEngine) extractRiskScore(result *HybridSearchResult) float32 {
	if result.VectorMetadata != nil {
		if risk, ok := result.VectorMetadata["risk_score"].(float64); ok {
			return float32(risk)
		}
		if risk, ok := result.VectorMetadata["risk_score"].(float32); ok {
			return risk
		}
	}
	return 0.0
}

func (se *SearchEngine) categorizeComplexity(complexity int) string {
	if complexity <= 5 {
		return "low"
	} else if complexity <= 15 {
		return "medium"
	}
	return "high"
}

func (se *SearchEngine) categorizeRisk(riskScore float32) string {
	if riskScore <= 0.3 {
		return "low"
	} else if riskScore <= 0.7 {
		return "medium"
	}
	return "high"
}

func (se *SearchEngine) isSymbolMatch(query, symbol string) bool {
	// Simple substring match for now
	// Could be enhanced with fuzzy matching, camelCase splitting, etc.
	return len(query) >= 3 && (symbol == query ||
		symbol == query+"()" ||
		len(symbol) >= len(query) && symbol[:len(query)] == query)
}

// GetSuggestions provides search suggestions based on session content.
func (se *SearchEngine) GetSuggestions(ctx context.Context, sessionID string, limit int) ([]string, error) {
	// TODO: Implement intelligent search suggestions
	// This could analyze session content to suggest relevant searches
	return []string{}, nil
}

// HealthCheck verifies search engine functionality.
func (se *SearchEngine) HealthCheck(ctx context.Context) error {
	// Check embedding service
	if err := se.embeddingService.HealthCheck(ctx); err != nil {
		return fmt.Errorf("embedding service health check failed: %w", err)
	}

	// Check vector bridge
	if err := se.vectorBridge.vectorClient.Ping(ctx); err != nil {
		return fmt.Errorf("vector client health check failed: %w", err)
	}

	return nil
}
