package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cogent/services/internal/alert"
	"github.com/cogent/services/internal/config"
	"github.com/cogent/services/internal/consumer"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg := config.Load()
	config.BindFlags(&cfg, flag.CommandLine)
	flag.Parse()

	alerter := alert.NewAlerter(cfg, logger)

	bc := consumer.NewBaseConsumer(
		cfg.BootstrapServers,
		cfg.Topic,
		"cogent-alerting",
		1, // flush after every event (alerter has no batch state)
		time.Second,
		alerter,
		logger,
	)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	logger.Info("consumer-alerting started")
	bc.Run(ctx)
	logger.Info("consumer-alerting stopped")
}
