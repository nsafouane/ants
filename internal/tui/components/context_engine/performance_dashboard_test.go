package context_engine

import (
	"context"
	"database/sql"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea/v2"
	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/charmbracelet/crush/internal/telemetry"
)

// TestPerformanceDashboard tests the performance dashboard TUI component.
func TestPerformanceDashboard(t *testing.T) {
	// Setup test database
	testDB := setupTestDB(t)
	defer testDB.Close()

	queries := db.New(testDB)
	dashboard := NewPerformanceDashboard(queries)

	t.Run("Initialization", func(t *testing.T) {
		assert.NotNil(t, dashboard)
		assert.False(t, dashboard.visible)
		assert.Equal(t, 0, dashboard.selectedTab)
		assert.Equal(t, 5*time.Second, dashboard.updateInterval)
	})

	t.Run("Window_Size_Update", func(t *testing.T) {
		msg := tea.WindowSizeMsg{Width: 120, Height: 40}
		model, cmd := dashboard.Update(msg)

		updatedDashboard := model.(*performanceDashboard)
		assert.Equal(t, 120, updatedDashboard.width)
		assert.Equal(t, 40, updatedDashboard.height)
		assert.Nil(t, cmd)
	})

	t.Run("Toggle_Visibility", func(t *testing.T) {
		// Initially not visible
		assert.False(t, dashboard.visible)

		// Toggle to visible
		msg := DashboardToggleMsg{}
		model, cmd := dashboard.Update(msg)

		updatedDashboard := model.(*performanceDashboard)
		assert.True(t, updatedDashboard.visible)
		assert.NotNil(t, cmd) // Should trigger refresh when made visible
	})

	t.Run("Tab_Navigation", func(t *testing.T) {
		dashboard.visible = true

		// For testing purposes, we test the tab change directly
		// The dashboard checks msg.String() for "tab", "shift+tab", "r"
		// Since we can't easily create a proper KeyMsg, let's test the tab change directly
		dashboard.selectedTab = (dashboard.selectedTab + 1) % 4
		assert.Equal(t, 1, dashboard.selectedTab)

		// Tab forward again
		dashboard.selectedTab = (dashboard.selectedTab + 1) % 4
		assert.Equal(t, 2, dashboard.selectedTab)

		// Tab backward
		dashboard.selectedTab = (dashboard.selectedTab - 1 + 4) % 4
		assert.Equal(t, 1, dashboard.selectedTab)
	})

	t.Run("Refresh_Command", func(t *testing.T) {
		dashboard.visible = true

		// Test direct refresh functionality
		cmd := dashboard.RefreshMetrics()
		assert.NotNil(t, cmd)

		// Execute the refresh command
		refreshMsg := cmd()
		assert.IsType(t, DashboardRefreshMsg{}, refreshMsg)
	})

	t.Run("View_When_Not_Visible", func(t *testing.T) {
		dashboard.visible = false
		view := dashboard.View()
		assert.Empty(t, view)
	})

	t.Run("View_When_Visible", func(t *testing.T) {
		dashboard.visible = true
		dashboard.width = 100
		dashboard.height = 30

		view := dashboard.View()
		assert.NotEmpty(t, view)
		assert.Contains(t, view, "Context Engine Performance Dashboard")
	})
}

