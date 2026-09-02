package dao

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Duke1616/eiam/pkg/gormx"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrCancellationTerminal = errors.New("execution 已经进入其他终态")
)

// ExecutionCancellation 持久化执行取消意图及其物理信号投递状态。
type ExecutionCancellation struct {
	ID             int64         `gorm:"primaryKey;column:id;type:bigint;autoIncrement"`
	TenantID       int64         `gorm:"column:tenant_id;type:bigint unsigned;not null;uniqueIndex:uk_execution_cancellation_request,priority:1;index"`
	RequestID      string        `gorm:"column:request_id;type:varchar(128);not null;uniqueIndex:uk_execution_cancellation_request,priority:2"`
	ExecutionID    sql.NullInt64 `gorm:"column:execution_id;type:bigint;uniqueIndex"`
	Reason         string        `gorm:"column:reason;type:varchar(500);not null"`
	DeliveryStatus string        `gorm:"column:delivery_status;type:ENUM('WAITING_EXECUTION','PENDING','SENT');not null;index:idx_cancellation_delivery,priority:1"`
	AttemptCount   int           `gorm:"column:attempt_count;type:int;not null;default:0"`
	NextAttemptAt  int64         `gorm:"column:next_attempt_at;type:bigint;not null;default:0;index:idx_cancellation_delivery,priority:2"`
	LastError      string        `gorm:"column:last_error;type:text"`
	CTime          int64         `gorm:"column:ctime;type:bigint;not null"`
	UTime          int64         `gorm:"column:utime;type:bigint;not null"`
}

func (ExecutionCancellation) TableName() string { return "execution_cancellations" }

type ExecutionCancellationDAO interface {
	// Request 按 execution ID 或 request ID 锁定并保存取消意图。
	Request(ctx context.Context, executionID int64, requestID, reason string) error
	// Attach 将当前租户下早到的取消意图绑定到新建 execution。
	Attach(ctx context.Context, executionID int64, requestID string) (bool, error)
	// ListPending 跨租户查询到期的待投递记录，调用方需使用无租户 Context。
	ListPending(ctx context.Context, limit int) ([]ExecutionCancellation, error)
	// MarkSent 将当前租户下的待投递记录标记为已发送。
	MarkSent(ctx context.Context, id int64) error
	// MarkFailed 记录当前租户下的投递错误和下次重试时间。
	MarkFailed(ctx context.Context, id int64, reason string, nextAttemptAt int64) error
}

type GORMExecutionCancellationDAO struct{ db *gorm.DB }

func NewGORMExecutionCancellationDAO(db *gorm.DB) ExecutionCancellationDAO {
	return &GORMExecutionCancellationDAO{db: db}
}

func (g *GORMExecutionCancellationDAO) Request(ctx context.Context, executionID int64,
	requestID, reason string) error {
	return g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		resolvedRequestID, err := resolveCancellationRequestID(tx, executionID, requestID)
		if err != nil {
			return err
		}
		// 所有入口固定先锁取消记录、再锁 execution，避免 Request 与 Attach 形成反向锁序。
		cancellation, err := lockOrCreateCancellation(tx, resolvedRequestID, reason)
		if err != nil {
			return err
		}
		execution, err := lockCancellationTarget(tx, executionID, resolvedRequestID)
		if errors.Is(err, gorm.ErrRecordNotFound) && executionID <= 0 {
			return keepCancellationWaiting(tx, cancellation.ID, reason)
		}
		if err != nil {
			return err
		}
		return bindCancellation(tx, cancellation, execution, reason)
	})
}

func (g *GORMExecutionCancellationDAO) Attach(ctx context.Context, executionID int64,
	requestID string) (bool, error) {
	attached := false
	err := g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		cancellation, err := lockCancellation(tx, requestID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		var execution TaskExecution
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", executionID).First(&execution).Error; err != nil {
			return err
		}
		if err := bindCancellation(tx, cancellation, execution, cancellation.Reason); err != nil {
			return err
		}
		attached = true
		return nil
	})
	return attached, err
}

