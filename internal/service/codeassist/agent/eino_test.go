package agent

import (
	"context"
	"fmt"
	"testing"

	"github.com/Duke1616/etask/internal/ai"
	"github.com/stretchr/testify/require"
)

type providerStub struct {
	turns    [][]ai.Event
	requests []ai.Request
}

func (*providerStub) Name() string  { return "test" }
func (*providerStub) Model() string { return "test-model" }

func (p *providerStub) Stream(_ context.Context, request ai.Request) (ai.Stream, error) {
	p.requests = append(p.requests, request)
	if len(p.turns) == 0 {
		return nil, fmt.Errorf("unexpected model turn")
	}
	stream := &streamStub{events: p.turns[0]}
	p.turns = p.turns[1:]
	return stream, nil
}

type streamStub struct {
	events  []ai.Event
	current int
}

func (s *streamStub) Next() bool {
	if s.current >= len(s.events) {
		return false
	}
	s.current++
	return true
}

func (s *streamStub) Current() ai.Event { return s.events[s.current-1] }
func (s *streamStub) Err() error        { return nil }
func (s *streamStub) Close() error      { return nil }

func TestEinoRunnerLoopsThroughToolsAndAggregatesUsage(t *testing.T) {
	provider := &providerStub{turns: [][]ai.Event{
		{
			{Type: ai.EventTypeToolCall, ToolCall: &ai.ToolCall{
				Name: "read_files", Arguments: `{"paths":["site.yml"]}`,
			}},
			{Type: ai.EventTypeCompleted, Usage: ai.Usage{InputTokens: 10, OutputTokens: 2}},
		},
		{
			{Type: ai.EventTypeToolCall, ToolCall: &ai.ToolCall{
				Name: "propose", Arguments: `{"summary":"done"}`,
			}},
			{Type: ai.EventTypeCompleted, Usage: ai.Usage{InputTokens: 30, OutputTokens: 20}},
		},
	}}
	readCalled, proposeCalled := false, false
	runner := NewEinoRunner(provider)

	result, err := runner.Run(t.Context(), Request{
		Instructions: "inspect project", Input: "update site.yml", MaxTurns: 6,
		Tools: []Tool{
			{
				Definition: testTool("read_files"),
				Run: func(context.Context, string) (string, error) {
					readCalled = true
					return `{"files":[{"path":"site.yml","content":"---\n"}]}`, nil
				},
			},
			{
				Definition: testTool("propose"), ReturnDirectly: true,
				Run: func(context.Context, string) (string, error) {
					proposeCalled = true
					return "change set created", nil
				},
			},
		},
	})

	require.NoError(t, err)
	require.True(t, readCalled)
	require.True(t, proposeCalled)
	require.Equal(t, "change set created", result.Text)
	require.Equal(t, ai.Usage{InputTokens: 40, OutputTokens: 22}, result.Usage)
	require.Len(t, provider.requests, 2)
	require.Contains(t, provider.requests[1].Input,
		`"content":{"files":[{"path":"site.yml","content":"---\n"}]}`)
}

func testTool(name string) ai.Tool {
	return ai.Tool{
		Name: name, Description: name,
		Parameters: map[string]any{
			"type": "object", "properties": map[string]any{},
			"additionalProperties": false,
		},
	}
}
