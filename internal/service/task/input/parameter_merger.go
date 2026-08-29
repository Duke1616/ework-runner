// Package input 负责组装任务执行阶段使用的最终参数和变量。
package input

import (
	"encoding/json"

	runnerSvc "github.com/Duke1616/etask/internal/service/runner"
	"github.com/samber/lo"
)

// ParameterMergeInput 描述参数的各个来源。
// 来源优先级从低到高依次为 Runner 默认值、任务参数、绑定参数、运行时覆盖。
type ParameterMergeInput struct {
	RunnerDefaults   map[string]json.RawMessage
	TaskParams       map[string]string
	BindingParams    map[string]string
	RuntimeOverrides map[string]string
}

// ParameterMerger 合并执行阶段使用的普通参数。
type ParameterMerger struct{}

// Merge 按 Runner 默认值、任务参数、绑定参数、运行时覆盖的顺序合并参数。
func (ParameterMerger) Merge(input ParameterMergeInput) (map[string]string, error) {
	merged, err := runnerSvc.MergeParameterDefaults(input.RunnerDefaults, input.TaskParams)
	if err != nil {
		return nil, err
	}
	return lo.Assign(merged, input.BindingParams, input.RuntimeOverrides), nil
}
