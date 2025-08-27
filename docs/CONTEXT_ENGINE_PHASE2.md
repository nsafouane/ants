# Context Engine Phase 2: Advanced Features

This document provides comprehensive documentation for Phase 2 Context Engine features including Vector Database integration, Full-Text Search (FTS5), and Built-in Telemetry & Monitoring.

## Overview

Phase 2 extends the Context Engine with advanced capabilities for enhanced code understanding, semantic search, and performance monitoring:

- **Vector Database Integration**: Qdrant-based semantic similarity search with Ollama embeddings
- **SQLite FTS5 Full-Text Search**: Fast text search with intelligent tokenization
- **Built-in Telemetry & Monitoring**: Real-time performance tracking and alerting

## Features

### 2.1 Vector Database Integration

#### Key Components
- **Embedding Generation**: Uses Ollama LLM provider for generating code embeddings
- **Qdrant Integration**: Vector storage and similarity search
- **Hybrid Bridge**: Connects vector operations with SQL database for metadata

#### Configuration

```json
{
  "context_engine": {
    "vector_db": {
      "type": "qdrant",
      "url": "http://localhost",
      "port": 6334,
      "collection": "code_embeddings",
      "timeout": "30s",
      "batch_size": 100,
      "recreate_collection": false
    },
    "embedding": {
      "provider": "ollama",
      "model": "codellama:latest",
      "base_url": "http://localhost:11434",
      "timeout": "60s",
      "max_retries": 3
    }
  }
}
```

#### Usage Example

```go
package main

import (
    "context"
    "github.com/charmbracelet/crush/internal/vector"
    "github.com/charmbracelet/crush/internal/config"
)

func main() {
    // Initialize vector service
    cfg := config.LoadVectorDBConfig()
    vectorService, err := vector.NewVectorService(cfg)
    if err != nil {
        log.Fatal(err)
    }

    // Generate and store embeddings
    ctx := context.Background()
    sessionID := "example-session"
    
    codeContent := `func ProcessData(input []byte) error {
        // Validate input
        if len(input) == 0 {
            return errors.New("empty input")
        }
        // Process data
        return nil
    }`

    metadata := &vector.EmbeddingMetadata{
        Language:   "go",
        Symbol:     "ProcessData",
        Kind:       "function",
        FilePath:   "/src/processor.go",
        SessionID:  sessionID,
    }

    // Store code embedding
    nodeID := int64(123)
    err = vectorService.StoreCodeEmbedding(ctx, nodeID, codeContent, metadata)
    if err != nil {
        log.Fatal(err)
    }

    // Search for similar code
    query := "validate input data"
    results, err := vectorService.SearchSimilar(ctx, query, sessionID, 10)
    if err != nil {
        log.Fatal(err)
    }

    for _, result := range results {
        fmt.Printf("Found similar code: %s (score: %.3f)\n", 
            result.Symbol, result.Score)
    }
}
```

### 2.2 SQLite FTS5 Full-Text Search

#### Key Components
- **Multi-language Support**: Tokenization for Go, Python, JavaScript, Java, SQL
- **Content Types**: Code content, file content, documentation
- **Search Optimization**: Ranking algorithms and query enhancement

#### Configuration

```json
{
  "context_engine": {
    "fts5": {
      "enabled": true,
      "rebuild_on_startup": false,
      "auto_optimize": true,
      "optimization_interval": "1h",
      "index_options": {
        "extract_comments": true,
        "extract_identifiers": true,
        "process_docstrings": true,
        "batch_size": 100
      }
    }
  }
}
```

#### Usage Example

```go
package main

import (
    "context"
    "github.com/charmbracelet/crush/internal/search"
)

func main() {
    // Initialize FTS service
    ftsService := search.NewFTSService(database)
    
    ctx := context.Background()
    sessionID := "example-session"

    // Index code content
    err := ftsService.IndexCodeContent(
        ctx,
        nodeID,
        sessionID,
        "/src/auth.go",
        "go",
        "function",
        "AuthenticateUser",
        codeContent,
        search.DefaultIndexingOptions(),
    )

    // Perform search
    options := &search.SearchOptions{
        Query:       "authentication user login",
        SessionID:   sessionID,
        Language:    &language,
        Limit:       20,
        IncludeCode: true,
        IncludeFile: true,
        IncludeDocs: true,
    }

    results, err := ftsService.Search(ctx, options)
    if err != nil {
        log.Fatal(err)
    }

    for _, result := range results {
        fmt.Printf("Found: %s (%s) - Score: %.3f\n", 
            *result.Symbol, result.Type, result.Score)
    }
}
```

### 2.3 Built-in Telemetry & Monitoring

#### Key Components
- **KPI Tracking**: Performance metrics for all Context Engine components
- **Real-time Dashboard**: TUI-based monitoring interface
- **Alerting System**: Configurable performance thresholds and alerts

#### Configuration

