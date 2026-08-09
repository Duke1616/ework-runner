package codeassist

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
)

func (s *service) getChangeSet(ctx context.Context, id int64) (domain.AIChangeSet, error) {
	if id <= 0 {
		return domain.AIChangeSet{}, fmt.Errorf("%w: invalid AI change set ID", errs.ErrInvalidParameter)
	}
	changeSet, err := s.repo.GetChangeSetByID(ctx, id)
	if err != nil {
		return domain.AIChangeSet{}, err
	}
	conversation, err := s.userConversation(ctx, changeSet.ConversationID)
	if err != nil || conversation.ProjectID != changeSet.ProjectID {
		return domain.AIChangeSet{}, fmt.Errorf("AI change set is not accessible")
	}
	return changeSet, nil
}

func (s *service) ApplyChangeSet(ctx context.Context,
	id int64) ([]domain.CodebookProjectChangeResult, error) {
	changeSet, err := s.getChangeSet(ctx, id)
	if err != nil {
		return nil, err
	}
	if changeSet.Status == domain.AIChangeSetStatusApplied {
		return appliedChangeSetResults(changeSet.Items), nil
	}
	if changeSet.Status != domain.AIChangeSetStatusValidated &&
		changeSet.Status != domain.AIChangeSetStatusApplying {
		return nil, fmt.Errorf("%w: AI change set cannot be applied in status %s",
			errs.ErrAIChangeSetConflict, changeSet.Status)
	}
	for _, item := range changeSet.Items {
		if hasDiagnosticErrors(validateCandidate(ctx, item.Language, item.Code)) {
			return nil, fmt.Errorf("AI change set validation failed for %s", item.Path)
		}
	}

	applyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
	defer cancel()
	if err = s.repo.ClaimChangeSet(applyCtx, changeSet.ID); err != nil {
		return nil, err
	}
	releaseClaim := true
	defer func() {
		if releaseClaim {
			_ = s.repo.ReleaseChangeSet(applyCtx, changeSet.ID)
		}
	}()

	results, err := s.codebooks.ApplyProjectChangeSet(applyCtx, domain.CodebookProjectChangeSet{
		ProjectID: changeSet.ProjectID, BaseRevision: changeSet.BaseRevision,
		Changes: codebookChanges(changeSet),
	})
	if err != nil {
		return nil, err
	}
	appliedItems, err := mergeAppliedResults(changeSet.Items, results)
	if err != nil {
		return nil, err
	}
	if err = s.repo.MarkChangeSetApplied(applyCtx, changeSet.ID, appliedItems); err != nil {
		latest, latestErr := s.getChangeSet(applyCtx, changeSet.ID)
		if latestErr == nil && latest.Status == domain.AIChangeSetStatusApplied {
			releaseClaim = false
			return appliedChangeSetResults(latest.Items), nil
		}
		return nil, err
	}
	releaseClaim = false
	return results, nil
}

func codebookChanges(changeSet domain.AIChangeSet) []domain.CodebookProjectChange {
	changes := make([]domain.CodebookProjectChange, 0, len(changeSet.Items))
	for index, item := range changeSet.Items {
		operation := domain.CodebookChangeOperationCreate
		if item.Operation == domain.AIChangeOperationUpdate {
			operation = domain.CodebookChangeOperationUpdate
		}
		changes = append(changes, domain.CodebookProjectChange{
			Operation: operation, Path: item.Path, NodeID: item.NodeID,
			ExpectedCurrentVersionID: item.BaseVersionID, ExpectedHash: item.BaseHash,
			Code: item.Code, Message: changeSetVersionMessage(changeSet),
			SourceKey: fmt.Sprintf("ai-change-set:%d:item:%d", changeSet.ID, index+1),
		})
	}
	return changes
}

func mergeAppliedResults(items []domain.AIChangeItem,
	results []domain.CodebookProjectChangeResult) ([]domain.AIChangeItem, error) {
	byPath := make(map[string]domain.CodebookProjectChangeResult, len(results))
	for _, result := range results {
		byPath[strings.ToLower(result.Path)] = result
	}
	applied := append([]domain.AIChangeItem(nil), items...)
	for index := range applied {
		result, exists := byPath[strings.ToLower(applied[index].Path)]
		if !exists {
			return nil, fmt.Errorf("missing applied result for %s", applied[index].Path)
		}
		applied[index].NodeID = result.NodeID
		applied[index].AppliedVersionID = result.VersionID
	}
	return applied, nil
}

func changeSetVersionMessage(changeSet domain.AIChangeSet) string {
	message := fmt.Sprintf("AI change set #%d", changeSet.ID)
	if summary := strings.TrimSpace(changeSet.Summary); summary != "" {
		message += ": " + summary
	}
	runes := []rune(message)
	if len(runes) > 255 {
		message = string(runes[:255])
	}
	return message
}

func appliedChangeSetResults(items []domain.AIChangeItem) []domain.CodebookProjectChangeResult {
	result := make([]domain.CodebookProjectChangeResult, 0, len(items))
	for _, item := range items {
		result = append(result, domain.CodebookProjectChangeResult{
			Path: item.Path, NodeID: item.NodeID, VersionID: item.AppliedVersionID,
		})
	}
	return result
}
