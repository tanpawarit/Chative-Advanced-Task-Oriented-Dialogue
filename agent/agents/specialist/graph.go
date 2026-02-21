package specialist

import (
	"context"
	"fmt"

	einomodel "github.com/cloudwego/eino/components/model"
	einoprompt "github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// compilePlannerGraph builds the graph for the Planner agent.
// This graph takes user input and uses a structured LLM to output a goal plan (JSON).
// It acts as the "Decision Maker" for the Orchestrator, deciding the next step based on conversation context.
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

// compileSpecialistStructuredGraph builds the graph for the Specialist's final response generation.
// It is responsible for crafting the final reply to the user, including slot updates and state changes,
// in a structured JSON format. This node is executed after any necessary tool calls are completed
// or immediately if no tools are needed.
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

// compileStructuredLLMGraph is a helper to build a basic graph that:
// 1. Constructs a prompt from System Prompt + Input.
// 2. Invokes the Chat Model.
// 3. Parses the output JSON into a Go struct (T).
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
