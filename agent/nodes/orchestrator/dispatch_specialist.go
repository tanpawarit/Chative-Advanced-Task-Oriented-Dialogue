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
	tools contractx.ToolGateway,
) (*GraphState, error) {
	if in == nil || in.ActiveGoal == nil {
		return nil, ErrNoActiveGoal
	}

	msg, updates, err := dispatchToSpecialist(ctx, in.Text, in.MemorySummary, in.ActiveGoal, models, tools)
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
	tools contractx.ToolGateway,
) (string, contractx.StateUpdates, error) {
	specialist, agentType, err := pickSpecialist(activeGoal, models)
	if err != nil {
		return "", contractx.StateUpdates{}, err
	}

	req := contractx.SpecialistRequest{
		UserMessage:   userMessage,
		MemorySummary: memorySummary,
		ActiveGoal:    activeGoal,
	}

	pass1, err := specialist.Run(ctx, req)
	if err != nil {
		return "", contractx.StateUpdates{}, err
	}

	if len(pass1.ToolRequests) == 0 {
		return strings.TrimSpace(pass1.Message), pass1.StateUpdates, nil
	}

	toolResults, err := tools.Execute(ctx, string(agentType), pass1.ToolRequests)
	if err != nil {
		return "", contractx.StateUpdates{}, err
	}

	req.ToolResults = toolResults
	pass2, err := specialist.Run(ctx, req)
	if err != nil {
		return "", contractx.StateUpdates{}, err
	}
	if len(pass2.ToolRequests) > 0 {
		return "", contractx.StateUpdates{}, fmt.Errorf("%w: specialist requested tools in pass 2", contractx.ErrSchemaViolation)
	}

	merged := mergeStateUpdates(pass1.StateUpdates, pass2.StateUpdates)
	return strings.TrimSpace(pass2.Message), merged, nil
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

func mergeStateUpdates(a, b contractx.StateUpdates) contractx.StateUpdates {
	out := contractx.StateUpdates{SlotsPatch: map[string]any{}}

	for k, v := range a.SlotsPatch {
		out.SlotsPatch[k] = v
	}
	for k, v := range b.SlotsPatch {
		out.SlotsPatch[k] = v
	}

	if len(a.Missing) > 0 {
		out.Missing = a.Missing
	}
	if len(b.Missing) > 0 {
		out.Missing = b.Missing
	}
	if strings.TrimSpace(a.NextQuestion) != "" {
		out.NextQuestion = strings.TrimSpace(a.NextQuestion)
	}
	if strings.TrimSpace(b.NextQuestion) != "" {
		out.NextQuestion = strings.TrimSpace(b.NextQuestion)
	}
	if strings.TrimSpace(a.SetStatus) != "" {
		out.SetStatus = strings.TrimSpace(a.SetStatus)
	}
	if strings.TrimSpace(b.SetStatus) != "" {
		out.SetStatus = strings.TrimSpace(b.SetStatus)
	}
	out.MarkDone = a.MarkDone || b.MarkDone

	if strings.TrimSpace(a.MemoryUpdate) != "" {
		out.MemoryUpdate = strings.TrimSpace(a.MemoryUpdate)
	}
	if strings.TrimSpace(b.MemoryUpdate) != "" {
		out.MemoryUpdate = strings.TrimSpace(b.MemoryUpdate)
	}

	if len(out.SlotsPatch) == 0 {
		out.SlotsPatch = nil
	}
	return out
}
