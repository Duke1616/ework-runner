package resource

import (
	"encoding/json"

	"github.com/Duke1616/eiam/pkg/web/capability"
	"github.com/Duke1616/etask/internal/domain"
	poolSvc "github.com/Duke1616/etask/internal/service/pool"
	"github.com/ecodeclub/ginx"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

var _ ginx.Handler = &Handler{}

type Handler struct {
	catalogSvc poolSvc.CatalogService
	capability.IRegistry
}

func NewHandler(catalogSvc poolSvc.CatalogService) *Handler {
	return &Handler{
		catalogSvc: catalogSvc,
		IRegistry:  capability.NewRegistry("task", "resource", "执行资源"),
	}
}

func (h *Handler) PublicRoutes(_ *gin.Engine) {
}

func (h *Handler) IdentifyRoutes(_ *gin.Engine) {
}

func (h *Handler) PrivateRoutes(server *gin.Engine) {
	g := server.Group("/api/resource")

	g.GET("/list", h.Define("执行资源列表", "view").
		Bind(ginx.B[ListReq](h.List)),
	)
}

func (h *Handler) List(ctx *ginx.Context, req ListReq) (ginx.Result, error) {
	page, err := h.catalogSvc.ListAuthorizedPools(ctx, poolSvc.CatalogListRequest{
		Kind:    domain.ExecutionPoolKind(req.Kind),
		Offset:  req.Offset,
		Limit:   req.Limit,
		Keyword: req.Keyword,
	})
	if err != nil {
		return systemErrorResult, err
	}

	matcher := poolSvc.NewBindingMatcher(page.Bindings)
	nodesByPool, err := h.catalogSvc.ListNodesForPools(ctx, page.Pools)
	if err != nil {
		return systemErrorResult, err
	}
	resources := make([]ResourceVO, 0, len(page.Pools))
	for _, pool := range page.Pools {
		resource := h.toVO(pool, nodesByPool[pool.Name])
		if len(resource.Handlers) == 0 {
			if matcher.Allow(pool.Name, "") {
				resources = append(resources, resource)
			}
			continue
		}
		resource.Handlers = lo.Filter(resource.Handlers, func(handler HandlerDetail, _ int) bool {
			return matcher.Allow(pool.Name, handler.Name)
		})
		if len(resource.Handlers) > 0 {
			resources = append(resources, resource)
		}
	}

	return ginx.Result{
		Data: ListResp{
			Resources: resources,
			Total:     page.Total,
		},
		Msg: "success",
	}, nil
}

func (h *Handler) toVO(pool domain.ExecutionPool, nodes []poolSvc.Node) ResourceVO {
	return ResourceVO{
		Name:           pool.Name,
		Desc:           pool.Desc,
		Kind:           pool.Kind.String(),
		Transport:      pool.Transport.String(),
		DispatchMode:   pool.DispatchMode.String(),
		IsolationLevel: pool.IsolationLevel.String(),
		Topic:          pool.Metadata["topic"],
		Handlers:       parseHandlers(pool.Metadata["supported_handlers"]),
		Nodes:          toNodeDetails(nodes),
	}
}

func toNodeDetails(nodes []poolSvc.Node) []NodeDetail {
	return lo.Map(nodes, func(node poolSvc.Node, _ int) NodeDetail {
		return NodeDetail{
			ID:      node.ID,
			Address: node.Address,
		}
	})
}

func parseHandlers(raw string) []HandlerDetail {
	if raw == "" {
		return nil
	}
	var handlers []HandlerDetail
	_ = json.Unmarshal([]byte(raw), &handlers)
	return handlers
}
