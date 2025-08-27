package context_engine

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/telemetry"
	"github.com/charmbracelet/crush/internal/tui/styles"
	"github.com/charmbracelet/crush/internal/tui/util"
)

// PerformanceDashboard provides real-time performance monitoring for Context Engine.
type PerformanceDashboard interface {
	util.Model
	SetCollectorService(collector *telemetry.CollectorService)
	RefreshMetrics() tea.Cmd
	SetUpdateInterval(interval time.Duration)
}

type performanceDashboard struct {
	// Dependencies
	collectorService *telemetry.CollectorService
	queries          *db.Queries

	// Current metrics data
	metrics      map[string]interface{}
	systemHealth []db.SystemHealth
	activeAlerts []db.PerformanceAlert
	alertCounts  []db.CountActiveAlertsBySeverityRow

	// UI state
	width          int
	height         int
	visible        bool
	lastUpdate     time.Time
	updateInterval time.Duration
	refreshing     bool

	// View options
	showMetrics bool
	showHealth  bool
	showAlerts  bool
	selectedTab int // 0=Overview, 1=Metrics, 2=Health, 3=Alerts
}

// Dashboard messages
type DashboardRefreshMsg struct {
	Metrics      map[string]interface{}
	SystemHealth []db.SystemHealth
	ActiveAlerts []db.PerformanceAlert
	AlertCounts  []db.CountActiveAlertsBySeverityRow
	Error        error
}

type DashboardToggleMsg struct{}
type DashboardTabChangeMsg struct{ Tab int }

// NewPerformanceDashboard creates a new performance monitoring dashboard.
func NewPerformanceDashboard(queries *db.Queries) *performanceDashboard {
	return &performanceDashboard{
		queries:        queries,
		updateInterval: 5 * time.Second,
		visible:        false,
		showMetrics:    true,
		showHealth:     true,
		showAlerts:     true,
		selectedTab:    0,
	}
}

func (d *performanceDashboard) Init() tea.Cmd {
	return tea.Tick(d.updateInterval, func(time.Time) tea.Msg {
		return d.refreshMetrics()
	})
}

func (d *performanceDashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.width = msg.Width
		d.height = msg.Height
		return d, nil

	case tea.KeyMsg:
		if !d.visible {
			return d, nil
		}

		switch msg.String() {
		case "tab":
			d.selectedTab = (d.selectedTab + 1) % 4
			return d, nil
		case "shift+tab":
			d.selectedTab = (d.selectedTab - 1 + 4) % 4
			return d, nil
		case "r":
			return d, d.RefreshMetrics()
		}

	case DashboardRefreshMsg:
		d.refreshing = false
		if msg.Error == nil {
			d.metrics = msg.Metrics
			d.systemHealth = msg.SystemHealth
			d.activeAlerts = msg.ActiveAlerts
			d.alertCounts = msg.AlertCounts
			d.lastUpdate = time.Now()
		}

		// Schedule next refresh
		return d, tea.Tick(d.updateInterval, func(time.Time) tea.Msg {
			return d.refreshMetrics()
		})

	case DashboardToggleMsg:
		d.visible = !d.visible
		if d.visible {
			return d, d.RefreshMetrics()
		}
		return d, nil

	case DashboardTabChangeMsg:
		d.selectedTab = msg.Tab
		return d, nil
	}

	return d, nil
}

func (d *performanceDashboard) View() string {
	if !d.visible {
		return ""
	}

	t := styles.CurrentTheme()

	// Dashboard header
	header := d.renderHeader()

	// Tab navigation
	tabBar := d.renderTabBar()

	// Content based on selected tab
	var content string
	switch d.selectedTab {
	case 0:
		content = d.renderOverview()
	case 1:
		content = d.renderMetricsDetail()
	case 2:
		content = d.renderSystemHealth()
	case 3:
		content = d.renderAlertsDetail()
	default:
		content = d.renderOverview()
	}

	// Footer with refresh info
	footer := d.renderFooter()

	// Combine all sections
	dashboard := lipgloss.JoinVertical(lipgloss.Left,
		header,
		tabBar,
		content,
		footer,
	)

	// Add border and constrain size
	finalDashboard := t.S().Base.
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Border).
		Width(min(d.width-2, 120)).
		Height(min(d.height-2, 30)).
		Render(dashboard)

	return finalDashboard
}

