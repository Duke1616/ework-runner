package manager

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/eiam/pkg/web/capability"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/service/task"
	terminationSvc "github.com/Duke1616/etask/internal/service/termination"
	"github.com/Duke1616/etask/internal/sse"
	"github.com/Duke1616/etask/pkg/grpc/interceptors/bizid"
	"github.com/ecodeclub/ekit/slice"
	"github.com/ecodeclub/ginx"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

var _ ginx.Handler = &Handler{}

type Handler struct {
	svc         task.Service
	logSvc      task.LogService
	execSvc     task.ExecutionService
	termination terminationSvc.Service
	events      *sse.Hubs
	capability.IRegistry
	executionRegistry capability.IRegistry
}

func (h *Handler) PublicRoutes(_ *gin.Engine) {

}

func (h *Handler) IdentifyRoutes(_ *gin.Engine) {}

func NewHandler(svc task.Service, logSvc task.LogService, execSvc task.ExecutionService,
	events *sse.Hubs, termination terminationSvc.Service) *Handler {
	return &Handler{
		svc:               svc,
		logSvc:            logSvc,
		execSvc:           execSvc,
		termination:       termination,
		events:            events,
		IRegistry:         capability.NewRegistry("task", "manager", "任务管理"),
		executionRegistry: capability.NewRegistry("task", "execution", "任务执行"),
	}
}

func (h *Handler) PrivateRoutes(server *gin.Engine) {
	g := server.Group("/api/manager")
	// --- 实时事件流 ---
	// 流式接口统一收敛到 /api/streams/<module>/，便于网关按稳定前缀应用 SSE 策略。
	streams := server.Group("/api/streams/manager")
	streams.GET("/task-events", h.Capability("订阅任务状态事件", "task_events").
		NoSync().
		Handle(ginx.W(h.StreamEvents)),
	)
	streams.GET("/tasks/:id/executions", h.Capability("订阅任务执行事件", "execution_events").
		NoSync().
		Handle(ginx.W(h.StreamTaskExecutions)),
	)
	streams.GET("/executions/:id/logs", h.executionRegistry.Capability("查看执行日志流", "logs").
		NoSync().
		Handle(ginx.W(h.StreamExecutionLogs)),
	)

	// --- 任务管理 ---
	g.POST("/create", h.Capability("创建任务", "add").
		Needs("ticket:executor:view").
		Handle(ginx.B[CreateTaskReq](h.Create)),
	)
	g.POST("/update", h.Capability("更新任务", "edit").
		Needs("ticket:executor:view").
		Handle(ginx.B[UpdateTaskReq](h.Update)),
	)
	g.POST("/list", h.Capability("任务列表", "view").
		Needs("task:manager:task_events").
		Handle(ginx.B[PageReq](h.List)),
	)
	g.GET("/detail/:id", h.Capability("任务详情", "get").
		Handle(ginx.W(h.Detail)),
	)
	g.DELETE("/delete/:id", h.Capability("删除任务", "delete").
		Handle(ginx.W(h.Delete)),
	)

	// --- 执行监控 ---
	g.POST("/logs", h.Capability("任务日志", "logs").
		Needs("task:execution:logs", "task:manager:executions", "task:manager:execution_events").
		Handle(ginx.B[GetLogsReq](h.GetLogs)),
	)

	g.POST("/executions", h.Capability("执行记录", "executions").
		Needs("task:manager:execution_events").
		Handle(ginx.B[ListExecutionsReq](h.ListExecutions)),
	)
	g.GET("/executions/:id/parameters", h.Capability("执行参数", "execution_parameters").
		Needs("task:manager:executions").
		Handle(ginx.W(h.ExecutionParameters)),
	)
	g.POST("/executions/:id/terminate", h.executionRegistry.Capability("终止执行", "terminate").
		Handle(ginx.B[TerminateExecutionReq](h.TerminateExecution)),
	)

	// --- 任务控制 ---
	g.POST("/stop/:id", h.Capability("停止任务", "stop").
		Needs("task:execution:terminate").
		Handle(ginx.W(h.Stop)),
	)
	g.POST("/run", h.Capability("运行任务", "start").
		Handle(ginx.B[RunTaskReq](h.Run)),
	)
}

