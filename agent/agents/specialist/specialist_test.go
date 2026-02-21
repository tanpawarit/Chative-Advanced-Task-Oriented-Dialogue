package specialist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
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

func TestSpecialistRunBlockedUsesStructuredAsk(t *testing.T) {
	t.Parallel()

	structured := &fakeStructuredRunner{
		invoke: func(ctx context.Context, in map[string]any) (specialistLLMOutput, error) {
			payload := mustDecodePayload(t, in)
			if payload["mode"] != "ask" {
				t.Fatalf("expected mode=ask, got %v", payload["mode"])
			}
			return specialistLLMOutput{
				Message: "budget เท่าไหร่ครับ",
				StateUpdates: contractx.StateUpdates{
					SetStatus:    string(statex.GoalBlocked),
					Missing:      []string{"budget"},
					NextQuestion: "budget เท่าไหร่ครับ",
				},
			}, nil
		},
	}
	reactGen := &fakeReactGenerator{
		generate: func(ctx context.Context, in []*schema.Message) (*schema.Message, error) {
			return nil, errors.New("react should not be called in blocked mode")
		},
	}

	spec := &specialistImpl{
		agentType:        contractx.AgentTypeSales,
		systemPrompt:     "sales-prompt",
		structuredRunner: structured,
		reactAgent:       reactGen,
	}

	goal := statex.CreateGoal("g1", "sales.recommend_item", 50, time.Now())
	goal.SetMissing([]string{"budget"}, "budget เท่าไหร่ครับ")

	resp, err := spec.Run(context.Background(), contractx.SpecialistRequest{
		UserMessage:   "ช่วยแนะนำเมาส์",
		MemorySummary: "",
		ActiveGoal:    goal,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if resp.Message != "budget เท่าไหร่ครับ" {
		t.Fatalf("unexpected message: %q", resp.Message)
	}
	if reactGen.calls != 0 {
		t.Fatalf("expected react not called, got %d", reactGen.calls)
	}
	if structured.Calls() != 1 {
		t.Fatalf("expected structured called once, got %d", structured.Calls())
	}
}

func TestSpecialistRunActiveToolThenFinalize(t *testing.T) {
	t.Parallel()

	tools := &fakeToolExecutor{
		results: map[string]contractx.ToolResult{
			"inventory.query": {Tool: "inventory.query", Result: "ok"},
		},
	}

	structured := &fakeStructuredRunner{
		invoke: func(ctx context.Context, in map[string]any) (specialistLLMOutput, error) {
			payload := mustDecodePayload(t, in)
			if payload["mode"] != "finalize" {
				t.Fatalf("expected mode=finalize, got %v", payload["mode"])
			}

			rawToolResults, ok := payload["tool_results"].([]any)
			if !ok || len(rawToolResults) != 1 {
				t.Fatalf("expected one tool result in payload, got %#v", payload["tool_results"])
			}

			return specialistLLMOutput{
				Message: "รุ่น A พร้อมส่งครับ",
				StateUpdates: contractx.StateUpdates{
					SetStatus: string(statex.GoalDone),
				},
			}, nil
		},
	}

	var toolResultJSON string
	reactGen := &fakeReactGenerator{
		generate: func(ctx context.Context, in []*schema.Message) (*schema.Message, error) {
			adapter := &reactToolAdapter{
				info:     &schema.ToolInfo{Name: "inventory.query"},
				executor: tools.Execute,
			}
			out, err := adapter.InvokableRun(ctx, `{"query":"gaming mouse under 1500"}`)
			if err != nil {
				return nil, err
			}
			toolResultJSON = out
			return schema.AssistantMessage("tool call completed", nil), nil
		},
	}

	spec := &specialistImpl{
		agentType:        contractx.AgentTypeSales,
		systemPrompt:     "sales-prompt",
		structuredRunner: structured,
		reactAgent:       reactGen,
		reactTraceFactory: func() (einoagent.AgentOption, func() []contractx.ToolResult) {
			return einoagent.AgentOption{}, func() []contractx.ToolResult {
				return extractToolResultsFromMessages([]*schema.Message{
					schema.ToolMessage(toolResultJSON, "call-1", schema.WithToolName("inventory.query")),
				})
			}
		},
	}

	goal := statex.CreateGoal("g1", "sales.recommend_item", 50, time.Now())
	resp, err := spec.Run(context.Background(), contractx.SpecialistRequest{
		UserMessage:   "หาเมาส์งบ 1500",
		MemorySummary: "",
		ActiveGoal:    goal,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if resp.Message != "รุ่น A พร้อมส่งครับ" {
		t.Fatalf("unexpected message: %q", resp.Message)
	}
	if !resp.StateUpdates.MarkDone {
		t.Fatalf("expected MarkDone=true when set_status=done")
	}
	calls := tools.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(calls))
	}
	if calls[0].tool != "inventory.query" {
		t.Fatalf("unexpected tool name: %s", calls[0].tool)
	}
	if calls[0].args["query"] != "gaming mouse under 1500" {
		t.Fatalf("unexpected tool args: %#v", calls[0].args)
	}
}

func TestSpecialistRunActiveNoToolReturnsActMessage(t *testing.T) {
	t.Parallel()

	structured := &fakeStructuredRunner{
		invoke: func(ctx context.Context, in map[string]any) (specialistLLMOutput, error) {
			payload := mustDecodePayload(t, in)
			if payload["mode"] != "finalize" {
				t.Fatalf("expected mode=finalize, got %v", payload["mode"])
			}
			if payload["act_message"] != "ตอบได้เลยโดยไม่ต้องใช้ tool" {
				t.Fatalf("unexpected act_message: %v", payload["act_message"])
			}
			if raw, exists := payload["tool_results"]; exists {
				rawToolResults, ok := raw.([]any)
				if !ok || len(rawToolResults) != 0 {
					t.Fatalf("expected empty tool_results, got %#v", payload["tool_results"])
				}
			}
			return specialistLLMOutput{
				Message: "ตอบได้เลยโดยไม่ต้องใช้ tool",
				StateUpdates: contractx.StateUpdates{
					SetStatus: string(statex.GoalDone),
				},
			}, nil
		},
	}
	reactGen := &fakeReactGenerator{
		generate: func(ctx context.Context, in []*schema.Message) (*schema.Message, error) {
			return schema.AssistantMessage("ตอบได้เลยโดยไม่ต้องใช้ tool", nil), nil
		},
	}

	spec := &specialistImpl{
		agentType:        contractx.AgentTypeSupport,
		systemPrompt:     "support-prompt",
		structuredRunner: structured,
		reactAgent:       reactGen,
	}

	goal := statex.CreateGoal("g1", "support.troubleshoot", 100, time.Now())
	resp, err := spec.Run(context.Background(), contractx.SpecialistRequest{
		UserMessage: "เครื่องรีสตาร์ตเอง",
		ActiveGoal:  goal,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if resp.Message != "ตอบได้เลยโดยไม่ต้องใช้ tool" {
		t.Fatalf("unexpected message: %q", resp.Message)
	}
	if !resp.StateUpdates.MarkDone {
		t.Fatalf("expected MarkDone=true when set_status=done")
	}
	if structured.Calls() != 1 {
		t.Fatalf("expected structured called once, got %d", structured.Calls())
	}
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
		agentType:        contractx.AgentTypeSupport,
		systemPrompt:     "support-prompt",
		structuredRunner: structured,
		reactAgent:       reactGen,
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

func TestSpecialistRunActiveNoToolEmptyActMessageFallsBackToStructured(t *testing.T) {
	t.Parallel()

	structured := &fakeStructuredRunner{
		invoke: func(ctx context.Context, in map[string]any) (specialistLLMOutput, error) {
			payload := mustDecodePayload(t, in)
			if payload["mode"] != "finalize" {
				t.Fatalf("expected mode=finalize, got %v", payload["mode"])
			}
			return specialistLLMOutput{Message: "ขอข้อมูลเพิ่มอีกนิดครับ"}, nil
		},
	}
	reactGen := &fakeReactGenerator{
		generate: func(ctx context.Context, in []*schema.Message) (*schema.Message, error) {
			return schema.AssistantMessage("   ", nil), nil
		},
	}

	spec := &specialistImpl{
		agentType:        contractx.AgentTypeSales,
		systemPrompt:     "sales-prompt",
		structuredRunner: structured,
		reactAgent:       reactGen,
	}

	goal := statex.CreateGoal("g1", "sales.recommend_item", 50, time.Now())
	resp, err := spec.Run(context.Background(), contractx.SpecialistRequest{
		UserMessage: "ขอคำแนะนำเมาส์",
		ActiveGoal:  goal,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if resp.Message != "ขอข้อมูลเพิ่มอีกนิดครับ" {
		t.Fatalf("unexpected message: %q", resp.Message)
	}
	if structured.Calls() != 1 {
		t.Fatalf("expected structured called once, got %d", structured.Calls())
	}
}

func TestSpecialistRunStructuredMissingRequiresNextQuestion(t *testing.T) {
	t.Parallel()

	structured := &fakeStructuredRunner{
		invoke: func(ctx context.Context, in map[string]any) (specialistLLMOutput, error) {
			return specialistLLMOutput{
				Message: "ขอรายละเอียดเพิ่ม",
				StateUpdates: contractx.StateUpdates{
					Missing: []string{"budget"},
				},
			}, nil
		},
	}

	spec := &specialistImpl{
		agentType:        contractx.AgentTypeSales,
		systemPrompt:     "sales-prompt",
		structuredRunner: structured,
		reactAgent:       &fakeReactGenerator{},
	}

	goal := statex.CreateGoal("g1", "sales.recommend_item", 50, time.Now())
	goal.SetMissing([]string{"budget"}, "budget เท่าไหร่ครับ")

	_, err := spec.Run(context.Background(), contractx.SpecialistRequest{
		UserMessage: "ช่วยแนะนำของ",
		ActiveGoal:  goal,
	})
	if !errors.Is(err, contractx.ErrSchemaViolation) {
		t.Fatalf("expected ErrSchemaViolation, got %v", err)
	}
	if !strings.Contains(err.Error(), "next_question required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSpecialistRunConcurrentToolResultsIsolation(t *testing.T) {
	t.Parallel()

	const runs = 20
	goal := statex.CreateGoal("g1", "sales.recommend_item", 50, time.Now())

	var seenMu sync.Mutex
	seen := make(map[string]int, runs)

	structured := &fakeStructuredRunner{
		invoke: func(ctx context.Context, in map[string]any) (specialistLLMOutput, error) {
			payload, err := decodePayload(in)
			if err != nil {
				return specialistLLMOutput{}, err
			}

			rawToolResults, ok := payload["tool_results"].([]any)
			if !ok {
				return specialistLLMOutput{}, fmt.Errorf("tool_results has type %T", payload["tool_results"])
			}
			if len(rawToolResults) != 1 {
				return specialistLLMOutput{}, fmt.Errorf("expected one tool result, got %d", len(rawToolResults))
			}

			m, ok := rawToolResults[0].(map[string]any)
			if !ok {
				return specialistLLMOutput{}, fmt.Errorf("unexpected tool result type %T", rawToolResults[0])
			}
			val := fmt.Sprint(m["result"])
			if !strings.HasPrefix(val, "run-") {
				return specialistLLMOutput{}, fmt.Errorf("unexpected tool result value %q", val)
			}

			seenMu.Lock()
			seen[val]++
			seenMu.Unlock()

			return specialistLLMOutput{Message: "ok"}, nil
		},
	}

	reactGen := &fakeReactGenerator{
		generate: func(ctx context.Context, in []*schema.Message) (*schema.Message, error) {
			return schema.AssistantMessage("tool path", nil), nil
		},
	}

	var seq atomic.Int64
	spec := &specialistImpl{
		agentType:        contractx.AgentTypeSales,
		systemPrompt:     "sales-prompt",
		structuredRunner: structured,
		reactAgent:       reactGen,
		reactTraceFactory: func() (einoagent.AgentOption, func() []contractx.ToolResult) {
			id := seq.Add(1)
			return einoagent.AgentOption{}, func() []contractx.ToolResult {
				return []contractx.ToolResult{{
					Tool:   "inventory.query",
					Result: fmt.Sprintf("run-%d", id),
				}}
			}
		},
	}

	var wg sync.WaitGroup
	errCh := make(chan error, runs)
	for i := 0; i < runs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := spec.Run(context.Background(), contractx.SpecialistRequest{
				UserMessage: "หาเมาส์",
				ActiveGoal:  goal,
			})
			if err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("Run() error = %v", err)
	}

	seenMu.Lock()
	defer seenMu.Unlock()
	if len(seen) != runs {
		t.Fatalf("expected %d unique tool results, got %d", runs, len(seen))
	}
	for value, count := range seen {
		if count != 1 {
			t.Fatalf("tool result %q appeared %d times", value, count)
		}
	}
}

func TestExtractToolResultsFromMessagesParsesInOrder(t *testing.T) {
	t.Parallel()

	messages := []*schema.Message{
		schema.AssistantMessage("thinking", nil),
		schema.ToolMessage(`{"tool":"inventory.query","result":"A"}`, "call-1", schema.WithToolName("inventory.query")),
		schema.ToolMessage(`{"result":"42"}`, "call-2", schema.WithToolName("math.evaluate")),
	}

	results := extractToolResultsFromMessages(messages)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Tool != "inventory.query" {
		t.Fatalf("unexpected first tool: %s", results[0].Tool)
	}
	if results[1].Tool != "math.evaluate" {
		t.Fatalf("unexpected second tool: %s", results[1].Tool)
	}
}

func TestExtractToolResultsFromMessagesSkipsInvalidPayload(t *testing.T) {
	t.Parallel()

	messages := []*schema.Message{
		schema.ToolMessage("{bad json", "call-1", schema.WithToolName("inventory.query")),
		schema.ToolMessage(`{"result":"missing_tool_name"}`, "call-2"),
		schema.ToolMessage(`{"tool":"inventory.query","result":"ok"}`, "call-3", schema.WithToolName("inventory.query")),
	}

	results := extractToolResultsFromMessages(messages)
	if len(results) != 1 {
		t.Fatalf("expected 1 valid result, got %d", len(results))
	}
	if results[0].Tool != "inventory.query" {
		t.Fatalf("unexpected tool: %s", results[0].Tool)
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
