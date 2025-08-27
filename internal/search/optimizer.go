package search

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/vector"
)

// SearchOptimizer enhances search results with intelligent ranking and query optimization.
type SearchOptimizer struct {
	ftsService   *FTSService
	vectorSearch *vector.SearchEngine

	// Configuration
	hybridEnabled     bool
	weightConfig      *SearchWeights
	rankingStrategies []RankingStrategy
}

// SearchWeights defines weights for different ranking factors.
type SearchWeights struct {
	// Base signals
	FTSScore    float64 `json:"fts_score"`    // FTS5 relevance score
	VectorScore float64 `json:"vector_score"` // Vector similarity score
	ExactMatch  float64 `json:"exact_match"`  // Exact symbol/path match

	// Quality signals
	HasTests    float64 `json:"has_tests"`    // Code has tests
	HasDocs     float64 `json:"has_docs"`     // Code has documentation
	CodeQuality float64 `json:"code_quality"` // General quality score

	// Recency and popularity
	RecentlyModified float64 `json:"recently_modified"` // Recently changed code
	AccessFrequency  float64 `json:"access_frequency"`  // Frequently accessed
	EntryPoint       float64 `json:"entry_point"`       // Public/main functions

	// Context relevance
	SameFile      float64 `json:"same_file"`      // Results from same file
	SamePackage   float64 `json:"same_package"`   // Results from same package
	DependencyRef float64 `json:"dependency_ref"` // Referenced in dependencies
}

// RankingStrategy defines different ranking approaches.
type RankingStrategy string

const (
	RankingRelevance  RankingStrategy = "relevance"  // Pure relevance-based
	RankingRecency    RankingStrategy = "recency"    // Favor recent changes
	RankingQuality    RankingStrategy = "quality"    // Favor high-quality code
	RankingPopularity RankingStrategy = "popularity" // Favor popular/used code
	RankingContextual RankingStrategy = "contextual" // Context-aware ranking
	RankingHybrid     RankingStrategy = "hybrid"     // Combine multiple factors
)

// OptimizedSearchOptions extends SearchOptions with optimization features.
type OptimizedSearchOptions struct {
	*SearchOptions

	// Optimization features
	Strategy          RankingStrategy `json:"strategy"`            // Primary ranking strategy
	HybridSearchRatio float64         `json:"hybrid_search_ratio"` // FTS vs Vector balance (0-1)
	ContextPath       *string         `json:"context_path"`        // Current file/path context
	BoostSimilar      bool            `json:"boost_similar"`       // Boost similar code patterns

	// Query enhancement
	ExpandQuery     bool `json:"expand_query"`     // Expand with synonyms
	SpellCorrection bool `json:"spell_correction"` // Apply spell correction
	AutoComplete    bool `json:"auto_complete"`    // Suggest completions

	// Result enhancement
	ClusterResults   bool `json:"cluster_results"`   // Group related results
	DiversifyResults bool `json:"diversify_results"` // Ensure result diversity
	ExplainRanking   bool `json:"explain_ranking"`   // Include ranking explanation
}

// OptimizedSearchResult extends SearchResult with optimization metadata.
type OptimizedSearchResult struct {
	*SearchResult

	// Enhanced scoring
	OptimizedScore  float64          `json:"optimized_score"`  // Final optimized score
	ScoreComponents *ScoreComponents `json:"score_components"` // Score breakdown
	RankingFactors  []string         `json:"ranking_factors"`  // Why this result ranks high

	// Context information
	ContextRelevance  float64 `json:"context_relevance"`  // Relevance to current context
	SimilarityCluster *int    `json:"similarity_cluster"` // Cluster ID for similar results

	// Metadata
	AccessCount    int             `json:"access_count"`    // How often accessed
	LastAccessed   *time.Time      `json:"last_accessed"`   // When last accessed
	QualityMetrics *QualityMetrics `json:"quality_metrics"` // Code quality indicators
}

// ScoreComponents breaks down the final score calculation.
type ScoreComponents struct {
	FTSComponent        float64 `json:"fts_component"`        // FTS5 contribution
	VectorComponent     float64 `json:"vector_component"`     // Vector similarity contribution
	QualityComponent    float64 `json:"quality_component"`    // Quality boost
	RecencyComponent    float64 `json:"recency_component"`    // Recency boost
	ContextComponent    float64 `json:"context_component"`    // Context relevance boost
	PopularityComponent float64 `json:"popularity_component"` // Popularity boost
}

