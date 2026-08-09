package codebook

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
)

// WorkspaceSourceReader 提供当前项目源码树。
type WorkspaceSourceReader interface {
	// Tree 查询指定项目下的全部源码节点。
	Tree(ctx context.Context, projectID int64) ([]domain.Codebook, error)
	// SystemTree 查询指定 SYSTEM 根项目及其源码节点。
	SystemTree(ctx context.Context, rootID int64) ([]domain.Codebook, error)
}

// WorkspaceArtifactReader 提供工作区展示所需的不可变制品内容。
type WorkspaceArtifactReader interface {
	// ActiveContents 查询当前执行会使用的激活制品清单，并排除来源项目自身。
	ActiveContents(ctx context.Context, sourceProjectID int64) ([]domain.ArtifactContent, error)
	// ReadFile 从指定制品发布中读取一个文件的不可变内容。
	ReadFile(ctx context.Context, releaseID int64, digest, filePath string) (string, error)
}

// WorkspaceService 定义代码工作区的只读查询能力。
type WorkspaceService interface {
	// Tree 查询当前项目源码和实际激活制品组成的工作区树。
	Tree(ctx context.Context, projectID int64) ([]domain.WorkspaceNode, error)
	// SystemTree 查询 SYSTEM 项目工作区树。
	SystemTree(ctx context.Context, rootID int64) ([]domain.WorkspaceNode, error)
	// ReadArtifactFile 读取工作区中不可变制品文件的内容。
	ReadArtifactFile(ctx context.Context, projectID, releaseID int64, digest, filePath string) (string, error)
}

type workspaceService struct {
	repo      WorkspaceSourceReader
	artifacts WorkspaceArtifactReader
}

// NewWorkspaceService 创建代码工作区只读查询服务。
func NewWorkspaceService(repo WorkspaceSourceReader, artifacts WorkspaceArtifactReader) WorkspaceService {
	return &workspaceService{repo: repo, artifacts: artifacts}
}

// Tree 查询当前项目源码和实际激活制品组成的工作区树。
func (s *workspaceService) Tree(ctx context.Context, projectID int64) ([]domain.WorkspaceNode, error) {
	if projectID <= 0 {
		return nil, fmt.Errorf("%w: 项目 ID 非法: %d", errs.ErrInvalidParameter, projectID)
	}
	source, err := s.repo.Tree(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("查询项目源码树失败: %w", err)
	}
	contents, err := s.artifacts.ActiveContents(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("查询工作区制品失败: %w", err)
	}

	// 工作区固定为 project、system、dependencies 三层，前端无需自行拼接运行路径。
	project := workspaceRoot("layer:project", "project", domain.WorkspaceLayerProject, false)
	project.ProjectID = projectID
	project.RuntimePath = ""
	project.Children, err = buildProjectNodes(source, projectID)
	if err != nil {
		return nil, err
	}
	system := workspaceRoot("layer:system", "system", domain.WorkspaceLayerSystem, true)
	dependencies := workspaceRoot("layer:dependencies", "dependencies", domain.WorkspaceLayerDependency, true)
	// SYSTEM 填充固定层；每个租户制品库按英文 namespace 形成独立依赖目录。
	for _, content := range contents {
		switch content.Release.Scope {
		case domain.CodebookScopeSystem:
			system.ReleaseID = content.Release.ID
			system.Digest = content.Release.Digest
			system.CTime = content.Release.CTime
			system.UTime = content.Release.CTime
			system.Children = buildArtifactNodes(content, "system", domain.WorkspaceLayerSystem)
		case domain.CodebookScopeTenant:
			namespace := content.Release.Namespace
			rootPath := path.Join("dependencies", namespace)
			library := workspaceRoot("dependency:"+namespace, namespace, domain.WorkspaceLayerDependency, true)
			library.ProjectID = content.Release.ProjectID
			library.ReleaseID = content.Release.ID
			library.Digest = content.Release.Digest
			library.Namespace = namespace
			library.RuntimePath = rootPath
			library.CTime = content.Release.CTime
			library.UTime = content.Release.CTime
			library.Children = buildArtifactNodes(content, rootPath, domain.WorkspaceLayerDependency)
			dependencies.Children = append(dependencies.Children, library)
		}
	}
	sort.Slice(dependencies.Children, func(i, j int) bool {
		return dependencies.Children[i].Name < dependencies.Children[j].Name
	})
	return []domain.WorkspaceNode{project, system, dependencies}, nil
}

