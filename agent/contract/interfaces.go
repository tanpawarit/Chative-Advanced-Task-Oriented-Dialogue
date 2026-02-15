package contract

import "context"

type Planner interface {
	Plan(ctx context.Context, req PlannerRequest) (PlannerResponse, error)
}

type Specialist interface {
	Run(ctx context.Context, req SpecialistRequest) (SpecialistResponse, error)
}

type Registry interface {
	Planner() Planner
	Sales() Specialist
	Support() Specialist
}

type ToolGateway interface {
	Execute(ctx context.Context, agentType string, reqs []ToolRequest) ([]ToolResult, error)
}

type MemoryStore interface {
	ReadSummary(ctx context.Context, customerID string) (string, error)
	WriteSummary(ctx context.Context, customerID string, update string) error
}
