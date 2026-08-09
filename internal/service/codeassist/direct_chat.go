package codeassist

import (
	"context"
	"fmt"
	"strings"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/etask/internal/ai"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/service/codeassist/recipe"
)

func (s *service) runDirectChat(ctx context.Context, conversation domain.AIConversation,
	assistantMessageID int64, selectedRecipe recipe.Definition, instructions string,
	history []domain.AIMessage, userContent string, prepared preparedContext,
	content *strings.Builder, emit EventEmitter) (ai.Usage, error) {
	request := ai.Request{
		Instructions: instructions,
		Input:        buildPrompt(history, userContent, prepared),
		UserKey: fmt.Sprintf("%d:%d", ctxutil.GetTenantID(ctx).Int64(),
			ctxutil.GetUserID(ctx).Int64()),
	}
	if prepared.node.ID > 0 && selectedRecipe.AllowsChanges {
		request.Tools = []ai.Tool{currentFileChangeTool()}
	}
	stream, err := s.provider.Stream(ctx, request)
	if err != nil {
		return ai.Usage{}, err
	}
	defer stream.Close()

	var proposal string
	var usage ai.Usage
	completed, proposalReceived := false, false
	for stream.Next() {
		event := stream.Current()
		switch event.Type {
		case ai.EventTypeTextDelta:
			content.WriteString(event.Text)
			if err = emit(StreamEvent{
				Type: StreamEventTypeDelta, MessageID: assistantMessageID, Text: event.Text,
			}); err != nil {
				return usage, err
			}
		case ai.EventTypeToolCallStarted:
			if err = emit(StreamEvent{
				Type: StreamEventTypeProgress, MessageID: assistantMessageID,
				Text: "正在生成候选变更",
			}); err != nil {
				return usage, err
			}
		case ai.EventTypeToolCall:
			if event.ToolCall == nil || event.ToolCall.Name != proposeCurrentFileToolName {
				continue
			}
			if proposalReceived {
				return usage, fmt.Errorf("AI response contains multiple file changes")
			}
			proposal, proposalReceived = event.ToolCall.Arguments, true
			if err = emit(StreamEvent{
				Type: StreamEventTypeProgress, MessageID: assistantMessageID,
				Text: "正在校验候选变更",
			}); err != nil {
				return usage, err
			}
		case ai.EventTypeCompleted:
			completed, usage = true, event.Usage
		case ai.EventTypeFailed:
			if event.Err == nil {
				event.Err = fmt.Errorf("AI response failed")
			}
			return usage, event.Err
		}
	}
	if err = stream.Err(); err != nil {
		return usage, err
	}
	if !completed {
		return usage, fmt.Errorf("AI response ended without completion")
	}
	if content.Len() == 0 && !proposalReceived {
		return usage, fmt.Errorf("模型未返回可展示的文本或候选变更")
	}
	if proposalReceived {
		_, err = s.createCurrentFileChangeSet(ctx, conversation, assistantMessageID,
			prepared, selectedRecipe, proposal)
	}
	return usage, err
}
