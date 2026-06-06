# Kafka / Redpanda dual-listener setup

Redpanda is configured with two listeners:

| Listener | Address | Used by |
|---|---|---|
| `PLAINTEXT` | `redpanda:9092` | Docker-internal (Go consumers, server) |
| `PLAINTEXT_HOST` | `localhost:19092` | Host-native clients (Python SDK, dev tools) |

**Why this matters:**
Kafka advertises its address back to clients after the initial connection. If a host-native client connects to `localhost:9092` (the internal listener), Kafka returns `redpanda:9092` as the broker address — which is unresolvable from the host. This caused `KafkaTimeoutError: Failed to update metadata after 60.0 secs`.

**The fix is in deploy/docker-compose.yml:**
```yaml
redpanda:
  command:
    - --kafka-addr=PLAINTEXT://0.0.0.0:9092,PLAINTEXT_HOST://0.0.0.0:19092
    - --advertise-kafka-addr=PLAINTEXT://redpanda:9092,PLAINTEXT_HOST://localhost:19092
  ports:
    - "9092:9092"
    - "19092:19092"
```

**Docker Go services** use `environment: BOOTSTRAP_SERVERS: redpanda:9092` to override the `.env` file's `localhost:19092`.

**Host-native Python** uses `localhost:19092` from `.env` / `.env.example`.

**Never change** the `BOOTSTRAP_SERVERS` default in `.env.example` back to `localhost:9092` — that would break Python examples running from the host.
