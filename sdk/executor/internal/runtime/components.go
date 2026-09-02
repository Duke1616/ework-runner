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
	"github.com/Duke1616/etask/pkg/grpc/interceptors/tenant"
	artifactgrpc "github.com/Duke1616/etask/sdk/executor/artifact/grpc"
	enginepkg "github.com/Duke1616/etask/sdk/executor/internal/engine"
	"github.com/Duke1616/etask/sdk/executor/logging/egolog"
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
		return fmt.Errorf("executor 任务处理器注册失败: %w", e.registrationErr)
	}
	if err := e.prepareArtifacts(); err != nil {
		return err
	}
	if err := e.connectScheduler(); err != nil {
		return err
	}
	e.initEngine()
	e.initServer()
	e.initialized = true
	return nil
}

// Server 返回供应用启动的 gRPC Server；尚未初始化时返回 nil。
func (e *Executor) Server() *grpcpkg.Server {
	return e.server
}

func (e *Executor) prepareArtifacts() error {
	if e.artifacts == nil {
		return nil
	}
	// 清理过期制品缓存
	if err := e.artifacts.Prune(); err != nil {
		return fmt.Errorf("清理制品缓存失败: %w", err)
	}
	return nil
}

func (e *Executor) connectScheduler() error {
	// 连接调度中心
	connection, err := grpcpkg.NewClientConn(
		e.registry,
		grpcpkg.WithServiceName(e.config.Client.Name),
		grpcpkg.WithClientJWTAuth(e.config.Client.AuthToken),
		grpcpkg.WithTimeout(10*time.Second),
	)
	if err != nil {
		return fmt.Errorf("连接调度中心失败: %w", err)
	}
	// 初始化微服务客户端
	e.reporterClient = reporterv1.NewReporterServiceClient(connection)
	e.agentClient = executorv1.NewAgentServiceClient(connection)
	e.executionClient = executorv1.NewTaskExecutionServiceClient(connection)
	e.artifactClient = artifactv1.NewArtifactServiceClient(connection)
	e.schedulerConn = connection
	return nil
}

func (e *Executor) initEngine() {
	// 装配执行引擎与辅助组件
	e.engine = enginepkg.New(e.hr, e.artifacts,
		enginepkg.WithArtifactDownloader(artifactgrpc.NewDownloader(e.artifactClient)),
		enginepkg.WithProgressReporter(executionProgressReporter{
			executions: e.executions, reporter: e.reporterClient,
		}),
		enginepkg.WithSystemLogger(egolog.New(e.logger)),
	)
}

func (e *Executor) initServer() {
	// 初始化 gRPC 服务并注册执行服务
	e.server = grpcpkg.NewServer(
		e.config.Server,
		e.registry,
		grpcpkg.WithJWTAuth(e.config.Server.AuthToken),
		grpcpkg.WithMetadata(e.buildMetadata()),
	)
	executorv1.RegisterExecutorServiceServer(e.server.Server, e)
}

func (e *Executor) pullTasks(ctx context.Context) {
	e.logger.Info("Executor 已进入 PULL 模式")
	for ctx.Err() == nil {
		requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		// 长轮询拉取任务
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
			// 短暂退避，避免空转
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}
		if response != nil && response.HasTask && response.TaskReq != nil {
			e.logger.Info("拉取到待执行任务", elog.Int64("eid", response.TaskReq.GetEid()))
			// 启动本地执行，显式注入任务绑定的租户上下文
			taskCtx := tenant.Set(context.Background(), response.TaskReq.GetTenantId())
			if _, executeErr := e.Execute(taskCtx, response.TaskReq); executeErr != nil {
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
