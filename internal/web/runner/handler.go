package runner

import (
	"errors"

	"github.com/Duke1616/eiam/pkg/web/capability"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	runnerSvc "github.com/Duke1616/etask/internal/service/runner"
	"github.com/ecodeclub/ekit/slice"
	"github.com/ecodeclub/ginx"
	"github.com/gin-gonic/gin"
)

var _ ginx.Handler = &Handler{}

type Handler struct {
	svc runnerSvc.Service
	capability.IRegistry
}

func NewHandler(svc runnerSvc.Service) *Handler {
	return &Handler{
		svc:       svc,
		IRegistry: capability.NewRegistry("task", "runner", "脚本引擎/执行单元"),
	}
}

func (h *Handler) PublicRoutes(_ *gin.Engine) {
}

func (h *Handler) IdentifyRoutes(_ *gin.Engine) {
}

func (h *Handler) PrivateRoutes(server *gin.Engine) {
	g := server.Group("/api/runner")
	g.POST("/register", h.Capability("注册执行单元", "add").
		Handle(ginx.B[RegisterRunnerReq](h.Register)),
	)
	g.POST("/list", h.Capability("执行单元列表", "view").
		Needs(
			"task:runner:ids",
			"task:runner:view_by_codebook_id",
			"task:runner:view_exclude_codebook_id",
		).
		Handle(ginx.B[ListRunnerReq](h.List)),
	)
	g.GET("/detail/:id", h.Capability("执行单元详情", "get").
		Handle(ginx.W(h.Detail)),
	)
	g.POST("/update", h.Capability("更新执行单元", "edit").
		Handle(ginx.B[UpdateRunnerReq](h.Update)),
	)
	g.DELETE("/delete/:id", h.Capability("删除执行单元", "delete").
		Handle(ginx.W(h.Delete)),
	)
	g.POST("/list/by_ids", h.Capability("批量查询执行单元", "ids").
		NoSync().
		Handle(ginx.B[ListRunnerByIDsReq](h.ListByIDs)),
	)
	g.GET("/list/:codebook_id", h.Capability("当前绑定执行单元", "view_by_codebook_id").
		NoSync().
		Handle(ginx.W(h.ListByCodebookID)),
	)
	g.POST("/list/exclude_codebook_id", h.Capability("复用执行单元", "view_exclude_codebook_id").
		NoSync().
		Handle(ginx.B[ListExcludeCodebookIDReq](h.ListExcludeCodebookID)),
	)
}

func (h *Handler) Register(ctx *ginx.Context, req RegisterRunnerReq) (ginx.Result, error) {
	id, err := h.svc.Create(ctx, h.toDomain(req))
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Msg: "success", Data: id}, nil
}

func (h *Handler) List(ctx *ginx.Context, req ListRunnerReq) (ginx.Result, error) {
	rs, total, err := h.svc.List(ctx, req.Offset, req.Limit, req.Keyword, req.Kind)
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Msg: "success", Data: h.toListResp(rs, total)}, nil
}

func (h *Handler) Detail(ctx *ginx.Context) (ginx.Result, error) {
	id, err := ctx.Param("id").AsInt64()
	if err != nil {
		return invalidIDResult, err
	}
	r, err := h.svc.FindByID(ctx, id)
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Msg: "success", Data: h.toVO(r)}, nil
}

func (h *Handler) Update(ctx *ginx.Context, req UpdateRunnerReq) (ginx.Result, error) {
	r, err := h.toUpdateDomain(ctx, req)
	if err != nil {
		return h.translateError(err), err
	}
	count, err := h.svc.Update(ctx, r)
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Msg: "success", Data: count}, nil
}

func (h *Handler) Delete(ctx *ginx.Context) (ginx.Result, error) {
	id, err := ctx.Param("id").AsInt64()
	if err != nil {
		return invalidIDResult, err
	}
	count, err := h.svc.Delete(ctx, id)
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Msg: "success", Data: count}, nil
}

func (h *Handler) ListByIDs(ctx *ginx.Context, req ListRunnerByIDsReq) (ginx.Result, error) {
	rs, err := h.svc.ListByIDs(ctx, req.IDs)
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Msg: "success", Data: h.toListResp(rs, int64(len(rs)))}, nil
}

