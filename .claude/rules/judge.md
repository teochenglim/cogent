# Judge rules

**Binaries:** `consumer-judge` (real-time), `eval` (batch). Both use `services/internal/judge/`.

## What it scores

Only spans with `operation = "llm_call"` AND no existing `eval_score`. Skips everything else silently.

## Output

Emits a new event back to Kafka with:
- `operation = "evaluation"`
- `eval_source = "realtime"` (consumer-judge) or `"batch_eval"` (eval CLI)
- `eval_score`, `eval_label`, `eval_reason` populated from LLM response

The other consumers pick this up and store it. Scores are linked to the original span via `parent_span_id`.

## Prompt template variables

The judge prompt must use `{{PROMPT}}` and `{{COMPLETION}}` as placeholders (double braces). These are replaced at runtime with the full content fetched from S3.

## Required JSON output keys

The LLM must return only a JSON object with these keys (even if your rubric uses different dimension names):
```json
{"relevance": 0.0, "faithfulness": 0.0, "safety": 0.0, "overall": 0.0, "label": "good|acceptable|bad", "reason": "..."}
```
`eval_score` is stored as `overall`. `eval_label` is stored as `label`.

## Default prompt is a placeholder

`deploy/prompts/judge_default.txt` scores generic relevance/faithfulness/safety. It is intentionally generic. Replace it with a domain rubric after the first run. Pass via `--judge-prompt-file` — no redeployment needed.

## Config precedence

`CLI flag > env var > default` for all judge parameters. Key defaults:
- `JUDGE_BASE_URL` = `http://localhost:11434/v1` (Ollama)
- `JUDGE_MODEL` = `llama3.2`
- `JUDGE_RPS` = `2` (token bucket rate limit)

## Embed

The default prompt is embedded in the binary at build time via `go:embed`. The binary is fully standalone — no external files needed unless you want a custom prompt.
