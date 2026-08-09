package codeassist

import (
	"context"
	"fmt"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/etask/internal/ai"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
)

const conversationListLimit = 50

func (s *service) CreateConversation(ctx context.Context, projectID int64,
	title string) (domain.AIConversation, error) {
	if err := ai.EnsureAvailable(s.provider); err != nil {
		return domain.AIConversation{}, err
	}
	if _, err := s.codebooks.GetProjectByID(ctx, projectID); err != nil {
		return domain.AIConversation{}, err
	}
	conversation := domain.AIConversation{
		UserID: ctxutil.GetUserID(ctx).Int64(), ProjectID: projectID,
		Title: title, Provider: s.provider.Name(), Model: s.provider.Model(),
		Status: domain.AIConversationStatusActive,
	}
	if err := conversation.ValidateCreate(); err != nil {
		return domain.AIConversation{}, err
	}
	return s.repo.CreateConversation(ctx, conversation)
}

func (s *service) ListConversations(ctx context.Context, projectID int64) ([]domain.AIConversation, error) {
	if projectID <= 0 {
		return nil, fmt.Errorf("%w: invalid AI conversation project", errs.ErrInvalidParameter)
	}
	if _, err := s.codebooks.GetProjectByID(ctx, projectID); err != nil {
		return nil, err
	}
	return s.repo.ListConversations(ctx, ctxutil.GetUserID(ctx).Int64(), projectID, conversationListLimit)
}

func (s *service) ConversationDetail(ctx context.Context,
	conversationID int64) ([]domain.AIMessage, []domain.AIChangeSet, error) {
	if _, err := s.userConversation(ctx, conversationID); err != nil {
		return nil, nil, err
	}
	messages, err := s.repo.ListMessages(ctx, conversationID, 200)
	if err != nil {
		return nil, nil, err
	}
	changeSets, err := s.repo.ListChangeSets(ctx, conversationID)
	return messages, changeSets, err
}

func (s *service) userConversation(ctx context.Context, id int64) (domain.AIConversation, error) {
	if id <= 0 {
		return domain.AIConversation{}, fmt.Errorf("%w: invalid AI conversation ID", errs.ErrInvalidParameter)
	}
	conversation, err := s.repo.GetConversationByID(ctx, id)
	if err != nil {
		return domain.AIConversation{}, err
	}
	if conversation.UserID != ctxutil.GetUserID(ctx).Int64() {
		return domain.AIConversation{}, fmt.Errorf("AI conversation is not accessible")
	}
	return conversation, nil
}
