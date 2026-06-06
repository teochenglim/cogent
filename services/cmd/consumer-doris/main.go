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
	"github.com/cogent/services/internal/doris"
	"github.com/cogent/services/internal/storage"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg := config.Load()
	config.BindFlags(&cfg, flag.CommandLine)
	flag.Parse()

	store, err := storage.NewClient(cfg, logger)
	if err != nil {
		logger.Fatal("failed to create storage client", zap.Error(err))
	}

	writer, err := doris.NewWriter(cfg, store, logger)
	if err != nil {
		logger.Fatal("failed to create doris writer", zap.Error(err))
	}

	bc := consumer.NewBaseConsumer(
		cfg.BootstrapServers,
		cfg.Topic,
		"cogent-doris",
		1000,
		5*time.Second,
		writer,
		logger,
	)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	logger.Info("consumer-doris started")
	bc.Run(ctx)
	logger.Info("consumer-doris stopped")
}
