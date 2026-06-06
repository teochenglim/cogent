# Cogent SDK — Python Instrumentation Library

## Overview

The Python SDK is the only component agents interact with directly. It must be
minimal, dependency-light, and never block the agent. The critical constraint:
if the observability backend (Redpanda, MinIO, GreptimeDB) is unavailable, the
agent continues running unaffected.

**Dependencies:** `kafka-python`, `pydantic>=2.0`, `minio`. Nothing else.

## sdk/schema.py — Event model

Use Pydantic v2. All fields after the first six are Optional with default None.
The schema is the contract between the Python SDK and all Go consumers — never
add a field here without updating `internal/schema/event.go`.

```python
class AgentEvent(BaseModel):
    # Correlation (always present)
    trace_id:                  str           # UUID4
    span_id:                   str           # UUID4
    parent_span_id:            Optional[str] # UUID4 or None
    start_time:                float         # Unix seconds, microsecond precision
    end_time:                  float
    duration_ms:               float

    # Agent context
    agent_name:                str
    operation:                 str           # llm_call | tool_call | retrieval
                                             # | handoff | evaluation
                                             # | human_annotation | custom
    service_name:              str
    environment:               str

    # LLM metadata (no inline content)
    model:                     Optional[str]
    provider:                  Optional[str]
    input_tokens:              Optional[int]
    output_tokens:             Optional[int]
    cost_usd:                  Optional[float]
    finish_reason:             Optional[str]

    # Payload previews (always 500 chars max, set whenever content exists)
    prompt_preview:            Optional[str]
    completion_preview:        Optional[str]
    tool_input_preview:        Optional[str]
    tool_output_preview:       Optional[str]

    # S3 reference keys (always set when payload exists)
    prompt_ref:                Optional[str]  # e.g. trace_id/span_id/prompt
    completion_ref:            Optional[str]
    tool_input_ref:            Optional[str]
    tool_output_ref:           Optional[str]

    # Full payload sizes (always set when content exists)
    prompt_size_bytes:         Optional[int]
    completion_size_bytes:     Optional[int]
    tool_input_size_bytes:     Optional[int]
    tool_output_size_bytes:    Optional[int]

    # Tool metadata
    tool_name:                 Optional[str]
    tool_error:                Optional[str]  # always inline; errors are small

    # Evaluation
    eval_score:                Optional[float]  # 0.0 to 1.0
    eval_label:                Optional[str]
    eval_reason:               Optional[str]
    eval_source:               Optional[str]  # realtime | batch_eval
                                              # | human_annotation

    # Arbitrary metadata
    metadata:                  Optional[dict]
```

**Payload rule — always enforced in offload.py, never in schema.py:**
- `preview = content[:500]` (or None if content is None)
- Full payload always uploaded to S3
- `_preview`, `_ref`, and `_size_bytes` fields are set by the offloader
- The raw content fields (`prompt`, `completion`, `tool_input`, `tool_output`)
  do NOT exist in the schema — they are transient inputs to `offload.prepare()`

## sdk/offload.py — Payload splitter

```python
class PayloadOffloader:
    """
    Splits every content field into a 500-char preview plus an S3 upload.
    No threshold. No conditional logic. All content always goes to S3.
    """

    def __init__(
        self,
        endpoint: str,
        bucket: str,
        access_key: str,
        secret_key: str,
        secure: bool = False,
    ): ...
    # Initialises minio.Minio client. Bucket must already exist.

    def prepare(
        self,
        field_name: str,  # prompt | completion | tool_input | tool_output
        content: str,
        trace_id: str,
        span_id: str,
    ) -> tuple[str, str, int]:
        """
        Returns (preview, ref_key, size_bytes).
        Uploads full content to S3 in a background thread (fire and forget).
        Never blocks the calling span.
        preview   = content[:500]
        ref_key   = f"{trace_id}/{span_id}/{field_name}"
        size_bytes = len(content.encode())
        """

    def fetch(self, ref_key: str) -> str:
        """Download and return full content by S3 key. Synchronous."""
```

**No-op mode** — when `MINIO_ENDPOINT` is not set, `PayloadOffloader` operates
without S3:
- `preview = content[:500]`
- `ref_key = None`
- `size_bytes = len(content.encode())`
- No upload attempted
- Graceful degradation for local dev without MinIO running

The no-op mode is selected at construction time by passing `endpoint=None`.
`AgentTelemetry` passes `None` when the env var is absent.

## sdk/telemetry.py — Main entry point

Keep this file under 200 lines. Context propagation uses `contextvars`.

