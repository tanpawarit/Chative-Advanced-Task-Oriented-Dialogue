package state

import (
	"errors"
	"fmt"
	"time"
)

// SessionState is the persistent source-of-truth for ATOD-style workflow control.
// - Interleaving: ActiveGoalID + GoalStack + GoalStatus (suspended/active/done)
// - Dependency: Goal.Missing + Goal.NextQuestion + GoalStatus (blocked)
type SessionState struct {
	// Identity
	SessionID   string `json:"session_id"`
	WorkspaceID string `json:"workspace_id"`
	CustomerID  string `json:"customer_id"`
	ChannelType string `json:"channel_type"`

	// ATOD core
	ActiveGoalID string           `json:"active_goal_id,omitempty"`
	GoalStack    []string         `json:"goal_stack,omitempty"` // LIFO: suspend/resume
	Goals        map[string]*Goal `json:"goals,omitempty"`      // goal_id -> goal

	UpdatedAt time.Time `json:"updated_at"`
}

type GoalStatus string

const (
	GoalActive    GoalStatus = "active"
	GoalBlocked   GoalStatus = "blocked"
	GoalSuspended GoalStatus = "suspended"
	GoalDone      GoalStatus = "done"
)

type Goal struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`     // "sales.recommend_item" | "support.troubleshoot" | etc.
	Status       GoalStatus     `json:"status"`   // active/blocked/suspended/done
	Priority     int            `json:"priority"` // higher wins
	Slots        map[string]any `json:"slots,omitempty"`
	Missing      []string       `json:"missing,omitempty"`
	NextQuestion string         `json:"next_question,omitempty"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

/* ----------------------------- Goal helpers ----------------------------- */

func (g *Goal) IsBlocked() bool {
	return g != nil && g.Status == GoalBlocked
}

func (g *Goal) IsDone() bool {
	return g != nil && g.Status == GoalDone
}

func (g *Goal) SetSlot(key string, val any) {
	if g.Slots == nil {
		g.Slots = make(map[string]any, 8)
	}
	g.Slots[key] = val
}

func (g *Goal) SetMissing(missing []string, nextQuestion string) {
	g.Missing = missing

	// don't override terminal/paused states
	if g.Status == GoalDone || g.Status == GoalSuspended {
		// still keep NextQuestion coherent if you want
		if len(missing) == 0 {
			g.NextQuestion = ""
		} else {
			g.NextQuestion = nextQuestion
		}
		return
	}

	if len(missing) == 0 {
		if g.Status == GoalBlocked {
			g.Status = GoalActive
		}
		g.NextQuestion = ""
		return
	}

	g.Status = GoalBlocked
	g.NextQuestion = nextQuestion
}

/* -------------------------- SessionState helpers ------------------------- */

var (
	ErrNilGoalID         = errors.New("goal id is empty")
	ErrGoalNotFound      = errors.New("goal not found")
	ErrNoActiveGoal      = errors.New("no active goal")
	ErrStackCorrupt      = errors.New("goal stack corrupt")
	ErrInvalidTransition = errors.New("invalid goal transition")
)

func NewSessionState(sessionID, workspaceID, customerID, channelType string, now time.Time) *SessionState {
	return &SessionState{
		SessionID:   sessionID,
		WorkspaceID: workspaceID,
		CustomerID:  customerID,
		ChannelType: channelType,
		Goals:       make(map[string]*Goal, 4),
		UpdatedAt:   now.UTC(),
	}
}

func (s *SessionState) Touch(now time.Time) {
	s.UpdatedAt = now.UTC()
}

// EnsureGoalsMap makes sure s.Goals is initialized.
func (s *SessionState) EnsureGoalsMap() {
	if s.Goals == nil {
		s.Goals = make(map[string]*Goal, 4)
	}
}

// ActiveGoal returns the currently active goal pointer (or nil).
func (s *SessionState) ActiveGoal() *Goal {
	if s == nil || s.ActiveGoalID == "" || s.Goals == nil {
		return nil
	}
	return s.Goals[s.ActiveGoalID]
}