func (h *Handler) TerminateExecution(ctx *ginx.Context, req TerminateExecutionReq) (ginx.Result, error) {
	id, err := ctx.Param("id").AsInt64()
	if err != nil || id <= 0 {
		return invalidParameterResult(fmt.Errorf("执行 ID 非法")), nil
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "管理员手动终止"
	}
	if len([]rune(reason)) > 500 {
		return invalidParameterResult(fmt.Errorf("终止原因不能超过 500 字")), nil
	}
	if h.termination == nil {
		return systemErrorResult, fmt.Errorf("执行终止服务未初始化")
	}
	if err = h.termination.RequestExecution(ctx, id, reason); err != nil {
		return systemErrorResult, err
	}
	if execution, findErr := h.execSvc.FindByID(ctx, id); findErr == nil && h.events != nil && execution.Task.ID > 0 {
		h.events.Executions.Broadcast(execution.Task.ID, sse.TaskExecutionEvent{
			ID:              execution.ID,
			TaskID:          execution.Task.ID,
			TaskName:        execution.Task.Name,
			StartTime:       execution.StartTime,
			EndTime:         execution.EndTime,
			Status:          execution.Status.String(),
			RunningProgress: execution.RunningProgress,
			ExecutorNodeId:  execution.ExecutorNodeID,
			TaskResult:      execution.TaskResult,
			CTime:           execution.CTime,
		})
	}
	return ginx.Result{Msg: "success"}, nil
}

func (h *Handler) Detail(ctx *ginx.Context) (ginx.Result, error) {
	id, err := ctx.Param("id").AsInt64()
	if err != nil {
		return systemErrorResult, err
	}

	t, err := h.svc.GetByID(ctx, id)
	if err != nil {
		return systemErrorResult, err
	}

	return ginx.Result{
		Data: toVO(t),
		Msg:  "success",
	}, nil
}

func (h *Handler) Delete(ctx *ginx.Context) (ginx.Result, error) {
	id, err := ctx.Param("id").AsInt64()
	if err != nil {
		return systemErrorResult, err
	}

	err = h.svc.Delete(ctx, id)
	if err != nil {
		return ginx.Result{
			Code: SystemErrorCode,
			Msg:  err.Error(),
		}, err
	}

	return ginx.Result{
		Msg: "success",
	}, nil
}

func (h *Handler) Stop(ctx *ginx.Context) (ginx.Result, error) {
	id, err := ctx.Param("id").AsInt64()
	if err != nil {
		return systemErrorResult, err
	}

	err = h.svc.Stop(ctx, id)
	if err != nil {
		return ginx.Result{
			Code: SystemErrorCode,
			Msg:  err.Error(),
		}, err
	}

	// 停止成功后，主动拉取最新任务状态并广播
	if t, errGet := h.svc.GetByID(ctx, id); errGet == nil {
		h.events.Tasks.Broadcast(t.TenantID, sse.TaskStatusEvent{
			TaskID:   t.ID,
			Status:   t.Status.String(),
			NextTime: t.NextTime,
			Version:  t.Version,
		})
	}

	return ginx.Result{
		Msg: "success",
	}, nil
}

func (h *Handler) Run(ctx *ginx.Context, req RunTaskReq) (ginx.Result, error) {
	err := h.svc.Run(ctx, req.ID, req.CronExpr, req.ParamOverrides)
	if err != nil {
		return ginx.Result{
			Code: SystemErrorCode,
			Msg:  err.Error(),
		}, err
	}

	// 启动成功后，主动拉取最新任务状态并广播
	if t, errGet := h.svc.GetByID(ctx, req.ID); errGet == nil {
		h.events.Tasks.Broadcast(t.TenantID, sse.TaskStatusEvent{
			TaskID:   t.ID,
			Status:   t.Status.String(),
			NextTime: t.NextTime,
			Version:  t.Version,
		})
	}

	return ginx.Result{
		Msg: "success",
	}, nil
}

// StreamEvents 实时向前端推送任务状态变更事件流的 SSE 接口
func (h *Handler) StreamEvents(ctx *ginx.Context) (ginx.Result, error) {
	tenantID := ctxutil.GetTenantID(ctx).Int64()
	h.events.Tasks.Stream(ctx, tenantID, sse.TASK_STATUS_CHANGE_EVENT, 20*time.Second)
	return ginx.Result{}, ginx.ErrNoResponse
}

// StreamExecutionLogs 实时推送特定执行记录的日志流的 SSE 接口
func (h *Handler) StreamExecutionLogs(ctx *ginx.Context) (ginx.Result, error) {
	id, err := ctx.Param("id").AsInt64()
	if err != nil {
		return systemErrorResult, err
	}
	if _, err = h.execSvc.FindByID(ctx, id); err != nil {
		return systemErrorResult, err
	}
	h.events.Logs.Stream(ctx, id, sse.TASK_LOG_EVENT, 20*time.Second)
	return ginx.Result{}, ginx.ErrNoResponse
}