func (d *performanceDashboard) renderHeader() string {
	t := styles.CurrentTheme()

	title := "Context Engine Performance Dashboard"
	status := "●"
	statusColor := t.Green

	if d.refreshing {
		status = "○"
		statusColor = t.Yellow
	} else if len(d.activeAlerts) > 0 {
		// Check for critical alerts
		for _, alert := range d.activeAlerts {
			if alert.Severity == "critical" {
				statusColor = t.Red
				break
			} else if alert.Severity == "high" {
				statusColor = t.Yellow
			}
		}
	}

	headerContent := lipgloss.JoinHorizontal(lipgloss.Left,
		t.S().Base.Foreground(statusColor).Render(status),
		" ",
		t.S().Text.Bold(true).Render(title),
	)

	return t.S().Base.
		Background(t.Accent).
		Foreground(t.White).
		Padding(0, 1).
		Width(100).
		Render(headerContent)
}

func (d *performanceDashboard) renderTabBar() string {
	t := styles.CurrentTheme()

	tabs := []string{"Overview", "Metrics", "Health", "Alerts"}
	var tabButtons []string

	for i, tab := range tabs {
		style := t.S().Base.Padding(0, 2)
		if i == d.selectedTab {
			style = style.Background(t.Accent).Foreground(t.White).Bold(true)
		} else {
			style = style.Foreground(t.Muted)
		}

		// Add alert indicator for alerts tab
		tabText := tab
		if i == 3 && len(d.activeAlerts) > 0 {
			tabText = fmt.Sprintf("%s (%d)", tab, len(d.activeAlerts))
		}

		tabButtons = append(tabButtons, style.Render(tabText))
	}

	return lipgloss.JoinHorizontal(lipgloss.Left, tabButtons...)
}

