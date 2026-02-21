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

type reactTraceFactory func() (einoagent.AgentOption, func() []contractx.ToolResult)

func newMessageFutureTrace() (einoagent.AgentOption, func() []contractx.ToolResult) {
	opt, future := react.WithMessageFuture()
	return opt, func() []contractx.ToolResult {
		return extractToolResultsFromFuture(future)
	}
}

type specialistImpl struct {
	agentType         contractx.AgentType
	systemPrompt      string
	structuredRunner  compose.Runnable[map[string]any, specialistLLMOutput]
	reactAgent        reactGenerator
	reactTraceFactory reactTraceFactory
}

type specialistLLMOutput struct {
	Message      string                 `json:"message"`
	StateUpdates contractx.StateUpdates `json:"state_updates,omitempty"`
}

type specialistMode string

const (
	specialistModeAsk      specialistMode = "ask"
	specialistModeAct      specialistMode = "act"
	specialistModeFinalize specialistMode = "finalize"
)

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
	Mode          specialistMode         `json:"mode"`
	UserMessage   string                 `json:"user_message"`
	MemorySummary string                 `json:"memory_summary"`
	ActiveGoal    specialistGoalSummary  `json:"active_goal"`
	ToolResults   []contractx.ToolResult `json:"tool_results,omitempty"`
	ActMessage    string                 `json:"act_message,omitempty"`
}

type reactPhaseResult struct {
	ActMessage  string
	ToolResults []contractx.ToolResult
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
		agentType:         agentType,
		systemPrompt:      systemPrompt,
		structuredRunner:  structuredRunner,
		reactAgent:        reactAgent,
		reactTraceFactory: newMessageFutureTrace,
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

	isBlocked := req.ActiveGoal.IsBlocked() || len(req.ActiveGoal.Missing) > 0
	if isBlocked {
		return s.runStructured(ctx, req, true, "")
	}

	if len(req.ToolResults) > 0 {
		return s.runStructured(ctx, req, false, "")
	}

	reactOut, err := s.runReAct(ctx, req)
	if err != nil {
		return contractx.SpecialistResponse{}, err
	}

	req.ToolResults = reactOut.ToolResults
	if resp, ok := tryParseSpecialistJSONResponse(reactOut.ActMessage); ok {
		return resp, nil
	}
	return s.runStructured(ctx, req, false, reactOut.ActMessage)
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
		ToolRequests: nil,
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

func (s *specialistImpl) runStructured(
	ctx context.Context,
	req contractx.SpecialistRequest,
	isBlocked bool,
	actMessage string,
) (contractx.SpecialistResponse, error) {
	mode := specialistModeFinalize
	if isBlocked {
		mode = specialistModeAsk
	}

	payload := specialistPayload{
		Mode:          mode,
		UserMessage:   req.UserMessage,
		MemorySummary: req.MemorySummary,
		ActiveGoal:    summarizeGoal(req.ActiveGoal),
		ToolResults:   req.ToolResults,
	}
	if mode == specialistModeFinalize && len(req.ToolResults) == 0 {
		if trimmed := strings.TrimSpace(actMessage); trimmed != "" {
			payload.ActMessage = trimmed
		}
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

func (s *specialistImpl) runReAct(
	ctx context.Context,
	req contractx.SpecialistRequest,
) (reactPhaseResult, error) {
	payload := specialistPayload{
		Mode:          specialistModeAct,
		UserMessage:   req.UserMessage,
		MemorySummary: req.MemorySummary,
		ActiveGoal:    summarizeGoal(req.ActiveGoal),
	}
	input, err := json.Marshal(payload)
	if err != nil {
		return reactPhaseResult{}, fmt.Errorf("%w: marshal tool planning payload: %v", contractx.ErrValidation, err)
	}

	var collectToolResults func() []contractx.ToolResult
	options := make([]einoagent.AgentOption, 0, 1)
	if s.reactTraceFactory != nil {
		opt, collector := s.reactTraceFactory()
		options = append(options, opt)
		collectToolResults = collector
	}

	msg, err := s.reactAgent.Generate(ctx, []*schema.Message{
		schema.SystemMessage(s.systemPrompt),
		schema.UserMessage(string(input)),
	}, options...)
	if err != nil {
		return reactPhaseResult{}, fmt.Errorf("%w: specialist react invoke: %v", contractx.ErrModelInvoke, err)
	}

	content := ""
	if msg != nil {
		content = strings.TrimSpace(msg.Content)
	}

	toolResults := []contractx.ToolResult(nil)
	if collectToolResults != nil {
		toolResults = collectToolResults()
	}

	return reactPhaseResult{
		ActMessage:  content,
		ToolResults: toolResults,
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

func extractToolResultsFromFuture(future react.MessageFuture) []contractx.ToolResult {
	if future == nil {
		return nil
	}
	iter := future.GetMessages()
	if iter == nil {
		return nil
	}

	messages := make([]*schema.Message, 0, 8)
	for {
		msg, ok, err := iter.Next()
		if err != nil || !ok {
			break
		}
		if msg != nil {
			messages = append(messages, msg)
		}
	}
	return extractToolResultsFromMessages(messages)
}

func extractToolResultsFromMessages(messages []*schema.Message) []contractx.ToolResult {
	results := make([]contractx.ToolResult, 0, len(messages))
	for _, msg := range messages {
		result, ok := parseToolResultMessage(msg)
		if !ok {
			continue
		}
		results = append(results, result)
	}
	return results
}

func parseToolResultMessage(msg *schema.Message) (contractx.ToolResult, bool) {
	if msg == nil || msg.Role != schema.Tool {
		return contractx.ToolResult{}, false
	}
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return contractx.ToolResult{}, false
	}

	var result contractx.ToolResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return contractx.ToolResult{}, false
	}
	if strings.TrimSpace(result.Tool) == "" {
		result.Tool = strings.TrimSpace(msg.ToolName)
	}
	if strings.TrimSpace(result.Tool) == "" {
		return contractx.ToolResult{}, false
	}
	return result, true
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
