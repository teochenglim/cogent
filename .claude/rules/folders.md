# Folder map

| Folder | What it is |
|---|---|
| `cogent/sdk/` | Python SDK — `schema.py` (AgentEvent), `offload.py` (PayloadOffloader), `telemetry.py` (AgentTelemetry + _Span), `decorators.py` (@observe) |
| `services/cmd/` | Six Go binaries — each `main.go` is thin wiring only. `consumer-greptime`, `consumer-doris`, `consumer-judge`, `consumer-alerting`, `server`, `eval` |
| `services/internal/` | Shared Go packages — `schema`, `config`, `consumer` (base loop), `greptime` (writer + queries), `doris` (writer + queries), `storage` (MinIO), `judge`, `alert` |
| `services/ui/` | Static HTML/CSS/JS embedded into the `server` binary at build time via `go:embed` |
| `examples/` | Runnable Python demos — `single_agent.py`, `multi_agent.py`. Run after `make up` to emit real traces |
| `deploy/` | Docker Compose, DDL (`schema/greptime.sql`, `schema/doris.sql`), Grafana dashboards, init scripts, judge prompt |
| `docs/` | User-facing documentation in AsciiDoc — `user-guide.adoc` |
| `arch/` | Human-readable design docs — original design intent, trade-off rationale, technology choices |
| `tests/python/` | Python SDK unit tests — no running services needed, all mocked |
| `bin/` | Compiled Go binaries (git-ignored, output of `make build-go`) |
