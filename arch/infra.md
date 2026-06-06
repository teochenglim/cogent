# Cogent Infrastructure — DDL, Docker Compose, Grafana, Makefile

## Local dev prerequisites (macOS with Colima)

Doris BE requires `vm.max_map_count ≥ 2000000` for memory-mapped storage. The Colima VM defaults to ~65530. Set it once after `colima start`:

```bash
colima ssh -- sudo sysctl -w vm.max_map_count=2000000
# persist across restarts:
colima ssh -- sudo sh -c 'echo vm.max_map_count=2000000 > /etc/sysctl.d/99-doris.conf'
```

Without this, the Doris BE container starts but crashes in a loop before binding any ports.

## deploy/schema/greptime.sql — Hot tier DDL

Narrow table. No full content columns — only metadata, previews, refs, sizes.
Rows are predictably small. Fast time-series queries.

```sql
CREATE TABLE agent_telemetry (
  ts                       TimestampMillisecond NOT NULL,
  trace_id                 STRING,
  span_id                  STRING,
  parent_span_id           STRING,
  start_time               DOUBLE,
  end_time                 DOUBLE,
  duration_ms              DOUBLE,
  agent_name               STRING,
  operation                STRING,
  service_name             STRING,
  environment              STRING,
  model                    STRING,
  provider                 STRING,
  input_tokens             INT32,
  output_tokens            INT32,
  cost_usd                 DOUBLE,
  finish_reason            STRING,
  prompt_preview           STRING,
  prompt_ref               STRING,
  prompt_size_bytes        INT64,
  completion_preview       STRING,
  completion_ref           STRING,
  completion_size_bytes    INT64,
  tool_name                STRING,
  tool_input_preview       STRING,
  tool_input_ref           STRING,
  tool_input_size_bytes    INT64,
  tool_output_preview      STRING,
  tool_output_ref          STRING,
  tool_output_size_bytes   INT64,
  tool_error               STRING,
  eval_score               DOUBLE,
  eval_label               STRING,
  eval_source              STRING,
  PRIMARY KEY (agent_name, operation, trace_id, span_id),
  TIME INDEX (ts)
);
```

## deploy/schema/doris.sql — Warm tier DDL

Full content table. Source of truth for text search and human review.
Inverted indexes on text columns — the reason ClickHouse was abandoned at
Braintrust. Doris 4.0 handles native full-text search at this scale.

```sql
CREATE DATABASE IF NOT EXISTS cogent;
USE cogent;

CREATE TABLE agent_telemetry (
  trace_id               VARCHAR(36)   NOT NULL,
  span_id                VARCHAR(36)   NOT NULL,
  parent_span_id         VARCHAR(36),
  start_time             DOUBLE,
  end_time               DOUBLE,
  duration_ms            DOUBLE,
  agent_name             VARCHAR(128),
  operation              VARCHAR(64),
  service_name           VARCHAR(128),
  environment            VARCHAR(32),
  model                  VARCHAR(128),
  provider               VARCHAR(64),
  input_tokens           INT,
  output_tokens          INT,
  cost_usd               DOUBLE,
  finish_reason          VARCHAR(32),
  prompt                 TEXT,
  prompt_preview         VARCHAR(512),
  prompt_ref             VARCHAR(512),
  prompt_size_bytes      BIGINT,
  completion             TEXT,
  completion_preview     VARCHAR(512),
  completion_ref         VARCHAR(512),
  completion_size_bytes  BIGINT,
  tool_name              VARCHAR(128),
  tool_input             TEXT,
  tool_input_preview     VARCHAR(512),
  tool_input_ref         VARCHAR(512),
  tool_input_size_bytes  BIGINT,
  tool_output            TEXT,
  tool_output_preview    VARCHAR(512),
  tool_output_ref        VARCHAR(512),
  tool_output_size_bytes BIGINT,
  tool_error             TEXT,
  eval_score             DOUBLE,
  eval_label             VARCHAR(64),
  eval_reason            TEXT,
  eval_source            VARCHAR(32),
  metadata               TEXT,
  dt                     DATE          NOT NULL
)
DUPLICATE KEY(trace_id, span_id)
PARTITION BY RANGE(dt)(
  PARTITION p_default VALUES LESS THAN ("2099-01-01")
)
DISTRIBUTED BY HASH(trace_id) BUCKETS 8
PROPERTIES ("replication_num" = "1");

-- Inverted indexes for full-text search
-- This is the capability that caused Braintrust to abandon ClickHouse.
-- Doris 4.0 handles native inverted index on TEXT columns.
CREATE INDEX idx_prompt
  ON agent_telemetry(prompt)      USING INVERTED;
CREATE INDEX idx_completion
  ON agent_telemetry(completion)  USING INVERTED;
CREATE INDEX idx_tool_output
  ON agent_telemetry(tool_output) USING INVERTED;
CREATE INDEX idx_tool_error
  ON agent_telemetry(tool_error)  USING INVERTED;
```

