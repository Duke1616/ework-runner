package blobstore

import (
	"context"
	"errors"
	"io"
)

//go:generate go tool mockgen -source=./blobstore.go -package=blobstoremocks -destination=./mocks/blobstore.mock.go -typed

var ErrNotFound = errors.New("制品对象不存在")

// Store 仅负责持久化不可变字节对象，不承载 Codebook 或任务领域语义。
type Store interface {
	// Put 保存不可变对象，key 由上层制品服务生成。
	Put(ctx context.Context, key string, src io.Reader, size int64, checksum string) error
	// Open 打开指定对象的只读数据流。
	Open(ctx context.Context, key string) (io.ReadCloser, error)
}
