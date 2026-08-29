package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	executorv1 "github.com/Duke1616/etask/api/proto/gen/etask/executor/v1"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	repositorymocks "github.com/Duke1616/etask/internal/repository/mocks"
	poolSvc "github.com/Duke1616/etask/internal/service/pool"
	taskmocks "github.com/Duke1616/etask/internal/service/task/mocks"
	"github.com/gotomicro/ego/core/elog"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockAuthorizer struct {
	allowed bool
	err     error
}

func (m mockAuthorizer) IsAllowed(_ context.Context, _ poolSvc.CheckBindingRequest) (bool, error) {
	return m.allowed, m.err
}

func TestService_PullTask_Validation(t *testing.T) {
	testCases := []struct {
		name      string
		request   *executorv1.PullTaskRequest
		wantError string
	}{
		{
			name:      "缺少服务名称",
			request:   &executorv1.PullTaskRequest{NodeId: "node-1", Handlers: []string{"shell"}},
			wantError: "服务名称不能为空",
		},
		{
			name:      "缺少节点 ID",
			request:   &executorv1.PullTaskRequest{ServiceName: "executor", Handlers: []string{"shell"}},
			wantError: "节点 ID 不能为空",
		},
		{
			name:      "缺少处理器",
			request:   &executorv1.PullTaskRequest{ServiceName: "executor", NodeId: "node-1"},
			wantError: "至少需要声明一个处理器",
		},
	}

	server := &AgentServer{}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := server.PullTask(t.Context(), tc.request)
			require.ErrorContains(t, err, tc.wantError)
		})
	}
}

func TestService_PullTask_Workflow(t *testing.T) {
	testCases := []struct {
		name      string
		mock      func(ctrl *gomock.Controller) (*repositorymocks.MockTaskExecutionRepository, *taskmocks.MockExecutionService, poolSvc.ExecutionPoolAuthorizer)
		validate  func(t *testing.T, res *executorv1.PullTaskResponse, err error)
		cancelCtx bool
	}{
		{
			name: "成功抢占并交付待拉取任务",
			mock: func(ctrl *gomock.Controller) (*repositorymocks.MockTaskExecutionRepository, *taskmocks.MockExecutionService, poolSvc.ExecutionPoolAuthorizer) {
				repo := repositorymocks.NewMockTaskExecutionRepository(ctrl)
				execSvc := taskmocks.NewMockExecutionService(ctrl)
				auth := mockAuthorizer{allowed: true}

				execution := domain.TaskExecution{
					ID:       88,
					TenantID: 10,
					Task: domain.Task{
						ID:   1,
						Name: "测试任务",
						GrpcConfig: &domain.GrpcConfig{
							ServiceName: "executor-service",
							HandlerName: "shell",
							Params:      map[string]string{"arg1": "val1"},
						},
					},
					Program: domain.NewInlineProgram("echo hello"),
				}

				repo.EXPECT().ClaimPullTask(gomock.Any(), "executor-service", "node-1", []string{"shell"}).
					Return(execution, nil)

				return repo, execSvc, auth
			},
			validate: func(t *testing.T, res *executorv1.PullTaskResponse, err error) {
				require.NoError(t, err)
				require.True(t, res.GetHasTask())
				require.Equal(t, int64(88), res.GetTaskReq().GetEid())
				require.Equal(t, int64(1), res.GetTaskReq().GetTaskId())
				require.Equal(t, "测试任务", res.GetTaskReq().GetTaskName())
				require.Equal(t, "shell", res.GetTaskReq().GetTaskHandlerName())
				require.Equal(t, int64(10), res.GetTaskReq().GetTenantId())
			},
		},
		{
			name: "抢占到任务但执行授权已被撤销_标记失败并返回无任务",
			mock: func(ctrl *gomock.Controller) (*repositorymocks.MockTaskExecutionRepository, *taskmocks.MockExecutionService, poolSvc.ExecutionPoolAuthorizer) {
				repo := repositorymocks.NewMockTaskExecutionRepository(ctrl)
				execSvc := taskmocks.NewMockExecutionService(ctrl)
				auth := mockAuthorizer{allowed: false}

				execution := domain.TaskExecution{
					ID:       99,
					TenantID: 10,
					Task: domain.Task{
						ID:   2,
						Name: "未授权任务",
						GrpcConfig: &domain.GrpcConfig{
							ServiceName: "executor-service",
							HandlerName: "shell",
						},
					},
					Program: domain.NewInlineProgram("echo unauthorized"),
				}

				gomock.InOrder(
					repo.EXPECT().ClaimPullTask(gomock.Any(), "executor-service", "node-1", []string{"shell"}).
						Return(execution, nil),
					execSvc.EXPECT().UpdateState(gomock.Any(), gomock.Cond(func(x any) bool {
						state, ok := x.(domain.ExecutionState)
						return ok && state.ID == 99 && state.Status == domain.TaskExecutionStatusFailed
					})).Return(nil),
					repo.EXPECT().ClaimPullTask(gomock.Any(), "executor-service", "node-1", []string{"shell"}).
						Return(domain.TaskExecution{}, errs.ErrExecutionNotFound).AnyTimes(),
				)

				return repo, execSvc, auth
			},
			cancelCtx: true,
			validate: func(t *testing.T, res *executorv1.PullTaskResponse, err error) {
				require.NoError(t, err)
				require.False(t, res.GetHasTask())
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo, execSvc, auth := tc.mock(ctrl)
			server := &AgentServer{
				execRepo:   repo,
				execSvc:    execSvc,
				authorizer: auth,
				logger:     elog.DefaultLogger,
			}

			ctx := t.Context()
			if tc.cancelCtx {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 50*time.Millisecond)
				defer cancel()
			}

			res, err := server.PullTask(ctx, &executorv1.PullTaskRequest{
				ServiceName: "executor-service",
				NodeId:      "node-1",
				Handlers:    []string{"shell"},
			})
			tc.validate(t, res, err)
		})
	}
}

