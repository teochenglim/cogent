# Cogent Go Services — Consumers

## Overview

All Go services share one module at `services/`. They are built into separate
binaries from `cmd/` subdirectories. A single multi-stage Dockerfile builds all
binaries; docker-compose selects which binary to run per container.

**Go version:** 1.22+

**Dependencies:**
```
github.com/segmentio/kafka-go       v0.4+   Kafka consumer + producer
github.com/go-sql-driver/mysql      v1.7+   GreptimeDB + Doris MySQL protocol
github.com/minio/minio-go/v7        v7+     S3/MinIO client
github.com/sashabaranov/go-openai   v1+     OAI-compatible judge calls
go.uber.org/zap                     v1.27+  Structured logging
github.com/google/uuid              v1.6+   UUID generation
```

`net/http` is used for Doris Stream Load and the server — no external HTTP
framework needed unless routing complexity warrants `chi`.

## internal/config/config.go — Shared configuration

All services load config via `internal/config`. The loader follows
`CLI flag > environment variable > default` precedence.

```go
// Config holds all runtime configuration for Cogent services.
// Values are resolved: CLI flag overrides env var overrides built-in default.
type Config struct {
    // Kafka / Redpanda
    BootstrapServers string // BOOTSTRAP_SERVERS, default "localhost:9092"
    Topic            string // TOPIC, default "cogent-telemetry"

    // MinIO / S3
    MinioEndpoint  string // MINIO_ENDPOINT
    MinioAccessKey string // MINIO_ACCESS_KEY, default "minioadmin"
    MinioSecretKey string // MINIO_SECRET_KEY, default "minioadmin"
    MinioBucket    string // MINIO_BUCKET, default "cogent-payloads"
    MinioSecure    bool   // MINIO_SECURE, default false

    // GreptimeDB
    GreptimeHost     string // GREPTIME_HOST, default "localhost"
    GreptimePort     int    // GREPTIME_PORT, default 4002
    GreptimeDatabase string // GREPTIME_DATABASE, default "public"

    // Doris
    DorisFEHost     string // DORIS_FE_HOST, default "localhost"
    DorisFEHTTPPort int    // DORIS_FE_HTTP_PORT, default 8030
    DorisMySQLPort  int    // DORIS_MYSQL_PORT, default 9030
    DorisUser       string // DORIS_USER, default "root"
    DorisPassword   string // DORIS_PASSWORD, default ""
    DorisDatabase   string // DORIS_DATABASE, default "cogent"

    // Judge (see DESIGN_JUDGE.md for full judge config)
    JudgeBaseURL    string  // JUDGE_BASE_URL, default "http://localhost:11434/v1"
    JudgeModel      string  // JUDGE_MODEL, default "llama3.2"
    JudgeAPIKey     string  // JUDGE_API_KEY, default "ollama"
    JudgeRPS        float64 // JUDGE_RPS, default 2
    JudgePromptFile string  // JUDGE_PROMPT_FILE, default ""

    // Server
    ServerPort         string // SERVER_PORT, default "8090"
    ServerReadTimeout  string // SERVER_READ_TIMEOUT, default "30s"
    ServerWriteTimeout string // SERVER_WRITE_TIMEOUT, default "60s"

    // Alerting
    AlertMaxSpansPerTrace int     // ALERT_MAX_SPANS_PER_TRACE, default 200
    AlertCostBudgetUSD    float64 // ALERT_COST_BUDGET_USD, default 1.0
    AlertMaxSpanBytes     int64   // ALERT_MAX_SPAN_BYTES, default 52428800 (50 MB)
    AlertWindowSeconds    int     // ALERT_WINDOW_SECONDS, default 60
    AlertWebhookURL       string  // ALERT_WEBHOOK_URL, default ""
}

// Load reads the config from environment variables.
// Call once at startup, then pass Config by value to each subsystem.
func Load() Config

// BindFlags registers CLI flags onto an existing *Config and a *flag.FlagSet.
// Caller calls flag.Parse() after BindFlags, then Load() merges env + flags.
// Flags override env vars.
func BindFlags(cfg *Config, fs *flag.FlagSet)
```

## internal/schema/event.go — Go event struct

Mirrors the Python `AgentEvent` schema exactly. JSON tags match Python field
names. Used by all consumers and the API server.

