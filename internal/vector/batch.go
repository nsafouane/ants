package vector

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/charmbracelet/crush/internal/db"
)

// BatchProcessor handles large-scale embedding processing with session management.
type BatchProcessor struct {
	embeddingService EmbeddingGenerator
	vectorBridge     *Bridge
	queries          QueryInterface

	// Configuration
	maxConcurrency   int              // Maximum concurrent embedding generations
	batchSize        int              // Number of embeddings to process per batch
	retryAttempts    int              // Number of retry attempts for failed embeddings
	retryDelay       time.Duration    // Delay between retries
	progressCallback ProgressCallback // Optional progress reporting

	// Session management
	sessionMutex   sync.RWMutex
	activeSessions map[string]*SessionBatch // Currently processing sessions
}

// SessionBatch represents a batch processing session with its state.
type SessionBatch struct {
	SessionID      string       `json:"session_id"`
	TotalNodes     int          `json:"total_nodes"`
	ProcessedNodes int          `json:"processed_nodes"`
	FailedNodes    int          `json:"failed_nodes"`
	StartTime      time.Time    `json:"start_time"`
	LastUpdate     time.Time    `json:"last_update"`
	Status         BatchStatus  `json:"status"`
	ErrorLog       []BatchError `json:"error_log"`

	// Cancellation support
	cancel context.CancelFunc
	ctx    context.Context
}

// BatchStatus represents the status of a batch processing operation.
type BatchStatus string

const (
	BatchStatusPending   BatchStatus = "pending"
	BatchStatusRunning   BatchStatus = "running"
	BatchStatusPaused    BatchStatus = "paused"
	BatchStatusCompleted BatchStatus = "completed"
	BatchStatusFailed    BatchStatus = "failed"
	BatchStatusCancelled BatchStatus = "cancelled"
)

// BatchError represents an error that occurred during batch processing.
type BatchError struct {
	NodeID    int64     `json:"node_id"`
	Error     string    `json:"error"`
	Timestamp time.Time `json:"timestamp"`
	Attempt   int       `json:"attempt"`
}

// ProgressCallback is called to report batch processing progress.
type ProgressCallback func(sessionID string, progress *BatchProgress)

// BatchProgress contains detailed progress information for a batch operation.
type BatchProgress struct {
	SessionID          string        `json:"session_id"`
	TotalNodes         int           `json:"total_nodes"`
	ProcessedNodes     int           `json:"processed_nodes"`
	FailedNodes        int           `json:"failed_nodes"`
	SuccessRate        float64       `json:"success_rate"`
	ElapsedTime        time.Duration `json:"elapsed_time"`
	EstimatedRemaining time.Duration `json:"estimated_remaining"`
	NodesPerSecond     float64       `json:"nodes_per_second"`
	Status             BatchStatus   `json:"status"`
}

// BatchProcessingOptions configures batch processing behavior.
type BatchProcessingOptions struct {
	MaxConcurrency    int              `json:"max_concurrency"`    // Maximum concurrent operations
	BatchSize         int              `json:"batch_size"`         // Nodes per batch
	RetryAttempts     int              `json:"retry_attempts"`     // Retry failed nodes
	RetryDelay        time.Duration    `json:"retry_delay"`        // Delay between retries
	ProgressCallback  ProgressCallback `json:"-"`                  // Progress reporting
	SkipExisting      bool             `json:"skip_existing"`      // Skip nodes with existing embeddings
	OverwriteExisting bool             `json:"overwrite_existing"` // Overwrite existing embeddings
	ContentProvider   ContentProvider  `json:"-"`                  // Provides code content for nodes
}

// ContentProvider retrieves code content for processing.
type ContentProvider interface {
	GetCodeContent(ctx context.Context, node *db.CodeNode) (string, error)
	GetCodeContents(ctx context.Context, nodes []*db.CodeNode) ([]string, error)
}

// DefaultBatchProcessingOptions returns optimized defaults for batch processing.
func DefaultBatchProcessingOptions() *BatchProcessingOptions {
	return &BatchProcessingOptions{
		MaxConcurrency:    8,  // 8 concurrent embedding generations
		BatchSize:         50, // 50 nodes per batch
		RetryAttempts:     3,  // 3 retry attempts
		RetryDelay:        2 * time.Second,
		SkipExisting:      true,  // Skip existing by default
		OverwriteExisting: false, // Don't overwrite by default
	}
}

