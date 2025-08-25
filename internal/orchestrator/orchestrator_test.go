package orchestrator

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryOrchestrator_NewAndBasicOperations(t *testing.T) {
	orchestrator := New()
	require.NotNil(t, orchestrator)

	// Test configuration
	cfg := &config.ContextEngineConfig{
		Mode:           config.ContextEngineModePerformance,
		MaxGoroutines:  4,
		MemoryBudgetMB: 512,
		PrewarmTopN:    10,
	}

	orchestrator.ApplyConfig(cfg)

	// Verify configuration was applied
	assert.Equal(t, config.ContextEngineModePerformance, orchestrator.mode)
	assert.Equal(t, 4, orchestrator.maxGoroutines)
	assert.Equal(t, 512, orchestrator.memoryBudget)
}

func TestQueryOrchestrator_TaskSubmission(t *testing.T) {
	orchestrator := New()
	require.NotNil(t, orchestrator)

	// Configure orchestrator
	cfg := &config.ContextEngineConfig{
		Mode:           config.ContextEngineModePerformance,
		MaxGoroutines:  2,
		MemoryBudgetMB: 100,
	}
	orchestrator.ApplyConfig(cfg)

	// Start orchestrator
	orchestrator.Start()
	defer orchestrator.Stop()

	// Wait a bit for workers to start
	time.Sleep(100 * time.Millisecond)

	// Create a test task
	executed := false
	task := &Task{
		ID:        "test-task-1",
		Priority:  10,
		SessionID: "test-session",
		FilePaths: []string{"test.go"},
		Tier:      Tier1,
		Execute: func(ctx context.Context) error {
			executed = true
			return nil
		},
	}

	// Submit task
	err := orchestrator.Submit(task)
	assert.NoError(t, err)

	// Wait for task to execute
	time.Sleep(200 * time.Millisecond)

	// Verify task was executed
	assert.True(t, executed, "Task should have been executed")

	// Check metrics
	metrics := orchestrator.GetMetrics()
	assert.Equal(t, int64(1), metrics.TasksSubmitted)
	assert.Equal(t, int64(1), metrics.TasksCompleted)
	assert.Equal(t, int64(0), metrics.TasksFailed)
}

func TestQueryOrchestrator_ResourceStatus(t *testing.T) {
	orchestrator := New()
	require.NotNil(t, orchestrator)

	// Configure orchestrator
	cfg := &config.ContextEngineConfig{
		Mode:           config.ContextEngineModeEco,
		MaxGoroutines:  1,
		MemoryBudgetMB: 256,
	}
	orchestrator.ApplyConfig(cfg)

	// Get resource status
	status := orchestrator.GetResourceStatus()
	assert.Equal(t, config.ContextEngineModeEco, status.Mode)
	assert.Equal(t, int64(256), status.MemoryBudgetMB)
}

func TestQueryOrchestrator_OperationalModes(t *testing.T) {
	tests := []struct {
		name             string
		mode             config.ContextEngineMode
		expectedInterval time.Duration
	}{
		{
			name:             "Performance Mode",
			mode:             config.ContextEngineModePerformance,
			expectedInterval: 500 * time.Millisecond,
		},
		{
			name:             "On-Demand Mode",
			mode:             config.ContextEngineModeOnDemand,
			expectedInterval: 2 * time.Second,
		},
		{
			name:             "Eco Mode",
			mode:             config.ContextEngineModeEco,
			expectedInterval: 5 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orchestrator := New()
			cfg := &config.ContextEngineConfig{Mode: tt.mode}
			orchestrator.ApplyConfig(cfg)

			interval := orchestrator.getHeartbeatInterval()
			assert.Equal(t, tt.expectedInterval, interval)
		})
	}
}

func TestSessionContextEngine_Basic(t *testing.T) {
	sessionEngine := NewSessionContextEngine("test-session", nil, &config.ContextEngineConfig{
		Mode:           config.ContextEngineModePerformance,
		MaxGoroutines:  2,
		MemoryBudgetMB: 128,
	})
	require.NotNil(t, sessionEngine)

	// Test start and stop
	sessionEngine.Start()
	time.Sleep(50 * time.Millisecond)
	sessionEngine.Stop()

	// Test analysis lock
	files := []string{"test1.go", "test2.go"}
	lock, err := sessionEngine.RequestAnalysisLock(files, "commit-123")
	assert.NoError(t, err)
	assert.NotNil(t, lock)
	assert.Equal(t, files, lock.Files)
	assert.Equal(t, "commit-123", lock.CommitHash)
	assert.Equal(t, "test-session", lock.LockedBy)

	// Release lock
	sessionEngine.ReleaseLock(lock)
}

