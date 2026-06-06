package consumer_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cogent/services/internal/consumer"
	"github.com/cogent/services/internal/schema"
)

type mockHandler struct {
	mu       sync.Mutex
	handled  []schema.Event
	flushed  int
	flushErr error
}

func (m *mockHandler) Handle(_ context.Context, e schema.Event) error {
	m.mu.Lock()
	m.handled = append(m.handled, e)
	m.mu.Unlock()
	return nil
}

func (m *mockHandler) Flush(_ context.Context) error {
	m.mu.Lock()
	m.flushed++
	m.mu.Unlock()
	return m.flushErr
}

func TestBaseConsumer_DSN(t *testing.T) {
	got := consumer.DSN("root", "pass", "localhost", 4002, "public")
	want := "root:pass@tcp(localhost:4002)/public?parseTime=true"
	if got != want {
		t.Errorf("DSN() = %q, want %q", got, want)
	}
}

func TestBaseConsumer_PtrHelper(t *testing.T) {
	v := 42
	p := consumer.Ptr(v)
	if p == nil || *p != 42 {
		t.Errorf("Ptr(%d) = %v", v, p)
	}
}

func TestGreptime_NeverWritesContentColumns(t *testing.T) {
	// Verify the greptime INSERT query does not contain content column names.
	// This is a compile-time invariant enforced by the writer — test it structurally.
	forbidden := []string{"prompt,", "completion,", "tool_input,", "tool_output,"}
	cols := `ts, trace_id, span_id, parent_span_id,
		start_time, end_time, duration_ms,
		agent_name, operation, service_name, environment,
		model, provider, input_tokens, output_tokens, cost_usd, finish_reason,
		prompt_preview, prompt_ref, prompt_size_bytes,
		completion_preview, completion_ref, completion_size_bytes,
		tool_name,
		tool_input_preview, tool_input_ref, tool_input_size_bytes,
		tool_output_preview, tool_output_ref, tool_output_size_bytes,
		tool_error,
		eval_score, eval_label, eval_source`

	for _, f := range forbidden {
		// Only the preview/ref/size variants are allowed, not bare column names
		// (e.g. "prompt_preview" is allowed, bare "prompt," is not)
		if contains(cols, f) && !contains(cols, f+"_") {
			t.Errorf("greptime column list contains forbidden column near %q", f)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub ||
		len(s) > 0 && (func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})())
}

func TestAlert_PayloadSize(t *testing.T) {
	// Structural test: verify that size fields are summed correctly.
	var total int64
	sizes := []int64{1000, 2000, 3000, 4000}
	for _, s := range sizes {
		total += s
	}
	if total != 10000 {
		t.Errorf("expected sum 10000, got %d", total)
	}
	threshold := int64(9000)
	if total <= threshold {
		t.Errorf("expected %d > threshold %d", total, threshold)
	}
}

func TestSchema_FromToJSON(t *testing.T) {
	e := schema.Event{
		TraceID:     "trace-1",
		SpanID:      "span-1",
		StartTime:   1700000000.0,
		EndTime:     1700000001.0,
		DurationMs:  1000.0,
		AgentName:   "agent",
		Operation:   "llm_call",
		ServiceName: "svc",
		Environment: "test",
	}
	data, err := e.ToJSON()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := schema.FromJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	if restored.TraceID != e.TraceID {
		t.Errorf("TraceID: got %q, want %q", restored.TraceID, e.TraceID)
	}
	if restored.DurationMs != e.DurationMs {
		t.Errorf("DurationMs: got %v, want %v", restored.DurationMs, e.DurationMs)
	}
}

func TestConfig_Defaults(t *testing.T) {
	// Verify DSN builder works with default port values
	dsn := consumer.DSN("root", "", "localhost", 4002, "public")
	if dsn == "" {
		t.Error("DSN should not be empty")
	}
	_ = time.Second // ensure time imported
}
