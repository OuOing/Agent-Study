package agent

import (
	"context"
	"testing"
)

type recordingRecorder struct {
	events []RunEvent
}

func (r *recordingRecorder) Record(_ context.Context, event RunEvent) error {
	r.events = append(r.events, event)
	return nil
}

func TestOrchestratorRecordsModelAndToolEvents(t *testing.T) {
	recorder := &recordingRecorder{}
	orchestrator := NewOrchestratorWithRecorder(
		DemoModel{},
		NewRegistry(SearchNotesTool{}),
		4,
		recorder,
	)

	answer, err := orchestrator.Run(WithRunID(context.Background(), "run-123"), "整理会议待办")
	if err != nil {
		t.Fatal(err)
	}
	if answer == "" {
		t.Fatal("expected final answer")
	}

	if len(recorder.events) != 3 {
		t.Fatalf("expected 3 events, got %d: %#v", len(recorder.events), recorder.events)
	}
	if recorder.events[0].Kind != "model_call" || recorder.events[0].DecisionType != "tool_call" {
		t.Fatalf("unexpected first event: %#v", recorder.events[0])
	}
	if recorder.events[1].Kind != "tool_call" || recorder.events[1].ToolName != "search_notes" {
		t.Fatalf("unexpected second event: %#v", recorder.events[1])
	}
	if recorder.events[2].Kind != "model_call" || recorder.events[2].DecisionType != "final" {
		t.Fatalf("unexpected third event: %#v", recorder.events[2])
	}
}
