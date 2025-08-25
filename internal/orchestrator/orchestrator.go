package orchestrator

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/pubsub"
)

// QueryOrchestrator is the central coordination component for Context Engine tasks.
type QueryOrchestrator struct {
	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Config-driven fields
	mode          config.ContextEngineMode
	maxGoroutines int
	memoryBudget  int

	// Enhanced task management
	taskQueue       []*Task // Priority queue
	taskCh          chan *Task
	workersStarted  bool
	resourceMonitor *ResourceMonitor

	// Session management
	sessions sync.Map // map[string]*SessionContextEngine
	db       db.Querier
	eventBus *pubsub.Broker[OrchestratorEvent]

	// Performance metrics
	metrics *PerformanceMetrics
}

// New creates a new QueryOrchestrator instance with default background context.
func New() *QueryOrchestrator {
	ctx, cancel := context.WithCancel(context.Background())

	o := &QueryOrchestrator{
		ctx:             ctx,
		cancel:          cancel,
		taskCh:          make(chan *Task, 100),
		taskQueue:       make([]*Task, 0),
		resourceMonitor: &ResourceMonitor{},
		eventBus:        pubsub.NewBroker[OrchestratorEvent](),
		metrics:         &PerformanceMetrics{},
	}

	// Initialize resource monitoring
	o.updateResourceMonitoring()

	return o
}

// SetDatabase configures the database querier for session management
func (o *QueryOrchestrator) SetDatabase(db db.Querier) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.db = db
}

// Task represents a unit of work submitted to the orchestrator.
type Task struct {
	ID        string
	Priority  int // higher is executed first
	SessionID string
	FilePaths []string
	Tier      AnalysisTier
	Execute   func(ctx context.Context) error
	CreatedAt time.Time
}

// AnalysisTier represents the analysis tier level
type AnalysisTier int

const (
	Tier1 AnalysisTier = iota // Structural analysis
	Tier2                     // AI analysis
	Tier3                     // Deep reasoning
)

// OrchestratorEvent represents events from the orchestrator
type OrchestratorEvent struct {
	Type      string
	SessionID string
	TaskID    string
	Metadata  map[string]interface{}
	Timestamp time.Time
}

// ResourceMonitor tracks system resource usage
type ResourceMonitor struct {
	mu               sync.RWMutex
	memoryUsageMB    int64
	activeGoroutines int32
	lastUpdate       time.Time
}

// PerformanceMetrics tracks orchestrator performance
type PerformanceMetrics struct {
	mu                sync.RWMutex
	TasksSubmitted    int64
	TasksCompleted    int64
	TasksFailed       int64
	AverageLatencyMs  float64
	QueueLength       int
	ActiveSessions    int
	LastMetricsUpdate time.Time
}

// Submit enqueues a task for execution with priority-based scheduling.
func (o *QueryOrchestrator) Submit(t *Task) error {
	if t == nil || t.Execute == nil {
		return nil
	}

	// Set creation time
	t.CreatedAt = time.Now()

	o.mu.Lock()
	defer o.mu.Unlock()

	// Update metrics
	o.metrics.TasksSubmitted++

	// Check resource constraints
	if !o.canAcceptTask(t) {
		return ErrResourceConstraints
	}

	// Add to priority queue
	o.taskQueue = append(o.taskQueue, t)
	o.sortTaskQueue()

	// Try to send immediately to workers
	select {
	case o.taskCh <- t:
		// Remove from queue since it's being processed
		o.removeFromQueue(t.ID)
		return nil
	default:
		// Worker pool is busy, task stays in queue
		o.metrics.QueueLength = len(o.taskQueue)

		// Publish queued event
		o.eventBus.Publish(pubsub.EventType("task_queued"), OrchestratorEvent{
			Type:      "task_queued",
			SessionID: t.SessionID,
			TaskID:    t.ID,
			Metadata: map[string]interface{}{
				"priority": t.Priority,
				"tier":     t.Tier,
			},
			Timestamp: time.Now(),
		})

		return nil
	}
}

// ErrResourceConstraints is returned when resource limits prevent task submission
var ErrResourceConstraints = fmt.Errorf("resource constraints prevent task submission")

