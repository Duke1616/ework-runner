package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/pkg/security"
	"github.com/Duke1616/etask/internal/repository/dao"
	"github.com/Duke1616/etask/pkg/sqlx"
)

type TaskRepository interface {
	// Create 创建任务
	Create(ctx context.Context, task domain.Task) (domain.Task, error)
	// GetByID 根据ID获取任务
	GetByID(ctx context.Context, id int64) (domain.Task, error)
	// GetByName 根据名称获取任务
	GetByName(ctx context.Context, name string) (domain.Task, error)
	// SchedulableTasks 获取可调度的任务列表，preemptedTimeoutMs 表示处于 PREEMPTED 状态任务的超时时间（毫秒）
	SchedulableTasks(ctx context.Context, preemptedTimeoutMs int64, limit int) ([]domain.Task, error)
	// Acquire 抢占任务
	Acquire(ctx context.Context, id, version int64, scheduleNodeID string) (domain.Task, error)
	// Release 释放任务
	Release(ctx context.Context, id int64, scheduleNodeID string) (domain.Task, error)
	// Renew 续约所有抢占到的任务
	Renew(ctx context.Context, scheduleNodeID string) error
	// UpdateNextTime 更新任务的下次执行时间
	UpdateNextTime(ctx context.Context, id, version, nextTime int64) (domain.Task, error)
	// UpdateScheduleParams 更新调度参数
	UpdateScheduleParams(ctx context.Context, id, version int64, scheduleParams map[string]string) (domain.Task, error)
	// FindByPlanID 根据计划ID获取所有子任务
	FindByPlanID(ctx context.Context, planID int64) ([]domain.Task, error)
	// UpdateStatus 更新任务状态
	UpdateStatus(ctx context.Context, id int64, status domain.TaskStatus) (domain.Task, error)
	// Start 启动任务，并原子保存仅供下一次调度消费的参数覆盖。
	Start(ctx context.Context, id int64, paramOverrides map[string]string) error
	// ResetSchedule 重置一次性任务的调度信息，并原子保存仅供该次调度消费的参数覆盖。
	ResetSchedule(ctx context.Context, task domain.Task, paramOverrides map[string]string) error
	// UpdateExecMode 更新任务的执行模式快照
	UpdateExecMode(ctx context.Context, id int64, mode domain.ExecMode) error
	// Retry 手动重试任务
	Retry(ctx context.Context, id, version, nextTime int64) (domain.Task, error)
	// List 分页获取任务列表
	List(ctx context.Context, bizID int64, offset, limit int) ([]domain.Task, error)
	// Count 获取任务总数
	Count(ctx context.Context, bizID int64) (int64, error)
	// Update 更新任务配置
	Update(ctx context.Context, task domain.Task) error
	// Delete 删除任务
	Delete(ctx context.Context, id int64) error
}

type taskRepository struct {
	taskDAO             dao.TaskDAO
	paramOverrideDAO    dao.TaskParamOverrideDAO
	notificationRuleDAO dao.TaskNotificationRuleDAO
	protector           security.VariableCipher
}

// NewTaskRepository 创建负责重建 Task 聚合的仓储。
func NewTaskRepository(taskDAO dao.TaskDAO, paramOverrideDAO dao.TaskParamOverrideDAO,
	notificationRuleDAO dao.TaskNotificationRuleDAO, protector security.VariableCipher) TaskRepository {
	return &taskRepository{
		taskDAO:             taskDAO,
		paramOverrideDAO:    paramOverrideDAO,
		notificationRuleDAO: notificationRuleDAO,
		protector:           protector,
	}
}

func (r *taskRepository) FindByPlanID(ctx context.Context, planID int64) ([]domain.Task, error) {
	daoTasks, err := r.taskDAO.FindByPlanID(ctx, planID)
	if err != nil {
		return nil, err
	}
	return r.toDomains(daoTasks)
}

