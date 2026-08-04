package runtime

import (
	"context"
	"fmt"

	reporterv1 "github.com/Duke1616/etask/api/proto/gen/etask/reporter/v1"
	"github.com/Duke1616/etask/sdk/executor/internal/task"
)

type executionProgressReporter struct {
	executions executionStore
	reporter   reporterv1.ReporterServiceClient
}

func (r executionProgressReporter) ReportProgress(ctx context.Context,
	info task.TaskInfo, progress int32) error {
	state, exists := r.executions.Progress(info.ExecutionID, progress)
	if !exists {
		return fmt.Errorf("execution %d 不在运行中", info.ExecutionID)
	}
	if r.reporter == nil {
		return nil
	}
	_, err := r.reporter.Report(ctx, &reporterv1.ReportRequest{ExecutionState: state})
	return err
}

var _ task.ProgressReporter = executionProgressReporter{}
