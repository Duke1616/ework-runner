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

// SourceRef 描述一次执行固定使用的不可变项目源码。
type SourceRef struct {
	SourceID      int64
	Digest        string
	BlobChecksum  string
	Size          int64
	Format        string
	FormatVersion int32
}

// Downloader 将制品内容流式写入目标，具体传输协议由运行环境适配。
type Downloader interface {
	DownloadArtifact(ctx context.Context, ref Ref, target io.Writer) error
	DownloadSource(ctx context.Context, ref SourceRef, target io.Writer) error
}

// PreparedArtifacts 表示一次任务执行使用的不可变制品视图。
type PreparedArtifacts interface {
	// SourceRoot 返回程序项目来源的本地不可变目录；INLINE 或无来源时为空。
	SourceRoot() string
	// Roots 返回 Handler 可读取的默认制品层和具名依赖层目录。
	Roots() executor.ArtifactRoots
}

// Preparer 负责下载、物化和清理任务制品。
type Preparer interface {
	// Prune 清理无效或超出容量限制的本地制品缓存。
	Prune() error
	// Prepare 下载并准备任务使用的程序来源和依赖制品。
	Prepare(ctx context.Context, downloader Downloader, source *SourceRef, dependencies []Ref) (PreparedArtifacts, error)
}
