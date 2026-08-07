package python

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/Duke1616/etask/internal/grpc/scripts/engine"
	"github.com/Duke1616/etask/internal/grpc/scripts/language"
	"github.com/Duke1616/etask/sdk/executor"
)

// Adapter 构造 Python 脚本命令。
type Adapter struct {
	binary string
}

// New 创建 Python 语言适配器。
func New(binary string) *Adapter {
	if binary == "" {
		binary = "python"
	}
	return &Adapter{binary: binary}
}

// Name 返回处理器名称。
func (a *Adapter) Name() string {
	return "python"
}

// Description 返回处理器功能描述。
func (a *Adapter) Description() string {
	return "执行 Python 脚本代码的基础处理器"
}

// Extension 返回 Python 文件扩展名。
func (a *Adapter) Extension() string {
	return ".py"
}

// Metadata 返回 Python 脚本参数元数据。
func (a *Adapter) Metadata() []executor.Parameter {
	return language.Metadata()
}

// Prepare 创建 Python 命令，并通过受控文件传递参数和变量。
func (a *Adapter) Prepare(ctx context.Context, workspace engine.Workspace,
	input engine.Input) (engine.PreparedCommand, error) {
	environment, err := language.FileInput(workspace, input)
	if err != nil {
		return engine.PreparedCommand{}, err
	}
	command := exec.CommandContext(ctx, a.binary, workspace.EntryPoint())
	return engine.PreparedCommand{
		Command: language.ConfigureCancellation(command), Environment: environment,
	}, nil
}

// Validate 校验 Python 解释器是否存在。
func (a *Adapter) Validate() error {
	if _, err := exec.LookPath(a.binary); err != nil {
		return fmt.Errorf("未找到 Python 解释器 %s: %w", a.binary, err)
	}
	return nil
}

var _ engine.Adapter = (*Adapter)(nil)
