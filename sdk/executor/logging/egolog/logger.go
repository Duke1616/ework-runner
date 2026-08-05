// Package egolog adapts EGO's logger to the Executor SDK logging contract.
package egolog

import (
	"github.com/Duke1616/etask/sdk/executor"
	"github.com/gotomicro/ego/core/elog"
)

type logger struct {
	inner *elog.Component
}

// New adapts an EGO component logger to executor.SystemLogger.
func New(inner *elog.Component) executor.SystemLogger {
	if inner == nil {
		return nil
	}
	return logger{inner: inner}
}

func (l logger) Debug(message string, fields ...any) { l.inner.Debug(message, egoFields(fields)...) }
func (l logger) Info(message string, fields ...any)  { l.inner.Info(message, egoFields(fields)...) }
func (l logger) Warn(message string, fields ...any)  { l.inner.Warn(message, egoFields(fields)...) }
func (l logger) Error(message string, fields ...any) { l.inner.Error(message, egoFields(fields)...) }

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

var _ executor.SystemLogger = logger{}
