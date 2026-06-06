# Python SDK rules

**Entry points:** `cogent/sdk/telemetry.py` (`AgentTelemetry`, `_Span`), `cogent/sdk/offload.py` (`PayloadOffloader`), `cogent/sdk/schema.py` (`AgentEvent`)

**Dependencies: kafka-python, pydantic>=2, minio — nothing else.** Do not add dependencies.

## Schema contract

`cogent/sdk/schema.py` (`AgentEvent`) and `services/internal/schema/event.go` (`Event`) must stay in sync. Adding a field to one requires adding it to the other. The Python schema is Pydantic v2; the Go schema is a plain struct.

## Payload split — always in offload.py, never in schema.py

`prompt`, `completion`, `tool_input`, `tool_output` are intercepted in `_Span.log()` before emitting. The raw strings are never put in the Kafka message. The offloader produces:
- `_preview` — first 500 chars
- `_ref` — S3 key (`{trace_id}/{span_id}/{field}`)
- `_size_bytes` — len of original content

S3 upload is background (ThreadPoolExecutor). Failure is silent — span still emits, `_ref` stays null.

## Trace ID propagation

`trace_id` propagates via `contextvars.ContextVar`. Child spans created inside a `with tel.span(...)` block automatically inherit `trace_id` and set `parent_span_id`. No manual ID threading needed.

## Kafka producer flush

`KafkaProducer.send()` is async. After send, call `flush(timeout=10)` before process exit — otherwise the background delivery thread may not finish. This is done in `_Span.__exit__` → `telemetry.py`.

## Operation values

Valid values for `operation`: `llm_call`, `tool_call`, `retrieval`, `handoff`, `evaluation`, `human_annotation`, `custom`. The judge only scores spans with `operation = "llm_call"`.
