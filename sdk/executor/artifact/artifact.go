// Package artifact 定义 Executor 节点和自定义执行引擎可选的制品物化契约。
package artifact

import (
	"context"
	"io"

	"github.com/Duke1616/etask/sdk/executor"
)

// Ref 描述一次执行固定使用的不可变制品。
type Ref struct {
	ReleaseID     int64
	Digest        string
	BlobChecksum  string
	Size          int64
	Format        string
	FormatVersion int32
	MountName     string
}

// Downloader 将制品内容流式写入目标，具体传输协议由运行环境适配。
type Downloader interface {
	Download(ctx context.Context, ref Ref, target io.Writer) error
}

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
	Prepare(ctx context.Context, downloader Downloader, refs []Ref) (PreparedArtifacts, error)
}
