package ioc

import (
	"github.com/Duke1616/ework-runner/internal/event"
	"github.com/Duke1616/ework-runner/internal/service/acquirer"
	"github.com/Duke1616/ework-runner/internal/service/invoker"
	"github.com/Duke1616/ework-runner/internal/service/runner"
	"github.com/Duke1616/ework-runner/internal/service/task"
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
