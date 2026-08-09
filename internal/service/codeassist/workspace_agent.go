package codeassist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/etask/internal/domain"
	codeassistagent "github.com/Duke1616/etask/internal/service/codeassist/agent"
	"github.com/Duke1616/etask/internal/service/codeassist/recipe"
)

const (
	workspaceAgentMaxTurns     = 6
	workspaceAgentMaxFiles     = 30
	workspaceAgentMaxReadBytes = 512 * 1024
	workspaceAgentMaxDuration  = 4 * time.Minute
)

const workspaceAgentGuidance = `你正在受控的工作区 Agent 中运行：
- 需要项目文件内容时调用 read_workspace_files，不要猜测未读取的内容。
- 用户明确要求修改项目时，以 propose_changeset 提交完整变更集并结束。
- 每一轮最多调用一个工具；工具报错时根据错误修正参数。
- 不执行脚本，不直接修改文件，也不声称已经应用变更。`

type workspaceReadBudget struct {
	files map[string]int
	bytes int
}

func (s *service) runWorkspaceAgent(ctx context.Context, conversation domain.AIConversation,
	messageID int64, selectedRecipe recipe.Definition, instructions string,
	history []domain.AIMessage, userContent string, prepared preparedContext,
	emit EventEmitter) (codeassistagent.Result, error) {
	agentCtx, cancel := context.WithTimeout(ctx, workspaceAgentMaxDuration)
	defer cancel()

	budget := workspaceReadBudget{files: make(map[string]int)}
	readFiles := func(toolCtx context.Context, arguments string) (string, error) {
		if err := emit(StreamEvent{Type: StreamEventTypeProgress, MessageID: messageID,
			Text: "正在读取相关项目文件"}); err != nil {
			return "", err
		}
		result, err := s.readWorkspaceFiles(toolCtx, conversation.ProjectID,
			prepared, arguments, &budget)
		if err == nil {
			return string(result), nil
		}
		var toolErr *workspaceToolError
		if !errors.As(err, &toolErr) {
			return "", err
		}
		encoded, encodeErr := json.Marshal(map[string]string{"error": toolErr.Error()})
		if encodeErr != nil {
			return "", fmt.Errorf("encode workspace tool error: %w", encodeErr)
		}
		return string(encoded), nil
	}
	proposeChanges := func(toolCtx context.Context, arguments string) (string, error) {
		if err := emit(StreamEvent{Type: StreamEventTypeProgress, MessageID: messageID,
			Text: "正在校验多文件变更"}); err != nil {
			return "", err
		}
		changeSet, err := s.createWorkspaceChangeSet(toolCtx, conversation, messageID,
			prepared, selectedRecipe, arguments)
		if err != nil {
			return "", err
		}
		if summary := strings.TrimSpace(changeSet.Summary); summary != "" {
			return "已生成多文件项目变更建议：" + summary, nil
		}
		return "已生成多文件项目变更建议。", nil
	}

	return s.workspaceAgent.Run(agentCtx, codeassistagent.Request{
		Instructions: instructions + "\n\n" + workspaceAgentGuidance,
		Input:        buildPrompt(history, userContent, prepared),
		UserKey: fmt.Sprintf("%d:%d", ctxutil.GetTenantID(ctx).Int64(),
			ctxutil.GetUserID(ctx).Int64()),
		MaxTurns: workspaceAgentMaxTurns,
		Tools: []codeassistagent.Tool{
			{Definition: readWorkspaceFilesTool(), Run: readFiles},
			{
				Definition: proposeChangeSetTool(), Run: proposeChanges,
				ReturnDirectly: true,
			},
		},
	})
}
