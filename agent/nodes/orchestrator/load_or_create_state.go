package orchestratornode

import (
	"context"
	"errors"
	"fmt"
	"time"

	contractx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/contract"
	statex "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/state"
)

func LoadOrCreateState(
	ctx context.Context,
	in *GraphState,
	store statex.Store,
	workspaceID string,
	customerID string,
	channelType string,
) (*GraphState, error) {
	if in == nil {
		return nil, fmt.Errorf("%w: graph state is nil", contractx.ErrValidation)
	}

	st, err := loadOrCreateState(ctx, store, in.SessionID, workspaceID, customerID, channelType, in.Now)
	if err != nil {
		return nil, err
	}
	in.Session = st
	return in, nil
}

func loadOrCreateState(
	ctx context.Context,
	store statex.Store,
	sessionID string,
	workspaceID string,
	customerID string,
	channelType string,
	now time.Time,
) (*statex.SessionState, error) {
	st, err := store.Load(ctx, sessionID)
	if err == nil {
		return st, nil
	}
	if !errors.Is(err, statex.ErrStateNotFound) {
		return nil, err
	}

	return statex.NewSessionState(sessionID, workspaceID, customerID, channelType, now), nil
}
