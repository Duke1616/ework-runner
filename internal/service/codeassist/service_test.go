package codeassist

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"testing"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/etask/internal/ai"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository"
	codeassistRecipe "github.com/Duke1616/etask/internal/service/codeassist/recipe"
	codebookSvc "github.com/Duke1616/etask/internal/service/codebook"
	codebookmocks "github.com/Duke1616/etask/internal/service/codebook/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func hashContent(content string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
}

type codeAssistRepositoryStub struct {
	repository.CodeAssistRepository
	conversation  domain.AIConversation
	messages      []domain.AIMessage
	claimed       bool
	claimToken    string
	releaseToken  string
	failed        domain.AIMessage
	failStatus    domain.AIMessageStatus
	changeSet     domain.AIChangeSet
	changeClaimed bool
}

func (r *codeAssistRepositoryStub) CreateChangeSet(_ context.Context,
	changeSet domain.AIChangeSet) (domain.AIChangeSet, error) {
	changeSet.ID = 41
	r.changeSet = changeSet
	return changeSet, nil
}

func (r *codeAssistRepositoryStub) GetChangeSetByID(context.Context,
	int64) (domain.AIChangeSet, error) {
	return r.changeSet, nil
}

func (r *codeAssistRepositoryStub) ClaimChangeSet(context.Context, int64) error {
	r.changeClaimed = true
	return nil
}

func (r *codeAssistRepositoryStub) ReleaseChangeSet(context.Context, int64) error {
	r.changeClaimed = false
	return nil
}

func (r *codeAssistRepositoryStub) MarkChangeSetApplied(_ context.Context, _ int64,
	items []domain.AIChangeItem) error {
	r.changeClaimed = false
	r.changeSet.Status = domain.AIChangeSetStatusApplied
	r.changeSet.Items = items
	return nil
}

func (r *codeAssistRepositoryStub) GetConversationByID(context.Context,
	int64) (domain.AIConversation, error) {
	return r.conversation, nil
}

func (r *codeAssistRepositoryStub) ClaimConversation(_ context.Context, _ int64, _ int64,
	runToken string) error {
	r.claimed = true
	r.claimToken = runToken
	return nil
}

func (r *codeAssistRepositoryStub) ReleaseConversation(_ context.Context, _ int64,
	runToken string) error {
	r.claimed = false
	r.releaseToken = runToken
	return nil
}

func (r *codeAssistRepositoryStub) ListMessages(context.Context, int64,
	int) ([]domain.AIMessage, error) {
	return append([]domain.AIMessage(nil), r.messages...), nil
}

func (r *codeAssistRepositoryStub) CreateMessage(_ context.Context,
	message domain.AIMessage) (domain.AIMessage, error) {
	message.ID = int64(len(r.messages) + 1)
	r.messages = append(r.messages, message)
	return message, nil
}

func (r *codeAssistRepositoryStub) CompleteMessage(_ context.Context, message domain.AIMessage) error {
	message.Status = domain.AIMessageStatusCompleted
	for index := range r.messages {
		if r.messages[index].ID == message.ID {
			r.messages[index] = message
		}
	}
	return nil
}

func (r *codeAssistRepositoryStub) FailMessage(_ context.Context, message domain.AIMessage,
	status domain.AIMessageStatus, _ string) error {
	r.failed = message
	r.failStatus = status
	return nil
}

type workspaceStub struct{ codebookSvc.WorkspaceService }

func (workspaceStub) Tree(context.Context, int64) ([]domain.WorkspaceNode, error) {
	return []domain.WorkspaceNode{{
		Name: "system", RuntimePath: "system", Layer: domain.WorkspaceLayerSystem,
	}}, nil
}

type providerStub struct {
	events      []ai.Event
	lastRequest *ai.Request
}

type queuedProviderStub struct {
	turns    [][]ai.Event
	requests []ai.Request
}

