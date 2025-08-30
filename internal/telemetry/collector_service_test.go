package telemetry

import (
	"context"
	"database/sql"
	"runtime"
	"testing"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/pubsub"
)

// TestMetricsCollector tests the core metrics collection functionality.
func TestMetricsCollector(t *testing.T) {
	ctx := context.Background()
	testDB := setupTestDB(t)
	defer testDB.Close()

	// Create event broker
	eventBroker := pubsub.NewBroker[MetricEvent]()

	// Create metrics collector
	queries := db.New(testDB)
	collector := NewMetricsCollector(queries, testDB, eventBroker, DefaultMetricsConfig())

	t.Run("Initialization", func(t *testing.T) {
		assert.NotNil(t, collector)
		assert.NotNil(t, collector.metrics)
		assert.NotNil(t, collector.config)
		assert.False(t, collector.running)
	})

	t.Run("Start_Stop", func(t *testing.T) {
		err := collector.Start(ctx)
		require.NoError(t, err)
		assert.True(t, collector.running)

		err = collector.Stop(ctx)
		require.NoError(t, err)
		assert.False(t, collector.running)
	})

	t.Run("Tier1_Analysis_Metrics", func(t *testing.T) {
		err := collector.Start(ctx)
		require.NoError(t, err)
		defer collector.Stop(ctx)

		// Record successful analysis
		duration := 2 * time.Second
		sessionID := "test-session-1"
		collector.RecordTier1AnalysisTime(ctx, duration, true, sessionID)

		// Verify metrics were updated
		metrics := collector.GetMetrics()
		assert.Contains(t, metrics, "tier_1_analysis_time")

		tier1Metric := metrics["tier_1_analysis_time"].(*TimingMetric)
		assert.Equal(t, int64(1), tier1Metric.Count)
		assert.Equal(t, duration, tier1Metric.Average)
	})

	t.Run("AI_Worker_Query_Metrics", func(t *testing.T) {
		err := collector.Start(ctx)
		require.NoError(t, err)
		defer collector.Stop(ctx)

		// Record AI worker query
		latency := 1500 * time.Millisecond
		sessionID := "test-session-2"
		collector.RecordAIWorkerQuery(ctx, latency, true, sessionID)

		// Verify metrics
		metrics := collector.GetMetrics()
		aiWorkerMetric := metrics["ai_worker_query_latency"].(*TimingMetric)
		assert.Equal(t, int64(1), aiWorkerMetric.Count)
		assert.Equal(t, latency, aiWorkerMetric.Average)
	})

	t.Run("Knowledge_Graph_Metrics", func(t *testing.T) {
		err := collector.Start(ctx)
		require.NoError(t, err)
		defer collector.Stop(ctx)

		// Update knowledge graph stats
		sizeMB := 15.5
		nodesCount := int64(1000)
		depsCount := int64(2500)

		collector.UpdateKnowledgeGraphSize(ctx, sizeMB, nodesCount, depsCount)

		// Verify metrics
		metrics := collector.GetMetrics()
		kgSizeMetric := metrics["knowledge_graph_size"].(*GaugeMetric)
		assert.Equal(t, sizeMB, kgSizeMetric.Value)
	})

	t.Run("Vector_DB_Metrics", func(t *testing.T) {
		err := collector.Start(ctx)
		require.NoError(t, err)
		defer collector.Stop(ctx)

		// Update vector DB status
		status := "healthy"
		details := map[string]interface{}{
			"index_size": 1000,
			"last_sync":  time.Now(),
		}
		collector.UpdateVectorDBIndexStatus(ctx, status, details)

		// Record vector DB latency
		latency := 50 * time.Millisecond
		sessionID := "test-session-3"
		collector.RecordVectorDBLatency(ctx, latency, sessionID)

		// Verify metrics
		metrics := collector.GetMetrics()
		vectorDBMetric := metrics["vector_db_index_status"].(*StatusMetric)
		assert.Equal(t, status, vectorDBMetric.Status)
	})

	t.Run("Cache_Metrics", func(t *testing.T) {
		err := collector.Start(ctx)
		require.NoError(t, err)
		defer collector.Stop(ctx)

		// Update cache metrics
		hits := int64(80)
		total := int64(100)
		sizeMB := 32.5

		collector.UpdateCacheHitRate(ctx, hits, total, sizeMB)

		// Record evictions
		evictions := int64(5)
		collector.RecordCacheEviction(ctx, evictions)

		// Verify metrics
		metrics := collector.GetMetrics()
		cacheHitMetric := metrics["cache_hit_rate"].(*RateMetric)
		assert.Equal(t, hits, cacheHitMetric.Numerator)
		assert.Equal(t, total, cacheHitMetric.Denominator)
		assert.Equal(t, 0.8, cacheHitMetric.Rate)
	})

	t.Run("Search_Metrics", func(t *testing.T) {
		err := collector.Start(ctx)
		require.NoError(t, err)
		defer collector.Stop(ctx)

		// Record search latency
		latency := 150 * time.Millisecond
		sessionID := "test-session-4"
		collector.RecordSearchLatency(ctx, latency, sessionID)

		// Update search accuracy
		relevant := int64(85)
		total := int64(100)
		collector.UpdateSearchAccuracy(ctx, relevant, total)

		// Update indexing throughput
		docsIndexed := int64(50)
		duration := 2 * time.Second
		collector.UpdateIndexingThroughput(ctx, docsIndexed, duration)

		// Verify metrics
		metrics := collector.GetMetrics()
		searchLatencyMetric := metrics["search_latency"].(*TimingMetric)
		assert.Equal(t, int64(1), searchLatencyMetric.Count)
		assert.Equal(t, latency, searchLatencyMetric.Average)
	})

	t.Run("Health_Check", func(t *testing.T) {
		err := collector.HealthCheck(ctx)
		assert.NoError(t, err)
	})
}

