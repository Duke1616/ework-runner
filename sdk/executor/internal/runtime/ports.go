package runtime

// 本文件定义 Runtime 依赖的窄接口。

import (
	"context"

	executorv1 "github.com/Duke1616/etask/api/proto/gen/etask/executor/v1"
)

type executionStore interface {
	// Begin 登记运行状态，相同 ID 正在运行时拒绝重复启动。
	Begin(state *executorv1.ExecutionState, cancel context.CancelCauseFunc) (*executorv1.ExecutionState, bool)
	// Finish 保存执行终态和任务结果。
	Finish(id int64, status executorv1.ExecutionStatus, result string) (*executorv1.ExecutionState, bool)
	// Get 查询执行状态副本。
	Get(id int64) (*executorv1.ExecutionState, bool)
	// Cancel 取消运行中的执行。
	Cancel(id int64) (*executorv1.ExecutionState, bool)
	// Terminate 将执行置为不可恢复的 CANCELLED 终态。
	Terminate(id int64, reason string) (*executorv1.ExecutionState, bool)
}