```go
// Event mirrors the Python AgentEvent schema.
// All pointer fields are optional (nil == not set).
type Event struct {
    TraceID      string  `json:"trace_id"`
    SpanID       string  `json:"span_id"`
    ParentSpanID *string `json:"parent_span_id"`
    StartTime    float64 `json:"start_time"`
    EndTime      float64 `json:"end_time"`
    DurationMs   float64 `json:"duration_ms"`

    AgentName   string `json:"agent_name"`
    Operation   string `json:"operation"`
    ServiceName string `json:"service_name"`
    Environment string `json:"environment"`

    Model        *string  `json:"model"`
    Provider     *string  `json:"provider"`
    InputTokens  *int32   `json:"input_tokens"`
    OutputTokens *int32   `json:"output_tokens"`
    CostUSD      *float64 `json:"cost_usd"`
    FinishReason *string  `json:"finish_reason"`

    PromptPreview     *string `json:"prompt_preview"`
    CompletionPreview *string `json:"completion_preview"`
    ToolInputPreview  *string `json:"tool_input_preview"`
    ToolOutputPreview *string `json:"tool_output_preview"`

    PromptRef     *string `json:"prompt_ref"`
    CompletionRef *string `json:"completion_ref"`
    ToolInputRef  *string `json:"tool_input_ref"`
    ToolOutputRef *string `json:"tool_output_ref"`

    PromptSizeBytes     *int64 `json:"prompt_size_bytes"`
    CompletionSizeBytes *int64 `json:"completion_size_bytes"`
    ToolInputSizeBytes  *int64 `json:"tool_input_size_bytes"`
    ToolOutputSizeBytes *int64 `json:"tool_output_size_bytes"`

    ToolName  *string `json:"tool_name"`
    ToolError *string `json:"tool_error"`

    EvalScore  *float64 `json:"eval_score"`
    EvalLabel  *string  `json:"eval_label"`
    EvalReason *string  `json:"eval_reason"`
    EvalSource *string  `json:"eval_source"`

    Metadata map[string]any `json:"metadata"`
}

// FromJSON parses a JSON byte slice into an Event.
func FromJSON(data []byte) (Event, error)

// ToJSON serialises an Event to JSON bytes.
func (e Event) ToJSON() ([]byte, error)
```

## internal/consumer/base.go — Base consumer

Shared poll loop used by all four consumer commands. Each consumer provides a
`Handler` implementation; the base handles Kafka mechanics.

```go
// Handler processes a single decoded event.
// Implementations must be safe to call from a single goroutine.
// Return a non-nil error to trigger retry/logging; the loop never crashes.
type Handler interface {
    Handle(ctx context.Context, event schema.Event) error
    // Flush is called when the batch is full or the flush interval fires.
    // Implementations write their buffered batch here.
    Flush(ctx context.Context) error
}

// BaseConsumer manages the Kafka poll loop, batching, and graceful shutdown.
type BaseConsumer struct {
    bootstrapServers string
    topic            string
    groupID          string
    batchSize        int           // flush when buffer reaches this count
    flushInterval    time.Duration // flush at least this often regardless
    handler          Handler
    logger           *zap.Logger
}

// Run starts the poll loop. Blocks until ctx is cancelled or SIGTERM received.
// On shutdown: drains the buffer, commits offsets, closes the Kafka reader.
func (b *BaseConsumer) Run(ctx context.Context)
```

**Error policy:** Failed `Handle()` calls are logged with zap at error level.
The event is skipped (dead-lettered to stderr). The loop never crashes.
Exponential backoff (max 30 s) on repeated `Flush()` errors.

**Batch flushing:** Buffer holds up to `batchSize` events (default per
consumer). A `time.Ticker` fires every `flushInterval` and flushes whatever
is buffered. Both thresholds always active simultaneously.

## cmd/consumer-greptime — Hot tier writer

**Group ID:** `cogent-greptime`

Writes **only** metadata and preview fields to GreptimeDB. Never writes full
content columns and never fetches from S3. Rows stay narrow for fast time-series
queries.

**Batch:** 500 events or 2 seconds, whichever comes first.

**Connection:** `go-sql-driver/mysql` → GreptimeDB port 4002 (MySQL protocol).

**Columns written** (all correlation, agent context, LLM metadata, all preview
fields, all ref fields, all size_bytes fields, tool metadata, eval fields):

```sql
INSERT INTO agent_telemetry (
    ts, trace_id, span_id, parent_span_id,
    start_time, end_time, duration_ms,
    agent_name, operation, service_name, environment,
    model, provider, input_tokens, output_tokens, cost_usd, finish_reason,
    prompt_preview, prompt_ref, prompt_size_bytes,
    completion_preview, completion_ref, completion_size_bytes,
    tool_name,
    tool_input_preview, tool_input_ref, tool_input_size_bytes,
    tool_output_preview, tool_output_ref, tool_output_size_bytes,
    tool_error,
    eval_score, eval_label, eval_source
) VALUES (?, ?, ?, ?, ...)
```

Retry: exponential backoff, max 5 attempts, then log and skip.

## cmd/consumer-doris — Warm tier writer

**Group ID:** `cogent-doris`

Writes **all** fields to Doris including full content. For each event with any
`*_ref` field set: fetches full content from S3 via `internal/storage` before
writing to Doris. Doris is the content store and full-text search engine.

**Batch:** 1000 events or 5 seconds.

**Primary write path — HTTP Stream Load:**
```
POST http://{DORIS_FE_HOST}:{DORIS_FE_HTTP_PORT}/api/cogent/agent_telemetry/_stream_load
Headers:
  format: json
  strip_outer_array: true
  Authorization: Basic base64(user:password)
Body: JSON array of event rows
```

