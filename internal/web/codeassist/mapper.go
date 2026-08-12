package codeassist

import "github.com/Duke1616/etask/internal/domain"

func toChatRequest(req ChatReq) domain.AIChatRequest {
	return domain.AIChatRequest{
		ConversationID: req.ConversationID, ProfileID: req.ProfileID, Content: req.Content,
		Context: domain.AIChatContext{
			NodeID: req.Context.NodeID, BaseVersionID: req.Context.BaseVersionID,
			EditorCode: req.Context.EditorCode,
		},
	}
}

func toConversationVO(source domain.AIConversation) ConversationVO {
	return ConversationVO{
		ID: source.ID, Title: source.Title, Model: source.Model, UTime: source.UTime,
	}
}

func toMessageVO(source domain.AIMessage) MessageVO {
	return MessageVO{
		ID:   source.ID,
		Role: string(source.Role), Content: source.Content, Status: string(source.Status),
		InputTokens: source.InputTokens, OutputTokens: source.OutputTokens,
		LatencyMillis: source.LatencyMillis, ErrorMessage: source.ErrorMessage,
		CTime: source.CTime,
	}
}

func toChangeSetVO(source domain.AIChangeSet) ChangeSetVO {
	result := ChangeSetVO{
		ID: source.ID, MessageID: source.MessageID, BaseRevision: source.BaseRevision,
		Summary: source.Summary, Status: string(source.Status),
		Items: make([]ChangeItemVO, 0, len(source.Items)),
	}
	for _, item := range source.Items {
		result.Items = append(result.Items, ChangeItemVO{
			Operation: string(item.Operation), Path: item.Path, SourcePath: item.SourcePath,
			NodeID: item.NodeID, BaseVersionID: item.BaseVersionID,
			BaseHash: item.BaseHash,
			Language: item.Language, Code: item.Code,
			Diagnostics:      toDiagnosticVOs(item.Diagnostics),
			AppliedVersionID: item.AppliedVersionID,
		})
	}
	return result
}

func toDiagnosticVOs(source []domain.AIDiagnostic) []DiagnosticVO {
	result := make([]DiagnosticVO, 0, len(source))
	for _, diagnostic := range source {
		result = append(result, DiagnosticVO{
			Severity: string(diagnostic.Severity), Code: diagnostic.Code,
			Message: diagnostic.Message,
		})
	}
	return result
}
