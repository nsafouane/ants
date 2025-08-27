package vector

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/charmbracelet/crush/internal/db"
)

// EmbeddingProcessor handles the complete workflow of generating and storing embeddings.
type EmbeddingProcessor struct {
	embeddingService EmbeddingGenerator
	vectorBridge     *Bridge
	queries          QueryInterface
}

// NewEmbeddingProcessor creates a new embedding processor with the given dependencies.
func NewEmbeddingProcessor(embeddingService EmbeddingGenerator, vectorBridge *Bridge, queries QueryInterface) *EmbeddingProcessor {
	return &EmbeddingProcessor{
		embeddingService: embeddingService,
		vectorBridge:     vectorBridge,
		queries:          queries,
	}
}

// ProcessCodeNode generates and stores embeddings for a code node.
func (p *EmbeddingProcessor) ProcessCodeNode(ctx context.Context, node *db.CodeNode, codeContent string) error {
	if node == nil {
		return fmt.Errorf("code node cannot be nil")
	}

	if codeContent == "" {
		return fmt.Errorf("code content cannot be empty")
	}

	// Build metadata from code node
	metadata := p.buildMetadataFromCodeNode(node)

	// Generate embedding
	embedding, err := p.embeddingService.GenerateCodeEmbedding(ctx, codeContent, metadata)
	if err != nil {
		return fmt.Errorf("failed to generate embedding for node %d: %w", node.ID, err)
	}

	// Store embedding via bridge (which handles both vector DB and SQL DB)
	if err := p.vectorBridge.SyncEmbedding(ctx, node.ID, embedding, metadata); err != nil {
		return fmt.Errorf("failed to store embedding for node %d: %w", node.ID, err)
	}

	slog.Debug("Successfully processed embedding for code node",
		"node_id", node.ID,
		"path", node.Path,
		"symbol", node.Symbol.String,
		"embedding_dims", len(embedding))

	return nil
}

// ProcessCodeNodes processes multiple code nodes in batch.
func (p *EmbeddingProcessor) ProcessCodeNodes(ctx context.Context, nodes []*db.CodeNode, codeContents []string) error {
	if len(nodes) != len(codeContents) {
		return fmt.Errorf("nodes and code contents length mismatch: %d vs %d", len(nodes), len(codeContents))
	}

	for i, node := range nodes {
		if err := p.ProcessCodeNode(ctx, node, codeContents[i]); err != nil {
			// Log error but continue processing other nodes
			slog.Error("Failed to process code node embedding",
				"node_id", node.ID,
				"path", node.Path,
				"error", err)
		}
	}

	return nil
}

// ProcessSessionNodes processes all code nodes for a given session.
func (p *EmbeddingProcessor) ProcessSessionNodes(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session ID cannot be empty")
	}

	// Get all code nodes for the session
	nodes, err := p.queries.ListCodeNodesBySession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to list code nodes for session %s: %w", sessionID, err)
	}

	if len(nodes) == 0 {
		slog.Debug("No code nodes found for session", "session_id", sessionID)
		return nil
	}

	// TODO: In a real implementation, you would fetch the actual code content
	// from files or the database. For now, this is a placeholder.
	codeContents := make([]string, len(nodes))
	for i, node := range nodes {
		// Placeholder: This should fetch actual code content
		codeContents[i] = fmt.Sprintf("// Code for node %d at %s", node.ID, node.Path)
	}

	nodePointers := make([]*db.CodeNode, len(nodes))
	for i := range nodes {
		nodePointers[i] = &nodes[i]
	}

	return p.ProcessCodeNodes(ctx, nodePointers, codeContents)
}

// ReprocessNode regenerates and updates embedding for an existing code node.
func (p *EmbeddingProcessor) ReprocessNode(ctx context.Context, nodeID int64, codeContent string) error {
	// Get the existing code node
	node, err := p.queries.GetCodeNode(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("failed to get code node %d: %w", nodeID, err)
	}

	// Delete existing embedding first
	if err := p.vectorBridge.DeleteEmbedding(ctx, nodeID); err != nil {
		slog.Warn("Failed to delete existing embedding", "node_id", nodeID, "error", err)
		// Continue anyway, as we'll overwrite it
	}

	// Process the node with new content
	return p.ProcessCodeNode(ctx, &node, codeContent)
}

// DeleteNodeEmbedding removes embedding for a code node.
func (p *EmbeddingProcessor) DeleteNodeEmbedding(ctx context.Context, nodeID int64) error {
	return p.vectorBridge.DeleteEmbedding(ctx, nodeID)
}

// DeleteSessionEmbeddings removes all embeddings for a session.
func (p *EmbeddingProcessor) DeleteSessionEmbeddings(ctx context.Context, sessionID string) error {
	return p.vectorBridge.DeleteEmbeddingsBySession(ctx, sessionID)
}

// buildMetadataFromCodeNode creates EmbeddingMetadata from a database CodeNode.
func (p *EmbeddingProcessor) buildMetadataFromCodeNode(node *db.CodeNode) *EmbeddingMetadata {
	metadata := &EmbeddingMetadata{
		NodeID:    node.ID,
		SessionID: node.SessionID,
		Path:      node.Path,
	}

	// Extract optional fields
	if node.Language.Valid {
		metadata.Language = node.Language.String
	}
	if node.Kind.Valid {
		metadata.Kind = node.Kind.String
	}
	if node.Symbol.Valid {
		metadata.Symbol = node.Symbol.String
	}
	if node.StartLine.Valid {
		metadata.StartLine = int(node.StartLine.Int64)
	}
	if node.EndLine.Valid {
		metadata.EndLine = int(node.EndLine.Int64)
		if node.StartLine.Valid {
			metadata.LineCount = metadata.EndLine - metadata.StartLine + 1
		}
	}

	// TODO: Extract additional metadata from node.Metadata JSON field
	// This could include complexity, change frequency, etc.

	return metadata
}

// GetEmbeddingStats returns statistics about embeddings for a session.
func (p *EmbeddingProcessor) GetEmbeddingStats(ctx context.Context, sessionID string) (*EmbeddingStats, error) {
	// Get total code nodes count
	totalNodes, err := p.queries.CountCodeNodesBySession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to count code nodes: %w", err)
	}

	// Get embeddings count (from vector DB via bridge)
	embeddingsCount, err := p.vectorBridge.vectorClient.CountEmbeddingsBySessionID(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to count embeddings: %w", err)
	}

	return &EmbeddingStats{
		SessionID:       sessionID,
		TotalNodes:      totalNodes,
		EmbeddingsCount: embeddingsCount,
		CoveragePercent: float64(embeddingsCount) / float64(totalNodes) * 100,
		Model:           p.embeddingService.GetModel(),
	}, nil
}

// EmbeddingStats contains statistics about embeddings for a session.
type EmbeddingStats struct {
	SessionID       string  `json:"session_id"`
	TotalNodes      int64   `json:"total_nodes"`
	EmbeddingsCount int64   `json:"embeddings_count"`
	CoveragePercent float64 `json:"coverage_percent"`
	Model           string  `json:"model"`
}

// HealthCheck verifies that all components are working correctly.
func (p *EmbeddingProcessor) HealthCheck(ctx context.Context) error {
	// Check embedding service
	if err := p.embeddingService.HealthCheck(ctx); err != nil {
		return fmt.Errorf("embedding service health check failed: %w", err)
	}

	// Check vector client
	if err := p.vectorBridge.vectorClient.Ping(ctx); err != nil {
		return fmt.Errorf("vector client health check failed: %w", err)
	}

	return nil
}
