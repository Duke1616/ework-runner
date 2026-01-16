package executor

import (
	"context"
	"fmt"
	"time"

	executorv1 "github.com/Duke1616/ework-runner/api/proto/gen/executor/v1"
	reporterv1 "github.com/Duke1616/ework-runner/api/proto/gen/reporter/v1"
	grpcpkg "github.com/Duke1616/ework-runner/pkg/grpc"
	"github.com/Duke1616/ework-runner/pkg/grpc/registry/etcd"
	"github.com/ecodeclub/ekit/syncx"
	"github.com/gotomicro/ego/core/elog"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Executor 极简 Executor 实现
type Executor struct {
	executorv1.UnimplementedExecutorServiceServer

	config  *Config
	handler func(*Context) error

	// 内部组件
	server         *Server
	reporterClient reporterv1.ReporterServiceClient
	logger         *elog.Component

	// 状态管理 - 使用 syncx.Map
	states  *syncx.Map[int64, *executorv1.ExecutionState]
	cancels *syncx.Map[int64, context.CancelFunc]
}

// NewExecutor 创建 Executor
func NewExecutor(cfg *Config) (*Executor, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &Executor{
		config:  cfg,
		logger:  elog.DefaultLogger.With(elog.FieldComponentName("executor")),
		states:  &syncx.Map[int64, *executorv1.ExecutionState]{},
		cancels: &syncx.Map[int64, context.CancelFunc]{},
	}, nil
}

// MustNewExecutor 创建 Executor (panic on error)
func MustNewExecutor(cfg *Config) *Executor {
	exec, err := NewExecutor(cfg)
	if err != nil {
		panic(err)
	}
	return exec
}

// RegisterHandler 注册任务处理函数
func (e *Executor) RegisterHandler(handler func(*Context) error) *Executor {
	e.handler = handler
	return e
}

// Start 启动 Executor
func (e *Executor) Start() error {
	if e.handler == nil {
		return fmt.Errorf("未注册处理函数,请先调用 RegisterHandler")
	}

	// 初始化组件
	if err := e.initComponents(); err != nil {
		return err
	}

	e.logger.Info("Executor 启动成功",
		elog.String("nodeID", e.config.NodeID),
		elog.String("serviceName", e.config.ServiceName),
		elog.String("addr", e.config.Addr))

	// 启动 gRPC Server (阻塞)
	return e.server.Start()
}

// initComponents 初始化所有组件
func (e *Executor) initComponents() error {
	// 1. 创建 etcd Registry
	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints: e.config.EtcdEndpoints,
	})
	if err != nil {
		return fmt.Errorf("创建 etcd 客户端失败: %w", err)
	}

	reg, err := etcd.NewRegistry(etcdClient)
	if err != nil {
		return fmt.Errorf("创建服务注册失败: %w", err)
	}

	// 2. 创建 Reporter 客户端 - 使用 resolver 方式支持服务发现和负载均衡
	// NOTE: Reporter 可能有多个节点,使用 resolver 自动发现和负载均衡
	// NOTE: 使用 executor:// scheme (自定义resolver),服务名包含 service/ 前缀
	reporterConn, err := grpc.NewClient(
		fmt.Sprintf("executor:///%s/%s", grpcpkg.ServicePrefix, e.config.ReporterServiceName),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
		grpc.WithResolvers(grpcpkg.NewResolverBuilder(reg, 10*time.Second)),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("连接 reporter 失败: %w", err)
	}
	e.reporterClient = reporterv1.NewReporterServiceClient(reporterConn)

	// 3. 创建 gRPC Server
	e.server = NewServer(e.config.NodeID, e.config.ServiceName, e.config.Addr, reg)

	// 4. 注册 Executor 服务
	executorv1.RegisterExecutorServiceServer(e.server.Server, e)

	return nil
}

