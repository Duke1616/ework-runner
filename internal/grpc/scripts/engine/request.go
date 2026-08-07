package engine

import (
	"fmt"

	"github.com/Duke1616/etask/sdk/executor"
)

type executionRequest struct {
	program   *executor.Program
	args      string
	variables string
}

func resolveRequest(task *executor.Context, adapterName string, config Config) (executionRequest, error) {
	program := task.Program()
	if program == nil {
		return executionRequest{}, fmt.Errorf("[%s] 程序来源不能为空", adapterName)
	}
	args, err := task.GetResolvedParam("args")
	if err != nil {
		return executionRequest{}, fmt.Errorf("解析命令参数失败: %w", err)
	}
	variables, err := task.GetResolvedParam("variables")
	if err != nil {
		return executionRequest{}, fmt.Errorf("解析变量参数失败: %w", err)
	}
	checks := []struct {
		name  string
		value string
		limit int64
	}{
		{name: "运行参数", value: args, limit: config.MaxArgsSize},
		{name: "运行变量", value: variables, limit: config.MaxVariablesSize},
	}
	for _, check := range checks {
		if int64(len(check.value)) > check.limit {
			return executionRequest{}, fmt.Errorf("%s大小超过限制: %d > %d 字节", check.name, len(check.value), check.limit)
		}
	}
	if program.Kind == executor.ProgramKindInline {
		if int64(len(program.Inline.Code)) > config.MaxCodeSize {
			return executionRequest{}, fmt.Errorf("代码大小超过限制: %d > %d 字节",
				len(program.Inline.Code), config.MaxCodeSize)
		}
	}
	return executionRequest{program: program, args: args, variables: variables}, nil
}
