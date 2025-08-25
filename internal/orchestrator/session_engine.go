package orchestrator

import (
	"context"
	"sync"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/pubsub"
)

// SessionContextEngine is a session-scoped Context Engine service that manages
// analysis tasks and knowledge base queries for a specific user session.
type SessionContextEngine struct {
	sessionID string
	db        db.Querier
	config    *config.ContextEngineConfig

	// Event coordination
	eventBus *pubsub.Broker[ContextEvent]

	// Analysis state
	analysisLocks sync.Map // map[string]*AnalysisLock

	// Context for this session engine
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// ContextEvent represents events within the Context Engine
type ContextEvent struct {
	Type      ContextEventType
	SessionID string
	FilePath  string
	NodeID    string
	Metadata  map[string]interface{}
	Timestamp time.Time
}

type ContextEventType int

const (
	AnalysisStarted ContextEventType = iota
	AnalysisCompleted
	AnalysisFailed
	NodeCreated
	NodeUpdated
	DependencyAdded
	CacheHit
	CacheMiss
)

// AnalysisLock represents a lock on analysis for specific files
type AnalysisLock struct {
	Files      []string
	CommitHash string
	LockedAt   time.Time
	LockedBy   string
}

// NewSessionContextEngine creates a new session-scoped Context Engine
func NewSessionContextEngine(sessionID string, db db.Querier, config *config.ContextEngineConfig) *SessionContextEngine {
	ctx, cancel := context.WithCancel(context.Background())

	return &SessionContextEngine{
		sessionID: sessionID,
		db:        db,
		config:    config,
		eventBus:  pubsub.NewBroker[ContextEvent](),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start begins the session-scoped Context Engine
func (s *SessionContextEngine) Start() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		// Session-specific background tasks
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				// Periodic maintenance tasks for this session
				s.performMaintenance()
			}
		}
	}()
}

// Stop gracefully shuts down the session Context Engine
func (s *SessionContextEngine) Stop() {
	s.cancel()
	s.wg.Wait()
}

// Subscribe returns a channel for Context Engine events for this session
func (s *SessionContextEngine) Subscribe(ctx context.Context) <-chan pubsub.Event[ContextEvent] {
	return s.eventBus.Subscribe(ctx)
}

// RequestAnalysisLock attempts to acquire a lock for analyzing specific files
func (s *SessionContextEngine) RequestAnalysisLock(files []string, commitHash string) (*AnalysisLock, error) {
	lockKey := s.generateLockKey(files, commitHash)

	// Check if already locked
	if existing, exists := s.analysisLocks.Load(lockKey); exists {
		return existing.(*AnalysisLock), nil
	}

	// Create new lock
	lock := &AnalysisLock{
		Files:      files,
		CommitHash: commitHash,
		LockedAt:   time.Now(),
		LockedBy:   s.sessionID,
	}

	s.analysisLocks.Store(lockKey, lock)

	// Publish event
	s.eventBus.Publish(pubsub.EventType("analysis_started"), ContextEvent{
		Type:      AnalysisStarted,
		SessionID: s.sessionID,
		FilePath:  files[0], // Primary file
		Metadata: map[string]interface{}{
			"files_count": len(files),
			"commit_hash": commitHash,
		},
		Timestamp: time.Now(),
	})

	return lock, nil
}

// ReleaseLock releases an analysis lock
func (s *SessionContextEngine) ReleaseLock(lock *AnalysisLock) {
	lockKey := s.generateLockKey(lock.Files, lock.CommitHash)
	s.analysisLocks.Delete(lockKey)

	// Publish completion event
	s.eventBus.Publish(pubsub.EventType("analysis_completed"), ContextEvent{
		Type:      AnalysisCompleted,
		SessionID: s.sessionID,
		FilePath:  lock.Files[0],
		Metadata: map[string]interface{}{
			"duration_seconds": time.Since(lock.LockedAt).Seconds(),
		},
		Timestamp: time.Now(),
	})
}

// GetSessionNodes retrieves all code nodes for this session
func (s *SessionContextEngine) GetSessionNodes(ctx context.Context) ([]db.CodeNode, error) {
	// Use SQLC generated query to get nodes for this session
	return s.db.ListCodeNodesBySession(ctx, s.sessionID)
}

// GetKnowledgeStats returns statistics about the knowledge base for this session
func (s *SessionContextEngine) GetKnowledgeStats(ctx context.Context) (*KnowledgeStats, error) {
	nodes, err := s.GetSessionNodes(ctx)
	if err != nil {
		return nil, err
	}

	stats := &KnowledgeStats{
		SessionID:   s.sessionID,
		TotalNodes:  len(nodes),
		LastUpdated: time.Now(),
	}

	// Count by type
	stats.ByType = make(map[string]int)
	for _, node := range nodes {
		if node.Kind.Valid {
			stats.ByType[node.Kind.String]++
		}
	}

	return stats, nil
}

// performMaintenance runs periodic maintenance tasks
func (s *SessionContextEngine) performMaintenance() {
	// Clean up stale analysis locks (older than 1 hour)
	cutoff := time.Now().Add(-1 * time.Hour)

	s.analysisLocks.Range(func(key, value interface{}) bool {
		lock := value.(*AnalysisLock)
		if lock.LockedAt.Before(cutoff) {
			s.analysisLocks.Delete(key)
		}
		return true
	})
}

// generateLockKey creates a consistent key for analysis locks
func (s *SessionContextEngine) generateLockKey(files []string, commitHash string) string {
	// Simple key generation - in production might want something more sophisticated
	key := s.sessionID + ":" + commitHash
	for _, file := range files {
		key += ":" + file
	}
	return key
}

// KnowledgeStats holds statistics about the knowledge base for a session
type KnowledgeStats struct {
	SessionID   string
	TotalNodes  int
	ByType      map[string]int
	LastUpdated time.Time
}