// StreamTaskExecutions 实时推送特定任务的执行记录及进度流的 SSE 接口
func (h *Handler) StreamTaskExecutions(ctx *ginx.Context) (ginx.Result, error) {
	id, err := ctx.Param("id").AsInt64()
	if err != nil {
		return systemErrorResult, err
	}
	if _, err = h.svc.GetByID(ctx, id); err != nil {
		return systemErrorResult, err
	}
	h.events.Executions.Stream(ctx, id, sse.TASK_EXECUTION_EVENT, 20*time.Second)
	return ginx.Result{}, ginx.ErrNoResponse
}

func (h *Handler) Update(ctx *ginx.Context, req UpdateTaskReq) (ginx.Result, error) {
	err := h.svc.Update(ctx, toUpdateDomain(req))
	if err != nil {
		return ginx.Result{
			Code: SystemErrorCode,
			Msg:  err.Error(),
		}, err
	}

	return ginx.Result{
		Msg: "success",
	}, nil
}

func (h *Handler) GetLogs(ctx *ginx.Context, req GetLogsReq) (ginx.Result, error) {
	logs, total, err := h.logSvc.GetLogs(ctx, req.ExecutionID, req.MinID, req.Limit)
	if err != nil {
		return systemErrorResult, err
	}
	return ginx.Result{
		Data: ListLogResp{
			Total: total,
			Logs: slice.Map(logs, func(_ int, src domain.TaskExecutionLog) TaskLogVO {
				return TaskLogVO{
					ID:          src.ID,
					TaskID:      src.TaskID,
					ExecutionID: src.ExecutionID,
					Content:     src.Content,
					CTime:       src.CTime,
				}
			}),
		},
		Msg: "success",
	}, nil
}

func (h *Handler) ListExecutions(ctx *ginx.Context, req ListExecutionsReq) (ginx.Result, error) {
	executions, total, err := h.execSvc.ListByTaskID(ctx, req.TaskID, req.Offset, req.Limit)
	if err != nil {
		return systemErrorResult, err
	}

	return ginx.Result{
		Data: ListExecutionResp{
			Total: total,
			Executions: slice.Map(executions, func(_ int, src domain.TaskExecution) TaskExecutionVO {
				return TaskExecutionVO{
					ID:              src.ID,
					TaskID:          src.Task.ID,
					TaskName:        src.Task.Name,
					StartTime:       src.StartTime,
					EndTime:         src.EndTime,
					Status:          src.Status.String(),
					RunningProgress: src.RunningProgress,
					ExecutorNodeId:  src.ExecutorNodeID,
					TaskResult:      src.TaskResult,
					CTime:           src.CTime,
				}
			}),
		},
		Msg: "success",
	}, nil
}

func (h *Handler) ExecutionParameters(ctx *ginx.Context) (ginx.Result, error) {
	id, err := ctx.Param("id").AsInt64()
	if err != nil || id <= 0 {
		return invalidParameterResult(fmt.Errorf("执行 ID 非法")), nil
	}
	execution, err := h.execSvc.FindByID(ctx, id)
	if err != nil {
		return systemErrorResult, err
	}
	return ginx.Result{Data: toExecutionParametersVO(execution), Msg: "success"}, nil
}

func toExecutionParametersVO(execution domain.TaskExecution) ExecutionParametersVO {
	params := make(map[string]string)
	if execution.Task.GrpcConfig != nil {
		for key, value := range execution.Task.GrpcConfig.Params {
			params[key] = value
		}
	} else if execution.Task.HTTPConfig != nil {
		for key, value := range execution.Task.HTTPConfig.Params {
			params[key] = value
		}
	}
	// ParamOverrides is the authoritative record of manual-start overrides.
	// Apply it explicitly so the endpoint remains correct even when an older
	// execution snapshot did not fold the override into its protocol config.
	for key, value := range execution.ParamOverrides {
		params[key] = value
	}
	for key, value := range execution.Task.ScheduleParams {
		params[key] = value
	}

	parameters := make([]ExecutionParameterVO, 0, len(params))
	for _, key := range slices.Sorted(maps.Keys(params)) {
		_, manualOverride := execution.ParamOverrides[key]
		_, scheduleOverride := execution.Task.ScheduleParams[key]
		source := "TASK_SNAPSHOT"
		switch {
		case scheduleOverride:
			source = "SCHEDULE_OVERRIDE"
		case manualOverride:
			source = "MANUAL_OVERRIDE"
		}
		parameters = append(parameters, ExecutionParameterVO{
			Key: key, Value: params[key], Source: source,
			ManualOverride: manualOverride, ScheduleOverride: scheduleOverride,
		})
	}
	slices.SortStableFunc(parameters, func(left, right ExecutionParameterVO) int {
		leftOverridden := left.ManualOverride || left.ScheduleOverride
		rightOverridden := right.ManualOverride || right.ScheduleOverride
		if leftOverridden != rightOverridden {
			if leftOverridden {
				return -1
			}
			return 1
		}
		return strings.Compare(left.Key, right.Key)
	})
	return ExecutionParametersVO{
		ExecutionID: execution.ID, Parameters: parameters,
		ManualOverrideCount:   len(execution.ParamOverrides),
		ScheduleOverrideCount: len(execution.Task.ScheduleParams),
	}
}

