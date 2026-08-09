package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Duke1616/etask/internal/ai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// providerModel adapts etask's stable provider boundary to Eino.
type providerModel struct {
	provider ai.Provider
	userKey  string
	tools    []ai.Tool
	usage    *usageAccumulator
	callID   *atomic.Uint64
}

var _ model.ToolCallingChatModel = (*providerModel)(nil)

func newProviderModel(provider ai.Provider, userKey string, usage *usageAccumulator) *providerModel {
	return &providerModel{
		provider: provider, userKey: userKey, usage: usage, callID: &atomic.Uint64{},
	}
}

func (m *providerModel) WithTools(infos []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	tools, err := einoToolsToAITools(infos)
	if err != nil {
		return nil, err
	}
	clone := *m
	clone.tools = tools
	return &clone, nil
}

func (m *providerModel) Generate(ctx context.Context, messages []*schema.Message,
	_ ...model.Option) (*schema.Message, error) {
	request, err := m.providerRequest(messages)
	if err != nil {
		return nil, err
	}
	stream, err := m.provider.Stream(ctx, request)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	var content strings.Builder
	toolCalls := make([]schema.ToolCall, 0, 1)
	var usage ai.Usage
	completed := false
	for stream.Next() {
		event := stream.Current()
		switch event.Type {
		case ai.EventTypeTextDelta:
			content.WriteString(event.Text)
		case ai.EventTypeToolCall:
			if event.ToolCall != nil {
				toolCalls = append(toolCalls, schema.ToolCall{
					ID: fmt.Sprintf("agent_call_%d", m.callID.Add(1)), Type: "function",
					Function: schema.FunctionCall{
						Name: event.ToolCall.Name, Arguments: event.ToolCall.Arguments,
					},
				})
			}
		case ai.EventTypeCompleted:
			completed, usage = true, event.Usage
		case ai.EventTypeFailed:
			if event.Err == nil {
				event.Err = fmt.Errorf("AI response failed")
			}
			return nil, event.Err
		}
	}
	if err = stream.Err(); err != nil {
		return nil, err
	}
	if !completed {
		return nil, fmt.Errorf("AI response ended without completion")
	}
	if len(toolCalls) > 1 {
		return nil, fmt.Errorf("agent must call one tool at a time")
	}
	if content.Len() == 0 && len(toolCalls) == 0 {
		return nil, fmt.Errorf("agent returned an empty response")
	}

	m.usage.Add(usage)
	message := schema.AssistantMessage(content.String(), toolCalls)
	message.ResponseMeta = &schema.ResponseMeta{Usage: &schema.TokenUsage{
		PromptTokens: int(usage.InputTokens), CompletionTokens: int(usage.OutputTokens),
		TotalTokens: int(usage.InputTokens + usage.OutputTokens),
	}}
	return message, nil
}

func (m *providerModel) Stream(ctx context.Context, messages []*schema.Message,
	opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	message, err := m.Generate(ctx, messages, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{message}), nil
}

type usageAccumulator struct {
	mu    sync.Mutex
	usage ai.Usage
}

func (a *usageAccumulator) Add(value ai.Usage) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.usage.InputTokens += value.InputTokens
	a.usage.OutputTokens += value.OutputTokens
}

func (a *usageAccumulator) Value() ai.Usage {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.usage
}
