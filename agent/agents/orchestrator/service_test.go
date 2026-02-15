package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	contractx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/contract"
	statex "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/state"
)

type fakeStore struct {
	loadState *statex.SessionState
	loadErr   error
	saveErr   error
	saved     []*statex.SessionState
}

func (f *fakeStore) Load(ctx context.Context, sessionID string) (*statex.SessionState, error) {
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	if f.loadState == nil {
		return nil, statex.ErrStateNotFound
	}
	return cloneSessionState(f.loadState), nil
}

func (f *fakeStore) Save(ctx context.Context, st *statex.SessionState) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, cloneSessionState(st))
	return nil
}

func (f *fakeStore) Delete(ctx context.Context, sessionID string) error {
	return nil
}

type memoryWrite struct {
	customerID string
	update     string
}

type fakeMemory struct {
	summary  string
	readErr  error
	writeErr error
	writes   []memoryWrite
}

func (f *fakeMemory) ReadSummary(ctx context.Context, customerID string) (string, error) {
	if f.readErr != nil {
		return "", f.readErr
	}
	return f.summary, nil
}

func (f *fakeMemory) WriteSummary(ctx context.Context, customerID string, update string) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	f.writes = append(f.writes, memoryWrite{customerID: customerID, update: update})
	return nil
}

type fakePlanner struct {
	resp  contractx.PlannerResponse
	err   error
	calls int
}

func (f *fakePlanner) Plan(ctx context.Context, req contractx.PlannerRequest) (contractx.PlannerResponse, error) {
	f.calls++
	if f.err != nil {
		return contractx.PlannerResponse{}, f.err
	}
	return f.resp, nil
}

type fakeSpecialist struct {
	responses []contractx.SpecialistResponse
	err       error
	calls     int
	lastReqs  []contractx.SpecialistRequest
}

func (f *fakeSpecialist) Run(ctx context.Context, req contractx.SpecialistRequest) (contractx.SpecialistResponse, error) {
	f.calls++
	f.lastReqs = append(f.lastReqs, req)
	if f.err != nil {
		return contractx.SpecialistResponse{}, f.err
	}
	idx := f.calls - 1
	if idx >= len(f.responses) {
		return contractx.SpecialistResponse{}, fmt.Errorf("no specialist response left at call=%d", f.calls)
	}
	return f.responses[idx], nil
}

type toolCallRecord struct {
	agentType string
	reqs      []contractx.ToolRequest
}

type fakeTools struct {
	results []contractx.ToolResult
	err     error
	calls   []toolCallRecord
}

func (f *fakeTools) Execute(ctx context.Context, agentType string, reqs []contractx.ToolRequest) ([]contractx.ToolResult, error) {
	f.calls = append(f.calls, toolCallRecord{
		agentType: agentType,
		reqs:      append([]contractx.ToolRequest(nil), reqs...),
	})
	if f.err != nil {
		return nil, f.err
	}
	return append([]contractx.ToolResult(nil), f.results...), nil
}

type fakeRegistry struct {
	planner contractx.Planner
	sales   contractx.Specialist
	support contractx.Specialist
}

func (f *fakeRegistry) Planner() contractx.Planner {
	return f.planner
}

func (f *fakeRegistry) Sales() contractx.Specialist {
	return f.sales
}

func (f *fakeRegistry) Support() contractx.Specialist {
	return f.support
}

func TestHandleMessageInvalidInput(t *testing.T) {
	t.Parallel()

	o := newTestOrchestrator(t,
		&fakeStore{},
		&fakeRegistry{
			planner: &fakePlanner{},
			sales:   &fakeSpecialist{},
			support: &fakeSpecialist{},
		},
		&fakeTools{},
		&fakeMemory{},
	)

	_, err := o.HandleMessage(context.Background(), "   ", "hello")
	if !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("expected ErrInvalidSession, got %v", err)
	}

	_, err = o.HandleMessage(context.Background(), "s1", "    ")
	if !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("expected ErrInvalidMessage, got %v", err)
	}
}

