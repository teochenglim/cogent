package config

import (
	"flag"
	"os"
	"strconv"
	"time"
)

// Config holds all runtime configuration for Cogent services.
// Resolution order: CLI flag > environment variable > built-in default.
type Config struct {
	// Kafka / Redpanda
	BootstrapServers string
	Topic            string

	// MinIO / S3
	MinioEndpoint  string
	MinioAccessKey string
	MinioSecretKey string
	MinioBucket    string
	MinioSecure    bool

	// GreptimeDB
	GreptimeHost     string
	GreptimePort     int
	GreptimeDatabase string

	// Doris
	DorisFEHost     string
	DorisFEHTTPPort int
	DorisMySQLPort  int
	DorisUser       string
	DorisPassword   string
	DorisDatabase   string

	// Judge
	JudgeBaseURL    string
	JudgeModel      string
	JudgeAPIKey     string
	JudgeRPS        float64
	JudgePromptFile string
	JudgeTimeout    time.Duration

	// Server
	ServerPort         string
	ServerReadTimeout  time.Duration
	ServerWriteTimeout time.Duration

	// Alerting
	AlertMaxSpansPerTrace int
	AlertCostBudgetUSD    float64
	AlertMaxSpanBytes     int64
	AlertWindowSeconds    int
	AlertWebhookURL       string
}

// Load reads configuration from environment variables with built-in defaults.
func Load() Config {
	return Config{
		BootstrapServers: envStr("BOOTSTRAP_SERVERS", "localhost:9092"),
		Topic:            envStr("TOPIC", "cogent-telemetry"),

		MinioEndpoint:  envStr("MINIO_ENDPOINT", ""),
		MinioAccessKey: envStr("MINIO_ACCESS_KEY", "minioadmin"),
		MinioSecretKey: envStr("MINIO_SECRET_KEY", "minioadmin"),
		MinioBucket:    envStr("MINIO_BUCKET", "cogent-payloads"),
		MinioSecure:    envBool("MINIO_SECURE", false),

		GreptimeHost:     envStr("GREPTIME_HOST", "localhost"),
		GreptimePort:     envInt("GREPTIME_PORT", 4002),
		GreptimeDatabase: envStr("GREPTIME_DATABASE", "public"),

		DorisFEHost:     envStr("DORIS_FE_HOST", "localhost"),
		DorisFEHTTPPort: envInt("DORIS_FE_HTTP_PORT", 8030),
		DorisMySQLPort:  envInt("DORIS_MYSQL_PORT", 9030),
		DorisUser:       envStr("DORIS_USER", "root"),
		DorisPassword:   envStr("DORIS_PASSWORD", ""),
		DorisDatabase:   envStr("DORIS_DATABASE", "cogent"),

		JudgeBaseURL:    envStr("JUDGE_BASE_URL", "http://localhost:11434/v1"),
		JudgeModel:      envStr("JUDGE_MODEL", "llama3.2"),
		JudgeAPIKey:     envStr("JUDGE_API_KEY", "ollama"),
		JudgeRPS:        envFloat("JUDGE_RPS", 2.0),
		JudgePromptFile: envStr("JUDGE_PROMPT_FILE", ""),
		JudgeTimeout:    envDuration("JUDGE_TIMEOUT", 30*time.Second),

		ServerPort:         envStr("SERVER_PORT", "8090"),
		ServerReadTimeout:  envDuration("SERVER_READ_TIMEOUT", 30*time.Second),
		ServerWriteTimeout: envDuration("SERVER_WRITE_TIMEOUT", 60*time.Second),

		AlertMaxSpansPerTrace: envInt("ALERT_MAX_SPANS_PER_TRACE", 200),
		AlertCostBudgetUSD:    envFloat("ALERT_COST_BUDGET_USD", 1.0),
		AlertMaxSpanBytes:     int64(envInt("ALERT_MAX_SPAN_BYTES", 52428800)),
		AlertWindowSeconds:    envInt("ALERT_WINDOW_SECONDS", 60),
		AlertWebhookURL:       envStr("ALERT_WEBHOOK_URL", ""),
	}
}

// BindFlags registers CLI flags that override env vars.
// Call flag.Parse() after BindFlags, then the flag values will take effect
// because Load() reads from environment but callers can patch cfg fields
// directly from flag values. Pass the *Config returned by Load().
func BindFlags(cfg *Config, fs *flag.FlagSet) {
	fs.StringVar(&cfg.BootstrapServers, "bootstrap-servers", cfg.BootstrapServers, "Kafka bootstrap servers")
	fs.StringVar(&cfg.Topic, "topic", cfg.Topic, "Kafka topic")
	fs.StringVar(&cfg.JudgeBaseURL, "judge-base-url", cfg.JudgeBaseURL, "OAI-compatible judge endpoint")
	fs.StringVar(&cfg.JudgeModel, "judge-model", cfg.JudgeModel, "Judge model name")
	fs.StringVar(&cfg.JudgeAPIKey, "judge-api-key", cfg.JudgeAPIKey, "Judge API key")
	fs.Float64Var(&cfg.JudgeRPS, "judge-rps", cfg.JudgeRPS, "Judge requests per second")
	fs.StringVar(&cfg.JudgePromptFile, "judge-prompt-file", cfg.JudgePromptFile, "Path to custom judge prompt")
	fs.StringVar(&cfg.ServerPort, "port", cfg.ServerPort, "Server listen port")
	fs.StringVar(&cfg.AlertWebhookURL, "alert-webhook-url", cfg.AlertWebhookURL, "Alert webhook URL")
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
