package agent

import (
	"context"

	"github.com/asitha-w/warden/internal/domain"
)

// Reasoner is the model-facing side of an agent. The scripted reasoner runs
// offline from playbooks; the Claude reasoner calls the API with tools.
type Reasoner interface {
	// Investigate drives one investigation run through the toolbox until conclude.
	Investigate(ctx context.Context, run *domain.Run, tb *Toolbox) error
	// Chat answers a question for the orchestrator or about a run. onDelta
	// receives streamed text; the full answer is returned.
	Chat(ctx context.Context, req ChatRequest, onDelta func(string)) (string, error)
	// Name is shown in the console.
	Name() string
}

// ChatRequest carries what the model needs to answer.
type ChatRequest struct {
	System   string
	Context  string
	Question string
	History  []domain.ChatMessage
	Tools    []ToolDef
	Call     func(ctx context.Context, name string, in map[string]any) (string, error)
	RunID    string // for usage accounting; empty for the orchestrator
	AgentID  string
	MaxCalls int
}
