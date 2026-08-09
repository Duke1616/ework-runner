package dao

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	"github.com/Duke1616/etask/pkg/sorter"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const codebookImportBatchSize = 100

type CodebookImportFile struct {
	Path        string
	Code        string
	StorageType string
	ObjectKey   string
	Size        int64
	ContentType string
	Hash        string
	Message     string
	SourceKey   string
}

type CodebookImport struct {
	ProjectID int64
	ParentID  int64
	Files     []CodebookImportFile
}

type CodebookImportResult struct {
	FileCount      int
	DirectoryCount int
}

// Import 在一个事务中导入项目文件树，并且只递增一次项目源码修订号。
func (g *GORMCodebookDAO) Import(ctx context.Context,
	request CodebookImport) (CodebookImportResult, error) {
	var result CodebookImportResult
	err := g.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		nodes, err := loadCodebookImportNodes(tx, request.ProjectID)
		if err != nil {
			return err
		}
		plan, err := buildCodebookImportPlan(request, nodes, time.Now().UnixMilli())
		if err != nil {
			return err
		}
		if err = plan.persist(tx, codebookAuthorUserID(ctx)); err != nil {
			return err
		}
		result = plan.result()
		return bumpProjectSourceRevision(tx, domain.CodebookScopeTenant.String(), request.ProjectID)
	})
	return result, codebookWriteError(err)
}

func loadCodebookImportNodes(tx *gorm.DB, projectID int64) ([]Codebook, error) {
	var project CodebookProject
	if err := tx.Where("id = ? AND scope = ? AND status = ?", projectID,
		domain.CodebookScopeTenant.String(), domain.CodebookProjectStatusNormal.String()).
		First(&project).Error; err != nil {
		return nil, err
	}
	var nodes []Codebook
	err := tx.Where("project_id = ? AND scope = ?", projectID,
		domain.CodebookScopeTenant.String()).Find(&nodes).Error
	return nodes, err
}

type codebookImportNode struct {
	entity    Codebook
	parent    *codebookImportNode
	children  map[string]*codebookImportNode
	nextSort  int64
	file      *CodebookImportFile
	versionID int64
}

type codebookImportPlan struct {
	directories [][]*codebookImportNode
	files       []*codebookImportNode
	now         int64
}

