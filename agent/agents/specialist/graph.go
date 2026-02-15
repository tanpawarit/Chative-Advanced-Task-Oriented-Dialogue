package specialist

import (
	"context"
	"fmt"
	"strings"

	einomodel "github.com/cloudwego/eino/components/model"
	einoprompt "github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	contractx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/contract"
)

func compilePlannerGraph(
	ctx context.Context,
	chatModel einomodel.BaseChatModel,
	systemPrompt string,
) (compose.Runnable[map[string]any, plannerLLMOutput], error) {
	runner, err := compileStructuredLLMGraph[plannerLLMOutput](ctx, chatModel, systemPrompt, "planner.model_graph")
	if err != nil {
		return nil, fmt.Errorf("compile planner graph: %w", err)
	}
	return runner, nil
}

func compileSpecialistStructuredGraph(
	ctx context.Context,
	chatModel einomodel.BaseChatModel,
	systemPrompt string,
) (compose.Runnable[map[string]any, specialistLLMOutput], error) {
	runner, err := compileStructuredLLMGraph[specialistLLMOutput](ctx, chatModel, systemPrompt, "specialist.structured_graph")
	if err != nil {
		return nil, fmt.Errorf("compile specialist structured graph: %w", err)
	}
	return runner, nil
}

func compileSpecialistToolPlanningGraph(
	ctx context.Context,
	chatModel einomodel.BaseChatModel,
	systemPrompt string,
) (compose.Runnable[map[string]any, *schema.Message], error) {
	template := einoprompt.FromMessages(
		schema.FString,
		schema.SystemMessage(systemPrompt),
		schema.UserMessage("{input}"),
	)

	graph := compose.NewGraph[map[string]any, *schema.Message]()
	if err := graph.AddChatTemplateNode("prompt", template); err != nil {
		return nil, fmt.Errorf("add tool planning prompt node: %w", err)
	}
	if err := graph.AddChatModelNode("model", chatModel); err != nil {
		return nil, fmt.Errorf("add tool planning model node: %w", err)
	}
	if err := graph.AddEdge(compose.START, "prompt"); err != nil {
		return nil, fmt.Errorf("add tool planning edge start->prompt: %w", err)
	}
	if err := graph.AddEdge("prompt", "model"); err != nil {
		return nil, fmt.Errorf("add tool planning edge prompt->model: %w", err)
	}
	if err := graph.AddEdge("model", compose.END); err != nil {
		return nil, fmt.Errorf("add tool planning edge model->end: %w", err)
	}

	runner, err := graph.Compile(ctx, compose.WithGraphName("specialist.tool_planning_graph"))
	if err != nil {
		return nil, fmt.Errorf("compile specialist tool planning graph: %w", err)
	}
	return runner, nil
}

type specialistGraphState struct {
	Req       contractx.SpecialistRequest
	IsBlocked bool
}

