package pubsub

import (
	"context"
	"time"
)

const (
	// Generic lifecycle events
	CreatedEvent EventType = "created"
	UpdatedEvent EventType = "updated"
	DeletedEvent EventType = "deleted"

	// Context Engine Analysis Pipeline Events
	Tier1AnalysisStartedEvent   EventType = "tier1_analysis_started"
	Tier1AnalysisCompletedEvent EventType = "tier1_analysis_completed"
	Tier1AnalysisFailedEvent    EventType = "tier1_analysis_failed"

	Tier2AnalysisStartedEvent   EventType = "tier2_analysis_started"
	Tier2AnalysisCompletedEvent EventType = "tier2_analysis_completed"
	Tier2AnalysisFailedEvent    EventType = "tier2_analysis_failed"

	Tier3AnalysisStartedEvent   EventType = "tier3_analysis_started"
	Tier3AnalysisCompletedEvent EventType = "tier3_analysis_completed"
	Tier3AnalysisFailedEvent    EventType = "tier3_analysis_failed"

	// Knowledge Graph Events
	CodeNodeCreatedEvent   EventType = "code_node_created"
	CodeNodeUpdatedEvent   EventType = "code_node_updated"
	CodeNodeDeletedEvent   EventType = "code_node_deleted"
	DependencyAddedEvent   EventType = "dependency_added"
	DependencyRemovedEvent EventType = "dependency_removed"
	EmbeddingCreatedEvent  EventType = "embedding_created"
	EmbeddingUpdatedEvent  EventType = "embedding_updated"

	// Resource Management Events
	WorkerPoolSaturatedEvent    EventType = "worker_pool_saturated"
	MemoryThresholdEvent        EventType = "memory_threshold_exceeded"
	ResourceConstraintEvent     EventType = "resource_constraint_detected"
	OperationalModeChangedEvent EventType = "operational_mode_changed"

	// File System Events
	FileDeletedEvent EventType = "file_deleted"
	FileRenamedEvent EventType = "file_renamed"
	FileChangedEvent EventType = "file_changed"

	// Traversal Events
	TraversalStartedEvent   EventType = "traversal_started"
	TraversalCompletedEvent EventType = "traversal_completed"
	TraversalFailedEvent    EventType = "traversal_failed"

	// Git Analysis Events
	GitAnalysisStartedEvent   EventType = "git_analysis_started"
	GitAnalysisCompletedEvent EventType = "git_analysis_completed"
	GitAnalysisFailedEvent    EventType = "git_analysis_failed"

	// Cache and Maintenance Events
	CacheInvalidatedEvent EventType = "cache_invalidated"
	CleanupStartedEvent   EventType = "cleanup_started"
	CleanupCompletedEvent EventType = "cleanup_completed"
)

type Suscriber[T any] interface {
	Subscribe(context.Context) <-chan Event[T]
}

type (
	// EventType identifies the type of event
	EventType string

	// Event represents an event in the lifecycle of a resource
	Event[T any] struct {
		Type    EventType
		Payload T
	}

	Publisher[T any] interface {
		Publish(EventType, T)
	}
)

// Context Engine Event Payloads

// AnalysisEvent represents an analysis pipeline event
type AnalysisEvent struct {
	SessionID     string                 `json:"session_id"`
	NodeID        int64                  `json:"node_id,omitempty"`
	Path          string                 `json:"path"`
	Tier          int                    `json:"tier"`
	WorkerID      string                 `json:"worker_id,omitempty"`
	StartTime     time.Time              `json:"start_time"`
	EndTime       time.Time              `json:"end_time,omitempty"`
	Duration      time.Duration          `json:"duration,omitempty"`
	Error         string                 `json:"error,omitempty"`
	CorrelationID string                 `json:"correlation_id"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// CodeNodeEvent represents knowledge graph node events
type CodeNodeEvent struct {
	NodeID     int64                  `json:"node_id"`
	SessionID  string                 `json:"session_id"`
	Path       string                 `json:"path"`
	Symbol     string                 `json:"symbol,omitempty"`
	Kind       string                 `json:"kind,omitempty"`
	Language   string                 `json:"language,omitempty"`
	Timestamp  time.Time              `json:"timestamp"`
	ChangeType string                 `json:"change_type"` // "created", "updated", "deleted"
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// DependencyEvent represents dependency relationship events
type DependencyEvent struct {
	DependencyID int64                  `json:"dependency_id"`
	FromNodeID   int64                  `json:"from_node_id"`
	ToNodeID     int64                  `json:"to_node_id"`
	Relation     string                 `json:"relation"`
	Timestamp    time.Time              `json:"timestamp"`
	ChangeType   string                 `json:"change_type"` // "added", "removed"
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// EmbeddingEvent represents vector embedding events
type EmbeddingEvent struct {
	EmbeddingID int64                  `json:"embedding_id"`
	NodeID      int64                  `json:"node_id"`
	SessionID   string                 `json:"session_id"`
	VectorID    string                 `json:"vector_id,omitempty"`
	Dimensions  int                    `json:"dimensions"`
	Timestamp   time.Time              `json:"timestamp"`
	ChangeType  string                 `json:"change_type"` // "created", "updated"
	Model       string                 `json:"model,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// ResourceEvent represents resource management events
type ResourceEvent struct {
	EventType      string                 `json:"event_type"`
	Timestamp      time.Time              `json:"timestamp"`
	ResourceType   string                 `json:"resource_type"` // "memory", "cpu", "workers"
	CurrentValue   float64                `json:"current_value"`
	Threshold      float64                `json:"threshold,omitempty"`
	Severity       string                 `json:"severity"` // "low", "medium", "high", "critical"
	Component      string                 `json:"component,omitempty"`
	SessionID      string                 `json:"session_id,omitempty"`
	Recommendation string                 `json:"recommendation,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// FileSystemEvent represents file system change events
type FileSystemEvent struct {
	Path          string                 `json:"path"`
	OldPath       string                 `json:"old_path,omitempty"` // For rename events
	EventType     string                 `json:"event_type"`         // "deleted", "renamed", "changed"
	Timestamp     time.Time              `json:"timestamp"`
	SessionID     string                 `json:"session_id,omitempty"`
	AffectedNodes []int64                `json:"affected_nodes,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// CacheEvent represents cache lifecycle events
type CacheEvent struct {
	CacheType     string                 `json:"cache_type"` // "ast", "analysis", "embeddings"
	Action        string                 `json:"action"`     // "invalidated", "cleanup_started", "cleanup_completed"
	Timestamp     time.Time              `json:"timestamp"`
	SessionID     string                 `json:"session_id,omitempty"`
	ItemsAffected int                    `json:"items_affected,omitempty"`
	Reason        string                 `json:"reason,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// OperationalModeEvent represents operational mode changes
type OperationalModeEvent struct {
	OldMode     string                 `json:"old_mode"`
	NewMode     string                 `json:"new_mode"`
	Timestamp   time.Time              `json:"timestamp"`
	Reason      string                 `json:"reason,omitempty"`
	TriggeredBy string                 `json:"triggered_by"` // "user", "system", "resource_constraint"
	SessionID   string                 `json:"session_id,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}