func (h *Handler) ListByCodebookID(ctx *ginx.Context) (ginx.Result, error) {
	codebookID, err := ctx.Param("codebook_id").AsInt64()
	if err != nil {
		return invalidIDResult, err
	}
	rs, err := h.svc.ListByCodebookID(ctx, codebookID)
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Msg: "success", Data: h.toListResp(rs, int64(len(rs)))}, nil
}

func (h *Handler) ListExcludeCodebookID(ctx *ginx.Context, req ListExcludeCodebookIDReq) (ginx.Result, error) {
	rs, total, err := h.svc.ListExcludeCodebookID(ctx, req.Offset, req.Limit, req.CodebookID, req.Keyword, req.Kind)
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Msg: "success", Data: h.toListResp(rs, total)}, nil
}

func (h *Handler) translateError(err error) ginx.Result {
	if errors.Is(err, errs.ErrInvalidParameter) {
		return invalidParameterResult(err)
	}
	return systemErrorResult
}

func (h *Handler) toDomain(req RegisterRunnerReq) domain.Runner {
	return domain.Runner{
		Name:           req.Name,
		CodebookSecret: req.CodebookSecret,
		CodebookID:     req.CodebookID,
		ProgramKind:    domain.ProgramKind(req.ProgramKind),
		Tags:           req.Tags,
		Kind:           domain.RunnerKind(req.Kind),
		Variables:      h.toVariablesDomain(req.Variables),
		Action:         domain.RunnerActionRegistered,
		Target:         req.Target,
		Handler:        req.Handler,
		Desc:           req.Desc,
	}
}

func (h *Handler) toUpdateDomain(ctx *ginx.Context, req UpdateRunnerReq) (domain.Runner, error) {
	old, err := h.svc.FindByID(ctx, req.ID)
	if err != nil {
		return domain.Runner{}, err
	}
	oldVars := slice.ToMap(old.Variables, func(element domain.RunnerVariable) string {
		return element.Key
	})
	return domain.Runner{
		ID:             req.ID,
		Name:           req.Name,
		CodebookSecret: req.CodebookSecret,
		CodebookID:     req.CodebookID,
		ProgramKind:    domain.ProgramKind(req.ProgramKind),
		Tags:           req.Tags,
		Kind:           domain.RunnerKind(req.Kind),
		Variables:      h.toUpdateVariablesDomain(oldVars, req.Variables),
		Action:         domain.RunnerActionRegistered,
		Target:         req.Target,
		Handler:        req.Handler,
		Desc:           req.Desc,
	}, nil
}

func (h *Handler) toVariablesDomain(req []Variable) []domain.RunnerVariable {
	return slice.Map(req, func(_ int, src Variable) domain.RunnerVariable {
		return domain.RunnerVariable{
			Key:    src.Key,
			Secret: src.Secret,
			Value:  src.Value,
		}
	})
}

func (h *Handler) toUpdateVariablesDomain(oldVars map[string]domain.RunnerVariable, req []Variable) []domain.RunnerVariable {
	return slice.Map(req, func(_ int, src Variable) domain.RunnerVariable {
		value := src.Value
		if src.Secret {
			val, ok := oldVars[src.Key]
			if ok && src.Value == "" {
				value = val.Value
			}
		}
		return domain.RunnerVariable{
			Key:    src.Key,
			Secret: src.Secret,
			Value:  value,
		}
	})
}

func (h *Handler) toListResp(rs []domain.Runner, total int64) ListRunnersResp {
	return ListRunnersResp{
		Total: total,
		Runners: slice.Map(rs, func(_ int, src domain.Runner) RunnerVO {
			return h.toVO(src)
		}),
	}
}

func (h *Handler) toVO(req domain.Runner) RunnerVO {
	return RunnerVO{
		ID:          req.ID,
		Name:        req.Name,
		Kind:        req.Kind.String(),
		CodebookID:  req.CodebookID,
		ProgramKind: string(req.ProgramKind),
		Tags:        req.Tags,
		Desc:        req.Desc,
		Target:      req.Target,
		Handler:     req.Handler,
		CTime:       req.CTime,
		UTime:       req.UTime,
		Variables: slice.Map(req.Variables, func(_ int, src domain.RunnerVariable) Variable {
			if src.Secret {
				return Variable{Key: src.Key, Secret: src.Secret}
			}
			return Variable{Key: src.Key, Secret: src.Secret, Value: src.Value}
		}),
	}
}
