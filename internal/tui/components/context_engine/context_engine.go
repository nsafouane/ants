package context_engine

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/orchestrator"
	"github.com/charmbracelet/crush/internal/tui/styles"
	"github.com/charmbracelet/crush/internal/tui/util"
	"github.com/charmbracelet/lipgloss/v2"
)

// ContextEngineCmp provides a TUI component for displaying Context Engine status
type ContextEngineCmp interface {
	util.Model
	SetOrchestrator(orch *orchestrator.QueryOrchestrator)
	// UpdateMetrics(metrics orchestrator.PerformanceMetrics)
	UpdateMetrics(metrics *orchestrator.PerformanceMetrics)
	UpdateResourceStatus(status orchestrator.ResourceStatus)
}

type contextEngineCmp struct {
	orchestrator   *orchestrator.QueryOrchestrator
	metrics        *orchestrator.PerformanceMetrics
	resourceStatus orchestrator.ResourceStatus
	width          int
	height         int
	visible        bool
	lastUpdate     time.Time
}

// OrchestratorMetricsMsg carries orchestrator metrics updates
type OrchestratorMetricsMsg struct {
	Metrics *orchestrator.PerformanceMetrics
}

// ResourceStatusMsg carries resource status updates
type ResourceStatusMsg struct {
	Status orchestrator.ResourceStatus
}

// ToggleContextEngineMsg toggles the context engine display
type ToggleContextEngineMsg struct{}

func (m *contextEngineCmp) Init() tea.Cmd {
	return nil
}

func (m *contextEngineCmp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case OrchestratorMetricsMsg:
		m.metrics = msg.Metrics
		m.lastUpdate = time.Now()
		return m, nil

	case ResourceStatusMsg:
		m.resourceStatus = msg.Status
		m.lastUpdate = time.Now()
		return m, nil

	case ToggleContextEngineMsg:
		m.visible = !m.visible
		return m, nil
	}

	return m, nil
}

func (m *contextEngineCmp) View() string {
	if !m.visible || m.orchestrator == nil {
		return ""
	}

	t := styles.CurrentTheme()

	// Header
	header := t.S().Base.
		Background(t.Accent).
		Foreground(t.White).
		Padding(0, 1).
		Bold(true).
		Render("Context Engine Status")

	// Operational mode indicator
	modeColor := t.Green
	switch m.resourceStatus.Mode {
	case config.ContextEngineModeEco:
		modeColor = t.Yellow
	case config.ContextEngineModeOnDemand:
		modeColor = t.Blue
	case config.ContextEngineModePerformance:
		modeColor = t.Green
	}

	modeIndicator := t.S().Base.
		Background(modeColor).
		Foreground(t.White).
		Padding(0, 1).
		Render(fmt.Sprintf("Mode: %s", strings.ToUpper(string(m.resourceStatus.Mode))))

	// Resource usage
	memUsagePct := float64(0)
	if m.resourceStatus.MemoryBudgetMB > 0 {
		memUsagePct = float64(m.resourceStatus.MemoryUsageMB) / float64(m.resourceStatus.MemoryBudgetMB) * 100
	}

	resourceInfo := []string{
		fmt.Sprintf("Memory: %d/%d MB (%.1f%%)",
			m.resourceStatus.MemoryUsageMB,
			m.resourceStatus.MemoryBudgetMB,
			memUsagePct),
		fmt.Sprintf("Goroutines: %d", m.resourceStatus.ActiveGoroutines),
	}

	// Performance metrics
	var metricsInfo []string
	if m.metrics != nil {
		metricsInfo = []string{
			fmt.Sprintf("Tasks: %d submitted, %d completed, %d failed",
				m.metrics.TasksSubmitted,
				m.metrics.TasksCompleted,
				m.metrics.TasksFailed),
			fmt.Sprintf("Queue: %d tasks waiting", m.metrics.QueueLength),
			fmt.Sprintf("Avg Latency: %.2fms", m.metrics.AverageLatencyMs),
			fmt.Sprintf("Active Sessions: %d", m.metrics.ActiveSessions),
		}
	} else {
		metricsInfo = []string{"No metrics available"}
	}

	// Last update timestamp
	updateTime := "Never"
	if !m.lastUpdate.IsZero() {
		updateTime = m.lastUpdate.Format("15:04:05")
	}

	// Build the content sections
	var sections []string

	// Mode and resource section
	modeSection := lipgloss.JoinHorizontal(lipgloss.Left,
		modeIndicator,
		" ",
		t.S().Text.Render(strings.Join(resourceInfo, " • ")))
	sections = append(sections, modeSection)

	// Metrics section
	metricsSection := t.S().Muted.Render(strings.Join(metricsInfo, " • "))
	sections = append(sections, metricsSection)

	// Last update
	updateSection := t.S().Subtle.Render(fmt.Sprintf("Last updated: %s", updateTime))
	sections = append(sections, updateSection)

	// Combine all sections
	content := lipgloss.JoinVertical(lipgloss.Left, sections...)

	// Create the full panel
	panel := lipgloss.JoinVertical(lipgloss.Left,
		header,
		t.S().Base.Padding(0, 1).Render(content),
	)

	// Add border
	finalPanel := t.S().Base.
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Border).
		Width(m.width - 2).
		Render(panel)

	return finalPanel
}

func (m *contextEngineCmp) SetOrchestrator(orch *orchestrator.QueryOrchestrator) {
	m.orchestrator = orch

	// Get initial metrics if orchestrator is available
	if orch != nil {
		// Safely get metrics and status
		defer func() {
			if r := recover(); r != nil {
				// Handle any panics from uninitialized orchestrator
				m.metrics = nil
				m.resourceStatus = orchestrator.ResourceStatus{}
			}
		}()
		metrics := orch.GetMetrics()
		m.metrics = &metrics
		m.resourceStatus = orch.GetResourceStatus()
		m.lastUpdate = time.Now()
	}
}

func (m *contextEngineCmp) UpdateMetrics(metrics *orchestrator.PerformanceMetrics) {
	m.metrics = metrics
	m.lastUpdate = time.Now()
}

func (m *contextEngineCmp) UpdateResourceStatus(status orchestrator.ResourceStatus) {
	m.resourceStatus = status
	m.lastUpdate = time.Now()
}

// Toggle shows/hides the Context Engine panel
func (m *contextEngineCmp) Toggle() {
	m.visible = !m.visible
}

// IsVisible returns whether the Context Engine panel is currently visible
func (m *contextEngineCmp) IsVisible() bool {
	return m.visible
}

// NewContextEngineCmp creates a new Context Engine TUI component
func NewContextEngineCmp() ContextEngineCmp {
	return &contextEngineCmp{
		visible:    false, // Hidden by default
		lastUpdate: time.Time{},
	}
}

// GetMetricsUpdateCmd returns a command to periodically update metrics
func GetMetricsUpdateCmd(orch *orchestrator.QueryOrchestrator) tea.Cmd {
	if orch == nil {
		return nil
	}

	return tea.Tick(5*time.Second, func(time.Time) tea.Msg {
		metrics := orch.GetMetrics()
		return OrchestratorMetricsMsg{
			Metrics: &metrics,
		}
	})
}

// GetResourceStatusUpdateCmd returns a command to periodically update resource status
func GetResourceStatusUpdateCmd(orch *orchestrator.QueryOrchestrator) tea.Cmd {
	if orch == nil {
		return nil
	}

	return tea.Tick(10*time.Second, func(time.Time) tea.Msg {
		return ResourceStatusMsg{
			Status: orch.GetResourceStatus(),
		}
	})
}
