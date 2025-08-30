// Package graph provides structural knowledge graph construction capabilities
// for the Context Engine's Tier 1 analysis pipeline.
package graph

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/charmbracelet/crush/internal/ast"
	"github.com/charmbracelet/crush/internal/complexity"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/git"
	"github.com/charmbracelet/crush/internal/linking"
	"github.com/charmbracelet/crush/internal/pubsub"
)

// BuilderConfig holds configuration for the graph builder.
type BuilderConfig struct {
	SessionID    string `json:"session_id"`
	MaxNodes     int    `json:"max_nodes"`
	BatchSize    int    `json:"batch_size"`
}

// NodeMetadata represents metadata for a code node.
type NodeMetadata struct {
	Complexity      *complexity.ComplexityMetrics `json:"complexity,omitempty"`
	GitInfo         *git.FileHotspot              `json:"git_info,omitempty"`
	LineCount       int                           `json:"line_count"`
	ContentHash     string                        `json:"content_hash"`
	LastAnalyzed    time.Time                     `json:"last_analyzed"`
}

// BuildResult represents the result of graph construction.
type BuildResult struct {
	SessionID           string        `json:"session_id"`
	NodesCreated        int           `json:"nodes_created"`
	DependenciesCreated int           `json:"dependencies_created"`
	FilesProcessed      int           `json:"files_processed"`
	BuildTime           time.Duration `json:"build_time"`
	Warnings            []string      `json:"warnings"`
}

// Builder constructs knowledge graphs from Tier 1 analysis results.
type Builder struct {
	config   *BuilderConfig
	db       db.Querier
	broker   *pubsub.Broker[map[string]interface{}]
	logger   *slog.Logger
	stats    *BuildStats
	statsLock sync.RWMutex
}

// BuildStats tracks graph building statistics.
type BuildStats struct {
	GraphsBuilt      int64         `json:"graphs_built"`
	TotalNodes       int64         `json:"total_nodes"`
	TotalBuildTime   time.Duration `json:"total_build_time"`
	LastBuildTime    time.Time     `json:"last_build_time"`
}

// New creates a new graph builder.
func New(cfg *BuilderConfig, database db.Querier, broker *pubsub.Broker[map[string]interface{}]) *Builder {
	if cfg == nil {
		cfg = &BuilderConfig{
			MaxNodes:  10000,
			BatchSize: 100,
		}
	}

	return &Builder{
		config: cfg,
		db:     database,
		broker: broker,
		logger: slog.With("component", "graph_builder"),
		stats:  &BuildStats{},
	}
}

// BuildFromAnalysis constructs the knowledge graph from Tier 1 analysis results.
func (b *Builder) BuildFromAnalysis(ctx context.Context, 
	astResults map[string]*ast.ParseResult,
	linkingResult *linking.AnalysisResult,
	complexityResults map[string]*complexity.FileComplexity,
	gitResult *git.AnalysisResult) (*BuildResult, error) {

	startTime := time.Now()
	
	b.logger.Info("Starting knowledge graph construction",
		"session_id", b.config.SessionID,
		"ast_files", len(astResults))

	result := &BuildResult{
		SessionID: b.config.SessionID,
		Warnings:  make([]string, 0),
	}

	// Process AST results to create code nodes
	nodeCount, err := b.processASTResults(ctx, astResults, complexityResults, gitResult)
	if err != nil {
		return nil, fmt.Errorf("failed to process AST results: %w", err)
	}
	result.NodesCreated = nodeCount
	result.FilesProcessed = len(astResults)

	// Process linking results to create dependencies
	depCount, err := b.processLinkingResults(ctx, linkingResult)
	if err != nil {
		return nil, fmt.Errorf("failed to process linking results: %w", err)
	}
	result.DependenciesCreated = depCount

	result.BuildTime = time.Since(startTime)
	
	// Update statistics
	b.updateStats(result)

	b.logger.Info("Knowledge graph construction completed",
		"nodes_created", result.NodesCreated,
		"dependencies_created", result.DependenciesCreated,
		"build_time", result.BuildTime)

	return result, nil
}

