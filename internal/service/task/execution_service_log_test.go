package task

import (
	"context"
	"errors"
	"testing"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/etask/internal/domain"
	repomocks "github.com/Duke1616/etask/internal/repository/mocks"
	taskmocks "github.com/Duke1616/etask/internal/service/task/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestExecutionServiceAppendExecutionLogs(t *testing.T) {
	testCases := []struct {
		name        string
		setup       func(t *testing.T, repo *repomocks.MockTaskExecutionRepository, logSvc *taskmocks.MockLogService) (context.Context, int64, int64, []string)
		wantErr     error
		assertCalls func(t *testing.T)
	}{
		{
			name: "空日志切片直接跳过",
			setup: func(t *testing.T, repo *repomocks.MockTaskExecutionRepository, logSvc *taskmocks.MockLogService) (context.Context, int64, int64, []string) {
				return context.Background(), 100, 200, nil
			},
			wantErr: nil,
		},
		{
			name: "非法执行ID直接报错",
			setup: func(t *testing.T, repo *repomocks.MockTaskExecutionRepository, logSvc *taskmocks.MockLogService) (context.Context, int64, int64, []string) {
				return context.Background(), 0, 200, []string{"log line"}
			},
			wantErr: errors.New("执行 ID 非法: 0"),
		},
		{
			name: "正常委托保存日志",
			setup: func(t *testing.T, repo *repomocks.MockTaskExecutionRepository, logSvc *taskmocks.MockLogService) (context.Context, int64, int64, []string) {
				ctx := ctxutil.WithTenantID(context.Background(), 42)
				logSvc.EXPECT().AddLog(ctx, gomock.Any()).
					DoAndReturn(func(callCtx context.Context, log domain.TaskExecutionLog) (domain.TaskExecutionLog, error) {
						require.Equal(t, int64(1001), log.ExecutionID)
						require.Equal(t, int64(2001), log.TaskID)
						require.Equal(t, "line1\nline2", log.Content)
						return log, nil
					})
				return ctx, 1001, 2001, []string{"line1", "line2"}
			},
			wantErr: nil,
		},
		{
			name: "保存日志底层报错正常透传",
			setup: func(t *testing.T, repo *repomocks.MockTaskExecutionRepository, logSvc *taskmocks.MockLogService) (context.Context, int64, int64, []string) {
				ctx := ctxutil.WithTenantID(context.Background(), 42)
				logSvc.EXPECT().AddLog(ctx, gomock.Any()).Return(domain.TaskExecutionLog{}, errors.New("db error"))
				return ctx, 1002, 2002, []string{"single line"}
			},
			wantErr: errors.New("保存任务日志失败: executionID=1002: db error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			repo := repomocks.NewMockTaskExecutionRepository(ctrl)
			logSvc := taskmocks.NewMockLogService(ctrl)
			svc := &executionService{
				repo:   repo,
				logSvc: logSvc,
			}

			ctx, executionID, taskID, logs := tc.setup(t, repo, logSvc)
			err := svc.AppendExecutionLogs(ctx, executionID, taskID, logs)

			if tc.wantErr != nil {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.wantErr.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}
