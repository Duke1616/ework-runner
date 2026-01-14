package errs

import "errors"

var (
	ErrTaskPreemptFailed              = errors.New("任务抢占失败")
	ErrTaskRenewFailed                = errors.New("任务续约失败")
	ErrTaskReleaseFailed              = errors.New("任务释放失败")
	ErrTaskUpdateNextTimeFailed       = errors.New("任务更新下次执行时间失败")
	ErrTaskUpdateScheduleParamsFailed = errors.New("任务更新调度参数失败")

	ErrExecutionNotFound            = errors.New("执行记录不存在")
	ErrInvalidTaskExecutionStatus   = errors.New("执行记录状态非法")
	ErrInterruptTaskExecutionFailed = errors.New("中断任务执行失败")

	ErrInvalidTaskCronExpr        = errors.New("无效的cron表达式")
	ErrInvalidTaskScheduleNodeID  = errors.New("无效的调度节点ID")
	ErrInvalidTaskExecutionMethod = errors.New("任务执行方式非法")
	ErrInvalidTaskShardingRule    = errors.New("分片规则非法")
	ErrTaskShardingRuleNotFound   = errors.New("分片规则未找到")

	ErrSetExecutionStateRunningFailed        = errors.New("设置运行状态失败")
	ErrUpdateExecutionStatusFailed           = errors.New("更新任务执行记录状态失败")
	ErrUpdateExecutionStatusAndEndTimeFailed = errors.New("更新任务执行记录状态和结束时间失败")
	ErrUpdateExecutionRunningProgressFailed  = errors.New("更新任务执行记录的运行状态失败")
	ErrUpdateExecutionRetryResultFailed      = errors.New("更新任务执行记录的重试结果失败")

	ErrExecutionMaxRetriesExceeded   = errors.New("超过最大重试次数")
	ErrExecutionStateHandlerNotFound = errors.New("执行状态处理器未找到")

	ErrInitPlanFailed = errors.New("plan和实际创建的任务不符")
	ErrExceedLimit    = errors.New("抢资源超出限制")
)
