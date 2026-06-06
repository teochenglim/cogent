# Cogent — Distributed LLM Agent Observability

## What Cogent is

A stream-first observability platform for distributed LLM agents.
No ClickHouse. No proprietary platforms. Apache 2.0 licensed.

**Name:** Cogent means "clear, logical, and convincing." It describes what an
agent's trace should be. The "Cog" implies the gears of a system turning.

## Design principles

**Stream-first.** Agents emit one structured event per action onto Redpanda
(Kafka-compatible) and move on immediately. No synchronous coupling to any
observability backend. If the backend is down the agent does not feel it.

**Log-trace unification.** Each event carries trace_id, span_id, and
parent_span_id. The event IS the span. No separate span backend. No OTel
collector required.

**Always-split payload model.** Prompt, completion, tool_input, and
tool_output are ALWAYS split into preview (first 500 chars stored inline in DB)
and full payload (always written to S3). No threshold logic. No "sometimes
inline sometimes not". DB rows are always narrow and predictably sized. UI
lazy-loads full content on click.

Phil Hetzel (Braintrust) measured 1 GB traces and 20 MB single spans in
production agent workloads. The always-split model handles this without DB row
bloat or query performance degradation.

**Two-tier storage.** GreptimeDB is the hot tier (metadata, numerics, previews,
real-time dashboard). Doris is the warm tier (full content, full-text search via
native inverted index, governance, batch eval). Doris 4.0 inverted index on text
columns is the reason ClickHouse was abandoned at Braintrust — ClickHouse could
not do text-based indexing at this scale. Doris handles it natively.

**Metrics are queries.** Token counts, costs, latency, eval scores are
structured fields. Metrics dashboards are SQL GROUP BY. Nothing is
pre-aggregated or emitted separately.

**Evals and observability share one data model.** Production traces and batch
eval runs use the same event schema, the same topic, and the same backends. The
only difference is the eval_source field value. This is Phil Hetzel's core
insight: observability runs real-time with unknown inputs, evals run batch with
known inputs. Same problem.

## Technology choices

**Python SDK.** Agents are written in Python. The instrumentation library must
be Python. Kept minimal — schema, Kafka emit, preview split, S3 upload. Zero
heavy dependencies.

**Go services (everything else).** All consumers, the API server, the
annotation UI server, and the batch eval CLI are written in Go.
- Kafka consumers are long-running, high-throughput processes. Python GIL makes
  concurrent batch processing single-threaded. Goroutines handle fan-out cleanly.
- Single static binary per service. Trivial to containerise. No pip, no
  virtualenv, no dependency hell in containers.
- MySQL driver (go-sql-driver/mysql) works identically against GreptimeDB and
  Doris MySQL protocol.
- minio-go SDK is better maintained than Python equivalent.
- net/http + embedded static files = self-contained UI binary, same pattern as
  Prometheus, Grafana Agent, and MapWatcher.

**UI embedded in Go binary.** Static HTML/CSS/JS files are embedded into the Go
binary using go:embed. The annotation and trace viewer UI is served directly
from the binary with no separate web server or CDN needed. UI fetches data from
the same binary's REST API endpoints. Same pattern as Prometheus UI. Deploy one
binary, get both API and UI.

## Stack

```
Transport:     Redpanda (Kafka-compatible, default)
               AutoMQ = production upgrade path (S3-native,
               cheaper at scale, Kafka-compatible, change
               BOOTSTRAP_SERVERS only)
Object store:  MinIO (local) / any S3-compatible (production)
               all content payloads always written here
Hot tier:      GreptimeDB (metadata + previews, real-time queries)
Warm tier:     Apache Doris (full content, FTS, governance)
Judge LLM:     Any OpenAI-compatible endpoint (Ollama default)
UI + API:      Go binary with embedded static files
Dashboards:    Grafana (GreptimeDB + Doris datasources)
```

## Architecture

```
Python Agent ──► Redpanda ──► consumer-greptime ──► GreptimeDB ──┐
               (one event       consumer-doris  ──► Doris        ├──► Grafana
                per action)     consumer-judge  ──► Redpanda     │
                     │          consumer-alert  ──► stdout       │
                     │                                           │
               MinIO/S3 ◄── sdk offload                          │
                     │                                           │
                     └──► server (Go binary) ──► UI port 8090    ┘
                          REST API + embedded HTML
                          lazy loads S3 on click
```

## Repository structure