func (r *taskRepository) Create(ctx context.Context, task domain.Task) (domain.Task, error) {
	entity, err := r.toEntity(task)
	if err != nil {
		return domain.Task{}, err
	}
	created, err := r.taskDAO.Create(ctx, entity)
	if err != nil {
		return domain.Task{}, err
	}
	return r.toDomain(created)
}

func (r *taskRepository) GetByID(ctx context.Context, id int64) (domain.Task, error) {
	daoTask, err := r.taskDAO.GetByID(ctx, id)
	if err != nil {
		return domain.Task{}, err
	}
	if err = r.loadTaskAssociations(ctx, daoTask); err != nil {
		return domain.Task{}, err
	}
	return r.toDomain(daoTask)
}

func (r *taskRepository) GetByName(ctx context.Context, name string) (domain.Task, error) {
	daoTask, err := r.taskDAO.GetByName(ctx, name)
	if err != nil {
		return domain.Task{}, err
	}
	if err = r.loadTaskAssociations(ctx, daoTask); err != nil {
		return domain.Task{}, err
	}
	return r.toDomain(daoTask)
}

// loadTaskAssociations 加载并装配单个 Task 聚合的关联配置。
func (r *taskRepository) loadTaskAssociations(ctx context.Context, task *dao.Task) error {
	overrideRules, err := r.paramOverrideDAO.FindRulesByTaskID(ctx, task.ID)
	if err != nil {
		return err
	}
	notificationRules, err := r.notificationRuleDAO.FindByTaskID(ctx, task.ID)
	if err != nil {
		return err
	}
	pending, exists, err := r.paramOverrideDAO.FindPendingByTaskID(ctx, task.ID)
	if err != nil {
		return err
	}

	task.ParamOverrideRules = overrideRules
	task.NotificationRules = notificationRules
	if exists {
		task.PendingParamOverrides = pending.Overrides
	}
	return nil
}

func (r *taskRepository) SchedulableTasks(ctx context.Context, preemptedTimeoutMs int64, limit int) ([]domain.Task, error) {
	tasks, err := r.taskDAO.FindScheduleTasks(ctx, preemptedTimeoutMs, limit)
	if err != nil {
		return nil, err
	}
	return r.toDomains(tasks)
}

func (r *taskRepository) Acquire(ctx context.Context, id, version int64, scheduleNodeID string) (domain.Task, error) {
	task, err := r.taskDAO.Acquire(ctx, id, version, scheduleNodeID)
	if err != nil {
		return domain.Task{}, err
	}
	return r.toDomain(task)
}

func (r *taskRepository) Release(ctx context.Context, id int64, scheduleNodeID string) (domain.Task, error) {
	task, err := r.taskDAO.Release(ctx, id, scheduleNodeID)
	if err != nil {
		return domain.Task{}, err
	}
	return r.toDomain(task)
}

func (r *taskRepository) Renew(ctx context.Context, scheduleNodeID string) error {
	return r.taskDAO.Renew(ctx, scheduleNodeID)
}

func (r *taskRepository) UpdateNextTime(ctx context.Context, id, version, nextTime int64) (domain.Task, error) {
	task, err := r.taskDAO.UpdateNextTime(ctx, id, version, nextTime)
	if err != nil {
		return domain.Task{}, err
	}
	return r.toDomain(task)
}

func (r *taskRepository) UpdateScheduleParams(ctx context.Context, id, version int64, scheduleParams map[string]string) (domain.Task, error) {
	task, err := r.taskDAO.UpdateScheduleParams(ctx, id, version, scheduleParams)
	if err != nil {
		return domain.Task{}, err
	}
	return r.toDomain(task)
}

// UpdateStatus 更新任务状态
func (r *taskRepository) UpdateStatus(ctx context.Context, id int64, status domain.TaskStatus) (domain.Task, error) {
	task, err := r.taskDAO.UpdateStatus(ctx, id, status.String())
	if err != nil {
		return domain.Task{}, err
	}
	return r.toDomain(task)
}

func (r *taskRepository) Start(ctx context.Context, id int64,
	paramOverrides map[string]string) error {
	return r.taskDAO.Start(ctx, id, paramOverrides)
}

