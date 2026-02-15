package specialist

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	contractx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/contract"
	statex "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/state"
)

type specialistImpl struct {
	agentType        contractx.AgentType
	structuredRunner compose.Runnable[map[string]any, specialistLLMOutput]
	toolRunner       compose.Runnable[map[string]any, *schema.Message]
	runtimeRunner    compose.Runnable[contractx.SpecialistRequest, contractx.SpecialistResponse]
	allowedTools     map[string]struct{}
}

type specialistLLMOutput struct {
	Message      string                 `json:"message"`
	StateUpdates contractx.StateUpdates `json:"state_updates,omitempty"`
}

func newSpecialist(
	ctx context.Context,
	agentType contractx.AgentType,
	chatModel einomodel.ToolCallingChatModel,
	systemPrompt string,
) (*specialistImpl, error) {
	structuredRunner, err := compileSpecialistStructuredGraph(ctx, chatModel, systemPrompt)
	if err != nil {
		return nil, fmt.Errorf("%w: compile structured specialist graph: %v", contractx.ErrModelInvoke, err)
	}

	tools := specialistTools(agentType)
	toolModel, err := chatModel.WithTools(tools)
	if err != nil {
		return nil, fmt.Errorf("%w: bind tools for specialist=%s: %v", contractx.ErrModelInvoke, agentType, err)
	}
	toolRunner, err := compileSpecialistToolPlanningGraph(ctx, toolModel, systemPrompt)
	if err != nil {
		return nil, fmt.Errorf("%w: compile tool planner graph: %v", contractx.ErrModelInvoke, err)
	}

	allowedTools := make(map[string]struct{}, len(tools))
	for _, t := range tools {
		if t == nil || strings.TrimSpace(t.Name) == "" {
			continue
		}
		allowedTools[t.Name] = struct{}{}
	}

	spec := &specialistImpl{
		agentType:        agentType,
		structuredRunner: structuredRunner,
		toolRunner:       toolRunner,
		allowedTools:     allowedTools,
	}

	runtimeRunner, err := compileSpecialistRuntimeGraph(ctx, spec.runToolPlanning, spec.runStructured)
	if err != nil {
		return nil, fmt.Errorf("%w: compile specialist runtime graph: %v", contractx.ErrModelInvoke, err)
	}
	spec.runtimeRunner = runtimeRunner

	return spec, nil
}

func (s *specialistImpl) Run(ctx context.Context, req contractx.SpecialistRequest) (contractx.SpecialistResponse, error) {
	out, err := s.runtimeRunner.Invoke(ctx, req)
	if err != nil {
		return contractx.SpecialistResponse{}, err
	}
	return out, nil
}

func (s *specialistImpl) runStructured(
	ctx context.Context,
	req contractx.SpecialistRequest,
	isBlocked bool,
) (contractx.SpecialistResponse, error) {
	mode := "finalize"
	if isBlocked {
		mode = "ask"
	}

	payload := map[string]any{
		"mode":           mode,
		"user_message":   req.UserMessage,
		"memory_summary": req.MemorySummary,
		"active_goal":    summarizeGoal(req.ActiveGoal),
		"tool_results":   req.ToolResults,
	}
	input, err := json.Marshal(payload)
	if err != nil {
		return contractx.SpecialistResponse{}, fmt.Errorf("%w: marshal specialist payload: %v", contractx.ErrValidation, err)
	}

	out, err := s.structuredRunner.Invoke(ctx, map[string]any{
		"input": string(input),
	})
	if err != nil {
		return contractx.SpecialistResponse{}, fmt.Errorf("%w: specialist invoke: %v", contractx.ErrModelInvoke, err)
	}

	message := strings.TrimSpace(out.Message)
	if message == "" {
		return contractx.SpecialistResponse{}, fmt.Errorf("%w: specialist message is empty", contractx.ErrSchemaViolation)
	}

	if len(out.StateUpdates.Missing) > 0 && strings.TrimSpace(out.StateUpdates.NextQuestion) == "" {
		return contractx.SpecialistResponse{}, fmt.Errorf("%w: next_question required when missing is set", contractx.ErrSchemaViolation)
	}

	if strings.EqualFold(out.StateUpdates.SetStatus, string(statex.GoalDone)) {
		out.StateUpdates.MarkDone = true
	}

	return contractx.SpecialistResponse{
		Message:      message,
		ToolRequests: nil,
		StateUpdates: out.StateUpdates,
	}, nil
}

