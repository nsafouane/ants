package telemetry

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/pubsub"
)

// MetricsCollector handles collection and storage of Context Engine metrics.
type MetricsCollector struct {
	// Database
	queries *db.Queries
	db      *sql.DB

	// Event integration
	eventBroker *pubsub.Broker[MetricEvent]

	// In-memory metrics storage
	metrics *MetricsStore

	// Configuration
	config *MetricsConfig

	// State management
	running bool
	mu      sync.RWMutex
}

// MetricsStore holds current metric values in memory for fast access.
type MetricsStore struct {
	// Tier 1 Analysis Metrics
	Tier1AnalysisTime *TimingMetric  `json:"tier_1_analysis_time"`
	Tier1Success      *CounterMetric `json:"tier_1_success_count"`
	Tier1Failures     *CounterMetric `json:"tier_1_failure_count"`

	// AI Worker Query Metrics
	AIWorkerLatency  *TimingMetric  `json:"ai_worker_query_latency"`
	AIWorkerRequests *CounterMetric `json:"ai_worker_requests"`
	AIWorkerErrors   *CounterMetric `json:"ai_worker_errors"`

	// Knowledge Graph Metrics
	KnowledgeGraphSize *GaugeMetric `json:"knowledge_graph_size"`
	NodesCount         *GaugeMetric `json:"nodes_count"`
	DependenciesCount  *GaugeMetric `json:"dependencies_count"`

	// Vector Database Metrics
	VectorDBIndexStatus *StatusMetric `json:"vector_db_index_status"`
	VectorDBLatency     *TimingMetric `json:"vector_db_latency"`
	EmbeddingsCount     *GaugeMetric  `json:"embeddings_count"`

	// Cache Metrics
	CacheHitRate   *RateMetric    `json:"cache_hit_rate"`
	CacheSize      *GaugeMetric   `json:"cache_size"`
	CacheEvictions *CounterMetric `json:"cache_evictions"`

	// System Resource Metrics
	MemoryUsage   *GaugeMetric `json:"memory_usage"`
	CPUUsage      *GaugeMetric `json:"cpu_usage"`
	DiskUsage     *GaugeMetric `json:"disk_usage"`
	ActiveWorkers *GaugeMetric `json:"active_workers"`

	// Search Performance Metrics
	SearchLatency      *TimingMetric `json:"search_latency"`
	SearchAccuracy     *RateMetric   `json:"search_accuracy"`
	IndexingThroughput *RateMetric   `json:"indexing_throughput"`

	mu sync.RWMutex
}

// MetricsConfig configures metrics collection behavior.
type MetricsConfig struct {
	// Collection settings
	CollectionInterval time.Duration `json:"collection_interval"` // How often to collect metrics
	RetentionPeriod    time.Duration `json:"retention_period"`    // How long to keep metrics
	BatchSize          int           `json:"batch_size"`          // Batch size for database writes

	// Storage settings
	EnableDatabase bool `json:"enable_database"` // Store metrics in database
	EnableMemory   bool `json:"enable_memory"`   // Keep metrics in memory
	EnableEvents   bool `json:"enable_events"`   // Publish metric events

	// Performance settings
	BufferSize        int `json:"buffer_size"`          // Event buffer size
	MaxMetricsPerType int `json:"max_metrics_per_type"` // Max metrics to keep per type
}

// Metric types

// TimingMetric tracks duration measurements (e.g., analysis time, query latency).
type TimingMetric struct {
	Name         string        `json:"name"`
	Count        int64         `json:"count"`
	Sum          time.Duration `json:"sum"`
	Min          time.Duration `json:"min"`
	Max          time.Duration `json:"max"`
	Average      time.Duration `json:"average"`
	Percentile95 time.Duration `json:"p95"`
	LastUpdated  time.Time     `json:"last_updated"`

	// Internal tracking
	values []time.Duration
	mu     sync.RWMutex
}

