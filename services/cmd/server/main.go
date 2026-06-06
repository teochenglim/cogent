package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cogent/services/internal/config"
	dorisdb "github.com/cogent/services/internal/doris"
	greptimedb "github.com/cogent/services/internal/greptime"
	"github.com/cogent/services/internal/schema"
	"github.com/cogent/services/internal/storage"
	ui "github.com/cogent/services/ui"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type server struct {
	greptime *greptimedb.DB
	doris    *dorisdb.DB
	store    *storage.Client
	kafkaW   *kafka.Writer
	cfg      config.Config
	logger   *zap.Logger
}

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg := config.Load()
	config.BindFlags(&cfg, flag.CommandLine)
	flag.Parse()

	gdb, err := greptimedb.NewDB(cfg)
	if err != nil {
		logger.Fatal("greptime connect", zap.Error(err))
	}
	ddb, err := dorisdb.NewDB(cfg)
	if err != nil {
		logger.Fatal("doris connect", zap.Error(err))
	}
	store, err := storage.NewClient(cfg, logger)
	if err != nil {
		logger.Fatal("storage client", zap.Error(err))
	}
	kafkaW := kafka.NewWriter(kafka.WriterConfig{
		Brokers: []string{cfg.BootstrapServers},
		Topic:   cfg.Topic,
	})

	srv := &server{
		greptime: gdb,
		doris:    ddb,
		store:    store,
		kafkaW:   kafkaW,
		cfg:      cfg,
		logger:   logger,
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(zapMiddleware(logger))
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware)

	// REST API
	r.Route("/api", func(r chi.Router) {
		r.Get("/health", srv.handleHealth)
		r.Get("/stats", srv.handleStats)
		r.Get("/traces", srv.handleListTraces)
		r.Get("/traces/{traceID}", srv.handleGetTrace)
		r.Get("/spans/{spanID}/payload", srv.handleGetPayload)
		r.Get("/search", srv.handleSearch)
		r.Get("/justifications", srv.handleJustifications)
		r.Post("/annotate", srv.handleAnnotate)
	})

	// Embedded UI — SPA fallback: unknown paths serve index.html
	staticFS, _ := fs.Sub(ui.StaticFiles, ".")
	fileServer := http.FileServer(http.FS(staticFS))
	r.Handle("/static/*", fileServer)
	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		// Serve specific HTML files by path; fall back to index.html
		p := strings.TrimPrefix(req.URL.Path, "/")
		if p == "trace.html" || p == "span.html" || p == "annotate.html" {
			http.ServeFileFS(w, req, staticFS, p)
			return
		}
		http.ServeFileFS(w, req, staticFS, "index.html")
	})

	httpSrv := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      r,
		ReadTimeout:  cfg.ServerReadTimeout,
		WriteTimeout: cfg.ServerWriteTimeout,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go func() {
		logger.Info("server started", zap.String("port", cfg.ServerPort))
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server error", zap.Error(err))
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down server")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	_ = httpSrv.Shutdown(shutCtx)
}

// --- Handlers ---

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": "0.1.0"})
}

