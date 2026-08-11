package language

import "github.com/Duke1616/etask/sdk/executor"

// Metadata 返回脚本处理器共用的参数定义。
func Metadata() []executor.Parameter {
	return []executor.Parameter{
		ArgsParameter("脚本执行参数"),
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
		Key: "args", Role: executor.ParameterRoleArgs, Desc: desc,
		Bindings: map[string]executor.Binding{
			"static": &executor.BindingOption{
				Label: "参数内容 (JSON)", Placeholder: `{"name": "zhangsan", "age": 18}`,
				Component: "code-editor", Config: map[string]string{"language": "json"},
			},
		},
	}
}
