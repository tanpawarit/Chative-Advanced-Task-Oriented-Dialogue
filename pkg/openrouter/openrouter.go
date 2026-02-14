package openrouter

import (
	"context"
	"fmt"
	"strings"
	"time"

	openaimodel "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	openaisdk "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

type LLMBuilder interface {
	New(ctx context.Context) (model.ToolCallingChatModel, error)
}

var _ LLMBuilder = (*OpenRouterConfig)(nil)

var (
	OpenRouterReasoningBlacklist = map[string]bool{
		"x-ai/grok-4.1-fast": true,
	}
)

type OpenRouterConfig struct {
	BaseURL            string        `envconfig:"BASE_URL" split_words:"true" default:"https://openrouter.ai/api/v1"`
	APIKey             string        `envconfig:"API_KEY" split_words:"true" required:"true"`
	Model              string        `envconfig:"MODEL" split_words:"true" required:"true"`
	MaxCompletionToken *int          `envconfig:"MAX_COMPLETION_TOKEN" split_words:"true" default:"2000"`
	Temperature        float32       `envconfig:"TEMPERATURE" split_words:"true" default:"0.5"`
	Timeout            time.Duration `envconfig:"TIMEOUT" split_words:"true" default:"30s"`
	SiteURL            string        `envconfig:"SITE_URL" split_words:"true"`
	SiteName           string        `envconfig:"SITE_NAME" split_words:"true"`
}

// Config is kept as an alias for backward compatibility.
type Config = OpenRouterConfig

func (c *OpenRouterConfig) New(ctx context.Context) (model.ToolCallingChatModel, error) {
	modelName := strings.TrimSpace(c.Model)

	conf := &openaimodel.ChatModelConfig{
		BaseURL:     strings.TrimRight(c.BaseURL, "/"),
		APIKey:      strings.TrimSpace(c.APIKey),
		Model:       modelName,
		MaxTokens:   c.MaxCompletionToken,
		Temperature: &c.Temperature,
		Timeout:     c.Timeout,
	}

	if OpenRouterReasoningBlacklist[modelName] {
		conf.ExtraFields = map[string]any{
			"reasoning": map[string]any{
				"exclude": true,
				"effort":  "none",
			},
		}
	}

	m, err := openaimodel.NewChatModel(ctx, conf)
	if err != nil {
		return nil, fmt.Errorf("openrouter: create chat model: %w", err)
	}

	return m, nil
}

// NewClient creates a new OpenAI SDK client configured for OpenRouter.
func NewClient(cfg Config) *openaisdk.Client {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil
	}

	opts := []option.RequestOption{
		option.WithAPIKey(strings.TrimSpace(cfg.APIKey)),
	}

	if trimmed := strings.TrimRight(cfg.BaseURL, "/"); trimmed != "" {
		opts = append(opts, option.WithBaseURL(trimmed))
	}

	// Add OpenRouter specific headers
	if cfg.SiteURL != "" {
		opts = append(opts, option.WithHeader("HTTP-Referer", cfg.SiteURL))
	}
	if cfg.SiteName != "" {
		opts = append(opts, option.WithHeader("X-Title", cfg.SiteName))
	}

	client := openaisdk.NewClient(opts...)
	return &client
}