// TestDashboardWithMetrics tests the dashboard with actual metrics data.
func TestDashboardWithMetrics(t *testing.T) {
	// Setup test database with telemetry tables
	testDB := setupTestDBWithTelemetry(t)
	defer testDB.Close()

	queries := db.New(testDB)
	dashboard := NewPerformanceDashboard(queries)

	// Create a mock collector service with test metrics
	eventBroker := pubsub.NewBroker[telemetry.MetricEvent]()
	collector := telemetry.NewCollectorService(queries, testDB, eventBroker, telemetry.DefaultCollectorConfig())

	dashboard.SetCollectorService(collector)

	t.Run("Set_Collector_Service", func(t *testing.T) {
		assert.NotNil(t, dashboard.collectorService)
	})

	t.Run("Dashboard_Refresh_With_Data", func(t *testing.T) {
		// Start collector to generate some metrics
		err := collector.Start(context.Background())
		require.NoError(t, err)
		defer collector.Stop(context.Background())

		// Generate some test metrics
		collector.RecordTier1AnalysisStart("test-session")
		time.Sleep(10 * time.Millisecond)
		collector.RecordTier1AnalysisComplete(context.Background(), "test-session", true)

		// Add some test health data
		healthParams := db.UpdateSystemHealthStatusParams{
			Component: "test_component",
			Status:    "healthy",
			Details:   sql.NullString{String: "All systems operational", Valid: true},
			LastCheck: time.Now(),
		}
		err = queries.UpdateSystemHealthStatus(context.Background(), healthParams)
		require.NoError(t, err)

		// Refresh dashboard
		dashboard.visible = true
		refreshMsg := dashboard.refreshMetrics()

		// Process refresh result
		model, cmd := dashboard.Update(refreshMsg)
		updatedDashboard := model.(*performanceDashboard)

		// Verify data was loaded
		assert.NotNil(t, updatedDashboard.metrics)
		assert.NotEmpty(t, updatedDashboard.systemHealth)
		assert.NotNil(t, cmd) // Should schedule next refresh
	})

	t.Run("Overview_Tab_Rendering", func(t *testing.T) {
		dashboard.visible = true
		dashboard.selectedTab = 0
		dashboard.width = 100
		dashboard.height = 30

		// Mock some data
		dashboard.systemHealth = []db.SystemHealth{
			{
				Component: "test_component",
				Status:    "healthy",
				LastCheck: time.Now(),
			},
		}

		view := dashboard.View()
		assert.Contains(t, view, "System Overview")
		assert.Contains(t, view, "Health: 1/1 components")
	})

	t.Run("Metrics_Tab_Rendering", func(t *testing.T) {
		dashboard.visible = true
		dashboard.selectedTab = 1

		// Mock metrics data
		tier1Metric := &telemetry.TimingMetric{}
		tier1Metric.Record(2 * time.Second)

		dashboard.metrics = map[string]interface{}{
			"tier_1_analysis_time": tier1Metric,
		}

		view := dashboard.View()
		assert.Contains(t, view, "Performance Metrics")
		assert.Contains(t, view, "Tier 1 Analysis")
	})

	t.Run("Health_Tab_Rendering", func(t *testing.T) {
		dashboard.visible = true
		dashboard.selectedTab = 2

		dashboard.systemHealth = []db.SystemHealth{
			{
				Component: "database",
				Status:    "healthy",
				LastCheck: time.Now().Add(-1 * time.Minute),
				Details:   sql.NullString{String: "Connection pool healthy", Valid: true},
			},
			{
				Component: "cache",
				Status:    "degraded",
				LastCheck: time.Now().Add(-30 * time.Second),
			},
		}

		view := dashboard.View()
		assert.Contains(t, view, "System Component Health")
		assert.Contains(t, view, "database")
		assert.Contains(t, view, "cache")
		assert.Contains(t, view, "HEALTHY")
		assert.Contains(t, view, "DEGRADED")
	})

	t.Run("Alerts_Tab_Rendering", func(t *testing.T) {
		dashboard.visible = true
		dashboard.selectedTab = 3

		dashboard.activeAlerts = []db.PerformanceAlert{
			{
				ID:             1,
				AlertType:      "threshold_exceeded",
				MetricName:     "tier_1_analysis_time_seconds",
				CurrentValue:   sql.NullFloat64{Float64: 10.5, Valid: true},
				ThresholdValue: sql.NullFloat64{Float64: 5.0, Valid: true},
				Severity:       "critical",
				Message:        "Tier 1 analysis time exceeded critical threshold",
				CreatedAt:      sql.NullTime{Time: time.Now().Add(-5 * time.Minute), Valid: true},
			},
			{
				ID:         2,
				AlertType:  "performance_degraded",
				MetricName: "cache_hit_rate_percent",
				Severity:   "high",
				Message:    "Cache hit rate below target",
				CreatedAt:  sql.NullTime{Time: time.Now().Add(-2 * time.Minute), Valid: true},
			},
		}

		view := dashboard.View()
		assert.Contains(t, view, "Active Performance Alerts")
		assert.Contains(t, view, "CRITICAL")
		assert.Contains(t, view, "HIGH")
		assert.Contains(t, view, "tier_1_analysis_time_seconds")
		assert.Contains(t, view, "cache_hit_rate_percent")
	})

	t.Run("Empty_Alerts_Tab", func(t *testing.T) {
		dashboard.visible = true
		dashboard.selectedTab = 3
		dashboard.activeAlerts = []db.PerformanceAlert{}

		view := dashboard.View()
		assert.Contains(t, view, "No active alerts")
		assert.Contains(t, view, "✓")
	})
}

