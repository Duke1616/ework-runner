package event

import (
	"context"
	"sync"
	"time"

	internaldomain "github.com/Duke1616/etask/internal/domain"
	executionevent "github.com/Duke1616/etask/internal/event/execution"
	"github.com/Duke1616/etask/pkg/tasklog"
	"github.com/gotomicro/ego/core/elog"
)

type kafkaTaskLogger struct {
	*tasklog.Logger
}

type kafkaLogSink struct {
	publisher  ExecutionEventPublisher
	dispatchID string
	execution  internaldomain.ExecutionState

	mu       sync.Mutex
	sequence uint64
}

func newKafkaTaskLogger(ctx context.Context, publisher ExecutionEventPublisher,
	command ExecuteCommand, logger *elog.Component) *kafkaTaskLogger {
	return newKafkaTaskLoggerWithOptions(ctx, publisher, command, logger,
		tasklog.DefaultBufferSize, tasklog.DefaultFlushPeriod)
}

func newKafkaTaskLoggerWithOptions(ctx context.Context, publisher ExecutionEventPublisher,
	command ExecuteCommand, logger *elog.Component, bufferSize int, interval time.Duration) *kafkaTaskLogger {
	sink := &kafkaLogSink{
		publisher: publisher, dispatchID: command.DispatchID,
		execution: internaldomain.ExecutionState{
			ID: command.ExecutionID, TaskID: command.TaskID, TaskName: command.TaskName,
		},
	}
	buffer := tasklog.New(ctx, sink, tasklog.Options{
		BufferSize: bufferSize, FlushPeriod: interval,
		OnError: func(err error) {
			if logger != nil {
				logger.Error("发布 Agent 实时日志失败",
					elog.Int64("executionID", command.ExecutionID), elog.FieldErr(err))
			}
		},
	})
	return &kafkaTaskLogger{Logger: buffer}
}

func (s *kafkaLogSink) WriteBatch(ctx context.Context, logs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	nextSequence := s.sequence + 1
	err := s.publisher.PublishLogs(ctx, executionevent.LogBatch{
		DispatchID: s.dispatchID,
		Sequence:   nextSequence,
		State:      s.execution,
		Logs:       logs,
	})
	if err == nil {
		s.sequence = nextSequence
	}
	return err
}
