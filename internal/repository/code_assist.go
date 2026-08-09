package repository

import (
	"context"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository/dao"
)

// CodeAssistRepository 定义 AI 会话和候选代码仓储能力。
type CodeAssistRepository interface {
	// CreateConversation 创建 AI 会话。
	CreateConversation(ctx context.Context, conversation domain.AIConversation) (domain.AIConversation, error)
	// GetConversationByID 查询 AI 会话。
	GetConversationByID(ctx context.Context, id int64) (domain.AIConversation, error)
	// ListConversations 查询用户在项目下的 AI 会话。
	ListConversations(ctx context.Context, userID, projectID int64, limit int) ([]domain.AIConversation, error)
	// ClaimConversation 原子占用 AI 会话。
	ClaimConversation(ctx context.Context, id, userID int64, runToken string) error
	// ReleaseConversation 释放 AI 会话。
	ReleaseConversation(ctx context.Context, id int64, runToken string) error
	// CreateMessage 创建 AI 消息。
	CreateMessage(ctx context.Context, message domain.AIMessage) (domain.AIMessage, error)
	// CompleteMessage 保存模型回复和用量。
	CompleteMessage(ctx context.Context, message domain.AIMessage) error
	// FailMessage 标记模型消息失败。
	FailMessage(ctx context.Context, message domain.AIMessage,
		status domain.AIMessageStatus, errorMessage string) error
	// ListMessages 查询 AI 会话消息。
	ListMessages(ctx context.Context, conversationID int64, limit int) ([]domain.AIMessage, error)
	// CreateChangeSet 保存完整候选变更。
	CreateChangeSet(ctx context.Context, changeSet domain.AIChangeSet) (domain.AIChangeSet, error)
	// ListChangeSets 查询会话中的项目级候选变更。
	ListChangeSets(ctx context.Context, conversationID int64) ([]domain.AIChangeSet, error)
	// GetChangeSetByID 查询一个项目级候选变更。
	GetChangeSetByID(ctx context.Context, id int64) (domain.AIChangeSet, error)
	// ClaimChangeSet 原子占用待应用的项目级候选变更。
	ClaimChangeSet(ctx context.Context, id int64) error
	// ReleaseChangeSet 释放应用失败的项目级候选变更。
	ReleaseChangeSet(ctx context.Context, id int64) error
	// MarkChangeSetApplied 原子记录变更集的文件应用结果。
	MarkChangeSetApplied(ctx context.Context, id int64, items []domain.AIChangeItem) error
}

type codeAssistRepository struct{ dao *dao.GORMCodeAssistDAO }

// NewCodeAssistRepository 创建 AI 仓储。
func NewCodeAssistRepository(source *dao.GORMCodeAssistDAO) CodeAssistRepository {
	return &codeAssistRepository{dao: source}
}

func (r *codeAssistRepository) CreateConversation(ctx context.Context,
	conversation domain.AIConversation) (domain.AIConversation, error) {
	created, err := r.dao.CreateConversation(ctx, toAIConversationEntity(conversation))
	return toAIConversationDomain(created), err
}

func (r *codeAssistRepository) GetConversationByID(ctx context.Context,
	id int64) (domain.AIConversation, error) {
	conversation, err := r.dao.GetConversationByID(ctx, id)
	return toAIConversationDomain(conversation), err
}

func (r *codeAssistRepository) ListConversations(ctx context.Context, userID, projectID int64,
	limit int) ([]domain.AIConversation, error) {
	entities, err := r.dao.ListConversations(ctx, userID, projectID, limit)
	result := make([]domain.AIConversation, 0, len(entities))
	for _, entity := range entities {
		result = append(result, toAIConversationDomain(entity))
	}
	return result, err
}

func (r *codeAssistRepository) ClaimConversation(ctx context.Context, id, userID int64,
	runToken string) error {
	return r.dao.ClaimConversation(ctx, id, userID, runToken)
}

func (r *codeAssistRepository) ReleaseConversation(ctx context.Context, id int64,
	runToken string) error {
	return r.dao.ReleaseConversation(ctx, id, runToken)
}

func (r *codeAssistRepository) CreateMessage(ctx context.Context,
	message domain.AIMessage) (domain.AIMessage, error) {
	created, err := r.dao.CreateMessage(ctx, toAIMessageEntity(message))
	return toAIMessageDomain(created), err
}

func (r *codeAssistRepository) CompleteMessage(ctx context.Context, message domain.AIMessage) error {
	return r.dao.CompleteMessage(ctx, toAIMessageEntity(message))
}

func (r *codeAssistRepository) FailMessage(ctx context.Context, message domain.AIMessage,
	status domain.AIMessageStatus, errorMessage string) error {
	return r.dao.FailMessage(ctx, toAIMessageEntity(message), string(status), errorMessage)
}

func (r *codeAssistRepository) ListMessages(ctx context.Context, conversationID int64,
	limit int) ([]domain.AIMessage, error) {
	entities, err := r.dao.ListMessages(ctx, conversationID, limit)
	result := make([]domain.AIMessage, 0, len(entities))
	for _, entity := range entities {
		result = append(result, toAIMessageDomain(entity))
	}
	return result, err
}

func (r *codeAssistRepository) CreateChangeSet(ctx context.Context,
	changeSet domain.AIChangeSet) (domain.AIChangeSet, error) {
	created, err := r.dao.CreateChangeSet(ctx, toAIChangeSetEntity(changeSet))
	return toAIChangeSetDomain(created), err
}

func (r *codeAssistRepository) ListChangeSets(ctx context.Context,
	conversationID int64) ([]domain.AIChangeSet, error) {
	sets, err := r.dao.ListChangeSets(ctx, conversationID)
	result := make([]domain.AIChangeSet, 0, len(sets))
	for _, changeSet := range sets {
		result = append(result, toAIChangeSetDomain(changeSet))
	}
	return result, err
}

func (r *codeAssistRepository) GetChangeSetByID(ctx context.Context,
	id int64) (domain.AIChangeSet, error) {
	changeSet, err := r.dao.GetChangeSetByID(ctx, id)
	return toAIChangeSetDomain(changeSet), err
}

func (r *codeAssistRepository) ClaimChangeSet(ctx context.Context, id int64) error {
	return r.dao.ClaimChangeSet(ctx, id)
}

func (r *codeAssistRepository) ReleaseChangeSet(ctx context.Context, id int64) error {
	return r.dao.ReleaseChangeSet(ctx, id)
}

func (r *codeAssistRepository) MarkChangeSetApplied(ctx context.Context, id int64,
	items []domain.AIChangeItem) error {
	return r.dao.MarkChangeSetApplied(ctx, id, changeItemsColumn(items))
}
