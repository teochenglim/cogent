package greptime

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/cogent/services/internal/config"
	"github.com/cogent/services/internal/consumer"
	"github.com/cogent/services/internal/schema"
	_ "github.com/go-sql-driver/mysql"
	"go.uber.org/zap"
)

// Writer batches events and writes them to GreptimeDB.
// It never writes full content columns — only previews, refs, and sizes.
type Writer struct {
	db     *sql.DB
	buf    []schema.Event
	logger *zap.Logger
}

// NewWriter opens a MySQL connection to GreptimeDB.
func NewWriter(cfg config.Config, logger *zap.Logger) (*Writer, error) {
	dsn := consumer.DSN(
		"root", "",
		cfg.GreptimeHost, cfg.GreptimePort,
		cfg.GreptimeDatabase,
	)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("greptime: open db: %w", err)
	}
	db.SetMaxOpenConns(4)
	db.SetConnMaxLifetime(5 * time.Minute)
	return &Writer{db: db, logger: logger}, nil
}

// Handle adds one event to the pending batch.
func (w *Writer) Handle(_ context.Context, e schema.Event) error {
	w.buf = append(w.buf, e)
	return nil
}

// Flush writes the buffered batch as a single INSERT. Clears the buffer on success.
// INVARIANT: never writes prompt, completion, tool_input, tool_output content columns.
func (w *Writer) Flush(ctx context.Context) error {
	if len(w.buf) == 0 {
		return nil
	}

	const cols = `ts, trace_id, span_id, parent_span_id,
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

	placeholders := strings.Repeat(",(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)", len(w.buf))
	placeholders = placeholders[1:] // trim leading comma

	query := "INSERT INTO agent_telemetry (" + cols + ") VALUES " + placeholders

	args := make([]any, 0, len(w.buf)*34)
	for _, e := range w.buf {
		args = append(args,
			time.UnixMicro(int64(e.StartTime*1e6)).UTC(),
			e.TraceID, e.SpanID, e.ParentSpanID,
			e.StartTime, e.EndTime, e.DurationMs,
			e.AgentName, e.Operation, e.ServiceName, e.Environment,
			e.Model, e.Provider, e.InputTokens, e.OutputTokens, e.CostUSD, e.FinishReason,
			e.PromptPreview, e.PromptRef, e.PromptSizeBytes,
			e.CompletionPreview, e.CompletionRef, e.CompletionSizeBytes,
			e.ToolName,
			e.ToolInputPreview, e.ToolInputRef, e.ToolInputSizeBytes,
			e.ToolOutputPreview, e.ToolOutputRef, e.ToolOutputSizeBytes,
			e.ToolError,
			e.EvalScore, e.EvalLabel, e.EvalSource,
		)
	}

	_, err := w.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("greptime: flush %d events: %w", len(w.buf), err)
	}
	w.logger.Info("greptime: flushed", zap.Int("count", len(w.buf)))
	w.buf = w.buf[:0]
	return nil
}
