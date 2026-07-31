package task_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/service/task"
	taskmocks "github.com/Duke1616/etask/internal/service/task/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestAppendExecutionLogs(t *testing.T) {
	t.Run("合并日志后写入", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		logs := taskmocks.NewMockLogService(ctrl)
		logs.EXPECT().AddLog(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, log domain.TaskExecutionLog) (domain.TaskExecutionLog, error) {
				require.Equal(t, int64(10), log.ExecutionID)
				require.Equal(t, int64(20), log.TaskID)
				require.Equal(t, "first\nsecond", log.Content)
				return log, nil
			})
		svc := task.NewExecutionService("", nil, nil, logs, nil, nil, nil, nil, nil, nil)

		err := svc.AppendExecutionLogs(context.Background(), 10, 20, []string{"first", "second"})
		require.NoError(t, err)
	})

	t.Run("透传日志持久化错误", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		logs := taskmocks.NewMockLogService(ctrl)
		logs.EXPECT().AddLog(gomock.Any(), gomock.Any()).
			Return(domain.TaskExecutionLog{}, errors.New("数据库不可用"))
		svc := task.NewExecutionService("", nil, nil, logs, nil, nil, nil, nil, nil, nil)

		err := svc.AppendExecutionLogs(context.Background(), 10, 20, []string{"third"})
		require.ErrorContains(t, err, "数据库不可用")
	})
}
