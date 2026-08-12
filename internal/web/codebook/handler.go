package codebook

import (
	"errors"
	"fmt"

	"github.com/Duke1616/eiam/pkg/web/capability"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	codebookSvc "github.com/Duke1616/etask/internal/service/codebook"
	"github.com/ecodeclub/ginx"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

var _ ginx.Handler = &Handler{}

type Handler struct {
	svc       codebookSvc.Service
	workspace codebookSvc.WorkspaceService
	files     codebookSvc.ProjectFileService
	deletion  codebookSvc.ProjectDeletionService
	capability.IRegistry
}

func NewHandler(svc codebookSvc.Service, workspace codebookSvc.WorkspaceService,
	files codebookSvc.ProjectFileService, deletion codebookSvc.ProjectDeletionService) *Handler {
	return &Handler{
		svc:       svc,
		workspace: workspace,
		files:     files,
		deletion:  deletion,
		IRegistry: capability.NewRegistry("task", "codebook", "脚本引擎"),
	}
}

func (h *Handler) PublicRoutes(_ *gin.Engine) {
}

func (h *Handler) PrivateRoutes(server *gin.Engine) {
	cb := func(name, code string) *capability.Builder {
		return h.Capability(name, code).Group("脚本引擎/脚本模板")
	}
	version := func(name, code string) *capability.Builder {
		return h.Capability(name, code).Group("脚本引擎/版本管理")
	}
	project := func(name, code string) *capability.Builder {
		return h.Capability(name, code).Group("脚本引擎/项目管理")
	}
	g := server.Group("/api/codebook")
	g.POST("/create", cb("创建模板", "add").
		Needs("task:codebook:import").
		Handle(ginx.B[CreateReq](h.Create)),
	)
	g.POST("/children", cb("代码资源子节点", "children").
		NoSync().
		Handle(ginx.B[ChildrenReq](h.Children)),
	)
	g.GET("/tree/:project_id", cb("代码资源树", "view_tree").
		Needs("task:codebook:children").
		Handle(ginx.W(h.Tree)),
	)
	g.POST("/workspace/file", cb("读取制品文件", "view_workspace_tree").
		Handle(ginx.B[WorkspaceFileReq](h.WorkspaceFile)),
	)
	g.GET("/detail/:id", cb("模板详情", "get").
		Needs("task:codebook:download").
		Handle(ginx.W(h.Detail)),
	)
	g.POST("/update", cb("更新模板", "edit").
		Handle(ginx.B[UpdateReq](h.Update)),
	)
	g.POST("/rename", cb("重命名模板", "rename").
		Needs("task:codebook:edit").
		Handle(ginx.B[RenameReq](h.Rename)),
	)
	g.POST("/sort", cb("模板排序", "sort").
		Handle(ginx.B[SortReq](h.Sort)),
	)
	g.POST("/import", cb("导入项目文件", "import").
		NoSync().
		Handle(ginx.W(h.Import)),
	)
	g.GET("/file/:id/download", cb("下载项目文件", "download").
		NoSync().
		Handle(ginx.W(h.Download)),
	)
	g.DELETE("/delete/:id", cb("删除脚本模板", "delete").
		Handle(ginx.W(h.Delete)),
	)

	vg := g.Group("/version")
	vg.POST("/create", version("创建版本", "add_version").
		Handle(ginx.B[CreateVersionReq](h.CreateVersion)),
	)
	vg.POST("/list", version("版本列表", "view_version").
		Handle(ginx.B[ListVersionsReq](h.ListVersions)),
	)
	vg.GET("/detail/:id", version("版本详情", "get_version").
		Handle(ginx.W(h.VersionDetail)),
	)
	vg.POST("/use", version("使用版本", "use_version").
		Handle(ginx.B[UseVersionReq](h.UseVersion)),
	)

	// 项目路由
	pg := g.Group("/project")
	pg.POST("/create", project("创建项目", "add_project").
		Handle(ginx.B[CreateProjectReq](h.CreateProject)),
	)
	pg.POST("/list", project("项目列表", "view_project").
		Needs("task:codebook:get_project", "task:codebook:reference_projects").
		Handle(ginx.B[ListProjectsReq](h.ListProject)),
	)
	pg.GET("/detail/:id", project("项目详情", "get_project").
		NoSync().
		Handle(ginx.W(h.ProjectDetail)),
	)
	pg.POST("/references", project("可引用项目列表", "reference_projects").
		NoSync().
		Handle(ginx.B[ListReferenceProjectsReq](h.ListReferenceProjects)),
	)
	pg.POST("/update", project("更新项目", "edit_project").
		Handle(ginx.B[UpdateProjectReq](h.UpdateProject)),
	)
	pg.POST("/archive/:id", project("归档项目", "delete_project").
		Handle(ginx.W(h.ArchiveProject)),
	)
	pg.POST("/restore/:id", project("恢复项目", "restore_project").
		Handle(ginx.W(h.RestoreProject)),
	)
	pg.GET("/delete-impact/:id", project("项目删除影响", "project_delete_impact").
		NoSync().
		Handle(ginx.W(h.ProjectDeleteImpact)),
	)
	pg.DELETE("/delete/:id", project("删除项目", "purge_project").
		Needs("task:codebook:project_delete_impact").
		Handle(ginx.B[ProjectDeleteReq](h.DeleteProject)),
	)
}

func (h *Handler) IdentifyRoutes(_ *gin.Engine) {
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
		return invalidCodebookIDError, err
	}
	c, err := h.svc.GetByID(ctx, id)
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Data: h.toVO(c), Msg: "success"}, nil
}