// CounterMetric tracks cumulative counts (e.g., request counts, error counts).
type CounterMetric struct {
	Name        string    `json:"name"`
	Value       int64     `json:"value"`
	LastUpdated time.Time `json:"last_updated"`

	mu sync.RWMutex
}

// GaugeMetric tracks current values that can go up or down (e.g., memory usage, active connections).
type GaugeMetric struct {
	Name        string    `json:"name"`
	Value       float64   `json:"value"`
	Unit        string    `json:"unit"`
	LastUpdated time.Time `json:"last_updated"`

	mu sync.RWMutex
}

// RateMetric tracks rates and percentages (e.g., cache hit rate, success rate).
type RateMetric struct {
	Name        string    `json:"name"`
	Numerator   int64     `json:"numerator"`
	Denominator int64     `json:"denominator"`
	Rate        float64   `json:"rate"` // Calculated as numerator/denominator
	LastUpdated time.Time `json:"last_updated"`

	mu sync.RWMutex
}

// StatusMetric tracks status information (e.g., index health, service status).
type StatusMetric struct {
	Name        string                 `json:"name"`
	Status      string                 `json:"status"` // "healthy", "degraded", "unhealthy"
	Details     map[string]interface{} `json:"details"`
	LastUpdated time.Time              `json:"last_updated"`

	mu sync.RWMutex
}

// MetricEvent represents a metric update event.
type MetricEvent struct {
	Type      string            `json:"type"`       // "timing", "counter", "gauge", "rate", "status"
	Name      string            `json:"name"`       // Metric name
	Value     interface{}       `json:"value"`      // Metric value
	Unit      string            `json:"unit"`       // Unit of measurement
	Tags      map[string]string `json:"tags"`       // Additional tags
	Timestamp time.Time         `json:"timestamp"`  // When the metric was recorded
	SessionID string            `json:"session_id"` // Associated session
}

// DefaultMetricsConfig returns sensible defaults for metrics collection.
func DefaultMetricsConfig() *MetricsConfig {
	return &MetricsConfig{
		CollectionInterval: 30 * time.Second,
		RetentionPeriod:    24 * time.Hour,
		BatchSize:          100,
		EnableDatabase:     true,
		EnableMemory:       true,
		EnableEvents:       true,
		BufferSize:         1000,
		MaxMetricsPerType:  1000,
	}
}

// NewMetricsCollector creates a new metrics collector.
func NewMetricsCollector(queries *db.Queries, database *sql.DB, eventBroker *pubsub.Broker[MetricEvent], config *MetricsConfig) *MetricsCollector {
	if config == nil {
		config = DefaultMetricsConfig()
	}

	return &MetricsCollector{
		queries:     queries,
		db:          database,
		eventBroker: eventBroker,
		config:      config,
		metrics:     NewMetricsStore(),
	}
}