func (*queuedProviderStub) Name() string  { return "fake" }
func (*queuedProviderStub) Model() string { return "fake-code-model" }
func (p *queuedProviderStub) Stream(_ context.Context, request ai.Request) (ai.Stream, error) {
	p.requests = append(p.requests, request)
	if len(p.turns) == 0 {
		return nil, fmt.Errorf("unexpected model turn")
	}
	events := p.turns[0]
	p.turns = p.turns[1:]
	return &streamStub{events: events}, nil
}

func (providerStub) Name() string  { return "fake" }
func (providerStub) Model() string { return "fake-code-model" }
func (p providerStub) Stream(_ context.Context, request ai.Request) (ai.Stream, error) {
	if p.lastRequest != nil {
		*p.lastRequest = request
	}
	return &streamStub{events: p.events}, nil
}

type streamStub struct {
	events  []ai.Event
	current int
}

func (s *streamStub) Next() bool {
	if s.current >= len(s.events) {
		return false
	}
	s.current++
	return true
}
func (s *streamStub) Current() ai.Event { return s.events[s.current-1] }
func (s *streamStub) Err() error        { return nil }
func (s *streamStub) Close() error      { return nil }

func TestServiceChatCreatesSingleFileChangeSet(t *testing.T) {
	controller := gomock.NewController(t)
	codebooks := codebookmocks.NewMockService(controller)
	repo := &codeAssistRepositoryStub{conversation: domain.AIConversation{
		ID: 1, UserID: 2, ProjectID: 3, Status: domain.AIConversationStatusActive,
	}}
	baseCode := "import sys\nprint(sys.argv[1])\n"
	baseHash := hashContent(baseCode)
	codebooks.EXPECT().GetProjectByID(gomock.Any(), int64(3)).Return(domain.CodebookProject{
		ID: 3, SourceRevision: 7,
	}, nil)
	codebooks.EXPECT().GetByID(gomock.Any(), int64(10)).Times(2).Return(domain.Codebook{
		ID: 10, ProjectID: 3, Name: "task.py", Kind: domain.CodebookKindFile,
		Code: baseCode, CurrentVersionID: 20,
	}, nil)
	codebooks.EXPECT().GetVersionByID(gomock.Any(), int64(20)).Times(2).Return(domain.CodebookVersion{
		ID: 20, NodeID: 10, Code: baseCode, Hash: baseHash,
	}, nil)
	var modelRequest ai.Request
	provider := providerStub{lastRequest: &modelRequest, events: []ai.Event{
		{Type: ai.EventTypeTextDelta, Text: "已经生成修复建议。"},
		{Type: ai.EventTypeToolCallStarted},
		{Type: ai.EventTypeToolCall, ToolCall: &ai.ToolCall{
			Name:      proposeCurrentFileToolName,
			Arguments: `{"summary":"升级运行协议","content":"import json\nimport os\nwith open(os.environ['ETASK_ARGS_FILE']) as f:\n    print(json.load(f))\n"}`,
		}},
		{Type: ai.EventTypeCompleted,
			Usage: ai.Usage{InputTokens: 10, OutputTokens: 20}},
	}}
	workspace := workspaceTreeStub{nodes: []domain.WorkspaceNode{{
		Kind: domain.CodebookKindDirectory, Layer: domain.WorkspaceLayerProject,
		Children: []domain.WorkspaceNode{{
			RuntimePath: "task.py", SourceID: 10, Kind: domain.CodebookKindFile,
			Layer: domain.WorkspaceLayerProject,
		}},
	}}}
	service := NewService(repo, codebooks, workspace, provider, codeassistRecipe.NewCatalog())
	ctx := ctxutil.WithTenantID(t.Context(), 1)
	ctx = ctxutil.WithUserID(ctx, 2)
	events := make([]StreamEvent, 0)

	err := service.Chat(ctx, domain.AIChatRequest{
		ConversationID: 1, Content: "升级当前脚本",
		Context: domain.AIChatContext{
			NodeID: 10, BaseVersionID: 20, EditorCode: baseCode,
		},
	}, func(event StreamEvent) error {
		events = append(events, event)
		return nil
	})

	require.NoError(t, err)
	require.False(t, repo.claimed)
	require.Equal(t, int64(41), repo.changeSet.ID)
	require.Equal(t, int64(7), repo.changeSet.BaseRevision)
	require.Equal(t, domain.AIChangeSetStatusValidated, repo.changeSet.Status)
	require.Len(t, repo.changeSet.Items, 1)
	require.Equal(t, "task.py", repo.changeSet.Items[0].Path)
	require.Equal(t, domain.AIChangeOperationUpdate, repo.changeSet.Items[0].Operation)
	require.Len(t, modelRequest.Tools, 1)
	require.Equal(t, "fake", repo.messages[1].Provider)
	require.Equal(t, repo.claimToken, repo.releaseToken)
	require.Equal(t, []StreamEventType{
		StreamEventTypeStarted,
		StreamEventTypeDelta,
		StreamEventTypeProgress,
		StreamEventTypeProgress,
		StreamEventTypeCompleted,
	}, streamEventTypes(events))
}