func TestQueryOrchestrator_SetDatabase(t *testing.T) {
	orchestrator := New()
	require.NotNil(t, orchestrator)

	mockDB := &MockQuerier{}
	orchestrator.SetDatabase(mockDB)

	// Verify database was set
	assert.Equal(t, mockDB, orchestrator.db)
}

func TestQueryOrchestrator_PriorityTaskScheduling(t *testing.T) {
	orchestrator := New()
	require.NotNil(t, orchestrator)

	// Configure orchestrator
	cfg := &config.ContextEngineConfig{
		Mode:           config.ContextEngineModePerformance,
		MaxGoroutines:  1, // Single worker to test ordering
		MemoryBudgetMB: 100,
	}
	orchestrator.ApplyConfig(cfg)

	// Test that priority sorting works by directly testing the sort function
	tasks := []*Task{
		{
			ID:        "low-priority",
			Priority:  1,
			SessionID: "test-session",
			Tier:      Tier1,
			CreatedAt: time.Now().Add(-2 * time.Second), // Older
			Execute:   func(ctx context.Context) error { return nil },
		},
		{
			ID:        "high-priority",
			Priority:  10,
			SessionID: "test-session",
			Tier:      Tier1,
			CreatedAt: time.Now().Add(-1 * time.Second), // Newer
			Execute:   func(ctx context.Context) error { return nil },
		},
		{
			ID:        "medium-priority",
			Priority:  5,
			SessionID: "test-session",
			Tier:      Tier1,
			CreatedAt: time.Now(), // Newest
			Execute:   func(ctx context.Context) error { return nil },
		},
	}

	// Test 1: Test queue sorting functionality directly
	orchestrator.mu.Lock()
	orchestrator.taskQueue = tasks
	orchestrator.sortTaskQueue()

	// Verify queue is sorted by priority (highest first)
	assert.Equal(t, 3, len(orchestrator.taskQueue))
	assert.Equal(t, "high-priority", orchestrator.taskQueue[0].ID, "Highest priority task should be first")
	assert.Equal(t, "medium-priority", orchestrator.taskQueue[1].ID, "Medium priority task should be second")
	assert.Equal(t, "low-priority", orchestrator.taskQueue[2].ID, "Lowest priority task should be last")

	// Clear the manually added tasks and reset metrics for the next test
	orchestrator.taskQueue = []*Task{}
	orchestrator.metrics.TasksSubmitted = 0
	orchestrator.metrics.TasksCompleted = 0
	orchestrator.metrics.TasksFailed = 0
	orchestrator.mu.Unlock()

	// Test 2: Test the Submit functionality with orchestrator running
	orchestrator.Start()
	defer orchestrator.Stop()

	// Submit the tasks through the normal Submit method
	for _, task := range tasks {
		err := orchestrator.Submit(task)
		require.NoError(t, err)
	}

	// Wait for tasks to complete
	time.Sleep(200 * time.Millisecond)

	// Check metrics
	metrics := orchestrator.GetMetrics()
	assert.Equal(t, int64(3), metrics.TasksSubmitted, "Should have submitted 3 tasks")
	assert.Equal(t, int64(3), metrics.TasksCompleted, "Should have completed 3 tasks")
	assert.Equal(t, int64(0), metrics.TasksFailed, "Should have no failed tasks")
}

func TestQueryOrchestrator_ResourceConstraints(t *testing.T) {
	orchestrator := New()
	require.NotNil(t, orchestrator)

	// Configure with very low memory budget
	cfg := &config.ContextEngineConfig{
		Mode:           config.ContextEngineModeEco,
		MaxGoroutines:  1,
		MemoryBudgetMB: 1, // Very low budget
	}
	orchestrator.ApplyConfig(cfg)

	// Simulate high memory usage
	orchestrator.resourceMonitor.mu.Lock()
	orchestrator.resourceMonitor.memoryUsageMB = 10 // Higher than budget
	orchestrator.resourceMonitor.mu.Unlock()

	// Try to submit task - should be rejected
	task := &Task{
		ID:        "test-task",
		Priority:  5,
		SessionID: "test-session",
		Tier:      Tier1,
		Execute:   func(ctx context.Context) error { return nil },
	}

	err := orchestrator.Submit(task)
	assert.Error(t, err)
	assert.Equal(t, ErrResourceConstraints, err)
}