func (s *workspaceService) SystemTree(ctx context.Context, rootID int64) ([]domain.WorkspaceNode, error) {
	if rootID <= 0 {
		return nil, fmt.Errorf("%w: SYSTEM 项目 ID 非法: %d", errs.ErrInvalidParameter, rootID)
	}
	source, err := s.repo.SystemTree(ctx, rootID)
	if err != nil {
		return nil, fmt.Errorf("查询 SYSTEM 项目源码树失败: %w", err)
	}
	var root *domain.Codebook
	filtered := make([]domain.Codebook, 0, len(source))
	for i := range source {
		if source[i].ID == rootID {
			root = &source[i]
			continue
		}
		node := source[i]
		if node.ParentID == rootID {
			node.ParentID = 0
		}
		filtered = append(filtered, node)
	}
	if root == nil || !root.IsDirectory() || root.Scope != domain.CodebookScopeSystem {
		return nil, fmt.Errorf("%w: SYSTEM 项目根节点非法: %d", errs.ErrInvalidParameter, rootID)
	}
	children, err := buildProjectNodes(filtered, rootID)
	if err != nil {
		return nil, err
	}
	project := workspaceRoot("system-project:"+fmt.Sprint(rootID), root.Name, domain.WorkspaceLayerProject, true)
	project.Scope = domain.CodebookScopeSystem
	project.ProjectID = rootID
	project.RuntimePath = ""
	project.Children = children
	return []domain.WorkspaceNode{project}, nil
}

// ReadArtifactFile 读取工作区中不可变制品文件的内容。
func (s *workspaceService) ReadArtifactFile(ctx context.Context,
	projectID, releaseID int64, digest, filePath string) (string, error) {
	if projectID <= 0 {
		return "", fmt.Errorf("%w: 项目 ID 非法: %d", errs.ErrInvalidParameter, projectID)
	}
	contents, err := s.artifacts.ActiveContents(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("查询当前激活制品失败: %w", err)
	}
	// 读取前确认发布和文件仍属于当前工作区，禁止借接口访问历史制品。
	active := false
	for _, content := range contents {
		if content.Release.ID != releaseID || content.Release.Digest != digest {
			continue
		}
		for _, file := range content.Files {
			if file.Path == filePath {
				active = true
				break
			}
		}
		break
	}
	if !active {
		return "", fmt.Errorf("%w: 工作区文件不属于当前激活制品", errs.ErrInvalidParameter)
	}
	return s.artifacts.ReadFile(ctx, releaseID, digest, filePath)
}

func workspaceRoot(key, name string, layer domain.WorkspaceLayer, readonly bool) domain.WorkspaceNode {
	return domain.WorkspaceNode{
		Key: key, Name: name, Kind: domain.CodebookKindDirectory,
		Scope: workspaceScope(layer), Layer: layer, RuntimePath: name,
		Readonly: readonly, Children: make([]domain.WorkspaceNode, 0),
	}
}

func workspaceScope(layer domain.WorkspaceLayer) domain.CodebookScope {
	if layer == domain.WorkspaceLayerSystem {
		return domain.CodebookScopeSystem
	}
	return domain.CodebookScopeTenant
}

