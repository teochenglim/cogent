# Cogent

**Self-hosted observability for LLM agents.** Every call, every tool use, every handoff — streamed, stored, searchable. Apache 2.0.

---

## Why Cogent

You built an agent. Now it's in production and something is wrong — a loop that ran 400 times, a prompt that cost $12, a tool that silently returned garbage. You have no idea what happened.

Cogent gives you the trace. Three lines of instrumentation code. Zero coupling to your agent logic. If the observability stack goes down, your agents keep running — they emit fire-and-forget events and move on.

Built for the numbers that break SaaS tools: gigabyte traces, megabyte prompts, agents that call agents that call tools at 3 AM.

---

## Quickstart

```bash
git clone https://github.com/cogent/cogent && cd cogent
cp .env.example .env
make up
```

> **macOS + Colima:** run `colima ssh -- sudo sysctl -w vm.max_map_count=2000000` once after `colima start` — Doris needs it.

Then send some traces:

```bash
pip install -e ".[sdk]"
python examples/single_agent.py
python examples/multi_agent.py
```

Open **http://localhost:8090** — traces appear within seconds.

---

## What you get

```
Trace list                          Trace waterfall
─────────────────────────────       ───────────────────────────────────────
 Trace ID   Agent       Cost         orchestrator          420ms  $0.006
 a1b2c3..   risk-scorer $0.002       └─ data-fetcher       12ms   tool
 d4e5f6..   support-bot $0.001          └─ sentiment-analy 88ms   $0.001
 g7h8i9..   planner     $0.008          └─ risk-scorer     310ms  $0.002
                                                            ✓ score: 0.91
```

**Trace view** — every span in the run, indented by depth, with duration, cost, and eval badge.

**Span detail** — prompt/completion preview inline; click to load the full payload from S3.

**Full-text search** — `GET /api/search?q=acct_8821` finds every span that mentioned it.

**Human annotation** — score any span with a label, justification, and 0–1 score. Stored alongside LLM eval scores.

**Grafana dashboards** — token spend, cost per agent, latency percentiles, eval score trends. Available at `http://localhost:3000`.

---

## Instrument your agent in three lines

```python
from cogent.sdk import AgentTelemetry, PayloadOffloader

tel = AgentTelemetry(
    bootstrap_servers="localhost:19092",
    offloader=PayloadOffloader(endpoint="localhost:9000", bucket="cogent-payloads"),
)

with tel.span("llm_call", agent_name="my-agent") as span:
    response = llm.complete(prompt)
    span.log(
        prompt=prompt,
        completion=response.text,
        model="claude-sonnet-4-6",
        input_tokens=response.usage.input,
        output_tokens=response.usage.output,
        cost_usd=0.003,
    )
```

Child spans inherit `trace_id` automatically — no manual ID threading.

See [`docs/user-guide.adoc`](docs/user-guide.adoc) for the full SDK reference.

---

## How it works

```
Agent process
    │  span.log()  ← returns immediately, never blocks the agent
    ▼
Redpanda (Kafka-compatible)
    │
    ├──► consumer-greptime ──► GreptimeDB   time-series metadata, previews, costs
    ├──► consumer-doris    ──► Doris        full text, inverted-index search
    ├──► consumer-judge    ──► auto-scores every llm_call with an LLM judge
    └──► consumer-alerting ──► webhook on runaway loop / cost spike / oversized payload
    │
    └──► MinIO / S3 ◄── SDK always offloads full payloads (prompts, completions, tool I/O)
                                 DB rows stay narrow. UI lazy-loads on click.
    │
    └──► server :8090 ── trace list, waterfall, span detail, annotation, search
```

**Why two databases?**
GreptimeDB answers time-series queries fast (dashboard, trace list). Doris answers full-text search over full prompt/completion content with a native inverted index. Every event goes to both simultaneously via independent Kafka consumer groups.

**Why Kafka?**
Decouples your agent from the observability backend completely. Agent emits and moves on. Consumer backlogs during restarts, catches up, no data loss. You can add new consumers (alerting, eval, audit) without touching agent code.

---

## Stack

| Component | Purpose | Local default | Production swap |
|---|---|---|---|
| Redpanda | Kafka-compatible event bus | `localhost:19092` | AutoMQ (S3-native, same protocol) |
| MinIO | Payload object store | `localhost:9000` | AWS S3, GCS |
| GreptimeDB | Hot tier — time-series queries | `localhost:4002` | GreptimeDB cluster |
| Apache Doris | Warm tier — full-text search | `localhost:9030` | Doris cluster |
| Grafana | Dashboards | `localhost:3000` | — |

---

## The judge

`consumer-judge` scores every `llm_call` event automatically using any OpenAI-compatible endpoint:

```bash
./bin/consumer-judge \
  --judge-model gpt-4o-mini \
  --judge-base-url https://api.openai.com/v1 \
  --judge-api-key sk-... \
  --judge-prompt-file /path/to/your_rubric.txt
```

The default rubric scores relevance, faithfulness, and safety. Replace it with a domain-specific rubric — see [`deploy/prompts/judge_default.txt`](deploy/prompts/judge_default.txt) and the [user guide](docs/user-guide.adoc#the-judge).

---

## Building and testing

```bash
make build-go      # six Go binaries → ./bin/
make build-python  # pip install -e .[sdk]
make test-go       # go test ./... in services/
make test-python   # pytest tests/python/
```

---

## Directories

| Path | What it is |
|---|---|
| `cogent/sdk/` | Python instrumentation library — `AgentTelemetry`, `PayloadOffloader`, `@observe` |
| `examples/` | Runnable demos — single agent and multi-agent pipeline |
| `services/` | Six Go binaries — four consumers, API server, batch eval CLI |
| `deploy/` | Docker Compose, DDL, Grafana dashboards, judge prompt |
| `docs/` | Full user guide (AsciiDoc) |
| `arch/` | Architecture design docs — overview, SDK, infra, judge, server, services |

---

## Documentation

- **[User Guide](docs/user-guide.adoc)** — installation, SDK reference, deployment, judge, alerting, UI, batch eval
- **[Architecture](arch/overview.md)** — design principles, technology choices, storage model
- **[deploy/README.md](deploy/README.md)** — Docker Compose reference, startup order, service ports

---

## License

Apache 2.0 — see [LICENSE](LICENSE).
