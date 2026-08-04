package task

// 本文件实现任务日志缓冲、脱敏和上报。

import (
	"context"
	"fmt"
	"strings"

	executorv1 "github.com/Duke1616/etask/api/proto/gen/etask/executor/v1"
	reporterv1 "github.com/Duke1616/etask/api/proto/gen/etask/reporter/v1"
	"github.com/Duke1616/etask/pkg/tasklog"
	"github.com/gotomicro/ego/core/elog"
)

// Logger 任务日志记录器接口
type Logger interface {
	// Log 记录一条支持格式化参数的任务日志。
	Log(format string, args ...any)
	// Close 刷新剩余日志并释放后台资源。
	Close()
}

type maskingTaskLogger struct {
	next  Logger
	masks []string
}

func newMaskingTaskLogger(next Logger, masks []string) Logger {
	if len(masks) == 0 {
		return next
	}
	return &maskingTaskLogger{next: next, masks: masks}
}

func (l *maskingTaskLogger) Log(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	for _, mask := range l.masks {
		message = strings.ReplaceAll(message, mask, "[MASKED]")
	}
	l.next.Log("%s", message)
}

func (l *maskingTaskLogger) Close() {
	l.next.Close()
}

type grpcLogSink struct {
	executionID int64
	reporter    reporterv1.ReporterServiceClient
}

func (s *grpcLogSink) WriteBatch(ctx context.Context, logs []string) error {
	if s.reporter == nil {
		return nil
	}
	_, err := s.reporter.Report(ctx, &reporterv1.ReportRequest{
		ExecutionState: &executorv1.ExecutionState{
			Id: s.executionID,
		},
		LogChunks: logs,
		LogOnly:   true,
	})
	return err
}

func newTaskLogger(ctx context.Context, executionID int64,
	reporter reporterv1.ReporterServiceClient, sysLogger *elog.Component) Logger {
	return tasklog.New(ctx, &grpcLogSink{executionID: executionID, reporter: reporter}, tasklog.Options{
		OnError: func(err error) {
			if sysLogger != nil {
				sysLogger.Error("上报任务日志失败", elog.FieldErr(err))
			}
		},
	})
}
