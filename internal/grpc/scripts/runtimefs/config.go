package runtimefs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// WorkspaceConfig 描述工作区目录和清理策略。
type WorkspaceConfig struct {
	Dir    string
	MaxAge time.Duration
}

// NormalizeWorkspaceConfig 补全并校验工作区配置。
func NormalizeWorkspaceConfig(config WorkspaceConfig) (WorkspaceConfig, error) {
	dir, err := ResolveDirectory(config.Dir, filepath.Join(os.TempDir(), "etask", "runs"))
	if err != nil {
		return WorkspaceConfig{}, fmt.Errorf("解析脚本工作区目录失败: %w", err)
	}
	config.Dir = dir
	if config.MaxAge <= 0 {
		config.MaxAge = 24 * time.Hour
	}
	return config, nil
}

// ResolveDirectory 按配置或默认值解析绝对目录。
func ResolveDirectory(value, fallback string) (string, error) {
	if strings.TrimSpace(value) == "" {
		value = fallback
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absolute), nil
}