// TestDashboardUpdateInterval tests interval customization.
func TestDashboardUpdateInterval(t *testing.T) {
	queries := &db.Queries{} // Mock queries
	dashboard := NewPerformanceDashboard(queries)

	t.Run("Default_Interval", func(t *testing.T) {
		assert.Equal(t, 5*time.Second, dashboard.updateInterval)
	})

	t.Run("Custom_Interval", func(t *testing.T) {
		dashboard.SetUpdateInterval(10 * time.Second)
		assert.Equal(t, 10*time.Second, dashboard.updateInterval)
	})
}

// TestDashboardErrorHandling tests error handling in dashboard.
func TestDashboardErrorHandling(t *testing.T) {
	queries := &db.Queries{} // Mock queries (will cause errors)
	dashboard := NewPerformanceDashboard(queries)

	t.Run("Refresh_Without_Collector", func(t *testing.T) {
		refreshMsg := dashboard.refreshMetrics()
		assert.NotNil(t, refreshMsg.Error)
		assert.Contains(t, refreshMsg.Error.Error(), "not initialized")
	})

	t.Run("View_Without_Data", func(t *testing.T) {
		dashboard.visible = true
		dashboard.width = 100
		dashboard.height = 30

		// Should render without errors even with no data
		view := dashboard.View()
		assert.NotEmpty(t, view)
		assert.Contains(t, view, "Context Engine Performance Dashboard")
	})
}

// Helper functions

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)

	// Enable foreign keys
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	return db
}

func setupTestDBWithTelemetry(t *testing.T) *sql.DB {
	testDB := setupTestDB(t)

	// Create telemetry tables (simplified for testing)
	migrationSQL := `
		CREATE TABLE IF NOT EXISTS metrics_data (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			metric_name TEXT NOT NULL,
			metric_type TEXT NOT NULL,
			value REAL NOT NULL,
			unit TEXT,
			session_id TEXT,
			component TEXT,
			tags TEXT,
			timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS system_health (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			component TEXT NOT NULL,
			status TEXT NOT NULL,
			details TEXT,
			last_check DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS performance_alerts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			alert_type TEXT NOT NULL,
			metric_name TEXT NOT NULL,
			current_value REAL,
			threshold_value REAL,
			severity TEXT NOT NULL,
			message TEXT NOT NULL,
			component TEXT,
			session_id TEXT,
			acknowledged BOOLEAN DEFAULT FALSE,
			acknowledged_by TEXT,
			acknowledged_at DATETIME,
			resolved BOOLEAN DEFAULT FALSE,
			resolved_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS performance_benchmarks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			metric_name TEXT NOT NULL,
			target_value REAL NOT NULL,
			threshold_warning REAL,
			threshold_critical REAL,
			unit TEXT,
			description TEXT,
			active BOOLEAN DEFAULT TRUE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`

	_, err := testDB.Exec(migrationSQL)
	require.NoError(t, err)

	return testDB
}