// processASTResults creates code nodes from AST analysis results.
func (b *Builder) processASTResults(ctx context.Context,
	astResults map[string]*ast.ParseResult,
	complexityResults map[string]*complexity.FileComplexity,
	gitResult *git.AnalysisResult) (int, error) {

	nodeCount := 0
	
	for filePath, astResult := range astResults {
		select {
		case <-ctx.Done():
			return nodeCount, ctx.Err()
		default:
		}

		// Get complexity info for this file
		var fileComplexity *complexity.FileComplexity
		if complexityResults != nil {
			fileComplexity = complexityResults[filePath]
		}

		// Create nodes for each symbol in the file
		for _, symbol := range astResult.Symbols {
			node, err := b.createCodeNode(ctx, astResult, symbol, fileComplexity)
			if err != nil {
				b.logger.Warn("Failed to create code node", 
					"file", filePath,
					"symbol", symbol.Name,
					"error", err)
				continue
			}

			if node != nil {
				nodeCount++
			}
		}
	}

	return nodeCount, nil
}

// createCodeNode creates a single code node in the database.
func (b *Builder) createCodeNode(ctx context.Context,
	astResult *ast.ParseResult,
	symbol *ast.Symbol,
	fileComplexity *complexity.FileComplexity) (*db.CodeNode, error) {

	// Find matching function complexity if available
	var funcComplexity *complexity.FunctionComplexity
	if fileComplexity != nil {
		for _, fn := range fileComplexity.Functions {
			if fn.Name == symbol.Name {
				funcComplexity = fn
				break
			}
		}
	}

	// Build metadata
	metadata := &NodeMetadata{
		LineCount:    int(symbol.EndPoint.Row - symbol.StartPoint.Row + 1),
		ContentHash:  astResult.ContentHash,
		LastAnalyzed: time.Now(),
	}

	if funcComplexity != nil {
		metadata.Complexity = &funcComplexity.Metrics
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Create the code node
	node, err := b.db.CreateCodeNode(ctx, db.CreateCodeNodeParams{
		SessionID: b.config.SessionID,
		Path:      astResult.FilePath,
		Symbol:    sql.NullString{String: symbol.Name, Valid: true},
		Kind:      sql.NullString{String: symbol.Kind, Valid: true},
		StartLine: sql.NullInt64{Int64: int64(symbol.StartPoint.Row), Valid: true},
		EndLine:   sql.NullInt64{Int64: int64(symbol.EndPoint.Row), Valid: true},
		Language:  sql.NullString{String: astResult.Language, Valid: true},
		Metadata:  string(metadataJSON),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create code node: %w", err)
	}

	return &node, nil
}

// processLinkingResults creates dependencies from linking analysis results.
func (b *Builder) processLinkingResults(ctx context.Context, linkingResult *linking.AnalysisResult) (int, error) {
	if linkingResult == nil {
		return 0, nil
	}

	depCount := 0

	for _, ref := range linkingResult.References {
		select {
		case <-ctx.Done():
			return depCount, ctx.Err()
		default:
		}

		// Find the source and target nodes
		sourceNode, err := b.findNodeBySymbol(ctx, ref.FromFile, ref.FromSymbol)
		if err != nil {
			continue
		}

		targetNode, err := b.findNodeBySymbol(ctx, ref.ToFile, ref.ToSymbol)
		if err != nil {
			continue
		}

		// Create dependency
		_, err = b.db.CreateDependency(ctx, db.CreateDependencyParams{
			FromNode: sourceNode.ID,
			ToNode:   targetNode.ID,
			Relation: sql.NullString{String: ref.ReferenceType, Valid: true},
			Metadata: "{}",
		})

		if err != nil {
			continue
		}

		depCount++
	}

	return depCount, nil
}

// findNodeBySymbol finds a code node by file path and symbol name.
func (b *Builder) findNodeBySymbol(ctx context.Context, filePath, symbolName string) (*db.CodeNode, error) {
	nodes, err := b.db.ListCodeNodesByPath(ctx, filePath)
	if err != nil {
		return nil, err
	}

	for _, node := range nodes {
		if node.Symbol.Valid && node.Symbol.String == symbolName {
			return &node, nil
		}
	}

	return nil, fmt.Errorf("node not found: %s in %s", symbolName, filePath)
}

// updateStats updates the builder's statistics.
func (b *Builder) updateStats(result *BuildResult) {
	b.statsLock.Lock()
	defer b.statsLock.Unlock()

	b.stats.GraphsBuilt++
	b.stats.TotalNodes += int64(result.NodesCreated)
	b.stats.TotalBuildTime += result.BuildTime
	b.stats.LastBuildTime = time.Now()
}

// GetStats returns the current builder statistics.
func (b *Builder) GetStats() BuildStats {
	b.statsLock.RLock()
	defer b.statsLock.RUnlock()
	return *b.stats
}