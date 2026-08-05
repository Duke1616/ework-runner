package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/ecodeclub/mq-api"
)

//go:generate go tool mockgen -source=./consumer.go -package=executionmocks -destination=./mocks/consumer.mock.go -typed

// EventHandler 定义 Kafka 事件接入所需的应用层能力。
type EventHandler interface {
	// FindByID 查询事件对应的任务执行快照，用于避免重复处理已完成的执行。
	FindByID(ctx context.Context, id int64) (domain.TaskExecution, error)
	// AppendExecutionLogs 持久化一批增量日志并广播给 SSE 订阅者。
	AppendExecutionLogs(ctx context.Context, executionID, taskID int64, logs []string) error
	// UpdateState 将执行终态交给统一状态机处理。
	UpdateState(ctx context.Context, state domain.ExecutionState) error
}

// EventConsumer 将 Agent 事件分别路由到日志服务和执行状态机。
type EventConsumer struct {
	executions EventHandler
	mu         sync.Mutex
	processed  map[string]*eventEntry
}

type eventEntry struct {
	startedAt time.Time
	done      chan struct{}
	err       error
}

// NewEventConsumer 创建 Scheduler 侧 Agent 事件消费者。
func NewEventConsumer(executions EventHandler) *EventConsumer {
	return &EventConsumer{executions: executions, processed: make(map[string]*eventEntry)}
}

// Consume 校验并分发一条严格格式的执行事件。
func (c *EventConsumer) Consume(ctx context.Context, message *mq.Message) error {
	var event Event
	if err := json.Unmarshal(message.Value, &event); err != nil {
		return fmt.Errorf("解析 Agent 执行事件失败: %w", err)
	}
	if err := event.Validate(); err != nil {
		return err
	}
	entry, owner := c.begin(event.EventID)
	if !owner {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-entry.done:
			return entry.err
		}
	}
	var handleErr error
	defer func() { c.finish(event.EventID, entry, handleErr) }()
	if event.Type == EventTypeLogBatch {
		if err := c.executions.AppendExecutionLogs(ctx, event.State.ID, event.State.TaskID, event.Logs); err != nil {
			handleErr = err
			return err
		}
		return nil
	}
	if len(event.Logs) > 0 {
		if err := c.executions.AppendExecutionLogs(ctx, event.State.ID, event.State.TaskID, event.Logs); err != nil {
			handleErr = err
			return err
		}
	}
	execution, err := c.executions.FindByID(ctx, event.State.ID)
	if err != nil {
		handleErr = err
		return err
	}
	if execution.Status.IsTerminalStatus() {
		return nil
	}
	if err = c.executions.UpdateState(ctx, event.State); err != nil {
		handleErr = err
		return err
	}
	return nil
}

func (c *EventConsumer) begin(eventID string) (*eventEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for id, entry := range c.processed {
		select {
		case <-entry.done:
			if now.Sub(entry.startedAt) >= 30*time.Minute {
				delete(c.processed, id)
			}
		default:
		}
	}
	if entry := c.processed[eventID]; entry != nil {
		return entry, false
	}
	entry := &eventEntry{startedAt: now, done: make(chan struct{})}
	c.processed[eventID] = entry
	return entry, true
}

func (c *EventConsumer) finish(eventID string, entry *eventEntry, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry.err = err
	if err != nil {
		if current := c.processed[eventID]; current == entry {
			delete(c.processed, eventID)
		}
	}
	close(entry.done)
}
