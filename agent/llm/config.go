package llm

import (
	"fmt"
	"strings"
	"time"

	contractx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/contract"
	openrouterx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/pkg/openrouter"
)

type Config struct {
	BaseURL            string        `envconfig:"BASE_URL" split_words:"true" default:"https://openrouter.ai/api/v1"`
	APIKey             string        `envconfig:"API_KEY" split_words:"true" required:"true"`
	Model              string        `envconfig:"MODEL" split_words:"true" required:"true"`
	MaxCompletionToken int           `envconfig:"MAX_COMPLETION_TOKEN" split_words:"true" default:"2000"`
	Temperature        float32       `envconfig:"TEMPERATURE" split_words:"true" default:"0.5"`
	Timeout            time.Duration `envconfig:"TIMEOUT" split_words:"true" default:"30s"`
	SiteURL            string        `envconfig:"SITE_URL" split_words:"true"`
	SiteName           string        `envconfig:"SITE_NAME" split_words:"true"`

	OrchestratorModel       string  `envconfig:"ORCHESTRATOR_MODEL" split_words:"true"`
	SalesModel              string  `envconfig:"SALES_MODEL" split_words:"true"`
	SupportModel            string  `envconfig:"SUPPORT_MODEL" split_words:"true"`
	OrchestratorTemperature float32 `envconfig:"ORCHESTRATOR_TEMPERATURE" split_words:"true" default:"-1"`
	SalesTemperature        float32 `envconfig:"SALES_TEMPERATURE" split_words:"true" default:"-1"`
	SupportTemperature      float32 `envconfig:"SUPPORT_TEMPERATURE" split_words:"true" default:"-1"`
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.APIKey) == "" {
		return fmt.Errorf("%w: openrouter api key is required", contractx.ErrValidation)
	}
	if strings.TrimSpace(c.Model) == "" {
		return fmt.Errorf("%w: default model is required", contractx.ErrValidation)
	}
	return nil
}

func (c Config) OpenRouterFor(agentType contractx.AgentType) openrouterx.Config {
	modelName := strings.TrimSpace(c.Model)
	temp := c.Temperature

	switch agentType {
	case contractx.AgentTypePlanner:
		if v := strings.TrimSpace(c.OrchestratorModel); v != "" {
			modelName = v
		}
		if c.OrchestratorTemperature >= 0 {
			temp = c.OrchestratorTemperature
		}
	case contractx.AgentTypeSales:
		if v := strings.TrimSpace(c.SalesModel); v != "" {
			modelName = v
		}
		if c.SalesTemperature >= 0 {
			temp = c.SalesTemperature
		}
	case contractx.AgentTypeSupport:
		if v := strings.TrimSpace(c.SupportModel); v != "" {
			modelName = v
		}
		if c.SupportTemperature >= 0 {
			temp = c.SupportTemperature
		}
	}

	maxCompletionToken := c.MaxCompletionToken
	return openrouterx.Config{
		BaseURL:            strings.TrimSpace(c.BaseURL),
		APIKey:             strings.TrimSpace(c.APIKey),
		Model:              modelName,
		MaxCompletionToken: &maxCompletionToken,
		Temperature:        temp,
		Timeout:            c.Timeout,
		SiteURL:            strings.TrimSpace(c.SiteURL),
		SiteName:           strings.TrimSpace(c.SiteName),
	}
}
