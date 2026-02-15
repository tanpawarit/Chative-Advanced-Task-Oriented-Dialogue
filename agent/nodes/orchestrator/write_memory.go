package orchestratornode

import (
	"context"
	"fmt"

	contractx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/contract"
)

func WriteMemory(
	ctx context.Context,
	in *GraphState,
	memory contractx.MemoryStore,
) (*GraphState, error) {
	if in == nil || in.Session == nil {
		return nil, fmt.Errorf("%w: graph session is nil", contractx.ErrValidation)
	}

	if err := memory.WriteSummary(ctx, in.Session.CustomerID, in.StateUpdates.MemoryUpdate); err != nil {
		return nil, err
	}
	return in, nil
}
