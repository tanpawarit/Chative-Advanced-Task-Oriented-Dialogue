package orchestratornode

import (
	"context"
	"fmt"

	contractx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/contract"
)

func PlanGoal(
	ctx context.Context,
	in *GraphState,
	planner contractx.Planner,
) (*GraphState, error) {
	if in == nil || in.Session == nil {
		return nil, fmt.Errorf("%w: graph session is nil", contractx.ErrValidation)
	}

	planResp, err := planner.Plan(ctx, contractx.PlannerRequest{
		UserMessage:   in.Text,
		MemorySummary: in.MemorySummary,
		Session:       in.Session,
		Now:           in.Now,
	})
	if err != nil {
		return nil, err
	}

	in.PlanResp = planResp
	return in, nil
}
