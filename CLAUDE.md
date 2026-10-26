# Cogent — Claude Code Project Map

Stream-first LLM agent observability. Python SDK → Kafka → Go consumers → GreptimeDB + Doris → REST API + UI.

## Directory map

```
cogent/sdk/            Python SDK (AgentTelemetry, PayloadOffloader, @observe)
services/
  cmd/*/main.go        Six Go binaries — thin wiring only, ~50-80 lines each
  internal/            All shared Go packages (schema, config, consumer, greptime, doris, storage, judge, alert)
examples/              Runnable demos — run after `make up`
deploy/                Docker Compose, DDL, Grafana dashboards, judge prompt
docs/user-guide.adoc   Full user guide (AsciiDoc)
arch/                  Architecture design docs (overview, sdk, infra, judge, server, services)
tests/python/          Python SDK unit tests (no services needed)
```

## Build / test

```bash
make build-go        # → ./bin/{consumer-greptime,consumer-doris,consumer-judge,consumer-alerting,server,eval}
make build-python    # pip install -e .[sdk]
make test-go         # cd services && go test ./...
make test-python     # pytest tests/python/
make up              # full Docker stack on localhost
make release VERSION=x.y.z  # bump + commit + tag + push
```

## Rules (see `.claude/rules/` for each)

| File | Covers |
|---|---|
| [`folders.md`](.claude/rules/folders.md) | What every top-level folder is and what lives inside it |
| [`kafka.md`](.claude/rules/kafka.md) | Dual-listener setup, why port 19092 vs 9092, Docker override pattern |
| [`storage.md`](.claude/rules/storage.md) | Always-split payload model — never store raw content in DB |
| [`search.md`](.claude/rules/search.md) | Doris `SEARCH()` DSL syntax, GreptimeDB LIKE fallback |
| [`sdk.md`](.claude/rules/sdk.md) | Schema sync contract, payload offload, trace ID propagation, Kafka flush |
| [`judge.md`](.claude/rules/judge.md) | What gets scored, prompt template vars, required JSON keys, config precedence |
| [`server.md`](.claude/rules/server.md) | Single binary + embedded UI, json tags, annotation Kafka flow, server struct |

## Git / release workflow

- **Never run `git commit`, `git tag`, or `git push` yourself.** Make the file changes, run the tests, and stop — the user commits and releases.
- CVE bump flow: `cd services && go get <module>@<fixed-version> && go mod tidy`, then `go build ./... && go test ./...`. Leave `go.mod`/`go.sum` uncommitted.
- Releasing is the user's step: `make release VERSION=x.y.z` (bump + commit + tag + push, triggers GitHub Actions).
