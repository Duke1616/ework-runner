// Package ansible 实现基于 ansible-playbook 命令的 PROJECT 程序适配器。
package ansible

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Duke1616/etask/internal/grpc/scripts/engine"
	"github.com/Duke1616/etask/internal/grpc/scripts/language"
	"github.com/Duke1616/etask/internal/grpc/scripts/language/ansible/connection"
	"github.com/Duke1616/etask/sdk/executor"
	"github.com/samber/lo"
)

var variableNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Adapter 构造 ansible-playbook 命令并准备 extra vars 文件。
type Adapter struct {
	binary      string
	connections connection.Preparer
}

// Option 配置 Ansible Adapter 的 Agent 本地能力。
type Option func(*Adapter)

// WithSSHConnectionPreparer 为 Adapter 配置 SSH 连接准备服务。
func WithSSHConnectionPreparer(preparer connection.Preparer) Option {
	return func(adapter *Adapter) { adapter.connections = preparer }
}

// New 创建 Ansible 语言适配器。
func New(binary string, options ...Option) *Adapter {
	if strings.TrimSpace(binary) == "" {
		binary = "ansible-playbook"
	}
	adapter := &Adapter{binary: binary}
	for _, option := range options {
		option(adapter)
	}
	return adapter
}

// Name 返回处理器名称。
func (a *Adapter) Name() string { return "ansible" }

// Description 返回处理器功能描述。
func (a *Adapter) Description() string {
	return "从完整代码项目执行 Ansible Playbook"
}

// ProgramKinds 返回 Ansible 支持的程序来源类型。
func (a *Adapter) ProgramKinds() []executor.ProgramKind {
	return []executor.ProgramKind{executor.ProgramKindProject}
}

// Extension 返回 Playbook 默认扩展名。
func (a *Adapter) Extension() string { return ".yml" }

// Metadata 返回 Ansible Handler 支持的参数元数据。
func (a *Adapter) Metadata() []executor.Parameter {
	return []executor.Parameter{
		ansibleParameter("inventory", "主机清单", "", "project-file-picker", "选择项目内文件", nil),
		ansibleParameter("credential_ref", "默认 SSH 凭据", "", "select-input", "inventory 未指定时使用", map[string]string{
			"options": credentialOptions(a.connections),
		}),
		ansibleParameter("limit", "执行范围", "", "input", "例如 web:&staging", nil),
		ansibleParameter("tags", "执行标签", "", "input", "例如 deploy,restart", nil),
		{
			Key: "vars", Role: executor.ParameterRoleVariables, Desc: "剧本变量", Default: "[]",
			Bindings: map[string]executor.Binding{
				"manual": &executor.BindingOption{
					Label: "手动配置", Placeholder: `[{"key":"environment","value":"staging","secret":false}]`,
					Component: "kv-input",
				},
				"runner": &executor.BindingOption{
					Label: "执行单元变量", Placeholder: "请选择执行单元...", Component: "runner-picker",
				},
			},
		},
		ansibleParameter("skip_tags", "排除标签", "", "input", "例如 database,download", nil),
		ansibleParameter("check", "预演模式", "false", "boolean-switch", "", nil),
		ansibleParameter("diff", "变更差异", "false", "boolean-switch", "", nil),
		ansibleParameter("become", "权限提升", "false", "boolean-switch", "", nil),
		ansibleParameter("become_user", "提权用户", "", "input", "例如 root", nil),
		ansibleParameter("forks", "并发任务数", "0", "number-input", "使用 Ansible 默认值", map[string]string{
			"min": "0", "max": "100",
		}),
		ansibleParameter("verbosity", "日志级别", "0", "select-input", "", map[string]string{
			"options": `[{"label":"标准日志","value":"0"},{"label":"详细日志 (-v)","value":"1"},{"label":"调试信息 (-vv)","value":"2"},{"label":"连接调试 (-vvv)","value":"3"},{"label":"深度连接调试 (-vvvv)","value":"4"}]`,
		}),
		ansibleParameter("extra_args", "扩展参数", "", "input", `例如 --start-at-task "Deploy application"`, nil),
	}
}

func ansibleParameter(key, desc, defaultValue, component, placeholder string,
	config map[string]string) executor.Parameter {
	return executor.Parameter{
		Key: key, Desc: desc, Default: defaultValue,
		Bindings: map[string]executor.Binding{
			"static": &executor.BindingOption{
				Label: "固定值", Component: component, Placeholder: placeholder, Config: config,
			},
		},
	}
}

