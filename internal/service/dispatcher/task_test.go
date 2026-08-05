package dispatcher_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	acquirermocks "github.com/Duke1616/etask/internal/service/acquirer/mocks"
	"github.com/Duke1616/etask/internal/service/dispatcher"
	dispatchermocks "github.com/Duke1616/etask/internal/service/dispatcher/mocks"
	"go.uber.org/mock/gomock"
)

func TestTaskDispatcherRunPlansAfterAcquire(t *testing.T) {
	acquireErr := errors.New("抢占失败")
	planningErr := errors.New("规划失败")
	testCases := []struct {
		name         string
		acquireErr   error
		planningErr  error
		wantPlanned  bool
		wantReleased bool
	}{
		{name: "路由使用抢占后的任务", planningErr: planningErr, wantPlanned: true, wantReleased: true},
		{name: "抢占失败时不规划路由", acquireErr: acquireErr},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			acquired := domain.Task{ID: 10, Version: 8, ExecMode: domain.ExecModePush}
			acquirer := acquirermocks.NewMockTaskAcquirer(ctrl)
			routes := dispatchermocks.NewMockRoutePlanner(ctrl)
			acquirer.EXPECT().Acquire(gomock.Any(), int64(10), int64(7), "scheduler-1").
				Return(acquired, testCase.acquireErr)
			if testCase.wantPlanned {
				routes.EXPECT().Plan(gomock.Any(), acquired).
					Return(dispatcher.Route{}, testCase.planningErr)
			}
			if testCase.wantReleased {
				acquirer.EXPECT().Release(gomock.Any(), acquired.ID, "scheduler-1").Return(nil)
			}
			taskDispatcher := dispatcher.NewTaskDispatcher("scheduler-1", nil, acquirer, nil, routes)

			err := taskDispatcher.Run(context.Background(), domain.Task{ID: 10, Version: 7})
			wantErr := testCase.planningErr
			if wantErr == nil {
				wantErr = testCase.acquireErr
			}
			if !errors.Is(err, wantErr) {
				t.Fatalf("Run() 错误 = %v, 期望包含 %v", err, wantErr)
			}
		})
	}
}
