package main

import (
	"fmt"

	configx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/pkg/config"
	_ "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/pkg/logger/autoload"
	openrouterx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/pkg/openrouter"
	qstashx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/pkg/qstash"
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

	qstashCfg := configx.MustNew[qstashx.Config]("QSTASH")
	qstashClient := qstashx.MustNew(*qstashCfg)
	_ = qstashClient

	fmt.Println("Config and clients loaded")
}
