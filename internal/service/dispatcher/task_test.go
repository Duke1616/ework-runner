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
	"github.com/stretchr/testify/require"
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
			taskDispatcher := dispatcher.NewTaskDispatcher("scheduler-1", nil, acquirer, nil, routes,
				dispatcher.Config{})

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

func TestTaskDispatcherSkipsExpiredPreemptionWithActiveExecution(t *testing.T) {
	ctrl := gomock.NewController(t)
	executions := taskmocks.NewMockExecutionService(ctrl)
	executions.EXPECT().HasNonTerminalByTaskID(gomock.Any(), int64(10)).Return(true, nil)

	acquirer := acquirermocks.NewMockTaskAcquirer(ctrl)
	acquirer.EXPECT().Acquire(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	d := dispatcher.NewTaskDispatcher("scheduler-1", executions, acquirer, nil, nil, dispatcher.Config{})
	err := d.Run(t.Context(), domain.Task{
		ID: 10, Status: domain.TaskStatusPreempted,
	})
	require.NoError(t, err)
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

	d := dispatcher.NewTaskDispatcher("scheduler-1", executions, nil, remote, nil, dispatcher.Config{})
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

	d := dispatcher.NewTaskDispatcher("scheduler-1", executions, nil, nil, nil, dispatcher.Config{})
	if err := d.Reschedule(t.Context(), execution); err != nil {
		t.Fatalf("Reschedule() returned error: %v", err)
	}
}

func TestTaskDispatcherLimitsConcurrentInvocations(t *testing.T) {
	ctrl := gomock.NewController(t)
	executions := taskmocks.NewMockExecutionService(ctrl)
	remote := invokermocks.NewMockInvoker(ctrl)
	firstRelease := make(chan struct{})
	started := make(chan int64, 2)
	updated := make(chan struct{}, 2)

	executions.EXPECT().UpdateScheduleResult(gomock.Any(), gomock.Any(), gomock.Any(),
		domain.TaskExecutionStatusPrepare, gomock.Any(), int64(0), gomock.Any(), "", "").
		Return(true, nil).Times(2)
	remote.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, execution domain.TaskExecution) (domain.ExecutionState, error) {
			started <- execution.ID
			if execution.ID == 1 {
				<-firstRelease
			}
			return domain.ExecutionState{ID: execution.ID, Status: domain.TaskExecutionStatusRunning}, nil
		}).Times(2)
	executions.EXPECT().UpdateState(gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, domain.ExecutionState) error {
			updated <- struct{}{}
			return nil
		}).Times(2)

	d := dispatcher.NewTaskDispatcher("scheduler-1", executions, nil, remote, nil, dispatcher.Config{
		MaxConcurrentTasks:  1,
		TokenAcquireTimeout: time.Second,
	})
	first := domain.TaskExecution{
		ID: 1, Status: domain.TaskExecutionStatusFailedRetryable,
		Route: domain.ExecutionRoute{DispatchMode: domain.ExecModePush},
	}
	second := domain.TaskExecution{
		ID: 2, Status: domain.TaskExecutionStatusFailedRetryable,
		Route: domain.ExecutionRoute{DispatchMode: domain.ExecModePush},
	}

	if err := d.Retry(t.Context(), first); err != nil {
		t.Fatalf("启动第一个调用失败: %v", err)
	}
	if id := <-started; id != first.ID {
		t.Fatalf("第一个启动的 execution ID = %d, 期望 %d", id, first.ID)
	}
	secondResult := make(chan error, 1)
	go func() {
		secondResult <- d.Retry(t.Context(), second)
	}()

	select {
	case id := <-started:
		t.Fatalf("第一个调用释放令牌前启动了 execution %d", id)
	case <-time.After(50 * time.Millisecond):
	}
	close(firstRelease)

	select {
	case id := <-started:
		if id != second.ID {
			t.Fatalf("第二个启动的 execution ID = %d, 期望 %d", id, second.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("第一个调用结束后，第二个调用未获得执行令牌")
	}
	if err := <-secondResult; err != nil {
		t.Fatalf("启动第二个调用失败: %v", err)
	}
	for range 2 {
		select {
		case <-updated:
		case <-time.After(time.Second):
			t.Fatal("执行状态未及时更新")
		}
	}
}
