package specialist

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	einomodel "github.com/cloudwego/eino/components/model"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	einoagent "github.com/cloudwego/eino/flow/agent"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	contractx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/contract"
	statex "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/state"
	toolx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/tool"
)

type reactGenerator interface {
	Generate(ctx context.Context, input []*schema.Message, opts ...einoagent.AgentOption) (*schema.Message, error)
}

type toolExecutor = toolx.Executor

type specialistImpl struct {
	agentType    contractx.AgentType
	systemPrompt string
	reactAgent   reactGenerator
}

type specialistLLMOutput struct {
	Message      string                 `json:"message"`
	StateUpdates contractx.StateUpdates `json:"state_updates,omitempty"`
}

type specialistGoalSummary struct {
	ID           string            `json:"id,omitempty"`
	Type         string            `json:"type,omitempty"`
	Status       statex.GoalStatus `json:"status,omitempty"`
	Priority     int               `json:"priority,omitempty"`
	Slots        map[string]any    `json:"slots,omitempty"`
	Missing      []string          `json:"missing,omitempty"`
	NextQuestion string            `json:"next_question,omitempty"`
}

type specialistPayload struct {
	UserMessage   string                `json:"user_message"`
	MemorySummary string                `json:"memory_summary"`
	ActiveGoal    specialistGoalSummary `json:"active_goal"`
}

type reactPhaseResult struct {
	ActMessage string
}

func newSpecialist(
	ctx context.Context,
	agentType contractx.AgentType,
	chatModel einomodel.ToolCallingChatModel,
	systemPrompt string,
) (*specialistImpl, error) {
	toolInfos, executeTool := toolx.BuildForAgent(agentType)
	executor := executeTool
	if executor == nil {
		executor = toolx.DefaultExecutor(agentType)
	}
	reactTools := make([]einotool.BaseTool, 0, len(toolInfos))
	for _, ti := range toolInfos {
		if ti == nil || strings.TrimSpace(ti.Name) == "" {
			continue
		}
		reactTools = append(reactTools, &reactToolAdapter{
			info:     ti,
			executor: executor,
		})
	}

	reactAgent, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: chatModel,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools:               reactTools,
			ExecuteSequentially: true,
		},
		GraphName: fmt.Sprintf("specialist.%s.react_agent", agentType),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: compile specialist react agent: %v", contractx.ErrModelInvoke, err)
	}

	spec := &specialistImpl{
		agentType:    agentType,
		systemPrompt: systemPrompt,
		reactAgent:   reactAgent,
	}

	return spec, nil
}

func (s *specialistImpl) Run(ctx context.Context, req contractx.SpecialistRequest) (contractx.SpecialistResponse, error) {
	if req.ActiveGoal == nil {
		return contractx.SpecialistResponse{}, fmt.Errorf("%w: active goal is required", contractx.ErrValidation)
	}
	if strings.TrimSpace(req.ActiveGoal.Type) == "" {
		return contractx.SpecialistResponse{}, fmt.Errorf("%w: active goal type is required", contractx.ErrValidation)
	}

	reactOut, err := s.runReAct(ctx, req)
	if err != nil {
		return contractx.SpecialistResponse{}, err
	}

	if resp, ok := tryParseSpecialistJSONResponse(reactOut.ActMessage); ok {
		return resp, nil
	}
	return contractx.SpecialistResponse{}, fmt.Errorf("%w: failed to parse JSON from specialist response (model might not have followed JSON instruction): %s", contractx.ErrModelInvoke, reactOut.ActMessage)
}

func tryParseSpecialistJSONResponse(raw string) (contractx.SpecialistResponse, bool) {
	out, ok := parseSpecialistLLMOutput(raw)
	if !ok {
		return contractx.SpecialistResponse{}, false
	}

	message := strings.TrimSpace(out.Message)
	if message == "" {
		return contractx.SpecialistResponse{}, false
	}
	if len(out.StateUpdates.Missing) > 0 && strings.TrimSpace(out.StateUpdates.NextQuestion) == "" {
		return contractx.SpecialistResponse{}, false
	}
	if strings.EqualFold(out.StateUpdates.SetStatus, string(statex.GoalDone)) {
		out.StateUpdates.MarkDone = true
	}

	return contractx.SpecialistResponse{
		Message:      message,
		StateUpdates: out.StateUpdates,
	}, true
}

