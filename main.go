package main

import (
	"context"
	"fmt"

	specialistx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/agents/specialist"
	llmx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/llm"
	statex "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/state"
	configx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/pkg/config"
	_ "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/pkg/logger/autoload"
	openrouterx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/pkg/openrouter"
)

type AppConfig struct {
	ZepAPIKey string `envconfig:"ZEP_API_KEY" required:"true"`
	LLMModel  string `envconfig:"LLM_MODEL" required:"true"`
}

func main() {
	_ = configx.MustNew[AppConfig]("")

	openRouterCfg := configx.MustNew[openrouterx.Config]("OPENROUTER")
	if openrouterx.NewClient(*openRouterCfg) == nil {
		panic("failed to initialize openrouter client")
	}

	modelCfg := configx.MustNew[llmx.Config]("OPENROUTER")
	if _, err := specialistx.NewRegistry(context.Background(), *modelCfg); err != nil {
		panic(err)
	}

	upstashRedisCfg := configx.MustNew[statex.UpstashRedisConfig]("UPSTASH_REDIS")
	if _, err := statex.NewUpstashRedisStore(*upstashRedisCfg); err != nil {
		panic(err)
	}

	fmt.Println("Config and clients loaded")
}
