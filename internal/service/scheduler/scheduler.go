package scheduler

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	"github.com/Duke1616/etask/internal/service/acquirer"
	"github.com/Duke1616/etask/internal/service/dispatcher"
	"github.com/Duke1616/etask/internal/service/task"
	"github.com/Duke1616/etask/internal/sse"
	"github.com/gotomicro/ego/core/constant"
	"github.com/gotomicro/ego/core/elog"
	"github.com/gotomicro/ego/server"
)

var _ server.Server = &Scheduler{}

// Scheduler 分布式任务调度器
type Scheduler struct {
	nodeID     string
	dispatcher dispatcher.Dispatcher
	taskSvc    task.Service
	acquirer   acquirer.TaskAcquirer
	events     *sse.Hubs
	config     Config
	ctx        context.Context
	cancel     context.CancelFunc
	logger     *elog.Component
}

// Config 调度器配置
type Config struct {
	BatchTimeout     time.Duration `mapstructure:"batch_timeout" yaml:"batch_timeout"`         // 批量查询超时时间
	BatchSize        int           `mapstructure:"batch_size" yaml:"batch_size"`               // 批量获取任务数量
	PreemptedTimeout time.Duration `mapstructure:"preempted_timeout" yaml:"preempted_timeout"` // PREEMPTED 状态任务的超时时间
	ScheduleInterval time.Duration `mapstructure:"schedule_interval" yaml:"schedule_interval"` // 调度间隔
	RenewInterval    time.Duration `mapstructure:"renew_interval" yaml:"renew_interval"`       // 续约间隔
}

// NewScheduler 创建调度器实例
func NewScheduler(
	nodeID string,
	dispatcher dispatcher.Dispatcher,
	taskSvc task.Service,
	acquirer acquirer.TaskAcquirer,
	events *sse.Hubs,
	config Config,
) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		nodeID:     nodeID,
		dispatcher: dispatcher,
		taskSvc:    taskSvc,
		acquirer:   acquirer,
		events:     events,
		config:     config,
		ctx:        ctx,
		cancel:     cancel,
		logger:     elog.DefaultLogger.With(elog.FieldComponentName("Scheduler")),
	}
}

// NodeID 返回当前调度节点 ID。
func (s *Scheduler) NodeID() string {
	return s.nodeID
}

// Name 返回调度器服务名称。
func (s *Scheduler) Name() string {
	return fmt.Sprintf("Scheduler-%s", s.nodeID)
}

// PackageName 返回调度器组件标识。
func (s *Scheduler) PackageName() string {
	return "scheduler.Scheduler"
}

// Init 完成服务启动前的初始化。
func (s *Scheduler) Init() error {
	return nil
}

// Start 启动调度器
func (s *Scheduler) Start() error {
	s.logger.Info("启动分布式任务调度器", elog.String("nodeID", s.nodeID))

	// 启动调度循环
	go s.scheduleLoop()

	// 启动续约循环
	go s.renewLoop()
	return nil
}

// scheduleLoop 主调度循环
func (s *Scheduler) scheduleLoop() {
	for {
		if s.ctx.Err() != nil {
			s.logger.Info("调度循环结束")
			return
		}

		// 获取可调度的任务列表
		scheduleCtx, cancelFunc := context.WithTimeout(s.ctx, s.config.BatchTimeout)
		tasks, err := s.taskSvc.SchedulableTasks(scheduleCtx, s.config.PreemptedTimeout.Milliseconds(), s.config.BatchSize)
		cancelFunc()
		if err != nil {
			s.logger.Error("获取可调度任务失败", elog.FieldErr(err))
		}
		// 没有可以调度的任务就睡一会
		if len(tasks) == 0 {
			s.logger.Debug("没有可调度的任务")
			if !s.waitScheduleInterval() {
				return
			}
			continue
		}

		s.logger.Info("发现可调度任务", elog.Int("count", len(tasks)))
		// 开始调度
		successCount := 0
		for i := range tasks {
			if err = s.scheduleOnce(tasks[i]); err != nil {
				s.logger.Error("调度任务失败",
					elog.Int64("taskID", tasks[i].ID),
					elog.String("taskName", tasks[i].Name),
					elog.FieldErr(err))
			} else {
				successCount++
			}
		}
		s.logger.Info("本次调度信息",
			elog.Int("success", successCount),
			elog.Int("total", len(tasks)))

		// 整批任务均派发失败时进行退避，避免永久性配置错误形成热循环。
		if successCount == 0 && !s.waitScheduleInterval() {
			return
		}
	}
}