// ResetSchedule 重置一次性任务的调度信息，并保存该次执行的参数覆盖。
func (r *taskRepository) ResetSchedule(ctx context.Context, task domain.Task,
	paramOverrides map[string]string) error {
	entity, err := r.toEntity(task)
	if err != nil {
		return err
	}
	return r.taskDAO.ResetSchedule(ctx, entity, paramOverrides)
}

// UpdateExecMode 更新任务的执行模式快照
func (r *taskRepository) UpdateExecMode(ctx context.Context, id int64, mode domain.ExecMode) error {
	return r.taskDAO.UpdateExecMode(ctx, id, mode.String())
}

func (r *taskRepository) Retry(ctx context.Context, id, version, nextTime int64) (domain.Task, error) {
	task, err := r.taskDAO.Retry(ctx, id, version, nextTime)
	if err != nil {
		return domain.Task{}, err
	}
	return r.toDomain(task)
}

func (r *taskRepository) List(ctx context.Context, bizID int64, offset, limit int) ([]domain.Task, error) {
	tasks, err := r.taskDAO.List(ctx, bizID, offset, limit)
	if err != nil {
		return nil, err
	}
	if err = r.loadNotificationRules(ctx, tasks); err != nil {
		return nil, err
	}
	return r.toDomains(tasks)
}

// loadNotificationRules 批量加载并装配任务列表中的执行通知规则。
func (r *taskRepository) loadNotificationRules(ctx context.Context, tasks []*dao.Task) error {
	taskByID := make(map[int64]*dao.Task, len(tasks))
	taskIDs := make([]int64, 0, len(tasks))
	for _, task := range tasks {
		if task == nil {
			continue
		}
		taskByID[task.ID] = task
		taskIDs = append(taskIDs, task.ID)
	}
	rules, err := r.notificationRuleDAO.FindByTaskIDs(ctx, taskIDs)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if task := taskByID[rule.TaskID]; task != nil {
			task.NotificationRules = append(task.NotificationRules, rule)
		}
	}
	return nil
}

func (r *taskRepository) Count(ctx context.Context, bizID int64) (int64, error) {
	return r.taskDAO.Count(ctx, bizID)
}

func (r *taskRepository) Update(ctx context.Context, task domain.Task) error {
	entity, err := r.toEntity(task)
	if err != nil {
		return err
	}
	return r.taskDAO.Update(ctx, entity)
}

func (r *taskRepository) Delete(ctx context.Context, id int64) error {
	return r.taskDAO.Delete(ctx, id)
}

