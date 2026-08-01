package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	artifactv1 "github.com/Duke1616/etask/api/proto/gen/etask/artifact/v1"
	servicemocks "github.com/Duke1616/etask/internal/agent/service/mocks"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/sdk/executor"
	"go.uber.org/mock/gomock"
)

func TestServiceTerminateCancelsCurrentAndFutureDelivery(t *testing.T) {
	t.Run("终止运行中的 execution", func(t *testing.T) {
		fixture := newServiceFixture(t)
		fixture.handler.block = make(chan struct{})
		done := make(chan error, 1)
		logger := newTaskLoggerMock(t)
		go func() {
			_, err := fixture.service.Receive(context.Background(),
				executionRequest(fixture, "dispatch-1", logger))
			done <- err
		}()
		<-fixture.handler.started
		if !fixture.service.Terminate(fixture.execution.ID, "管理员强制结束") {
			t.Fatal("Terminate() 未找到运行中的 execution")
		}
		close(fixture.handler.block)
		if err := <-done; !errors.Is(err, ErrExecutionTerminated) {
			t.Fatalf("Receive() 错误 = %v, 期望 ErrExecutionTerminated", err)
		}
	})

	t.Run("终止先到时阻止后续执行", func(t *testing.T) {
		fixture := newServiceFixture(t)
		fixture.service.Terminate(fixture.execution.ID, "管理员强制结束")
		_, err := fixture.service.Receive(context.Background(),
			executionRequest(fixture, "dispatch-late", newTaskLoggerMock(t)))
		if !errors.Is(err, ErrExecutionTerminated) {
			t.Fatalf("Receive() 错误 = %v, 期望 ErrExecutionTerminated", err)
		}
		if fixture.handler.runs.Load() != 0 {
			t.Fatalf("终止后的 Handler 仍执行了 %d 次", fixture.handler.runs.Load())
		}
	})
}

func TestServiceReceive(t *testing.T) {
	testCases := []struct {
		name   string
		before func(t *testing.T) (*serviceFixture, func())
		run    func(t *testing.T, fixture *serviceFixture)
		after  func(t *testing.T, fixture *serviceFixture)
	}{
		{
			name: "相同派发并发到达时只执行一次",
			before: func(t *testing.T) (*serviceFixture, func()) {
				fixture := newServiceFixture(t)
				fixture.handler.block = make(chan struct{})
				return fixture, func() {}
			},
			run: func(t *testing.T, fixture *serviceFixture) {
				var wg sync.WaitGroup
				outputs := make([]string, 2)
				errs := make([]error, 2)
				loggers := []*servicemocks.MockTaskLogger{newTaskLoggerMock(t), newTaskLoggerMock(t)}
				for index := range outputs {
					wg.Add(1)
					go func(index int) {
						defer wg.Done()
						output, err := fixture.service.Receive(context.Background(), executionRequest(fixture, "dispatch-1", loggers[index]))
						outputs[index], errs[index] = output.Result, err
					}(index)
				}
				<-fixture.handler.started
				close(fixture.handler.block)
				wg.Wait()
				for index, err := range errs {
					if err != nil {
						t.Fatalf("Receive()[%d] 返回意外错误: %v", index, err)
					}
					if outputs[index] != `{"status":"ok"}` {
						t.Fatalf("Receive()[%d] 结果 = %q", index, outputs[index])
					}
				}
			},
			after: func(t *testing.T, fixture *serviceFixture) {
				if count := fixture.handler.runs.Load(); count != 1 {
					t.Fatalf("Handler 执行次数 = %d, 期望 1", count)
				}
			},
		},
		{
			name: "不同派发可以重试同一个执行记录",
			before: func(t *testing.T) (*serviceFixture, func()) {
				return newServiceFixture(t), func() {}
			},
			run: func(t *testing.T, fixture *serviceFixture) {
				for _, dispatchID := range []string{"dispatch-1", "dispatch-2"} {
					if _, err := fixture.service.Receive(context.Background(), executionRequest(fixture, dispatchID, newTaskLoggerMock(t))); err != nil {
						t.Fatalf("Receive() 返回意外错误: %v", err)
					}
				}
			},
			after: func(t *testing.T, fixture *serviceFixture) {
				if count := fixture.handler.runs.Load(); count != 2 {
					t.Fatalf("Handler 执行次数 = %d, 期望 2", count)
				}
			},
		},
		{
			name: "制品引用通过共享引擎准备并清理",
			before: func(t *testing.T) (*serviceFixture, func()) {
				fixture := newServiceFixture(t)
				fixture.execution.Artifacts = []domain.ArtifactRef{{
					ReleaseID: 1, Digest: strings.Repeat("a", 64), BlobChecksum: strings.Repeat("b", 64),
					Size: 1, Format: "tar.zst", FormatVersion: 1, Scope: domain.CodebookScopeSystem,
				}}
				return fixture, func() {}
			},
			run: func(t *testing.T, fixture *serviceFixture) {
				if _, err := fixture.service.Receive(context.Background(), executionRequest(fixture, "dispatch-1", newTaskLoggerMock(t))); err != nil {
					t.Fatalf("Receive() 返回意外错误: %v", err)
				}
			},
			after: func(t *testing.T, fixture *serviceFixture) {
				if fixture.preparer.prepares.Load() != 1 || fixture.preparer.prepared.closes.Load() != 1 {
					t.Fatalf("制品准备/清理次数 = %d/%d, 期望 1/1",
						fixture.preparer.prepares.Load(), fixture.preparer.prepared.closes.Load())
				}
				if roots := fixture.handler.roots.Load(); roots != "/system:/dependencies" {
					t.Fatalf("Handler 收到的制品目录 = %v", roots)
				}
			},
		},
		{
			name: "敏感变量不会进入 Kafka 结果日志",
			before: func(t *testing.T) (*serviceFixture, func()) {
				fixture := newServiceFixture(t)
				fixture.execution.Task.GrpcConfig.Params = map[string]string{
					"variables": `[{"key":"TOKEN","value":"top-secret","secret":true}]`,
				}
				fixture.handler.logMessage = "token=top-secret"
				return fixture, func() {}
			},
			run: func(t *testing.T, fixture *serviceFixture) {
				ctrl := gomock.NewController(t)
				logger := servicemocks.NewMockTaskLogger(ctrl)
				gomock.InOrder(
					logger.EXPECT().Log("%s", "token=[MASKED]"),
					logger.EXPECT().Close(),
				)
				_, err := fixture.service.Receive(context.Background(), executionRequest(fixture, "dispatch-1", logger))
				if err != nil {
					t.Fatalf("Receive() 返回意外错误: %v", err)
				}
			},
			after: func(t *testing.T, fixture *serviceFixture) {},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture, cleanup := testCase.before(t)
			defer cleanup()
			testCase.run(t, fixture)
			testCase.after(t, fixture)
		})
	}
}

