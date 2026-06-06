package schema_test

import (
	"testing"

	"github.com/cogent/services/internal/schema"
)

func TestEvent_JSONRoundTrip(t *testing.T) {
	model := "gpt-4"
	score := 0.9
	e := schema.Event{
		TraceID: "t1", SpanID: "s1",
		StartTime: 1700000000.0, EndTime: 1700000001.0, DurationMs: 1000.0,
		AgentName: "agent", Operation: "llm_call",
		ServiceName: "svc", Environment: "test",
		Model:     &model,
		EvalScore: &score,
	}

	data, err := e.ToJSON()
	if err != nil {
		t.Fatal(err)
	}
	got, err := schema.FromJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.TraceID != e.TraceID {
		t.Errorf("TraceID: got %q, want %q", got.TraceID, e.TraceID)
	}
	if got.Model == nil || *got.Model != model {
		t.Errorf("Model: got %v, want %q", got.Model, model)
	}
	if got.EvalScore == nil || *got.EvalScore != score {
		t.Errorf("EvalScore: got %v, want %f", got.EvalScore, score)
	}
}
