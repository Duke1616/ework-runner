package codeassist

import (
	"context"
	"time"

	"github.com/Duke1616/etask/internal/ai"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository"
	codeassistagent "github.com/Duke1616/etask/internal/service/codeassist/agent"
	codebookSvc "github.com/Duke1616/etask/internal/service/codebook"
)

const (
	maxUserMessageLength = 16 * 1024
	maxEditorCodeLength  = 256 * 1024
	messageHistoryLimit  = 20
	messageSettleTimeout = 10 * time.Second
)

// StreamEventType 表示发送给前端的 AI 对话事件类型。
type StreamEventType string

const (
	StreamEventTypeStarted   StreamEventType = "message.started"
	StreamEventTypeDelta     StreamEventType = "message.delta"
	StreamEventTypeProgress  StreamEventType = "message.progress"
	StreamEventTypeCompleted StreamEventType = "message.completed"
	StreamEventTypeFailed    StreamEventType = "message.failed"
)

// StreamEvent 是 CodeAssist 输出给 Web 层的稳定事件。
type StreamEvent struct {
	Type      StreamEventType
	MessageID int64
	Text      string
	Usage     ai.Usage
	Err       error
}

// EventEmitter 接收一次对话产生的实时事件。
type EventEmitter func(StreamEvent) error

// Service 定义 AI 代码助手业务能力。
type Service interface {
	// CreateConversation 创建项目级 AI 会话。
	CreateConversation(ctx context.Context, projectID int64, title string) (domain.AIConversation, error)
	// ListConversations 查询当前用户在项目下的 AI 会话。
	ListConversations(ctx context.Context, projectID int64) ([]domain.AIConversation, error)
	// ConversationDetail 查询当前用户的会话消息和候选变更。
	ConversationDetail(ctx context.Context, conversationID int64) ([]domain.AIMessage,
		[]domain.AIChangeSet, error)
	// Chat 执行一次流式对话。
	Chat(ctx context.Context, request domain.AIChatRequest, emit EventEmitter) error
	// ApplyChangeSet 原子应用一个或多个文件的候选变更。
	ApplyChangeSet(ctx context.Context, id int64) ([]domain.CodebookProjectChangeResult, error)
}

type service struct {
	repo           repository.CodeAssistRepository
	codebooks      codebookSvc.Service
	workspace      codebookSvc.WorkspaceService
	provider       ai.Provider
	workspaceAgent codeassistagent.Runner
}

// Option 配置 CodeAssist Harness 的可替换组件。
type Option func(*service)

// WithWorkspaceAgent 替换默认的 Eino 工作区 Agent，主要用于扩展和测试。
func WithWorkspaceAgent(agent codeassistagent.Runner) Option {
	return func(s *service) {
		if agent != nil {
			s.workspaceAgent = agent
		}
	}
}

// NewService 创建 AI 代码助手服务。
func NewService(repo repository.CodeAssistRepository, codebooks codebookSvc.Service,
	workspace codebookSvc.WorkspaceService, provider ai.Provider,
	options ...Option) Service {
	result := &service{
		repo: repo, codebooks: codebooks, workspace: workspace, provider: provider,
	}
	result.workspaceAgent = codeassistagent.NewEinoRunner(provider)
	for _, option := range options {
		option(result)
	}
	return result
}
