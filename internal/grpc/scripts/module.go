package scripts

import (
	"fmt"
	"sync"

	"github.com/Duke1616/etask/internal/grpc/scripts/engine"
	"github.com/Duke1616/etask/internal/grpc/scripts/language/ansible"
	ansibleconnection "github.com/Duke1616/etask/internal/grpc/scripts/language/ansible/connection"
	"github.com/Duke1616/etask/internal/grpc/scripts/language/python"
	"github.com/Duke1616/etask/internal/grpc/scripts/language/shell"
	"github.com/Duke1616/etask/internal/grpc/scripts/runtimefs"
	"github.com/Duke1616/etask/sdk/executor"
)

// Runtime 组合脚本执行端口的默认实现，并管理其启动生命周期。
type Runtime struct {
	handlers   []executor.TaskHandler
	adapters   []engine.Adapter
	workspaces engine.WorkspaceFactory
	archiver   engine.Archiver
	initOnce   sync.Once
	initErr    error
}

// NewRuntime 根据配置装配工作区、归档器和语言处理器。
func NewRuntime(config RuntimeConfig) (*Runtime, error) {
	profile, err := config.executionProfile()
	if err != nil {
		return nil, err
	}
	// 文件系统能力先独立构造，语言处理器只依赖抽象端口。
	workspaces, err := runtimefs.NewWorkspaceFactory(config.workspaceConfig(), profile.workspace)
	if err != nil {
		return nil, err
	}
	archiver, err := runtimefs.NewArchiver(config.archiveConfig())
	if err != nil {
		return nil, err
	}
	credentials := make(map[string]ansibleconnection.CredentialConfig, len(config.Ansible.Credentials))
	for reference, credential := range config.Ansible.Credentials {
		credentials[reference] = ansibleconnection.CredentialConfig{
			Type: credential.Type, Username: credential.Username,
			PrivateKeyFile: credential.PrivateKeyFile, PasswordFile: credential.PasswordFile,
		}
	}
	credentialProvider, err := ansibleconnection.NewLocalCredentialProvider(config.Ansible.CredentialRoot, credentials)
	if err != nil {
		return nil, err
	}
	hostKeyProvider, err := ansibleconnection.NewFileHostKeyProvider(config.Ansible.KnownHostsFile)
	if err != nil {
		return nil, err
	}
	connections := ansibleconnection.NewSSHPreparer(
		credentialProvider, hostKeyProvider, ansibleconnection.WithSSHPassBinary(config.Ansible.SSHPassBinary),
	)
	adapters := []engine.Adapter{
		shell.New(config.ShellBinary),
		python.New(config.PythonBinary),
		ansible.New(config.Ansible.Binary, ansible.WithSSHConnectionPreparer(connections)),
	}
	// 每种语言复用同一套执行编排，仅替换命令和参数适配逻辑。
	handlers := make([]executor.TaskHandler, 0, len(adapters))
	for _, adapter := range adapters {
		handler, handlerErr := engine.NewHandler(
			config.engineConfig(), adapter, workspaces, archiver, profile.launcher,
		)
		if handlerErr != nil {
			return nil, fmt.Errorf("创建 %s 脚本处理器失败: %w", adapter.Name(), handlerErr)
		}
		handlers = append(handlers, handler)
	}
	return &Runtime{
		handlers: handlers, adapters: adapters, workspaces: workspaces, archiver: archiver,
	}, nil
}

// Handlers 返回已装配的脚本任务处理器副本。
func (r *Runtime) Handlers() []executor.TaskHandler {
	return append([]executor.TaskHandler(nil), r.handlers...)
}

// Prune 清理启动前遗留的工作区和归档目录。
func (r *Runtime) Prune() error {
	if err := r.workspaces.Prune(); err != nil {
		return fmt.Errorf("清理脚本工作区失败: %w", err)
	}
	if err := r.archiver.Prune(); err != nil {
		return fmt.Errorf("清理脚本归档失败: %w", err)
	}
	return nil
}

// Initialize 校验运行依赖并清理上次异常退出遗留的文件。
func (r *Runtime) Initialize() error {
	r.initOnce.Do(func() {
		// 先验证解释器与目录，避免清理完成后才发现节点无法执行任务。
		if r.initErr = r.ValidateExecutables(); r.initErr == nil {
			r.initErr = r.Prune()
		}
	})
	return r.initErr
}

// ValidateExecutables 校验解释器与运行目录在启动时可用。
func (r *Runtime) ValidateExecutables() error {
	for _, adapter := range r.adapters {
		if err := adapter.Validate(); err != nil {
			return err
		}
	}
	if err := r.workspaces.Validate(); err != nil {
		return fmt.Errorf("脚本工作区目录不可用: %w", err)
	}
	if err := r.archiver.Validate(); err != nil {
		return fmt.Errorf("脚本归档目录不可用: %w", err)
	}
	return nil
}
