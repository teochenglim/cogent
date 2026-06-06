package doris

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/cogent/services/internal/config"
	"github.com/cogent/services/internal/consumer"
	_ "github.com/go-sql-driver/mysql"
)

// DB wraps a sql.DB for Doris queries.
type DB struct {
	db *sql.DB
}

// NewDB opens a MySQL connection to Doris for queries.
func NewDB(cfg config.Config) (*DB, error) {
	dsn := consumer.DSN(cfg.DorisUser, cfg.DorisPassword, cfg.DorisFEHost, cfg.DorisMySQLPort, cfg.DorisDatabase)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("doris: %w", err)
	}
	db.SetMaxOpenConns(4)
	return &DB{db: db}, nil
}

// SearchResult is a full-text search hit.
type SearchResult struct {
	TraceID           string   `json:"trace_id"`
	SpanID            string   `json:"span_id"`
	AgentName         string   `json:"agent_name"`
	StartTime         float64  `json:"start_time"`
	PromptPreview     *string  `json:"prompt_preview"`
	CompletionPreview *string  `json:"completion_preview"`
	EvalScore         *float64 `json:"eval_score"`
}

// Search performs full-text search using Doris SEARCH() DSL (Doris 4.x inverted index).
func (d *DB) Search(ctx context.Context, q, agentName string, startTs, endTs int64, limit, offset int) ([]SearchResult, error) {
	searchExpr := fmt.Sprintf("prompt:%s OR completion:%s OR tool_input:%s OR tool_output:%s", q, q, q, q)
	query := `SELECT trace_id, span_id, agent_name, start_time,
		prompt_preview, completion_preview, eval_score
		FROM agent_telemetry
		WHERE SEARCH(?)
		AND dt BETWEEN FROM_UNIXTIME(?) AND FROM_UNIXTIME(?)`
	args := []any{searchExpr, startTs, endTs}
	if agentName != "" {
		query += " AND agent_name = ?"
		args = append(args, agentName)
	}
	query += " ORDER BY start_time DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := d.db.QueryContext(ctx, query, args...)
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

// Justification is a human annotation record.
type Justification struct {
	SpanID    string   `json:"span_id"`
	TraceID   string   `json:"trace_id"`
	AgentName string   `json:"agent_name"`
	Score     *float64 `json:"eval_score"`
	Label     *string  `json:"eval_label"`
	Reason    *string  `json:"eval_reason"`
	StartTime float64  `json:"start_time"`
}

// ListJustifications returns recent human annotation events.
func (d *DB) ListJustifications(ctx context.Context, agentName string, limit int) ([]Justification, error) {
	q := `SELECT span_id, trace_id, agent_name, eval_score, eval_label, eval_reason, start_time
		FROM agent_telemetry
		WHERE operation = 'human_annotation'`
	args := []any{}
	if agentName != "" {
		q += " AND agent_name = ?"
		args = append(args, agentName)
	}
	q += " ORDER BY start_time DESC LIMIT ?"
	args = append(args, limit)

	rows, err := d.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Justification
	for rows.Next() {
		var j Justification
		if err := rows.Scan(&j.SpanID, &j.TraceID, &j.AgentName,
			&j.Score, &j.Label, &j.Reason, &j.StartTime); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}
