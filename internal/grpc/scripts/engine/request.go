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

type resolvedParameters struct {
	semantic map[executor.ParameterRole]string
	extras   map[string]string
}

func resolveRequest(task *executor.Context, adapterName string,
	parameters []executor.Parameter, config Config) (executionRequest, error) {
	program := task.Program()
	if err := validateProgram(program, adapterName, config); err != nil {
		return executionRequest{}, err
	}
	resolved, err := resolveParameters(task, adapterName, parameters, config)
	if err != nil {
		return executionRequest{}, err
	}
	return executionRequest{program: program, input: resolved.input()}, nil
}

func validateProgram(program *executor.Program, adapterName string, config Config) error {
	if program == nil {
		return fmt.Errorf("[%s] 程序来源不能为空", adapterName)
	}
	if program.Kind == executor.ProgramKindInline && int64(len(program.Inline.Code)) > config.MaxCodeSize {
		return fmt.Errorf("代码大小超过限制: %d > %d 字节", len(program.Inline.Code), config.MaxCodeSize)
	}
	return nil
}

func resolveParameters(task *executor.Context, adapterName string,
	parameters []executor.Parameter, config Config) (resolvedParameters, error) {
	resolved := resolvedParameters{
		semantic: make(map[executor.ParameterRole]string),
		extras:   make(map[string]string, len(parameters)),
	}
	for _, parameter := range parameters {
		role, err := parameterRole(parameter)
		if err != nil {
			return resolvedParameters{}, err
		}
		if role != "" {
			if _, exists := resolved.semantic[role]; exists {
				return resolvedParameters{}, fmt.Errorf("[%s] 只能声明一个 %s 语义参数", adapterName, role)
			}
		}
		value, err := resolveParameterValue(task, parameter, role)
		if err != nil {
			return resolvedParameters{}, err
		}
		limit := parameterSizeLimit(role, config)
		if int64(len(value)) > limit {
			return resolvedParameters{}, fmt.Errorf("参数 %s 大小超过限制: %d > %d 字节",
				parameter.Key, len(value), limit)
		}
		if role != "" {
			resolved.semantic[role] = value
		} else {
			resolved.extras[parameter.Key] = value
		}
	}
	return resolved, nil
}

func resolveParameterValue(task *executor.Context, parameter executor.Parameter,
	role executor.ParameterRole) (string, error) {
	// Runner 等上游已经固定变量快照的执行以快照为准；普通 Handler 仍可使用自身参数。
	if role == executor.ParameterRoleVariables && task.HasVariables() {
		encoded, err := json.Marshal(task.Variables())
		if err != nil {
			return "", fmt.Errorf("序列化执行变量失败: %w", err)
		}
		return string(encoded), nil
	}
	if role == executor.ParameterRoleVariables {
		return "[]", nil
	}
	value, err := task.GetResolvedParam(parameter.Key)
	if err != nil {
		return "", fmt.Errorf("解析参数 %s 失败: %w", parameter.Key, err)
	}
	return value, nil
}

func parameterSizeLimit(role executor.ParameterRole, config Config) int64 {
	if role == executor.ParameterRoleVariables {
		return config.MaxVariablesSize
	}
	return config.MaxArgsSize
}

func (r resolvedParameters) input() Input {
	return Input{
		Args:      r.semantic[executor.ParameterRoleArgs],
		Variables: r.semantic[executor.ParameterRoleVariables],
		Params:    r.extras,
	}
}

func parameterRole(parameter executor.Parameter) (executor.ParameterRole, error) {
	role := parameter.Role
	switch role {
	case "":
		return "", nil
	case executor.ParameterRoleArgs:
		return executor.ParameterRoleArgs, nil
	case executor.ParameterRoleVariables:
		return executor.ParameterRoleVariables, nil
	default:
		return "", fmt.Errorf("参数 %s 声明了不支持的语义角色: %s", parameter.Key, role)
	}
}