func resolveCancellationRequestID(tx *gorm.DB, executionID int64, requestID string) (string, error) {
	if executionID <= 0 {
		return requestID, nil
	}
	var execution TaskExecution
	if err := tx.Select("id", "source", "request_id").
		Where("id = ?", executionID).First(&execution).Error; err != nil {
		return "", err
	}
	resolved := fmt.Sprintf("etask:%s:%d", execution.Source, execution.ID)
	if execution.Source == domain.TaskExecutionSourceWorkflow.String() {
		if !execution.RequestID.Valid || execution.RequestID.String == "" {
			return "", fmt.Errorf("WORKFLOW execution %d 缺少 request ID", executionID)
		}
		resolved = execution.RequestID.String
	}
	if requestID != "" && requestID != resolved {
		return "", fmt.Errorf("execution ID 与 request ID 不匹配")
	}
	return resolved, nil
}

func lockCancellationTarget(tx *gorm.DB, executionID int64, requestID string) (TaskExecution, error) {
	var execution TaskExecution
	query := tx.Clauses(clause.Locking{Strength: "UPDATE"})
	if executionID > 0 {
		err := query.Where("id = ?", executionID).First(&execution).Error
		return execution, err
	}
	err := query.Where("source = ? AND request_id = ?",
		domain.TaskExecutionSourceWorkflow.String(), requestID).First(&execution).Error
	return execution, err
}

func lockCancellation(tx *gorm.DB, requestID string) (ExecutionCancellation, error) {
	var cancellation ExecutionCancellation
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("request_id = ?", requestID).
		First(&cancellation).Error
	return cancellation, err
}

func lockOrCreateCancellation(tx *gorm.DB, requestID, reason string) (ExecutionCancellation, error) {
	now := time.Now().UnixMilli()
	cancellation := ExecutionCancellation{
		RequestID: requestID, Reason: reason,
		DeliveryStatus: string(domain.CancellationWaitingExecution), CTime: now, UTime: now,
	}
	if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{
		{Name: "tenant_id"}, {Name: "request_id"},
	}, DoNothing: true}).Create(&cancellation).Error; err != nil {
		return ExecutionCancellation{}, err
	}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("request_id = ?", requestID).
		First(&cancellation).Error; err != nil {
		return ExecutionCancellation{}, err
	}
	return cancellation, nil
}

func keepCancellationWaiting(tx *gorm.DB, cancellationID int64, reason string) error {
	now := time.Now().UnixMilli()
	return tx.Model(&ExecutionCancellation{}).Where("id = ?", cancellationID).Updates(map[string]any{
		"reason": reason, "delivery_status": domain.CancellationWaitingExecution,
		"last_error": "", "next_attempt_at": 0, "utime": now,
	}).Error
}

func bindCancellation(tx *gorm.DB, cancellation ExecutionCancellation,
	execution TaskExecution, reason string) error {
	status := domain.TaskExecutionStatus(execution.Status)
	if !status.IsValid() || (status.IsTerminalStatus() && !status.IsCancelled()) {
		return ErrCancellationTerminal
	}
	if !status.IsCancelled() {
		if err := cancelExecution(tx, execution.ID, reason); err != nil {
			return err
		}
		// 任务抢占状态不能依赖执行器回传完成事件才能释放。
		// 执行器可能已经断开，或者 HTTP/Local 路由根本没有独立的终止控制面；
		// 此时取消事务仍应让正式任务恢复到可调度状态。
		if err := releaseTaskOnCancellation(tx, execution); err != nil {
			return err
		}
	}
	return scheduleCancellationDelivery(tx, cancellation, execution.ID, reason)
}

