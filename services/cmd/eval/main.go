package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cogent/services/internal/config"
	"github.com/cogent/services/internal/consumer"
	"github.com/cogent/services/internal/judge"
	"github.com/cogent/services/internal/schema"
	"github.com/cogent/services/internal/storage"
	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg := config.Load()
	config.BindFlags(&cfg, flag.CommandLine)

	var (
		agentName  string
		startDate  string
		endDate    string
		sampleRate float64
		dryRun     bool
	)
	flag.StringVar(&agentName, "agent-name", "", "Filter by agent name")
	flag.StringVar(&startDate, "start", "", "Start date YYYY-MM-DD (required)")
	flag.StringVar(&endDate, "end", "", "End date YYYY-MM-DD (required)")
	flag.Float64Var(&sampleRate, "sample-rate", 1.0, "Fraction of events to score (0.0-1.0)")
	flag.BoolVar(&dryRun, "dry-run", false, "Score but do not emit evaluation events")
	flag.Parse()

	if startDate == "" || endDate == "" {
		fmt.Fprintln(os.Stderr, "Error: --start and --end are required")
		flag.Usage()
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	store, err := storage.NewClient(cfg, logger)
	if err != nil {
		logger.Fatal("storage client", zap.Error(err))
	}

	j, err := judge.NewJudge(cfg, logger)
	if err != nil {
		logger.Fatal("judge init", zap.Error(err))
	}

	var writer *kafka.Writer
	if !dryRun {
		writer = kafka.NewWriter(kafka.WriterConfig{
			Brokers: []string{cfg.BootstrapServers},
			Topic:   cfg.Topic,
		})
		defer writer.Close()
	}

	// Query Doris for unscored llm_call events in date range
	dsn := consumer.DSN(cfg.DorisUser, cfg.DorisPassword, cfg.DorisFEHost, cfg.DorisMySQLPort, cfg.DorisDatabase)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		logger.Fatal("doris connect", zap.Error(err))
	}
	defer db.Close()

	q := `SELECT trace_id, span_id, agent_name, environment,
		prompt_ref, completion_ref, prompt_preview, completion_preview
		FROM agent_telemetry
		WHERE operation = 'llm_call'
		AND (eval_score IS NULL OR eval_score = 0)
		AND dt >= ? AND dt <= ?`
	args := []any{startDate, endDate}
	if agentName != "" {
		q += " AND agent_name = ?"
		args = append(args, agentName)
	}

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		logger.Fatal("query doris", zap.Error(err))
	}
	defer rows.Close()

	type candidate struct {
		TraceID           string
		SpanID            string
		AgentName         string
		Environment       string
		PromptRef         *string
		CompletionRef     *string
		PromptPreview     *string
		CompletionPreview *string
	}

	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.TraceID, &c.SpanID, &c.AgentName, &c.Environment,
			&c.PromptRef, &c.CompletionRef, &c.PromptPreview, &c.CompletionPreview); err != nil {
			logger.Warn("scan error", zap.Error(err))
			continue
		}
		candidates = append(candidates, c)
	}

	// Sample
	if sampleRate < 1.0 {
		rand.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })
		n := int(float64(len(candidates)) * sampleRate)
		candidates = candidates[:n]
	}

	fmt.Printf("Events found:  %d\n", len(candidates))

	var (
		scored   int
		scoreSum float64
		buckets  [5]int // 0-0.2, 0.2-0.4, 0.4-0.6, 0.6-0.8, 0.8-1.0
	)
	start := time.Now()

	for _, c := range candidates {
		prompt := deref(c.PromptRef)
		if p, err := store.Fetch(ctx, prompt); err == nil && p != "" {
			prompt = p
		} else {
			prompt = deref(c.PromptPreview)
		}
		completion := deref(c.CompletionRef)
		if comp, err := store.Fetch(ctx, completion); err == nil && comp != "" {
			completion = comp
		} else {
			completion = deref(c.CompletionPreview)
		}

		if prompt == "" || completion == "" {
			continue
		}

		result, err := j.Score(ctx, prompt, completion)
		if err != nil {
			logger.Warn("score failed", zap.String("span_id", c.SpanID), zap.Error(err))
			continue
		}
		scored++
		scoreSum += result.Overall
		idx := int(result.Overall * 5)
		if idx >= 5 {
			idx = 4
		}
		buckets[idx]++

		if !dryRun && writer != nil {
			newSpanID := uuid.New().String()
			evalSrc := "batch_eval"
			evalEvent := schema.Event{
				TraceID:      c.TraceID,
				SpanID:       newSpanID,
				ParentSpanID: &c.SpanID,
				StartTime:    float64(time.Now().UnixMicro()) / 1e6,
				EndTime:      float64(time.Now().UnixMicro()) / 1e6,
				DurationMs:   0,
				AgentName:    c.AgentName,
				Operation:    "evaluation",
				ServiceName:  "cogent-eval",
				Environment:  c.Environment,
				EvalScore:    &result.Overall,
				EvalLabel:    &result.Label,
				EvalReason:   &result.Reason,
				EvalSource:   &evalSrc,
			}
			data, _ := evalEvent.ToJSON()
			_ = writer.WriteMessages(ctx, kafka.Message{Value: data})
		}
	}

	elapsed := time.Since(start)
	mean := 0.0
	if scored > 0 {
		mean = scoreSum / float64(scored)
	}

	fmt.Printf("Events scored: %d\n", scored)
	fmt.Printf("Mean score:    %.3f\n", mean)
	fmt.Printf("Score dist:    [0-0.2]: %d  [0.2-0.4]: %d  [0.4-0.6]: %d  [0.6-0.8]: %d  [0.8-1.0]: %d\n",
		buckets[0], buckets[1], buckets[2], buckets[3], buckets[4])
	fmt.Printf("Wall time:     %s\n", elapsed.Round(time.Second))
	if dryRun {
		fmt.Println("(dry-run: no events emitted)")
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