```json
{
  "context_engine": {
    "telemetry": {
      "enabled": true,
      "collection_interval": "30s",
      "retention_period": "24h",
      "enable_database_storage": true,
      "enable_performance_alerts": true,
      "components": {
        "system_metrics": true,
        "analysis_metrics": true,
        "vector_metrics": true,
        "search_metrics": true,
        "cache_metrics": true
      },
      "performance_targets": {
        "tier_1_analysis_time_seconds": 5.0,
        "ai_worker_query_latency_ms": 2000.0,
        "search_latency_ms": 200.0,
        "vector_db_query_latency_ms": 100.0,
        "cache_hit_rate_percent": 80.0
      }
    }
  }
}
```

#### Usage Example

```go
package main

import (
    "context"
    "github.com/charmbracelet/crush/internal/telemetry"
    "github.com/charmbracelet/crush/internal/db"
    "github.com/charmbracelet/crush/internal/pubsub"
)

func main() {
    // Initialize telemetry
    queries := db.New(database)
    eventBroker := pubsub.NewBroker[telemetry.MetricEvent]()
    
    config := telemetry.DefaultCollectorConfig()
    collector := telemetry.NewCollectorService(queries, database, eventBroker, config)
    
    ctx := context.Background()
    err := collector.Start(ctx)
    if err != nil {
        log.Fatal(err)
    }
    defer collector.Stop(ctx)

    // Record metrics during operations
    sessionID := "example-session"
    
    // Tier 1 analysis
    collector.RecordTier1AnalysisStart(sessionID)
    // ... perform analysis ...
    collector.RecordTier1AnalysisComplete(ctx, sessionID, true)

    // AI worker query
    queryID := "query-123"
    collector.RecordAIWorkerQueryStart(queryID)
    // ... process AI query ...
    collector.RecordAIWorkerQueryComplete(ctx, queryID, sessionID, true)

    // Vector operations
    operationID := "vector-op-456"
    collector.RecordVectorQueryStart(operationID)
    // ... perform vector search ...
    collector.RecordVectorQueryComplete(ctx, operationID, sessionID)

    // Search operations
    searchID := "search-789"
    collector.RecordSearchStart(searchID)
    // ... perform search ...
    collector.RecordSearchComplete(ctx, searchID, sessionID, 15, 20) // 15 relevant out of 20

    // Cache operations
    collector.RecordCacheHit(ctx)
    collector.RecordCacheMiss(ctx)
    collector.UpdateCacheSize(ctx, 50*1024*1024) // 50MB

    // Get current metrics
    metrics := collector.GetCollectorMetrics()
    fmt.Printf("Current metrics: %+v\n", metrics)
}
```

## Performance Targets

Phase 2 is designed to meet the following performance targets:

| Metric | Target | Warning Threshold | Critical Threshold |
|--------|--------|-------------------|-------------------|
| Tier 1 Analysis Time | < 5s | 8s | 15s |
| AI Worker Query Latency | < 2s | 5s | 10s |
| Search Latency | < 200ms | 500ms | 1000ms |
| Vector DB Query Latency | < 100ms | 300ms | 1000ms |
| Cache Hit Rate | > 80% | 60% | 40% |
| Memory Usage (Performance Mode) | < 512MB | 768MB | 1024MB |
| Indexing Throughput | > 25 docs/sec | 15 docs/sec | 5 docs/sec |

## Operational Modes

### Eco Mode
- **Memory Budget**: 256MB
- **Metrics Collection**: Reduced frequency
- **Vector Operations**: Disabled for memory savings
- **Cache**: Basic caching only

```json
{
  "context_engine": {
    "mode": "eco",
    "telemetry": {
      "collection_interval": "60s",
      "components": {
        "vector_metrics": false,
        "cache_metrics": false
      }
    }
  }
}
```

### Performance Mode
- **Memory Budget**: 512MB
- **Metrics Collection**: High frequency
- **All Features**: Enabled
- **Optimizations**: Maximum performance

```json
{
  "context_engine": {
    "mode": "performance",
    "telemetry": {
      "collection_interval": "10s",
      "components": {
        "system_metrics": true,
        "analysis_metrics": true,
        "vector_metrics": true,
        "search_metrics": true,
        "cache_metrics": true
      }
    }
  }
}
```

## TUI Dashboard

The performance monitoring dashboard provides real-time visualization of Context Engine metrics:

### Features
- **Overview Tab**: System health summary and recent alerts
- **Metrics Tab**: Detailed performance metrics with trends
- **Health Tab**: Component status and health checks
- **Alerts Tab**: Active performance alerts and history

### Navigation
- `Tab` / `Shift+Tab`: Navigate between tabs
- `r`: Manual refresh
- `q`: Close dashboard

### Dashboard Integration

```go
package main

import (
    "github.com/charmbracelet/crush/internal/tui/components/context_engine"
    "github.com/charmbracelet/crush/internal/telemetry"
)

func main() {
    // Create dashboard
    queries := db.New(database)
    dashboard := context_engine.NewPerformanceDashboard(queries)
    
    // Set collector service
    dashboard.SetCollectorService(collectorService)
    
    // Configure update interval
    dashboard.SetUpdateInterval(5 * time.Second)
    
    // The dashboard integrates with the main TUI application
    // and can be toggled with DashboardToggleMsg
}
```