func buildProjectNodes(source []domain.Codebook, projectID int64) ([]domain.WorkspaceNode, error) {
	children := make(map[int64][]domain.Codebook, len(source))
	known := make(map[int64]struct{}, len(source))
	for _, node := range source {
		known[node.ID] = struct{}{}
		children[node.ParentID] = append(children[node.ParentID], node)
	}
	for _, node := range source {
		if node.ParentID > 0 {
			if _, ok := known[node.ParentID]; !ok {
				return nil, fmt.Errorf("代码资源节点 %d 的父节点 %d 不存在", node.ID, node.ParentID)
			}
		}
	}
	visiting := make(map[int64]bool, len(source))
	visited := make(map[int64]bool, len(source))
	var build func(int64, string) ([]domain.WorkspaceNode, error)
	build = func(parentID int64, parentPath string) ([]domain.WorkspaceNode, error) {
		result := make([]domain.WorkspaceNode, 0, len(children[parentID]))
		for _, node := range children[parentID] {
			if visiting[node.ID] {
				return nil, fmt.Errorf("代码资源目录存在循环引用，节点 ID=%d", node.ID)
			}
			visiting[node.ID] = true
			visited[node.ID] = true
			runtimePath := path.Join(parentPath, node.Name)
			value := domain.WorkspaceNode{
				Key: fmt.Sprintf("project:%d", node.ID), SourceID: node.ID,
				Name: node.Name, Owner: node.Owner, Kind: node.Kind, Scope: node.Scope,
				Layer: domain.WorkspaceLayerProject, RuntimePath: runtimePath,
				Readonly:  node.Scope == domain.CodebookScopeSystem,
				ProjectID: projectID, ParentID: node.ParentID, SortNo: node.SortNo,
				DownloadOnly: node.StorageType == domain.CodebookContentBlob, Size: node.Size,
				CTime: node.CTime, UTime: node.UTime,
				Children: make([]domain.WorkspaceNode, 0),
			}
			var buildErr error
			value.Children, buildErr = build(node.ID, runtimePath)
			delete(visiting, node.ID)
			if buildErr != nil {
				return nil, buildErr
			}
			result = append(result, value)
		}
		return result, nil
	}
	result, err := build(0, "")
	if err != nil {
		return nil, err
	}
	if len(visited) != len(source) {
		return nil, fmt.Errorf("代码资源树包含无法从根目录访问的节点")
	}
	return result, nil
}

type artifactTreeNode struct {
	name     string
	filePath string
	children map[string]*artifactTreeNode
}

func buildArtifactNodes(content domain.ArtifactContent, rootPath string,
	layer domain.WorkspaceLayer) []domain.WorkspaceNode {
	root := &artifactTreeNode{children: make(map[string]*artifactTreeNode)}
	for _, file := range content.Files {
		current := root
		segments := strings.Split(file.Path, "/")
		for index, segment := range segments {
			next := current.children[segment]
			if next == nil {
				next = &artifactTreeNode{name: segment, children: make(map[string]*artifactTreeNode)}
				current.children[segment] = next
			}
			if index == len(segments)-1 {
				next.filePath = file.Path
			}
			current = next
		}
	}
	return convertArtifactNodes(root, content.Release, rootPath, layer, content.Files)
}

func convertArtifactNodes(parent *artifactTreeNode, release domain.ArtifactRelease,
	parentPath string, layer domain.WorkspaceLayer,
	files []domain.ArtifactManifestFile) []domain.WorkspaceNode {
	// 制品清单不保存逐文件时间；不可变节点统一使用所属发布的创建时间。
	names := make([]string, 0, len(parent.children))
	for name := range parent.children {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]domain.WorkspaceNode, 0, len(names))
	for _, name := range names {
		node := parent.children[name]
		runtimePath := path.Join(parentPath, name)
		kind := domain.CodebookKindDirectory
		if node.filePath != "" {
			kind = domain.CodebookKindFile
		}
		value := domain.WorkspaceNode{
			Key:       fmt.Sprintf("artifact:%d:%s", release.ID, runtimePath),
			ReleaseID: release.ID, Digest: release.Digest, ArtifactPath: node.filePath,
			Name: name, Kind: kind, Scope: release.Scope, Layer: layer,
			RuntimePath: runtimePath, Readonly: true, ProjectID: release.ProjectID,
			Size:      artifactNodeSize(files, node.filePath),
			CTime:     release.CTime,
			UTime:     release.CTime,
			Namespace: release.Namespace, Children: make([]domain.WorkspaceNode, 0),
		}
		value.Children = convertArtifactNodes(node, release, runtimePath, layer, files)
		result = append(result, value)
	}
	return result
}

func artifactNodeSize(files []domain.ArtifactManifestFile, filePath string) int64 {
	for _, file := range files {
		if file.Path == filePath {
			return file.Size
		}
	}
	return 0
}