func executionRequest(fixture *serviceFixture, dispatchID string, logger executor.TaskLogger) ExecutionRequest {
	return ExecutionRequest{DispatchID: dispatchID, Execution: fixture.execution, TaskLogger: logger}
}

func newTaskLoggerMock(t *testing.T) *servicemocks.MockTaskLogger {
	t.Helper()
	logger := servicemocks.NewMockTaskLogger(gomock.NewController(t))
	logger.EXPECT().Close()
	return logger
}

type serviceFixture struct {
	service   Service
	handler   *serviceHandlerFake
	preparer  *servicePreparerFake
	execution domain.TaskExecution
}

func newServiceFixture(t *testing.T) *serviceFixture {
	t.Helper()
	handler := &serviceHandlerFake{started: make(chan struct{}, 1)}
	preparer := &servicePreparerFake{prepared: &servicePreparedFake{
		roots: executor.ArtifactRoots{Default: "/system", Dependencies: "/dependencies"},
	}}
	return &serviceFixture{
		service:  NewService([]executor.TaskHandler{handler}, preparer, nil, nil),
		handler:  handler,
		preparer: preparer,
		execution: domain.TaskExecution{
			ID: 10, TenantID: 20,
			Task: domain.Task{ID: 30, Name: "测试任务", GrpcConfig: &domain.GrpcConfig{HandlerName: handler.Name()}},
		},
	}
}

type serviceHandlerFake struct {
	runs       atomic.Int32
	started    chan struct{}
	block      chan struct{}
	roots      atomic.Value
	logMessage string
}

func (h *serviceHandlerFake) Name() string                   { return "test" }
func (h *serviceHandlerFake) Desc() string                   { return "测试处理器" }
func (h *serviceHandlerFake) Metadata() []executor.Parameter { return nil }
func (h *serviceHandlerFake) Run(ctx *executor.Context) error {
	h.runs.Add(1)
	select {
	case h.started <- struct{}{}:
	default:
	}
	roots := ctx.ArtifactRoots()
	h.roots.Store(roots.Default + ":" + roots.Dependencies)
	if h.block != nil {
		<-h.block
	}
	if h.logMessage != "" {
		ctx.Log("%s", h.logMessage)
	}
	ctx.SetResult("status", "ok")
	return nil
}

type servicePreparerFake struct {
	prepares atomic.Int32
	prepared *servicePreparedFake
}

func (p *servicePreparerFake) Prune() error { return nil }
func (p *servicePreparerFake) Prepare(context.Context, artifactv1.ArtifactServiceClient,
	[]*artifactv1.ArtifactRef) (executor.PreparedArtifacts, error) {
	p.prepares.Add(1)
	return p.prepared, nil
}

type servicePreparedFake struct {
	roots  executor.ArtifactRoots
	closes atomic.Int32
}

func (p *servicePreparedFake) Roots() executor.ArtifactRoots { return p.roots }
func (p *servicePreparedFake) Close() error {
	p.closes.Add(1)
	return nil
}
