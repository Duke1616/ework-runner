package web

import (
	"github.com/Duke1616/ecmdb/internal/execute/internal/domain"
	"github.com/Duke1616/ecmdb/internal/execute/internal/service"
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
	g := server.Group("/api/execute")
	g.POST("/start", ginx.WrapBody[WorkerReq](h.StartWorker))
	g.POST("/stop", ginx.WrapBody[WorkerReq](h.StopWorker))
}

func (h *Handler) StartWorker(ctx *gin.Context, req WorkerReq) (ginx.Result, error) {
	return ginx.Result{
		Msg: "启动服务成功",
	}, nil
}

func (h *Handler) StopWorker(ctx *gin.Context, req WorkerReq) (ginx.Result, error) {
	return ginx.Result{
		Msg: "停止服务成功",
	}, nil
}

func (h *Handler) toDomain(req WorkerReq) domain.Worker {
	return domain.Worker{
		Name:  req.Name,
		Desc:  req.Desc,
		Topic: req.Topic,
	}

}