func TestHandleMessageNoToolPath(t *testing.T) {
	t.Parallel()

	store := &fakeStore{loadErr: statex.ErrStateNotFound}
	planner := &fakePlanner{
		resp: contractx.PlannerResponse{
			Goal: contractx.GoalPatch{
				GoalType: "sales.recommend_item",
				Priority: 50,
			},
		},
	}
	sales := &fakeSpecialist{
		responses: []contractx.SpecialistResponse{
			{
				Message: "ลองรุ่น A ก่อนครับ",
				StateUpdates: contractx.StateUpdates{
					SlotsPatch: map[string]any{"budget": 1500},
					SetStatus:  string(statex.GoalActive),
				},
			},
		},
	}
	memory := &fakeMemory{summary: "likes lightweight mouse"}
	tools := &fakeTools{}

	o := newTestOrchestrator(t,
		store,
		&fakeRegistry{
			planner: planner,
			sales:   sales,
			support: &fakeSpecialist{},
		},
		tools,
		memory,
	)

	reply, err := o.HandleMessage(context.Background(), "session-1", "แนะนำเมาส์เกมมิ่งหน่อย")
	if err != nil {
		t.Fatalf("HandleMessage() error = %v", err)
	}
	if reply != "ลองรุ่น A ก่อนครับ" {
		t.Fatalf("unexpected reply: %q", reply)
	}
	if planner.calls != 1 {
		t.Fatalf("expected planner called once, got %d", planner.calls)
	}
	if sales.calls != 1 {
		t.Fatalf("expected sales specialist called once, got %d", sales.calls)
	}
	if len(tools.calls) != 0 {
		t.Fatalf("expected no tool calls, got %d", len(tools.calls))
	}
	if len(store.saved) != 1 {
		t.Fatalf("expected one save, got %d", len(store.saved))
	}
	if len(memory.writes) != 1 {
		t.Fatalf("expected one memory write, got %d", len(memory.writes))
	}
}

func TestHandleMessageToolPassPath(t *testing.T) {
	t.Parallel()

	store := &fakeStore{loadErr: statex.ErrStateNotFound}
	planner := &fakePlanner{
		resp: contractx.PlannerResponse{
			Goal: contractx.GoalPatch{
				GoalType: "sales.recommend_item",
				Priority: 50,
			},
		},
	}
	sales := &fakeSpecialist{
		responses: []contractx.SpecialistResponse{
			{
				ToolRequests: []contractx.ToolRequest{
					{
						Tool: "inventory.query",
						Args: map[string]any{"query": "gaming mouse under 1500"},
					},
				},
			},
			{
				Message: "รุ่น A ราคาเหมาะกับงบและพร้อมส่งครับ",
				StateUpdates: contractx.StateUpdates{
					SetStatus: string(statex.GoalDone),
				},
			},
		},
	}
	tools := &fakeTools{
		results: []contractx.ToolResult{
			{Tool: "inventory.query", Result: "ok"},
		},
	}

	o := newTestOrchestrator(t,
		store,
		&fakeRegistry{
			planner: planner,
			sales:   sales,
			support: &fakeSpecialist{},
		},
		tools,
		&fakeMemory{},
	)

	reply, err := o.HandleMessage(context.Background(), "session-2", "หาเมาส์เกมมิ่งงบ 1500")
	if err != nil {
		t.Fatalf("HandleMessage() error = %v", err)
	}
	if reply != "รุ่น A ราคาเหมาะกับงบและพร้อมส่งครับ" {
		t.Fatalf("unexpected reply: %q", reply)
	}
	if sales.calls != 2 {
		t.Fatalf("expected sales specialist called twice, got %d", sales.calls)
	}
	if len(tools.calls) != 1 {
		t.Fatalf("expected one tool execution, got %d", len(tools.calls))
	}
	if tools.calls[0].agentType != string(contractx.AgentTypeSales) {
		t.Fatalf("unexpected tool agent type: %s", tools.calls[0].agentType)
	}
}

