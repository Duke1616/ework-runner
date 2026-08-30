package blobstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Local struct {
	root string
}

func NewLocal(root string) (*Local, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("本地制品存储根目录不能为空")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("解析本地制品存储根目录失败: %w", err)
	}
	if err = os.MkdirAll(abs, 0o750); err != nil {
		return nil, fmt.Errorf("创建本地制品存储根目录失败: %w", err)
	}
	return &Local{root: abs}, nil
}

func (l *Local) Put(ctx context.Context, key string, src io.Reader, options PutOptions) error {
	path, err := l.resolve(key)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("创建本地制品对象目录失败: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".artifact-*.part")
	if err != nil {
		return fmt.Errorf("创建本地制品临时文件失败: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()

	h := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(tmp, h), &contextReader{ctx: ctx, r: src})
	if copyErr != nil {
		return fmt.Errorf("写入本地制品临时文件失败: %w", copyErr)
	}
	if options.Size >= 0 && written != options.Size {
		return fmt.Errorf("Blob 大小不一致: 预期=%d 实际=%d", options.Size, written)
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if options.Checksum != "" && !strings.EqualFold(actual, options.Checksum) {
		return fmt.Errorf("Blob 校验和不一致: 预期=%s 实际=%s", options.Checksum, actual)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("同步本地制品临时文件失败: %w", err)
	}
	if err = tmp.Chmod(0o640); err != nil {
		return fmt.Errorf("设置本地制品文件权限失败: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("关闭本地制品临时文件失败: %w", err)
	}
	// 同目录 rename 保证读者只能看到完整文件。内容寻址对象允许相同 key 的幂等覆盖。
	if err = os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("提交本地制品对象失败: %w", err)
	}
	return nil
}

func (l *Local) Delete(_ context.Context, key string) error {
	path, err := l.resolve(key)
	if err != nil {
		return err
	}
	if err = os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("删除本地 Blob 对象失败: %w", err)
	}
	return nil
}

func (l *Local) Open(_ context.Context, key string) (io.ReadCloser, error) {
	path, err := l.resolve(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("打开本地制品对象失败: %w", err)
	}
	return f, nil
}

func (l *Local) Exists(_ context.Context, key string) (bool, error) {
	path, err := l.resolve(key)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("检查本地制品对象失败: %w", err)
}

func (l *Local) resolve(key string) (string, error) {
	cleaned, err := cleanAndValidateKey(key)
	if err != nil {
		return "", err
	}
	path := filepath.Join(l.root, filepath.FromSlash(cleaned))
	rel, err := filepath.Rel(l.root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: 制品对象键超出存储根目录 %q", ErrInvalidKey, key)
	}
	return path, nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
		return r.r.Read(p)
	}
}
