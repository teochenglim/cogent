package config_test

import (
	"flag"
	"testing"
	"time"

	"github.com/cogent/services/internal/config"
)

func TestConfig_LoadDefaultsAndFlagOverride(t *testing.T) {
	cfg := config.Load()

	if cfg.Topic != "cogent-telemetry" {
		t.Errorf("Topic default: got %q", cfg.Topic)
	}
	if cfg.JudgeTimeout != 30*time.Second {
		t.Errorf("JudgeTimeout default: got %v", cfg.JudgeTimeout)
	}

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	config.BindFlags(&cfg, fs)
	if err := fs.Parse([]string{"-judge-model", "gpt-4", "-port", "9000"}); err != nil {
		t.Fatal(err)
	}

	if cfg.JudgeModel != "gpt-4" {
		t.Errorf("JudgeModel flag override: got %q", cfg.JudgeModel)
	}
	if cfg.ServerPort != "9000" {
		t.Errorf("ServerPort flag override: got %q", cfg.ServerPort)
	}
}
