package engine

import (
	"encoding/json"
	"fmt"

	"github.com/Duke1616/etask/sdk/executor"
)

type executionRequest struct {
	program *executor.Program
	input   Input
}

func resolveRequest(task *executor.Context, adapterName string,
	parameters []executor.Parameter, config Config) (executionRequest, error) {
	program := task.Program()
	if program == nil {
		return executionRequest{}, fmt.Errorf("[%s] 程序来源不能为空", adapterName)
	}
	params := make(map[string]string, len(parameters))
	args := ""
	argsResolved := false
	variables := ""
	variablesResolved := false
	for _, parameter := range parameters {
		role := parameterRole(parameter)
		if role == executor.ParameterRoleArgs && argsResolved {
			return executionRequest{}, fmt.Errorf("[%s] 只能声明一个 args 语义参数", adapterName)
		}
		if role == executor.ParameterRoleVariables && variablesResolved {
			return executionRequest{}, fmt.Errorf("[%s] 只能声明一个 variables 语义参数", adapterName)
		}
		value := ""
		var err error
		if role == executor.ParameterRoleVariables && task.HasVariables() && !task.HasParam(parameter.Key) {
			encoded, encodeErr := json.Marshal(task.Variables())
			if encodeErr != nil {
				return executionRequest{}, fmt.Errorf("序列化执行变量失败: %w", encodeErr)
			}
			value = string(encoded)
		} else {
			value, err = task.GetResolvedParam(parameter.Key)
			if err != nil {
				return executionRequest{}, fmt.Errorf("解析参数 %s 失败: %w", parameter.Key, err)
			}
		}
		limit := config.MaxArgsSize
		if role == executor.ParameterRoleVariables {
			limit = config.MaxVariablesSize
		}
		if int64(len(value)) > limit {
			return executionRequest{}, fmt.Errorf("参数 %s 大小超过限制: %d > %d 字节",
				parameter.Key, len(value), limit)
		}
		switch role {
		case executor.ParameterRoleArgs:
			args, argsResolved = value, true
			continue
		case executor.ParameterRoleVariables:
			variables, variablesResolved = value, true
			continue
		}
		params[parameter.Key] = value
	}
	if program.Kind == executor.ProgramKindInline {
		if int64(len(program.Inline.Code)) > config.MaxCodeSize {
			return executionRequest{}, fmt.Errorf("代码大小超过限制: %d > %d 字节",
				len(program.Inline.Code), config.MaxCodeSize)
		}
	}
	return executionRequest{program: program, input: Input{
		Args: args, Variables: variables, Params: params,
	}}, nil
}

func parameterRole(parameter executor.Parameter) executor.ParameterRole {
	if parameter.Role != "" {
		return parameter.Role
	}
	// 固定 Key 仅用于兼容尚未声明 Role 的旧版通用 Handler 元数据。
	switch executor.ParameterRole(parameter.Key) {
	case executor.ParameterRoleArgs:
		return executor.ParameterRoleArgs
	case executor.ParameterRoleVariables:
		return executor.ParameterRoleVariables
	default:
		return ""
	}
}
