package task

import (
	"context"
	"fmt"
	"time"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	"github.com/Duke1616/etask/internal/repository"
	poolSvc "github.com/Duke1616/etask/internal/service/pool"
	programSvc "github.com/Duke1616/etask/internal/service/program"
	runnerSvc "github.com/Duke1616/etask/internal/service/runner"
	"github.com/Duke1616/etask/pkg/cryptox"
	"golang.org/x/sync/errgroup"
)

// Service 任务服务接口
type Service interface {
	// Create 创建任务
	Create(ctx context.Context, task domain.Task) (domain.Task, error)
	// SchedulableTasks 获取可调度的任务列表，preemptedTimeoutMs 表示处于 PREEMPTED 状态任务的超时时间（毫秒）
	SchedulableTasks(ctx context.Context, preemptedTimeoutMs int64, limit int) ([]domain.Task, error)
	// UpdateNextTime 更新任务的下次执行时间
	UpdateNextTime(ctx context.Context, id int64) (domain.Task, error)
	// GetByID 根据ID获取task
	GetByID(ctx context.Context, id int64) (domain.Task, error)
	// GetByName 根据名称获取task
	GetByName(ctx context.Context, name string) (domain.Task, error)
	// UpdateScheduleParams 更新调度参数
	UpdateScheduleParams(ctx context.Context, task domain.Task, params map[string]string) (domain.Task, error)
	// RetryByID 根据 ID 重试任务
	RetryByID(ctx context.Context, id int64) (domain.Task, error)
	// RetryByName 根据名称重试任务
	RetryByName(ctx context.Context, name string) (domain.Task, error)
	// List 分页获取任务列表
	List(ctx context.Context, bizID int64, offset, limit int) ([]domain.Task, int64, error)
	// Update 更新任务配置
	Update(ctx context.Context, task domain.Task) error
	// Delete 删除任务
	Delete(ctx context.Context, id int64) error
	// Stop 停止任务
	Stop(ctx context.Context, id int64) error
	// Run 运行任务（从停止状态恢复）。一次性任务可传入 cronExpr 修改下次执行时间。
	Run(ctx context.Context, id int64, cronExpr string, paramOverrides map[string]domain.RunParamOverride) error
	// AuthorizeExecutionPool 校验任务是否被授权使用配置的执行资源池。
	AuthorizeExecutionPool(ctx context.Context, task domain.Task) error
}

type service struct {
	repo             repository.TaskRepository
	poolAuthorizer   poolSvc.ExecutionPoolAuthorizer
	metadataProvider poolSvc.HandlerMetadataProvider
	runnerSvc        runnerSvc.Service
}

// NewService 创建任务服务实例
func NewService(repo repository.TaskRepository, authorizer poolSvc.ExecutionPoolAuthorizer,
	metadataProvider poolSvc.HandlerMetadataProvider, runnerService runnerSvc.Service) Service {
	return &service{
		repo:             repo,
		poolAuthorizer:   authorizer,
		metadataProvider: metadataProvider,
		runnerSvc:        runnerService,
	}
}

func (s *service) Create(ctx context.Context, task domain.Task) (domain.Task, error) {
	if err := s.prepareTaskConfiguration(ctx, &task); err != nil {
		return domain.Task{}, err
	}
	if err := s.setNextScheduleTime(&task); err != nil {
		return domain.Task{}, err
	}

	return s.repo.Create(ctx, task)
}

func (s *service) SchedulableTasks(ctx context.Context, preemptedTimeoutMs int64, limit int) ([]domain.Task, error) {
	return s.repo.SchedulableTasks(ctx, preemptedTimeoutMs, limit)
}

func (s *service) UpdateNextTime(ctx context.Context, id int64) (domain.Task, error) {
	task, err := s.GetByID(ctx, id)
	if err != nil {
		return domain.Task{}, err
	}

	// 一次性任务：如果 NextTime 在过去（或为0表示立即执行），说明已执行完成，直接设置为 COMPLETED
	// 这样可以避免 CalculateNextTime 计算出下一次时间
	if task.Type.IsOneTime() && task.NextTime >= 0 && task.NextTime < time.Now().UnixMilli() {
		updated, updateErr := s.repo.UpdateStatus(ctx, id, domain.TaskStatusCompleted)
		updated.NotificationRules = task.NotificationRules
		return updated, updateErr
	}

	// 计算下次执行时间
	if err = s.setNextScheduleTime(&task); err != nil {
		return domain.Task{}, err
	}

	// 如果下次执行时间为零值，说明 cron 不再触发，直接返回（保持原状态）
	if task.NextTime == 0 {
		return task, nil
	}

	updated, err := s.repo.UpdateNextTime(ctx, task.ID, task.Version, task.NextTime)
	updated.NotificationRules = task.NotificationRules
	return updated, err
}

