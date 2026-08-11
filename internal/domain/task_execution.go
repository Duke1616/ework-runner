package domain

import (
	"strconv"

	executorv1 "github.com/Duke1616/etask/api/proto/gen/etask/executor/v1"
)

// TaskExecutionSource 表示执行记录的业务来源。
type TaskExecutionSource string

const (
	// TaskExecutionSourceTask 表示由正式任务调度产生的执行记录。
	TaskExecutionSourceTask TaskExecutionSource = "TASK"
	// TaskExecutionSourceCodebookPreview 表示由 Codebook 试运行产生的临时执行记录。
	TaskExecutionSourceCodebookPreview TaskExecutionSource = "CODEBOOK_PREVIEW"
	// TaskExecutionSourceWorkflow 表示由外部工作流系统提交的正式执行记录。
	TaskExecutionSourceWorkflow TaskExecutionSource = "WORKFLOW"
)

// String 返回执行来源的字符串值。
func (s TaskExecutionSource) String() string {
	return string(s)
}

// IsValid 判断执行来源是否属于当前领域支持的类型。
func (s TaskExecutionSource) IsValid() bool {
	switch s {
	case TaskExecutionSourceTask, TaskExecutionSourceCodebookPreview, TaskExecutionSourceWorkflow:
		return true
	default:
		return false
	}
}

// IsCodebookPreview 判断当前执行是否来自 Codebook 试运行。
func (s TaskExecutionSource) IsCodebookPreview() bool {
	return s == TaskExecutionSourceCodebookPreview
}

// IsWorkflow 判断当前执行是否来自外部工作流系统。
func (s TaskExecutionSource) IsWorkflow() bool {
	return s == TaskExecutionSourceWorkflow
}

// AllowsEmptyTaskID 判断执行来源是否允许不关联 etask 正式任务。
func (s TaskExecutionSource) AllowsEmptyTaskID() bool {
	return s.IsCodebookPreview() || s.IsWorkflow()
}

// TaskExecutionStatus 任务执行状态
type TaskExecutionStatus string

const (
	TaskExecutionStatusUnknown           TaskExecutionStatus = "UNKNOWN"
	TaskExecutionStatusWaitingPull       TaskExecutionStatus = "WAITING_PULL"       // 等待边缘节点拉取
	TaskExecutionStatusPrepare           TaskExecutionStatus = "PREPARE"            // 已创建，准备执行
	TaskExecutionStatusRunning           TaskExecutionStatus = "RUNNING"            // 正在执行
	TaskExecutionStatusSuccess           TaskExecutionStatus = "SUCCESS"            // 执行成功
	TaskExecutionStatusFailed            TaskExecutionStatus = "FAILED"             // 执行失败（不可重试）
	TaskExecutionStatusFailedRetryable   TaskExecutionStatus = "FAILED_RETRYABLE"   // 执行失败（可重试）
	TaskExecutionStatusFailedRescheduled TaskExecutionStatus = "FAILED_RESCHEDULED" // 执行失败（重调度）
	TaskExecutionStatusCancelled         TaskExecutionStatus = "CANCELLED"          // 外部请求强制终止
)

func (t TaskExecutionStatus) String() string {
	return string(t)
}

func (t TaskExecutionStatus) IsValid() bool {
	switch t {
	case TaskExecutionStatusWaitingPull,
		TaskExecutionStatusPrepare,
		TaskExecutionStatusRunning,
		TaskExecutionStatusSuccess,
		TaskExecutionStatusFailed,
		TaskExecutionStatusFailedRetryable,
		TaskExecutionStatusFailedRescheduled,
		TaskExecutionStatusCancelled:
		return true
	default:
		return false
	}
}

func TaskExecutionStatusFromProto(status executorv1.ExecutionStatus) TaskExecutionStatus {
	switch status {
	case executorv1.ExecutionStatus_RUNNING:
		return TaskExecutionStatusRunning
	case executorv1.ExecutionStatus_SUCCESS:
		return TaskExecutionStatusSuccess
	case executorv1.ExecutionStatus_FAILED:
		return TaskExecutionStatusFailed
	case executorv1.ExecutionStatus_FAILED_RETRYABLE:
		return TaskExecutionStatusFailedRetryable
	case executorv1.ExecutionStatus_FAILED_RESCHEDULABLE:
		return TaskExecutionStatusFailedRescheduled
	case executorv1.ExecutionStatus_CANCELLED:
		return TaskExecutionStatusCancelled
	default:
		return TaskExecutionStatusUnknown
	}
}

func (t TaskExecutionStatus) IsPrepare() bool {
	return t == TaskExecutionStatusPrepare
}

func (t TaskExecutionStatus) IsRunning() bool {
	return t == TaskExecutionStatusRunning
}

func (t TaskExecutionStatus) IsSuccess() bool {
	return t == TaskExecutionStatusSuccess
}

func (t TaskExecutionStatus) IsFailed() bool {
	return t == TaskExecutionStatusFailed
}

