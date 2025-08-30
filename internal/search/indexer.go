package search

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/charmbracelet/crush/internal/vector"
)

// IncrementalIndexer handles real-time updates to search indexes based on code changes.
type IncrementalIndexer struct {
	// Services
	ftsService      *FTSService
	vectorProcessor *vector.EmbeddingProcessor
	queries         *db.Queries
	codeNodeBroker  *pubsub.Broker[pubsub.CodeNodeEvent]
	fileBroker      *pubsub.Broker[pubsub.FileSystemEvent]

	// Configuration
	config *IndexerConfig

	// State management
	indexQueue     chan *IndexTask
	batchProcessor *BatchIndexProcessor
	running        bool
	mu             sync.RWMutex

	// Statistics
	stats *IndexerStats
}

// IndexerConfig configures the incremental indexer behavior.
type IndexerConfig struct {
	// Processing configuration
	BatchSize    int           `json:"batch_size"`    // Number of items to process in batch
	BatchTimeout time.Duration `json:"batch_timeout"` // Maximum time to wait for batch
	WorkerCount  int           `json:"worker_count"`  // Number of concurrent workers
	QueueSize    int           `json:"queue_size"`    // Index task queue size

	// Index update strategy
	UpdateFTS    bool `json:"update_fts"`    // Enable FTS5 index updates
	UpdateVector bool `json:"update_vector"` // Enable vector index updates
	UpdateAsync  bool `json:"update_async"`  // Process updates asynchronously

	// Performance tuning
	DebounceInterval time.Duration `json:"debounce_interval"` // Debounce rapid changes
	MaxRetries       int           `json:"max_retries"`       // Maximum retry attempts
	RetryDelay       time.Duration `json:"retry_delay"`       // Delay between retries
}

// IndexTask represents a single indexing operation.
type IndexTask struct {
	// Task identification
	TaskID    string    `json:"task_id"`    // Unique task identifier
	NodeID    int64     `json:"node_id"`    // Code node ID
	SessionID string    `json:"session_id"` // Session identifier
	Operation string    `json:"operation"`  // "create", "update", "delete"
	Priority  int       `json:"priority"`   // Task priority (0=low, 10=high)
	CreatedAt time.Time `json:"created_at"` // Task creation time

	// Content data
	Path     string  `json:"path"`     // File path
	Language *string `json:"language"` // Programming language
	Kind     *string `json:"kind"`     // Code element type
	Symbol   *string `json:"symbol"`   // Symbol name
	Content  string  `json:"content"`  // Code content

	// Retry tracking
	Attempts  int   `json:"attempts"`   // Number of attempts
	LastError error `json:"last_error"` // Last error encountered
}

// BatchIndexProcessor handles batch processing of index tasks.
type BatchIndexProcessor struct {
	indexer *IncrementalIndexer
	batch   []*IndexTask
	timer   *time.Timer
	mu      sync.Mutex
}

// IndexerStats tracks indexing performance and statistics.
type IndexerStats struct {
	// Counters
	TasksProcessed   int64 `json:"tasks_processed"`   // Total tasks processed
	TasksSucceeded   int64 `json:"tasks_succeeded"`   // Successfully processed tasks
	TasksFailed      int64 `json:"tasks_failed"`      // Failed tasks
	BatchesProcessed int64 `json:"batches_processed"` // Total batches processed

	// Performance metrics
	AverageLatency  time.Duration `json:"average_latency"`   // Average processing time
	QueueDepth      int           `json:"queue_depth"`       // Current queue depth
	LastProcessedAt *time.Time    `json:"last_processed_at"` // Last processing time

	// Error tracking
	LastError error   `json:"last_error"` // Most recent error
	ErrorRate float64 `json:"error_rate"` // Error rate percentage

	mu sync.RWMutex
}

// DefaultIndexerConfig returns sensible defaults for incremental indexing.
func DefaultIndexerConfig() *IndexerConfig {
	return &IndexerConfig{
		BatchSize:        10,
		BatchTimeout:     5 * time.Second,
		WorkerCount:      2,
		QueueSize:        1000,
		UpdateFTS:        true,
		UpdateVector:     true,
		UpdateAsync:      true,
		DebounceInterval: 500 * time.Millisecond,
		MaxRetries:       3,
		RetryDelay:       1 * time.Second,
	}
}

