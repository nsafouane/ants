package context_engine

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/orchestrator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewContextEngineCmp(t *testing.T) {
	cmp := NewContextEngineCmp()
	require.NotNil(t, cmp)

	// Verify initial state
	concreteCmp := cmp.(*contextEngineCmp)
	assert.False(t, concreteCmp.visible)
	assert.Nil(t, concreteCmp.orchestrator)
	assert.True(t, concreteCmp.lastUpdate.IsZero())
}

func TestContextEngineCmp_Init(t *testing.T) {
	cmp := NewContextEngineCmp()
	require.NotNil(t, cmp)

	cmd := cmp.Init()
	assert.Nil(t, cmd, "Init should return no command")
}

func TestContextEngineCmp_Update_WindowSizeMsg(t *testing.T) {
	cmp := NewContextEngineCmp()
	require.NotNil(t, cmp)

	// Test window size message
	msg := tea.WindowSizeMsg{
		Width:  1024,
		Height: 768,
	}

	model, cmd := cmp.Update(msg)
	require.NotNil(t, model)
	assert.Nil(t, cmd)

	concreteCmp := model.(*contextEngineCmp)
	assert.Equal(t, 1024, concreteCmp.width)
	assert.Equal(t, 768, concreteCmp.height)
}

func TestContextEngineCmp_Update_OrchestratorMetricsMsg(t *testing.T) {
	cmp := NewContextEngineCmp()
	require.NotNil(t, cmp)

	// Test orchestrator metrics message
	metrics := &orchestrator.PerformanceMetrics{
		TasksSubmitted:    10,
		TasksCompleted:    8,
		TasksFailed:       2,
		AverageLatencyMs:  25.5,
		QueueLength:       3,
		ActiveSessions:    2,
		LastMetricsUpdate: time.Now(),
	}

	msg := OrchestratorMetricsMsg{Metrics: metrics}

	model, cmd := cmp.Update(msg)
	require.NotNil(t, model)
	assert.Nil(t, cmd)

	concreteCmp := model.(*contextEngineCmp)
	assert.Equal(t, metrics, concreteCmp.metrics)
	assert.WithinDuration(t, time.Now(), concreteCmp.lastUpdate, time.Second)
}

func TestContextEngineCmp_Update_ResourceStatusMsg(t *testing.T) {
	cmp := NewContextEngineCmp()
	require.NotNil(t, cmp)

	// Test resource status message
	status := orchestrator.ResourceStatus{
		MemoryUsageMB:    256,
		MemoryBudgetMB:   512,
		ActiveGoroutines: 8,
		LastUpdate:       time.Now(),
		Mode:             config.ContextEngineModePerformance,
	}

	msg := ResourceStatusMsg{Status: status}

	model, cmd := cmp.Update(msg)
	require.NotNil(t, model)
	assert.Nil(t, cmd)

	concreteCmp := model.(*contextEngineCmp)
	assert.Equal(t, status, concreteCmp.resourceStatus)
	assert.WithinDuration(t, time.Now(), concreteCmp.lastUpdate, time.Second)
}

func TestContextEngineCmp_Update_ToggleContextEngineMsg(t *testing.T) {
	cmp := NewContextEngineCmp()
	require.NotNil(t, cmp)

	concreteCmp := cmp.(*contextEngineCmp)
	initialVisible := concreteCmp.visible

	// Test toggle message
	msg := ToggleContextEngineMsg{}

	model, cmd := cmp.Update(msg)
	require.NotNil(t, model)
	assert.Nil(t, cmd)

	updatedCmp := model.(*contextEngineCmp)
	assert.Equal(t, !initialVisible, updatedCmp.visible)

	// Test toggle again
	model2, cmd2 := model.Update(msg)
	require.NotNil(t, model2)
	assert.Nil(t, cmd2)

	finalCmp := model2.(*contextEngineCmp)
	assert.Equal(t, initialVisible, finalCmp.visible)
}

func TestContextEngineCmp_View_NotVisible(t *testing.T) {
	cmp := NewContextEngineCmp()
	require.NotNil(t, cmp)

	// When not visible, should return empty string
	view := cmp.View()
	assert.Empty(t, view)
}

