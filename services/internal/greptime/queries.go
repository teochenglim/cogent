package greptime

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/cogent/services/internal/config"
	"github.com/cogent/services/internal/consumer"
	_ "github.com/go-sql-driver/mysql"
)

// DB wraps a *sql.DB connected to GreptimeDB.
type DB struct {
	db *sql.DB
}

// NewDB opens a MySQL connection to GreptimeDB for queries.
func NewDB(cfg config.Config) (*DB, error) {
	dsn := consumer.DSN("root", "", cfg.GreptimeHost, cfg.GreptimePort, cfg.GreptimeDatabase)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("greptime: %w", err)
	}
	db.SetMaxOpenConns(8)
	db.SetConnMaxLifetime(5 * time.Minute)
	return &DB{db: db}, nil
}

// TraceSummary is a row from the trace list query.
type TraceSummary struct {
	TraceID          string   `json:"trace_id"`
	AgentName        string   `json:"agent_name"`
	SpanCount        int      `json:"span_count"`
	TotalCostUSD     *float64 `json:"total_cost_usd"`
	AvgEvalScore     *float64 `json:"avg_eval_score"`
	MaxSpanSizeBytes *int64   `json:"max_span_size_bytes"`
	StartTime        float64  `json:"start_time"`
	EndTime          float64  `json:"end_time"`
	DurationMs       float64  `json:"duration_ms"`
}

