package web

import (
	"github.com/Duke1616/ecmdb/internal/runner/internal/domain"
	"github.com/Duke1616/ecmdb/internal/runner/internal/service"
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
	g := server.Group("/api/runner")
	g.POST("/register", ginx.WrapBody[RegisterRunnerReq](h.Register))
}

func (h *Handler) Register(ctx *gin.Context, req RegisterRunnerReq) (ginx.Result, error) {
	err := h.svc.Register(ctx, h.toDomain(req))

	return ginx.Result{}, err
}

func (h *Handler) toDomain(req RegisterRunnerReq) domain.Runner {
	return domain.Runner{
		CodebookUid:    req.CodebookUid,
		CodebookSecret: req.CodebookSecret,
		Name:           req.Name,
		Tags:           req.Tags,
		Desc:           req.Desc,
	}
}
