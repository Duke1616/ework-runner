package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	artifactv1 "github.com/Duke1616/etask/api/proto/gen/etask/artifact/v1"
	executorv1 "github.com/Duke1616/etask/api/proto/gen/etask/executor/v1"
	"github.com/Duke1616/etask/internal/agent/domain"
	internaldomain "github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/sdk/executor"
	"github.com/Duke1616/etask/sdk/executor/artifact"
	artifactgrpc "github.com/Duke1616/etask/sdk/executor/artifact/grpc"
	executionengine "github.com/Duke1616/etask/sdk/executor/engine"
	"github.com/Duke1616/etask/sdk/executor/logging/egolog"
	"github.com/gotomicro/ego/core/elog"
)

const executionRetention = 30 * time.Minute

var ErrExecutionTerminated = errors.New("execution 已被强制终止")

//go:generate go tool mockgen -package=servicemocks -destination=./mocks/task_logger.mock.go -typed github.com/Duke1616/etask/sdk/executor TaskLogger

// Service 定义独立 Kafka Agent 的执行能力。
type Service interface {
	// Receive 幂等执行一条 Kafka 命令。
	Receive(ctx context.Context, request ExecutionRequest) (domain.ExecutionOutput, error)
	// Terminate 终止本 Agent 上的 execution；终止先到时会保留墓碑阻止后续执行。
	Terminate(executionID int64, reason string) bool
	// ListHandlers 列出支持的任务处理器详情。
	ListHandlers() []executor.HandlerMeta
}

// ExecutionRequest 汇总一次 Agent 执行所需的输入。
// TaskLogger 由执行引擎持有，并在 Receive 返回前关闭。
type ExecutionRequest struct {
	DispatchID string
	Execution  internaldomain.TaskExecution
	TaskLogger executor.TaskLogger
}

type service struct {
	registry        *executionengine.HandlerRegistry
	engine          *executionengine.Engine
	executionClient executorv1.TaskExecutionServiceClient
	logger          *elog.Component
	mu              sync.Mutex
	executions      map[string]*executionEntry
	active          map[int64]*executionEntry
	terminated      map[int64]time.Time
}

type executionEntry struct {
	startedAt   time.Time
	executionID int64
	done        chan struct{}
	cancel      context.CancelFunc
	terminated  bool
	output      domain.ExecutionOutput
	err         error
}

// NewService 创建独立 Agent 执行服务。
func NewService(handlers []executor.TaskHandler, preparer artifact.Preparer,
	artifactClient artifactv1.ArtifactServiceClient,
	executions executorv1.TaskExecutionServiceClient) (Service, error) {
	registry := executionengine.NewHandlerRegistry()
	if err := registry.Register(handlers...); err != nil {
		return nil, fmt.Errorf("注册 Agent 任务处理器失败: %w", err)
	}
	logger := elog.DefaultLogger.With(elog.FieldComponentName("agent.execution"))
	return &service{
		registry: registry,
		engine: executionengine.New(registry, preparer,
			executionengine.WithArtifactDownloader(artifactgrpc.NewDownloader(artifactClient)),
			executionengine.WithLogger(egolog.New(logger)),
		),
		executionClient: executions,
		logger:          logger,
		executions:      make(map[string]*executionEntry),
		active:          make(map[int64]*executionEntry),
		terminated:      make(map[int64]time.Time),
	}, nil
}

// ListHandlers 列出支持的任务处理器详情。
func (s *service) ListHandlers() []executor.HandlerMeta {
	return s.registry.ListMetas()
}