func TestService_GetTaskExecution(t *testing.T) {
	testCases := []struct {
		name        string
		executionID int64
		mock        func(ctrl *gomock.Controller) *taskmocks.MockExecutionService
		wantStatus  executorv1.ExecutionStatus
		wantErrCode codes.Code
	}{
		{
			name:        "成功获取并准确映射 FAILED_RESCHEDULABLE 状态",
			executionID: 101,
			mock: func(ctrl *gomock.Controller) *taskmocks.MockExecutionService {
				svc := taskmocks.NewMockExecutionService(ctrl)
				svc.EXPECT().FindByID(gomock.Any(), int64(101)).Return(domain.TaskExecution{
					ID:         101,
					Status:     domain.TaskExecutionStatusFailedRescheduled,
					TaskResult: `{"error":"node died"}`,
				}, nil)
				return svc
			},
			wantStatus: executorv1.ExecutionStatus_FAILED_RESCHEDULABLE,
		},
		{
			name:        "非法 ID 参数校验失败",
			executionID: 0,
			mock: func(ctrl *gomock.Controller) *taskmocks.MockExecutionService {
				return taskmocks.NewMockExecutionService(ctrl)
			},
			wantErrCode: codes.InvalidArgument,
		},
		{
			name:        "记录未找到返回 NotFound",
			executionID: 404,
			mock: func(ctrl *gomock.Controller) *taskmocks.MockExecutionService {
				svc := taskmocks.NewMockExecutionService(ctrl)
				svc.EXPECT().FindByID(gomock.Any(), int64(404)).Return(domain.TaskExecution{}, errors.New("not found"))
				return svc
			},
			wantErrCode: codes.NotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			execSvc := tc.mock(ctrl)
			server := &AgentServer{execSvc: execSvc}

			res, err := server.GetTaskExecution(t.Context(), &executorv1.GetTaskExecutionRequest{
				ExecutionId: tc.executionID,
			})

			if tc.wantErrCode != codes.OK {
				require.Error(t, err)
				require.Equal(t, tc.wantErrCode, status.Code(err))
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.executionID, res.GetExecution().GetId())
			require.Equal(t, tc.wantStatus, res.GetExecution().GetStatus())
		})
	}
}

