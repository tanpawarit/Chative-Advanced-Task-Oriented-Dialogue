package prompt

import (
	_ "embed"
	"strings"
)

var (
	//go:embed template/planner.txt
	plannerRaw string

	//go:embed template/sales.txt
	salesRaw string

	//go:embed template/support.txt
	supportRaw string
)

// PromptSet holds loaded prompt content.
type PromptSet struct {
	Planner string
	Sales   string
	Support string
}

// LoadPromptSet returns a PromptSet with trimmed prompt strings.
// This is safe to call concurrently; the embed is compile-time, and trimming is cheap.
func LoadPromptSet() PromptSet {
	return PromptSet{
		Planner: strings.TrimSpace(plannerRaw),
		Sales:   strings.TrimSpace(salesRaw),
		Support: strings.TrimSpace(supportRaw),
	}
}