func (d *performanceDashboard) renderOverview() string {
	t := styles.CurrentTheme()

	// Quick stats
	var stats []string

	// System health summary
	healthyComponents := 0
	totalComponents := len(d.systemHealth)
	for _, health := range d.systemHealth {
		if health.Status == "healthy" {
			healthyComponents++
		}
	}

	healthColor := t.Green
	if healthyComponents < totalComponents {
		healthColor = t.Yellow
		if healthyComponents < totalComponents/2 {
			healthColor = t.Red
		}
	}

	stats = append(stats,
		t.S().Base.Foreground(healthColor).Render(
			fmt.Sprintf("Health: %d/%d components", healthyComponents, totalComponents)))

	// Alert summary
	criticalAlerts := 0
	highAlerts := 0
	for _, alert := range d.activeAlerts {
		if alert.Severity == "critical" {
			criticalAlerts++
		} else if alert.Severity == "high" {
			highAlerts++
		}
	}

	alertColor := t.Green
	alertText := "No active alerts"
	if criticalAlerts > 0 {
		alertColor = t.Red
		alertText = fmt.Sprintf("%d critical alerts", criticalAlerts)
	} else if highAlerts > 0 {
		alertColor = t.Yellow
		alertText = fmt.Sprintf("%d high priority alerts", highAlerts)
	} else if len(d.activeAlerts) > 0 {
		alertColor = t.Blue
		alertText = fmt.Sprintf("%d alerts", len(d.activeAlerts))
	}

	stats = append(stats,
		t.S().Base.Foreground(alertColor).Render(fmt.Sprintf("Alerts: %s", alertText)))

	// Key performance metrics
	if d.metrics != nil {
		// Tier 1 Analysis Time
		if tier1Metric, exists := d.metrics["tier_1_analysis_time"]; exists {
			if metric, ok := tier1Metric.(*telemetry.TimingMetric); ok && metric.Count > 0 {
				avgSeconds := metric.Average.Seconds()
				perfColor := t.Green
				if avgSeconds > 8 {
					perfColor = t.Red
				} else if avgSeconds > 5 {
					perfColor = t.Yellow
				}
				stats = append(stats,
					t.S().Base.Foreground(perfColor).Render(
						fmt.Sprintf("Tier 1 Analysis: %.2fs avg", avgSeconds)))
			}
		}

		// Cache Hit Rate
		if cacheMetric, exists := d.metrics["cache_hit_rate"]; exists {
			if metric, ok := cacheMetric.(*telemetry.RateMetric); ok && metric.Denominator > 0 {
				hitRate := metric.Rate * 100
				perfColor := t.Green
				if hitRate < 60 {
					perfColor = t.Red
				} else if hitRate < 80 {
					perfColor = t.Yellow
				}
				stats = append(stats,
					t.S().Base.Foreground(perfColor).Render(
						fmt.Sprintf("Cache Hit Rate: %.1f%%", hitRate)))
			}
		}
	}

	// Recent activity (last 5 alerts)
	var recentActivity []string
	if len(d.activeAlerts) > 0 {
		recentActivity = append(recentActivity, t.S().Text.Bold(true).Render("Recent Alerts:"))

		// Sort alerts by creation time (most recent first)
		sortedAlerts := make([]db.PerformanceAlert, len(d.activeAlerts))
		copy(sortedAlerts, d.activeAlerts)
		sort.Slice(sortedAlerts, func(i, j int) bool {
			return sortedAlerts[i].CreatedAt.After(sortedAlerts[j].CreatedAt)
		})

		for i, alert := range sortedAlerts {
			if i >= 5 { // Limit to 5 most recent
				break
			}

			severity := alert.Severity
			timeAgo := time.Since(alert.CreatedAt).Truncate(time.Second)

			severityColor := t.Muted
			switch severity {
			case "critical":
				severityColor = t.Red
			case "high":
				severityColor = t.Yellow
			case "medium":
				severityColor = t.Blue
			}

			alertLine := lipgloss.JoinHorizontal(lipgloss.Left,
				t.S().Base.Foreground(severityColor).Render(fmt.Sprintf("[%s]", strings.ToUpper(severity))),
				" ",
				t.S().Text.Render(alert.Message),
				" ",
				t.S().Subtle.Render(fmt.Sprintf("(%s ago)", timeAgo)))

			recentActivity = append(recentActivity, alertLine)
		}
	} else {
		recentActivity = append(recentActivity, t.S().Muted.Render("No recent alerts"))
	}

	// Build overview layout
	overview := lipgloss.JoinVertical(lipgloss.Left,
		t.S().Text.Bold(true).Render("System Overview"),
		"",
		strings.Join(stats, " • "),
		"",
		strings.Join(recentActivity, "\n"),
	)

	return t.S().Base.Padding(1).Render(overview)
}

