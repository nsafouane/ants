package pubsub

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEventTracer_AnalysisLifecycle(t *testing.T) {
	t.Parallel()

	// Setup
	broker := NewBroker[AnalysisEvent]()
	tracer := NewEventTracer(broker)
	tracer.Start()
	defer tracer.Stop()

	// Subscribe to events
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	events := broker.Subscribe(ctx)

	// Start analysis trace
	correlationID := tracer.StartAnalysisTrace("session-123", "/src/main.go", 42)
	require.NotEmpty(t, correlationID)

	// Verify trace was created
	trace, exists := tracer.GetTrace(correlationID)
	require.True(t, exists)
	require.Equal(t, "session-123", trace.SessionID)
	require.Equal(t, "/src/main.go", trace.Path)
	require.Equal(t, int64(42), trace.NodeID)
	require.Equal(t, "running", trace.Status)

	// Record tier 1 start
	tracer.RecordTierStart(correlationID, 1, "worker-1")

	// Check for tier 1 started event
	select {
	case event := <-events:
		require.Equal(t, Tier1AnalysisStartedEvent, event.Type)
		require.Equal(t, correlationID, event.Payload.CorrelationID)
		require.Equal(t, 1, event.Payload.Tier)
		require.Equal(t, "worker-1", event.Payload.WorkerID)
	case <-time.After(1 * time.Second):
		t.Fatal("Expected tier 1 started event")
	}

	// Record tier 1 completion
	metadata := map[string]interface{}{"nodes_found": 15}
	tracer.RecordTierComplete(correlationID, 1, 15, metadata)

	// Check for tier 1 completed event
	select {
	case event := <-events:
		require.Equal(t, Tier1AnalysisCompletedEvent, event.Type)
		require.Equal(t, correlationID, event.Payload.CorrelationID)
		require.Equal(t, 1, event.Payload.Tier)
		require.GreaterOrEqual(t, event.Payload.Duration, time.Duration(0))
		require.Equal(t, metadata, event.Payload.Metadata)
	case <-time.After(1 * time.Second):
		t.Fatal("Expected tier 1 completed event")
	}

	// Verify tier trace was updated
	trace, _ = tracer.GetTrace(correlationID)
	tierTrace := trace.TierResults[1]
	require.NotNil(t, tierTrace)
	require.Equal(t, "completed", tierTrace.Status)
	require.Equal(t, 15, tierTrace.ResultSize)
	require.GreaterOrEqual(t, tierTrace.Duration, time.Duration(0))

	// Complete analysis
	tracer.CompleteAnalysisTrace(correlationID)
	trace, _ = tracer.GetTrace(correlationID)
	require.Equal(t, "completed", trace.Status)
	require.GreaterOrEqual(t, trace.TotalDuration, time.Duration(0))
}

func TestEventTracer_TierError(t *testing.T) {
	t.Parallel()

	// Setup
	broker := NewBroker[AnalysisEvent]()
	tracer := NewEventTracer(broker)
	tracer.Start()
	defer tracer.Stop()

	// Subscribe to events
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	events := broker.Subscribe(ctx)

	// Start analysis and tier
	correlationID := tracer.StartAnalysisTrace("session-456", "/src/error.go", 123)
	tracer.RecordTierStart(correlationID, 2, "worker-2")

	// Consume the started event
	<-events

	// Record tier error
	err := fmt.Errorf("analysis failed: out of memory")
	tracer.RecordTierError(correlationID, 2, err)

	// Check for tier 2 failed event
	select {
	case event := <-events:
		require.Equal(t, Tier2AnalysisFailedEvent, event.Type)
		require.Equal(t, correlationID, event.Payload.CorrelationID)
		require.Equal(t, 2, event.Payload.Tier)
		require.Equal(t, "analysis failed: out of memory", event.Payload.Error)
	case <-time.After(1 * time.Second):
		t.Fatal("Expected tier 2 failed event")
	}

	// Verify trace was marked as failed
	trace, _ := tracer.GetTrace(correlationID)
	require.Equal(t, "failed", trace.Status)
	require.Contains(t, trace.Error, "Tier 2 failed")

	tierTrace := trace.TierResults[2]
	require.Equal(t, "failed", tierTrace.Status)
	require.Equal(t, "analysis failed: out of memory", tierTrace.Error)
}

