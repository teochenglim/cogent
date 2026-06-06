# Always-split payload model

**Rule:** `prompt`, `completion`, `tool_input`, and `tool_output` are NEVER stored in the database. No threshold, no exceptions.

## What happens to payload fields

The Python SDK (`cogent/sdk/telemetry.py`) intercepts these fields in `_Span.log()` and converts them before emitting the Kafka message:

```
prompt → prompt_preview (first 500 chars)
        prompt_ref      (S3 key: {trace_id}/{span_id}/prompt)
        prompt_size_bytes
```

The Kafka message contains only `_preview`, `_ref`, and `_size_bytes`. The raw field is never emitted.

S3 upload is async (background thread). If it fails, the span is still emitted with a null `_ref`.

## Why this model exists

Phil Hetzel (Braintrust) observed 1 GB traces and 20 MB single spans in production agent workloads. Storing payloads inline makes DB rows unpredictably large and time-series queries slow. The always-split model keeps rows at a fixed, predictable size regardless of payload content.

## UI behaviour

The trace detail view (`trace.html`) shows the 500-char preview immediately. Clicking "Load full payload" triggers `GET /api/spans/{spanID}/payload?field=prompt`, which:
1. Reads `prompt_ref` from GreptimeDB
2. Fetches the object from MinIO/S3
3. Returns the full content

## Consequences for code changes

- Never add code that stores raw prompt/completion content in GreptimeDB or Doris rows
- The `services/internal/schema/event.go` Event struct has no `Prompt`/`Completion` fields — only the split variants
- If adding a new payload field, follow the same pattern: add `_preview`, `_ref`, `_size_bytes` to the schema
