package codebook

import (
	"context"
	"errors"
	"fmt"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	"github.com/Duke1616/etask/internal/repository"
	"github.com/Duke1616/etask/pkg/sorter"
	"github.com/samber/lo"
	"golang.org/x/sync/errgroup"
)

//go:generate go tool mockgen -source=./codebook.go -package=codebookmocks -destination=./mocks/codebook.mock.go -typed

// Service 定义脚本模板及项目业务操作。
type Service interface {
	// Create 校验并创建脚本模板。
	Create(ctx context.Context, req domain.Codebook) (int64, error)
	// GetByID 根据主键 ID 获取脚本模板。
	GetByID(ctx context.Context, id int64) (domain.Codebook, error)
	// GetVersionByID 根据主键 ID 获取脚本版本。
	GetVersionByID(ctx context.Context, id int64) (domain.CodebookVersion, error)
	// ListVersions 获取指定脚本模板的版本列表。
	ListVersions(ctx context.Context, nodeID int64) ([]domain.CodebookVersion, error)
	// List 分页获取脚本模板列表和总数。
	List(ctx context.Context, offset, limit int64) ([]domain.Codebook, int64, error)
	// Children 获取指定项目和目录下的代码资源节点。
	Children(ctx context.Context, projectID, parentID int64) ([]domain.Codebook, error)
	// Update 校验并更新脚本模板。
	Update(ctx context.Context, req domain.Codebook) (int64, error)
	// CreateVersion 校验并创建脚本版本。
	CreateVersion(ctx context.Context, req domain.CodebookVersionCreate) (int64, error)
	// UseVersion 设置脚本模板当前使用版本。
	UseVersion(ctx context.Context, nodeID, versionID int64) (int64, error)
	// Sort 拖拽排序代码资源节点，支持跨目录移动。
	Sort(ctx context.Context, id, targetParentID, targetPosition int64) error
	// Delete 根据主键 ID 删除脚本模板。
	Delete(ctx context.Context, id int64) (int64, error)

	// CreateProject 校验并创建脚本项目。
	CreateProject(ctx context.Context, req domain.CodebookProject) (int64, error)
	// GetProjectByID 根据主键 ID 获取脚本项目。
	GetProjectByID(ctx context.Context, id int64) (domain.CodebookProject, error)
	// ListProjects 按状态分页获取脚本项目列表和总数。
	ListProjects(ctx context.Context, status domain.CodebookProjectStatus,
		offset, limit int64) ([]domain.CodebookProject, int64, error)
	// UpdateProject 校验并更新脚本项目。
	UpdateProject(ctx context.Context, req domain.CodebookProject) (int64, error)
	// ArchiveProject 根据主键 ID 归档脚本项目。
	ArchiveProject(ctx context.Context, id int64) (int64, error)
	// RestoreProject 根据主键 ID 恢复已归档的脚本项目。
	RestoreProject(ctx context.Context, id int64) (int64, error)
}

type service struct {
	repo   repository.ICodebookRepository
	sorter *sorter.Sorter[domain.CodebookSortItem, domain.CodebookSortItem]
}

// NewService 创建脚本模板服务。
func NewService(repo repository.ICodebookRepository) Service {
	return &service{
		repo: repo,
		sorter: sorter.NewSorter[domain.CodebookSortItem, domain.CodebookSortItem](
			func(elem domain.CodebookSortItem, idx int) domain.CodebookSortItem {
				elem.SortNo = int64(idx+1) * sorter.DefaultIndexGap
				return elem
			},
		),
	}
}

// Create 校验并创建脚本模板。
func (s *service) Create(ctx context.Context, req domain.Codebook) (int64, error) {
	if err := s.inheritParentContext(ctx, &req); err != nil {
		return 0, err
	}
	if err := req.Validate(); err != nil {
		return 0, err
	}
	if err := validateCodebookWriteScope(ctx, req.Scope); err != nil {
		return 0, err
	}
	if err := s.prepareSortNo(ctx, &req); err != nil {
		return 0, err
	}
	id, err := s.repo.Create(ctx, req)
	return id, codebookNameConflict(err, req.Name)
}

// GetByID 根据主键 ID 获取脚本模板。
func (s *service) GetByID(ctx context.Context, id int64) (domain.Codebook, error) {
	if id <= 0 {
		return domain.Codebook{}, fmt.Errorf("%w: 代码资源 ID 非法: %d", errs.ErrInvalidParameter, id)
	}
	return s.repo.GetByID(ctx, id)
}