func TestEventTracer_ActiveTraces(t *testing.T) {
	t.Parallel()

	broker := NewBroker[AnalysisEvent]()
	tracer := NewEventTracer(broker)
	tracer.Start()
	defer tracer.Stop()

	// Start multiple traces
	corr1 := tracer.StartAnalysisTrace("session-1", "/src/file1.go", 1)
	_ = tracer.StartAnalysisTrace("session-1", "/src/file2.go", 2) // corr2 not used in this test
	corr3 := tracer.StartAnalysisTrace("session-2", "/src/file3.go", 3)

	// Check active traces
	active := tracer.GetActiveTraces()
	require.Len(t, active, 3)

	// Complete one trace
	tracer.CompleteAnalysisTrace(corr1)

	// Check active traces again
	active = tracer.GetActiveTraces()
	require.Len(t, active, 2)

	// Check traces by session
	session1Traces := tracer.GetTracesBySession("session-1")
	require.Len(t, session1Traces, 2)

	session2Traces := tracer.GetTracesBySession("session-2")
	require.Len(t, session2Traces, 1)
	require.Equal(t, corr3, session2Traces[0].CorrelationID)
}

func TestContextEngineEvents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		eventType EventType
		payload   interface{}
	}{
		{
			name:      "CodeNodeCreated",
			eventType: CodeNodeCreatedEvent,
			payload: CodeNodeEvent{
				NodeID:     123,
				SessionID:  "session-test",
				Path:       "/src/test.go",
				Symbol:     "TestFunction",
				Kind:       "function",
				Language:   "go",
				Timestamp:  time.Now(),
				ChangeType: "created",
			},
		},
		{
			name:      "DependencyAdded",
			eventType: DependencyAddedEvent,
			payload: DependencyEvent{
				DependencyID: 456,
				FromNodeID:   123,
				ToNodeID:     789,
				Relation:     "imports",
				Timestamp:    time.Now(),
				ChangeType:   "added",
			},
		},
		{
			name:      "ResourceConstraint",
			eventType: ResourceConstraintEvent,
			payload: ResourceEvent{
				EventType:      "memory_constraint",
				Timestamp:      time.Now(),
				ResourceType:   "memory",
				CurrentValue:   512.0,
				Threshold:      400.0,
				Severity:       "high",
				Component:      "orchestrator",
				Recommendation: "Consider switching to Eco mode",
			},
		},
		{
			name:      "FileDeleted",
			eventType: FileDeletedEvent,
			payload: FileSystemEvent{
				Path:          "/src/deleted.go",
				EventType:     "deleted",
				Timestamp:     time.Now(),
				SessionID:     "session-test",
				AffectedNodes: []int64{123, 456},
			},
		},
		{
			name:      "OperationalModeChanged",
			eventType: OperationalModeChangedEvent,
			payload: OperationalModeEvent{
				OldMode:     "Performance",
				NewMode:     "Eco",
				Timestamp:   time.Now(),
				Reason:      "High memory usage detected",
				TriggeredBy: "system",
				SessionID:   "session-test",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create brokers for different event types
			codeNodeBroker := NewBroker[CodeNodeEvent]()
			depBroker := NewBroker[DependencyEvent]()
			resourceBroker := NewBroker[ResourceEvent]()
			fileBroker := NewBroker[FileSystemEvent]()
			modeBroker := NewBroker[OperationalModeEvent]()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			// Test publishing and receiving the specific event type
			switch tt.eventType {
			case CodeNodeCreatedEvent:
				payload := tt.payload.(CodeNodeEvent)
				events := codeNodeBroker.Subscribe(ctx)
				codeNodeBroker.Publish(tt.eventType, payload)

				select {
				case event := <-events:
					require.Equal(t, tt.eventType, event.Type)
					require.Equal(t, payload.NodeID, event.Payload.NodeID)
					require.Equal(t, payload.Path, event.Payload.Path)
				case <-time.After(1 * time.Second):
					t.Fatal("Expected event not received")
				}

			case DependencyAddedEvent:
				payload := tt.payload.(DependencyEvent)
				events := depBroker.Subscribe(ctx)
				depBroker.Publish(tt.eventType, payload)

				select {
				case event := <-events:
					require.Equal(t, tt.eventType, event.Type)
					require.Equal(t, payload.DependencyID, event.Payload.DependencyID)
				case <-time.After(1 * time.Second):
					t.Fatal("Expected event not received")
				}

			case ResourceConstraintEvent:
				payload := tt.payload.(ResourceEvent)
				events := resourceBroker.Subscribe(ctx)
				resourceBroker.Publish(tt.eventType, payload)

				select {
				case event := <-events:
					require.Equal(t, tt.eventType, event.Type)
					require.Equal(t, payload.ResourceType, event.Payload.ResourceType)
					require.Equal(t, payload.Severity, event.Payload.Severity)
				case <-time.After(1 * time.Second):
					t.Fatal("Expected event not received")
				}

			case FileDeletedEvent:
				payload := tt.payload.(FileSystemEvent)
				events := fileBroker.Subscribe(ctx)
				fileBroker.Publish(tt.eventType, payload)

				select {
				case event := <-events:
					require.Equal(t, tt.eventType, event.Type)
					require.Equal(t, payload.Path, event.Payload.Path)
					require.Equal(t, payload.AffectedNodes, event.Payload.AffectedNodes)
				case <-time.After(1 * time.Second):
					t.Fatal("Expected event not received")
				}

			case OperationalModeChangedEvent:
				payload := tt.payload.(OperationalModeEvent)
				events := modeBroker.Subscribe(ctx)
				modeBroker.Publish(tt.eventType, payload)

				select {
				case event := <-events:
					require.Equal(t, tt.eventType, event.Type)
					require.Equal(t, payload.OldMode, event.Payload.OldMode)
					require.Equal(t, payload.NewMode, event.Payload.NewMode)
				case <-time.After(1 * time.Second):
					t.Fatal("Expected event not received")
				}
			}
		})
	}
}

