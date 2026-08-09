package codeassist

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/Duke1616/etask/internal/domain"
)

type readWorkspaceFilesArguments struct {
	Paths []string `json:"paths"`
}

// workspaceToolError 表示可以安全反馈给模型并允许其修正的工具输入错误。
type workspaceToolError struct{ message string }

func (e *workspaceToolError) Error() string { return e.message }

func workspaceToolErrorf(format string, args ...any) error {
	return &workspaceToolError{message: fmt.Sprintf(format, args...)}
}

type workspaceFileContent struct {
	Path          string `json:"path"`
	Layer         string `json:"layer"`
	Readonly      bool   `json:"readonly"`
	NodeID        int64  `json:"node_id,omitempty"`
	BaseVersionID int64  `json:"base_version_id,omitempty"`
	BaseHash      string `json:"base_hash,omitempty"`
	Content       string `json:"content"`
}

func (s *service) readWorkspaceFiles(ctx context.Context, projectID int64,
	prepared preparedContext, arguments string, budget *workspaceReadBudget) (json.RawMessage, error) {
	var request readWorkspaceFilesArguments
	if err := json.Unmarshal([]byte(arguments), &request); err != nil {
		return nil, workspaceToolErrorf("invalid workspace read request")
	}
	if len(request.Paths) == 0 || len(request.Paths) > 12 {
		return nil, workspaceToolErrorf("workspace read must contain between 1 and 12 paths")
	}
	index := indexWorkspaceNodes(prepared.workspaceTree)
	result := make([]workspaceFileContent, 0, len(request.Paths))
	seenRequest := make(map[string]struct{}, len(request.Paths))
	for _, rawPath := range request.Paths {
		filePath, err := normalizeWorkspacePath(rawPath)
		if err != nil {
			return nil, workspaceToolErrorf("invalid workspace path: %s", rawPath)
		}
		key := strings.ToLower(filePath)
		if _, exists := seenRequest[key]; exists {
			continue
		}
		seenRequest[key] = struct{}{}
		if sensitiveWorkspacePath(filePath) {
			return nil, workspaceToolErrorf("workspace file is not available to AI: %s", filePath)
		}
		node, exists := index[key]
		if !exists || node.Kind != domain.CodebookKindFile {
			return nil, workspaceToolErrorf("workspace file does not exist: %s", filePath)
		}
		content, err := s.loadWorkspaceFile(ctx, projectID, prepared, node)
		if err != nil {
			return nil, err
		}
		if containsSensitiveWorkspaceContent(content.Content) {
			return nil, workspaceToolErrorf("workspace file contains credential material and is not available to AI: %s",
				filePath)
		}
		if _, exists = budget.files[key]; !exists {
			if len(budget.files) >= workspaceAgentMaxFiles {
				return nil, workspaceToolErrorf("workspace agent exceeded the file read limit")
			}
			if budget.bytes+len(content.Content) > workspaceAgentMaxReadBytes {
				return nil, workspaceToolErrorf("workspace agent exceeded the content read limit")
			}
			budget.files[key] = len(content.Content)
			budget.bytes += len(content.Content)
		}
		result = append(result, content)
	}
	encoded, err := json.Marshal(map[string]any{"files": result})
	if err != nil {
		return nil, fmt.Errorf("encode workspace files: %w", err)
	}
	return encoded, nil
}

func sensitiveWorkspacePath(filePath string) bool {
	lower := strings.ToLower(filePath)
	base := path.Base(lower)
	if base == ".env" || strings.HasPrefix(base, ".env.") ||
		base == "id_rsa" || base == "id_ed25519" ||
		strings.Contains(base, "vault_pass") || strings.Contains(base, "vault-password") {
		return true
	}
	return strings.HasSuffix(base, ".pem") || strings.HasSuffix(base, ".key")
}

func containsSensitiveWorkspaceContent(content string) bool {
	lower := strings.ToLower(content)
	patterns := []string{
		"-----begin private key-----", "-----begin rsa private key-----",
		"-----begin openssh private key-----", "ansible_password:",
		"ansible_become_password:",
	}
	for _, pattern := range patterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

func (s *service) loadWorkspaceFile(ctx context.Context, projectID int64,
	prepared preparedContext, node domain.WorkspaceNode) (workspaceFileContent, error) {
	result := workspaceFileContent{
		Path: node.RuntimePath, Layer: string(node.Layer), Readonly: node.Readonly,
	}
	if node.Layer == domain.WorkspaceLayerProject {
		file, err := s.codebooks.GetByID(ctx, node.SourceID)
		if err != nil {
			return workspaceFileContent{}, err
		}
		if !file.IsFile() || file.ProjectID != projectID ||
			file.StorageType == domain.CodebookContentBlob {
			return workspaceFileContent{}, workspaceToolErrorf("workspace file is not editable text: %s",
				node.RuntimePath)
		}
		version, err := s.codebooks.GetVersionByID(ctx, file.CurrentVersionID)
		if err != nil {
			return workspaceFileContent{}, err
		}
		code := file.Code
		if prepared.node.ID == file.ID && prepared.editorCode != "" {
			code = prepared.editorCode
		}
		result.NodeID = file.ID
		result.BaseVersionID = version.ID
		result.BaseHash = versionContentHash(version.Hash, version.Code)
		result.Content = code
		return result, nil
	}
	content, err := s.workspace.ReadArtifactFile(ctx, projectID, node.ReleaseID,
		node.Digest, node.ArtifactPath)
	if err != nil {
		return workspaceFileContent{}, err
	}
	result.Readonly = true
	result.Content = content
	return result, nil
}

func versionContentHash(hash, content string) string {
	if hash != "" {
		return hash
	}
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func indexWorkspaceNodes(nodes []domain.WorkspaceNode) map[string]domain.WorkspaceNode {
	result := make(map[string]domain.WorkspaceNode)
	var walk func([]domain.WorkspaceNode)
	walk = func(values []domain.WorkspaceNode) {
		for _, node := range values {
			if node.RuntimePath != "" {
				result[strings.ToLower(node.RuntimePath)] = node
			}
			walk(node.Children)
		}
	}
	walk(nodes)
	return result
}

func normalizeWorkspacePath(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || strings.HasPrefix(value, "/") {
		return "", fmt.Errorf("invalid workspace path: %q", value)
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("invalid workspace path: %q", value)
	}
	segments := strings.Split(cleaned, "/")
	if len(cleaned) > 512 || len(segments) > 64 {
		return "", fmt.Errorf("workspace path is too long: %q", value)
	}
	for _, segment := range segments {
		if segment == "" || len(segment) > 128 || strings.ContainsRune(segment, '\x00') {
			return "", fmt.Errorf("invalid workspace path segment: %q", segment)
		}
	}
	return cleaned, nil
}
