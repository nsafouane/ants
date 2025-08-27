package telemetry

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"time"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/pubsub"
)

// CollectorService implements metrics collection for all Context Engine components.
type CollectorService struct {
	// Core components
	metricsCollector *MetricsCollector
	queries          *db.Queries
	database         *sql.DB

	// Event integration
	eventBroker *pubsub.Broker[MetricEvent]

	// Component integrations
	analysisMetrics *AnalysisMetrics
	vectorMetrics   *VectorMetrics
	searchMetrics   *SearchMetrics
	cacheMetrics    *CacheMetrics
	systemMetrics   *SystemMetrics

	// Configuration
	config *CollectorConfig

	// State management
	running bool
	mu      sync.RWMutex
}

// CollectorConfig configures the metrics collection service.
type CollectorConfig struct {
	// Collection intervals
	SystemMetricsInterval   time.Duration `json:"system_metrics_interval"`
	CacheMetricsInterval    time.Duration `json:"cache_metrics_interval"`
	DatabaseCleanupInterval time.Duration `json:"database_cleanup_interval"`

	// Performance settings
	MaxConcurrentMetrics int `json:"max_concurrent_metrics"`
	MetricBufferSize     int `json:"metric_buffer_size"`
	BatchProcessingSize  int `json:"batch_processing_size"`

	// Component settings
	CollectSystemMetrics   bool `json:"collect_system_metrics"`
	CollectAnalysisMetrics bool `json:"collect_analysis_metrics"`
	CollectVectorMetrics   bool `json:"collect_vector_metrics"`
	CollectSearchMetrics   bool `json:"collect_search_metrics"`
	CollectCacheMetrics    bool `json:"collect_cache_metrics"`

	// Alert settings
	EnablePerformanceAlerts  bool    `json:"enable_performance_alerts"`
	AlertThresholdMultiplier float64 `json:"alert_threshold_multiplier"`
}

// Component-specific metric collectors

// AnalysisMetrics collects metrics from the analysis engine.
type AnalysisMetrics struct {
	// Tier 1 Analysis tracking
	analysisStartTimes map[string]time.Time
	analysisResults    map[string]bool
	mu                 sync.RWMutex
}

// VectorMetrics collects metrics from vector database operations.
type VectorMetrics struct {
	// Vector operation tracking
	embeddingStartTimes map[string]time.Time
	queryStartTimes     map[string]time.Time
	indexingStats       map[string]int64
	mu                  sync.RWMutex
}

// SearchMetrics collects metrics from search operations.
type SearchMetrics struct {
	// Search operation tracking
	searchStartTimes   map[string]time.Time
	indexingStartTimes map[string]time.Time
	accuracyResults    map[string]SearchAccuracyResult
	mu                 sync.RWMutex
}

// CacheMetrics collects metrics from cache operations.
type CacheMetrics struct {
	// Cache statistics
	hits      int64
	misses    int64
	evictions int64
	sizeBytes int64
	mu        sync.RWMutex
}

// SystemMetrics collects system resource metrics.
type SystemMetrics struct {
	// System resource tracking
	lastCPUTime   time.Time
	lastMemStats  runtime.MemStats
	activeWorkers int64
	mu            sync.RWMutex
}

// SearchAccuracyResult tracks search result accuracy.
type SearchAccuracyResult struct {
	RelevantResults int64
	TotalResults    int64
	Timestamp       time.Time
}

// DefaultCollectorConfig returns sensible defaults for the collector service.
func DefaultCollectorConfig() *CollectorConfig {
	return &CollectorConfig{
		SystemMetricsInterval:    30 * time.Second,
		CacheMetricsInterval:     60 * time.Second,
		DatabaseCleanupInterval:  24 * time.Hour,
		MaxConcurrentMetrics:     10,
		MetricBufferSize:         1000,
		BatchProcessingSize:      50,
		CollectSystemMetrics:     true,
		CollectAnalysisMetrics:   true,
		CollectVectorMetrics:     true,
		CollectSearchMetrics:     true,
		CollectCacheMetrics:      true,
		EnablePerformanceAlerts:  true,
		AlertThresholdMultiplier: 1.2, // 20% over target triggers alert
	}
}

