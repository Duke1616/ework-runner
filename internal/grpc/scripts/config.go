package scripts

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Duke1616/etask/internal/grpc/scripts/engine"
	"github.com/Duke1616/etask/internal/grpc/scripts/runtimefs"
)

const (
	SandboxModeAuto     = "auto"
	SandboxModeRequired = "required"
	SandboxModeOff      = "off"
)

// SandboxConfig 配置脚本子进程的非特权运行身份。
type SandboxConfig struct {
	Mode string `mapstructure:"mode" yaml:"mode"`
	UID  int64  `mapstructure:"uid" yaml:"uid"`
	GID  int64  `mapstructure:"gid" yaml:"gid"`
}

// ArchiveConfig 描述脚本执行现场归档配置。
type ArchiveConfig struct {
	Enabled    *bool         `mapstructure:"enabled" yaml:"enabled"`
	FailedOnly bool          `mapstructure:"failed_only" yaml:"failed_only"`
	Dir        string        `mapstructure:"dir" yaml:"dir"`
	MaxAge     time.Duration `mapstructure:"max_age" yaml:"max_age"`
	MaxSize    int64         `mapstructure:"max_size" yaml:"max_size"`
}

// RuntimeConfig 汇总脚本执行编排、工作区、解释器和归档配置。
type RuntimeConfig struct {
	WorkspaceDir     string        `mapstructure:"workspace_dir" yaml:"workspace_dir"`
	WorkspaceMaxAge  time.Duration `mapstructure:"workspace_max_age" yaml:"workspace_max_age"`
	PythonBinary     string        `mapstructure:"python_binary" yaml:"python_binary"`
	ShellBinary      string        `mapstructure:"shell_binary" yaml:"shell_binary"`
	MaxCodeSize      int64         `mapstructure:"max_code_size" yaml:"max_code_size"`
	MaxArgsSize      int64         `mapstructure:"max_args_size" yaml:"max_args_size"`
	MaxVariablesSize int64         `mapstructure:"max_variables_size" yaml:"max_variables_size"`
	MaxLogLineSize   int           `mapstructure:"max_log_line_size" yaml:"max_log_line_size"`
	MaxResultSize    int64         `mapstructure:"max_result_size" yaml:"max_result_size"`
	Sandbox          SandboxConfig `mapstructure:"sandbox" yaml:"sandbox"`
	Archive          ArchiveConfig `mapstructure:"archive" yaml:"archive"`
}

func (c RuntimeConfig) engineConfig(sandbox engine.Sandbox) engine.Config {
	return engine.Config{
		MaxCodeSize:      c.MaxCodeSize,
		MaxArgsSize:      c.MaxArgsSize,
		MaxVariablesSize: c.MaxVariablesSize,
		MaxLogLineSize:   c.MaxLogLineSize,
		MaxResultSize:    c.MaxResultSize,
		Sandbox:          sandbox,
	}
}

func (c RuntimeConfig) workspaceConfig(sandbox engine.Sandbox) runtimefs.WorkspaceConfig {
	return runtimefs.WorkspaceConfig{
		Dir: c.WorkspaceDir, MaxAge: c.WorkspaceMaxAge, Sandbox: sandbox,
	}
}

func (c SandboxConfig) resolve(euid int) (engine.Sandbox, error) {
	mode := strings.ToLower(strings.TrimSpace(c.Mode))
	if mode == "" {
		mode = SandboxModeAuto
	}
	if mode != SandboxModeAuto && mode != SandboxModeRequired && mode != SandboxModeOff {
		return engine.Sandbox{}, fmt.Errorf("脚本沙箱模式非法: %s", c.Mode)
	}
	if mode == SandboxModeOff || (mode == SandboxModeAuto && euid != 0) {
		return engine.Sandbox{}, nil
	}
	if euid != 0 {
		return engine.Sandbox{}, fmt.Errorf("脚本沙箱 required 模式要求 Executor 以 root 启动")
	}
	uid, gid := c.UID, c.GID
	if uid == 0 {
		uid = 65534
	}
	if gid == 0 {
		gid = 65534
	}
	if uid <= 0 || gid <= 0 || uid > math.MaxUint32 || gid > math.MaxUint32 {
		return engine.Sandbox{}, fmt.Errorf("脚本沙箱 UID/GID 非法: %d/%d", uid, gid)
	}
	return engine.Sandbox{Enabled: true, UID: uint32(uid), GID: uint32(gid)}, nil
}

func (c RuntimeConfig) resolveSandbox() (engine.Sandbox, error) {
	return c.Sandbox.resolve(effectiveUID())
}

func (c RuntimeConfig) archiveConfig() runtimefs.ArchiveConfig {
	enabled := true
	if c.Archive.Enabled != nil {
		enabled = *c.Archive.Enabled
	}
	return runtimefs.ArchiveConfig{
		Enabled:    enabled,
		FailedOnly: c.Archive.FailedOnly,
		Dir:        c.Archive.Dir,
		MaxAge:     c.Archive.MaxAge,
		MaxSize:    c.Archive.MaxSize,
	}
}
