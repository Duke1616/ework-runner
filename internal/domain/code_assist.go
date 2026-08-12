package domain

import (
	"fmt"
	"strings"

	"github.com/Duke1616/etask/internal/errs"
)

// AIConversationStatus 表示 AI 会话的运行状态。
type AIConversationStatus string

const (
	AIConversationStatusActive  AIConversationStatus = "ACTIVE"
	AIConversationStatusRunning AIConversationStatus = "RUNNING"
)

// AIMessageRole 表示对话消息角色。
type AIMessageRole string

const (
	AIMessageRoleUser      AIMessageRole = "USER"
	AIMessageRoleAssistant AIMessageRole = "ASSISTANT"
)

// AIMessageStatus 表示消息生成状态。
type AIMessageStatus string

const (
	AIMessageStatusStreaming AIMessageStatus = "STREAMING"
	AIMessageStatusCompleted AIMessageStatus = "COMPLETED"
	AIMessageStatusFailed    AIMessageStatus = "FAILED"
	AIMessageStatusCancelled AIMessageStatus = "CANCELLED"
)

// AIChangeSetStatus 表示候选变更的状态。
type AIChangeSetStatus string

const (
	AIChangeSetStatusDraft          AIChangeSetStatus = "DRAFT"
	AIChangeSetStatusValidated      AIChangeSetStatus = "VALIDATED"
	AIChangeSetStatusApplying       AIChangeSetStatus = "APPLYING"
	AIChangeSetStatusCleanupPending AIChangeSetStatus = "CLEANUP_PENDING"
	AIChangeSetStatusApplied        AIChangeSetStatus = "APPLIED"
)

// AIChangeOperation 表示项目文件变更类型。目录操作和跨目录移动暂不开放给模型。
type AIChangeOperation string

const (
	AIChangeOperationCreate AIChangeOperation = "CREATE"
	AIChangeOperationUpdate AIChangeOperation = "UPDATE"
	AIChangeOperationRename AIChangeOperation = "RENAME"
	AIChangeOperationDelete AIChangeOperation = "DELETE"
)

// AIDiagnosticSeverity 表示候选代码诊断级别。
type AIDiagnosticSeverity string

const (
	AIDiagnosticSeverityError   AIDiagnosticSeverity = "ERROR"
	AIDiagnosticSeverityWarning AIDiagnosticSeverity = "WARNING"
)

// AIConversation 是一个项目内的持久化 AI 对话。
type AIConversation struct {
	ID        int64
	TenantID  int64
	UserID    int64
	ProjectID int64
	Title     string
	Provider  string
	Model     string
	Status    AIConversationStatus
	CTime     int64
	UTime     int64
}

// ValidateCreate 校验创建会话所需的信息。
func (c *AIConversation) ValidateCreate() error {
	c.Title = strings.TrimSpace(c.Title)
	if c.UserID <= 0 || c.ProjectID <= 0 {
		return fmt.Errorf("%w: invalid AI conversation owner or project", errs.ErrInvalidParameter)
	}
	if c.Title == "" {
		c.Title = "新对话"
	}
	if len(c.Title) > 128 {
		return fmt.Errorf("%w: AI conversation title is too long", errs.ErrInvalidParameter)
	}
	return nil
}

// AIMessage 是会话中的一条用户或模型消息。
type AIMessage struct {
	ID             int64
	TenantID       int64
	ConversationID int64
	Role           AIMessageRole
	Content        string
	Status         AIMessageStatus
	Provider       string
	Model          string
	ProfileID      string
	ProfileVersion string
	InputTokens    int64
	OutputTokens   int64
	LatencyMillis  int64
	ErrorMessage   string
	CTime          int64
	UTime          int64
}

// AIDiagnostic 描述候选代码的确定性检查结果。
type AIDiagnostic struct {
	Severity AIDiagnosticSeverity `json:"severity"`
	Code     string               `json:"code"`
	Message  string               `json:"message"`
}

// AIChangeSet 是模型针对一个或多个 Codebook 文件生成的候选变更。
type AIChangeSet struct {
	ID             int64
	TenantID       int64
	ConversationID int64
	MessageID      int64
	ProjectID      int64
	BaseRevision   int64
	Summary        string
	Status         AIChangeSetStatus
	Items          []AIChangeItem
	CTime          int64
	UTime          int64
}