// GetGoal returns a goal by id.
func (s *SessionState) GetGoal(goalID string) (*Goal, bool) {
	if s == nil || s.Goals == nil {
		return nil, false
	}
	g, ok := s.Goals[goalID]
	return g, ok
}

// AddGoal adds (or replaces) a goal in the map.
func (s *SessionState) AddGoal(g *Goal) error {
	if s == nil {
		return errors.New("nil session state")
	}
	if g == nil || g.ID == "" {
		return ErrNilGoalID
	}
	s.EnsureGoalsMap()
	s.Goals[g.ID] = g
	return nil
}

// PushGoal pushes a goal onto the GoalStack (LIFO). Does not change statuses.
// Use SuspendAndActivate or ResumePrevious for safe workflow transitions.
func (s *SessionState) PushGoal(goalID string) error {
	if s == nil {
		return errors.New("nil session state")
	}
	if goalID == "" {
		return ErrNilGoalID
	}
	s.GoalStack = append(s.GoalStack, goalID)
	return nil
}

// PopGoal pops from GoalStack (LIFO).
func (s *SessionState) PopGoal() (string, bool) {
	if s == nil || len(s.GoalStack) == 0 {
		return "", false
	}
	last := s.GoalStack[len(s.GoalStack)-1]
	s.GoalStack = s.GoalStack[:len(s.GoalStack)-1]
	return last, true
}

// PeekGoal peeks at top of stack.
func (s *SessionState) PeekGoal() (string, bool) {
	if s == nil || len(s.GoalStack) == 0 {
		return "", false
	}
	return s.GoalStack[len(s.GoalStack)-1], true
}

// SetActiveGoal sets ActiveGoalID and ensures stack top is aligned (optional strict mode).
// For POC, we keep it permissive: if stack is empty, it initializes it with the goal.
func (s *SessionState) SetActiveGoal(goalID string) error {
	if s == nil {
		return errors.New("nil session state")
	}
	if goalID == "" {
		return ErrNilGoalID
	}
	if _, ok := s.GetGoal(goalID); !ok {
		return fmt.Errorf("%w: %s", ErrGoalNotFound, goalID)
	}
	s.ActiveGoalID = goalID
	// Ensure stack has at least the active goal
	if len(s.GoalStack) == 0 {
		s.GoalStack = []string{goalID}
		return nil
	}
	// Optional: align top with active goal (do not mutate deeper stack)
	top, _ := s.PeekGoal()
	if top != goalID {
		s.GoalStack = append(s.GoalStack, goalID)
	}
	return nil
}

// SuspendAndActivate performs an interleaving transition:
// - Current active goal -> suspended (if exists and not done)
// - Push new goal id on stack
// - Set ActiveGoalID = new goal, new goal -> active (if not blocked/done)
// NOTE: If new goal is blocked, we keep it blocked; active goal can be blocked too.
func (s *SessionState) SuspendAndActivate(newGoalID string, now time.Time) error {
	if s == nil {
		return errors.New("nil session state")
	}
	if newGoalID == "" {
		return ErrNilGoalID
	}
	newGoal, ok := s.GetGoal(newGoalID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrGoalNotFound, newGoalID)
	}
	if newGoal.Status == GoalDone {
		return fmt.Errorf("%w: cannot activate done goal %s", ErrInvalidTransition, newGoalID)
	}

	// Suspend current active goal (if any)
	if cur := s.ActiveGoal(); cur != nil && cur.ID != newGoalID {
		// Only suspend if not done
		if cur.Status != GoalDone {
			cur.Status = GoalSuspended
			cur.UpdatedAt = now.UTC()
		}
	}

	// Activate new goal (unless blocked/done already)
	if newGoal.Status == "" {
		newGoal.Status = GoalActive
	}
	if newGoal.Status == GoalSuspended {
		// resuming a suspended goal explicitly -> make it active
		newGoal.Status = GoalActive
	}
	newGoal.UpdatedAt = now.UTC()

	// Stack + active
	s.ActiveGoalID = newGoalID
	if len(s.GoalStack) == 0 {
		s.GoalStack = []string{newGoalID}
	} else {
		// Ensure we don't push duplicates back-to-back
		top, _ := s.PeekGoal()
		if top != newGoalID {
			s.GoalStack = append(s.GoalStack, newGoalID)
		}
	}
	s.Touch(now)
	return nil
}

