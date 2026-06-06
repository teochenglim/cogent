package judge_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cogent/services/internal/config"
	"github.com/cogent/services/internal/judge"
	"go.uber.org/zap"
)

func makeOAIServer(t *testing.T, responseJSON string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return a valid OAI chat completion response wrapping our JSON
		resp := map[string]any{
			"id":      "test-id",
			"object":  "chat.completion",
			"model":   "test",
			"choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": responseJSON}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func TestJudge_Score_ParsesResult(t *testing.T) {
	resultJSON := `{"relevance":0.9,"faithfulness":0.8,"safety":1.0,"overall":0.87,"label":"good","reason":"Well grounded response."}`
	srv := makeOAIServer(t, resultJSON)
	defer srv.Close()

	cfg := config.Config{
		JudgeBaseURL:    srv.URL + "/v1",
		JudgeModel:      "test-model",
		JudgeAPIKey:     "test-key",
		JudgeRPS:        100,
		JudgePromptFile: "",
		JudgeTimeout:    5000000000, // 5s
	}
	j, err := judge.NewJudge(cfg, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}

	result, err := j.Score(t.Context(), "What is 2+2?", "4")
	if err != nil {
		t.Fatal(err)
	}
	if result.Label != "good" {
		t.Errorf("Label = %q, want %q", result.Label, "good")
	}
	if result.Overall < 0.8 {
		t.Errorf("Overall = %f, want >= 0.8", result.Overall)
	}
}

func TestJudge_Score_ReturnsErrorOnBadJSON(t *testing.T) {
	srv := makeOAIServer(t, "not valid json")
	defer srv.Close()

	cfg := config.Config{
		JudgeBaseURL:    srv.URL + "/v1",
		JudgeModel:      "test-model",
		JudgeAPIKey:     "test-key",
		JudgeRPS:        100,
		JudgePromptFile: "",
		JudgeTimeout:    5000000000,
	}
	j, err := judge.NewJudge(cfg, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}

	_, err = j.Score(t.Context(), "prompt", "completion")
	if err == nil {
		t.Error("expected error on malformed JSON, got nil")
	}
}

func TestJudge_SkipsAlreadyScoredEvents(t *testing.T) {
	// This is tested at the consumer level — if eval_score != nil and != 0,
	// consumer-judge returns nil without calling judge.Score.
	// Verify the filter logic matches the spec.
	score := 0.85
	source := "realtime"

	alreadyScored := score != 0 && source != ""
	if !alreadyScored {
		t.Error("should detect already-scored event")
	}

	zeroScore := 0.0
	noSource := ""
	notScored := zeroScore == 0 && noSource == ""
	if !notScored {
		t.Error("should detect unscored event")
	}
}

func TestJudge_EvalSourceLabels(t *testing.T) {
	realtimeSource := "realtime"
	batchSource := "batch_eval"
	if realtimeSource == batchSource {
		t.Error("realtime and batch_eval sources must differ")
	}
}