// NewBatchProcessor creates a new batch processor with the given configuration.
func NewBatchProcessor(
	embeddingService EmbeddingGenerator,
	vectorBridge *Bridge,
	queries QueryInterface,
	options *BatchProcessingOptions,
) *BatchProcessor {
	if options == nil {
		options = DefaultBatchProcessingOptions()
	}

	return &BatchProcessor{
		embeddingService: embeddingService,
		vectorBridge:     vectorBridge,
		queries:          queries,
		maxConcurrency:   options.MaxConcurrency,
		batchSize:        options.BatchSize,
		retryAttempts:    options.RetryAttempts,
		retryDelay:       options.RetryDelay,
		progressCallback: options.ProgressCallback,
		activeSessions:   make(map[string]*SessionBatch),
	}
}

// ProcessSessionBatch processes all code nodes for a session in optimized batches.
func (bp *BatchProcessor) ProcessSessionBatch(ctx context.Context, sessionID string, options *BatchProcessingOptions) (*BatchProgress, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session ID cannot be empty")
	}

	// Check if session is already being processed
	bp.sessionMutex.RLock()
	if existingBatch, exists := bp.activeSessions[sessionID]; exists {
		bp.sessionMutex.RUnlock()
		return bp.getBatchProgress(existingBatch), fmt.Errorf("session %s is already being processed", sessionID)
	}
	bp.sessionMutex.RUnlock()

	// Get all code nodes for the session
	nodes, err := bp.queries.ListCodeNodesBySession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to list code nodes for session %s: %w", sessionID, err)
	}

	if len(nodes) == 0 {
		slog.Info("No code nodes found for session", "session_id", sessionID)
		return &BatchProgress{
			SessionID:      sessionID,
			TotalNodes:     0,
			ProcessedNodes: 0,
			SuccessRate:    100.0,
			Status:         BatchStatusCompleted,
		}, nil
	}

	// Create session batch context with cancellation
	sessionCtx, cancel := context.WithCancel(ctx)

	// Initialize session batch
	sessionBatch := &SessionBatch{
		SessionID:      sessionID,
		TotalNodes:     len(nodes),
		ProcessedNodes: 0,
		FailedNodes:    0,
		StartTime:      time.Now(),
		LastUpdate:     time.Now(),
		Status:         BatchStatusRunning,
		ErrorLog:       make([]BatchError, 0),
		cancel:         cancel,
		ctx:            sessionCtx,
	}

	// Register active session
	bp.sessionMutex.Lock()
	bp.activeSessions[sessionID] = sessionBatch
	bp.sessionMutex.Unlock()

	// Ensure cleanup on completion
	defer func() {
		bp.sessionMutex.Lock()
		delete(bp.activeSessions, sessionID)
		bp.sessionMutex.Unlock()
		cancel()
	}()

	slog.Info("Starting batch processing for session",
		"session_id", sessionID,
		"total_nodes", len(nodes),
		"batch_size", bp.batchSize,
		"max_concurrency", bp.maxConcurrency)

	// Filter nodes if needed
	nodesToProcess := nodes
	if options != nil && options.SkipExisting {
		nodesToProcess, err = bp.filterExistingEmbeddings(sessionCtx, nodes)
		if err != nil {
			sessionBatch.Status = BatchStatusFailed
			return bp.getBatchProgress(sessionBatch), fmt.Errorf("failed to filter existing embeddings: %w", err)
		}

		slog.Info("Filtered nodes for processing",
			"session_id", sessionID,
			"original_count", len(nodes),
			"filtered_count", len(nodesToProcess))
	}

	// Process nodes in batches with concurrency control
	err = bp.processBatchesWithConcurrency(sessionCtx, sessionID, nodesToProcess, sessionBatch, options)

	// Update final status
	if err != nil {
		sessionBatch.Status = BatchStatusFailed
		return bp.getBatchProgress(sessionBatch), err
	}

	sessionBatch.Status = BatchStatusCompleted
	sessionBatch.LastUpdate = time.Now()

	finalProgress := bp.getBatchProgress(sessionBatch)

	slog.Info("Completed batch processing for session",
		"session_id", sessionID,
		"processed_nodes", finalProgress.ProcessedNodes,
		"failed_nodes", finalProgress.FailedNodes,
		"success_rate", finalProgress.SuccessRate,
		"elapsed_time", finalProgress.ElapsedTime)

	return finalProgress, nil
}