// NewIncrementalIndexer creates a new incremental indexing service.
func NewIncrementalIndexer(
	ftsService *FTSService,
	vectorProcessor *vector.EmbeddingProcessor,
	queries *db.Queries,
	codeNodeBroker *pubsub.Broker[pubsub.CodeNodeEvent],
	fileBroker *pubsub.Broker[pubsub.FileSystemEvent],
	config *IndexerConfig,
) *IncrementalIndexer {
	if config == nil {
		config = DefaultIndexerConfig()
	}

	indexer := &IncrementalIndexer{
		ftsService:      ftsService,
		vectorProcessor: vectorProcessor,
		queries:         queries,
		codeNodeBroker:  codeNodeBroker,
		fileBroker:      fileBroker,
		config:          config,
		indexQueue:      make(chan *IndexTask, config.QueueSize),
		stats:           &IndexerStats{},
	}

	indexer.batchProcessor = &BatchIndexProcessor{
		indexer: indexer,
		batch:   make([]*IndexTask, 0, config.BatchSize),
	}

	return indexer
}

// Start begins the incremental indexing service.
func (ii *IncrementalIndexer) Start(ctx context.Context) error {
	ii.mu.Lock()
	defer ii.mu.Unlock()

	if ii.running {
		return fmt.Errorf("incremental indexer is already running")
	}

	slog.Info("Starting incremental indexer",
		"batch_size", ii.config.BatchSize,
		"worker_count", ii.config.WorkerCount,
		"queue_size", ii.config.QueueSize)

	// Subscribe to code change events
	if err := ii.subscribeToEvents(ctx); err != nil {
		return fmt.Errorf("failed to subscribe to events: %w", err)
	}

	// Start worker goroutines
	for i := 0; i < ii.config.WorkerCount; i++ {
		go ii.worker(ctx, i)
	}

	// Start batch processor
	go ii.batchProcessor.start(ctx)

	ii.running = true
	slog.Info("Incremental indexer started successfully")

	return nil
}

// Stop gracefully shuts down the incremental indexer.
func (ii *IncrementalIndexer) Stop(ctx context.Context) error {
	ii.mu.Lock()
	defer ii.mu.Unlock()

	if !ii.running {
		return nil
	}

	slog.Info("Stopping incremental indexer")

	// Close the index queue to signal workers to stop
	close(ii.indexQueue)

	// Wait for workers to finish (simplified - in production would use sync.WaitGroup)
	time.Sleep(2 * time.Second)

	ii.running = false
	slog.Info("Incremental indexer stopped")

	return nil
}

// subscribeToEvents registers event handlers for code changes.
func (ii *IncrementalIndexer) subscribeToEvents(ctx context.Context) error {
	// Subscribe to code node events
	codeNodeEvents := ii.codeNodeBroker.Subscribe(ctx)
	go func() {
		for event := range codeNodeEvents {
			switch event.Type {
			case pubsub.CodeNodeCreatedEvent:
				ii.handleCodeNodeCreated(ctx, &event)
			case pubsub.CodeNodeUpdatedEvent:
				ii.handleCodeNodeUpdated(ctx, &event)
			case pubsub.CodeNodeDeletedEvent:
				ii.handleCodeNodeDeleted(ctx, &event)
			}
		}
	}()

	// Subscribe to file events
	fileEvents := ii.fileBroker.Subscribe(ctx)
	go func() {
		for event := range fileEvents {
			switch event.Type {
			case pubsub.FileDeletedEvent:
				ii.handleFileDeleted(ctx, &event)
			case pubsub.FileChangedEvent:
				ii.handleFileUpdated(ctx, &event)
			}
		}
	}()

	return nil
}

// Event handlers

func (ii *IncrementalIndexer) handleCodeNodeCreated(ctx context.Context, event *pubsub.Event[pubsub.CodeNodeEvent]) error {
	return ii.processCodeNodeEvent(ctx, event, "create")
}

func (ii *IncrementalIndexer) handleCodeNodeUpdated(ctx context.Context, event *pubsub.Event[pubsub.CodeNodeEvent]) error {
	return ii.processCodeNodeEvent(ctx, event, "update")
}

func (ii *IncrementalIndexer) handleCodeNodeDeleted(ctx context.Context, event *pubsub.Event[pubsub.CodeNodeEvent]) error {
	return ii.processCodeNodeEvent(ctx, event, "delete")
}

func (ii *IncrementalIndexer) handleFileCreated(ctx context.Context, event *pubsub.Event[pubsub.FileSystemEvent]) error {
	return ii.processFileEvent(ctx, event, "create")
}

