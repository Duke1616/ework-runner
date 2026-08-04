// Package artifact 定义 Executor 节点和自定义执行引擎可选的制品物化契约。
package artifact

import (
	"context"

	artifactv1 "github.com/Duke1616/etask/api/proto/gen/etask/artifact/v1"
	"github.com/Duke1616/etask/sdk/executor"
)

// PreparedArtifacts 表示一次任务执行使用的不可变制品视图。
type PreparedArtifacts interface {
	// Roots 返回 Handler 可读取的默认制品层和具名依赖层目录。
	Roots() executor.ArtifactRoots
	// Close 释放准备器可能持有的任务级资源；无状态实现可直接返回 nil。
	Close() error
}

// Preparer 负责下载、物化和清理任务制品。
type Preparer interface {
	// Prune 清理无效或超出容量限制的本地制品缓存。
	Prune() error
	// Prepare 下载并准备任务使用的制品运行现场。
	Prepare(ctx context.Context, client artifactv1.ArtifactServiceClient,
		refs []*artifactv1.ArtifactRef) (PreparedArtifacts, error)
}