// NewCollectorService creates a new metrics collection service.
func NewCollectorService(queries *db.Queries, database *sql.DB, eventBroker *pubsub.Broker[MetricEvent], config *CollectorConfig) *CollectorService {
	if config == nil {
		config = DefaultCollectorConfig()
	}

	metricsConfig := DefaultMetricsConfig()
	metricsCollector := NewMetricsCollector(queries, database, eventBroker, metricsConfig)

	return &CollectorService{
		metricsCollector: metricsCollector,
		queries:          queries,
		database:         database,
		eventBroker:      eventBroker,
		config:           config,

		// Initialize component metrics
		analysisMetrics: &AnalysisMetrics{
			analysisStartTimes: make(map[string]time.Time),
			analysisResults:    make(map[string]bool),
		},
		vectorMetrics: &VectorMetrics{
			embeddingStartTimes: make(map[string]time.Time),
			queryStartTimes:     make(map[string]time.Time),
			indexingStats:       make(map[string]int64),
		},
		searchMetrics: &SearchMetrics{
			searchStartTimes:   make(map[string]time.Time),
			indexingStartTimes: make(map[string]time.Time),
			accuracyResults:    make(map[string]SearchAccuracyResult),
		},
		cacheMetrics: &CacheMetrics{},
		systemMetrics: &SystemMetrics{
			lastCPUTime: time.Now(),
		},
	}
}

// Start begins metrics collection for all components.
func (cs *CollectorService) Start(ctx context.Context) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.running {
		return nil
	}

	// Start the underlying metrics collector
	if err := cs.metricsCollector.Start(ctx); err != nil {
		return fmt.Errorf("failed to start metrics collector: %w", err)
	}

	// Start component-specific collection goroutines
	if cs.config.CollectSystemMetrics {
		go cs.systemMetricsLoop(ctx)
	}

	if cs.config.CollectCacheMetrics {
		go cs.cacheMetricsLoop(ctx)
	}

	// Start database cleanup routine
	go cs.databaseCleanupLoop(ctx)

	// Start performance monitoring
	if cs.config.EnablePerformanceAlerts {
		go cs.performanceMonitoringLoop(ctx)
	}

	cs.running = true
	slog.Info("Metrics collection service started")

	return nil
}

// Stop gracefully stops metrics collection.
func (cs *CollectorService) Stop(ctx context.Context) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if !cs.running {
		return nil
	}

	// Stop the underlying metrics collector
	if err := cs.metricsCollector.Stop(ctx); err != nil {
		slog.Warn("Error stopping metrics collector", "error", err)
	}

	cs.running = false
	slog.Info("Metrics collection service stopped")

	return nil
}

// KPI Metric Collection Methods

// RecordTier1AnalysisStart records the start of a Tier 1 analysis operation.
func (cs *CollectorService) RecordTier1AnalysisStart(sessionID string) {
	if !cs.config.CollectAnalysisMetrics {
		return
	}

	cs.analysisMetrics.mu.Lock()
	cs.analysisMetrics.analysisStartTimes[sessionID] = time.Now()
	cs.analysisMetrics.mu.Unlock()
}

// RecordTier1AnalysisComplete records the completion of a Tier 1 analysis operation.
func (cs *CollectorService) RecordTier1AnalysisComplete(ctx context.Context, sessionID string, success bool) {
	if !cs.config.CollectAnalysisMetrics {
		return
	}

	cs.analysisMetrics.mu.Lock()
	startTime, exists := cs.analysisMetrics.analysisStartTimes[sessionID]
	if exists {
		delete(cs.analysisMetrics.analysisStartTimes, sessionID)
		cs.analysisMetrics.analysisResults[sessionID] = success
	}
	cs.analysisMetrics.mu.Unlock()

	if exists {
		duration := time.Since(startTime)
		cs.metricsCollector.RecordTier1AnalysisTime(ctx, duration, success, sessionID)

		// Check performance target
		cs.checkPerformanceTarget(ctx, "tier_1_analysis_time_seconds", duration.Seconds(), "analysis_engine", sessionID)
	}
}

// RecordAIWorkerQueryStart records the start of an AI worker query.
func (cs *CollectorService) RecordAIWorkerQueryStart(queryID string) {
	if !cs.config.CollectAnalysisMetrics {
		return
	}

	cs.analysisMetrics.mu.Lock()
	cs.analysisMetrics.analysisStartTimes[queryID] = time.Now()
	cs.analysisMetrics.mu.Unlock()
}

// RecordAIWorkerQueryComplete records the completion of an AI worker query.
func (cs *CollectorService) RecordAIWorkerQueryComplete(ctx context.Context, queryID, sessionID string, success bool) {
	if !cs.config.CollectAnalysisMetrics {
		return
	}

	cs.analysisMetrics.mu.Lock()
	startTime, exists := cs.analysisMetrics.analysisStartTimes[queryID]
	if exists {
		delete(cs.analysisMetrics.analysisStartTimes, queryID)
	}
	cs.analysisMetrics.mu.Unlock()

	if exists {
		duration := time.Since(startTime)
		cs.metricsCollector.RecordAIWorkerQuery(ctx, duration, success, sessionID)

		// Check performance target
		cs.checkPerformanceTarget(ctx, "ai_worker_query_latency_ms", float64(duration.Nanoseconds())/1e6, "ai_worker", sessionID)
	}
}