func (s *service) GetByID(ctx context.Context, id int64) (domain.Task, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *service) GetByName(ctx context.Context, name string) (domain.Task, error) {
	return s.repo.GetByName(ctx, name)
}

func (s *service) UpdateScheduleParams(ctx context.Context, task domain.Task, params map[string]string) (domain.Task, error) {
	task.UpdateScheduleParams(params)
	return s.repo.UpdateScheduleParams(ctx, task.ID, task.Version, task.ScheduleParams)
}

func (s *service) RetryByID(ctx context.Context, id int64) (domain.Task, error) {
	task, err := s.GetByID(ctx, id)
	if err != nil {
		return domain.Task{}, err
	}

	return s.retry(ctx, task)
}

func (s *service) RetryByName(ctx context.Context, name string) (domain.Task, error) {
	task, err := s.GetByName(ctx, name)
	if err != nil {
		return domain.Task{}, err
	}

	return s.retry(ctx, task)
}

func (s *service) retry(ctx context.Context, task domain.Task) (domain.Task, error) {
	// 运行中的任务不允许重试，防止状态竞争
	if task.Status == domain.TaskStatusPreempted {
		return domain.Task{}, fmt.Errorf("任务正在运行中，请等结束后再重试")
	}
	if err := s.AuthorizeExecutionPool(ctx, task); err != nil {
		return domain.Task{}, err
	}

	// 重置为立即执行
	return s.repo.Retry(ctx, task.ID, task.Version, time.Now().UnixMilli())
}

func (s *service) List(ctx context.Context, bizID int64, offset, limit int) ([]domain.Task, int64, error) {
	var (
		tasks []domain.Task
		total int64
	)
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		var err error
		tasks, err = s.repo.List(ctx, bizID, offset, limit)
		return err
	})
	eg.Go(func() error {
		var err error
		total, err = s.repo.Count(ctx, bizID)
		return err
	})
	if err := eg.Wait(); err != nil {
		return nil, 0, err
	}
	return tasks, total, nil
}

func (s *service) Update(ctx context.Context, task domain.Task) error {
	// 1. 获取原任务信息用于比对
	oldTask, err := s.repo.GetByID(ctx, task.ID)
	if err != nil {
		return err
	}
	if task.TenantID == 0 {
		task.TenantID = oldTask.TenantID
	}
	preserveMaskedVariables(oldTask, &task)
	if err = s.prepareTaskConfiguration(ctx, &task); err != nil {
		return err
	}

	// 2. 如果 Cron 表达式发生变化，重新计算下次执行时间
	if oldTask.CronExpr != task.CronExpr {
		if err = s.setNextScheduleTime(&task); err != nil {
			return err
		}
	} else {
		task.NextTime = oldTask.NextTime
	}

	return s.repo.Update(ctx, task)
}

// preserveMaskedVariables 将接口回显的脱敏占位符还原为旧值，避免未修改敏感变量时覆盖真实密文。
func preserveMaskedVariables(oldTask domain.Task, task *domain.Task) {
	if task == nil || task.GrpcConfig == nil || oldTask.GrpcConfig == nil {
		return
	}
	oldByKey := make(map[string]domain.RunnerVariable, len(oldTask.GrpcConfig.Variables))
	for _, item := range oldTask.GrpcConfig.Variables {
		oldByKey[item.Key] = item
	}
	for index, item := range task.GrpcConfig.Variables {
		if !item.Secret || item.Value != cryptox.DefaultMask {
			continue
		}
		old, ok := oldByKey[item.Key]
		if ok && old.Secret {
			task.GrpcConfig.Variables[index].Value = old.Value
		}
	}
}

// prepareTaskConfiguration 补全并校验创建、更新任务时使用的配置。
func (s *service) prepareTaskConfiguration(ctx context.Context, task *domain.Task) error {
	if err := s.bindRunner(ctx, task); err != nil {
		return err
	}
	if err := s.validateStructuredVariables(ctx, *task); err != nil {
		return err
	}
	if err := task.Validate(); err != nil {
		return err
	}
	if err := s.AuthorizeExecutionPool(ctx, *task); err != nil {
		return err
	}
	return s.validateParamOverrideRules(ctx, *task)
}

// validateStructuredVariables 保证变量只能通过独立字段传递，禁止再次写入普通参数。
func (s *service) validateStructuredVariables(ctx context.Context, task domain.Task) error {
	if task.GrpcConfig == nil {
		return nil
	}
	if err := domain.ValidateVariableItems(task.GrpcConfig.Variables); err != nil {
		return err
	}
	if s.metadataProvider == nil || task.RunnerID != 0 {
		return nil
	}
	keys, err := s.metadataProvider.VariableParameterKeys(ctx, poolSvc.CheckBindingRequest{
		PoolName: task.GrpcConfig.ServiceName, HandlerName: task.GrpcConfig.HandlerName,
	})
	if err != nil {
		return fmt.Errorf("查询 Handler 变量参数失败: %w", err)
	}
	for key := range keys {
		if mode := task.Metadata[key]; mode != "runner" {
			if _, exists := task.GrpcConfig.Params[key]; exists {
				return fmt.Errorf("%w: 结构化变量参数 %s 必须通过 grpc_config.variables 传递", errs.ErrInvalidParameter, key)
			}
		}
	}
	return nil
}