// TestCollectorService tests the full metrics collection service.
func TestCollectorService(t *testing.T) {
	ctx := context.Background()
	testDB := setupTestDB(t)
	defer testDB.Close()

	// Run database migrations to ensure telemetry tables exist
	err := runTelemetryMigrations(testDB)
	require.NoError(t, err)

	// Create event broker and collector service
	eventBroker := pubsub.NewBroker[MetricEvent]()
	queries := db.New(testDB)
	config := DefaultCollectorConfig()
	config.SystemMetricsInterval = 100 * time.Millisecond // Fast for testing

	collector := NewCollectorService(queries, testDB, eventBroker, config)

	t.Run("Service_Lifecycle", func(t *testing.T) {
		err := collector.Start(ctx)
		require.NoError(t, err)
		assert.True(t, collector.running)

		// Wait a bit for background processes
		time.Sleep(200 * time.Millisecond)

		err = collector.Stop(ctx)
		require.NoError(t, err)
		assert.False(t, collector.running)
	})

	t.Run("Analysis_Workflow", func(t *testing.T) {
		err := collector.Start(ctx)
		require.NoError(t, err)
		defer collector.Stop(ctx)

		sessionID := "analysis-test-session"

		// Simulate Tier 1 analysis workflow
		collector.RecordTier1AnalysisStart(sessionID)
		time.Sleep(50 * time.Millisecond) // Simulate processing time
		collector.RecordTier1AnalysisComplete(ctx, sessionID, true)

		// Verify metrics were recorded
		metrics := collector.GetCollectorMetrics()
		assert.Contains(t, metrics, "tier_1_analysis_time")
	})

	t.Run("AI_Worker_Workflow", func(t *testing.T) {
		err := collector.Start(ctx)
		require.NoError(t, err)
		defer collector.Stop(ctx)

		queryID := "ai-query-123"
		sessionID := "ai-test-session"

		// Simulate AI worker query workflow
		collector.RecordAIWorkerQueryStart(queryID)
		time.Sleep(30 * time.Millisecond) // Simulate query time
		collector.RecordAIWorkerQueryComplete(ctx, queryID, sessionID, true)

		// Verify metrics
		metrics := collector.GetCollectorMetrics()
		assert.Contains(t, metrics, "ai_worker_query_latency")
	})

	t.Run("Vector_Operations_Workflow", func(t *testing.T) {
		err := collector.Start(ctx)
		require.NoError(t, err)
		defer collector.Stop(ctx)

		operationID := "vector-op-456"
		sessionID := "vector-test-session"

		// Simulate vector embedding workflow
		collector.RecordVectorEmbeddingStart(operationID)
		time.Sleep(25 * time.Millisecond)
		collector.RecordVectorEmbeddingComplete(ctx, operationID, sessionID)

		// Simulate vector query workflow
		queryID := "vector-query-789"
		collector.RecordVectorQueryStart(queryID)
		time.Sleep(15 * time.Millisecond)
		collector.RecordVectorQueryComplete(ctx, queryID, sessionID)

		// Update vector DB status
		collector.UpdateVectorDBIndexStatus(ctx, "healthy", map[string]interface{}{
			"embeddings_count": 5000,
		})

		// Verify metrics
		metrics := collector.GetCollectorMetrics()
		assert.Contains(t, metrics, "vector_db_index_status")
	})

	t.Run("Search_Operations_Workflow", func(t *testing.T) {
		err := collector.Start(ctx)
		require.NoError(t, err)
		defer collector.Stop(ctx)

		searchID := "search-op-111"
		sessionID := "search-test-session"

		// Simulate search workflow
		collector.RecordSearchStart(searchID)
		time.Sleep(20 * time.Millisecond)
		collector.RecordSearchComplete(ctx, searchID, sessionID, 8, 10) // 80% accuracy

		// Verify metrics
		metrics := collector.GetCollectorMetrics()
		assert.Contains(t, metrics, "search_latency")
	})

	t.Run("Cache_Operations", func(t *testing.T) {
		err := collector.Start(ctx)
		require.NoError(t, err)
		defer collector.Stop(ctx)

		// Simulate cache operations
		collector.RecordCacheHit(ctx)
		collector.RecordCacheHit(ctx)
		collector.RecordCacheMiss(ctx)

		collector.UpdateCacheSize(ctx, 1024*1024*50) // 50MB
		collector.RecordCacheEviction(ctx, 3)

		// Allow time for cache metrics loop
		time.Sleep(150 * time.Millisecond)

		// Verify cache metrics were updated
		metrics := collector.GetCollectorMetrics()
		assert.Contains(t, metrics, "cache_hit_rate")
	})

	t.Run("Health_Check", func(t *testing.T) {
		err := collector.Start(ctx)
		require.NoError(t, err)
		defer collector.Stop(ctx)

		err = collector.HealthCheck(ctx)
		assert.NoError(t, err)
	})
}

