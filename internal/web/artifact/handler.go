package artifact

import (
	"errors"

	"github.com/Duke1616/eiam/pkg/web/capability"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	artifactSvc "github.com/Duke1616/etask/internal/service/artifact"
	"github.com/Duke1616/etask/pkg/contract/perm"
	"github.com/ecodeclub/ginx"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

var _ ginx.Handler = &Handler{}

type Handler struct {
	svc artifactSvc.Service
	capability.IRegistry
}

func NewHandler(svc artifactSvc.Service) *Handler {
	return &Handler{
		svc:       svc,
		IRegistry: capability.NewRegistry("task", "artifact", "脚本引擎/制品仓库"),
	}
}

func (h *Handler) PublicRoutes(_ *gin.Engine) {}

func (h *Handler) IdentifyRoutes(_ *gin.Engine) {}

func (h *Handler) PrivateRoutes(server *gin.Engine) {
	g := server.Group("/api/artifact")
	g.POST("/publish", h.Define("发布制品", "publish").
		Bind(ginx.B[PublishReq](h.Publish)),
	)
	g.POST("/list", h.Define("制品发布记录", "view").
		Needs(perm.Artifact.Status).
		Bind(ginx.B[ListReq](h.List)),
	)
	g.POST("/activate", h.Define("切换制品", "activate").
		Bind(ginx.B[ActivateReq](h.Activate)),
	)
	g.POST("/status", h.Define("制品状态", "status").
		NoSync().
		Bind(ginx.B[StatusReq](h.Status)),
	)
}

func (h *Handler) Publish(ctx *ginx.Context, req PublishReq) (ginx.Result, error) {
	release, err := h.svc.Publish(ctx, target(req.Scope, req.ProjectID), req.Message)
	if err != nil {
		return translateError(err), err
	}
	return ginx.Result{Data: toReleaseVO(release), Msg: "发布成功"}, nil
}

func (h *Handler) List(ctx *ginx.Context, req ListReq) (ginx.Result, error) {
	releases, total, err := h.svc.List(ctx, target(req.Scope, req.ProjectID), req.Offset, req.Limit)
	if err != nil {
		return translateError(err), err
	}
	return ginx.Result{Data: ListResp{
		Total: total,
		Releases: lo.Map(releases, func(release domain.ArtifactRelease, _ int) Release {
			return toReleaseVO(release)
		}),
	}, Msg: "查询成功"}, nil
}

func (h *Handler) Activate(ctx *ginx.Context, req ActivateReq) (ginx.Result, error) {
	if err := h.svc.Activate(ctx, target(req.Scope, req.ProjectID), req.ID); err != nil {
		return translateError(err), err
	}
	return ginx.Result{Msg: "切换成功"}, nil
}

func (h *Handler) Status(ctx *ginx.Context, req StatusReq) (ginx.Result, error) {
	status, err := h.svc.Status(ctx, target(req.Scope, req.ProjectID))
	if err != nil {
		return translateError(err), err
	}
	return ginx.Result{Data: toStatusVO(status), Msg: "查询成功"}, nil
}

func target(scope string, projectID int64) domain.ArtifactTarget {
	if scope == "" {
		scope = domain.CodebookScopeSystem.String()
	}
	return domain.ArtifactTarget{Scope: domain.CodebookScope(scope), ProjectID: projectID}
}

func toReleaseVO(release domain.ArtifactRelease) Release {
	return Release{
		ID:             release.ID,
		TenantID:       release.TenantID,
		Scope:          release.Scope.String(),
		ProjectID:      release.ProjectID,
		SourceRevision: release.SourceRevision,
		Digest:         release.Digest,
		BlobChecksum:   release.BlobChecksum,
		Size:           release.Size,
		Format:         release.Format,
		FormatVersion:  release.FormatVersion,
		Message:        release.Message,
		AuthorUserID:   release.AuthorUserID,
		Active:         release.Active,
		CTime:          release.CTime,
	}
}

func toStatusVO(status domain.ArtifactStatus) Status {
	result := Status{
		Scope:          status.Target.Scope.String(),
		ProjectID:      status.Target.ProjectID,
		SourceRevision: status.SourceRevision,
		PendingChanges: status.PendingChanges,
	}
	if status.Active != nil {
		active := toReleaseVO(*status.Active)
		result.Active = &active
	}
	return result
}

func translateError(err error) ginx.Result {
	if errors.Is(err, errs.ErrInvalidParameter) {
		return invalidParameterResult(err)
	}
	return systemErrorResult
}