func TestServiceChatRunsBoundedWorkspaceAgentAndCreatesChangeSet(t *testing.T) {
	controller := gomock.NewController(t)
	codebooks := codebookmocks.NewMockService(controller)
	repo := &codeAssistRepositoryStub{conversation: domain.AIConversation{
		ID: 1, UserID: 2, ProjectID: 3, Status: domain.AIConversationStatusActive,
	}}
	baseCode := "---\n- hosts: all\n  roles:\n    - common\n"
	baseHash := hashContent(baseCode)
	codebooks.EXPECT().GetProjectByID(gomock.Any(), int64(3)).Return(domain.CodebookProject{
		ID: 3, SourceRevision: 7,
	}, nil)
	codebooks.EXPECT().GetByID(gomock.Any(), int64(10)).Times(2).Return(domain.Codebook{
		ID: 10, ProjectID: 3, Name: "site.yml", Kind: domain.CodebookKindFile,
		Code: baseCode, StorageType: domain.CodebookContentInline, CurrentVersionID: 20,
	}, nil)
	codebooks.EXPECT().GetVersionByID(gomock.Any(), int64(20)).Times(2).
		Return(domain.CodebookVersion{ID: 20, NodeID: 10, Hash: baseHash}, nil)
	workspace := workspaceTreeStub{nodes: []domain.WorkspaceNode{{
		Name: "project", Kind: domain.CodebookKindDirectory,
		Layer: domain.WorkspaceLayerProject, Children: []domain.WorkspaceNode{{
			Name: "site.yml", RuntimePath: "site.yml", SourceID: 10,
			ProjectID: 3, Kind: domain.CodebookKindFile, Layer: domain.WorkspaceLayerProject,
		}},
	}}}
	provider := &queuedProviderStub{turns: [][]ai.Event{
		{
			{Type: ai.EventTypeToolCall, ToolCall: &ai.ToolCall{
				Name: readWorkspaceFilesToolName, Arguments: `{"paths":["site.yml"]}`,
			}},
			{Type: ai.EventTypeCompleted, Usage: ai.Usage{InputTokens: 10, OutputTokens: 2}},
		},
		{
			{Type: ai.EventTypeTextDelta, Text: "已生成 nginx role。"},
			{Type: ai.EventTypeToolCall, ToolCall: &ai.ToolCall{
				Name: proposeChangeSetToolName,
				Arguments: `{"summary":"新增 nginx role","changes":[` +
					`{"operation":"update","path":"site.yml","content":"---\n- hosts: all\n  roles:\n    - common\n    - nginx\n"},` +
					`{"operation":"create","path":"roles/nginx/tasks/main.yml","content":"---\n- name: Install nginx\n  ansible.builtin.package:\n    name: nginx\n    state: present\n"}]}`,
			}},
			{Type: ai.EventTypeCompleted, Usage: ai.Usage{InputTokens: 30, OutputTokens: 20}},
		},
	}}
	service := NewService(repo, codebooks, workspace, provider, codeassistRecipe.NewCatalog())
	ctx := ctxutil.WithUserID(ctxutil.WithTenantID(t.Context(), 1), 2)
	events := make([]StreamEvent, 0)

	err := service.Chat(ctx, domain.AIChatRequest{
		ConversationID: 1, RecipeID: codeassistRecipe.AnsibleProjectID,
		Content: "增加一个 nginx role，并接入 site playbook",
	}, func(event StreamEvent) error {
		events = append(events, event)
		return nil
	})

	require.NoError(t, err)
	require.Len(t, provider.requests, 2)
	require.Len(t, provider.requests[0].Tools, 2)
	require.Contains(t, provider.requests[0].Instructions, "处理当前 Ansible 项目")
	require.Contains(t, provider.requests[1].Input, `"content":"---\n- hosts: all`)
	require.Equal(t, int64(41), repo.changeSet.ID)
	require.Equal(t, int64(7), repo.changeSet.BaseRevision)
	require.Equal(t, domain.AIChangeSetStatusValidated, repo.changeSet.Status)
	require.Len(t, repo.changeSet.Items, 2)
	require.Equal(t, domain.AIChangeOperationUpdate, repo.changeSet.Items[0].Operation)
	require.Equal(t, int64(20), repo.changeSet.Items[0].BaseVersionID)
	require.Equal(t, domain.AIChangeOperationCreate, repo.changeSet.Items[1].Operation)
	require.Equal(t, "yaml", repo.changeSet.Items[1].Language)
	require.Equal(t, ai.Usage{InputTokens: 40, OutputTokens: 22},
		events[len(events)-1].Usage)
	require.Equal(t, []StreamEventType{
		StreamEventTypeStarted, StreamEventTypeProgress, StreamEventTypeProgress,
		StreamEventTypeDelta, StreamEventTypeCompleted,
	}, streamEventTypes(events))
}

