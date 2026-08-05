package engine

import "github.com/Duke1616/etask/sdk/executor/internal/task"

type taskSystemLogger struct {
	next   task.SystemLogger
	fields []any
}

func scopedSystemLogger(logger task.SystemLogger, info task.TaskInfo) task.SystemLogger {
	if logger == nil {
		return nil
	}
	return taskSystemLogger{next: logger, fields: []any{
		"executionID", info.ExecutionID,
		"taskID", info.TaskID,
		"taskName", info.Name,
	}}
}

func (l taskSystemLogger) Debug(message string, fields ...any) {
	l.next.Debug(message, l.withTaskFields(fields)...)
}
func (l taskSystemLogger) Info(message string, fields ...any) {
	l.next.Info(message, l.withTaskFields(fields)...)
}
func (l taskSystemLogger) Warn(message string, fields ...any) {
	l.next.Warn(message, l.withTaskFields(fields)...)
}
func (l taskSystemLogger) Error(message string, fields ...any) {
	l.next.Error(message, l.withTaskFields(fields)...)
}

func (l taskSystemLogger) withTaskFields(fields []any) []any {
	values := make([]any, 0, len(l.fields)+len(fields))
	values = append(values, l.fields...)
	return append(values, fields...)
}

var _ task.SystemLogger = taskSystemLogger{}
