package orchestrator

import (
    "context"
    "sync"
    "time"

    "github.com/charmbracelet/crush/internal/config"
)

// QueryOrchestrator is the central coordination component for Context Engine tasks.
type QueryOrchestrator struct {
    mu     sync.Mutex
    ctx    context.Context
    cancel context.CancelFunc
    wg     sync.WaitGroup

    // Config-driven fields
    mode          config.ContextEngineMode
    maxGoroutines int
    memoryBudget  int

    // TODO: add task queues, worker pools, event bus integration
    // Task queue and workers
    taskCh chan *Task
    // used to stop workers
    workersStarted bool
}

// New creates a new QueryOrchestrator instance with default background context.
func New() *QueryOrchestrator {
    ctx, cancel := context.WithCancel(context.Background())
    return &QueryOrchestrator{ctx: ctx, cancel: cancel, taskCh: make(chan *Task, 100)}
}

// Task represents a unit of work submitted to the orchestrator.
type Task struct {
    ID      string
    Priority int // higher is executed first (not yet used)
    Execute func(ctx context.Context) error
}

// Submit enqueues a task for execution and returns immediately.
func (o *QueryOrchestrator) Submit(t *Task) error {
    if t == nil || t.Execute == nil {
        return nil
    }
    o.mu.Lock()
    defer o.mu.Unlock()
    select {
    case o.taskCh <- t:
        return nil
    default:
        // queue full, attempt non-blocking send with small fallback
        go func() { o.taskCh <- t }()
        return nil
    }
}

// startWorkerPool starts workers according to maxGoroutines (or default).
func (o *QueryOrchestrator) startWorkerPool() {
    if o.workersStarted {
        return
    }
    max := o.maxGoroutines
    if max <= 0 {
        // choose a sane default based on GOMAXPROCS
        max = 4
    }
    for i := 0; i < max; i++ {
        o.wg.Add(1)
        go func() {
            defer o.wg.Done()
            for {
                select {
                case <-o.ctx.Done():
                    return
                case t, ok := <-o.taskCh:
                    if !ok {
                        return
                    }
                    // Run task with a derived context
                    _ = t.Execute(o.ctx)
                }
            }
        }()
    }
    o.workersStarted = true
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
        // adjust heartbeat based on mode
        interval := time.Second
        switch o.mode {
        case config.ContextEngineModePerformance:
            interval = 500 * time.Millisecond
        case config.ContextEngineModeOnDemand:
            interval = 2 * time.Second
        case config.ContextEngineModeEco:
            interval = 5 * time.Second
        }
        ticker := time.NewTicker(interval)
        defer ticker.Stop()
        // start workers based on config
        o.startWorkerPool()
        for {
            select {
            case <-o.ctx.Done():
                return
            case <-ticker.C:
                // heartbeat / housekeeping; later use budgets and worker pools
            }
        }
    }()
}

// Stop signals the orchestrator to shut down and waits for workers to finish.
func (o *QueryOrchestrator) Stop() {
    o.mu.Lock()
    o.cancel()
    o.mu.Unlock()
    // close the task channel to allow workers to exit if not already canceled
    close(o.taskCh)
    o.wg.Wait()
}
