package codeassist

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
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
	Operation  string `json:"operation"`
	Path       string `json:"path"`
	SourcePath string `json:"source_path"`
	Content    string `json:"content"`
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
		if item.Operation != domain.AIChangeOperationRename {
			totalBytes += len(item.Code)
		}
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
	switch strings.ToLower(strings.TrimSpace(change.Operation)) {
	case "create":
		return buildCreateChangeItem(ctx, change, filePath, workspace)
	case "update":
		return s.buildUpdateChangeItem(ctx, change, filePath, workspace)
	case "rename":
		return s.buildRenameChangeItem(ctx, change, filePath, workspace)
	case "delete":
		return s.buildDeleteChangeItem(ctx, change, filePath, workspace)
	default:
		return domain.AIChangeItem{}, fmt.Errorf("unsupported AI change operation: %s", change.Operation)
	}
}

func buildCreateChangeItem(ctx context.Context, change proposedFileChange, filePath string,
	workspace map[string]domain.WorkspaceNode) (domain.AIChangeItem, error) {
	if err := validateProposedContent(filePath, change.Content); err != nil {
		return domain.AIChangeItem{}, err
	}
	if strings.TrimSpace(change.SourcePath) != "" {
		return domain.AIChangeItem{}, fmt.Errorf("AI create change cannot contain source_path: %s", filePath)
	}
	if _, exists := workspace[strings.ToLower(filePath)]; exists {
		return domain.AIChangeItem{}, fmt.Errorf("AI change set cannot create existing path: %s", filePath)
	}
	if err := validateCreateParent(filePath, workspace); err != nil {
		return domain.AIChangeItem{}, err
	}
	item := domain.AIChangeItem{
		Operation: domain.AIChangeOperationCreate, Path: filePath,
		Language: workspaceFileLanguage(filePath), Code: change.Content,
	}
	item.Diagnostics = validateCandidate(ctx, item.Language, item.Code)
	return item, nil
}

func (s *service) buildUpdateChangeItem(ctx context.Context, change proposedFileChange,
	filePath string, workspace map[string]domain.WorkspaceNode) (domain.AIChangeItem, error) {
	if err := validateProposedContent(filePath, change.Content); err != nil {
		return domain.AIChangeItem{}, err
	}
	if source := strings.TrimSpace(change.SourcePath); source != "" && !strings.EqualFold(source, filePath) {
		return domain.AIChangeItem{}, fmt.Errorf("AI update source_path must match path: %s", filePath)
	}
	node, exists := workspace[strings.ToLower(filePath)]
	if !isWritableProjectFile(node, exists) {
		return domain.AIChangeItem{}, fmt.Errorf("AI change set cannot update path: %s", filePath)
	}
	item := domain.AIChangeItem{
		Operation: domain.AIChangeOperationUpdate, Path: filePath,
		Language: workspaceFileLanguage(filePath), Code: change.Content,
	}
	item, err := s.withBaseVersion(ctx, node, item)
	if err != nil {
		return domain.AIChangeItem{}, err
	}
	item.Diagnostics = validateCandidate(ctx, item.Language, item.Code)
	return item, nil
}

func (s *service) buildRenameChangeItem(ctx context.Context, change proposedFileChange,
	targetPath string, workspace map[string]domain.WorkspaceNode) (domain.AIChangeItem, error) {
	if change.Content != "" {
		return domain.AIChangeItem{}, fmt.Errorf("AI rename change cannot contain content: %s", targetPath)
	}
	sourcePath, err := normalizeWorkspacePath(change.SourcePath)
	if err != nil {
		return domain.AIChangeItem{}, err
	}
	if strings.EqualFold(sourcePath, targetPath) || path.Dir(sourcePath) != path.Dir(targetPath) {
		return domain.AIChangeItem{}, fmt.Errorf("AI change set can only rename a file within its directory: %s", sourcePath)
	}
	if _, exists := workspace[strings.ToLower(targetPath)]; exists {
		return domain.AIChangeItem{}, fmt.Errorf("AI change set cannot rename to existing path: %s", targetPath)
	}
	node, exists := workspace[strings.ToLower(sourcePath)]
	if !isWritableProjectFile(node, exists) {
		return domain.AIChangeItem{}, fmt.Errorf("AI change set cannot rename path: %s", sourcePath)
	}
	return s.withBaseVersion(ctx, node, domain.AIChangeItem{
		Operation: domain.AIChangeOperationRename, SourcePath: sourcePath, Path: targetPath,
		Language: workspaceFileLanguage(targetPath),
	})
}

