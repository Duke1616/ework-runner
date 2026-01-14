package ioc

import (
	"github.com/Duke1616/ecmdb/internal/event"
	"github.com/Duke1616/ecmdb/internal/service/acquirer"
	"github.com/Duke1616/ecmdb/internal/service/invoker"
	"github.com/Duke1616/ecmdb/internal/service/runner"
	"github.com/Duke1616/ecmdb/internal/service/task"
)

func InitRunner(
	nodeID string,
	taskSvc task.Service,
	execSvc task.ExecutionService,
	taskAcquirer acquirer.TaskAcquirer,
	invoker invoker.Invoker,
	producer event.CompleteProducer,
) runner.Runner {
	return runner.NewNormalTaskRunner(
		nodeID,
		taskSvc,
		execSvc,
		taskAcquirer,
		invoker,
		producer,
	)
}
