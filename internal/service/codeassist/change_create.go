package codeassist

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/Duke1616/etask/internal/domain"
)

const (
	changeSetMaxFiles     = 30
	changeSetMaxFileBytes = 64 * 1024
	changeSetMaxBytes     = 512 * 1024
)

type changeSetArguments struct {
	Summary string               `json:"summary"`
	Changes []proposedFileChange `json:"changes"`
}

type proposedFileChange struct {
	Operation string `json:"operation"`
	Path      string `json:"path"`
	Content   string `json:"content"`
}

func (s *service) createWorkspaceChangeSet(ctx context.Context,
	conversation domain.AIConversation, messageID int64, prepared preparedContext,
	arguments string) (domain.AIChangeSet, error) {
	var proposal changeSetArguments
	if err := json.Unmarshal([]byte(arguments), &proposal); err != nil {
		return domain.AIChangeSet{}, fmt.Errorf("invalid AI change set: %w", err)
	}
	return s.createChangeSet(ctx, conversation, messageID, prepared, proposal)
}

func (s *service) createChangeSet(ctx context.Context, conversation domain.AIConversation,
	messageID int64, prepared preparedContext, proposal changeSetArguments) (domain.AIChangeSet, error) {
	if !prepared.projectWritable {
		return domain.AIChangeSet{}, fmt.Errorf("AI cannot propose changes for a readonly project")
	}
	if len(proposal.Changes) == 0 || len(proposal.Changes) > changeSetMaxFiles {
		return domain.AIChangeSet{}, fmt.Errorf("AI change set must contain between 1 and %d files",
			changeSetMaxFiles)
	}
	workspace := indexWorkspaceNodes(prepared.workspaceTree)
	items := make([]domain.AIChangeItem, 0, len(proposal.Changes))
	totalBytes, hasErrors := 0, false
	for _, change := range proposal.Changes {
		item, err := s.buildChangeItem(ctx, change, workspace)
		if err != nil {
			return domain.AIChangeSet{}, err
		}
		totalBytes += len(item.Code)
		if totalBytes > changeSetMaxBytes {
			return domain.AIChangeSet{}, fmt.Errorf("AI change set content is too large")
		}
		hasErrors = hasErrors || hasDiagnosticErrors(item.Diagnostics)
		items = append(items, item)
	}

	status := domain.AIChangeSetStatusValidated
	if hasErrors {
		status = domain.AIChangeSetStatusDraft
	}
	changeSet := domain.AIChangeSet{
		ConversationID: conversation.ID, MessageID: messageID, ProjectID: conversation.ProjectID,
		BaseRevision: prepared.project.SourceRevision, Summary: proposal.Summary,
		Status: status, Items: items,
	}
	if err := changeSet.Prepare(); err != nil {
		return domain.AIChangeSet{}, err
	}
	return s.repo.CreateChangeSet(ctx, changeSet)
}

func (s *service) buildChangeItem(ctx context.Context, change proposedFileChange,
	workspace map[string]domain.WorkspaceNode) (domain.AIChangeItem, error) {
	filePath, err := normalizeWorkspacePath(change.Path)
	if err != nil {
		return domain.AIChangeItem{}, err
	}
	if !utf8.ValidString(change.Content) || strings.ContainsRune(change.Content, '\x00') {
		return domain.AIChangeItem{}, fmt.Errorf("AI change set file is not valid text: %s", filePath)
	}
	if len(change.Content) > changeSetMaxFileBytes {
		return domain.AIChangeItem{}, fmt.Errorf("AI change set file is too large: %s", filePath)
	}

	existing, exists := workspace[strings.ToLower(filePath)]
	item := domain.AIChangeItem{
		Path: filePath, Language: workspaceFileLanguage(filePath), Code: change.Content,
	}
	switch strings.ToLower(strings.TrimSpace(change.Operation)) {
	case "create":
		if exists {
			return domain.AIChangeItem{}, fmt.Errorf("AI change set cannot create existing path: %s", filePath)
		}
		if err = validateCreateParent(filePath, workspace); err != nil {
			return domain.AIChangeItem{}, err
		}
		item.Operation = domain.AIChangeOperationCreate
	case "update":
		if !exists || existing.Kind != domain.CodebookKindFile ||
			existing.Layer != domain.WorkspaceLayerProject || existing.Readonly {
			return domain.AIChangeItem{}, fmt.Errorf("AI change set cannot update path: %s", filePath)
		}
		file, getErr := s.codebooks.GetByID(ctx, existing.SourceID)
		if getErr != nil {
			return domain.AIChangeItem{}, getErr
		}
		version, getErr := s.codebooks.GetVersionByID(ctx, file.CurrentVersionID)
		if getErr != nil {
			return domain.AIChangeItem{}, getErr
		}
		item.Operation = domain.AIChangeOperationUpdate
		item.NodeID, item.BaseVersionID = file.ID, version.ID
		item.BaseHash = versionContentHash(version.Hash, file.Code)
	default:
		return domain.AIChangeItem{}, fmt.Errorf("unsupported AI change operation: %s", change.Operation)
	}
	item.Diagnostics = validateCandidate(ctx, item.Language, item.Code)
	return item, nil
}

func validateCreateParent(filePath string, workspace map[string]domain.WorkspaceNode) error {
	parent := filepath.ToSlash(filepath.Dir(filePath))
	for parent != "." && parent != "" {
		if node, exists := workspace[strings.ToLower(parent)]; exists {
			if node.Kind != domain.CodebookKindDirectory {
				return fmt.Errorf("AI change set parent is not a directory: %s", parent)
			}
			if node.Layer != domain.WorkspaceLayerProject || node.Readonly {
				return fmt.Errorf("AI change set cannot create inside readonly path: %s", parent)
			}
		}
		parent = filepath.ToSlash(filepath.Dir(parent))
	}
	return nil
}

func workspaceFileLanguage(name string) string {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".j2") {
		return "jinja2"
	}
	switch filepath.Ext(lower) {
	case ".yml", ".yaml":
		return "yaml"
	case ".py":
		return "python"
	case ".sh", ".bash":
		return "shell"
	case ".ini", ".cfg":
		return "ini"
	default:
		return "text"
	}
}
