# Architecture

Design documents for Cogent. These reflect the original design intent and are a useful reference for understanding why specific choices were made.

| Document | Contents |
|---|---|
| [overview.md](overview.md) | Design principles, technology choices, data model, trade-offs |
| [sdk.md](sdk.md) | Python SDK — schema, payload split model, telemetry, decorators |
| [infra.md](infra.md) | Infrastructure — Redpanda, GreptimeDB, Doris, MinIO, Docker Compose |
| [services.md](services.md) | Go consumers — base consumer, greptime writer, doris writer, alerting |
| [judge.md](judge.md) | LLM judge consumer — scoring, batch eval, prompt format |
| [server.md](server.md) | REST API server — endpoints, embedded UI, search routing |
