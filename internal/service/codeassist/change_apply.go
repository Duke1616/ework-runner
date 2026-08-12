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
	switch changeSet.Status {
	case domain.AIChangeSetStatusApplied:
		return appliedChangeSetResults(changeSet.Items), nil
	case domain.AIChangeSetStatusCleanupPending:
		return s.completePendingCleanup(ctx, changeSet)
	case domain.AIChangeSetStatusValidated, domain.AIChangeSetStatusApplying:
		// Continue with the database apply phase.
	default:
		return nil, fmt.Errorf("%w: AI change set cannot be applied in status %s",
			errs.ErrAIChangeSetConflict, changeSet.Status)
	}
	if err = revalidateChangeSet(ctx, changeSet.Items); err != nil {
		return nil, err
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

	changes, err := codebookChanges(changeSet)
	if err != nil {
		return nil, err
	}
	results, err := s.codebooks.ApplyProjectChangeSet(applyCtx, domain.CodebookProjectChangeSet{
		ProjectID: changeSet.ProjectID, BaseRevision: changeSet.BaseRevision,
		Changes: changes,
	})
	if err != nil {
		return nil, err
	}
	appliedItems, err := mergeAppliedResults(changeSet.Items, results)
	if err != nil {
		return nil, err
	}
	cleanupPending := hasCleanupObjects(appliedItems)
	if err = s.repo.RecordChangeSetApplied(applyCtx, changeSet.ID, appliedItems, cleanupPending); err != nil {
		latest, latestErr := s.getChangeSet(applyCtx, changeSet.ID)
		if latestErr == nil && (latest.Status == domain.AIChangeSetStatusApplied ||
			latest.Status == domain.AIChangeSetStatusCleanupPending) {
			releaseClaim = false
			if latest.Status == domain.AIChangeSetStatusCleanupPending {
				return s.completePendingCleanup(ctx, latest)
			}
			return appliedChangeSetResults(latest.Items), nil
		}
		return nil, err
	}
	releaseClaim = false
	if cleanupPending {
		changeSet.Items = appliedItems
		return s.completePendingCleanup(ctx, changeSet)
	}
	return appliedChangeSetResults(appliedItems), nil
}

func revalidateChangeSet(ctx context.Context, items []domain.AIChangeItem) error {
	for _, item := range items {
		if item.Operation == domain.AIChangeOperationRename || item.Operation == domain.AIChangeOperationDelete {
			continue
		}
		if hasDiagnosticErrors(validateCandidate(ctx, item.Language, item.Code)) {
			return fmt.Errorf("AI change set validation failed for %s", item.Path)
		}
	}
	return nil
}

func codebookChanges(changeSet domain.AIChangeSet) ([]domain.CodebookProjectChange, error) {
	changes := make([]domain.CodebookProjectChange, 0, len(changeSet.Items))
	for index, item := range changeSet.Items {
		var operation domain.CodebookChangeOperation
		switch item.Operation {
		case domain.AIChangeOperationCreate:
			operation = domain.CodebookChangeOperationCreate
		case domain.AIChangeOperationUpdate:
			operation = domain.CodebookChangeOperationUpdate
		case domain.AIChangeOperationRename:
			operation = domain.CodebookChangeOperationRename
		case domain.AIChangeOperationDelete:
			operation = domain.CodebookChangeOperationDelete
		default:
			return nil, fmt.Errorf("unsupported AI change operation: %s", item.Operation)
		}
		changes = append(changes, domain.CodebookProjectChange{
			Operation: operation, Path: item.Path, SourcePath: item.SourcePath, NodeID: item.NodeID,
			ExpectedCurrentVersionID: item.BaseVersionID, ExpectedHash: item.BaseHash,
			Code: item.Code, Message: changeSetVersionMessage(changeSet),
			SourceKey:         fmt.Sprintf("ai-change-set:%d:item:%d", changeSet.ID, index+1),
			CleanupObjectKeys: append([]string(nil), item.CleanupObjectKeys...),
		})
	}
	return changes, nil
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
		applied[index].CleanupObjectKeys = append([]string(nil), result.CleanupObjectKeys...)
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
			Path: item.Path, SourcePath: item.SourcePath,
			Operation: aiToCodebookOperation(item.Operation),
			NodeID:    item.NodeID, VersionID: item.AppliedVersionID,
			CleanupObjectKeys: append([]string(nil), item.CleanupObjectKeys...),
		})
	}
	return result
}

func aiToCodebookOperation(operation domain.AIChangeOperation) domain.CodebookChangeOperation {
	return domain.CodebookChangeOperation(operation)
}

func hasCleanupObjects(items []domain.AIChangeItem) bool {
	for _, item := range items {
		if item.Operation == domain.AIChangeOperationDelete && len(item.CleanupObjectKeys) > 0 {
			return true
		}
	}
	return false
}

func cleanupObjectKeys(items []domain.AIChangeItem) []string {
	keys := make([]string, 0)
	seen := make(map[string]struct{})
	for _, item := range items {
		if item.Operation != domain.AIChangeOperationDelete {
			continue
		}
		for _, key := range item.CleanupObjectKeys {
			if key == "" {
				continue
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			keys = append(keys, key)
		}
	}
	return keys
}

func (s *service) completePendingCleanup(ctx context.Context,
	changeSet domain.AIChangeSet) ([]domain.CodebookProjectChangeResult, error) {
	results := appliedChangeSetResults(changeSet.Items)
	if s.files == nil {
		return results, fmt.Errorf("AI change set was applied, but object cleanup is unavailable")
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
	defer cancel()
	if err := s.files.CleanupObjects(cleanupCtx, cleanupObjectKeys(changeSet.Items)); err != nil {
		return results, fmt.Errorf("AI change set was applied, but object cleanup failed: %w", err)
	}
	completeCtx, completeCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer completeCancel()
	if err := s.repo.CompleteChangeSetCleanup(completeCtx, changeSet.ID); err != nil {
		latest, latestErr := s.getChangeSet(completeCtx, changeSet.ID)
		if latestErr == nil && latest.Status == domain.AIChangeSetStatusApplied {
			return results, nil
		}
		return results, fmt.Errorf("object cleanup succeeded, but completing AI change set failed: %w", err)
	}
	return results, nil
}
