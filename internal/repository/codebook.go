package repository

import (
	"context"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository/dao"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"gorm.io/gorm"
)

// ErrCodebookNotFound 表示脚本模板不存在。
var ErrCodebookNotFound = gorm.ErrRecordNotFound

// ICodebookRepository 定义代码资源及项目的领域仓储操作。
type ICodebookRepository interface {
	// Create 保存代码资源，未传入 Secret 时自动生成。
	Create(ctx context.Context, req domain.Codebook) (int64, error)
	// GetByID 根据主键 ID 加载代码资源。
	GetByID(ctx context.Context, id int64) (domain.Codebook, error)
	// GetNodeByID 根据主键 ID 加载代码节点元信息，不加载源码内容。
	GetNodeByID(ctx context.Context, id int64) (domain.Codebook, error)
	// ListVersions 查询指定代码节点下的所有版本。
	ListVersions(ctx context.Context, nodeID int64) ([]domain.CodebookVersion, error)
	// GetVersionByID 根据主键 ID 加载代码版本。
	GetVersionByID(ctx context.Context, id int64) (domain.CodebookVersion, error)
	// List 分页查询代码资源。
	List(ctx context.Context, offset, limit int64) ([]domain.Codebook, error)
	// ListChildren 查询指定项目和父节点下的子节点。
	ListChildren(ctx context.Context, projectID, parentID int64) ([]domain.Codebook, error)
	// ListChildrenByScope 查询指定租户项目或系统作用域下的子节点。
	ListChildrenByScope(ctx context.Context, projectID, parentID int64, scope domain.CodebookScope) ([]domain.Codebook, error)
	// Tree 查询指定项目下的全部源码节点。
	Tree(ctx context.Context, projectID int64) ([]domain.Codebook, error)
	// Total 统计代码资源总数。
	Total(ctx context.Context) (int64, error)
	// GetMaxSortNo 查询指定租户项目、作用域和父节点下最大的排序号。
	GetMaxSortNo(ctx context.Context, projectID, parentID int64, scope domain.CodebookScope) (int64, error)
	// Update 更新代码资源可变字段。
	Update(ctx context.Context, req domain.Codebook) (int64, error)
	// CreateVersion 创建代码版本。
	CreateVersion(ctx context.Context, req domain.CodebookVersionCreate) (int64, error)
	// UseVersion 设置代码节点当前使用版本。
	UseVersion(ctx context.Context, nodeID, versionID int64) (int64, error)
	// UpdateSort 更新单个代码资源排序。
	UpdateSort(ctx context.Context, item domain.CodebookSortItem) error
	// BatchUpdateSort 批量更新代码资源排序。
	BatchUpdateSort(ctx context.Context, items []domain.CodebookSortItem) error
	// Import 在一个事务中导入项目文件树。
	Import(ctx context.Context, request domain.CodebookImport) (domain.CodebookImportResult, error)
	// Delete 根据主键 ID 删除代码资源。
	Delete(ctx context.Context, id int64) (domain.CodebookDeleteResult, error)

	// CreateProject 插入一个代码资源项目。
	CreateProject(ctx context.Context, req domain.CodebookProject) (int64, error)
	// GetProjectByID 根据主键 ID 查询代码资源项目。
	GetProjectByID(ctx context.Context, id int64) (domain.CodebookProject, error)
	// ListProjects 按状态分页查询代码资源项目。
	ListProjects(ctx context.Context, status domain.CodebookProjectStatus,
		offset, limit int64) ([]domain.CodebookProject, error)
	// TotalProjects 按状态统计代码资源项目总数。
	TotalProjects(ctx context.Context, status domain.CodebookProjectStatus) (int64, error)
	// GetProjectMaxSortNo 查询当前租户项目最大的排序号。
	GetProjectMaxSortNo(ctx context.Context) (int64, error)
	// UpdateProject 更新代码资源项目。
	UpdateProject(ctx context.Context, req domain.CodebookProject) (int64, error)
	// ArtifactNamespaceExists 判断当前租户是否存在同名制品导入命名空间。
	ArtifactNamespaceExists(ctx context.Context, namespace string, excludeID int64) (bool, error)
	// ArchiveProject 归档代码资源项目。
	ArchiveProject(ctx context.Context, id int64) (int64, error)
	// RestoreProject 恢复已归档的代码资源项目。
	RestoreProject(ctx context.Context, id int64) (int64, error)
}