// Prepare 创建受控输入文件并构造 ansible-playbook 命令。
func (a *Adapter) Prepare(ctx context.Context, workspace engine.Workspace,
	input engine.Input) (engine.PreparedCommand, error) {
	variablesInput := input.Variables
	// 兼容旧执行快照；新执行通过统一变量字段传入。
	if strings.TrimSpace(variablesInput) == "" {
		variablesInput = input.Params["vars"]
	}
	variables, err := buildExtraVars(variablesInput)
	if err != nil {
		return engine.PreparedCommand{}, err
	}
	options, err := parsePlaybookOptions(input.Params)
	if err != nil {
		return engine.PreparedCommand{}, err
	}
	inventoryFile := ""
	if strings.TrimSpace(options.Inventory) != "" {
		inventoryFile, err = validateInventory(workspace.ProgramRoot(), options.Inventory)
		if err != nil {
			return engine.PreparedCommand{}, err
		}
	}
	usesCredential := false
	var secretMasks []string
	credentialRef := strings.TrimSpace(input.Params["credential_ref"])
	if a.connections != nil {
		connectionVariables, prepareErr := a.connections.Prepare(workspace, connection.Request{
			DefaultReference: credentialRef, InventoryFile: inventoryFile,
		})
		if prepareErr != nil {
			return engine.PreparedCommand{}, prepareErr
		}
		usesCredential = len(connectionVariables.Variables) > 0
		secretMasks = connectionVariables.SecretMasks
		for key, value := range connectionVariables.Variables {
			variables[key] = value
		}
	} else if credentialRef != "" {
		return engine.PreparedCommand{}, fmt.Errorf("Ansible 未配置 SSH 连接准备服务")
	}
	extraVars, err := json.Marshal(variables)
	if err != nil {
		return engine.PreparedCommand{}, fmt.Errorf("序列化 Ansible Extra Vars 失败: %w", err)
	}
	extraVarsFile, err := workspace.WriteFile("ansible-extra-vars.json", extraVars, 0o600)
	if err != nil {
		return engine.PreparedCommand{}, fmt.Errorf("写入 Ansible Extra Vars 失败: %w", err)
	}
	workspaceRoot := filepath.Dir(extraVarsFile)
	environment := []string{
		"ANSIBLE_HOME=" + filepath.Join(workspaceRoot, ".ansible"),
		"ANSIBLE_LOCAL_TEMP=" + filepath.Join(workspaceRoot, ".ansible-tmp"),
		"ANSIBLE_RETRY_FILES_ENABLED=False",
		"ANSIBLE_FORCE_COLOR=True",
	}
	if usesCredential {
		environment = append(environment, "ANSIBLE_HOST_KEY_CHECKING=True")
	}
	commandArgs, err := options.commandArgsWithInventory(inventoryFile)
	if err != nil {
		return engine.PreparedCommand{}, err
	}
	commandArgs = append(commandArgs, "--extra-vars", "@"+extraVarsFile, workspace.EntryPoint())
	command := exec.CommandContext(ctx, a.binary, commandArgs...)
	return engine.PreparedCommand{
		Command: language.ConfigureCancellation(command), Environment: environment, SecretMasks: secretMasks,
	}, nil
}

// Validate 校验本地凭据；ansible-playbook 缺失仍由具体任务返回执行错误。
func (a *Adapter) Validate() error {
	if a.connections != nil {
		return a.connections.Validate()
	}
	return nil
}

func buildExtraVars(raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		raw = "[]"
	}
	var variables []executor.Variable
	if err := json.Unmarshal([]byte(raw), &variables); err != nil {
		return nil, fmt.Errorf("解析 Ansible Extra Vars 失败: 必须是变量数组: %w", err)
	}
	result := make(map[string]any, len(variables))
	for _, variable := range variables {
		key := strings.TrimSpace(variable.Key)
		if !variableNamePattern.MatchString(key) {
			return nil, fmt.Errorf("Ansible Extra Vars 名称非法: %q", variable.Key)
		}
		if connection.ReservedVariable(key) {
			return nil, fmt.Errorf("Ansible 连接凭据必须通过 credential_ref 配置，不能写入变量 %q", key)
		}
		result[key] = variable.Value
	}
	return result, nil
}

func credentialOptions(connections connection.Preparer) string {
	if connections == nil {
		return "[]"
	}
	refs := connections.References()
	type credentialOption struct {
		Label string `json:"label"`
		Value string `json:"value"`
	}
	options := lo.Map(refs, func(ref string, _ int) credentialOption {
		return credentialOption{Label: ref, Value: ref}
	})
	value, err := json.Marshal(options)
	if err != nil {
		return "[]"
	}
	return string(value)
}

var _ engine.Adapter = (*Adapter)(nil)