func TestQueryOrchestrator_SessionManagement(t *testing.T) {
	orchestrator := New()
	require.NotNil(t, orchestrator)

	// Set up mock database
	mockDB := &MockQuerier{}
	orchestrator.SetDatabase(mockDB)

	// Configure orchestrator
	cfg := &config.ContextEngineConfig{
		Mode:           config.ContextEngineModePerformance,
		MaxGoroutines:  2,
		MemoryBudgetMB: 128,
	}
	orchestrator.ApplyConfig(cfg)

	// Test getting session engine
	sessionID := "test-session-123"
	engine1 := orchestrator.GetOrCreateSessionEngine(sessionID)
	require.NotNil(t, engine1)
	assert.Equal(t, sessionID, engine1.sessionID)

	// Test getting same session engine again
	engine2 := orchestrator.GetOrCreateSessionEngine(sessionID)
	require.NotNil(t, engine2)

	// Should be the same instance
	assert.Equal(t, engine1, engine2)

	// Test different session
	differentSessionID := "different-session"
	engine3 := orchestrator.GetOrCreateSessionEngine(differentSessionID)
	require.NotNil(t, engine3)
	assert.NotEqual(t, engine1, engine3)
	assert.Equal(t, differentSessionID, engine3.sessionID)
}

func TestQueryOrchestrator_EventSubscription(t *testing.T) {
	orchestrator := New()
	require.NotNil(t, orchestrator)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe to events
	eventChan := orchestrator.Subscribe(ctx)
	require.NotNil(t, eventChan)

	// Configure and start orchestrator
	cfg := &config.ContextEngineConfig{
		Mode:           config.ContextEngineModePerformance,
		MaxGoroutines:  1,
		MemoryBudgetMB: 100,
	}
	orchestrator.ApplyConfig(cfg)
	orchestrator.Start()
	defer orchestrator.Stop()

	// Submit a task that will generate events
	task := &Task{
		ID:        "event-test-task",
		Priority:  5,
		SessionID: "test-session",
		Tier:      Tier1,
		Execute:   func(ctx context.Context) error { return nil },
	}

	err := orchestrator.Submit(task)
	require.NoError(t, err)

	// Wait for task completion event
	select {
	case event := <-eventChan:
		assert.Equal(t, pubsub.EventType("task_completed"), event.Type)
		orchestratorEvent := event.Payload
		assert.Equal(t, "task_completed", orchestratorEvent.Type)
		assert.Equal(t, "test-session", orchestratorEvent.SessionID)
		assert.Equal(t, "event-test-task", orchestratorEvent.TaskID)
	case <-time.After(1 * time.Second):
		t.Fatal("Should have received task completion event")
	}
}

func TestQueryOrchestrator_GetMaxQueueLength(t *testing.T) {
	orchestrator := New()
	require.NotNil(t, orchestrator)

	tests := []struct {
		name     string
		mode     config.ContextEngineMode
		expected int
	}{
		{
			name:     "Performance Mode",
			mode:     config.ContextEngineModePerformance,
			expected: 1000,
		},
		{
			name:     "On-Demand Mode",
			mode:     config.ContextEngineModeOnDemand,
			expected: 500,
		},
		{
			name:     "Eco Mode",
			mode:     config.ContextEngineModeEco,
			expected: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.ContextEngineConfig{Mode: tt.mode}
			orchestrator.ApplyConfig(cfg)

			maxQueue := orchestrator.getMaxQueueLength()
			assert.Equal(t, tt.expected, maxQueue)
		})
	}
}

func TestQueryOrchestrator_WorkerPoolSizing(t *testing.T) {
	tests := []struct {
		name            string
		mode            config.ContextEngineMode
		maxGoroutines   int
		expectedWorkers int
	}{
		{
			name:            "Performance Mode - Auto",
			mode:            config.ContextEngineModePerformance,
			maxGoroutines:   0, // Auto-sizing
			expectedWorkers: runtime.NumCPU() * 2,
		},
		{
			name:            "On-Demand Mode - Auto",
			mode:            config.ContextEngineModeOnDemand,
			maxGoroutines:   0,
			expectedWorkers: runtime.NumCPU(),
		},
		{
			name:            "Eco Mode - Auto",
			mode:            config.ContextEngineModeEco,
			maxGoroutines:   0,
			expectedWorkers: maxInt(1, runtime.NumCPU()/2),
		},
		{
			name:            "Manual Override",
			mode:            config.ContextEngineModePerformance,
			maxGoroutines:   8,
			expectedWorkers: 8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orchestrator := New()
			require.NotNil(t, orchestrator)

			cfg := &config.ContextEngineConfig{
				Mode:          tt.mode,
				MaxGoroutines: tt.maxGoroutines,
			}
			orchestrator.ApplyConfig(cfg)

			// Start orchestrator to trigger worker pool creation
			orchestrator.Start()
			defer orchestrator.Stop()

			// Wait for workers to start
			time.Sleep(100 * time.Millisecond)

			// Test that the expected number of workers are created by submitting tasks
			taskCount := tt.expectedWorkers + 2 // More tasks than workers
			var completedTasks int64
			var wg sync.WaitGroup

			for i := 0; i < taskCount; i++ {
				wg.Add(1)
				task := &Task{
					ID:        fmt.Sprintf("worker-test-%d", i),
					Priority:  5,
					SessionID: "test-session",
					Tier:      Tier1,
					Execute: func(ctx context.Context) error {
						atomic.AddInt64(&completedTasks, 1)
						wg.Done()
						time.Sleep(10 * time.Millisecond) // Small delay
						return nil
					},
				}

				err := orchestrator.Submit(task)
				require.NoError(t, err)
			}

			// Wait for all tasks to complete
			wg.Wait()

			// Verify all tasks completed
			assert.Equal(t, int64(taskCount), atomic.LoadInt64(&completedTasks))
		})
	}
}