func (h *Handler) Children(ctx *ginx.Context, req ChildrenReq) (ginx.Result, error) {
	scope := domain.CodebookScope(req.Scope)
	if req.Scope != "" && !scope.Valid() {
		err := fmt.Errorf("%w: 作用域非法", errs.ErrInvalidParameter)
		return invalidParameterResult(err), err
	}
	var cs []domain.Codebook
	var err error
	if scope == domain.CodebookScopeSystem {
		cs, err = h.svc.SystemChildren(ctx, req.ProjectID, req.ParentID)
	} else {
		cs, err = h.svc.Children(ctx, req.ProjectID, req.ParentID)
	}
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Msg: "success", Data: h.toListResp(cs, int64(len(cs)))}, nil
}

func (h *Handler) Tree(ctx *ginx.Context) (ginx.Result, error) {
	projectID, err := ctx.Param("project_id").AsInt64()
	if err != nil {
		return invalidProjectIDError, err
	}
	scope := domain.CodebookScopeTenant
	scopeValue := ctx.Query("scope").StringOrDefault("")
	if scopeValue != "" {
		scope = domain.CodebookScope(scopeValue)
	}
	if !scope.Valid() {
		err = fmt.Errorf("%w: 作用域非法", errs.ErrInvalidParameter)
		return invalidParameterResult(err), err
	}
	var nodes []domain.WorkspaceNode
	if scope == domain.CodebookScopeSystem {
		nodes, err = h.workspace.SystemTree(ctx, projectID)
	} else {
		nodes, err = h.workspace.Tree(ctx, projectID)
	}
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Msg: "success", Data: WorkspaceTreeResp{Nodes: h.toWorkspaceNodes(nodes)}}, nil
}

// WorkspaceFile 读取工作区中已激活制品的不可变文件内容。
func (h *Handler) WorkspaceFile(ctx *ginx.Context, req WorkspaceFileReq) (ginx.Result, error) {
	code, err := h.workspace.ReadArtifactFile(ctx, req.ProjectID, req.ReleaseID, req.Digest, req.ArtifactPath)
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Msg: "success", Data: WorkspaceFileResp{Code: code}}, nil
}

func (h *Handler) Update(ctx *ginx.Context, req UpdateReq) (ginx.Result, error) {
	count, err := h.svc.Update(ctx, h.toUpdateDomain(req))
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Data: count, Msg: "success"}, nil
}

