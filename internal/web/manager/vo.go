package manager

import "github.com/Duke1616/etask/internal/domain"

type CreateTaskReq struct {
	Name                string                         `json:"name"`
	RunnerID            int64                          `json:"runner_id"`
	Type                string                         `json:"type"`      // 任务类型: RECURRING-定时任务, ONE_TIME-一次性任务
	CronExpr            string                         `json:"cron_expr"` // cron 表达式（定时任务必填，一次性任务可选用于定时触发）
	GrpcConfig          *GrpcConfig                    `json:"grpc_config"`
	HTTPConfig          *HTTPConfig                    `json:"http_config"`
	RetryConfig         *RetryConfig                   `json:"retry_config"`
	MaxExecutionSeconds int64                          `json:"max_execution_seconds"` // 最大执行秒数，默认24小时
	ScheduleParams      map[string]string              `json:"schedule_params"`       // 调度参数（如分页偏移量、处理进度等）
	Metadata            map[string]string              `json:"metadata"`              // 任务参数元数据
	Program             *ProgramSpec                   `json:"program"`               // 程序来源
	ParamOverrideRules  []domain.TaskParamOverrideRule `json:"param_override_rules"`
}

type ProgramSpec struct {
	Kind    string              `json:"kind"`
	Inline  *InlineProgramSpec  `json:"inline,omitempty"`
	Project *ProjectProgramSpec `json:"project,omitempty"`
}

type InlineProgramSpec struct {
	Code       string `json:"code,omitempty"`
	CodebookID int64  `json:"codebook_id,omitempty"`
}

type ProjectProgramSpec struct {
	EntryCodebookID int64 `json:"entry_codebook_id"`
}

type GrpcConfig struct {
	ServiceName string            `json:"service_name"` // 服务名称
	HandlerName string            `json:"handler_name"` // 执行节点支持的方法名称， 如 shell、python、demo
	Params      map[string]string `json:"params"`       // 传递参数
}

type HTTPConfig struct {
	Endpoint string            `json:"endpoint"`
	Headers  map[string]string `json:"headers"`
	Params   map[string]string `json:"params"`
}

type RetryConfig struct {
	MaxRetries      int32 `json:"max_retries"`
	InitialInterval int64 `json:"initial_interval"` // 毫秒
	MaxInterval     int64 `json:"max_interval"`     // 毫秒
}

type PageReq struct {
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}

type IdReq struct {
	ID int64 `json:"id"`
}

type RunTaskReq struct {
	ID             int64                              `json:"id"`
	CronExpr       string                             `json:"cron_expr,omitempty"`
	ParamOverrides map[string]domain.RunParamOverride `json:"param_overrides,omitempty"`
}

type UpdateTaskReq struct {
	ID                  int64                          `json:"id"`
	RunnerID            int64                          `json:"runner_id"`
	Name                string                         `json:"name"`
	Type                string                         `json:"type"`      // 任务类型: RECURRING-定时任务, ONE_TIME-一次性任务
	CronExpr            string                         `json:"cron_expr"` // cron 表达式（定时任务必填，一次性任务可选用于定时触发）
	GrpcConfig          *GrpcConfig                    `json:"grpc_config"`
	HTTPConfig          *HTTPConfig                    `json:"http_config"`
	RetryConfig         *RetryConfig                   `json:"retry_config"`
	MaxExecutionSeconds int64                          `json:"max_execution_seconds"` // 最大执行秒数，默认24小时
	ScheduleParams      map[string]string              `json:"schedule_params"`       // 调度参数
	Metadata            map[string]string              `json:"metadata"`              // 任务参数元数据
	Program             *ProgramSpec                   `json:"program"`               // 程序来源
	ParamOverrideRules  []domain.TaskParamOverrideRule `json:"param_override_rules"`
}

type TaskVO struct {
	ID                  int64                          `json:"id"`
	RunnerID            int64                          `json:"runner_id"`
	Name                string                         `json:"name"`
	Type                string                         `json:"type"`
	CronExpr            string                         `json:"cron_expr"`
	Status              string                         `json:"status"`
	NextTime            int64                          `json:"next_time"`
	MaxExecutionSeconds int64                          `json:"max_execution_seconds"`
	GrpcConfig          *GrpcConfig                    `json:"grpc_config"`
	HTTPConfig          *HTTPConfig                    `json:"http_config"`
	RetryConfig         *RetryConfig                   `json:"retry_config"`
	ScheduleParams      map[string]string              `json:"schedule_params"`
	CTime               int64                          `json:"ctime"`
	UTime               int64                          `json:"utime"`
	Metadata            map[string]string              `json:"metadata"`
	Program             *ProgramSpec                   `json:"program"`
	ParamOverrideRules  []domain.TaskParamOverrideRule `json:"param_override_rules"`
}

type ListTaskResp struct {
	Total int64    `json:"total"`
	Tasks []TaskVO `json:"tasks"`
}

type GetLogsReq struct {
	ExecutionID int64 `json:"execution_id" form:"execution_id"`
	MinID       int64 `json:"min_id" form:"min_id"`
	Limit       int   `json:"limit" form:"limit"`
}

type ListExecutionsReq struct {
	TaskID int64 `json:"task_id" form:"task_id"`
	Offset int   `json:"offset" form:"offset"`
	Limit  int   `json:"limit" form:"limit"`
}

type TerminateExecutionReq struct {
	Reason string `json:"reason"`
}

type TaskLogVO struct {
	ID          int64  `json:"id"`
	TaskID      int64  `json:"task_id"`
	ExecutionID int64  `json:"execution_id"`
	Content     string `json:"content"`
	CTime       int64  `json:"ctime"`
}

type ListLogResp struct {
	Total int64       `json:"total"`
	Logs  []TaskLogVO `json:"logs"`
}
type TaskExecutionVO struct {
	ID              int64  `json:"id"`
	TaskID          int64  `json:"task_id"`
	TaskName        string `json:"task_name"`
	StartTime       int64  `json:"start_time"`
	EndTime         int64  `json:"end_time"`
	Status          string `json:"status"`
	RunningProgress int32  `json:"running_progress"`
	ExecutorNodeId  string `json:"executor_node_id"`
	TaskResult      string `json:"task_result"`
	CTime           int64  `json:"ctime"`
}

type ListExecutionResp struct {
	Total      int64             `json:"total"`
	Executions []TaskExecutionVO `json:"executions"`
}