// NewMetricsStore initializes a new metrics store with all KPI metrics.
func NewMetricsStore() *MetricsStore {
	return &MetricsStore{
		// Tier 1 Analysis Metrics
		Tier1AnalysisTime: &TimingMetric{Name: "tier_1_analysis_time_seconds", values: make([]time.Duration, 0, 1000)},
		Tier1Success:      &CounterMetric{Name: "tier_1_success_count"},
		Tier1Failures:     &CounterMetric{Name: "tier_1_failure_count"},

		// AI Worker Query Metrics
		AIWorkerLatency:  &TimingMetric{Name: "ai_worker_query_latency_ms", values: make([]time.Duration, 0, 1000)},
		AIWorkerRequests: &CounterMetric{Name: "ai_worker_requests_total"},
		AIWorkerErrors:   &CounterMetric{Name: "ai_worker_errors_total"},

		// Knowledge Graph Metrics
		KnowledgeGraphSize: &GaugeMetric{Name: "knowledge_graph_size_mb", Unit: "MB"},
		NodesCount:         &GaugeMetric{Name: "knowledge_graph_nodes_count", Unit: "count"},
		DependenciesCount:  &GaugeMetric{Name: "knowledge_graph_dependencies_count", Unit: "count"},

		// Vector Database Metrics
		VectorDBIndexStatus: &StatusMetric{Name: "vector_db_index_status", Details: make(map[string]interface{})},
		VectorDBLatency:     &TimingMetric{Name: "vector_db_query_latency_ms", values: make([]time.Duration, 0, 1000)},
		EmbeddingsCount:     &GaugeMetric{Name: "vector_db_embeddings_count", Unit: "count"},

		// Cache Metrics
		CacheHitRate:   &RateMetric{Name: "cache_hit_rate_percent"},
		CacheSize:      &GaugeMetric{Name: "cache_size_mb", Unit: "MB"},
		CacheEvictions: &CounterMetric{Name: "cache_evictions_total"},

		// System Resource Metrics
		MemoryUsage:   &GaugeMetric{Name: "memory_usage_mb", Unit: "MB"},
		CPUUsage:      &GaugeMetric{Name: "cpu_usage_percent", Unit: "%"},
		DiskUsage:     &GaugeMetric{Name: "disk_usage_mb", Unit: "MB"},
		ActiveWorkers: &GaugeMetric{Name: "active_workers_count", Unit: "count"},

		// Search Performance Metrics
		SearchLatency:      &TimingMetric{Name: "search_latency_ms", values: make([]time.Duration, 0, 1000)},
		SearchAccuracy:     &RateMetric{Name: "search_accuracy_percent"},
		IndexingThroughput: &RateMetric{Name: "indexing_throughput_docs_per_sec"},
	}
}

// Start begins metrics collection.
func (mc *MetricsCollector) Start(ctx context.Context) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if mc.running {
		return nil
	}

	// Start collection goroutine
	go mc.collectionLoop(ctx)

	mc.running = true
	return nil
}

// Stop gracefully stops metrics collection.
func (mc *MetricsCollector) Stop(ctx context.Context) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if !mc.running {
		return nil
	}

	mc.running = false
	return nil
}

// KPI Metric Recording Methods

// RecordTier1AnalysisTime records the time taken for Tier 1 analysis.
func (mc *MetricsCollector) RecordTier1AnalysisTime(ctx context.Context, duration time.Duration, success bool, sessionID string) {
	mc.metrics.Tier1AnalysisTime.Record(duration)

	if success {
		mc.metrics.Tier1Success.Increment()
	} else {
		mc.metrics.Tier1Failures.Increment()
	}

	// Publish event
	if mc.config.EnableEvents {
		mc.publishMetricEvent("timing", "tier_1_analysis_time_seconds", duration.Seconds(), "seconds", sessionID)
	}
}

// RecordAIWorkerQuery records AI worker query latency.
func (mc *MetricsCollector) RecordAIWorkerQuery(ctx context.Context, latency time.Duration, success bool, sessionID string) {
	mc.metrics.AIWorkerLatency.Record(latency)
	mc.metrics.AIWorkerRequests.Increment()

	if !success {
		mc.metrics.AIWorkerErrors.Increment()
	}

	// Publish event
	if mc.config.EnableEvents {
		mc.publishMetricEvent("timing", "ai_worker_query_latency_ms", float64(latency.Nanoseconds())/1e6, "ms", sessionID)
	}
}

// UpdateKnowledgeGraphSize updates the knowledge graph size metric.
func (mc *MetricsCollector) UpdateKnowledgeGraphSize(ctx context.Context, sizeMB float64, nodesCount, depsCount int64) {
	mc.metrics.KnowledgeGraphSize.Set(sizeMB)
	mc.metrics.NodesCount.Set(float64(nodesCount))
	mc.metrics.DependenciesCount.Set(float64(depsCount))

	// Publish events
	if mc.config.EnableEvents {
		mc.publishMetricEvent("gauge", "knowledge_graph_size_mb", sizeMB, "MB", "")
		mc.publishMetricEvent("gauge", "knowledge_graph_nodes_count", float64(nodesCount), "count", "")
	}
}

