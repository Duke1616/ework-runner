package web

import (
	"github.com/Duke1616/ecmdb/internal/worker/internal/domain"
	"github.com/Duke1616/ecmdb/internal/worker/internal/service"
	"github.com/Duke1616/ecmdb/pkg/ginx"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	svc service.Service
}

func NewHandler(svc service.Service) *Handler {
	return &Handler{
		svc: svc,
	}
}

func (h *Handler) RegisterRoutes(server *gin.Engine) {
	g := server.Group("/api/worker")
	g.POST("/start", ginx.WrapBody[StartWorkerReq](h.StartWorker))
	g.POST("/stop", ginx.WrapBody[StopWorker](h.StopWorker))
}

func (h *Handler) StartWorker(ctx *gin.Context, req StartWorkerReq) (ginx.Result, error) {
	if err := h.svc.Start(ctx, h.toDomain(req)); err != nil {
		return ginx.Result{}, err
	}

	return ginx.Result{
		Msg: "启动服务成功",
	}, nil
}

func (h *Handler) StopWorker(ctx *gin.Context, req StopWorker) (ginx.Result, error) {
	return ginx.Result{}, nil
}

func (h *Handler) toDomain(req StartWorkerReq) domain.Worker {
	return domain.Worker{
		Name:  req.Name,
		Desc:  req.Desc,
		Topic: req.Topic,
	}

}
