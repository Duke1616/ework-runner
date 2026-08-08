package repository

import (
	"fmt"
	"path"
	"strings"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository/dao"
	"github.com/samber/lo"
)

type artifactSnapshot struct {
	nodes    []dao.Codebook
	versions map[int64]dao.CodebookVersion
	paths    *artifactPathResolver
}

func newArtifactSnapshot(nodes []dao.Codebook, versions []dao.CodebookVersion) artifactSnapshot {
	return artifactSnapshot{
		nodes: nodes,
		versions: lo.SliceToMap(versions, func(version dao.CodebookVersion) (int64, dao.CodebookVersion) {
			return version.ID, version
		}),
		paths: newArtifactPathResolver(nodes),
	}
}

func (s artifactSnapshot) Files() ([]domain.ArtifactFile, error) {
	files := make([]domain.ArtifactFile, 0, len(s.versions))
	// 目录节点只参与路径恢复，发布内容只包含具备当前版本的文件节点。
	for _, node := range s.nodes {
		if node.Kind != domain.CodebookKindFile.String() {
			continue
		}
		version, ok := s.versions[node.CurrentVersionID]
		if !ok {
			return nil, fmt.Errorf("代码资源文件 %s 的当前版本 %d 不存在", node.Name, node.CurrentVersionID)
		}
		filePath, err := s.paths.Resolve(node)
		if err != nil {
			return nil, err
		}
		files = append(files, domain.ArtifactFile{
			Path: filePath, Hash: version.Hash, Code: version.Code,
			StorageType: domain.CodebookContentStorage(version.StorageType),
			ObjectKey:   version.ObjectKey, Size: version.Size,
		})
	}
	return files, nil
}

func artifactCurrentVersionIDs(nodes []dao.Codebook) ([]int64, error) {
	ids := make([]int64, 0, len(nodes))
	for _, node := range nodes {
		if node.Kind != domain.CodebookKindFile.String() {
			continue
		}
		if node.CurrentVersionID <= 0 {
			return nil, fmt.Errorf("代码资源文件 %s 尚未设置当前版本", node.Name)
		}
		ids = append(ids, node.CurrentVersionID)
	}
	return ids, nil
}

type artifactPathResolver struct {
	nodes    map[int64]dao.Codebook
	cache    map[int64]string
	visiting map[int64]bool
}

func newArtifactPathResolver(nodes []dao.Codebook) *artifactPathResolver {
	return &artifactPathResolver{
		nodes: lo.SliceToMap(nodes, func(node dao.Codebook) (int64, dao.Codebook) {
			return node.ID, node
		}),
		cache:    make(map[int64]string, len(nodes)),
		visiting: make(map[int64]bool, len(nodes)),
	}
}

func (r *artifactPathResolver) Resolve(node dao.Codebook) (string, error) {
	// 相同父目录只解析一次，大目录树不会重复递归回溯。
	if cached, ok := r.cache[node.ID]; ok {
		return cached, nil
	}
	if r.visiting[node.ID] {
		return "", fmt.Errorf("代码资源目录存在循环引用，节点 ID=%d", node.ID)
	}
	if err := validateArtifactPathSegment(node.Name); err != nil {
		return "", fmt.Errorf("代码资源节点 %d 名称非法: %w", node.ID, err)
	}

	// visiting 标记当前递归链，单独用于发现损坏数据中的父子循环。
	r.visiting[node.ID] = true
	defer delete(r.visiting, node.ID)
	resolved := node.Name
	if node.ParentID > 0 {
		parent, ok := r.nodes[node.ParentID]
		if !ok {
			return "", fmt.Errorf("代码资源节点 %d 的父节点 %d 不存在", node.ID, node.ParentID)
		}
		parentPath, err := r.Resolve(parent)
		if err != nil {
			return "", err
		}
		resolved = path.Join(parentPath, node.Name)
	}
	r.cache[node.ID] = resolved
	return resolved, nil
}

func validateArtifactPathSegment(name string) error {
	if strings.TrimSpace(name) == "" || name == "." || name == ".." {
		return fmt.Errorf("名称不能为空或特殊路径")
	}
	if strings.ContainsAny(name, "/\\\x00") {
		return fmt.Errorf("名称不能包含路径分隔符或空字符")
	}
	return nil
}
