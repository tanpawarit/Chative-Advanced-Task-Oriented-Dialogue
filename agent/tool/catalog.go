package tool

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/schema"
	contractx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/contract"
)

type Executor func(ctx context.Context, tool string, args map[string]any) (contractx.ToolResult, error)

func BuildForAgent(agentType contractx.AgentType) ([]*schema.ToolInfo, Executor) {
	return infosForAgent(agentType), NewExecutor(agentType)
}

func NewExecutor(agentType contractx.AgentType) Executor {
	fallback := DefaultExecutor(agentType)
	return func(ctx context.Context, tool string, args map[string]any) (contractx.ToolResult, error) {
		switch tool {
		case ToolMathEvaluate:
			return executeMathTool(tool, args)
		default:
			return fallback(ctx, tool, args)
		}
	}
}

func DefaultExecutor(agentType contractx.AgentType) Executor {
	return func(ctx context.Context, tool string, _ map[string]any) (contractx.ToolResult, error) {
		return contractx.ToolResult{
			Tool:  tool,
			Error: fmt.Sprintf("tool=%s is unavailable for agent=%s", tool, agentType),
		}, nil
	}
}

func infosForAgent(agentType contractx.AgentType) []*schema.ToolInfo {
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