// QualityMetrics provides detailed quality information.
type QualityMetrics struct {
	HasTests         bool    `json:"has_tests"`         // Has unit tests
	HasDocumentation bool    `json:"has_documentation"` // Has documentation
	TestCoverage     float64 `json:"test_coverage"`     // Test coverage percentage
	ComplexityScore  int     `json:"complexity_score"`  // Cyclomatic complexity
	LinesOfCode      int     `json:"lines_of_code"`     // Code size
	TechnicalDebt    float64 `json:"technical_debt"`    // Technical debt indicators
}

// DefaultSearchWeights returns balanced weights for general search.
func DefaultSearchWeights() *SearchWeights {
	return &SearchWeights{
		FTSScore:         1.0,  // Primary signal
		VectorScore:      0.8,  // Strong semantic signal
		ExactMatch:       2.0,  // Strong boost for exact matches
		HasTests:         0.3,  // Quality boost
		HasDocs:          0.2,  // Documentation boost
		CodeQuality:      0.4,  // General quality
		RecentlyModified: 0.2,  // Slight recency boost
		AccessFrequency:  0.3,  // Popular code boost
		EntryPoint:       0.25, // Public function boost
		SameFile:         0.15, // Context relevance
		SamePackage:      0.1,  // Package relevance
		DependencyRef:    0.2,  // Dependency relevance
	}
}

// NewSearchOptimizer creates a new search optimizer.
func NewSearchOptimizer(ftsService *FTSService, vectorSearch *vector.SearchEngine) *SearchOptimizer {
	return &SearchOptimizer{
		ftsService:        ftsService,
		vectorSearch:      vectorSearch,
		hybridEnabled:     true,
		weightConfig:      DefaultSearchWeights(),
		rankingStrategies: []RankingStrategy{RankingHybrid},
	}
}

// OptimizedSearch performs intelligent search with advanced ranking.
func (so *SearchOptimizer) OptimizedSearch(ctx context.Context, options *OptimizedSearchOptions) ([]*OptimizedSearchResult, error) {
	if options == nil {
		return nil, fmt.Errorf("search options cannot be nil")
	}

	if options.SearchOptions == nil {
		options.SearchOptions = DefaultSearchOptions()
	}

	startTime := time.Now()

	slog.Debug("Starting optimized search",
		"query", options.Query,
		"strategy", options.Strategy,
		"hybrid_ratio", options.HybridSearchRatio,
		"context_path", options.ContextPath)

	// Step 1: Enhance query if requested
	enhancedQuery := options.Query
	if options.ExpandQuery {
		enhancedQuery = so.expandQuery(enhancedQuery)
	}
	if options.SpellCorrection {
		enhancedQuery = so.correctSpelling(enhancedQuery)
	}

	// Step 2: Perform hybrid search if enabled
	var ftsResults []*SearchResult
	var vectorResults []*vector.SmartSearchResult
	var err error

	// Get FTS5 results
	options.SearchOptions.Query = enhancedQuery
	ftsResults, err = so.ftsService.Search(ctx, options.SearchOptions)
	if err != nil {
		slog.Warn("FTS search failed", "error", err)
	}

	// Get vector results if hybrid search is enabled
	if so.hybridEnabled && options.HybridSearchRatio > 0 {
		vectorOptions := &vector.SmartSearchOptions{
			Query:     enhancedQuery,
			SessionID: &options.SessionID,
			Languages: []string{},
			Limit:     options.Limit * 2, // Get more for better fusion
		}

		if options.Language != nil {
			vectorOptions.Languages = []string{*options.Language}
		}

		vectorResults, err = so.vectorSearch.SmartSearch(ctx, vectorOptions)
		if err != nil {
			slog.Warn("Vector search failed", "error", err)
		}
	}

	// Step 3: Combine and optimize results
	combinedResults := so.combineResults(ftsResults, vectorResults, options)

	// Step 4: Apply intelligent ranking
	optimizedResults := so.applyIntelligentRanking(ctx, combinedResults, options)

	// Step 5: Apply post-processing
	finalResults := so.postProcessResults(optimizedResults, options)

	slog.Info("Optimized search completed",
		"query", options.Query,
		"fts_results", len(ftsResults),
		"vector_results", len(vectorResults),
		"final_results", len(finalResults),
		"processing_time", time.Since(startTime))

	return finalResults, nil
}

