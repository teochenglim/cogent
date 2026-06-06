# Cogent Server — REST API + Embedded UI

## Overview

A single Go binary (`cmd/server`) serves both the REST API and the embedded UI.
Deploy one binary on one port (default 8090) and you get everything. Same
pattern as Prometheus. No separate web server, no CDN, no Node process.

Static HTML/CSS/JS files live in `services/ui/` and are embedded into the binary
at build time via `go:embed`.

## cmd/server/main.go

```
Server port:           SERVER_PORT (default 8090)
Read timeout:          SERVER_READ_TIMEOUT (default 30s)
Write timeout:         SERVER_WRITE_TIMEOUT (default 60s)
```

**CLI flag overrides** follow the standard `flag > env > default` precedence:
```bash
./server --port 9000 --read-timeout 60s
```

**Routing:**
- `/api/*` → REST API handlers (GreptimeDB + Doris + S3 + Redpanda)
- `/*` → `http.FileServer` serving embedded `ui/` with `index.html` fallback
  (SPA-style: any unknown path returns `index.html`)

`chi` router used for clean `/api/` prefix grouping and middleware (logging,
panic recovery, CORS for local dev). No other HTTP framework.

**Middleware stack:**
1. Request ID injection
2. zap request logger (method, path, status, duration)
3. Panic recovery → 500 response
4. CORS: `Access-Control-Allow-Origin: *` in dev (disable in production)

## REST API endpoints

### GET /api/health

Returns service liveness.

```json
{"status": "ok", "version": "0.1.0"}
```

### GET /api/stats

Aggregate stats for the dashboard header strip. Queries GreptimeDB.

```json
{
  "total_traces_today": 142,
  "total_cost_today_usd": 3.84,
  "avg_eval_score": 0.76,
  "active_agents": 5
}
```

### GET /api/traces

Recent traces, grouped by `trace_id`. Queries GreptimeDB.

**Query params:**
| Param | Type | Default | Description |
|---|---|---|---|
| `agent_name` | string | `""` | Filter by agent |
| `environment` | string | `""` | Filter by environment |
| `start` | int64 | now-24h | Unix timestamp |
| `end` | int64 | now | Unix timestamp |
| `limit` | int | 50 | Max rows |
| `offset` | int | 0 | Pagination offset |

**Response:** array of trace summaries
```json
[{
  "trace_id": "abc-123",
  "agent_name": "research-agent",
  "span_count": 12,
  "total_cost_usd": 0.024,
  "avg_eval_score": 0.81,
  "max_span_size_bytes": 45000,
  "start_time": 1700000000.0,
  "end_time": 1700000012.3,
  "duration_ms": 12300.0
}]
```

### GET /api/traces/{trace_id}

All spans for a trace, sorted by `start_time`. Queries GreptimeDB.
Calculates `depth` from `parent_span_id` chain in application code (not SQL).

**Response:** array of span detail objects
```json
[{
  "span_id": "def-456",
  "parent_span_id": null,
  "depth": 0,
  "operation": "llm_call",
  "agent_name": "orchestrator",
  "model": "claude-sonnet-4-6",
  "duration_ms": 1200.0,
  "cost_usd": 0.0021,
  "input_tokens": 842,
  "output_tokens": 310,
  "prompt_preview": "Summarise the following...",
  "completion_preview": "Here is a summary...",
  "tool_name": null,
  "tool_error": null,
  "eval_score": 0.87,
  "prompt_ref": "abc-123/def-456/prompt",
  "completion_ref": "abc-123/def-456/completion",
  "prompt_size_bytes": 3412,
  "completion_size_bytes": 1204
}]
```

### GET /api/spans/{span_id}/payload

Lazy load endpoint — called only when a user explicitly clicks "Load full
content" in the UI. Fetches from S3 via `internal/storage`.

**Query params:**
| Param | Values |
|---|---|
| `field` | `prompt` \| `completion` \| `tool_input` \| `tool_output` |

The server resolves the `*_ref` field for the given span from GreptimeDB, then
fetches the full content from S3.

**Response:**
```json
{"field": "prompt", "content": "...full text of the prompt..."}
```

**Error responses:**
- `404` if span_id not found
- `404` if ref for requested field is null (no content uploaded)
- `502` if S3 fetch fails

### GET /api/search

Full-text search over prompts and completions. Queries Doris using
`MATCH_ALL` on the inverted indexes.

**Query params:**
| Param | Type | Default | Description |
|---|---|---|---|
| `q` | string | required | Search term |
| `agent_name` | string | `""` | Filter |
| `start` | int64 | now-7d | Unix timestamp |
| `end` | int64 | now | Unix timestamp |
| `limit` | int | 50 | Max rows |
| `offset` | int | 0 | Pagination |

**Response:** array of search hits
```json
[{
  "trace_id": "abc-123",
  "span_id": "def-456",
  "agent_name": "research-agent",
  "start_time": 1700000000.0,
  "prompt_preview": "...snippet...",
  "completion_preview": "...snippet...",
  "eval_score": 0.87
}]
```

### GET /api/justifications

Recent human annotation justifications — feeds the judge improvement pipeline.
Queries Doris where `operation = "human_annotation"`.

**Query params:** `agent_name`, `limit` (default 100)

**Response:** array of annotation events (same fields as trace response but
includes `eval_reason` as the justification text).

### POST /api/annotate

Submit a human annotation. Validates input and emits to Redpanda.

