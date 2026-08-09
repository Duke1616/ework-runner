package codeassist

import (
	"encoding/json"
	"strings"

	"github.com/Duke1616/etask/internal/domain"
)

const maxHistoryMessageBytes = 16 * 1024

func buildPrompt(history []domain.AIMessage, userContent string, prepared preparedContext) string {
	type historyItem struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	payload := struct {
		History       []historyItem    `json:"conversation_history"`
		CurrentFile   map[string]any   `json:"current_file,omitempty"`
		WorkspacePath []map[string]any `json:"workspace_paths,omitempty"`
		UserRequest   string           `json:"user_request"`
	}{
		History: make([]historyItem, 0, len(history)), UserRequest: userContent,
	}
	for _, message := range history {
		if message.Status != domain.AIMessageStatusCompleted || strings.TrimSpace(message.Content) == "" {
			continue
		}
		payload.History = append(payload.History, historyItem{
			Role: string(message.Role), Content: compactHistoryContent(message.Content),
		})
	}
	if prepared.node.ID > 0 {
		payload.CurrentFile = map[string]any{
			"name": prepared.node.Name, "node_id": prepared.node.ID,
			"base_version_id": prepared.base.ID, "code": prepared.editorCode,
		}
	}
	if len(prepared.workspaceTree) > 0 {
		payload.WorkspacePath = workspacePaths(prepared.workspaceTree, 500)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return `{"user_request":"Unable to encode Codebook context."}`
	}
	return string(encoded)
}

func compactHistoryContent(content string) string {
	if len(content) <= maxHistoryMessageBytes {
		return content
	}
	tail := strings.ToValidUTF8(content[len(content)-maxHistoryMessageBytes:], "")
	return "[较早内容已截断]\n" + tail
}

func workspacePaths(nodes []domain.WorkspaceNode, limit int) []map[string]any {
	result := make([]map[string]any, 0, min(limit, len(nodes)))
	appendWorkspacePaths(&result, nodes, limit)
	return result
}

func appendWorkspacePaths(result *[]map[string]any, nodes []domain.WorkspaceNode, limit int) {
	for _, node := range nodes {
		if len(*result) >= limit {
			return
		}
		*result = append(*result, map[string]any{
			"path": node.RuntimePath, "layer": node.Layer, "kind": node.Kind,
		})
		appendWorkspacePaths(result, node.Children, limit)
	}
}
