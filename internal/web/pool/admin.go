package pool

import (
	"context"
	"errors"
	"fmt"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/eiam/pkg/web/capability"
	"github.com/Duke1616/eiam/pkg/web/middleware"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	"github.com/Duke1616/etask/internal/repository"
	poolSvc "github.com/Duke1616/etask/internal/service/pool"
	"github.com/ecodeclub/ginx"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

var _ ginx.Handler = &AdminHandler{}

type AdminHandler struct {
	bindingSvc poolSvc.BindingService
	catalogSvc poolSvc.CatalogService
	capability.IRegistry
}

func NewAdminHandler(bindingSvc poolSvc.BindingService, catalogSvc poolSvc.CatalogService) *AdminHandler {
	return &AdminHandler{
		bindingSvc: bindingSvc,
		catalogSvc: catalogSvc,
		IRegistry: capability.NewRegistry("task", "execution-pool", "资源池管理").
			DefaultScope(capability.ScopeSystem),
	}
}

func (h *AdminHandler) PublicRoutes(_ *gin.Engine) {
}

func (h *AdminHandler) IdentifyRoutes(_ *gin.Engine) {
}

func (h *AdminHandler) PrivateRoutes(server *gin.Engine) {
	g := server.Group("/api/execution-pool/admin")

	g.POST("/list", h.Capability("资源池管理列表", "admin_view").
		Handle(ginx.B[ListPoolsReq](h.ListPools)),
	)
	g.POST("/bindings/list", h.Capability("资源池绑定管理列表", "admin_bindings_view").
		Needs("iam:tenant:view_by_ids").
		Handle(middleware.BTO[ListBindingsReq](h.ListBindings)),
	)
	g.POST("/bindings/bind", h.Capability("管理绑定资源池", "admin_bind").
		Handle(middleware.BTO[BindReq](h.Bind)),
	)
	g.DELETE("/bindings/unbind", h.Capability("管理解绑资源池", "admin_unbind").
		Handle(middleware.BTO[BindingKeyReq](h.Unbind)),
	)
	g.POST("/bindings/enable", h.Capability("管理启用资源池绑定", "admin_enable").
		Handle(middleware.BTO[BindingKeyReq](h.Enable)),
	)
	g.POST("/bindings/disable", h.Capability("管理禁用资源池绑定", "admin_disable").
		Handle(middleware.BTO[BindingKeyReq](h.Disable)),
	)
}

func (h *AdminHandler) ListPools(ctx *ginx.Context, req ListPoolsReq) (ginx.Result, error) {
	page, err := h.catalogSvc.ListPools(ctx, poolSvc.PoolListRequest{
		Offset:       req.Offset,
		Limit:        req.Limit,
		Keyword:      req.Keyword,
		Kind:         domain.ExecutionPoolKind(req.Kind),
		Transport:    domain.ExecutionTransport(req.Transport),
		DispatchMode: domain.ExecMode(req.DispatchMode),
		Status:       domain.ExecutionPoolStatus(req.Status),
	})
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{
		Data: ListPoolsResp{
			Total: page.Total,
			Pools: lo.Map(page.Pools, func(pool domain.ExecutionPool, _ int) PoolVO {
				return h.toPoolVO(pool)
			}),
		},
		Msg: "success",
	}, nil
}

func (h *AdminHandler) ListBindings(ctx *ginx.Context, req ListBindingsReq) (ginx.Result, error) {
	bindings, err := h.bindingSvc.AdminList(ctx, poolSvc.ListBindingsRequest{
		PoolName:   req.PoolName,
		Status:     domain.ExecutionPoolBindingStatus(req.Status),
		AllTenants: req.AllTenants,
	})
	if err != nil {
		return h.translateError(err), err
	}

	return ginx.Result{
		Data: ListBindingsResp{
			Bindings: lo.Map(bindings, func(binding domain.ExecutionPoolBinding, _ int) BindingVO {
				return h.toVO(binding)
			}),
		},
		Msg: "success",
	}, nil
}

func (h *AdminHandler) Bind(ctx *ginx.Context, req BindReq) (ginx.Result, error) {
	if err := requireTenantContext(ctx); err != nil {
		return h.translateError(err), err
	}
	err := h.bindingSvc.BindMany(ctx, poolSvc.BindingManyRequest{
		PoolName:     req.PoolName,
		HandlerNames: h.bindHandlerNames(req),
		Desc:         req.Desc,
	})
	if err != nil {
		return h.translateError(err), err
	}
	return successResult(), nil
}

func (h *AdminHandler) Unbind(ctx *ginx.Context, req BindingKeyReq) (ginx.Result, error) {
	if err := requireTenantContext(ctx); err != nil {
		return h.translateError(err), err
	}
	err := h.bindingSvc.Unbind(ctx, h.toBindingKey(req))
	if err != nil {
		return h.translateError(err), err
	}
	return successResult(), nil
}

func (h *AdminHandler) Enable(ctx *ginx.Context, req BindingKeyReq) (ginx.Result, error) {
	if err := requireTenantContext(ctx); err != nil {
		return h.translateError(err), err
	}
	err := h.bindingSvc.Enable(ctx, h.toBindingKey(req))
	if err != nil {
		return h.translateError(err), err
	}
	return successResult(), nil
}

func (h *AdminHandler) Disable(ctx *ginx.Context, req BindingKeyReq) (ginx.Result, error) {
	if err := requireTenantContext(ctx); err != nil {
		return h.translateError(err), err
	}
	err := h.bindingSvc.Disable(ctx, h.toBindingKey(req))
	if err != nil {
		return h.translateError(err), err
	}
	return successResult(), nil
}

func requireTenantContext(ctx context.Context) error {
	if ctxutil.GetTenantID(ctx).Int64() <= 0 {
		return fmt.Errorf("%w: 目标租户不能为空", errs.ErrInvalidParameter)
	}
	return nil
}

func (h *AdminHandler) bindHandlerNames(req BindReq) []string {
	if len(req.HandlerNames) > 0 {
		return req.HandlerNames
	}
	return []string{req.HandlerName}
}

func (h *AdminHandler) toBindingKey(req BindingKeyReq) poolSvc.BindingKey {
	return poolSvc.BindingKey{
		PoolName:    req.PoolName,
		HandlerName: req.HandlerName,
	}
}

func (h *AdminHandler) toPoolVO(pool domain.ExecutionPool) PoolVO {
	return PoolVO{
		ID:             pool.ID,
		Name:           pool.Name,
		Kind:           pool.Kind.String(),
		Transport:      pool.Transport.String(),
		DispatchMode:   pool.DispatchMode.String(),
		IsolationLevel: pool.IsolationLevel.String(),
		Desc:           pool.Desc,
		Status:         pool.Status.String(),
		Metadata:       pool.Metadata,
		CTime:          pool.CTime,
		UTime:          pool.UTime,
	}
}

func (h *AdminHandler) toVO(binding domain.ExecutionPoolBinding) BindingVO {
	handlerName := binding.HandlerName
	if binding.IsWildcard() {
		handlerName = domain.ExecutionPoolHandlerWildcard
	}
	return BindingVO{
		ID:          binding.ID,
		TenantID:    binding.TenantID,
		PoolName:    binding.PoolName,
		HandlerName: handlerName,
		Status:      binding.Status.String(),
		Desc:        binding.Desc,
		CTime:       binding.CTime,
		UTime:       binding.UTime,
	}
}

func (h *AdminHandler) translateError(err error) ginx.Result {
	switch {
	case errors.Is(err, errs.ErrInvalidParameter):
		return invalidParameterResult(err)
	case errors.Is(err, repository.ErrExecutionPoolNotFound),
		errors.Is(err, repository.ErrExecutionPoolBindingNotFound):
		return notFoundResult(err)
	default:
		return systemErrorResult
	}
}