func TestService_GetExecutionLogs(t *testing.T) {
	testCases := []struct {
		name      string
		mock      func(ctrl *gomock.Controller) *taskmocks.MockLogService
		wantCount int
		wantMaxID int64
		wantErr   bool
	}{
		{
			name: "成功获取日志并计算最大 ID",
			mock: func(ctrl *gomock.Controller) *taskmocks.MockLogService {
				mock := taskmocks.NewMockLogService(ctrl)
				mock.EXPECT().GetLogs(gomock.Any(), int64(10), int64(0), 100).Return(
					[]domain.TaskExecutionLog{
						{ID: 1, CTime: 1000, Content: "line 1"},
						{ID: 5, CTime: 1002, Content: "line 2"},
						{ID: 3, CTime: 1001, Content: "line 3"},
					}, int64(3), nil)
				return mock
			},
			wantCount: 3,
			wantMaxID: 5,
		},
		{
			name: "日志服务查询异常返回错误",
			mock: func(ctrl *gomock.Controller) *taskmocks.MockLogService {
				mock := taskmocks.NewMockLogService(ctrl)
				mock.EXPECT().GetLogs(gomock.Any(), int64(10), int64(0), 100).Return(
					nil, int64(0), errors.New("db disconnect"))
				return mock
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			logSvc := tc.mock(ctrl)
			server := &AgentServer{logSvc: logSvc, logger: elog.DefaultLogger}

			res, err := server.GetExecutionLogs(t.Context(), &executorv1.GetExecutionLogsRequest{
				ExecutionId: 10, MinId: 0, Limit: 100,
			})
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, res.GetLogs(), tc.wantCount)
			require.Equal(t, tc.wantMaxID, res.GetMaxId())
		})
	}
}

func TestService_BatchListTaskExecutions(t *testing.T) {
	testCases := []struct {
		name       string
		taskIDs    []int64
		mock       func(ctrl *gomock.Controller) *repositorymocks.MockTaskExecutionRepository
		wantGroups int
	}{
		{
			name:    "全部无效 ID 直接返回空结果无需查库",
			taskIDs: []int64{0, -1, -5},
			mock: func(ctrl *gomock.Controller) *repositorymocks.MockTaskExecutionRepository {
				return repositorymocks.NewMockTaskExecutionRepository(ctrl)
			},
			wantGroups: 0,
		},
		{
			name:    "过滤非法 ID 并按 TaskID 分组聚合",
			taskIDs: []int64{10, 0, 20},
			mock: func(ctrl *gomock.Controller) *repositorymocks.MockTaskExecutionRepository {
				repo := repositorymocks.NewMockTaskExecutionRepository(ctrl)
				repo.EXPECT().FindByTaskIDs(gomock.Any(), []int64{10, 20}).Return([]domain.TaskExecution{
					{ID: 1, Task: domain.Task{ID: 10}},
					{ID: 2, Task: domain.Task{ID: 10}},
					{ID: 3, Task: domain.Task{ID: 20}},
				}, nil)
				return repo
			},
			wantGroups: 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := tc.mock(ctrl)
			server := &AgentServer{execRepo: repo, logger: elog.DefaultLogger}

			res, err := server.BatchListTaskExecutions(t.Context(), &executorv1.BatchListTaskExecutionsRequest{
				TaskIds: tc.taskIDs,
			})
			require.NoError(t, err)
			require.Len(t, res.GetResults(), tc.wantGroups)
		})
	}
}

func TestNormalizeHandlerNames(t *testing.T) {
	testCases := []struct {
		name   string
		values []string
		want   []string
	}{
		{name: "去除空白和重复值", values: []string{" shell ", "", "python", "shell"}, want: []string{"shell", "python"}},
		{name: "空输入", values: nil, want: []string{}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, normalizeHandlerNames(tc.values))
		})
	}
}