func (d *performanceDashboard) renderMetricsDetail() string {
	t := styles.CurrentTheme()

	if d.metrics == nil {
		return t.S().Base.Padding(1).Render(t.S().Muted.Render("No metrics data available"))
	}

	var metricSections []string

	// Performance metrics
	perfMetrics := []string{
		"Performance Metrics:",
	}

	if tier1, exists := d.metrics["tier_1_analysis_time"]; exists {
		if metric, ok := tier1.(*telemetry.TimingMetric); ok {
			perfMetrics = append(perfMetrics,
				fmt.Sprintf("  Tier 1 Analysis: %.2fs avg, %d samples",
					metric.Average.Seconds(), metric.Count))
		}
	}

	if aiWorker, exists := d.metrics["ai_worker_query_latency"]; exists {
		if metric, ok := aiWorker.(*telemetry.TimingMetric); ok {
			latencyMs := float64(metric.Average.Nanoseconds()) / 1e6
			perfMetrics = append(perfMetrics,
				fmt.Sprintf("  AI Worker Latency: %.1fms avg, %d samples",
					latencyMs, metric.Count))
		}
	}

	if search, exists := d.metrics["search_latency"]; exists {
		if metric, ok := search.(*telemetry.TimingMetric); ok {
			latencyMs := float64(metric.Average.Nanoseconds()) / 1e6
			perfMetrics = append(perfMetrics,
				fmt.Sprintf("  Search Latency: %.1fms avg, %d samples",
					latencyMs, metric.Count))
		}
	}

	metricSections = append(metricSections, strings.Join(perfMetrics, "\n"))

	// Resource metrics
	resMetrics := []string{
		"Resource Metrics:",
	}

	if kg, exists := d.metrics["knowledge_graph_size"]; exists {
		if metric, ok := kg.(*telemetry.GaugeMetric); ok {
			resMetrics = append(resMetrics,
				fmt.Sprintf("  Knowledge Graph: %.1f MB", metric.Value))
		}
	}

	if cache, exists := d.metrics["cache_hit_rate"]; exists {
		if metric, ok := cache.(*telemetry.RateMetric); ok {
			resMetrics = append(resMetrics,
				fmt.Sprintf("  Cache Hit Rate: %.1f%% (%d/%d)",
					metric.Rate*100, metric.Numerator, metric.Denominator))
		}
	}

	metricSections = append(metricSections, strings.Join(resMetrics, "\n"))

	return t.S().Base.Padding(1).Render(strings.Join(metricSections, "\n\n"))
}

func (d *performanceDashboard) renderSystemHealth() string {
	t := styles.CurrentTheme()

	if len(d.systemHealth) == 0 {
		return t.S().Base.Padding(1).Render(t.S().Muted.Render("No system health data available"))
	}

	var healthLines []string
	healthLines = append(healthLines, t.S().Text.Bold(true).Render("System Component Health:"))
	healthLines = append(healthLines, "")

	for _, health := range d.systemHealth {
		status := health.Status
		statusColor := t.Green
		statusSymbol := "●"

		switch status {
		case "healthy":
			statusColor = t.Green
			statusSymbol = "●"
		case "degraded":
			statusColor = t.Yellow
			statusSymbol = "◐"
		case "unhealthy":
			statusColor = t.Red
			statusSymbol = "○"
		default:
			statusColor = t.Muted
			statusSymbol = "?"
		}

		lastCheck := time.Since(health.LastCheck).Truncate(time.Second)

		healthLine := lipgloss.JoinHorizontal(lipgloss.Left,
			t.S().Base.Foreground(statusColor).Render(statusSymbol),
			" ",
			t.S().Text.Width(20).Render(health.Component),
			" ",
			t.S().Base.Foreground(statusColor).Render(strings.ToUpper(status)),
			" ",
			t.S().Subtle.Render(fmt.Sprintf("(checked %s ago)", lastCheck)),
		)

		healthLines = append(healthLines, healthLine)

		// Show details if available
		if health.Details.Valid && health.Details.String != "" {
			healthLines = append(healthLines,
				t.S().Subtle.Render(fmt.Sprintf("    %s", health.Details.String)))
		}
	}

	return t.S().Base.Padding(1).Render(strings.Join(healthLines, "\n"))
}

