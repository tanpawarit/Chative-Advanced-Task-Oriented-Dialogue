package specialist

import (
	"context"
	"errors"
	"testing"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	contractx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/contract"
	statex "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/state"
)

type fakeToolCallingModel struct {
	responses []*schema.Message
	err       error
	idx       int
}

func (f *fakeToolCallingModel) Generate(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.Message, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.idx >= len(f.responses) {
		return nil, errors.New("no fake response left")
	}
	msg := f.responses[f.idx]
	f.idx++
	return msg, nil
}

func (f *fakeToolCallingModel) Stream(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("stream not implemented in fake model")
}

func (f *fakeToolCallingModel) WithTools(tools []*schema.ToolInfo) (einomodel.ToolCallingChatModel, error) {
	return f, nil
}

func TestPlannerPlanSuccess(t *testing.T) {
	t.Parallel()

	fake := &fakeToolCallingModel{
		responses: []*schema.Message{
			{
				Content: `{"goal_id":"g1","goal_type":"sales.recommend_item","priority":50,"slots_patch":{"budget":1500},"missing":["category"],"next_question":"What category do you want?"}`,
			},
		},
	}

	planner, err := newPlanner(context.Background(), fake, "planner prompt")
	if err != nil {
		t.Fatalf("newPlanner() error = %v", err)
	}

	out, err := planner.Plan(context.Background(), contractx.PlannerRequest{
		UserMessage:   "recommend me something",
		MemorySummary: "likes gaming gear",
		Session:       nil,
	})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	if out.Goal.GoalID != "g1" {
		t.Fatalf("unexpected goal id: %s", out.Goal.GoalID)
	}
	if out.Goal.GoalType != "sales.recommend_item" {
		t.Fatalf("unexpected goal type: %s", out.Goal.GoalType)
	}
	if out.Goal.Priority != 50 {
		t.Fatalf("unexpected priority: %d", out.Goal.Priority)
	}
	if len(out.Goal.Missing) != 1 || out.Goal.Missing[0] != "category" {
		t.Fatalf("unexpected missing: %#v", out.Goal.Missing)
	}
}

func TestPlannerPlanSchemaFailure(t *testing.T) {
	t.Parallel()

	fake := &fakeToolCallingModel{
		responses: []*schema.Message{
			{
				Content: `{"goal_type":"sales.recommend_item","priority":0,"slots_patch":{}}`,
			},
		},
	}

	planner, err := newPlanner(context.Background(), fake, "planner prompt")
	if err != nil {
		t.Fatalf("newPlanner() error = %v", err)
	}

	_, err = planner.Plan(context.Background(), contractx.PlannerRequest{
		UserMessage: "recommend me a mouse",
	})
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, contractx.ErrSchemaViolation) {
		t.Fatalf("expected ErrSchemaViolation, got %v", err)
	}
}

func TestSpecialistBlockedReturnsQuestion(t *testing.T) {
	t.Parallel()

	fake := &fakeToolCallingModel{
		responses: []*schema.Message{
			{
				Content: `{"message":"What is your budget?","state_updates":{"set_status":"blocked","missing":["budget"],"next_question":"What is your budget?"}}`,
			},
		},
	}

	spec, err := newSpecialist(context.Background(), contractx.AgentTypeSales, fake, "sales prompt")
	if err != nil {
		t.Fatalf("newSpecialist() error = %v", err)
	}

	resp, err := spec.Run(context.Background(), contractx.SpecialistRequest{
		UserMessage: "recommend gaming mouse",
		ActiveGoal: &statex.Goal{
			ID:           "g1",
			Type:         "sales.recommend_item",
			Status:       statex.GoalBlocked,
			Missing:      []string{"budget"},
			NextQuestion: "What is your budget?",
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if resp.Message == "" {
		t.Fatal("expected non-empty message")
	}
	if len(resp.ToolRequests) != 0 {
		t.Fatalf("expected no tool requests, got %#v", resp.ToolRequests)
	}
}

func TestSpecialistToolCallMapping(t *testing.T) {
	t.Parallel()

	fake := &fakeToolCallingModel{
		responses: []*schema.Message{
			{
				Role: schema.Assistant,
				ToolCalls: []schema.ToolCall{
					{
						ID:   "call_1",
						Type: "function",
						Function: schema.FunctionCall{
							Name:      "inventory.query",
							Arguments: `{"query":"gaming mouse under 1500"}`,
						},
					},
				},
			},
		},
	}

	spec, err := newSpecialist(context.Background(), contractx.AgentTypeSales, fake, "sales prompt")
	if err != nil {
		t.Fatalf("newSpecialist() error = %v", err)
	}

	resp, err := spec.Run(context.Background(), contractx.SpecialistRequest{
		UserMessage: "recommend gaming mouse",
		ActiveGoal: &statex.Goal{
			ID:       "g1",
			Type:     "sales.recommend_item",
			Status:   statex.GoalActive,
			Priority: 50,
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(resp.ToolRequests) != 1 {
		t.Fatalf("expected 1 tool request, got %d", len(resp.ToolRequests))
	}
	if resp.ToolRequests[0].Tool != "inventory.query" {
		t.Fatalf("unexpected tool name: %s", resp.ToolRequests[0].Tool)
	}
	if resp.ToolRequests[0].Args["query"] != "gaming mouse under 1500" {
		t.Fatalf("unexpected args: %#v", resp.ToolRequests[0].Args)
	}
}
