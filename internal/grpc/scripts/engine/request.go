package engine

import (
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
	for _, parameter := range parameters {
		value, err := task.GetResolvedParam(parameter.Key)
		if err != nil {
			return executionRequest{}, fmt.Errorf("解析参数 %s 失败: %w", parameter.Key, err)
		}
		limit := config.MaxArgsSize
		if parameter.Key == "variables" {
			limit = config.MaxVariablesSize
		}
		if int64(len(value)) > limit {
			return executionRequest{}, fmt.Errorf("参数 %s 大小超过限制: %d > %d 字节",
				parameter.Key, len(value), limit)
		}
		params[parameter.Key] = value
	}
	if program.Kind == executor.ProgramKindInline {
		if int64(len(program.Inline.Code)) > config.MaxCodeSize {
			return executionRequest{}, fmt.Errorf("代码大小超过限制: %d > %d 字节",
				len(program.Inline.Code), config.MaxCodeSize)
		}
	}
	args, variables := params["args"], params["variables"]
	delete(params, "args")
	delete(params, "variables")
	return executionRequest{program: program, input: Input{
		Args: args, Variables: variables, Params: params,
	}}, nil
}