func TestContextEngineCmp_View_NoOrchestrator(t *testing.T) {
	cmp := NewContextEngineCmp()
	require.NotNil(t, cmp)

	// Make visible but no orchestrator
	concreteCmp := cmp.(*contextEngineCmp)
	concreteCmp.visible = true

	view := cmp.View()
	assert.Empty(t, view)
}

func TestContextEngineCmp_View_WithData(t *testing.T) {
	cmp := NewContextEngineCmp()
	require.NotNil(t, cmp)

	// Set up component with data
	concreteCmp := cmp.(*contextEngineCmp)
	concreteCmp.visible = true
	concreteCmp.width = 80
	concreteCmp.height = 24
	concreteCmp.orchestrator = orchestrator.New() // Use properly initialized orchestrator

	// Set metrics
	concreteCmp.metrics = &orchestrator.PerformanceMetrics{
		TasksSubmitted:    100,
		TasksCompleted:    95,
		TasksFailed:       5,
		AverageLatencyMs:  15.2,
		QueueLength:       2,
		ActiveSessions:    3,
		LastMetricsUpdate: time.Now(),
	}

	// Set resource status
	concreteCmp.resourceStatus = orchestrator.ResourceStatus{
		MemoryUsageMB:    128,
		MemoryBudgetMB:   512,
		ActiveGoroutines: 12,
		LastUpdate:       time.Now(),
		Mode:             config.ContextEngineModePerformance,
	}

	concreteCmp.lastUpdate = time.Now()

	view := cmp.View()
	assert.NotEmpty(t, view)

	// Check that view contains expected content
	assert.Contains(t, view, "Context Engine Status")
	assert.Contains(t, view, "PERFORMANCE")
	assert.Contains(t, view, "Memory: 128/512 MB")
	assert.Contains(t, view, "Tasks: 100 submitted, 95 completed, 5 failed")
	assert.Contains(t, view, "Queue: 2 tasks waiting")
	// Note: "Avg Latency: 15.20ms" might be wrapped, so just check for the number
	assert.Contains(t, view, "15.20ms")
	assert.Contains(t, view, "Active Sessions: 3")
}

func TestContextEngineCmp_SetOrchestrator(t *testing.T) {
	cmp := NewContextEngineCmp()
	require.NotNil(t, cmp)

	// Create properly initialized orchestrator
	orchestrator := orchestrator.New()
	require.NotNil(t, orchestrator)

	// Set orchestrator
	cmp.SetOrchestrator(orchestrator)

	concreteCmp := cmp.(*contextEngineCmp)
	assert.Equal(t, orchestrator, concreteCmp.orchestrator)

	// Verify metrics and status were populated
	assert.WithinDuration(t, time.Now(), concreteCmp.lastUpdate, time.Second)
}

func TestContextEngineCmp_UpdateMetrics(t *testing.T) {
	cmp := NewContextEngineCmp()
	require.NotNil(t, cmp)

	metrics := &orchestrator.PerformanceMetrics{
		TasksSubmitted:   50,
		TasksCompleted:   45,
		TasksFailed:      5,
		AverageLatencyMs: 20.0,
		QueueLength:      1,
		ActiveSessions:   2,
	}

	cmp.UpdateMetrics(metrics)

	concreteCmp := cmp.(*contextEngineCmp)
	assert.Equal(t, metrics, concreteCmp.metrics)
	assert.WithinDuration(t, time.Now(), concreteCmp.lastUpdate, time.Second)
}

func TestContextEngineCmp_UpdateResourceStatus(t *testing.T) {
	cmp := NewContextEngineCmp()
	require.NotNil(t, cmp)

	status := orchestrator.ResourceStatus{
		MemoryUsageMB:    64,
		MemoryBudgetMB:   256,
		ActiveGoroutines: 6,
		LastUpdate:       time.Now(),
		Mode:             config.ContextEngineModeEco,
	}

	cmp.UpdateResourceStatus(status)

	concreteCmp := cmp.(*contextEngineCmp)
	assert.Equal(t, status, concreteCmp.resourceStatus)
	assert.WithinDuration(t, time.Now(), concreteCmp.lastUpdate, time.Second)
}

func TestContextEngineCmp_Toggle(t *testing.T) {
	cmp := NewContextEngineCmp()
	require.NotNil(t, cmp)

	concreteCmp := cmp.(*contextEngineCmp)
	initialVisible := concreteCmp.visible

	// Test toggle functionality
	concreteCmp.Toggle()
	assert.Equal(t, !initialVisible, concreteCmp.visible)

	concreteCmp.Toggle()
	assert.Equal(t, initialVisible, concreteCmp.visible)
}

