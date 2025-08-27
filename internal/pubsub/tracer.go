package pubsub

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// EventTracer provides correlation and tracing for Context Engine events
type EventTracer struct {
	mu              sync.RWMutex
	correlations    map[string]*AnalysisTrace
	broker          *Broker[AnalysisEvent]
	maxTraces       int
	cleanupInterval time.Duration
	done            chan struct{}
}

// AnalysisTrace tracks the lifecycle of an analysis operation
type AnalysisTrace struct {
	CorrelationID string                 `json:"correlation_id"`
	SessionID     string                 `json:"session_id"`
	Path          string                 `json:"path"`
	NodeID        int64                  `json:"node_id,omitempty"`
	StartTime     time.Time              `json:"start_time"`
	EndTime       time.Time              `json:"end_time,omitempty"`
	TotalDuration time.Duration          `json:"total_duration,omitempty"`
	TierResults   map[int]*TierTrace     `json:"tier_results"`
	Status        string                 `json:"status"` // "running", "completed", "failed"
	Error         string                 `json:"error,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// TierTrace tracks individual tier analysis
type TierTrace struct {
	Tier       int                    `json:"tier"`
	StartTime  time.Time              `json:"start_time"`
	EndTime    time.Time              `json:"end_time,omitempty"`
	Duration   time.Duration          `json:"duration,omitempty"`
	WorkerID   string                 `json:"worker_id,omitempty"`
	Status     string                 `json:"status"` // "running", "completed", "failed"
	Error      string                 `json:"error,omitempty"`
	ResultSize int                    `json:"result_size,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// NewEventTracer creates a new event tracer
func NewEventTracer(broker *Broker[AnalysisEvent]) *EventTracer {
	return &EventTracer{
		correlations:    make(map[string]*AnalysisTrace),
		broker:          broker,
		maxTraces:       1000,
		cleanupInterval: 10 * time.Minute,
		done:            make(chan struct{}),
	}
}

// Start begins the event tracer background processing
func (et *EventTracer) Start() {
	go et.cleanupLoop()
}

// Stop stops the event tracer
func (et *EventTracer) Stop() {
	select {
	case <-et.done:
		return
	default:
		close(et.done)
	}
}

// StartAnalysisTrace creates a new analysis trace
func (et *EventTracer) StartAnalysisTrace(sessionID, path string, nodeID int64) string {
	correlationID := uuid.New().String()

	trace := &AnalysisTrace{
		CorrelationID: correlationID,
		SessionID:     sessionID,
		Path:          path,
		NodeID:        nodeID,
		StartTime:     time.Now(),
		TierResults:   make(map[int]*TierTrace),
		Status:        "running",
		Metadata:      make(map[string]interface{}),
	}

	et.mu.Lock()
	et.correlations[correlationID] = trace
	et.mu.Unlock()

	return correlationID
}

// RecordTierStart records the start of a tier analysis
func (et *EventTracer) RecordTierStart(correlationID string, tier int, workerID string) {
	et.mu.Lock()
	defer et.mu.Unlock()

	trace, exists := et.correlations[correlationID]
	if !exists {
		return
	}

	tierTrace := &TierTrace{
		Tier:      tier,
		StartTime: time.Now(),
		WorkerID:  workerID,
		Status:    "running",
		Metadata:  make(map[string]interface{}),
	}

	trace.TierResults[tier] = tierTrace

	// Publish tier started event
	event := AnalysisEvent{
		SessionID:     trace.SessionID,
		NodeID:        trace.NodeID,
		Path:          trace.Path,
		Tier:          tier,
		WorkerID:      workerID,
		StartTime:     tierTrace.StartTime,
		CorrelationID: correlationID,
	}

	switch tier {
	case 1:
		et.broker.Publish(Tier1AnalysisStartedEvent, event)
	case 2:
		et.broker.Publish(Tier2AnalysisStartedEvent, event)
	case 3:
		et.broker.Publish(Tier3AnalysisStartedEvent, event)
	}
}

// RecordTierComplete records the completion of a tier analysis
func (et *EventTracer) RecordTierComplete(correlationID string, tier int, resultSize int, metadata map[string]interface{}) {
	et.mu.Lock()
	defer et.mu.Unlock()

	trace, exists := et.correlations[correlationID]
	if !exists {
		return
	}

	tierTrace, exists := trace.TierResults[tier]
	if !exists {
		return
	}

	tierTrace.EndTime = time.Now()
	tierTrace.Duration = tierTrace.EndTime.Sub(tierTrace.StartTime)
	tierTrace.Status = "completed"
	tierTrace.ResultSize = resultSize
	if metadata != nil {
		tierTrace.Metadata = metadata
	}

	// Publish tier completed event
	event := AnalysisEvent{
		SessionID:     trace.SessionID,
		NodeID:        trace.NodeID,
		Path:          trace.Path,
		Tier:          tier,
		WorkerID:      tierTrace.WorkerID,
		StartTime:     tierTrace.StartTime,
		EndTime:       tierTrace.EndTime,
		Duration:      tierTrace.Duration,
		CorrelationID: correlationID,
		Metadata:      metadata,
	}

	switch tier {
	case 1:
		et.broker.Publish(Tier1AnalysisCompletedEvent, event)
	case 2:
		et.broker.Publish(Tier2AnalysisCompletedEvent, event)
	case 3:
		et.broker.Publish(Tier3AnalysisCompletedEvent, event)
	}
}

// RecordTierError records an error in tier analysis
func (et *EventTracer) RecordTierError(correlationID string, tier int, err error) {
	et.mu.Lock()
	defer et.mu.Unlock()

	trace, exists := et.correlations[correlationID]
	if !exists {
		return
	}

	tierTrace, exists := trace.TierResults[tier]
	if !exists {
		return
	}

	tierTrace.EndTime = time.Now()
	tierTrace.Duration = tierTrace.EndTime.Sub(tierTrace.StartTime)
	tierTrace.Status = "failed"
	tierTrace.Error = err.Error()

	// Mark the overall trace as failed
	trace.Status = "failed"
	trace.Error = fmt.Sprintf("Tier %d failed: %v", tier, err)

	// Publish tier failed event
	event := AnalysisEvent{
		SessionID:     trace.SessionID,
		NodeID:        trace.NodeID,
		Path:          trace.Path,
		Tier:          tier,
		WorkerID:      tierTrace.WorkerID,
		StartTime:     tierTrace.StartTime,
		EndTime:       tierTrace.EndTime,
		Duration:      tierTrace.Duration,
		Error:         err.Error(),
		CorrelationID: correlationID,
	}

	switch tier {
	case 1:
		et.broker.Publish(Tier1AnalysisFailedEvent, event)
	case 2:
		et.broker.Publish(Tier2AnalysisFailedEvent, event)
	case 3:
		et.broker.Publish(Tier3AnalysisFailedEvent, event)
	}
}

// CompleteAnalysisTrace marks an analysis trace as complete
func (et *EventTracer) CompleteAnalysisTrace(correlationID string) {
	et.mu.Lock()
	defer et.mu.Unlock()

	trace, exists := et.correlations[correlationID]
	if !exists {
		return
	}

	if trace.Status == "running" {
		trace.Status = "completed"
	}
	trace.EndTime = time.Now()
	trace.TotalDuration = trace.EndTime.Sub(trace.StartTime)
}

// GetTrace retrieves an analysis trace by correlation ID
func (et *EventTracer) GetTrace(correlationID string) (*AnalysisTrace, bool) {
	et.mu.RLock()
	defer et.mu.RUnlock()

	trace, exists := et.correlations[correlationID]
	return trace, exists
}

// GetActiveTraces returns all currently active traces
func (et *EventTracer) GetActiveTraces() []*AnalysisTrace {
	et.mu.RLock()
	defer et.mu.RUnlock()

	var active []*AnalysisTrace
	for _, trace := range et.correlations {
		if trace.Status == "running" {
			active = append(active, trace)
		}
	}
	return active
}

// GetTracesBySession returns all traces for a specific session
func (et *EventTracer) GetTracesBySession(sessionID string) []*AnalysisTrace {
	et.mu.RLock()
	defer et.mu.RUnlock()

	var traces []*AnalysisTrace
	for _, trace := range et.correlations {
		if trace.SessionID == sessionID {
			traces = append(traces, trace)
		}
	}
	return traces
}

// cleanupLoop removes old completed traces to prevent memory leaks
func (et *EventTracer) cleanupLoop() {
	ticker := time.NewTicker(et.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			et.cleanup()
		case <-et.done:
			return
		}
	}
}

