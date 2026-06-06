package judge

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cogent/services/internal/config"
	openai "github.com/sashabaranov/go-openai"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

//go:embed default_prompt.txt
var defaultPrompt string

// Result holds the parsed LLM scoring response.
type Result struct {
	Relevance    float64 `json:"relevance"`
	Faithfulness float64 `json:"faithfulness"`
	Safety       float64 `json:"safety"`
	Overall      float64 `json:"overall"`
	Label        string  `json:"label"` // good | acceptable | bad
	Reason       string  `json:"reason"`
}

// Judge evaluates prompt+completion pairs via an OAI-compatible LLM.
type Judge struct {
	client     *openai.Client
	model      string
	promptTmpl string
	limiter    *rate.Limiter
	timeout    time.Duration
	logger     *zap.Logger
}

// NewJudge creates a Judge from config.
// If cfg.JudgePromptFile is non-empty, loads the prompt from that path.
// Otherwise falls back to the embedded default prompt.
func NewJudge(cfg config.Config, logger *zap.Logger) (*Judge, error) {
	oacfg := openai.DefaultConfig(cfg.JudgeAPIKey)
	oacfg.BaseURL = cfg.JudgeBaseURL
	client := openai.NewClientWithConfig(oacfg)

	tmpl := defaultPrompt
	if cfg.JudgePromptFile != "" {
		data, err := os.ReadFile(cfg.JudgePromptFile)
		if err != nil {
			return nil, fmt.Errorf("judge: read prompt file %s: %w", cfg.JudgePromptFile, err)
		}
		tmpl = string(data)
	}

	limiter := rate.NewLimiter(rate.Limit(cfg.JudgeRPS), 1)

	return &Judge{
		client:     client,
		model:      cfg.JudgeModel,
		promptTmpl: tmpl,
		limiter:    limiter,
		timeout:    cfg.JudgeTimeout,
		logger:     logger,
	}, nil
}

// Score evaluates one prompt+completion pair.
// Blocks until the rate limiter permits the request.
func (j *Judge) Score(ctx context.Context, prompt, completion string) (Result, error) {
	if err := j.limiter.Wait(ctx); err != nil {
		return Result{}, fmt.Errorf("judge: rate limiter: %w", err)
	}

	filled := strings.ReplaceAll(j.promptTmpl, "{{PROMPT}}", prompt)
	filled = strings.ReplaceAll(filled, "{{COMPLETION}}", completion)

	reqCtx, cancel := context.WithTimeout(ctx, j.timeout)
	defer cancel()

	resp, err := j.client.CreateChatCompletion(reqCtx, openai.ChatCompletionRequest{
		Model: j.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: filled},
		},
		Temperature: 0,
	})
	if err != nil {
		return Result{}, fmt.Errorf("judge: LLM call: %w", err)
	}

	if len(resp.Choices) == 0 {
		return Result{}, fmt.Errorf("judge: no choices in response")
	}

	raw := resp.Choices[0].Message.Content
	var result Result
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		j.logger.Error("judge: failed to parse LLM JSON", zap.String("raw", raw), zap.Error(err))
		return Result{}, fmt.Errorf("judge: parse response JSON: %w", err)
	}
	return result, nil
}
