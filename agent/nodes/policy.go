package orchestratornode

import (
	"strings"

	statex "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/state"
)

func shouldInterleave(current *statex.Goal, candidate *statex.Goal) bool {
	if candidate == nil {
		return false
	}
	if current == nil {
		return true
	}
	if current.ID == candidate.ID {
		return false
	}
	return candidate.Priority > current.Priority
}

func defaultPriorityForGoalType(goalType string) int {
	switch {
	case strings.HasPrefix(goalType, "support."):
		return 100
	case strings.HasPrefix(goalType, "sales."):
		return 50
	default:
		return 10
	}
}
