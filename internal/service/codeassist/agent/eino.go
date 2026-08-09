package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Duke1616/etask/internal/ai"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
)

var _ Runner = (*EinoRunner)(nil)

// EinoRunner uses Eino ReAct for bounded model/tool orchestration.
type EinoRunner struct {
	provider ai.Provider
}

func NewEinoRunner(provider ai.Provider) *EinoRunner {
	return &EinoRunner{provider: provider}
}

func (r *EinoRunner) Run(ctx context.Context, request Request) (Result, error) {
	if r.provider == nil {
		return Result{}, fmt.Errorf("agent provider is required")
	}
	if request.MaxTurns <= 0 || len(request.Tools) == 0 {
		return Result{}, fmt.Errorf("agent limits and tools are required")
	}

	tools := make([]tool.BaseTool, 0, len(request.Tools))
	direct := make(map[string]struct{})
	for _, value := range request.Tools {
		invokable, err := newInvokableTool(value.Definition, value.Run)
		if err != nil {
			return Result{}, err
		}
		tools = append(tools, invokable)
		if value.ReturnDirectly {
			direct[value.Definition.Name] = struct{}{}
		}
	}

	usage := &usageAccumulator{}
	model := newProviderModel(r.provider, request.UserKey, usage)
	reactAgent, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: model,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: tools, ExecuteSequentially: true,
		},
		MaxStep:            request.MaxTurns * 2,
		ToolReturnDirectly: direct,
		GraphName:          "CodeAssistAgent",
	})
	if err != nil {
		return Result{}, fmt.Errorf("create Eino agent: %w", err)
	}

	message, err := reactAgent.Generate(ctx, []*schema.Message{
		schema.SystemMessage(request.Instructions),
		schema.UserMessage(request.Input),
	})
	if err != nil {
		return Result{}, fmt.Errorf("run Eino agent: %w", err)
	}
	if message == nil || strings.TrimSpace(message.Content) == "" {
		return Result{}, fmt.Errorf("agent returned an empty response")
	}
	return Result{Text: message.Content, Usage: usage.Value()}, nil
}

type invokableTool struct {
	info *schema.ToolInfo
	run  ToolFunc
}

var _ tool.InvokableTool = (*invokableTool)(nil)

func newInvokableTool(definition ai.Tool, run ToolFunc) (*invokableTool, error) {
	if run == nil || strings.TrimSpace(definition.Name) == "" {
		return nil, fmt.Errorf("agent tool definition and runner are required")
	}
	encoded, err := json.Marshal(definition.Parameters)
	if err != nil {
		return nil, fmt.Errorf("encode tool %s schema: %w", definition.Name, err)
	}
	parameters := &jsonschema.Schema{}
	if err = json.Unmarshal(encoded, parameters); err != nil {
		return nil, fmt.Errorf("decode tool %s schema: %w", definition.Name, err)
	}
	return &invokableTool{
		info: &schema.ToolInfo{
			Name: definition.Name, Desc: definition.Description,
			ParamsOneOf: schema.NewParamsOneOfByJSONSchema(parameters),
		},
		run: run,
	}, nil
}

func (t *invokableTool) Info(context.Context) (*schema.ToolInfo, error) { return t.info, nil }

func (t *invokableTool) InvokableRun(ctx context.Context, arguments string,
	_ ...tool.Option) (string, error) {
	return t.run(ctx, arguments)
}
