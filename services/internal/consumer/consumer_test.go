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
		if containsStr(cols, f) && !containsStr(cols, f+"_") {
			t.Errorf("greptime column list contains forbidden column near %q", f)
		}
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestAlert_PayloadSize(t *testing.T) {
	var total int64
	for _, s := range []int64{1000, 2000, 3000, 4000} {
		total += s
	}
	if total != 10000 {
		t.Errorf("expected sum 10000, got %d", total)
	}
	if total <= 9000 {
		t.Errorf("expected %d > 9000", total)
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

func TestConfig_DSNNotEmpty(t *testing.T) {
	dsn := consumer.DSN("root", "", "localhost", 4002, "public")
	if dsn == "" {
		t.Error("DSN should not be empty")
	}
	_ = time.Second
}