// UpdateVectorDBIndexStatus updates vector database index status.
func (mc *MetricsCollector) UpdateVectorDBIndexStatus(ctx context.Context, status string, details map[string]interface{}) {
	mc.metrics.VectorDBIndexStatus.SetStatus(status, details)

	// Publish event
	if mc.config.EnableEvents {
		mc.publishMetricEvent("status", "vector_db_index_status", status, "", "")
	}
}

// RecordVectorDBLatency records vector database query latency.
func (mc *MetricsCollector) RecordVectorDBLatency(ctx context.Context, latency time.Duration, sessionID string) {
	mc.metrics.VectorDBLatency.Record(latency)

	// Publish event
	if mc.config.EnableEvents {
		mc.publishMetricEvent("timing", "vector_db_query_latency_ms", float64(latency.Nanoseconds())/1e6, "ms", sessionID)
	}
}

// UpdateCacheHitRate updates cache hit rate metrics.
func (mc *MetricsCollector) UpdateCacheHitRate(ctx context.Context, hits, total int64, sizeMB float64) {
	mc.metrics.CacheHitRate.Update(hits, total)
	mc.metrics.CacheSize.Set(sizeMB)

	// Publish events
	if mc.config.EnableEvents {
		rate := float64(hits) / float64(total) * 100
		mc.publishMetricEvent("rate", "cache_hit_rate_percent", rate, "%", "")
	}
}

// RecordCacheEviction records cache eviction events.
func (mc *MetricsCollector) RecordCacheEviction(ctx context.Context, count int64) {
	mc.metrics.CacheEvictions.Add(count)

	// Publish event
	if mc.config.EnableEvents {
		mc.publishMetricEvent("counter", "cache_evictions_total", float64(count), "count", "")
	}
}

// UpdateSystemResources updates system resource usage metrics.
func (mc *MetricsCollector) UpdateSystemResources(ctx context.Context, memoryMB, cpuPercent, diskMB float64, activeWorkers int) {
	mc.metrics.MemoryUsage.Set(memoryMB)
	mc.metrics.CPUUsage.Set(cpuPercent)
	mc.metrics.DiskUsage.Set(diskMB)
	mc.metrics.ActiveWorkers.Set(float64(activeWorkers))

	// Publish events
	if mc.config.EnableEvents {
		mc.publishMetricEvent("gauge", "memory_usage_mb", memoryMB, "MB", "")
		mc.publishMetricEvent("gauge", "cpu_usage_percent", cpuPercent, "%", "")
	}
}

// RecordSearchLatency records search operation latency.
func (mc *MetricsCollector) RecordSearchLatency(ctx context.Context, latency time.Duration, sessionID string) {
	mc.metrics.SearchLatency.Record(latency)

	// Publish event
	if mc.config.EnableEvents {
		mc.publishMetricEvent("timing", "search_latency_ms", float64(latency.Nanoseconds())/1e6, "ms", sessionID)
	}
}

// UpdateSearchAccuracy updates search accuracy metrics.
func (mc *MetricsCollector) UpdateSearchAccuracy(ctx context.Context, relevant, total int64) {
	mc.metrics.SearchAccuracy.Update(relevant, total)

	// Publish event
	if mc.config.EnableEvents {
		accuracy := float64(relevant) / float64(total) * 100
		mc.publishMetricEvent("rate", "search_accuracy_percent", accuracy, "%", "")
	}
}

// UpdateIndexingThroughput updates indexing throughput metrics.
func (mc *MetricsCollector) UpdateIndexingThroughput(ctx context.Context, docsIndexed int64, duration time.Duration) {
	throughput := float64(docsIndexed) / duration.Seconds()
	mc.metrics.IndexingThroughput.Update(docsIndexed, int64(duration.Seconds()))

	// Publish event
	if mc.config.EnableEvents {
		mc.publishMetricEvent("rate", "indexing_throughput_docs_per_sec", throughput, "docs/sec", "")
	}
}

// Metric type implementations

