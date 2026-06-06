# Search routing: Doris → GreptimeDB fallback

`GET /api/search?q=...` is handled in `services/cmd/server/main.go` `handleSearch`.

## Routing logic

1. Try Doris `SEARCH()` DSL (full-text, full content)
2. If Doris returns an error OR zero results → fall back to GreptimeDB `LIKE` scan on preview columns
3. Return whichever result set is non-empty

## Doris query syntax

Doris 4.1 uses `SEARCH()` DSL — NOT `MATCH_ALL()`. The syntax is:

```sql
WHERE SEARCH('prompt:term OR completion:term OR tool_input:term OR tool_output:term')
```

`MATCH_ALL(column, ?)` does not exist in Doris 4.1 and returns `Error 1105: Can not found function 'MATCH_ALL'`.

The expression is built in `services/internal/doris/queries.go`:
```go
searchExpr := fmt.Sprintf("prompt:%s OR completion:%s OR tool_input:%s OR tool_output:%s", q, q, q, q)
```

## GreptimeDB fallback

Searches `prompt_preview`, `completion_preview`, `tool_input_preview`, `tool_output_preview` columns with `LIKE '%term%'`. Limited to the 500-char previews so may miss matches that appear only in the full payload.

## When Doris wins

Doris is only populated by `consumer-doris`. If that consumer has never run (or Doris is down), Doris returns zero results and the server falls back to GreptimeDB. The server logs:
```
WARN  doris search failed, falling back to greptime  error=...
```

Once `consumer-doris` is running and Doris has data, Doris results take priority (full content, not just previews).