// Receive 幂等执行一条 Kafka 命令。
func (s *service) Receive(ctx context.Context, request ExecutionRequest) (domain.ExecutionOutput, error) {
	execution := request.Execution
	if request.DispatchID == "" || execution.ID <= 0 || execution.Task.GrpcConfig == nil || request.TaskLogger == nil {
		return domain.ExecutionOutput{}, fmt.Errorf("agent 执行命令缺少派发 ID、执行 ID、处理器配置或日志器")
	}
	// dispatchID 是消息重投的幂等键；非 owner 等待首次执行结果而不重复运行。
	entry, owner, runCtx := s.begin(ctx, request.DispatchID, execution.ID)
	if !owner {
		request.TaskLogger.Close()
		select {
		case <-ctx.Done():
			return domain.ExecutionOutput{}, ctx.Err()
		case <-entry.done:
			return entry.output, entry.err
		}
	}
	if err := s.ensureNotCancelled(runCtx, execution.ID); err != nil {
		request.TaskLogger.Close()
		if errors.Is(err, ErrExecutionTerminated) {
			s.Terminate(execution.ID, err.Error())
		}
		err = s.finish(entry, domain.ExecutionOutput{}, err)
		return domain.ExecutionOutput{}, err
	}
	refs, err := internaldomain.ArtifactRefsToExecutor(execution.Artifacts)
	if err != nil {
		request.TaskLogger.Close()
		s.finish(entry, domain.ExecutionOutput{}, err)
		return domain.ExecutionOutput{}, err
	}
	// 与独立 Executor 复用同一个 Engine，制品和 Handler 行为保持一致。
	result, err := s.engine.Execute(runCtx, executionengine.Command{
		Task: executor.TaskInfo{
			ExecutionID: execution.ID, TaskID: execution.Task.ID,
			Name: execution.Task.Name, Handler: execution.Task.GrpcConfig.HandlerName,
		},
		Params: execution.GRPCParams(), Parameters: s.handlerMetadata(execution.Task.GrpcConfig.HandlerName),
		Artifacts: refs, TaskLogger: request.TaskLogger,
	})
	output := domain.ExecutionOutput{Result: result.Value}
	err = s.finish(entry, output, err)
	return output, err
}

func (s *service) ensureNotCancelled(ctx context.Context, executionID int64) error {
	if s.executionClient == nil {
		return nil
	}
	response, err := s.executionClient.GetTaskExecution(ctx, &executorv1.GetTaskExecutionRequest{
		ExecutionId: executionID,
	})
	if err != nil {
		return fmt.Errorf("查询 execution 最新状态失败: %w", err)
	}
	if response.GetExecution().GetStatus() == executorv1.ExecutionStatus_CANCELLED {
		return ErrExecutionTerminated
	}
	return nil
}

func (s *service) handlerMetadata(name string) []executor.Parameter {
	handler, ok := s.registry.Get(name)
	if !ok {
		return nil
	}
	return handler.Metadata()
}

func (s *service) begin(ctx context.Context, dispatchID string,
	executionID int64) (*executionEntry, bool, context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	// 新命令进入时顺便清理过期终态，避免额外维护后台清理协程。
	for id, entry := range s.executions {
		select {
		case <-entry.done:
			if now.Sub(entry.startedAt) >= executionRetention {
				delete(s.executions, id)
			}
		default:
		}
	}
	for id, terminatedAt := range s.terminated {
		if now.Sub(terminatedAt) >= executionRetention {
			delete(s.terminated, id)
		}
	}
	if entry := s.executions[dispatchID]; entry != nil {
		return entry, false, nil
	}
	entry := &executionEntry{startedAt: now, executionID: executionID, done: make(chan struct{})}
	s.executions[dispatchID] = entry
	if _, stopped := s.terminated[executionID]; stopped {
		entry.terminated = true
		entry.err = ErrExecutionTerminated
		close(entry.done)
		return entry, false, nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	entry.cancel = cancel
	s.active[executionID] = entry
	return entry, true, runCtx
}

func (s *service) finish(entry *executionEntry, output domain.ExecutionOutput, err error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry.terminated {
		err = ErrExecutionTerminated
	}
	entry.output = output
	entry.err = err
	entry.cancel = nil
	if s.active[entry.executionID] == entry {
		delete(s.active, entry.executionID)
	}
	close(entry.done)
	return err
}

func (s *service) Terminate(executionID int64, _ string) bool {
	s.mu.Lock()
	s.terminated[executionID] = time.Now()
	entry := s.active[executionID]
	if entry != nil {
		entry.terminated = true
	}
	var cancel context.CancelFunc
	if entry != nil {
		cancel = entry.cancel
	}
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return entry != nil
}
