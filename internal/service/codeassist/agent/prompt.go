package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Duke1616/etask/internal/ai"
	"github.com/cloudwego/eino/schema"
)

func (m *providerModel) providerRequest(messages []*schema.Message) (ai.Request, error) {
	instructions := make([]string, 0, 1)
	history := make([]promptMessage, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		if message.Role == schema.System {
			if content := strings.TrimSpace(message.Content); content != "" {
				instructions = append(instructions, content)
			}
			continue
		}
		item := promptMessage{
			Role: string(message.Role), Content: message.Content,
			ToolCallID: message.ToolCallID, ToolName: message.ToolName,
		}
		if message.Role == schema.Tool && json.Valid([]byte(message.Content)) {
			item.Content = json.RawMessage(message.Content)
		}
		for _, call := range message.ToolCalls {
			item.ToolCalls = append(item.ToolCalls, promptToolCall{
				Name: call.Function.Name, Arguments: call.Function.Arguments,
			})
		}
		history = append(history, item)
	}
	if len(history) == 0 {
		return ai.Request{}, fmt.Errorf("Eino model input contains no user or tool messages")
	}

	input, _ := history[0].Content.(string)
	if len(history) != 1 || history[0].Role != string(schema.User) {
		encoded, err := json.Marshal(struct {
			Messages []promptMessage `json:"messages"`
		}{Messages: history})
		if err != nil {
			return ai.Request{}, fmt.Errorf("encode Eino message history: %w", err)
		}
		input = string(encoded)
	}
	return ai.Request{
		Instructions: strings.Join(instructions, "\n\n"), Input: input,
		Tools: append([]ai.Tool(nil), m.tools...), UserKey: m.userKey,
	}, nil
}

type promptMessage struct {
	Role       string           `json:"role"`
	Content    any              `json:"content,omitempty"`
	ToolCalls  []promptToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolName   string           `json:"tool_name,omitempty"`
}

type promptToolCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

func einoToolsToAITools(infos []*schema.ToolInfo) ([]ai.Tool, error) {
	result := make([]ai.Tool, 0, len(infos))
	for _, info := range infos {
		if info == nil || strings.TrimSpace(info.Name) == "" {
			return nil, fmt.Errorf("Eino tool info is missing a name")
		}
		parameters := map[string]any{"type": "object", "additionalProperties": false}
		if info.ParamsOneOf != nil {
			definition, err := info.ParamsOneOf.ToJSONSchema()
			if err != nil {
				return nil, fmt.Errorf("convert Eino tool %s schema: %w", info.Name, err)
			}
			encoded, err := json.Marshal(definition)
			if err != nil {
				return nil, fmt.Errorf("encode Eino tool %s schema: %w", info.Name, err)
			}
			if err = json.Unmarshal(encoded, &parameters); err != nil {
				return nil, fmt.Errorf("decode Eino tool %s schema: %w", info.Name, err)
			}
		}
		result = append(result, ai.Tool{
			Name: info.Name, Description: info.Desc, Parameters: parameters,
		})
	}
	return result, nil
}
