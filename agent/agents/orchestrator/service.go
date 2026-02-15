package orchestrator

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/cloudwego/eino/compose"
	contractx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/contract"
	nodex "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/nodes/orchestrator"
	statex "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/state"
)

var (
	ErrInvalidMessage = nodex.ErrInvalidMessage
	ErrInvalidSession = nodex.ErrInvalidSession
	ErrNoActiveGoal   = nodex.ErrNoActiveGoal
)

type Config struct {
	WorkspaceID string
	CustomerID  string
	ChannelType string
}

type Orchestrator struct {
	store  statex.Store
	models contractx.Registry
	tools  contractx.ToolGateway
	memory contractx.MemoryStore

	graphRunner compose.Runnable[nodex.GraphInput, nodex.GraphOutput]

	workspaceID string
	customerID  string
	channelType string

	now func() time.Time
}

func New(
	store statex.Store,
	models contractx.Registry,
	tools contractx.ToolGateway,
	memory contractx.MemoryStore,
	cfg Config,
) (*Orchestrator, error) {
	if store == nil {
		return nil, errors.New("state store is required")
	}
	if models == nil {
		return nil, errors.New("model registry is required")
	}
	if tools == nil {
		return nil, errors.New("tool gateway is required")
	}
	if memory == nil {
		memory = noopMemoryStore{}
	}

	workspaceID := strings.TrimSpace(cfg.WorkspaceID)
	if workspaceID == "" {
		workspaceID = "default-workspace"
	}
	customerID := strings.TrimSpace(cfg.CustomerID)
	if customerID == "" {
		customerID = "default-customer"
	}
	channelType := strings.TrimSpace(cfg.ChannelType)
	if channelType == "" {
		channelType = "chat"
	}

	o := &Orchestrator{
		store:       store,
		models:      models,
		tools:       tools,
		memory:      memory,
		workspaceID: workspaceID,
		customerID:  customerID,
		channelType: channelType,
		now:         time.Now,
	}

	graphRunner, err := o.compileHandleMessageGraph(context.Background())
	if err != nil {
		return nil, err
	}
	o.graphRunner = graphRunner

	return o, nil
}

func (o *Orchestrator) HandleMessage(ctx context.Context, sessionID string, text string) (string, error) {
	out, err := o.graphRunner.Invoke(ctx, nodex.GraphInput{
		SessionID: sessionID,
		Text:      text,
	})
	if err != nil {
		return "", err
	}
	return out.Reply, nil
}

type noopMemoryStore struct{}

func (noopMemoryStore) ReadSummary(context.Context, string) (string, error) {
	return "", nil
}

func (noopMemoryStore) WriteSummary(context.Context, string, string) error {
	return nil
}