// TestPerformanceOverhead measures the performance overhead of metrics collection.
func TestPerformanceOverhead(t *testing.T) {
	ctx := context.Background()
	testDB := setupTestDB(t)
	defer testDB.Close()

	// Run migrations
	err := runTelemetryMigrations(testDB)
	require.NoError(t, err)

	eventBroker := pubsub.NewBroker[MetricEvent]()
	queries := db.New(testDB)
	collector := NewCollectorService(queries, testDB, eventBroker, DefaultCollectorConfig())

	err = collector.Start(ctx)
	require.NoError(t, err)
	defer collector.Stop(ctx)

	t.Run("Metrics_Collection_Overhead", func(t *testing.T) {
		iterations := 1000
		sessionID := "performance-test"

		// Measure baseline performance (no metrics)
		start := time.Now()
		for i := 0; i < iterations; i++ {
			// Simulate a simple operation
			time.Sleep(time.Microsecond)
		}
		baselineDuration := time.Since(start)

		// Measure performance with metrics collection
		start = time.Now()
		for i := 0; i < iterations; i++ {
			collector.RecordTier1AnalysisStart(sessionID)
			time.Sleep(time.Microsecond) // Simulate operation
			collector.RecordTier1AnalysisComplete(ctx, sessionID, true)
		}
		withMetricsDuration := time.Since(start)

		// Calculate overhead
		overhead := withMetricsDuration - baselineDuration
		overheadPercent := float64(overhead) / float64(baselineDuration) * 100

		t.Logf("Baseline duration: %v", baselineDuration)
		t.Logf("With metrics duration: %v", withMetricsDuration)
		t.Logf("Overhead: %v (%.2f%%)", overhead, overheadPercent)

		// Assert that overhead is reasonable (< 20%)
		assert.Less(t, overheadPercent, 20.0, "Metrics collection overhead should be less than 20%")
	})

	t.Run("Memory_Usage", func(t *testing.T) {
		var memBefore, memAfter runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&memBefore)

		// Collect many metrics
		sessionID := "memory-test"
		for i := 0; i < 10000; i++ {
			collector.RecordTier1AnalysisStart(sessionID)
			collector.RecordTier1AnalysisComplete(ctx, sessionID, true)
		}

		runtime.GC()
		runtime.ReadMemStats(&memAfter)

		memIncrease := memAfter.Alloc - memBefore.Alloc
		t.Logf("Memory increase: %d bytes", memIncrease)

		// Assert reasonable memory usage (< 10MB for 10k metrics)
		assert.Less(t, memIncrease, uint64(10*1024*1024), "Memory usage should be reasonable")
	})
}

