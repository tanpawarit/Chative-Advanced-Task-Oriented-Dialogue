package specialist

import (
	"context"
	"fmt"

	contractx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/contract"
	llmx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/llm"
	promptx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/prompt"
)

type registryImpl struct {
	planner contractx.Planner
	sales   contractx.Specialist
	support contractx.Specialist
}

func (r *registryImpl) Planner() contractx.Planner {
	return r.planner
}

func (r *registryImpl) Sales() contractx.Specialist {
	return r.sales
}

func (r *registryImpl) Support() contractx.Specialist {
	return r.support
}

func NewRegistry(ctx context.Context, cfg llmx.Config) (contractx.Registry, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	prompts := promptx.LoadPromptSet()

	orchestratorModelCfg := cfg.OpenRouterFor(contractx.AgentTypeOrchestrator)
	orchestratorModel, err := orchestratorModelCfg.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: create orchestrator model: %v", contractx.ErrModelInvoke, err)
	}
	salesModelCfg := cfg.OpenRouterFor(contractx.AgentTypeSales)
	salesModel, err := salesModelCfg.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: create sales model: %v", contractx.ErrModelInvoke, err)
	}
	supportModelCfg := cfg.OpenRouterFor(contractx.AgentTypeSupport)
	supportModel, err := supportModelCfg.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: create support model: %v", contractx.ErrModelInvoke, err)
	}

	planner, err := newPlanner(ctx, orchestratorModel, prompts.Orchestrator)
	if err != nil {
		return nil, err
	}

	sales, err := newSpecialist(ctx, contractx.AgentTypeSales, salesModel, prompts.Sales)
	if err != nil {
		return nil, err
	}
	support, err := newSpecialist(ctx, contractx.AgentTypeSupport, supportModel, prompts.Support)
	if err != nil {
		return nil, err
	}

	return &registryImpl{
		planner: planner,
		sales:   sales,
		support: support,
	}, nil
}