// cleanup removes old traces to prevent memory leaks
func (et *EventTracer) cleanup() {
	et.mu.Lock()
	defer et.mu.Unlock()

	// If we're under the limit, no cleanup needed
	if len(et.correlations) <= et.maxTraces {
		return
	}

	// Remove completed traces older than 1 hour
	cutoff := time.Now().Add(-1 * time.Hour)
	for id, trace := range et.correlations {
		if trace.Status != "running" && trace.EndTime.Before(cutoff) {
			delete(et.correlations, id)
		}
	}

	// If still over limit, remove oldest completed traces
	if len(et.correlations) > et.maxTraces {
		type traceWithID struct {
			id    string
			trace *AnalysisTrace
		}

		var completed []traceWithID
		for id, trace := range et.correlations {
			if trace.Status != "running" {
				completed = append(completed, traceWithID{id, trace})
			}
		}

		// Sort by end time and remove oldest
		for i := 0; i < len(completed) && len(et.correlations) > et.maxTraces; i++ {
			oldest := completed[0]
			for j := 1; j < len(completed); j++ {
				if completed[j].trace.EndTime.Before(oldest.trace.EndTime) {
					oldest = completed[j]
				}
			}
			delete(et.correlations, oldest.id)

			// Remove from completed slice
			for j, item := range completed {
				if item.id == oldest.id {
					completed = append(completed[:j], completed[j+1:]...)
					break
				}
			}
		}
	}
}