func (s *server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.greptime.GetStats(r.Context())
	if err != nil {
		s.logger.Error("stats query", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *server) handleListTraces(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	agentName := q.Get("agent_name")
	environment := q.Get("environment")
	now := time.Now().Unix()
	startTs := parseInt64(q.Get("start"), now-86400)
	endTs := parseInt64(q.Get("end"), now)
	limit := parseInt(q.Get("limit"), 50)
	offset := parseInt(q.Get("offset"), 0)

	traces, err := s.greptime.ListTraces(r.Context(), agentName, environment, startTs, endTs, limit, offset)
	if err != nil {
		s.logger.Error("list traces", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if traces == nil {
		traces = []greptimedb.TraceSummary{}
	}
	writeJSON(w, http.StatusOK, traces)
}

func (s *server) handleGetTrace(w http.ResponseWriter, r *http.Request) {
	traceID := chi.URLParam(r, "traceID")
	spans, err := s.greptime.GetTrace(r.Context(), traceID)
	if err != nil {
		s.logger.Error("get trace", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if spans == nil {
		spans = []greptimedb.SpanDetail{}
	}
	writeJSON(w, http.StatusOK, spans)
}

func (s *server) handleGetPayload(w http.ResponseWriter, r *http.Request) {
	spanID := chi.URLParam(r, "spanID")
	field := r.URL.Query().Get("field")
	valid := map[string]bool{"prompt": true, "completion": true, "tool_input": true, "tool_output": true}
	if !valid[field] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid field"})
		return
	}

	ref, err := s.greptime.GetSpanRef(r.Context(), spanID, field)
	if err != nil || ref == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "ref not found"})
		return
	}

	content, err := s.store.Fetch(r.Context(), ref)
	if err != nil {
		s.logger.Error("s3 fetch", zap.String("ref", ref), zap.Error(err))
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "S3 fetch failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"field": field, "content": content})
}

func (s *server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	term := q.Get("q")
	if term == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "q is required"})
		return
	}
	now := time.Now().Unix()
	agentName := q.Get("agent_name")
	start := parseInt64(q.Get("start"), now-7*86400)
	end := parseInt64(q.Get("end"), now)
	limit := parseInt(q.Get("limit"), 50)
	offset := parseInt(q.Get("offset"), 0)

	// Try Doris full-text search first; fall back to GreptimeDB LIKE search.
	dorisResults, err := s.doris.Search(r.Context(), term, agentName, start, end, limit, offset)
	if err == nil && len(dorisResults) > 0 {
		writeJSON(w, http.StatusOK, dorisResults)
		return
	}
	if err != nil {
		s.logger.Warn("doris search failed, falling back to greptime", zap.Error(err))
	}

	results, err := s.greptime.Search(r.Context(), term, agentName, start, end, limit, offset)
	if err != nil {
		s.logger.Error("greptime search", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if results == nil {
		results = []greptimedb.SearchResult{}
	}
	writeJSON(w, http.StatusOK, results)
}

func (s *server) handleJustifications(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := parseInt(q.Get("limit"), 100)
	results, err := s.doris.ListJustifications(r.Context(), q.Get("agent_name"), limit)
	if err != nil {
		s.logger.Error("justifications", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if results == nil {
		results = []dorisdb.Justification{}
	}
	writeJSON(w, http.StatusOK, results)
}

type annotateRequest struct {
	SpanID        string  `json:"span_id"`
	TraceID       string  `json:"trace_id"`
	Score         float64 `json:"score"`
	Label         string  `json:"label"`
	Justification string  `json:"justification"`
}

func (s *server) handleAnnotate(w http.ResponseWriter, r *http.Request) {
	var req annotateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	validLabels := map[string]bool{"good": true, "acceptable": true, "bad": true}
	if req.SpanID == "" || req.TraceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "span_id and trace_id are required"})
		return
	}
	if req.Score < 0 || req.Score > 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "score must be 0.0-1.0"})
		return
	}
	if !validLabels[req.Label] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "label must be good, acceptable, or bad"})
		return
	}
	if len(req.Justification) < 20 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "justification must be at least 20 characters"})
		return
	}

	now := float64(time.Now().UnixMicro()) / 1e6
	newSpanID := uuid.New().String()
	evalSrc := "human_annotation"
	e := schema.Event{
		TraceID:      req.TraceID,
		SpanID:       newSpanID,
		ParentSpanID: &req.SpanID,
		StartTime:    now,
		EndTime:      now,
		DurationMs:   0,
		AgentName:    "human",
		Operation:    "human_annotation",
		ServiceName:  "cogent-server",
		Environment:  "production",
		EvalScore:    &req.Score,
		EvalLabel:    &req.Label,
		EvalReason:   &req.Justification,
		EvalSource:   &evalSrc,
	}

	data, err := e.ToJSON()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "serialise event"})
		return
	}
	if err := s.kafkaW.WriteMessages(r.Context(), kafka.Message{Value: data}); err != nil {
		s.logger.Error("kafka write annotate", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "emit event"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func parseInt(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

func parseInt64(s string, def int64) int64 {
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	return def
}

func zapMiddleware(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			logger.Info("request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", ww.Status()),
				zap.Duration("duration", time.Since(start)),
			)
		})
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Silence unused import error
var _ = fmt.Sprintf