func (s *specialistImpl) runToolPlanning(ctx context.Context, req contractx.SpecialistRequest) (contractx.SpecialistResponse, error) {
	payload := map[string]any{
		"mode":           "act",
		"user_message":   req.UserMessage,
		"memory_summary": req.MemorySummary,
		"active_goal":    summarizeGoal(req.ActiveGoal),
	}
	input, err := json.Marshal(payload)
	if err != nil {
		return contractx.SpecialistResponse{}, fmt.Errorf("%w: marshal tool planning payload: %v", contractx.ErrValidation, err)
	}

	msg, err := s.toolRunner.Invoke(ctx, map[string]any{
		"input": string(input),
	})
	if err != nil {
		return contractx.SpecialistResponse{}, fmt.Errorf("%w: tool planning invoke: %v", contractx.ErrModelInvoke, err)
	}
	if msg == nil {
		return contractx.SpecialistResponse{}, fmt.Errorf("%w: empty tool planning response", contractx.ErrSchemaViolation)
	}

	toolRequests, err := toToolRequests(msg.ToolCalls)
	if err != nil {
		return contractx.SpecialistResponse{}, err
	}

	if len(toolRequests) == 0 {
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			return contractx.SpecialistResponse{}, fmt.Errorf("%w: active mode requires tool requests", contractx.ErrSchemaViolation)
		}
		return contractx.SpecialistResponse{
			Message: content,
		}, nil
	}

	for _, tr := range toolRequests {
		if _, ok := s.allowedTools[tr.Tool]; !ok {
			return contractx.SpecialistResponse{}, fmt.Errorf("%w: tool=%s is not allowed for agent=%s", contractx.ErrSchemaViolation, tr.Tool, s.agentType)
		}
	}

	return contractx.SpecialistResponse{
		ToolRequests: toolRequests,
	}, nil
}

func toToolRequests(calls []schema.ToolCall) ([]contractx.ToolRequest, error) {
	if len(calls) == 0 {
		return nil, nil
	}
	reqs := make([]contractx.ToolRequest, 0, len(calls))
	for _, call := range calls {
		tool := strings.TrimSpace(call.Function.Name)
		if tool == "" {
			return nil, fmt.Errorf("%w: tool call name is empty", contractx.ErrSchemaViolation)
		}

		args := map[string]any{}
		rawArgs := strings.TrimSpace(call.Function.Arguments)
		if rawArgs != "" {
			if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
				return nil, fmt.Errorf("%w: invalid tool args for tool=%s: %v", contractx.ErrSchemaViolation, tool, err)
			}
		}

		reqs = append(reqs, contractx.ToolRequest{
			Tool: tool,
			Args: args,
		})
	}
	return reqs, nil
}

func specialistTools(agentType contractx.AgentType) []*schema.ToolInfo {
	switch agentType {
	case contractx.AgentTypeSales:
		return []*schema.ToolInfo{
			{
				Name: "inventory.query",
				Desc: "Query product inventory, stock, and price by user constraints.",
				ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
					"query": {Type: schema.String, Desc: "Natural language query", Required: true},
				}),
			},
			{
				Name: "math.evaluate",
				Desc: "Evaluate a mathematical expression.",
				ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
					"expression": {Type: schema.String, Desc: "Expression to evaluate", Required: true},
				}),
			},
		}
	case contractx.AgentTypeSupport:
		return []*schema.ToolInfo{
			{
				Name: "knowledge_base.search",
				Desc: "Search troubleshooting knowledge base and return evidence snippets.",
				ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
					"query": {Type: schema.String, Desc: "Troubleshooting query", Required: true},
				}),
			},
			{
				Name: "math.evaluate",
				Desc: "Evaluate a mathematical expression.",
				ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
					"expression": {Type: schema.String, Desc: "Expression to evaluate", Required: true},
				}),
			},
		}
	default:
		return nil
	}
}

func summarizeGoal(g *statex.Goal) map[string]any {
	if g == nil {
		return map[string]any{}
	}
	return map[string]any{
		"id":            g.ID,
		"type":          g.Type,
		"status":        g.Status,
		"priority":      g.Priority,
		"slots":         g.Slots,
		"missing":       g.Missing,
		"next_question": g.NextQuestion,
	}
}
