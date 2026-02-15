package orchestratornode

import (
	"fmt"
	"strings"
	"time"

	contractx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/contract"
	statex "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/state"
)

func ApplyStateUpdates(in *GraphState) (*GraphState, error) {
	if in == nil || in.Session == nil || in.ActiveGoal == nil {
		return nil, fmt.Errorf("%w: graph state is incomplete", contractx.ErrValidation)
	}

	if err := applyStateUpdates(in.Session, in.ActiveGoal.ID, in.StateUpdates, in.Now); err != nil {
		return nil, err
	}
	return in, nil
}

func applyStateUpdates(
	st *statex.SessionState,
	goalID string,
	updates contractx.StateUpdates,
	now time.Time,
) error {
	if st == nil {
		return fmt.Errorf("%w: nil state", contractx.ErrValidation)
	}
	if strings.TrimSpace(goalID) == "" {
		return fmt.Errorf("%w: goal id is empty", contractx.ErrValidation)
	}

	goal, ok := st.GetGoal(goalID)
	if !ok {
		return fmt.Errorf("%w: goal id=%s", statex.ErrGoalNotFound, goalID)
	}

	for k, v := range updates.SlotsPatch {
		goal.SetSlot(k, v)
	}

	if len(updates.Missing) > 0 || strings.TrimSpace(updates.NextQuestion) != "" {
		goal.SetMissing(updates.Missing, strings.TrimSpace(updates.NextQuestion))
	}

	setStatus := strings.TrimSpace(updates.SetStatus)
	if setStatus != "" {
		switch statex.GoalStatus(setStatus) {
		case statex.GoalActive:
			goal.Status = statex.GoalActive
		case statex.GoalBlocked:
			if len(goal.Missing) == 0 || strings.TrimSpace(goal.NextQuestion) == "" {
				return fmt.Errorf("%w: blocked status requires missing+next_question", contractx.ErrValidation)
			}
			goal.Status = statex.GoalBlocked
		case statex.GoalSuspended:
			goal.Status = statex.GoalSuspended
		case statex.GoalDone:
			updates.MarkDone = true
		default:
			return fmt.Errorf("%w: invalid set_status=%q", contractx.ErrValidation, setStatus)
		}
	}

	if updates.MarkDone {
		return st.MarkGoalDone(goalID, now)
	}

	goal.UpdatedAt = now.UTC()
	st.Touch(now)
	return nil
}