// RecordVectorEmbeddingStart records the start of vector embedding generation.
func (cs *CollectorService) RecordVectorEmbeddingStart(operationID string) {
	if !cs.config.CollectVectorMetrics {
		return
	}

	cs.vectorMetrics.mu.Lock()
	cs.vectorMetrics.embeddingStartTimes[operationID] = time.Now()
	cs.vectorMetrics.mu.Unlock()
}

// RecordVectorEmbeddingComplete records the completion of vector embedding generation.
func (cs *CollectorService) RecordVectorEmbeddingComplete(ctx context.Context, operationID, sessionID string) {
	if !cs.config.CollectVectorMetrics {
		return
	}

	cs.vectorMetrics.mu.Lock()
	startTime, exists := cs.vectorMetrics.embeddingStartTimes[operationID]
	if exists {
		delete(cs.vectorMetrics.embeddingStartTimes, operationID)
	}
	cs.vectorMetrics.mu.Unlock()

	if exists {
		duration := time.Since(startTime)
		cs.metricsCollector.RecordVectorDBLatency(ctx, duration, sessionID)
	}
}

// RecordVectorQueryStart records the start of a vector database query.
func (cs *CollectorService) RecordVectorQueryStart(queryID string) {
	if !cs.config.CollectVectorMetrics {
		return
	}

	cs.vectorMetrics.mu.Lock()
	cs.vectorMetrics.queryStartTimes[queryID] = time.Now()
	cs.vectorMetrics.mu.Unlock()
}

// RecordVectorQueryComplete records the completion of a vector database query.
func (cs *CollectorService) RecordVectorQueryComplete(ctx context.Context, queryID, sessionID string) {
	if !cs.config.CollectVectorMetrics {
		return
	}

	cs.vectorMetrics.mu.Lock()
	startTime, exists := cs.vectorMetrics.queryStartTimes[queryID]
	if exists {
		delete(cs.vectorMetrics.queryStartTimes, queryID)
	}
	cs.vectorMetrics.mu.Unlock()

	if exists {
		duration := time.Since(startTime)
		cs.metricsCollector.RecordVectorDBLatency(ctx, duration, sessionID)

		// Check performance target
		cs.checkPerformanceTarget(ctx, "vector_db_query_latency_ms", float64(duration.Nanoseconds())/1e6, "vector_db", sessionID)
	}
}

// RecordSearchStart records the start of a search operation.
func (cs *CollectorService) RecordSearchStart(searchID string) {
	if !cs.config.CollectSearchMetrics {
		return
	}

	cs.searchMetrics.mu.Lock()
	cs.searchMetrics.searchStartTimes[searchID] = time.Now()
	cs.searchMetrics.mu.Unlock()
}

// RecordSearchComplete records the completion of a search operation.
func (cs *CollectorService) RecordSearchComplete(ctx context.Context, searchID, sessionID string, relevantResults, totalResults int64) {
	if !cs.config.CollectSearchMetrics {
		return
	}

	cs.searchMetrics.mu.Lock()
	startTime, exists := cs.searchMetrics.searchStartTimes[searchID]
	if exists {
		delete(cs.searchMetrics.searchStartTimes, searchID)
		cs.searchMetrics.accuracyResults[searchID] = SearchAccuracyResult{
			RelevantResults: relevantResults,
			TotalResults:    totalResults,
			Timestamp:       time.Now(),
		}
	}
	cs.searchMetrics.mu.Unlock()

	if exists {
		duration := time.Since(startTime)
		cs.metricsCollector.RecordSearchLatency(ctx, duration, sessionID)
		cs.metricsCollector.UpdateSearchAccuracy(ctx, relevantResults, totalResults)

		// Check performance target
		cs.checkPerformanceTarget(ctx, "search_latency_ms", float64(duration.Nanoseconds())/1e6, "search", sessionID)
	}
}

// UpdateKnowledgeGraphStats updates knowledge graph size and node counts.
func (cs *CollectorService) UpdateKnowledgeGraphStats(ctx context.Context, sizeMB float64, nodesCount, depsCount int64) {
	cs.metricsCollector.UpdateKnowledgeGraphSize(ctx, sizeMB, nodesCount, depsCount)
}

// UpdateVectorDBIndexStatus updates vector database index health.
func (cs *CollectorService) UpdateVectorDBIndexStatus(ctx context.Context, status string, details map[string]interface{}) {
	cs.metricsCollector.UpdateVectorDBIndexStatus(ctx, status, details)
}

