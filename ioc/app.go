package ioc

import (
	"context"
	"fmt"
	"strings"

	"github.com/Duke1616/etask/internal/grpc/scripts"
	"github.com/Duke1616/etask/internal/service/scheduler"
	grpcpkg "github.com/Duke1616/etask/pkg/grpc"
	"github.com/Duke1616/etask/pkg/grpc/registry"
	"github.com/Duke1616/etask/sdk/executor/artifact"
	"github.com/gotomicro/ego/server"
	"github.com/gotomicro/ego/server/egin"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const (
	ModeAll       = "all"
	ModeScheduler = "scheduler"
	ModeAgent     = "agent"
	ModeExecutor  = "executor"
)

// Task 是随应用生命周期启动的后台任务。
type Task interface {
	// Start 启动后台任务。
	Start(ctx context.Context)
}

// Base 保存所有运行模式都会使用的服务发现基础设施。
type Base struct {
	Registry registry.Registry
	Etcd     *clientv3.Client
}

// ExecutionRuntime 保存 Agent 和 Executor 共享的本地执行能力。
type ExecutionRuntime struct {
	ArtifactPreparer artifact.Preparer
	ScriptRuntime    *scripts.Runtime
}

// SchedulerApplication 聚合调度中心共享依赖构建出的全部服务。
type SchedulerApplication struct {
	Web       *egin.Component
	GRPC      *grpcpkg.Server
	Scheduler *scheduler.Scheduler
	Tasks     []Task
}

// Servers 返回调度中心需要启动的服务。
func (a *SchedulerApplication) Servers() []server.Server {
	return []server.Server{a.Web, a.GRPC, a.Scheduler}
}

// App 保存当前进程实际启用的服务和后台任务。
type App struct {
	servers []server.Server
	tasks   []Task
}

// NewApp 创建空应用容器。
func NewApp() *App { return &App{} }

// LoadByModes 按固定顺序装配所选运行模式。
func (a *App) LoadByModes(base *Base, modes []string) error {
	selected, err := normalizeModes(modes)
	if err != nil {
		return err
	}
	if selected[ModeScheduler] {
		application := InitSchedulerApplication(base)
		a.servers = append(a.servers, application.Servers()...)
		a.tasks = append(a.tasks, application.Tasks...)
	}

	var runtime *ExecutionRuntime
	if selected[ModeAgent] || selected[ModeExecutor] {
		runtime = InitExecutionRuntime()
	}
	if selected[ModeAgent] {
		a.servers = append(a.servers, InitAgentModule(base, runtime))
	}
	if selected[ModeExecutor] {
		a.servers = append(a.servers, InitExecutorModule(base, runtime))
	}
	return nil
}

// GetServers 返回当前进程需要启动的服务副本。
func (a *App) GetServers() []server.Server {
	return append([]server.Server(nil), a.servers...)
}

// StartBackgroundTasks 启动当前模式附带的后台任务。
func (a *App) StartBackgroundTasks(ctx context.Context) {
	for _, task := range a.tasks {
		go task.Start(ctx)
	}
}

func normalizeModes(modes []string) (map[string]bool, error) {
	selected := make(map[string]bool, 3)
	for _, mode := range modes {
		mode = strings.ToLower(strings.TrimSpace(mode))
		switch mode {
		case ModeAll:
			selected[ModeScheduler] = true
			selected[ModeAgent] = true
			selected[ModeExecutor] = true
		case ModeScheduler, ModeAgent, ModeExecutor:
			selected[mode] = true
		case "":
			return nil, fmt.Errorf("启动模式不能为空")
		default:
			return nil, fmt.Errorf("不支持的启动模式: %s", mode)
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("至少需要指定一个启动模式")
	}
	return selected, nil
}