// Rename 重命名代码资源节点，保留节点 ID、版本和目录层级。
func (h *Handler) Rename(ctx *ginx.Context, req RenameReq) (ginx.Result, error) {
	count, err := h.svc.Rename(ctx, req.ID, req.Name)
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Data: count, Msg: "success"}, nil
}

func (h *Handler) Sort(ctx *ginx.Context, req SortReq) (ginx.Result, error) {
	if err := h.svc.Sort(ctx, req.ID, req.TargetParentID, req.TargetPosition); err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Msg: "排序成功"}, nil
}

func (h *Handler) Delete(ctx *ginx.Context) (ginx.Result, error) {
	id, err := ctx.Param("id").AsInt64()
	if err != nil {
		return invalidCodebookIDError, err
	}
	count, err := h.files.Delete(ctx, id)
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Data: count, Msg: "success"}, nil
}

func (h *Handler) CreateVersion(ctx *ginx.Context, req CreateVersionReq) (ginx.Result, error) {
	id, err := h.svc.CreateVersion(ctx, h.toVersionDomain(req))
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Data: id, Msg: "success"}, nil
}

func (h *Handler) ListVersions(ctx *ginx.Context, req ListVersionsReq) (ginx.Result, error) {
	versions, err := h.svc.ListVersions(ctx, req.NodeID)
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Msg: "success", Data: h.toVersionListResp(versions)}, nil
}

func (h *Handler) VersionDetail(ctx *ginx.Context) (ginx.Result, error) {
	id, err := ctx.Param("id").AsInt64()
	if err != nil {
		return invalidCodebookIDError, err
	}
	version, err := h.svc.GetVersionByID(ctx, id)
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Data: h.toVersionVO(version), Msg: "success"}, nil
}

func (h *Handler) UseVersion(ctx *ginx.Context, req UseVersionReq) (ginx.Result, error) {
	count, err := h.svc.UseVersion(ctx, req.NodeID, req.VersionID)
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Data: count, Msg: "success"}, nil
}

// 项目接口实现
func (h *Handler) CreateProject(ctx *ginx.Context, req CreateProjectReq) (ginx.Result, error) {
	id, err := h.svc.CreateProject(ctx, h.toProjectDomain(req))
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Data: id, Msg: "success"}, nil
}

func (h *Handler) ListProject(ctx *ginx.Context, req ListProjectsReq) (ginx.Result, error) {
	ps, total, err := h.svc.ListProjects(ctx, domain.CodebookProjectListQuery{
		Scope: domain.CodebookScope(req.Scope), Status: domain.CodebookProjectStatus(req.Status),
		Keyword: req.Keyword, Offset: req.Offset, Limit: req.Limit,
	})
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Msg: "success", Data: h.toProjectListResp(ps, total)}, nil
}

func (h *Handler) ProjectDetail(ctx *ginx.Context) (ginx.Result, error) {
	id, err := ctx.Param("id").AsInt64()
	if err != nil {
		return invalidProjectIDError, err
	}
	project, err := h.svc.GetProjectByID(ctx, id)
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Msg: "success", Data: h.toProjectVO(project)}, nil
}

func (h *Handler) ListReferenceProjects(ctx *ginx.Context, req ListReferenceProjectsReq) (ginx.Result, error) {
	ps, total, err := h.svc.ListReferenceProjects(ctx, req.Keyword, req.ExcludeProjectID, req.Offset, req.Limit)
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Msg: "success", Data: h.toProjectListResp(ps, total)}, nil
}

func (h *Handler) UpdateProject(ctx *ginx.Context, req UpdateProjectReq) (ginx.Result, error) {
	project := h.toUpdateProjectDomain(req)
	count, err := h.svc.UpdateProject(ctx, project)
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Data: count, Msg: "success"}, nil
}

