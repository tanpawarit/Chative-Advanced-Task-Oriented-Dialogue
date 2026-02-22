package specialist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/compose"
	einoagent "github.com/cloudwego/eino/flow/agent"
	"github.com/cloudwego/eino/schema"
	contractx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/contract"
	statex "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/state"
)

type fakeStructuredRunner struct {
	invoke func(context.Context, map[string]any) (specialistLLMOutput, error)

	mu    sync.Mutex
	calls int
}

func (f *fakeStructuredRunner) Invoke(ctx context.Context, in map[string]any, opts ...compose.Option) (specialistLLMOutput, error) {
	f.mu.Lock()
	f.calls++
	invoke := f.invoke
	f.mu.Unlock()

	if invoke == nil {
		return specialistLLMOutput{}, errors.New("invoke is not configured")
	}
	return invoke(ctx, in)
}

func (f *fakeStructuredRunner) Stream(ctx context.Context, in map[string]any, opts ...compose.Option) (*schema.StreamReader[specialistLLMOutput], error) {
	out, err := f.Invoke(ctx, in, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]specialistLLMOutput{out}), nil
}

func (f *fakeStructuredRunner) Collect(ctx context.Context, in *schema.StreamReader[map[string]any], opts ...compose.Option) (specialistLLMOutput, error) {
	return specialistLLMOutput{}, errors.New("collect is not implemented")
}

func (f *fakeStructuredRunner) Transform(ctx context.Context, in *schema.StreamReader[map[string]any], opts ...compose.Option) (*schema.StreamReader[specialistLLMOutput], error) {
	return nil, errors.New("transform is not implemented")
}

func (f *fakeStructuredRunner) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type fakeReactGenerator struct {
	generate func(context.Context, []*schema.Message) (*schema.Message, error)
	calls    int
}

func (f *fakeReactGenerator) Generate(ctx context.Context, in []*schema.Message, opts ...einoagent.AgentOption) (*schema.Message, error) {
	f.calls++
	if f.generate == nil {
		return nil, errors.New("generate is not configured")
	}
	return f.generate(ctx, in)
}

type toolCallRecord struct {
	tool string
	args map[string]any
}

type fakeToolExecutor struct {
	results map[string]contractx.ToolResult
	err     error

	mu    sync.Mutex
	calls []toolCallRecord
}

func (f *fakeToolExecutor) Execute(ctx context.Context, tool string, args map[string]any) (contractx.ToolResult, error) {
	copied := map[string]any{}
	for k, v := range args {
		copied[k] = v
	}

	f.mu.Lock()
	f.calls = append(f.calls, toolCallRecord{tool: tool, args: copied})
	f.mu.Unlock()

	if f.err != nil {
		return contractx.ToolResult{}, f.err
	}
	if out, ok := f.results[tool]; ok {
		return out, nil
	}
	return contractx.ToolResult{Tool: tool, Error: "tool returned no result"}, nil
}

func (f *fakeToolExecutor) Calls() []toolCallRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]toolCallRecord, len(f.calls))
	copy(out, f.calls)
	return out
}

func TestSpecialistRunActiveNoToolActJSONSkipsStructured(t *testing.T) {
	t.Parallel()

	structured := &fakeStructuredRunner{
		invoke: func(ctx context.Context, in map[string]any) (specialistLLMOutput, error) {
			return specialistLLMOutput{}, errors.New("structured should not be called")
		},
	}
	reactGen := &fakeReactGenerator{
		generate: func(ctx context.Context, in []*schema.Message) (*schema.Message, error) {
			return schema.AssistantMessage(`{"message":"แก้ได้เลยครับ","state_updates":{"set_status":"done"}}`, nil), nil
		},
	}

	spec := &specialistImpl{
		agentType:    contractx.AgentTypeSupport,
		systemPrompt: "support-prompt",
		reactAgent:   reactGen,
	}

	goal := statex.CreateGoal("g1", "support.troubleshoot", 100, time.Now())
	resp, err := spec.Run(context.Background(), contractx.SpecialistRequest{
		UserMessage: "เครื่องรีสตาร์ตเอง",
		ActiveGoal:  goal,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if resp.Message != "แก้ได้เลยครับ" {
		t.Fatalf("unexpected message: %q", resp.Message)
	}
	if !resp.StateUpdates.MarkDone {
		t.Fatalf("expected MarkDone=true when set_status=done")
	}
	if structured.Calls() != 0 {
		t.Fatalf("expected structured not called, got %d", structured.Calls())
	}
}





func TestReactToolAdapterRejectsInvalidArgs(t *testing.T) {
	t.Parallel()

	adapter := &reactToolAdapter{
		info:     &schema.ToolInfo{Name: "inventory.query"},
		executor: (&fakeToolExecutor{}).Execute,
	}

	_, err := adapter.InvokableRun(context.Background(), "{bad json")
	if !errors.Is(err, contractx.ErrSchemaViolation) {
		t.Fatalf("expected ErrSchemaViolation, got %v", err)
	}
	if !strings.Contains(err.Error(), "invalid tool args") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReactToolAdapterRequiresExecutor(t *testing.T) {
	t.Parallel()

	adapter := &reactToolAdapter{info: &schema.ToolInfo{Name: "inventory.query"}}

	_, err := adapter.InvokableRun(context.Background(), `{"query":"test"}`)
	if !errors.Is(err, contractx.ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
	if !strings.Contains(err.Error(), "tool executor is not configured") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func Example_reactToolAdapter_InvokableRun() {
	adapter := &reactToolAdapter{
		info: &schema.ToolInfo{Name: "math.evaluate"},
		executor: (&fakeToolExecutor{
			results: map[string]contractx.ToolResult{
				"math.evaluate": {Tool: "math.evaluate", Result: "42"},
			},
		}).Execute,
	}

	out, err := adapter.InvokableRun(context.Background(), `{"expression":"40+2"}`)
	fmt.Println(err == nil, strings.Contains(out, `"math.evaluate"`))
	// Output: true true
}

func decodePayload(in map[string]any) (map[string]any, error) {
	raw, ok := in["input"].(string)
	if !ok {
		return nil, fmt.Errorf("input payload must be string, got %T", in["input"])
	}
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, fmt.Errorf("invalid payload json: %w", err)
	}
	return payload, nil
}

func mustDecodePayload(t *testing.T, in map[string]any) map[string]any {
	t.Helper()
	payload, err := decodePayload(in)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	return payload
}
