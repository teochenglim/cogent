# Server rules

**One binary, one port.** `services/cmd/server/main.go` serves both the REST API (`/api/*`) and the embedded UI (`/*`). No separate web server, no CDN. Default port: `8090`.

## UI embedding

Static files live in `services/ui/`. Embedded at build time via `go:embed`. The binary is fully standalone — copy it anywhere, it serves the full UI.

SPA routing: any path that isn't `/api/*`, `/static/*`, `trace.html`, `span.html`, or `annotate.html` returns `index.html`.

## Response structs must have json tags

All response structs in `greptime/queries.go` and `doris/queries.go` MUST have `json:"snake_case"` tags. Without them Go serialises as PascalCase (`TraceID`), which breaks the JS UI that reads `t.trace_id`.

## Search routing

`handleSearch` tries Doris first, falls back to GreptimeDB. See `.claude/rules/search.md`.

## Annotation flow

`POST /api/annotate` does NOT write to a database directly. It emits a new Kafka event (`operation = "human_annotation"`), which consumer-greptime and consumer-doris then store. This keeps the annotation in the same event stream as everything else.

## Dependencies injected via server struct

```go
type server struct {
    greptime *greptimedb.DB
    doris    *dorisdb.DB
    store    *storage.Client   // MinIO/S3
    kafkaW   *kafka.Writer     // for /api/annotate
    cfg      config.Config
    logger   *zap.Logger
}
```

Adding a new handler: add the dependency to the struct if needed, wire it in `main()`, add the route under `r.Route("/api", ...)`.

## CORS

`Access-Control-Allow-Origin: *` is set unconditionally. Fine for self-hosted local dev. Remove or tighten for a production deployment exposed to the internet.
