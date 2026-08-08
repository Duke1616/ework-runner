package domain

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Duke1616/etask/internal/errs"
)

var artifactNamespacePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

type CodebookScope string

const (
	CodebookScopeSystem CodebookScope = "SYSTEM"
	CodebookScopeTenant CodebookScope = "TENANT"
)

func (s CodebookScope) String() string {
	return string(s)
}

func (s CodebookScope) Valid() bool {
	return s == CodebookScopeSystem || s == CodebookScopeTenant
}

// ValidateWriteAccess 校验租户是否可以写入当前代码资源作用域。
func (s CodebookScope) ValidateWriteAccess(tenantID, systemTenantID int64) error {
	if !s.Valid() {
		return fmt.Errorf("%w: 不支持的作用域: %s", errs.ErrInvalidParameter, s)
	}
	if tenantID <= 0 {
		return fmt.Errorf("%w: 缺少租户上下文，不能写入该作用域", errs.ErrInvalidParameter)
	}
	if s == CodebookScopeSystem && tenantID != systemTenantID {
		return fmt.Errorf("%w: 只有系统租户可以写入 SYSTEM 作用域", errs.ErrInvalidParameter)
	}
	return nil
}

type CodebookKind string

const (
	CodebookKindDirectory CodebookKind = "DIRECTORY"
	CodebookKindFile      CodebookKind = "FILE"
)

func (k CodebookKind) String() string {
	return string(k)
}

func (k CodebookKind) Valid() bool {
	return k == CodebookKindDirectory || k == CodebookKindFile
}

// CodebookContentStorage 表示代码版本内容的持久化位置。
type CodebookContentStorage string

const (
	CodebookContentInline CodebookContentStorage = "INLINE"
	CodebookContentBlob   CodebookContentStorage = "BLOB"
)

func (s CodebookContentStorage) String() string { return string(s) }

func (s CodebookContentStorage) Valid() bool {
	return s == CodebookContentInline || s == CodebookContentBlob
}

type CodebookProjectStatus string

const (
	CodebookProjectStatusNormal   CodebookProjectStatus = "NORMAL"
	CodebookProjectStatusArchived CodebookProjectStatus = "ARCHIVED"
)

func (s CodebookProjectStatus) String() string {
	return string(s)
}

func (s CodebookProjectStatus) Valid() bool {
	return s == CodebookProjectStatusNormal || s == CodebookProjectStatusArchived
}

// CodebookProject 表示租户级代码资源项目，系统组件库不归属具体项目。
type CodebookProject struct {
	ID                int64
	TenantID          int64
	Scope             CodebookScope
	Name              string
	Desc              string
	SortNo            int64
	Status            CodebookProjectStatus
	ArtifactEnabled   bool
	ArtifactNamespace string
	SourceRevision    int64
	CTime             int64
	UTime             int64
}

// Codebook 表示 etask 负责维护的代码节点，目录和文件统一建模。
type Codebook struct {
	ID               int64
	TenantID         int64
	Scope            CodebookScope
	ProjectID        int64
	ParentID         int64
	PathIDs          string
	Depth            int
	Name             string
	Owner            string
	Kind             CodebookKind
	SortNo           int64
	Code             string
	StorageType      CodebookContentStorage
	ObjectKey        string
	Size             int64
	ContentType      string
	Secret           string
	CurrentVersionID int64
	CurrentVersionNo int64
	CTime            int64
	UTime            int64
}

type CodebookVersion struct {
	ID           int64
	NodeID       int64
	TenantID     int64
	Scope        CodebookScope
	VersionNo    int64
	Code         string
	StorageType  CodebookContentStorage
	ObjectKey    string
	Size         int64
	ContentType  string
	Hash         string
	Message      string
	AuthorUserID int64
	CTime        int64
}

// CodebookVersionCreate 描述一次代码版本写入。
// ExpectedCurrentVersionID 和 SourceKey 仅用于写入时的并发控制与幂等，
// 不属于已保存版本本身。
type CodebookVersionCreate struct {
	NodeID                   int64
	Code                     string
	Message                  string
	ExpectedCurrentVersionID int64
	SourceKey                string
}

type CodebookSearch struct {
	ProjectID int64
	ParentID  int64
	Scope     CodebookScope
	Keyword   string
	Kind      CodebookKind
}

type CodebookSortItem struct {
	ID        int64
	ProjectID int64
	ParentID  int64
	PathIDs   string
	Depth     int
	SortNo    int64
}

// CodebookImportFile 描述已经完成内容持久化的待导入项目文件。
type CodebookImportFile struct {
	Path        string
	Code        string
	StorageType CodebookContentStorage
	ObjectKey   string
	Size        int64
	ContentType string
	Hash        string
}

// CodebookImport 描述一次原子项目文件树导入。
type CodebookImport struct {
	ProjectID int64
	ParentID  int64
	Files     []CodebookImportFile
}