// TestEventIntegration tests integration with the pubsub event system.
func TestEventIntegration(t *testing.T) {
	ctx := context.Background()
	testDB := setupTestDB(t)
	defer testDB.Close()

	// Run migrations
	err := runTelemetryMigrations(testDB)
	require.NoError(t, err)

	eventBroker := pubsub.NewBroker[MetricEvent]()
	queries := db.New(testDB)

	config := DefaultCollectorConfig()
	config.CollectSystemMetrics = true // Enable system metrics collection

	collector := NewCollectorService(queries, testDB, eventBroker, config)

	t.Run("Event_Publication", func(t *testing.T) {
		err := collector.Start(ctx)
		require.NoError(t, err)
		defer collector.Stop(ctx)

		// Subscribe to metric events
		events := make(chan MetricEvent, 10)
		// Note: Event subscription to be implemented
		// eventBroker.Subscribe("tier_1_analysis_time_seconds", func(event MetricEvent) {
		// 	events <- event
		// })

		// Record a metric that should generate an event
		sessionID := "event-test"
		collector.RecordTier1AnalysisStart(sessionID)
		collector.RecordTier1AnalysisComplete(ctx, sessionID, true)

		// Wait for event
		select {
		case event := <-events:
			assert.Equal(t, "timing", event.Type)
			assert.Equal(t, "tier_1_analysis_time_seconds", event.Name)
			assert.Equal(t, sessionID, event.SessionID)
			assert.NotZero(t, event.Timestamp)
		case <-time.After(1 * time.Second):
			t.Fatal("Expected metric event was not received")
		}
	})
}