func TestServiceApplyChangeSetUsesAtomicCodebookOperation(t *testing.T) {
	controller := gomock.NewController(t)
	codebooks := codebookmocks.NewMockService(controller)
	repo := &codeAssistRepositoryStub{
		conversation: domain.AIConversation{ID: 1, UserID: 2, ProjectID: 3},
		changeSet: domain.AIChangeSet{
			ID: 41, ConversationID: 1, ProjectID: 3, BaseRevision: 7,
			Summary: "新增 nginx role", Status: domain.AIChangeSetStatusValidated,
			Items: []domain.AIChangeItem{
				{Operation: domain.AIChangeOperationUpdate, Path: "site.yml",
					NodeID: 10, BaseVersionID: 20, BaseHash: hashContent("old"),
					Language: "yaml", Code: "---\n- hosts: all\n"},
				{Operation: domain.AIChangeOperationCreate,
					Path: "roles/nginx/tasks/main.yml", Language: "yaml",
					Code: "---\n- name: Install nginx\n  ansible.builtin.package:\n    name: nginx\n"},
			},
		},
	}
	codebooks.EXPECT().ApplyProjectChangeSet(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, request domain.CodebookProjectChangeSet) (
			[]domain.CodebookProjectChangeResult, error) {
			require.Equal(t, int64(3), request.ProjectID)
			require.Equal(t, int64(7), request.BaseRevision)
			require.Len(t, request.Changes, 2)
			require.Equal(t, domain.CodebookChangeOperationUpdate, request.Changes[0].Operation)
			require.Equal(t, "ai-change-set:41:item:1", request.Changes[0].SourceKey)
			require.Equal(t, domain.CodebookChangeOperationCreate, request.Changes[1].Operation)
			return []domain.CodebookProjectChangeResult{
				{Path: "site.yml", NodeID: 10, VersionID: 21},
				{Path: "roles/nginx/tasks/main.yml", NodeID: 11, VersionID: 22},
			}, nil
		})
	service := NewService(repo, codebooks, workspaceStub{}, providerStub{},
		codeassistRecipe.NewCatalog())
	ctx := ctxutil.WithUserID(ctxutil.WithTenantID(t.Context(), 1), 2)

	results, err := service.ApplyChangeSet(ctx, 41)

	require.NoError(t, err)
	require.Len(t, results, 2)
	require.False(t, repo.changeClaimed)
	require.Equal(t, domain.AIChangeSetStatusApplied, repo.changeSet.Status)
	require.Equal(t, int64(11), repo.changeSet.Items[1].NodeID)
	require.Equal(t, int64(22), repo.changeSet.Items[1].AppliedVersionID)
}