func TestHandleMessageEmptySpecialistMessage(t *testing.T) {
	t.Parallel()

	o := newTestOrchestrator(t,
		&fakeStore{loadErr: statex.ErrStateNotFound},
		&fakeRegistry{
			planner: &fakePlanner{
				resp: contractx.PlannerResponse{
					Goal: contractx.GoalPatch{
						GoalType: "sales.recommend_item",
						Priority: 50,
					},
				},
			},
			sales: &fakeSpecialist{
				responses: []contractx.SpecialistResponse{
					{Message: "   "},
				},
			},
			support: &fakeSpecialist{},
		},
		&fakeTools{},
		&fakeMemory{},
	)

	_, err := o.HandleMessage(context.Background(), "session-3", "hello")
	if !errors.Is(err, contractx.ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
	if !strings.Contains(err.Error(), "specialist returned empty message") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestHandleMessageMarkDoneResumesPreviousGoal(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	st := statex.NewSessionState("session-4", "workspace", "customer", "chat", now)

	prev := statex.CreateGoal("g_prev", "support.troubleshoot", 100, now)
	prev.Status = statex.GoalSuspended

	active := statex.CreateGoal("g_active", "sales.recommend_item", 50, now)
	active.Status = statex.GoalActive

	if err := st.AddGoal(prev); err != nil {
		t.Fatalf("AddGoal(prev) error = %v", err)
	}
	if err := st.AddGoal(active); err != nil {
		t.Fatalf("AddGoal(active) error = %v", err)
	}
	st.ActiveGoalID = active.ID
	st.GoalStack = []string{prev.ID, active.ID}

	store := &fakeStore{loadState: st}
	planner := &fakePlanner{
		resp: contractx.PlannerResponse{
			Goal: contractx.GoalPatch{
				GoalID:   "g_active",
				GoalType: "sales.recommend_item",
				Priority: 50,
			},
		},
	}
	sales := &fakeSpecialist{
		responses: []contractx.SpecialistResponse{
			{
				Message: "จัดการเรียบร้อยแล้ว",
				StateUpdates: contractx.StateUpdates{
					SetStatus: string(statex.GoalDone),
				},
			},
		},
	}

	o := newTestOrchestrator(t,
		store,
		&fakeRegistry{
			planner: planner,
			sales:   sales,
			support: &fakeSpecialist{},
		},
		&fakeTools{},
		&fakeMemory{},
	)

	reply, err := o.HandleMessage(context.Background(), "session-4", "ขอบคุณ")
	if err != nil {
		t.Fatalf("HandleMessage() error = %v", err)
	}
	if reply != "จัดการเรียบร้อยแล้ว" {
		t.Fatalf("unexpected reply: %q", reply)
	}
	if len(store.saved) != 1 {
		t.Fatalf("expected one save, got %d", len(store.saved))
	}

	saved := store.saved[0]
	if saved.Goals["g_active"].Status != statex.GoalDone {
		t.Fatalf("expected g_active done, got %s", saved.Goals["g_active"].Status)
	}
	if saved.ActiveGoalID != "g_prev" {
		t.Fatalf("expected active goal switched to g_prev, got %s", saved.ActiveGoalID)
	}
	if saved.Goals["g_prev"].Status != statex.GoalActive {
		t.Fatalf("expected g_prev active, got %s", saved.Goals["g_prev"].Status)
	}
}

func TestHandleMessageSaveErrorPropagates(t *testing.T) {
	t.Parallel()

	saveErr := errors.New("save failed")
	store := &fakeStore{
		loadErr: statex.ErrStateNotFound,
		saveErr: saveErr,
	}
	memory := &fakeMemory{}

	o := newTestOrchestrator(t,
		store,
		&fakeRegistry{
			planner: &fakePlanner{
				resp: contractx.PlannerResponse{
					Goal: contractx.GoalPatch{
						GoalType: "sales.recommend_item",
						Priority: 50,
					},
				},
			},
			sales: &fakeSpecialist{
				responses: []contractx.SpecialistResponse{
					{Message: "ok"},
				},
			},
			support: &fakeSpecialist{},
		},
		&fakeTools{},
		memory,
	)

	_, err := o.HandleMessage(context.Background(), "session-5", "hello")
	if !errors.Is(err, saveErr) {
		t.Fatalf("expected save error, got %v", err)
	}
	if len(memory.writes) != 0 {
		t.Fatalf("memory write must not be called on save error, got %d", len(memory.writes))
	}
}

func TestHandleMessageWriteMemoryErrorPropagates(t *testing.T) {
	t.Parallel()

	writeErr := errors.New("write memory failed")
	memory := &fakeMemory{
		writeErr: writeErr,
	}
	store := &fakeStore{loadErr: statex.ErrStateNotFound}

	o := newTestOrchestrator(t,
		store,
		&fakeRegistry{
			planner: &fakePlanner{
				resp: contractx.PlannerResponse{
					Goal: contractx.GoalPatch{
						GoalType: "sales.recommend_item",
						Priority: 50,
					},
				},
			},
			sales: &fakeSpecialist{
				responses: []contractx.SpecialistResponse{
					{Message: "ok"},
				},
			},
			support: &fakeSpecialist{},
		},
		&fakeTools{},
		memory,
	)

	_, err := o.HandleMessage(context.Background(), "session-6", "hello")
	if !errors.Is(err, writeErr) {
		t.Fatalf("expected write memory error, got %v", err)
	}
	if len(store.saved) != 1 {
		t.Fatalf("expected state already saved before memory error, got %d", len(store.saved))
	}
}

func newTestOrchestrator(
	t *testing.T,
	store statex.Store,
	registry contractx.Registry,
	tools contractx.ToolGateway,
	memory contractx.MemoryStore,
) *Orchestrator {
	t.Helper()
	o, err := New(store, registry, tools, memory, Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return o
}

func cloneSessionState(in *statex.SessionState) *statex.SessionState {
	if in == nil {
		return nil
	}
	raw, err := json.Marshal(in)
	if err != nil {
		panic(err)
	}
	var out statex.SessionState
	if err := json.Unmarshal(raw, &out); err != nil {
		panic(err)
	}
	out.EnsureGoalsMap()
	return &out
}