// Cache metrics methods

// RecordCacheHit records a cache hit.
func (cs *CollectorService) RecordCacheHit(ctx context.Context) {
	if !cs.config.CollectCacheMetrics {
		return
	}

	cs.cacheMetrics.mu.Lock()
	cs.cacheMetrics.hits++
	cs.cacheMetrics.mu.Unlock()
}

// RecordCacheMiss records a cache miss.
func (cs *CollectorService) RecordCacheMiss(ctx context.Context) {
	if !cs.config.CollectCacheMetrics {
		return
	}

	cs.cacheMetrics.mu.Lock()
	cs.cacheMetrics.misses++
	cs.cacheMetrics.mu.Unlock()
}

// RecordCacheEviction records cache evictions.
func (cs *CollectorService) RecordCacheEviction(ctx context.Context, count int64) {
	if !cs.config.CollectCacheMetrics {
		return
	}

	cs.cacheMetrics.mu.Lock()
	cs.cacheMetrics.evictions += count
	cs.cacheMetrics.mu.Unlock()

	cs.metricsCollector.RecordCacheEviction(ctx, count)
}

// UpdateCacheSize updates current cache size.
func (cs *CollectorService) UpdateCacheSize(ctx context.Context, sizeBytes int64) {
	if !cs.config.CollectCacheMetrics {
		return
	}

	sizeMB := float64(sizeBytes) / 1024 / 1024

	cs.cacheMetrics.mu.Lock()
	cs.cacheMetrics.sizeBytes = sizeBytes
	hits := cs.cacheMetrics.hits
	total := cs.cacheMetrics.hits + cs.cacheMetrics.misses
	cs.cacheMetrics.mu.Unlock()

	if total > 0 {
		cs.metricsCollector.UpdateCacheHitRate(ctx, hits, total, sizeMB)
	}
}

// Performance monitoring

// checkPerformanceTarget checks if a metric exceeds performance targets.
func (cs *CollectorService) checkPerformanceTarget(ctx context.Context, metricName string, value float64, component, sessionID string) {
	if !cs.config.EnablePerformanceAlerts {
		return
	}

	// Get performance benchmark
	benchmark, err := cs.queries.GetPerformanceBenchmark(ctx, metricName)
	if err != nil {
		return // No benchmark defined
	}

	var alertType, severity, message string
	var thresholdValue float64

	// Check against thresholds
	if value > benchmark.ThresholdCritical.Float64 {
		alertType = "performance_degraded"
		severity = "critical"
		thresholdValue = benchmark.ThresholdCritical.Float64
		message = fmt.Sprintf("Critical performance degradation: %s = %.2f (critical threshold: %.2f)", metricName, value, thresholdValue)
	} else if value > benchmark.ThresholdWarning.Float64 {
		alertType = "threshold_exceeded"
		severity = "high"
		thresholdValue = benchmark.ThresholdWarning.Float64
		message = fmt.Sprintf("Performance threshold exceeded: %s = %.2f (warning threshold: %.2f)", metricName, value, thresholdValue)
	} else if value > benchmark.TargetValue*cs.config.AlertThresholdMultiplier {
		alertType = "threshold_exceeded"
		severity = "medium"
		thresholdValue = benchmark.TargetValue * cs.config.AlertThresholdMultiplier
		message = fmt.Sprintf("Performance target exceeded: %s = %.2f (target: %.2f)", metricName, value, benchmark.TargetValue)
	} else {
		return // Within acceptable range
	}

	// Insert performance alert
	params := db.InsertPerformanceAlertParams{
		AlertType:      alertType,
		MetricName:     metricName,
		CurrentValue:   sql.NullFloat64{Float64: value, Valid: true},
		ThresholdValue: sql.NullFloat64{Float64: thresholdValue, Valid: true},
		Severity:       severity,
		Message:        message,
		Component:      sql.NullString{String: component, Valid: true},
		SessionID:      sql.NullString{String: sessionID, Valid: true},
	}

	alertID, err := cs.queries.InsertPerformanceAlert(ctx, params)
	if err != nil {
		slog.Warn("Failed to insert performance alert", "error", err)
		return
	}

	slog.Warn("Performance alert triggered",
		"alert_id", alertID,
		"metric", metricName,
		"value", value,
		"threshold", thresholdValue,
		"severity", severity,
		"component", component)
}

// System metrics collection loops

// systemMetricsLoop collects system resource metrics periodically.
func (cs *CollectorService) systemMetricsLoop(ctx context.Context) {
	ticker := time.NewTicker(cs.config.SystemMetricsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cs.collectSystemMetrics(ctx)
		}
	}
}