// processBatchesWithConcurrency processes nodes in batches with controlled concurrency.
func (bp *BatchProcessor) processBatchesWithConcurrency(
	ctx context.Context,
	sessionID string,
	nodes []db.CodeNode,
	sessionBatch *SessionBatch,
	options *BatchProcessingOptions,
) error {
	// Create semaphore for concurrency control
	sem := make(chan struct{}, bp.maxConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex // Protects shared state

	// Process nodes in batches
	for i := 0; i < len(nodes); i += bp.batchSize {
		select {
		case <-ctx.Done():
			sessionBatch.Status = BatchStatusCancelled
			return ctx.Err()
		default:
		}

		end := i + bp.batchSize
		if end > len(nodes) {
			end = len(nodes)
		}

		batch := nodes[i:end]

		wg.Add(1)
		go func(batchNodes []db.CodeNode) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			// Process this batch
			bp.processBatch(ctx, sessionID, batchNodes, sessionBatch, &mu, options)
		}(batch)
	}

	// Wait for all batches to complete
	wg.Wait()

	return nil
}

// processBatch processes a single batch of nodes.
func (bp *BatchProcessor) processBatch(
	ctx context.Context,
	sessionID string,
	nodes []db.CodeNode,
	sessionBatch *SessionBatch,
	mu *sync.Mutex,
	options *BatchProcessingOptions,
) {
	for _, node := range nodes {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Process single node with retries
		err := bp.processNodeWithRetries(ctx, &node, options)

		// Update session state
		mu.Lock()
		if err != nil {
			sessionBatch.FailedNodes++
			sessionBatch.ErrorLog = append(sessionBatch.ErrorLog, BatchError{
				NodeID:    node.ID,
				Error:     err.Error(),
				Timestamp: time.Now(),
				Attempt:   bp.retryAttempts + 1, // Final attempt
			})
			slog.Warn("Failed to process node after retries",
				"session_id", sessionID,
				"node_id", node.ID,
				"error", err)
		} else {
			sessionBatch.ProcessedNodes++
		}
		sessionBatch.LastUpdate = time.Now()

		// Report progress if callback is set
		if bp.progressCallback != nil {
			progress := bp.getBatchProgress(sessionBatch)
			bp.progressCallback(sessionID, progress)
		}
		mu.Unlock()
	}
}

// processNodeWithRetries processes a single node with retry logic.
func (bp *BatchProcessor) processNodeWithRetries(ctx context.Context, node *db.CodeNode, options *BatchProcessingOptions) error {
	var lastErr error

	for attempt := 0; attempt <= bp.retryAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Add delay between retries (except first attempt)
		if attempt > 0 {
			time.Sleep(bp.retryDelay)
		}

		// Get code content
		codeContent, err := bp.getCodeContent(ctx, node, options)
		if err != nil {
			lastErr = fmt.Errorf("failed to get code content: %w", err)
			continue
		}

		// Build metadata
		metadata := bp.buildMetadataFromCodeNode(node)

		// Generate embedding
		embedding, err := bp.embeddingService.GenerateCodeEmbedding(ctx, codeContent, metadata)
		if err != nil {
			lastErr = fmt.Errorf("failed to generate embedding: %w", err)
			continue
		}

		// Store embedding
		err = bp.vectorBridge.SyncEmbedding(ctx, node.ID, embedding, metadata)
		if err != nil {
			lastErr = fmt.Errorf("failed to store embedding: %w", err)
			continue
		}

		// Success
		return nil
	}

	return fmt.Errorf("failed after %d attempts: %w", bp.retryAttempts+1, lastErr)
}

// getCodeContent retrieves code content for a node using the content provider.
func (bp *BatchProcessor) getCodeContent(ctx context.Context, node *db.CodeNode, options *BatchProcessingOptions) (string, error) {
	if options != nil && options.ContentProvider != nil {
		return options.ContentProvider.GetCodeContent(ctx, node)
	}

	// Fallback: create placeholder content
	// TODO: Implement actual file content retrieval
	return fmt.Sprintf("// Code for node %d at %s\n// Language: %s\n// Kind: %s\n",
		node.ID,
		node.Path,
		node.Language.String,
		node.Kind.String), nil
}

// filterExistingEmbeddings removes nodes that already have embeddings.
func (bp *BatchProcessor) filterExistingEmbeddings(ctx context.Context, nodes []db.CodeNode) ([]db.CodeNode, error) {
	filtered := make([]db.CodeNode, 0, len(nodes))

	for _, node := range nodes {
		_, err := bp.queries.GetEmbeddingByNode(ctx, node.ID)
		if err != nil {
			if err == sql.ErrNoRows {
				// No embedding exists, include this node
				filtered = append(filtered, node)
			} else {
				// Database error
				return nil, fmt.Errorf("failed to check embedding for node %d: %w", node.ID, err)
			}
		}
		// Embedding exists, skip this node
	}

	return filtered, nil
}

