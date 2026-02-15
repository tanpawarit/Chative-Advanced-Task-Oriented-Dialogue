package orchestratornode

import (
	"fmt"
	"strings"
	"time"

	contractx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/contract"
	statex "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/state"
)

func ApplyPlan(in *GraphState) (*GraphState, error) {
	if in == nil || in.Session == nil {
		return nil, fmt.Errorf("%w: graph session is nil", contractx.ErrValidation)
	}

	activeGoal, err := applyPlan(in.Session, in.PlanResp, in.Now)
	if err != nil {
		return nil, err
	}
	if activeGoal == nil {
		return nil, ErrNoActiveGoal
	}

	in.ActiveGoal = activeGoal
	return in, nil
}

func applyPlan(
	st *statex.SessionState,
	plan contractx.PlannerResponse,
	now time.Time,
) (*statex.Goal, error) {
	if st == nil {
		return nil, fmt.Errorf("%w: session state is nil", contractx.ErrValidation)
	}

	goalType := strings.TrimSpace(plan.Goal.GoalType)
	if !strings.HasPrefix(goalType, "sales.") && !strings.HasPrefix(goalType, "support.") {
		return nil, fmt.Errorf("%w: unsupported goal type=%q", contractx.ErrValidation, goalType)
	}

	targetGoal, created, err := findOrCreateGoal(st, plan.Goal, now)
	if err != nil {
		return nil, err
	}

	if plan.Goal.Priority > 0 {
		targetGoal.Priority = plan.Goal.Priority
	} else if targetGoal.Priority <= 0 {
		targetGoal.Priority = defaultPriorityForGoalType(goalType)
	}
	targetGoal.Type = goalType

	for k, v := range plan.Goal.SlotsPatch {
		targetGoal.SetSlot(k, v)
	}
	targetGoal.SetMissing(plan.Goal.Missing, plan.Goal.NextQuestion)
	targetGoal.UpdatedAt = now.UTC()

	current := st.ActiveGoal()
	if created {
		if shouldInterleave(current, targetGoal) {
			if err := st.SuspendAndActivate(targetGoal.ID, now); err != nil {
				return nil, err
			}
		}
	} else if current == nil {
		if err := st.SetActiveGoal(targetGoal.ID); err != nil {
			return nil, err
		}
	} else if shouldInterleave(current, targetGoal) {
		if err := st.SuspendAndActivate(targetGoal.ID, now); err != nil {
			return nil, err
		}
	}

	st.Touch(now)
	return st.ActiveGoal(), nil
}

func findOrCreateGoal(
	st *statex.SessionState,
	patch contractx.GoalPatch,
	now time.Time,
) (*statex.Goal, bool, error) {
	if st == nil {
		return nil, false, fmt.Errorf("%w: nil state", contractx.ErrValidation)
	}
	st.EnsureGoalsMap()

	if goalID := strings.TrimSpace(patch.GoalID); goalID != "" {
		if g, ok := st.GetGoal(goalID); ok {
			return g, false, nil
		}
	}

	if active := st.ActiveGoal(); active != nil && active.Type == patch.GoalType && !active.IsDone() {
		return active, false, nil
	}

	goalID := strings.TrimSpace(patch.GoalID)
	if goalID == "" {
		goalID = newGoalID(patch.GoalType, now)
	}
	g := statex.CreateGoal(goalID, patch.GoalType, patch.Priority, now)
	if g.Priority <= 0 {
		g.Priority = defaultPriorityForGoalType(patch.GoalType)
	}
	if err := st.AddGoal(g); err != nil {
		return nil, false, err
	}
	return g, true, nil
}

func newGoalID(goalType string, now time.Time) string {
	safeType := strings.ReplaceAll(strings.TrimSpace(goalType), ".", "_")
	if safeType == "" {
		safeType = "goal"
	}
	return fmt.Sprintf("%s_%d", safeType, now.UnixNano())
}