## Database Schema

Phase 2 adds several new tables for telemetry and search:

### Telemetry Tables
- `metrics_data`: Time-series metrics storage
- `metrics_metadata`: Metric definitions and configuration
- `performance_benchmarks`: Performance targets and thresholds
- `system_health`: Component health status
- `performance_alerts`: Active alerts and notifications

### Search Tables (FTS5 Virtual Tables)
- `code_content_fts`: Full-text search for code content
- `file_content_fts`: Full-text search for file content
- `documentation_fts`: Full-text search for documentation

### Migration

Phase 2 database changes are handled automatically through migrations:

```bash
# Migrations are located in internal/db/migrations/
# - 20250827000001_create_fts_tables.sql
# - 20250827000002_create_telemetry_tables.sql
```

## Integration with Existing Systems

### Event System Integration
Phase 2 integrates with the existing pubsub event system:

```go
// Telemetry events
type MetricEvent struct {
    Type      string                 `json:"type"`
    Name      string                 `json:"name"`
    Value     interface{}            `json:"value"`
    Timestamp time.Time              `json:"timestamp"`
    SessionID string                 `json:"session_id"`
}

// Subscribe to metric events
eventBroker.Subscribe("tier_1_analysis_time_seconds", func(event MetricEvent) {
    // Process metric event
    log.Printf("Metric recorded: %s = %v", event.Name, event.Value)
})
```

### Incremental Indexing
Search indexes are updated automatically when content changes:

```go
// Code node events trigger incremental indexing
type CodeNodeEvent struct {
    Type     string  `json:"type"` // "created", "updated", "deleted"
    NodeID   int64   `json:"node_id"`
    SessionID string `json:"session_id"`
}

// File system events trigger file indexing
type FileSystemEvent struct {
    Type      string `json:"type"` // "created", "modified", "deleted"
    Path      string `json:"path"`
    SessionID string `json:"session_id"`
}
```

## Testing

Phase 2 includes comprehensive test suites:

### Unit Tests
```bash
# Run telemetry tests
go test ./internal/telemetry/ -v

# Run search tests
go test ./internal/search/ -v

# Run vector tests
go test ./internal/vector/ -v

# Run TUI dashboard tests
go test ./internal/tui/components/context_engine/ -v
```

### Integration Tests
```bash
# Run Phase 2 integration tests
go test ./internal/search/ -run TestPhase2Integration -v

# Run performance validation
go test ./internal/telemetry/ -run TestPhase2PerformanceValidation -v
```

### Performance Benchmarks
```bash
# Run performance benchmarks
go test ./internal/telemetry/ -run TestPerformanceOverhead -v
go test ./internal/telemetry/ -run TestResourceModeValidation -v
```

## Troubleshooting

### Common Issues

#### Vector Database Connection
```bash
# Check Qdrant is running
curl http://localhost:6334/health

# Check collection status
curl http://localhost:6334/collections/code_embeddings
```

#### FTS5 Not Available
```bash
# Verify SQLite has FTS5 support
sqlite3 ":memory:" "SELECT fts5_version();"
```

#### High Memory Usage
- Switch to Eco mode for memory-constrained environments
- Adjust telemetry collection intervals
- Disable vector operations if not needed

#### Performance Alerts
- Check system health in the TUI dashboard
- Review performance benchmarks
- Verify resource allocation matches operational mode

### Logs and Debugging

Enable debug logging for detailed information:

```json
{
  "context_engine": {
    "debug": true,
    "telemetry": {
      "debug_metrics": true
    },
    "vector_db": {
      "debug": true
    }
  }
}
```

## Migration from Phase 1

Upgrading from Phase 1 to Phase 2:

1. **Database Migration**: Automatic via migration files
2. **Configuration Updates**: Add Phase 2 settings to config
3. **Dependencies**: Ollama for embeddings, optional Qdrant
4. **Memory**: Increase allocation for new features

### Backward Compatibility
Phase 2 maintains full backward compatibility with Phase 1:
- Existing code analysis continues to work
- Session management unchanged  
- Core orchestrator functionality preserved

## Best Practices

### Configuration
- Start with default settings and adjust based on usage
- Monitor performance metrics to optimize configuration
- Use appropriate operational mode for your environment

### Performance
- Enable telemetry to monitor system performance
- Use the dashboard to identify bottlenecks
- Adjust batch sizes based on available resources

### Maintenance
- Regularly clean old metrics data (automatic)
- Monitor index sizes and rebuild if necessary
- Review and adjust performance thresholds

## Conclusion

Phase 2 transforms the Context Engine into a comprehensive code understanding platform with advanced search capabilities and real-time monitoring. The integration of vector databases, full-text search, and telemetry provides powerful tools for code analysis while maintaining performance and reliability.

For additional support or questions, refer to the implementation files in:
- `internal/vector/` - Vector database operations
- `internal/search/` - FTS5 search implementation  
- `internal/telemetry/` - Metrics and monitoring
- `internal/tui/components/context_engine/` - Dashboard interface