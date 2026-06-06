# deploy/

Everything needed to run the Cogent stack: Docker Compose, DDL, Grafana dashboards, and init scripts.

## macOS with Colima — required one-time setup

Doris BE requires `vm.max_map_count ≥ 2000000` for memory-mapped storage. Colima's VM defaults to ~65530, which causes the BE container to crash-loop silently.

```bash
# Apply immediately (lost on colima restart without the next step)
colima ssh -- sudo sysctl -w vm.max_map_count=2000000

# Persist across restarts
colima ssh -- sudo sh -c 'echo vm.max_map_count=2000000 > /etc/sysctl.d/99-doris.conf'
```

Run this once after `colima start` before `make infra-up`.

## Usage

```bash
# From repo root:
make infra-up          # start Redpanda, MinIO, GreptimeDB, Doris, Grafana
make infra-down        # stop infra containers

make up                # start full stack including Go services
make down              # stop everything
make logs              # tail all container logs
```

## Startup order

1. Redpanda, MinIO, GreptimeDB start independently.
2. `*-init` containers run once when their dependency is healthy (create topics, buckets, schema).
3. Doris FE starts; Doris BE starts concurrently (only needs FE container to exist, not healthy).
4. BE registers with FE via `FE_SERVERS`/`BE_ADDR` env vars. Once a BE is alive, FE's `SELECT 1` healthcheck passes.
5. `doris-init` runs after FE is healthy — waits for BE alive, then applies `schema/doris.sql`.
6. Go service containers start after all `*-init` containers have completed.

## Services

| Service | Ports | Notes |
|---|---|---|
| Redpanda | 9092 (Kafka), 8081 (Schema Registry), 8082 (HTTP Proxy) | |
| MinIO | 9000 (API), 9001 (Console) | admin: minioadmin / minioadmin |
| GreptimeDB | 4000 (HTTP), 4001 (gRPC), 4002 (MySQL) | |
| Doris FE | 8030 (HTTP/UI), 9030 (MySQL) | |
| Grafana | 3000 | anonymous admin access enabled |
| Server | 8090 | trace viewer + REST API |

## Files

```
deploy/
├── docker-compose.yml       # full stack definition
├── schema/
│   ├── greptime.sql         # GreptimeDB DDL (applied by greptimedb-init)
│   └── doris.sql            # Doris DDL (applied by doris-init)
├── scripts/
│   └── doris-init.sh        # waits for FE+BE, applies doris.sql
├── grafana/
│   ├── datasources.yml      # GreptimeDB + Doris datasource provisioning
│   └── dashboards/          # pre-built dashboards
└── prompts/
    └── judge_default.txt    # default LLM judge rubric (replace per domain)
```
