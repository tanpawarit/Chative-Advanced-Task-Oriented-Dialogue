package prompt

import (
	_ "embed"
	"strings"
	"sync"
)

var (
	//go:embed template/orchestrator.txt
	orchestratorRaw string

	//go:embed template/sales.txt
	salesRaw string

	//go:embed template/support.txt
	supportRaw string
)

// PromptSet holds loaded prompt content.
type PromptSet struct {
	Orchestrator string
	Sales        string
	Support      string
}

// LoadPromptSet returns a PromptSet with trimmed prompt strings.
// This is safe to call concurrently; the embed is compile-time, and trimming is cheap.
func LoadPromptSet() PromptSet {
	return PromptSet{
		Orchestrator: strings.TrimSpace(orchestratorRaw),
		Sales:        strings.TrimSpace(salesRaw),
		Support:      strings.TrimSpace(supportRaw),
	}
}

// PromptLoader loads and caches prompt sets.
type PromptLoader struct {
	once sync.Once
	set  PromptSet
}

// Get returns the cached PromptSet, initializing it on first call.
func (p *PromptLoader) Get() PromptSet {
	p.once.Do(func() {
		p.set = LoadPromptSet()
	})
	return p.set
}

// Global loader instance for convenient access.
var globalLoader = &PromptLoader{}

// GetPromptSet returns the global cached PromptSet.
func GetPromptSet() PromptSet {
	return globalLoader.Get()
}
