package specialist

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	contractx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/contract"
	statex "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/state"
)

type plannerImpl struct {
	runner compose.Runnable[map[string]any, plannerLLMOutput]
}

type plannerLLMOutput struct {
	GoalID       string         `json:"goal_id,omitempty"`
	GoalType     string         `json:"goal_type"`
	Priority     int            `json:"priority"`
	SlotsPatch   map[string]any `json:"slots_patch,omitempty"`
	Missing      []string       `json:"missing,omitempty"`
	NextQuestion string         `json:"next_question,omitempty"`
}

func newPlanner(ctx context.Context, chatModel einomodel.BaseChatModel, systemPrompt string) (*plannerImpl, error) {
	runner, err := compilePlannerGraph(ctx, chatModel, systemPrompt)
	if err != nil {
		return nil, fmt.Errorf("%w: compile planner graph: %v", contractx.ErrModelInvoke, err)
	}
	return &plannerImpl{runner: runner}, nil
}

func (p *plannerImpl) Plan(ctx context.Context, req contractx.PlannerRequest) (contractx.PlannerResponse, error) {
	if strings.TrimSpace(req.UserMessage) == "" {
		return contractx.PlannerResponse{}, fmt.Errorf("%w: user message is required", contractx.ErrValidation)
	}

	payload := map[string]any{
		"user_message":   req.UserMessage,
		"memory_summary": req.MemorySummary,
		"session":        summarizeSession(req.Session),
	}
	inputBytes, err := json.Marshal(payload)
	if err != nil {
		return contractx.PlannerResponse{}, fmt.Errorf("%w: marshal planner payload: %v", contractx.ErrValidation, err)
	}

	out, err := p.runner.Invoke(ctx, map[string]any{
		"input": string(inputBytes),
	})
	if err != nil {
		return contractx.PlannerResponse{}, fmt.Errorf("%w: planner invoke: %v", contractx.ErrModelInvoke, err)
	}

	resp := contractx.PlannerResponse{
		Goal: contractx.GoalPatch{
			GoalID:       strings.TrimSpace(out.GoalID),
			GoalType:     strings.TrimSpace(out.GoalType),
			Priority:     out.Priority,
			SlotsPatch:   out.SlotsPatch,
			Missing:      out.Missing,
			NextQuestion: strings.TrimSpace(out.NextQuestion),
		},
	}

	if err := validatePlannerResponse(resp); err != nil {
		return contractx.PlannerResponse{}, err
	}

	return resp, nil
}

func validatePlannerResponse(resp contractx.PlannerResponse) error {
	goalType := strings.TrimSpace(resp.Goal.GoalType)
	if !isSupportedGoalType(goalType) {
		return fmt.Errorf("%w: unsupported goal_type=%q", contractx.ErrSchemaViolation, goalType)
	}
	if resp.Goal.Priority <= 0 {
		return fmt.Errorf("%w: priority must be > 0", contractx.ErrSchemaViolation)
	}
	if resp.Goal.SlotsPatch == nil {
		resp.Goal.SlotsPatch = map[string]any{}
	}
	if len(resp.Goal.Missing) > 0 && strings.TrimSpace(resp.Goal.NextQuestion) == "" {
		return fmt.Errorf("%w: blocked goal must include next_question", contractx.ErrSchemaViolation)
	}
	if len(resp.Goal.Missing) == 0 {
		resp.Goal.NextQuestion = ""
	}
	return nil
}

func summarizeSession(st *statex.SessionState) map[string]any {
	if st == nil {
		return map[string]any{}
	}

	goals := make([]map[string]any, 0, len(st.Goals))
	for id, g := range st.Goals {
		if g == nil {
			continue
		}
		goals = append(goals, map[string]any{
			"id":            id,
			"type":          g.Type,
			"status":        g.Status,
			"priority":      g.Priority,
			"slots":         g.Slots,
			"missing":       g.Missing,
			"next_question": g.NextQuestion,
		})
	}

	return map[string]any{
		"active_goal_id": st.ActiveGoalID,
		"goal_stack":     st.GoalStack,
		"goals":          goals,
	}
}

func isSupportedGoalType(goalType string) bool {
	return strings.HasPrefix(goalType, "sales.") || strings.HasPrefix(goalType, "support.")
}