## deploy/docker-compose.yml

All Go service images are built from a single multi-stage Dockerfile in
`services/`. Each container selects which binary to run via `command`.

```yaml
services:

  redpanda:
    image: redpandadata/redpanda:latest
    command: redpanda start --smp 1 --overprovisioned --node-id 0
             --kafka-addr PLAINTEXT://0.0.0.0:9092
             --advertise-kafka-addr PLAINTEXT://redpanda:9092
    ports: ["9092:9092", "8081:8081", "8082:8082"]
    volumes: [redpanda-data:/var/lib/redpanda/data]
    healthcheck:
      test: ["CMD", "rpk", "cluster", "info"]
      interval: 5s
      retries: 12

  redpanda-init:
    image: redpandadata/redpanda:latest
    depends_on: {redpanda: {condition: service_healthy}}
    entrypoint: ["/bin/sh", "-c"]
    command: deploy/scripts/redpanda-init.sh
    # Creates topic cogent-telemetry: 8 partitions, 7-day retention

  minio:
    image: minio/minio:latest
    command: server /data --console-address :9001
    ports: ["9000:9000", "9001:9001"]
    environment:
      MINIO_ROOT_USER: ${MINIO_ACCESS_KEY:-minioadmin}
      MINIO_ROOT_PASSWORD: ${MINIO_SECRET_KEY:-minioadmin}
    volumes: [minio-data:/data]
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:9000/minio/health/live"]
      interval: 5s
      retries: 12

  minio-init:
    image: minio/mc:latest
    depends_on: {minio: {condition: service_healthy}}
    entrypoint: ["/bin/sh", "-c"]
    command: |
      mc alias set local http://minio:9000 minioadmin minioadmin
      mc mb --ignore-existing local/cogent-payloads
      mc anonymous set none local/cogent-payloads

  greptimedb:
    image: greptime/greptimedb:latest
    command: standalone start
    ports: ["4000:4000", "4001:4001", "4002:4002"]
    volumes: [greptimedb-data:/tmp/greptimedb]
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:4000/health"]
      interval: 5s
      retries: 12

  doris-fe:
    # Verify the exact tag at hub.docker.com/r/apache/doris before use.
    # Tag naming changes across major Doris versions.
    image: apache/doris:2.1-fe-x86_64
    ports: ["8030:8030", "9030:9030"]
    volumes: [doris-fe-data:/opt/apache-doris/fe/doris-meta]
    environment:
      FE_SERVERS: "fe1:doris-fe:9010"
      FE_ID: 1
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8030/api/bootstrap"]
      interval: 10s
      retries: 18

  doris-be:
    image: apache/doris:2.1-be-x86_64
    depends_on: {doris-fe: {condition: service_healthy}}
    ports: ["9060:9060"]
    volumes: [doris-be-data:/opt/apache-doris/be/storage]
    environment:
      FE_SERVERS: "fe1:doris-fe:9010"
      BE_ADDR: "doris-be:9050"

  doris-init:
    build:
      context: deploy/
      dockerfile: Dockerfile.init   # tiny alpine + mysql-client image
    depends_on: [doris-be]
    command: ["/bin/sh", "scripts/doris-init.sh"]

  consumer-greptime:
    build: {context: services/, dockerfile: Dockerfile}
    command: /consumer-greptime
    depends_on: [redpanda-init, greptimedb]
    restart: unless-stopped
    env_file: .env

  consumer-doris:
    build: {context: services/, dockerfile: Dockerfile}
    command: /consumer-doris
    depends_on: [redpanda-init, doris-init, minio-init]
    restart: unless-stopped
    env_file: .env

  consumer-judge:
    build: {context: services/, dockerfile: Dockerfile}
    command: /consumer-judge
    depends_on: [redpanda-init, minio-init]
    restart: unless-stopped
    env_file: .env

  consumer-alerting:
    build: {context: services/, dockerfile: Dockerfile}
    command: /consumer-alerting
    depends_on: [redpanda-init]
    restart: unless-stopped
    env_file: .env

  server:
    build: {context: services/, dockerfile: Dockerfile}
    command: /server
    ports: ["8090:8090"]
    depends_on: [redpanda-init, greptimedb, doris-init, minio-init]
    restart: unless-stopped
    env_file: .env

  grafana:
    image: grafana/grafana:latest
    ports: ["3000:3000"]
    volumes:
      - ./deploy/grafana/datasources.yml:/etc/grafana/provisioning/datasources/datasources.yml
      - ./deploy/grafana/dashboards:/etc/grafana/provisioning/dashboards
    depends_on: [greptimedb, doris-init]
    environment:
      GF_AUTH_ANONYMOUS_ENABLED: "true"
      GF_AUTH_ANONYMOUS_ORG_ROLE: "Admin"

volumes:
  redpanda-data:
  minio-data:
  greptimedb-data:
  doris-fe-data:
  doris-be-data:
```

