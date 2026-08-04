package engine

import (
	"context"

	executorv1 "github.com/Duke1616/etask/api/proto/gen/etask/executor/v1"
	reporterv1 "github.com/Duke1616/etask/api/proto/gen/etask/reporter/v1"
	"github.com/Duke1616/etask/pkg/tasklog"
	"github.com/Duke1616/etask/sdk/executor/internal/task"
	"github.com/gotomicro/ego/core/elog"
)

type grpcLogSink struct {
	executionID int64
	reporter    reporterv1.ReporterServiceClient
}

func (s *grpcLogSink) WriteBatch(ctx context.Context, logs []string) error {
	_, err := s.reporter.Report(ctx, &reporterv1.ReportRequest{
		ExecutionState: &executorv1.ExecutionState{Id: s.executionID},
		LogChunks:      logs,
		LogOnly:        true,
	})
	return err
}

func newTaskLogger(ctx context.Context, executionID int64,
	reporter reporterv1.ReporterServiceClient, logger *elog.Component) task.Logger {
	if reporter == nil {
		return nil
	}
	return tasklog.New(ctx, &grpcLogSink{executionID: executionID, reporter: reporter}, tasklog.Options{
		OnError: func(err error) {
			if logger != nil {
				logger.Error("上报任务日志失败", elog.FieldErr(err))
			}
		},
	})
}

type grpcProgressReporter struct {
	client reporterv1.ReporterServiceClient
}

func (r grpcProgressReporter) ReportProgress(ctx context.Context, taskInfo task.TaskInfo, progress int32) error {
	_, err := r.client.Report(ctx, &reporterv1.ReportRequest{ExecutionState: &executorv1.ExecutionState{
		Id: taskInfo.ExecutionID, TaskId: taskInfo.TaskID, TaskName: taskInfo.Name,
		Status: executorv1.ExecutionStatus_RUNNING, RunningProgress: progress,
		ExecutorNodeId: taskInfo.ExecutorNodeID,
	}})
	return err
}

type egoSystemLogger struct {
	logger *elog.Component
}

func newSystemLogger(logger *elog.Component, taskInfo task.TaskInfo) task.SystemLogger {
	if logger == nil {
		return nil
	}
	return egoSystemLogger{logger: logger.With(
		elog.Int64("executionID", taskInfo.ExecutionID),
		elog.Int64("taskID", taskInfo.TaskID),
		elog.String("taskName", taskInfo.Name),
	)}
}

func (l egoSystemLogger) Debug(message string, fields ...any) {
	l.logger.Debug(message, egoFields(fields)...)
}
func (l egoSystemLogger) Info(message string, fields ...any) {
	l.logger.Info(message, egoFields(fields)...)
}
func (l egoSystemLogger) Warn(message string, fields ...any) {
	l.logger.Warn(message, egoFields(fields)...)
}
func (l egoSystemLogger) Error(message string, fields ...any) {
	l.logger.Error(message, egoFields(fields)...)
}

func egoFields(values []any) []elog.Field {
	fields := make([]elog.Field, 0, len(values))
	for index := 0; index < len(values); index++ {
		if field, ok := values[index].(elog.Field); ok {
			fields = append(fields, field)
			continue
		}
		if key, ok := values[index].(string); ok && index+1 < len(values) {
			fields = append(fields, elog.Any(key, values[index+1]))
			index++
			continue
		}
		fields = append(fields, elog.Any("value", values[index]))
	}
	return fields
}