func (h *Handler) ArchiveProject(ctx *ginx.Context) (ginx.Result, error) {
	id, err := ctx.Param("id").AsInt64()
	if err != nil {
		return invalidProjectIDError, err
	}
	count, err := h.svc.ArchiveProject(ctx, id)
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Data: count, Msg: "success"}, nil
}

func (h *Handler) RestoreProject(ctx *ginx.Context) (ginx.Result, error) {
	id, err := ctx.Param("id").AsInt64()
	if err != nil {
		return invalidProjectIDError, err
	}
	count, err := h.svc.RestoreProject(ctx, id)
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Data: count, Msg: "success"}, nil
}

func (h *Handler) ProjectDeleteImpact(ctx *ginx.Context) (ginx.Result, error) {
	id, err := ctx.Param("id").AsInt64()
	if err != nil {
		return invalidProjectIDError, err
	}
	impact, err := h.deletion.Preview(ctx, id)
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Msg: "success", Data: h.toProjectDeleteImpactVO(impact)}, nil
}

func (h *Handler) DeleteProject(ctx *ginx.Context, req ProjectDeleteReq) (ginx.Result, error) {
	id, err := ctx.Param("id").AsInt64()
	if err != nil {
		return invalidProjectIDError, err
	}
	err = h.deletion.Delete(ctx, id, req.ProjectName)
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{Msg: "项目已删除", Data: id}, nil
}

func (h *Handler) translateError(err error) ginx.Result {
	if errors.Is(err, errs.ErrCodebookNameConflict) {
		return codebookNameConflictResult(err)
	}
	if errors.Is(err, errs.ErrInvalidParameter) {
		return invalidParameterResult(err)
	}
	return systemErrorResult
}

func (h *Handler) toDomain(req CreateReq) domain.Codebook {
	return domain.Codebook{
		ProjectID: req.ProjectID,
		Name:      req.Name,
		Owner:     req.Owner,
		Code:      req.Code,
		ParentID:  req.ParentID,
		Scope:     domain.CodebookScope(req.Scope),
		Kind:      domain.CodebookKind(req.Kind),
		SortNo:    req.SortNo,
	}
}

func (h *Handler) toUpdateDomain(req UpdateReq) domain.Codebook {
	return domain.Codebook{
		ID:        req.ID,
		ProjectID: req.ProjectID,
		Name:      req.Name,
		Owner:     req.Owner,
		Code:      req.Code,
		Scope:     domain.CodebookScope(req.Scope),
		SortNo:    req.SortNo,
	}
}

func (h *Handler) toListResp(cs []domain.Codebook, total int64) ListCodebooksResp {
	return ListCodebooksResp{
		Total: total,
		Codebooks: lo.Map(cs, func(src domain.Codebook, _ int) Codebook {
			return h.toVO(src)
		}),
	}
}

func (h *Handler) toWorkspaceNodes(nodes []domain.WorkspaceNode) []WorkspaceNode {
	return lo.Map(nodes, func(node domain.WorkspaceNode, _ int) WorkspaceNode {
		return WorkspaceNode{
			Key: node.Key, SourceID: node.SourceID, ReleaseID: node.ReleaseID,
			Digest: node.Digest, ArtifactPath: node.ArtifactPath, Name: node.Name,
			Owner: node.Owner, Kind: node.Kind.String(), Scope: node.Scope.String(),
			Layer: string(node.Layer), RuntimePath: node.RuntimePath, Readonly: node.Readonly,
			ProjectID: node.ProjectID, ParentID: node.ParentID, SortNo: node.SortNo,
			DownloadOnly: node.DownloadOnly, Size: node.Size,
			CTime: node.CTime, UTime: node.UTime,
			Namespace: node.Namespace, Children: h.toWorkspaceNodes(node.Children),
		}
	})
}

