package variable

import (
	"errors"

	"github.com/Duke1616/eiam/pkg/web/capability"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	"github.com/Duke1616/etask/internal/pkg/security"
	variablepkg "github.com/Duke1616/etask/internal/pkg/variable"
	variableSvc "github.com/Duke1616/etask/internal/service/variable"
	"github.com/Duke1616/etask/pkg/contract/perm"
	"github.com/Duke1616/etask/pkg/cryptox"
	"github.com/ecodeclub/ginx"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

var _ ginx.Handler = &Handler{}

type Handler struct {
	svc variableSvc.Service
	capability.IRegistry
}

func NewHandler(svc variableSvc.Service) *Handler {
	return &Handler{
		svc:       svc,
		IRegistry: capability.NewRegistry("task", "variable", "全局变量"),
	}
}

func (h *Handler) PublicRoutes(_ *gin.Engine) {
}

func (h *Handler) IdentifyRoutes(_ *gin.Engine) {
}

func (h *Handler) PrivateRoutes(server *gin.Engine) {
	g := server.Group("/api/variable")
	g.POST("/create", h.Define("创建全局变量", "add").
		Bind(ginx.B[CreateReq](h.Create)),
	)
	g.POST("/list", h.Define("全局变量列表", "view").
		Needs(perm.Variable.Get).
		Bind(ginx.B[ListReq](h.List)),
	)
	g.GET("/detail/:id", h.Define("全局变量详情", "get").
		Bind(ginx.W(h.Detail)),
	)
	g.POST("/update", h.Define("更新全局变量", "edit").
		Needs(perm.Variable.Get).
		Bind(ginx.B[UpdateReq](h.Update)),
	)
	g.DELETE("/delete/:id", h.Define("删除全局变量", "delete").
		Needs(perm.Variable.Get).
		Bind(ginx.W(h.Delete)),
	)
}

func (h *Handler) Create(ctx *ginx.Context, req CreateReq) (ginx.Result, error) {
	id, err := h.svc.Create(ctx, h.toDomain(req))
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Data: id, Msg: "success"}, nil
}

func (h *Handler) Detail(ctx *ginx.Context) (ginx.Result, error) {
	id, err := ctx.Param("id").AsInt64()
	if err != nil {
		return invalidVariableIDError, err
	}
	variable, err := h.svc.FindByID(ctx, id)
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Data: h.toVO(variable), Msg: "success"}, nil
}

func (h *Handler) List(ctx *ginx.Context, req ListReq) (ginx.Result, error) {
	variables, total, err := h.svc.List(ctx, req.Offset, req.Limit, req.Keyword)
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{
		Msg: "success",
		Data: ListVariablesResp{
			Total: total,
			Variables: lo.Map(variables, func(src domain.Variable, _ int) VariableVO {
				return h.toVO(src)
			}),
		},
	}, nil
}

func (h *Handler) Update(ctx *ginx.Context, req UpdateReq) (ginx.Result, error) {
	variable, err := h.toUpdateDomain(ctx, req)
	if err != nil {
		return h.translateError(err), err
	}
	count, err := h.svc.Update(ctx, variable)
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Data: count, Msg: "success"}, nil
}

func (h *Handler) Delete(ctx *ginx.Context) (ginx.Result, error) {
	id, err := ctx.Param("id").AsInt64()
	if err != nil {
		return invalidVariableIDError, err
	}
	count, err := h.svc.Delete(ctx, id)
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Data: count, Msg: "success"}, nil
}

func (h *Handler) translateError(err error) ginx.Result {
	if errors.Is(err, errs.ErrInvalidParameter) {
		return invalidParameterResult(err)
	}
	return systemErrorResult
}

func (h *Handler) toDomain(req CreateReq) domain.Variable {
	return domain.Variable{
		Key:    req.Key,
		Value:  req.Value,
		Secret: req.Secret,
	}
}

func (h *Handler) toUpdateDomain(ctx *ginx.Context, req UpdateReq) (domain.Variable, error) {
	variable := domain.Variable{
		ID:     req.ID,
		Key:    req.Key,
		Value:  req.Value,
		Secret: req.Secret,
	}
	if variable.Secret && (variable.Value == "" || variable.Value == cryptox.DefaultMask) {
		old, err := h.svc.FindByID(ctx, req.ID)
		if err != nil {
			return domain.Variable{}, err
		}
		variable.KeepSecretValueFrom(old)
	}
	return variable, nil
}

func (h *Handler) toVO(req domain.Variable) VariableVO {
	masked := security.NewVariableMasker().MaskVariables([]variablepkg.Item{{
		Key: req.Key, Value: req.Value, Secret: req.Secret,
	}})[0]
	return VariableVO{
		ID:     req.ID,
		Key:    req.Key,
		Value:  masked.Value,
		Secret: req.Secret,
		CTime:  req.CTime,
		UTime:  req.UTime,
	}
}