// CodebookImportResult 汇总一次项目文件导入的写入结果。
type CodebookImportResult struct {
	FileCount      int
	DirectoryCount int
}

// CodebookDeleteResult 汇总节点删除结果及其待清理的外部内容对象。
type CodebookDeleteResult struct {
	NodeCount  int64
	ObjectKeys []string
}

const CodebookRootPathIDs = "/"

func (p *CodebookProject) FillDefaults() {
	if p.Scope == "" {
		p.Scope = CodebookScopeTenant
	}
	if p.Status == "" {
		p.Status = CodebookProjectStatusNormal
	}
	p.Name = strings.TrimSpace(p.Name)
	p.Desc = strings.TrimSpace(p.Desc)
	p.ArtifactNamespace = strings.TrimSpace(p.ArtifactNamespace)
}

func (p *CodebookProject) Validate() error {
	p.FillDefaults()
	if !p.Scope.Valid() {
		return fmt.Errorf("%w: 不支持的作用域: %s", errs.ErrInvalidParameter, p.Scope)
	}
	if p.Scope == CodebookScopeSystem {
		return fmt.Errorf("%w: 系统组件库不需要项目", errs.ErrInvalidParameter)
	}
	if p.Name == "" {
		return fmt.Errorf("%w: 项目名称不能为空", errs.ErrInvalidParameter)
	}
	if p.ArtifactEnabled {
		if p.ArtifactNamespace == "" {
			return fmt.Errorf("%w: 制品库导入命名空间不能为空", errs.ErrInvalidParameter)
		}
	}
	if p.ArtifactNamespace != "" && !artifactNamespacePattern.MatchString(p.ArtifactNamespace) {
		return fmt.Errorf("%w: 制品库导入命名空间只能包含小写英文字母、数字和下划线，且必须以字母开头",
			errs.ErrInvalidParameter)
	}
	if p.ArtifactNamespace == "etask" {
		return fmt.Errorf("%w: etask 是 SYSTEM 组件库保留命名空间", errs.ErrInvalidParameter)
	}
	return nil
}

func (p *CodebookProject) MergeForUpdate(old CodebookProject) {
	p.TenantID = old.TenantID
	p.Scope = old.Scope
	p.Status = old.Status
	if p.SortNo == 0 {
		p.SortNo = old.SortNo
	}
}

func (c *Codebook) FillDefaults() {
	if c.Scope == "" {
		c.Scope = CodebookScopeTenant
	}
	if c.Kind == "" {
		c.Kind = CodebookKindFile
	}
	c.Name = strings.TrimSpace(c.Name)
	c.Owner = strings.TrimSpace(c.Owner)
}

func (c *Codebook) Validate() error {
	c.FillDefaults()
	if !c.Scope.Valid() {
		return fmt.Errorf("%w: 不支持的作用域: %s", errs.ErrInvalidParameter, c.Scope)
	}
	if c.Scope == CodebookScopeSystem && c.ProjectID != 0 {
		return fmt.Errorf("%w: 系统组件库的 project_id 必须为 0", errs.ErrInvalidParameter)
	}
	if c.Scope == CodebookScopeTenant && c.ProjectID <= 0 {
		return fmt.Errorf("%w: 租户代码资源必须指定项目", errs.ErrInvalidParameter)
	}
	if !c.Kind.Valid() {
		return fmt.Errorf("%w: 不支持的节点类型: %s", errs.ErrInvalidParameter, c.Kind)
	}
	if c.Name == "" {
		return fmt.Errorf("%w: 名称不能为空", errs.ErrInvalidParameter)
	}
	if c.Kind == CodebookKindDirectory && c.Code != "" {
		return fmt.Errorf("%w: 目录不能包含代码内容", errs.ErrInvalidParameter)
	}
	if c.Kind == CodebookKindDirectory && c.Secret != "" {
		return fmt.Errorf("%w: 目录不能配置访问密钥", errs.ErrInvalidParameter)
	}
	return nil
}

func (c *Codebook) MergeForUpdate(old Codebook) {
	if c.TenantID == 0 {
		c.TenantID = old.TenantID
	}
	if c.Scope == "" {
		c.Scope = old.Scope
	}
	if c.Kind == "" {
		c.Kind = old.Kind
	}
	if c.Name == "" {
		c.Name = old.Name
	}
	if c.Owner == "" {
		c.Owner = old.Owner
	}
	if c.Code == "" {
		c.Code = old.Code
	}
	if c.Secret == "" {
		c.Secret = old.Secret
	}
	if c.SortNo == 0 {
		c.SortNo = old.SortNo
	}
	if c.ProjectID == 0 {
		c.ProjectID = old.ProjectID
	}
}

func (c *Codebook) IsFile() bool {
	return c != nil && c.Kind == CodebookKindFile
}

