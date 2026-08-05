package ioc

import (
	"github.com/Duke1616/etask/internal/repository"
	"github.com/Duke1616/etask/internal/service/acquirer"
	"github.com/Duke1616/etask/internal/service/dispatcher"
	"github.com/Duke1616/etask/internal/service/invoker"
	"github.com/Duke1616/etask/internal/service/picker"
	"github.com/Duke1616/etask/internal/service/task"
	config "github.com/Duke1616/etask/pkg/config"
)

// InitDispatcherConfig 读取任务执行并发控制配置。
func InitDispatcherConfig() dispatcher.Config {
	var cfg dispatcher.Config
	if err := config.UnmarshalKey("scheduler", &cfg); err != nil {
		panic(err)
	}
	if err := cfg.Validate(); err != nil {
		panic(err)
	}
	return cfg
}

// InitDispatcher 创建任务派发器。
func InitDispatcher(
	nodeID string,
	execSvc task.ExecutionService,
	taskAcquirer acquirer.TaskAcquirer,
	invoker invoker.Invoker,
	routes dispatcher.RoutePlanner,
	cfg dispatcher.Config,
) dispatcher.Dispatcher {
	return dispatcher.NewTaskDispatcher(
		nodeID,
		execSvc,
		taskAcquirer,
		invoker,
		routes,
		cfg,
	)
}

// InitRoutePlanner 创建任务派发路由规划器。
func InitRoutePlanner(
	poolRepo repository.ExecutionPoolRepository,
	targetPicker picker.ExecutorNodePicker,
) dispatcher.RoutePlanner {
	return dispatcher.NewRoutePlanner(poolRepo, targetPicker)
}
