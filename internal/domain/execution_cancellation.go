package domain

// CancellationDeliveryStatus 描述终止信号的持久化投递阶段。
type CancellationDeliveryStatus string

const (
	CancellationWaitingExecution CancellationDeliveryStatus = "WAITING_EXECUTION"
	CancellationPending          CancellationDeliveryStatus = "PENDING"
	CancellationSent             CancellationDeliveryStatus = "SENT"
)

// ExecutionCancellation 是按工作流 request ID 幂等保存的取消意图。
type ExecutionCancellation struct {
	ID             int64
	TenantID       int64
	RequestID      string
	ExecutionID    int64
	Reason         string
	DeliveryStatus CancellationDeliveryStatus
	AttemptCount   int
	NextAttemptAt  int64
	LastError      string
	CTime          int64
	UTime          int64
}
