package ioc

import (
	"github.com/Duke1616/ecmdb/internal/service/acquirer"
	"github.com/Duke1616/ecmdb/internal/service/picker"
	"github.com/Duke1616/ecmdb/internal/service/runner"
	"github.com/Duke1616/ecmdb/internal/service/scheduler"
	"github.com/Duke1616/ecmdb/internal/service/task"
	"github.com/google/uuid"
	"github.com/gotomicro/ego/core/econf"
)

func InitNodeID() string {
	return uuid.New().String()
}

func InitScheduler(
	nodeID string,
	runner runner.Runner,
	taskSvc task.Service,
	execSvc task.ExecutionService,
	acquirer acquirer.TaskAcquirer,
	nodePicker picker.ExecutorNodePicker,
) *scheduler.Scheduler {
	var cfg scheduler.Config
	err := econf.UnmarshalKey("scheduler", &cfg)
	if err != nil {
		panic(err)
	}

	return scheduler.NewScheduler(
		nodeID,
		runner,
		taskSvc,
		execSvc,
		acquirer,
		cfg,
		nodePicker,
	)
}