// releaseTaskOnCancellation 在取消执行的同一事务中释放其抢占的正式任务。
// 调度节点条件是必要的：取消请求可能晚于一次新的抢占到达，不能清除新节点的锁。
// 一次性任务不应重新进入调度队列，因此直接置为 COMPLETED；手动 Start/Retry 仍可再次执行。
func releaseTaskOnCancellation(tx *gorm.DB, execution TaskExecution) error {
	if execution.TaskID <= 0 || execution.TaskScheduleNodeID == "" {
		return nil
	}
	status := StatusActive
	if execution.TaskType == string(domain.TaskTypeOneTime) {
		status = StatusCompleted
	}
	result := tx.Model(&Task{}).
		Where("id = ? AND status = ? AND schedule_node_id = ?",
			execution.TaskID, StatusPreempted, execution.TaskScheduleNodeID).
		Updates(map[string]any{
			"status":           status,
			"schedule_node_id": gorm.Expr("NULL"),
			"version":          gorm.Expr("version + 1"),
			"utime":            time.Now().UnixMilli(),
		})
	if result.Error != nil {
		return fmt.Errorf("取消执行后释放任务失败: %w", result.Error)
	}
	// RowsAffected 为 0 是幂等场景（任务已经释放、被停用或由其他节点重新抢占），
	// 不应阻止取消状态提交。
	return nil
}

func cancelExecution(tx *gorm.DB, executionID int64, reason string) error {
	now := time.Now().UnixMilli()
	result := withExecutionStatusCAS(tx.Model(&TaskExecution{}), executionID,
		cancellableExecutionStatuses()).Updates(map[string]any{
		"status": TaskExecutionStatusCancelled, "etime": now,
		"task_result": reason, "utime": now,
	})
	if result.Error != nil {
		return fmt.Errorf("%w: %v", errs.ErrUpdateExecutionStatusFailed, result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrCancellationTerminal
	}
	return nil
}

func scheduleCancellationDelivery(tx *gorm.DB, cancellation ExecutionCancellation,
	executionID int64, reason string) error {
	now := time.Now().UnixMilli()
	deliveryStatus := string(domain.CancellationPending)
	nextAttemptAt := now
	if cancellation.DeliveryStatus == string(domain.CancellationSent) {
		deliveryStatus = string(domain.CancellationSent)
		nextAttemptAt = 0
		reason = cancellation.Reason
	}
	return tx.Model(&ExecutionCancellation{}).Where("id = ?", cancellation.ID).
		Updates(map[string]any{
			"execution_id": executionID, "reason": reason, "delivery_status": deliveryStatus,
			"last_error": "", "next_attempt_at": nextAttemptAt, "utime": now,
		}).Error
}

func cancellableExecutionStatuses() []string {
	statuses := domain.NonTerminalTaskExecutionStatuses()
	result := make([]string, 0, len(statuses))
	for _, status := range statuses {
		result = append(result, status.String())
	}
	return result
}

func (g *GORMExecutionCancellationDAO) ListPending(ctx context.Context,
	limit int) ([]ExecutionCancellation, error) {
	var cancellations []ExecutionCancellation
	err := g.db.WithContext(ctx).
		Scopes(gormx.IgnoreTenant()).
		Where("delivery_status = ? AND execution_id IS NOT NULL AND next_attempt_at <= ?",
			domain.CancellationPending, time.Now().UnixMilli()).
		Order("next_attempt_at ASC, id ASC").Limit(limit).Find(&cancellations).Error
	return cancellations, err
}

func (g *GORMExecutionCancellationDAO) MarkSent(ctx context.Context, id int64) error {
	return pendingCancellation(g.db.WithContext(ctx), id).Updates(map[string]any{
		"delivery_status": domain.CancellationSent, "last_error": "",
		"next_attempt_at": 0, "utime": time.Now().UnixMilli(),
	}).Error
}

func (g *GORMExecutionCancellationDAO) MarkFailed(ctx context.Context, id int64, reason string,
	nextAttemptAt int64) error {
	return pendingCancellation(g.db.WithContext(ctx), id).Updates(map[string]any{
		"attempt_count": gorm.Expr("attempt_count + 1"), "last_error": reason,
		"next_attempt_at": nextAttemptAt, "utime": time.Now().UnixMilli(),
	}).Error
}

func pendingCancellation(db *gorm.DB, id int64) *gorm.DB {
	return db.Model(&ExecutionCancellation{}).
		Where("id = ? AND delivery_status = ?", id, domain.CancellationPending)
}
