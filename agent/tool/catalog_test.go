package tool

import (
	"context"
	"testing"

	contractx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/contract"
)

func TestBuildForAgentSales(t *testing.T) {
	t.Parallel()

	infos, executor := BuildForAgent(contractx.AgentTypeSales)
	if len(infos) != 2 {
		t.Fatalf("expected 2 tool infos, got %d", len(infos))
	}
	if infos[0].Name != "inventory.query" {
		t.Fatalf("unexpected first tool: %s", infos[0].Name)
	}
	if infos[1].Name != "math.evaluate" {
		t.Fatalf("unexpected second tool: %s", infos[1].Name)
	}
	if executor == nil {
		t.Fatal("executor must not be nil")
	}
}

func TestDefaultExecutorUnavailableMessage(t *testing.T) {
	t.Parallel()

	executor := DefaultExecutor(contractx.AgentTypeSupport)
	out, err := executor(context.Background(), "knowledge_base.search", map[string]any{"query": "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Tool != "knowledge_base.search" {
		t.Fatalf("unexpected tool: %s", out.Tool)
	}
	if out.Error == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestNewExecutorMathEvaluate(t *testing.T) {
	t.Parallel()

	executor := NewExecutor(contractx.AgentTypeSales)
	out, err := executor(context.Background(), ToolMathEvaluate, map[string]any{
		"expression": "2 + 3 * (4 - 1)",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Error != "" {
		t.Fatalf("unexpected tool error: %s", out.Error)
	}
	result, ok := out.Result.(MathEvaluateOutput)
	if !ok {
		t.Fatalf("unexpected result type: %T", out.Result)
	}
	if result.Result != 11 {
		t.Fatalf("unexpected result: %v", result.Result)
	}
}

func TestNewExecutorMathEvaluateInvalidExpression(t *testing.T) {
	t.Parallel()

	executor := NewExecutor(contractx.AgentTypeSales)
	out, err := executor(context.Background(), ToolMathEvaluate, map[string]any{
		"expression": "2 + abc",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Error == "" {
		t.Fatal("expected validation error")
	}
}
