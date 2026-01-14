package domain

// Report 进度上报结构
type Report struct {
	ExecutionState ExecutionState `json:"executionState"`
}

type ExecutionState struct {
	ID              int64               `json:"id"`              // 执行实例ID
	TaskID          int64               `json:"taskId"`          // 任务ID
	TaskName        string              `json:"taskName"`        // 任务名称
	Status          TaskExecutionStatus `json:"status"`          // 执行状态
	RunningProgress int32               `json:"runningProgress"` // 进度 0-100，RUNNING 状态才有意义
	// 执行节点请求调度节点执行重调度，调度节点无需调interrupt，调度节点可以直接重调度，因为此时执行节点必然已经停止了
	RequestReschedule bool              `json:"requestReschedule"`
	RescheduleParams  map[string]string `json:"rescheduleParams"`
	// 执行节点的 nodeID，用于记录是哪个节点处理了任务
	ExecutorNodeID string `json:"executorNodeId"`
}

type BatchReport struct {
	Reports []*Report `json:"reports"`
}
