package orchestratornode

import (
	"context"
	"fmt"

	contractx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/contract"
)

func ReadMemory(
	ctx context.Context,
	in *GraphState,
	memory contractx.MemoryStore,
) (*GraphState, error) {
	if in == nil || in.Session == nil {
		return nil, fmt.Errorf("%w: graph session is nil", contractx.ErrValidation)
	}

	summary, err := memory.ReadSummary(ctx, in.Session.CustomerID)
	if err != nil {
		return nil, err
	}
	in.MemorySummary = summary
	return in, nil
}