// AIChangeItem 是候选变更中的一个文件操作，始终随 ChangeSet 整体持久化。
type AIChangeItem struct {
	Operation         AIChangeOperation `json:"operation"`
	Path              string            `json:"path"`
	SourcePath        string            `json:"source_path,omitempty"`
	NodeID            int64             `json:"node_id,omitempty"`
	BaseVersionID     int64             `json:"base_version_id,omitempty"`
	BaseHash          string            `json:"base_hash,omitempty"`
	Language          string            `json:"language"`
	Code              string            `json:"code"`
	Diagnostics       []AIDiagnostic    `json:"diagnostics,omitempty"`
	AppliedVersionID  int64             `json:"applied_version_id,omitempty"`
	CleanupObjectKeys []string          `json:"cleanup_object_keys,omitempty"`
}

// Prepare 规范化并校验项目级候选变更的持久化前置条件。
func (s *AIChangeSet) Prepare() error {
	s.Summary = strings.TrimSpace(s.Summary)
	if s.ConversationID <= 0 || s.MessageID <= 0 || s.ProjectID <= 0 || s.BaseRevision < 0 {
		return fmt.Errorf("invalid AI change set context")
	}
	if len(s.Items) == 0 {
		return fmt.Errorf("AI change set is empty")
	}
	if s.Status == "" {
		s.Status = AIChangeSetStatusDraft
	}
	seen := make(map[string]struct{}, len(s.Items))
	touchedNodes := make(map[int64]struct{}, len(s.Items))
	for index := range s.Items {
		item := &s.Items[index]
		item.Path = strings.TrimSpace(item.Path)
		item.SourcePath = strings.TrimSpace(item.SourcePath)
		item.Language = strings.ToLower(strings.TrimSpace(item.Language))
		key := strings.ToLower(item.Path)
		if item.Path == "" {
			return fmt.Errorf("AI change item path is empty")
		}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("AI change set contains duplicate path: %s", item.Path)
		}
		seen[key] = struct{}{}
		switch item.Operation {
		case AIChangeOperationCreate:
			if strings.TrimSpace(item.Code) == "" || item.NodeID != 0 ||
				item.BaseVersionID != 0 || item.BaseHash != "" {
				return fmt.Errorf("AI create item contains an existing file context: %s", item.Path)
			}
		case AIChangeOperationUpdate:
			if strings.TrimSpace(item.Code) == "" || item.NodeID <= 0 ||
				item.BaseVersionID <= 0 || item.BaseHash == "" {
				return fmt.Errorf("AI update item is missing its base version: %s", item.Path)
			}
		case AIChangeOperationRename:
			if item.SourcePath == "" || strings.EqualFold(item.SourcePath, item.Path) ||
				item.Code != "" || item.NodeID <= 0 || item.BaseVersionID <= 0 || item.BaseHash == "" {
				return fmt.Errorf("AI rename item is missing its source or base version: %s", item.Path)
			}
		case AIChangeOperationDelete:
			if item.SourcePath != "" || item.Code != "" || item.NodeID <= 0 ||
				item.BaseVersionID <= 0 || item.BaseHash == "" {
				return fmt.Errorf("AI delete item is missing its base version: %s", item.Path)
			}
		default:
			return fmt.Errorf("unsupported AI change operation: %s", item.Operation)
		}
		if item.NodeID > 0 {
			if _, exists := touchedNodes[item.NodeID]; exists {
				return fmt.Errorf("AI change set touches node more than once: %d", item.NodeID)
			}
			touchedNodes[item.NodeID] = struct{}{}
		}
	}
	return nil
}

// AIChatContext 描述一次对话附带的编辑器上下文。
type AIChatContext struct {
	NodeID        int64
	BaseVersionID int64
	EditorCode    string
}

// AIChatRequest 描述用户发送的一条 AI 消息。
type AIChatRequest struct {
	ConversationID int64
	ProfileID      string
	Content        string
	Context        AIChatContext
}