// bindRunner 将执行单元提供的程序和执行路由固化到任务配置，任务参数只作为覆盖值保留。
func (s *service) bindRunner(ctx context.Context, task *domain.Task) error {
	if task.RunnerID == 0 {
		return nil
	}
	if task.RunnerID < 0 {
		return fmt.Errorf("%w: runner_id = %d", errs.ErrInvalidParameter, task.RunnerID)
	}
	if task.HTTPConfig != nil {
		return fmt.Errorf("%w: 指定执行单元时不能同时配置 HTTP 执行目标", errs.ErrInvalidParameter)
	}
	runner, err := s.runnerSvc.FindByID(ctx, task.RunnerID)
	if err != nil {
		return fmt.Errorf("查询执行单元失败: %w", err)
	}
	if runner.Action != domain.RunnerActionRegistered {
		return fmt.Errorf("%w: 执行单元未启用", errs.ErrInvalidParameter)
	}
	program, err := programSvc.SpecFromRunnerBinding(runner.CodebookID, runner.ProgramKind)
	if err != nil {
		return err
	}
	var overrides map[string]string
	if task.GrpcConfig != nil {
		overrides = task.GrpcConfig.Params
	}
	task.Program = program
	// Runner 模式不使用普通 gRPC 任务的参数绑定；清除切换协议时可能残留的旧 metadata。
	task.Metadata = nil
	task.GrpcConfig = &domain.GrpcConfig{
		ServiceName: runner.Target,
		HandlerName: runner.Handler,
		Params:      overrides,
	}
	return nil
}

func (s *service) setNextScheduleTime(task *domain.Task) error {
	nextTime, err := task.CalculateNextTime()
	if err != nil {
		return fmt.Errorf("%w: %w", errs.ErrInvalidTaskCronExpr, err)
	}

	if nextTime.IsZero() {
		task.NextTime = 0
	} else {
		task.NextTime = nextTime.UnixMilli()
	}

	return nil
}

func (s *service) Delete(ctx context.Context, id int64) error {
	task, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// 判定是否已经停止，如果不是停止状态禁止删除
	// 只有 INACTIVE (手动停止) 和 COMPLETED (一次性任务执行完成) 视为停止状态
	if task.Status != domain.TaskStatusInactive && task.Status != domain.TaskStatusCompleted {
		return fmt.Errorf("只能删除已停止的任务（当前状态: %s），请先停止任务后再试", task.Status)
	}

	return s.repo.Delete(ctx, id)
}

func (s *service) Stop(ctx context.Context, id int64) error {
	_, err := s.repo.UpdateStatus(ctx, id, domain.TaskStatusInactive)
	return err
}

func (s *service) Run(ctx context.Context, id int64, cronExpr string,
	paramOverrides map[string]domain.RunParamOverride) error {
	task, err := s.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err = s.AuthorizeExecutionPool(ctx, task); err != nil {
		return err
	}
	if len(paramOverrides) > 0 {
		if err = s.validateParamOverrideRules(ctx, task); err != nil {
			return err
		}
	}
	serialized, err := validateAndSerializeOverrides(task.ParamOverrideRules, paramOverrides)
	if err != nil {
		return err
	}

	if cronExpr != "" {
		if !task.Type.IsOneTime() {
			return fmt.Errorf("只有一次性任务支持传递执行时间表达式")
		}
		if task.Status == domain.TaskStatusPreempted {
			return fmt.Errorf("任务正在运行中，请等结束后再运行")
		}

		task.CronExpr = cronExpr
		task.Status = domain.TaskStatusActive
		if err = s.setNextScheduleTime(&task); err != nil {
			return err
		}
		return s.repo.ResetSchedule(ctx, task, serialized)
	}

	return s.repo.Start(ctx, id, serialized)
}

func (s *service) AuthorizeExecutionPool(ctx context.Context, task domain.Task) error {
	if task.GrpcConfig == nil || s.poolAuthorizer == nil {
		return nil
	}

	allowed, err := s.poolAuthorizer.IsAllowed(ctx, poolSvc.CheckBindingRequest{
		PoolName:    task.GrpcConfig.ServiceName,
		HandlerName: task.GrpcConfig.HandlerName,
	})
	if err != nil {
		return err
	}
	if !allowed {
		return fmt.Errorf("%w: pool=%s handler=%s",
			errs.ErrExecutionPoolNotAllowed,
			task.GrpcConfig.ServiceName,
			task.GrpcConfig.HandlerName,
		)
	}
	return nil
}