func (c *Codebook) IsDirectory() bool {
	return c != nil && c.Kind == CodebookKindDirectory
}

func (c *Codebook) ApplyRoot() {
	if c == nil {
		return
	}
	c.ParentID = 0
	c.PathIDs = CodebookRootPathIDs
	c.Depth = 0
}

func (c *Codebook) ApplyParent(parent Codebook) error {
	if c == nil {
		return nil
	}
	if !parent.IsDirectory() {
		return fmt.Errorf("%w: 父级节点不是目录", errs.ErrInvalidParameter)
	}
	if c.Scope == "" {
		c.Scope = parent.Scope
	}
	if c.Scope != parent.Scope {
		return fmt.Errorf("%w: 子节点作用域必须和父级目录一致", errs.ErrInvalidParameter)
	}
	if c.ProjectID == 0 {
		c.ProjectID = parent.ProjectID
	}
	if c.ProjectID != parent.ProjectID {
		return fmt.Errorf("%w: 子节点项目必须和父级目录一致", errs.ErrInvalidParameter)
	}
	if c.TenantID == 0 {
		c.TenantID = parent.TenantID
	}
	if c.TenantID != parent.TenantID {
		return fmt.Errorf("%w: 子节点租户必须和父级目录一致", errs.ErrInvalidParameter)
	}
	c.ParentID = parent.ID
	c.PathIDs = parent.ChildPathIDs()
	c.Depth = parent.Depth + 1
	return nil
}

func (c *Codebook) ChildPathIDs() string {
	if c == nil {
		return CodebookRootPathIDs
	}
	return fmt.Sprintf("%s%d/", c.PathIDs, c.ID)
}

func (c *Codebook) ResolveMoveTarget(parent *Codebook) (CodebookSortItem, error) {
	if c == nil {
		return CodebookSortItem{}, fmt.Errorf("%w: 代码资源不能为空", errs.ErrInvalidParameter)
	}
	item := c.ToSortItem()
	if parent == nil {
		item.ParentID = 0
		item.PathIDs = CodebookRootPathIDs
		item.Depth = 0
		return item, nil
	}
	if parent.ID == c.ID {
		return CodebookSortItem{}, fmt.Errorf("%w: 不能移动到自身下面", errs.ErrInvalidParameter)
	}
	if !parent.IsDirectory() {
		return CodebookSortItem{}, fmt.Errorf("%w: 目标父级不是目录", errs.ErrInvalidParameter)
	}
	if parent.Scope != c.Scope {
		return CodebookSortItem{}, fmt.Errorf("%w: 目标父级作用域必须和当前节点一致", errs.ErrInvalidParameter)
	}
	if parent.ProjectID != c.ProjectID {
		return CodebookSortItem{}, fmt.Errorf("%w: 目标父级项目必须和当前节点一致", errs.ErrInvalidParameter)
	}
	if parent.TenantID != c.TenantID {
		return CodebookSortItem{}, fmt.Errorf("%w: 目标父级租户必须和当前节点一致", errs.ErrInvalidParameter)
	}
	if strings.HasPrefix(parent.PathIDs, c.ChildPathIDs()) {
		return CodebookSortItem{}, fmt.Errorf("%w: 不能移动到自己的子节点下面", errs.ErrInvalidParameter)
	}
	item.ParentID = parent.ID
	item.PathIDs = parent.ChildPathIDs()
	item.Depth = parent.Depth + 1
	return item, nil
}

func (c *Codebook) ToSortItem() CodebookSortItem {
	if c == nil {
		return CodebookSortItem{}
	}
	return CodebookSortItem{
		ID:        c.ID,
		ParentID:  c.ParentID,
		ProjectID: c.ProjectID,
		PathIDs:   c.PathIDs,
		Depth:     c.Depth,
		SortNo:    c.SortNo,
	}
}

func (v *CodebookVersionCreate) PrepareForNode(node Codebook) error {
	if v == nil {
		return nil
	}
	if v.NodeID <= 0 {
		return fmt.Errorf("%w: 代码资源 ID 非法: %d", errs.ErrInvalidParameter, v.NodeID)
	}
	if !node.IsFile() {
		return fmt.Errorf("%w: 目录不能创建版本", errs.ErrInvalidParameter)
	}
	if v.ExpectedCurrentVersionID < 0 {
		return fmt.Errorf("%w: 预期当前版本 ID 非法", errs.ErrInvalidParameter)
	}
	v.Message = strings.TrimSpace(v.Message)
	v.SourceKey = strings.TrimSpace(v.SourceKey)
	if len(v.SourceKey) > 128 {
		return fmt.Errorf("%w: 版本幂等来源过长", errs.ErrInvalidParameter)
	}
	return nil
}

func (c CodebookSortItem) GetID() int64 {
	return c.ID
}

func (c CodebookSortItem) GetSortKey() int64 {
	return c.SortNo
}
