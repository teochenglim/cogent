package alert_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cogent/services/internal/alert"
	"github.com/cogent/services/internal/config"
	"github.com/cogent/services/internal/schema"
	"go.uber.org/zap"
)

func TestAlert_RunawayLoopFiresAtThreshold(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := config.Load()
	cfg.AlertMaxSpansPerTrace = 3
	cfg.AlertCostBudgetUSD = 1.0
	cfg.AlertMaxSpanBytes = 1 << 26
	cfg.AlertWindowSeconds = 60
	cfg.AlertWebhookURL = srv.URL

	a := alert.NewAlerter(cfg, zap.NewNop())
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_ = a.Handle(ctx, schema.Event{
			TraceID: "trace-loop", SpanID: fmt.Sprintf("s%d", i),
			AgentName: "agent", Operation: "llm_call",
			ServiceName: "svc", Environment: "test",
		})
	}
	time.Sleep(100 * time.Millisecond)

	// threshold=3 → fires exactly once at spanCount==4
	if n := hits.Load(); n != 1 {
		t.Errorf("expected 1 alert, got %d", n)
	}
}