**Fallback write path — MySQL INSERT:** if Stream Load returns non-200, fall
back to `go-sql-driver/mysql` INSERT. Log the Stream Load error before
switching.

**S3 fetch:** for each `*_ref` that is non-nil, call `internal/storage.Fetch()`
to retrieve the full payload. Set the full content column (`prompt`, `completion`,
`tool_input`, `tool_output`) in the Doris row. If S3 fetch fails, write the row
with `NULL` for that content column and log a warning.

## cmd/consumer-alerting — Real-time alerting

**Group ID:** `cogent-alerting`

Maintains an in-memory rolling window using `sync.Map`. Keyed by `(trace_id,
agent_name)` composite key. TTL = `ALERT_WINDOW_SECONDS` (default 60 s). A
background goroutine sweeps expired keys every 10 s.

**Three alert conditions:**

| Alert | Condition | Config env var | Default |
|---|---|---|---|
| Runaway loop | span count per trace_id > threshold | `ALERT_MAX_SPANS_PER_TRACE` | 200 |
| Cost spike | sum(cost_usd) per agent_name > budget | `ALERT_COST_BUDGET_USD` | $1.00 |
| Payload size | any single span sum(size_bytes) > limit | `ALERT_MAX_SPAN_BYTES` | 50 MB |

**Alert output:**
- Structured JSON to stdout (Loki-ingestible via Promtail or Vector)
- Optional HTTP POST to `ALERT_WEBHOOK_URL` if set (JSON body, best-effort,
  no retry — alerting must not block the consumer loop)

**Does not batch** — alerts are evaluated per-event in real time with no buffer
delay. Event processing is still asynchronous from the agent's perspective
because the consumer group is independent from the agent's Kafka producer.

## internal/storage/storage.go — S3/MinIO client

```go
// Client wraps minio-go for fetching full content payloads from S3.
type Client struct { ... }

// NewClient creates a Client from config. Returns a no-op client if
// cfg.MinioEndpoint is empty (graceful degradation in local dev).
func NewClient(cfg config.Config) (*Client, error)

// Fetch downloads and returns the full content for a given S3 ref key.
// ref_key format: "{trace_id}/{span_id}/{field_name}"
func (c *Client) Fetch(ctx context.Context, key string) (string, error)

// Put uploads content to S3. Used by the Python SDK equivalent in tests.
func (c *Client) Put(ctx context.Context, key string, content string) error
```

## internal/greptime/writer.go — GreptimeDB batch writer

```go
// Writer holds a *sql.DB connection to GreptimeDB and buffers rows.
type Writer struct { ... }

// NewWriter creates a Writer from config using go-sql-driver/mysql.
func NewWriter(cfg config.Config) (*Writer, error)

// Buffer adds an event to the pending batch.
func (w *Writer) Buffer(e schema.Event)

// Flush writes the buffered batch as a single prepared-statement INSERT.
// Clears the buffer on success. Returns an error (caller retries).
func (w *Writer) Flush(ctx context.Context) error
```

**Invariant:** `Flush` NEVER writes `prompt`, `completion`, `tool_input`, or
`tool_output` columns. A linter test in `greptime_test.go` verifies this by
inspecting the generated SQL.

## internal/doris/writer.go — Doris Stream Load writer

```go
// Writer manages HTTP Stream Load with MySQL fallback for Doris.
type Writer struct { ... }

// NewWriter creates a Writer from config.
func NewWriter(cfg config.Config) (*Writer, error)

// Buffer adds a fully-hydrated event (with full content from S3) to the batch.
func (w *Writer) Buffer(e schema.Event)

// Flush sends the batch via Stream Load. Falls back to MySQL INSERT on failure.
func (w *Writer) Flush(ctx context.Context) error
```

## tests/go/consumer_test.go

- Batch flush fires when count threshold is reached
- Batch flush fires when time threshold fires (mock clock)
- Graceful shutdown drains the buffer before exiting
- Failed `Handle()` logs error at error level and continues — loop does not crash
- Exponential backoff engaged on repeated `Flush()` errors

## tests/go/greptime_test.go

- NEVER writes `prompt`, `completion`, `tool_input`, `tool_output` columns
  (inspect the INSERT SQL string — fail if any of these column names appear)
- Writes all preview and ref fields correctly
- Batch INSERT uses the correct column list

## tests/go/doris_test.go

- Fetches S3 content before writing when `*_ref` is non-nil
- Falls back to MySQL INSERT when Stream Load returns non-200
- Batch size and flush interval are both respected
- S3 fetch failure writes NULL content column and logs warning (does not skip row)

## tests/go/alert_test.go

- Runaway loop fires at the correct span count threshold
- Cost spike fires at the correct cost_usd threshold
- Payload size alert fires when sum of size_bytes fields exceeds limit
- Rolling window expires old events correctly (synthetic clock)
- `ALERT_WEBHOOK_URL` POST is attempted when set; failure does not block loop
