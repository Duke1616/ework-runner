package codeassist

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	"github.com/google/uuid"
)

func (s *service) Chat(ctx context.Context, request domain.AIChatRequest, emit EventEmitter) error {
	if emit == nil {
		return fmt.Errorf("%w: AI event emitter is required", errs.ErrInvalidParameter)
	}
	request.Content = strings.TrimSpace(request.Content)
	if request.ConversationID <= 0 || request.Content == "" || len(request.Content) > maxUserMessageLength {
		return fmt.Errorf("%w: invalid AI chat request", errs.ErrInvalidParameter)
	}
	profile, err := resolveProfile(request.ProfileID)
	if err != nil {
		return err
	}
	conversation, err := s.userConversation(ctx, request.ConversationID)
	if err != nil {
		return err
	}
	userID := ctxutil.GetUserID(ctx).Int64()
	runToken := uuid.NewString()
	if err = s.repo.ClaimConversation(ctx, conversation.ID, userID, runToken); err != nil {
		return err
	}
	defer func() {
		settleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), messageSettleTimeout)
		defer cancel()
		_ = s.repo.ReleaseConversation(settleCtx, conversation.ID, runToken)
	}()

	chatContext, err := s.prepareContext(ctx, conversation, request.Context)
	if err != nil {
		return err
	}
	history, err := s.repo.ListMessages(ctx, conversation.ID, messageHistoryLimit)
	if err != nil {
		return err
	}
	if _, err = s.repo.CreateMessage(ctx, domain.AIMessage{
		ConversationID: conversation.ID, Role: domain.AIMessageRoleUser,
		Content: request.Content, Status: domain.AIMessageStatusCompleted,
		ProfileID: profile.ID, ProfileVersion: profile.Version,
	}); err != nil {
		return err
	}
	assistantMessage, err := s.repo.CreateMessage(ctx, domain.AIMessage{
		ConversationID: conversation.ID, Role: domain.AIMessageRoleAssistant,
		Status:   domain.AIMessageStatusStreaming,
		Provider: s.provider.Name(), Model: s.provider.Model(),
		ProfileID: profile.ID, ProfileVersion: profile.Version,
	})
	if err != nil {
		return err
	}
	startedAt := time.Now()
	var content strings.Builder
	// 请求断开后仍用独立上下文收敛消息和会话状态，避免遗留 STREAMING 记录。
	fail := func(cause error) error {
		settleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), messageSettleTimeout)
		defer cancel()
		status := domain.AIMessageStatusFailed
		if ctx.Err() != nil {
			status = domain.AIMessageStatusCancelled
		}
		assistantMessage.Content = content.String()
		assistantMessage.LatencyMillis = time.Since(startedAt).Milliseconds()
		_ = s.repo.FailMessage(settleCtx, assistantMessage, status, cause.Error())
		_ = emit(StreamEvent{
			Type: StreamEventTypeFailed, MessageID: assistantMessage.ID, Err: cause,
		})
		return cause
	}
	if err = emit(StreamEvent{Type: StreamEventTypeStarted, MessageID: assistantMessage.ID}); err != nil {
		return fail(err)
	}
	result, err := s.runWorkspaceAgent(ctx, conversation, assistantMessage.ID, profile,
		history, request.Content, chatContext, emit)
	if err != nil {
		return fail(err)
	}
	content.WriteString(result.Text)
	assistantMessage.Content = result.Text
	assistantMessage.InputTokens = result.Usage.InputTokens
	assistantMessage.OutputTokens = result.Usage.OutputTokens
	assistantMessage.LatencyMillis = time.Since(startedAt).Milliseconds()
	if strings.TrimSpace(result.Text) != "" {
		if err = emit(StreamEvent{Type: StreamEventTypeDelta,
			MessageID: assistantMessage.ID, Text: result.Text}); err != nil {
			return fail(err)
		}
	}
	if err = s.repo.CompleteMessage(ctx, assistantMessage); err != nil {
		return fail(err)
	}
	return emit(StreamEvent{
		Type: StreamEventTypeCompleted, MessageID: assistantMessage.ID, Usage: result.Usage,
	})
}
