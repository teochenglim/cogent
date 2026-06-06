package doris

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cogent/services/internal/config"
	"github.com/cogent/services/internal/consumer"
	"github.com/cogent/services/internal/schema"
	"github.com/cogent/services/internal/storage"
	_ "github.com/go-sql-driver/mysql"
	"go.uber.org/zap"
)

// dorisRow is the full Doris row including hydrated content from S3.
type dorisRow struct {
	schema.Event
	Prompt     *string `json:"prompt,omitempty"`
	Completion *string `json:"completion,omitempty"`
	ToolInput  *string `json:"tool_input,omitempty"`
	ToolOutput *string `json:"tool_output,omitempty"`
	Dt         string  `json:"dt"`
}

// Writer batches events and writes them to Doris with S3 hydration.
type Writer struct {
	cfg     config.Config
	store   *storage.Client
	db      *sql.DB
	buf     []dorisRow
	httpCli *http.Client
	logger  *zap.Logger
}

// NewWriter creates a Writer connected to Doris.
func NewWriter(cfg config.Config, store *storage.Client, logger *zap.Logger) (*Writer, error) {
	dsn := consumer.DSN(
		cfg.DorisUser, cfg.DorisPassword,
		cfg.DorisFEHost, cfg.DorisMySQLPort,
		cfg.DorisDatabase,
	)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("doris: open db: %w", err)
	}
	return &Writer{
		cfg:     cfg,
		store:   store,
		db:      db,
		httpCli: &http.Client{Timeout: 60 * time.Second},
		logger:  logger,
	}, nil
}

// Handle hydrates S3 content and buffers the row.
func (w *Writer) Handle(ctx context.Context, e schema.Event) error {
	row := dorisRow{
		Event: e,
		Dt:    time.UnixMicro(int64(e.StartTime * 1e6)).UTC().Format("2006-01-02"),
	}
	fetch := func(ref *string) *string {
		if ref == nil || *ref == "" {
			return nil
		}
		content, err := w.store.Fetch(ctx, *ref)
		if err != nil {
			w.logger.Warn("s3 fetch failed", zap.String("ref", *ref), zap.Error(err))
			return nil
		}
		return &content
	}
	row.Prompt = fetch(e.PromptRef)
	row.Completion = fetch(e.CompletionRef)
	row.ToolInput = fetch(e.ToolInputRef)
	row.ToolOutput = fetch(e.ToolOutputRef)
	w.buf = append(w.buf, row)
	return nil
}

// Flush sends via Stream Load; falls back to MySQL INSERT on failure.
func (w *Writer) Flush(ctx context.Context) error {
	if len(w.buf) == 0 {
		return nil
	}
	data, err := json.Marshal(w.buf)
	if err != nil {
		return fmt.Errorf("doris: marshal batch: %w", err)
	}
	if err := w.streamLoad(ctx, data); err != nil {
		w.logger.Warn("stream load failed, falling back to mysql", zap.Error(err))
		if err2 := w.mysqlInsert(ctx); err2 != nil {
			return fmt.Errorf("doris: both stream load and mysql insert failed: %w", err2)
		}
	}
	w.logger.Info("doris: flushed", zap.Int("count", len(w.buf)))
	w.buf = w.buf[:0]
	return nil
}

func (w *Writer) streamLoad(ctx context.Context, data []byte) error {
	url := fmt.Sprintf("http://%s:%d/api/%s/agent_telemetry/_stream_load",
		w.cfg.DorisFEHost, w.cfg.DorisFEHTTPPort, w.cfg.DorisDatabase)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	creds := base64.StdEncoding.EncodeToString(
		[]byte(w.cfg.DorisUser + ":" + w.cfg.DorisPassword))
	req.Header.Set("Authorization", "Basic "+creds)
	req.Header.Set("format", "json")
	req.Header.Set("strip_outer_array", "true")
	req.Header.Set("Expect", "100-continue")
	req.ContentLength = int64(len(data))

	resp, err := w.httpCli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("stream load HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (w *Writer) mysqlInsert(ctx context.Context) error {
	prefix := `INSERT INTO agent_telemetry
		(trace_id,span_id,parent_span_id,start_time,end_time,duration_ms,
		 agent_name,operation,service_name,environment,
		 model,provider,input_tokens,output_tokens,cost_usd,finish_reason,
		 prompt,prompt_preview,prompt_ref,prompt_size_bytes,
		 completion,completion_preview,completion_ref,completion_size_bytes,
		 tool_name,tool_input,tool_input_preview,tool_input_ref,tool_input_size_bytes,
		 tool_output,tool_output_preview,tool_output_ref,tool_output_size_bytes,
		 tool_error,eval_score,eval_label,eval_reason,eval_source,metadata,dt)
		VALUES `
	placeholders := strings.Repeat(",(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)", len(w.buf))
	q := prefix + placeholders[1:]

	args := make([]any, 0, len(w.buf)*40)
	for _, r := range w.buf {
		metaJSON, _ := json.Marshal(r.Metadata)
		args = append(args,
			r.TraceID, r.SpanID, r.ParentSpanID, r.StartTime, r.EndTime, r.DurationMs,
			r.AgentName, r.Operation, r.ServiceName, r.Environment,
			r.Model, r.Provider, r.InputTokens, r.OutputTokens, r.CostUSD, r.FinishReason,
			r.Prompt, r.PromptPreview, r.PromptRef, r.PromptSizeBytes,
			r.Completion, r.CompletionPreview, r.CompletionRef, r.CompletionSizeBytes,
			r.ToolName,
			r.ToolInput, r.ToolInputPreview, r.ToolInputRef, r.ToolInputSizeBytes,
			r.ToolOutput, r.ToolOutputPreview, r.ToolOutputRef, r.ToolOutputSizeBytes,
			r.ToolError,
			r.EvalScore, r.EvalLabel, r.EvalReason, r.EvalSource,
			string(metaJSON), r.Dt,
		)
	}
	_, err := w.db.ExecContext(ctx, q, args...)
	return err
}