// toEntity 将领域模型转换为DAO模型
func (r *taskRepository) toEntity(task domain.Task) (dao.Task, error) {
	var scheduleNodeID sql.NullString
	if task.ScheduleNodeID != "" {
		scheduleNodeID = sql.NullString{String: task.ScheduleNodeID, Valid: true}
	}

	var runnerID sql.NullInt64
	if task.RunnerID > 0 {
		runnerID = sql.NullInt64{Int64: task.RunnerID, Valid: true}
	}

	var grpcConfig sqlx.JSONColumn[domain.GrpcConfig]
	if task.GrpcConfig != nil {
		config := *task.GrpcConfig
		protected := config.Variables
		if len(config.Variables) > 0 {
			if r.protector == nil {
				return dao.Task{}, fmt.Errorf("保护任务变量失败: 未配置变量保护器")
			}
			var err error
			protected, err = r.protector.EncryptVariables(config.Variables)
			if err != nil {
				return dao.Task{}, fmt.Errorf("保护任务变量失败: %w", err)
			}
		}
		config.Variables = protected
		grpcConfig = sqlx.JSONColumn[domain.GrpcConfig]{Val: config, Valid: true}
	}

	var httpConfig sqlx.JSONColumn[domain.HTTPConfig]
	if task.HTTPConfig != nil {
		httpConfig = sqlx.JSONColumn[domain.HTTPConfig]{Val: *task.HTTPConfig, Valid: true}
	}

	var program sqlx.JSONColumn[domain.ProgramSpec]
	if task.Program != nil {
		program = sqlx.JSONColumn[domain.ProgramSpec]{Val: *task.Program, Valid: true}
	}

	var retryConfig sqlx.JSONColumn[domain.RetryConfig]
	if task.RetryConfig != nil {
		retryConfig = sqlx.JSONColumn[domain.RetryConfig]{Val: *task.RetryConfig, Valid: true}
	}

	var scheduleParams sqlx.JSONColumn[map[string]string]
	if task.ScheduleParams != nil {
		scheduleParams = sqlx.JSONColumn[map[string]string]{Val: task.ScheduleParams, Valid: true}
	}

	var metadata sqlx.JSONColumn[map[string]string]
	if task.Metadata != nil {
		metadata = sqlx.JSONColumn[map[string]string]{Val: task.Metadata, Valid: true}
	}

	return dao.Task{
		ID:                  task.ID,
		TenantID:            task.TenantID,
		BizID:               task.BizID,
		BizKey:              task.BizKey,
		Name:                task.Name,
		RunnerID:            runnerID,
		Type:                task.Type.String(),
		CronExpr:            task.CronExpr,
		GrpcConfig:          grpcConfig,
		Program:             program,
		HTTPConfig:          httpConfig,
		RetryConfig:         retryConfig,
		ScheduleParams:      scheduleParams,
		MaxExecutionSeconds: task.MaxExecutionSeconds,
		ScheduleNodeID:      scheduleNodeID,
		NextTime:            task.NextTime,
		Status:              task.Status.String(),
		Version:             task.Version,
		Ctime:               task.CTime,
		Utime:               task.UTime,
		ExecMode:            task.ExecMode.String(),
		Metadata:            metadata,
		NotificationRules:   toDAOExecutionNotificationRules(task.NotificationRules),
		ParamOverrideRules:  toDAOOverrideRules(task.ParamOverrideRules),
	}, nil
}

// toDomain 将DAO模型转换为领域模型
func (r *taskRepository) toDomain(daoTask *dao.Task) (domain.Task, error) {
	var scheduleNodeID string
	if daoTask.ScheduleNodeID.Valid {
		scheduleNodeID = daoTask.ScheduleNodeID.String
	}
	var runnerID int64
	if daoTask.RunnerID.Valid {
		runnerID = daoTask.RunnerID.Int64
	}

	var grpcConfig *domain.GrpcConfig
	if daoTask.GrpcConfig.Valid {
		config := daoTask.GrpcConfig.Val
		revealed := config.Variables
		if len(config.Variables) > 0 {
			if r.protector == nil {
				return domain.Task{}, fmt.Errorf("恢复任务变量失败: 未配置变量保护器")
			}
			var err error
			revealed, err = r.protector.DecryptVariables(config.Variables)
			if err != nil {
				return domain.Task{}, fmt.Errorf("恢复任务变量失败: %w", err)
			}
		}
		config.Variables = revealed
		grpcConfig = &config
	}

	var httpConfig *domain.HTTPConfig
	if daoTask.HTTPConfig.Valid {
		httpConfig = &daoTask.HTTPConfig.Val
	}

	var program *domain.ProgramSpec
	if daoTask.Program.Valid {
		program = &daoTask.Program.Val
	}

	var retryConfig *domain.RetryConfig
	if daoTask.RetryConfig.Valid {
		retryConfig = &daoTask.RetryConfig.Val
	}

	var scheduleParams map[string]string
	if daoTask.ScheduleParams.Valid {
		scheduleParams = daoTask.ScheduleParams.Val
	}

	var metadata map[string]string
	if daoTask.Metadata.Valid {
		metadata = daoTask.Metadata.Val
	}

	return domain.Task{
		ID:                    daoTask.ID,
		TenantID:              daoTask.TenantID,
		BizID:                 daoTask.BizID,
		BizKey:                daoTask.BizKey,
		Name:                  daoTask.Name,
		RunnerID:              runnerID,
		Type:                  domain.TaskType(daoTask.Type),
		CronExpr:              daoTask.CronExpr,
		GrpcConfig:            grpcConfig,
		Program:               program,
		HTTPConfig:            httpConfig,
		RetryConfig:           retryConfig,
		MaxExecutionSeconds:   daoTask.MaxExecutionSeconds,
		ScheduleParams:        scheduleParams,
		ScheduleNodeID:        scheduleNodeID,
		NextTime:              daoTask.NextTime,
		Status:                domain.TaskStatus(daoTask.Status),
		Version:               daoTask.Version,
		UTime:                 daoTask.Utime,
		CTime:                 daoTask.Ctime,
		ExecMode:              domain.ExecMode(daoTask.ExecMode),
		Metadata:              metadata,
		NotificationRules:     toDomainExecutionNotificationRules(daoTask.NotificationRules),
		PendingParamOverrides: daoTask.PendingParamOverrides.Val,
		ParamOverrideRules:    toDomainOverrideRules(daoTask.ParamOverrideRules),
	}, nil
}