func TestEventTracer_CleanupOldTraces(t *testing.T) {
	// This test verifies the cleanup mechanism
	broker := NewBroker[AnalysisEvent]()
	tracer := NewEventTracer(broker)
	tracer.maxTraces = 3 // Set low limit for testing

	// Create several traces
	for i := 0; i < 5; i++ {
		corr := tracer.StartAnalysisTrace(fmt.Sprintf("session-%d", i), fmt.Sprintf("/file%d.go", i), int64(i))
		tracer.CompleteAnalysisTrace(corr)
	}

	// Manually trigger cleanup
	tracer.cleanup()

	// Should have removed some traces
	require.LessOrEqual(t, len(tracer.correlations), tracer.maxTraces)
}

func TestEventBrokerIntegration(t *testing.T) {
	t.Parallel()

	// Test that the existing broker works with new event types
	broker := NewBroker[AnalysisEvent]()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	events := broker.Subscribe(ctx)

	// Test multiple subscribers
	events2 := broker.Subscribe(ctx)

	require.Equal(t, 2, broker.GetSubscriberCount())

	// Publish an event
	event := AnalysisEvent{
		SessionID:     "test-session",
		Path:          "/test/file.go",
		Tier:          1,
		StartTime:     time.Now(),
		CorrelationID: "test-correlation",
	}

	broker.Publish(Tier1AnalysisStartedEvent, event)

	// Both subscribers should receive the event
	for i := 0; i < 2; i++ {
		var received Event[AnalysisEvent]
		if i == 0 {
			select {
			case received = <-events:
			case <-time.After(1 * time.Second):
				t.Fatal("Event not received by subscriber 1")
			}
		} else {
			select {
			case received = <-events2:
			case <-time.After(1 * time.Second):
				t.Fatal("Event not received by subscriber 2")
			}
		}

		require.Equal(t, Tier1AnalysisStartedEvent, received.Type)
		require.Equal(t, event.SessionID, received.Payload.SessionID)
		require.Equal(t, event.CorrelationID, received.Payload.CorrelationID)
	}

	broker.Shutdown()
	require.Equal(t, 0, broker.GetSubscriberCount())
}