## services/Dockerfile — Multi-stage Go build

```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /app/bin/consumer-greptime ./cmd/consumer-greptime
RUN go build -o /app/bin/consumer-doris    ./cmd/consumer-doris
RUN go build -o /app/bin/consumer-judge    ./cmd/consumer-judge
RUN go build -o /app/bin/consumer-alerting ./cmd/consumer-alerting
RUN go build -o /app/bin/server            ./cmd/server
RUN go build -o /app/bin/eval              ./cmd/eval

FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=builder /app/bin/* /
```

## deploy/scripts/redpanda-init.sh

```bash
#!/bin/sh
rpk topic create cogent-telemetry \
  --partitions 8 \
  --replicas 1 \
  --topic-config retention.ms=604800000  # 7 days
```

## deploy/scripts/doris-init.sh

Timing is critical. If this script exits before the BE is healthy, the whole
stack fails silently (INSERT will succeed but data goes nowhere).

```bash
#!/bin/sh
set -e

# Wait for FE MySQL port — up to 120s
echo "Waiting for Doris FE..."
for i in $(seq 1 60); do
  if mysql -h doris-fe -P 9030 -u root -e "SELECT 1" >/dev/null 2>&1; then
    echo "FE ready."
    break
  fi
  sleep 2
done

# Register BE
mysql -h doris-fe -P 9030 -u root -e \
  "ALTER SYSTEM ADD BACKEND 'doris-be:9050';" || true

# Wait for BE to be alive
echo "Waiting for BE to come alive..."
for i in $(seq 1 60); do
  ALIVE=$(mysql -h doris-fe -P 9030 -u root -se \
    "SHOW BACKENDS\G" 2>/dev/null | grep -c "Alive: true" || true)
  if [ "$ALIVE" -ge 1 ]; then
    echo "BE alive."
    break
  fi
  sleep 2
done

# Run DDL
mysql -h doris-fe -P 9030 -u root < /scripts/schema/greptime.sql || true
mysql -h doris-fe -P 9030 -u root < /scripts/schema/doris.sql
echo "Doris init complete."
```

## deploy/grafana/datasources.yml

```yaml
apiVersion: 1
datasources:
  - name: GreptimeDB
    type: mysql
    url: greptimedb:4002
    database: public
    editable: false

  - name: Doris
    type: mysql
    url: doris-fe:9030
    database: cogent
    user: root
    editable: false
```

## deploy/grafana/dashboards/agent-overview.json

Dashboard variables: `$trace_id` (text), `$search_term` (text),
`$agent_name` (dropdown from GreptimeDB distinct values).

All panels use GreptimeDB as datasource unless noted.

| # | Panel | Query type | Notes |
|---|---|---|---|
| 1 | Total cost today by agent | Bar chart | `SUM(cost_usd) GROUP BY agent_name` |
| 2 | Token usage over time by model | Time series | `SUM(input_tokens+output_tokens) GROUP BY model, time bucket 1h` |
| 3 | Avg eval score by agent (24 h) | Stat panels | Split by `eval_source` |
| 4 | P95 duration_ms by operation | Time series | |
| 5 | Tool error rate | Bar chart | `errors / total tool_calls` |
| 6 | Payload size trend | Time series | AVG of each `*_size_bytes` field |
| 7 | Active traces | Table | trace_id, agent, spans, cost, avg score, max span bytes, last seen |
| 8 | Span timeline for $trace_id | Table | sorted by start_time, depth, coloured eval score |
| 9 | Full-text search | Table | **Doris datasource** — `MATCH_ALL(prompt, '$search_term') OR MATCH_ALL(completion, '$search_term')` |
| 10 | Link to Cogent UI | Text panel | `http://localhost:8090` |