func (ii *IncrementalIndexer) handleFileUpdated(ctx context.Context, event *pubsub.Event[pubsub.FileSystemEvent]) error {
	return ii.processFileEvent(ctx, event, "update")
}

func (ii *IncrementalIndexer) handleFileDeleted(ctx context.Context, event *pubsub.Event[pubsub.FileSystemEvent]) error {
	return ii.processFileEvent(ctx, event, "delete")
}

// processCodeNodeEvent handles code node change events.
func (ii *IncrementalIndexer) processCodeNodeEvent(ctx context.Context, event *pubsub.Event[pubsub.CodeNodeEvent], operation string) error {
	// Extract node information from event payload
	payload := event.Payload

	// Create index task
	task := &IndexTask{
		TaskID:    fmt.Sprintf("node_%d_%s_%d", payload.NodeID, operation, time.Now().UnixNano()),
		NodeID:    payload.NodeID,
		SessionID: payload.SessionID,
		Operation: operation,
		Priority:  5, // Medium priority
		CreatedAt: time.Now(),
		Path:      payload.Path,
	}

	// Set additional data from payload
	if payload.Language != "" {
		task.Language = &payload.Language
	}
	if payload.Kind != "" {
		task.Kind = &payload.Kind
	}
	if payload.Symbol != "" {
		task.Symbol = &payload.Symbol
	}

	// Extract content from metadata if available
	if payload.Metadata != nil {
		if content, ok := payload.Metadata["content"].(string); ok {
			task.Content = content
		}
	}

	// Queue the task
	return ii.queueTask(task)
}

// processFileEvent handles file change events.
func (ii *IncrementalIndexer) processFileEvent(ctx context.Context, event *pubsub.Event[pubsub.FileSystemEvent], operation string) error {
	// Extract file information from event payload
	payload := event.Payload

	// Create file-level index task
	task := &IndexTask{
		TaskID:    fmt.Sprintf("file_%s_%s_%d", payload.Path, operation, time.Now().UnixNano()),
		SessionID: payload.SessionID,
		Path:      payload.Path,
		Operation: operation,
		Priority:  3, // Lower priority for file-level changes
		CreatedAt: time.Now(),
	}

	// Extract additional data from metadata if available
	if payload.Metadata != nil {
		if content, ok := payload.Metadata["content"].(string); ok {
			task.Content = content
		}
		if language, ok := payload.Metadata["language"].(string); ok {
			task.Language = &language
		}
	}

	return ii.queueTask(task)
}

// queueTask adds a task to the processing queue.
func (ii *IncrementalIndexer) queueTask(task *IndexTask) error {
	ii.mu.RLock()
	running := ii.running
	ii.mu.RUnlock()

	if !running {
		return fmt.Errorf("indexer is not running")
	}

	select {
	case ii.indexQueue <- task:
		ii.stats.mu.Lock()
		ii.stats.QueueDepth++
		ii.stats.mu.Unlock()

		slog.Debug("Task queued for indexing",
			"task_id", task.TaskID,
			"operation", task.Operation,
			"node_id", task.NodeID,
			"path", task.Path)
		return nil

	default:
		return fmt.Errorf("index queue is full")
	}
}

// worker processes index tasks from the queue.
func (ii *IncrementalIndexer) worker(ctx context.Context, workerID int) {
	slog.Debug("Starting index worker", "worker_id", workerID)

	for {
		select {
		case task, ok := <-ii.indexQueue:
			if !ok {
				slog.Debug("Index worker stopping", "worker_id", workerID)
				return
			}

			ii.stats.mu.Lock()
			ii.stats.QueueDepth--
			ii.stats.mu.Unlock()

			// Process the task
			if ii.config.UpdateAsync {
				// Add to batch processor
				ii.batchProcessor.addTask(task)
			} else {
				// Process immediately
				ii.processTask(ctx, task)
			}

		case <-ctx.Done():
			slog.Debug("Index worker context cancelled", "worker_id", workerID)
			return
		}
	}
}

