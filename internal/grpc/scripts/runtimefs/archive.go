package runtimefs

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Duke1616/etask/internal/grpc/scripts/engine"
	"github.com/Duke1616/etask/sdk/executor"
)

// ArchiveConfig 描述执行现场归档策略。
type ArchiveConfig struct {
	Enabled    bool
	FailedOnly bool
	Dir        string
	MaxAge     time.Duration
	MaxSize    int64
}

// Archiver 使用本地文件系统保存脚本执行现场。
type Archiver struct {
	config ArchiveConfig
}

// NewArchiver 创建文件系统归档器并补全归档目录。
func NewArchiver(config ArchiveConfig) (*Archiver, error) {
	dir, err := ResolveDirectory(config.Dir, filepath.Join(os.TempDir(), "etask", "archive"))
	if err != nil {
		return nil, fmt.Errorf("解析脚本归档目录失败: %w", err)
	}
	config.Dir = dir
	return &Archiver{config: config}, nil
}

// Archive 按配置保存脚本、参数和脱敏后的变量。
func (a *Archiver) Archive(record engine.ArchiveRecord) error {
	if !a.config.Enabled || (a.config.FailedOnly && !record.Failed) {
		return nil
	}
	if err := os.MkdirAll(a.config.Dir, 0o750); err != nil {
		return fmt.Errorf("创建脚本归档根目录失败: %w", err)
	}
	prefix := fmt.Sprintf("%d_%s-", record.ExecutionID, time.Now().Format("20060102150405"))
	directory, err := os.MkdirTemp(a.config.Dir, prefix)
	if err != nil {
		return fmt.Errorf("创建脚本归档目录失败: %w", err)
	}
	// 归档只有全部文件写入成功才保留，失败时删除不完整目录。
	completed := false
	defer func() {
		if !completed {
			_ = os.RemoveAll(directory)
		}
	}()
	if err = copyFile(record.CodeFile, directory); err != nil {
		return err
	}
	if record.Args != "" {
		if err = os.WriteFile(filepath.Join(directory, "scripts.args"), []byte(record.Args), 0o600); err != nil {
			return fmt.Errorf("归档脚本参数失败: %w", err)
		}
	}
	// Secret 变量只保留结构信息，不把明文值写入归档。
	if variables := sanitizeVariables(record.Variables); len(variables) > 0 {
		if err = os.WriteFile(filepath.Join(directory, "scripts.vars.json"), variables, 0o600); err != nil {
			return fmt.Errorf("归档脚本变量失败: %w", err)
		}
	}
	completed = true
	return nil
}

// Prune 清理超过保存时间或容量限制的归档目录。
func (a *Archiver) Prune() error {
	if !a.config.Enabled {
		return nil
	}
	return PruneDirectory(a.config.Dir, a.config.MaxAge, a.config.MaxSize)
}

// Validate 校验启用归档时目录是否可写。
func (a *Archiver) Validate() error {
	if !a.config.Enabled {
		return nil
	}
	return ValidateDirectory(a.config.Dir)
}

func copyFile(source, destinationDir string) error {
	if source == "" {
		return nil
	}
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("打开待归档脚本失败: %w", err)
	}
	defer input.Close()
	output, err := os.OpenFile(
		filepath.Join(destinationDir, "scripts"+filepath.Ext(source)),
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
		0o700,
	)
	if err != nil {
		return fmt.Errorf("创建归档脚本失败: %w", err)
	}
	defer output.Close()

	if _, err = io.Copy(output, input); err != nil {
		return fmt.Errorf("复制归档脚本失败: %w", err)
	}
	return nil
}

func sanitizeVariables(raw string) []byte {
	if raw == "" {
		return nil
	}
	var variables []executor.Variable
	if err := json.Unmarshal([]byte(raw), &variables); err != nil {
		return nil
	}
	for i := range variables {
		if variables[i].Secret {
			variables[i].Value = ""
		}
	}
	result, err := json.Marshal(variables)
	if err != nil {
		return nil
	}
	return result
}

var _ engine.Archiver = (*Archiver)(nil)