func buildCodebookImportPlan(request CodebookImport, existing []Codebook,
	now int64) (*codebookImportPlan, error) {
	root, byID := buildCodebookImportTree(existing)
	target := root
	if request.ParentID > 0 {
		var exists bool
		target, exists = byID[request.ParentID]
		if !exists || target.entity.Kind != domain.CodebookKindDirectory.String() {
			return nil, gorm.ErrRecordNotFound
		}
	}

	plan := &codebookImportPlan{now: now}
	files := append([]CodebookImportFile(nil), request.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	for index := range files {
		if err := plan.addFile(request.ProjectID, target, &files[index]); err != nil {
			return nil, err
		}
	}
	return plan, nil
}

func buildCodebookImportTree(existing []Codebook) (*codebookImportNode,
	map[int64]*codebookImportNode) {
	root := &codebookImportNode{
		entity:   Codebook{PathIDs: domain.CodebookRootPathIDs, Depth: -1},
		children: make(map[string]*codebookImportNode),
	}
	byID := make(map[int64]*codebookImportNode, len(existing))
	for index := range existing {
		node := &codebookImportNode{
			entity: existing[index], children: make(map[string]*codebookImportNode),
		}
		byID[node.entity.ID] = node
	}
	for _, node := range byID {
		parent := root
		if node.entity.ParentID > 0 {
			parent = byID[node.entity.ParentID]
			if parent == nil {
				continue
			}
		}
		node.parent = parent
		parent.children[importNameKey(node.entity.Name)] = node
		if node.entity.SortNo > parent.nextSort {
			parent.nextSort = node.entity.SortNo
		}
	}
	return root, byID
}

func (p *codebookImportPlan) addFile(projectID int64, target *codebookImportNode,
	file *CodebookImportFile) error {
	parent := target
	segments := strings.Split(file.Path, "/")
	for index, name := range segments {
		last := index == len(segments)-1
		if child, exists := parent.children[importNameKey(name)]; exists {
			if last {
				return fmt.Errorf("%w: %s", errs.ErrCodebookNameConflict, file.Path)
			}
			if child.entity.Kind != domain.CodebookKindDirectory.String() {
				return fmt.Errorf("%w: \u8def\u5f84 %s \u5df2\u5b58\u5728\u540c\u540d\u6587\u4ef6", errs.ErrCodebookNameConflict,
					strings.Join(segments[:index+1], "/"))
			}
			parent = child
			continue
		}

		node := newCodebookImportNode(projectID, parent, name, p.now)
		parent.children[importNameKey(name)] = node
		if last {
			node.entity.Kind = domain.CodebookKindFile.String()
			node.entity.Secret = uuid.NewString()
			node.file = file
			p.files = append(p.files, node)
			continue
		}
		node.entity.Kind = domain.CodebookKindDirectory.String()
		p.addDirectory(index, node)
		parent = node
	}
	return nil
}

func newCodebookImportNode(projectID int64, parent *codebookImportNode,
	name string, now int64) *codebookImportNode {
	parent.nextSort += sorter.DefaultIndexGap
	return &codebookImportNode{
		entity: Codebook{
			Scope: domain.CodebookScopeTenant.String(), ProjectID: projectID,
			Name: name, SortNo: parent.nextSort, CTime: now, UTime: now,
		},
		parent: parent, children: make(map[string]*codebookImportNode),
	}
}

func (p *codebookImportPlan) addDirectory(level int, node *codebookImportNode) {
	for len(p.directories) <= level {
		p.directories = append(p.directories, nil)
	}
	p.directories[level] = append(p.directories[level], node)
}

func (p *codebookImportPlan) persist(tx *gorm.DB, authorUserID int64) error {
	for _, level := range p.directories {
		if err := createCodebookImportNodes(tx, level); err != nil {
			return err
		}
	}
	if err := createCodebookImportNodes(tx, p.files); err != nil {
		return err
	}
	versions := make([]CodebookVersion, len(p.files))
	for index, node := range p.files {
		file := node.file
		versions[index] = CodebookVersion{
			NodeID: node.entity.ID, Scope: node.entity.Scope, VersionNo: 1,
			Code: file.Code, StorageType: file.StorageType, ObjectKey: file.ObjectKey,
			Size: file.Size, ContentType: file.ContentType, Hash: file.Hash,
			Message: file.Message, AuthorUserID: authorUserID, CTime: p.now,
		}
		if file.SourceKey != "" {
			versions[index].SourceKey = &file.SourceKey
		}
	}
	if len(versions) == 0 {
		return nil
	}
	if err := tx.CreateInBatches(&versions, codebookImportBatchSize).Error; err != nil {
		return err
	}
	for index := range p.files {
		p.files[index].versionID = versions[index].ID
	}
	return updateCodebookImportVersions(tx, p.files, versions, p.now)
}

func createCodebookImportNodes(tx *gorm.DB, nodes []*codebookImportNode) error {
	if len(nodes) == 0 {
		return nil
	}
	entities := make([]Codebook, len(nodes))
	for index, node := range nodes {
		parent := node.parent.entity
		node.entity.ParentID = parent.ID
		node.entity.PathIDs = domain.CodebookRootPathIDs
		node.entity.Depth = 0
		if parent.ID > 0 {
			node.entity.PathIDs = importChildPathIDs(parent)
			node.entity.Depth = parent.Depth + 1
		}
		entities[index] = node.entity
	}
	if err := tx.CreateInBatches(&entities, codebookImportBatchSize).Error; err != nil {
		return err
	}
	for index := range nodes {
		nodes[index].entity = entities[index]
	}
	return nil
}

func updateCodebookImportVersions(tx *gorm.DB, nodes []*codebookImportNode,
	versions []CodebookVersion, now int64) error {
	for start := 0; start < len(nodes); start += codebookImportBatchSize {
		end := min(start+codebookImportBatchSize, len(nodes))
		ids := make([]int64, 0, end-start)
		args := make([]any, 0, (end-start)*2)
		var expression strings.Builder
		expression.WriteString("CASE id")
		for index := start; index < end; index++ {
			expression.WriteString(" WHEN ? THEN ?")
			ids = append(ids, nodes[index].entity.ID)
			args = append(args, nodes[index].entity.ID, versions[index].ID)
		}
		expression.WriteString(" ELSE current_version_id END")
		if err := tx.Model(&Codebook{}).Where("id IN ?", ids).Updates(map[string]any{
			"current_version_id": gorm.Expr(expression.String(), args...),
			"utime":              now,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (p *codebookImportPlan) result() CodebookImportResult {
	count := 0
	for _, level := range p.directories {
		count += len(level)
	}
	return CodebookImportResult{FileCount: len(p.files), DirectoryCount: count}
}

func importNameKey(name string) string {
	return strings.ToLower(name)
}

func importChildPathIDs(node Codebook) string {
	return fmt.Sprintf("%s%d/", node.PathIDs, node.ID)
}
