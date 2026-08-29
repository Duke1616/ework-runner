package artifact

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	artifactarchive "github.com/Duke1616/etask/internal/artifact/archive"
	executorartifact "github.com/Duke1616/etask/sdk/executor/artifact"
	"golang.org/x/sync/singleflight"
)

const (
	defaultMaxArtifactSize      = int64(512 << 20)
	defaultMaxUnpackedSize      = int64(2 << 30)
	defaultMaxArtifactFileCount = 10000
	defaultMaxArtifactCacheSize = int64(10 << 30)
	artifactDownloadTimeout     = 10 * time.Minute
)

// Config 描述制品缓存目录和资源限制。
type Config struct {
	Dir             string `mapstructure:"dir" yaml:"dir"`
	MaxDownloadSize int64  `mapstructure:"max_download_size" yaml:"max_download_size"`
	MaxUnpackedSize int64  `mapstructure:"max_unpacked_size" yaml:"max_unpacked_size"`
	MaxFileCount    int    `mapstructure:"max_file_count" yaml:"max_file_count"`
	MaxCacheSize    int64  `mapstructure:"max_cache_size" yaml:"max_cache_size"`
}

type cacheLayout struct {
	root string
}

func (l cacheLayout) tempDir() string   { return filepath.Join(l.root, "tmp") }
func (l cacheLayout) layersDir() string { return filepath.Join(l.root, "layers") }
func (l cacheLayout) layerDir(ref layerRef) string {
	return filepath.Join(l.layersDir(), ref.cacheKey())
}

func (l cacheLayout) ensure() error {
	if err := os.MkdirAll(l.tempDir(), artifactarchive.PermDir); err != nil {
		return fmt.Errorf("创建制品缓存临时目录失败: %w", err)
	}
	if err := os.MkdirAll(l.layersDir(), artifactarchive.PermDir); err != nil {
		return fmt.Errorf("创建制品缓存层目录失败: %w", err)
	}
	for _, dir := range []string{l.root, l.tempDir(), l.layersDir()} {
		if err := os.Chmod(dir, artifactarchive.PermDir); err != nil {
			return fmt.Errorf("收紧制品缓存目录权限失败: %w", err)
		}
	}
	return nil
}

type artifactCache struct {
	cfg    Config
	layout cacheLayout
	codec  *artifactarchive.Codec
	group  singleflight.Group
}

func newArtifactCache(cfg Config) *artifactCache {
	if strings.TrimSpace(cfg.Dir) == "" {
		cfg.Dir = filepath.Join(os.TempDir(), "etask", "artifact-cache")
	}
	if cfg.MaxDownloadSize <= 0 {
		cfg.MaxDownloadSize = defaultMaxArtifactSize
	}
	if cfg.MaxUnpackedSize <= 0 {
		cfg.MaxUnpackedSize = defaultMaxUnpackedSize
	}
	if cfg.MaxFileCount <= 0 {
		cfg.MaxFileCount = defaultMaxArtifactFileCount
	}
	if cfg.MaxCacheSize <= 0 {
		cfg.MaxCacheSize = defaultMaxArtifactCacheSize
	}
	return &artifactCache{
		cfg: cfg, layout: cacheLayout{root: cfg.Dir}, codec: artifactarchive.New(""),
	}
}

func (c *artifactCache) Prune() error {
	if err := c.layout.ensure(); err != nil {
		return err
	}
	return pruneCache(c.layout.root, c.cfg.MaxCacheSize)
}

func (c *artifactCache) Ensure(ctx context.Context, downloader executorartifact.Downloader,
	ref layerRef) (string, error) {
	if downloader == nil {
		return "", fmt.Errorf("制品下载客户端尚未初始化")
	}
	if ref.size > c.cfg.MaxDownloadSize {
		return "", fmt.Errorf("制品大小超出限制: %d", ref.size)
	}

	// 并发控制设计：
	// 1. 使用 SingleFlight 合并相同 cacheKey 的并发请求，防止并发任务引发网络带宽与磁盘 I/O 尖峰；
	// 2. 使用 context.WithoutCancel(ctx) 将底层下载物化与单个调用方的 Context 解绑；即便某个任务
	//    中途取消，物化仍可在后台以 artifactDownloadTimeout 独立完成，使其余并发者和未来任务仍可复用缓存。
	result := c.group.DoChan(ref.cacheKey(), func() (any, error) {
		workCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), artifactDownloadTimeout)
		defer cancel()
		return c.ensureOnce(workCtx, downloader, ref)
	})
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case value := <-result:
		if value.Err != nil {
			return "", value.Err
		}
		return value.Val.(string), nil
	}
}

func (c *artifactCache) ensureOnce(ctx context.Context, downloader executorartifact.Downloader,
	ref layerRef) (string, error) {
	if err := c.layout.ensure(); err != nil {
		return "", err
	}
	targetDir := c.layout.layerDir(ref)
	if readyArtifact(targetDir, ref) {
		touchArtifact(targetDir)
		return targetDir, nil
	}
	return c.materialize(ctx, downloader, ref, targetDir)
}