// processTask handles a single index task.
func (ii *IncrementalIndexer) processTask(ctx context.Context, task *IndexTask) {
	startTime := time.Now()

	slog.Debug("Processing index task",
		"task_id", task.TaskID,
		"operation", task.Operation,
		"node_id", task.NodeID)

	var err error
	switch task.Operation {
	case "create", "update":
		err = ii.processCreateOrUpdate(ctx, task)
	case "delete":
		err = ii.processDelete(ctx, task)
	default:
		err = fmt.Errorf("unknown operation: %s", task.Operation)
	}

	// Update statistics
	duration := time.Since(startTime)
	ii.updateStats(task, err, duration)

	if err != nil {
		slog.Warn("Index task failed",
			"task_id", task.TaskID,
			"error", err,
			"attempts", task.Attempts)

		// Retry if configured
		if task.Attempts < ii.config.MaxRetries {
			task.Attempts++
			task.LastError = err

			// Requeue after delay
			go func() {
				time.Sleep(ii.config.RetryDelay)
				ii.queueTask(task)
			}()
		}
	} else {
		slog.Debug("Index task completed successfully",
			"task_id", task.TaskID,
			"duration", duration)
	}
}

// processCreateOrUpdate handles create and update operations.
func (ii *IncrementalIndexer) processCreateOrUpdate(ctx context.Context, task *IndexTask) error {
	// Fetch fresh data if needed
	if task.Content == "" && task.NodeID > 0 {
		// Would fetch content from database
		// For now, using placeholder
		slog.Debug("Would fetch content for node", "node_id", task.NodeID)
	}

	var errors []error

	// Update FTS5 index
	if ii.config.UpdateFTS && task.Content != "" {
		if task.NodeID > 0 {
			err := ii.ftsService.IndexCodeContent(
				ctx,
				task.NodeID,
				task.SessionID,
				task.Path,
				getStringValue(task.Language),
				getStringValue(task.Kind),
				getStringValue(task.Symbol),
				task.Content,
				DefaultIndexingOptions(),
			)
			if err != nil {
				errors = append(errors, fmt.Errorf("FTS indexing failed: %w", err))
			}
		} else {
			// File-level indexing
			err := ii.ftsService.IndexFileContent(
				ctx,
				task.SessionID,
				task.Path,
				getStringValue(task.Language),
				task.Content,
				int64(len(task.Content)),
			)
			if err != nil {
				errors = append(errors, fmt.Errorf("FTS file indexing failed: %w", err))
			}
		}
	}

	// Update vector index
	if ii.config.UpdateVector && task.NodeID > 0 && task.Content != "" {
		// Create code node for vector processing
		codeNode := &db.CodeNode{
			ID:        task.NodeID,
			SessionID: task.SessionID,
			Path:      task.Path,
		}

		if task.Language != nil {
			codeNode.Language.String = *task.Language
			codeNode.Language.Valid = true
		}

		if task.Kind != nil {
			codeNode.Kind.String = *task.Kind
			codeNode.Kind.Valid = true
		}

		if task.Symbol != nil {
			codeNode.Symbol.String = *task.Symbol
			codeNode.Symbol.Valid = true
		}

		err := ii.vectorProcessor.ProcessCodeNode(ctx, codeNode, task.Content)
		if err != nil {
			errors = append(errors, fmt.Errorf("vector indexing failed: %w", err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("indexing errors: %v", errors)
	}

	return nil
}

// processDelete handles delete operations.
func (ii *IncrementalIndexer) processDelete(ctx context.Context, task *IndexTask) error {
	var errors []error

	// Remove from FTS5 index
	if ii.config.UpdateFTS {
		if task.NodeID > 0 {
			err := ii.ftsService.RemoveFromIndex(ctx, task.NodeID)
			if err != nil {
				errors = append(errors, fmt.Errorf("FTS removal failed: %w", err))
			}
		} else {
			// File-level removal
			// Would implement file-level removal from FTS
			slog.Debug("Would remove file from FTS index", "path", task.Path)
		}
	}

	// Remove from vector index (would be implemented with vector bridge)
	if ii.config.UpdateVector && task.NodeID > 0 {
		// Would remove from vector database
		slog.Debug("Would remove from vector index", "node_id", task.NodeID)
	}

	if len(errors) > 0 {
		return fmt.Errorf("deletion errors: %v", errors)
	}

	return nil
}

// Batch processor implementation

func (bp *BatchIndexProcessor) start(ctx context.Context) {
	bp.timer = time.NewTimer(bp.indexer.config.BatchTimeout)
	defer bp.timer.Stop()

	for {
		select {
		case <-bp.timer.C:
			bp.processBatch(ctx)
			bp.timer.Reset(bp.indexer.config.BatchTimeout)

		case <-ctx.Done():
			// Process remaining batch on shutdown
			bp.processBatch(ctx)
			return
		}
	}
}

func (bp *BatchIndexProcessor) addTask(task *IndexTask) {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	bp.batch = append(bp.batch, task)

	if len(bp.batch) >= bp.indexer.config.BatchSize {
		// Reset timer and process batch
		if !bp.timer.Stop() {
			<-bp.timer.C
		}
		bp.timer.Reset(bp.indexer.config.BatchTimeout)

		go bp.processBatch(context.Background())
	}
}

func (bp *BatchIndexProcessor) processBatch(ctx context.Context) {
	bp.mu.Lock()
	if len(bp.batch) == 0 {
		bp.mu.Unlock()
		return
	}

	// Take current batch
	batch := make([]*IndexTask, len(bp.batch))
	copy(batch, bp.batch)
	bp.batch = bp.batch[:0] // Clear batch
	bp.mu.Unlock()

	slog.Debug("Processing index batch", "size", len(batch))

	// Process each task in the batch
	for _, task := range batch {
		bp.indexer.processTask(ctx, task)
	}

	// Update batch statistics
	bp.indexer.stats.mu.Lock()
	bp.indexer.stats.BatchesProcessed++
	bp.indexer.stats.mu.Unlock()
}

// updateStats updates indexer performance statistics.
func (ii *IncrementalIndexer) updateStats(task *IndexTask, err error, duration time.Duration) {
	ii.stats.mu.Lock()
	defer ii.stats.mu.Unlock()

	ii.stats.TasksProcessed++
	if err == nil {
		ii.stats.TasksSucceeded++
	} else {
		ii.stats.TasksFailed++
		ii.stats.LastError = err
	}

	// Update average latency (simplified calculation)
	if ii.stats.TasksProcessed == 1 {
		ii.stats.AverageLatency = duration
	} else {
		// Exponential moving average
		alpha := 0.1
		ii.stats.AverageLatency = time.Duration(float64(ii.stats.AverageLatency)*(1-alpha) + float64(duration)*alpha)
	}

	// Update error rate
	if ii.stats.TasksProcessed > 0 {
		ii.stats.ErrorRate = float64(ii.stats.TasksFailed) / float64(ii.stats.TasksProcessed) * 100
	}

	now := time.Now()
	ii.stats.LastProcessedAt = &now
}

// GetStats returns current indexer statistics.
func (ii *IncrementalIndexer) GetStats() IndexerStats {
	ii.stats.mu.RLock()
	defer ii.stats.mu.RUnlock()

	// Return a copy without the mutex to avoid copying lock value
	return IndexerStats{
		TasksProcessed:   ii.stats.TasksProcessed,
		TasksSucceeded:   ii.stats.TasksSucceeded,
		TasksFailed:      ii.stats.TasksFailed,
		BatchesProcessed: ii.stats.BatchesProcessed,
		AverageLatency:   ii.stats.AverageLatency,
		QueueDepth:       ii.stats.QueueDepth,
		LastProcessedAt:  ii.stats.LastProcessedAt,
		LastError:        ii.stats.LastError,
		ErrorRate:        ii.stats.ErrorRate,
		// Note: mu field is intentionally omitted to avoid copying the mutex
	}
}

// ManualReindex triggers a manual reindex of specific content.
func (ii *IncrementalIndexer) ManualReindex(ctx context.Context, nodeID int64, sessionID string, priority int) error {
	task := &IndexTask{
		TaskID:    fmt.Sprintf("manual_%d_%d", nodeID, time.Now().UnixNano()),
		NodeID:    nodeID,
		SessionID: sessionID,
		Operation: "update",
		Priority:  priority,
		CreatedAt: time.Now(),
	}

	return ii.queueTask(task)
}

// HealthCheck verifies indexer health and performance.
func (ii *IncrementalIndexer) HealthCheck(ctx context.Context) error {
	ii.stats.mu.RLock()
	defer ii.stats.mu.RUnlock()

	// Check if indexer is running
	ii.mu.RLock()
	running := ii.running
	ii.mu.RUnlock()

	if !running {
		return fmt.Errorf("indexer is not running")
	}

	// Check error rate
	if ii.stats.ErrorRate > 50 { // More than 50% errors
		return fmt.Errorf("high error rate: %.1f%%", ii.stats.ErrorRate)
	}

	// Check queue depth
	if ii.stats.QueueDepth > ii.config.QueueSize/2 {
		return fmt.Errorf("queue depth too high: %d", ii.stats.QueueDepth)
	}

	return nil
}

// Utility functions

func getStringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