// getBatchProgress calculates current progress for a session batch.
func (bp *BatchProcessor) getBatchProgress(sessionBatch *SessionBatch) *BatchProgress {
	elapsed := time.Since(sessionBatch.StartTime)

	var successRate float64
	processedTotal := sessionBatch.ProcessedNodes + sessionBatch.FailedNodes
	if processedTotal > 0 {
		successRate = float64(sessionBatch.ProcessedNodes) / float64(processedTotal) * 100
	}

	var nodesPerSecond float64
	if elapsed.Seconds() > 0 {
		nodesPerSecond = float64(processedTotal) / elapsed.Seconds()
	}

	var estimatedRemaining time.Duration
	if nodesPerSecond > 0 {
		remainingNodes := sessionBatch.TotalNodes - processedTotal
		estimatedRemaining = time.Duration(float64(remainingNodes)/nodesPerSecond) * time.Second
	}

	return &BatchProgress{
		SessionID:          sessionBatch.SessionID,
		TotalNodes:         sessionBatch.TotalNodes,
		ProcessedNodes:     sessionBatch.ProcessedNodes,
		FailedNodes:        sessionBatch.FailedNodes,
		SuccessRate:        successRate,
		ElapsedTime:        elapsed,
		EstimatedRemaining: estimatedRemaining,
		NodesPerSecond:     nodesPerSecond,
		Status:             sessionBatch.Status,
	}
}

// buildMetadataFromCodeNode creates EmbeddingMetadata from a database CodeNode.
func (bp *BatchProcessor) buildMetadataFromCodeNode(node *db.CodeNode) *EmbeddingMetadata {
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

	return metadata
}

// GetSessionProgress returns current progress for a session.
func (bp *BatchProcessor) GetSessionProgress(sessionID string) (*BatchProgress, error) {
	bp.sessionMutex.RLock()
	defer bp.sessionMutex.RUnlock()

	sessionBatch, exists := bp.activeSessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("no active batch found for session %s", sessionID)
	}

	return bp.getBatchProgress(sessionBatch), nil
}

// CancelSession cancels batch processing for a session.
func (bp *BatchProcessor) CancelSession(sessionID string) error {
	bp.sessionMutex.RLock()
	sessionBatch, exists := bp.activeSessions[sessionID]
	bp.sessionMutex.RUnlock()

	if !exists {
		return fmt.Errorf("no active batch found for session %s", sessionID)
	}

	sessionBatch.cancel()
	sessionBatch.Status = BatchStatusCancelled

	slog.Info("Cancelled batch processing for session", "session_id", sessionID)
	return nil
}

// PauseSession pauses batch processing for a session.
func (bp *BatchProcessor) PauseSession(sessionID string) error {
	bp.sessionMutex.Lock()
	defer bp.sessionMutex.Unlock()

	sessionBatch, exists := bp.activeSessions[sessionID]
	if !exists {
		return fmt.Errorf("no active batch found for session %s", sessionID)
	}

	if sessionBatch.Status != BatchStatusRunning {
		return fmt.Errorf("session %s is not running (status: %s)", sessionID, sessionBatch.Status)
	}

	sessionBatch.Status = BatchStatusPaused
	slog.Info("Paused batch processing for session", "session_id", sessionID)
	return nil
}

// ResumeSession resumes batch processing for a paused session.
func (bp *BatchProcessor) ResumeSession(sessionID string) error {
	bp.sessionMutex.Lock()
	defer bp.sessionMutex.Unlock()

	sessionBatch, exists := bp.activeSessions[sessionID]
	if !exists {
		return fmt.Errorf("no active batch found for session %s", sessionID)
	}

	if sessionBatch.Status != BatchStatusPaused {
		return fmt.Errorf("session %s is not paused (status: %s)", sessionID, sessionBatch.Status)
	}

	sessionBatch.Status = BatchStatusRunning
	slog.Info("Resumed batch processing for session", "session_id", sessionID)
	return nil
}

// ListActiveSessions returns a list of currently active batch processing sessions.
func (bp *BatchProcessor) ListActiveSessions() map[string]*BatchProgress {
	bp.sessionMutex.RLock()
	defer bp.sessionMutex.RUnlock()

	result := make(map[string]*BatchProgress)
	for sessionID, sessionBatch := range bp.activeSessions {
		result[sessionID] = bp.getBatchProgress(sessionBatch)
	}

	return result
}

// HealthCheck verifies that the batch processor is functioning correctly.
func (bp *BatchProcessor) HealthCheck(ctx context.Context) error {
	// Check embedding service
	if err := bp.embeddingService.HealthCheck(ctx); err != nil {
		return fmt.Errorf("embedding service health check failed: %w", err)
	}

	// Check vector bridge
	if err := bp.vectorBridge.vectorClient.Ping(ctx); err != nil {
		return fmt.Errorf("vector client health check failed: %w", err)
	}

	return nil
}