func parseSpecialistLLMOutput(raw string) (specialistLLMOutput, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return specialistLLMOutput{}, false
	}

	candidates := []string{trimmed}
	if strings.HasPrefix(trimmed, "```") {
		unfenced := strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
		if idx := strings.IndexByte(unfenced, '\n'); idx >= 0 {
			unfenced = unfenced[idx+1:]
		}
		unfenced = strings.TrimSpace(strings.TrimSuffix(unfenced, "```"))
		if unfenced != "" {
			candidates = append(candidates, unfenced)
		}
	}

	if start := strings.IndexByte(trimmed, '{'); start >= 0 {
		if end := strings.LastIndexByte(trimmed, '}'); end > start {
			candidates = append(candidates, trimmed[start:end+1])
		}
	}

	for _, c := range candidates {
		var out specialistLLMOutput
		if err := json.Unmarshal([]byte(c), &out); err != nil {
			continue
		}
		if strings.TrimSpace(out.Message) == "" {
			continue
		}
		return out, true
	}

	return specialistLLMOutput{}, false
}

func (s *specialistImpl) runReAct(
	ctx context.Context,
	req contractx.SpecialistRequest,
) (reactPhaseResult, error) {
	payload := specialistPayload{
		UserMessage:   req.UserMessage,
		MemorySummary: req.MemorySummary,
		ActiveGoal:    summarizeGoal(req.ActiveGoal),
	}
	input, err := json.Marshal(payload)
	if err != nil {
		return reactPhaseResult{}, fmt.Errorf("%w: marshal tool planning payload: %v", contractx.ErrValidation, err)
	}

	msg, err := s.reactAgent.Generate(ctx, []*schema.Message{
		schema.SystemMessage(s.systemPrompt),
		schema.UserMessage(string(input)),
	})
	if err != nil {
		return reactPhaseResult{}, fmt.Errorf("%w: specialist react invoke: %v", contractx.ErrModelInvoke, err)
	}

	content := ""
	if msg != nil {
		content = strings.TrimSpace(msg.Content)
	}

	return reactPhaseResult{
		ActMessage: content,
	}, nil
}

type reactToolAdapter struct {
	info     *schema.ToolInfo
	executor toolExecutor
}

var _ einotool.InvokableTool = (*reactToolAdapter)(nil)

func (t *reactToolAdapter) Info(ctx context.Context) (*schema.ToolInfo, error) {
	if t == nil || t.info == nil || strings.TrimSpace(t.info.Name) == "" {
		return nil, fmt.Errorf("%w: tool info is required", contractx.ErrValidation)
	}
	return t.info, nil
}

func (t *reactToolAdapter) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...einotool.Option) (string, error) {
	if t == nil || t.info == nil || strings.TrimSpace(t.info.Name) == "" {
		return "", fmt.Errorf("%w: tool metadata is missing", contractx.ErrValidation)
	}

	if t.executor == nil {
		return "", fmt.Errorf("%w: tool executor is not configured", contractx.ErrValidation)
	}

	args := map[string]any{}
	rawArgs := strings.TrimSpace(argumentsInJSON)
	if rawArgs != "" {
		if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
			return "", fmt.Errorf("%w: invalid tool args for tool=%s: %v", contractx.ErrSchemaViolation, t.info.Name, err)
		}
	}

	result, err := t.executor(ctx, t.info.Name, args)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(result.Tool) == "" {
		result.Tool = t.info.Name
	}

	content, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("%w: marshal tool result for tool=%s: %v", contractx.ErrValidation, t.info.Name, err)
	}
	return string(content), nil
}

func summarizeGoal(g *statex.Goal) specialistGoalSummary {
	if g == nil {
		return specialistGoalSummary{}
	}
	return specialistGoalSummary{
		ID:           g.ID,
		Type:         g.Type,
		Status:       g.Status,
		Priority:     g.Priority,
		Slots:        g.Slots,
		Missing:      g.Missing,
		NextQuestion: g.NextQuestion,
	}
}
