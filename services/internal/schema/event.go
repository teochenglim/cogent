package schema

import (
	"encoding/json"
	"fmt"
)

// Event mirrors the Python AgentEvent schema exactly.
// All pointer fields are optional (nil == not set).
// JSON tags match Python field names.
type Event struct {
	TraceID      string  `json:"trace_id"`
	SpanID       string  `json:"span_id"`
	ParentSpanID *string `json:"parent_span_id,omitempty"`
	StartTime    float64 `json:"start_time"`
	EndTime      float64 `json:"end_time"`
	DurationMs   float64 `json:"duration_ms"`

	AgentName   string `json:"agent_name"`
	Operation   string `json:"operation"`
	ServiceName string `json:"service_name"`
	Environment string `json:"environment"`

	Model        *string  `json:"model,omitempty"`
	Provider     *string  `json:"provider,omitempty"`
	InputTokens  *int32   `json:"input_tokens,omitempty"`
	OutputTokens *int32   `json:"output_tokens,omitempty"`
	CostUSD      *float64 `json:"cost_usd,omitempty"`
	FinishReason *string  `json:"finish_reason,omitempty"`

	PromptPreview     *string `json:"prompt_preview,omitempty"`
	CompletionPreview *string `json:"completion_preview,omitempty"`
	ToolInputPreview  *string `json:"tool_input_preview,omitempty"`
	ToolOutputPreview *string `json:"tool_output_preview,omitempty"`

	PromptRef     *string `json:"prompt_ref,omitempty"`
	CompletionRef *string `json:"completion_ref,omitempty"`
	ToolInputRef  *string `json:"tool_input_ref,omitempty"`
	ToolOutputRef *string `json:"tool_output_ref,omitempty"`

	PromptSizeBytes     *int64 `json:"prompt_size_bytes,omitempty"`
	CompletionSizeBytes *int64 `json:"completion_size_bytes,omitempty"`
	ToolInputSizeBytes  *int64 `json:"tool_input_size_bytes,omitempty"`
	ToolOutputSizeBytes *int64 `json:"tool_output_size_bytes,omitempty"`

	ToolName  *string `json:"tool_name,omitempty"`
	ToolError *string `json:"tool_error,omitempty"`

	EvalScore  *float64 `json:"eval_score,omitempty"`
	EvalLabel  *string  `json:"eval_label,omitempty"`
	EvalReason *string  `json:"eval_reason,omitempty"`
	EvalSource *string  `json:"eval_source,omitempty"`

	Metadata map[string]any `json:"metadata,omitempty"`
}

// FromJSON parses a JSON byte slice into an Event.
// Returns an error if required fields (trace_id, span_id, start_time) are missing.
func FromJSON(data []byte) (Event, error) {
	var e Event
	if err := json.Unmarshal(data, &e); err != nil {
		return Event{}, err
	}
	if e.TraceID == "" || e.SpanID == "" || e.StartTime == 0 {
		return Event{}, fmt.Errorf("invalid event: missing required fields (trace_id=%q span_id=%q start_time=%v)", e.TraceID, e.SpanID, e.StartTime)
	}
	return e, nil
}

// ToJSON serialises an Event to JSON bytes.
func (e Event) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}
