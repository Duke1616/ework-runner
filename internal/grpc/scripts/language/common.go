package language

import (
	"fmt"

	"github.com/Duke1616/etask/internal/grpc/scripts/engine"
	"github.com/Duke1616/etask/sdk/executor"
)

// FileInput 将参数与变量写入权限受控（0600）的文件，并返回包含绝对路径的统一环境变量。
func FileInput(workspace engine.Workspace, input engine.Input) ([]string, error) {
	args := input.Args
	if args == "" {
		args = "{}"
	}
	variables := input.Variables
	if variables == "" {
		variables = "[]"
	}
	argsFile, err := workspace.WriteFile("args.json", []byte(args), 0o600)
	if err != nil {
		return nil, fmt.Errorf("写入脚本参数文件失败: %w", err)
	}
	variablesFile, err := workspace.WriteFile("variables.json", []byte(variables), 0o600)
	if err != nil {
		return nil, fmt.Errorf("写入脚本变量文件失败: %w", err)
	}
	return []string{
		"ETASK_ARGS_FILE=" + argsFile,
		"ETASK_VARIABLES_FILE=" + variablesFile,
	}, nil
}

// Metadata 返回脚本处理器共用的参数定义。
func Metadata() []executor.Parameter {
	return []executor.Parameter{
		ArgsParameter("执行参数"),
		{
			Key: "variables", Role: executor.ParameterRoleVariables, Desc: "环境变量",
			Bindings: map[string]executor.Binding{
				"static": &executor.BindingOption{
					Label: "手动输入", Placeholder: `[{"key": "K1", "value": "V1", "secret": false}]`,
					Component: "kv-input",
				},
				"runner": &executor.BindingOption{
					Label: "执行单元引用", Placeholder: "请选择执行单元...", Component: "runner-picker",
				},
			},
		},
	}
}

// ArgsParameter 返回各类执行器共用的 JSON 入参定义。
func ArgsParameter(desc string) executor.Parameter {
	return executor.Parameter{
		Key: "args", Role: executor.ParameterRoleArgs, Desc: desc, RuntimeOverridable: true,
		Bindings: map[string]executor.Binding{
			"static": &executor.BindingOption{
				Label: "参数内容 (JSON)", Placeholder: `{"name": "zhangsan", "age": 18}`,
				Component: "code-editor", Config: map[string]string{"language": "json"},
			},
		},
	}
}