type workspaceTreeStub struct {
	codebookSvc.WorkspaceService
	nodes []domain.WorkspaceNode
}

func (s workspaceTreeStub) Tree(context.Context, int64) ([]domain.WorkspaceNode, error) {
	return s.nodes, nil
}

func TestServiceChatRejectsMultipleFileChangesBeforePersisting(t *testing.T) {
	controller := gomock.NewController(t)
	codebooks := codebookmocks.NewMockService(controller)
	repo := &codeAssistRepositoryStub{conversation: domain.AIConversation{
		ID: 1, UserID: 2, ProjectID: 3, Status: domain.AIConversationStatusActive,
	}}
	baseCode := "print('old')\n"
	codebooks.EXPECT().GetProjectByID(gomock.Any(), int64(3)).Return(domain.CodebookProject{
		ID: 3, SourceRevision: 7,
	}, nil)
	codebooks.EXPECT().GetByID(gomock.Any(), int64(10)).Return(domain.Codebook{
		ID: 10, ProjectID: 3, Name: "task.py", Kind: domain.CodebookKindFile,
		Code: baseCode, CurrentVersionID: 20,
	}, nil)
	codebooks.EXPECT().GetVersionByID(gomock.Any(), int64(20)).Return(domain.CodebookVersion{
		ID: 20, NodeID: 10, Code: baseCode, Hash: hashContent(baseCode),
	}, nil)
	provider := providerStub{events: []ai.Event{
		{Type: ai.EventTypeToolCall, ToolCall: &ai.ToolCall{
			Name: proposeCurrentFileToolName, Arguments: `{"summary":"first","content":"print(1)\n"}`,
		}},
		{Type: ai.EventTypeToolCall, ToolCall: &ai.ToolCall{
			Name: proposeCurrentFileToolName, Arguments: `{"summary":"second","content":"print(2)\n"}`,
		}},
		{Type: ai.EventTypeCompleted},
	}}
	workspace := workspaceTreeStub{nodes: []domain.WorkspaceNode{{
		Kind: domain.CodebookKindDirectory, Layer: domain.WorkspaceLayerProject,
		Children: []domain.WorkspaceNode{{
			RuntimePath: "task.py", SourceID: 10, Kind: domain.CodebookKindFile,
			Layer: domain.WorkspaceLayerProject,
		}},
	}}}
	service := NewService(repo, codebooks, workspace, provider, codeassistRecipe.NewCatalog())
	ctx := ctxutil.WithUserID(ctxutil.WithTenantID(t.Context(), 1), 2)

	err := service.Chat(ctx, domain.AIChatRequest{
		ConversationID: 1, RecipeID: codeassistRecipe.GeneralID, Content: "生成两个方案",
		Context: domain.AIChatContext{
			NodeID: 10, BaseVersionID: 20, EditorCode: baseCode,
		},
	}, func(StreamEvent) error { return nil })

	require.EqualError(t, err, "AI response contains multiple file changes")
	require.Zero(t, repo.changeSet.ID)
	require.Equal(t, domain.AIMessageStatusFailed, repo.failStatus)
}