// collectSystemMetrics gathers current system resource usage.
func (cs *CollectorService) collectSystemMetrics(ctx context.Context) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Memory usage in MB
	memoryMB := float64(memStats.Sys) / 1024 / 1024

	// CPU usage estimation (simplified)
	// In a real implementation, you'd use a proper CPU monitoring library
	cpuPercent := 0.0 // Placeholder

	// Active goroutines as a proxy for active workers
	activeWorkers := runtime.NumGoroutine()

	// Update system metrics
	cs.metricsCollector.UpdateSystemResources(ctx, memoryMB, cpuPercent, 0, activeWorkers)
}

// cacheMetricsLoop periodically updates cache metrics.
func (cs *CollectorService) cacheMetricsLoop(ctx context.Context) {
	ticker := time.NewTicker(cs.config.CacheMetricsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cs.updateCacheMetrics(ctx)
		}
	}
}

// updateCacheMetrics updates cache hit rate and other cache metrics.
func (cs *CollectorService) updateCacheMetrics(ctx context.Context) {
	cs.cacheMetrics.mu.RLock()
	hits := cs.cacheMetrics.hits
	misses := cs.cacheMetrics.misses
	sizeBytes := cs.cacheMetrics.sizeBytes
	cs.cacheMetrics.mu.RUnlock()

	total := hits + misses
	if total > 0 {
		sizeMB := float64(sizeBytes) / 1024 / 1024
		cs.metricsCollector.UpdateCacheHitRate(ctx, hits, total, sizeMB)
	}
}

// databaseCleanupLoop periodically cleans old metrics data.
func (cs *CollectorService) databaseCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(cs.config.DatabaseCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cs.cleanupOldMetrics(ctx)
		}
	}
}

// cleanupOldMetrics removes old metrics data based on retention policies.
func (cs *CollectorService) cleanupOldMetrics(ctx context.Context) {
	// Clean metrics older than retention period (e.g., 30 days)
	cutoffTime := time.Now().AddDate(0, 0, -30)

	err := cs.queries.CleanOldMetrics(ctx, cutoffTime)
	if err != nil {
		slog.Warn("Failed to clean old metrics", "error", err)
	} else {
		slog.Debug("Cleaned old metrics", "cutoff_time", cutoffTime)
	}
}

// performanceMonitoringLoop monitors performance and generates alerts.
func (cs *CollectorService) performanceMonitoringLoop(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second) // Check every minute
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cs.checkSystemHealth(ctx)
		}
	}
}

// checkSystemHealth performs overall system health checks.
func (cs *CollectorService) checkSystemHealth(ctx context.Context) {
	// Check database connectivity
	if err := cs.database.PingContext(ctx); err != nil {
		cs.updateComponentHealth(ctx, "database", "unhealthy", map[string]interface{}{
			"error": err.Error(),
		})
	} else {
		cs.updateComponentHealth(ctx, "database", "healthy", nil)
	}

	// Check metrics collector health
	if err := cs.metricsCollector.HealthCheck(ctx); err != nil {
		cs.updateComponentHealth(ctx, "metrics_collector", "unhealthy", map[string]interface{}{
			"error": err.Error(),
		})
	} else {
		cs.updateComponentHealth(ctx, "metrics_collector", "healthy", nil)
	}
}

// updateComponentHealth updates component health status.
func (cs *CollectorService) updateComponentHealth(ctx context.Context, component, status string, details map[string]interface{}) {
	detailsJSON := ""
	if details != nil {
		// In a real implementation, you'd marshal details to JSON
		detailsJSON = fmt.Sprintf("%v", details)
	}

	params := db.UpdateSystemHealthStatusParams{
		Component: component,
		Status:    status,
		Details:   sql.NullString{String: detailsJSON, Valid: detailsJSON != ""},
		LastCheck: time.Now(),
	}

	err := cs.queries.UpdateSystemHealthStatus(ctx, params)
	if err != nil {
		slog.Warn("Failed to update component health", "component", component, "error", err)
	}
}

// GetCollectorMetrics returns current metrics from the collector.
func (cs *CollectorService) GetCollectorMetrics() map[string]interface{} {
	return cs.metricsCollector.GetMetrics()
}

// HealthCheck verifies the collector service health.
func (cs *CollectorService) HealthCheck(ctx context.Context) error {
	cs.mu.RLock()
	running := cs.running
	cs.mu.RUnlock()

	if !running {
		return fmt.Errorf("collector service not running")
	}

	return cs.metricsCollector.HealthCheck(ctx)
}