func (d *performanceDashboard) renderAlertsDetail() string {
	t := styles.CurrentTheme()

	if len(d.activeAlerts) == 0 {
		return t.S().Base.Padding(1).Render(t.S().Text.Foreground(t.Green).Render("✓ No active alerts"))
	}

	var alertLines []string
	alertLines = append(alertLines, t.S().Text.Bold(true).Render("Active Performance Alerts:"))
	alertLines = append(alertLines, "")

	// Sort alerts by severity and creation time
	sortedAlerts := make([]db.PerformanceAlert, len(d.activeAlerts))
	copy(sortedAlerts, d.activeAlerts)
	sort.Slice(sortedAlerts, func(i, j int) bool {
		severityOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		if severityOrder[sortedAlerts[i].Severity] != severityOrder[sortedAlerts[j].Severity] {
			return severityOrder[sortedAlerts[i].Severity] < severityOrder[sortedAlerts[j].Severity]
		}
		return sortedAlerts[i].CreatedAt.After(sortedAlerts[j].CreatedAt)
	})

	for _, alert := range sortedAlerts {
		severity := alert.Severity
		severityColor := t.Muted
		severitySymbol := "!"

		switch severity {
		case "critical":
			severityColor = t.Red
			severitySymbol = "🔴"
		case "high":
			severityColor = t.Yellow
			severitySymbol = "🟡"
		case "medium":
			severityColor = t.Blue
			severitySymbol = "🔵"
		case "low":
			severityColor = t.Muted
			severitySymbol = "⚪"
		}

		timeAgo := time.Since(alert.CreatedAt).Truncate(time.Second)

		alertHeader := lipgloss.JoinHorizontal(lipgloss.Left,
			severitySymbol,
			" ",
			t.S().Base.Foreground(severityColor).Bold(true).Render(strings.ToUpper(severity)),
			" ",
			t.S().Text.Render(alert.MetricName),
			" ",
			t.S().Subtle.Render(fmt.Sprintf("(%s ago)", timeAgo)),
		)

		alertLines = append(alertLines, alertHeader)
		alertLines = append(alertLines, t.S().Base.Padding(0, 2).Render(alert.Message))

		// Show current vs threshold values if available
		if alert.CurrentValue.Valid && alert.ThresholdValue.Valid {
			valueInfo := fmt.Sprintf("Current: %.2f | Threshold: %.2f",
				alert.CurrentValue.Float64, alert.ThresholdValue.Float64)
			alertLines = append(alertLines,
				t.S().Subtle.Padding(0, 2).Render(valueInfo))
		}

		alertLines = append(alertLines, "")
	}

	return t.S().Base.Padding(1).Render(strings.Join(alertLines, "\n"))
}

func (d *performanceDashboard) renderFooter() string {
	t := styles.CurrentTheme()

	updateTime := "Never"
	if !d.lastUpdate.IsZero() {
		updateTime = d.lastUpdate.Format("15:04:05")
	}

	footerText := fmt.Sprintf("Last updated: %s | Refresh: %s | Press 'r' to refresh, 'tab' to navigate",
		updateTime, d.updateInterval)

	return t.S().Subtle.Render(footerText)
}

func (d *performanceDashboard) refreshMetrics() DashboardRefreshMsg {
	if d.collectorService == nil || d.queries == nil {
		return DashboardRefreshMsg{Error: fmt.Errorf("collector service or queries not initialized")}
	}

	ctx := context.Background()

	// Get current metrics
	metrics := d.collectorService.GetCollectorMetrics()

	// Get system health
	recentTime := time.Now().Add(-10 * time.Minute) // Last 10 minutes
	systemHealth, err := d.queries.GetSystemHealth(ctx, recentTime)
	if err != nil {
		systemHealth = []db.SystemHealth{} // Continue with empty health data
	}

	// Get active alerts
	activeAlerts, err := d.queries.GetActiveAlerts(ctx)
	if err != nil {
		activeAlerts = []db.PerformanceAlert{} // Continue with no alerts
	}

	// Get alert counts by severity
	alertCounts, err := d.queries.CountActiveAlertsBySeverity(ctx)
	if err != nil {
		alertCounts = []db.CountActiveAlertsBySeverityRow{} // Continue with no counts
	}

	return DashboardRefreshMsg{
		Metrics:      metrics,
		SystemHealth: systemHealth,
		ActiveAlerts: activeAlerts,
		AlertCounts:  alertCounts,
	}
}

// Interface implementation

func (d *performanceDashboard) SetCollectorService(collector *telemetry.CollectorService) {
	d.collectorService = collector
}

func (d *performanceDashboard) RefreshMetrics() tea.Cmd {
	d.refreshing = true
	return func() tea.Msg {
		return d.refreshMetrics()
	}
}

func (d *performanceDashboard) SetUpdateInterval(interval time.Duration) {
	d.updateInterval = interval
}

// Helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
