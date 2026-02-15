package contract

import (
	"time"

	statex "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/state"
)

type AgentType string

const (
	AgentTypeOrchestrator AgentType = "orchestrator"
	AgentTypeSales        AgentType = "sales"
	AgentTypeSupport      AgentType = "support"
)

type PlannerRequest struct {
	UserMessage   string               `json:"user_message"`
	MemorySummary string               `json:"memory_summary"`
	Session       *statex.SessionState `json:"session"`
	Now           time.Time            `json:"now"`
}

type PlannerResponse struct {
	Goal GoalPatch `json:"goal"`
}

type GoalPatch struct {
	GoalID       string         `json:"goal_id,omitempty"`
	GoalType     string         `json:"goal_type"`
	Priority     int            `json:"priority"`
	SlotsPatch   map[string]any `json:"slots_patch,omitempty"`
	Missing      []string       `json:"missing,omitempty"`
	NextQuestion string         `json:"next_question,omitempty"`
}

type SpecialistRequest struct {
	UserMessage   string       `json:"user_message"`
	MemorySummary string       `json:"memory_summary"`
	ActiveGoal    *statex.Goal `json:"active_goal"`
	ToolResults   []ToolResult `json:"tool_results,omitempty"`
}

type SpecialistResponse struct {
	Message      string        `json:"message"`
	ToolRequests []ToolRequest `json:"tool_requests,omitempty"`
	StateUpdates StateUpdates  `json:"state_updates,omitempty"`
}

type StateUpdates struct {
	SlotsPatch   map[string]any `json:"slots_patch,omitempty"`
	SetStatus    string         `json:"set_status,omitempty"`
	Missing      []string       `json:"missing,omitempty"`
	NextQuestion string         `json:"next_question,omitempty"`
	MemoryUpdate string         `json:"memory_update,omitempty"`
	MarkDone     bool           `json:"mark_done,omitempty"`
}

type ToolRequest struct {
	Tool string         `json:"tool"`
	Args map[string]any `json:"args,omitempty"`
}

type ToolResult struct {
	Tool   string `json:"tool"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}
