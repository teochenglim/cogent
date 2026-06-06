# Contributing to Cogent

## Reading the code

Start here to understand the system end-to-end:

1. **[arch/overview.md](arch/overview.md)** — design principles, why two databases, why Kafka
2. **[cogent/sdk/telemetry.py](cogent/sdk/telemetry.py)** — Python SDK: how a span becomes a Kafka message
3. **[services/internal/schema/event.go](services/internal/schema/event.go)** — the canonical event schema shared by all Go services
4. **[services/internal/consumer/base.go](services/internal/consumer/base.go)** — the base Kafka consumer all four consumers extend
5. **[services/cmd/server/main.go](services/cmd/server/main.go)** — REST API handlers and search routing

## Project layout

```
cogent/sdk/          Python SDK (AgentTelemetry, PayloadOffloader, @observe)
services/
  cmd/               Six Go binaries — each main.go is < 60 lines
    consumer-greptime/
    consumer-doris/
    consumer-judge/
    consumer-alerting/
    server/
    eval/
  internal/          Shared Go packages
    schema/          Event struct + FromJSON/ToJSON
    config/          Config struct + flag/env loading
    consumer/        Base consumer loop
    greptime/        GreptimeDB writer + query layer
    doris/           Doris Stream Load writer + query layer
    storage/         MinIO/S3 client
    judge/           LLM scoring logic
    alert/           Alerting logic
examples/            Runnable Python examples (realistic, randomised payloads)
deploy/              Docker Compose + DDL + Grafana + judge prompt
docs/                User guide (AsciiDoc)
arch/                Architecture design docs
tests/python/        Python SDK tests (no running services required)
```

## Dev setup

```bash
# Clone and start infra
git clone https://github.com/cogent/cogent && cd cogent
cp .env.example .env
make infra-up        # starts Redpanda, MinIO, GreptimeDB, Doris, Grafana
                     # macOS+Colima: run `colima ssh -- sudo sysctl -w vm.max_map_count=2000000` first

# Build and test
make build-go        # six Go binaries → ./bin/
make build-python    # pip install -e .[sdk]
make test-go
make test-python
```

## Running a single service locally (against Docker infra)

```bash
make run-server              # API + UI on :8090
make run-consumer-greptime   # consume from Redpanda → GreptimeDB
```

## Code conventions

**Go:**
- All services share the `internal/` packages — add there, not in `cmd/`
- Each `cmd/*/main.go` should stay short (~50-80 lines): load config, wire dependencies, run
- New Kafka consumers: embed `consumer.BaseConsumer`, implement `consumer.Writer`
- JSON tags on all response structs — Go defaults to PascalCase which breaks the JS UI

**Python:**
- SDK stays zero-dependency (stdlib + kafka-python + boto3 only)
- All public SDK classes/functions need a docstring
- No `print` in library code; use `logging`

**Both:**
- No comments that explain what the code does — names should do that
- Only comment the non-obvious: hidden constraints, workarounds, subtle invariants

## Testing

All Go tests in `services/internal/` run without external dependencies (no Kafka, no databases). Tests use interfaces and fakes, not mocks of external services.

All Python tests in `tests/python/` use `unittest.mock` — no running services required.

## Submitting a PR

- One logical change per PR
- New Go consumers: add a test for the `Writer` implementation
- New API endpoints: add the handler to `server/main.go` and document in `docs/user-guide.adoc`
- New SDK fields: update `services/internal/schema/event.go`, `cogent/sdk/schema.py`, and the schema reference table in `docs/user-guide.adoc`
