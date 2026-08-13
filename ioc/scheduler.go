package ioc

import (
	"github.com/Duke1616/etask/internal/service/acquirer"
	"github.com/Duke1616/etask/internal/service/dispatcher"
	"github.com/Duke1616/etask/internal/service/scheduler"
	"github.com/Duke1616/etask/internal/service/task"
	"github.com/Duke1616/etask/internal/sse"
	config "github.com/Duke1616/etask/pkg/config"
	"github.com/google/uuid"
)

// InitNodeID 创建本次进程使用的调度节点 ID。
func InitNodeID() string {
	return uuid.New().String()
}

// InitScheduler 从配置中创建调度器。
func InitScheduler(
	nodeID string,
	dispatcher dispatcher.Dispatcher,
	taskSvc task.Service,
	acquirer acquirer.TaskAcquirer,
	events *sse.Hubs,
) *scheduler.Scheduler {
	var cfg scheduler.Config
	err := config.UnmarshalKey("scheduler", &cfg)
	if err != nil {
		panic(err)
	}

	return scheduler.NewScheduler(
		nodeID,
		dispatcher,
		taskSvc,
		acquirer,
		events,
		cfg,
	)
}
