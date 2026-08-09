package repository

import (
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository/dao"
	"github.com/Duke1616/etask/pkg/sqlx"
)

func toAIConversationEntity(source domain.AIConversation) dao.AIConversation {
	return dao.AIConversation{
		ID: source.ID, TenantID: source.TenantID, UserID: source.UserID,
		ProjectID: source.ProjectID, Title: source.Title, Provider: source.Provider,
		Model: source.Model, Status: string(source.Status), CTime: source.CTime, UTime: source.UTime,
	}
}

func toAIConversationDomain(source dao.AIConversation) domain.AIConversation {
	return domain.AIConversation{
		ID: source.ID, TenantID: source.TenantID, UserID: source.UserID,
		ProjectID: source.ProjectID, Title: source.Title, Provider: source.Provider,
		Model: source.Model, Status: domain.AIConversationStatus(source.Status),
		CTime: source.CTime, UTime: source.UTime,
	}
}

func toAIMessageEntity(source domain.AIMessage) dao.AIMessage {
	return dao.AIMessage{
		ID: source.ID, TenantID: source.TenantID, ConversationID: source.ConversationID,
		Role: string(source.Role), Content: source.Content, Status: string(source.Status),
		Provider: source.Provider, Model: source.Model,
		RecipeID: source.RecipeID, RecipeVersion: source.RecipeVersion,
		InputTokens: source.InputTokens, OutputTokens: source.OutputTokens,
		LatencyMillis: source.LatencyMillis, ErrorMessage: source.ErrorMessage,
		CTime: source.CTime, UTime: source.UTime,
	}
}

func toAIMessageDomain(source dao.AIMessage) domain.AIMessage {
	return domain.AIMessage{
		ID: source.ID, TenantID: source.TenantID, ConversationID: source.ConversationID,
		Role: domain.AIMessageRole(source.Role), Content: source.Content,
		Status: domain.AIMessageStatus(source.Status), Provider: source.Provider, Model: source.Model,
		RecipeID: source.RecipeID, RecipeVersion: source.RecipeVersion,
		InputTokens: source.InputTokens, OutputTokens: source.OutputTokens,
		LatencyMillis: source.LatencyMillis, ErrorMessage: source.ErrorMessage,
		CTime: source.CTime, UTime: source.UTime,
	}
}

func toAIChangeSetEntity(source domain.AIChangeSet) dao.AIChangeSet {
	return dao.AIChangeSet{
		ID: source.ID, TenantID: source.TenantID, ConversationID: source.ConversationID,
		MessageID: source.MessageID, ProjectID: source.ProjectID,
		BaseRevision: source.BaseRevision, Summary: source.Summary,
		Items:  changeItemsColumn(source.Items),
		Status: string(source.Status), CTime: source.CTime, UTime: source.UTime,
	}
}

func toAIChangeSetDomain(source dao.AIChangeSet) domain.AIChangeSet {
	return domain.AIChangeSet{
		ID: source.ID, TenantID: source.TenantID, ConversationID: source.ConversationID,
		MessageID: source.MessageID, ProjectID: source.ProjectID,
		BaseRevision: source.BaseRevision, Summary: source.Summary, Items: source.Items.Val,
		Status: domain.AIChangeSetStatus(source.Status),
		CTime:  source.CTime, UTime: source.UTime,
	}
}

func changeItemsColumn(items []domain.AIChangeItem) sqlx.JSONColumn[[]domain.AIChangeItem] {
	return sqlx.JSONColumn[[]domain.AIChangeItem]{Val: items, Valid: true}
}
