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
	appCfg := configx.MustNew[AppConfig]("")
	_ = appCfg

	openRouterCfg := configx.MustNew[openrouterx.Config]("OPENROUTER")
	openRouterClient := openrouterx.NewClient(*openRouterCfg)
	if openRouterClient == nil {
		panic("failed to initialize openrouter client")
	}
	modelCfg := configx.MustNew[llmx.Config]("OPENROUTER")
	modelRegistry, err := specialistx.NewRegistry(context.Background(), *modelCfg)
	if err != nil {
		panic(err)
	}
	_ = modelRegistry

	upstashRedisCfg := configx.MustNew[statex.UpstashRedisConfig]("UPSTASH_REDIS")
	upstashRedisStore, err := statex.NewUpstashRedisStore(*upstashRedisCfg)
	if err != nil {
		panic(err)
	}
	_ = upstashRedisStore

	fmt.Println("Config and clients loaded")
}