// combineResults merges FTS and vector search results.
func (so *SearchOptimizer) combineResults(ftsResults []*SearchResult, vectorResults []*vector.SmartSearchResult, options *OptimizedSearchOptions) []*OptimizedSearchResult {
	resultMap := make(map[string]*OptimizedSearchResult)

	// Process FTS results
	for _, fts := range ftsResults {
		key := so.generateResultKey(fts.Path, fts.NodeID)

		optimized := &OptimizedSearchResult{
			SearchResult: fts,
			ScoreComponents: &ScoreComponents{
				FTSComponent: fts.Score,
			},
		}

		resultMap[key] = optimized
	}

	// Process vector results and merge with FTS
	ratio := options.HybridSearchRatio
	if ratio == 0 {
		ratio = 0.5 // Default 50/50 balance
	}

	for _, vec := range vectorResults {
		key := so.generateResultKey(vec.HybridSearchResult.CodeNode.Path, &vec.HybridSearchResult.CodeNode.ID)

		if existing, exists := resultMap[key]; exists {
			// Merge with existing FTS result
			existing.ScoreComponents.VectorComponent = vec.FinalScore
			existing.OptimizedScore = existing.ScoreComponents.FTSComponent*(1-ratio) + vec.FinalScore*ratio
		} else {
			// Create new result from vector data
			searchResult := so.convertVectorToSearchResult(vec)
			optimized := &OptimizedSearchResult{
				SearchResult: searchResult,
				ScoreComponents: &ScoreComponents{
					VectorComponent: vec.FinalScore,
				},
				OptimizedScore: vec.FinalScore * ratio,
			}
			resultMap[key] = optimized
		}
	}

	// Convert map to slice
	results := make([]*OptimizedSearchResult, 0, len(resultMap))
	for _, result := range resultMap {
		results = append(results, result)
	}

	return results
}

// applyIntelligentRanking applies the specified ranking strategy.
func (so *SearchOptimizer) applyIntelligentRanking(ctx context.Context, results []*OptimizedSearchResult, options *OptimizedSearchOptions) []*OptimizedSearchResult {
	switch options.Strategy {
	case RankingRelevance:
		return so.rankByRelevance(results, options)
	case RankingRecency:
		return so.rankByRecency(results, options)
	case RankingQuality:
		return so.rankByQuality(results, options)
	case RankingPopularity:
		return so.rankByPopularity(results, options)
	case RankingContextual:
		return so.rankByContext(results, options)
	case RankingHybrid:
		return so.rankByHybrid(results, options)
	default:
		return so.rankByHybrid(results, options)
	}
}

// rankByHybrid applies comprehensive hybrid ranking.
func (so *SearchOptimizer) rankByHybrid(results []*OptimizedSearchResult, options *OptimizedSearchOptions) []*OptimizedSearchResult {
	weights := so.weightConfig

	for _, result := range results {
		// Start with base score
		score := result.OptimizedScore
		if score == 0 {
			score = result.Score
		}

		components := result.ScoreComponents
		if components == nil {
			components = &ScoreComponents{}
			result.ScoreComponents = components
		}

		factors := []string{}

		// Exact match boost
		if so.isExactMatch(options.Query, result) {
			boost := weights.ExactMatch
			score += boost
			components.FTSComponent += boost
			factors = append(factors, "exact_match")
		}

		// Quality boosts
		quality := so.calculateQualityScore(result)
		qualityBoost := quality * weights.CodeQuality
		score += qualityBoost
		components.QualityComponent = qualityBoost
		if quality > 0.5 {
			factors = append(factors, "high_quality")
		}

		// Context relevance
		if options.ContextPath != nil {
			contextBoost := so.calculateContextRelevance(result, *options.ContextPath) * weights.SameFile
			score += contextBoost
			components.ContextComponent = contextBoost
			if contextBoost > 0 {
				factors = append(factors, "context_relevant")
			}
		}

		// Recency boost
		if result.UpdatedAt != nil {
			recencyBoost := so.calculateRecencyScore(*result.UpdatedAt) * weights.RecentlyModified
			score += recencyBoost
			components.RecencyComponent = recencyBoost
			if recencyBoost > 0.1 {
				factors = append(factors, "recently_modified")
			}
		}

		// Popularity boost
		popularityBoost := so.calculatePopularityScore(result) * weights.AccessFrequency
		score += popularityBoost
		components.PopularityComponent = popularityBoost
		if popularityBoost > 0.1 {
			factors = append(factors, "popular")
		}

		result.OptimizedScore = score
		result.RankingFactors = factors
	}

	// Sort by optimized score
	sort.Slice(results, func(i, j int) bool {
		return results[i].OptimizedScore > results[j].OptimizedScore
	})

	return results
}

