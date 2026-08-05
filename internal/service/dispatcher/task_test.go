package dispatcher_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Duke1616/etask/internal/domain"
	acquirermocks "github.com/Duke1616/etask/internal/service/acquirer/mocks"
	"github.com/Duke1616/etask/internal/service/dispatcher"
	dispatchermocks "github.com/Duke1616/etask/internal/service/dispatcher/mocks"
	invokermocks "github.com/Duke1616/etask/internal/service/invoker/mocks"
	taskmocks "github.com/Duke1616/etask/internal/service/task/mocks"
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

func TestTaskDispatcherRetryClaimsExecutionBeforeInvoke(t *testing.T) {
	ctrl := gomock.NewController(t)
	executions := taskmocks.NewMockExecutionService(ctrl)
	remote := invokermocks.NewMockInvoker(ctrl)
	done := make(chan struct{})
	execution := domain.TaskExecution{
		ID: 10, Status: domain.TaskExecutionStatusFailedRetryable,
		Task:  domain.Task{ScheduleParams: map[string]string{"attempt": "2"}},
		Route: domain.ExecutionRoute{DispatchMode: domain.ExecModePush},
	}

	firstClaim := executions.EXPECT().UpdateScheduleResult(gomock.Any(), execution.ID,
		[]domain.TaskExecutionStatus{domain.TaskExecutionStatusFailedRetryable},
		domain.TaskExecutionStatusPrepare, execution.RunningProgress, int64(0),
		execution.Task.ScheduleParams, "", "").Return(true, nil)
	remote.EXPECT().Run(gomock.Any(), execution).Return(domain.ExecutionState{
		ID: execution.ID, Status: domain.TaskExecutionStatusRunning,
	}, nil)
	executions.EXPECT().UpdateState(gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, domain.ExecutionState) error {
			close(done)
			return nil
		})
	executions.EXPECT().UpdateScheduleResult(gomock.Any(), execution.ID,
		[]domain.TaskExecutionStatus{domain.TaskExecutionStatusFailedRetryable},
		domain.TaskExecutionStatusPrepare, execution.RunningProgress, int64(0),
		execution.Task.ScheduleParams, "", "").After(firstClaim.Call).Return(false, nil)

	d := dispatcher.NewTaskDispatcher("scheduler-1", executions, nil, remote, nil)
	if err := d.Retry(t.Context(), execution); err != nil {
		t.Fatalf("first Retry() returned error: %v", err)
	}
	if err := d.Retry(t.Context(), execution); err != nil {
		t.Fatalf("second Retry() returned error: %v", err)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("claimed execution was not invoked")
	}
}

func TestTaskDispatcherRescheduleUsesRescheduledStatusAsClaimSource(t *testing.T) {
	ctrl := gomock.NewController(t)
	executions := taskmocks.NewMockExecutionService(ctrl)
	execution := domain.TaskExecution{
		ID: 11, Status: domain.TaskExecutionStatusFailedRescheduled,
		Route: domain.ExecutionRoute{DispatchMode: domain.ExecModePush},
	}
	executions.EXPECT().UpdateScheduleResult(gomock.Any(), execution.ID,
		[]domain.TaskExecutionStatus{domain.TaskExecutionStatusFailedRescheduled},
		domain.TaskExecutionStatusPrepare, execution.RunningProgress, int64(0),
		execution.Task.ScheduleParams, "", "").Return(false, nil)

	d := dispatcher.NewTaskDispatcher("scheduler-1", executions, nil, nil, nil)
	if err := d.Reschedule(t.Context(), execution); err != nil {
		t.Fatalf("Reschedule() returned error: %v", err)
	}
}