// GetVersionByID 根据主键 ID 获取脚本版本。
func (s *service) GetVersionByID(ctx context.Context, id int64) (domain.CodebookVersion, error) {
	if id <= 0 {
		return domain.CodebookVersion{}, fmt.Errorf("%w: 版本 ID 非法: %d", errs.ErrInvalidParameter, id)
	}
	return s.repo.GetVersionByID(ctx, id)
}

// ListVersions 获取指定脚本模板的版本列表。
func (s *service) ListVersions(ctx context.Context, nodeID int64) ([]domain.CodebookVersion, error) {
	if nodeID <= 0 {
		return nil, fmt.Errorf("%w: 代码资源 ID 非法: %d", errs.ErrInvalidParameter, nodeID)
	}
	node, err := s.repo.GetNodeByID(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	if !node.IsFile() {
		return nil, fmt.Errorf("%w: 目录没有版本", errs.ErrInvalidParameter)
	}
	return s.repo.ListVersions(ctx, nodeID)
}

// List 分页获取脚本模板列表和总数。
func (s *service) List(ctx context.Context, offset, limit int64) ([]domain.Codebook, int64, error) {
	var (
		eg    errgroup.Group
		res   []domain.Codebook
		total int64
	)
	eg.Go(func() error {
		var err error
		res, err = s.repo.List(ctx, offset, limit)
		return err
	})
	eg.Go(func() error {
		var err error
		total, err = s.repo.Total(ctx)
		return err
	})
	if err := eg.Wait(); err != nil {
		return nil, 0, err
	}
	return res, total, nil
}

// Children 获取指定项目和目录下的代码资源节点。
func (s *service) Children(ctx context.Context, projectID, parentID int64) ([]domain.Codebook, error) {
	if parentID < 0 {
		return nil, fmt.Errorf("%w: 父级目录 ID 非法: %d", errs.ErrInvalidParameter, parentID)
	}
	return s.repo.ListChildren(ctx, projectID, parentID)
}

// Update 校验并更新脚本模板。
func (s *service) Update(ctx context.Context, req domain.Codebook) (int64, error) {
	if req.ID <= 0 {
		return 0, fmt.Errorf("%w: 代码资源 ID 非法: %d", errs.ErrInvalidParameter, req.ID)
	}
	old, err := s.repo.GetByID(ctx, req.ID)
	if err != nil {
		return 0, err
	}
	if req.Scope != "" && req.Scope != old.Scope {
		return 0, fmt.Errorf("%w: 代码资源作用域不能修改", errs.ErrInvalidParameter)
	}
	if req.ProjectID != 0 && req.ProjectID != old.ProjectID {
		return 0, fmt.Errorf("%w: 普通更新不能移动代码资源所属项目，请使用排序移动接口", errs.ErrInvalidParameter)
	}
	if req.Kind != "" && req.Kind != old.Kind {
		return 0, fmt.Errorf("%w: 代码资源节点类型不能修改", errs.ErrInvalidParameter)
	}
	req.MergeForUpdate(old)
	req.ProjectID = old.ProjectID
	req.ParentID = old.ParentID
	req.PathIDs = old.PathIDs
	req.Depth = old.Depth
	req.Kind = old.Kind
	if err = req.Validate(); err != nil {
		return 0, err
	}
	if err = validateCodebookWriteScope(ctx, req.Scope); err != nil {
		return 0, err
	}
	count, err := s.repo.Update(ctx, req)
	return count, codebookNameConflict(err, req.Name)
}

// CreateVersion 校验并创建脚本版本。
func (s *service) CreateVersion(ctx context.Context, req domain.CodebookVersionCreate) (int64, error) {
	if req.NodeID <= 0 {
		return 0, fmt.Errorf("%w: 代码资源 ID 非法: %d", errs.ErrInvalidParameter, req.NodeID)
	}
	node, err := s.repo.GetNodeByID(ctx, req.NodeID)
	if err != nil {
		return 0, err
	}
	if err = req.PrepareForNode(node); err != nil {
		return 0, err
	}
	if err = validateCodebookWriteScope(ctx, node.Scope); err != nil {
		return 0, err
	}
	return s.repo.CreateVersion(ctx, req)
}

// UseVersion 设置脚本模板当前使用版本。
func (s *service) UseVersion(ctx context.Context, nodeID, versionID int64) (int64, error) {
	if nodeID <= 0 {
		return 0, fmt.Errorf("%w: 代码资源 ID 非法: %d", errs.ErrInvalidParameter, nodeID)
	}
	if versionID <= 0 {
		return 0, fmt.Errorf("%w: 版本 ID 非法: %d", errs.ErrInvalidParameter, versionID)
	}
	node, err := s.repo.GetNodeByID(ctx, nodeID)
	if err != nil {
		return 0, err
	}
	if err = validateCodebookWriteScope(ctx, node.Scope); err != nil {
		return 0, err
	}
	return s.repo.UseVersion(ctx, nodeID, versionID)
}

// Sort 拖拽排序代码资源节点，支持跨目录移动。
func (s *service) Sort(ctx context.Context, id, targetParentID, targetPosition int64) error {
	if id <= 0 {
		return fmt.Errorf("%w: 代码资源 ID 非法: %d", errs.ErrInvalidParameter, id)
	}
	if targetParentID < 0 {
		return fmt.Errorf("%w: 目标父级目录 ID 非法: %d", errs.ErrInvalidParameter, targetParentID)
	}
	dragged, err := s.repo.GetNodeByID(ctx, id)
	if err != nil {
		return err
	}
	if err = validateCodebookWriteScope(ctx, dragged.Scope); err != nil {
		return err
	}
	draggedItem, err := s.resolveTarget(ctx, dragged, targetParentID)
	if err != nil {
		return err
	}
	children, err := s.repo.ListChildrenByScope(ctx, dragged.ProjectID, draggedItem.ParentID, dragged.Scope)
	if err != nil {
		return err
	}
	items := lo.Map(children, func(src domain.Codebook, _ int) domain.CodebookSortItem {
		return src.ToSortItem()
	})
	plan := s.sorter.PlanReorder(items, draggedItem, targetPosition)
	if plan.NeedRebalance {
		for i := range plan.Items {
			plan.Items[i].ParentID = draggedItem.ParentID
			plan.Items[i].ProjectID = dragged.ProjectID
			plan.Items[i].PathIDs = draggedItem.PathIDs
			plan.Items[i].Depth = draggedItem.Depth
		}
		return s.repo.BatchUpdateSort(ctx, plan.Items)
	}
	draggedItem.SortNo = plan.NewSortKey
	return s.repo.UpdateSort(ctx, draggedItem)
}

// Delete 根据主键 ID 删除脚本模板。
func (s *service) Delete(ctx context.Context, id int64) (int64, error) {
	if id <= 0 {
		return 0, fmt.Errorf("%w: 代码资源 ID 非法: %d", errs.ErrInvalidParameter, id)
	}
	node, err := s.repo.GetNodeByID(ctx, id)
	if err != nil {
		return 0, err
	}
	if err = validateCodebookWriteScope(ctx, node.Scope); err != nil {
		return 0, err
	}
	result, err := s.repo.Delete(ctx, id)
	return result.NodeCount, err
}

func (s *service) prepareSortNo(ctx context.Context, req *domain.Codebook) error {
	if req.SortNo > 0 {
		return nil
	}
	sortNo, err := s.repo.GetMaxSortNo(ctx, req.ProjectID, req.ParentID, req.Scope)
	if err != nil {
		return err
	}
	req.SortNo = sortNo + sorter.DefaultIndexGap
	return nil
}

func (s *service) resolveTarget(ctx context.Context, dragged domain.Codebook, targetParentID int64) (domain.CodebookSortItem, error) {
	if targetParentID == 0 {
		return dragged.ResolveMoveTarget(nil)
	}
	parent, err := s.repo.GetNodeByID(ctx, targetParentID)
	if err != nil {
		return domain.CodebookSortItem{}, err
	}
	return dragged.ResolveMoveTarget(&parent)
}

func (s *service) inheritParentContext(ctx context.Context, req *domain.Codebook) error {
	if req.ParentID == 0 {
		return nil
	}
	parent, err := s.repo.GetNodeByID(ctx, req.ParentID)
	if err != nil {
		return err
	}
	return req.ApplyParent(parent)
}

func validateCodebookWriteScope(ctx context.Context, scope domain.CodebookScope) error {
	tenantID := ctxutil.GetTenantID(ctx).Int64()
	return scope.ValidateWriteAccess(tenantID, ctxutil.SystemTenantID)
}

func codebookNameConflict(err error, name string) error {
	if errors.Is(err, errs.ErrCodebookNameConflict) {
		return fmt.Errorf("%w：%s", errs.ErrCodebookNameConflict, name)
	}
	return err
}

func (s *service) CreateProject(ctx context.Context, req domain.CodebookProject) (int64, error) {
	tenantID := ctxutil.GetTenantID(ctx).Int64()
	if tenantID <= 0 {
		return 0, fmt.Errorf("%w: 租户 ID 不能为空", errs.ErrInvalidParameter)
	}
	req.Scope = domain.CodebookScopeTenant
	if err := req.Validate(); err != nil {
		return 0, err
	}
	if err := s.validateArtifactNamespace(ctx, req.ArtifactNamespace, 0); err != nil {
		return 0, err
	}
	if req.SortNo <= 0 {
		sortNo, err := s.repo.GetProjectMaxSortNo(ctx)
		if err != nil {
			return 0, err
		}
		req.SortNo = sortNo + sorter.DefaultIndexGap
	}
	return s.repo.CreateProject(ctx, req)
}

func (s *service) GetProjectByID(ctx context.Context, id int64) (domain.CodebookProject, error) {
	if id <= 0 {
		return domain.CodebookProject{}, fmt.Errorf("%w: 项目 ID 非法: %d", errs.ErrInvalidParameter, id)
	}
	return s.repo.GetProjectByID(ctx, id)
}

func (s *service) ListProjects(ctx context.Context, status domain.CodebookProjectStatus,
	offset, limit int64) ([]domain.CodebookProject, int64, error) {
	if status == "" {
		status = domain.CodebookProjectStatusNormal
	}
	if !status.Valid() {
		return nil, 0, fmt.Errorf("%w: 项目状态非法: %s", errs.ErrInvalidParameter, status)
	}
	var (
		eg    errgroup.Group
		res   []domain.CodebookProject
		total int64
	)
	eg.Go(func() error {
		var err error
		res, err = s.repo.ListProjects(ctx, status, offset, limit)
		return err
	})
	eg.Go(func() error {
		var err error
		total, err = s.repo.TotalProjects(ctx, status)
		return err
	})
	if err := eg.Wait(); err != nil {
		return nil, 0, err
	}
	return res, total, nil
}

func (s *service) UpdateProject(ctx context.Context, req domain.CodebookProject) (int64, error) {
	if req.ID <= 0 {
		return 0, fmt.Errorf("%w: 项目 ID 非法: %d", errs.ErrInvalidParameter, req.ID)
	}
	old, err := s.repo.GetProjectByID(ctx, req.ID)
	if err != nil {
		return 0, err
	}
	if old.ArtifactNamespace != "" && req.ArtifactNamespace != "" && req.ArtifactNamespace != old.ArtifactNamespace {
		return 0, fmt.Errorf("%w: 制品库导入命名空间设置后不能修改", errs.ErrInvalidParameter)
	}
	if req.ArtifactNamespace == "" {
		req.ArtifactNamespace = old.ArtifactNamespace
	}
	req.MergeForUpdate(old)
	if err = req.Validate(); err != nil {
		return 0, err
	}
	if err = s.validateArtifactNamespace(ctx, req.ArtifactNamespace, req.ID); err != nil {
		return 0, err
	}
	return s.repo.UpdateProject(ctx, req)
}

func (s *service) validateArtifactNamespace(ctx context.Context, namespace string, excludeID int64) error {
	if namespace == "" {
		return nil
	}
	exists, err := s.repo.ArtifactNamespaceExists(ctx, namespace, excludeID)
	if err != nil {
		return fmt.Errorf("查询制品库导入命名空间失败: %w", err)
	}
	if exists {
		return fmt.Errorf("%w: 制品库导入命名空间 %s 已存在", errs.ErrInvalidParameter, namespace)
	}
	return nil
}

func (s *service) ArchiveProject(ctx context.Context, id int64) (int64, error) {
	if id <= 0 {
		return 0, fmt.Errorf("%w: 项目 ID 非法: %d", errs.ErrInvalidParameter, id)
	}
	return s.repo.ArchiveProject(ctx, id)
}

func (s *service) RestoreProject(ctx context.Context, id int64) (int64, error) {
	if id <= 0 {
		return 0, fmt.Errorf("%w: 项目 ID 非法: %d", errs.ErrInvalidParameter, id)
	}
	return s.repo.RestoreProject(ctx, id)
}
