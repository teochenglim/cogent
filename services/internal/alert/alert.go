package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/cogent/services/internal/config"
	"github.com/cogent/services/internal/schema"
	"go.uber.org/zap"
)

type traceWindow struct {
	spanCount int
	costUSD   float64
	expiresAt time.Time
}

type agentWindow struct {
	costUSD   float64
	expiresAt time.Time
}

// Alerter evaluates real-time alert conditions per event.
// It maintains in-memory rolling windows using sync.Map.
type Alerter struct {
	cfg       config.Config
	traceWins sync.Map // key: trace_id → *traceWindow
	agentWins sync.Map // key: agent_name → *agentWindow
	httpCli   *http.Client
	logger    *zap.Logger
}

// NewAlerter creates an Alerter and starts the sweep goroutine.
func NewAlerter(cfg config.Config, logger *zap.Logger) *Alerter {
	a := &Alerter{
		cfg:     cfg,
		httpCli: &http.Client{Timeout: 5 * time.Second},
		logger:  logger,
	}
	go a.sweepLoop()
	return a
}

// Handle evaluates alert conditions for one event. Never blocks the consumer loop.
func (a *Alerter) Handle(_ context.Context, e schema.Event) error {
	window := time.Duration(a.cfg.AlertWindowSeconds) * time.Second
	now := time.Now()

	// --- Runaway loop: span count per trace ---
	tw := a.loadOrStoreTrace(e.TraceID, window, now)
	tw.spanCount++
	if tw.spanCount == a.cfg.AlertMaxSpansPerTrace+1 {
		a.fire("runaway_loop", map[string]any{
			"trace_id":   e.TraceID,
			"agent_name": e.AgentName,
			"span_count": tw.spanCount,
			"threshold":  a.cfg.AlertMaxSpansPerTrace,
		})
	}

	// --- Cost spike: cumulative cost per agent ---
	aw := a.loadOrStoreAgent(e.AgentName, window, now)
	if e.CostUSD != nil {
		aw.costUSD += *e.CostUSD
		if aw.costUSD >= a.cfg.AlertCostBudgetUSD && aw.costUSD-*e.CostUSD < a.cfg.AlertCostBudgetUSD {
			a.fire("cost_spike", map[string]any{
				"agent_name": e.AgentName,
				"cost_usd":   aw.costUSD,
				"budget_usd": a.cfg.AlertCostBudgetUSD,
			})
		}
	}

	// --- Payload size: single span total bytes ---
	var totalBytes int64
	if e.PromptSizeBytes != nil {
		totalBytes += *e.PromptSizeBytes
	}
	if e.CompletionSizeBytes != nil {
		totalBytes += *e.CompletionSizeBytes
	}
	if e.ToolInputSizeBytes != nil {
		totalBytes += *e.ToolInputSizeBytes
	}
	if e.ToolOutputSizeBytes != nil {
		totalBytes += *e.ToolOutputSizeBytes
	}
	if totalBytes > a.cfg.AlertMaxSpanBytes {
		a.fire("payload_size", map[string]any{
			"span_id":         e.SpanID,
			"trace_id":        e.TraceID,
			"agent_name":      e.AgentName,
			"total_bytes":     totalBytes,
			"threshold_bytes": a.cfg.AlertMaxSpanBytes,
		})
	}
	return nil
}

// Flush is a no-op — alerts are emitted per-event in real time.
func (a *Alerter) Flush(_ context.Context) error { return nil }

func (a *Alerter) loadOrStoreTrace(traceID string, window time.Duration, now time.Time) *traceWindow {
	v, _ := a.traceWins.LoadOrStore(traceID, &traceWindow{expiresAt: now.Add(window)})
	return v.(*traceWindow)
}

func (a *Alerter) loadOrStoreAgent(agentName string, window time.Duration, now time.Time) *agentWindow {
	v, _ := a.agentWins.LoadOrStore(agentName, &agentWindow{expiresAt: now.Add(window)})
	return v.(*agentWindow)
}

func (a *Alerter) fire(kind string, fields map[string]any) {
	fields["alert"] = kind
	fields["ts"] = time.Now().UTC().Format(time.RFC3339)

	data, _ := json.Marshal(fields)
	a.logger.Warn("ALERT", zap.String("kind", kind), zap.String("payload", string(data)))

	if a.cfg.AlertWebhookURL != "" {
		go func() {
			resp, err := a.httpCli.Post(a.cfg.AlertWebhookURL, "application/json", bytes.NewReader(data))
			if err != nil {
				a.logger.Warn("webhook post failed", zap.Error(err))
				return
			}
			resp.Body.Close()
		}()
	}
}

// sweepLoop removes expired window entries every 10 seconds.
func (a *Alerter) sweepLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		a.traceWins.Range(func(k, v any) bool {
			if v.(*traceWindow).expiresAt.Before(now) {
				a.traceWins.Delete(k)
			}
			return true
		})
		a.agentWins.Range(func(k, v any) bool {
			if v.(*agentWindow).expiresAt.Before(now) {
				a.agentWins.Delete(k)
			}
			return true
		})
	}
}
