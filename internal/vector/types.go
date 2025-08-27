package vector

import "context"

// VectorClient defines the interface for vector database operations.
type VectorClient interface {
	// Client management
	Close() error
	Ping(ctx context.Context) error

	// Collection management
	CollectionExists(ctx context.Context) (bool, error)
	CreateCollection(ctx context.Context) error
	EnsureCollection(ctx context.Context) error

	// Embedding operations
	UpsertEmbedding(ctx context.Context, embedding *CodeEmbedding) error
	UpsertEmbeddings(ctx context.Context, embeddings []*CodeEmbedding) error
	DeleteEmbedding(ctx context.Context, vectorID string) error
	DeleteEmbeddingsByNodeID(ctx context.Context, nodeID int64) error
	DeleteEmbeddingsBySessionID(ctx context.Context, sessionID string) error

	// Search operations
	SearchSimilar(ctx context.Context, vector []float32, limit int, filter map[string]interface{}) ([]*SearchResult, error)

	// Statistics
	CountEmbeddings(ctx context.Context) (int64, error)
	CountEmbeddingsBySessionID(ctx context.Context, sessionID string) (int64, error)
}

// Ensure Client implements VectorClient interface
var _ VectorClient = (*Client)(nil)

// EmbeddingMetadata contains metadata fields for code embeddings.
type EmbeddingMetadata struct {
	// Core identification
	NodeID    int64  `json:"node_id"`    // References code_nodes.id
	SessionID string `json:"session_id"` // Session scope

	// Code structure
	Path       string `json:"path"`       // File path
	Language   string `json:"language"`   // Programming language
	Kind       string `json:"kind"`       // function, class, method, etc.
	Symbol     string `json:"symbol"`     // Symbol name
	StartLine  int    `json:"start_line"` // Starting line number
	EndLine    int    `json:"end_line"`   // Ending line number
	Complexity int    `json:"complexity"` // Cyclomatic complexity
	LineCount  int    `json:"line_count"` // Lines of code

	// Git and change tracking
	LastModified int64  `json:"last_modified"` // Unix timestamp
	ChangeFreq   string `json:"change_freq"`   // hot, warm, cold
	Authors      int    `json:"authors"`       // Number of unique authors

	// Analysis tier results
	TierCompleted int      `json:"tier_completed"` // Highest completed analysis tier (1, 2, 3)
	Tags          []string `json:"tags"`           // Semantic tags from Tagger Ant
	Summary       string   `json:"summary"`        // One-line summary from Doc Ant
	RiskScore     float32  `json:"risk_score"`     // Risk assessment from Reasoning Ant

	// Search optimization
	HasTests     bool `json:"has_tests"`     // Whether code has associated tests
	HasDocs      bool `json:"has_docs"`      // Whether code has documentation
	IsEntry      bool `json:"is_entry"`      // Whether this is an entry point
	IsPublic     bool `json:"is_public"`     // Whether symbol is public/exported
	IsDeprecated bool `json:"is_deprecated"` // Whether code is marked deprecated
}

// ToMap converts EmbeddingMetadata to a map for Qdrant payload.
func (m *EmbeddingMetadata) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"node_id":        m.NodeID,
		"session_id":     m.SessionID,
		"path":           m.Path,
		"language":       m.Language,
		"kind":           m.Kind,
		"symbol":         m.Symbol,
		"start_line":     m.StartLine,
		"end_line":       m.EndLine,
		"complexity":     m.Complexity,
		"line_count":     m.LineCount,
		"last_modified":  m.LastModified,
		"change_freq":    m.ChangeFreq,
		"authors":        m.Authors,
		"tier_completed": m.TierCompleted,
		"tags":           m.Tags,
		"summary":        m.Summary,
		"risk_score":     m.RiskScore,
		"has_tests":      m.HasTests,
		"has_docs":       m.HasDocs,
		"is_entry":       m.IsEntry,
		"is_public":      m.IsPublic,
		"is_deprecated":  m.IsDeprecated,
	}
}

// SearchFilter provides typed filtering options for vector search.
type SearchFilter struct {
	// Session and scope filters
	SessionID *string  `json:"session_id,omitempty"`
	Languages []string `json:"languages,omitempty"`
	Kinds     []string `json:"kinds,omitempty"`

	// Code structure filters
	MinComplexity *int `json:"min_complexity,omitempty"`
	MaxComplexity *int `json:"max_complexity,omitempty"`
	MinLineCount  *int `json:"min_line_count,omitempty"`
	MaxLineCount  *int `json:"max_line_count,omitempty"`

	// Analysis tier filters
	MinTierCompleted *int     `json:"min_tier_completed,omitempty"`
	RequiredTags     []string `json:"required_tags,omitempty"`
	ExcludedTags     []string `json:"excluded_tags,omitempty"`

	// Quality filters
	HasTests     *bool `json:"has_tests,omitempty"`
	HasDocs      *bool `json:"has_docs,omitempty"`
	IsPublic     *bool `json:"is_public,omitempty"`
	IsEntry      *bool `json:"is_entry,omitempty"`
	IsDeprecated *bool `json:"is_deprecated,omitempty"`

	// Risk and change filters
	MaxRiskScore    *float32 `json:"max_risk_score,omitempty"`
	ChangeFrequency *string  `json:"change_frequency,omitempty"` // hot, warm, cold
}

// ToQdrantFilter converts SearchFilter to Qdrant filter conditions.
func (f *SearchFilter) ToQdrantFilter() map[string]interface{} {
	filter := make(map[string]interface{})

	if f.SessionID != nil {
		filter["session_id"] = *f.SessionID
	}

	if len(f.Languages) > 0 && len(f.Languages) == 1 {
		filter["language"] = f.Languages[0]
	}

	if len(f.Kinds) > 0 && len(f.Kinds) == 1 {
		filter["kind"] = f.Kinds[0]
	}

	if f.MinTierCompleted != nil {
		filter["tier_completed"] = *f.MinTierCompleted
	}

	if f.HasTests != nil {
		filter["has_tests"] = *f.HasTests
	}

	if f.HasDocs != nil {
		filter["has_docs"] = *f.HasDocs
	}

	if f.IsPublic != nil {
		filter["is_public"] = *f.IsPublic
	}

	if f.IsEntry != nil {
		filter["is_entry"] = *f.IsEntry
	}

	if f.IsDeprecated != nil {
		filter["is_deprecated"] = *f.IsDeprecated
	}

	if f.ChangeFrequency != nil {
		filter["change_freq"] = *f.ChangeFrequency
	}

	return filter
}

// SearchOptions provides configuration for similarity search operations.
type SearchOptions struct {
	Limit           int           `json:"limit"`            // Maximum number of results
	Threshold       float32       `json:"threshold"`        // Minimum similarity score
	Filter          *SearchFilter `json:"filter"`           // Filtering options
	IncludeMetadata bool          `json:"include_metadata"` // Whether to include full metadata
}

// DefaultSearchOptions returns sensible defaults for search operations.
func DefaultSearchOptions() *SearchOptions {
	return &SearchOptions{
		Limit:           10,
		Threshold:       0.7,
		Filter:          nil,
		IncludeMetadata: true,
	}
}
