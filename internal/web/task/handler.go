package task

import (
	"github.com/Duke1616/ework-runner/internal/domain"
	"github.com/Duke1616/ework-runner/internal/service/task"
	"github.com/ecodeclub/ginx"
	"github.com/gin-gonic/gin"
)

var _ ginx.Handler = &Handler{}

type Handler struct {
	svc task.Service
}

func (h *Handler) PublicRoutes(_ *gin.Engine) {
}

func NewHandler(svc task.Service) *Handler {
	return &Handler{
		svc: svc,
	}
}

func (h *Handler) PrivateRoutes(server *gin.Engine) {
	g := server.Group("/api/task")
	g.POST("/create", ginx.B[CreateTaskReq](h.Create))
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

func toDomain(req CreateTaskReq) domain.Task {
	return domain.Task{
		Name:                req.Name,
		Type:                domain.TaskType(req.Type),
		CronExpr:            req.CronExpr,
		MaxExecutionSeconds: req.MaxExecutionSeconds,
		ScheduleParams:      req.ScheduleParams,
		GrpcConfig: &domain.GrpcConfig{
			ServiceName: req.GrpcConfig.ServiceName,
			HandlerName: req.GrpcConfig.HandlerName,
			Params:      req.GrpcConfig.Params,
		},
		HTTPConfig: &domain.HTTPConfig{
			Endpoint: req.HTTPConfig.Endpoint,
			Params:   req.HTTPConfig.Params,
		},
		RetryConfig: &domain.RetryConfig{
			MaxRetries:      req.RetryConfig.MaxRetries,
			MaxInterval:     req.RetryConfig.MaxInterval,
			InitialInterval: req.RetryConfig.InitialInterval,
		},
		Status:  domain.TaskStatusActive,
		Version: 1,
	}
}