// ListTraces returns trace summaries grouped by trace_id.
func (g *DB) ListTraces(ctx context.Context, agentName, environment string, startTs, endTs int64, limit, offset int) ([]TraceSummary, error) {
	q := `SELECT trace_id, agent_name,
		COUNT(*) AS span_count,
		SUM(cost_usd) AS total_cost,
		AVG(eval_score) AS avg_score,
		MAX(COALESCE(prompt_size_bytes,0)+COALESCE(completion_size_bytes,0)) AS max_span_bytes,
		MIN(start_time) AS start_t,
		MAX(end_time) AS end_t,
		(MAX(end_time)-MIN(start_time))*1000 AS dur_ms
		FROM agent_telemetry
		WHERE ts BETWEEN FROM_UNIXTIME(?) AND FROM_UNIXTIME(?)`
	args := []any{startTs, endTs}
	if agentName != "" {
		q += " AND agent_name = ?"
		args = append(args, agentName)
	}
	if environment != "" {
		q += " AND environment = ?"
		args = append(args, environment)
	}
	q += " GROUP BY trace_id, agent_name ORDER BY start_t DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := g.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TraceSummary
	for rows.Next() {
		var t TraceSummary
		if err := rows.Scan(&t.TraceID, &t.AgentName, &t.SpanCount,
			&t.TotalCostUSD, &t.AvgEvalScore, &t.MaxSpanSizeBytes,
			&t.StartTime, &t.EndTime, &t.DurationMs); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// SpanDetail is a single span row for the trace detail view.
type SpanDetail struct {
	SpanID              string   `json:"span_id"`
	ParentSpanID        *string  `json:"parent_span_id"`
	Depth               int      `json:"depth"`
	Operation           string   `json:"operation"`
	AgentName           string   `json:"agent_name"`
	Model               *string  `json:"model"`
	ToolName            *string  `json:"tool_name"`
	DurationMs          float64  `json:"duration_ms"`
	CostUSD             *float64 `json:"cost_usd"`
	InputTokens         *int32   `json:"input_tokens"`
	OutputTokens        *int32   `json:"output_tokens"`
	PromptPreview       *string  `json:"prompt_preview"`
	CompletionPreview   *string  `json:"completion_preview"`
	ToolInputPreview    *string  `json:"tool_input_preview"`
	ToolOutputPreview   *string  `json:"tool_output_preview"`
	PromptRef           *string  `json:"prompt_ref"`
	CompletionRef       *string  `json:"completion_ref"`
	ToolInputRef        *string  `json:"tool_input_ref"`
	ToolOutputRef       *string  `json:"tool_output_ref"`
	PromptSizeBytes     *int64   `json:"prompt_size_bytes"`
	CompletionSizeBytes *int64   `json:"completion_size_bytes"`
	ToolInputSizeBytes  *int64   `json:"tool_input_size_bytes"`
	ToolOutputSizeBytes *int64   `json:"tool_output_size_bytes"`
	ToolError           *string  `json:"tool_error"`
	EvalScore           *float64 `json:"eval_score"`
	EvalLabel           *string  `json:"eval_label"`
	EvalSource          *string  `json:"eval_source"`
	StartTime           float64  `json:"start_time"`
}

// GetTrace returns all spans for a trace sorted by start_time, with depth calculated.
func (g *DB) GetTrace(ctx context.Context, traceID string) ([]SpanDetail, error) {
	q := `SELECT span_id, parent_span_id, operation, agent_name, model, tool_name,
		duration_ms, cost_usd, input_tokens, output_tokens,
		prompt_preview, completion_preview, tool_input_preview, tool_output_preview,
		prompt_ref, completion_ref, tool_input_ref, tool_output_ref,
		prompt_size_bytes, completion_size_bytes, tool_input_size_bytes, tool_output_size_bytes,
		tool_error, eval_score, eval_label, eval_source, start_time
		FROM agent_telemetry
		WHERE trace_id = ?
		ORDER BY start_time ASC`

	rows, err := g.db.QueryContext(ctx, q, traceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var spans []SpanDetail
	for rows.Next() {
		var s SpanDetail
		if err := rows.Scan(&s.SpanID, &s.ParentSpanID, &s.Operation, &s.AgentName,
			&s.Model, &s.ToolName, &s.DurationMs, &s.CostUSD, &s.InputTokens, &s.OutputTokens,
			&s.PromptPreview, &s.CompletionPreview, &s.ToolInputPreview, &s.ToolOutputPreview,
			&s.PromptRef, &s.CompletionRef, &s.ToolInputRef, &s.ToolOutputRef,
			&s.PromptSizeBytes, &s.CompletionSizeBytes, &s.ToolInputSizeBytes, &s.ToolOutputSizeBytes,
			&s.ToolError, &s.EvalScore, &s.EvalLabel, &s.EvalSource, &s.StartTime); err != nil {
			return nil, err
		}
		spans = append(spans, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Calculate depth from parent_span_id chain
	depthMap := map[string]int{"": -1}
	for i := range spans {
		s := &spans[i]
		parent := ""
		if s.ParentSpanID != nil {
			parent = *s.ParentSpanID
		}
		parentDepth, ok := depthMap[parent]
		if !ok {
			parentDepth = -1
		}
		s.Depth = parentDepth + 1
		depthMap[s.SpanID] = s.Depth
	}
	return spans, nil
}

// SearchResult mirrors the Doris search result shape so the server can use either backend.
type SearchResult struct {
	TraceID           string   `json:"trace_id"`
	SpanID            string   `json:"span_id"`
	AgentName         string   `json:"agent_name"`
	StartTime         float64  `json:"start_time"`
	PromptPreview     *string  `json:"prompt_preview"`
	CompletionPreview *string  `json:"completion_preview"`
	EvalScore         *float64 `json:"eval_score"`
}

// Search does a LIKE search across preview columns. Used when Doris is unavailable.
func (g *DB) Search(ctx context.Context, q, agentName string, startTs, endTs int64, limit, offset int) ([]SearchResult, error) {
	pattern := "%" + q + "%"
	query := `SELECT trace_id, span_id, agent_name, start_time,
		prompt_preview, completion_preview, eval_score
		FROM agent_telemetry
		WHERE (prompt_preview LIKE ? OR completion_preview LIKE ?
		       OR tool_input_preview LIKE ? OR tool_output_preview LIKE ?)
		AND ts BETWEEN FROM_UNIXTIME(?) AND FROM_UNIXTIME(?)`
	args := []any{pattern, pattern, pattern, pattern, startTs, endTs}
	if agentName != "" {
		query += " AND agent_name = ?"
		args = append(args, agentName)
	}
	query += " ORDER BY start_time DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := g.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.TraceID, &r.SpanID, &r.AgentName, &r.StartTime,
			&r.PromptPreview, &r.CompletionPreview, &r.EvalScore); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Stats holds aggregate dashboard stats.
type Stats struct {
	TotalTracesToday  int      `json:"total_traces_today"`
	TotalCostTodayUSD float64  `json:"total_cost_today_usd"`
	AvgEvalScore      *float64 `json:"avg_eval_score"`
	ActiveAgents      int      `json:"active_agents"`
}

// GetStats returns aggregate stats for the dashboard header.
func (g *DB) GetStats(ctx context.Context) (Stats, error) {
	q := `SELECT
		COUNT(DISTINCT trace_id),
		COALESCE(SUM(cost_usd), 0),
		AVG(eval_score),
		COUNT(DISTINCT agent_name)
		FROM agent_telemetry
		WHERE ts >= NOW() - INTERVAL 1 DAY`
	var s Stats
	if err := g.db.QueryRowContext(ctx, q).Scan(
		&s.TotalTracesToday, &s.TotalCostTodayUSD, &s.AvgEvalScore, &s.ActiveAgents,
	); err != nil {
		return Stats{}, err
	}
	return s, nil
}

// GetSpanRef returns the S3 ref key for a specific field of a span.
func (g *DB) GetSpanRef(ctx context.Context, spanID, field string) (string, error) {
	col := map[string]string{
		"prompt":      "prompt_ref",
		"completion":  "completion_ref",
		"tool_input":  "tool_input_ref",
		"tool_output": "tool_output_ref",
	}[field]
	if col == "" {
		return "", fmt.Errorf("unknown field %q", field)
	}
	var ref sql.NullString
	if err := g.db.QueryRowContext(ctx,
		"SELECT "+col+" FROM agent_telemetry WHERE span_id = ? LIMIT 1", spanID,
	).Scan(&ref); err != nil {
		return "", err
	}
	return ref.String, nil
}
