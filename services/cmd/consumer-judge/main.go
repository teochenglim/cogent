package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cogent/services/internal/config"
	"github.com/cogent/services/internal/consumer"
	"github.com/cogent/services/internal/judge"
	"github.com/cogent/services/internal/schema"
	"github.com/cogent/services/internal/storage"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type judgeConsumer struct {
	judge  *judge.Judge
	store  *storage.Client
	writer *kafka.Writer
	cfg    config.Config
	logger *zap.Logger
}

func (jc *judgeConsumer) Handle(ctx context.Context, e schema.Event) error {
	// Only score llm_call events with no existing eval
	if e.Operation != "llm_call" {
		return nil
	}
	if e.EvalScore != nil && *e.EvalScore != 0 {
		return nil
	}
	if e.EvalSource != nil && *e.EvalSource != "" {
		return nil
	}

	prompt, err := jc.store.Fetch(ctx, deref(e.PromptRef))
	if err != nil || prompt == "" {
		prompt = deref(e.PromptPreview)
	}
	completion, err := jc.store.Fetch(ctx, deref(e.CompletionRef))
	if err != nil || completion == "" {
		completion = deref(e.CompletionPreview)
	}

	if prompt == "" || completion == "" {
		jc.logger.Warn("skipping span: no prompt or completion", zap.String("span_id", e.SpanID))
		return nil
	}

	result, err := jc.judge.Score(ctx, prompt, completion)
	if err != nil {
		jc.logger.Error("judge score failed", zap.String("span_id", e.SpanID), zap.Error(err))
		return err
	}

	newSpanID := uuid.New().String()
	evalSrc := "realtime"
	evalEvent := schema.Event{
		TraceID:      e.TraceID,
		SpanID:       newSpanID,
		ParentSpanID: &e.SpanID,
		StartTime:    float64(time.Now().UnixMicro()) / 1e6,
		EndTime:      float64(time.Now().UnixMicro()) / 1e6,
		DurationMs:   0,
		AgentName:    e.AgentName,
		Operation:    "evaluation",
		ServiceName:  "cogent-judge",
		Environment:  e.Environment,
		EvalScore:    &result.Overall,
		EvalLabel:    &result.Label,
		EvalReason:   &result.Reason,
		EvalSource:   &evalSrc,
	}

	data, err := evalEvent.ToJSON()
	if err != nil {
		return err
	}
	return jc.writer.WriteMessages(ctx, kafka.Message{Value: data})
}

func (jc *judgeConsumer) Flush(_ context.Context) error { return nil }

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg := config.Load()
	config.BindFlags(&cfg, flag.CommandLine)
	flag.Parse()

	store, err := storage.NewClient(cfg, logger)
	if err != nil {
		logger.Fatal("storage client", zap.Error(err))
	}

	j, err := judge.NewJudge(cfg, logger)
	if err != nil {
		logger.Fatal("judge init", zap.Error(err))
	}

	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers: []string{cfg.BootstrapServers},
		Topic:   cfg.Topic,
	})

	jc := &judgeConsumer{
		judge:  j,
		store:  store,
		writer: writer,
		cfg:    cfg,
		logger: logger,
	}

	bc := consumer.NewBaseConsumer(
		cfg.BootstrapServers,
		cfg.Topic,
		"cogent-judge",
		1,
		time.Second,
		jc,
		logger,
	)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	logger.Info("consumer-judge started",
		zap.String("model", cfg.JudgeModel),
		zap.String("base_url", cfg.JudgeBaseURL),
		zap.Float64("rps", cfg.JudgeRPS),
	)
	bc.Run(ctx)
	logger.Info("consumer-judge stopped")
}
