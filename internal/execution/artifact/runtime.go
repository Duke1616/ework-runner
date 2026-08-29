// Package artifact 实现 Agent 和 Executor 共用的制品下载、缓存和本地物化。
package artifact

import (
	"context"
	"sync"

	"github.com/Duke1616/etask/sdk/executor"
	executorartifact "github.com/Duke1616/etask/sdk/executor/artifact"
	"golang.org/x/sync/errgroup"
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
	if len(layers.namedLayers) > 0 {
		prepared.roots.Named = make(map[string]string, len(layers.namedLayers))
	}

	// 源码层、默认层和各具名层互相独立，使用 errgroup 并行拉取与物化，降低任务准备耗时。
	g, gCtx := errgroup.WithContext(ctx)
	if layers.sourceLayer != nil {
		g.Go(func() error {
			root, ensureErr := r.cache.Ensure(gCtx, downloader, *layers.sourceLayer)
			if ensureErr != nil {
				return ensureErr
			}
			prepared.sourceRoot = root
			return nil
		})
	}
	if layers.hasDefault {
		g.Go(func() error {
			root, ensureErr := r.cache.Ensure(gCtx, downloader, layers.defaultLayer)
			if ensureErr != nil {
				return ensureErr
			}
			prepared.roots.Default = root
			return nil
		})
	}

	var mu sync.Mutex
	for _, ref := range layers.namedLayers {
		namedRef := ref
		g.Go(func() error {
			root, ensureErr := r.cache.Ensure(gCtx, downloader, namedRef)
			if ensureErr != nil {
				return ensureErr
			}
			mu.Lock()
			prepared.roots.Named[namedRef.mountName] = root
			mu.Unlock()
			return nil
		})
	}

	if err = g.Wait(); err != nil {
		return nil, err
	}
	return prepared, nil
}

var _ executorartifact.Preparer = (*Runtime)(nil)