func TestServiceChatRejectsEmptyCompletedResponse(t *testing.T) {
	repo := &codeAssistRepositoryStub{conversation: domain.AIConversation{
		ID: 1, UserID: 2, ProjectID: 3, Status: domain.AIConversationStatusActive,
	}}
	provider := providerStub{events: []ai.Event{{
		Type:  ai.EventTypeCompleted,
		Usage: ai.Usage{InputTokens: 943, OutputTokens: 8192},
	}}}
	service := NewService(repo, nil, workspaceStub{}, provider, codeassistRecipe.NewCatalog())
	ctx := ctxutil.WithUserID(ctxutil.WithTenantID(t.Context(), 1), 2)

	err := service.Chat(ctx, domain.AIChatRequest{
		ConversationID: 1, RecipeID: codeassistRecipe.GeneralID, Content: "修改代码",
	}, func(StreamEvent) error { return nil })

	require.EqualError(t, err, "模型未返回可展示的文本或候选变更")
	require.Equal(t, domain.AIMessageStatusFailed, repo.failStatus)
	require.Equal(t, int64(943), repo.failed.InputTokens)
	require.Equal(t, int64(8192), repo.failed.OutputTokens)
}

func TestServiceChatRequiresRecipeFileContext(t *testing.T) {
	repo := &codeAssistRepositoryStub{conversation: domain.AIConversation{
		ID: 1, UserID: 2, ProjectID: 3, Status: domain.AIConversationStatusActive,
	}}
	service := NewService(repo, nil, workspaceStub{}, providerStub{}, codeassistRecipe.NewCatalog())
	ctx := ctxutil.WithUserID(ctxutil.WithTenantID(t.Context(), 1), 2)

	err := service.Chat(ctx, domain.AIChatRequest{
		ConversationID: 1, RecipeID: "codebook.edit", Content: "修改代码",
	}, func(StreamEvent) error { return nil })

	require.ErrorContains(t, err, "AI recipe requires a Codebook file context")
	require.False(t, repo.claimed)
	require.Empty(t, repo.messages)
}

func TestServiceChatSettlesInterruptedMessage(t *testing.T) {
	testCases := []struct {
		name   string
		before func(StreamEvent, context.CancelFunc) error
		after  func(*testing.T, *codeAssistRepositoryStub, error)
	}{
		{
			name: "连接在部分输出后断开",
			before: func(event StreamEvent, cancel context.CancelFunc) error {
				if event.Type != StreamEventTypeDelta {
					return nil
				}
				cancel()
				return context.Canceled
			},
			after: func(t *testing.T, repo *codeAssistRepositoryStub, err error) {
				require.ErrorIs(t, err, context.Canceled)
				require.Equal(t, domain.AIMessageStatusCancelled, repo.failStatus)
				require.Equal(t, "部分回复", repo.failed.Content)
			},
		},
		{
			name: "开始事件写入失败",
			before: func(event StreamEvent, _ context.CancelFunc) error {
				if event.Type == StreamEventTypeStarted {
					return errors.New("SSE writer is unavailable")
				}
				return nil
			},
			after: func(t *testing.T, repo *codeAssistRepositoryStub, err error) {
				require.EqualError(t, err, "SSE writer is unavailable")
				require.Equal(t, domain.AIMessageStatusFailed, repo.failStatus)
				require.Empty(t, repo.failed.Content)
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			repo := &codeAssistRepositoryStub{conversation: domain.AIConversation{
				ID: 1, UserID: 2, ProjectID: 3, Status: domain.AIConversationStatusActive,
			}}
			provider := providerStub{events: []ai.Event{
				{Type: ai.EventTypeTextDelta, Text: "部分回复"},
				{Type: ai.EventTypeCompleted},
			}}
			service := NewService(repo, nil, workspaceStub{}, provider, codeassistRecipe.NewCatalog())
			baseCtx, cancel := context.WithCancel(t.Context())
			ctx := ctxutil.WithUserID(ctxutil.WithTenantID(baseCtx, 1), 2)
			t.Cleanup(cancel)

			err := service.Chat(ctx, domain.AIChatRequest{
				ConversationID: 1, RecipeID: codeassistRecipe.GeneralID, Content: "分析脚本",
			}, func(event StreamEvent) error {
				return testCase.before(event, cancel)
			})

			testCase.after(t, repo, err)
			require.False(t, repo.claimed)
		})
	}
}

func streamEventTypes(events []StreamEvent) []StreamEventType {
	result := make([]StreamEventType, 0, len(events))
	for _, event := range events {
		result = append(result, event.Type)
	}
	return result
}
