package manager

import (
	"time"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/eiam/pkg/web/capability"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/service/task"
	"github.com/Duke1616/etask/internal/sse"
	"github.com/Duke1616/etask/pkg/grpc/interceptors/bizid"
	"github.com/ecodeclub/ekit/slice"
	"github.com/ecodeclub/ginx"
	"github.com/gin-gonic/gin"
)

var _ ginx.Handler = &Handler{}

type Handler struct {
	svc     task.Service
	logSvc  task.LogService
	execSvc task.ExecutionService
	events  *sse.Hubs
	capability.IRegistry
	executionRegistry capability.IRegistry
}

func (h *Handler) PublicRoutes(_ *gin.Engine) {

}

func (h *Handler) IdentifyRoutes(_ *gin.Engine) {}

func NewHandler(svc task.Service, logSvc task.LogService, execSvc task.ExecutionService,
	events *sse.Hubs) *Handler {
	return &Handler{
		svc:               svc,
		logSvc:            logSvc,
		execSvc:           execSvc,
		events:            events,
		IRegistry:         capability.NewRegistry("task", "manager", "任务管理"),
		executionRegistry: capability.NewRegistry("task", "execution", "任务执行"),
	}
}

func (h *Handler) PrivateRoutes(server *gin.Engine) {
	g := server.Group("/api/manager")
	// --- 任务事件 ---
	g.GET("/task-events/stream", h.Capability("订阅任务状态事件", "task_events").
		NoSync().
		Handle(ginx.W(h.StreamEvents)),
	)
	g.GET("/tasks/:id/executions/stream", h.Capability("订阅任务执行事件", "execution_events").
		NoSync().
		Handle(ginx.W(h.StreamTaskExecutions)),
	)
	g.GET("/executions/:id/logs/stream", h.executionRegistry.Capability("查看执行日志流", "logs").
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

	// --- 任务控制 ---
	g.POST("/stop/:id", h.Capability("停止任务", "stop").
		Handle(ginx.W(h.Stop)),
	)
	g.POST("/run", h.Capability("运行任务", "start").
		Handle(ginx.B[RunTaskReq](h.Run)),
	)
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
		})
	}

	return ginx.Result{
		Msg: "success",
	}, nil
}

func (h *Handler) Run(ctx *ginx.Context, req RunTaskReq) (ginx.Result, error) {
	err := h.svc.Run(ctx, req.ID, req.CronExpr)
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
	return ginx.Result{}, nil
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
	return ginx.Result{}, nil
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
	return ginx.Result{}, nil
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
			Tasks: sliceMap(tasks, func(src domain.Task) TaskVO {
				return toVO(src)
			}),
		},
		Msg: "success",
	}, nil
}

func sliceMap[T, R any](data []T, f func(src T) R) []R {
	res := make([]R, 0, len(data))
	for _, v := range data {
		res = append(res, f(v))
	}
	return res
}

func toVO(src domain.Task) TaskVO {
	vo := TaskVO{
		ID:                  src.ID,
		Name:                src.Name,
		Type:                src.Type.String(),
		CronExpr:            src.CronExpr,
		Status:              src.Status.String(),
		NextTime:            src.NextTime,
		MaxExecutionSeconds: src.MaxExecutionSeconds,
		ScheduleParams:      src.ScheduleParams,
		Metadata:            src.Metadata,
		Program:             toProgramVO(src.Program),
		CTime:               src.CTime,
		UTime:               src.UTime,
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
		Type:                domain.TaskType(req.Type),
		CronExpr:            req.CronExpr,
		MaxExecutionSeconds: req.MaxExecutionSeconds,
		ScheduleParams:      req.ScheduleParams,
		Status:              domain.TaskStatusActive,
		BizID:               bizid.Task,
		Metadata:            req.Metadata,
		Program:             toDomainProgram(req.Program),
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
		Name:                req.Name,
		Type:                domain.TaskType(req.Type),
		CronExpr:            req.CronExpr,
		MaxExecutionSeconds: req.MaxExecutionSeconds,
		ScheduleParams:      req.ScheduleParams,
		BizID:               bizid.Task,
		Metadata:            req.Metadata,
		Program:             toDomainProgram(req.Program),
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