func (s *Scheduler) waitScheduleInterval() bool {
	timer := time.NewTimer(s.config.ScheduleInterval)
	defer timer.Stop()
	select {
	case <-s.ctx.Done():
		s.logger.Info("调度循环结束")
		return false
	case <-timer.C:
		return true
	}
}

// scheduleOnce 调度单个任务
func (s *Scheduler) scheduleOnce(task domain.Task) error {
	ctx := taskContext(s.ctx, task.TenantID)
	err := s.dispatcher.Run(ctx, task)
	if errors.Is(err, errs.ErrProgramSourceUnavailable) {
		s.stopTaskWithUnavailableProgram(ctx, task)
	}
	return err
}

// stopTaskWithUnavailableProgram 停用仍指向同一失效程序来源的任务。
// 再次读取并比较程序配置，避免并发编辑已经修复任务后被旧调度结果误停用。
func (s *Scheduler) stopTaskWithUnavailableProgram(ctx context.Context, scheduled domain.Task) {
	current, err := s.taskSvc.GetByID(ctx, scheduled.ID)
	if err != nil {
		s.logger.Error("查询程序来源失效任务失败",
			elog.Int64("taskID", scheduled.ID), elog.FieldErr(err))
		return
	}
	if current.Status == domain.TaskStatusInactive || current.Status == domain.TaskStatusCompleted ||
		!reflect.DeepEqual(current.Program, scheduled.Program) {
		return
	}
	if err = s.taskSvc.Stop(ctx, scheduled.ID); err != nil {
		s.logger.Error("自动停用程序来源失效任务失败",
			elog.Int64("taskID", scheduled.ID), elog.String("taskName", scheduled.Name),
			elog.FieldErr(err))
		return
	}
	if stopped, getErr := s.taskSvc.GetByID(ctx, scheduled.ID); getErr == nil &&
		s.events != nil && s.events.Tasks != nil {
		s.events.Tasks.Broadcast(stopped.TenantID, sse.TaskStatusEvent{
			TaskID: stopped.ID, Status: stopped.Status.String(),
			NextTime: stopped.NextTime, Version: stopped.Version,
		})
	}
	s.logger.Warn("程序来源不存在，任务已自动停用",
		elog.Int64("taskID", scheduled.ID), elog.String("taskName", scheduled.Name))
}

// taskContext 注入租户及原始租户，供后续 GORM 租户插件使用。
func taskContext(ctx context.Context, tenantID int64) context.Context {
	if tenantID <= 0 {
		return ctx
	}
	ctx = ctxutil.WithTenantID(ctx, tenantID)
	return ctxutil.WithOriginTenantID(ctx, tenantID)
}

// renewLoop 续约循环
func (s *Scheduler) renewLoop() {
	ticker := time.NewTicker(s.config.RenewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			err := s.acquirer.Renew(s.ctx, s.nodeID)
			if err != nil {
				s.logger.Error("批量续约失败", elog.FieldErr(err))
			}
		}
	}
}

// Stop 停止调度器
func (s *Scheduler) Stop() error {
	s.logger.Info("停止分布式任务调度器", elog.String("nodeID", s.nodeID))
	// 取消上下文
	if s.cancel != nil {
		s.cancel()
	}
	return nil
}

// GracefulStop 停止调度循环和续约循环。
func (s *Scheduler) GracefulStop(_ context.Context) error {
	s.logger.Info("停止分布式任务调度器", elog.String("nodeID", s.nodeID))
	if s.cancel != nil {
		s.cancel()
	}
	return nil
}

// Info 返回调度器运行状态。
func (s *Scheduler) Info() *server.ServiceInfo {
	info := server.ApplyOptions(
		server.WithName(s.Name()),
		server.WithKind(constant.ServiceProvider),
	)
	info.Healthy = s.ctx.Err() == nil
	return &info
}