Panel 9 uses Doris native inverted index. This is the query that required a
custom Tantivy index at Braintrust. Doris handles it natively.

## .env.example

```bash
# Transport
BOOTSTRAP_SERVERS=localhost:9092
TOPIC=cogent-telemetry

# Object storage (MinIO local / S3 in production)
MINIO_ENDPOINT=localhost:9000
MINIO_ACCESS_KEY=minioadmin
MINIO_SECRET_KEY=minioadmin
MINIO_BUCKET=cogent-payloads
MINIO_SECURE=false

# GreptimeDB
GREPTIME_HOST=localhost
GREPTIME_PORT=4002
GREPTIME_DATABASE=public

# Doris
DORIS_FE_HOST=localhost
DORIS_FE_HTTP_PORT=8030
DORIS_MYSQL_PORT=9030
DORIS_USER=root
DORIS_PASSWORD=
DORIS_DATABASE=cogent

# Judge LLM (OpenAI-compatible)
# Override at runtime: --judge-model gpt-4o --judge-base-url https://api.openai.com/v1
JUDGE_BASE_URL=http://localhost:11434/v1
JUDGE_MODEL=llama3.2
JUDGE_API_KEY=ollama
JUDGE_RPS=2
JUDGE_PROMPT_FILE=
# JUDGE_PROMPT_FILE=/etc/cogent/prompts/my_domain_judge.txt

# Server
SERVER_PORT=8090
SERVER_READ_TIMEOUT=30s
SERVER_WRITE_TIMEOUT=60s

# Alerting
ALERT_MAX_SPANS_PER_TRACE=200
ALERT_COST_BUDGET_USD=1.0
ALERT_MAX_SPAN_BYTES=52428800
ALERT_WINDOW_SECONDS=60
ALERT_WEBHOOK_URL=
```

## Makefile

```makefile
.PHONY: build-go build-python test-python test-go docker-build up down logs

build-go:
	cd services && go build -o ../bin/consumer-greptime ./cmd/consumer-greptime
	cd services && go build -o ../bin/consumer-doris    ./cmd/consumer-doris
	cd services && go build -o ../bin/consumer-judge    ./cmd/consumer-judge
	cd services && go build -o ../bin/consumer-alerting ./cmd/consumer-alerting
	cd services && go build -o ../bin/server            ./cmd/server
	cd services && go build -o ../bin/eval              ./cmd/eval

build-python:
	pip install -e ".[sdk]"

test-python:
	pytest tests/python/ -v

test-go:
	cd services && go test ./...

docker-build:
	docker compose build

up:
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f

# Run batch eval over last 7 days (dry run to preview scores)
eval-dry:
	./bin/eval --start $$(date -d '7 days ago' +%Y-%m-%d) \
	           --end $$(date +%Y-%m-%d) \
	           --dry-run
```

## Doris image tags

Verify the exact tag at `hub.docker.com/r/apache/doris` before use. The tag
naming convention changes between major Doris versions. Check the Doris play
repo (`~/code/doris-play`) for the currently pinned working tags for the local
environment.

## Production upgrade paths

| Change | What to update |
|---|---|
| Redpanda → AutoMQ | Set `BOOTSTRAP_SERVERS` to AutoMQ endpoint. No code change. |
| MinIO → AWS S3 | Set `MINIO_ENDPOINT=s3.amazonaws.com`, update access keys. |
| MinIO → GCS | Set `MINIO_ENDPOINT=storage.googleapis.com`, update creds. |
| Single Doris → Doris cluster | Update `DORIS_FE_HOST` to load balancer. Update `FE_SERVERS` in compose. |
| Ollama → OpenAI | `JUDGE_BASE_URL=https://api.openai.com/v1`, `JUDGE_API_KEY=sk-...` |
| Custom judge prompt | `JUDGE_PROMPT_FILE=/path/to/your_domain_judge.txt` |
