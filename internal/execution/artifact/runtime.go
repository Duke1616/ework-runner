// Package artifact 实现 Agent 和 Executor 共用的制品下载、缓存和本地物化。
package artifact

import (
	"context"

	artifactv1 "github.com/Duke1616/etask/api/proto/gen/etask/artifact/v1"
	"github.com/Duke1616/etask/sdk/executor"
)

// Prepared 描述任务可直接使用的不可变缓存目录。
type Prepared struct {
	roots executor.ArtifactRoots
}

// Roots 返回 Handler 可读取的制品目录。
func (p Prepared) Roots() executor.ArtifactRoots {
	return p.roots
}

// Close 实现 PreparedArtifacts 契约；不可变缓存层没有任务级资源需要释放。
func (p Prepared) Close() error {
	return nil
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

// Prepare 下载任务声明的默认制品层和具名依赖层。
func (r *Runtime) Prepare(ctx context.Context, client artifactv1.ArtifactServiceClient,
	refs []*artifactv1.ArtifactRef) (executor.PreparedArtifacts, error) {
	layers, err := parseLayerSet(refs)
	if err != nil {
		return nil, err
	}
	prepared := Prepared{roots: executor.ArtifactRoots{}}
	if layers.hasDefault {
		prepared.roots.Default, err = r.cache.Ensure(ctx, client, layers.defaultLayer)
		if err != nil {
			return nil, err
		}
	}

	if len(layers.namedLayers) > 0 {
		prepared.roots.Named = make(map[string]string, len(layers.namedLayers))
	}
	for _, ref := range layers.namedLayers {
		root, ensureErr := r.cache.Ensure(ctx, client, ref)
		if ensureErr != nil {
			return nil, ensureErr
		}
		prepared.roots.Named[ref.mountName] = root
	}
	return prepared, nil
}

var _ executor.ArtifactPreparer = (*Runtime)(nil)