// Execute 实现 ExecutorServiceServer.Execute
func (e *Executor) Execute(ctx context.Context, req *executorv1.ExecuteRequest) (*executorv1.ExecuteResponse, error) {
	eid := req.GetEid()

	// 检查是否已经在执行
	if state, ok := e.states.Load(eid); ok {
		return &executorv1.ExecuteResponse{ExecutionState: state}, nil
	}

	// 创建初始状态
	state := &executorv1.ExecutionState{
		Id:              eid,
		TaskId:          req.GetTaskId(),
		TaskName:        req.GetTaskName(),
		Status:          executorv1.ExecutionStatus_RUNNING,
		RunningProgress: 0,
		ExecutorNodeId:  e.config.NodeID,
	}
	e.states.Store(eid, state)

	// 创建任务上下文
	taskCtx := newContext(eid, req.GetTaskId(), req.GetTaskName(), req.GetParams(), e.reporterClient, e.logger)

	// 创建可取消上下文
	runCtx, cancel := context.WithCancel(context.Background())
	e.cancels.Store(eid, cancel)

	// 异步执行任务
	go e.executeTask(runCtx, taskCtx, eid)

	return &executorv1.ExecuteResponse{ExecutionState: state}, nil
}

// executeTask 执行用户任务
func (e *Executor) executeTask(runCtx context.Context, taskCtx *Context, eid int64) {
	defer func() {
		e.cancels.Delete(eid)
	}()

	logger := taskCtx.Logger()

	// 调用用户处理函数
	err := e.handler(taskCtx)

	// 确定最终状态
	var finalStatus executorv1.ExecutionStatus
	if runCtx.Err() != nil {
		finalStatus = executorv1.ExecutionStatus_FAILED_RESCHEDULABLE
		logger.Warn("任务被中断")
	} else if err != nil {
		finalStatus = executorv1.ExecutionStatus_FAILED
		logger.Error("任务执行失败", elog.FieldErr(err))
	} else {
		finalStatus = executorv1.ExecutionStatus_SUCCESS
		logger.Info("任务执行成功")
	}

	// 更新并上报最终状态
	e.reportFinalResult(eid, finalStatus)
}

// reportFinalResult 上报最终结果
func (e *Executor) reportFinalResult(eid int64, status executorv1.ExecutionStatus) {
	state, exists := e.states.Load(eid)
	if exists {
		state.Status = status
		if status == executorv1.ExecutionStatus_SUCCESS {
			state.RunningProgress = 100
		}
		e.states.Store(eid, state)

		// 上报给 Reporter
		_, err := e.reporterClient.Report(context.Background(), &reporterv1.ReportRequest{
			ExecutionState: state,
		})
		if err != nil {
			e.logger.Error("上报最终状态失败", elog.FieldErr(err))
		}
	}
}

// Query 实现 ExecutorServiceServer.Query
func (e *Executor) Query(ctx context.Context, req *executorv1.QueryRequest) (*executorv1.QueryResponse, error) {
	eid := req.GetEid()

	if state, ok := e.states.Load(eid); ok {
		return &executorv1.QueryResponse{ExecutionState: state}, nil
	}

	return &executorv1.QueryResponse{
		ExecutionState: &executorv1.ExecutionState{
			Id:     eid,
			Status: executorv1.ExecutionStatus_UNKNOWN,
		},
	}, nil
}

// Interrupt 实现 ExecutorServiceServer.Interrupt
func (e *Executor) Interrupt(ctx context.Context, req *executorv1.InterruptRequest) (*executorv1.InterruptResponse, error) {
	eid := req.GetEid()

	if cancel, ok := e.cancels.Load(eid); ok {
		cancel()

		if state, ok := e.states.Load(eid); ok {
			return &executorv1.InterruptResponse{
				Success:        true,
				ExecutionState: state,
			}, nil
		}
	}

	return &executorv1.InterruptResponse{Success: false}, nil
}

// Prepare 实现 ExecutorServiceServer.Prepare
func (e *Executor) Prepare(ctx context.Context, req *executorv1.PrepareRequest) (*executorv1.PrepareResponse, error) {
	return &executorv1.PrepareResponse{
		Params: make(map[string]string),
	}, nil
}