func (t TaskExecutionStatus) IsFailedRetryable() bool {
	return t == TaskExecutionStatusFailedRetryable
}

func (t TaskExecutionStatus) IsFailedRescheduled() bool {
	return t == TaskExecutionStatusFailedRescheduled
}

func (t TaskExecutionStatus) IsCancelled() bool {
	return t == TaskExecutionStatusCancelled
}

func (t TaskExecutionStatus) IsTerminalStatus() bool {
	return t.IsSuccess() || t.IsFailed() || t.IsCancelled()
}

// NonTerminalTaskExecutionStatuses 返回允许继续迁移的执行状态。
// 返回新切片，避免调用方修改共享状态集合。
func NonTerminalTaskExecutionStatuses() []TaskExecutionStatus {
	return []TaskExecutionStatus{
		TaskExecutionStatusWaitingPull,
		TaskExecutionStatusPrepare,
		TaskExecutionStatusRunning,
		TaskExecutionStatusFailedRetryable,
		TaskExecutionStatusFailedRescheduled,
	}
}

// ExecutionVariableSet 保存本次执行固定使用的变量集合。
type ExecutionVariableSet struct {
	Items []RunnerVariable `json:"items"`
}

// ToProto 转换为执行器协议使用的变量快照。
func (s *ExecutionVariableSet) ToProto() *executorv1.VariableSet {
	if s == nil {
		return nil
	}
	items := make([]*executorv1.Variable, 0, len(s.Items))
	for _, variable := range s.Items {
		items = append(items, &executorv1.Variable{
			Key: variable.Key, Value: variable.Value, Secret: variable.Secret,
		})
	}
	return &executorv1.VariableSet{Items: items}
}

// TaskExecution 任务执行记录
type TaskExecution struct {
	ID              int64
	TenantID        int64                 // 租户ID，多租户隔离
	Source          TaskExecutionSource   // 执行来源，区分正式任务与临时试运行
	RequestID       string                // 外部来源提供的幂等请求标识
	Deadline        int64                 // 任务执行截止时间（毫秒时间戳）
	ExecutorNodeID  string                // 执行节点的 nodeID，用于记录是哪个节点处理了任务
	StartTime       int64                 // 开始时间
	EndTime         int64                 // 结束时间
	RetryCount      int64                 // 已重试次数
	NextRetryTime   int64                 // 下次重试时间
	RunningProgress int32                 // 进度 0-100，RUNNING 状态才有意义
	Status          TaskExecutionStatus   // 执行状态
	TaskResult      string                // 任务执行的结构化结果（JSON格式）
	CTime           int64                 // 创建时间
	UTime           int64                 // 更新时间
	Task            Task                  // 创建时刻从Task冗余的信息
	Program         *Program              // 本次执行固定的程序来源；非程序型 Handler 为空
	Artifacts       []ArtifactRef         // 本次执行固定的 SYSTEM 和具名依赖制品层
	Variables       *ExecutionVariableSet // 本次执行固定的变量快照；普通 Handler 可不提供
	Route           ExecutionRoute        // 创建时固定的传输和派发路由
}

func (te *TaskExecution) MergeTaskScheduleParams(scheduleParams map[string]string) {
	if len(scheduleParams) == 0 {
		return
	}
	if len(te.Task.ScheduleParams) == 0 {
		te.Task.ScheduleParams = scheduleParams
	} else {
		// 覆盖
		for k, v := range scheduleParams {
			te.Task.ScheduleParams[k] = v
		}
	}
}

// GRPCParams 获取gRPC执行参数（业务参数 + 调度参数）
// 调度参数优先级更高，会覆盖同名的业务参数
func (te *TaskExecution) GRPCParams() map[string]string {
	result := make(map[string]string)

	// 1. 先添加业务参数
	if te.Task.GrpcConfig != nil && te.Task.GrpcConfig.Params != nil {
		for k, v := range te.Task.GrpcConfig.Params {
			result[k] = v
		}
	}

	// 2. 添加/覆盖调度参数（优先级更高）
	if te.Task.ScheduleParams != nil {
		for k, v := range te.Task.ScheduleParams {
			result[k] = v
		}
	}

	// 3. 添加任务执行超时参数
	result["max_execution_seconds"] = strconv.FormatInt(te.Task.MaxExecutionSeconds, 10)

	return result
}

func ExecutionStateFromProto(protoState *executorv1.ExecutionState) ExecutionState {
	if protoState == nil {
		return ExecutionState{}
	}
	return ExecutionState{
		ID:                protoState.GetId(),
		TaskID:            protoState.GetTaskId(),
		TaskName:          protoState.GetTaskName(),
		Status:            TaskExecutionStatusFromProto(protoState.GetStatus()),
		RunningProgress:   protoState.GetRunningProgress(),
		RequestReschedule: protoState.GetRequestReschedule(),
		RescheduleParams:  protoState.GetRescheduledParams(),
		ExecutorNodeID:    protoState.GetExecutorNodeId(),
		TaskResult:        protoState.GetTaskResult(),
	}
}