// canAcceptTask checks if the orchestrator can accept a new task based on resource constraints
func (o *QueryOrchestrator) canAcceptTask(_ *Task) bool {
	// Check memory budget
	if o.memoryBudget > 0 {
		o.resourceMonitor.mu.RLock()
		currentUsage := o.resourceMonitor.memoryUsageMB
		o.resourceMonitor.mu.RUnlock()

		if currentUsage > int64(o.memoryBudget) {
			return false
		}
	}

	// Check queue length based on operational mode
	maxQueueLength := o.getMaxQueueLength()
	return len(o.taskQueue) < maxQueueLength
}

// getMaxQueueLength returns the maximum queue length based on operational mode
func (o *QueryOrchestrator) getMaxQueueLength() int {
	switch o.mode {
	case config.ContextEngineModePerformance:
		return 1000
	case config.ContextEngineModeOnDemand:
		return 500
	case config.ContextEngineModeEco:
		return 100
	default:
		return 500
	}
}

// sortTaskQueue sorts the task queue by priority (highest first) and creation time
func (o *QueryOrchestrator) sortTaskQueue() {
	sort.Slice(o.taskQueue, func(i, j int) bool {
		// Higher priority first
		if o.taskQueue[i].Priority != o.taskQueue[j].Priority {
			return o.taskQueue[i].Priority > o.taskQueue[j].Priority
		}
		// If same priority, earlier creation time first
		return o.taskQueue[i].CreatedAt.Before(o.taskQueue[j].CreatedAt)
	})
}

// removeFromQueue removes a task from the queue by ID
func (o *QueryOrchestrator) removeFromQueue(taskID string) {
	for i, task := range o.taskQueue {
		if task.ID == taskID {
			// Remove task from slice
			o.taskQueue = append(o.taskQueue[:i], o.taskQueue[i+1:]...)
			break
		}
	}
	o.metrics.QueueLength = len(o.taskQueue)
}

// startWorkerPool starts workers according to maxGoroutines and operational mode.
func (o *QueryOrchestrator) startWorkerPool() {
	if o.workersStarted {
		return
	}

	max := o.maxGoroutines
	if max <= 0 {
		// Choose workers based on mode and available CPUs
		cpus := runtime.NumCPU()
		switch o.mode {
		case config.ContextEngineModePerformance:
			max = cpus * 2 // Aggressive
		case config.ContextEngineModeOnDemand:
			max = cpus // Conservative
		case config.ContextEngineModeEco:
			max = maxInt(1, cpus/2) // Very conservative
		default:
			max = cpus
		}
	}

	for i := 0; i < max; i++ {
		o.wg.Add(1)
		go func(workerID int) {
			defer o.wg.Done()

			for {
				select {
				case <-o.ctx.Done():
					return

				case t, ok := <-o.taskCh:
					if !ok {
						return
					}

					// Track task execution
					startTime := time.Now()

					// Execute task with context
					err := t.Execute(o.ctx)

					// Update metrics
					duration := time.Since(startTime)
					o.updateTaskMetrics(t, err, duration)

					// Publish completion event
					eventType := "task_completed"
					if err != nil {
						eventType = "task_failed"
					}

					o.eventBus.Publish(pubsub.EventType(eventType), OrchestratorEvent{
						Type:      eventType,
						SessionID: t.SessionID,
						TaskID:    t.ID,
						Metadata: map[string]interface{}{
							"duration_ms": duration.Milliseconds(),
							"worker_id":   workerID,
							"error":       err,
						},
						Timestamp: time.Now(),
					})

					// Try to get next task from queue
					o.tryScheduleNextTask()
				}
			}
		}(i)
	}

	o.workersStarted = true
}

// maxInt returns the maximum of two integers
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// updateTaskMetrics updates performance metrics after task completion
func (o *QueryOrchestrator) updateTaskMetrics(_ *Task, err error, duration time.Duration) {
	o.metrics.mu.Lock()
	defer o.metrics.mu.Unlock()

	if err != nil {
		o.metrics.TasksFailed++
	} else {
		o.metrics.TasksCompleted++
	}

	// Update average latency (simple moving average)
	totalTasks := o.metrics.TasksCompleted + o.metrics.TasksFailed
	if totalTasks > 0 {
		o.metrics.AverageLatencyMs = (o.metrics.AverageLatencyMs*float64(totalTasks-1) + float64(duration.Milliseconds())) / float64(totalTasks)
	}

	o.metrics.LastMetricsUpdate = time.Now()
}

