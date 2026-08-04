package runtime

// 本文件负责运行组件装配和 PULL 循环。

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	artifactv1 "github.com/Duke1616/etask/api/proto/gen/etask/artifact/v1"
	executorv1 "github.com/Duke1616/etask/api/proto/gen/etask/executor/v1"
	reporterv1 "github.com/Duke1616/etask/api/proto/gen/etask/reporter/v1"
	grpcpkg "github.com/Duke1616/etask/pkg/grpc"
	enginepkg "github.com/Duke1616/etask/sdk/executor/internal/engine"
	"github.com/gotomicro/ego/core/elog"
)

// InitComponents 初始化制品缓存、调度中心客户端和 Executor gRPC 服务。
func (e *Executor) InitComponents() error {
	e.initMu.Lock()
	defer e.initMu.Unlock()
	if e.initialized {
		return nil
	}
	if e.registrationErr != nil {
		return fmt.Errorf("Executor 任务处理器注册失败: %w", e.registrationErr)
	}
	// 启动前清理上次异常退出留下的半成品，避免任务读取不完整缓存。
	if e.artifacts != nil {
		if err := e.artifacts.Prune(); err != nil {
			return fmt.Errorf("清理制品缓存失败: %w", err)
		}
	}
	// 调度中心的上报、拉取和制品下载共用同一条带认证的客户端连接。
	connection, err := grpcpkg.NewClientConn(
		e.registry,
		grpcpkg.WithServiceName(e.config.Client.Name),
		grpcpkg.WithClientJWTAuth(e.config.Client.AuthToken),
		grpcpkg.WithTimeout(10*time.Second),
	)
	if err != nil {
		return fmt.Errorf("连接调度中心失败: %w", err)
	}
	e.reporterClient = reporterv1.NewReporterServiceClient(connection)
	e.agentClient = executorv1.NewAgentServiceClient(connection)
	e.executionClient = executorv1.NewTaskExecutionServiceClient(connection)
	e.artifactClient = artifactv1.NewArtifactServiceClient(connection)
	e.schedulerConn = connection
	// Engine 持有节点生命周期内稳定的客户端；单次 Command 只描述任务输入。
	e.engine = enginepkg.New(e.hr, e.artifacts,
		enginepkg.WithArtifactClient(e.artifactClient),
		enginepkg.WithReporter(e.reporterClient),
		enginepkg.WithProgressReporter(executionProgressReporter{
			executions: e.executions, reporter: e.reporterClient,
		}),
		enginepkg.WithLogger(e.logger),
	)
	// gRPC Server 负责 PUSH 模式接收任务，同时通过 metadata 发布节点能力。
	e.server = grpcpkg.NewServer(
		e.config.Server,
		e.registry,
		grpcpkg.WithJWTAuth(e.config.Server.AuthToken),
		grpcpkg.WithMetadata(e.buildMetadata()),
	)
	executorv1.RegisterExecutorServiceServer(e.server.Server, e)
	// PULL 循环只在节点显式声明 PULL 模式时启动，并由生命周期统一取消。
	if e.config.Mode == "PULL" {
		pullCtx, cancel := context.WithCancel(context.Background())
		e.pullCancel = cancel
		go e.pullTasks(pullCtx)
	}
	e.initialized = true
	return nil
}

// Server 返回供应用启动的 gRPC Server；尚未初始化时返回 nil。
func (e *Executor) Server() *grpcpkg.Server {
	return e.server
}

func (e *Executor) pullTasks(ctx context.Context) {
	e.logger.Info("Executor 已进入 PULL 模式")
	for ctx.Err() == nil {
		// 长轮询只上报当前节点实际注册的 Handler，调度中心据此匹配任务。
		requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		response, err := e.agentClient.PullTask(requestCtx, &executorv1.PullTaskRequest{
			ServiceName: e.config.Server.ServiceName,
			NodeId:      e.config.Server.ServiceId,
			Handlers:    e.hr.Names(),
		})
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			e.logger.Warn("拉取任务失败", elog.FieldErr(err))
			// 短暂退避，避免调度中心不可用时形成高频空转。
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}
		if response != nil && response.HasTask && response.TaskReq != nil {
			e.logger.Info("拉取到待执行任务", elog.Int64("eid", response.TaskReq.GetEid()))
			if _, executeErr := e.Execute(context.Background(), response.TaskReq); executeErr != nil {
				e.logger.Error("启动拉取任务失败",
					elog.Int64("eid", response.TaskReq.GetEid()), elog.FieldErr(executeErr))
			}
		}
	}
}

func (e *Executor) buildMetadata() map[string]any {
	// Handler 元数据随节点注册发布，供调度和管理端发现节点能力。
	metadata, err := json.Marshal(e.hr.ListMetas())
	if err != nil {
		e.logger.Error("序列化处理器元数据失败", elog.FieldErr(err))
		metadata = []byte("[]")
	}
	return map[string]any{
		"role": RoleName, "desc": e.config.Desc, "supported_handlers": string(metadata),
		"mode": e.config.Mode, "isolation_level": e.config.IsolationLevel,
	}
}