func (s *service) buildDeleteChangeItem(ctx context.Context, change proposedFileChange,
	filePath string, workspace map[string]domain.WorkspaceNode) (domain.AIChangeItem, error) {
	if change.Content != "" {
		return domain.AIChangeItem{}, fmt.Errorf("AI delete change cannot contain content: %s", filePath)
	}
	if source := strings.TrimSpace(change.SourcePath); source != "" && !strings.EqualFold(source, filePath) {
		return domain.AIChangeItem{}, fmt.Errorf("AI delete source_path must match path: %s", filePath)
	}
	node, exists := workspace[strings.ToLower(filePath)]
	if !isWritableProjectFile(node, exists) {
		return domain.AIChangeItem{}, fmt.Errorf("AI change set cannot delete path: %s", filePath)
	}
	item, err := s.withBaseVersion(ctx, node, domain.AIChangeItem{
		Operation: domain.AIChangeOperationDelete, Path: filePath,
		Language: workspaceFileLanguage(filePath),
	})
	if err != nil {
		return domain.AIChangeItem{}, err
	}
	versions, err := s.codebooks.ListVersions(ctx, node.SourceID)
	if err != nil {
		return domain.AIChangeItem{}, err
	}
	seen := make(map[string]struct{}, len(versions))
	for _, version := range versions {
		if version.StorageType != domain.CodebookContentBlob || version.ObjectKey == "" {
			continue
		}
		if _, exists := seen[version.ObjectKey]; exists {
			continue
		}
		seen[version.ObjectKey] = struct{}{}
		item.CleanupObjectKeys = append(item.CleanupObjectKeys, version.ObjectKey)
	}
	return item, nil
}

func (s *service) withBaseVersion(ctx context.Context, node domain.WorkspaceNode,
	item domain.AIChangeItem) (domain.AIChangeItem, error) {
	file, err := s.codebooks.GetByID(ctx, node.SourceID)
	if err != nil {
		return domain.AIChangeItem{}, err
	}
	version, err := s.codebooks.GetVersionByID(ctx, file.CurrentVersionID)
	if err != nil {
		return domain.AIChangeItem{}, err
	}
	item.NodeID, item.BaseVersionID = file.ID, version.ID
	item.BaseHash = versionContentHash(version.Hash, file.Code)
	return item, nil
}

func isWritableProjectFile(node domain.WorkspaceNode, exists bool) bool {
	return exists && node.Kind == domain.CodebookKindFile &&
		node.Layer == domain.WorkspaceLayerProject && !node.Readonly
}

func validateProposedContent(filePath, content string) error {
	if !utf8.ValidString(content) || strings.ContainsRune(content, '\x00') {
		return fmt.Errorf("AI change set file is not valid text: %s", filePath)
	}
	if len(content) > changeSetMaxFileBytes {
		return fmt.Errorf("AI change set file is too large: %s", filePath)
	}
	return nil
}

func validateCreateParent(filePath string, workspace map[string]domain.WorkspaceNode) error {
	parent := path.Dir(filePath)
	for parent != "." && parent != "" {
		if node, exists := workspace[strings.ToLower(parent)]; exists {
			if node.Kind != domain.CodebookKindDirectory {
				return fmt.Errorf("AI change set parent is not a directory: %s", parent)
			}
			if node.Layer != domain.WorkspaceLayerProject || node.Readonly {
				return fmt.Errorf("AI change set cannot create inside readonly path: %s", parent)
			}
		}
		parent = path.Dir(parent)
	}
	return nil
}

func workspaceFileLanguage(name string) string {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".j2") {
		return "jinja2"
	}
	switch path.Ext(lower) {
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