// MarkGoalDone marks a goal done. If it is the active goal, it will also resume previous goal (if any).
func (s *SessionState) MarkGoalDone(goalID string, now time.Time) error {
	if s == nil {
		return errors.New("nil session state")
	}
	if goalID == "" {
		return ErrNilGoalID
	}
	g, ok := s.GetGoal(goalID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrGoalNotFound, goalID)
	}
	g.Status = GoalDone
	g.NextQuestion = ""
	g.Missing = nil
	g.UpdatedAt = now.UTC()

	// If this is active, try to resume previous goal.
	if s.ActiveGoalID == goalID {
		_, _ = s.ResumePrevious(now) // ignore "nothing to resume"
	}
	s.Touch(now)
	return nil
}

// ResumePrevious resumes the previous goal in stack after current active goal is done.
// Behavior:
// - Pop current top (should be current active goal)
// - Set new top as active goal
// - If new top is suspended, set it to active
// Returns resumed goalID.
func (s *SessionState) ResumePrevious(now time.Time) (string, bool) {
	if s == nil || len(s.GoalStack) == 0 {
		return "", false
	}

	// If stack top is active goal, pop it.
	top, _ := s.PeekGoal()
	if s.ActiveGoalID != "" && top == s.ActiveGoalID {
		_, _ = s.PopGoal()
	}

	// Now resume new top.
	prevID, ok := s.PeekGoal()
	if !ok {
		s.ActiveGoalID = ""
		s.Touch(now)
		return "", false
	}

	prevGoal, ok := s.GetGoal(prevID)
	if !ok {
		// Stack corruption: refers to missing goal.
		s.ActiveGoalID = ""
		s.Touch(now)
		return "", false
	}

	// Only change status if it was suspended. If blocked, keep blocked.
	if prevGoal.Status == GoalSuspended {
		prevGoal.Status = GoalActive
	}
	prevGoal.UpdatedAt = now.UTC()

	s.ActiveGoalID = prevID
	s.Touch(now)
	return prevID, true
}

func (s *SessionState) Validate() error {
	if s.Goals == nil {
		return nil
	}
	// if active set, must exist
	if s.ActiveGoalID != "" {
		if _, ok := s.Goals[s.ActiveGoalID]; !ok {
			return fmt.Errorf("%w: active_goal_id=%s", ErrGoalNotFound, s.ActiveGoalID)
		}
	}
	// stack refs must exist
	for _, id := range s.GoalStack {
		if _, ok := s.Goals[id]; !ok {
			return fmt.Errorf("%w: stack has missing goal_id=%s", ErrStackCorrupt, id)
		}
	}
	// blocked goal should have missing + next_question
	for _, g := range s.Goals {
		if g.Status == GoalBlocked && (len(g.Missing) == 0 || g.NextQuestion == "") {
			return fmt.Errorf("blocked goal %s must have missing and next_question", g.ID)
		}
	}
	return nil
}

/* -------------------------- Convenience functions ------------------------ */

// CreateGoal is a convenience constructor that sets timestamps.
func CreateGoal(id, goalType string, priority int, now time.Time) *Goal {
	return &Goal{
		ID:        id,
		Type:      goalType,
		Status:    GoalActive,
		Priority:  priority,
		Slots:     make(map[string]any, 8),
		UpdatedAt: now.UTC(),
	}
}
