package preview

import "github.com/Duke1616/etask/internal/pkg/variable"

type Variable = variable.Item

type RunReq struct {
	RunnerID            int64             `json:"runner_id"`
	Params              map[string]string `json:"params"`
	Variables           []Variable        `json:"variables"`
	MaxExecutionSeconds int64             `json:"max_execution_seconds"`
}

type StatusReq struct {
	ExecutionID int64 `json:"execution_id"`
}

type LogsReq struct {
	ExecutionID int64 `json:"execution_id"`
	MinID       int64 `json:"min_id"`
	Limit       int   `json:"limit"`
}

type ExecutionVO struct {
	ID              int64  `json:"id"`
	TaskName        string `json:"task_name"`
	StartTime       int64  `json:"start_time"`
	EndTime         int64  `json:"end_time"`
	Status          string `json:"status"`
	RunningProgress int32  `json:"running_progress"`
	ExecutorNodeID  string `json:"executor_node_id"`
	TaskResult      string `json:"task_result"`
	CTime           int64  `json:"ctime"`
}

type LogVO struct {
	ID          int64  `json:"id"`
	ExecutionID int64  `json:"execution_id"`
	Content     string `json:"content"`
	CTime       int64  `json:"ctime"`
}

type LogsResp struct {
	Total int64   `json:"total"`
	Logs  []LogVO `json:"logs"`
}
