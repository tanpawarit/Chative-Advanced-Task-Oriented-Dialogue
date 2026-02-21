package orchestratornode

import (
	"context"
	"fmt"
	"strings"

	contractx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/contract"
	statex "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/state"
)

func DispatchSpecialist(
	ctx context.Context,
	in *GraphState,
	models contractx.Registry,
) (*GraphState, error) {
	if in == nil || in.ActiveGoal == nil {
		return nil, ErrNoActiveGoal
	}

	msg, updates, err := dispatchToSpecialist(ctx, in.Text, in.MemorySummary, in.ActiveGoal, models)
	if err != nil {
		return nil, err
	}

	in.Message = msg
	in.StateUpdates = updates
	return in, nil
}

func dispatchToSpecialist(
	ctx context.Context,
	userMessage string,
	memorySummary string,
	activeGoal *statex.Goal,
	models contractx.Registry,
) (string, contractx.StateUpdates, error) {
	specialist, _, err := pickSpecialist(activeGoal, models)
	if err != nil {
		return "", contractx.StateUpdates{}, err
	}

	req := contractx.SpecialistRequest{
		UserMessage:   userMessage,
		MemorySummary: memorySummary,
		ActiveGoal:    activeGoal,
	}

	resp, err := specialist.Run(ctx, req)
	if err != nil {
		return "", contractx.StateUpdates{}, err
	}
	if len(resp.ToolRequests) > 0 {
		return "", contractx.StateUpdates{}, fmt.Errorf("%w: specialist returned unexpected tool requests", contractx.ErrSchemaViolation)
	}

	return strings.TrimSpace(resp.Message), resp.StateUpdates, nil
}

func pickSpecialist(activeGoal *statex.Goal, models contractx.Registry) (contractx.Specialist, contractx.AgentType, error) {
	if activeGoal == nil {
		return nil, "", ErrNoActiveGoal
	}

	goalType := strings.TrimSpace(activeGoal.Type)
	switch {
	case strings.HasPrefix(goalType, "sales."):
		return models.Sales(), contractx.AgentTypeSales, nil
	case strings.HasPrefix(goalType, "support."):
		return models.Support(), contractx.AgentTypeSupport, nil
	default:
		return nil, "", fmt.Errorf("%w: unsupported goal type=%q", contractx.ErrValidation, goalType)
	}
}
