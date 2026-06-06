package storage_test

import (
	"context"
	"testing"

	"github.com/cogent/services/internal/config"
	"github.com/cogent/services/internal/storage"
	"go.uber.org/zap"
)

func TestStorage_NoopClientFetchReturnsEmpty(t *testing.T) {
	cfg := config.Load()
	cfg.MinioEndpoint = ""

	client, err := storage.NewClient(cfg, zap.NewNop())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	content, err := client.Fetch(context.Background(), "trace/span/prompt")
	if err != nil {
		t.Errorf("Fetch noop: unexpected error: %v", err)
	}
	if content != "" {
		t.Errorf("Fetch noop: expected empty string, got %q", content)
	}
}