```
cogent/
├── sdk/                        # Python instrumentation library
│   ├── __init__.py
│   ├── telemetry.py            # AgentTelemetry + _Span
│   ├── schema.py               # Pydantic event model
│   ├── offload.py              # Preview split + S3 upload
│   └── decorators.py           # @observe() decorator
│
├── services/                   # Go services (one Go module)
│   ├── go.mod
│   ├── go.sum
│   ├── cmd/
│   │   ├── consumer-greptime/main.go
│   │   ├── consumer-doris/main.go
│   │   ├── consumer-judge/main.go
│   │   ├── consumer-alerting/main.go
│   │   ├── server/main.go      # API + embedded UI binary
│   │   └── eval/main.go        # Batch eval CLI
│   ├── internal/
│   │   ├── config/             # Env var + flag loading, shared config
│   │   ├── schema/             # Go struct mirroring Python schema
│   │   ├── consumer/           # Base consumer (poll loop, batch buffer,
│   │   │                       # graceful shutdown)
│   │   ├── greptime/           # GreptimeDB writer
│   │   ├── doris/              # Doris Stream Load + MySQL fallback
│   │   ├── storage/            # MinIO/S3 client (fetch full payload)
│   │   ├── judge/              # OAI-compatible judge logic + prompt loader
│   │   └── alert/              # Alert logic + webhook
│   └── ui/                     # Embedded static files
│       ├── embed.go
│       ├── index.html
│       ├── trace.html
│       ├── span.html
│       ├── annotate.html
│       └── static/
│           ├── app.css
│           └── app.js
│
├── deploy/
│   ├── docker-compose.yml
│   ├── grafana/
│   │   ├── datasources.yml
│   │   └── dashboards/agent-overview.json
│   ├── schema/
│   │   ├── greptime.sql
│   │   └── doris.sql
│   ├── prompts/
│   │   └── judge_default.txt   # Default judge prompt (replace per domain)
│   └── scripts/
│       ├── doris-init.sh
│       └── redpanda-init.sh
│
├── examples/
│   ├── single_agent.py
│   ├── multi_agent.py
│   └── langgraph_example.py
│
├── tests/
│   ├── python/
│   │   ├── test_telemetry.py
│   │   ├── test_offload.py
│   │   └── test_schema.py
│   └── go/
│       ├── consumer_test.go
│       ├── greptime_test.go
│       ├── doris_test.go
│       ├── judge_test.go
│       └── alert_test.go
│
├── .env.example
├── README.md
├── pyproject.toml
└── Makefile
```

## Detailed design documents

| Document | Covers |
|---|---|
| [DESIGN_SDK.md](DESIGN_SDK.md) | Python SDK — schema, offload, telemetry, decorators, tests |
| [DESIGN_SERVICES.md](DESIGN_SERVICES.md) | Go consumers (greptime, doris, alerting) + base consumer |
| [DESIGN_JUDGE.md](DESIGN_JUDGE.md) | Judge consumer, prompt design, runtime config overrides, batch eval CLI |
| [DESIGN_SERVER.md](DESIGN_SERVER.md) | REST API server + embedded UI |
| [DESIGN_INFRA.md](DESIGN_INFRA.md) | DDL, docker-compose, Grafana, Makefile, deployment scripts |

## Runtime configuration model

All services follow `CLI flag > environment variable > default` precedence.
Details per service in [DESIGN_JUDGE.md](DESIGN_JUDGE.md) (judge, most
overridable) and [DESIGN_INFRA.md](DESIGN_INFRA.md) (.env.example reference).

## Production upgrade paths

| Component | Dev | Production |
|---|---|---|
| Transport | Redpanda | AutoMQ (change BOOTSTRAP_SERVERS only) |
| Object store | MinIO | AWS S3 / GCS (change endpoint + creds) |
| Hot tier | GreptimeDB standalone | GreptimeDB cluster |
| Warm tier | Single Doris FE+BE | Doris cluster |

## Comparison

| Feature | Cogent | Langfuse | Phoenix | Braintrust |
|---|---|---|---|---|
| ClickHouse | No | Yes | No | No (custom) |
| 1 GB+ traces | Yes (S3 offload) | No | No | Yes |
| Full-text search | Doris inv. index | No | No | Tantivy |
| Agent coupling | None (async) | Sync API | Sync | Sync |
| Human annotation | Yes | Yes | Basic | Yes (core) |
| Evals + obs unified | Yes | Partial | Partial | Yes (core) |
| Lazy load UI | Yes | No | No | Yes |
| Offline replay | Kafka offset | No | No | No |
| Licence | Apache 2.0 | MIT | ELv2 | Proprietary |
| UI deployment | Single binary | 6 containers | 1 container | SaaS |

## Implementation constraints

- Go 1.22+, Python 3.11+
- All Go services built from `services/` as a single module, multi-stage Docker
- No hardcoded values anywhere — every value comes from config
- zap logger throughout Go code, not fmt.Println
- Python `logging` module throughout, not print
- Every exported Go function/type has a godoc comment
- Every Python public class/method has a docstring