// TestDatabaseIntegration tests database storage and retrieval of metrics.
func TestDatabaseIntegration(t *testing.T) {
	ctx := context.Background()
	testDB := setupTestDB(t)
	defer testDB.Close()

	// Run migrations
	err := runTelemetryMigrations(testDB)
	require.NoError(t, err)

	queries := db.New(testDB)

	t.Run("Metric_Data_Storage", func(t *testing.T) {
		// Insert metric data
		params := db.InsertMetricDataParams{
			MetricName: "test_metric",
			MetricType: "timing",
			Value:      1.5,
			Unit:       sql.NullString{String: "seconds", Valid: true},
			SessionID:  sql.NullString{String: "test-session", Valid: true},
			Component:  sql.NullString{String: "test_component", Valid: true},
			Tags:       sql.NullString{String: `{"env":"test"}`, Valid: true},
			Timestamp:  time.Now(),
		}

		err := queries.InsertMetricData(ctx, params)
		require.NoError(t, err)

		// Retrieve metric data
		getParams := db.GetMetricDataParams{
			MetricName:  "test_metric",
			Timestamp:   time.Now().Add(-1 * time.Hour),
			Timestamp_2: time.Now().Add(1 * time.Hour),
			Limit:       10,
		}

		metrics, err := queries.GetMetricData(ctx, getParams)
		require.NoError(t, err)
		assert.Len(t, metrics, 1)
		assert.Equal(t, "test_metric", metrics[0].MetricName)
		assert.Equal(t, 1.5, metrics[0].Value)
	})

	t.Run("Performance_Benchmarks", func(t *testing.T) {
		// Insert performance benchmark
		params := db.InsertPerformanceBenchmarkParams{
			MetricName:        "test_latency",
			TargetValue:       1.0,
			ThresholdWarning:  sql.NullFloat64{Float64: 2.0, Valid: true},
			ThresholdCritical: sql.NullFloat64{Float64: 5.0, Valid: true},
			Unit:              sql.NullString{String: "seconds", Valid: true},
			Description:       sql.NullString{String: "Test latency benchmark", Valid: true},
			Active:            sql.NullBool{Bool: true, Valid: true},
		}

		err := queries.InsertPerformanceBenchmark(ctx, params)
		require.NoError(t, err)

		// Retrieve benchmark
		benchmark, err := queries.GetPerformanceBenchmark(ctx, "test_latency")
		require.NoError(t, err)
		assert.Equal(t, "test_latency", benchmark.MetricName)
		assert.Equal(t, 1.0, benchmark.TargetValue)
	})

	t.Run("System_Health_Status", func(t *testing.T) {
		// Update system health
		params := db.UpdateSystemHealthStatusParams{
			Component: "test_component",
			Status:    "healthy",
			Details:   sql.NullString{String: `{"version":"1.0"}`, Valid: true},
			LastCheck: time.Now(),
		}

		err := queries.UpdateSystemHealthStatus(ctx, params)
		require.NoError(t, err)

		// Retrieve health status
		health, err := queries.GetComponentHealth(ctx, "test_component")
		require.NoError(t, err)
		assert.Equal(t, "test_component", health.Component)
		assert.Equal(t, "healthy", health.Status)
	})

	t.Run("Performance_Alerts", func(t *testing.T) {
		// Insert performance alert
		params := db.InsertPerformanceAlertParams{
			AlertType:      "threshold_exceeded",
			MetricName:     "test_metric",
			CurrentValue:   sql.NullFloat64{Float64: 3.0, Valid: true},
			ThresholdValue: sql.NullFloat64{Float64: 2.0, Valid: true},
			Severity:       "high",
			Message:        "Test metric exceeded threshold",
			Component:      sql.NullString{String: "test_component", Valid: true},
			SessionID:      sql.NullString{String: "test-session", Valid: true},
		}

		alertID, err := queries.InsertPerformanceAlert(ctx, params)
		require.NoError(t, err)
		assert.NotZero(t, alertID)

		// Retrieve active alerts
		alerts, err := queries.GetActiveAlerts(ctx)
		require.NoError(t, err)
		assert.NotEmpty(t, alerts)

		// Acknowledge alert
		ackParams := db.AcknowledgeAlertParams{
			ID:             alertID,
			AcknowledgedBy: sql.NullString{String: "test_user", Valid: true},
		}
		err = queries.AcknowledgeAlert(ctx, ackParams)
		require.NoError(t, err)

		// Resolve alert
		err = queries.ResolveAlert(ctx, alertID)
		require.NoError(t, err)
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

func runTelemetryMigrations(db *sql.DB) error {
	// Read and execute the telemetry migration
	migrationSQL := `
		-- Create telemetry tables (simplified version for testing)
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

		CREATE TABLE IF NOT EXISTS metrics_metadata (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			metric_name TEXT NOT NULL UNIQUE,
			metric_type TEXT NOT NULL,
			description TEXT,
			unit TEXT,
			component TEXT,
			enabled BOOLEAN DEFAULT TRUE,
			retention_days INTEGER DEFAULT 30,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
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
	`

	_, err := db.Exec(migrationSQL)
	return err
}