type codebookRepository struct {
	dao        dao.CodebookDAO
	projectDao dao.CodebookProjectDAO
}

// NewCodebookRepository 创建基于 CodebookDAO 和 CodebookProjectDAO 的代码资源仓储。
func NewCodebookRepository(codebookDAO dao.CodebookDAO, projectDAO dao.CodebookProjectDAO) ICodebookRepository {
	return &codebookRepository{
		dao:        codebookDAO,
		projectDao: projectDAO,
	}
}

// Create 保存代码资源，未传入 Secret 时自动生成。
func (repo *codebookRepository) Create(ctx context.Context, req domain.Codebook) (int64, error) {
	req.FillDefaults()
	if err := repo.preparePath(ctx, &req); err != nil {
		return 0, err
	}
	return repo.dao.Create(ctx, repo.toEntity(req), req.Code)
}

// GetByID 根据主键 ID 加载代码资源。
func (repo *codebookRepository) GetByID(ctx context.Context, id int64) (domain.Codebook, error) {
	c, err := repo.dao.GetByID(ctx, id)
	if err != nil {
		return domain.Codebook{}, err
	}
	return repo.fillCode(ctx, repo.toDomain(c))
}

// GetNodeByID 根据主键 ID 加载代码节点元信息，不加载源码内容。
func (repo *codebookRepository) GetNodeByID(ctx context.Context, id int64) (domain.Codebook, error) {
	c, err := repo.dao.GetByID(ctx, id)
	if err != nil {
		return domain.Codebook{}, err
	}
	return repo.toDomain(c), nil
}

// List 分页查询代码资源。
func (repo *codebookRepository) List(ctx context.Context, offset, limit int64) ([]domain.Codebook, error) {
	cs, err := repo.dao.List(ctx, offset, limit)
	if err != nil {
		return nil, err
	}
	return repo.fillCurrentVersionNo(ctx, lo.Map(cs, func(src dao.Codebook, _ int) domain.Codebook {
		return repo.toDomain(src)
	}))
}

