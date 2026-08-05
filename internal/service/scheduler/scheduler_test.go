package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/service/task"
	"github.com/gotomicro/ego/core/elog"
	"github.com/stretchr/testify/require"
)

func TestSchedulerScheduleOnce(t *testing.T) {
	testCases := []struct {
		name         string
		tenantID     int64
		wantDispatch int
	}{
		{name: "路由成功后派发任务", tenantID: 20, wantDispatch: 1},
		{name: "系统任务不注入租户", wantDispatch: 1},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			dispatcher := &dispatcherStub{}
			scheduler := &Scheduler{
				dispatcher: dispatcher,
				ctx:        context.Background(),
				logger:     elog.DefaultLogger.With(elog.FieldComponentName("scheduler.test")),
			}
			err := scheduler.scheduleOnce(domain.Task{ID: 10, TenantID: testCase.tenantID})
			if err != nil {
				t.Fatalf("scheduleOnce() 返回意外错误: %v", err)
			}
			if dispatcher.calls != testCase.wantDispatch {
				t.Fatalf("Dispatcher.Run() 调用次数 = %d, 期望 %d", dispatcher.calls, testCase.wantDispatch)
			}
			if dispatcher.tenantID != testCase.tenantID {
				t.Fatalf("Dispatcher 收到租户 ID = %d, 期望 %d", dispatcher.tenantID, testCase.tenantID)
			}
			if dispatcher.originTenantID != testCase.tenantID {
				t.Fatalf("Dispatcher 收到原始租户 ID = %d, 期望 %d",
					dispatcher.originTenantID, testCase.tenantID)
			}
		})
	}
}

type dispatcherStub struct {
	calls          int
	tenantID       int64
	originTenantID int64
}

func (d *dispatcherStub) Run(ctx context.Context, _ domain.Task) error {
	d.calls++
	d.tenantID = ctxutil.GetTenantID(ctx).Int64()
	d.originTenantID = ctxutil.GetOriginTenantID(ctx).Int64()
	return nil
}
func (d *dispatcherStub) Retry(context.Context, domain.TaskExecution) error      { return nil }
func (d *dispatcherStub) Reschedule(context.Context, domain.TaskExecution) error { return nil }

func TestSchedulerScheduleLoopStopsDuringIdleWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	queried := make(chan struct{}, 1)
	done := make(chan struct{})
	scheduler := &Scheduler{
		taskSvc: &schedulerTaskServiceStub{queried: queried},
		config: Config{
			BatchTimeout:     time.Second,
			ScheduleInterval: time.Hour,
		},
		ctx:    ctx,
		cancel: cancel,
		logger: elog.DefaultLogger.With(elog.FieldComponentName("scheduler.test")),
	}
	go func() {
		defer close(done)
		scheduler.scheduleLoop()
	}()

	select {
	case <-queried:
	case <-time.After(time.Second):
		t.Fatal("调度循环未查询可调度任务")
	}
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("调度循环未在取消后及时退出")
	}
	require.ErrorIs(t, scheduler.ctx.Err(), context.Canceled)
}

type schedulerTaskServiceStub struct {
	task.Service
	queried chan<- struct{}
}

func (s *schedulerTaskServiceStub) SchedulableTasks(context.Context, int64, int) ([]domain.Task, error) {
	select {
	case s.queried <- struct{}{}:
	default:
	}
	return nil, nil
}