// postProcessResults applies final result processing.
func (so *SearchOptimizer) postProcessResults(results []*OptimizedSearchResult, options *OptimizedSearchOptions) []*OptimizedSearchResult {
	// Apply clustering if requested
	if options.ClusterResults {
		results = so.clusterSimilarResults(results)
	}

	// Apply diversification if requested
	if options.DiversifyResults {
		results = so.diversifyResults(results)
	}

	// Apply final limit
	if len(results) > options.Limit {
		results = results[:options.Limit]
	}

	// Add quality metrics if requested
	if options.ExplainRanking {
		for _, result := range results {
			result.QualityMetrics = so.buildQualityMetrics(result)
		}
	}

	return results
}

// Helper methods for ranking

func (so *SearchOptimizer) isExactMatch(query string, result *OptimizedSearchResult) bool {
	query = strings.ToLower(strings.TrimSpace(query))

	// Check symbol match
	if result.Symbol != nil {
		if strings.ToLower(*result.Symbol) == query {
			return true
		}
	}

	// Check path match
	if strings.Contains(strings.ToLower(result.Path), query) {
		return true
	}

	return false
}

func (so *SearchOptimizer) calculateQualityScore(result *OptimizedSearchResult) float64 {
	score := 0.0

	// Basic quality indicators (would be enhanced with real data)
	if result.Comments != nil && len(*result.Comments) > 50 {
		score += 0.3 // Has meaningful comments
	}

	if result.Symbol != nil && len(*result.Symbol) > 0 {
		// Well-named functions/classes
		if !strings.Contains(*result.Symbol, "temp") && !strings.Contains(*result.Symbol, "test") {
			score += 0.2
		}
	}

	// Code length (neither too short nor too long)
	contentLength := len(result.Content)
	if contentLength > 100 && contentLength < 1000 {
		score += 0.2
	}

	return math.Min(score, 1.0)
}

func (so *SearchOptimizer) calculateContextRelevance(result *OptimizedSearchResult, contextPath string) float64 {
	// Same file
	if result.Path == contextPath {
		return 1.0
	}

	// Same directory
	resultDir := strings.Join(strings.Split(result.Path, "/")[:len(strings.Split(result.Path, "/"))-1], "/")
	contextDir := strings.Join(strings.Split(contextPath, "/")[:len(strings.Split(contextPath, "/"))-1], "/")

	if resultDir == contextDir {
		return 0.7
	}

	// Same package/module (simplified)
	if len(resultDir) > 0 && len(contextDir) > 0 {
		resultParts := strings.Split(resultDir, "/")
		contextParts := strings.Split(contextDir, "/")

		// Check for common prefix
		commonParts := 0
		minLen := len(resultParts)
		if len(contextParts) < minLen {
			minLen = len(contextParts)
		}

		for i := 0; i < minLen; i++ {
			if resultParts[i] == contextParts[i] {
				commonParts++
			} else {
				break
			}
		}

		if commonParts > 0 {
			return float64(commonParts) / float64(math.Max(float64(len(resultParts)), float64(len(contextParts))))
		}
	}

	return 0.0
}

func (so *SearchOptimizer) calculateRecencyScore(lastModified time.Time) float64 {
	daysSince := time.Since(lastModified).Hours() / 24

	// Recent changes get higher scores
	if daysSince < 1 {
		return 1.0 // Modified today
	} else if daysSince < 7 {
		return 0.8 // Modified this week
	} else if daysSince < 30 {
		return 0.5 // Modified this month
	} else if daysSince < 90 {
		return 0.2 // Modified this quarter
	}

	return 0.0
}

