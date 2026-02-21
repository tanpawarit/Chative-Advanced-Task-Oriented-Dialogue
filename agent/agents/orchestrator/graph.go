package orchestrator

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/compose"
	nodex "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/nodes"
)

func (o *Orchestrator) compileHandleMessageGraph(
	ctx context.Context,
) (compose.Runnable[nodex.GraphInput, nodex.GraphOutput], error) {
	graph := compose.NewGraph[nodex.GraphInput, nodex.GraphOutput]()

	if err := graph.AddLambdaNode("validate_request",
		compose.InvokableLambda(func(ctx context.Context, in nodex.GraphInput) (*nodex.GraphState, error) {
			return nodex.ValidateRequest(in, o.now)
		}),
	); err != nil {
		return nil, fmt.Errorf("add node validate_request: %w", err)
	}

	if err := graph.AddLambdaNode("load_or_create_state",
		compose.InvokableLambda(func(ctx context.Context, in *nodex.GraphState) (*nodex.GraphState, error) {
			return nodex.LoadOrCreateState(ctx, in, o.store, o.workspaceID, o.customerID, o.channelType)
		}),
	); err != nil {
		return nil, fmt.Errorf("add node load_or_create_state: %w", err)
	}

	if err := graph.AddLambdaNode("read_memory",
		compose.InvokableLambda(func(ctx context.Context, in *nodex.GraphState) (*nodex.GraphState, error) {
			return nodex.ReadMemory(ctx, in, o.memory)
		}),
	); err != nil {
		return nil, fmt.Errorf("add node read_memory: %w", err)
	}

	if err := graph.AddLambdaNode("plan_goal",
		compose.InvokableLambda(func(ctx context.Context, in *nodex.GraphState) (*nodex.GraphState, error) {
			return nodex.PlanGoal(ctx, in, o.models.Planner())
		}),
	); err != nil {
		return nil, fmt.Errorf("add node plan_goal: %w", err)
	}

	if err := graph.AddLambdaNode("apply_plan",
		compose.InvokableLambda(func(ctx context.Context, in *nodex.GraphState) (*nodex.GraphState, error) {
			return nodex.ApplyPlan(in)
		}),
	); err != nil {
		return nil, fmt.Errorf("add node apply_plan: %w", err)
	}

	if err := graph.AddLambdaNode("dispatch_specialist",
		compose.InvokableLambda(func(ctx context.Context, in *nodex.GraphState) (*nodex.GraphState, error) {
			return nodex.DispatchSpecialist(ctx, in, o.models)
		}),
	); err != nil {
		return nil, fmt.Errorf("add node dispatch_specialist: %w", err)
	}

	if err := graph.AddLambdaNode("apply_state_updates",
		compose.InvokableLambda(func(ctx context.Context, in *nodex.GraphState) (*nodex.GraphState, error) {
			return nodex.ApplyStateUpdates(in)
		}),
	); err != nil {
		return nil, fmt.Errorf("add node apply_state_updates: %w", err)
	}

	if err := graph.AddLambdaNode("validate_and_save_state",
		compose.InvokableLambda(func(ctx context.Context, in *nodex.GraphState) (*nodex.GraphState, error) {
			return nodex.ValidateAndSaveState(ctx, in, o.store)
		}),
	); err != nil {
		return nil, fmt.Errorf("add node validate_and_save_state: %w", err)
	}

	if err := graph.AddLambdaNode("write_memory",
		compose.InvokableLambda(func(ctx context.Context, in *nodex.GraphState) (*nodex.GraphState, error) {
			return nodex.WriteMemory(ctx, in, o.memory)
		}),
	); err != nil {
		return nil, fmt.Errorf("add node write_memory: %w", err)
	}

	if err := graph.AddLambdaNode("finalize_reply",
		compose.InvokableLambda(func(ctx context.Context, in *nodex.GraphState) (nodex.GraphOutput, error) {
			return nodex.FinalizeReply(in)
		}),
	); err != nil {
		return nil, fmt.Errorf("add node finalize_reply: %w", err)
	}

	edges := [][2]string{
		{compose.START, "validate_request"},
		{"validate_request", "load_or_create_state"},
		{"load_or_create_state", "read_memory"},
		{"read_memory", "plan_goal"},
		{"plan_goal", "apply_plan"},
		{"apply_plan", "dispatch_specialist"},
		{"dispatch_specialist", "apply_state_updates"},
		{"apply_state_updates", "validate_and_save_state"},
		{"validate_and_save_state", "write_memory"},
		{"write_memory", "finalize_reply"},
		{"finalize_reply", compose.END},
	}

	for _, edge := range edges {
		if err := graph.AddEdge(edge[0], edge[1]); err != nil {
			return nil, fmt.Errorf("add edge %s->%s: %w", edge[0], edge[1], err)
		}
	}

	runner, err := graph.Compile(ctx, compose.WithGraphName("orchestrator.handle_message"))
	if err != nil {
		return nil, fmt.Errorf("compile orchestrator graph: %w", err)
	}
	return runner, nil
}
