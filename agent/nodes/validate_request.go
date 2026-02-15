package orchestratornode

import (
	"errors"
	"strings"
	"time"

	contractx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/contract"
	statex "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/state"
)

var (
	ErrInvalidMessage = errors.New("message is empty")
	ErrInvalidSession = errors.New("session id is empty")
	ErrNoActiveGoal   = errors.New("active goal is missing")
)

type GraphInput struct {
	SessionID string
	Text      string
}

type GraphOutput struct {
	Reply string
}

type GraphState struct {
	SessionID string
	Text      string
	Now       time.Time

	Session       *statex.SessionState
	MemorySummary string
	PlanResp      contractx.PlannerResponse
	ActiveGoal    *statex.Goal

	Message      string
	StateUpdates contractx.StateUpdates
}

func ValidateRequest(in GraphInput, nowFn func() time.Time) (*GraphState, error) {
	sessionID := strings.TrimSpace(in.SessionID)
	if sessionID == "" {
		return nil, ErrInvalidSession
	}

	text := strings.TrimSpace(in.Text)
	if text == "" {
		return nil, ErrInvalidMessage
	}

	return &GraphState{
		SessionID: sessionID,
		Text:      text,
		Now:       nowFn().UTC(),
	}, nil
}