// tryScheduleNextTask attempts to schedule the next high-priority task from the queue
func (o *QueryOrchestrator) tryScheduleNextTask() {
	o.mu.Lock()
	defer o.mu.Unlock()

	if len(o.taskQueue) == 0 {
		return
	}

	// Take the highest priority task
	nextTask := o.taskQueue[0]

	// Try to send to worker channel (non-blocking)
	select {
	case o.taskCh <- nextTask:
		// Successfully scheduled, remove from queue
		o.taskQueue = o.taskQueue[1:]
		o.metrics.QueueLength = len(o.taskQueue)
	default:
		// Worker pool is still busy, leave task in queue
	}
}

// ApplyConfig updates orchestrator runtime settings from ContextEngineConfig.
func (o *QueryOrchestrator) ApplyConfig(cfg *config.ContextEngineConfig) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if cfg == nil {
		return
	}
	o.mode = cfg.Mode
	o.maxGoroutines = cfg.MaxGoroutines
	o.memoryBudget = cfg.MemoryBudgetMB
}

// Start begins background processing for the orchestrator.
func (o *QueryOrchestrator) Start() {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.wg.Add(1)
	go func() {
		defer o.wg.Done()

		// Adjust heartbeat based on mode
		interval := o.getHeartbeatInterval()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Start workers based on config
		o.startWorkerPool()

		// Start resource monitor
		o.wg.Add(1)
		go o.runResourceMonitor()

		for {
			select {
			case <-o.ctx.Done():
				return
			case <-ticker.C:
				o.performHousekeeping()
			}
		}
	}()
}

// getHeartbeatInterval returns the appropriate heartbeat interval based on operational mode
func (o *QueryOrchestrator) getHeartbeatInterval() time.Duration {
	switch o.mode {
	case config.ContextEngineModePerformance:
		return 500 * time.Millisecond
	case config.ContextEngineModeOnDemand:
		return 2 * time.Second
	case config.ContextEngineModeEco:
		return 5 * time.Second
	default:
		return time.Second
	}
}

// performHousekeeping runs periodic maintenance tasks
func (o *QueryOrchestrator) performHousekeeping() {
	// Update metrics
	o.updateMetrics()

	// Clean up stale sessions
	o.cleanupStaleSessions()

	// Update resource monitoring
	o.updateResourceMonitoring()

	// Process queued tasks if workers are available
	o.processQueuedTasks()
}

// runResourceMonitor continuously monitors system resources
func (o *QueryOrchestrator) runResourceMonitor() {
	defer o.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-o.ctx.Done():
			return
		case <-ticker.C:
			o.updateResourceMonitoring()
		}
	}
}

// updateMetrics updates orchestrator performance metrics
func (o *QueryOrchestrator) updateMetrics() {
	o.metrics.mu.Lock()
	defer o.metrics.mu.Unlock()

	// Count active sessions
	activeCount := 0
	o.sessions.Range(func(key, value interface{}) bool {
		activeCount++
		return true
	})

	o.metrics.ActiveSessions = activeCount
	o.metrics.QueueLength = len(o.taskQueue)
	o.metrics.LastMetricsUpdate = time.Now()
}

// cleanupStaleSessions removes inactive session engines
func (o *QueryOrchestrator) cleanupStaleSessions() {
	cutoff := time.Now().Add(-1 * time.Hour) // Sessions inactive for 1 hour

	var staleSessions []string
	o.sessions.Range(func(key, value interface{}) bool {
		sessionID := key.(string)
		sessionEngine := value.(*SessionContextEngine)

		// Check if session is stale (simple heuristic - could be improved)
		stats, err := sessionEngine.GetKnowledgeStats(o.ctx)
		if err != nil || stats.LastUpdated.Before(cutoff) {
			staleSessions = append(staleSessions, sessionID)
		}

		return true
	})

	// Clean up stale sessions
	for _, sessionID := range staleSessions {
		if sessionEngine, exists := o.sessions.LoadAndDelete(sessionID); exists {
			sessionEngine.(*SessionContextEngine).Stop()
		}
	}
}