func (h *Handler) Create(ctx *ginx.Context, req CreateTaskReq) (ginx.Result, error) {
	create, err := h.svc.Create(ctx, toDomain(req))
	if err != nil {
		return systemErrorResult, err
	}

	return ginx.Result{
		Data: create,
		Msg:  "success",
	}, nil
}

func (h *Handler) List(ctx *ginx.Context, req PageReq) (ginx.Result, error) {
	tasks, total, err := h.svc.List(ctx, bizid.Task, req.Offset, req.Limit)
	if err != nil {
		return systemErrorResult, err
	}

	return ginx.Result{
		Data: ListTaskResp{
			Total: total,
			Tasks: lo.Map(tasks, func(src domain.Task, _ int) TaskVO {
				return toVO(src)
			}),
		},
		Msg: "success",
	}, nil
}

func toVO(src domain.Task) TaskVO {
	vo := TaskVO{
		ID:                     src.ID,
		RunnerID:               src.RunnerID,
		Name:                   src.Name,
		Type:                   src.Type.String(),
		CronExpr:               src.CronExpr,
		Status:                 src.Status.String(),
		NextTime:               src.NextTime,
		MaxExecutionSeconds:    src.MaxExecutionSeconds,
		ScheduleParams:         src.ScheduleParams,
		Metadata:               src.Metadata,
		Program:                toProgramVO(src.Program),
		ParamOverrideRules:     src.ParamOverrideRules,
		ExecutionNotifications: toNotificationVOs(src.NotificationRules),
		CTime:                  src.CTime,
		UTime:                  src.UTime,
		Version:                src.Version,
	}

	if src.GrpcConfig != nil {
		vo.GrpcConfig = &GrpcConfig{
			ServiceName: src.GrpcConfig.ServiceName,
			HandlerName: src.GrpcConfig.HandlerName,
			Params:      src.GrpcConfig.Params,
		}
	}

	if src.HTTPConfig != nil {
		vo.HTTPConfig = &HTTPConfig{
			Endpoint: src.HTTPConfig.Endpoint,
			Headers:  src.HTTPConfig.Headers,
			Params:   src.HTTPConfig.Params,
		}
	}

	if src.RetryConfig != nil {
		vo.RetryConfig = &RetryConfig{
			MaxRetries:      src.RetryConfig.MaxRetries,
			MaxInterval:     src.RetryConfig.MaxInterval,
			InitialInterval: src.RetryConfig.InitialInterval,
		}
	}

	return vo
}

func toDomain(req CreateTaskReq) domain.Task {
	t := domain.Task{
		Name:                req.Name,
		RunnerID:            req.RunnerID,
		Type:                domain.TaskType(req.Type),
		CronExpr:            req.CronExpr,
		MaxExecutionSeconds: req.MaxExecutionSeconds,
		ScheduleParams:      req.ScheduleParams,
		Status:              domain.TaskStatusActive,
		BizID:               bizid.Task,
		Metadata:            req.Metadata,
		Program:             toDomainProgram(req.Program),
		ParamOverrideRules:  req.ParamOverrideRules,
		NotificationRules:   toDomainNotifications(req.ExecutionNotifications),
	}

	if req.GrpcConfig != nil {
		t.GrpcConfig = &domain.GrpcConfig{
			ServiceName: req.GrpcConfig.ServiceName,
			HandlerName: req.GrpcConfig.HandlerName,
			Params:      req.GrpcConfig.Params,
		}
	}

	if req.HTTPConfig != nil {
		t.HTTPConfig = &domain.HTTPConfig{
			Endpoint: req.HTTPConfig.Endpoint,
			Headers:  req.HTTPConfig.Headers,
			Params:   req.HTTPConfig.Params,
		}
	}

	if req.RetryConfig != nil {
		t.RetryConfig = &domain.RetryConfig{
			MaxRetries:      req.RetryConfig.MaxRetries,
			MaxInterval:     req.RetryConfig.MaxInterval,
			InitialInterval: req.RetryConfig.InitialInterval,
		}
	}

	return t
}

