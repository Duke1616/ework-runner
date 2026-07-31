package execution

import (
	"fmt"
	"strings"

	"github.com/Duke1616/etask/internal/domain"
)

const (
	// EventTopic 承载有序的 Agent 日志和终态事件。
	EventTopic    = "execution_result_events"
	SchemaVersion = 1

	EventTypeLogBatch EventType = "LOG_BATCH"
	EventTypeFinished EventType = "FINISHED"
)

type EventType string

// Event 是 Agent 写入统一结果 Topic 的严格版本化事件。
// LOG_BATCH 的 Logs 是增量日志；FINISHED 的 Logs 仅包含实时发送失败的尾部日志。
type Event struct {
	Version    int                   `json:"version"`
	DispatchID string                `json:"dispatch_id"`
	EventID    string                `json:"event_id"`
	Type       EventType             `json:"type"`
	Sequence   uint64                `json:"sequence,omitempty"`
	State      domain.ExecutionState `json:"state"`
	Logs       []string              `json:"logs,omitempty"`
}

// LogBatch 是 Publisher 接收的增量日志命令。
type LogBatch struct {
	DispatchID string
	Sequence   uint64
	State      domain.ExecutionState
	Logs       []string
}

// Finished 是 Publisher 接收的执行终态命令。
type Finished struct {
	DispatchID  string
	State       domain.ExecutionState
	PendingLogs []string
}

func NewLogBatchEvent(batch LogBatch) Event {
	return Event{
		Version: SchemaVersion, DispatchID: batch.DispatchID,
		EventID: LogEventID(batch.DispatchID, batch.Sequence),
		Type:    EventTypeLogBatch, Sequence: batch.Sequence,
		State: batch.State, Logs: batch.Logs,
	}
}

func NewFinishedEvent(finished Finished) Event {
	return Event{
		Version: SchemaVersion, DispatchID: finished.DispatchID,
		EventID: FinishedEventID(finished.DispatchID), Type: EventTypeFinished,
		State: finished.State, Logs: finished.PendingLogs,
	}
}

// Validate 校验事件信封及其类型专属字段。
func (e Event) Validate() error {
	if e.Version != SchemaVersion {
		return fmt.Errorf("Agent 执行事件版本非法: %d", e.Version)
	}
	if strings.TrimSpace(e.DispatchID) == "" || strings.TrimSpace(e.EventID) == "" || e.State.ID <= 0 {
		return fmt.Errorf("Agent 执行事件身份信息非法: dispatch_id=%q event_id=%q execution_id=%d",
			e.DispatchID, e.EventID, e.State.ID)
	}
	switch e.Type {
	case EventTypeLogBatch:
		if e.Sequence == 0 || len(e.Logs) == 0 {
			return fmt.Errorf("Agent 日志事件内容非法: event_id=%q sequence=%d logs=%d",
				e.EventID, e.Sequence, len(e.Logs))
		}
		if e.EventID != LogEventID(e.DispatchID, e.Sequence) {
			return fmt.Errorf("Agent 日志事件 ID 非法: %q", e.EventID)
		}
	case EventTypeFinished:
		if e.Sequence != 0 || e.EventID != FinishedEventID(e.DispatchID) || !e.State.Status.IsTerminalStatus() {
			return fmt.Errorf("Agent 终态事件内容非法: event_id=%q sequence=%d status=%s",
				e.EventID, e.Sequence, e.State.Status)
		}
	default:
		return fmt.Errorf("Agent 执行事件类型非法: %s", e.Type)
	}
	return nil
}

func LogEventID(dispatchID string, sequence uint64) string {
	return fmt.Sprintf("%s:log:%d", dispatchID, sequence)
}

func FinishedEventID(dispatchID string) string {
	return dispatchID + ":finished"
}
