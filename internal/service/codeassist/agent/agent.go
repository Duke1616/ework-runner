// Package agent provides the model/tool orchestration boundary for CodeAssist.
package agent

import (
	"context"

	"github.com/Duke1616/etask/internal/ai"
)

// ToolFunc executes one controlled tool call.
type ToolFunc func(ctx context.Context, arguments string) (string, error)

// Tool binds a model-visible definition to business-owned execution logic.
type Tool struct {
	Definition     ai.Tool
	Run            ToolFunc
	ReturnDirectly bool
}

// Request describes one bounded agent run.
type Request struct {
	Instructions string
	Input        string
	UserKey      string
	MaxTurns     int
	Tools        []Tool
}

// Result contains the final display text and aggregate model usage.
type Result struct {
	Text  string
	Usage ai.Usage
}

// Runner orchestrates model and tool calls without owning business rules.
type Runner interface {
	Run(ctx context.Context, request Request) (Result, error)
}