func (h *Handler) toVO(req domain.Codebook) Codebook {
	return Codebook{
		ID:               req.ID,
		TenantID:         req.TenantID,
		Scope:            req.Scope.String(),
		ProjectID:        req.ProjectID,
		ParentID:         req.ParentID,
		PathIDs:          req.PathIDs,
		Depth:            req.Depth,
		Name:             req.Name,
		Owner:            req.Owner,
		Kind:             req.Kind.String(),
		SortNo:           req.SortNo,
		Code:             req.Code,
		Size:             req.Size,
		DownloadOnly:     req.StorageType == domain.CodebookContentBlob,
		Secret:           req.Secret,
		CurrentVersionID: req.CurrentVersionID,
		CurrentVersionNo: req.CurrentVersionNo,
		CTime:            req.CTime,
		UTime:            req.UTime,
	}
}

func (h *Handler) toVersionDomain(req CreateVersionReq) domain.CodebookVersionCreate {
	return domain.CodebookVersionCreate{
		NodeID:  req.NodeID,
		Code:    req.Code,
		Message: req.Message,
	}
}

func (h *Handler) toVersionListResp(versions []domain.CodebookVersion) ListVersionsResp {
	return ListVersionsResp{
		Versions: lo.Map(versions, func(src domain.CodebookVersion, _ int) Version {
			return h.toVersionVO(src)
		}),
	}
}

func (h *Handler) toVersionVO(req domain.CodebookVersion) Version {
	return Version{
		ID:           req.ID,
		NodeID:       req.NodeID,
		TenantID:     req.TenantID,
		Scope:        req.Scope.String(),
		VersionNo:    req.VersionNo,
		Code:         req.Code,
		Hash:         req.Hash,
		Message:      req.Message,
		AuthorUserID: req.AuthorUserID,
		CTime:        req.CTime,
	}
}

func (h *Handler) toProjectDomain(req CreateProjectReq) domain.CodebookProject {
	return domain.CodebookProject{
		Name:              req.Name,
		Desc:              req.Desc,
		ArtifactEnabled:   req.ArtifactEnabled,
		ArtifactNamespace: req.ArtifactNamespace,
	}
}

func (h *Handler) toUpdateProjectDomain(req UpdateProjectReq) domain.CodebookProject {
	return domain.CodebookProject{
		ID:                req.ID,
		Name:              req.Name,
		Desc:              req.Desc,
		SortNo:            req.SortNo,
		ArtifactEnabled:   req.ArtifactEnabled,
		ArtifactNamespace: req.ArtifactNamespace,
	}
}

func (h *Handler) toProjectListResp(ps []domain.CodebookProject, total int64) ListProjectsResp {
	return ListProjectsResp{
		Total: total,
		Projects: lo.Map(ps, func(src domain.CodebookProject, _ int) Project {
			return h.toProjectVO(src)
		}),
	}
}

func (h *Handler) toProjectVO(req domain.CodebookProject) Project {
	return Project{
		ID:                req.ID,
		TenantID:          req.TenantID,
		Scope:             req.Scope.String(),
		Name:              req.Name,
		Desc:              req.Desc,
		SortNo:            req.SortNo,
		Status:            req.Status.String(),
		ArtifactEnabled:   req.ArtifactEnabled,
		ArtifactNamespace: req.ArtifactNamespace,
		SourceRevision:    req.SourceRevision,
		CTime:             req.CTime,
		UTime:             req.UTime,
	}
}

func (h *Handler) toProjectDeleteImpactVO(req domain.ProjectDeleteImpact) ProjectDeleteImpact {
	return ProjectDeleteImpact{
		TaskCount: req.TaskCount, ActiveTaskCount: req.ActiveTaskCount,
		CodebookNodeCount: req.CodebookNodeCount, CodebookVersionCount: req.CodebookVersionCount,
		ArtifactReleaseCount: req.ArtifactReleaseCount,
		ArtifactReleaseBytes: req.ArtifactReleaseBytes, RetainedArtifactReleaseCount: req.RetainedArtifactReleaseCount,
		ProjectSourceCount: req.ProjectSourceCount, ProjectSourceBytes: req.ProjectSourceBytes,
		RetainedProjectSourceCount: req.RetainedProjectSourceCount, AIConversationCount: req.AIConversationCount,
	}
}
