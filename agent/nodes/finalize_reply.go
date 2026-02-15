package orchestratornode

import (
	"fmt"
	"strings"

	contractx "github.com/tanpawarit/Chative-Advanced-Task-Oriented-Dialogue/agent/contract"
)

func FinalizeReply(in *GraphState) (GraphOutput, error) {
	if in == nil {
		return GraphOutput{}, fmt.Errorf("%w: graph state is nil", contractx.ErrValidation)
	}

	reply := strings.TrimSpace(in.Message)
	if reply == "" {
		return GraphOutput{}, fmt.Errorf("%w: specialist returned empty message", contractx.ErrValidation)
	}
	return GraphOutput{Reply: reply}, nil
}