// updateResourceMonitoring updates resource usage statistics
func (o *QueryOrchestrator) updateResourceMonitoring() {
	o.resourceMonitor.mu.Lock()
	defer o.resourceMonitor.mu.Unlock()

	// Update memory usage (simplified - in production would use actual memory stats)
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	o.resourceMonitor.memoryUsageMB = int64(m.Alloc / 1024 / 1024)

	// Update goroutine count
	o.resourceMonitor.activeGoroutines = int32(runtime.NumGoroutine())

	o.resourceMonitor.lastUpdate = time.Now()

	// Check if we're hitting resource limits
	if o.memoryBudget > 0 && o.resourceMonitor.memoryUsageMB > int64(o.memoryBudget) {
		// Publish resource warning event
		o.eventBus.Publish(pubsub.EventType("resource_warning"), OrchestratorEvent{
			Type: "resource_warning",
			Metadata: map[string]interface{}{
				"memory_usage_mb": o.resourceMonitor.memoryUsageMB,
				"memory_budget":   o.memoryBudget,
			},
			Timestamp: time.Now(),
		})
	}
}

// processQueuedTasks tries to process tasks from the queue
func (o *QueryOrchestrator) processQueuedTasks() {
	o.mu.Lock()
	defer o.mu.Unlock()

	// Try to schedule waiting tasks
	for len(o.taskQueue) > 0 {
		nextTask := o.taskQueue[0]

		select {
		case o.taskCh <- nextTask:
			// Successfully scheduled
			o.taskQueue = o.taskQueue[1:]
			o.metrics.QueueLength = len(o.taskQueue)
		default:
			// Worker pool is busy, stop trying
			break
		}
	}
}

// Stop signals the orchestrator to shut down and waits for workers to finish.
func (o *QueryOrchestrator) Stop() {
	o.mu.Lock()
	o.cancel()
	o.mu.Unlock()

	// Stop all session engines
	o.sessions.Range(func(key, value interface{}) bool {
		sessionEngine := value.(*SessionContextEngine)
		sessionEngine.Stop()
		return true
	})

	// Close the task channel to allow workers to exit
	close(o.taskCh)
	o.wg.Wait()
}

// GetOrCreateSessionEngine returns the session-scoped Context Engine for a session
func (o *QueryOrchestrator) GetOrCreateSessionEngine(sessionID string) *SessionContextEngine {
	// Try to get existing session engine
	if engine, exists := o.sessions.Load(sessionID); exists {
		return engine.(*SessionContextEngine)
	}

	// Create new session engine
	config := o.getSessionConfig()
	sessionEngine := NewSessionContextEngine(sessionID, o.db, config)

	// Store and start the session engine
	o.sessions.Store(sessionID, sessionEngine)
	sessionEngine.Start()

	return sessionEngine
}

// getSessionConfig returns the Context Engine config for session engines
func (o *QueryOrchestrator) getSessionConfig() *config.ContextEngineConfig {
	return &config.ContextEngineConfig{
		Mode:           o.mode,
		MaxGoroutines:  o.maxGoroutines,
		MemoryBudgetMB: o.memoryBudget,
	}
}

// GetMetrics returns current performance metrics
func (o *QueryOrchestrator) GetMetrics() PerformanceMetrics {
	o.metrics.mu.RLock()
	defer o.metrics.mu.RUnlock()

	return PerformanceMetrics{
		TasksSubmitted:    o.metrics.TasksSubmitted,
		TasksCompleted:    o.metrics.TasksCompleted,
		TasksFailed:       o.metrics.TasksFailed,
		AverageLatencyMs:  o.metrics.AverageLatencyMs,
		QueueLength:       o.metrics.QueueLength,
		ActiveSessions:    o.metrics.ActiveSessions,
		LastMetricsUpdate: o.metrics.LastMetricsUpdate,
	}
}

// GetResourceStatus returns current resource usage
func (o *QueryOrchestrator) GetResourceStatus() ResourceStatus {
	o.resourceMonitor.mu.RLock()
	defer o.resourceMonitor.mu.RUnlock()

	return ResourceStatus{
		MemoryUsageMB:    o.resourceMonitor.memoryUsageMB,
		MemoryBudgetMB:   int64(o.memoryBudget),
		ActiveGoroutines: o.resourceMonitor.activeGoroutines,
		LastUpdate:       o.resourceMonitor.lastUpdate,
		Mode:             o.mode,
	}
}

// Subscribe returns a channel for orchestrator events
func (o *QueryOrchestrator) Subscribe(ctx context.Context) <-chan pubsub.Event[OrchestratorEvent] {
	return o.eventBus.Subscribe(ctx)
}

// ResourceStatus holds current resource usage information
type ResourceStatus struct {
	MemoryUsageMB    int64
	MemoryBudgetMB   int64
	ActiveGoroutines int32
	LastUpdate       time.Time
	Mode             config.ContextEngineMode
}
