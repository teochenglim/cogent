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
	"github.com/cogent/services/internal/greptime"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg := config.Load()
	config.BindFlags(&cfg, flag.CommandLine)
	flag.Parse()

	writer, err := greptime.NewWriter(cfg, logger)
	if err != nil {
		logger.Fatal("failed to create greptime writer", zap.Error(err))
	}

	bc := consumer.NewBaseConsumer(
		cfg.BootstrapServers,
		cfg.Topic,
		"cogent-greptime",
		500,
		2*time.Second,
		writer,
		logger,
	)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	logger.Info("consumer-greptime started")
	bc.Run(ctx)
	logger.Info("consumer-greptime stopped")
}