// toDomains 批量转换任务，并保留每一条敏感变量的解密错误。
func (r *taskRepository) toDomains(source []*dao.Task) ([]domain.Task, error) {
	result := make([]domain.Task, 0, len(source))
	for _, item := range source {
		task, err := r.toDomain(item)
		if err != nil {
			return nil, err
		}
		result = append(result, task)
	}
	return result, nil
}

func toDAOExecutionNotificationRules(
	rules []domain.ExecutionNotificationRule) []dao.TaskExecutionNotificationRule {
	result := make([]dao.TaskExecutionNotificationRule, 0, len(rules))
	for _, rule := range rules {
		result = append(result, dao.TaskExecutionNotificationRule{
			TriggerStatus: rule.TriggerStatus.String(),
			TemplateSetID: rule.TemplateSetID,
			Enabled:       rule.Enabled,
			Recipients: sqlx.JSONColumn[[]domain.NotificationRecipient]{
				Val: rule.Recipients, Valid: true,
			},
			Channels: sqlx.JSONColumn[[]domain.NotificationChannel]{
				Val: rule.Channels, Valid: true,
			},
		})
	}
	return result
}

func toDomainExecutionNotificationRules(
	rules []dao.TaskExecutionNotificationRule) []domain.ExecutionNotificationRule {
	result := make([]domain.ExecutionNotificationRule, 0, len(rules))
	for _, rule := range rules {
		result = append(result, domain.ExecutionNotificationRule{
			TriggerStatus: domain.TaskExecutionStatus(rule.TriggerStatus),
			TemplateSetID: rule.TemplateSetID,
			Enabled:       rule.Enabled,
			Recipients:    rule.Recipients.Val, Channels: rule.Channels.Val,
		})
	}
	return result
}

func toDAOOverrideRules(rules []domain.TaskParamOverrideRule) []dao.TaskParamOverrideRule {
	result := make([]dao.TaskParamOverrideRule, 0, len(rules))
	for _, rule := range rules {
		result = append(result, dao.TaskParamOverrideRule{
			ParamKey: rule.ParamKey,
			InputConfig: sqlx.JSONColumn[dao.TaskParamOverrideInputConfig]{
				Val: dao.TaskParamOverrideInputConfig{
					AllowedModes: rule.AllowedModes, DefaultMode: rule.DefaultMode,
					SelectConfig: rule.SelectConfig,
				},
				Valid: true,
			},
		})
	}
	return result
}

func toDomainOverrideRules(rules []dao.TaskParamOverrideRule) []domain.TaskParamOverrideRule {
	result := make([]domain.TaskParamOverrideRule, 0, len(rules))
	for _, rule := range rules {
		result = append(result, domain.TaskParamOverrideRule{
			ParamKey:     rule.ParamKey,
			AllowedModes: rule.InputConfig.Val.AllowedModes,
			DefaultMode:  rule.InputConfig.Val.DefaultMode,
			SelectConfig: rule.InputConfig.Val.SelectConfig,
		})
	}
	return result
}