func (so *SearchOptimizer) calculatePopularityScore(result *OptimizedSearchResult) float64 {
	// Simplified popularity calculation
	// In a real implementation, this would use access logs, references, etc.

	score := 0.0

	// Public functions/classes are more popular
	if result.Symbol != nil {
		symbol := *result.Symbol
		if strings.Title(symbol) == symbol { // PascalCase indicates public
			score += 0.3
		}
	}

	// Entry points are popular
	if result.Symbol != nil && (*result.Symbol == "main" || *result.Symbol == "Main") {
		score += 0.5
	}

	// Common paths suggest popularity
	if strings.Contains(result.Path, "/api/") || strings.Contains(result.Path, "/cmd/") {
		score += 0.2
	}

	return score
}

// Utility methods

func (so *SearchOptimizer) generateResultKey(path string, nodeID *int64) string {
	if nodeID != nil {
		return fmt.Sprintf("%s#%d", path, *nodeID)
	}
	return path
}

func (so *SearchOptimizer) convertVectorToSearchResult(vec *vector.SmartSearchResult) *SearchResult {
	result := &SearchResult{
		Type:      "code",
		Score:     vec.FinalScore,
		SessionID: vec.HybridSearchResult.CodeNode.SessionID,
		Path:      vec.HybridSearchResult.CodeNode.Path,
		Content:   "", // Would need to fetch content
	}

	if vec.HybridSearchResult.CodeNode.ID != 0 {
		result.NodeID = &vec.HybridSearchResult.CodeNode.ID
	}

	if vec.HybridSearchResult.CodeNode.Symbol.Valid {
		result.Symbol = &vec.HybridSearchResult.CodeNode.Symbol.String
	}

	if vec.HybridSearchResult.CodeNode.Language.Valid {
		result.Language = &vec.HybridSearchResult.CodeNode.Language.String
	}

	if vec.HybridSearchResult.CodeNode.Kind.Valid {
		result.Kind = &vec.HybridSearchResult.CodeNode.Kind.String
	}

	return result
}

func (so *SearchOptimizer) expandQuery(query string) string {
	// Simple query expansion - in practice would use thesaurus, ontologies, etc.
	expansions := map[string][]string{
		"auth":     {"authentication", "authorize", "login"},
		"db":       {"database", "storage", "persistence"},
		"api":      {"endpoint", "service", "handler"},
		"test":     {"testing", "spec", "verification"},
		"config":   {"configuration", "settings", "options"},
		"user":     {"account", "profile", "person"},
		"data":     {"information", "content", "payload"},
		"process":  {"handle", "execute", "run"},
		"validate": {"verify", "check", "confirm"},
		"error":    {"exception", "failure", "problem"},
	}

	words := strings.Fields(strings.ToLower(query))
	var expanded []string

	for _, word := range words {
		expanded = append(expanded, word)
		if synonyms, exists := expansions[word]; exists {
			expanded = append(expanded, synonyms...)
		}
	}

	return strings.Join(expanded, " ")
}

func (so *SearchOptimizer) correctSpelling(query string) string {
	// Simple spell correction - in practice would use proper spell checking
	corrections := map[string]string{
		"authentiction": "authentication",
		"databse":       "database",
		"fucntion":      "function",
		"varible":       "variable",
		"lenght":        "length",
		"recieve":       "receive",
		"seperate":      "separate",
		"occured":       "occurred",
	}

	words := strings.Fields(strings.ToLower(query))
	corrected := make([]string, len(words))

	for i, word := range words {
		if correction, exists := corrections[word]; exists {
			corrected[i] = correction
		} else {
			corrected[i] = word
		}
	}

	return strings.Join(corrected, " ")
}

func (so *SearchOptimizer) clusterSimilarResults(results []*OptimizedSearchResult) []*OptimizedSearchResult {
	// Simple clustering by path similarity
	clusters := make(map[int][]*OptimizedSearchResult)
	clusterID := 0

	for _, result := range results {
		assigned := false

		// Try to assign to existing cluster
		for id, cluster := range clusters {
			if len(cluster) > 0 && so.areSimilarPaths(result.Path, cluster[0].Path) {
				result.SimilarityCluster = &id
				clusters[id] = append(clusters[id], result)
				assigned = true
				break
			}
		}

		// Create new cluster if not assigned
		if !assigned {
			result.SimilarityCluster = &clusterID
			clusters[clusterID] = []*OptimizedSearchResult{result}
			clusterID++
		}
	}

	return results
}