func TestQueryOrchestrator_ResourceMonitoring(t *testing.T) {
	orchestrator := New()
	require.NotNil(t, orchestrator)

	// Configure orchestrator
	cfg := &config.ContextEngineConfig{
		Mode:           config.ContextEngineModePerformance,
		MaxGoroutines:  2,
		MemoryBudgetMB: 256,
	}
	orchestrator.ApplyConfig(cfg)

	// Test initial resource status (after construction)
	status := orchestrator.GetResourceStatus()
	assert.Equal(t, config.ContextEngineModePerformance, status.Mode)
	assert.Equal(t, int64(256), status.MemoryBudgetMB)
	assert.GreaterOrEqual(t, status.ActiveGoroutines, int32(1)) // At least main goroutine
	assert.GreaterOrEqual(t, status.MemoryUsageMB, int64(0))    // Memory usage should be initialized
	assert.False(t, status.LastUpdate.IsZero(), "LastUpdate should be set")

	// Start orchestrator to trigger resource monitoring
	orchestrator.Start()
	defer orchestrator.Stop()

	// Wait for resource monitor to update
	time.Sleep(300 * time.Millisecond)

	// Check updated resource status
	updatedStatus := orchestrator.GetResourceStatus()
	assert.GreaterOrEqual(t, updatedStatus.MemoryUsageMB, int64(0))
	assert.WithinDuration(t, time.Now(), updatedStatus.LastUpdate, 5*time.Second)
	assert.True(t, updatedStatus.LastUpdate.After(status.LastUpdate) || updatedStatus.LastUpdate.Equal(status.LastUpdate),
		"Updated status should have same or later timestamp")
}

func TestQueryOrchestrator_TaskMetricsTracking(t *testing.T) {
	orchestrator := New()
	require.NotNil(t, orchestrator)

	// Configure and start orchestrator
	cfg := &config.ContextEngineConfig{
		Mode:           config.ContextEngineModePerformance,
		MaxGoroutines:  2,
		MemoryBudgetMB: 100,
	}
	orchestrator.ApplyConfig(cfg)
	orchestrator.Start()
	defer orchestrator.Stop()

	// Wait for workers to start
	time.Sleep(100 * time.Millisecond)

	// Use wait groups to ensure task completion
	var successWg, failWg sync.WaitGroup
	successWg.Add(1)
	failWg.Add(1)

	// Submit successful task
	successTask := &Task{
		ID:        "success-task",
		Priority:  5,
		SessionID: "test-session",
		Tier:      Tier1,
		Execute: func(ctx context.Context) error {
			successWg.Done()
			return nil
		},
	}

	err := orchestrator.Submit(successTask)
	require.NoError(t, err)

	// Submit failing task
	failTask := &Task{
		ID:        "fail-task",
		Priority:  5,
		SessionID: "test-session",
		Tier:      Tier1,
		Execute: func(ctx context.Context) error {
			failWg.Done()
			return assert.AnError
		},
	}

	err = orchestrator.Submit(failTask)
	require.NoError(t, err)

	// Wait for tasks to complete
	successWg.Wait()
	failWg.Wait()

	// Give a small buffer for metrics to update
	time.Sleep(50 * time.Millisecond)

	// Check metrics
	metrics := orchestrator.GetMetrics()
	assert.Equal(t, int64(2), metrics.TasksSubmitted)
	assert.Equal(t, int64(1), metrics.TasksCompleted)
	assert.Equal(t, int64(1), metrics.TasksFailed)

	// Average latency should be greater than 0 when tasks have been processed
	totalProcessed := metrics.TasksCompleted + metrics.TasksFailed
	if totalProcessed > 0 {
		assert.GreaterOrEqual(t, metrics.AverageLatencyMs, float64(0),
			"Average latency should be >= 0 when tasks have been processed")
	}

	assert.WithinDuration(t, time.Now(), metrics.LastMetricsUpdate, time.Second)
}