// ListVersions 查询指定代码节点下的所有版本。
func (repo *codebookRepository) ListVersions(ctx context.Context, nodeID int64) ([]domain.CodebookVersion, error) {
	versions, err := repo.dao.ListVersions(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	return lo.Map(versions, func(src dao.CodebookVersion, _ int) domain.CodebookVersion {
		return repo.toVersionDomain(src)
	}), nil
}

// GetVersionByID 根据主键 ID 加载代码版本。
func (repo *codebookRepository) GetVersionByID(ctx context.Context, id int64) (domain.CodebookVersion, error) {
	version, err := repo.dao.GetVersionByID(ctx, id)
	if err != nil {
		return domain.CodebookVersion{}, err
	}
	return repo.toVersionDomain(version), nil
}

// ListChildren 查询指定项目和父节点下的子节点。
func (repo *codebookRepository) ListChildren(ctx context.Context, projectID, parentID int64) ([]domain.Codebook, error) {
	if parentID > 0 {
		parent, err := repo.dao.GetByID(ctx, parentID)
		if err != nil {
			return nil, err
		}
		projectID = parent.ProjectID
	}
	cs, err := repo.dao.ListChildren(ctx, projectID, parentID)
	if err != nil {
		return nil, err
	}
	return repo.fillCurrentVersionNo(ctx, lo.Map(cs, func(src dao.Codebook, _ int) domain.Codebook {
		return repo.toDomain(src)
	}))
}

// ListChildrenByScope 查询指定租户项目或系统作用域下的子节点。
func (repo *codebookRepository) ListChildrenByScope(ctx context.Context, projectID, parentID int64, scope domain.CodebookScope) ([]domain.Codebook, error) {
	if parentID > 0 {
		parent, err := repo.dao.GetByID(ctx, parentID)
		if err != nil {
			return nil, err
		}
		projectID = parent.ProjectID
		scope = domain.CodebookScope(parent.Scope)
	}
	cs, err := repo.dao.ListChildrenBySpace(ctx, projectID, parentID, scope.String())
	if err != nil {
		return nil, err
	}
	return repo.fillCurrentVersionNo(ctx, lo.Map(cs, func(src dao.Codebook, _ int) domain.Codebook {
		return repo.toDomain(src)
	}))
}

// Tree 查询指定项目下的全部源码节点。
func (repo *codebookRepository) Tree(ctx context.Context, projectID int64) ([]domain.Codebook, error) {
	cs, err := repo.dao.Tree(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return repo.fillCurrentVersionNo(ctx, lo.Map(cs, func(src dao.Codebook, _ int) domain.Codebook {
		return repo.toDomain(src)
	}))
}

// Total 统计代码资源总数。
func (repo *codebookRepository) Total(ctx context.Context) (int64, error) {
	return repo.dao.Count(ctx)
}

// GetMaxSortNo 查询指定租户项目、作用域和父节点下最大的排序号。
func (repo *codebookRepository) GetMaxSortNo(ctx context.Context, projectID, parentID int64, scope domain.CodebookScope) (int64, error) {
	if parentID > 0 {
		parent, err := repo.dao.GetByID(ctx, parentID)
		if err != nil {
			return 0, err
		}
		projectID = parent.ProjectID
		scope = domain.CodebookScope(parent.Scope)
	}
	return repo.dao.GetMaxSortNo(ctx, projectID, parentID, scope.String())
}

// Update 更新代码资源可变字段。
func (repo *codebookRepository) Update(ctx context.Context, req domain.Codebook) (int64, error) {
	old, err := repo.dao.GetByID(ctx, req.ID)
	if err != nil {
		return 0, err
	}
	entity := repo.toEntity(req)
	entity.ProjectID = old.ProjectID
	entity.ParentID = old.ParentID
	entity.PathIDs = old.PathIDs
	entity.Depth = old.Depth
	entity.Kind = old.Kind
	entity.Secret = old.Secret
	entity.CurrentVersionID = old.CurrentVersionID
	return repo.dao.Update(ctx, entity, req.Code)
}

// CreateVersion 创建代码版本。
func (repo *codebookRepository) CreateVersion(ctx context.Context,
	req domain.CodebookVersionCreate) (int64, error) {
	return repo.dao.CreateVersion(ctx, repo.toVersionEntity(req))
}

// UseVersion 设置代码节点当前使用版本。
func (repo *codebookRepository) UseVersion(ctx context.Context, nodeID, versionID int64) (int64, error) {
	return repo.dao.UseVersion(ctx, nodeID, versionID)
}

// UpdateSort 更新单个代码资源排序。
func (repo *codebookRepository) UpdateSort(ctx context.Context, item domain.CodebookSortItem) error {
	return repo.dao.UpdateSort(ctx, repo.toSortEntity(item))
}

// BatchUpdateSort 批量更新代码资源排序。
func (repo *codebookRepository) BatchUpdateSort(ctx context.Context, items []domain.CodebookSortItem) error {
	return repo.dao.BatchUpdateSort(ctx, lo.Map(items, func(src domain.CodebookSortItem, _ int) dao.CodebookSortItem {
		return repo.toSortEntity(src)
	}))
}

// Import 在一个事务中导入项目文件树。
func (repo *codebookRepository) Import(ctx context.Context,
	request domain.CodebookImport) (domain.CodebookImportResult, error) {
	result, err := repo.dao.Import(ctx, dao.CodebookImport{
		ProjectID: request.ProjectID, ParentID: request.ParentID,
		Files: lo.Map(request.Files, func(file domain.CodebookImportFile, _ int) dao.CodebookImportFile {
			return dao.CodebookImportFile{
				Path: file.Path, Code: file.Code, StorageType: file.StorageType.String(),
				ObjectKey: file.ObjectKey, Size: file.Size, ContentType: file.ContentType, Hash: file.Hash,
			}
		}),
	})
	return domain.CodebookImportResult{
		FileCount: result.FileCount, DirectoryCount: result.DirectoryCount,
	}, err
}

// Delete 根据主键 ID 删除代码资源。
func (repo *codebookRepository) Delete(ctx context.Context, id int64) (domain.CodebookDeleteResult, error) {
	result, err := repo.dao.Delete(ctx, id)
	return domain.CodebookDeleteResult{
		NodeCount: result.NodeCount, ObjectKeys: result.ObjectKeys,
	}, err
}

func (repo *codebookRepository) fillCode(ctx context.Context, c domain.Codebook) (domain.Codebook, error) {
	if !c.IsFile() {
		return c, nil
	}
	if c.CurrentVersionID == 0 {
		return c, nil
	}
	version, err := repo.dao.GetCurrentVersion(ctx, c.ID)
	if err != nil {
		return domain.Codebook{}, err
	}
	version = normalizeVersion(version)
	c.Code = version.Code
	c.StorageType = domain.CodebookContentStorage(version.StorageType)
	c.ObjectKey = version.ObjectKey
	c.Size = version.Size
	c.ContentType = version.ContentType
	c.CurrentVersionNo = version.VersionNo
	return c, nil
}

func (repo *codebookRepository) fillCurrentVersionNo(ctx context.Context, cs []domain.Codebook) ([]domain.Codebook, error) {
	versionIDs := lo.Uniq(lo.FilterMap(cs, func(c domain.Codebook, _ int) (int64, bool) {
		return c.CurrentVersionID, c.IsFile() && c.CurrentVersionID > 0
	}))
	if len(versionIDs) == 0 {
		return cs, nil
	}
	versions, err := repo.dao.ListVersionsByIDs(ctx, versionIDs)
	if err != nil {
		return nil, err
	}
	versionMap := lo.SliceToMap(versions, func(v dao.CodebookVersion) (int64, dao.CodebookVersion) {
		return v.ID, v
	})
	for i := range cs {
		version, exists := versionMap[cs[i].CurrentVersionID]
		if !exists {
			continue
		}
		version = normalizeVersion(version)
		cs[i].CurrentVersionNo = version.VersionNo
		cs[i].StorageType = domain.CodebookContentStorage(version.StorageType)
		cs[i].Size = version.Size
		cs[i].ContentType = version.ContentType
	}
	return cs, nil
}

func (repo *codebookRepository) preparePath(ctx context.Context, req *domain.Codebook) error {
	if req.ParentID == 0 {
		req.ApplyRoot()
		return nil
	}
	parent, err := repo.dao.GetByID(ctx, req.ParentID)
	if err != nil {
		return err
	}
	return req.ApplyParent(repo.toDomain(parent))
}

func (repo *codebookRepository) toEntity(req domain.Codebook) dao.Codebook {
	req.FillDefaults()
	secret := req.Secret
	if secret == "" && req.IsFile() {
		secret = uuid.NewString()
	}
	return dao.Codebook{
		ID:               req.ID,
		Scope:            req.Scope.String(),
		ProjectID:        req.ProjectID,
		ParentID:         req.ParentID,
		PathIDs:          req.PathIDs,
		Depth:            req.Depth,
		Name:             req.Name,
		Owner:            req.Owner,
		Kind:             req.Kind.String(),
		SortNo:           req.SortNo,
		Secret:           secret,
		CurrentVersionID: req.CurrentVersionID,
		CTime:            req.CTime,
		UTime:            req.UTime,
	}
}

func (repo *codebookRepository) toDomain(req dao.Codebook) domain.Codebook {
	return domain.Codebook{
		ID:               req.ID,
		TenantID:         req.TenantID,
		Scope:            domain.CodebookScope(req.Scope),
		ProjectID:        req.ProjectID,
		ParentID:         req.ParentID,
		PathIDs:          req.PathIDs,
		Depth:            req.Depth,
		Name:             req.Name,
		Owner:            req.Owner,
		Kind:             domain.CodebookKind(req.Kind),
		SortNo:           req.SortNo,
		Secret:           req.Secret,
		CurrentVersionID: req.CurrentVersionID,
		CTime:            req.CTime,
		UTime:            req.UTime,
	}
}

func (repo *codebookRepository) toVersionEntity(
	req domain.CodebookVersionCreate) dao.CodebookVersionCreate {
	return dao.CodebookVersionCreate{
		Version: dao.CodebookVersion{
			NodeID: req.NodeID, Code: req.Code, Message: req.Message,
		},
		ExpectedCurrentVersionID: req.ExpectedCurrentVersionID,
		SourceKey:                optionalString(req.SourceKey),
	}
}

func (repo *codebookRepository) toVersionDomain(req dao.CodebookVersion) domain.CodebookVersion {
	req = normalizeVersion(req)
	return domain.CodebookVersion{
		ID:           req.ID,
		NodeID:       req.NodeID,
		TenantID:     req.TenantID,
		Scope:        domain.CodebookScope(req.Scope),
		VersionNo:    req.VersionNo,
		Code:         req.Code,
		StorageType:  domain.CodebookContentStorage(req.StorageType),
		ObjectKey:    req.ObjectKey,
		Size:         req.Size,
		ContentType:  req.ContentType,
		Hash:         req.Hash,
		Message:      req.Message,
		AuthorUserID: req.AuthorUserID,
		CTime:        req.CTime,
	}
}

func normalizeVersion(version dao.CodebookVersion) dao.CodebookVersion {
	if version.StorageType == "" {
		version.StorageType = domain.CodebookContentInline.String()
	}
	if version.StorageType == domain.CodebookContentInline.String() {
		version.Size = int64(len(version.Code))
		if version.ContentType == "" {
			version.ContentType = "text/plain; charset=utf-8"
		}
	}
	return version
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func (repo *codebookRepository) toSortEntity(req domain.CodebookSortItem) dao.CodebookSortItem {
	return dao.CodebookSortItem{
		ID:        req.ID,
		ProjectID: req.ProjectID,
		ParentID:  req.ParentID,
		PathIDs:   req.PathIDs,
		Depth:     req.Depth,
		SortNo:    req.SortNo,
	}
}

// Redirect repository implementations for Project methods to repo itself.
func (repo *codebookRepository) CreateProject(ctx context.Context, req domain.CodebookProject) (int64, error) {
	return repo.projectDao.Create(ctx, repo.toProjectEntity(req))
}

func (repo *codebookRepository) GetProjectByID(ctx context.Context, id int64) (domain.CodebookProject, error) {
	p, err := repo.projectDao.GetByID(ctx, id)
	if err != nil {
		return domain.CodebookProject{}, err
	}
	return repo.toProjectDomain(p), nil
}

func (repo *codebookRepository) ListProjects(ctx context.Context, status domain.CodebookProjectStatus,
	offset, limit int64) ([]domain.CodebookProject, error) {
	ps, err := repo.projectDao.List(ctx, status.String(), offset, limit)
	if err != nil {
		return nil, err
	}
	return lo.Map(ps, func(src dao.CodebookProject, _ int) domain.CodebookProject {
		return repo.toProjectDomain(src)
	}), nil
}

func (repo *codebookRepository) TotalProjects(ctx context.Context,
	status domain.CodebookProjectStatus) (int64, error) {
	return repo.projectDao.Count(ctx, status.String())
}

func (repo *codebookRepository) GetProjectMaxSortNo(ctx context.Context) (int64, error) {
	return repo.projectDao.GetMaxSortNo(ctx)
}

func (repo *codebookRepository) UpdateProject(ctx context.Context, req domain.CodebookProject) (int64, error) {
	return repo.projectDao.Update(ctx, repo.toProjectEntity(req))
}

func (repo *codebookRepository) ArtifactNamespaceExists(ctx context.Context, namespace string, excludeID int64) (bool, error) {
	return repo.projectDao.ArtifactNamespaceExists(ctx, namespace, excludeID)
}

func (repo *codebookRepository) ArchiveProject(ctx context.Context, id int64) (int64, error) {
	return repo.projectDao.Archive(ctx, id)
}

func (repo *codebookRepository) RestoreProject(ctx context.Context, id int64) (int64, error) {
	return repo.projectDao.Restore(ctx, id)
}

func (repo *codebookRepository) toProjectEntity(p domain.CodebookProject) dao.CodebookProject {
	var namespace *string
	if p.ArtifactNamespace != "" {
		namespace = &p.ArtifactNamespace
	}
	return dao.CodebookProject{
		ID:                p.ID,
		Scope:             p.Scope.String(),
		Name:              p.Name,
		Desc:              p.Desc,
		SortNo:            p.SortNo,
		Status:            p.Status.String(),
		ArtifactEnabled:   p.ArtifactEnabled,
		ArtifactNamespace: namespace,
		SourceRevision:    p.SourceRevision,
		CTime:             p.CTime,
		UTime:             p.UTime,
	}
}

func (repo *codebookRepository) toProjectDomain(p dao.CodebookProject) domain.CodebookProject {
	var namespace string
	if p.ArtifactNamespace != nil {
		namespace = *p.ArtifactNamespace
	}
	return domain.CodebookProject{
		ID:                p.ID,
		TenantID:          p.TenantID,
		Scope:             domain.CodebookScope(p.Scope),
		Name:              p.Name,
		Desc:              p.Desc,
		SortNo:            p.SortNo,
		Status:            domain.CodebookProjectStatus(p.Status),
		ArtifactEnabled:   p.ArtifactEnabled,
		ArtifactNamespace: namespace,
		SourceRevision:    p.SourceRevision,
		CTime:             p.CTime,
		UTime:             p.UTime,
	}
}