func (so *SearchOptimizer) areSimilarPaths(path1, path2 string) bool {
	// Simple path similarity check
	dir1 := strings.Join(strings.Split(path1, "/")[:len(strings.Split(path1, "/"))-1], "/")
	dir2 := strings.Join(strings.Split(path2, "/")[:len(strings.Split(path2, "/"))-1], "/")
	return dir1 == dir2
}

func (so *SearchOptimizer) diversifyResults(results []*OptimizedSearchResult) []*OptimizedSearchResult {
	// Ensure diversity by path, language, and type
	seen := make(map[string]bool)
	diversified := []*OptimizedSearchResult{}

	for _, result := range results {
		key := ""
		if result.Language != nil {
			key += *result.Language + ":"
		}
		if result.Kind != nil {
			key += *result.Kind + ":"
		}
		key += strings.Join(strings.Split(result.Path, "/")[:2], "/") // First two path segments

		if !seen[key] || len(diversified) < 3 {
			diversified = append(diversified, result)
			seen[key] = true
		}
	}

	return diversified
}

func (so *SearchOptimizer) buildQualityMetrics(result *OptimizedSearchResult) *QualityMetrics {
	// Build detailed quality metrics
	return &QualityMetrics{
		HasTests:         result.Comments != nil && strings.Contains(*result.Comments, "test"),
		HasDocumentation: result.Comments != nil && len(*result.Comments) > 20,
		TestCoverage:     0.0, // Would be calculated from actual test data
		ComplexityScore:  so.estimateComplexity(result.Content),
		LinesOfCode:      strings.Count(result.Content, "\n"),
		TechnicalDebt:    0.0, // Would be calculated from static analysis
	}
}

func (so *SearchOptimizer) estimateComplexity(content string) int {
	// Simple complexity estimation based on keywords
	complexity := 1 // Base complexity

	keywords := []string{"if", "for", "while", "switch", "case", "try", "catch"}
	for _, keyword := range keywords {
		complexity += strings.Count(strings.ToLower(content), keyword)
	}

	return complexity
}

// Additional ranking methods (simplified implementations)

func (so *SearchOptimizer) rankByRelevance(results []*OptimizedSearchResult, options *OptimizedSearchOptions) []*OptimizedSearchResult {
	// Sort purely by FTS + vector scores
	sort.Slice(results, func(i, j int) bool {
		scoreI := results[i].ScoreComponents.FTSComponent + results[i].ScoreComponents.VectorComponent
		scoreJ := results[j].ScoreComponents.FTSComponent + results[j].ScoreComponents.VectorComponent
		return scoreI > scoreJ
	})
	return results
}

func (so *SearchOptimizer) rankByRecency(results []*OptimizedSearchResult, options *OptimizedSearchOptions) []*OptimizedSearchResult {
	// Boost recently modified results
	for _, result := range results {
		if result.UpdatedAt != nil {
			recencyBoost := so.calculateRecencyScore(*result.UpdatedAt)
			result.OptimizedScore = result.Score + recencyBoost*2.0 // Double weight for recency
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].OptimizedScore > results[j].OptimizedScore
	})
	return results
}

func (so *SearchOptimizer) rankByQuality(results []*OptimizedSearchResult, options *OptimizedSearchOptions) []*OptimizedSearchResult {
	// Boost high-quality code
	for _, result := range results {
		qualityScore := so.calculateQualityScore(result)
		result.OptimizedScore = result.Score + qualityScore*1.5 // 1.5x weight for quality
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].OptimizedScore > results[j].OptimizedScore
	})
	return results
}

func (so *SearchOptimizer) rankByPopularity(results []*OptimizedSearchResult, options *OptimizedSearchOptions) []*OptimizedSearchResult {
	// Boost popular/frequently used code
	for _, result := range results {
		popularityScore := so.calculatePopularityScore(result)
		result.OptimizedScore = result.Score + popularityScore*1.2 // 1.2x weight for popularity
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].OptimizedScore > results[j].OptimizedScore
	})
	return results
}

func (so *SearchOptimizer) rankByContext(results []*OptimizedSearchResult, options *OptimizedSearchOptions) []*OptimizedSearchResult {
	// Boost contextually relevant results
	if options.ContextPath != nil {
		for _, result := range results {
			contextScore := so.calculateContextRelevance(result, *options.ContextPath)
			result.OptimizedScore = result.Score + contextScore*3.0 // High weight for context
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].OptimizedScore > results[j].OptimizedScore
	})
	return results
}