```python
class AgentTelemetry:
    """
    Initialise once per process. Reuse across all agents in the process.
    KafkaProducer is created once and reused — it is thread-safe.
    """

    def __init__(
        self,
        bootstrap_servers: str,
        topic: str = "cogent-telemetry",
        offloader: PayloadOffloader | None = None,
        service_name: str = "default",
        environment: str = "production",
    ): ...

    def span(
        self,
        operation: str,
        agent_name: str,
        **meta,
    ) -> "_Span":
        """Create a span context manager. Extra kwargs stored in metadata."""


class _Span:
    """
    Context manager for one agent action.
    Supports both sync (with) and async (async with).
    """
```

### _Span lifecycle

**`__enter__` / `__aenter__`:**
1. Generate `span_id` (UUID4)
2. Inherit `trace_id` from `contextvars` or generate new UUID4 if no active
   trace
3. Set `parent_span_id` from current context (innermost active span)
4. Record `start_time = time.time()`
5. Push self onto the `contextvars` span stack

**`__exit__` / `__aexit__`:**
1. Record `end_time`, calculate `duration_ms`
2. If `span.log()` was never called manually: auto-emit with fields collected
   in `__enter__` / `__meta__`
3. Pop self from the `contextvars` span stack
4. On exception: set `tool_error = str(exception)`, re-raise

**`span.log(**fields)`:**
1. For each of `prompt`, `completion`, `tool_input`, `tool_output` present in
   `fields`: call `offloader.prepare()`, replace with `_preview`, `_ref`,
   `_size_bytes` fields
2. Validate the full assembled event dict against `AgentEvent` (Pydantic)
3. `producer.send(topic, json_bytes)` — non-blocking Kafka send
4. Return immediately

## sdk/decorators.py — @observe

```python
@observe(operation="llm_call", agent_name="my_agent")
def my_function(...): ...
```

- Works on sync and async functions
- Uses `function.__name__` as `operation` when not provided
- If the return value is `str`: passed as `completion` field to `span.log()`
- If an exception is raised: captured as `tool_error`, re-raised
- Extra kwargs forwarded to `span.log()`

## Tests

### tests/python/test_schema.py

- Minimal valid event passes Pydantic validation
- Fully populated event passes validation
- Optional fields default to None
- `duration_ms` calculated correctly
- JSON round-trip preserves all fields

### tests/python/test_offload.py

- Every content field always split (no threshold)
- `preview` is always first 500 chars
- `ref_key` format: `{trace_id}/{span_id}/{field_name}`
- `size_bytes` always populated
- `fetch()` round-trip returns original content
- No-op mode: `preview` set, `ref` is None, no upload attempted

### tests/python/test_telemetry.py

- `trace_id` propagates into nested spans
- `parent_span_id` is correct for child spans
- Two concurrent asyncio tasks have isolated `contextvars` contexts
- `span.log()` emits exactly one Kafka message
- Auto-emit on `__exit__` when `log()` was not called
- Exception captured as `tool_error` and re-raised
- Mock `KafkaProducer` and `PayloadOffloader` — do not need running services

## pyproject.toml

```toml
[project]
name = "cogent"
version = "0.1.0"
requires-python = ">=3.11"
description = "Stream-first observability for distributed LLM agents"

[project.optional-dependencies]
sdk = [
    "kafka-python>=2.0",
    "pydantic>=2.0",
    "minio>=7.0",
]
dev = [
    "pytest",
    "pytest-asyncio",
]
```

## Example usage

### Single agent

```python
from cogent.sdk import AgentTelemetry, PayloadOffloader
import os

offloader = PayloadOffloader(
    endpoint=os.getenv("MINIO_ENDPOINT"),
    bucket=os.getenv("MINIO_BUCKET", "cogent-payloads"),
    access_key=os.getenv("MINIO_ACCESS_KEY", "minioadmin"),
    secret_key=os.getenv("MINIO_SECRET_KEY", "minioadmin"),
)

telemetry = AgentTelemetry(
    bootstrap_servers=os.getenv("BOOTSTRAP_SERVERS", "localhost:9092"),
    offloader=offloader,
    service_name="my-service",
    environment="production",
)

with telemetry.span("llm_call", agent_name="research-agent") as span:
    response = llm.complete(prompt)
    span.log(
        prompt=prompt,
        completion=response.text,
        model="claude-sonnet-4-6",
        provider="anthropic",
        input_tokens=response.usage.input_tokens,
        output_tokens=response.usage.output_tokens,
        cost_usd=0.0021,
    )
```

### Multi-agent parent/child spans

```python
with telemetry.span("llm_call", agent_name="orchestrator") as parent:
    parent.log(prompt=prompt, completion=plan)
    # child automatically inherits trace_id and sets parent_span_id
    with telemetry.span("tool_call", agent_name="researcher") as child:
        result = search_tool(query)
        child.log(
            tool_name="web_search",
            tool_input=query,
            tool_output=result,
        )
```

### @observe decorator

```python
from cogent.sdk import observe

@observe(operation="llm_call", agent_name="summarizer")
async def summarize(text: str) -> str:
    return await llm.complete(f"Summarize: {text}")
```
