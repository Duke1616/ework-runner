package sse

import ssekit "github.com/Duke1616/etask/pkg/sse"

const (
	TASK_STATUS_CHANGE_EVENT = "task_status_change"
	TASK_LOG_EVENT           = "task_log"
	TASK_EXECUTION_EVENT     = "task_execution"
)

// TaskStatusEvent 任务状态变更事件定义
type TaskStatusEvent struct {
	TaskID   int64  `json:"task_id"`
	Status   string `json:"status"`
	NextTime int64  `json:"next_time"`
}

// TaskLogEvent 任务日志实时推送事件定义
type TaskLogEvent struct {
	ID          int64  `json:"id"`
	TaskID      int64  `json:"task_id"`
	ExecutionID int64  `json:"execution_id"`
	Content     string `json:"content"`
	CTime       int64  `json:"c_time"`
}

// TaskExecutionEvent 任务执行记录变更事件定义
type TaskExecutionEvent struct {
	ID              int64  `json:"id"`
	TaskID          int64  `json:"task_id"`
	TaskName        string `json:"task_name"`
	StartTime       int64  `json:"start_time"`
	EndTime         int64  `json:"end_time"`
	Status          string `json:"status"`
	RunningProgress int32  `json:"running_progress"`
	ExecutorNodeId  string `json:"executor_node_id"`
	TaskResult      string `json:"task_result"`
	CTime           int64  `json:"c_time"`
}

// Hubs 汇总调度中心进程内共享的实时事件通道。
type Hubs struct {
	Tasks      *ssekit.TopicHub[int64, TaskStatusEvent]
	Logs       *ssekit.TopicHub[int64, TaskLogEvent]
	Executions *ssekit.TopicHub[int64, TaskExecutionEvent]
}

// NewHubs 创建一组独立的实时事件通道。
func NewHubs() *Hubs {
	return &Hubs{
		Tasks:      ssekit.NewTopicHub[int64, TaskStatusEvent](),
		Logs:       ssekit.NewTopicHub[int64, TaskLogEvent](),
		Executions: ssekit.NewTopicHub[int64, TaskExecutionEvent](),
	}
}