// TimingMetric methods
func (tm *TimingMetric) Record(duration time.Duration) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.Count++
	tm.Sum += duration
	tm.LastUpdated = time.Now()

	if tm.Count == 1 || duration < tm.Min {
		tm.Min = duration
	}
	if tm.Count == 1 || duration > tm.Max {
		tm.Max = duration
	}

	tm.Average = tm.Sum / time.Duration(tm.Count)

	// Store value for percentile calculation
	tm.values = append(tm.values, duration)
	if len(tm.values) > 1000 { // Limit memory usage
		tm.values = tm.values[len(tm.values)-1000:]
	}

	// Calculate 95th percentile
	tm.calculatePercentiles()
}

func (tm *TimingMetric) calculatePercentiles() {
	if len(tm.values) == 0 {
		return
	}

	// Simple 95th percentile calculation
	values := make([]time.Duration, len(tm.values))
	copy(values, tm.values)

	// Sort values
	for i := 0; i < len(values)-1; i++ {
		for j := i + 1; j < len(values); j++ {
			if values[i] > values[j] {
				values[i], values[j] = values[j], values[i]
			}
		}
	}

	p95Index := int(float64(len(values)) * 0.95)
	if p95Index >= len(values) {
		p95Index = len(values) - 1
	}
	tm.Percentile95 = values[p95Index]
}

// CounterMetric methods
func (cm *CounterMetric) Increment() {
	cm.Add(1)
}

func (cm *CounterMetric) Add(value int64) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.Value += value
	cm.LastUpdated = time.Now()
}

// GaugeMetric methods
func (gm *GaugeMetric) Set(value float64) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	gm.Value = value
	gm.LastUpdated = time.Now()
}

// RateMetric methods
func (rm *RateMetric) Update(numerator, denominator int64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.Numerator = numerator
	rm.Denominator = denominator
	if denominator > 0 {
		rm.Rate = float64(numerator) / float64(denominator)
	}
	rm.LastUpdated = time.Now()
}

// StatusMetric methods
func (sm *StatusMetric) SetStatus(status string, details map[string]interface{}) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.Status = status
	sm.Details = details
	sm.LastUpdated = time.Now()
}

// Helper methods

func (mc *MetricsCollector) publishMetricEvent(metricType, name string, value interface{}, unit, sessionID string) {
	event := MetricEvent{
		Type:      metricType,
		Name:      name,
		Value:     value,
		Unit:      unit,
		Timestamp: time.Now(),
		SessionID: sessionID,
	}

	mc.eventBroker.Publish(pubsub.EventType(name), event)
}

func (mc *MetricsCollector) collectionLoop(ctx context.Context) {
	ticker := time.NewTicker(mc.config.CollectionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			mc.collectSystemMetrics(ctx)
		}
	}
}

func (mc *MetricsCollector) collectSystemMetrics(ctx context.Context) {
	// This would collect system metrics like memory, CPU, etc.
	// For now, placeholder implementation
}

// GetMetrics returns a snapshot of current metrics.
func (mc *MetricsCollector) GetMetrics() map[string]interface{} {
	mc.metrics.mu.RLock()
	defer mc.metrics.mu.RUnlock()

	return map[string]interface{}{
		"tier_1_analysis_time":    mc.metrics.Tier1AnalysisTime,
		"ai_worker_query_latency": mc.metrics.AIWorkerLatency,
		"knowledge_graph_size":    mc.metrics.KnowledgeGraphSize,
		"vector_db_index_status":  mc.metrics.VectorDBIndexStatus,
		"cache_hit_rate":          mc.metrics.CacheHitRate,
		"search_latency":          mc.metrics.SearchLatency,
		"indexing_throughput":     mc.metrics.IndexingThroughput,
	}
}

// HealthCheck verifies metrics collector health.
func (mc *MetricsCollector) HealthCheck(ctx context.Context) error {
	mc.mu.RLock()
	running := mc.running
	mc.mu.RUnlock()

	if !running {
		return nil // Not an error if not started
	}

	// Check database connectivity if enabled
	if mc.config.EnableDatabase {
		if err := mc.db.PingContext(ctx); err != nil {
			return err
		}
	}

	return nil
}
