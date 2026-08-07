// Package artifact 实现 Agent 和 Executor 共用的制品下载、缓存和本地物化。
package artifact

import (
	"context"

	"github.com/Duke1616/etask/sdk/executor"
	executorartifact "github.com/Duke1616/etask/sdk/executor/artifact"
)

// Prepared 描述任务可直接使用的不可变缓存目录。
type Prepared struct {
	sourceRoot string
	roots      executor.ArtifactRoots
}

// SourceRoot 返回已准备的程序项目来源目录。
func (p Prepared) SourceRoot() string {
	return p.sourceRoot
}

// Roots 返回 Handler 可读取的制品目录。
func (p Prepared) Roots() executor.ArtifactRoots {
	return p.roots
}

// Runtime 管理制品缓存和本地物化。
type Runtime struct {
	cache *artifactCache
}

// NewRuntime 创建制品运行时。
func NewRuntime(config Config) *Runtime {
	return &Runtime{cache: newArtifactCache(config)}
}

// Prune 清理未完成下载和超出容量限制的缓存层。
func (r *Runtime) Prune() error {
	return r.cache.Prune()
}

// Prepare 下载任务声明的项目来源、默认制品层和具名依赖层。
func (r *Runtime) Prepare(ctx context.Context, downloader executorartifact.Downloader,
	source *executorartifact.SourceRef, refs []executorartifact.Ref) (executorartifact.PreparedArtifacts, error) {
	layers, err := parseLayerSet(source, refs)
	if err != nil {
		return nil, err
	}
	prepared := Prepared{roots: executor.ArtifactRoots{}}
	if layers.sourceLayer != nil {
		prepared.sourceRoot, err = r.cache.Ensure(ctx, downloader, *layers.sourceLayer)
		if err != nil {
			return nil, err
		}
	}
	if layers.hasDefault {
		prepared.roots.Default, err = r.cache.Ensure(ctx, downloader, layers.defaultLayer)
		if err != nil {
			return nil, err
		}
	}

	if len(layers.namedLayers) > 0 {
		prepared.roots.Named = make(map[string]string, len(layers.namedLayers))
	}
	for _, ref := range layers.namedLayers {
		root, ensureErr := r.cache.Ensure(ctx, downloader, ref)
		if ensureErr != nil {
			return nil, ensureErr
		}
		prepared.roots.Named[ref.mountName] = root
	}
	return prepared, nil
}

var _ executorartifact.Preparer = (*Runtime)(nil)