func toUpdateDomain(req UpdateTaskReq) domain.Task {
	t := domain.Task{
		ID:                  req.ID,
		RunnerID:            req.RunnerID,
		Name:                req.Name,
		Type:                domain.TaskType(req.Type),
		CronExpr:            req.CronExpr,
		MaxExecutionSeconds: req.MaxExecutionSeconds,
		ScheduleParams:      req.ScheduleParams,
		BizID:               bizid.Task,
		Metadata:            req.Metadata,
		Program:             toDomainProgram(req.Program),
		ParamOverrideRules:  req.ParamOverrideRules,
		NotificationRules:   toDomainNotifications(req.ExecutionNotifications),
	}

	if req.GrpcConfig != nil {
		t.GrpcConfig = &domain.GrpcConfig{
			ServiceName: req.GrpcConfig.ServiceName,
			HandlerName: req.GrpcConfig.HandlerName,
			Params:      req.GrpcConfig.Params,
		}
	}

	if req.HTTPConfig != nil {
		t.HTTPConfig = &domain.HTTPConfig{
			Endpoint: req.HTTPConfig.Endpoint,
			Headers:  req.HTTPConfig.Headers,
			Params:   req.HTTPConfig.Params,
		}
	}

	if req.RetryConfig != nil {
		t.RetryConfig = &domain.RetryConfig{
			MaxRetries:      req.RetryConfig.MaxRetries,
			MaxInterval:     req.RetryConfig.MaxInterval,
			InitialInterval: req.RetryConfig.InitialInterval,
		}
	}

	return t
}

func toDomainNotifications(src []ExecutionNotificationRule) []domain.ExecutionNotificationRule {
	return lo.Map(src, toDomainNotification)
}

func toNotificationVOs(src []domain.ExecutionNotificationRule) []ExecutionNotificationRule {
	return lo.Map(src, toNotificationVO)
}

func toDomainNotification(rule ExecutionNotificationRule, _ int) domain.ExecutionNotificationRule {
	return domain.ExecutionNotificationRule{
		TriggerStatus: domain.TaskExecutionStatus(rule.TriggerStatus),
		TemplateSetID: rule.TemplateSetID,
		Enabled:       rule.Enabled,
		Channels:      lo.Map(rule.Channels, toDomainNotificationChannel),
		Recipients:    lo.Map(rule.Recipients, toDomainNotificationRecipient),
	}
}

func toDomainNotificationChannel(channel string, _ int) domain.NotificationChannel {
	return domain.NotificationChannel(channel)
}

func toDomainNotificationRecipient(recipient NotificationRecipient, _ int) domain.NotificationRecipient {
	return domain.NotificationRecipient{
		Type:      domain.NotificationRecipientType(recipient.Type),
		TargetIDs: recipient.TargetIDs,
	}
}

func toNotificationVO(rule domain.ExecutionNotificationRule, _ int) ExecutionNotificationRule {
	return ExecutionNotificationRule{
		TriggerStatus: rule.TriggerStatus.String(),
		TemplateSetID: rule.TemplateSetID,
		Enabled:       rule.Enabled,
		Channels:      lo.Map(rule.Channels, toNotificationChannel),
		Recipients:    lo.Map(rule.Recipients, toNotificationRecipient),
	}
}

func toNotificationChannel(channel domain.NotificationChannel, _ int) string {
	return string(channel)
}

func toNotificationRecipient(recipient domain.NotificationRecipient, _ int) NotificationRecipient {
	return NotificationRecipient{
		Type:      string(recipient.Type),
		TargetIDs: recipient.TargetIDs,
	}
}

func toDomainProgram(src *ProgramSpec) *domain.ProgramSpec {
	if src == nil {
		return nil
	}
	result := &domain.ProgramSpec{Kind: domain.ProgramKind(src.Kind)}
	if src.Inline != nil {
		result.Inline = &domain.InlineProgramSpec{
			Code: src.Inline.Code, CodebookID: src.Inline.CodebookID,
		}
	}
	if src.Project != nil {
		result.Project = &domain.ProjectProgramSpec{EntryCodebookID: src.Project.EntryCodebookID}
	}
	return result
}

func toProgramVO(src *domain.ProgramSpec) *ProgramSpec {
	if src == nil {
		return nil
	}
	result := &ProgramSpec{Kind: string(src.Kind)}
	if src.Inline != nil {
		result.Inline = &InlineProgramSpec{
			Code: src.Inline.Code, CodebookID: src.Inline.CodebookID,
		}
	}
	if src.Project != nil {
		result.Project = &ProjectProgramSpec{EntryCodebookID: src.Project.EntryCodebookID}
	}
	return result
}
