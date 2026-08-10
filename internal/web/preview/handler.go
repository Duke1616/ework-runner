package preview

import (
	"github.com/Duke1616/eiam/pkg/web/capability"
	"github.com/Duke1616/etask/internal/domain"
	previewSvc "github.com/Duke1616/etask/internal/service/preview"
	"github.com/ecodeclub/ekit/slice"
	"github.com/ecodeclub/ginx"
	"github.com/gin-gonic/gin"
)

var _ ginx.Handler = &Handler{}

type Handler struct {
	svc previewSvc.Service
	capability.IRegistry
}

// NewHandler 创建程序试运行 Web 处理器。
func NewHandler(svc previewSvc.Service) *Handler {
	return &Handler{
		svc:       svc,
		IRegistry: capability.NewRegistry("task", "preview", "脚本引擎"),
	}
}

// PublicRoutes 注册无需登录的公开路由。
func (h *Handler) PublicRoutes(_ *gin.Engine) {}

// IdentifyRoutes 注册只要求登录的路由。
func (h *Handler) IdentifyRoutes(_ *gin.Engine) {}

// PrivateRoutes 注册需要权限校验的试运行路由。
func (h *Handler) PrivateRoutes(server *gin.Engine) {
	permission := func(name, code string) *capability.Builder {
		return h.Capability(name, code).Group("脚本引擎/试运行")
	}
	g := server.Group("/api/codebook/preview")
	g.POST("/run", permission("执行脚本试运行", "run").
		Handle(ginx.B[RunReq](h.Run)),
	)
	g.POST("/status", permission("查看试运行", "view").
		Needs("task:execution:logs", "task:preview:view_logs").
		Handle(ginx.B[StatusReq](h.Status)),
	)
	g.POST("/logs", permission("查看试运行日志", "view_logs").
		NoSync().
		Handle(ginx.B[LogsReq](h.Logs)),
	)
}

// Run 创建并派发一次程序试运行。
func (h *Handler) Run(ctx *ginx.Context, req RunReq) (ginx.Result, error) {
	execution, err := h.svc.Run(ctx, previewSvc.RunCommand{
		RunnerID:            req.RunnerID,
		Args:                req.Args,
		MaxExecutionSeconds: req.MaxExecutionSeconds,
		Variables: slice.Map(req.Variables, func(_ int, variable Variable) domain.RunnerVariable {
			return domain.RunnerVariable{Key: variable.Key, Value: variable.Value, Secret: variable.Secret}
		}),
	})
	if err != nil {
		return invalidParameterResult(err), err
	}
	return ginx.Result{Data: toExecutionVO(execution), Msg: "试运行已开始"}, nil
}

// Status 查询一次程序试运行的最新状态。
func (h *Handler) Status(ctx *ginx.Context, req StatusReq) (ginx.Result, error) {
	execution, err := h.svc.Status(ctx, req.ExecutionID)
	if err != nil {
		return systemErrorResult, err
	}
	return ginx.Result{Data: toExecutionVO(execution), Msg: "查询成功"}, nil
}

// Logs 查询一次程序试运行的日志。
func (h *Handler) Logs(ctx *ginx.Context, req LogsReq) (ginx.Result, error) {
	logs, total, err := h.svc.Logs(ctx, req.ExecutionID, req.MinID, req.Limit)
	if err != nil {
		return systemErrorResult, err
	}
	return ginx.Result{Data: LogsResp{
		Total: total,
		Logs: slice.Map(logs, func(_ int, log domain.TaskExecutionLog) LogVO {
			return LogVO{ID: log.ID, ExecutionID: log.ExecutionID, Content: log.Content, CTime: log.CTime}
		}),
	}, Msg: "查询成功"}, nil
}

func toExecutionVO(execution domain.TaskExecution) ExecutionVO {
	return ExecutionVO{
		ID:              execution.ID,
		TaskName:        execution.Task.Name,
		StartTime:       execution.StartTime,
		EndTime:         execution.EndTime,
		Status:          execution.Status.String(),
		RunningProgress: execution.RunningProgress,
		ExecutorNodeID:  execution.ExecutorNodeID,
		TaskResult:      execution.TaskResult,
		CTime:           execution.CTime,
	}
}