func TestContextEngineCmp_IsVisible(t *testing.T) {
	cmp := NewContextEngineCmp()
	require.NotNil(t, cmp)

	concreteCmp := cmp.(*contextEngineCmp)

	// Initially not visible
	assert.False(t, concreteCmp.IsVisible())

	// Make visible
	concreteCmp.visible = true
	assert.True(t, concreteCmp.IsVisible())

	// Make not visible
	concreteCmp.visible = false
	assert.False(t, concreteCmp.IsVisible())
}

func TestGetMetricsUpdateCmd(t *testing.T) {
	// Test with nil orchestrator
	cmd := GetMetricsUpdateCmd(nil)
	assert.Nil(t, cmd)

	// Test with valid orchestrator (can't easily test the actual command execution
	// without running it, but we can verify it returns a command)
	orchestrator := orchestrator.New()
	cmd = GetMetricsUpdateCmd(orchestrator)
	assert.NotNil(t, cmd)
}

func TestGetResourceStatusUpdateCmd(t *testing.T) {
	// Test with nil orchestrator
	cmd := GetResourceStatusUpdateCmd(nil)
	assert.Nil(t, cmd)

	// Test with valid orchestrator
	orchestrator := orchestrator.New()
	cmd = GetResourceStatusUpdateCmd(orchestrator)
	assert.NotNil(t, cmd)
}

func TestContextEngineCmp_OperationalModeColors(t *testing.T) {
	cmp := NewContextEngineCmp()
	require.NotNil(t, cmp)

	concreteCmp := cmp.(*contextEngineCmp)
	concreteCmp.visible = true
	concreteCmp.width = 80
	concreteCmp.orchestrator = orchestrator.New()
	concreteCmp.lastUpdate = time.Now()

	// Test different operational modes
	modes := []config.ContextEngineMode{
		config.ContextEngineModePerformance,
		config.ContextEngineModeOnDemand,
		config.ContextEngineModeEco,
	}

	for _, mode := range modes {
		concreteCmp.resourceStatus = orchestrator.ResourceStatus{
			Mode:             mode,
			MemoryUsageMB:    100,
			MemoryBudgetMB:   256,
			ActiveGoroutines: 4,
			LastUpdate:       time.Now(),
		}

		view := cmp.View()
		assert.NotEmpty(t, view)

		// Verify mode is displayed
		switch mode {
		case config.ContextEngineModePerformance:
			assert.Contains(t, view, "PERFORMANCE")
		case config.ContextEngineModeOnDemand:
			assert.Contains(t, view, "ON_DEMAND")
		case config.ContextEngineModeEco:
			assert.Contains(t, view, "ECO")
		}
	}
}

func TestContextEngineCmp_MemoryUsageCalculation(t *testing.T) {
	cmp := NewContextEngineCmp()
	require.NotNil(t, cmp)

	concreteCmp := cmp.(*contextEngineCmp)
	concreteCmp.visible = true
	concreteCmp.width = 80
	concreteCmp.orchestrator = orchestrator.New()
	concreteCmp.lastUpdate = time.Now()

	// Test memory usage percentage calculation
	tests := []struct {
		name        string
		usage       int64
		budget      int64
		expectedPct string
	}{
		{
			name:        "50% usage",
			usage:       256,
			budget:      512,
			expectedPct: "50.0%",
		},
		{
			name:        "25% usage",
			usage:       128,
			budget:      512,
			expectedPct: "25.0%",
		},
		{
			name:        "100% usage",
			usage:       512,
			budget:      512,
			expectedPct: "100.0%",
		},
		{
			name:        "No budget",
			usage:       256,
			budget:      0,
			expectedPct: "0.0%", // Should handle division by zero
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			concreteCmp.resourceStatus = orchestrator.ResourceStatus{
				Mode:             config.ContextEngineModePerformance,
				MemoryUsageMB:    tt.usage,
				MemoryBudgetMB:   tt.budget,
				ActiveGoroutines: 4,
				LastUpdate:       time.Now(),
			}

			view := cmp.View()
			assert.NotEmpty(t, view)

			// Verify percentage is displayed correctly
			assert.Contains(t, view, tt.expectedPct)
		})
	}
}
