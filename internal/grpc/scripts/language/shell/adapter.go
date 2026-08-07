package shell

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/Duke1616/etask/internal/grpc/scripts/engine"
	"github.com/Duke1616/etask/internal/grpc/scripts/language"
	"github.com/Duke1616/etask/sdk/executor"
)

var variableNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Adapter 构造 Shell 脚本命令并管理 Shell 变量文件。
type Adapter struct {
	binary string
}

// New 创建 Shell 语言适配器。
func New(binary string) *Adapter {
	if binary == "" {
		binary = "/bin/bash"
	}
	return &Adapter{binary: binary}
}

// Name 返回处理器名称。
func (a *Adapter) Name() string {
	return "shell"
}

// Description 返回处理器功能描述。
func (a *Adapter) Description() string {
	return "执行 Shell 脚本命令的基础处理器"
}

// Extension 返回 Shell 文件扩展名。
func (a *Adapter) Extension() string {
	return ".sh"
}

// Metadata 返回 Shell 脚本参数元数据。
func (a *Adapter) Metadata() []executor.Parameter {
	return language.Metadata()
}

// Prepare 创建变量文件，并通过受控文件传递参数和变量。
func (a *Adapter) Prepare(ctx context.Context, workspace engine.Workspace,
	input engine.Input) (engine.PreparedCommand, error) {
	// 变量同时写入受限 env 文件和进程环境，兼顾 source 与直接读取两种脚本方式。
	variablesFile, variableEnv, err := prepareVariables(workspace, input.Variables)
	if err != nil {
		return engine.PreparedCommand{}, err
	}
	environment := append(variableEnv, "ETASK_SHELL_ENV_FILE="+variablesFile)
	// args 和原始 variables 通过固定文件传递，不再依赖位置参数。
	fileEnv, err := language.FileInput(workspace, input)
	if err != nil {
		return engine.PreparedCommand{}, err
	}
	environment = append(environment, fileEnv...)
	command := exec.CommandContext(ctx, a.binary, "-e", workspace.EntryPoint())
	return engine.PreparedCommand{
		Command: language.ConfigureCancellation(command), Environment: environment,
	}, nil
}

// Validate 校验 Shell 解释器是否存在。
func (a *Adapter) Validate() error {
	if _, err := exec.LookPath(a.binary); err != nil {
		return fmt.Errorf("未找到 Shell 解释器 %s: %w", a.binary, err)
	}
	return nil
}

func prepareVariables(workspace engine.Workspace, raw string) (string, []string, error) {
	if strings.TrimSpace(raw) == "" {
		raw = "[]"
	}
	var variables []executor.Variable
	if err := json.Unmarshal([]byte(raw), &variables); err != nil {
		return "", nil, fmt.Errorf("解析 Shell 变量失败: %w", err)
	}
	// 合并同名变量并校验保留名称，避免覆盖运行时控制变量。
	merged := make(map[string]executor.Variable, len(variables))
	keys := make([]string, 0, len(variables))
	for _, variable := range variables {
		variable.Key = strings.TrimSpace(variable.Key)
		if !variableNamePattern.MatchString(variable.Key) {
			return "", nil, fmt.Errorf("Shell 变量名非法: %q", variable.Key)
		}
		if strings.HasPrefix(variable.Key, "ETASK_") || variable.Key == "EWORK_RESULT_FD" {
			return "", nil, fmt.Errorf("Shell 变量名使用了系统保留前缀: %s", variable.Key)
		}
		if strings.ContainsRune(variable.Value, '\x00') {
			return "", nil, fmt.Errorf("Shell 变量 %s 包含空字符", variable.Key)
		}
		if _, exists := merged[variable.Key]; !exists {
			keys = append(keys, variable.Key)
		}
		merged[variable.Key] = variable
	}
	// env 文件使用 Shell 单引号转义，文件权限限制为当前用户可读写。
	var content strings.Builder
	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		variable := merged[key]
		content.WriteString(variable.Key)
		content.WriteByte('=')
		content.WriteString(quote(variable.Value))
		content.WriteByte('\n')
		environment = append(environment, variable.Key+"="+variable.Value)
	}
	path, err := workspace.WriteFile("variables.env", []byte(content.String()), 0o600)
	if err != nil {
		return "", nil, fmt.Errorf("写入 Shell 变量文件失败: %w", err)
	}
	return path, environment, nil
}

func quote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

var _ engine.Adapter = (*Adapter)(nil)