func compileSpecialistRuntimeGraph(
	ctx context.Context,
	toolFlow func(context.Context, contractx.SpecialistRequest) (contractx.SpecialistResponse, error),
	structuredFlow func(context.Context, contractx.SpecialistRequest, bool) (contractx.SpecialistResponse, error),
) (compose.Runnable[contractx.SpecialistRequest, contractx.SpecialistResponse], error) {
	graph := compose.NewGraph[contractx.SpecialistRequest, contractx.SpecialistResponse]()

	if err := graph.AddLambdaNode("validate_and_prepare",
		compose.InvokableLambda(func(ctx context.Context, req contractx.SpecialistRequest) (*specialistGraphState, error) {
			if req.ActiveGoal == nil {
				return nil, fmt.Errorf("%w: active goal is required", contractx.ErrValidation)
			}
			if strings.TrimSpace(req.ActiveGoal.Type) == "" {
				return nil, fmt.Errorf("%w: active goal type is required", contractx.ErrValidation)
			}

			return &specialistGraphState{
				Req:       req,
				IsBlocked: req.ActiveGoal.IsBlocked() || len(req.ActiveGoal.Missing) > 0,
			}, nil
		}),
	); err != nil {
		return nil, fmt.Errorf("add specialist runtime validate node: %w", err)
	}

	if err := graph.AddLambdaNode("tool_path",
		compose.InvokableLambda(func(ctx context.Context, in *specialistGraphState) (contractx.SpecialistResponse, error) {
			if in == nil {
				return contractx.SpecialistResponse{}, fmt.Errorf("%w: specialist graph state is nil", contractx.ErrValidation)
			}
			return toolFlow(ctx, in.Req)
		}),
	); err != nil {
		return nil, fmt.Errorf("add specialist runtime tool node: %w", err)
	}

	if err := graph.AddLambdaNode("structured_path",
		compose.InvokableLambda(func(ctx context.Context, in *specialistGraphState) (contractx.SpecialistResponse, error) {
			if in == nil {
				return contractx.SpecialistResponse{}, fmt.Errorf("%w: specialist graph state is nil", contractx.ErrValidation)
			}
			return structuredFlow(ctx, in.Req, in.IsBlocked)
		}),
	); err != nil {
		return nil, fmt.Errorf("add specialist runtime structured node: %w", err)
	}

	branch := compose.NewGraphBranch(
		func(ctx context.Context, in *specialistGraphState) (string, error) {
			if in == nil {
				return "", fmt.Errorf("%w: specialist graph state is nil", contractx.ErrValidation)
			}
			if !in.IsBlocked && len(in.Req.ToolResults) == 0 {
				return "tool_path", nil
			}
			return "structured_path", nil
		},
		map[string]bool{
			"tool_path":       true,
			"structured_path": true,
		},
	)

	if err := graph.AddBranch("validate_and_prepare", branch); err != nil {
		return nil, fmt.Errorf("add specialist runtime branch: %w", err)
	}
	if err := graph.AddEdge(compose.START, "validate_and_prepare"); err != nil {
		return nil, fmt.Errorf("add specialist runtime edge start->validate: %w", err)
	}
	if err := graph.AddEdge("tool_path", compose.END); err != nil {
		return nil, fmt.Errorf("add specialist runtime edge tool->end: %w", err)
	}
	if err := graph.AddEdge("structured_path", compose.END); err != nil {
		return nil, fmt.Errorf("add specialist runtime edge structured->end: %w", err)
	}

	runner, err := graph.Compile(ctx, compose.WithGraphName("specialist.runtime_graph"))
	if err != nil {
		return nil, fmt.Errorf("compile specialist runtime graph: %w", err)
	}
	return runner, nil
}

func compileStructuredLLMGraph[T any](
	ctx context.Context,
	chatModel einomodel.BaseChatModel,
	systemPrompt string,
	graphName string,
) (compose.Runnable[map[string]any, T], error) {
	template := einoprompt.FromMessages(
		schema.FString,
		schema.SystemMessage(systemPrompt),
		schema.UserMessage("{input}"),
	)

	parser := schema.NewMessageJSONParser[T](&schema.MessageJSONParseConfig{
		ParseFrom: schema.MessageParseFromContent,
	})

	graph := compose.NewGraph[map[string]any, T]()
	if err := graph.AddChatTemplateNode("prompt", template); err != nil {
		return nil, fmt.Errorf("add structured prompt node: %w", err)
	}
	if err := graph.AddChatModelNode("model", chatModel); err != nil {
		return nil, fmt.Errorf("add structured model node: %w", err)
	}
	if err := graph.AddLambdaNode("parse_json", compose.MessageParser(parser)); err != nil {
		return nil, fmt.Errorf("add structured parser node: %w", err)
	}

	if err := graph.AddEdge(compose.START, "prompt"); err != nil {
		return nil, fmt.Errorf("add structured edge start->prompt: %w", err)
	}
	if err := graph.AddEdge("prompt", "model"); err != nil {
		return nil, fmt.Errorf("add structured edge prompt->model: %w", err)
	}
	if err := graph.AddEdge("model", "parse_json"); err != nil {
		return nil, fmt.Errorf("add structured edge model->parse: %w", err)
	}
	if err := graph.AddEdge("parse_json", compose.END); err != nil {
		return nil, fmt.Errorf("add structured edge parse->end: %w", err)
	}

	runner, err := graph.Compile(ctx, compose.WithGraphName(graphName))
	if err != nil {
		return nil, fmt.Errorf("compile structured graph: %w", err)
	}
	return runner, nil
}
