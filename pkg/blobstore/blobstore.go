package blobstore

import (
	"context"
	"errors"
	"io"
)

//go:generate go tool mockgen -source=./blobstore.go -package=blobstoremocks -destination=./mocks/blobstore.mock.go -typed

var (
	ErrNotFound   = errors.New("Blob 对象不存在")
	ErrInvalidKey = errors.New("非法的制品对象键")
)

// PutOptions 描述保存 Blob 对象时需要校验和记录的元数据。
type PutOptions struct {
	Size        int64
	Checksum    string
	ContentType string
}

// Store 仅负责持久化不可变字节对象，不承载 Codebook 或任务领域语义。
type Store interface {
	// Put 保存不可变对象，key 由上层服务生成。
	Put(ctx context.Context, key string, src io.Reader, options PutOptions) error
	// Open 打开指定对象的只读数据流。
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	// Exists 判断指定对象是否存在。
	Exists(ctx context.Context, key string) (bool, error)
	// Delete 删除指定对象；对象不存在时也视为成功。
	Delete(ctx context.Context, key string) error
}
