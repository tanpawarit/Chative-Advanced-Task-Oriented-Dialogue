package orchestratornode

import (
	"context"
	"fmt"

	contractx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/contract"
	statex "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/state"
)

func ValidateAndSaveState(
	ctx context.Context,
	in *GraphState,
	store statex.Store,
) (*GraphState, error) {
	if in == nil || in.Session == nil {
		return nil, fmt.Errorf("%w: graph session is nil", contractx.ErrValidation)
	}

	in.Session.Touch(in.Now)
	if err := in.Session.Validate(); err != nil {
		return nil, fmt.Errorf("state validation failed: %w", err)
	}
	if err := store.Save(ctx, in.Session); err != nil {
		return nil, err
	}

	return in, nil
}
