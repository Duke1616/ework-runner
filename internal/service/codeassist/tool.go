package codeassist

import "github.com/Duke1616/etask/internal/ai"

const proposeCurrentFileToolName = "propose_current_file"

const (
	readWorkspaceFilesToolName = "read_workspace_files"
	proposeChangeSetToolName   = "propose_changeset"
)

func currentFileChangeTool() ai.Tool {
	return ai.Tool{
		Name:        proposeCurrentFileToolName,
		Description: "提交完整的候选脚本代码。只有用户明确要求生成、修改或修复代码时才调用。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"summary": map[string]any{"type": "string"},
				"content": map[string]any{"type": "string"},
			},
			"required":             []string{"summary", "content"},
			"additionalProperties": false,
		},
	}
}

func workspaceAgentTools() []ai.Tool {
	return []ai.Tool{
		readWorkspaceFilesTool(),
		proposeChangeSetTool(),
	}
}

func readWorkspaceFilesTool() ai.Tool {
	return ai.Tool{
		Name:        readWorkspaceFilesToolName,
		Description: "读取当前工作区中与任务相关的文本文件。只读取确实需要的文件；可以分多轮继续读取。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"paths": map[string]any{
					"type": "array", "items": map[string]any{"type": "string"},
					"minItems": 1, "maxItems": 12,
				},
			},
			"required": []string{"paths"}, "additionalProperties": false,
		},
	}
}

func proposeChangeSetTool() ai.Tool {
	return ai.Tool{
		Name:        proposeChangeSetToolName,
		Description: "提交一个完整、可审阅的项目文件变更集。只有用户明确要求生成、修改或修复项目时才调用。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"summary": map[string]any{"type": "string"},
				"changes": map[string]any{
					"type": "array", "minItems": 1, "maxItems": 30,
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"operation": map[string]any{
								"type": "string", "enum": []string{"create", "update"},
							},
							"path":    map[string]any{"type": "string"},
							"content": map[string]any{"type": "string"},
						},
						"required":             []string{"operation", "path", "content"},
						"additionalProperties": false,
					},
				},
			},
			"required": []string{"summary", "changes"}, "additionalProperties": false,
		},
	}
}