**Request body:**
```json
{
  "span_id": "def-456",
  "trace_id": "abc-123",
  "score": 0.9,
  "label": "good",
  "justification": "The agent correctly cited the source document and..."
}
```

**Validation:**
- `score`: float, 0.0–1.0
- `label`: one of `"good"`, `"acceptable"`, `"bad"`
- `justification`: string, min 20 chars

**Kafka emit (on success):**
```
operation      = "human_annotation"
trace_id       = from body
span_id        = new UUID4
parent_span_id = body.span_id
eval_score     = body.score
eval_label     = body.label
eval_reason    = body.justification
eval_source    = "human_annotation"
```

**Response:** `{"ok": true}` or `{"error": "validation message"}`

## UI (services/ui/)

Four HTML pages. Plain HTML + CSS + vanilla JavaScript. No framework.
`fetch()` calls to the REST API. Must work for non-technical domain experts
(lawyers, clinicians, compliance officers). Responsive for laptop screens.

### ui/embed.go

```go
//go:embed *.html static/
var StaticFiles embed.FS
```

### ui/index.html — Trace list + search

**Top bar:**
- Cogent logo + name
- Search input → `GET /api/search?q=...` on submit
- `agent_name` filter dropdown (populated from `GET /api/traces` distinct values)
- Date range picker (start/end, defaults to last 24 h)
- Stats strip: total cost today | avg eval score | active agents

**Search results** appear below the stats strip when a search is active. Each
result links to `trace.html#{trace_id}`.

**Main table — recent traces:**

| Column | Source field |
|---|---|
| Trace ID (truncated, clickable) | `trace_id` |
| Agent | `agent_name` |
| Spans | `span_count` |
| Cost | `total_cost_usd` |
| Avg score | `avg_eval_score` (coloured) |
| Max span size | `max_span_size_bytes` |
| Duration | `duration_ms` |
| When | `start_time` as "3 min ago" |

Row click → `trace.html#{trace_id}`. Pagination: prev/next.

Auto-refreshes every 30 seconds via `setInterval`.

### ui/trace.html — Span timeline

On load: reads `trace_id` from URL hash → `GET /api/traces/{trace_id}`.

**Span tree** rendered as indented table. `depth` field drives left indent
(16 px per level) + vertical CSS connector lines. No canvas required.

**Columns:** depth+operation (indented), agent_name, model/tool, duration ms,
tokens, cost, size (prompt+completion bytes), eval score (coloured), time.

**Row expand (click):**
1. Shows `prompt_preview` and `completion_preview` immediately (already in
   response, zero extra network calls)
2. "Load full content" button below preview
3. On click: `GET /api/spans/{span_id}/payload?field=prompt` and
   `?field=completion` in parallel → replaces preview with full text
4. Button changes to "Collapse"

Tool call spans show `tool_input_preview` / `tool_output_preview` with their
own "Load full" buttons.

Error spans (`tool_error` set) → entire row highlighted red.

Each `llm_call` row has an "Annotate" button → opens
`annotate.html#{span_id},{trace_id}`.

Back button → `index.html`.

### ui/span.html — Full span detail

Standalone shareable page for a single span.
- Loads full metadata + previews on page load
- Full content lazy-loaded on click (same as trace.html)
- Shows eval history for this span (all `operation=evaluation` events where
  `parent_span_id = this span_id`)
- Shows all human annotations for this span
- Copyable S3 ref keys for debugging

### ui/annotate.html — Human annotation form

Designed for domain experts — minimal, unambiguous.
Reads `span_id` and `trace_id` from URL hash (format: `#{span_id},{trace_id}`).

**Displays:**
- Agent name, timestamp, model
- Prompt preview + "Load full" button
- Completion preview + "Load full" button

**Form:**
- Score: slider 0–10 (mapped to 0.0–1.0 on submit)
- Label: radio buttons — Good / Acceptable / Bad
- Justification: textarea, required, min 20 characters enforced client-side
- Submit button

On submit: `POST /api/annotate`. On success: "Your annotation has been
recorded." + link back to the trace.

No authentication required for MVP. Add auth (basic HTTP auth or token) later
without changing the form structure.

### ui/static/app.css

Clean, minimal design. Dark sidebar + white main area.

- Monospace font for span IDs and content previews
- Eval score colours: green ≥ 0.8, yellow ≥ 0.5, red < 0.5
- Depth indentation: 16 px per level, left border line (CSS only)
- No external CSS framework

### ui/static/app.js

Shared utilities — no page-specific logic in this file.

```javascript
formatDuration(ms)         // "1.2s" or "340ms"
formatBytes(n)             // "1.2 MB" or "340 KB"
formatCost(usd)            // "$0.0021"
timeAgo(unixTs)            // "3 minutes ago"
fetchJSON(url, opts)       // fetch() wrapper with error handling

lazyLoadPayload(spanId, field, targetEl)
  // GET /api/spans/{spanId}/payload?field={field}
  // shows spinner, replaces targetEl innerHTML with full content
```

Each HTML page has its own inline `<script>` for page-specific logic. `app.js`
is shared utilities only.

## Go dependencies (services/go.mod)

```
module github.com/cogent/services

require (
    github.com/go-chi/chi/v5              v5+
    github.com/segmentio/kafka-go         v0.4+
    github.com/go-sql-driver/mysql        v1.7+
    github.com/minio/minio-go/v7          v7+
    github.com/sashabaranov/go-openai     v1+
    go.uber.org/zap                       v1.27+
    github.com/google/uuid                v1.6+
    golang.org/x/time                     latest  (rate limiter)
)
```
